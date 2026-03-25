CREATE TABLE IF NOT EXISTS probes (
  id TEXT PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  station_name TEXT NOT NULL,
  group_name TEXT,
  base_url TEXT NOT NULL,
  api_key_hash TEXT NOT NULL,
  api_key_masked TEXT NOT NULL,
  claimed_channel TEXT,
  expected_model_family TEXT,
  status TEXT NOT NULL,
  trust_score INTEGER NOT NULL,
  verdict TEXT NOT NULL,
  http_status INTEGER,
  detected_endpoint TEXT,
  response_time_ms INTEGER,
  is_openai_compatible BOOLEAN NOT NULL DEFAULT FALSE,
  primary_family TEXT,
  detected_families_json JSONB NOT NULL,
  model_ids_json JSONB NOT NULL,
  response_headers_json JSONB NOT NULL,
  suspicion_reasons_json JSONB NOT NULL,
  notes_json JSONB NOT NULL,
  error_message TEXT,
  raw_excerpt TEXT
);

CREATE INDEX IF NOT EXISTS idx_probes_created_at ON probes(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_probes_station_name ON probes(station_name);
CREATE INDEX IF NOT EXISTS idx_probes_group_name ON probes(group_name);
CREATE INDEX IF NOT EXISTS idx_probes_verdict ON probes(verdict);
