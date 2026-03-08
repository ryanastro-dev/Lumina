CREATE TABLE IF NOT EXISTS scan_jobs (
  id UUID PRIMARY KEY,
  request_id TEXT,
  text TEXT NOT NULL,
  threshold REAL NOT NULL,
  top_k INTEGER NOT NULL,
  status TEXT NOT NULL,
  overall_score REAL,
  is_plagiarism BOOLEAN,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS scan_matches (
  job_id UUID NOT NULL REFERENCES scan_jobs(id) ON DELETE CASCADE,
  rank INTEGER NOT NULL,
  document_id TEXT NOT NULL,
  source TEXT,
  chunk_id TEXT NOT NULL,
  matched_text TEXT NOT NULL,
  semantic_score REAL NOT NULL,
  exact_score REAL NOT NULL,
  final_score REAL NOT NULL,
  PRIMARY KEY (job_id, rank)
);

CREATE INDEX IF NOT EXISTS idx_scan_jobs_status_created_at
  ON scan_jobs(status, created_at DESC);
