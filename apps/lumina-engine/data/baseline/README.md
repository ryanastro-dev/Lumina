## Baseline Dataset Plan (20-50 docs)

Create a small English-only baseline set with these groups:

1. `exact-copy` (5-10 docs)
- Source and copied text are almost identical.

2. `light-paraphrase` (5-10 docs)
- Same idea, minor word changes.

3. `heavy-paraphrase` (5-10 docs)
- Same meaning, restructured sentences.

4. `unrelated` (5-10 docs)
- Different topic, should score low.

5. `proper-citation` (2-5 docs)
- Quoted text with citation markers.

This scaffold includes:
- `manifest.json` with source/case metadata
- `sources/` for canonical source texts
- `cases/` grouped by scenario

Run calibration:
```powershell
uv run python scripts/calibrate_threshold.py `
  --manifest data/baseline/manifest.json `
  --output-json data/baseline/report.json
```

Use this set to calibrate threshold before production.
