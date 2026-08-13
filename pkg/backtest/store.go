package backtest

import (
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	internalstorage "github.com/jftrade/jftrade-main/pkg/backtest/internal/storage"
)

const (
	KLineTable                = internalstorage.KLineTable
	KLineSessionScopeRegular  = internalstorage.KLineSessionScopeRegular
	KLineSessionScopeExtended = internalstorage.KLineSessionScopeExtended
	rehabTypeForwardCode      = internalstorage.RehabTypeForwardCode
)

type KLineStore = internalstorage.KLineStore
type FutuKLineStore = KLineStore

func NewKLineStore(dbPath, providerID string) (*KLineStore, error) {
	return internalstorage.NewKLineStore(dbPath, providerID)
}

func NewFutuKLineStore(dbPath string) (*FutuKLineStore, error) {
	return internalstorage.NewFutuKLineStore(dbPath)
}

func RehabTypeName(rehabType int32) string {
	return internalstorage.RehabTypeName(rehabType)
}

func expectedKLineSchemaColumns() []string {
	return internalstorage.ExpectedKLineSchemaColumns()
}

func intervalStorageValue(interval types.Interval) int64 {
	return internalstorage.IntervalStorageValue(interval)
}

func intervalFromStorageValue(value int64) (types.Interval, error) {
	return internalstorage.IntervalFromStorageValue(value)
}

func KLineTableName(symbol string, interval types.Interval, rehabType string) string {
	return internalstorage.KLineTableName(symbol, interval, rehabType)
}

func KLineTableNameForSessionScope(symbol string, interval types.Interval, rehabType string, sessionScope string) string {
	return internalstorage.KLineTableNameForSessionScope(symbol, interval, rehabType, sessionScope)
}

func KLineTableNameForProviderAndSessionScope(providerID, symbol string, interval types.Interval, rehabType string, sessionScope string) string {
	return internalstorage.KLineTableNameForProviderAndSessionScope(providerID, symbol, interval, rehabType, sessionScope)
}

func NormalizeKLineSessionScopeName(scope string) string {
	return internalstorage.NormalizeKLineSessionScopeName(scope)
}
