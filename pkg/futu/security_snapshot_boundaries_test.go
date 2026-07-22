package futu

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/jftrade/jftrade-main/pkg/broker"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsecuritysnapshotpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsecuritysnapshot"
)

func TestSecurityDetailsFromSnapshotMapsDerivativeAndMarketExtensionData(t *testing.T) {
	snapshot := &qotgetsecuritysnapshotpb.Snapshot{
		Basic: &qotgetsecuritysnapshotpb.SnapshotBasicData{
			Name:        new("Derivative Basket"),
			Type:        futuTestPtr(int32(qotcommonpb.SecurityType_SecurityType_Drvt)),
			UpdateTime:  new("2026-06-30 16:00:00"),
			CurPrice:    new(101.5),
			SecStatus:   futuTestPtr(int32(qotcommonpb.SecurityStatus_SecurityStatus_Normal)),
			AskVol:      new(int64(120)),
			BidVol:      new(int64(90)),
			AskPrice:    new(101.6),
			BidPrice:    new(101.4),
			HpVolume:    new(1000.5),
			HpAskVol:    new(120.5),
			HpBidVol:    new(90.5),
			VolumeRatio: new(1.2),
			AfterMarket: &qotcommonpb.PreAfterMarketData{
				Price:      new(102.0),
				HighPrice:  new(103.0),
				LowPrice:   new(101.0),
				Volume:     new(int64(500)),
				Turnover:   new(51000.0),
				ChangeVal:  new(0.5),
				ChangeRate: new(0.49),
				Amplitude:  new(1.9),
			},
		},
		WarrantExData: &qotgetsecuritysnapshotpb.WarrantSnapshotExData{
			ConversionRate:     new(100.0),
			WarrantType:        futuTestPtr(int32(qotcommonpb.WarrantType_WarrantType_Bull)),
			StrikePrice:        new(100.5),
			MaturityTime:       new("2026-12-31"),
			EndTradeTime:       new("2026-12-24"),
			Owner:              testHKSecurity("00700"),
			RecoveryPrice:      new(80.5),
			StreetVolumn:       new(int64(3000)),
			IssueVolumn:        new(int64(10000)),
			StreetRate:         new(0.3),
			Delta:              new(0.6),
			ImpliedVolatility:  new(0.25),
			Premium:            new(0.12),
			MaturityTimestamp:  new(1798675200.0),
			EndTradeTimestamp:  new(1798070400.0),
			Leverage:           new(5.5),
			Ipop:               new(0.8),
			BreakEvenPoint:     new(105.5),
			ConversionPrice:    new(1.01),
			PriceRecoveryRatio: new(0.7),
			Score:              new(88.0),
			UpperStrikePrice:   new(120.0),
			LowerStrikePrice:   new(90.0),
			InLinePriceStatus:  futuTestPtr(int32(qotcommonpb.PriceType_PriceType_WithIn)),
			IssuerCode:         new("UBS"),
		},
		OptionExData: &qotgetsecuritysnapshotpb.OptionSnapshotExData{
			Type:                 futuTestPtr(int32(qotcommonpb.OptionType_OptionType_Call)),
			Owner:                testUSSecurity("AAPL"),
			StrikeTime:           new("2026-09-18"),
			StrikePrice:          new(200.0),
			ContractSize:         new(int32(100)),
			ContractSizeFloat:    new(100.5),
			OpenInterest:         new(int32(4500)),
			ImpliedVolatility:    new(0.33),
			Premium:              new(5.5),
			Delta:                new(0.55),
			Gamma:                new(0.03),
			Vega:                 new(0.2),
			Theta:                new(-0.01),
			Rho:                  new(0.04),
			StrikeTimestamp:      new(1789689600.0),
			IndexOptionType:      futuTestPtr(int32(qotcommonpb.IndexOptionType_IndexOptionType_Normal)),
			NetOpenInterest:      new(int32(3000)),
			ExpiryDateDistance:   new(int32(80)),
			ContractNominalValue: new(20000.0),
			OwnerLotMultiplier:   new(1.5),
			OptionAreaType:       futuTestPtr(int32(qotcommonpb.OptionAreaType_OptionAreaType_American)),
			ContractMultiplier:   new(100.0),
		},
		IndexExData:  &qotgetsecuritysnapshotpb.IndexSnapshotExData{RaiseCount: new(int32(10)), FallCount: new(int32(5)), EqualCount: new(int32(2))},
		PlateExData:  &qotgetsecuritysnapshotpb.PlateSnapshotExData{RaiseCount: new(int32(8)), FallCount: new(int32(6)), EqualCount: new(int32(1))},
		FutureExData: &qotgetsecuritysnapshotpb.FutureSnapshotExData{LastSettlePrice: new(99.5), Position: new(int32(1000)), PositionChange: new(int32(-20)), LastTradeTime: new("2026-09-18"), LastTradeTimestamp: new(1789689600.0), IsMainContract: new(true)},
		TrustExData:  &qotgetsecuritysnapshotpb.TrustSnapshotExData{DividendYield: new(0.04), Aum: new(1000000.0), OutstandingUnits: new(int64(500000)), NetAssetValue: new(10.5), Premium: new(0.02), AssetClass: futuTestPtr(int32(qotcommonpb.AssetClass_AssetClass_Bond))},
	}

	details := securityDetailsFromSnapshot(snapshot, "HK.09988")
	if details == nil {
		t.Fatal("securityDetailsFromSnapshot() = nil")
		return
	}
	if details.InstrumentID != "HK.09988" || details.Market != "HK" || details.Symbol != "09988" {
		t.Fatalf("canonical fallback ref = %#v", details)
	}
	if details.SecurityType != "Drvt" || details.SessionStatus != "Normal" || details.CurrentPrice.String() != "101.5" {
		t.Fatalf("basic mapped fields = %#v", details)
	}
	if details.ProductClass != broker.ProductClassOption ||
		details.MarketSegment != broker.MarketSegmentDerivatives {
		t.Fatalf("product identity = %s/%s", details.ProductClass, details.MarketSegment)
	}
	if details.AfterMarket == nil || details.AfterMarket.Volume == nil || *details.AfterMarket.Volume != 500 || !details.AfterMarket.Price.Equal(decimal.NewFromFloat(102.0)) {
		t.Fatalf("after-market quote = %#v", details.AfterMarket)
	}
	if details.Warrant == nil || details.Warrant.WarrantType != "Bull" || details.Warrant.Owner.Symbol != "00700" || details.Warrant.InLinePriceStatus != "WithIn" {
		t.Fatalf("warrant details = %#v", details.Warrant)
	}
	if details.Warrant.IssuerCode == nil || *details.Warrant.IssuerCode != "UBS" || details.Warrant.Leverage == nil || !details.Warrant.Leverage.Equal(decimal.NewFromFloat(5.5)) {
		t.Fatalf("warrant optional fields = %#v", details.Warrant)
	}
	if details.Option == nil || details.Option.OptionType != "Call" || details.Option.Owner.Symbol != "AAPL" || details.Option.OptionAreaType != "American" {
		t.Fatalf("option details = %#v", details.Option)
	}
	if details.Option.NetOpenInterest == nil || *details.Option.NetOpenInterest != 3000 || details.Option.ContractMultiplier == nil || !details.Option.ContractMultiplier.Equal(decimal.NewFromFloat(100.0)) {
		t.Fatalf("option optional fields = %#v", details.Option)
	}
	if details.Index == nil || details.Index.RaiseCount != 10 || details.Plate == nil || details.Plate.FallCount != 6 {
		t.Fatalf("index/plate breadth details = %#v/%#v", details.Index, details.Plate)
	}
	if details.Future == nil || details.Future.PositionChange != -20 || !details.Future.IsMainContract {
		t.Fatalf("future details = %#v", details.Future)
	}
	if details.Trust == nil || details.Trust.AssetClass != "Bond" || details.Trust.OutstandingUnit != 500000 {
		t.Fatalf("trust details = %#v", details.Trust)
	}
}

func TestSecurityDetailsFromSnapshotHandlesMissingBasicAndUnknownEnums(t *testing.T) {
	if details := securityDetailsFromSnapshot(nil, "US.AAPL"); details != nil {
		t.Fatalf("nil snapshot details = %#v", details)
	}
	if details := securityDetailsFromSnapshot(&qotgetsecuritysnapshotpb.Snapshot{}, "US.AAPL"); details != nil {
		t.Fatalf("missing basic details = %#v", details)
	}

	details := securityDetailsFromSnapshot(&qotgetsecuritysnapshotpb.Snapshot{
		Basic: &qotgetsecuritysnapshotpb.SnapshotBasicData{
			Security:  testUSSecurity("AAPL"),
			Type:      new(int32(99999)),
			SecStatus: new(int32(99999)),
		},
	}, "US.AAPL")
	if details == nil {
		t.Fatal("details with basic security = nil")
		return
	}
	if details.SecurityType != "" || details.SessionStatus != "" {
		t.Fatalf("unknown enum names = securityType %q status %q", details.SecurityType, details.SessionStatus)
	}
	if details.ProductClass != broker.ProductClassUnknown ||
		details.MarketSegment != broker.MarketSegmentSecurities {
		t.Fatalf("unknown product identity = %s/%s", details.ProductClass, details.MarketSegment)
	}
}

func testUSSecurity(code string) *qotcommonpb.Security {
	return &qotcommonpb.Security{Market: futuTestPtr(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: &code}
}
