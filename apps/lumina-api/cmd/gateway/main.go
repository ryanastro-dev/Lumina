package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lumina/lumina/apps/lumina-api/internal/clients/lumina_engine"
	"github.com/lumina/lumina/apps/lumina-api/internal/clients/lumina_engine/apicontract"
	"github.com/lumina/lumina/apps/lumina-api/internal/plagiarism/jobs"
)

type healthPayload struct {
	Status       string `json:"status"`
	Service      string `json:"service"`
	EngineStatus string `json:"engine_status,omitempty"`
	Error        string `json:"error,omitempty"`
}

type cleanupHealthPayload struct {
	Status  string               `json:"status"`
	Service string               `json:"service"`
	Cleanup jobs.CleanupSnapshot `json:"cleanup"`
}

type cleanupSnapshotProvider interface {
	Snapshot() jobs.CleanupSnapshot
}

type checkJobAcceptedResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

type checkJobStatusResponse struct {
	JobID       string                     `json:"job_id"`
	Status      string                     `json:"status"`
	CreatedAt   time.Time                  `json:"created_at"`
	StartedAt   *time.Time                 `json:"started_at,omitempty"`
	CompletedAt *time.Time                 `json:"completed_at,omitempty"`
	Error       *string                    `json:"error,omitempty"`
	Result      *apicontract.CheckResponse `json:"result,omitempty"`
}

type checkJobManager interface {
	Submit(ctx context.Context, requestID string, request apicontract.CheckRequest) (*jobs.Job, error)
	Get(ctx context.Context, jobID string) (*jobs.Job, error)
	Cancel(ctx context.Context, jobID string) error
}

const (
	defaultPort                      = "8080"
	defaultEngineBaseURL             = "http://localhost:8000"
	defaultEngineTimeout             = 120
	defaultJobRetentionHours         = 168
	defaultJobCleanupIntervalMinutes = 60
	defaultDBMaxOpenConns            = 25
	defaultDBMaxIdleConns            = 10
	defaultDBConnMaxLifetimeMins     = 5
	maxPDFUploadBytes                = 10 << 20 // 10 MiB
	maxJSONBodyBytes                 = 2 << 20  // 2 MiB

	apiKeyHeader        = "X-API-Key"
	protectedPathPrefix = "/v1/"

	checkJobBasePath = "/v1/plagiarism/check-jobs"
)

func main() {
	port := envOrDefault("PORT", defaultPort)
	engineBaseURL := envOrDefault("LUMINA_ENGINE_BASE_URL", "")
	if engineBaseURL == "" {
		engineBaseURL = envOrDefault("AI_PROCESSING_BASE_URL", defaultEngineBaseURL)
	}

	apiKey := strings.TrimSpace(os.Getenv("LUMINA_API_KEY"))
	engineTimeoutSeconds := envIntOrDefault("LUMINA_ENGINE_TIMEOUT_SECONDS", defaultEngineTimeout)
	jobRetentionHours := envIntOrDefault("LUMINA_JOB_RETENTION_HOURS", defaultJobRetentionHours)
	jobCleanupIntervalMinutes := envIntOrDefault("LUMINA_JOB_CLEANUP_INTERVAL_MINUTES", defaultJobCleanupIntervalMinutes)
	dbMaxOpenConns := envIntOrDefault("LUMINA_DB_MAX_OPEN_CONNS", defaultDBMaxOpenConns)
	dbMaxIdleConns := envIntOrDefault("LUMINA_DB_MAX_IDLE_CONNS", defaultDBMaxIdleConns)
	dbConnMaxLifetimeMinutes := envIntOrDefault("LUMINA_DB_CONN_MAX_LIFETIME_MINUTES", defaultDBConnMaxLifetimeMins)
	if dbMaxIdleConns > dbMaxOpenConns {
		dbMaxIdleConns = dbMaxOpenConns
	}

	engineClient, err := lumina_engine.NewClient(engineBaseURL, &http.Client{Timeout: time.Duration(engineTimeoutSeconds) * time.Second})
	if err != nil {
		log.Fatalf("init lumina-engine client: %v", err)
	}

	postgresDSN := strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	if postgresDSN == "" {
		log.Fatalf("POSTGRES_DSN is required")
	}

	db, err := sql.Open("pgx", postgresDSN)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	db.SetMaxOpenConns(dbMaxOpenConns)
	db.SetMaxIdleConns(dbMaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(dbConnMaxLifetimeMinutes) * time.Minute)
	defer db.Close()

	startupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := db.PingContext(startupCtx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}

	jobStore := jobs.NewPostgresStore(db)
	if err := jobStore.EnsureSchema(startupCtx); err != nil {
		log.Fatalf("ensure postgres schema: %v", err)
	}

	jobManager := jobs.NewManager(jobStore, engineClient, time.Duration(engineTimeoutSeconds)*time.Second)

	cleanupMonitor := jobs.StartCleanupLoop(
		context.Background(),
		jobStore,
		time.Duration(jobRetentionHours)*time.Hour,
		time.Duration(jobCleanupIntervalMinutes)*time.Minute,
		log.Default(),
	)
	cleanupSnapshot := cleanupMonitor.Snapshot()

	handler := newServerHandlerWithJobsAndCleanup(engineClient, jobManager, cleanupMonitor, apiKey)

	addr := ":" + port
	log.Printf("lumina-api listening on %s", addr)
	log.Printf("lumina-engine upstream: %s", engineBaseURL)
	log.Printf("lumina-engine timeout: %ds", engineTimeoutSeconds)
	log.Printf("postgres persistence: enabled")
	log.Printf("postgres pool: max_open=%d max_idle=%d conn_max_lifetime=%dm", dbMaxOpenConns, dbMaxIdleConns, dbConnMaxLifetimeMinutes)
	if cleanupSnapshot.Enabled {
		log.Printf("job cleanup: enabled (retention=%ds interval=%ds)", cleanupSnapshot.RetentionSeconds, cleanupSnapshot.IntervalSeconds)
	} else {
		log.Printf("job cleanup: disabled (%s)", cleanupSnapshot.Reason)
	}
	if apiKey == "" {
		log.Printf("api key auth: disabled")
	} else {
		log.Printf("api key auth: enabled for %s*", protectedPathPrefix)
	}
	log.Fatal(http.ListenAndServe(addr, handler))
}

func newServerHandler(engineClient lumina_engine.Client, apiKey string) http.Handler {
	return newServerHandlerWithJobs(engineClient, nil, apiKey)
}

func newServerHandlerWithJobs(engineClient lumina_engine.Client, jobManager checkJobManager, apiKey string) http.Handler {
	return newServerHandlerWithJobsAndCleanup(engineClient, jobManager, nil, apiKey)
}

func newServerHandlerWithJobsAndCleanup(engineClient lumina_engine.Client, jobManager checkJobManager, cleanupProvider cleanupSnapshotProvider, apiKey string) http.Handler {
	handler := http.Handler(newServerMux(engineClient, jobManager, cleanupProvider))
	handler = withOptionalAPIKey(handler, apiKey)
	handler = withRequestID(handler)
	return handler
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(lumina_engine.HeaderRequestID))
		if requestID == "" {
			requestID = newRequestID()
		}

		w.Header().Set(lumina_engine.HeaderRequestID, requestID)

		ctx := lumina_engine.WithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withOptionalAPIKey(next http.Handler, apiKey string) http.Handler {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, protectedPathPrefix) {
			next.ServeHTTP(w, r)
			return
		}

		provided := strings.TrimSpace(r.Header.Get(apiKeyHeader))
		if !constantTimeEqual(provided, trimmed) {
			writeJSON(w, http.StatusUnauthorized, apicontract.ErrorResponse{
				Error:   "unauthorized",
				Message: "Missing or invalid API key.",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func newRequestID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err == nil {
		return hex.EncodeToString(data[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func newServerMux(engineClient lumina_engine.Client, jobManager checkJobManager, cleanupProvider cleanupSnapshotProvider) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthPayload{Status: "ok", Service: "lumina-api"})
	})

	mux.HandleFunc("/health/cleanup", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := jobs.CleanupSnapshot{Enabled: false, Reason: "job cleanup is not configured"}
		if cleanupProvider != nil {
			snapshot = cleanupProvider.Snapshot()
		}

		statusCode := http.StatusOK
		status := "ok"
		switch {
		case !snapshot.Enabled:
			status = "disabled"
		case snapshot.LastError != "":
			status = "degraded"
			statusCode = http.StatusServiceUnavailable
		}

		writeJSON(w, statusCode, cleanupHealthPayload{
			Status:  status,
			Service: "lumina-api",
			Cleanup: snapshot,
		})
	})

	mux.HandleFunc("/health/engine", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		res, err := engineClient.Health(ctx)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, healthPayload{
				Status:  "degraded",
				Service: "lumina-api",
				Error:   err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, healthPayload{
			Status:       "ok",
			Service:      "lumina-api",
			EngineStatus: res.Status,
		})
	})

	mux.HandleFunc("/v1/plagiarism/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apicontract.ErrorResponse{
				Error:   "method_not_allowed",
				Message: "Use POST for this endpoint.",
			})
			return
		}

		var request apicontract.IngestRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
				Error:   "invalid_json",
				Message: "Invalid request body.",
				Details: &[]map[string]interface{}{{"reason": err.Error()}},
			})
			return
		}

		response, err := engineClient.Ingest(r.Context(), request)
		if err != nil {
			writeUpstreamError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, response)
	})

	mux.HandleFunc(checkJobBasePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apicontract.ErrorResponse{
				Error:   "method_not_allowed",
				Message: "Use POST for this endpoint.",
			})
			return
		}

		if jobManager == nil {
			writeJSON(w, http.StatusServiceUnavailable, apicontract.ErrorResponse{
				Error:   "feature_unavailable",
				Message: "Async check jobs are not configured.",
			})
			return
		}

		var request apicontract.CheckRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
				Error:   "invalid_json",
				Message: "Invalid request body.",
				Details: &[]map[string]interface{}{{"reason": err.Error()}},
			})
			return
		}

		requestID, _ := lumina_engine.RequestIDFromContext(r.Context())
		job, err := jobManager.Submit(r.Context(), requestID, request)
		if err != nil {
			switch {
			case errors.Is(err, jobs.ErrInvalidRequest):
				writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
					Error:   "invalid_request",
					Message: err.Error(),
				})
			default:
				writeJSON(w, http.StatusInternalServerError, apicontract.ErrorResponse{
					Error:   "job_submit_failed",
					Message: "Unable to submit check job.",
					Details: &[]map[string]interface{}{{"reason": err.Error()}},
				})
			}
			return
		}

		writeJSON(w, http.StatusAccepted, checkJobAcceptedResponse{
			JobID:  job.ID,
			Status: job.Status,
		})
	})

	mux.HandleFunc(checkJobBasePath+"/", func(w http.ResponseWriter, r *http.Request) {
		if jobManager == nil {
			writeJSON(w, http.StatusServiceUnavailable, apicontract.ErrorResponse{
				Error:   "feature_unavailable",
				Message: "Async check jobs are not configured.",
			})
			return
		}

		pathSuffix := strings.Trim(strings.TrimPrefix(r.URL.Path, checkJobBasePath+"/"), "/")
		if pathSuffix == "" {
			writeJSON(w, http.StatusNotFound, apicontract.ErrorResponse{
				Error:   "not_found",
				Message: "Check job not found.",
			})
			return
		}

		segments := strings.Split(pathSuffix, "/")
		if len(segments) == 1 {
			if r.Method != http.MethodGet {
				writeJSON(w, http.StatusMethodNotAllowed, apicontract.ErrorResponse{
					Error:   "method_not_allowed",
					Message: "Use GET for this endpoint.",
				})
				return
			}

			jobID := strings.TrimSpace(segments[0])
			if jobID == "" {
				writeJSON(w, http.StatusNotFound, apicontract.ErrorResponse{
					Error:   "not_found",
					Message: "Check job not found.",
				})
				return
			}

			job, err := jobManager.Get(r.Context(), jobID)
			if err != nil {
				switch {
				case errors.Is(err, jobs.ErrNotFound):
					writeJSON(w, http.StatusNotFound, apicontract.ErrorResponse{
						Error:   "not_found",
						Message: "Check job not found.",
					})
				case errors.Is(err, jobs.ErrInvalidRequest):
					writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
						Error:   "invalid_request",
						Message: err.Error(),
					})
				default:
					writeJSON(w, http.StatusInternalServerError, apicontract.ErrorResponse{
						Error:   "job_read_failed",
						Message: "Unable to read check job status.",
						Details: &[]map[string]interface{}{{"reason": err.Error()}},
					})
				}
				return
			}

			writeJSON(w, http.StatusOK, mapCheckJobStatusResponse(job))
			return
		}

		if len(segments) == 2 && segments[1] == "cancel" {
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, apicontract.ErrorResponse{
					Error:   "method_not_allowed",
					Message: "Use POST for this endpoint.",
				})
				return
			}

			jobID := strings.TrimSpace(segments[0])
			if jobID == "" {
				writeJSON(w, http.StatusNotFound, apicontract.ErrorResponse{
					Error:   "not_found",
					Message: "Check job not found.",
				})
				return
			}

			if err := jobManager.Cancel(r.Context(), jobID); err != nil {
				switch {
				case errors.Is(err, jobs.ErrNotFound):
					writeJSON(w, http.StatusNotFound, apicontract.ErrorResponse{
						Error:   "not_found",
						Message: "Check job not found.",
					})
				case errors.Is(err, jobs.ErrInvalidRequest):
					writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
						Error:   "invalid_request",
						Message: err.Error(),
					})
				case errors.Is(err, jobs.ErrInvalidTransition):
					writeJSON(w, http.StatusConflict, apicontract.ErrorResponse{
						Error:   "job_not_cancelable",
						Message: err.Error(),
					})
				default:
					writeJSON(w, http.StatusInternalServerError, apicontract.ErrorResponse{
						Error:   "job_cancel_failed",
						Message: "Unable to cancel check job.",
						Details: &[]map[string]interface{}{{"reason": err.Error()}},
					})
				}
				return
			}

			writeJSON(w, http.StatusAccepted, checkJobAcceptedResponse{
				JobID:  jobID,
				Status: jobs.StatusCanceled,
			})
			return
		}

		writeJSON(w, http.StatusNotFound, apicontract.ErrorResponse{
			Error:   "not_found",
			Message: "Check job not found.",
		})
	})

	mux.HandleFunc("/v1/plagiarism/check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apicontract.ErrorResponse{
				Error:   "method_not_allowed",
				Message: "Use POST for this endpoint.",
			})
			return
		}

		var request apicontract.CheckRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
				Error:   "invalid_json",
				Message: "Invalid request body.",
				Details: &[]map[string]interface{}{{"reason": err.Error()}},
			})
			return
		}

		response, err := engineClient.Check(r.Context(), request)
		if err != nil {
			writeUpstreamError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, response)
	})

	mux.HandleFunc("/v1/pdf/extract", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apicontract.ErrorResponse{
				Error:   "method_not_allowed",
				Message: "Use POST for this endpoint.",
			})
			return
		}

		if err := r.ParseMultipartForm(maxPDFUploadBytes); err != nil {
			writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
				Error:   "invalid_multipart",
				Message: "Invalid multipart/form-data payload.",
				Details: &[]map[string]interface{}{{"reason": err.Error()}},
			})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			errorCode := "missing_file"
			message := "Multipart field `file` is required."
			if !errors.Is(err, http.ErrMissingFile) {
				errorCode = "invalid_multipart"
				message = "Invalid multipart/form-data payload."
			}
			writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
				Error:   errorCode,
				Message: message,
				Details: &[]map[string]interface{}{{"reason": err.Error()}},
			})
			return
		}
		defer file.Close()

		content, err := io.ReadAll(io.LimitReader(file, maxPDFUploadBytes+1))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
				Error:   "file_read_error",
				Message: "Unable to read uploaded file.",
				Details: &[]map[string]interface{}{{"reason": err.Error()}},
			})
			return
		}
		if len(content) == 0 {
			writeJSON(w, http.StatusBadRequest, apicontract.ErrorResponse{
				Error:   "empty_file",
				Message: "Uploaded file is empty.",
			})
			return
		}
		if len(content) > maxPDFUploadBytes {
			writeJSON(w, http.StatusRequestEntityTooLarge, apicontract.ErrorResponse{
				Error:   "file_too_large",
				Message: "Uploaded file exceeds 10 MiB limit.",
			})
			return
		}

		filename := "upload.pdf"
		if header != nil && header.Filename != "" {
			filename = header.Filename
		}

		response, err := engineClient.ExtractPDF(r.Context(), filename, content)
		if err != nil {
			writeUpstreamError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, response)
	})

	return mux
}

func mapCheckJobStatusResponse(job *jobs.Job) checkJobStatusResponse {
	response := checkJobStatusResponse{
		JobID:       job.ID,
		Status:      job.Status,
		CreatedAt:   job.CreatedAt,
		StartedAt:   job.StartedAt,
		CompletedAt: job.CompletedAt,
		Error:       job.ErrorMessage,
	}

	if job.OverallScore != nil && job.IsPlagiarism != nil {
		result := apicontract.CheckResponse{
			IsPlagiarism: *job.IsPlagiarism,
			OverallScore: *job.OverallScore,
			Threshold:    job.Threshold,
			Matches:      job.Matches,
		}
		response.Result = &result
	}

	return response
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, out any) error {
	if w != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err != nil {
			return err
		}
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envIntOrDefault(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}

	return value
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	var apiErr *lumina_engine.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.StatusCode
		if status == 0 {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, apiErr.Response)
		return
	}

	writeJSON(w, http.StatusBadGateway, apicontract.ErrorResponse{
		Error:   "upstream_request_failed",
		Message: err.Error(),
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("encode response error: %v", err)
	}
}
