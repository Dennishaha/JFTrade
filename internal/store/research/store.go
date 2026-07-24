package research

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	domain "github.com/jftrade/jftrade-main/internal/research"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

type Store struct {
	db   *sqliteconn.DB
	path string
}

func Open(ctx context.Context, path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("research database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create research database directory: %w", err)
	}
	db, err := sqliteconn.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open research database: %w", err)
	}
	store := &Store{db: db, path: path}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

type presetRow struct {
	ID                 string `db:"preset_id"`
	Name               string `db:"name"`
	QuerySchemaVersion int    `db:"query_schema_version"`
	QueryJSON          string `db:"query_json"`
	Revision           int64  `db:"revision"`
	CreatedAt          string `db:"created_at"`
	UpdatedAt          string `db:"updated_at"`
}

func (s *Store) ListScreenPresets(ctx context.Context) ([]domain.ScreenPreset, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows := []presetRow{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT preset_id, name, query_schema_version, query_json, revision, created_at, updated_at
		FROM research_screen_presets ORDER BY updated_at DESC, preset_id`); err != nil {
		return nil, err
	}
	presets := make([]domain.ScreenPreset, 0, len(rows))
	for _, row := range rows {
		preset, err := row.preset()
		if err != nil {
			return nil, err
		}
		presets = append(presets, preset)
	}
	return presets, nil
}

func (s *Store) GetScreenPreset(ctx context.Context, presetID string) (domain.ScreenPreset, error) {
	if err := s.available(); err != nil {
		return domain.ScreenPreset{}, err
	}
	var row presetRow
	err := s.db.GetContext(ctx, &row, `SELECT preset_id, name, query_schema_version, query_json, revision, created_at, updated_at
		FROM research_screen_presets WHERE preset_id = ?`, strings.TrimSpace(presetID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ScreenPreset{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ScreenPreset{}, err
	}
	return row.preset()
}

func (s *Store) CreateScreenPreset(ctx context.Context, name string, definition broker.ScreenDefinitionV2, schemaVersion int) (domain.ScreenPreset, error) {
	if err := s.available(); err != nil {
		return domain.ScreenPreset{}, err
	}
	body, err := json.Marshal(definition)
	if err != nil {
		return domain.ScreenPreset{}, fmt.Errorf("encode research screen preset definition: %w", err)
	}
	presetID := "rsp_" + uuid.NewString()
	now := nowText()
	_, err = s.db.ExecContext(ctx, `INSERT INTO research_screen_presets
		(preset_id, name, name_key, query_schema_version, query_json, revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		presetID, name, presetNameKey(name), schemaVersion, string(body), now, now)
	if err != nil {
		return domain.ScreenPreset{}, mapWriteError(err)
	}
	return s.GetScreenPreset(ctx, presetID)
}

func (s *Store) UpdateScreenPreset(ctx context.Context, presetID, name string, definition broker.ScreenDefinitionV2, schemaVersion int, expectedRevision int64) (domain.ScreenPreset, error) {
	if err := s.available(); err != nil {
		return domain.ScreenPreset{}, err
	}
	body, err := json.Marshal(definition)
	if err != nil {
		return domain.ScreenPreset{}, fmt.Errorf("encode research screen preset definition: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE research_screen_presets
		SET name = ?, name_key = ?, query_schema_version = ?, query_json = ?, revision = revision + 1, updated_at = ?
		WHERE preset_id = ? AND revision = ?`,
		name, presetNameKey(name), schemaVersion, string(body), nowText(), strings.TrimSpace(presetID), expectedRevision)
	if err != nil {
		return domain.ScreenPreset{}, mapWriteError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.ScreenPreset{}, err
	}
	if affected == 0 {
		if _, err := s.GetScreenPreset(ctx, presetID); err != nil {
			return domain.ScreenPreset{}, err
		}
		return domain.ScreenPreset{}, domain.ErrConflict
	}
	return s.GetScreenPreset(ctx, presetID)
}

func (s *Store) DeleteScreenPreset(ctx context.Context, presetID string) error {
	if err := s.available(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM research_screen_presets WHERE preset_id = ?`, strings.TrimSpace(presetID))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (row presetRow) preset() (domain.ScreenPreset, error) {
	if row.QuerySchemaVersion != domain.QuerySchemaVersion {
		return domain.ScreenPreset{}, fmt.Errorf(
			"research screen preset %s query schema version %d is unsupported; only version %d is supported",
			row.ID, row.QuerySchemaVersion, domain.QuerySchemaVersion,
		)
	}
	var definition broker.ScreenDefinitionV2
	if err := json.Unmarshal([]byte(row.QueryJSON), &definition); err != nil {
		return domain.ScreenPreset{}, fmt.Errorf("decode research screen preset %s definition: %w", row.ID, err)
	}
	normalized, err := researchscreen.NormalizeDefinitionV2(definition)
	if err != nil {
		return domain.ScreenPreset{}, fmt.Errorf("validate research screen preset %s definition: %w", row.ID, err)
	}
	return domain.ScreenPreset{
		ID: row.ID, Name: row.Name, QuerySchemaVersion: domain.QuerySchemaVersion, Definition: normalized,
		Revision: row.Revision, CreatedAt: parseTime(row.CreatedAt), UpdatedAt: parseTime(row.UpdatedAt),
	}, nil
}

func (s *Store) available() error {
	if s == nil || s.db == nil {
		return domain.ErrUnavailable
	}
	return nil
}

func presetNameKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed") {
		return fmt.Errorf("%w: %v", domain.ErrConflict, err)
	}
	return err
}

func parseTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}
