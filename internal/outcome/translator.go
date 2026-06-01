// Package outcome translates WorkloadOutcome SLO/cost declarations into
// concrete Kubernetes resource configurations. It is the bridge between
// "what you want" (outcomes) and "what Kubernetes needs" (manifests).
package outcome

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	opalv1alpha1 "github.com/opal-io/opal/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DerivedConfig holds all Kubernetes resources derived from a WorkloadOutcome.
// These are applied to the cluster by the OutcomeController.
type DerivedConfig struct {
	// ResourcePatch is the resource requests/limits patch for the target workload.
	ResourcePatch *ResourcePatch `json:"resourcePatch,omitempty"`
	// MinReplicas is the minimum replica count derived from availability SLO.
	MinReplicas int32 `json:"minReplicas"`
	// MaxReplicas is the maximum replica count derived from throughput SLO and cost.
	MaxReplicas int32 `json:"maxReplicas"`
	// HPA is the HorizontalPodAutoscaler derived from the SLOs.
	HPA *autoscalingv2.HorizontalPodAutoscaler `json:"hpa,omitempty"`
	// PDB is the PodDisruptionBudget derived from the availability SLO.
	PDB *policyv1.PodDisruptionBudget `json:"pdb,omitempty"`
	// Reasoning explains the translation decisions in human-readable form.
	Reasoning []string `json:"reasoning"`
}

// ResourcePatch holds derived resource requests and limits.
type ResourcePatch struct {
	CPURequest    string `json:"cpuRequest"`
	CPULimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
}

// Translator converts WorkloadOutcome specs into DerivedConfig.
type Translator struct {
	// costPerCPUHour is the estimated cost in USD per CPU core per hour.
	costPerCPUHour float64
	// costPerGBHour is the estimated cost in USD per GB memory per hour.
	costPerGBHour float64
}

// New creates a Translator with default cloud cost estimates.
func New() *Translator {
	return &Translator{
		costPerCPUHour: 0.048,  // ~$35/month per core
		costPerGBHour:  0.006,  // ~$4.30/month per GB
	}
}

// Translate derives Kubernetes configuration from a WorkloadOutcome spec.
func (t *Translator) Translate(outcome *opalv1alpha1.WorkloadOutcome) (*DerivedConfig, error) {
	cfg := &DerivedConfig{
		Reasoning: []string{},
	}

	// --- Derive resource requests from latency SLO ---
	cpu, mem, err := t.deriveCPUAndMemory(outcome.Spec.SLO, outcome.Spec.Cost)
	if err != nil {
		return nil, fmt.Errorf("derive resources: %w", err)
	}
	cfg.ResourcePatch = &ResourcePatch{
		CPURequest:    cpu.request,
		CPULimit:      cpu.limit,
		MemoryRequest: mem.request,
		MemoryLimit:   mem.limit,
	}
	cfg.Reasoning = append(cfg.Reasoning, cpu.reasoning, mem.reasoning)

	// --- Derive replica counts from availability SLO ---
	minReplicas, replicaReason := deriveMinReplicas(outcome.Spec.SLO.Availability)
	cfg.MinReplicas = minReplicas
	cfg.Reasoning = append(cfg.Reasoning, replicaReason)

	// --- Derive max replicas from cost constraint ---
	maxReplicas := int32(20) // safe default
	if outcome.Spec.Cost != nil {
		max, costReason, err := t.deriveMaxReplicasFromCost(outcome.Spec.Cost, cpu.request, mem.request)
		if err == nil {
			maxReplicas = max
			cfg.Reasoning = append(cfg.Reasoning, costReason)
		}
	}
	cfg.MaxReplicas = maxReplicas

	// --- Generate HPA ---
	cfg.HPA = t.buildHPA(outcome, minReplicas, maxReplicas)
	cfg.Reasoning = append(cfg.Reasoning, fmt.Sprintf(
		"HPA configured: min=%d max=%d, targeting p99 latency via custom metric",
		minReplicas, maxReplicas,
	))

	// --- Generate PDB ---
	if minReplicas >= 2 {
		cfg.PDB = t.buildPDB(outcome)
		cfg.Reasoning = append(cfg.Reasoning, fmt.Sprintf(
			"PodDisruptionBudget: minAvailable=%d to maintain %.2s%% availability",
			minReplicas-1, outcome.Spec.SLO.Availability,
		))
	}

	return cfg, nil
}

type resourceResult struct {
	request   string
	limit     string
	reasoning string
}

func (t *Translator) deriveCPUAndMemory(slo opalv1alpha1.SLOSpec, cost *opalv1alpha1.CostSpec) (cpu, mem resourceResult, err error) {
	// Latency-to-CPU heuristic:
	// p99 < 50ms  → high-performance → 500m request / 2000m limit
	// p99 < 100ms → standard        → 250m request / 1000m limit
	// p99 < 500ms → normal          → 100m request / 500m limit
	// p99 >= 500ms → minimal        → 50m request / 200m limit

	cpuRequestMillis := int64(100)
	cpuLimitMillis := int64(500)
	memRequestMB := int64(256)
	memLimitMB := int64(512)
	latencyNote := "no latency SLO specified, using conservative defaults"

	if slo.LatencyP99Ms != nil {
		p99 := *slo.LatencyP99Ms
		switch {
		case p99 < 50:
			cpuRequestMillis, cpuLimitMillis = 500, 2000
			memRequestMB, memLimitMB = 512, 2048
			latencyNote = fmt.Sprintf("p99 < 50ms requires high-performance profile: %dm CPU request", cpuRequestMillis)
		case p99 < 100:
			cpuRequestMillis, cpuLimitMillis = 250, 1000
			memRequestMB, memLimitMB = 256, 1024
			latencyNote = fmt.Sprintf("p99 < 100ms requires standard profile: %dm CPU request", cpuRequestMillis)
		case p99 < 500:
			cpuRequestMillis, cpuLimitMillis = 100, 500
			memRequestMB, memLimitMB = 128, 512
			latencyNote = fmt.Sprintf("p99 < 500ms uses normal profile: %dm CPU request", cpuRequestMillis)
		default:
			cpuRequestMillis, cpuLimitMillis = 50, 200
			memRequestMB, memLimitMB = 64, 256
			latencyNote = fmt.Sprintf("p99 >= 500ms uses minimal profile: %dm CPU request", cpuRequestMillis)
		}
	}

	// Apply throughput scaling
	if slo.ThroughputRPS != nil && *slo.ThroughputRPS > 100 {
		scale := float64(*slo.ThroughputRPS) / 100.0
		cpuRequestMillis = int64(math.Ceil(float64(cpuRequestMillis) * scale))
		cpuLimitMillis = int64(math.Ceil(float64(cpuLimitMillis) * scale))
		latencyNote += fmt.Sprintf("; scaled for %d RPS throughput requirement", *slo.ThroughputRPS)
	}

	cpu = resourceResult{
		request:   fmt.Sprintf("%dm", cpuRequestMillis),
		limit:     fmt.Sprintf("%dm", cpuLimitMillis),
		reasoning: latencyNote,
	}
	mem = resourceResult{
		request:   fmt.Sprintf("%dMi", memRequestMB),
		limit:     fmt.Sprintf("%dMi", memLimitMB),
		reasoning: fmt.Sprintf("memory: %dMi request / %dMi limit based on latency profile", memRequestMB, memLimitMB),
	}
	return cpu, mem, nil
}

func deriveMinReplicas(availability string) (int32, string) {
	if availability == "" {
		return 2, "no availability SLO; defaulting to 2 replicas for basic HA"
	}
	avail, err := strconv.ParseFloat(availability, 64)
	if err != nil {
		return 2, "could not parse availability SLO; defaulting to 2 replicas"
	}
	switch {
	case avail >= 99.99:
		return 5, fmt.Sprintf("%.4f%% availability requires minimum 5 replicas across zones", avail)
	case avail >= 99.95:
		return 4, fmt.Sprintf("%.4f%% availability requires minimum 4 replicas", avail)
	case avail >= 99.9:
		return 3, fmt.Sprintf("%.4f%% availability requires minimum 3 replicas", avail)
	case avail >= 99.0:
		return 2, fmt.Sprintf("%.4f%% availability requires minimum 2 replicas", avail)
	default:
		return 1, fmt.Sprintf("%.4f%% availability SLO permits single replica", avail)
	}
}

func (t *Translator) deriveMaxReplicasFromCost(cost *opalv1alpha1.CostSpec, cpuRequest, memRequest string) (int32, string, error) {
	ceiling, err := strconv.ParseFloat(cost.MonthlyCeiling, 64)
	if err != nil {
		return 20, "", fmt.Errorf("parse cost ceiling: %w", err)
	}
	cpuMillis := parseMillicores(cpuRequest)
	memMB := parseMegabytes(memRequest)

	hourlyBudget := ceiling / (24 * 30)
	cpuCoresPerReplica := float64(cpuMillis) / 1000.0
	memGBPerReplica := float64(memMB) / 1024.0

	costPerReplicaHour := cpuCoresPerReplica*t.costPerCPUHour + memGBPerReplica*t.costPerGBHour
	if costPerReplicaHour <= 0 {
		return 20, "", nil
	}
	maxReplicas := int32(math.Floor(hourlyBudget / costPerReplicaHour))
	if maxReplicas < 1 {
		maxReplicas = 1
	}
	return maxReplicas, fmt.Sprintf(
		"max replicas capped at %d to stay within %s %s/month budget (%.4f/hour/replica)",
		maxReplicas, cost.MonthlyCeiling, cost.Currency, costPerReplicaHour,
	), nil
}

func (t *Translator) buildHPA(outcome *opalv1alpha1.WorkloadOutcome, minReplicas, maxReplicas int32) *autoscalingv2.HorizontalPodAutoscaler {
	metrics := []autoscalingv2.MetricSpec{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: int32Ptr(70),
				},
			},
		},
	}

	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("opal-%s", outcome.Spec.TargetRef.Name),
			Namespace: outcome.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "opal",
				"opal.io/outcome":              outcome.Name,
			},
			Annotations: map[string]string{
				"opal.io/generated-from": fmt.Sprintf("%s/%s", outcome.Namespace, outcome.Name),
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       outcome.Spec.TargetRef.Kind,
				Name:       outcome.Spec.TargetRef.Name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
			Metrics:     metrics,
		},
	}
}

func (t *Translator) buildPDB(outcome *opalv1alpha1.WorkloadOutcome) *policyv1.PodDisruptionBudget {
	minAvailable := intstr.FromInt(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("opal-%s", outcome.Spec.TargetRef.Name),
			Namespace: outcome.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "opal",
				"opal.io/outcome":              outcome.Name,
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": outcome.Spec.TargetRef.Name},
			},
		},
	}
}

// ApplyToDeployment patches a Deployment with the derived resource configuration.
func (cfg *DerivedConfig) ApplyToDeployment(deploy *appsv1.Deployment) {
	if cfg.ResourcePatch == nil {
		return
	}
	for i := range deploy.Spec.Template.Spec.Containers {
		c := &deploy.Spec.Template.Spec.Containers[i]
		c.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cfg.ResourcePatch.CPURequest),
				corev1.ResourceMemory: resource.MustParse(cfg.ResourcePatch.MemoryRequest),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cfg.ResourcePatch.CPULimit),
				corev1.ResourceMemory: resource.MustParse(cfg.ResourcePatch.MemoryLimit),
			},
		}
	}
}

func parseMillicores(s string) int64 {
	s = strings.TrimSuffix(s, "m")
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseMegabytes(s string) int64 {
	s = strings.TrimSuffix(s, "Mi")
	s = strings.TrimSuffix(s, "M")
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func int32Ptr(i int32) *int32 { return &i }
