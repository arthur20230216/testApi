package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	`

	_, err := r.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
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
