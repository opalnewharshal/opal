package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	opalv1alpha1 "github.com/opal-io/opal/api/v1alpha1"
	"github.com/opal-io/opal/internal/memory"
	opalmetrics "github.com/opal-io/opal/internal/metrics"
	"github.com/opal-io/opal/internal/outcome"
)

const (
	workloadOutcomeFinalizer    = "opal.io/workloadoutcome-finalizer"
	requeueAfter                = 5 * time.Minute
	// preOpalResourcesAnnotation stores a JSON snapshot of container resources
	// taken immediately before OPAL first patches a Deployment. Used by
	// OutcomeBindingReconciler to restore the original state on rollback.
	preOpalResourcesAnnotation = "opal.io/pre-opal-resources"
)

// containerResourceSnapshot is serialised into preOpalResourcesAnnotation.
type containerResourceSnapshot struct {
	Name     string                      `json:"name"`
	Requests corev1.ResourceList         `json:"requests,omitempty"`
	Limits   corev1.ResourceList         `json:"limits,omitempty"`
}

// WorkloadOutcomeReconciler reconciles WorkloadOutcome objects.
// It is the core controller that translates declared outcomes into
// Kubernetes configuration and drives the cluster toward those outcomes.
//
// +kubebuilder:rbac:groups=opal.io,resources=workloadoutcomes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opal.io,resources=workloadoutcomes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opal.io,resources=workloadoutcomes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
type WorkloadOutcomeReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Memory      *memory.Store
	Translator  *outcome.Translator
	EventRecorder corev1EventRecorder
}

type corev1EventRecorder interface {
	Event(object runtime.Object, eventtype, reason, message string)
	Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{})
}

// Reconcile is the main reconciliation loop for WorkloadOutcome.
func (r *WorkloadOutcomeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	start := time.Now()

	wo := &opalv1alpha1.WorkloadOutcome{}
	if err := r.Get(ctx, req.NamespacedName, wo); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	defer func() {
		opalmetrics.OutcomeReconcileDuration.
			WithLabelValues(req.Namespace, req.Name).
			Observe(time.Since(start).Seconds())
	}()

	// Handle deletion
	if !wo.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, wo)
	}

	// Ensure finalizer
	if !controllerutil.ContainsFinalizer(wo, workloadOutcomeFinalizer) {
		controllerutil.AddFinalizer(wo, workloadOutcomeFinalizer)
		if err := r.Update(ctx, wo); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set initial phase
	if wo.Status.Phase == "" {
		wo.Status.Phase = opalv1alpha1.OutcomePhaseAnalyzing
		if err := r.Status().Update(ctx, wo); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve target workload namespace
	targetNS := wo.Spec.TargetRef.Namespace
	if targetNS == "" {
		targetNS = wo.Namespace
	}

	// Fetch target deployment
	deploy := &appsv1.Deployment{}
	targetKey := types.NamespacedName{Name: wo.Spec.TargetRef.Name, Namespace: targetNS}
	if err := r.Get(ctx, targetKey, deploy); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("target deployment not found, requeueing", "target", targetKey)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get deployment: %w", err)
	}

	// Translate outcome to derived config
	derived, err := r.Translator.Translate(wo)
	if err != nil {
		logger.Error(err, "failed to translate outcome")
		opalmetrics.OutcomeReconcileTotal.WithLabelValues(req.Namespace, req.Name, "error").Inc()
		return r.setPhase(ctx, wo, opalv1alpha1.OutcomePhaseViolated, err.Error())
	}

	// Apply config if autoApply is enabled
	if wo.Spec.AutoApply {
		if err := r.applyDerivedConfig(ctx, wo, deploy, derived); err != nil {
			logger.Error(err, "failed to apply derived config")
			opalmetrics.OutcomeReconcileTotal.WithLabelValues(req.Namespace, req.Name, "apply_error").Inc()
			return r.setPhase(ctx, wo, opalv1alpha1.OutcomePhaseViolated, err.Error())
		}
		opalmetrics.ConfigAppliedTotal.WithLabelValues(req.Namespace, wo.Spec.TargetRef.Name, "success").Inc()
	}

	// Record decision in memory
	for _, reason := range derived.Reasoning {
		_, _ = r.Memory.Record(ctx, memory.Entry{
			WorkloadRef: fmt.Sprintf("%s/%s", targetNS, wo.Spec.TargetRef.Name),
			Type:        memory.EntryTypeAgentDecision,
			Narrative:   reason,
			Labels: map[string]string{
				"outcome":    wo.Name,
				"controller": "WorkloadOutcomeReconciler",
			},
		})
	}

	// Update status
	now := metav1.Now()
	wo.Status.Phase = opalv1alpha1.OutcomePhaseActive
	wo.Status.LastReconciledAt = &now
	wo.Status.AgentDecisions = []opalv1alpha1.AgentDecision{
		{
			AgentName: "outcome-controller",
			Action:    fmt.Sprintf("derived config for %s/%s", targetNS, wo.Spec.TargetRef.Name),
			Reasoning: fmt.Sprintf("%d reasoning steps applied", len(derived.Reasoning)),
			Timestamp: now,
			Applied:   wo.Spec.AutoApply,
		},
	}

	if err := r.Status().Update(ctx, wo); err != nil {
		return ctrl.Result{}, err
	}

	opalmetrics.OutcomeReconcileTotal.WithLabelValues(req.Namespace, req.Name, "success").Inc()
	logger.Info("reconciled WorkloadOutcome", "phase", wo.Status.Phase, "autoApply", wo.Spec.AutoApply)

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *WorkloadOutcomeReconciler) applyDerivedConfig(
	ctx context.Context,
	wo *opalv1alpha1.WorkloadOutcome,
	deploy *appsv1.Deployment,
	derived *outcome.DerivedConfig,
) error {
	// Snapshot original resources before OPAL touches the Deployment.
	// Written only once so subsequent reconciles don't overwrite the baseline.
	if deploy.Annotations == nil || deploy.Annotations[preOpalResourcesAnnotation] == "" {
		snapshots := make([]containerResourceSnapshot, 0, len(deploy.Spec.Template.Spec.Containers))
		for _, c := range deploy.Spec.Template.Spec.Containers {
			snapshots = append(snapshots, containerResourceSnapshot{
				Name:     c.Name,
				Requests: c.Resources.Requests.DeepCopy(),
				Limits:   c.Resources.Limits.DeepCopy(),
			})
		}
		raw, err := json.Marshal(snapshots)
		if err == nil {
			annoPatch := client.MergeFrom(deploy.DeepCopy())
			if deploy.Annotations == nil {
				deploy.Annotations = map[string]string{}
			}
			deploy.Annotations[preOpalResourcesAnnotation] = string(raw)
			if pErr := r.Patch(ctx, deploy, annoPatch); pErr != nil {
				return fmt.Errorf("save pre-opal resource snapshot: %w", pErr)
			}
		}
	}

	patch := client.MergeFrom(deploy.DeepCopy())
	derived.ApplyToDeployment(deploy)
	if err := r.Patch(ctx, deploy, patch); err != nil {
		return fmt.Errorf("patch deployment resources: %w", err)
	}

	if derived.HPA != nil {
		if err := r.applyHPA(ctx, wo, derived); err != nil {
			return err
		}
	}
	if derived.PDB != nil {
		if err := r.applyPDB(ctx, wo, derived); err != nil {
			return err
		}
	}
	return nil
}

func (r *WorkloadOutcomeReconciler) applyHPA(ctx context.Context, wo *opalv1alpha1.WorkloadOutcome, derived *outcome.DerivedConfig) error {
	hpa := derived.HPA.DeepCopy()
	if err := controllerutil.SetControllerReference(wo, hpa, r.Scheme); err != nil {
		return err
	}
	existing := hpa.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
		existing.Spec = hpa.Spec
		existing.Labels = hpa.Labels
		return nil
	})
	return err
}

func (r *WorkloadOutcomeReconciler) applyPDB(ctx context.Context, wo *opalv1alpha1.WorkloadOutcome, derived *outcome.DerivedConfig) error {
	pdb := derived.PDB.DeepCopy()
	if err := controllerutil.SetControllerReference(wo, pdb, r.Scheme); err != nil {
		return err
	}
	existing := pdb.DeepCopy()
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, existing, func() error {
		existing.Spec = pdb.Spec
		existing.Labels = pdb.Labels
		return nil
	})
	return err
}

func (r *WorkloadOutcomeReconciler) handleDeletion(ctx context.Context, wo *opalv1alpha1.WorkloadOutcome) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(wo, workloadOutcomeFinalizer) {
		// Record deletion in memory for audit trail
		_, _ = r.Memory.Record(ctx, memory.Entry{
			WorkloadRef: fmt.Sprintf("%s/%s", wo.Namespace, wo.Spec.TargetRef.Name),
			Type:        memory.EntryTypeConfigChange,
			Narrative:   fmt.Sprintf("WorkloadOutcome %s/%s deleted; OPAL management stopped", wo.Namespace, wo.Name),
			Labels:      map[string]string{"event": "outcome-deleted"},
		})
		controllerutil.RemoveFinalizer(wo, workloadOutcomeFinalizer)
		if err := r.Update(ctx, wo); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *WorkloadOutcomeReconciler) setPhase(ctx context.Context, wo *opalv1alpha1.WorkloadOutcome, phase opalv1alpha1.OutcomePhase, msg string) (ctrl.Result, error) {
	wo.Status.Phase = phase
	wo.Status.Conditions = append(wo.Status.Conditions, metav1.Condition{
		Type:               "Reconciling",
		Status:             metav1.ConditionFalse,
		Reason:             string(phase),
		Message:            msg,
		LastTransitionTime: metav1.Now(),
	})
	_ = r.Status().Update(ctx, wo)
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkloadOutcomeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opalv1alpha1.WorkloadOutcome{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
