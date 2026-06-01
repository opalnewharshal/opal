# ClusterMemory

## What is ClusterMemory?

ClusterMemory is OPAL's temporal event store — the episodic memory of your
cluster's behaviour. It is what allows OPAL to learn from the past and
predict the future.

Neither etcd (point-in-time state) nor Prometheus (raw metrics) provide what
OPAL needs: semantically-labelled, historically-rich, agent-queryable cluster
events. ClusterMemory fills this gap.

---

## What Gets Stored

ClusterMemory stores `Entry` records with these fields:

| Field | Description |
|---|---|
| `WorkloadRef` | `namespace/name` of the related workload |
| `Type` | Event classification (see types below) |
| `Timestamp` | When the event occurred |
| `Metrics` | Key-value numerical measurements |
| `Labels` | Categorical metadata for filtering |
| `Narrative` | Human-readable event description |
| `Outcome` | What happened after this event (for learning) |
| `RelatedEntryIDs` | Links to causally-related entries |

### Entry Types

| Type | When Recorded |
|---|---|
| `MetricSnapshot` | Periodic CPU/memory/RPS snapshots |
| `ScalingEvent` | HPA scale-up or scale-down |
| `Incident` | Service degradation or outage |
| `Deployment` | New code deployment |
| `SLOViolation` | SLO breach detected |
| `CostEvent` | Cost threshold approached or exceeded |
| `AgentDecision` | OPAL agent made a configuration change |
| `ConfigChange` | Kubernetes config was modified |
| `PatternDetected` | A recurring pattern was identified |

---

## Key Queries

### Get recent events for a workload

```go
entries, err := memStore.Query(ctx, memory.QueryFilter{
    WorkloadRef: "production/checkout",
    Since:       time.Now().Add(-24 * time.Hour),
    Types:       []memory.EntryType{memory.EntryTypeSLOViolation},
    Limit:       10,
})
```

### Find same-time-last-week patterns

```go
// What happened last 4 Fridays between 19:00-21:00?
entries, err := memStore.SameTimeLast(ctx,
    "production/checkout",
    time.Now(),        // reference time
    2*time.Hour,       // window
    4,                 // look back 4 weeks
)
```

### Store a pattern

```go
err := memStore.RecordPattern(ctx, memory.Pattern{
    WorkloadRef:        "production/checkout",
    Description:        "CPU spikes every Friday evening 19:00-20:30",
    Occurrences:        8,
    LastSeen:           time.Now(),
    RecurrenceInterval: 7 * 24 * time.Hour,
})
```

---

## Persistence

By default, ClusterMemory is in-process only — it resets when the controller
restarts. Enable persistence in the Helm values:

```yaml
memory:
  persistence:
    enabled: true
    size: 5Gi
    storageClass: standard
```

With persistence enabled, OPAL snapshots ClusterMemory to a PVC every 5 minutes
and restores it on startup. This means OPAL's knowledge survives controller
restarts and upgrades.

---

## Memory Growth

ClusterMemory automatically evicts oldest entries when the per-workload limit
is reached (default: 10,000 entries per workload). Configure with:

```bash
helm upgrade opal opal/opal \
  --set controller.memoryMaxEntries=50000
```

---

## Privacy Considerations

ClusterMemory stores cluster operational data (metrics, events, decisions).
It does not store user data, request payloads, or application data.
All stored narratives are generated from infrastructure metrics, not
application content.
