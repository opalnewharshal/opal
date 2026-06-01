package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OutcomeBindingSpec defines how a WorkloadOutcome is bound to a workload
// and what approval process governs configuration changes.
type OutcomeBindingSpec struct {
	// OutcomeRef references the WorkloadOutcome to bind.
	// +kubebuilder:validation:Required
	OutcomeRef corev1.ObjectReference `json:"outcomeRef"`

	// TargetRef identifies the workload to manage.
	// +kubebuilder:validation:Required
	TargetRef WorkloadRef `json:"targetRef"`

	// ApprovalPolicy controls how derived configurations are applied.
	// +kubebuilder:default=Manual
	// +kubebuilder:validation:Enum=Automatic;Manual;DryRun
	ApprovalPolicy ApprovalPolicy `json:"approvalPolicy,omitempty"`

	// ChangeWindow restricts when OPAL may apply configuration changes.
	// +optional
	ChangeWindow *ChangeWindowSpec `json:"changeWindow,omitempty"`

	// RollbackPolicy controls automatic rollback behaviour.
	// +optional
	RollbackPolicy *RollbackPolicySpec `json:"rollbackPolicy,omitempty"`
}

// ApprovalPolicy defines how configuration changes are applied.
// +kubebuilder:validation:Enum=Automatic;Manual;DryRun
type ApprovalPolicy string

const (
	// ApprovalPolicyAutomatic applies changes immediately without human review.
	ApprovalPolicyAutomatic ApprovalPolicy = "Automatic"
	// ApprovalPolicyManual creates a ConfigProposal requiring human approval.
	ApprovalPolicyManual ApprovalPolicy = "Manual"
	// ApprovalPolicyDryRun only shows what would change, never applies.
	ApprovalPolicyDryRun ApprovalPolicy = "DryRun"
)

// ChangeWindowSpec restricts when configuration changes may be applied.
type ChangeWindowSpec struct {
	// AllowedDays lists days of the week changes are permitted.
	// +kubebuilder:validation:items:Enum=Monday;Tuesday;Wednesday;Thursday;Friday;Saturday;Sunday
	AllowedDays []string `json:"allowedDays,omitempty"`

	// StartTime is the earliest time changes may be applied (HH:MM UTC).
	// +kubebuilder:validation:Pattern=`^([01][0-9]|2[0-3]):[0-5][0-9]$`
	StartTime string `json:"startTime,omitempty"`

	// EndTime is the latest time changes may be applied (HH:MM UTC).
	// +kubebuilder:validation:Pattern=`^([01][0-9]|2[0-3]):[0-5][0-9]$`
	EndTime string `json:"endTime,omitempty"`

	// Timezone is the IANA timezone for the change window. Defaults to UTC.
	// +kubebuilder:default="UTC"
	Timezone string `json:"timezone,omitempty"`
}

// RollbackPolicySpec defines automatic rollback behaviour.
type RollbackPolicySpec struct {
	// Enabled controls whether automatic rollback is active.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// TriggerAfterViolationCount is how many consecutive SLO violations
	// trigger an automatic rollback.
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	TriggerAfterViolationCount int32 `json:"triggerAfterViolationCount,omitempty"`

	// MaxRollbackAgeHours limits rollback to configs applied within this window.
	// +kubebuilder:default=24
	MaxRollbackAgeHours int32 `json:"maxRollbackAgeHours,omitempty"`
}

// OutcomeBindingStatus reflects the observed state of the binding.
type OutcomeBindingStatus struct {
	// Phase is the current state of the binding.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Bound;Applying;Applied;Violated;RolledBack
	Phase string `json:"phase,omitempty"`

	// Conditions provide detailed status information.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// AppliedConfigHash is the hash of the last applied configuration.
	// +optional
	AppliedConfigHash string `json:"appliedConfigHash,omitempty"`

	// LastAppliedAt is when the last configuration change was applied.
	// +optional
	LastAppliedAt *metav1.Time `json:"lastAppliedAt,omitempty"`

	// PendingProposalRef points to a ConfigProposal awaiting approval.
	// +optional
	PendingProposalRef *corev1.ObjectReference `json:"pendingProposalRef,omitempty"`

	// RollbackHistory records previous rollback events.
	// +optional
	RollbackHistory []RollbackEvent `json:"rollbackHistory,omitempty"`
}

// RollbackEvent records a rollback that occurred.
type RollbackEvent struct {
	// Timestamp when the rollback was triggered.
	Timestamp metav1.Time `json:"timestamp"`
	// Reason is why the rollback was triggered.
	Reason string `json:"reason"`
	// RestoredConfigHash is the config hash restored.
	RestoredConfigHash string `json:"restoredConfigHash"`
	// Outcome records whether the rollback resolved the issue.
	// +kubebuilder:validation:Enum=Resolved;Escalated;Unknown
	Outcome string `json:"outcome"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ob,categories=opal
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Outcome",type="string",JSONPath=".spec.outcomeRef.name"
// +kubebuilder:printcolumn:name="ApprovalPolicy",type="string",JSONPath=".spec.approvalPolicy"
// +kubebuilder:printcolumn:name="LastApplied",type="date",JSONPath=".status.lastAppliedAt"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OutcomeBinding binds a WorkloadOutcome to a workload and controls
// how AI-derived configurations are reviewed and applied.
type OutcomeBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OutcomeBindingSpec   `json:"spec,omitempty"`
	Status OutcomeBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OutcomeBindingList contains a list of OutcomeBinding.
type OutcomeBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OutcomeBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OutcomeBinding{}, &OutcomeBindingList{})
}
