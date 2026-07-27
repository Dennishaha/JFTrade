package trading

import (
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// Resource composes the consumer-owned execution ports with the minimal
// availability, maintenance and lifecycle surface required by application
// bootstrap. It intentionally exposes no SQLite or in-memory map details.
type Resource interface {
	trdsrv.OrderStore
	trdsrv.ExecutionPreviewStore
	trdsrv.ExecutionSubmissionStore
	trdsrv.ExecutionReconciliationStore
	trdsrv.ExecutionLedgerView
	trdsrv.ExecutionRetentionStore
	broker.PredictionQuoteStore
	dmsrv.BusyChecker
	dmsrv.Compactor
	Available() bool
	Close() error
}

var (
	_ trdsrv.OrderStore                   = (*Store)(nil)
	_ trdsrv.ExecutionPreviewStore        = (*Store)(nil)
	_ trdsrv.ExecutionSubmissionStore     = (*Store)(nil)
	_ trdsrv.ExecutionReconciliationStore = (*Store)(nil)
	_ trdsrv.ExecutionLedgerView          = (*Store)(nil)
	_ trdsrv.ExecutionRetentionStore      = (*Store)(nil)
	_ broker.PredictionQuoteStore         = (*Store)(nil)
	_ dmsrv.BusyChecker                   = (*Store)(nil)
	_ dmsrv.Compactor                     = (*Store)(nil)
	_ Resource                            = (*Store)(nil)
)
