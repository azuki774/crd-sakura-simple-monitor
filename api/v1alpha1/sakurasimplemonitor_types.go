package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HealthCheckProtocol is the supported protocol for Sakura simple monitor HTTP checks.
type HealthCheckProtocol string

const (
	HealthCheckProtocolHTTP  HealthCheckProtocol = "http"
	HealthCheckProtocolHTTPS HealthCheckProtocol = "https"
)

// HealthCheckSpec defines the monitor probe settings.
type HealthCheckSpec struct {
	// Protocol is limited to HTTP-based checks in the initial implementation.
	// +kubebuilder:validation:Enum=http;https
	Protocol HealthCheckProtocol `json:"protocol"`

	// Port is the destination TCP port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Path is the HTTP request path.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`

	// ExpectedStatus is the expected HTTP response code.
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=599
	ExpectedStatus int32 `json:"expectedStatus"`

	// TimeoutSeconds is the request timeout in seconds.
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds int32 `json:"timeoutSeconds"`

	// HTTP2 enables HTTP/2 for HTTPS checks.
	// +optional
	HTTP2 bool `json:"http2,omitempty"`
}

// NotificationsSpec defines the outbound webhook notification settings.
type NotificationsSpec struct {
	// WebhookURL is the destination webhook URL.
	// +kubebuilder:validation:Format=uri
	// +kubebuilder:validation:MinLength=1
	WebhookURL string `json:"webhookURL"`

	// Message is the fixed alert message body.
	// +kubebuilder:validation:MinLength=1
	Message string `json:"message"`

	// RepeatInterval is the re-notification interval in seconds.
	// +kubebuilder:validation:Minimum=3600
	// +kubebuilder:validation:Maximum=259200
	RepeatInterval int32 `json:"repeatInterval"`
}

// SakuraSimpleMonitorSpec defines the desired state of SakuraSimpleMonitor.
type SakuraSimpleMonitorSpec struct {
	// Target is the hostname or IP address to monitor.
	// +kubebuilder:validation:MinLength=1
	Target string `json:"target"`

	// HealthCheck defines how the controller should probe the target.
	HealthCheck HealthCheckSpec `json:"healthCheck"`

	// Interval is the monitor interval in minutes.
	// +kubebuilder:validation:Minimum=1
	Interval int32 `json:"interval"`

	// RetryInterval is the retry interval in seconds.
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=3600
	RetryInterval int32 `json:"retryInterval"`

	// Notifications defines how alerts are sent.
	Notifications NotificationsSpec `json:"notifications"`

	// Description is an optional operator-facing description.
	Description string `json:"description,omitempty"`
}

// SakuraSimpleMonitorStatus defines the observed state of SakuraSimpleMonitor.
type SakuraSimpleMonitorStatus struct {
	// MonitorID is the SakuraCloud simple monitor resource ID.
	MonitorID string `json:"monitorID,omitempty"`

	// ObservedGeneration is the latest reconciled generation.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions describes the reconciliation state using Kubernetes-standard conditions.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncedAt is the latest successful synchronization timestamp.
	LastSyncedAt *metav1.Time `json:"lastSyncedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=sakurasimplemonitors,scope=Namespaced,singular=sakurasimplemonitor
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.target"
// +kubebuilder:printcolumn:name="MonitorID",type="string",JSONPath=".status.monitorID"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SakuraSimpleMonitor is the Schema for the sakurasimplemonitors API.
type SakuraSimpleMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SakuraSimpleMonitorSpec   `json:"spec"`
	Status SakuraSimpleMonitorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SakuraSimpleMonitorList contains a list of SakuraSimpleMonitor.
type SakuraSimpleMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SakuraSimpleMonitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SakuraSimpleMonitor{}, &SakuraSimpleMonitorList{})
}
