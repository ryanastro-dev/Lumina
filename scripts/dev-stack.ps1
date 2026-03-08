param(
  [ValidateSet("up", "down", "status", "logs", "smoke", "seed", "seed-reset", "seed-dry-run", "rebuild")]
  [string]$Action = "status",
  [int]$Tail = 120,
  [int]$MaxSources = 0,
  [string]$Manifest = ""
)

$ErrorActionPreference = "Stop"

function Invoke-Compose {
  param([string[]]$ComposeArgs)

  & docker compose @ComposeArgs
  if ($LASTEXITCODE -ne 0) {
    throw "docker compose command failed: docker compose $($ComposeArgs -join ' ')"
  }
}

function Get-ContainerHealthStatus {
  param([string]$ContainerName)

  $status = & docker inspect --format "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}" $ContainerName 2>$null
  if ($LASTEXITCODE -ne 0) {
    return "missing"
  }

  return ($status | Out-String).Trim()
}

function Wait-AllHealthy {
  param([int]$TimeoutSeconds = 420)

  $containers = @(
    "lumina-qdrant",
    "lumina-postgres",
    "lumina-engine",
    "lumina-api",
    "lumina-web"
  )

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)

  while ((Get-Date) -lt $deadline) {
    $allHealthy = $true

    foreach ($name in $containers) {
      $status = Get-ContainerHealthStatus -ContainerName $name
      if ($status -ne "healthy") {
        $allHealthy = $false
        break
      }
    }

    if ($allHealthy) {
      Write-Host "All services are healthy."
      return
    }

    Start-Sleep -Seconds 3
  }

  throw "Timed out waiting for containers to become healthy."
}

function Invoke-SmokeCheck {
  Write-Host "Smoke: gateway /health"
  Invoke-RestMethod -Uri "http://localhost:8080/health" -Method Get | Out-Null

  Write-Host "Smoke: gateway /health/engine"
  Invoke-RestMethod -Uri "http://localhost:8080/health/engine" -Method Get | Out-Null

  Write-Host "Smoke: gateway /health/cleanup"
  Invoke-RestMethod -Uri "http://localhost:8080/health/cleanup" -Method Get | Out-Null

  Write-Host "Smoke: web proxy plagiarism/check"
  $body = @{ text = "smoke test text"; top_k = 3; threshold = 0.8 } | ConvertTo-Json -Compress
  Invoke-RestMethod -Uri "http://localhost:3000/api/plagiarism/check" -Method Post -ContentType "application/json" -Body $body | Out-Null

  Write-Host "Smoke checks passed."
}

function Invoke-Seed {
  param(
    [bool]$ResetCollection,
    [bool]$DryRun
  )

  $seedArgs = @("python", "scripts/seed_baseline_sources.py")

  if ($ResetCollection) {
    $seedArgs += "--reset-collection"
  }

  if ($DryRun) {
    $seedArgs += "--dry-run"
  }

  if ($MaxSources -gt 0) {
    $seedArgs += @("--max-sources", "$MaxSources")
  }

  if (-not [string]::IsNullOrWhiteSpace($Manifest)) {
    $seedArgs += @("--manifest", $Manifest)
  }

  $uv = Get-Command uv -ErrorAction SilentlyContinue
  if ($null -ne $uv) {
    Write-Host "Seeding via host uv environment..."
    Push-Location "apps/lumina-engine"
    try {
      & uv run @seedArgs
      if ($LASTEXITCODE -eq 0) {
        return
      }
    } finally {
      Pop-Location
    }
  }

  Write-Host "Falling back to seeding via running container..."
  $tailArgs = @()
  if ($seedArgs.Length -gt 2) {
    $tailArgs = $seedArgs[2..($seedArgs.Length - 1)]
  }
  $composeArgs = @("exec", "-T", "lumina-engine", "python", "scripts/seed_baseline_sources.py") + $tailArgs
  Invoke-Compose -ComposeArgs $composeArgs
}

switch ($Action) {
  "up" {
    Write-Host "Starting stack..."
    Invoke-Compose -ComposeArgs @("up", "-d")
    Wait-AllHealthy
  }
  "rebuild" {
    Write-Host "Rebuilding and starting stack..."
    Invoke-Compose -ComposeArgs @("up", "--build", "-d")
    Wait-AllHealthy
  }
  "down" {
    Write-Host "Stopping stack..."
    Invoke-Compose -ComposeArgs @("down")
  }
  "status" {
    Invoke-Compose -ComposeArgs @("ps")
  }
  "logs" {
    Invoke-Compose -ComposeArgs @("logs", "--no-color", "--tail", "$Tail")
  }
  "smoke" {
    Invoke-SmokeCheck
  }
  "seed" {
    Invoke-Seed -ResetCollection:$false -DryRun:$false
  }
  "seed-reset" {
    Invoke-Seed -ResetCollection:$true -DryRun:$false
  }
  "seed-dry-run" {
    Invoke-Seed -ResetCollection:$false -DryRun:$true
  }
  default {
    throw "Unsupported action: $Action"
  }
}




