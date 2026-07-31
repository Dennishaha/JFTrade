package futu

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"

	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
)

func TestSecurityDetailsMapPreservesCompleteBrokerNeutralWireShape(t *testing.T) {
	decimalValue := decimal.RequireFromString("12.34")
	floatValue := 1_700_000_000.25
	volumeValue := decimal.RequireFromString("1700000000.25")
	securityVolume := decimal.RequireFromString("9007199254740993")
	quoteVolume := decimal.RequireFromString("4321.25")
	volume := int64(4321)
	securityID := int64(9988)
	count := int32(17)
	flag := true
	text := "issuer-01"
	ref := &pkgfutu.SecurityRef{InstrumentID: "US.AAPL", Market: "US", Symbol: "AAPL"}

	details := &pkgfutu.SecurityDetails{
		InstrumentID: "US.AAPL", Market: "US", Symbol: "AAPL", SecurityID: &securityID,
		Name: "Apple", SecurityType: "Stock", ExchangeType: "NASDAQ",
		ListTime: "1980-12-12", ListTimestamp: &floatValue, Delisting: &flag,
		LotSize: 1, IsSuspend: false, PriceSpread: decimalValue,
		UpdateTime: "2026-07-26 09:31:00", UpdateTimestamp: &floatValue,
		HighPrice: decimalValue, OpenPrice: decimalValue, LowPrice: decimalValue,
		LastClosePrice: decimalValue, CurrentPrice: decimalValue, Volume: securityVolume,
		Turnover: decimalValue, TurnoverRate: decimalValue,
		AskPrice: &decimalValue, BidPrice: &decimalValue, AskVolume: &quoteVolume, BidVolume: &quoteVolume,
		Amplitude: &decimalValue, AveragePrice: &decimalValue, BidAskRatio: &decimalValue,
		VolumeRatio: &decimalValue, Highest52WeeksPrice: &decimalValue, Lowest52WeeksPrice: &decimalValue,
		HighestHistoryPrice: &decimalValue, LowestHistoryPrice: &decimalValue,
		SessionStatus: "REGULAR", ClosePrice5Minute: &decimalValue,
		PreMarket: &pkgfutu.ExtendedMarketQuote{
			Price: &decimalValue, HighPrice: &decimalValue, LowPrice: &decimalValue, Volume: &volumeValue,
			Turnover: &decimalValue, ChangeVal: &decimalValue, ChangeRate: &decimalValue,
			Amplitude: &decimalValue, QuoteTime: "2026-07-26T08:30:00Z",
		},
		AfterMarket: &pkgfutu.ExtendedMarketQuote{Price: &decimalValue, QuoteTime: "2026-07-26T20:00:00Z"},
		Overnight:   &pkgfutu.ExtendedMarketQuote{Price: &decimalValue, QuoteTime: "2026-07-27T01:00:00Z"},
		Equity: &pkgfutu.EquitySecurityDetails{
			IssuedShares: volume, IssuedMarketValue: decimalValue, NetAsset: decimalValue,
			NetProfit: decimalValue, EarningsPerShare: decimalValue, OutstandingShares: volume,
			OutstandingMarketVal: decimalValue, NetAssetPerShare: decimalValue,
			EarningsYieldRate: decimalValue, PERate: decimalValue, PBRate: decimalValue,
			PETTMRate: decimalValue, DividendTTM: &decimalValue, DividendRatioTTM: &decimalValue,
			DividendLFY: &decimalValue, DividendLFYRatio: &decimalValue,
		},
		Warrant: &pkgfutu.WarrantSecurityDetails{
			ConversionRate: decimalValue, WarrantType: "CALL", StrikePrice: decimalValue,
			MaturityTime: "2027-01-01", EndTradeTime: "2026-12-31", Owner: ref,
			RecoveryPrice: decimalValue, StreetVolume: volume, IssueVolume: volume,
			StreetRate: decimalValue, Delta: decimalValue, ImpliedVolatility: decimalValue,
			Premium: decimalValue, MaturityTimestamp: &floatValue, EndTradeTimestamp: &floatValue,
			Leverage: &decimalValue, InOutPriceRatio: &decimalValue, BreakEvenPoint: &decimalValue,
			ConversionPrice: &decimalValue, PriceRecoveryRatio: &decimalValue, Score: &decimalValue,
			UpperStrikePrice: &decimalValue, LowerStrikePrice: &decimalValue,
			InLinePriceStatus: "IN", IssuerCode: &text,
		},
		Option: &pkgfutu.OptionSecurityDetails{
			OptionType: "CALL", Owner: ref, StrikeTime: "2026-12-18", StrikePrice: decimalValue,
			ContractSize: count, ContractSizeFloat: &decimalValue, OpenInterest: count,
			ImpliedVolatility: decimalValue, Premium: decimalValue, Delta: decimalValue,
			Gamma: decimalValue, Vega: decimalValue, Theta: decimalValue, Rho: decimalValue,
			StrikeTimestamp: &floatValue, IndexOptionType: "NORMAL", NetOpenInterest: &count,
			ExpiryDateDistance: &count, ContractNominalValue: &decimalValue,
			OwnerLotMultiplier: &decimalValue, OptionAreaType: "AMERICAN",
			ContractMultiplier: &decimalValue,
		},
		Index: &pkgfutu.IndexSecurityDetails{RaiseCount: 3, FallCount: 2, EqualCount: 1},
		Plate: &pkgfutu.PlateSecurityDetails{RaiseCount: 4, FallCount: 5, EqualCount: 6},
		Future: &pkgfutu.FutureSecurityDetails{
			LastSettlePrice: decimalValue, Position: count, PositionChange: count,
			LastTradeTime: "2026-12-31", LastTradeTimestamp: &floatValue, IsMainContract: true,
		},
		Trust: &pkgfutu.TrustSecurityDetails{
			DividendYield: decimalValue, AUM: decimalValue, OutstandingUnit: volume,
			NetAssetValue: decimalValue, Premium: decimalValue, AssetClass: "EQUITY",
		},
	}

	wire := SecurityDetailsMap(details)
	if wire["instrumentId"] != "US.AAPL" || wire["securityId"] != securityID ||
		wire["priceSpread"] != "12.34" || wire["volume"] != "9007199254740993" ||
		wire["askVolume"] != "4321.25" || wire["bidVolume"] != "4321.25" ||
		wire["listTimestamp"] != json.Number("1700000000.25") {
		t.Fatalf("top-level wire fields = %#v", wire)
	}
	for _, removedKey := range []string{"highPrecisionVolume", "highPrecisionAskVol", "highPrecisionBidVol"} {
		if _, exists := wire[removedKey]; exists {
			t.Fatalf("obsolete %s field remains in wire: %#v", removedKey, wire)
		}
	}
	extended := requireSecurityMap(t, wire["extended"])
	preMarket := requireSecurityMap(t, extended["preMarket"])
	mappedVolume, ok := preMarket["volume"].(string)
	if preMarket["price"] != "12.34" || !ok || mappedVolume != volumeValue.String() ||
		preMarket["quoteTime"] != "2026-07-26T08:30:00Z" {
		t.Fatalf("extended quote = %#v", preMarket)
	}
	equity := requireSecurityMap(t, wire["equity"])
	if equity["issuedShares"] != volume || equity["dividendTTM"] != "12.34" {
		t.Fatalf("equity wire = %#v", equity)
	}
	warrant := requireSecurityMap(t, wire["warrant"])
	if warrant["issuerCode"] != text || warrant["maturityTimestamp"] != json.Number("1700000000.25") {
		t.Fatalf("warrant wire = %#v", warrant)
	}
	option := requireSecurityMap(t, wire["option"])
	if option["netOpenInterest"] != count || option["contractMultiplier"] != "12.34" {
		t.Fatalf("option wire = %#v", option)
	}
	if requireSecurityMap(t, wire["index"])["raiseCount"] != int32(3) ||
		requireSecurityMap(t, wire["plate"])["fallCount"] != int32(5) ||
		requireSecurityMap(t, wire["future"])["isMainContract"] != true ||
		requireSecurityMap(t, wire["trust"])["assetClass"] != "EQUITY" {
		t.Fatalf("specialized security blocks = %#v", wire)
	}
}

func TestSecurityDetailsMapKeepsMissingOptionalAndProductBlocksNull(t *testing.T) {
	if SecurityDetailsMap(nil) != nil || ExtendedMarketQuoteSecurityMap(nil) != nil || SecurityRefMap(nil) != nil {
		t.Fatal("nil Futu security models should remain nil")
	}
	if optionalInt32(nil) != nil || optionalString(nil) != nil {
		t.Fatal("nil option and warrant scalar pointers should remain nil")
	}

	wire := SecurityDetailsMap(&pkgfutu.SecurityDetails{InstrumentID: "HK.00700"})
	for _, key := range []string{"securityId", "listTimestamp", "delisting", "askPrice", "bidPrice", "askVolume", "bidVolume"} {
		if wire[key] != nil {
			t.Fatalf("%s = %#v, want nil", key, wire[key])
		}
	}
	for _, key := range []string{"equity", "warrant", "option", "index", "plate", "future", "trust"} {
		if !isNilSecurityBlock(wire[key]) {
			t.Fatalf("%s block = %#v, want nil", key, wire[key])
		}
	}
	extended := requireSecurityMap(t, wire["extended"])
	if !isNilSecurityBlock(extended["preMarket"]) ||
		!isNilSecurityBlock(extended["afterMarket"]) ||
		!isNilSecurityBlock(extended["overnight"]) {
		t.Fatalf("missing extended sessions = %#v", extended)
	}

	emptyQuote := ExtendedMarketQuoteSecurityMap(&pkgfutu.ExtendedMarketQuote{})
	for _, key := range []string{"price", "highPrice", "lowPrice", "volume", "turnover", "changeVal", "changeRate", "amplitude"} {
		if !isNilSecurityBlock(emptyQuote[key]) {
			t.Fatalf("empty extended quote %s = %#v, want nil", key, emptyQuote[key])
		}
	}
}

func TestSecurityRefMapUsesCanonicalIdentity(t *testing.T) {
	wire := SecurityRefMap(&pkgfutu.SecurityRef{InstrumentID: "HK.00700", Market: "HK", Symbol: "00700"})
	if wire["instrumentId"] != "HK.00700" || wire["market"] != "HK" || wire["symbol"] != "00700" {
		t.Fatalf("security reference = %#v", wire)
	}
}

func requireSecurityMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value type = %T, want map[string]any", value)
	}
	return result
}

func isNilSecurityBlock(value any) bool {
	if value == nil {
		return true
	}
	block, ok := value.(map[string]any)
	if ok {
		return block == nil
	}
	floatPointer, ok := value.(*float64)
	return ok && floatPointer == nil
}
