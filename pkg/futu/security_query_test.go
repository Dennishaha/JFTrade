package futu

import (
	"context"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestSecuritySnapshotQueriesValidateInputsBeforeOpenDCall(t *testing.T) {
	exchange := &Exchange{}
	ctx := context.Background()

	snapshots, err := exchange.querySecuritySnapshotList(ctx, nil)
	if err != nil || len(snapshots) != 0 {
		t.Fatalf("empty snapshot list = %#v err=%v", snapshots, err)
	}
	statics, err := exchange.queryStaticInfoList(ctx, []string{})
	if err != nil || len(statics) != 0 {
		t.Fatalf("empty static list = %#v err=%v", statics, err)
	}
	if _, err := exchange.querySecuritySnapshotList(ctx, []string{"bad-symbol"}); err == nil {
		t.Fatal("invalid snapshot symbol should fail before OpenD request")
	}
	if _, err := exchange.queryStaticInfoList(ctx, []string{"bad-symbol"}); err == nil {
		t.Fatal("invalid static symbol should fail before OpenD request")
	}
	if _, err := exchange.querySecuritySnapshot(ctx, "bad-symbol"); err == nil {
		t.Fatal("single snapshot query should reject invalid symbol")
	}
	if _, err := exchange.queryStaticInfo(ctx, "bad-symbol"); err == nil {
		t.Fatal("single static query should reject invalid symbol")
	}
}

func TestMergeStaticInfoFillsMissingSecurityMetadataWithoutOverwritingSnapshot(t *testing.T) {
	listTimestamp := 1_787_875_200.0
	strikeTimestamp := 1_789_603_200.0
	lastTradeTimestamp := 1_789_689_600.0
	delisting := false
	isMainContract := true
	details := &SecurityDetails{
		InstrumentID: "US.AAPL",
		Market:       "US",
		Symbol:       "AAPL",
		Name:         "Snapshot Name",
		SecurityType: "Stock",
		ListTime:     "2020-01-01",
		LotSize:      100,
		Warrant:      &WarrantSecurityDetails{WarrantType: "Bull", Owner: &SecurityRef{InstrumentID: "HK.00700", Market: "HK", Symbol: "00700"}},
		Option:       &OptionSecurityDetails{StrikeTime: "2026-01-01", StrikePrice: decimal.RequireFromString("180")},
		Future:       &FutureSecurityDetails{LastTradeTime: "2026-02-01"},
	}

	mergeStaticInfoIntoSecurityDetails(details, &qotcommonpb.SecurityStaticInfo{
		Basic: &qotcommonpb.SecurityStaticBasic{
			Security:      testUSSecurity("AAPL"),
			Id:            new(int64),
			Name:          new("Static Name"),
			SecType:       futuTestPtr(int32(qotcommonpb.SecurityType_SecurityType_Bond)),
			ExchType:      futuTestPtr(int32(qotcommonpb.ExchType_ExchType_US_NYSE)),
			ListTime:      new("2026-06-01"),
			ListTimestamp: &listTimestamp,
			Delisting:     &delisting,
			LotSize:       futuTestPtr(int32(1)),
		},
		WarrantExData: &qotcommonpb.WarrantStaticExData{
			Type:  futuTestPtr(int32(qotcommonpb.WarrantType_WarrantType_Bear)),
			Owner: testHKSecurity("09988"),
		},
		OptionExData: &qotcommonpb.OptionStaticExData{
			Type:            futuTestPtr(int32(qotcommonpb.OptionType_OptionType_Put)),
			Owner:           testUSSecurity("MSFT"),
			StrikeTime:      new("2026-09-18"),
			StrikePrice:     new(200.0),
			StrikeTimestamp: &strikeTimestamp,
			IndexOptionType: futuTestPtr(int32(qotcommonpb.IndexOptionType_IndexOptionType_Normal)),
		},
		FutureExData: &qotcommonpb.FutureStaticExData{
			LastTradeTime:      new("2026-09-19"),
			LastTradeTimestamp: &lastTradeTimestamp,
			IsMainContract:     &isMainContract,
		},
	})

	if details.Name != "Snapshot Name" || details.SecurityType != "Stock" || details.ListTime != "2020-01-01" || details.LotSize != 100 {
		t.Fatalf("snapshot-owned metadata was overwritten: %#v", details)
	}
	if details.ExchangeType != "US_NYSE" || details.SecurityID == nil || details.ListTimestamp == nil || details.Delisting == nil {
		t.Fatalf("missing static basic fields were not filled: %#v", details)
	}
	if details.Warrant == nil || details.Warrant.WarrantType != "Bull" || details.Warrant.Owner.Symbol != "00700" {
		t.Fatalf("existing warrant snapshot fields were overwritten: %#v", details.Warrant)
	}
	if details.Option == nil || details.Option.OptionType != "Put" || details.Option.Owner.Symbol != "MSFT" ||
		details.Option.StrikeTime != "2026-01-01" || !details.Option.StrikePrice.Equal(decimal.RequireFromString("180")) ||
		details.Option.StrikeTimestamp == nil || details.Option.IndexOptionType != "Normal" {
		t.Fatalf("option static merge = %#v", details.Option)
	}
	if details.Future == nil || details.Future.LastTradeTime != "2026-02-01" ||
		details.Future.LastTradeTimestamp == nil || !details.Future.IsMainContract {
		t.Fatalf("future static merge = %#v", details.Future)
	}
}

func TestSecurityReferenceHelpersPreservePartialCanonicalData(t *testing.T) {
	if ref := securityRefFromCanonical("US.AAPL"); ref.InstrumentID != "US.AAPL" || ref.Market != "US" || ref.Symbol != "AAPL" {
		t.Fatalf("canonical ref = %#v", ref)
	}
	if ref := securityRefFromCanonical("AAPL"); ref.InstrumentID != "AAPL" || ref.Market != "" || ref.Symbol != "" {
		t.Fatalf("partial canonical ref = %#v", ref)
	}
	if got := enumName(7, map[int32]string{7: "RAW"}); got != "RAW" {
		t.Fatalf("enumName raw = %q", got)
	}
	if cloned := cloneInt32Ptr(nil); cloned != nil {
		t.Fatalf("cloneInt32Ptr(nil) = %#v", cloned)
	}
	if ref := securityRefFromProto(nil); ref != nil {
		t.Fatalf("securityRefFromProto(nil) = %#v", ref)
	}
	if _, _, err := futuSecurityFromSymbol(strings.Repeat("X", 100)); err == nil {
		t.Fatal("oversized symbol should not be accepted")
	}
}
