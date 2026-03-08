package jobs

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCleanupStore struct {
	mu      sync.Mutex
	calls   int
	cutoffs []time.Time
	deleted int64
	err     error
}

func (s *fakeCleanupStore) CleanupTerminalOlderThan(_ context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls++
	s.cutoffs = append(s.cutoffs, cutoff)
	return s.deleted, s.err
}

func (s *fakeCleanupStore) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *fakeCleanupStore) latestCutoff() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cutoffs) == 0 {
		return time.Time{}
	}
	return s.cutoffs[len(s.cutoffs)-1]
}

func TestStartCleanupLoopDisabledForInvalidConfig(t *testing.T) {
	store := &fakeCleanupStore{}
	logger := log.New(io.Discard, "", 0)

	monitorByRetention := StartCleanupLoop(context.Background(), store, 0, time.Minute, logger)
	monitorByInterval := StartCleanupLoop(context.Background(), store, time.Hour, 0, logger)
	monitorByStore := StartCleanupLoop(context.Background(), nil, time.Hour, time.Minute, logger)

	if store.callCount() != 0 {
		t.Fatalf("expected no cleanup calls for disabled config, got %d", store.callCount())
	}

	tests := []struct {
		name    string
		monitor *CleanupMonitor
		reason  string
	}{
		{name: "retention", monitor: monitorByRetention, reason: "retention must be > 0"},
		{name: "interval", monitor: monitorByInterval, reason: "interval must be > 0"},
		{name: "store", monitor: monitorByStore, reason: "store is not configured"},
	}

	for _, tt := range tests {
		snapshot := tt.monitor.Snapshot()
		if snapshot.Enabled {
			t.Fatalf("expected disabled snapshot for %s", tt.name)
		}
		if snapshot.Reason != tt.reason {
			t.Fatalf("unexpected disabled reason for %s: %q", tt.name, snapshot.Reason)
		}
	}
}

func TestStartCleanupLoopRunsStartupAndPeriodicCleanup(t *testing.T) {
	store := &fakeCleanupStore{deleted: 2}
	logger := log.New(io.Discard, "", 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	retention := 2 * time.Hour
	interval := 15 * time.Millisecond
	monitor := StartCleanupLoop(ctx, store, retention, interval, logger)

	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		snapshot := monitor.Snapshot()
		if snapshot.TotalRuns >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	calls := store.callCount()
	if calls < 2 {
		t.Fatalf("expected startup + periodic cleanup calls, got %d", calls)
	}

	cutoff := store.latestCutoff()
	if cutoff.IsZero() {
		t.Fatalf("expected non-zero cutoff")
	}

	now := time.Now().UTC()
	minExpected := now.Add(-retention - 3*time.Second)
	maxExpected := now.Add(-retention + 1*time.Second)
	if cutoff.Before(minExpected) || cutoff.After(maxExpected) {
		t.Fatalf("unexpected cutoff %s outside expected range [%s, %s]", cutoff, minExpected, maxExpected)
	}

	snapshot := monitor.Snapshot()
	if !snapshot.Enabled {
		t.Fatalf("expected cleanup snapshot enabled")
	}
	if snapshot.TotalRuns < 2 {
		t.Fatalf("expected total runs >= 2, got %d", snapshot.TotalRuns)
	}
	if snapshot.TotalDeleted < 2 {
		t.Fatalf("expected total deleted >= 2, got %d", snapshot.TotalDeleted)
	}
	if snapshot.LastSuccessAt == nil {
		t.Fatalf("expected last success timestamp")
	}
	if snapshot.LastError != "" {
		t.Fatalf("expected last error to be cleared, got %q", snapshot.LastError)
	}
}

func TestStartCleanupLoopTracksFailure(t *testing.T) {
	store := &fakeCleanupStore{err: errors.New("db unavailable")}
	logger := log.New(io.Discard, "", 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	monitor := StartCleanupLoop(ctx, store, time.Hour, time.Hour, logger)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if store.callCount() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	snapshot := monitor.Snapshot()
	if snapshot.TotalRuns < 1 {
		t.Fatalf("expected at least one run, got %d", snapshot.TotalRuns)
	}
	if snapshot.TotalFailures < 1 {
		t.Fatalf("expected at least one failure, got %d", snapshot.TotalFailures)
	}
	if !strings.Contains(snapshot.LastError, "db unavailable") {
		t.Fatalf("expected failure reason to mention db unavailable, got %q", snapshot.LastError)
	}
	if snapshot.LastSuccessAt != nil {
		t.Fatalf("expected no successful cleanup timestamp when all runs fail")
	}
}
