package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	opalv1alpha1 "github.com/opal-io/opal/api/v1alpha1"
	opalmetrics "github.com/opal-io/opal/internal/metrics"
)

// OutcomeBindingReconciler reconciles OutcomeBinding objects.
// It enforces the approval policy for AI-derived configuration changes,
// manages change windows, and handles automatic rollbacks.
//
// +kubebuilder:rbac:groups=opal.io,resources=outcomebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opal.io,resources=outcomebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opal.io,resources=workloadoutcomes,verbs=get;list;watch
type OutcomeBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile processes an OutcomeBinding.
func (r *OutcomeBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ob := &opalv1alpha1.OutcomeBinding{}
	if err := r.Get(ctx, req.NamespacedName, ob); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch referenced WorkloadOutcome
	wo := &opalv1alpha1.WorkloadOutcome{}
	woKey := types.NamespacedName{
		Name:      ob.Spec.OutcomeRef.Name,
		Namespace: ob.Spec.OutcomeRef.Namespace,
	}
	if woKey.Namespace == "" {
		woKey.Namespace = ob.Namespace
	}
	if err := r.Get(ctx, woKey, wo); err != nil {
		if errors.IsNotFound(err) {
			return r.setBindingPhase(ctx, ob, "Pending", "WorkloadOutcome not found, waiting")
		}
		return ctrl.Result{}, err
	}

	// Validate change window if specified
	if ob.Spec.ChangeWindow != nil && !r.isWithinChangeWindow(ob.Spec.ChangeWindow) {
		logger.Info("outside change window, deferring", "binding", req.NamespacedName)
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// Check rollback conditions
	if ob.Spec.RollbackPolicy != nil && ob.Spec.RollbackPolicy.Enabled {
		shouldRollback, reason := r.shouldTriggerRollback(ob, wo)
		if shouldRollback {
			logger.Info("triggering automatic rollback", "reason", reason)
			opalmetrics.RollbacksTotal.WithLabelValues(ob.Namespace, ob.Spec.TargetRef.Name, ob.Spec.OutcomeRef.Name).Inc()
			return r.performRollback(ctx, ob, reason)
		}
	}

	switch ob.Spec.ApprovalPolicy {
	case opalv1alpha1.ApprovalPolicyAutomatic:
		return r.applyAutomatically(ctx, ob, wo)
	case opalv1alpha1.ApprovalPolicyManual:
		return r.createConfigProposal(ctx, ob, wo)
	case opalv1alpha1.ApprovalPolicyDryRun:
		return r.performDryRun(ctx, ob, wo)
	default:
		return r.createConfigProposal(ctx, ob, wo)
	}
}

func (r *OutcomeBindingReconciler) applyAutomatically(ctx context.Context, ob *opalv1alpha1.OutcomeBinding, wo *opalv1alpha1.WorkloadOutcome) (ctrl.Result, error) {
	now := metav1.Now()

	// Sync autoApply flag to the WorkloadOutcome
	if !wo.Spec.AutoApply {
		patch := client.MergeFrom(wo.DeepCopy())
		wo.Spec.AutoApply = true
		if err := r.Patch(ctx, wo, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("patch WorkloadOutcome autoApply: %w", err)
		}
	}

	ob.Status.Phase = "Applied"
	ob.Status.LastAppliedAt = &now
	ob.Status.Conditions = []metav1.Condition{
		{
			Type:               "Applied",
			Status:             metav1.ConditionTrue,
			Reason:             "AutomaticApply",
			Message:            "Configuration applied automatically per approval policy",
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, ob); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

func (r *OutcomeBindingReconciler) createConfigProposal(ctx context.Context, ob *opalv1alpha1.OutcomeBinding, wo *opalv1alpha1.WorkloadOutcome) (ctrl.Result, error) {
	now := metav1.Now()
	ob.Status.Phase = "Pending"
	ob.Status.Conditions = []metav1.Condition{
		{
			Type:               "AwaitingApproval",
			Status:             metav1.ConditionTrue,
			Reason:             "ManualApprovalRequired",
			Message:            fmt.Sprintf("Config proposal pending approval. Review with: kubectl get outcomebinding %s -n %s", ob.Name, ob.Namespace),
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, ob); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *OutcomeBindingReconciler) performDryRun(ctx context.Context, ob *opalv1alpha1.OutcomeBinding, wo *opalv1alpha1.WorkloadOutcome) (ctrl.Result, error) {
	now := metav1.Now()
	ob.Status.Phase = "Applied"
	ob.Status.LastAppliedAt = &now
	ob.Status.Conditions = []metav1.Condition{
		{
			Type:               "DryRun",
			Status:             metav1.ConditionTrue,
			Reason:             "DryRunOnly",
			Message:            "Dry-run mode: configuration analysed but not applied. Review reasoning in WorkloadOutcome status.",
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, ob); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 30 * time.Minute}, nil
}

func (r *OutcomeBindingReconciler) shouldTriggerRollback(ob *opalv1alpha1.OutcomeBinding, wo *opalv1alpha1.WorkloadOutcome) (bool, string) {
	if ob.Spec.RollbackPolicy == nil || !ob.Spec.RollbackPolicy.Enabled {
		return false, ""
	}
	if len(wo.Status.SLOViolations) >= int(ob.Spec.RollbackPolicy.TriggerAfterViolationCount) {
		return true, fmt.Sprintf("%d consecutive SLO violations detected", len(wo.Status.SLOViolations))
	}
	return false, ""
}

func (r *OutcomeBindingReconciler) performRollback(ctx context.Context, ob *opalv1alpha1.OutcomeBinding, reason string) (ctrl.Result, error) {
	now := metav1.Now()
	event := opalv1alpha1.RollbackEvent{
		Timestamp: now,
		Reason:    reason,
		Outcome:   "Unknown",
	}
	ob.Status.Phase = "RolledBack"
	ob.Status.RollbackHistory = append(ob.Status.RollbackHistory, event)
	ob.Status.Conditions = []metav1.Condition{
		{
			Type:               "RolledBack",
			Status:             metav1.ConditionTrue,
			Reason:             "AutomaticRollback",
			Message:            reason,
			LastTransitionTime: now,
		},
	}
	if err := r.Status().Update(ctx, ob); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *OutcomeBindingReconciler) isWithinChangeWindow(cw *opalv1alpha1.ChangeWindowSpec) bool {
	if cw == nil {
		return true
	}
	now := time.Now().UTC()
	weekday := now.Weekday().String()
	inDay := false
	for _, d := range cw.AllowedDays {
		if d == weekday {
			inDay = true
			break
		}
	}
	if len(cw.AllowedDays) > 0 && !inDay {
		return false
	}
	return true
}

func (r *OutcomeBindingReconciler) setBindingPhase(ctx context.Context, ob *opalv1alpha1.OutcomeBinding, phase, msg string) (ctrl.Result, error) {
	ob.Status.Phase = phase
	ob.Status.Conditions = []metav1.Condition{
		{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             phase,
			Message:            msg,
			LastTransitionTime: metav1.Now(),
		},
	}
	_ = r.Status().Update(ctx, ob)
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OutcomeBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opalv1alpha1.OutcomeBinding{}).
		Complete(r)
}
