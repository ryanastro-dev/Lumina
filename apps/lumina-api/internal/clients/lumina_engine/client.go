package lumina_engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/lumina/lumina/apps/lumina-api/internal/clients/lumina_engine/apicontract"
)

const (
	healthPath     = "/health"
	ingestPath     = "/ingest"
	checkPath      = "/check"
	extractPDFPath = "/extract-pdf"

	// HeaderRequestID is propagated from gateway to lumina-engine for trace correlation.
	HeaderRequestID = "X-Request-Id"
)

type contextKey string

const requestIDContextKey contextKey = "request_id"

// APIError represents an error response from lumina-engine.
type APIError struct {
	StatusCode int
	Response   apicontract.ErrorResponse
	RawBody    string
}

func (e *APIError) Error() string {
	if e.Response.Message != "" {
		return fmt.Sprintf("lumina-engine API error (%d): %s", e.StatusCode, e.Response.Message)
	}
	return fmt.Sprintf("lumina-engine API error (%d)", e.StatusCode)
}

// Client defines operations supported by lumina-engine.
type Client interface {
	Health(ctx context.Context) (*apicontract.HealthResponse, error)
	Ingest(ctx context.Context, request apicontract.IngestRequest) (*apicontract.IngestResponse, error)
	Check(ctx context.Context, request apicontract.CheckRequest) (*apicontract.CheckResponse, error)
	ExtractPDF(ctx context.Context, filename string, content []byte) (*apicontract.PdfExtractResponse, error)
}

// ServiceClient is an HTTP client for lumina-engine.
type ServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) (*ServiceClient, error) {
	normalized := strings.TrimSpace(baseURL)
	normalized = strings.TrimRight(normalized, "/")
	if normalized == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	return &ServiceClient{
		baseURL:    normalized,
		httpClient: httpClient,
	}, nil
}

// WithRequestID attaches request-id metadata to context for upstream propagation.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	trimmed := strings.TrimSpace(requestID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey, trimmed)
}

// RequestIDFromContext returns request-id metadata from context when present.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	raw := ctx.Value(requestIDContextKey)
	requestID, ok := raw.(string)
	if !ok {
		return "", false
	}

	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", false
	}

	return requestID, true
}

func (c *ServiceClient) Health(ctx context.Context) (*apicontract.HealthResponse, error) {
	var out apicontract.HealthResponse
	if err := c.doJSON(ctx, http.MethodGet, healthPath, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ServiceClient) Ingest(ctx context.Context, request apicontract.IngestRequest) (*apicontract.IngestResponse, error) {
	var out apicontract.IngestResponse
	if err := c.doJSON(ctx, http.MethodPost, ingestPath, request, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ServiceClient) Check(ctx context.Context, request apicontract.CheckRequest) (*apicontract.CheckResponse, error) {
	var out apicontract.CheckResponse
	if err := c.doJSON(ctx, http.MethodPost, checkPath, request, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *ServiceClient) ExtractPDF(ctx context.Context, filename string, content []byte) (*apicontract.PdfExtractResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err = part.Write(content); err != nil {
		return nil, fmt.Errorf("write file content: %w", err)
	}
	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+extractPDFPath, &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	applyRequestIDHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeAPIError(resp)
	}

	var out apicontract.PdfExtractResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

func (c *ServiceClient) doJSON(ctx context.Context, method, path string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	applyRequestIDHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp)
	}

	if responseBody == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(responseBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

func applyRequestIDHeader(req *http.Request) {
	requestID, ok := RequestIDFromContext(req.Context())
	if !ok {
		return
	}
	req.Header.Set(HeaderRequestID, requestID)
}

func decodeAPIError(resp *http.Response) error {
	data, _ := io.ReadAll(resp.Body)

	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		RawBody:    string(data),
		Response: apicontract.ErrorResponse{
			Error:   "http_error",
			Message: http.StatusText(resp.StatusCode),
		},
	}

	if len(data) == 0 {
		return apiErr
	}

	var decoded apicontract.ErrorResponse
	if err := json.Unmarshal(data, &decoded); err == nil {
		if decoded.Error != "" || decoded.Message != "" {
			apiErr.Response = decoded
		}
	}

	return apiErr
}
