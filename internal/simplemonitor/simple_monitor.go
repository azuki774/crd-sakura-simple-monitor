package simplemonitor

import (
	"context"
	"errors"

	monitoringv1alpha1 "github.com/azuki774/crd-sakura-simple-monitor/api/v1alpha1"
)

// ErrSimpleMonitorNotFound indicates that the SakuraCloud simple monitor no longer exists.
var ErrSimpleMonitorNotFound = errors.New("sakura simple monitor not found")

// SimpleMonitorClient synchronizes SakuraCloud simple monitor resources.
type SimpleMonitorClient interface {
	Create(ctx context.Context, desired SimpleMonitorDesired) (string, error)
	Read(ctx context.Context, id string) error
	Update(ctx context.Context, id string, desired SimpleMonitorDesired) error
}

// SimpleMonitorDesired is the SakuraCloud simple monitor shape derived from the Kubernetes resource.
type SimpleMonitorDesired struct {
	Target         string
	Description    string
	Tags           []string
	Protocol       monitoringv1alpha1.HealthCheckProtocol
	Port           int32
	Path           string
	ExpectedStatus int32
	TimeoutSeconds int32
	Interval       int32
	RetryInterval  int32
	WebhookURL     string
	RepeatInterval int32
}
