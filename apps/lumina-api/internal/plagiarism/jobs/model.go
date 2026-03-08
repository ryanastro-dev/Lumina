package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/lumina/lumina/apps/lumina-api/internal/clients/lumina_engine/apicontract"
)

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

var (
	ErrNotFound          = errors.New("job not found")
	ErrInvalidRequest    = errors.New("invalid request")
	ErrInvalidTransition = errors.New("invalid job transition")
)

type Job struct {
	ID           string
	RequestID    string
	Text         string
	Threshold    float32
	TopK         int
	Status       string
	OverallScore *float32
	IsPlagiarism *bool
	ErrorMessage *string
	CreatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
	Matches      []apicontract.MatchResult
}

type CreateParams struct {
	RequestID string
	Text      string
	Threshold float32
	TopK      int
}

type Store interface {
	EnsureSchema(ctx context.Context) error
	Create(ctx context.Context, params CreateParams) (*Job, error)
	MarkRunning(ctx context.Context, jobID string, startedAt time.Time) error
	MarkCompleted(ctx context.Context, jobID string, response apicontract.CheckResponse, completedAt time.Time) error
	MarkFailed(ctx context.Context, jobID, errorMessage string, completedAt time.Time) error
	MarkCanceled(ctx context.Context, jobID, reason string, completedAt time.Time) error
	CleanupTerminalOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	GetByID(ctx context.Context, jobID string) (*Job, error)
}
