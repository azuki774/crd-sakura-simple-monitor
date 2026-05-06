package accesslog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/azuki774/crd-sakura-simple-monitor/internal/logger"
	"github.com/hashicorp/go-retryablehttp"
	client "github.com/sacloud/api-client-go"
	iaas "github.com/sacloud/iaas-api-go"
)

const maxRetryResponseBodyLogBytes = 4096

// SakuraAPICaller records SakuraCloud API access logs without request or response bodies.
type SakuraAPICaller struct {
	caller  iaas.APICaller
	factory *client.Factory
	logger  logger.Logger
	now     func() time.Time
}

// NewSakuraAPICaller wraps an iaas APICaller with structured access logging.
func NewSakuraAPICaller(caller iaas.APICaller, log logger.Logger) *SakuraAPICaller {
	return &SakuraAPICaller{
		caller: caller,
		logger: namedLogger(log),
		now:    time.Now,
	}
}

// NewSakuraAPICallerFromEnv creates a SakuraCloud API caller with access logs from environment configuration.
func NewSakuraAPICallerFromEnv(log logger.Logger) *SakuraAPICaller {
	return NewSakuraAPICallerWithOptions(client.OptionsFromEnv(), log)
}

// NewSakuraAPICallerWithOptions creates a SakuraCloud API caller with access logs from explicit options.
func NewSakuraAPICallerWithOptions(opts *client.Options, log logger.Logger) *SakuraAPICaller {
	if len(opts.CheckRetryStatusCodes) == 0 {
		opts.CheckRetryStatusCodes = []int{
			http.StatusServiceUnavailable,
			http.StatusLocked,
		}
	}
	if opts.UserAgent == "" {
		opts.UserAgent = iaas.DefaultUserAgent
	}
	accessLogger := namedLogger(log)
	opts.CheckRetryFunc = retryLoggingCheckRetryFunc(opts.CheckRetryFunc, opts.CheckRetryStatusCodes, accessLogger)
	return &SakuraAPICaller{
		factory: client.NewFactory(opts),
		logger:  accessLogger,
		now:     time.Now,
	}
}

func namedLogger(log logger.Logger) logger.Logger {
	if log == nil {
		log = logger.NewSlogLogger(nil)
	}
	return log.WithName("sakura-api")
}

func (c *SakuraAPICaller) Do(ctx context.Context, method, uri string, body interface{}) ([]byte, error) {
	if c.factory != nil {
		return c.doWithFactory(ctx, method, uri, body)
	}
	return c.doWithCaller(ctx, method, uri, body)
}

func (c *SakuraAPICaller) doWithCaller(ctx context.Context, method, uri string, body interface{}) ([]byte, error) {
	start := c.now()
	data, err := c.caller.Do(ctx, method, uri, body)
	duration := c.now().Sub(start)

	if err != nil {
		c.logFailure(ctx, method, uri, duration, err)
		return nil, err
	}

	c.logSuccess(ctx, method, uri, duration, 0, len(data))
	return data, nil
}

func (c *SakuraAPICaller) doWithFactory(ctx context.Context, method, uri string, body interface{}) ([]byte, error) {
	req, err := newSakuraAPIRequest(ctx, method, uri, body)
	if err != nil {
		return nil, err
	}

	start := c.now()
	resp, err := c.factory.NewHttpRequestDoer().Do(req)
	duration := c.now().Sub(start)
	if err != nil {
		c.logFailure(ctx, method, uri, duration, err)
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logFailure(ctx, method, uri, duration, err, "statusCode", resp.StatusCode)
		return nil, err
	}

	if !isOKSakuraAPIStatus(resp.StatusCode) {
		errResponse := &iaas.APIErrorResponse{}
		if err := json.Unmarshal(data, errResponse); err != nil {
			apiErr := fmt.Errorf("error in response: %s", string(data))
			c.logFailure(ctx, method, uri, duration, apiErr, "statusCode", resp.StatusCode)
			return nil, apiErr
		}
		apiErr := iaas.NewAPIError(req.Method, req.URL, resp.StatusCode, errResponse)
		c.logFailure(ctx, method, uri, duration, apiErr)
		return nil, apiErr
	}

	c.logSuccess(ctx, method, uri, duration, resp.StatusCode, len(data))
	return data, nil
}

func newSakuraAPIRequest(ctx context.Context, method, uri string, body interface{}) (*http.Request, error) {
	requestURI := uri
	var bodyReader io.ReadSeeker
	if body != nil {
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		if method == http.MethodGet {
			requestURI = fmt.Sprintf("%s?%s", requestURI, bytes.NewBuffer(bodyJSON))
		} else {
			bodyReader = bytes.NewReader(bodyJSON)
		}
	}
	return http.NewRequestWithContext(ctx, method, requestURI, bodyReader)
}

func (c *SakuraAPICaller) logSuccess(ctx context.Context, method, uri string, duration time.Duration, statusCode, responseBytes int) {
	values := accessLogValues(method, uri, duration)
	if statusCode > 0 {
		values = append(values, "statusCode", statusCode)
	}
	values = append(values, "responseBytes", responseBytes)
	c.logger.Info(ctx, "SakuraCloud API access succeeded", values...)
}

func (c *SakuraAPICaller) logFailure(ctx context.Context, method, uri string, duration time.Duration, err error, extraValues ...interface{}) {
	values := accessLogValues(method, uri, duration)
	values = append(values, extraValues...)
	if apiErr, ok := err.(iaas.APIError); ok {
		values = append(values,
			"statusCode", apiErr.ResponseCode(),
			"errorCode", apiErr.Code(),
			"serial", apiErr.Serial(),
		)
	}
	c.logger.Error(ctx, err, "SakuraCloud API access failed", values...)
}

func retryLoggingCheckRetryFunc(
	base func(context.Context, *http.Response, error) (bool, error),
	retryStatusCodes []int,
	log logger.Logger,
) func(context.Context, *http.Response, error) (bool, error) {
	return func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		shouldRetry, retryErr := shouldRetrySakuraAPIRequest(ctx, resp, err, base, retryStatusCodes)
		if shouldRetry {
			logRetryableResponse(ctx, log, resp, err, retryErr)
		}
		return shouldRetry, retryErr
	}
}

func shouldRetrySakuraAPIRequest(
	ctx context.Context,
	resp *http.Response,
	err error,
	base func(context.Context, *http.Response, error) (bool, error),
	retryStatusCodes []int,
) (bool, error) {
	if base != nil {
		return base(ctx, resp, err)
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}
	if resp == nil {
		return false, nil
	}
	for _, status := range retryStatusCodes {
		if resp.StatusCode == status {
			return true, nil
		}
	}
	return false, nil
}

func logRetryableResponse(ctx context.Context, log logger.Logger, resp *http.Response, err, retryErr error) {
	values := []interface{}{}
	if resp != nil {
		method := ""
		uri := ""
		if resp.Request != nil {
			method = resp.Request.Method
			if resp.Request.URL != nil {
				uri = sanitizeAccessLogURI(resp.Request.URL.String())
			}
		}
		values = append(values,
			"method", method,
			"uri", uri,
			"statusCode", resp.StatusCode,
		)
		if requestValues, readErr := sanitizedRetryRequestValues(resp.Request); readErr != nil {
			values = append(values, "requestSummaryReadError", readErr.Error())
		} else {
			values = append(values, requestValues...)
		}

		body, truncated, readErr := readAndRestoreResponseBody(resp, maxRetryResponseBodyLogBytes)
		if readErr != nil {
			values = append(values, "responseBodyReadError", readErr.Error())
		} else {
			values = append(values, "responseBody", body, "responseBodyTruncated", truncated)
		}
	}
	if retryErr != nil {
		values = append(values, "retryError", retryErr.Error())
	}
	if err != nil {
		log.Error(ctx, err, "SakuraCloud API retrying after transport error", values...)
		return
	}
	log.Info(ctx, "SakuraCloud API retrying after response", values...)
}

func sanitizedRetryRequestValues(req *http.Request) ([]interface{}, error) {
	if req == nil || req.GetBody == nil {
		return nil, nil
	}
	bodyReader, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	defer bodyReader.Close() //nolint:errcheck

	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("request body is not JSON: %w", err)
	}
	return sakuraCommonServiceItemRequestSummary(payload), nil
}

func sakuraCommonServiceItemRequestSummary(payload map[string]interface{}) []interface{} {
	item := nestedMap(payload, "CommonServiceItem")
	if item == nil {
		return nil
	}

	status := nestedMap(item, "Status")
	provider := nestedMap(item, "Provider")
	settings := nestedMap(item, "Settings")
	simpleMonitor := nestedMap(settings, "SimpleMonitor")
	healthCheck := nestedMap(simpleMonitor, "HealthCheck")
	notifyEmail := nestedMap(simpleMonitor, "NotifyEmail")
	notifySlack := nestedMap(simpleMonitor, "NotifySlack")

	return []interface{}{
		"requestProviderClass", provider["Class"],
		"requestName", item["Name"],
		"requestStatusTarget", status["Target"],
		"requestDelayLoop", simpleMonitor["DelayLoop"],
		"requestRetryInterval", simpleMonitor["RetryInterval"],
		"requestMaxCheckAttempts", simpleMonitor["MaxCheckAttempts"],
		"requestEnabled", simpleMonitor["Enabled"],
		"requestTimeout", simpleMonitor["Timeout"],
		"requestNotifyInterval", simpleMonitor["NotifyInterval"],
		"requestNotifyEmailEnabled", notifyEmail["Enabled"],
		"requestNotifySlackEnabled", notifySlack["Enabled"],
		"requestSlackWebhookURLConfigured", notifySlack["IncomingWebhooksURL"] != nil && notifySlack["IncomingWebhooksURL"] != "",
		"requestHealthCheckProtocol", healthCheck["Protocol"],
		"requestHealthCheckHost", healthCheck["Host"],
		"requestHealthCheckPort", healthCheck["Port"],
		"requestHealthCheckPath", healthCheck["Path"],
		"requestHealthCheckStatus", healthCheck["Status"],
		"requestHealthCheckSNI", healthCheck["SNI"],
		"requestHealthCheckHTTP2", healthCheck["HTTP2"],
		"requestHealthCheckVerifySNI", healthCheck["VerifySNI"],
	}
}

func nestedMap(parent map[string]interface{}, key string) map[string]interface{} {
	if parent == nil {
		return map[string]interface{}{}
	}
	child, ok := parent[key].(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return child
}

func readAndRestoreResponseBody(resp *http.Response, limit int64) (string, bool, error) {
	if resp.Body == nil {
		return "", false, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	closeErr := resp.Body.Close()
	if err != nil {
		return "", false, err
	}
	if closeErr != nil {
		return "", false, closeErr
	}
	truncated := int64(len(body)) > limit
	if truncated {
		body = body[:limit]
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return string(body), truncated, nil
}

func accessLogValues(method, uri string, duration time.Duration) []interface{} {
	return []interface{}{
		"method", method,
		"uri", sanitizeAccessLogURI(uri),
		"duration", duration.String(),
	}
}

func isOKSakuraAPIStatus(code int) bool {
	switch code {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		return true
	default:
		return false
	}
}

func (c *SakuraAPICaller) setNowForTest(now func() time.Time) {
	c.now = now
}

func sanitizeAccessLogURI(rawURI string) string {
	parsed, err := url.Parse(rawURI)
	if err != nil {
		return rawURI
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

var _ iaas.APICaller = (*SakuraAPICaller)(nil)
