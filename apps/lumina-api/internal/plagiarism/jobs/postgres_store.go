package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lumina/lumina/apps/lumina-api/internal/clients/lumina_engine/apicontract"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("postgres store is not configured")
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS scan_jobs (
			id UUID PRIMARY KEY,
			request_id TEXT,
			text TEXT NOT NULL,
			threshold REAL NOT NULL,
			top_k INTEGER NOT NULL,
			status TEXT NOT NULL,
			overall_score REAL,
			is_plagiarism BOOLEAN,
			error_message TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS scan_matches (
			job_id UUID NOT NULL REFERENCES scan_jobs(id) ON DELETE CASCADE,
			rank INTEGER NOT NULL,
			document_id TEXT NOT NULL,
			source TEXT,
			chunk_id TEXT NOT NULL,
			matched_text TEXT NOT NULL,
			semantic_score REAL NOT NULL,
			exact_score REAL NOT NULL,
			final_score REAL NOT NULL,
			PRIMARY KEY (job_id, rank)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_scan_jobs_status_created_at ON scan_jobs(status, created_at DESC)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}

	return nil
}

func (s *PostgresStore) Create(ctx context.Context, params CreateParams) (*Job, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("postgres store is not configured")
	}

	jobID := uuid.NewString()

	var createdAt time.Time
	var requestID any
	if strings.TrimSpace(params.RequestID) == "" {
		requestID = nil
	} else {
		requestID = params.RequestID
	}

	const query = `
		INSERT INTO scan_jobs (id, request_id, text, threshold, top_k, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at
	`
	if err := s.db.QueryRowContext(ctx, query, jobID, requestID, params.Text, params.Threshold, params.TopK, StatusPending).Scan(&createdAt); err != nil {
		return nil, fmt.Errorf("insert scan job: %w", err)
	}

	return &Job{
		ID:        jobID,
		RequestID: params.RequestID,
		Text:      params.Text,
		Threshold: params.Threshold,
		TopK:      params.TopK,
		Status:    StatusPending,
		CreatedAt: createdAt,
	}, nil
}

func (s *PostgresStore) MarkRunning(ctx context.Context, jobID string, startedAt time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("postgres store is not configured")
	}

	const query = `
		UPDATE scan_jobs
		SET status = $2,
		    started_at = COALESCE(started_at, $3)
		WHERE id = $1
		  AND status = $4
	`

	result, err := s.db.ExecContext(ctx, query, jobID, StatusRunning, startedAt, StatusPending)
	if err != nil {
		return fmt.Errorf("mark running: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark running rows affected: %w", err)
	}
	if rowsAffected > 0 {
		return nil
	}

	status, err := s.currentStatus(ctx, jobID)
	if err != nil {
		return err
	}

	return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, status, StatusRunning)
}

func (s *PostgresStore) MarkCompleted(ctx context.Context, jobID string, response apicontract.CheckResponse, completedAt time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("postgres store is not configured")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const updateJobQuery = `
		UPDATE scan_jobs
		SET status = $2,
		    overall_score = $3,
		    is_plagiarism = $4,
		    error_message = NULL,
		    completed_at = $5
		WHERE id = $1
		  AND status = $6
	`
	result, err := tx.ExecContext(ctx, updateJobQuery, jobID, StatusCompleted, response.OverallScore, response.IsPlagiarism, completedAt, StatusRunning)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark completed rows affected: %w", err)
	}
	if rowsAffected == 0 {
		status, statusErr := s.currentStatus(ctx, jobID)
		if statusErr != nil {
			return statusErr
		}
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, status, StatusCompleted)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM scan_matches WHERE job_id = $1`, jobID); err != nil {
		return fmt.Errorf("clear matches: %w", err)
	}

	const insertMatchQuery = `
		INSERT INTO scan_matches (
			job_id, rank, document_id, source, chunk_id, matched_text,
			semantic_score, exact_score, final_score
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	for index, match := range response.Matches {
		rank := index + 1
		if _, err := tx.ExecContext(
			ctx,
			insertMatchQuery,
			jobID,
			rank,
			match.DocumentId,
			match.Source,
			match.ChunkId,
			match.MatchedText,
			match.SemanticScore,
			match.ExactScore,
			match.FinalScore,
		); err != nil {
			return fmt.Errorf("insert match rank=%d: %w", rank, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (s *PostgresStore) MarkFailed(ctx context.Context, jobID, errorMessage string, completedAt time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("postgres store is not configured")
	}

	const query = `
		WITH target AS (
			SELECT status
			FROM scan_jobs
			WHERE id = $1
			FOR UPDATE
		), updated AS (
			UPDATE scan_jobs
			SET status = $2,
			    error_message = $3,
			    completed_at = $4
			WHERE id = $1
			  AND status IN ($5, $6)
			RETURNING status
		)
		SELECT
			EXISTS(SELECT 1 FROM updated),
			COALESCE((SELECT status FROM updated), (SELECT status FROM target))
	`

	var (
		updated bool
		status  string
	)

	err := s.db.QueryRowContext(ctx, query, jobID, StatusFailed, strings.TrimSpace(errorMessage), completedAt, StatusPending, StatusRunning).Scan(&updated, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("mark failed: %w", err)
	}

	if updated || status == StatusFailed {
		return nil
	}

	return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, status, StatusFailed)
}

func (s *PostgresStore) MarkCanceled(ctx context.Context, jobID, reason string, completedAt time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("postgres store is not configured")
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Canceled by user."
	}

	const query = `
		WITH target AS (
			SELECT status
			FROM scan_jobs
			WHERE id = $1
			FOR UPDATE
		), updated AS (
			UPDATE scan_jobs
			SET status = $2,
			    error_message = $3,
			    completed_at = $4
			WHERE id = $1
			  AND status IN ($5, $6)
			RETURNING status
		)
		SELECT
			EXISTS(SELECT 1 FROM updated),
			COALESCE((SELECT status FROM updated), (SELECT status FROM target))
	`

	var (
		updated bool
		status  string
	)

	err := s.db.QueryRowContext(ctx, query, jobID, StatusCanceled, reason, completedAt, StatusPending, StatusRunning).Scan(&updated, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("mark canceled: %w", err)
	}

	if updated || status == StatusCanceled {
		return nil
	}

	return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, status, StatusCanceled)
}

func (s *PostgresStore) CleanupTerminalOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("postgres store is not configured")
	}

	const query = `
		DELETE FROM scan_jobs
		WHERE status IN ($1, $2, $3)
		  AND completed_at IS NOT NULL
		  AND completed_at < $4
	`

	result, err := s.db.ExecContext(ctx, query, StatusCompleted, StatusFailed, StatusCanceled, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleanup terminal jobs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cleanup rows affected: %w", err)
	}

	return rowsAffected, nil
}
func (s *PostgresStore) GetByID(ctx context.Context, jobID string) (*Job, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("postgres store is not configured")
	}

	const jobQuery = `
		SELECT
			id,
			COALESCE(request_id, ''),
			text,
			threshold,
			top_k,
			status,
			overall_score,
			is_plagiarism,
			error_message,
			created_at,
			started_at,
			completed_at
		FROM scan_jobs
		WHERE id = $1
	`

	var (
		job                    Job
		overallScore           sql.NullFloat64
		isPlagiarism           sql.NullBool
		errorMessage           sql.NullString
		startedAt, completedAt sql.NullTime
	)

	if err := s.db.QueryRowContext(ctx, jobQuery, jobID).Scan(
		&job.ID,
		&job.RequestID,
		&job.Text,
		&job.Threshold,
		&job.TopK,
		&job.Status,
		&overallScore,
		&isPlagiarism,
		&errorMessage,
		&job.CreatedAt,
		&startedAt,
		&completedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query scan job: %w", err)
	}

	if overallScore.Valid {
		value := float32(overallScore.Float64)
		job.OverallScore = &value
	}
	if isPlagiarism.Valid {
		value := isPlagiarism.Bool
		job.IsPlagiarism = &value
	}
	if errorMessage.Valid {
		value := errorMessage.String
		job.ErrorMessage = &value
	}
	if startedAt.Valid {
		value := startedAt.Time
		job.StartedAt = &value
	}
	if completedAt.Valid {
		value := completedAt.Time
		job.CompletedAt = &value
	}

	const matchesQuery = `
		SELECT document_id, source, chunk_id, matched_text, semantic_score, exact_score, final_score
		FROM scan_matches
		WHERE job_id = $1
		ORDER BY rank ASC
	`

	rows, err := s.db.QueryContext(ctx, matchesQuery, jobID)
	if err != nil {
		return nil, fmt.Errorf("query matches: %w", err)
	}
	defer rows.Close()

	matches := make([]apicontract.MatchResult, 0)
	for rows.Next() {
		var (
			match         apicontract.MatchResult
			source        sql.NullString
			semanticScore float64
			exactScore    float64
			finalScore    float64
		)

		if err := rows.Scan(
			&match.DocumentId,
			&source,
			&match.ChunkId,
			&match.MatchedText,
			&semanticScore,
			&exactScore,
			&finalScore,
		); err != nil {
			return nil, fmt.Errorf("scan match: %w", err)
		}

		if source.Valid {
			sourceValue := source.String
			match.Source = &sourceValue
		}
		match.SemanticScore = float32(semanticScore)
		match.ExactScore = float32(exactScore)
		match.FinalScore = float32(finalScore)
		matches = append(matches, match)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate matches: %w", err)
	}

	job.Matches = matches
	return &job, nil
}

func (s *PostgresStore) currentStatus(ctx context.Context, jobID string) (string, error) {
	const query = `SELECT status FROM scan_jobs WHERE id = $1`

	var status string
	if err := s.db.QueryRowContext(ctx, query, jobID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("query current status: %w", err)
	}

	return status, nil
}
