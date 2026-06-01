# Getting Started with OPAL

This guide walks you through installing OPAL and deploying your first
`WorkloadOutcome` in under 10 minutes.

## Prerequisites

- Kubernetes 1.28+ cluster (local: kind or minikube)
- kubectl configured for your cluster
- Helm 3.x
- [kagent](https://kagent.dev/docs/kagent/introduction/installation) installed
- An API key for an LLM provider (Anthropic, OpenAI, Google, or Ollama)

---

## Step 1: Install OPAL

### Create the LLM secret

```bash
kubectl create namespace opal-system

# Anthropic (Claude)
kubectl create secret generic opal-llm-secret \
  --namespace opal-system \
  --from-literal=api-key=sk-ant-YOUR_KEY_HERE

# OpenAI
# kubectl create secret generic opal-llm-secret \
#   --namespace opal-system \
#   --from-literal=api-key=sk-YOUR_KEY_HERE
```

### Install via Helm

```bash
helm repo add opal https://charts.opal.io
helm repo update

helm install opal opal/opal \
  --namespace opal-system \
  --set llm.provider=anthropic \
  --set llm.model=claude-sonnet-4-6 \
  --wait
```

### Verify installation

```bash
kubectl get pods -n opal-system
# NAME                                       READY   STATUS    RESTARTS   AGE
# opal-controller-manager-xxx               1/1     Running   0          30s
# opal-forecast-agent-xxx                   1/1     Running   0          30s
# opal-outcome-agent-xxx                    1/1     Running   0          30s
# opal-reconciler-agent-xxx                 1/1     Running   0          30s
# opal-audit-agent-xxx                      1/1     Running   0          30s

kubectl get crds | grep opal
# clusterforecasts.opal.io
# outcomebindings.opal.io
# workloadoutcomes.opal.io
```

---

## Step 2: Deploy a Sample Application

```bash
kubectl create namespace production

kubectl create deployment checkout \
  --image=nginx:latest \
  --replicas=1 \
  --namespace=production
```

---

## Step 3: Declare Your First WorkloadOutcome

Instead of setting resource requests and limits manually, declare what you want:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: opal.io/v1alpha1
kind: WorkloadOutcome
metadata:
  name: checkout-service
  namespace: production
spec:
  targetRef:
    kind: Deployment
    name: checkout
    namespace: production
  slo:
    latencyP99Ms: 100
    availability: "99.9"
    errorRatePercent: "0.5"
  cost:
    monthlyCeiling: "200"
    currency: USD
  horizon: 24h
  autoApply: false   # Start with dry-run to see what OPAL would do
EOF
```

---

## Step 4: Watch OPAL Reason

```bash
# Watch the outcome phase
kubectl get workloadoutcome checkout-service -n production -w

# See the reasoning OPAL applied
kubectl describe workloadoutcome checkout-service -n production

# Check generated Kubernetes Events
kubectl get events -n production --field-selector reason=OPALDecision
```

You should see OPAL derive configuration like:

```
OPAL derived config for checkout (production):
- CPU: 100m request / 500m limit (p99 < 500ms → normal profile)
- Memory: 128Mi request / 512Mi limit
- Min replicas: 3 (99.9% availability)
- Max replicas: 12 (within $200/month budget)
- HPA: target 70% CPU utilisation
- PDB: minAvailable=2
```

---

## Step 5: Enable AutoApply

Once you're confident in OPAL's reasoning:

```bash
kubectl patch workloadoutcome checkout-service -n production \
  --type=merge \
  -p '{"spec":{"autoApply":true}}'
```

OPAL will now apply and maintain the derived configuration continuously.

---

## Step 6: Create a ClusterForecast

Enable OPAL to predict future resource needs:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: opal.io/v1alpha1
kind: ClusterForecast
metadata:
  name: cluster-forecast
  namespace: opal-system
spec:
  horizon: 48h
  refreshInterval: 15m
  minConfidence: "0.70"
EOF
```

After 15 minutes, check the forecast:

```bash
kubectl get clusterforecast cluster-forecast -n opal-system -o yaml
```

---

## Step 7: Add a Binding with Change Window

For production workloads, add an `OutcomeBinding` to control when OPAL
applies changes:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: opal.io/v1alpha1
kind: OutcomeBinding
metadata:
  name: checkout-binding
  namespace: production
spec:
  outcomeRef:
    name: checkout-service
    namespace: production
  targetRef:
    kind: Deployment
    name: checkout
  approvalPolicy: Automatic
  changeWindow:
    allowedDays: [Monday, Tuesday, Wednesday, Thursday]
    startTime: "02:00"
    endTime: "06:00"
    timezone: UTC
  rollbackPolicy:
    enabled: true
    triggerAfterViolationCount: 3
EOF
```

---

## Next Steps

- Read the [Architecture Guide](architecture.md)
- Explore [WorkloadOutcome concepts](concepts/workload-outcomes.md)
- Learn about [ClusterForecast](concepts/cluster-forecast.md)
- Set up [ClusterMemory persistence](concepts/memory-store.md)
- See the [full samples](../config/samples/)

---

## Troubleshooting

**OPAL not applying changes?**
- Check `autoApply: true` is set on the WorkloadOutcome
- Verify the OutcomeBinding's change window is active
- Check controller logs: `kubectl logs -n opal-system deploy/opal-controller-manager`

**Forecast not updating?**
- Verify ClusterForecast exists in `opal-system` namespace
- Check ForecastAgent logs: `kubectl logs -n opal-system -l opal.io/role=forecaster`
- Ensure sufficient historical data (OPAL needs ~1 hour of data for basic forecasting)

**LLM agent errors?**
- Verify the `opal-llm-secret` exists and contains a valid API key
- Check agent logs: `kubectl logs -n opal-system -l app.kubernetes.io/component=forecast-agent`
