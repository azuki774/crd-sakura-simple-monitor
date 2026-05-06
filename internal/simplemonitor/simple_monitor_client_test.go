package simplemonitor

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
	iaas "github.com/sacloud/iaas-api-go"
	"github.com/sacloud/iaas-api-go/types"
)

func TestClientCreateUsesSDKCommonServiceItemEndpointAndBody(t *testing.T) {
	caller := &recordingAPICaller{
		response: []byte(`{"CommonServiceItem":{"ID":"123456789012"},"is_ok":true}`),
	}
	client := NewClient(caller)
	desired := validSimpleMonitorDesired()
	desired.Target = "192.0.2.100"
	desired.Protocol = monitoringv1alpha1.HealthCheckProtocolHTTP
	desired.Port = 80
	desired.Path = "/"
	desired.ExpectedStatus = 200
	desired.Interval = 1
	desired.TimeoutSeconds = 10
	desired.HTTP2 = true
	desired.RetryInterval = 20
	desired.RepeatInterval = 7200
	desired.Tags = []string{}

	monitorID, err := client.Create(context.Background(), desired)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if monitorID != "123456789012" {
		t.Fatalf("Create() monitorID = %q, want %q", monitorID, "123456789012")
	}
	if caller.method != http.MethodPost {
		t.Fatalf("Create() method = %q, want %q", caller.method, http.MethodPost)
	}
	wantURI := iaas.SakuraCloudAPIRoot + "/is1a/api/cloud/1.1/commonserviceitem"
	if caller.uri != wantURI {
		t.Fatalf("Create() uri = %q, want %q", caller.uri, wantURI)
	}
	if strings.Contains(caller.uri, "/cloud/zone/cloud/zone/") {
		t.Fatalf("Create() uri contains duplicated zone path: %q", caller.uri)
	}

	bodyJSON, err := json.Marshal(caller.body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(bodyJSON, &body); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}
	item := body["CommonServiceItem"].(map[string]interface{})
	settings := item["Settings"].(map[string]interface{})
	simpleMon := settings["SimpleMonitor"].(map[string]interface{})
	healthCheck := simpleMon["HealthCheck"].(map[string]interface{})
	notifyEmail := simpleMon["NotifyEmail"].(map[string]interface{})
	notifySlack := simpleMon["NotifySlack"].(map[string]interface{})
	status := item["Status"].(map[string]interface{})
	provider := item["Provider"].(map[string]interface{})

	assertJSONValue(t, item, "Name", "192.0.2.100")
	assertJSONValue(t, status, "Target", "192.0.2.100")
	assertJSONValue(t, provider, "Class", "simplemon")
	assertJSONValue(t, simpleMon, "DelayLoop", float64(60))
	assertJSONValue(t, simpleMon, "Enabled", "True")
	assertJSONValue(t, simpleMon, "Timeout", float64(10))
	assertJSONValue(t, simpleMon, "RetryInterval", float64(20))
	assertJSONValue(t, simpleMon, "NotifyInterval", float64(7200))
	assertJSONValue(t, healthCheck, "Protocol", "http")
	assertJSONValue(t, healthCheck, "Path", "/")
	assertJSONValue(t, healthCheck, "Status", "200")
	assertJSONValue(t, healthCheck, "HTTP2", "True")
	assertJSONValue(t, notifyEmail, "Enabled", "False")
	assertJSONValue(t, notifySlack, "Enabled", "True")
	if _, ok := item["Tags"].([]interface{}); !ok {
		t.Fatalf("CommonServiceItem.Tags = %#v, want array", item["Tags"])
	}
	if _, ok := item["Icon"].(map[string]interface{}); !ok {
		t.Fatalf("CommonServiceItem.Icon = %#v, want object", item["Icon"])
	}
}

func TestSimpleMonitorDesiredValidateSakuraRequestShapeTags(t *testing.T) {
	tests := []struct {
		name    string
		tags    []string
		wantErr string
	}{
		{
			name: "accepts generated tags",
			tags: []string{
				"managed-by-crd-sakura-simple-monitor",
				"k8s-kind-sakurasimplemonitor",
				"k8s-namespace-monitoring",
				"k8s-name-nostr-dev",
				"k8s-resource-monitoring-nostr-dev",
				"k8s-uid-12345678-1234-1234-1234-123456789012",
			},
		},
		{
			name:    "rejects slash",
			tags:    []string{"k8s-resource-monitoring/nostr-dev"},
			wantErr: "must match SakuraCloud tags format",
		},
		{
			name:    "rejects equals",
			tags:    []string{"k8s-name=nostr-dev"},
			wantErr: "must match SakuraCloud tags format",
		},
		{
			name:    "rejects empty tag",
			tags:    []string{""},
			wantErr: "tags must not contain an empty item",
		},
		{
			name:    "rejects more than ten tags",
			tags:    []string{"tag01", "tag02", "tag03", "tag04", "tag05", "tag06", "tag07", "tag08", "tag09", "tag10", "tag11"},
			wantErr: "tags must have at most 10 items",
		},
		{
			name:    "rejects whitespace",
			tags:    []string{"k8s-name-nostr dev"},
			wantErr: "must match SakuraCloud tags format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := validSimpleMonitorDesired()
			desired.Tags = tt.tags

			err := desired.validateSakuraRequestShape()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSakuraRequestShape() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateSakuraRequestShape() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateSakuraRequestShape() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestClientHealthStatusReadsEndpointAndMapsHealth(t *testing.T) {
	caller := &recordingAPICaller{
		response: []byte(`{"SimpleMonitor":{"Health":"UP"},"is_ok":true}`),
	}
	client := NewClient(caller)

	health, err := client.HealthStatus(context.Background(), "123456789012")
	if err != nil {
		t.Fatalf("HealthStatus() error = %v", err)
	}
	if health != monitoringv1alpha1.HealthStatusHealthy {
		t.Fatalf("HealthStatus() = %q, want %q", health, monitoringv1alpha1.HealthStatusHealthy)
	}
	if caller.method != http.MethodGet {
		t.Fatalf("HealthStatus() method = %q, want %q", caller.method, http.MethodGet)
	}
	wantURI := iaas.SakuraCloudAPIRoot + "/is1a/api/cloud/1.1/commonserviceitem/123456789012/health"
	if caller.uri != wantURI {
		t.Fatalf("HealthStatus() uri = %q, want %q", caller.uri, wantURI)
	}
	if caller.body != nil {
		t.Fatalf("HealthStatus() body = %#v, want nil", caller.body)
	}
}

func TestMapSakuraHealthStatus(t *testing.T) {
	tests := []struct {
		name   string
		health types.ESimpleMonitorHealth
		want   monitoringv1alpha1.HealthStatus
	}{
		{
			name:   "maps up to healthy",
			health: types.SimpleMonitorHealth.Up,
			want:   monitoringv1alpha1.HealthStatusHealthy,
		},
		{
			name:   "maps down to not healthy",
			health: types.SimpleMonitorHealth.Down,
			want:   monitoringv1alpha1.HealthStatusNotHealthy,
		},
		{
			name:   "maps empty to unknown",
			health: "",
			want:   monitoringv1alpha1.HealthStatusUnknown,
		},
		{
			name:   "maps unexpected value to unknown",
			health: types.ESimpleMonitorHealth("MAINTENANCE"),
			want:   monitoringv1alpha1.HealthStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapSakuraHealthStatus(tt.health); got != tt.want {
				t.Fatalf("mapSakuraHealthStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

type recordingAPICaller struct {
	method   string
	uri      string
	body     interface{}
	response []byte
	err      error
}

func (c *recordingAPICaller) Do(_ context.Context, method, uri string, body interface{}) ([]byte, error) {
	c.method = method
	c.uri = uri
	c.body = body
	return c.response, c.err
}

func assertJSONValue(t *testing.T, values map[string]interface{}, key string, want interface{}) {
	t.Helper()
	if got := values[key]; got != want {
		t.Fatalf("%s = %#v, want %#v", key, got, want)
	}
}

func validSimpleMonitorDesired() SimpleMonitorDesired {
	return SimpleMonitorDesired{
		Target:         "example.com",
		Description:    "test monitor",
		Protocol:       monitoringv1alpha1.HealthCheckProtocolHTTPS,
		Port:           443,
		Path:           "/healthz",
		ExpectedStatus: 200,
		TimeoutSeconds: 10,
		Interval:       1,
		RetryInterval:  20,
		WebhookURL:     "https://example.com/webhook",
		RepeatInterval: 7200,
	}
}
