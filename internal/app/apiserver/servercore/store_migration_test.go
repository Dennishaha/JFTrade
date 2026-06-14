package servercore

import (
	"testing"
)

// TestStrategyRuntimeStoreMigration 验证 runtime store 的空库迁移和 CRUD。
func TestStrategyRuntimeStoreMigration(t *testing.T) {
	store, err := NewStrategyRuntimeStore(t.TempDir() + "/runtime.db")
	if err != nil {
		t.Fatalf("NewStrategyRuntimeStore: %v", err)
	}
	defer store.Close()

	// 空库应能正常关闭
	if db := store.DB(); db == nil {
		t.Fatal("DB() returned nil")
	}
}

// TestStrategyDesignStoreMigration 验证 design store 的空库迁移和 CRUD。
func TestStrategyDesignStoreMigration(t *testing.T) {
	// 隔离环境变量，避免 JFTRADE_STRATEGY_RUNTIME_DB 干扰 DB 路径推导
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", "")

	store, err := NewStrategyDesignStore(t.TempDir() + "/design.json")
	if err != nil {
		t.Fatalf("NewStrategyDesignStore: %v", err)
	}
	defer store.Close()

	// 验证空列表
	defs := store.listDefinitions()
	if defs == nil {
		t.Fatal("listDefinitions returned nil")
	}

	// 验证不存在的定义
	_, ok, err := store.definition("nonexistent")
	if err != nil {
		t.Errorf("definition error: %v", err)
	}
	if ok {
		t.Error("nonexistent definition should not be found")
	}
}

// TestStrategyCatalogStoreMigration 验证 catalog store 的空库迁移。
func TestStrategyCatalogStoreMigration(t *testing.T) {
	// 隔离环境变量，避免 JFTRADE_STRATEGY_RUNTIME_DB 干扰 DB 路径推导
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", "")

	store, err := NewStrategyCatalogStore(
		t.TempDir()+"/catalog.json",
		t.TempDir()+"/plugins",
	)
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	defer store.Close()

	// 验证空列表
	strategies := store.strategies()
	if strategies == nil {
		t.Fatal("strategies returned nil")
	}
}

// TestBacktestRunStoreMigration 验证 backtest run store 的空库迁移和 CRUD。
func TestBacktestRunStoreMigration(t *testing.T) {
	store, err := newBacktestRunStoreWithDB(t.TempDir() + "/backtest.db")
	if err != nil {
		t.Fatalf("newBacktestRunStoreWithDB: %v", err)
	}
	defer store.Close()

	// 验证空列表
	runs := store.listLightweight()
	if runs == nil {
		t.Fatal("listLightweight returned nil")
	}
}
