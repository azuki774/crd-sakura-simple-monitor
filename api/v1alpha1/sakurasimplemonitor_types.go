/*
Copyright 2026 azuki774.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

// HealthStatus is the summarized monitor health reported in status.
type HealthStatus string

const (
	HealthStatusUnknown    HealthStatus = "Unknown"
	HealthStatusHealthy    HealthStatus = "Healthy"
	HealthStatusNotHealthy HealthStatus = "NotHealthy"
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
	// +kubebuilder:validation:Minimum=1
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
	// +kubebuilder:validation:Minimum=1
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

	// Health is the summarized monitor health reported by SakuraCloud.
	// +kubebuilder:validation:Enum=Unknown;Healthy;NotHealthy
	Health HealthStatus `json:"health,omitempty"`

	// LastSyncedAt is the latest successful synchronization timestamp.
	LastSyncedAt *metav1.Time `json:"lastSyncedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=sakurasimplemonitors,scope=Namespaced,singular=sakurasimplemonitor
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.target"
// +kubebuilder:printcolumn:name="Health",type="string",JSONPath=".status.health"
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
