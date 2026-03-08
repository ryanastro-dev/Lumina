# Web Client (Next.js)

Version baseline:
- Next.js `16.1.6`
- React `19.2.4`
- Node.js `>=20.9.0`

Responsibilities:
- document upload UI
- plagiarism result visualization
- calls to Go gateway only (browser never calls `lumina-engine` directly)

## Local Run

```powershell
Copy-Item .env.example .env.local
npm install
npm run dev
```

Default URL: `http://localhost:3000`

## Gateway Proxy Routes

- `POST /api/plagiarism/ingest` -> `lumina-api /v1/plagiarism/ingest`
- `POST /api/plagiarism/check` -> `lumina-api /v1/plagiarism/check`
- `POST /api/plagiarism/check-jobs` -> `lumina-api /v1/plagiarism/check-jobs`
- `GET /api/plagiarism/check-jobs/{job_id}` -> `lumina-api /v1/plagiarism/check-jobs/{job_id}`
- `POST /api/plagiarism/check-jobs/{job_id}/cancel` -> `lumina-api /v1/plagiarism/check-jobs/{job_id}/cancel`
- `POST /api/pdf/extract` -> `lumina-api /v1/pdf/extract`

## Optional API Key

If gateway enables `LUMINA_API_KEY`, set `LUMINA_GATEWAY_API_KEY` in `.env.local`.
Server-side proxy routes will forward it as `X-API-Key`.
