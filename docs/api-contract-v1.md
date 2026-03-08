# Lumina AI Processing API Contract v1

Version: `1.0.0`  
Scope: English-only Phase 1 API

Contract artifact (source of truth):
- `contracts/openapi/lumina-engine-v1.json`

## Base URL
- Local: `http://127.0.0.1:8000`

## Error Schema (all endpoints)
```json
{
  "error": "string_code",
  "message": "Human readable message",
  "details": null
}
```

`details` may be:
- `null`
- object (service reason/context)
- list (validation errors)

## Endpoints

### `GET /health`
Response `200`:
```json
{
  "status": "ok"
}
```

### `POST /ingest`
Purpose: chunk + embed + store in Qdrant

Request body:
```json
{
  "text": "Document text",
  "document_id": "optional-id",
  "source": "optional-source-name",
  "metadata": {
    "author": "alice"
  }
}
```

Response `200`:
```json
{
  "document_id": "doc-001",
  "chunk_count": 3,
  "collection": "lumina_documents"
}
```

Errors:
- `400` `chunking_error`
- `422` `validation_error`
- `503` `vector_store_unavailable`

### `POST /check`
Purpose: retrieve top-k candidates and return plagiarism score

Request body:
```json
{
  "text": "Text to evaluate",
  "top_k": 5,
  "threshold": 0.8
}
```

Response `200`:
```json
{
  "is_plagiarism": true,
  "overall_score": 0.91,
  "threshold": 0.8,
  "matches": [
    {
      "document_id": "doc-001",
      "source": "sample-a",
      "chunk_id": "chunk-uuid",
      "matched_text": "Matched source text",
      "semantic_score": 0.91,
      "exact_score": 0.34,
      "final_score": 0.91
    }
  ]
}
```

Errors:
- `400` `chunking_error`
- `422` `validation_error`
- `503` `vector_store_unavailable`

### `POST /extract-pdf`
Purpose: extract plain text from uploaded PDF file (`multipart/form-data`)

Form field:
- `file` (PDF file)

Response `200`:
```json
{
  "filename": "sample.pdf",
  "page_count": 2,
  "character_count": 1234,
  "text": "Extracted plain text"
}
```

Errors:
- `400` `invalid_file_type`, `empty_file`, `pdf_parse_error`
- `422` `validation_error`

## Backward Compatibility Rule
- Any change to field names, field types, endpoint paths, or status codes requires a contract version bump.

