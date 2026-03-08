$ErrorActionPreference = "Stop"

Write-Host "Run baseline calibration smoke test..."
Push-Location apps/lumina-engine
try {
  uv run python scripts/calibrate_threshold.py --manifest data/baseline/manifest.json
} finally {
  Pop-Location
}

Write-Host "Done."

