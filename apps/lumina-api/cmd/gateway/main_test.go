package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lumina/lumina/apps/lumina-api/internal/clients/lumina_engine"
	"github.com/lumina/lumina/apps/lumina-api/internal/clients/lumina_engine/apicontract"
	"github.com/lumina/lumina/apps/lumina-api/internal/plagiarism/jobs"
)

type fakeEngineClient struct {
	healthFn     func(context.Context) (*apicontract.HealthResponse, error)
	ingestFn     func(context.Context, apicontract.IngestRequest) (*apicontract.IngestResponse, error)
	checkFn      func(context.Context, apicontract.CheckRequest) (*apicontract.CheckResponse, error)
	extractPDFFn func(context.Context, string, []byte) (*apicontract.PdfExtractResponse, error)
}

type fakeCheckJobManager struct {
	submitFn func(context.Context, string, apicontract.CheckRequest) (*jobs.Job, error)
	getFn    func(context.Context, string) (*jobs.Job, error)
	cancelFn func(context.Context, string) error
}

type fakeCleanupSnapshotProvider struct {
	snapshot jobs.CleanupSnapshot
}

func (f fakeCleanupSnapshotProvider) Snapshot() jobs.CleanupSnapshot {
	return f.snapshot
}

func (f fakeCheckJobManager) Submit(ctx context.Context, requestID string, request apicontract.CheckRequest) (*jobs.Job, error) {
	if f.submitFn != nil {
		return f.submitFn(ctx, requestID, request)
	}
	return &jobs.Job{ID: "job-default", Status: jobs.StatusPending, CreatedAt: time.Now().UTC()}, nil
}

func (f fakeCheckJobManager) Get(ctx context.Context, jobID string) (*jobs.Job, error) {
	if f.getFn != nil {
		return f.getFn(ctx, jobID)
	}
	return &jobs.Job{ID: jobID, Status: jobs.StatusPending, CreatedAt: time.Now().UTC()}, nil
}

func (f fakeCheckJobManager) Cancel(ctx context.Context, jobID string) error {
	if f.cancelFn != nil {
		return f.cancelFn(ctx, jobID)
	}
	return nil
}

func (f fakeEngineClient) Health(ctx context.Context) (*apicontract.HealthResponse, error) {
	if f.healthFn != nil {
		return f.healthFn(ctx)
	}
	return &apicontract.HealthResponse{Status: "ok"}, nil
}

func (f fakeEngineClient) Ingest(ctx context.Context, req apicontract.IngestRequest) (*apicontract.IngestResponse, error) {
	if f.ingestFn != nil {
		return f.ingestFn(ctx, req)
	}
	return &apicontract.IngestResponse{}, nil
}

func (f fakeEngineClient) Check(ctx context.Context, req apicontract.CheckRequest) (*apicontract.CheckResponse, error) {
	if f.checkFn != nil {
		return f.checkFn(ctx, req)
	}
	return &apicontract.CheckResponse{IsPlagiarism: false, OverallScore: 0, Threshold: 0, Matches: []apicontract.MatchResult{}}, nil
}

func (f fakeEngineClient) ExtractPDF(ctx context.Context, filename string, content []byte) (*apicontract.PdfExtractResponse, error) {
	if f.extractPDFFn != nil {
		return f.extractPDFFn(ctx, filename, content)
	}
	return &apicontract.PdfExtractResponse{}, nil
}

func newCheckResponse() *apicontract.CheckResponse {
	return &apicontract.CheckResponse{
		IsPlagiarism: false,
		OverallScore: 0.1,
		Threshold:    0.8,
		Matches:      []apicontract.MatchResult{},
	}
}

func TestPlagiarismIngestProxySuccess(t *testing.T) {
	var seenText string
	handler := newServerHandler(fakeEngineClient{
		ingestFn: func(_ context.Context, req apicontract.IngestRequest) (*apicontract.IngestResponse, error) {
			seenText = req.Text
			return &apicontract.IngestResponse{DocumentId: "doc-1", ChunkCount: 2, Collection: "lumina_documents"}, nil
		},
	}, "")

	server := httptest.NewServer(handler)
	defer server.Close()

	payload := `{"text":"hello world"}`
	resp, err := http.Post(server.URL+"/v1/plagiarism/ingest", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post ingest: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if seenText != "hello world" {
		t.Fatalf("upstream request mismatch, got %q", seenText)
	}

	var out apicontract.IngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if out.DocumentId != "doc-1" || out.ChunkCount != 2 {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestPlagiarismCheckInvalidJSON(t *testing.T) {
	handler := newServerHandler(fakeEngineClient{}, "")
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/plagiarism/check", "application/json", strings.NewReader(`{"text":"a"}{"text":"b"}`))
	if err != nil {
		t.Fatalf("post check: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var out apicontract.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode error response: %v", err)
	}

	if out.Error != "invalid_json" {
		t.Fatalf("unexpected error code: %s", out.Error)
	}
}

func TestCheckJobSubmitAccepted(t *testing.T) {
	var seenRequestID string
	handler := newServerHandlerWithJobs(
		fakeEngineClient{},
		fakeCheckJobManager{
			submitFn: func(_ context.Context, requestID string, request apicontract.CheckRequest) (*jobs.Job, error) {
				seenRequestID = requestID
				if request.Text != "queued text" {
					t.Fatalf("unexpected request text: %s", request.Text)
				}
				return &jobs.Job{ID: "job-123", Status: jobs.StatusPending, CreatedAt: time.Now().UTC()}, nil
			},
		},
		"",
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+checkJobBasePath, strings.NewReader(`{"text":"queued text"}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(lumina_engine.HeaderRequestID, "req-123")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("submit check job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if seenRequestID != "req-123" {
		t.Fatalf("expected propagated request-id, got %q", seenRequestID)
	}

	var out checkJobAcceptedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.JobID != "job-123" || out.Status != jobs.StatusPending {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestCheckJobStatusCompleted(t *testing.T) {
	overallScore := float32(0.91)
	isPlagiarism := true

	handler := newServerHandlerWithJobs(
		fakeEngineClient{},
		fakeCheckJobManager{
			getFn: func(_ context.Context, jobID string) (*jobs.Job, error) {
				return &jobs.Job{
					ID:           jobID,
					Status:       jobs.StatusCompleted,
					CreatedAt:    time.Now().UTC(),
					OverallScore: &overallScore,
					IsPlagiarism: &isPlagiarism,
					Threshold:    0.8,
					Matches: []apicontract.MatchResult{{
						DocumentId:    "doc-1",
						ChunkId:       "chunk-1",
						MatchedText:   "similar sentence",
						SemanticScore: 0.9,
						ExactScore:    0.8,
						FinalScore:    0.86,
					}},
				}, nil
			},
		},
		"",
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + checkJobBasePath + "/job-xyz")
	if err != nil {
		t.Fatalf("get check job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var out checkJobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.JobID != "job-xyz" || out.Status != jobs.StatusCompleted {
		t.Fatalf("unexpected status payload: %+v", out)
	}
	if out.Result == nil {
		t.Fatalf("expected completed job to include result")
	}
	if len(out.Result.Matches) != 1 {
		t.Fatalf("unexpected match count: %d", len(out.Result.Matches))
	}
}

func TestCheckJobStatusNotFound(t *testing.T) {
	handler := newServerHandlerWithJobs(
		fakeEngineClient{},
		fakeCheckJobManager{
			getFn: func(_ context.Context, _ string) (*jobs.Job, error) {
				return nil, jobs.ErrNotFound
			},
		},
		"",
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + checkJobBasePath + "/missing")
	if err != nil {
		t.Fatalf("get check job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var out apicontract.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Error != "not_found" {
		t.Fatalf("unexpected error: %s", out.Error)
	}
}

func TestPDFExtractMissingFile(t *testing.T) {
	handler := newServerHandler(fakeEngineClient{}, "")
	server := httptest.NewServer(handler)
	defer server.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("note", "missing file"); err != nil {
		t.Fatalf("write form field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/pdf/extract", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	var out apicontract.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Error != "missing_file" {
		t.Fatalf("unexpected error code: %s", out.Error)
	}
}

func TestPDFExtractProxySuccess(t *testing.T) {
	var seenFilename string
	var seenBytes int
	handler := newServerHandler(fakeEngineClient{
		extractPDFFn: func(_ context.Context, filename string, content []byte) (*apicontract.PdfExtractResponse, error) {
			seenFilename = filename
			seenBytes = len(content)
			return &apicontract.PdfExtractResponse{
				Filename:       filename,
				PageCount:      1,
				CharacterCount: 12,
				Text:           "hello world!",
			}, nil
		},
	}, "")

	server := httptest.NewServer(handler)
	defer server.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "sample.pdf")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("%PDF-1.4\n")); err != nil {
		t.Fatalf("write file bytes: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/pdf/extract", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	if seenFilename != "sample.pdf" {
		t.Fatalf("unexpected filename: %s", seenFilename)
	}
	if seenBytes == 0 {
		t.Fatalf("expected file bytes to be passed to upstream")
	}

	var out apicontract.PdfExtractResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.PageCount != 1 || out.Text == "" {
		t.Fatalf("unexpected extract response: %+v", out)
	}
}

func TestRequestIDGeneratedAndPropagated(t *testing.T) {
	var seenRequestID string
	handler := newServerHandler(fakeEngineClient{
		checkFn: func(ctx context.Context, _ apicontract.CheckRequest) (*apicontract.CheckResponse, error) {
			requestID, ok := lumina_engine.RequestIDFromContext(ctx)
			if !ok {
				t.Fatalf("expected request-id in context")
			}
			seenRequestID = requestID
			return newCheckResponse(), nil
		},
	}, "")

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/plagiarism/check", "application/json", strings.NewReader(`{"text":"sample"}`))
	if err != nil {
		t.Fatalf("post check: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	responseRequestID := strings.TrimSpace(resp.Header.Get(lumina_engine.HeaderRequestID))
	if responseRequestID == "" {
		t.Fatalf("expected %s response header", lumina_engine.HeaderRequestID)
	}
	if seenRequestID == "" {
		t.Fatalf("expected upstream context request-id")
	}
	if responseRequestID != seenRequestID {
		t.Fatalf("response request-id (%s) != upstream request-id (%s)", responseRequestID, seenRequestID)
	}
}

func TestRequestIDPreservedFromIncomingHeader(t *testing.T) {
	const requestID = "req-test-123"

	var seenRequestID string
	handler := newServerHandler(fakeEngineClient{
		checkFn: func(ctx context.Context, _ apicontract.CheckRequest) (*apicontract.CheckResponse, error) {
			seenRequestID, _ = lumina_engine.RequestIDFromContext(ctx)
			return newCheckResponse(), nil
		},
	}, "")

	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/plagiarism/check", strings.NewReader(`{"text":"sample"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(lumina_engine.HeaderRequestID, requestID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if seenRequestID != requestID {
		t.Fatalf("expected request-id %q, got %q", requestID, seenRequestID)
	}

	responseRequestID := strings.TrimSpace(resp.Header.Get(lumina_engine.HeaderRequestID))
	if responseRequestID != requestID {
		t.Fatalf("expected response request-id %q, got %q", requestID, responseRequestID)
	}
}

func TestAPIKeyProtectsV1Routes(t *testing.T) {
	const apiKey = "secret-test-key"

	handler := newServerHandler(fakeEngineClient{
		checkFn: func(_ context.Context, _ apicontract.CheckRequest) (*apicontract.CheckResponse, error) {
			return newCheckResponse(), nil
		},
	}, apiKey)

	server := httptest.NewServer(handler)
	defer server.Close()

	healthResp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("expected health to be public, got %d", healthResp.StatusCode)
	}

	unauthorizedResp, err := http.Post(server.URL+"/v1/plagiarism/check", "application/json", strings.NewReader(`{"text":"sample"}`))
	if err != nil {
		t.Fatalf("post check without api key: %v", err)
	}
	defer unauthorizedResp.Body.Close()

	if unauthorizedResp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(unauthorizedResp.Body)
		t.Fatalf("expected 401, got %d: %s", unauthorizedResp.StatusCode, string(body))
	}

	var unauthorizedBody apicontract.ErrorResponse
	if err := json.NewDecoder(unauthorizedResp.Body).Decode(&unauthorizedBody); err != nil {
		t.Fatalf("decode unauthorized body: %v", err)
	}
	if unauthorizedBody.Error != "unauthorized" {
		t.Fatalf("unexpected unauthorized error code: %s", unauthorizedBody.Error)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/plagiarism/check", strings.NewReader(`{"text":"sample"}`))
	if err != nil {
		t.Fatalf("new authorized request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(apiKeyHeader, apiKey)

	authorizedResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("perform authorized request: %v", err)
	}
	defer authorizedResp.Body.Close()

	if authorizedResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(authorizedResp.Body)
		t.Fatalf("expected 200 for authorized request, got %d: %s", authorizedResp.StatusCode, string(body))
	}
}

func TestHealthCleanupDisabledWhenNotConfigured(t *testing.T) {
	handler := newServerHandlerWithJobs(fakeEngineClient{}, nil, "")
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health/cleanup")
	if err != nil {
		t.Fatalf("get cleanup health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var out cleanupHealthPayload
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode cleanup health: %v", err)
	}

	if out.Status != "disabled" {
		t.Fatalf("unexpected cleanup status: %q", out.Status)
	}
	if out.Cleanup.Enabled {
		t.Fatalf("expected cleanup to be disabled")
	}
	if out.Cleanup.Reason != "job cleanup is not configured" {
		t.Fatalf("unexpected cleanup reason: %q", out.Cleanup.Reason)
	}
}

func TestHealthCleanupDegradedOnLastError(t *testing.T) {
	runAt := time.Now().UTC()
	handler := newServerHandlerWithJobsAndCleanup(
		fakeEngineClient{},
		nil,
		fakeCleanupSnapshotProvider{snapshot: jobs.CleanupSnapshot{
			Enabled:          true,
			RetentionSeconds: 3600,
			IntervalSeconds:  60,
			LastRunAt:        &runAt,
			LastError:        "db timeout",
			TotalRuns:        2,
			TotalFailures:    1,
		}},
		"",
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health/cleanup")
	if err != nil {
		t.Fatalf("get cleanup health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 503, got %d: %s", resp.StatusCode, string(body))
	}

	var out cleanupHealthPayload
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode cleanup health: %v", err)
	}

	if out.Status != "degraded" {
		t.Fatalf("unexpected cleanup status: %q", out.Status)
	}
	if !out.Cleanup.Enabled {
		t.Fatalf("expected cleanup to be enabled")
	}
	if !strings.Contains(out.Cleanup.LastError, "db timeout") {
		t.Fatalf("expected cleanup error message, got %q", out.Cleanup.LastError)
	}
}

func TestHealthCleanupHealthy(t *testing.T) {
	runAt := time.Now().UTC()
	successAt := runAt.Add(-time.Minute)
	handler := newServerHandlerWithJobsAndCleanup(
		fakeEngineClient{},
		nil,
		fakeCleanupSnapshotProvider{snapshot: jobs.CleanupSnapshot{
			Enabled:          true,
			RetentionSeconds: 3600,
			IntervalSeconds:  60,
			LastRunAt:        &runAt,
			LastSuccessAt:    &successAt,
			LastDeletedCount: 3,
			TotalRuns:        5,
			TotalDeleted:     7,
		}},
		"",
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health/cleanup")
	if err != nil {
		t.Fatalf("get cleanup health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var out cleanupHealthPayload
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode cleanup health: %v", err)
	}

	if out.Status != "ok" {
		t.Fatalf("unexpected cleanup status: %q", out.Status)
	}
	if !out.Cleanup.Enabled {
		t.Fatalf("expected cleanup to be enabled")
	}
	if out.Cleanup.LastError != "" {
		t.Fatalf("unexpected cleanup error: %q", out.Cleanup.LastError)
	}
}

func TestCheckJobCancelAccepted(t *testing.T) {
	var seenJobID string
	handler := newServerHandlerWithJobs(
		fakeEngineClient{},
		fakeCheckJobManager{
			cancelFn: func(_ context.Context, jobID string) error {
				seenJobID = jobID
				return nil
			},
		},
		"",
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+checkJobBasePath+"/job-123/cancel", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("cancel check job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if seenJobID != "job-123" {
		t.Fatalf("unexpected canceled job id: %q", seenJobID)
	}

	var out checkJobAcceptedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.JobID != "job-123" || out.Status != jobs.StatusCanceled {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestCheckJobCancelConflict(t *testing.T) {
	handler := newServerHandlerWithJobs(
		fakeEngineClient{},
		fakeCheckJobManager{
			cancelFn: func(_ context.Context, _ string) error {
				return jobs.ErrInvalidTransition
			},
		},
		"",
	)

	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+checkJobBasePath+"/job-123/cancel", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("cancel check job: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var out apicontract.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Error != "job_not_cancelable" {
		t.Fatalf("unexpected error code: %s", out.Error)
	}
}

func TestCheckJobCancelFlowWithManager(t *testing.T) {
	store := newInMemoryJobStore()
	engine := fakeEngineClient{
		checkFn: func(ctx context.Context, _ apicontract.CheckRequest) (*apicontract.CheckResponse, error) {
			select {
			case <-time.After(500 * time.Millisecond):
				return &apicontract.CheckResponse{IsPlagiarism: false, OverallScore: 0.1, Threshold: 0.8, Matches: []apicontract.MatchResult{}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	manager := jobs.NewManager(store, engine, 3*time.Second)
	handler := newServerHandlerWithJobs(engine, manager, "")
	server := httptest.NewServer(handler)
	defer server.Close()

	submitResp, err := http.Post(server.URL+checkJobBasePath, "application/json", strings.NewReader(`{"text":"cancel flow"}`))
	if err != nil {
		t.Fatalf("submit check job: %v", err)
	}
	defer submitResp.Body.Close()

	if submitResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(submitResp.Body)
		t.Fatalf("unexpected submit status %d: %s", submitResp.StatusCode, string(body))
	}

	var accepted checkJobAcceptedResponse
	if err := json.NewDecoder(submitResp.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}

	cancelReq, err := http.NewRequest(http.MethodPost, server.URL+checkJobBasePath+"/"+accepted.JobID+"/cancel", nil)
	if err != nil {
		t.Fatalf("create cancel request: %v", err)
	}

	cancelResp, err := http.DefaultClient.Do(cancelReq)
	if err != nil {
		t.Fatalf("cancel check job: %v", err)
	}
	defer cancelResp.Body.Close()

	if cancelResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(cancelResp.Body)
		t.Fatalf("unexpected cancel status %d: %s", cancelResp.StatusCode, string(body))
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		statusResp, err := http.Get(server.URL + checkJobBasePath + "/" + accepted.JobID)
		if err != nil {
			t.Fatalf("poll check job: %v", err)
		}

		if statusResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(statusResp.Body)
			statusResp.Body.Close()
			t.Fatalf("unexpected status poll code %d: %s", statusResp.StatusCode, string(body))
		}

		var status checkJobStatusResponse
		if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
			statusResp.Body.Close()
			t.Fatalf("decode status response: %v", err)
		}
		statusResp.Body.Close()

		if status.Status == jobs.StatusCanceled {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for canceled status")
}

type inMemoryJobStore struct {
	mu      sync.Mutex
	counter int
	jobs    map[string]*jobs.Job
}

func newInMemoryJobStore() *inMemoryJobStore {
	return &inMemoryJobStore{jobs: make(map[string]*jobs.Job)}
}

func (s *inMemoryJobStore) EnsureSchema(_ context.Context) error {
	return nil
}

func (s *inMemoryJobStore) Create(_ context.Context, params jobs.CreateParams) (*jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counter++
	jobID := "job-mem-" + strconv.Itoa(s.counter)
	now := time.Now().UTC()

	job := &jobs.Job{
		ID:        jobID,
		RequestID: params.RequestID,
		Text:      params.Text,
		Threshold: params.Threshold,
		TopK:      params.TopK,
		Status:    jobs.StatusPending,
		CreatedAt: now,
		Matches:   []apicontract.MatchResult{},
	}
	s.jobs[jobID] = job

	return copyJob(job), nil
}

func (s *inMemoryJobStore) MarkRunning(_ context.Context, jobID string, startedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.ErrNotFound
	}

	job.Status = jobs.StatusRunning
	job.StartedAt = &startedAt
	return nil
}

func (s *inMemoryJobStore) MarkCompleted(_ context.Context, jobID string, response apicontract.CheckResponse, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.ErrNotFound
	}
	if job.Status != jobs.StatusRunning {
		return jobs.ErrInvalidTransition
	}

	job.Status = jobs.StatusCompleted
	overall := response.OverallScore
	isPlagiarism := response.IsPlagiarism
	job.OverallScore = &overall
	job.IsPlagiarism = &isPlagiarism
	job.CompletedAt = &completedAt
	job.ErrorMessage = nil
	job.Matches = append([]apicontract.MatchResult(nil), response.Matches...)
	return nil
}

func (s *inMemoryJobStore) MarkFailed(_ context.Context, jobID, errorMessage string, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.ErrNotFound
	}
	if job.Status == jobs.StatusFailed {
		return nil
	}
	if job.Status == jobs.StatusCompleted || job.Status == jobs.StatusCanceled {
		return jobs.ErrInvalidTransition
	}

	job.Status = jobs.StatusFailed
	message := errorMessage
	job.ErrorMessage = &message
	job.CompletedAt = &completedAt
	return nil
}

func (s *inMemoryJobStore) MarkCanceled(_ context.Context, jobID, reason string, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return jobs.ErrNotFound
	}
	if job.Status == jobs.StatusCanceled {
		return nil
	}
	if job.Status == jobs.StatusCompleted || job.Status == jobs.StatusFailed {
		return jobs.ErrInvalidTransition
	}

	job.Status = jobs.StatusCanceled
	message := strings.TrimSpace(reason)
	if message == "" {
		message = "Canceled by user."
	}
	job.ErrorMessage = &message
	job.CompletedAt = &completedAt
	return nil
}
func (s *inMemoryJobStore) CleanupTerminalOlderThan(_ context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var deleted int64
	for id, job := range s.jobs {
		if job.CompletedAt == nil {
			continue
		}
		if !job.CompletedAt.Before(cutoff) {
			continue
		}

		switch job.Status {
		case jobs.StatusCompleted, jobs.StatusFailed, jobs.StatusCanceled:
			delete(s.jobs, id)
			deleted++
		}
	}

	return deleted, nil
}
func (s *inMemoryJobStore) GetByID(_ context.Context, jobID string) (*jobs.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return nil, jobs.ErrNotFound
	}

	return copyJob(job), nil
}

func copyJob(input *jobs.Job) *jobs.Job {
	if input == nil {
		return nil
	}

	output := *input
	if input.OverallScore != nil {
		value := *input.OverallScore
		output.OverallScore = &value
	}
	if input.IsPlagiarism != nil {
		value := *input.IsPlagiarism
		output.IsPlagiarism = &value
	}
	if input.ErrorMessage != nil {
		value := *input.ErrorMessage
		output.ErrorMessage = &value
	}
	if input.StartedAt != nil {
		value := *input.StartedAt
		output.StartedAt = &value
	}
	if input.CompletedAt != nil {
		value := *input.CompletedAt
		output.CompletedAt = &value
	}
	output.Matches = append([]apicontract.MatchResult(nil), input.Matches...)

	return &output
}

func TestCheckJobSubmitAndPollFlowWithManager(t *testing.T) {
	store := newInMemoryJobStore()
	engine := fakeEngineClient{
		checkFn: func(_ context.Context, _ apicontract.CheckRequest) (*apicontract.CheckResponse, error) {
			time.Sleep(25 * time.Millisecond)
			return &apicontract.CheckResponse{
				IsPlagiarism: true,
				OverallScore: 0.91,
				Threshold:    0.8,
				Matches: []apicontract.MatchResult{{
					DocumentId:    "doc-async-1",
					ChunkId:       "chunk-async-1",
					MatchedText:   "suspected overlap",
					SemanticScore: 0.92,
					ExactScore:    0.81,
					FinalScore:    0.89,
				}},
			}, nil
		},
	}

	manager := jobs.NewManager(store, engine, 3*time.Second)
	handler := newServerHandlerWithJobs(engine, manager, "")
	server := httptest.NewServer(handler)
	defer server.Close()

	submitResp, err := http.Post(server.URL+checkJobBasePath, "application/json", strings.NewReader(`{"text":"queued flow"}`))
	if err != nil {
		t.Fatalf("submit check job: %v", err)
	}
	defer submitResp.Body.Close()

	if submitResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(submitResp.Body)
		t.Fatalf("unexpected submit status %d: %s", submitResp.StatusCode, string(body))
	}

	var accepted checkJobAcceptedResponse
	if err := json.NewDecoder(submitResp.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if accepted.JobID == "" {
		t.Fatalf("expected job id in submit response")
	}

	deadline := time.Now().Add(2 * time.Second)
	seenRunning := false
	for time.Now().Before(deadline) {
		statusResp, err := http.Get(server.URL + checkJobBasePath + "/" + accepted.JobID)
		if err != nil {
			t.Fatalf("poll check job: %v", err)
		}

		if statusResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(statusResp.Body)
			statusResp.Body.Close()
			t.Fatalf("unexpected status poll code %d: %s", statusResp.StatusCode, string(body))
		}

		var status checkJobStatusResponse
		if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
			statusResp.Body.Close()
			t.Fatalf("decode status response: %v", err)
		}
		statusResp.Body.Close()

		if status.Status == jobs.StatusRunning {
			seenRunning = true
		}
		if status.Status == jobs.StatusCompleted {
			if status.Result == nil {
				t.Fatalf("expected completed job to include result")
			}
			if !status.Result.IsPlagiarism {
				t.Fatalf("expected completed result to flag plagiarism")
			}
			if len(status.Result.Matches) != 1 {
				t.Fatalf("unexpected match count: %d", len(status.Result.Matches))
			}
			if !seenRunning {
				t.Fatalf("expected at least one running status before completion")
			}
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for check job completion")
}
