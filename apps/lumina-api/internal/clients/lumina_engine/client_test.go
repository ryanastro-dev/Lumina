package lumina_engine

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lumina/lumina/apps/lumina-api/internal/clients/lumina_engine/apicontract"
)

func TestIngestSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/ingest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var body apicontract.IngestRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Text == "" {
			t.Fatalf("text should not be empty")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"document_id":"doc-1","chunk_count":2,"collection":"lumina_documents"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	res, err := client.Ingest(context.Background(), apicontract.IngestRequest{Text: "sample"})
	if err != nil {
		t.Fatalf("Ingest error: %v", err)
	}

	if res.DocumentId != "doc-1" || res.ChunkCount != 2 {
		t.Fatalf("unexpected ingest response: %+v", res)
	}
}

func TestIngestAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"vector_store_unavailable","message":"Qdrant down","details":[{"reason":"timeout"}]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	_, err = client.Ingest(context.Background(), apicontract.IngestRequest{Text: "sample"})
	if err == nil {
		t.Fatalf("expected error")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status code: %d", apiErr.StatusCode)
	}
	if apiErr.Response.Error != "vector_store_unavailable" {
		t.Fatalf("unexpected error code: %s", apiErr.Response.Error)
	}
}

func TestExtractPDFMultipart(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/extract-pdf" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		ct := r.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil {
			t.Fatalf("parse media type: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("expected multipart/form-data, got %s", mediaType)
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(raw), "filename=\"sample.pdf\"") {
			t.Fatalf("expected filename in multipart body")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"filename":"sample.pdf","page_count":1,"character_count":10,"text":"hello text"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	res, err := client.ExtractPDF(context.Background(), "sample.pdf", []byte("%PDF-test"))
	if err != nil {
		t.Fatalf("ExtractPDF error: %v", err)
	}
	if res.PageCount != 1 || res.Filename != "sample.pdf" {
		t.Fatalf("unexpected response: %+v", res)
	}
}

func TestRequestIDHeaderForwarded(t *testing.T) {
	t.Parallel()

	const requestID = "req-forward-123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(HeaderRequestID); got != requestID {
			t.Fatalf("expected %s header %q, got %q", HeaderRequestID, requestID, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"is_plagiarism":false,"overall_score":0.1,"threshold":0.8,"matches":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, nil)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	ctx := WithRequestID(context.Background(), requestID)
	res, err := client.Check(ctx, apicontract.CheckRequest{Text: "sample"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected response")
	}
}
