package simplemonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/go-logr/logr"
	iaas "github.com/sacloud/iaas-api-go"
	"github.com/sacloud/iaas-api-go/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const maxSakuraTags = 10

var sakuraTagPattern = regexp.MustCompile(`^[A-Za-z0-9@][A-Za-z0-9._@-]*$`)

// simpleMonitorCreateRequest is the request body for creating a SimpleMonitor.
// It includes the Provider.Class field which is required by the SakuraCloud API.
type simpleMonitorCreateRequest struct {
	CommonServiceItem *simpleMonitorCreateRequestBody `json:"CommonServiceItem"`
}

type simpleMonitorCreateRequestBody struct {
	Name        string                `json:"Name"`
	Description string                `json:"Description,omitempty"`
	Status      simpleMonitorStatus   `json:"Status,omitempty"`
	Settings    simpleMonitorSettings `json:"Settings"`
	Provider    simpleMonitorProvider `json:"Provider"`
	Tags        types.Tags            `json:"Tags,omitempty"`
	Icon        *types.ID             `json:"Icon,omitempty"`
}

type simpleMonitorStatus struct {
	Target string `json:"Target"`
}

type simpleMonitorSettings struct {
	SimpleMonitor simpleMonitorConfig `json:"SimpleMonitor"`
}

type simpleMonitorConfig struct {
	DelayLoop        int                      `json:"DelayLoop"`
	MaxCheckAttempts int                      `json:"MaxCheckAttempts"`
	RetryInterval    int                      `json:"RetryInterval"`
	Enabled          types.StringFlag         `json:"Enabled"`
	Timeout          int                      `json:"Timeout"`
	HealthCheck      simpleMonitorHealthCheck `json:"HealthCheck"`
	NotifyEmail      simpleMonitorNotifyEmail `json:"NotifyEmail"`
	NotifySlack      simpleMonitorNotifySlack `json:"NotifySlack"`
	NotifyInterval   int                      `json:"NotifyInterval"`
}

type simpleMonitorHealthCheck struct {
	Protocol types.ESimpleMonitorProtocol `json:"Protocol"`
	Host     string                       `json:"Host"`
	Path     string                       `json:"Path,omitempty"`
	Port     types.StringNumber           `json:"Port,omitempty"`
	Status   types.StringNumber           `json:"Status,omitempty"`
	SNI      types.StringFlag             `json:"SNI,omitempty"`
	HTTP2    types.StringFlag             `json:"HTTP2,omitempty"`
}

type simpleMonitorNotifyEmail struct {
	Enabled types.StringFlag `json:"Enabled"`
	HTML    types.StringFlag `json:"HTML,omitempty"`
}

type simpleMonitorNotifySlack struct {
	Enabled             types.StringFlag `json:"Enabled"`
	IncomingWebhooksURL string           `json:"IncomingWebhooksURL,omitempty"`
}

type simpleMonitorProvider struct {
	Class string `json:"Class"`
}

// simpleMonitorResponse is the API response for SimpleMonitor operations.
type simpleMonitorResponse struct {
	CommonServiceItem *simpleMonitorResponseBody `json:"CommonServiceItem"`
	IsOK              bool                       `json:"is_ok"`
}

type simpleMonitorResponseBody struct {
	ID          string `json:"ID"`
	Name        string `json:"Name"`
	Description string `json:"Description,omitempty"`
}

// Client calls SakuraCloud SimpleMonitor API via HTTP.
type Client struct {
	op           iaas.SimpleMonitorAPI
	sakuraCaller iaas.APICaller
}

// NewClient creates a SimpleMonitor client from an API caller.
func NewClient(caller iaas.APICaller) *Client {
	return &Client{
		op:           iaas.NewSimpleMonitorOp(caller),
		sakuraCaller: caller,
	}
}

func (c *Client) Create(ctx context.Context, desired SimpleMonitorDesired) (string, error) {
	if err := desired.validateSakuraRequestShape(); err != nil {
		return "", err
	}
	logger := log.FromContext(ctx).WithName("sakura-simple-monitor")
	logger.Info("creating SakuraCloud simple monitor", "target", desired.Target, "tags", desired.Tags)

	reqBody := desired.toCreateRequestBody()
	data, err := c.sakuraCaller.Do(ctx, "POST", "/cloud/zone/is1a/api/cloud/1.1/commonserviceitem", reqBody)
	if err != nil {
		logSakuraAPIError(logger, "create", err)
		return "", err
	}

	var resp simpleMonitorResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		logger.Error(err, "failed to parse response", "response", string(data))
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !resp.IsOK || resp.CommonServiceItem == nil {
		return "", fmt.Errorf("API returned error: is_ok=false, response=%s", string(data))
	}

	logger.Info("created SakuraCloud simple monitor", "monitorID", resp.CommonServiceItem.ID, "target", desired.Target)
	return resp.CommonServiceItem.ID, nil
}

func (c *Client) Read(ctx context.Context, id string) error {
	logger := log.FromContext(ctx).WithName("sakura-simple-monitor")
	logger.Info("reading SakuraCloud simple monitor", "monitorID", id)
	_, err := c.op.Read(ctx, types.StringID(id))
	if iaas.IsNotFoundError(err) {
		logger.Info("SakuraCloud simple monitor was not found", "monitorID", id)
		return ErrSimpleMonitorNotFound
	}
	if err != nil {
		logSakuraAPIError(logger, "read", err)
		return err
	}
	logger.Info("read SakuraCloud simple monitor", "monitorID", id)
	return err
}

func logSakuraAPIError(logger logr.Logger, operation string, err error) {
	var apiErr iaas.APIError
	if !errors.As(err, &apiErr) {
		return
	}

	responseBody := ""
	apiStatus := ""
	if origErr := apiErr.OrigErr(); origErr != nil {
		apiStatus = origErr.Status
		body, marshalErr := json.Marshal(origErr)
		if marshalErr == nil {
			responseBody = string(body)
		}
	}

	logger.Error(
		err,
		"SakuraCloud API returned an error",
		"operation", operation,
		"responseCode", apiErr.ResponseCode(),
		"apiStatus", apiStatus,
		"serial", apiErr.Serial(),
		"errorCode", apiErr.Code(),
		"errorMessage", apiErr.Message(),
		"responseBody", responseBody,
	)
}

func (c *Client) Update(ctx context.Context, id string, desired SimpleMonitorDesired) error {
	if err := desired.validateSakuraRequestShape(); err != nil {
		return err
	}
	logger := log.FromContext(ctx).WithName("sakura-simple-monitor")
	logger.Info("updating SakuraCloud simple monitor", "monitorID", id, "target", desired.Target, "tags", desired.Tags)
	_, err := c.op.Update(ctx, types.StringID(id), desired.toUpdateRequest())
	if iaas.IsNotFoundError(err) {
		logger.Info("SakuraCloud simple monitor was not found during update", "monitorID", id)
		return ErrSimpleMonitorNotFound
	}
	if err != nil {
		logSakuraAPIError(logger, "update", err)
		return err
	}
	logger.Info("updated SakuraCloud simple monitor", "monitorID", id, "target", desired.Target)
	return err
}

func (d SimpleMonitorDesired) toCreateRequestBody() *simpleMonitorCreateRequest {
	return &simpleMonitorCreateRequest{
		CommonServiceItem: &simpleMonitorCreateRequestBody{
			Name:        d.Target,
			Description: d.Description,
			Status:      simpleMonitorStatus{Target: d.Target},
			Settings: simpleMonitorSettings{
				SimpleMonitor: simpleMonitorConfig{
					DelayLoop:        int(d.Interval) * 60,
					MaxCheckAttempts: 1,
					RetryInterval:    int(d.RetryInterval),
					Enabled:          types.StringTrue,
					Timeout:          int(d.TimeoutSeconds),
					HealthCheck:      d.toHealthCheck(),
					NotifyEmail:      simpleMonitorNotifyEmail{Enabled: types.StringFalse},
					NotifySlack:      simpleMonitorNotifySlack{Enabled: types.StringTrue, IncomingWebhooksURL: d.WebhookURL},
					NotifyInterval:   int(d.RepeatInterval),
				},
			},
			Provider: simpleMonitorProvider{Class: "simplemon"},
			Tags:     types.Tags(d.Tags),
		},
	}
}

func (d SimpleMonitorDesired) toUpdateRequest() *iaas.SimpleMonitorUpdateRequest {
	return &iaas.SimpleMonitorUpdateRequest{
		Description:        d.Description,
		Tags:               types.Tags(d.Tags),
		MaxCheckAttempts:   1,
		RetryInterval:      int(d.RetryInterval),
		DelayLoop:          int(d.Interval) * 60,
		Enabled:            types.StringTrue,
		HealthCheck:        d.toIAASHealthCheck(),
		NotifyEmailEnabled: types.StringFalse,
		NotifyEmailHTML:    types.StringFalse,
		NotifySlackEnabled: types.StringTrue,
		SlackWebhooksURL:   d.WebhookURL,
		NotifyInterval:     int(d.RepeatInterval),
		Timeout:            int(d.TimeoutSeconds),
	}
}

func (d SimpleMonitorDesired) toHealthCheck() simpleMonitorHealthCheck {
	return simpleMonitorHealthCheck{
		Protocol: types.ESimpleMonitorProtocol(d.Protocol),
		Port:     types.StringNumber(d.Port),
		Path:     d.Path,
		Status:   types.StringNumber(d.ExpectedStatus),
		SNI:      types.StringTrue,
		Host:     d.Target,
	}
}

func (d SimpleMonitorDesired) toIAASHealthCheck() *iaas.SimpleMonitorHealthCheck {
	return &iaas.SimpleMonitorHealthCheck{
		Protocol: types.ESimpleMonitorProtocol(d.Protocol),
		Port:     types.StringNumber(d.Port),
		Path:     d.Path,
		Status:   types.StringNumber(d.ExpectedStatus),
		SNI:      types.StringTrue,
		Host:     d.Target,
	}
}

func (d SimpleMonitorDesired) validateSakuraRequestShape() error {
	if len(d.Tags) > maxSakuraTags {
		return fmt.Errorf("tags must have at most %d items", maxSakuraTags)
	}
	for _, tag := range d.Tags {
		if tag == "" {
			return fmt.Errorf("tags must not contain an empty item")
		}
		if !sakuraTagPattern.MatchString(tag) {
			return fmt.Errorf("tag %q must match SakuraCloud tags format", tag)
		}
	}
	if d.Interval < 1 {
		return fmt.Errorf("interval must be greater than or equal to 1")
	}
	if d.RetryInterval < 10 || d.RetryInterval > 3600 {
		return fmt.Errorf("retry interval must be between 10 and 3600 seconds")
	}
	if d.RepeatInterval < 3600 || d.RepeatInterval > 259200 {
		return fmt.Errorf("repeat interval must be between 3600 and 259200 seconds")
	}
	return nil
}
