package watchlist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
	"github.com/jmoiron/sqlx"
)

type Store struct {
	db   *sqliteconn.DB
	path string
}

func Open(ctx context.Context, path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("watchlist database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create watchlist database directory: %w", err)
	}
	db, err := sqliteconn.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open watchlist database: %w", err)
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

func (s *Store) DB() *sqliteconn.DB {
	if s == nil {
		return nil
	}
	return s.db
}

type groupRow struct {
	ID        string `db:"group_id"`
	Name      string `db:"name"`
	IsDefault int    `db:"is_default"`
	Protected int    `db:"protected"`
	Revision  int64  `db:"revision"`
	ItemCount int    `db:"item_count"`
	CreatedAt string `db:"created_at"`
	UpdatedAt string `db:"updated_at"`
}

func (s *Store) ListGroups(ctx context.Context) ([]domain.Group, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows := []groupRow{}
	err := s.db.SelectContext(ctx, &rows, `SELECT g.group_id, g.name, g.is_default, g.protected, g.revision,
		COUNT(m.instrument_id) AS item_count, g.created_at, g.updated_at
		FROM watchlist_groups g LEFT JOIN watchlist_memberships m ON m.group_id = g.group_id
		GROUP BY g.group_id ORDER BY g.is_default DESC, g.created_at, g.group_id`)
	if err != nil {
		return nil, err
	}
	groups := make([]domain.Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, row.group())
	}
	return groups, nil
}

func (s *Store) GetGroup(ctx context.Context, groupID string) (domain.Group, error) {
	if err := s.available(); err != nil {
		return domain.Group{}, err
	}
	return getGroup(ctx, s.db, groupID)
}

func getGroup(ctx context.Context, query sqlx.QueryerContext, groupID string) (domain.Group, error) {
	var row groupRow
	err := sqlx.GetContext(ctx, query, &row, `SELECT g.group_id, g.name, g.is_default, g.protected, g.revision,
		(SELECT COUNT(*) FROM watchlist_memberships m WHERE m.group_id = g.group_id) AS item_count,
		g.created_at, g.updated_at FROM watchlist_groups g WHERE g.group_id = ?`, strings.TrimSpace(groupID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Group{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Group{}, err
	}
	return row.group(), nil
}

func (row groupRow) group() domain.Group {
	return domain.Group{ID: row.ID, Name: row.Name, IsDefault: row.IsDefault != 0, Protected: row.Protected != 0,
		Revision: row.Revision, ItemCount: row.ItemCount, CreatedAt: parseTime(row.CreatedAt), UpdatedAt: parseTime(row.UpdatedAt)}
}

func (s *Store) CreateGroup(ctx context.Context, name string) (domain.Group, error) {
	if err := s.available(); err != nil {
		return domain.Group{}, err
	}
	groupID, err := insertGroup(ctx, s.db, name)
	if err != nil {
		return domain.Group{}, err
	}
	return s.GetGroup(ctx, groupID)
}

type groupExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertGroup(ctx context.Context, executor groupExecutor, name string) (string, error) {
	groupID := "wlgrp_" + uuid.NewString()
	now := nowText()
	_, err := executor.ExecContext(ctx, `INSERT INTO watchlist_groups
		(group_id, name, name_key, is_default, protected, revision, created_at, updated_at)
		VALUES (?, ?, ?, 0, 0, 1, ?, ?)`,
		groupID, strings.TrimSpace(name), domain.GroupNameKey(name), now, now)
	if err != nil {
		return "", mapWriteError(err)
	}
	return groupID, nil
}

func (s *Store) UpdateGroup(ctx context.Context, groupID, name string, expectedRevision int64) (domain.Group, error) {
	if err := s.available(); err != nil {
		return domain.Group{}, err
	}
	group, err := s.GetGroup(ctx, groupID)
	if err != nil {
		return domain.Group{}, err
	}
	if group.Protected {
		return domain.Group{}, domain.ErrProtectedGroup
	}
	result, err := s.db.ExecContext(ctx, `UPDATE watchlist_groups SET name = ?, name_key = ?, revision = revision + 1, updated_at = ?
		WHERE group_id = ? AND revision = ?`, strings.TrimSpace(name), domain.GroupNameKey(name), nowText(), groupID, expectedRevision)
	if err != nil {
		return domain.Group{}, mapWriteError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.Group{}, err
	}
	if affected == 0 {
		if _, getErr := s.GetGroup(ctx, groupID); errors.Is(getErr, domain.ErrNotFound) {
			return domain.Group{}, getErr
		}
		return domain.Group{}, domain.ErrConflict
	}
	return s.GetGroup(ctx, groupID)
}

func (s *Store) DeleteGroup(ctx context.Context, groupID string) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.db.WriteTx(ctx, nil, func(tx *sqliteconn.Tx) error {
		group, err := getGroup(ctx, tx, groupID)
		if err != nil {
			return err
		}
		if group.Protected {
			return domain.ErrProtectedGroup
		}
		var instrumentIDs []string
		if err := tx.SelectContext(ctx, &instrumentIDs, `SELECT instrument_id FROM watchlist_memberships WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_membership_origins WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_memberships WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_bindings WHERE local_group_id = ?`, groupID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_groups WHERE group_id = ?`, groupID); err != nil {
			return err
		}
		for _, instrumentID := range instrumentIDs {
			if _, err := tx.ExecContext(ctx, `UPDATE watchlist_instruments SET membership_revision = membership_revision + 1, updated_at = ? WHERE instrument_id = ?`, nowText(), instrumentID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) available() error {
	if s == nil || s.db == nil {
		return domain.ErrUnavailable
	}
	return nil
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

func splitInstrumentID(value string) (string, string) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", value
	}
	return parts[0], parts[1]
}

func jsonText(value any) (string, error) {
	body, err := json.Marshal(value)
	return string(body), err
}

func scanJSON[T any](body string, target *[]T) error {
	if strings.TrimSpace(body) == "" {
		*target = []T{}
		return nil
	}
	return json.Unmarshal([]byte(body), target)
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
