package persistence

import (
	"context"
	"database/sql"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"gorm.io/gorm"
)

type SQLiteGormPool struct {
	db *sqliteconn.DB
}

type SQLiteGormTx struct {
	*sqliteconn.Tx
}

func NewSQLiteGormPool(db *sqliteconn.DB) *SQLiteGormPool {
	return &SQLiteGormPool{db: db}
}

func (p *SQLiteGormPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return p.db.PrepareContext(ctx, query)
}

func (p *SQLiteGormPool) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
}

func (p *SQLiteGormPool) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}

func (p *SQLiteGormPool) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.db.QueryRowContext(ctx, query, args...)
}

func (p *SQLiteGormPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.db.BeginWrite(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &SQLiteGormTx{Tx: tx}, nil
}
