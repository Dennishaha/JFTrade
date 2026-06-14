package storage

import (
	"github.com/jmoiron/sqlx"

	"github.com/c9s/bbgo/pkg/types"
)

// InsertKLine inserts a single K-line into the store. Duplicates (same
// end_time in the same series table) are replaced.
func (s *FutuKLineStore) InsertKLine(kline types.KLine, rehabType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.insertKLineLocked(kline, rehabType, nil)
}

func (s *FutuKLineStore) insertKLineLocked(kline types.KLine, rehabType string, ensuredTables map[string]struct{}) error {
	tableName := s.writeTableName(kline.Symbol, kline.Interval, rehabType)
	if ensuredTables != nil {
		if _, ok := ensuredTables[tableName]; !ok {
			if err := s.ensureKLineTable(tableName); err != nil {
				return err
			}
			ensuredTables[tableName] = struct{}{}
		}
	} else {
		if err := s.ensureKLineTable(tableName); err != nil {
			return err
		}
	}

	_, err := s.db.Exec(
		klineInsertStatement(tableName),
		timeToUnixMillis(kline.EndTime.Time()),
		timeToUnixMillis(kline.StartTime.Time()),
		kline.Open.String(),
		kline.High.String(),
		kline.Low.String(),
		kline.Close.String(),
		kline.Volume.String(),
	)
	return err
}

func klineInsertStatement(tableName string) string {
	return `INSERT INTO ` + quoteIdentifier(tableName) + ` (end_time, start_time, open, high, low, close, volume) VALUES (?, ?, ?, ?, ?, ?, ?) ` +
		`ON CONFLICT(end_time) DO UPDATE SET ` +
		`start_time = excluded.start_time, open = excluded.open, high = excluded.high, low = excluded.low, close = excluded.close, volume = excluded.volume`
}

// InsertKLines batch-inserts K-lines into the store.
func (s *FutuKLineStore) InsertKLines(klines []types.KLine, rehabType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(klines) == 0 {
		return nil
	}
	if len(klines) == 1 {
		return s.insertKLineLocked(klines[0], rehabType, nil)
	}

	tableNames := make([]string, len(klines))
	ensuredTables := make(map[string]struct{})
	for index, k := range klines {
		tableName := s.writeTableName(k.Symbol, k.Interval, rehabType)
		tableNames[index] = tableName
		if _, ok := ensuredTables[tableName]; ok {
			continue
		}
		if err := s.ensureKLineTable(tableName); err != nil {
			return err
		}
		ensuredTables[tableName] = struct{}{}
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	stmts := make(map[string]*sqlx.Stmt, len(ensuredTables))
	defer func() {
		for _, stmt := range stmts {
			_ = stmt.Close()
		}
	}()

	for index, k := range klines {
		tableName := tableNames[index]
		stmt, ok := stmts[tableName]
		if !ok {
			stmt, err = tx.Preparex(klineInsertStatement(tableName))
			if err != nil {
				_ = tx.Rollback()
				return err
			}
			stmts[tableName] = stmt
		}

		if _, err := stmt.Exec(
			timeToUnixMillis(k.EndTime.Time()),
			timeToUnixMillis(k.StartTime.Time()),
			k.Open.String(),
			k.High.String(),
			k.Low.String(),
			k.Close.String(),
			k.Volume.String(),
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return nil
}
