package simplemonitor

import (
	"strings"
	"testing"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
)

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
