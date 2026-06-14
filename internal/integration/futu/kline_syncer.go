package futu

import (
	"context"

	backtestservice "github.com/jftrade/jftrade-main/internal/backtest"
	backteststore "github.com/jftrade/jftrade-main/pkg/backtest"
	futuexchange "github.com/jftrade/jftrade-main/pkg/futu"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

type kLineSyncer struct {
	exchange *futuexchange.Exchange
	store    *backteststore.FutuKLineStore
}

var _ backtestservice.KLineSyncer = (*kLineSyncer)(nil)

// NewKLineSyncer creates the Futu-backed K-line synchronization adapter.
func NewKLineSyncer(dbPath string) (backtestservice.KLineSyncer, error) {
	store, err := backteststore.NewFutuKLineStore(dbPath)
	if err != nil {
		return nil, err
	}
	return &kLineSyncer{
		exchange: futuexchange.NewExchange(futuexchange.DefaultOpenDAddr),
		store:    store,
	}, nil
}

func (s *kLineSyncer) Sync(
	ctx context.Context,
	params backtestservice.KLineSyncParams,
	progress *backteststore.SyncProgress,
) error {
	return s.store.SyncKLines(
		ctx,
		s.exchange,
		params.Symbol,
		params.Intervals,
		params.Since,
		params.Until,
		toFutuRehabType(params.RehabType),
		params.SessionScope,
		progress,
	)
}

func (s *kLineSyncer) Close() error {
	return s.store.Close()
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
