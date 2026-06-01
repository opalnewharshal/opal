package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadOutcomeSpec defines the desired business outcomes for a workload.
// Instead of specifying how to run a workload (replicas, CPU, memory),
// operators declare what they want to achieve (latency, availability, cost).
type WorkloadOutcomeSpec struct {
	// TargetRef identifies the workload this outcome governs.
	// +kubebuilder:validation:Required
	TargetRef WorkloadRef `json:"targetRef"`

	// SLO defines the service level objectives that must be maintained.
	// +kubebuilder:validation:Required
	SLO SLOSpec `json:"slo"`

	// Cost defines spending constraints OPAL must respect.
	// +optional
	Cost *CostSpec `json:"cost,omitempty"`

	// Compliance lists regulatory frameworks the workload must satisfy.
	// OPAL uses this to constrain generated configurations.
	// +optional
	Compliance []string `json:"compliance,omitempty"`

	// Horizon defines how far ahead OPAL should predict and pre-position.
	// Must be a valid Go duration string (e.g., "24h", "72h").
	// Defaults to "24h".
	// +optional
	// +kubebuilder:default="24h"
	Horizon string `json:"horizon,omitempty"`

	// AutoApply enables OPAL to automatically apply derived configurations
	// without human approval. When false, OPAL creates a ConfigProposal
	// that must be manually approved.
	// +optional
	// +kubebuilder:default=false
	AutoApply bool `json:"autoApply,omitempty"`
}

// WorkloadRef identifies a Kubernetes workload resource.
type WorkloadRef struct {
	// Kind of the workload (Deployment, StatefulSet, DaemonSet, Rollout).
	// +kubebuilder:validation:Enum=Deployment;StatefulSet;DaemonSet;Rollout
	Kind string `json:"kind"`

	// Name of the workload.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the workload. Defaults to the WorkloadOutcome namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// SLOSpec defines service level objectives for a workload.
type SLOSpec struct {
	// LatencyP99Ms is the 99th-percentile latency target in milliseconds.
	// +optional
	// +kubebuilder:validation:Minimum=1
	LatencyP99Ms *int32 `json:"latencyP99Ms,omitempty"`

	// LatencyP95Ms is the 95th-percentile latency target in milliseconds.
	// +optional
	// +kubebuilder:validation:Minimum=1
	LatencyP95Ms *int32 `json:"latencyP95Ms,omitempty"`

	// Availability is the target availability percentage (e.g., "99.95").
	// +optional
	// +kubebuilder:validation:Pattern=`^(100|[0-9]{1,2}(\.[0-9]{1,4})?)$`
	Availability string `json:"availability,omitempty"`

	// ErrorRatePercent is the maximum acceptable error rate percentage.
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?$`
	ErrorRatePercent string `json:"errorRatePercent,omitempty"`

	// ThroughputRPS is the minimum throughput requirement in requests per second.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ThroughputRPS *int32 `json:"throughputRPS,omitempty"`
}

// CostSpec defines spending constraints.
type CostSpec struct {
	// MonthlyCeiling is the maximum monthly spend in the specified currency.
	// +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]{1,2})?$`
	MonthlyCeiling string `json:"monthlyCeiling"`

	// Currency is the ISO 4217 currency code (e.g., "USD", "EUR").
	// +kubebuilder:default="USD"
	Currency string `json:"currency,omitempty"`
}

// WorkloadOutcomeStatus reflects the observed state of the outcome management.
type WorkloadOutcomeStatus struct {
	// Phase represents the current lifecycle phase of this outcome.
	// +optional
	Phase OutcomePhase `json:"phase,omitempty"`

	// Conditions represent detailed status of the outcome management.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// DerivedConfigRef points to the ConfigMap containing the AI-derived
	// Kubernetes configuration for this outcome.
	// +optional
	DerivedConfigRef *ConfigRef `json:"derivedConfigRef,omitempty"`

	// LastReconciledAt is the timestamp of the last successful reconciliation.
	// +optional
	LastReconciledAt *metav1.Time `json:"lastReconciledAt,omitempty"`

	// SLOViolations lists any current SLO violations detected.
	// +optional
	SLOViolations []SLOViolation `json:"sloViolations,omitempty"`

	// CurrentCostEstimate is the current estimated monthly cost.
	// +optional
	CurrentCostEstimate string `json:"currentCostEstimate,omitempty"`

	// AgentDecisions records the last N decisions made by OPAL agents.
	// +optional
	AgentDecisions []AgentDecision `json:"agentDecisions,omitempty"`
}

// OutcomePhase represents the lifecycle phase of a WorkloadOutcome.
// +kubebuilder:validation:Enum=Pending;Analyzing;Active;Violated;Suspended
type OutcomePhase string

const (
	OutcomePhasePending   OutcomePhase = "Pending"
	OutcomePhaseAnalyzing OutcomePhase = "Analyzing"
	OutcomePhaseActive    OutcomePhase = "Active"
	OutcomePhaseViolated  OutcomePhase = "Violated"
	OutcomePhaseSuspended OutcomePhase = "Suspended"
)

// ConfigRef points to a derived configuration resource.
type ConfigRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// SLOViolation describes a detected SLO violation.
type SLOViolation struct {
	// Type identifies which SLO was violated.
	Type string `json:"type"`
	// ObservedValue is the measured value that violated the SLO.
	ObservedValue string `json:"observedValue"`
	// Threshold is the SLO threshold that was exceeded.
	Threshold string `json:"threshold"`
	// DetectedAt is when the violation was first detected.
	DetectedAt metav1.Time `json:"detectedAt"`
	// Severity is the assessed severity of the violation.
	// +kubebuilder:validation:Enum=Warning;Critical
	Severity string `json:"severity"`
}

// AgentDecision records a decision made by an OPAL agent.
type AgentDecision struct {
	// AgentName is the name of the kagent agent that made the decision.
	AgentName string `json:"agentName"`
	// Action describes the action taken.
	Action string `json:"action"`
	// Reasoning is the AI-generated explanation for the decision.
	Reasoning string `json:"reasoning"`
	// Timestamp is when the decision was made.
	Timestamp metav1.Time `json:"timestamp"`
	// Applied indicates whether the action was applied automatically.
	Applied bool `json:"applied"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=wo,categories=opal
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetRef.name"
// +kubebuilder:printcolumn:name="Availability",type="string",JSONPath=".spec.slo.availability"
// +kubebuilder:printcolumn:name="LatencyP99",type="integer",JSONPath=".spec.slo.latencyP99Ms"
// +kubebuilder:printcolumn:name="AutoApply",type="boolean",JSONPath=".spec.autoApply"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WorkloadOutcome is the Schema for the workloadoutcomes API.
// It allows operators to declare business outcomes instead of Kubernetes configuration.
type WorkloadOutcome struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadOutcomeSpec   `json:"spec,omitempty"`
	Status WorkloadOutcomeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadOutcomeList contains a list of WorkloadOutcome.
type WorkloadOutcomeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadOutcome `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadOutcome{}, &WorkloadOutcomeList{})
}
