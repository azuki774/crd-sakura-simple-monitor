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
	client "github.com/sacloud/api-client-go"
	iaas "github.com/sacloud/iaas-api-go"
)

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
	if opts.UserAgent == "" {
		opts.UserAgent = iaas.DefaultUserAgent
	}
	accessLogger := namedLogger(log)
	opts.CheckRetryFunc = noRetryCheckRetryFunc
	opts.CheckRetryStatusCodes = nil
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

func noRetryCheckRetryFunc(ctx context.Context, _ *http.Response, _ error) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return false, nil
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
