package futu

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
)

func TestSecuritySnapshotItemNormalizesOptionMetrics(t *testing.T) {
	snapshot := &qotgetsecuritysnapshotpb.Snapshot{
		Basic: &qotgetsecuritysnapshotpb.SnapshotBasicData{
			Security: newTestSecurity(
				int32(qotcommonpb.QotMarket_QotMarket_US_Security),
				"AAPL260717C00200000",
			),
			Type:     new(int32(qotcommonpb.SecurityType_SecurityType_Drvt)),
			CurPrice: new(4.2),
			BidPrice: new(4.1),
			AskPrice: new(4.3),
		},
		OptionExData: &qotgetsecuritysnapshotpb.OptionSnapshotExData{
			Type:              new(int32(qotcommonpb.OptionType_OptionType_Call)),
			Owner:             newTestSecurity(int32(qotcommonpb.QotMarket_QotMarket_US_Security), "AAPL"),
			StrikeTime:        new("2026-07-17"),
			StrikePrice:       new(200.0),
			ContractSize:      new(int32(100)),
			OpenInterest:      new(int32(900)),
			ImpliedVolatility: new(31.5),
			Premium:           new(1.2),
			Delta:             new(0.55),
			Gamma:             new(0.03),
			Vega:              new(0.12),
			Theta:             new(-0.08),
			Rho:               new(0.02),
		},
	}

	item, ok := securitySnapshotItemFromProto(snapshot, time.Now())
	if !ok {
		t.Fatal("securitySnapshotItemFromProto returned no item")
	}
	if item.ProductClass != broker.ProductClassOption ||
		item.MarketSegment != broker.MarketSegmentDerivatives {
		t.Fatalf("product identity = %s/%s", item.ProductClass, item.MarketSegment)
	}
	if item.BidPrice == nil || *item.BidPrice != 4.1 ||
		item.AskPrice == nil || *item.AskPrice != 4.3 {
		t.Fatalf("bid/ask = %#v/%#v", item.BidPrice, item.AskPrice)
	}
	if item.Option == nil ||
		item.Option.UnderlyingCode != "US.AAPL" ||
		item.Option.StrikePrice != 200 ||
		item.Option.ImpliedVolatility != 31.5 ||
		item.Option.Delta != 0.55 ||
		item.Option.Gamma != 0.03 ||
		item.Option.Theta != -0.08 ||
		item.Option.Vega != 0.12 {
		t.Fatalf("normalized option snapshot = %#v", item.Option)
	}
}

func newTestSecurity(market int32, code string) *qotcommonpb.Security {
	return &qotcommonpb.Security{Market: &market, Code: &code}
}
