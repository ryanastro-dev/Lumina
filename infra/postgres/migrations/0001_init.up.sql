CREATE TABLE IF NOT EXISTS documents (
  id UUID PRIMARY KEY,
  external_id TEXT UNIQUE,
  source TEXT,
  language TEXT NOT NULL DEFAULT 'en',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS plagiarism_checks (
  id UUID PRIMARY KEY,
  query_hash TEXT NOT NULL,
  threshold NUMERIC(4,3) NOT NULL,
  overall_score NUMERIC(5,4) NOT NULL,
  is_plagiarism BOOLEAN NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
