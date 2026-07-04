package adk

import (
	"context"
	"database/sql"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"gorm.io/gorm"
)

type sqliteGormPool struct {
	db *sqliteconn.DB
}

type sqliteGormTx struct {
	*sqliteconn.Tx
}

func newSQLiteGormPool(db *sqliteconn.DB) *sqliteGormPool {
	return &sqliteGormPool{db: db}
}

func (p *sqliteGormPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return p.db.PrepareContext(ctx, query)
}

func (p *sqliteGormPool) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
}

func (p *sqliteGormPool) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}

func (p *sqliteGormPool) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return p.db.QueryRowContext(ctx, query, args...)
}

func (p *sqliteGormPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.db.BeginWrite(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &sqliteGormTx{Tx: tx}, nil
}
