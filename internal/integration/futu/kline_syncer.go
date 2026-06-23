package futu

import (
	"context"

	backtestservice "github.com/jftrade/jftrade-main/internal/backtest"
	backteststore "github.com/jftrade/jftrade-main/pkg/backtest"
	futuexchange "github.com/jftrade/jftrade-main/pkg/futu"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

var (
	newFutuKLineStore = backteststore.NewFutuKLineStore
	newFutuExchange   = futuexchange.NewExchange
)

type kLineSyncer struct {
	exchange *futuexchange.Exchange
	store    *backteststore.FutuKLineStore
	syncFn   func(context.Context, backtestservice.KLineSyncParams, *backteststore.SyncProgress) error
	closeFn  func() error
}

var _ backtestservice.KLineSyncer = (*kLineSyncer)(nil)

// NewKLineSyncer creates the Futu-backed K-line synchronization adapter.
func NewKLineSyncer(dbPath string) (backtestservice.KLineSyncer, error) {
	store, err := newFutuKLineStore(dbPath)
	if err != nil {
		return nil, err
	}
	exchange := newFutuExchange(futuexchange.DefaultOpenDAddr)
	syncer := &kLineSyncer{
		exchange: exchange,
		store:    store,
	}
	syncer.syncFn = func(
		ctx context.Context,
		params backtestservice.KLineSyncParams,
		progress *backteststore.SyncProgress,
	) error {
		return store.SyncKLines(
			ctx,
			exchange,
			params.Symbol,
			params.Intervals,
			params.Since,
			params.Until,
			toFutuRehabType(params.RehabType),
			params.SessionScope,
			progress,
		)
	}
	syncer.closeFn = store.Close
	return syncer, nil
}

func (s *kLineSyncer) Sync(
	ctx context.Context,
	params backtestservice.KLineSyncParams,
	progress *backteststore.SyncProgress,
) error {
	return s.syncFn(ctx, params, progress)
}

func (s *kLineSyncer) Close() error {
	return s.closeFn()
}

func toFutuRehabType(rehabType backtestservice.RehabType) qotcommonpb.RehabType {
	switch rehabType {
	case backtestservice.RehabTypeNone:
		return qotcommonpb.RehabType_RehabType_None
	case backtestservice.RehabTypeBackward:
		return qotcommonpb.RehabType_RehabType_Backward
	default:
		return qotcommonpb.RehabType_RehabType_Forward
	}
}
