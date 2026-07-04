package sqliteconn

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
)

var ErrWriteQueryRequiresTransaction = errors.New("sqlite write queries that return rows require a managed write transaction")

// DB separates SQLite reads and writes into independent connection pools.
// Reads wait only for writes that were submitted before the read, then run in
// parallel with other reads and with later WAL writes.
type DB struct {
	reader      *sqlx.DB
	writer      *sqlx.DB
	coordinator *writeCoordinator
	closeOnce   sync.Once
	closeErr    error
}

// Tx is a write transaction that owns its position in the database write queue
// until Commit or Rollback completes.
type Tx struct {
	*sqlx.Tx
	ticket *writeTicket
	once   sync.Once
}

func (db *DB) Stats() sql.DBStats {
	return db.reader.Stats()
}

func (db *DB) WriteStats() sql.DBStats {
	return db.writer.Stats()
}

func (db *DB) Close() error {
	if db == nil {
		return nil
	}
	db.closeOnce.Do(func() {
		db.closeErr = errors.Join(db.reader.Close(), db.writer.Close())
	})
	return db.closeErr
}

func (db *DB) Ping() error {
	return db.PingContext(context.Background())
}

func (db *DB) PingContext(ctx context.Context) error {
	if err := db.waitForReads(ctx); err != nil {
		return err
	}
	return db.reader.PingContext(ctx)
}

func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	return db.ExecContext(context.Background(), query, args...)
}

func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	var result sql.Result
	err := db.runWrite(ctx, func(writer *sqlx.DB) error {
		var execErr error
		result, execErr = writer.ExecContext(ctx, query, args...)
		return execErr
	})
	return result, err
}

func (db *DB) NamedExec(query string, arg any) (sql.Result, error) {
	return db.NamedExecContext(context.Background(), query, arg)
}

func (db *DB) NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error) {
	boundQuery, args, err := sqlx.Named(query, arg)
	if err != nil {
		return nil, err
	}
	return db.ExecContext(ctx, db.writer.Rebind(boundQuery), args...)
}

func (db *DB) Get(dest any, query string, args ...any) error {
	return db.GetContext(context.Background(), dest, query, args...)
}

func (db *DB) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	if err := db.waitForReads(ctx); err != nil {
		return err
	}
	return db.reader.GetContext(ctx, dest, query, args...)
}

func (db *DB) Select(dest any, query string, args ...any) error {
	return db.SelectContext(context.Background(), dest, query, args...)
}

func (db *DB) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	if err := db.waitForReads(ctx); err != nil {
		return err
	}
	return db.reader.SelectContext(ctx, dest, query, args...)
}

func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(context.Background(), query, args...)
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if !isReadStatement(query) {
		return nil, ErrWriteQueryRequiresTransaction
	}
	if err := db.waitForReads(ctx); err != nil {
		return nil, err
	}
	return db.reader.QueryContext(ctx, query, args...)
}

func (db *DB) Queryx(query string, args ...any) (*sqlx.Rows, error) {
	return db.QueryxContext(context.Background(), query, args...)
}

func (db *DB) QueryxContext(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
	if !isReadStatement(query) {
		return nil, ErrWriteQueryRequiresTransaction
	}
	if err := db.waitForReads(ctx); err != nil {
		return nil, err
	}
	return db.reader.QueryxContext(ctx, query, args...)
}

func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.QueryRowContext(context.Background(), query, args...)
}

func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if !isReadStatement(query) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		return db.reader.QueryRowContext(cancelled, `SELECT 1`)
	}
	if err := db.waitForReads(ctx); err != nil {
		return db.reader.QueryRowContext(ctx, query, args...)
	}
	return db.reader.QueryRowContext(ctx, query, args...)
}

func (db *DB) QueryRowx(query string, args ...any) *sqlx.Row {
	return db.QueryRowxContext(context.Background(), query, args...)
}

func (db *DB) QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row {
	if !isReadStatement(query) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		return db.reader.QueryRowxContext(cancelled, `SELECT 1`)
	}
	if err := db.waitForReads(ctx); err != nil {
		return db.reader.QueryRowxContext(ctx, query, args...)
	}
	return db.reader.QueryRowxContext(ctx, query, args...)
}

func (db *DB) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	if !isReadStatement(query) {
		return nil, ErrWriteQueryRequiresTransaction
	}
	if err := db.waitForReads(ctx); err != nil {
		return nil, err
	}
	return db.reader.PrepareContext(ctx, query)
}

func (db *DB) Rebind(query string) string {
	return db.writer.Rebind(query)
}

func (db *DB) DriverName() string {
	return DriverName
}

func (db *DB) BindNamed(query string, arg any) (string, []any, error) {
	boundQuery, args, err := sqlx.Named(query, arg)
	if err != nil {
		return "", nil, err
	}
	return db.Rebind(boundQuery), args, nil
}

func (db *DB) BeginWrite(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	ticket := db.coordinator.enqueueWrite()
	if err := ticket.wait(ctx); err != nil {
		return nil, err
	}
	tx, err := db.writer.BeginTxx(ctx, opts)
	if err != nil {
		ticket.finish()
		return nil, err
	}
	return &Tx{Tx: tx, ticket: ticket}, nil
}

func (db *DB) WriteTx(ctx context.Context, opts *sql.TxOptions, fn func(*Tx) error) error {
	if fn == nil {
		return nil
	}
	tx, err := db.BeginWrite(ctx, opts)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

func (tx *Tx) Commit() error {
	if tx == nil || tx.Tx == nil {
		return sql.ErrTxDone
	}
	err := tx.Tx.Commit()
	tx.release()
	return err
}

func (tx *Tx) Rollback() error {
	if tx == nil || tx.Tx == nil {
		return sql.ErrTxDone
	}
	err := tx.Tx.Rollback()
	tx.release()
	return err
}

func (tx *Tx) release() {
	tx.once.Do(tx.ticket.finish)
}

func (db *DB) runWrite(ctx context.Context, fn func(*sqlx.DB) error) error {
	ticket := db.coordinator.enqueueWrite()
	if err := ticket.wait(ctx); err != nil {
		return err
	}
	defer ticket.finish()
	return fn(db.writer)
}

func (db *DB) waitForReads(ctx context.Context) error {
	return db.coordinator.readBarrier().wait(ctx)
}

func isReadStatement(query string) bool {
	keyword := firstSQLKeyword(query)
	switch keyword {
	case "SELECT", "EXPLAIN":
		return true
	case "PRAGMA":
		return isReadPragma(query)
	default:
		return false
	}
}

func isReadPragma(query string) bool {
	upper := strings.ToUpper(strings.TrimSpace(query))
	_, after, ok := strings.Cut(upper, "PRAGMA")
	if !ok {
		return false
	}
	remainder := strings.TrimSpace(after)
	end := strings.IndexAny(remainder, " \t\r\n(=;")
	if end < 0 {
		end = len(remainder)
	}
	name := strings.TrimSpace(remainder[:end])
	switch name {
	case "TABLE_INFO", "TABLE_XINFO", "INDEX_LIST", "INDEX_INFO", "INDEX_XINFO", "FOREIGN_KEY_LIST":
		return true
	case "DATABASE_LIST", "COMPILE_OPTIONS", "PRAGMA_LIST", "PAGE_SIZE", "FREELIST_COUNT", "JOURNAL_MODE", "BUSY_TIMEOUT", "FOREIGN_KEYS", "USER_VERSION":
		return !strings.ContainsAny(remainder, "=(")
	default:
		return false
	}
}

func firstSQLKeyword(query string) string {
	remaining := strings.TrimSpace(query)
	for remaining != "" {
		switch {
		case strings.HasPrefix(remaining, "--"):
			if newline := strings.IndexByte(remaining, '\n'); newline >= 0 {
				remaining = strings.TrimSpace(remaining[newline+1:])
				continue
			}
			return ""
		case strings.HasPrefix(remaining, "/*"):
			if end := strings.Index(remaining[2:], "*/"); end >= 0 {
				remaining = strings.TrimSpace(remaining[end+4:])
				continue
			}
			return ""
		}
		end := strings.IndexAny(remaining, " \t\r\n(")
		if end < 0 {
			end = len(remaining)
		}
		return strings.ToUpper(remaining[:end])
	}
	return ""
}

func (db *DB) String() string {
	return fmt.Sprintf("sqlite read_pool=%d write_pool=%d", db.Stats().MaxOpenConnections, db.WriteStats().MaxOpenConnections)
}
