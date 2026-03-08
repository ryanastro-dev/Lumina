# Lumina Monorepo

AI-powered plagiarism checker built with:
- `apps/lumina-web` (client)
- `apps/lumina-api` (gateway/core orchestration)
- `apps/lumina-engine` (embedding + similarity + PDF extraction)

Current version baseline:
- Next.js `16.1.6`
- React `19.2.4`
- Go `1.26.1`
- PostgreSQL `18-alpine`

## Repository Layout

```text
apps/
  lumina-web/
  lumina-api/
  lumina-engine/
contracts/
  openapi/
  codegen/
docs/
infra/
scripts/
```

## One-Command Docker Stack

```powershell
Copy-Item .env.example .env
docker compose up --build -d
```

Default endpoints:
- web: `http://localhost:3000`
- gateway: `http://localhost:8080/health` (cleanup metrics: `/health/cleanup`)
- engine: `http://localhost:8000/health`
- qdrant: `http://localhost:6333`

Notes:
- Startup now waits for healthy dependencies (`service_healthy`) between layers.
- First `lumina-engine` image build can take significant time because ML dependencies are large.

Stop stack:
```powershell
docker compose down
```

## Manual Local Development

1. Start infra services:
```powershell
docker compose up -d qdrant postgres
```

2. Start AI engine service:
```powershell
Set-Location apps/lumina-engine
Copy-Item .env.example .env
uv sync
uv run uvicorn app.main:app --reload
```

3. Export OpenAPI contract from the engine:
```powershell
Set-Location apps/lumina-engine
uv run python scripts/export_openapi.py
```

4. Generate contract types (engine + gateway, Go + TypeScript):
```powershell
Set-Location ..\..
powershell -ExecutionPolicy Bypass -File scripts/generate-contract-types.ps1
```

5. Start API gateway:
```powershell
Set-Location apps/lumina-api
Copy-Item .env.example .env
go run ./cmd/gateway
```

6. Start web app:
```powershell
Set-Location apps/lumina-web
Copy-Item .env.example .env.local
npm install
npm run dev
```

## Baseline Seed (Optional)

With engine running locally:
```powershell
Set-Location apps/lumina-engine
uv run python scripts/seed_baseline_sources.py --manifest data/baseline/manifest.json
```

## Dev Helper Script

Use the stack helper script for common operations:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action status
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action up
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action smoke
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action seed
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action seed-reset
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action down
```

Optional flags:
- `-MaxSources 2` for partial seed
- `-Manifest data/baseline/manifest.json` to choose manifest
- `-Tail 200` with `-Action logs`
## Optional API Key Mode

- Set `LUMINA_API_KEY` in `apps/lumina-api/.env` or root `.env` for Docker mode.
- Set same value as `LUMINA_GATEWAY_API_KEY` in `apps/lumina-web/.env.local`.
- In this mode, `lumina-api` requires `X-API-Key` on all `/v1/*` routes.





