# Lumina API Gateway Contract v1

Version: `1.0.0`  
Scope: Public gateway API exposed to web/external clients.

Contract artifact (source of truth):
- `contracts/openapi/lumina-gateway-v1.json`

## Base URL
- Local: `http://127.0.0.1:8080`

## Health Endpoints

- `GET /health`
- `GET /health/engine`
- `GET /health/cleanup`

`/health/cleanup` returns:
- `status = ok` when cleanup loop is healthy
- `status = disabled` when cleanup is intentionally off
- `status = degraded` (HTTP `503`) when cleanup is enabled but last run failed

## Public V1 Routes

- `POST /v1/plagiarism/ingest`
- `POST /v1/plagiarism/check`
- `POST /v1/plagiarism/check-jobs`
- `GET /v1/plagiarism/check-jobs/{job_id}`
- `POST /v1/plagiarism/check-jobs/{job_id}/cancel`
- `POST /v1/pdf/extract`

## Auth Behavior

Gateway API key mode is optional and controlled by environment:
- if `LUMINA_API_KEY` is unset, `/v1/*` routes are public
- if `LUMINA_API_KEY` is set, `/v1/*` requires `X-API-Key`

Health routes remain public in both modes.

## Backward Compatibility Rule

Any change to endpoint paths, status codes, or response/request field names/types requires a contract version bump.
