## Lumina AI Processing Layer (Phase 1, English-Only)

FastAPI microservice for:
- text ingest into Qdrant vector DB
- plagiarism check (`Exact Match + Semantic Similarity`)
- PDF text extraction for preprocessing

This is the first risky core slice for your architecture:
- Client Layer (Next.js): later
- API Gateway (Go): later
- AI Processing (this repo): now
- Data Storage (Qdrant): now

## Stack
- Python + FastAPI
- sentence-transformers: `all-MiniLM-L12-v2`
- Qdrant (Cosine similarity)
- `uv` for venv + dependency management

## Run

1. Copy env file:
```powershell
Copy-Item .env.example .env
```

2. Start Qdrant:
```powershell
Set-Location ../..
docker compose up -d qdrant
Set-Location apps/lumina-engine
```

3. Install dependencies (already handled by `uv add`, but safe to rerun):
```powershell
uv sync
```

4. Run API:
```powershell
uv run uvicorn app.main:app --reload
```

Health check:
```powershell
curl http://127.0.0.1:8000/health
```

## API Examples

### 1) Ingest text
```powershell
curl -X POST http://127.0.0.1:8000/ingest `
  -H "Content-Type: application/json" `
  -d "{\"document_id\":\"doc-001\",\"source\":\"sample-a\",\"text\":\"Artificial intelligence can assist with plagiarism detection by semantic search.\"}"
```

### 2) Check text
```powershell
curl -X POST http://127.0.0.1:8000/check `
  -H "Content-Type: application/json" `
  -d "{\"text\":\"Semantic search helps detect plagiarism in AI systems.\",\"top_k\":5,\"threshold\":0.80}"
```

### 3) Extract PDF text
```powershell
curl -X POST http://127.0.0.1:8000/extract-pdf `
  -F "file=@C:\path\to\sample.pdf"
```

## Notes
- Token chunking strategy is window + overlap (`CHUNK_SIZE`, `CHUNK_OVERLAP`).
- Final score is `max(exact_score, semantic_score)`.
- This phase is English-only by design for faster MVP delivery.
- Baseline dataset template is in `data/baseline/README.md`.

## Baseline Calibration
Run threshold calibration against baseline scaffold:

```powershell
uv run python scripts/calibrate_threshold.py `
  --manifest data/baseline/manifest.json `
  --output-json data/baseline/report.json
```

Optional tuning:
```powershell
uv run python scripts/calibrate_threshold.py `
  --thresholds 0.70,0.75,0.80,0.85,0.90 `
  --top-k 5 `
  --chunk-size 240 `
  --chunk-overlap 60
```

## Baseline Seed
Ingest baseline source documents through the running API:

```powershell
uv run python scripts/seed_baseline_sources.py `
  --manifest data/baseline/manifest.json
```

Optional reset (recreate collection before ingest):
```powershell
uv run python scripts/seed_baseline_sources.py `
  --manifest data/baseline/manifest.json `
  --reset-collection
```

Dry run:
```powershell
uv run python scripts/seed_baseline_sources.py --dry-run
```

## API Contract (Frozen v1)
- Human-readable contract: `../../docs/api-contract-v1.md`
- Machine-readable OpenAPI export:

```powershell
uv run python scripts/export_openapi.py
```

Output:
- `../../contracts/openapi/lumina-engine-v1.json` (source of truth)
- `../../docs/openapi-v1.json` (doc snapshot)
