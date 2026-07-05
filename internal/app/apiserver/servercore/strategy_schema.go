package servercore

import (
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
)

func initializeStrategyDatabase(db sqliteschema.Database, path string) error {
	return runtimeactivity.InitializeDatabase(db, path)
}
