package futu

import (
	"context"
	"errors"
	"testing"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
	backtestservice "github.com/jftrade/jftrade-main/internal/backtest"
	backteststore "github.com/jftrade/jftrade-main/pkg/backtest"
	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestToFutuRehabType(t *testing.T) {
	tests := []struct {
		name string
		in   backtestservice.RehabType
		want qotcommonpb.RehabType
	}{
		{name: "forward", in: backtestservice.RehabTypeForward, want: qotcommonpb.RehabType_RehabType_Forward},
		{name: "backward", in: backtestservice.RehabTypeBackward, want: qotcommonpb.RehabType_RehabType_Backward},
		{name: "none", in: backtestservice.RehabTypeNone, want: qotcommonpb.RehabType_RehabType_None},
		{name: "unknown defaults forward", in: backtestservice.RehabType("unknown"), want: qotcommonpb.RehabType_RehabType_Forward},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toFutuRehabType(tt.in); got != tt.want {
				t.Fatalf("toFutuRehabType(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewKLineSyncerUsesDefaultExchangeFactoryAndPropagatesStoreErrors(t *testing.T) {
	originalNewStore := newFutuKLineStore
	originalNewExchange := newFutuExchange
	t.Cleanup(func() {
		newFutuKLineStore = originalNewStore
		newFutuExchange = originalNewExchange
	})

	wantErr := errors.New("open store failed")
	newFutuKLineStore = func(string) (*backteststore.FutuKLineStore, error) {
		return nil, wantErr
	}
	if _, err := NewKLineSyncer(t.TempDir()); !errors.Is(err, wantErr) {
		t.Fatalf("NewKLineSyncer(store error) = %v, want %v", err, wantErr)
	}

	var gotAddr string
	newFutuKLineStore = backteststore.NewFutuKLineStore
	newFutuExchange = func(addr string) *pkgfutu.Exchange {
		gotAddr = addr
		return pkgfutu.NewExchange("127.0.0.1:1")
	}

	syncer, err := NewKLineSyncer(t.TempDir() + "/klines.db")
	if err != nil {
		t.Fatalf("NewKLineSyncer() error = %v", err)
	}
	impl, ok := syncer.(*kLineSyncer)
	if !ok {
		t.Fatalf("syncer type = %T, want *kLineSyncer", syncer)
	}
	if gotAddr != pkgfutu.DefaultOpenDAddr {
		t.Fatalf("newFutuExchange addr = %q, want %q", gotAddr, pkgfutu.DefaultOpenDAddr)
	}
	if impl.exchange == nil || impl.store == nil || impl.syncFn == nil || impl.closeFn == nil {
		t.Fatalf("constructed syncer = %#v", impl)
	}
	if err := impl.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestKLineSyncerSyncAndCloseDelegate(t *testing.T) {
	progress := &backteststore.SyncProgress{}
	params := backtestservice.KLineSyncParams{
		Symbol:       "US.AAPL",
		Intervals:    []bbgotypes.Interval{bbgotypes.Interval1m},
		Since:        time.Date(2026, time.June, 23, 9, 30, 0, 0, time.UTC),
		Until:        time.Date(2026, time.June, 23, 16, 0, 0, 0, time.UTC),
		RehabType:    backtestservice.RehabTypeBackward,
		SessionScope: "extended",
	}
	wantErr := errors.New("sync failed")
	var gotParams backtestservice.KLineSyncParams
	var gotProgress *backteststore.SyncProgress
	closeCalls := 0

	syncer := &kLineSyncer{
		syncFn: func(_ context.Context, in backtestservice.KLineSyncParams, progressArg *backteststore.SyncProgress) error {
			gotParams = in
			gotProgress = progressArg
			return wantErr
		},
		closeFn: func() error {
			closeCalls++
			return nil
		},
	}

	if err := syncer.Sync(context.Background(), params, progress); !errors.Is(err, wantErr) {
		t.Fatalf("Sync() error = %v, want %v", err, wantErr)
	}
	if gotParams.Symbol != params.Symbol || gotParams.RehabType != params.RehabType || gotParams.SessionScope != params.SessionScope || len(gotParams.Intervals) != 1 {
		t.Fatalf("Sync() params = %#v, want %#v", gotParams, params)
	}
	if gotProgress != progress {
		t.Fatalf("Sync() progress = %p, want %p", gotProgress, progress)
	}
	if err := syncer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("closeCalls = %d, want 1", closeCalls)
	}
}
