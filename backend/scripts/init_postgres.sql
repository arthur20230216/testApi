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

CREATE TABLE IF NOT EXISTS channel_models (
  id BIGSERIAL PRIMARY KEY,
  channel_name TEXT NOT NULL,
  model_id TEXT NOT NULL,
  is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(channel_name, model_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_models_channel_name ON channel_models(channel_name);
CREATE INDEX IF NOT EXISTS idx_channel_models_is_enabled ON channel_models(is_enabled);

CREATE TABLE IF NOT EXISTS admin_users (
  id BIGSERIAL PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_login_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS admin_sessions (
  id BIGSERIAL PRIMARY KEY,
  admin_user_id BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  user_agent TEXT,
  ip_address TEXT
);

CREATE INDEX IF NOT EXISTS idx_admin_sessions_admin_user_id ON admin_sessions(admin_user_id);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires_at ON admin_sessions(expires_at);

INSERT INTO channel_models(channel_name, model_id, is_enabled) VALUES
  ('cc', 'claude-sonnet-4.6', TRUE),
  ('cc', 'claude-opus-4.6', TRUE),
  ('codex', 'gpt-5.4', TRUE),
  ('codex', 'gpt-5.3-codex', TRUE)
ON CONFLICT(channel_name, model_id) DO NOTHING;
