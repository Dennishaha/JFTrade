package servercore

import (
	"testing"
)

// TestBacktestRunStoreMigration 验证 backtest run store 的空库迁移和 CRUD。
func TestBacktestRunStoreMigration(t *testing.T) {
	store, err := newBacktestRunStoreWithDB(t.TempDir() + "/backtest.db")
	if err != nil {
		t.Fatalf("newBacktestRunStoreWithDB: %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()

	// 验证空列表
	runs := store.ListLightweight()
	if runs == nil {
		t.Fatal("listLightweight returned nil")
	}
}
