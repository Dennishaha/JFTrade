package servercore

import backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"

const recoveredBacktestRunErrorText = backteststore.RecoveredRunErrorText

// Assembly-local aliases keep bootstrap and focused server tests readable while
// persistence, synchronization state, and locking live in internal/store/backtest.
type backtestRunStore = backteststore.Resource
type backtestSyncTaskStore = backteststore.SyncTaskResource

func newBacktestRunStore() backtestRunStore {
	return backteststore.NewInMemory()
}

func newBacktestRunStoreWithDB(dbPath string) (backtestRunStore, error) {
	return backteststore.New(dbPath)
}

func newBacktestSyncTaskStore() backtestSyncTaskStore {
	return backteststore.NewSyncTaskStore()
}
