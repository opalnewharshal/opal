package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	opalv1alpha1 "github.com/opal-io/opal/api/v1alpha1"
	"github.com/opal-io/opal/internal/forecast"
	opalmetrics "github.com/opal-io/opal/internal/metrics"
)

// ClusterForecastReconciler reconciles ClusterForecast objects.
// It drives the predictive control loop: periodically regenerating forecasts
// for all workloads with WorkloadOutcomes defined and writing predicted events
// back to ClusterForecast status.
//
// +kubebuilder:rbac:groups=opal.io,resources=clusterforecasts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opal.io,resources=clusterforecasts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opal.io,resources=workloadoutcomes,verbs=get;list;watch
type ClusterForecastReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Predictor forecast.Predictor
}

// Reconcile regenerates the cluster forecast.
func (r *ClusterForecastReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cf := &opalv1alpha1.ClusterForecast{}
	if err := r.Get(ctx, req.NamespacedName, cf); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	horizon, err := time.ParseDuration(cf.Spec.Horizon)
	if err != nil {
		horizon = 24 * time.Hour
	}
	refreshInterval, err := time.ParseDuration(cf.Spec.RefreshInterval)
	if err != nil {
		refreshInterval = 15 * time.Minute
	}
	minConf := 0.7
	if cf.Spec.MinConfidence != "" {
		if v, err := strconv.ParseFloat(cf.Spec.MinConfidence, 64); err == nil {
			minConf = v
		}
	}

	// List all WorkloadOutcomes
	woList := &opalv1alpha1.WorkloadOutcomeList{}
	if err := r.List(ctx, woList); err != nil {
		return ctrl.Result{}, fmt.Errorf("list WorkloadOutcomes: %w", err)
	}

	var forecasts []opalv1alpha1.WorkloadForecast
	for _, wo := range woList.Items {
		targetNS := wo.Spec.TargetRef.Namespace
		if targetNS == "" {
			targetNS = wo.Namespace
		}
		workloadRef := fmt.Sprintf("%s/%s", targetNS, wo.Spec.TargetRef.Name)

		wf, err := r.Predictor.Predict(ctx, forecast.PredictRequest{
			WorkloadRef:   workloadRef,
			Horizon:       horizon,
			Now:           time.Now().UTC(),
			MinConfidence: minConf,
		})
		if err != nil {
			logger.Error(err, "failed to predict for workload", "workload", workloadRef)
			opalmetrics.ForecastGenerateTotal.WithLabelValues("error").Inc()
			continue
		}

		forecasts = append(forecasts, *wf)

		// Update metrics
		for _, e := range wf.PredictedEvents {
			opalmetrics.ForecastPredictedEvents.
				WithLabelValues(workloadRef, e.Type, e.Severity).
				Set(1)
		}
	}

	overallHealth := computeOverallHealth(forecasts)

	now := metav1.Now()
	cf.Status.Forecasts = forecasts
	cf.Status.LastUpdated = &now
	cf.Status.ModelVersion = "statistical-v1"
	cf.Status.OverallClusterHealth = overallHealth
	cf.Status.Conditions = []metav1.Condition{
		{
			Type:               "ForecastReady",
			Status:             metav1.ConditionTrue,
			Reason:             "ForecastGenerated",
			Message:            fmt.Sprintf("Generated forecasts for %d workloads", len(forecasts)),
			LastTransitionTime: now,
		},
	}

	if err := r.Status().Update(ctx, cf); err != nil {
		return ctrl.Result{}, fmt.Errorf("update ClusterForecast status: %w", err)
	}

	opalmetrics.ForecastGenerateTotal.WithLabelValues("success").Inc()
	logger.Info("cluster forecast updated", "workloads", len(forecasts), "health", overallHealth)

	return ctrl.Result{RequeueAfter: refreshInterval}, nil
}

func computeOverallHealth(forecasts []opalv1alpha1.WorkloadForecast) string {
	if len(forecasts) == 0 {
		return "Healthy"
	}
	maxRisk := int32(0)
	for _, f := range forecasts {
		if f.RiskScore > maxRisk {
			maxRisk = f.RiskScore
		}
	}
	switch {
	case maxRisk >= 70:
		return "Critical"
	case maxRisk >= 40:
		return "AtRisk"
	default:
		return "Healthy"
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterForecastReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opalv1alpha1.ClusterForecast{}).
		Complete(r)
}
