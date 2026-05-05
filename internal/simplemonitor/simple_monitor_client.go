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

// Client adapts iaas-api-go to the controller-facing client interface.
type Client struct {
	op iaas.SimpleMonitorAPI
}

// NewClient creates a SimpleMonitor client from an API caller.
func NewClient(caller iaas.APICaller) *Client {
	return &Client{op: iaas.NewSimpleMonitorOp(caller)}
}

func (c *Client) Create(ctx context.Context, desired SimpleMonitorDesired) (string, error) {
	if err := desired.validateSakuraRequestShape(); err != nil {
		return "", err
	}
	logger := log.FromContext(ctx).WithName("sakura-simple-monitor")
	logger.Info("creating SakuraCloud simple monitor", "target", desired.Target, "tags", desired.Tags)

	req := desired.toCreateRequest()
	created, err := c.op.Create(ctx, req)
	if err != nil {
		logSakuraAPIError(logger, "create", err)
		return "", err
	}
	logger.Info("created SakuraCloud simple monitor", "monitorID", created.ID.String(), "target", desired.Target)
	return created.ID.String(), nil
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

func (d SimpleMonitorDesired) toCreateRequest() *iaas.SimpleMonitorCreateRequest {
	return &iaas.SimpleMonitorCreateRequest{
		Target:             d.Target,
		Description:        d.Description,
		Tags:               types.Tags(d.Tags),
		MaxCheckAttempts:   1,
		RetryInterval:      int(d.RetryInterval),
		DelayLoop:          int(d.Interval) * 60,
		Enabled:            types.StringTrue,
		HealthCheck:        d.toHealthCheck(),
		NotifyEmailEnabled: types.StringFalse,
		NotifyEmailHTML:    types.StringFalse,
		NotifySlackEnabled: types.StringTrue,
		SlackWebhooksURL:   d.WebhookURL,
		NotifyInterval:     int(d.RepeatInterval),
		Timeout:            int(d.TimeoutSeconds),
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
		HealthCheck:        d.toHealthCheck(),
		NotifyEmailEnabled: types.StringFalse,
		NotifyEmailHTML:    types.StringFalse,
		NotifySlackEnabled: types.StringTrue,
		SlackWebhooksURL:   d.WebhookURL,
		NotifyInterval:     int(d.RepeatInterval),
		Timeout:            int(d.TimeoutSeconds),
	}
}

func (d SimpleMonitorDesired) toHealthCheck() *iaas.SimpleMonitorHealthCheck {
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
