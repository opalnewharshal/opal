// Package memory provides the ClusterMemory temporal store — OPAL's episodic
// memory of cluster behaviour. Unlike etcd (point-in-time state) or Prometheus
// (raw metrics), ClusterMemory stores semantically-labelled events that agents
// can query by workload, time range, and behavioural pattern.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// EntryType classifies a memory entry for semantic retrieval.
type EntryType string

const (
	EntryTypeMetricSnapshot   EntryType = "MetricSnapshot"
	EntryTypeScalingEvent     EntryType = "ScalingEvent"
	EntryTypeIncident         EntryType = "Incident"
	EntryTypeDeployment       EntryType = "Deployment"
	EntryTypeSLOViolation     EntryType = "SLOViolation"
	EntryTypeCostEvent        EntryType = "CostEvent"
	EntryTypeAgentDecision    EntryType = "AgentDecision"
	EntryTypeConfigChange     EntryType = "ConfigChange"
	EntryTypePatternDetected  EntryType = "PatternDetected"
)

// Entry is a single episodic memory record.
type Entry struct {
	// ID uniquely identifies this entry.
	ID string `json:"id"`

	// WorkloadRef identifies the workload this entry relates to (namespace/name).
	WorkloadRef string `json:"workloadRef"`

	// Type classifies the entry for semantic retrieval.
	Type EntryType `json:"type"`

	// Timestamp is when this event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Metrics holds numerical measurements associated with this entry.
	Metrics map[string]float64 `json:"metrics,omitempty"`

	// Labels holds categorical metadata for filtering.
	Labels map[string]string `json:"labels,omitempty"`

	// Narrative is a human-readable description of the event, used by
	// agents for semantic reasoning and pattern retrieval.
	Narrative string `json:"narrative,omitempty"`

	// Outcome records what happened after this event (for learning).
	Outcome string `json:"outcome,omitempty"`

	// RelatedEntryIDs links causally-related entries.
	RelatedEntryIDs []string `json:"relatedEntryIds,omitempty"`
}

// QueryFilter specifies criteria for retrieving entries from the store.
type QueryFilter struct {
	// WorkloadRef filters to a specific workload. Empty means all workloads.
	WorkloadRef string
	// Types filters to specific entry types. Empty means all types.
	Types []EntryType
	// Since returns only entries at or after this time.
	Since time.Time
	// Until returns only entries before this time. Zero means now.
	Until time.Time
	// Limit caps the number of returned entries (most recent first).
	// Zero means no limit.
	Limit int
	// LabelMatches filters entries whose Labels contain all specified k/v pairs.
	LabelMatches map[string]string
}

// Pattern is a recurring behavioural pattern identified across multiple entries.
type Pattern struct {
	// WorkloadRef is the workload this pattern was identified in.
	WorkloadRef string `json:"workloadRef"`
	// Description explains the pattern in natural language.
	Description string `json:"description"`
	// Occurrences counts how many times this pattern has been observed.
	Occurrences int `json:"occurrences"`
	// LastSeen is when this pattern was most recently observed.
	LastSeen time.Time `json:"lastSeen"`
	// TypicalDuration is the average duration of events in this pattern.
	TypicalDuration time.Duration `json:"typicalDuration,omitempty"`
	// RecurrenceInterval is the typical time between pattern occurrences.
	RecurrenceInterval time.Duration `json:"recurrenceInterval,omitempty"`
	// SupportingEntryIDs is a sample of entries that exhibit this pattern.
	SupportingEntryIDs []string `json:"supportingEntryIds,omitempty"`
}

// Store is the ClusterMemory temporal event store.
// It is safe for concurrent use by multiple controllers and agents.
type Store struct {
	mu       sync.RWMutex
	entries  map[string]*Entry  // keyed by ID
	byWorker map[string][]string // workloadRef -> []entryID (insertion order)
	patterns map[string][]*Pattern

	maxEntriesPerWorkload int
	idCounter             uint64
}

// StoreOption configures a Store.
type StoreOption func(*Store)

// WithMaxEntriesPerWorkload limits per-workload entry retention.
func WithMaxEntriesPerWorkload(n int) StoreOption {
	return func(s *Store) { s.maxEntriesPerWorkload = n }
}

// New creates a new in-memory ClusterMemory store.
func New(opts ...StoreOption) *Store {
	s := &Store{
		entries:               make(map[string]*Entry),
		byWorker:              make(map[string][]string),
		patterns:              make(map[string][]*Pattern),
		maxEntriesPerWorkload: 10000,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Record adds a new entry to the store. It is safe to call concurrently.
func (s *Store) Record(ctx context.Context, e Entry) (string, error) {
	if e.WorkloadRef == "" {
		return "", fmt.Errorf("WorkloadRef is required")
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.idCounter++
	e.ID = fmt.Sprintf("%s-%d-%d", e.WorkloadRef, e.Timestamp.UnixNano(), s.idCounter)
	s.entries[e.ID] = &e

	ids := s.byWorker[e.WorkloadRef]
	ids = append(ids, e.ID)

	// evict oldest entries if over limit
	if len(ids) > s.maxEntriesPerWorkload {
		evict := ids[:len(ids)-s.maxEntriesPerWorkload]
		for _, id := range evict {
			delete(s.entries, id)
		}
		ids = ids[len(ids)-s.maxEntriesPerWorkload:]
	}
	s.byWorker[e.WorkloadRef] = ids

	return e.ID, nil
}

// Query retrieves entries matching the given filter.
// Results are returned newest-first.
func (s *Store) Query(ctx context.Context, f QueryFilter) ([]*Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var candidates []*Entry

	if f.WorkloadRef != "" {
		ids := s.byWorker[f.WorkloadRef]
		for _, id := range ids {
			if e, ok := s.entries[id]; ok {
				candidates = append(candidates, e)
			}
		}
	} else {
		for _, e := range s.entries {
			candidates = append(candidates, e)
		}
	}

	// sort newest first
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Timestamp.After(candidates[j].Timestamp)
	})

	var result []*Entry
	typeSet := make(map[EntryType]bool, len(f.Types))
	for _, t := range f.Types {
		typeSet[t] = true
	}
	until := f.Until
	if until.IsZero() {
		until = time.Now().Add(time.Hour)
	}

	for _, e := range candidates {
		if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
			continue
		}
		if e.Timestamp.After(until) {
			continue
		}
		if len(typeSet) > 0 && !typeSet[e.Type] {
			continue
		}
		if !matchesLabels(e, f.LabelMatches) {
			continue
		}
		result = append(result, e)
		if f.Limit > 0 && len(result) >= f.Limit {
			break
		}
	}
	return result, nil
}

// SameTimeLast returns entries for the same workload that occurred at
// roughly the same time-of-week in previous weeks. This is the core
// primitive for detecting seasonal/diurnal patterns.
func (s *Store) SameTimeLast(ctx context.Context, workloadRef string, t time.Time, window time.Duration, weeks int) ([]*Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	weekday := t.Weekday()
	hour := t.Hour()
	halfWindow := window / 2

	var result []*Entry
	ids := s.byWorker[workloadRef]
	for _, id := range ids {
		e, ok := s.entries[id]
		if !ok {
			continue
		}
		weeksAgo := int(t.Sub(e.Timestamp).Hours() / (24 * 7))
		if weeksAgo < 1 || weeksAgo > weeks {
			continue
		}
		if e.Timestamp.Weekday() != weekday {
			continue
		}
		entryHour := e.Timestamp.Hour()
		diff := abs(entryHour - hour)
		if diff > int(halfWindow.Hours()) {
			continue
		}
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})
	return result, nil
}

// RecordPattern stores an identified behavioural pattern.
func (s *Store) RecordPattern(ctx context.Context, p Pattern) error {
	if p.WorkloadRef == "" {
		return fmt.Errorf("WorkloadRef is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patterns[p.WorkloadRef] = append(s.patterns[p.WorkloadRef], &p)
	return nil
}

// GetPatterns returns all patterns for a workload.
func (s *Store) GetPatterns(ctx context.Context, workloadRef string) ([]*Pattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	patterns := s.patterns[workloadRef]
	out := make([]*Pattern, len(patterns))
	copy(out, patterns)
	return out, nil
}

// Snapshot returns a JSON snapshot of the store for persistence.
func (s *Store) Snapshot(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]*Entry, 0, len(s.entries))
	for _, e := range s.entries {
		all = append(all, e)
	}
	return json.Marshal(all)
}

// Restore loads entries from a JSON snapshot.
func (s *Store) Restore(ctx context.Context, data []byte) error {
	var entries []*Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("unmarshal snapshot: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		s.entries[e.ID] = e
		s.byWorker[e.WorkloadRef] = append(s.byWorker[e.WorkloadRef], e.ID)
	}
	return nil
}

func matchesLabels(e *Entry, required map[string]string) bool {
	for k, v := range required {
		if got, ok := e.Labels[k]; !ok || got != v {
			return false
		}
	}
	return true
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
