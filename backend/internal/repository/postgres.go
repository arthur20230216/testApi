package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"modelprobe/backend/internal/model"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresRepository struct {
	db *sql.DB
}

const probeSelectColumns = `
	id, created_at, station_name, group_name, base_url, api_key_hash, api_key_masked,
	claimed_channel, expected_model_family, status, rule_based_score, rule_based_verdict,
	trust_score, verdict, http_status, detected_endpoint, response_time_ms, is_openai_compatible,
	primary_family, detected_families_json, model_ids_json, response_headers_json,
	suspicion_reasons_json, notes_json, model_score, model_verdict, model_confidence,
	model_summary, model_supporting_signals_json, model_risk_signals_json, model_missing_evidence_json,
	model_reasoning_json, channel_score, channel_verdict, channel_confidence,
	channel_summary, channel_supporting_signals_json, channel_risk_signals_json,
	channel_missing_evidence_json, channel_consistency_json, channel_reasoning_json,
	channel_audit_model, channel_audit_error, audit_evidence_json, error_message, raw_excerpt
`

func NewPostgresRepository(databaseURL string) (*PostgresRepository, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	repo := &PostgresRepository{db: db}
	if err := repo.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

func (r *PostgresRepository) Close() error {
	return r.db.Close()
}

func (r *PostgresRepository) initSchema() error {
	schema := `
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
		rule_based_score INTEGER NOT NULL DEFAULT 0,
		rule_based_verdict TEXT NOT NULL DEFAULT 'needs_review',
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
		model_score INTEGER,
		model_verdict TEXT,
		model_confidence INTEGER,
		model_summary TEXT,
		model_supporting_signals_json JSONB NOT NULL DEFAULT '[]'::jsonb,
		model_risk_signals_json JSONB NOT NULL DEFAULT '[]'::jsonb,
		model_missing_evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb,
		model_reasoning_json JSONB,
		channel_score INTEGER,
		channel_verdict TEXT,
		channel_confidence INTEGER,
		channel_summary TEXT,
		channel_supporting_signals_json JSONB NOT NULL DEFAULT '[]'::jsonb,
		channel_risk_signals_json JSONB NOT NULL DEFAULT '[]'::jsonb,
		channel_missing_evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb,
		channel_consistency_json JSONB,
		channel_reasoning_json JSONB,
		channel_audit_model TEXT,
		channel_audit_error TEXT,
		audit_evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb,
		error_message TEXT,
		raw_excerpt TEXT
	);

	ALTER TABLE probes ADD COLUMN IF NOT EXISTS rule_based_score INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS rule_based_verdict TEXT NOT NULL DEFAULT 'needs_review';
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS model_score INTEGER;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS model_verdict TEXT;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS model_confidence INTEGER;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS model_summary TEXT;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS model_supporting_signals_json JSONB NOT NULL DEFAULT '[]'::jsonb;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS model_risk_signals_json JSONB NOT NULL DEFAULT '[]'::jsonb;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS model_missing_evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS model_reasoning_json JSONB;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_score INTEGER;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_verdict TEXT;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_confidence INTEGER;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_summary TEXT;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_supporting_signals_json JSONB NOT NULL DEFAULT '[]'::jsonb;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_risk_signals_json JSONB NOT NULL DEFAULT '[]'::jsonb;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_missing_evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_consistency_json JSONB;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_reasoning_json JSONB;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_audit_model TEXT;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS channel_audit_error TEXT;
	ALTER TABLE probes ADD COLUMN IF NOT EXISTS audit_evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb;

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

	CREATE TABLE IF NOT EXISTS system_settings (
		setting_key TEXT PRIMARY KEY,
		setting_value TEXT NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

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
	`

	_, err := r.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	if err := r.seedDefaultChannelModels(); err != nil {
		return err
	}

	return nil
}

func (r *PostgresRepository) seedDefaultChannelModels() error {
	defaultPairs := []struct {
		Channel string
		ModelID string
	}{
		{Channel: "cc", ModelID: "claude-sonnet-4.6"},
		{Channel: "cc", ModelID: "claude-opus-4.6"},
		{Channel: "codex", ModelID: "gpt-5.4"},
		{Channel: "codex", ModelID: "gpt-5.3-codex"},
	}

	for _, pair := range defaultPairs {
		_, err := r.db.Exec(
			`INSERT INTO channel_models (channel_name, model_id, is_enabled) VALUES ($1, $2, TRUE)
			 ON CONFLICT(channel_name, model_id) DO NOTHING`,
			pair.Channel,
			pair.ModelID,
		)
		if err != nil {
			return fmt.Errorf("seed channel model %s/%s: %w", pair.Channel, pair.ModelID, err)
		}
	}

	return nil
}

func (r *PostgresRepository) CreateProbe(ctx context.Context, probe model.ProbeRecord) error {
	detectedFamiliesJSON, _ := json.Marshal(probe.DetectedFamilies)
	modelIDsJSON, _ := json.Marshal(probe.ModelIDs)
	responseHeadersJSON, _ := json.Marshal(probe.ResponseHeaders)
	suspicionReasonsJSON, _ := json.Marshal(probe.SuspicionReasons)
	notesJSON, _ := json.Marshal(probe.Notes)
	modelSupportingSignalsJSON, _ := json.Marshal(probe.ModelSupportingSignals)
	modelRiskSignalsJSON, _ := json.Marshal(probe.ModelRiskSignals)
	modelMissingEvidenceJSON, _ := json.Marshal(probe.ModelMissingEvidence)
	modelReasoningJSON, _ := json.Marshal(probe.ModelReasoning)
	channelSupportingSignalsJSON, _ := json.Marshal(probe.ChannelSupportingSignals)
	channelRiskSignalsJSON, _ := json.Marshal(probe.ChannelRiskSignals)
	channelMissingEvidenceJSON, _ := json.Marshal(probe.ChannelMissingEvidence)
	channelConsistencyJSON, _ := json.Marshal(probe.ChannelConsistency)
	channelReasoningJSON, _ := json.Marshal(probe.ChannelReasoning)
	auditEvidenceJSON, _ := json.Marshal(probe.AuditEvidence)

	createdAt, err := time.Parse(time.RFC3339Nano, probe.CreatedAt)
	if err != nil {
		return fmt.Errorf("parse createdAt: %w", err)
	}

	query := `
	INSERT INTO probes (
		id, created_at, station_name, group_name, base_url, api_key_hash, api_key_masked,
		claimed_channel, expected_model_family, status, rule_based_score, rule_based_verdict,
		trust_score, verdict, http_status,
		detected_endpoint, response_time_ms, is_openai_compatible, primary_family,
		detected_families_json, model_ids_json, response_headers_json, suspicion_reasons_json,
		notes_json, model_score, model_verdict, model_confidence, model_summary,
		model_supporting_signals_json, model_risk_signals_json, model_missing_evidence_json,
		model_reasoning_json, channel_score, channel_verdict, channel_confidence, channel_summary,
		channel_supporting_signals_json, channel_risk_signals_json, channel_missing_evidence_json,
		channel_consistency_json, channel_reasoning_json, channel_audit_model, channel_audit_error,
		audit_evidence_json, error_message, raw_excerpt
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
		$13, $14, $15, $16, $17, $18, $19,
		$20::jsonb, $21::jsonb, $22::jsonb, $23::jsonb, $24::jsonb,
		$25, $26, $27, $28, $29::jsonb, $30::jsonb, $31::jsonb,
		$32::jsonb, $33, $34, $35, $36, $37::jsonb, $38::jsonb, $39::jsonb,
		$40::jsonb, $41::jsonb, $42, $43, $44::jsonb, $45, $46
	)
	`

	_, err = r.db.ExecContext(
		ctx,
		query,
		probe.ID,
		createdAt,
		probe.StationName,
		probe.GroupName,
		probe.BaseURL,
		probe.APIKeyHash,
		probe.APIKeyMasked,
		probe.ClaimedChannel,
		probe.ExpectedModelFamily,
		probe.Status,
		probe.RuleBasedScore,
		probe.RuleBasedVerdict,
		probe.TrustScore,
		probe.Verdict,
		probe.HTTPStatus,
		probe.DetectedEndpoint,
		probe.ResponseTimeMS,
		probe.IsOpenAICompatible,
		probe.PrimaryFamily,
		string(detectedFamiliesJSON),
		string(modelIDsJSON),
		string(responseHeadersJSON),
		string(suspicionReasonsJSON),
		string(notesJSON),
		probe.ModelScore,
		probe.ModelVerdict,
		probe.ModelConfidence,
		probe.ModelSummary,
		string(modelSupportingSignalsJSON),
		string(modelRiskSignalsJSON),
		string(modelMissingEvidenceJSON),
		nullableJSON(modelReasoningJSON),
		probe.ChannelScore,
		probe.ChannelVerdict,
		probe.ChannelConfidence,
		probe.ChannelSummary,
		string(channelSupportingSignalsJSON),
		string(channelRiskSignalsJSON),
		string(channelMissingEvidenceJSON),
		nullableJSON(channelConsistencyJSON),
		nullableJSON(channelReasoningJSON),
		probe.ChannelAuditModel,
		probe.ChannelAuditError,
		string(auditEvidenceJSON),
		probe.ErrorMessage,
		probe.RawExcerpt,
	)
	if err != nil {
		return fmt.Errorf("insert probe: %w", err)
	}

	return nil
}

func (r *PostgresRepository) GetProbeByID(ctx context.Context, id string) (*model.ProbeRecord, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+probeSelectColumns+` FROM probes WHERE id = $1`, id)

	record, err := scanProbe(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}

		return nil, err
	}

	return record, nil
}

func (r *PostgresRepository) ListRecentProbes(ctx context.Context, limit int) ([]model.ProbeRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+probeSelectColumns+` FROM probes ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list probes: %w", err)
	}
	defer rows.Close()

	items := make([]model.ProbeRecord, 0)
	for rows.Next() {
		record, err := scanProbe(rows)
		if err != nil {
			return nil, err
		}

		items = append(items, *record)
	}

	return items, rows.Err()
}

func (r *PostgresRepository) GetRanking(ctx context.Context, scope string, limit int) (model.RankingResponse, error) {
	column := "station_name"
	whereClause := "station_name <> ''"
	if scope == "group" {
		column = "group_name"
		whereClause = "group_name IS NOT NULL AND group_name <> ''"
	}

	baseQuery := fmt.Sprintf(`
		SELECT
			%[1]s AS name,
			COUNT(*) AS total_probes,
			ROUND(AVG(trust_score)::numeric, 1) AS avg_score,
			ROUND(AVG(CASE WHEN status = 'success' THEN 100.0 ELSE 0 END)::numeric, 1) AS success_rate,
			SUM(CASE WHEN verdict = 'high_risk' THEN 1 ELSE 0 END) AS high_risk_count,
			MAX(created_at) AS last_probe_at
		FROM probes
		WHERE %[2]s
		GROUP BY %[1]s
		HAVING COUNT(*) > 0
	`, column, whereClause)

	redRows, err := r.db.QueryContext(ctx, baseQuery+" ORDER BY avg_score DESC, total_probes DESC, last_probe_at DESC LIMIT $1", limit)
	if err != nil {
		return model.RankingResponse{}, fmt.Errorf("query red ranking: %w", err)
	}

	red, err := scanRankingRows(redRows)
	_ = redRows.Close()
	if err != nil {
		return model.RankingResponse{}, err
	}

	blackRows, err := r.db.QueryContext(ctx, baseQuery+" ORDER BY avg_score ASC, high_risk_count DESC, total_probes DESC LIMIT $1", limit)
	if err != nil {
		return model.RankingResponse{}, fmt.Errorf("query black ranking: %w", err)
	}
	defer blackRows.Close()

	black, err := scanRankingRows(blackRows)
	if err != nil {
		return model.RankingResponse{}, err
	}

	return model.RankingResponse{Red: red, Black: black}, nil
}

func (r *PostgresRepository) ListChannelModels(ctx context.Context) ([]model.ChannelModelEntry, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, channel_name, model_id, is_enabled, created_at, updated_at
		 FROM channel_models
		 ORDER BY channel_name ASC, model_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list channel models: %w", err)
	}
	defer rows.Close()

	items := make([]model.ChannelModelEntry, 0)
	for rows.Next() {
		var item model.ChannelModelEntry
		var createdAt time.Time
		var updatedAt time.Time
		if err := rows.Scan(
			&item.ID,
			&item.ChannelName,
			&item.ModelID,
			&item.IsEnabled,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan channel model row: %w", err)
		}

		item.ChannelName = strings.ToLower(strings.TrimSpace(item.ChannelName))
		item.ModelID = strings.ToLower(strings.TrimSpace(item.ModelID))
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *PostgresRepository) GetChannelModelMap(ctx context.Context, includeDisabled bool) (map[string][]string, error) {
	whereEnabled := ""
	if !includeDisabled {
		whereEnabled = "WHERE is_enabled = TRUE"
	}

	rows, err := r.db.QueryContext(
		ctx,
		fmt.Sprintf(`SELECT channel_name, model_id FROM channel_models %s ORDER BY channel_name ASC, model_id ASC`, whereEnabled),
	)
	if err != nil {
		return nil, fmt.Errorf("list channel model map: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var channel string
		var modelID string
		if err := rows.Scan(&channel, &modelID); err != nil {
			return nil, fmt.Errorf("scan channel model map row: %w", err)
		}

		channel = strings.ToLower(strings.TrimSpace(channel))
		modelID = strings.ToLower(strings.TrimSpace(modelID))
		result[channel] = append(result[channel], modelID)
	}

	return result, rows.Err()
}

func (r *PostgresRepository) IsModelAllowedForChannel(ctx context.Context, channel string, modelID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(
		ctx,
		`SELECT EXISTS(
			SELECT 1
			FROM channel_models
			WHERE channel_name = $1 AND model_id = $2 AND is_enabled = TRUE
		)`,
		strings.ToLower(strings.TrimSpace(channel)),
		strings.ToLower(strings.TrimSpace(modelID)),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check channel model: %w", err)
	}

	return exists, nil
}

func (r *PostgresRepository) UpsertChannelModel(ctx context.Context, request model.ChannelModelUpsertRequest) (*model.ChannelModelEntry, error) {
	normalized := request.Normalize()
	row := r.db.QueryRowContext(
		ctx,
		`INSERT INTO channel_models (channel_name, model_id, is_enabled, updated_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT(channel_name, model_id)
		 DO UPDATE SET is_enabled = EXCLUDED.is_enabled, updated_at = NOW()
		 RETURNING id, channel_name, model_id, is_enabled, created_at, updated_at`,
		normalized.ChannelName,
		normalized.ModelID,
		normalized.IsEnabled,
	)

	var item model.ChannelModelEntry
	var createdAt time.Time
	var updatedAt time.Time
	if err := row.Scan(
		&item.ID,
		&item.ChannelName,
		&item.ModelID,
		&item.IsEnabled,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, fmt.Errorf("upsert channel model: %w", err)
	}

	item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	return &item, nil
}

func (r *PostgresRepository) DeleteChannelModel(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM channel_models WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete channel model: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete channel model rows affected: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *PostgresRepository) GetSystemSettings(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT setting_key, setting_value FROM system_settings`)
	if err != nil {
		return nil, fmt.Errorf("get system settings: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan system setting: %w", err)
		}
		result[key] = value
	}

	return result, rows.Err()
}

func (r *PostgresRepository) UpsertSystemSettings(ctx context.Context, values map[string]*string) error {
	for key, value := range values {
		if value == nil {
			if _, err := r.db.ExecContext(ctx, `DELETE FROM system_settings WHERE setting_key = $1`, key); err != nil {
				return fmt.Errorf("delete system setting %s: %w", key, err)
			}
			continue
		}

		if _, err := r.db.ExecContext(
			ctx,
			`INSERT INTO system_settings (setting_key, setting_value, updated_at)
			 VALUES ($1, $2, NOW())
			 ON CONFLICT(setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value, updated_at = NOW()`,
			key,
			*value,
		); err != nil {
			return fmt.Errorf("upsert system setting %s: %w", key, err)
		}
	}

	return nil
}

func (r *PostgresRepository) UpdateProbeManual(ctx context.Context, id string, request model.ProbeManualUpdateRequest) (*model.ProbeRecord, error) {
	modelIDsJSON, _ := json.Marshal(request.ModelIDs)
	suspicionReasonsJSON, _ := json.Marshal(request.SuspicionReasons)
	notesJSON, _ := json.Marshal(request.Notes)

	result, err := r.db.ExecContext(
		ctx,
		`UPDATE probes
		 SET
			claimed_channel = $2,
			expected_model_family = $3,
			status = $4,
			verdict = $5,
			trust_score = $6,
			primary_family = $7,
			model_ids_json = $8::jsonb,
			suspicion_reasons_json = $9::jsonb,
			notes_json = $10::jsonb
		 WHERE id = $1`,
		id,
		request.ClaimedChannel,
		request.ExpectedModelFamily,
		request.Status,
		request.Verdict,
		request.TrustScore,
		nullableUpdateText(request.PrimaryFamily),
		string(modelIDsJSON),
		string(suspicionReasonsJSON),
		string(notesJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("update probe manually: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("update probe rows affected: %w", err)
	}
	if affected == 0 {
		return nil, sql.ErrNoRows
	}

	return r.GetProbeByID(ctx, id)
}

func (r *PostgresRepository) CountAdminUsers(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count admin users: %w", err)
	}
	return count, nil
}

func (r *PostgresRepository) CreateAdminUser(ctx context.Context, username string, passwordHash string) (*model.AdminUserRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`INSERT INTO admin_users (username, password_hash, updated_at)
		 VALUES ($1, $2, NOW())
		 RETURNING id, username, password_hash, created_at, updated_at, last_login_at`,
		strings.TrimSpace(username),
		passwordHash,
	)

	record, err := scanAdminUser(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("admin username already exists")
		}
		return nil, fmt.Errorf("create admin user: %w", err)
	}

	return record, nil
}

func (r *PostgresRepository) GetAdminUserByUsername(ctx context.Context, username string) (*model.AdminUserRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, created_at, updated_at, last_login_at
		 FROM admin_users
		 WHERE username = $1`,
		strings.TrimSpace(username),
	)

	record, err := scanAdminUser(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get admin user by username: %w", err)
	}

	return record, nil
}

func (r *PostgresRepository) GetAdminUserByID(ctx context.Context, id int64) (*model.AdminUserRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, created_at, updated_at, last_login_at
		 FROM admin_users
		 WHERE id = $1`,
		id,
	)

	record, err := scanAdminUser(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get admin user by id: %w", err)
	}

	return record, nil
}

func (r *PostgresRepository) UpdateAdminUserCredentials(ctx context.Context, id int64, username string, passwordHash string, updatePassword bool) (*model.AdminUserRecord, error) {
	query := `
		UPDATE admin_users
		SET username = $2,
			password_hash = CASE WHEN $4 THEN $3 ELSE password_hash END,
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, username, password_hash, created_at, updated_at, last_login_at
	`
	row := r.db.QueryRowContext(
		ctx,
		query,
		id,
		strings.TrimSpace(username),
		passwordHash,
		updatePassword,
	)

	record, err := scanAdminUser(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("admin username already exists")
		}
		return nil, fmt.Errorf("update admin user credentials: %w", err)
	}

	return record, nil
}

func (r *PostgresRepository) TouchAdminUserLastLogin(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE admin_users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("touch admin user last login: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateAdminSession(ctx context.Context, adminUserID int64, tokenHash string, expiresAt time.Time, userAgent string, ipAddress string) (*model.AdminSessionRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`INSERT INTO admin_sessions (admin_user_id, token_hash, expires_at, user_agent, ip_address)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, admin_user_id, token_hash, expires_at, created_at, last_seen_at, user_agent, ip_address`,
		adminUserID,
		tokenHash,
		expiresAt.UTC(),
		nullableUpdateText(userAgent),
		nullableUpdateText(ipAddress),
	)

	record, err := scanAdminSession(row)
	if err != nil {
		return nil, fmt.Errorf("create admin session: %w", err)
	}
	return record, nil
}

func (r *PostgresRepository) GetAdminSessionByTokenHash(ctx context.Context, tokenHash string) (*model.AdminSessionRecord, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, admin_user_id, token_hash, expires_at, created_at, last_seen_at, user_agent, ip_address
		 FROM admin_sessions
		 WHERE token_hash = $1`,
		tokenHash,
	)

	record, err := scanAdminSession(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get admin session by token hash: %w", err)
	}

	return record, nil
}

func (r *PostgresRepository) TouchAdminSession(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE admin_sessions SET last_seen_at = NOW() WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("touch admin session: %w", err)
	}
	return nil
}

func (r *PostgresRepository) DeleteAdminSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete admin session: %w", err)
	}
	return nil
}

func (r *PostgresRepository) DeleteExpiredAdminSessions(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at <= NOW()`)
	if err != nil {
		return fmt.Errorf("delete expired admin sessions: %w", err)
	}
	return nil
}

func scanRankingRows(rows *sql.Rows) ([]model.RankingItem, error) {
	items := make([]model.RankingItem, 0)

	for rows.Next() {
		var item model.RankingItem
		var lastProbeAt time.Time
		if err := rows.Scan(
			&item.Name,
			&item.TotalProbes,
			&item.AvgScore,
			&item.SuccessRate,
			&item.HighRiskCount,
			&lastProbeAt,
		); err != nil {
			return nil, fmt.Errorf("scan ranking row: %w", err)
		}

		item.LastProbeAt = lastProbeAt.UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}

	return items, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProbe(row scanner) (*model.ProbeRecord, error) {
	var record model.ProbeRecord
	var createdAt time.Time
	var groupName sql.NullString
	var claimedChannel sql.NullString
	var expectedModelFamily sql.NullString
	var ruleBasedVerdict string
	var httpStatus sql.NullInt64
	var detectedEndpoint sql.NullString
	var responseTimeMS sql.NullInt64
	var primaryFamily sql.NullString
	var modelScore sql.NullInt64
	var modelVerdict sql.NullString
	var modelConfidence sql.NullInt64
	var modelSummary sql.NullString
	var channelScore sql.NullInt64
	var channelVerdict sql.NullString
	var channelConfidence sql.NullInt64
	var channelSummary sql.NullString
	var modelReasoningJSON []byte
	var channelConsistencyJSON []byte
	var channelReasoningJSON []byte
	var channelAuditModel sql.NullString
	var channelAuditError sql.NullString
	var auditEvidenceJSON []byte
	var errorMessage sql.NullString
	var rawExcerpt sql.NullString
	var isOpenAICompatible bool
	var detectedFamiliesJSON []byte
	var modelIDsJSON []byte
	var responseHeadersJSON []byte
	var suspicionReasonsJSON []byte
	var notesJSON []byte
	var modelSupportingSignalsJSON []byte
	var modelRiskSignalsJSON []byte
	var modelMissingEvidenceJSON []byte
	var channelSupportingSignalsJSON []byte
	var channelRiskSignalsJSON []byte
	var channelMissingEvidenceJSON []byte

	err := row.Scan(
		&record.ID,
		&createdAt,
		&record.StationName,
		&groupName,
		&record.BaseURL,
		&record.APIKeyHash,
		&record.APIKeyMasked,
		&claimedChannel,
		&expectedModelFamily,
		&record.Status,
		&record.RuleBasedScore,
		&ruleBasedVerdict,
		&record.TrustScore,
		&record.Verdict,
		&httpStatus,
		&detectedEndpoint,
		&responseTimeMS,
		&isOpenAICompatible,
		&primaryFamily,
		&detectedFamiliesJSON,
		&modelIDsJSON,
		&responseHeadersJSON,
		&suspicionReasonsJSON,
		&notesJSON,
		&modelScore,
		&modelVerdict,
		&modelConfidence,
		&modelSummary,
		&modelSupportingSignalsJSON,
		&modelRiskSignalsJSON,
		&modelMissingEvidenceJSON,
		&modelReasoningJSON,
		&channelScore,
		&channelVerdict,
		&channelConfidence,
		&channelSummary,
		&channelSupportingSignalsJSON,
		&channelRiskSignalsJSON,
		&channelMissingEvidenceJSON,
		&channelConsistencyJSON,
		&channelReasoningJSON,
		&channelAuditModel,
		&channelAuditError,
		&auditEvidenceJSON,
		&errorMessage,
		&rawExcerpt,
	)
	if err != nil {
		return nil, err
	}

	record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	record.GroupName = nullStringPtr(groupName)
	record.ClaimedChannel = nullStringPtr(claimedChannel)
	record.ExpectedModelFamily = nullStringPtr(expectedModelFamily)
	record.RuleBasedVerdict = ruleBasedVerdict
	record.HTTPStatus = nullIntPtr(httpStatus)
	record.DetectedEndpoint = nullStringPtr(detectedEndpoint)
	record.ResponseTimeMS = nullIntPtr(responseTimeMS)
	record.PrimaryFamily = nullStringPtr(primaryFamily)
	record.ModelScore = nullIntPtr(modelScore)
	record.ModelVerdict = nullStringPtr(modelVerdict)
	record.ModelConfidence = nullIntPtr(modelConfidence)
	record.ModelSummary = nullStringPtr(modelSummary)
	record.ChannelScore = nullIntPtr(channelScore)
	record.ChannelVerdict = nullStringPtr(channelVerdict)
	record.ChannelConfidence = nullIntPtr(channelConfidence)
	record.ChannelSummary = nullStringPtr(channelSummary)
	record.ChannelAuditModel = nullStringPtr(channelAuditModel)
	record.ChannelAuditError = nullStringPtr(channelAuditError)
	record.ErrorMessage = nullStringPtr(errorMessage)
	record.RawExcerpt = nullStringPtr(rawExcerpt)
	record.IsOpenAICompatible = isOpenAICompatible

	if err := json.Unmarshal(detectedFamiliesJSON, &record.DetectedFamilies); err != nil {
		return nil, fmt.Errorf("decode detected families: %w", err)
	}

	if err := json.Unmarshal(modelIDsJSON, &record.ModelIDs); err != nil {
		return nil, fmt.Errorf("decode model ids: %w", err)
	}

	if err := json.Unmarshal(responseHeadersJSON, &record.ResponseHeaders); err != nil {
		return nil, fmt.Errorf("decode response headers: %w", err)
	}

	if err := json.Unmarshal(suspicionReasonsJSON, &record.SuspicionReasons); err != nil {
		return nil, fmt.Errorf("decode suspicion reasons: %w", err)
	}

	if err := json.Unmarshal(notesJSON, &record.Notes); err != nil {
		return nil, fmt.Errorf("decode notes: %w", err)
	}

	if err := json.Unmarshal(modelSupportingSignalsJSON, &record.ModelSupportingSignals); err != nil {
		return nil, fmt.Errorf("decode model supporting signals: %w", err)
	}

	if err := json.Unmarshal(modelRiskSignalsJSON, &record.ModelRiskSignals); err != nil {
		return nil, fmt.Errorf("decode model risk signals: %w", err)
	}

	if err := json.Unmarshal(modelMissingEvidenceJSON, &record.ModelMissingEvidence); err != nil {
		return nil, fmt.Errorf("decode model missing evidence: %w", err)
	}

	if err := json.Unmarshal(channelSupportingSignalsJSON, &record.ChannelSupportingSignals); err != nil {
		return nil, fmt.Errorf("decode channel supporting signals: %w", err)
	}

	if err := json.Unmarshal(channelRiskSignalsJSON, &record.ChannelRiskSignals); err != nil {
		return nil, fmt.Errorf("decode channel risk signals: %w", err)
	}

	if err := json.Unmarshal(channelMissingEvidenceJSON, &record.ChannelMissingEvidence); err != nil {
		return nil, fmt.Errorf("decode channel missing evidence: %w", err)
	}

	if len(modelReasoningJSON) > 0 && string(modelReasoningJSON) != "null" {
		var reasoning model.ModelReasoning
		if err := json.Unmarshal(modelReasoningJSON, &reasoning); err != nil {
			return nil, fmt.Errorf("decode model reasoning: %w", err)
		}
		record.ModelReasoning = &reasoning
	}

	if len(channelConsistencyJSON) > 0 && string(channelConsistencyJSON) != "null" {
		var consistency model.ChannelConsistency
		if err := json.Unmarshal(channelConsistencyJSON, &consistency); err != nil {
			return nil, fmt.Errorf("decode channel consistency: %w", err)
		}
		record.ChannelConsistency = &consistency
	}

	if len(channelReasoningJSON) > 0 && string(channelReasoningJSON) != "null" {
		var reasoning model.ChannelReasoning
		if err := json.Unmarshal(channelReasoningJSON, &reasoning); err != nil {
			return nil, fmt.Errorf("decode channel reasoning: %w", err)
		}
		record.ChannelReasoning = &reasoning
	}

	if err := json.Unmarshal(auditEvidenceJSON, &record.AuditEvidence); err != nil {
		return nil, fmt.Errorf("decode audit evidence: %w", err)
	}

	return &record, nil
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	copied := value.String
	return &copied
}

func nullIntPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}

	copied := int(value.Int64)
	return &copied
}

func nullableUpdateText(value string) any {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil
	}
	return normalized
}

func nullableJSON(value []byte) any {
	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	return trimmed
}

func scanAdminUser(row scanner) (*model.AdminUserRecord, error) {
	var record model.AdminUserRecord
	var createdAt time.Time
	var updatedAt time.Time
	var lastLoginAt sql.NullTime

	if err := row.Scan(
		&record.ID,
		&record.Username,
		&record.PasswordHash,
		&createdAt,
		&updatedAt,
		&lastLoginAt,
	); err != nil {
		return nil, err
	}

	record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	record.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	record.LastLoginAt = nullTimePtr(lastLoginAt)
	return &record, nil
}

func scanAdminSession(row scanner) (*model.AdminSessionRecord, error) {
	var record model.AdminSessionRecord
	var expiresAt time.Time
	var createdAt time.Time
	var lastSeenAt time.Time
	var userAgent sql.NullString
	var ipAddress sql.NullString

	if err := row.Scan(
		&record.ID,
		&record.AdminUserID,
		&record.TokenHash,
		&expiresAt,
		&createdAt,
		&lastSeenAt,
		&userAgent,
		&ipAddress,
	); err != nil {
		return nil, err
	}

	record.ExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
	record.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	record.LastSeenAt = lastSeenAt.UTC().Format(time.RFC3339Nano)
	record.UserAgent = nullStringPtr(userAgent)
	record.IPAddress = nullStringPtr(ipAddress)
	return &record, nil
}

func nullTimePtr(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}

	formatted := value.Time.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
