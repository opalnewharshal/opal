package memory

import (
	"context"
	"testing"
	"time"
)

func TestRecord(t *testing.T) {
	s := New()
	ctx := context.Background()

	id, err := s.Record(ctx, Entry{
		WorkloadRef: "production/checkout",
		Type:        EntryTypeMetricSnapshot,
		Metrics:     map[string]float64{"cpu_millicores": 450.0, "rps": 1200.0},
		Narrative:   "CPU approaching limit during peak hours",
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
}

func TestQuery_ByWorkload(t *testing.T) {
	s := New()
	ctx := context.Background()

	workload := "production/checkout"
	for i := 0; i < 5; i++ {
		_, err := s.Record(ctx, Entry{
			WorkloadRef: workload,
			Type:        EntryTypeMetricSnapshot,
			Timestamp:   time.Now().Add(-time.Duration(i) * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// record for a different workload
	_, _ = s.Record(ctx, Entry{
		WorkloadRef: "production/api-gateway",
		Type:        EntryTypeMetricSnapshot,
	})

	entries, err := s.Query(ctx, QueryFilter{WorkloadRef: workload})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

func TestQuery_Limit(t *testing.T) {
	s := New()
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		_, _ = s.Record(ctx, Entry{
			WorkloadRef: "production/checkout",
			Type:        EntryTypeMetricSnapshot,
			Timestamp:   time.Now().Add(-time.Duration(i) * time.Minute),
		})
	}

	entries, err := s.Query(ctx, QueryFilter{WorkloadRef: "production/checkout", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

func TestQuery_TypeFilter(t *testing.T) {
	s := New()
	ctx := context.Background()

	workload := "production/checkout"
	_, _ = s.Record(ctx, Entry{WorkloadRef: workload, Type: EntryTypeIncident})
	_, _ = s.Record(ctx, Entry{WorkloadRef: workload, Type: EntryTypeMetricSnapshot})
	_, _ = s.Record(ctx, Entry{WorkloadRef: workload, Type: EntryTypeSLOViolation})

	entries, err := s.Query(ctx, QueryFilter{
		WorkloadRef: workload,
		Types:       []EntryType{EntryTypeIncident, EntryTypeSLOViolation},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestQuery_SinceFilter(t *testing.T) {
	s := New()
	ctx := context.Background()

	workload := "production/checkout"
	now := time.Now()
	_, _ = s.Record(ctx, Entry{WorkloadRef: workload, Type: EntryTypeMetricSnapshot, Timestamp: now.Add(-2 * time.Hour)})
	_, _ = s.Record(ctx, Entry{WorkloadRef: workload, Type: EntryTypeMetricSnapshot, Timestamp: now.Add(-30 * time.Minute)})
	_, _ = s.Record(ctx, Entry{WorkloadRef: workload, Type: EntryTypeMetricSnapshot, Timestamp: now.Add(-5 * time.Minute)})

	entries, err := s.Query(ctx, QueryFilter{
		WorkloadRef: workload,
		Since:       now.Add(-1 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries since 1h ago, got %d", len(entries))
	}
}

func TestMaxEntriesPerWorkload(t *testing.T) {
	s := New(WithMaxEntriesPerWorkload(10))
	ctx := context.Background()

	workload := "production/checkout"
	for i := 0; i < 20; i++ {
		_, _ = s.Record(ctx, Entry{
			WorkloadRef: workload,
			Type:        EntryTypeMetricSnapshot,
			Timestamp:   time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	entries, err := s.Query(ctx, QueryFilter{WorkloadRef: workload})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 10 {
		t.Errorf("expected at most 10 entries, got %d", len(entries))
	}
}

func TestPatterns(t *testing.T) {
	s := New()
	ctx := context.Background()

	workload := "production/checkout"
	err := s.RecordPattern(ctx, Pattern{
		WorkloadRef:        workload,
		Description:        "CPU spikes every Friday evening between 18:00-20:00",
		Occurrences:        8,
		LastSeen:           time.Now(),
		RecurrenceInterval: 7 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	patterns, err := s.GetPatterns(ctx, workload)
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].Occurrences != 8 {
		t.Errorf("expected 8 occurrences, got %d", patterns[0].Occurrences)
	}
}

func TestSnapshot_Restore(t *testing.T) {
	s := New()
	ctx := context.Background()

	_, _ = s.Record(ctx, Entry{WorkloadRef: "production/checkout", Type: EntryTypeIncident, Narrative: "OOM kill"})

	snap, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}

	s2 := New()
	if err := s2.Restore(ctx, snap); err != nil {
		t.Fatal(err)
	}

	entries, err := s2.Query(ctx, QueryFilter{WorkloadRef: "production/checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after restore, got %d", len(entries))
	}
}
