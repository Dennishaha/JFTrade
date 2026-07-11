package watchlist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
	"github.com/jmoiron/sqlx"
)

type instrumentRow struct {
	ID         string  `db:"instrument_id"`
	Market     string  `db:"market"`
	Symbol     string  `db:"symbol"`
	Name       string  `db:"name"`
	Type       string  `db:"instrument_type"`
	Revision   int64   `db:"membership_revision"`
	ImportedAt *string `db:"last_imported_at"`
}

type instrumentGroupRow struct {
	InstrumentID string `db:"instrument_id"`
	GroupID      string `db:"group_id"`
	Name         string `db:"name"`
}

type instrumentSourceRow struct {
	InstrumentID string `db:"instrument_id"`
	SourceID     string `db:"source_id"`
}

func (s *Store) ListItems(ctx context.Context, options domain.ListItemsOptions) (domain.ItemPage, error) {
	if err := s.available(); err != nil {
		return domain.ItemPage{}, err
	}
	query := `SELECT i.instrument_id, i.market, i.symbol, i.name, i.instrument_type, i.membership_revision,
		(SELECT MAX(o.last_imported_at) FROM watchlist_membership_origins o WHERE o.instrument_id = i.instrument_id) AS last_imported_at
		FROM watchlist_instruments i WHERE EXISTS (
			SELECT 1 FROM watchlist_memberships member WHERE member.instrument_id = i.instrument_id`
	args := make([]any, 0, 6)
	if options.GroupID != "" {
		query += ` AND member.group_id = ?`
		args = append(args, options.GroupID)
	}
	query += `)`
	if options.Cursor != "" {
		query += ` AND i.instrument_id > ?`
		args = append(args, options.Cursor)
	}
	if options.Query != "" {
		query += ` AND (UPPER(i.instrument_id) LIKE UPPER(?) OR UPPER(i.name) LIKE UPPER(?))`
		pattern := "%" + options.Query + "%"
		args = append(args, pattern, pattern)
	}
	if options.Market != "" {
		if options.Market == "CN" {
			query += ` AND i.market IN ('SH', 'SZ')`
		} else {
			query += ` AND i.market = ?`
			args = append(args, options.Market)
		}
	}
	query += ` ORDER BY i.instrument_id LIMIT ?`
	args = append(args, options.Limit+1)
	rows := []instrumentRow{}
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return domain.ItemPage{}, err
	}
	page := domain.ItemPage{Items: []domain.Item{}}
	if len(rows) > options.Limit {
		page.NextCursor = rows[options.Limit-1].ID
		rows = rows[:options.Limit]
	}
	items, err := s.hydrateItems(ctx, rows)
	if err != nil {
		return domain.ItemPage{}, err
	}
	page.Items = items
	return page, nil
}

func (s *Store) hydrateItems(ctx context.Context, rows []instrumentRow) ([]domain.Item, error) {
	if len(rows) == 0 {
		return []domain.Item{}, nil
	}
	instrumentIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		instrumentIDs = append(instrumentIDs, row.ID)
	}

	groupQuery, groupArgs, err := sqlx.In(`SELECT m.instrument_id, g.group_id, g.name
		FROM watchlist_memberships m JOIN watchlist_groups g ON g.group_id = m.group_id
		WHERE m.instrument_id IN (?)
		ORDER BY m.instrument_id, g.is_default DESC, g.created_at, g.group_id`, instrumentIDs)
	if err != nil {
		return nil, err
	}
	groupRows := []instrumentGroupRow{}
	if err := s.db.SelectContext(ctx, &groupRows, groupQuery, groupArgs...); err != nil {
		return nil, err
	}
	groupsByInstrument := make(map[string][]domain.GroupRef, len(rows))
	for _, row := range groupRows {
		groupsByInstrument[row.InstrumentID] = append(
			groupsByInstrument[row.InstrumentID],
			domain.GroupRef{ID: row.GroupID, Name: row.Name},
		)
	}

	sourceQuery, sourceArgs, err := sqlx.In(`SELECT DISTINCT instrument_id, source_id
		FROM watchlist_membership_origins
		WHERE instrument_id IN (?)
		ORDER BY instrument_id, source_id`, instrumentIDs)
	if err != nil {
		return nil, err
	}
	sourceRows := []instrumentSourceRow{}
	if err := s.db.SelectContext(ctx, &sourceRows, sourceQuery, sourceArgs...); err != nil {
		return nil, err
	}
	sourcesByInstrument := make(map[string][]string, len(rows))
	for _, row := range sourceRows {
		sourcesByInstrument[row.InstrumentID] = append(
			sourcesByInstrument[row.InstrumentID],
			row.SourceID,
		)
	}

	items := make([]domain.Item, 0, len(rows))
	for _, row := range rows {
		groups := groupsByInstrument[row.ID]
		if groups == nil {
			groups = []domain.GroupRef{}
		}
		sources := sourcesByInstrument[row.ID]
		if sources == nil {
			sources = []string{}
		}
		instrument := domain.Instrument{
			ID: row.ID, Market: row.Market, Symbol: row.Symbol, Name: row.Name, Type: row.Type, Revision: row.Revision,
			SourceIDs: sources, GroupIDs: make([]string, 0, len(groups)),
		}
		for _, group := range groups {
			instrument.GroupIDs = append(instrument.GroupIDs, group.ID)
		}
		if row.ImportedAt != nil {
			parsed := parseTime(*row.ImportedAt)
			instrument.LastImportedAt = &parsed
		}
		items = append(items, domain.Item{Instrument: instrument, Groups: groups})
	}
	return items, nil
}

func listGroupRefs(ctx context.Context, query interface {
	SelectContext(context.Context, any, string, ...any) error
}, instrumentID string) ([]domain.GroupRef, error) {
	groups := []domain.GroupRef{}
	err := query.SelectContext(ctx, &groups, `SELECT g.group_id, g.name FROM watchlist_groups g
		JOIN watchlist_memberships m ON m.group_id = g.group_id
		WHERE m.instrument_id = ? ORDER BY g.is_default DESC, g.created_at, g.group_id`, instrumentID)
	return groups, err
}

func (s *Store) GetMemberships(ctx context.Context, instrumentID string) (domain.Memberships, error) {
	if err := s.available(); err != nil {
		return domain.Memberships{}, err
	}
	var revision int64
	err := s.db.GetContext(ctx, &revision, `SELECT membership_revision FROM watchlist_instruments WHERE instrument_id = ?`, instrumentID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Memberships{InstrumentID: instrumentID, Revision: 0, Groups: []domain.GroupRef{}}, nil
	}
	if err != nil {
		return domain.Memberships{}, err
	}
	groups, err := listGroupRefs(ctx, s.db, instrumentID)
	if err != nil {
		return domain.Memberships{}, err
	}
	return domain.Memberships{InstrumentID: instrumentID, Revision: revision, Groups: groups}, nil
}

func (s *Store) ReplaceMemberships(ctx context.Context, input domain.ReplaceMembershipsInput) (domain.Memberships, error) {
	if err := s.available(); err != nil {
		return domain.Memberships{}, err
	}
	err := s.db.WriteTx(ctx, nil, func(tx *sqliteconn.Tx) error { return replaceMembershipsTx(ctx, tx, input) })
	if err != nil {
		return domain.Memberships{}, err
	}
	return s.GetMemberships(ctx, input.InstrumentID)
}

func replaceMembershipsTx(ctx context.Context, tx *sqliteconn.Tx, input domain.ReplaceMembershipsInput) error {
	if err := ensureMembershipInstrument(ctx, tx, input.InstrumentID, input.ExpectedRevision); err != nil {
		return err
	}
	desired, createdGroups, err := resolveMembershipGroups(ctx, tx, input)
	if err != nil {
		return err
	}
	current, err := membershipGroupSet(ctx, tx, input.InstrumentID)
	if err != nil {
		return err
	}
	return applyMembershipDiff(ctx, tx, input.InstrumentID, desired, current, createdGroups)
}

func ensureMembershipInstrument(ctx context.Context, tx *sqliteconn.Tx, instrumentID string, expectedRevision int64) error {
	var currentRevision int64
	err := tx.GetContext(ctx, &currentRevision, `SELECT membership_revision FROM watchlist_instruments WHERE instrument_id = ?`, instrumentID)
	if err == nil {
		if currentRevision != expectedRevision {
			return domain.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if expectedRevision != 0 {
		return domain.ErrConflict
	}
	marketCode, symbol := splitInstrumentID(instrumentID)
	now := nowText()
	_, err = tx.ExecContext(ctx, `INSERT INTO watchlist_instruments
		(instrument_id, market, symbol, name, instrument_type, membership_revision, created_at, updated_at)
		VALUES (?, ?, ?, '', '', 0, ?, ?)`, instrumentID, marketCode, symbol, now, now)
	return mapWriteError(err)
}

func resolveMembershipGroups(ctx context.Context, tx *sqliteconn.Tx, input domain.ReplaceMembershipsInput) (map[string]struct{}, map[string]struct{}, error) {
	desired := make(map[string]struct{}, len(input.GroupIDs)+len(input.NewGroupNames))
	for _, groupID := range input.GroupIDs {
		var exists int
		if err := tx.GetContext(ctx, &exists, `SELECT 1 FROM watchlist_groups WHERE group_id = ?`, groupID); errors.Is(err, sql.ErrNoRows) {
			return nil, nil, domain.ErrNotFound
		} else if err != nil {
			return nil, nil, err
		}
		desired[groupID] = struct{}{}
	}
	created := make(map[string]struct{}, len(input.NewGroupNames))
	for _, name := range input.NewGroupNames {
		groupID, err := insertGroup(ctx, tx, name)
		if err != nil {
			return nil, nil, err
		}
		desired[groupID], created[groupID] = struct{}{}, struct{}{}
	}
	return desired, created, nil
}

func membershipGroupSet(ctx context.Context, tx *sqliteconn.Tx, instrumentID string) (map[string]struct{}, error) {
	groupIDs := []string{}
	if err := tx.SelectContext(ctx, &groupIDs, `SELECT group_id FROM watchlist_memberships WHERE instrument_id = ?`, instrumentID); err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(groupIDs))
	for _, groupID := range groupIDs {
		result[groupID] = struct{}{}
	}
	return result, nil
}

func applyMembershipDiff(ctx context.Context, tx *sqliteconn.Tx, instrumentID string, desired, current, created map[string]struct{}) error {
	added, removed := setDifference(desired, current), setDifference(current, desired)
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	for _, groupID := range removed {
		if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_memberships WHERE group_id = ? AND instrument_id = ?`, groupID, instrumentID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_membership_origins WHERE group_id = ? AND instrument_id = ?`, groupID, instrumentID); err != nil {
			return err
		}
	}
	for _, groupID := range added {
		if _, err := tx.ExecContext(ctx, `INSERT INTO watchlist_memberships (group_id, instrument_id, created_at) VALUES (?, ?, ?)`, groupID, instrumentID, nowText()); err != nil {
			return mapWriteError(err)
		}
	}
	now := nowText()
	if _, err := tx.ExecContext(ctx, `UPDATE watchlist_instruments SET membership_revision = membership_revision + 1, updated_at = ? WHERE instrument_id = ?`, now, instrumentID); err != nil {
		return err
	}
	affected := make(map[string]struct{}, len(added)+len(removed))
	for _, groupID := range append(added, removed...) {
		affected[groupID] = struct{}{}
	}
	for groupID := range affected {
		if _, justCreated := created[groupID]; justCreated {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE watchlist_groups SET revision = revision + 1, updated_at = ? WHERE group_id = ?`, now, groupID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GroupInstrumentIDs(ctx context.Context, groupID string) ([]string, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if _, err := s.GetGroup(ctx, groupID); err != nil {
		return nil, err
	}
	values := []string{}
	err := s.db.SelectContext(ctx, &values, `SELECT instrument_id FROM watchlist_memberships WHERE group_id = ? ORDER BY instrument_id`, groupID)
	return values, err
}

// UpdateInstrumentMetadata enriches existing watchlist rows without creating
// phantom instruments or changing membership/group revisions.
func (s *Store) UpdateInstrumentMetadata(ctx context.Context, metadata []domain.InstrumentMetadata) error {
	if err := s.available(); err != nil {
		return err
	}
	return s.db.WriteTx(ctx, nil, func(tx *sqliteconn.Tx) error {
		for _, item := range metadata {
			instrumentID := strings.TrimSpace(item.InstrumentID)
			name := strings.TrimSpace(item.Name)
			instrumentType := strings.TrimSpace(item.Type)
			if instrumentID == "" || (name == "" && instrumentType == "") {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE watchlist_instruments SET
				name = CASE WHEN ? <> '' THEN ? ELSE name END,
				instrument_type = CASE WHEN ? <> '' THEN ? ELSE instrument_type END,
				updated_at = ? WHERE instrument_id = ? AND (
					(? <> '' AND name <> ?) OR (? <> '' AND instrument_type <> ?)
				)`,
				name, name, instrumentType, instrumentType, nowText(), instrumentID,
				name, name, instrumentType, instrumentType); err != nil {
				return err
			}
		}
		return nil
	})
}

func setDifference(left, right map[string]struct{}) []string {
	result := make(map[string]struct{})
	for value := range left {
		if _, ok := right[value]; !ok {
			result[value] = struct{}{}
		}
	}
	return sortedKeys(result)
}

func ensureInstrument(ctx context.Context, tx *sqliteconn.Tx, member domain.RemoteMember) error {
	marketCode, symbol := splitInstrumentID(member.InstrumentID)
	now := nowText()
	_, err := tx.ExecContext(ctx, `INSERT INTO watchlist_instruments
		(instrument_id, market, symbol, name, instrument_type, membership_revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(instrument_id) DO UPDATE SET
			name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE watchlist_instruments.name END,
			instrument_type = CASE WHEN excluded.instrument_type <> '' THEN excluded.instrument_type ELSE watchlist_instruments.instrument_type END,
			updated_at = excluded.updated_at`, member.InstrumentID, marketCode, symbol, member.Name, member.Type, now, now)
	return err
}

func insertAlias(ctx context.Context, tx *sqliteconn.Tx, sourceID, kind, value, instrumentID string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO watchlist_instrument_aliases
		(source_id, alias_kind, alias_value, instrument_id, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(source_id, alias_kind, alias_value) DO UPDATE SET instrument_id = excluded.instrument_id, updated_at = excluded.updated_at`,
		sourceID, kind, value, instrumentID, nowText())
	if err != nil {
		return fmt.Errorf("upsert watchlist instrument alias: %w", err)
	}
	return nil
}
