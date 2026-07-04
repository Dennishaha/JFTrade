package servercore

import (
	"sync"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

const defaultExecutionPersistenceQueueSize = 1024

type brokerOrderCommandResponse = trdsrv.ExecutionCommandResponse

//nolint:unused // Referenced by Swagger annotations during go generate.
type executionPlaceOrderRequest = trdsrv.ExecutionPlaceRequest
type executionOrderSummaryResponse = trdsrv.ExecutionOrder
type executionOrderEventResponse = trdsrv.ExecutionOrderEvent
type executionOrdersResponse = trdsrv.ExecutionOrders
type executionOrderListFilter = trdsrv.ExecutionOrderFilter
type executionOrderEventsResponse = trdsrv.ExecutionOrderEvents

//nolint:unused // Referenced by Swagger annotations during go generate.
type executionOrderDetailsResponse = trdsrv.ExecutionOrderDetails
type executionPlacedOrderRecord = trdsrv.ExecutionPlacedOrderRecord

type executionOrderStore struct {
	mu                    sync.RWMutex
	persistenceMu         sync.Mutex
	persistence           *executionOrderSQLiteStore
	persistenceQueue      chan executionPersistenceItem
	persistenceWG         sync.WaitGroup
	persistenceClosed     bool
	seenFillRetentionDays int
	nextOrderSeq          uint64
	nextEventSeq          uint64
	orders                map[string]executionOrderSummaryResponse
	events                map[string][]executionOrderEventResponse
	brokerOrderIndex      map[string]string
	brokerOrderExIndex    map[string]string
	seenFillKeys          map[string]string
}

type executionPersistenceItem struct {
	kind      string
	order     executionOrderSummaryResponse
	event     executionOrderEventResponse
	fillKey   string
	createdAt string
	seqName   string
	seqValue  uint64
	cutoff    string
}

func newExecutionOrderStore() *executionOrderStore {
	return &executionOrderStore{
		orders:                make(map[string]executionOrderSummaryResponse),
		events:                make(map[string][]executionOrderEventResponse),
		brokerOrderIndex:      make(map[string]string),
		brokerOrderExIndex:    make(map[string]string),
		seenFillKeys:          make(map[string]string),
		seenFillRetentionDays: 90,
	}
}
