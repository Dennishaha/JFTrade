package backtest

import (
	"context"
	"strings"

	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
)

// KLineDatabase is the maintenance surface needed by application assembly.
type KLineDatabase interface {
	CompactDatabase(context.Context) error
	Close() error
}

// OpenKLineDatabase hides the reusable pkg/backtest SQLite implementation
// behind the storage boundary used by startup probes and data maintenance.
func OpenKLineDatabase(path string) (KLineDatabase, error) {
	return bt.NewFutuKLineStore(path)
}

// KLineMaintenance owns open/compact/close for the separately managed market
// history database. The application registry only supplies the configured
// path and never receives the concrete pkg/backtest store.
type KLineMaintenance struct {
	path string
}

// NewKLineMaintenance returns only the compaction port consumed by the
// application maintenance registry.
func NewKLineMaintenance(path string) dmsrv.Compactor {
	return &KLineMaintenance{path: strings.TrimSpace(path)}
}

var _ dmsrv.Compactor = (*KLineMaintenance)(nil)

func (m *KLineMaintenance) CompactMaintenanceResource(ctx context.Context) error {
	path := ""
	if m != nil {
		path = m.path
	}
	store, err := OpenKLineDatabase(path)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return store.CompactDatabase(ctx)
}
