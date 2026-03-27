package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"modelprobe/backend/internal/model"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresRepository struct {
	db *sql.DB
}

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

	createdAt, err := time.Parse(time.RFC3339Nano, probe.CreatedAt)
	if err != nil {
		return fmt.Errorf("parse createdAt: %w", err)
	}

	query := `
	INSERT INTO probes (
		id, created_at, station_name, group_name, base_url, api_key_hash, api_key_masked,
		claimed_channel, expected_model_family, status, trust_score, verdict, http_status,
		detected_endpoint, response_time_ms, is_openai_compatible, primary_family,
		detected_families_json, model_ids_json, response_headers_json, suspicion_reasons_json,
		notes_json, error_message, raw_excerpt
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
		$13, $14, $15, $16, $17, $18::jsonb, $19::jsonb, $20::jsonb,
		$21::jsonb, $22::jsonb, $23, $24
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
		probe.ErrorMessage,
		probe.RawExcerpt,
	)
	if err != nil {
		return fmt.Errorf("insert probe: %w", err)
	}

	return nil
}

func (r *PostgresRepository) GetProbeByID(ctx context.Context, id string) (*model.ProbeRecord, error) {
	row := r.db.QueryRowContext(ctx, `SELECT * FROM probes WHERE id = $1`, id)

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
	rows, err := r.db.QueryContext(ctx, `SELECT * FROM probes ORDER BY created_at DESC LIMIT $1`, limit)
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
	var httpStatus sql.NullInt64
	var detectedEndpoint sql.NullString
	var responseTimeMS sql.NullInt64
	var primaryFamily sql.NullString
	var errorMessage sql.NullString
	var rawExcerpt sql.NullString
	var isOpenAICompatible bool
	var detectedFamiliesJSON []byte
	var modelIDsJSON []byte
	var responseHeadersJSON []byte
	var suspicionReasonsJSON []byte
	var notesJSON []byte

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
	record.HTTPStatus = nullIntPtr(httpStatus)
	record.DetectedEndpoint = nullStringPtr(detectedEndpoint)
	record.ResponseTimeMS = nullIntPtr(responseTimeMS)
	record.PrimaryFamily = nullStringPtr(primaryFamily)
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
