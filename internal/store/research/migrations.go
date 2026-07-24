package research

import (
	"context"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

const (
	ComponentID   = sqliteschema.DatabaseResearch
	SchemaVersion = sqliteschema.ResearchVersion
)

func initializeSchema(ctx context.Context, db *sqliteconn.DB, path string) error {
	return sqliteschema.InitializeCurrent(ctx, db, path, ComponentID)
}

func nowText() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
