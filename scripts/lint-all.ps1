$ErrorActionPreference = "Stop"

Write-Host "Lint AI Processing (Python)..."
Push-Location apps/lumina-engine
try {
  uv run python -m compileall app scripts
} finally {
  Pop-Location
}

Write-Host "Done."

