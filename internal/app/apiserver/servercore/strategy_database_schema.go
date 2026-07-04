package servercore

import (
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	"github.com/jmoiron/sqlx"
)

func initializeStrategyDatabase(db *sqlx.DB, path string) error {
	return runtimeactivity.InitializeDatabase(db, path)
}
