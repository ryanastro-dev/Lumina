# Contract Code Generation

This folder stores generated types from OpenAPI contracts.

## Contract Inputs

- Engine: `contracts/openapi/lumina-engine-v1.json`
- Gateway: `contracts/openapi/lumina-gateway-v1.json`

## Generated Outputs

- Engine Go: `contracts/codegen/go/lumina_engine_v1.gen.go`
- Engine TS: `contracts/codegen/ts/lumina-engine-v1.d.ts`
- Gateway Go: `contracts/codegen/go/lumina_gateway_v1.gen.go`
- Gateway TS: `contracts/codegen/ts/lumina-gateway-v1.d.ts`

Generate with:
```powershell
# both engine + gateway
powershell -ExecutionPolicy Bypass -File scripts/generate-contract-types.ps1 -Target all

# engine only
powershell -ExecutionPolicy Bypass -File scripts/generate-contract-types.ps1 -Target engine

# gateway only
powershell -ExecutionPolicy Bypass -File scripts/generate-contract-types.ps1 -Target gateway
```

Mirror sync targets (auto-copied by script):
- Engine Go -> `apps/lumina-api/internal/clients/lumina_engine/apicontract/lumina_engine_v1.gen.go`
- Engine TS -> `apps/lumina-web/src/lib/api-client/apicontract.d.ts`
- Gateway TS -> `apps/lumina-web/src/lib/api-client/gateway-apicontract.d.ts`

Notes:
- Uses `npx openapi-typescript` and `go run .../oapi-codegen@latest`.
- Requires Node.js/npm and Go installed.

CI check:
- .github/workflows/ci.yml regenerates contracts and fails if generated files are stale.

