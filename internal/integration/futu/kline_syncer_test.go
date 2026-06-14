package futu

import (
	"testing"

	backtestservice "github.com/jftrade/jftrade-main/internal/backtest"
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
