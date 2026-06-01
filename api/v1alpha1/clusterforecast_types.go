package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterForecastSpec defines the parameters for cluster-wide prediction.
type ClusterForecastSpec struct {
	// Horizon is how far ahead to forecast.
	// +kubebuilder:default="24h"
	Horizon string `json:"horizon,omitempty"`

	// WorkloadSelectors optionally limits forecasting to matching workloads.
	// If empty, all workloads with WorkloadOutcomes are forecasted.
	// +optional
	WorkloadSelectors []WorkloadSelector `json:"workloadSelectors,omitempty"`

	// RefreshInterval controls how often the forecast is regenerated.
	// +kubebuilder:default="15m"
	RefreshInterval string `json:"refreshInterval,omitempty"`

	// MinConfidence is the minimum confidence score (0.0-1.0) for predictions
	// to be included in recommended actions.
	// +kubebuilder:default="0.7"
	// +kubebuilder:validation:Pattern=`^(0(\.[0-9]+)?|1(\.0+)?)$`
	MinConfidence string `json:"minConfidence,omitempty"`
}

// WorkloadSelector identifies workloads to include in forecasting.
type WorkloadSelector struct {
	// Namespace restricts selection to this namespace. Empty means all namespaces.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// LabelSelector filters workloads by labels.
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}

// ClusterForecastStatus holds the results of predictive analysis.
type ClusterForecastStatus struct {
	// Forecasts contains per-workload predictions.
	// +optional
	Forecasts []WorkloadForecast `json:"forecasts,omitempty"`

	// LastUpdated is when this forecast was last regenerated.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// ModelVersion is the identifier of the forecast model used.
	// +optional
	ModelVersion string `json:"modelVersion,omitempty"`

	// OverallClusterHealth summarises the predicted cluster health.
	// +optional
	// +kubebuilder:validation:Enum=Healthy;AtRisk;Critical
	OverallClusterHealth string `json:"overallClusterHealth,omitempty"`

	// Conditions represent the status of the forecast process.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// WorkloadForecast holds predictions for a single workload.
type WorkloadForecast struct {
	// WorkloadRef identifies the workload (namespace/name).
	WorkloadRef string `json:"workloadRef"`

	// PredictedEvents lists events expected to occur within the horizon.
	// +optional
	PredictedEvents []PredictedEvent `json:"predictedEvents,omitempty"`

	// Confidence is the overall confidence score for this workload's forecast (0.0-1.0).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	Confidence float64 `json:"confidence"`

	// RecommendedActions lists pre-emptive actions OPAL will take.
	// +optional
	RecommendedActions []RecommendedAction `json:"recommendedActions,omitempty"`

	// ResourceForecast predicts resource needs at future points in time.
	// +optional
	ResourceForecast []ResourcePoint `json:"resourceForecast,omitempty"`

	// RiskScore is a 0-100 score representing predicted risk of SLO violation.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	RiskScore int32 `json:"riskScore"`
}

// PredictedEvent describes a future cluster event.
type PredictedEvent struct {
	// Type classifies the predicted event.
	// +kubebuilder:validation:Enum=CPUSpike;MemoryPressure;LatencyDegradation;ScaleRequired;NodePressure;SLOViolationRisk;CostOverrun
	Type string `json:"type"`

	// PredictedAt is the predicted time of the event.
	PredictedAt metav1.Time `json:"predictedAt"`

	// Confidence is the probability this event will occur (0.0-1.0).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	Confidence float64 `json:"confidence"`

	// Severity is the anticipated impact severity.
	// +kubebuilder:validation:Enum=Low;Medium;High;Critical
	Severity string `json:"severity"`

	// Description explains the predicted event in human-readable terms.
	Description string `json:"description"`

	// SupportingEvidence lists historical observations supporting this prediction.
	// +optional
	SupportingEvidence []string `json:"supportingEvidence,omitempty"`
}

// RecommendedAction describes a pre-emptive action OPAL will take.
type RecommendedAction struct {
	// Action describes what will be done.
	Action string `json:"action"`

	// ScheduledFor is when the action will be executed.
	ScheduledFor metav1.Time `json:"scheduledFor"`

	// Reasoning is the AI explanation for why this action is recommended.
	Reasoning string `json:"reasoning"`

	// Status tracks whether the action has been applied.
	// +kubebuilder:validation:Enum=Pending;Applied;Skipped;Failed
	Status string `json:"status"`
}

// ResourcePoint is a predicted resource requirement at a specific future time.
type ResourcePoint struct {
	// Time is the future timestamp.
	Time metav1.Time `json:"time"`
	// CPUMillicores is the predicted CPU requirement.
	CPUMillicores int64 `json:"cpuMillicores"`
	// MemoryMB is the predicted memory requirement in megabytes.
	MemoryMB int64 `json:"memoryMB"`
	// Replicas is the predicted required replica count.
	Replicas int32 `json:"replicas"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cf,categories=opal
// +kubebuilder:printcolumn:name="Health",type="string",JSONPath=".status.overallClusterHealth"
// +kubebuilder:printcolumn:name="Workloads",type="integer",JSONPath=".status.forecasts[*]"
// +kubebuilder:printcolumn:name="LastUpdated",type="date",JSONPath=".status.lastUpdated"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterForecast is the Schema for the clusterforecasts API.
// It holds AI-generated predictions about future cluster state across
// all workloads with WorkloadOutcomes defined.
type ClusterForecast struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterForecastSpec   `json:"spec,omitempty"`
	Status ClusterForecastStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterForecastList contains a list of ClusterForecast.
type ClusterForecastList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterForecast `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterForecast{}, &ClusterForecastList{})
}
