# Lumina Architecture

## Layers

1. Client Layer: `apps/lumina-web`
2. API Gateway Layer: `apps/lumina-api`
3. AI Processing Layer: `apps/lumina-engine`
4. Data Layer:
   - Qdrant for vector search
   - PostgreSQL for gateway relational data

## Runtime Version Baseline

- Next.js: `16.1.6`
- React / React DOM: `19.2.4`
- TypeScript: `5.9.3`
- Go toolchain: `1.26.1`
- PostgreSQL image: `postgres:18-alpine`

## Contract First

- API contract sources:
  - Engine: `contracts/openapi/lumina-engine-v1.json`
  - Gateway: `contracts/openapi/lumina-gateway-v1.json`
- Human contract notes:
  - Engine: `docs/api-contract-v1.md`
  - Gateway: `docs/gateway-api-contract-v1.md`
- Type generation targets:
  - Go: `contracts/codegen/go`
  - TypeScript: `contracts/codegen/ts`

## Phase Focus

Current highest-risk area completed first:
- embedding + similarity scoring
- vector storage integration
- baseline threshold calibration
