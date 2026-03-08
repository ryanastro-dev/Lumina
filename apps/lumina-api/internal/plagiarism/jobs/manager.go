package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lumina/lumina/apps/lumina-api/internal/clients/lumina_engine/apicontract"
)

const (
	defaultThreshold = float32(0.8)
	defaultTopK      = 5
)

type Checker interface {
	Check(ctx context.Context, request apicontract.CheckRequest) (*apicontract.CheckResponse, error)
}

type Manager struct {
	store          Store
	checker        Checker
	processTimeout time.Duration

	mu            sync.Mutex
	cancelByJobID map[string]context.CancelFunc
}

func NewManager(store Store, checker Checker, processTimeout time.Duration) *Manager {
	if processTimeout <= 0 {
		processTimeout = 2 * time.Minute
	}

	return &Manager{
		store:          store,
		checker:        checker,
		processTimeout: processTimeout,
		cancelByJobID:  make(map[string]context.CancelFunc),
	}
}

func (m *Manager) Submit(ctx context.Context, requestID string, request apicontract.CheckRequest) (*Job, error) {
	if m.store == nil {
		return nil, fmt.Errorf("%w: store is not configured", ErrInvalidRequest)
	}
	if m.checker == nil {
		return nil, fmt.Errorf("%w: checker is not configured", ErrInvalidRequest)
	}

	text := strings.TrimSpace(request.Text)
	if text == "" {
		return nil, fmt.Errorf("%w: text is required", ErrInvalidRequest)
	}

	threshold := defaultThreshold
	if request.Threshold != nil {
		threshold = *request.Threshold
	}
	if threshold < 0 || threshold > 1 {
		return nil, fmt.Errorf("%w: threshold must be between 0 and 1", ErrInvalidRequest)
	}

	topK := defaultTopK
	if request.TopK != nil {
		topK = *request.TopK
	}
	if topK < 1 || topK > 20 {
		return nil, fmt.Errorf("%w: top_k must be between 1 and 20", ErrInvalidRequest)
	}

	job, err := m.store.Create(ctx, CreateParams{
		RequestID: strings.TrimSpace(requestID),
		Text:      text,
		Threshold: threshold,
		TopK:      topK,
	})
	if err != nil {
		return nil, err
	}

	checkRequest := apicontract.CheckRequest{
		Text:      text,
		Threshold: &threshold,
		TopK:      &topK,
	}

	processCtx, processCancel := context.WithTimeout(context.Background(), m.processTimeout)
	m.setCancelFunc(job.ID, processCancel)

	go m.process(job.ID, checkRequest, processCtx, processCancel)

	return job, nil
}

func (m *Manager) Get(ctx context.Context, jobID string) (*Job, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("%w: job_id is required", ErrInvalidRequest)
	}
	if m.store == nil {
		return nil, fmt.Errorf("%w: store is not configured", ErrInvalidRequest)
	}

	return m.store.GetByID(ctx, jobID)
}

func (m *Manager) Cancel(ctx context.Context, jobID string) error {
	trimmedJobID := strings.TrimSpace(jobID)
	if trimmedJobID == "" {
		return fmt.Errorf("%w: job_id is required", ErrInvalidRequest)
	}
	if m.store == nil {
		return fmt.Errorf("%w: store is not configured", ErrInvalidRequest)
	}

	cancelFunc := m.getCancelFunc(trimmedJobID)
	if cancelFunc != nil {
		cancelFunc()
	}

	reason := "Canceled by user."
	if err := m.store.MarkCanceled(ctx, trimmedJobID, reason, time.Now().UTC()); err != nil {
		return err
	}

	return nil
}

func (m *Manager) process(jobID string, request apicontract.CheckRequest, processCtx context.Context, processCancel context.CancelFunc) {
	defer processCancel()
	defer m.clearCancelFunc(jobID)

	runningCtx, runningCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := m.store.MarkRunning(runningCtx, jobID, time.Now().UTC())
	runningCancel()
	if err != nil {
		if errors.Is(err, ErrInvalidTransition) || errors.Is(err, ErrNotFound) {
			return
		}

		failureCtx, failureCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = m.store.MarkFailed(failureCtx, jobID, err.Error(), time.Now().UTC())
		failureCancel()
		return
	}

	response, err := m.checker.Check(processCtx, request)
	completedAt := time.Now().UTC()
	if err != nil {
		if errors.Is(processCtx.Err(), context.Canceled) {
			cancelCtx, cancelCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = m.store.MarkCanceled(cancelCtx, jobID, "Canceled by user.", completedAt)
			cancelCancel()
			return
		}

		failureCtx, failureCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = m.store.MarkFailed(failureCtx, jobID, err.Error(), completedAt)
		failureCancel()
		return
	}

	if response == nil {
		failureCtx, failureCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = m.store.MarkFailed(failureCtx, jobID, "empty response from checker", completedAt)
		failureCancel()
		return
	}

	completeCtx, completeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = m.store.MarkCompleted(completeCtx, jobID, *response, completedAt)
	completeCancel()
	if err != nil {
		if errors.Is(err, ErrInvalidTransition) {
			return
		}

		failureCtx, failureCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = m.store.MarkFailed(failureCtx, jobID, err.Error(), completedAt)
		failureCancel()
	}
}

func (m *Manager) setCancelFunc(jobID string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelByJobID[jobID] = cancel
}

func (m *Manager) getCancelFunc(jobID string) context.CancelFunc {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cancelByJobID[jobID]
}

func (m *Manager) clearCancelFunc(jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancelByJobID, jobID)
}
