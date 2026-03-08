<p align="center">
  <img src="https://img.shields.io/badge/Lumina-AI%20Plagiarism%20Checker-0f766e?style=for-the-badge&logo=sparkles&logoColor=white" alt="Lumina" />
</p>

<h1 align="center">🔬 Lumina</h1>

<p align="center">
  <strong>AI-Powered Plagiarism Detection System</strong><br/>
  Semantic similarity analysis using vector embeddings & hybrid scoring
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Next.js-16.1.6-000000?style=flat-square&logo=nextdotjs&logoColor=white" />
  <img src="https://img.shields.io/badge/React-19.2.4-61DAFB?style=flat-square&logo=react&logoColor=black" />
  <img src="https://img.shields.io/badge/Go-1.26.1-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/Python-3.13-3776AB?style=flat-square&logo=python&logoColor=white" />
  <img src="https://img.shields.io/badge/FastAPI-0.135+-009688?style=flat-square&logo=fastapi&logoColor=white" />
  <img src="https://img.shields.io/badge/MiniLM--L12--v2-384d%20Embeddings-FF6F00?style=flat-square&logo=huggingface&logoColor=white" />
  <img src="https://img.shields.io/badge/PostgreSQL-18-4169E1?style=flat-square&logo=postgresql&logoColor=white" />
  <img src="https://img.shields.io/badge/Qdrant-Vector%20DB-DC382D?style=flat-square&logo=qdrant&logoColor=white" />
  <img src="https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white" />
  <img src="https://img.shields.io/badge/TypeScript-5.9.3-3178C6?style=flat-square&logo=typescript&logoColor=white" />
  <img src="https://img.shields.io/badge/CI-GitHub%20Actions-2088FF?style=flat-square&logo=githubactions&logoColor=white" />
</p>

<p align="center">
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-architecture">Architecture</a> •
  <a href="#-features">Features</a> •
  <a href="#-api-reference">API Reference</a> •
  <a href="#-development">Development</a> •
  <a href="#-deployment">Deployment</a>
</p>

---

## 🌟 Overview

**Lumina** is a monorepo-based plagiarism detection platform that combines **semantic vector search** with **n-gram exact matching** to deliver hybrid similarity scoring. Documents are chunked, embedded using Sentence Transformers, and stored in a Qdrant vector database. When a query text is submitted, Lumina retrieves the most similar chunks and computes a final plagiarism score using both semantic and exact-match metrics.

### ✨ Key Highlights

- 🧠 **Hybrid Scoring** — Combines cosine-similarity embeddings with Jaccard shingling for precision
- ⚡ **Async Job Queue** — Long-running checks execute in background with real-time polling
- 📄 **PDF Text Extraction** — Upload PDFs and extract plain text through the API
- 🔒 **Optional API Key Auth** — Protect `/v1/*` routes with `X-API-Key` header
- 🐳 **One-Command Deploy** — Full Docker Compose stack with health-check orchestration
- 📜 **Contract-First Design** — OpenAPI specs auto-generate Go + TypeScript types
- 🧹 **Auto Cleanup** — Stale jobs are purged on a configurable retention schedule
- 🏥 **Deep Health Checks** — `/health`, `/health/engine`, `/health/cleanup` endpoints

---

## 🏗 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Client Layer                        │
│                     lumina-web (Next.js 16)                  │
│              React 19  •  BFF Proxy  •  Workbench UI        │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTP (proxy)
┌───────────────────────────▼─────────────────────────────────┐
│                       Gateway Layer                         │
│                    lumina-api (Go 1.26)                      │
│      REST API  •  Job Queue  •  Auth  •  Cleanup Loop       │
│                            │                                │
│                      ┌─────▼─────┐                          │
│                      │ PostgreSQL│ (Job persistence)        │
│                      └───────────┘                          │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTP (internal)
┌───────────────────────────▼─────────────────────────────────┐
│                    AI Processing Layer                       │
│                lumina-engine (Python / FastAPI)              │
│   Chunking  •  Embedding  •  Matching  •  PDF Extraction    │
│              Model: all-MiniLM-L12-v2 (384-dim)             │
│                            │                                │
│                      ┌─────▼─────┐                          │
│                      │  Qdrant   │ (Vector storage)         │
│                      └───────────┘                          │
└─────────────────────────────────────────────────────────────┘
```

| Layer             | Technology                    | Purpose                                           |
| ----------------- | ----------------------------- | ------------------------------------------------- |
| **Web**           | Next.js 16.1.6 / React 19.2.4 | Interactive workbench, BFF proxy routes           |
| **Gateway**       | Go 1.26.1 / stdlib `net/http` | API orchestration, auth, async jobs, cleanup      |
| **Engine**        | Python 3.13 / FastAPI         | Embedding, similarity, chunking, PDF extraction   |
| **Vector DB**     | Qdrant                        | Stores document chunk vectors for semantic search |
| **Relational DB** | PostgreSQL 18                 | Persists async scan jobs and match results        |

---

## 🚀 Quick Start

### Prerequisites

| Tool                       | Version |
| -------------------------- | ------- |
| 🐳 Docker + Docker Compose | v2+     |
| (Optional) Go              | 1.26+   |
| (Optional) Python + uv     | 3.13+   |
| (Optional) Node.js + npm   | 20.9+   |

### One-Command Docker Stack

```powershell
# 1. Clone the repository
git clone https://github.com/ryanastro-dev/Lumina.git
cd Lumina

# 2. Configure environment
Copy-Item .env.example .env

# 3. Build and start all services
docker compose up --build -d
```

> [!NOTE]
> First build takes significant time because `lumina-engine` downloads ML model dependencies (~2 GB).
> Subsequent builds use Docker layer caching and are much faster.

### Default Endpoints

| Service                | URL                                    | Purpose               |
| ---------------------- | -------------------------------------- | --------------------- |
| 🌐 **Web UI**          | `http://localhost:3000`                | Plagiarism workbench  |
| 🔌 **Gateway**         | `http://localhost:8080/health`         | API gateway           |
| 🔌 **Gateway Engine**  | `http://localhost:8080/health/engine`  | Engine connectivity   |
| 🔌 **Gateway Cleanup** | `http://localhost:8080/health/cleanup` | Cleanup loop status   |
| 🧠 **Engine**          | `http://localhost:8000/health`         | AI processing service |
| 📊 **Qdrant**          | `http://localhost:6333`                | Vector DB dashboard   |

### Stop the Stack

```powershell
docker compose down
```

---

## ✨ Features

### 📥 Document Ingestion

Upload source documents as plain text. Lumina automatically:

1. **Normalizes** whitespace
2. **Chunks** text into overlapping sliding windows (default: 240 tokens, 60 overlap)
3. **Embeds** each chunk using `sentence-transformers/all-MiniLM-L12-v2`
4. **Stores** vectors + metadata in Qdrant

### 🔍 Plagiarism Check

Submit query text for similarity analysis. The engine:

1. Chunks and embeds the query text
2. Searches Qdrant for top-K similar vectors per chunk
3. Computes **semantic score** (cosine similarity) and **exact score** (Jaccard shingling)
4. Returns `max(semantic, exact)` as the final score per match

### ⚡ Async Job Queue

For large texts requiring extended processing:

- **POST** `/v1/plagiarism/check-jobs` → returns `job_id` + `202 Accepted`
- **GET** `/v1/plagiarism/check-jobs/{id}` → poll status + results
- **POST** `/v1/plagiarism/check-jobs/{id}/cancel` → cancel a running job
- Jobs persist in PostgreSQL across gateway restarts

### 📄 PDF Extraction

Upload a PDF file and receive extracted plain text with page/character counts.

### 🧹 Automatic Job Cleanup

A background loop purges terminal jobs (`completed`, `failed`, `canceled`) older than the configured retention period (default: 7 days). Status is exposed at `/health/cleanup`.

---

## 📡 API Reference

### Engine API (`http://localhost:8000`)

| Method | Endpoint       | Description                         |
| ------ | -------------- | ----------------------------------- |
| `GET`  | `/health`      | Health check                        |
| `POST` | `/ingest`      | Ingest document into vector storage |
| `POST` | `/check`       | Check text similarity               |
| `POST` | `/extract-pdf` | Extract text from PDF (multipart)   |

### Gateway API (`http://localhost:8080`)

| Method | Endpoint                                | Description                  |
| ------ | --------------------------------------- | ---------------------------- |
| `GET`  | `/health`                               | Gateway health               |
| `GET`  | `/health/engine`                        | Engine connectivity          |
| `GET`  | `/health/cleanup`                       | Cleanup loop status          |
| `POST` | `/v1/plagiarism/ingest`                 | Proxy ingest to engine       |
| `POST` | `/v1/plagiarism/check`                  | Synchronous plagiarism check |
| `POST` | `/v1/plagiarism/check-jobs`             | Submit async check job       |
| `GET`  | `/v1/plagiarism/check-jobs/{id}`        | Get job status + results     |
| `POST` | `/v1/plagiarism/check-jobs/{id}/cancel` | Cancel a running job         |
| `POST` | `/v1/pdf/extract`                       | Proxy PDF extraction         |

> [!TIP]
> Full OpenAPI specs are available in `contracts/openapi/`. Human-readable contract docs are in `docs/`.

### Error Response Format

All endpoints return errors in a consistent envelope:

```json
{
  "error": "string_code",
  "message": "Human readable message",
  "details": null
}
```

---

## 🛠 Development

### Repository Layout

```
lumina/
├── apps/
│   ├── lumina-web/          # Next.js 16 frontend + BFF proxy
│   ├── lumina-api/          # Go API gateway + job system
│   └── lumina-engine/       # Python AI processing service
├── contracts/
│   ├── openapi/             # OpenAPI v3 specs (source of truth)
│   ├── codegen/             # Auto-generated Go + TypeScript types
│   └── postman/             # Postman collection
├── docs/                    # Architecture + API contract docs
├── infra/
│   ├── postgres/            # DB init scripts
│   └── qdrant/              # Qdrant config
├── scripts/                 # Dev helper scripts
├── docker-compose.yml
├── .env.example
└── .github/workflows/ci.yml # CI pipeline
```

### Manual Local Development

<details>
<summary><strong>1️⃣ Start infrastructure services</strong></summary>

```powershell
docker compose up -d qdrant postgres
```

</details>

<details>
<summary><strong>2️⃣ Start AI engine</strong></summary>

```powershell
Set-Location apps/lumina-engine
Copy-Item .env.example .env
uv sync
uv run uvicorn app.main:app --reload
```

</details>

<details>
<summary><strong>3️⃣ Generate contract types</strong></summary>

```powershell
# Export engine OpenAPI spec
Set-Location apps/lumina-engine
uv run python scripts/export_openapi.py

# Generate Go + TypeScript types from OpenAPI
Set-Location ../..
powershell -ExecutionPolicy Bypass -File scripts/generate-contract-types.ps1
```

</details>

<details>
<summary><strong>4️⃣ Start API gateway</strong></summary>

```powershell
Set-Location apps/lumina-api
Copy-Item .env.example .env
go run ./cmd/gateway
```

</details>

<details>
<summary><strong>5️⃣ Start web app</strong></summary>

```powershell
Set-Location apps/lumina-web
Copy-Item .env.example .env.local
npm install
npm run dev
```

</details>

### Dev Helper Script

Use the all-in-one stack helper for common operations:

```powershell
# Stack lifecycle
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action status
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action up
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action rebuild
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action down

# Verification
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action smoke

# Baseline seeding
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action seed
powershell -ExecutionPolicy Bypass -File scripts/dev-stack.ps1 -Action seed-reset
```

| Flag                                    | Description                           |
| --------------------------------------- | ------------------------------------- |
| `-MaxSources 2`                         | Limit number of sources to seed       |
| `-Manifest data/baseline/manifest.json` | Choose manifest file                  |
| `-Tail 200`                             | Log tail length (with `-Action logs`) |

### Running Tests

```powershell
# Go gateway tests
cd apps/lumina-api && go test ./...

# Python compile check
cd apps/lumina-engine && uv run python -m compileall app scripts

# Web build check
cd apps/lumina-web && npm run build
```

---

## 🐳 Deployment

### Docker Compose (Production)

```powershell
Copy-Item .env.example .env
# Edit .env with production values
docker compose up --build -d
```

The stack uses `service_healthy` dependency conditions to ensure correct startup order:

```
Qdrant ─────────────────┐
                        ├──▶ lumina-engine ──▶ lumina-api ──▶ lumina-web
PostgreSQL ─────────────┘
```

### Environment Variables

| Variable                              | Default                                   | Description                             |
| ------------------------------------- | ----------------------------------------- | --------------------------------------- |
| `GATEWAY_PORT`                        | `8080`                                    | Gateway listen port                     |
| `WEB_PORT`                            | `3000`                                    | Web UI port                             |
| `LUMINA_ENGINE_PORT`                  | `8000`                                    | Engine listen port                      |
| `LUMINA_API_KEY`                      | _(empty)_                                 | Optional API key for `/v1/*` protection |
| `POSTGRES_DB`                         | `lumina`                                  | Database name                           |
| `POSTGRES_USER`                       | `lumina`                                  | Database user                           |
| `POSTGRES_PASSWORD`                   | `lumina_dev`                              | Database password                       |
| `QDRANT_URL`                          | `http://localhost:6333`                   | Qdrant connection URL                   |
| `QDRANT_COLLECTION`                   | `lumina_documents`                        | Qdrant collection name                  |
| `EMBEDDING_MODEL`                     | `sentence-transformers/all-MiniLM-L12-v2` | HuggingFace model ID                    |
| `CHUNK_SIZE`                          | `240`                                     | Tokens per chunk                        |
| `CHUNK_OVERLAP`                       | `60`                                      | Overlap between chunks                  |
| `SIMILARITY_THRESHOLD`                | `0.80`                                    | Default plagiarism threshold            |
| `TOP_K`                               | `5`                                       | Results per query chunk                 |
| `LUMINA_ENGINE_TIMEOUT_SECONDS`       | `120`                                     | Engine request timeout                  |
| `LUMINA_JOB_RETENTION_HOURS`          | `168`                                     | Stale job retention (7 days)            |
| `LUMINA_JOB_CLEANUP_INTERVAL_MINUTES` | `60`                                      | Cleanup loop interval                   |

### 🔒 API Key Mode

To protect all `/v1/*` gateway routes with an API key:

1. Set `LUMINA_API_KEY` in your `.env` or `apps/lumina-api/.env`
2. Set `LUMINA_GATEWAY_API_KEY` (same value) in `apps/lumina-web/.env.local`
3. All requests to `/v1/*` must include the `X-API-Key` header
4. Health endpoints remain public in both modes

---

## 🔄 CI Pipeline

GitHub Actions runs on every push to `main` and on pull requests:

| Job           | What it does                                    |
| ------------- | ----------------------------------------------- |
| **contracts** | Regenerates Go + TS types and verifies no drift |
| **engine**    | Python compile check via `compileall`           |
| **api**       | Runs full Go test suite (`go test ./...`)       |
| **web**       | Builds Next.js production bundle                |

---

## ⚙️ Configuration

### Engine Tuning

| Parameter              | Default             | Effect                                            |
| ---------------------- | ------------------- | ------------------------------------------------- |
| `CHUNK_SIZE`           | 240                 | Larger = fewer chunks, less granular matching     |
| `CHUNK_OVERLAP`        | 60                  | Higher = more context preservation between chunks |
| `SIMILARITY_THRESHOLD` | 0.80                | Lower = more matches flagged as plagiarism        |
| `TOP_K`                | 5                   | Higher = more candidates evaluated per chunk      |
| `EMBEDDING_MODEL`      | `all-MiniLM-L12-v2` | Change for different accuracy/speed tradeoffs     |

### 🧠 Embedding Model — `all-MiniLM-L12-v2`

Lumina uses [**sentence-transformers/all-MiniLM-L12-v2**](https://huggingface.co/sentence-transformers/all-MiniLM-L12-v2) as its default embedding model — a compact, high-performance transformer fine-tuned for semantic similarity.

| Spec                    | Value                      |
| ----------------------- | -------------------------- |
| **Architecture**        | MiniLM (12-layer, 384-dim) |
| **Parameters**          | 33M                        |
| **Output Dimensions**   | 384                        |
| **Max Sequence Length** | 256 tokens                 |
| **Similarity Function** | Cosine Similarity          |
| **Training Data**       | 1B+ sentence pairs         |
| **Speed**               | ~2800 sentences/sec (GPU)  |
| **Model Size**          | ~134 MB                    |

**Why this model?**

- ⚡ Excellent speed-to-accuracy ratio for real-time plagiarism scoring
- 📐 384-dim vectors keep Qdrant storage lean while preserving semantic richness
- 🎯 Fine-tuned on semantic textual similarity (STS) benchmarks
- 🔄 Configurable via `EMBEDDING_MODEL` env var — swap to `all-mpnet-base-v2` (768-dim, higher accuracy) or `all-MiniLM-L6-v2` (384-dim, faster) without code changes

> [!NOTE]
> The model is pre-loaded during startup via warmup in the `lifespan` hook, so the first API request has zero cold-start latency.

---

## 📌 Version Baseline

| Component         | Version     |
| ----------------- | ----------- |
| Next.js           | `16.1.6`    |
| React / React DOM | `19.2.4`    |
| TypeScript        | `5.9.3`     |
| Go                | `1.26.1`    |
| Python            | `3.13`      |
| FastAPI           | `0.135+`    |
| PostgreSQL        | `18-alpine` |
| Qdrant            | `latest`    |
| Node.js (Docker)  | `24-alpine` |

---

## 📄 License

This project is for educational and demonstration purposes.

---

<p align="center">
  <sub>Semantic intelligence meets precision engineering — crafted for academic integrity</sub>
</p>
