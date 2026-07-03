package indicatorruntime

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestIndicatorEngineNilAndUninitializedRuntimeAreSafeNoops(t *testing.T) {
	var nilEngine *IndicatorEngine
	nilEngine.Push(types.KLine{}, market.SessionUnknown)
	if snapshot := nilEngine.Snapshot(); len(snapshot) != 0 {
		t.Fatalf("nil engine snapshot = %#v, want empty map", snapshot)
	}
	if snapshot := nilEngine.SnapshotBorrowed(); len(snapshot) != 0 {
		t.Fatalf("nil engine borrowed snapshot = %#v, want empty map", snapshot)
	}

	emptyEngine := &IndicatorEngine{}
	emptyEngine.Push(types.KLine{}, market.SessionRegular)
	if snapshot := emptyEngine.Snapshot(); len(snapshot) != 0 {
		t.Fatalf("empty engine snapshot = %#v, want empty map", snapshot)
	}
	if snapshot := emptyEngine.SnapshotBorrowed(); len(snapshot) != 0 {
		t.Fatalf("empty engine borrowed snapshot = %#v, want empty map", snapshot)
	}
}
