# API Gateway (Go)

Version baseline:
- Go `1.26.1`

Responsibilities:
- auth/rate-limit/orchestration
- call `lumina-engine`
- persist async plagiarism jobs + results in PostgreSQL

Contract source:
- `contracts/openapi/lumina-gateway-v1.json`

## Local Run

```powershell
Copy-Item .env.example .env
go run ./cmd/gateway
```

Default port: `8080`

## Required Environment

- `LUMINA_ENGINE_BASE_URL`
- `LUMINA_JOB_RETENTION_HOURS` (default: `168`)
- `LUMINA_JOB_CLEANUP_INTERVAL_MINUTES` (default: `60`)
- `POSTGRES_DSN`
- `LUMINA_DB_MAX_OPEN_CONNS` (default: `25`)
- `LUMINA_DB_MAX_IDLE_CONNS` (default: `10`)
- `LUMINA_DB_CONN_MAX_LIFETIME_MINUTES` (default: `5`)

At startup, the service validates DB connectivity and ensures required tables exist (`scan_jobs`, `scan_matches`). A background cleanup loop periodically deletes stale terminal jobs (`completed`, `failed`, `canceled`) older than the configured retention window. Runtime cleanup metrics are exposed at `GET /health/cleanup`.

## Routes

- `GET /health`
- `GET /health/engine`
- `GET /health/cleanup` (`ok` | `degraded` | `disabled`)
- `POST /v1/plagiarism/ingest`
- `POST /v1/plagiarism/check` (sync)
- `POST /v1/plagiarism/check-jobs` (async submit, returns `202`)
- `GET /v1/plagiarism/check-jobs/{job_id}` (poll status/result)
- `POST /v1/plagiarism/check-jobs/{job_id}/cancel` (async cancel, returns `202`)
- `POST /v1/pdf/extract`

## Request ID

- Header name: `X-Request-Id`
- If client sends `X-Request-Id`, gateway preserves it.
- If missing, gateway generates one and returns it on response headers.
- Gateway forwards request-id to `lumina-engine` for upstream correlation.
- Async jobs store request-id for traceability.

## Upstream Timeout

- Env: `LUMINA_ENGINE_TIMEOUT_SECONDS` (default: `120`)
- Applies to requests from gateway -> `lumina-engine`.
- Also used as async job processing timeout.

## Optional API Key

- Env: `LUMINA_API_KEY`
- When set, all `/v1/*` routes require header `X-API-Key` with the same value.
- Health routes stay public.
