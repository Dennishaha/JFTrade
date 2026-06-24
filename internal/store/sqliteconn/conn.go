package sqliteconn

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	// Register the modernc SQLite driver for database/sql.
	_ "modernc.org/sqlite"
)

const DriverName = "sqlite"

const defaultMaxOpenConns = 1

type Options struct {
	MaxOpenConns int
	MaxIdleConns int
}

type Option func(*Options)

func WithMaxOpenConns(value int) Option {
	return func(options *Options) {
		options.MaxOpenConns = value
	}
}

func WithMaxIdleConns(value int) Option {
	return func(options *Options) {
		options.MaxIdleConns = value
	}
}

func Open(path string, opts ...Option) (*sql.DB, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return nil, fmt.Errorf("sqlite database path is required")
	}
	db, err := sql.Open(DriverName, DSN(trimmedPath))
	if err != nil {
		return nil, err
	}
	configure(db, resolveOptions(opts...))
	return db, nil
}

func OpenX(path string, opts ...Option) (*sqlx.DB, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return nil, fmt.Errorf("sqlite database path is required")
	}
	db, err := sqlx.Open(DriverName, DSN(trimmedPath))
	if err != nil {
		return nil, err
	}
	configure(db.DB, resolveOptions(opts...))
	return db, nil
}

func DSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return strings.TrimSpace(path) +
		separator +
		"_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)"
}

func resolveOptions(opts ...Option) Options {
	options := Options{MaxOpenConns: defaultMaxOpenConns, MaxIdleConns: defaultMaxOpenConns}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.MaxOpenConns <= 0 {
		options.MaxOpenConns = defaultMaxOpenConns
	}
	if options.MaxIdleConns <= 0 || options.MaxIdleConns > options.MaxOpenConns {
		options.MaxIdleConns = options.MaxOpenConns
	}
	return options
}

func configure(db *sql.DB, options Options) {
	if db == nil {
		return
	}
	db.SetMaxOpenConns(options.MaxOpenConns)
	db.SetMaxIdleConns(options.MaxIdleConns)
}
