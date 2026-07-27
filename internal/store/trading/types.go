package trading

import (
	"sync"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

const defaultExecutionPersistenceQueueSize = 1024

type Store struct {
	mu                    sync.RWMutex
	submissionMu          sync.Mutex
	persistenceMu         sync.Mutex
	databaseMu            sync.Mutex
	persistence           *sqliteStore
	persistenceQueue      chan executionPersistenceItem
	persistenceWG         sync.WaitGroup
	persistenceClosed     bool
	seenFillRetentionDays int
	nextOrderSeq          uint64
	nextEventSeq          uint64
	orders                map[string]trdsrv.ExecutionOrder
	events                map[string][]trdsrv.ExecutionOrderEvent
	brokerOrderIndex      map[string]string
	brokerOrderExIndex    map[string]string
	seenFillKeys          map[string]string
}

type executionPersistenceItem struct {
	kind      string
	order     trdsrv.ExecutionOrder
	event     trdsrv.ExecutionOrderEvent
	fillKey   string
	createdAt string
	seqName   string
	seqValue  uint64
	cutoff    string
}

func newExecutionOrderStore() *Store {
	return &Store{
		orders:                make(map[string]trdsrv.ExecutionOrder),
		events:                make(map[string][]trdsrv.ExecutionOrderEvent),
		brokerOrderIndex:      make(map[string]string),
		brokerOrderExIndex:    make(map[string]string),
		seenFillKeys:          make(map[string]string),
		seenFillRetentionDays: 90,
	}
}
