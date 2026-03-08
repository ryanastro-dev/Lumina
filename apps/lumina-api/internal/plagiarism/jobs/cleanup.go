package jobs

import (
	"context"
	"log"
	"sync"
	"time"
)

type CleanupStore interface {
	CleanupTerminalOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type CleanupSnapshot struct {
	Enabled          bool       `json:"enabled"`
	Reason           string     `json:"reason,omitempty"`
	RetentionSeconds int64      `json:"retention_seconds"`
	IntervalSeconds  int64      `json:"interval_seconds"`
	LastTrigger      string     `json:"last_trigger,omitempty"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	LastCutoffAt     *time.Time `json:"last_cutoff_at,omitempty"`
	LastSuccessAt    *time.Time `json:"last_success_at,omitempty"`
	LastDeletedCount int64      `json:"last_deleted_count"`
	LastError        string     `json:"last_error,omitempty"`
	TotalRuns        int64      `json:"total_runs"`
	TotalFailures    int64      `json:"total_failures"`
	TotalDeleted     int64      `json:"total_deleted"`
}

type CleanupMonitor struct {
	mu       sync.RWMutex
	snapshot CleanupSnapshot
}

func newCleanupMonitor(enabled bool, reason string, retention, interval time.Duration) *CleanupMonitor {
	return &CleanupMonitor{
		snapshot: CleanupSnapshot{
			Enabled:          enabled,
			Reason:           reason,
			RetentionSeconds: int64(retention.Seconds()),
			IntervalSeconds:  int64(interval.Seconds()),
		},
	}
}

func (m *CleanupMonitor) Snapshot() CleanupSnapshot {
	if m == nil {
		return CleanupSnapshot{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return CleanupSnapshot{
		Enabled:          m.snapshot.Enabled,
		Reason:           m.snapshot.Reason,
		RetentionSeconds: m.snapshot.RetentionSeconds,
		IntervalSeconds:  m.snapshot.IntervalSeconds,
		LastTrigger:      m.snapshot.LastTrigger,
		LastRunAt:        copyTimePtr(m.snapshot.LastRunAt),
		LastCutoffAt:     copyTimePtr(m.snapshot.LastCutoffAt),
		LastSuccessAt:    copyTimePtr(m.snapshot.LastSuccessAt),
		LastDeletedCount: m.snapshot.LastDeletedCount,
		LastError:        m.snapshot.LastError,
		TotalRuns:        m.snapshot.TotalRuns,
		TotalFailures:    m.snapshot.TotalFailures,
		TotalDeleted:     m.snapshot.TotalDeleted,
	}
}

func (m *CleanupMonitor) markRun(trigger string, runAt, cutoff time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.snapshot.LastTrigger = trigger
	m.snapshot.LastRunAt = timePtr(runAt)
	m.snapshot.LastCutoffAt = timePtr(cutoff)
	m.snapshot.TotalRuns++
}

func (m *CleanupMonitor) markSuccess(runAt time.Time, deleted int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.snapshot.LastSuccessAt = timePtr(runAt)
	m.snapshot.LastDeletedCount = deleted
	m.snapshot.TotalDeleted += deleted
	m.snapshot.LastError = ""
}

func (m *CleanupMonitor) markFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.snapshot.TotalFailures++
	if err != nil {
		m.snapshot.LastError = err.Error()
	}
}

func copyTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func timePtr(value time.Time) *time.Time {
	clone := value
	return &clone
}

func StartCleanupLoop(ctx context.Context, store CleanupStore, retention, interval time.Duration, logger *log.Logger) *CleanupMonitor {
	if logger == nil {
		logger = log.Default()
	}

	if store == nil {
		logger.Printf("job cleanup disabled: store is not configured")
		return newCleanupMonitor(false, "store is not configured", retention, interval)
	}
	if retention <= 0 {
		logger.Printf("job cleanup disabled: retention must be > 0")
		return newCleanupMonitor(false, "retention must be > 0", retention, interval)
	}
	if interval <= 0 {
		logger.Printf("job cleanup disabled: interval must be > 0")
		return newCleanupMonitor(false, "interval must be > 0", retention, interval)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	monitor := newCleanupMonitor(true, "", retention, interval)

	run := func(trigger string) {
		runAt := time.Now().UTC()
		cutoff := runAt.Add(-retention)
		monitor.markRun(trigger, runAt, cutoff)

		runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		deleted, err := store.CleanupTerminalOlderThan(runCtx, cutoff)
		if err != nil {
			monitor.markFailure(err)
			logger.Printf("job cleanup %s error: %v", trigger, err)
			return
		}

		monitor.markSuccess(runAt, deleted)
		if deleted > 0 {
			logger.Printf("job cleanup %s removed %d stale jobs", trigger, deleted)
		}
	}

	run("startup")

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run("tick")
			}
		}
	}()

	return monitor
}
