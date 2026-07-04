package sqliteconn

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	// Register the modernc SQLite driver used by the separated read/write pools.
	_ "modernc.org/sqlite"
)

const DriverName = "sqlite"

const defaultMaxOpenConns = 8

type Options struct {
	MaxOpenConns int
	MaxIdleConns int
}

type Option func(*Options)

var openSQLX = sqlx.Open

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

func Open(path string, opts ...Option) (*DB, error) {
	return openDatabase(path, false, opts...)
}

func OpenX(path string, opts ...Option) (*DB, error) {
	return openDatabase(path, false, opts...)
}

func OpenReadOnly(path string, opts ...Option) (*DB, error) {
	return openDatabase(path, true, opts...)
}

func openDatabase(path string, readOnly bool, opts ...Option) (*DB, error) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return nil, fmt.Errorf("sqlite database path is required")
	}
	writerDSN := DSN(trimmedPath)
	readerDSN := ReadDSN(trimmedPath)
	if readOnly {
		writerDSN = ReadOnlyDSN(trimmedPath)
		readerDSN = writerDSN
	}

	writer, err := openSQLX(DriverName, writerDSN)
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)

	reader, err := openSQLX(DriverName, readerDSN)
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	options := resolveOptions(opts...)
	reader.SetMaxOpenConns(options.MaxOpenConns)
	reader.SetMaxIdleConns(options.MaxIdleConns)

	return &DB{
		reader:      reader,
		writer:      writer,
		coordinator: coordinatorForPath(trimmedPath),
	}, nil
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

func ReadDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return strings.TrimSpace(path) + separator + "_pragma=query_only(1)&_pragma=busy_timeout(10000)"
}

func ReadOnlyDSN(path string) string {
	trimmedPath := strings.TrimSpace(path)
	if !strings.HasPrefix(strings.ToLower(trimmedPath), "file:") {
		trimmedPath = "file:" + trimmedPath
	}
	separator := "?"
	if strings.Contains(trimmedPath, "?") {
		separator = "&"
	}
	if !strings.Contains(strings.ToLower(trimmedPath), "mode=") {
		trimmedPath += separator + "mode=ro"
		separator = "&"
	}
	return trimmedPath + separator + "_pragma=busy_timeout(10000)"
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
