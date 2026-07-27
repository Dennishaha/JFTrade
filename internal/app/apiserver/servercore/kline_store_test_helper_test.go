package servercore

import (
	"testing"

	storebacktest "github.com/jftrade/jftrade-main/internal/store/backtest"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type serverKLineSeedStore interface {
	InsertKLines([]bbgotypes.KLine, string) error
	Close() error
}

func openServerKLineSeedStore(t *testing.T, path string) serverKLineSeedStore {
	t.Helper()
	database, err := storebacktest.OpenKLineDatabase(path)
	if err != nil {
		t.Fatalf("open K-line database: %v", err)
	}
	store, ok := database.(serverKLineSeedStore)
	if !ok {
		_ = database.Close()
		t.Fatalf("K-line database %T does not support test seeding", database)
	}
	return store
}
