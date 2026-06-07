package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
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
	logger := log.FromContext(ctx)
	now := metav1.Now()

	targetNS := ob.Spec.TargetRef.Namespace
	if targetNS == "" {
		targetNS = ob.Namespace
	}

	// 1. Disable autoApply on the WorkloadOutcome to stop further changes.
	wo := &opalv1alpha1.WorkloadOutcome{}
	woKey := types.NamespacedName{Name: ob.Spec.OutcomeRef.Name, Namespace: ob.Spec.OutcomeRef.Namespace}
	if woKey.Namespace == "" {
		woKey.Namespace = ob.Namespace
	}
	if err := r.Get(ctx, woKey, wo); err == nil && wo.Spec.AutoApply {
		patch := client.MergeFrom(wo.DeepCopy())
		wo.Spec.AutoApply = false
		if pErr := r.Patch(ctx, wo, patch); pErr != nil {
			logger.Error(pErr, "could not disable autoApply during rollback")
		}
	}

	// 2. Restore the Deployment's pre-OPAL resource snapshot if one exists.
	deploy := &appsv1.Deployment{}
	deployKey := types.NamespacedName{Name: ob.Spec.TargetRef.Name, Namespace: targetNS}
	if err := r.Get(ctx, deployKey, deploy); err == nil {
		if raw, ok := deploy.Annotations[preOpalResourcesAnnotation]; ok && raw != "" {
			var snapshots []containerResourceSnapshotRollback
			if jsonErr := json.Unmarshal([]byte(raw), &snapshots); jsonErr == nil {
				patch := client.MergeFrom(deploy.DeepCopy())
				for i := range deploy.Spec.Template.Spec.Containers {
					for _, snap := range snapshots {
						if deploy.Spec.Template.Spec.Containers[i].Name == snap.Name {
							deploy.Spec.Template.Spec.Containers[i].Resources = corev1.ResourceRequirements{
								Requests: snap.Requests,
								Limits:   snap.Limits,
							}
							break
						}
					}
				}
				if pErr := r.Patch(ctx, deploy, patch); pErr != nil {
					logger.Error(pErr, "could not restore pre-OPAL resources during rollback")
				} else {
					logger.Info("restored pre-OPAL Deployment resources", "deployment", deployKey)
				}
			}
		}
	}

	// 3. Delete OPAL-managed HPA and PDB for this workload.
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	hpaKey := types.NamespacedName{
		Name:      fmt.Sprintf("opal-%s", ob.Spec.TargetRef.Name),
		Namespace: targetNS,
	}
	if err := r.Get(ctx, hpaKey, hpa); err == nil {
		if delErr := r.Delete(ctx, hpa); delErr != nil && !errors.IsNotFound(delErr) {
			logger.Error(delErr, "could not delete OPAL HPA during rollback")
		}
	}

	pdb := &policyv1.PodDisruptionBudget{}
	pdbKey := types.NamespacedName{
		Name:      fmt.Sprintf("opal-%s", ob.Spec.TargetRef.Name),
		Namespace: targetNS,
	}
	if err := r.Get(ctx, pdbKey, pdb); err == nil {
		if delErr := r.Delete(ctx, pdb); delErr != nil && !errors.IsNotFound(delErr) {
			logger.Error(delErr, "could not delete OPAL PDB during rollback")
		}
	}

	// 4. Record rollback in status.
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

// containerResourceSnapshotRollback mirrors containerResourceSnapshot from the
// WorkloadOutcome controller for JSON deserialisation during rollback.
type containerResourceSnapshotRollback struct {
	Name     string              `json:"name"`
	Requests corev1.ResourceList `json:"requests,omitempty"`
	Limits   corev1.ResourceList `json:"limits,omitempty"`
}

func (r *OutcomeBindingReconciler) isWithinChangeWindow(cw *opalv1alpha1.ChangeWindowSpec) bool {
	if cw == nil {
		return true
	}

	loc := time.UTC
	if cw.Timezone != "" && cw.Timezone != "UTC" {
		if tz, err := time.LoadLocation(cw.Timezone); err == nil {
			loc = tz
		}
	}
	now := time.Now().In(loc)

	// Check allowed days.
	if len(cw.AllowedDays) > 0 {
		weekday := now.Weekday().String()
		inDay := false
		for _, d := range cw.AllowedDays {
			if d == weekday {
				inDay = true
				break
			}
		}
		if !inDay {
			return false
		}
	}

	// Check time-of-day window.
	if cw.StartTime != "" && cw.EndTime != "" {
		var startH, startM, endH, endM int
		if _, err := fmt.Sscanf(cw.StartTime, "%d:%d", &startH, &startM); err != nil {
			return false
		}
		if _, err := fmt.Sscanf(cw.EndTime, "%d:%d", &endH, &endM); err != nil {
			return false
		}
		nowMins := now.Hour()*60 + now.Minute()
		startMins := startH*60 + startM
		endMins := endH*60 + endM

		if startMins <= endMins {
			// Normal window: e.g. 08:00–18:00
			if nowMins < startMins || nowMins >= endMins {
				return false
			}
		} else {
			// Overnight window: e.g. 22:00–06:00
			if nowMins < startMins && nowMins >= endMins {
				return false
			}
		}
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
