param(
  [ValidateSet("engine", "gateway", "all")]
  [string]$Target = "all",
  [string]$EngineOpenApiPath = "contracts/openapi/lumina-engine-v1.json",
  [string]$GatewayOpenApiPath = "contracts/openapi/lumina-gateway-v1.json"
)

$ErrorActionPreference = "Stop"

function Invoke-ContractCodegen {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [string]$OpenApiPath,
    [Parameter(Mandatory = $true)]
    [string]$TsOut,
    [Parameter(Mandatory = $true)]
    [string]$GoOut,
    [string]$GoPackage = "apicontract",
    [string]$GoMirrorOut = "",
    [string]$TsMirrorOut = ""
  )

  if (!(Test-Path $OpenApiPath)) {
    throw "[$Name] OpenAPI spec not found: $OpenApiPath"
  }

  Write-Host "[$Name] Generating TypeScript types..."
  npx -y openapi-typescript $OpenApiPath -o $TsOut
  if ($LASTEXITCODE -ne 0) {
    throw "[$Name] TypeScript codegen failed (exit=$LASTEXITCODE)"
  }

  Write-Host "[$Name] Generating Go types..."
  $goCodeRaw = go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest `
    -generate types `
    -package $GoPackage `
    $OpenApiPath
  if ($LASTEXITCODE -ne 0) {
    throw "[$Name] Go codegen failed (exit=$LASTEXITCODE)"
  }

  $goCode = if ($goCodeRaw -is [array]) { ($goCodeRaw -join "`n") + "`n" } else { [string]$goCodeRaw }
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($GoOut, $goCode, $utf8NoBom)

  if (-not [string]::IsNullOrWhiteSpace($GoMirrorOut) -and (Test-Path (Split-Path $GoMirrorOut -Parent))) {
    Copy-Item -Force $GoOut $GoMirrorOut
    Write-Host "[$Name] Synced Go contract file to $GoMirrorOut"
  }

  if (-not [string]::IsNullOrWhiteSpace($TsMirrorOut) -and (Test-Path (Split-Path $TsMirrorOut -Parent))) {
    Copy-Item -Force $TsOut $TsMirrorOut
    Write-Host "[$Name] Synced TS contract file to $TsMirrorOut"
  }

  Write-Host "[$Name] Done."
}

if ($Target -eq "engine" -or $Target -eq "all") {
  Invoke-ContractCodegen `
    -Name "engine" `
    -OpenApiPath $EngineOpenApiPath `
    -TsOut "contracts/codegen/ts/lumina-engine-v1.d.ts" `
    -GoOut "contracts/codegen/go/lumina_engine_v1.gen.go" `
    -GoPackage "apicontract" `
    -GoMirrorOut "apps/lumina-api/internal/clients/lumina_engine/apicontract/lumina_engine_v1.gen.go" `
    -TsMirrorOut "apps/lumina-web/src/lib/api-client/apicontract.d.ts"
}

if ($Target -eq "gateway" -or $Target -eq "all") {
  Invoke-ContractCodegen `
    -Name "gateway" `
    -OpenApiPath $GatewayOpenApiPath `
    -TsOut "contracts/codegen/ts/lumina-gateway-v1.d.ts" `
    -GoOut "contracts/codegen/go/lumina_gateway_v1.gen.go" `
    -GoPackage "apicontract" `
    -TsMirrorOut "apps/lumina-web/src/lib/api-client/gateway-apicontract.d.ts"
}

Write-Host "All requested contract code generation completed."

