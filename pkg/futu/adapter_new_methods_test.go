package futu

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

// --- Test: convertFundsSnapshot with full margin fields ---

func TestConvertFundsSnapshotFullMarginFields(t *testing.T) {
	debtCash := 50000.0
	isPDT := true
	pdtSeq := "3"
	beginningDTBP := 100000.0
	remainingDTBP := 75000.0
	dtCallAmount := 5000.0
	dtStatus := "UNLIMITED"
	exposureLevel := "NORMAL"
	exposureLimit := 2000000.0
	usedLimit := 800000.0
	remainingLimit := 1200000.0
	riskStatus := "LEVEL1"

	src := &BrokerFundsSnapshot{
		AccountID:          "12345",
		TradingEnvironment: "REAL",
		Market:             "HK",
		AccountType:        "CASH",
		TotalAssets:        new(float64(500000)),
		Cash:               new(float64(100000)),
		PurchasingPower:    new(float64(200000)),
		// Margin fields
		DebtCash:          &debtCash,
		IsPDT:             &isPDT,
		PDTSeq:            &pdtSeq,
		BeginningDTBP:     &beginningDTBP,
		RemainingDTBP:     &remainingDTBP,
		DTCallAmount:      &dtCallAmount,
		DTStatus:          &dtStatus,
		ExposureLevel:     &exposureLevel,
		ExposureLimit:     &exposureLimit,
		UsedLimit:         &usedLimit,
		RemainingLimit:    &remainingLimit,
		RiskStatus:        &riskStatus,
		InitialMargin:     new(float64(50000)),
		MaintenanceMargin: new(float64(25000)),
		MarginCallMargin:  new(float64(15000)),
	}

	result := convertFundsSnapshot(src)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify all margin fields passed through
	assertFloatPtrEqual(t, "debtCash", &debtCash, result.DebtCash)
	assertBoolPtrEqual(t, "isPdt", &isPDT, result.IsPDT)
	assertStringPtrEqual(t, "pdtSeq", &pdtSeq, result.PDTSeq)
	assertFloatPtrEqual(t, "beginningDTBP", &beginningDTBP, result.BeginningDTBP)
	assertFloatPtrEqual(t, "remainingDTBP", &remainingDTBP, result.RemainingDTBP)
	assertFloatPtrEqual(t, "dtCallAmount", &dtCallAmount, result.DTCallAmount)
	assertStringPtrEqual(t, "dtStatus", &dtStatus, result.DTStatus)
	assertStringPtrEqual(t, "exposureLevel", &exposureLevel, result.ExposureLevel)
	assertFloatPtrEqual(t, "exposureLimit", &exposureLimit, result.ExposureLimit)
	assertFloatPtrEqual(t, "usedLimit", &usedLimit, result.UsedLimit)
	assertFloatPtrEqual(t, "remainingLimit", &remainingLimit, result.RemainingLimit)
	assertFloatPtrEqual(t, "initialMargin", new(float64(50000)), result.InitialMargin)
	assertFloatPtrEqual(t, "maintenanceMargin", new(float64(25000)), result.MaintenanceMargin)
	assertFloatPtrEqual(t, "marginCallMargin", new(float64(15000)), result.MarginCallMargin)
	assertStringPtrEqual(t, "riskStatus", &riskStatus, result.RiskStatus)
}

func TestConvertFundsSnapshotNilMarginFields(t *testing.T) {
	src := &BrokerFundsSnapshot{
		AccountID:          "12345",
		TradingEnvironment: "REAL",
		Market:             "HK",
	}

	result := convertFundsSnapshot(src)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// All margin fields should be nil
	if result.DebtCash != nil {
		t.Fatal("expected nil debtCash")
	}
	if result.IsPDT != nil {
		t.Fatal("expected nil isPdt")
	}
	if result.ExposureLevel != nil {
		t.Fatal("expected nil exposureLevel")
	}
}

func TestConvertFundsSnapshotNilInput(t *testing.T) {
	result := convertFundsSnapshot(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

// --- Test: currencies & market assets pass through ---

func TestConvertFundsSnapshotCurrencyBalances(t *testing.T) {
	src := &BrokerFundsSnapshot{
		AccountID:          "12345",
		TradingEnvironment: "REAL",
		Market:             "HK",
		CurrencyBalances: []BrokerCurrencyBalanceSnapshot{
			{Currency: "HKD", Cash: new(100000.0)},
			{Currency: "USD", Cash: new(50000.0)},
		},
		MarketAssets: []BrokerMarketAssetSnapshot{
			{Market: "HK", Assets: new(float64(300000))},
			{Market: "US", Assets: new(float64(200000))},
		},
	}
	result := convertFundsSnapshot(src)
	if len(result.CurrencyBalances) != 2 {
		t.Fatalf("expected 2 currency balances, got %d", len(result.CurrencyBalances))
	}
	if result.CurrencyBalances[0].Currency != "HKD" {
		t.Fatalf("expected HKD, got %s", result.CurrencyBalances[0].Currency)
	}
	if len(result.MarketAssets) != 2 {
		t.Fatalf("expected 2 market assets, got %d", len(result.MarketAssets))
	}
}

// --- Test: securitiesFromSymbols ---

func TestSecuritiesFromSymbols(t *testing.T) {
	securities, err := securitiesFromSymbols([]string{"HK.00700", "US.AAPL"})
	if err != nil {
		t.Fatalf("securitiesFromSymbols: %v", err)
	}
	if len(securities) != 2 {
		t.Fatalf("expected 2 securities, got %d", len(securities))
	}
	if securities[0].GetCode() != "00700" {
		t.Fatalf("expected 00700, got %q", securities[0].GetCode())
	}
	if securities[1].GetCode() != "AAPL" {
		t.Fatalf("expected AAPL, got %q", securities[1].GetCode())
	}
}

func TestSecuritiesFromSymbolsInvalid(t *testing.T) {
	_, err := securitiesFromSymbols([]string{"INVALID"})
	if err == nil {
		t.Fatal("expected error for invalid symbol")
	}
}

func TestSecuritiesFromSymbolsEmpty(t *testing.T) {
	securities, err := securitiesFromSymbols([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(securities) != 0 {
		t.Fatal("expected empty list")
	}
}

// --- Test: securitySymbol ---

func TestSecuritySymbol(t *testing.T) {
	result := securitySymbol(&qotcommonpb.Security{Market: new(int32(1)), Code: new("00700")})
	if result != "HK.00700" {
		t.Fatalf("expected HK.00700, got %q", result)
	}
}

func TestSecuritySymbolNil(t *testing.T) {
	result := securitySymbol(nil)
	if result != "" {
		t.Fatalf("expected empty for nil, got %q", result)
	}
}

// --- Test: futuKLTypeFromIntervalString ---

func TestFutuKLTypeFromIntervalStringAll(t *testing.T) {
	tests := []struct {
		period   string
		expected qotcommonpb.KLType
	}{
		{"1m", qotcommonpb.KLType_KLType_1Min},
		{"1min", qotcommonpb.KLType_KLType_1Min},
		{"5m", qotcommonpb.KLType_KLType_5Min},
		{"5min", qotcommonpb.KLType_KLType_5Min},
		{"15m", qotcommonpb.KLType_KLType_15Min},
		{"30m", qotcommonpb.KLType_KLType_30Min},
		{"60m", qotcommonpb.KLType_KLType_60Min},
		{"1h", qotcommonpb.KLType_KLType_60Min},
		{"2h", qotcommonpb.KLType_KLType_120Min},
		{"4h", qotcommonpb.KLType_KLType_240Min},
		{"1d", qotcommonpb.KLType_KLType_Day},
		{"day", qotcommonpb.KLType_KLType_Day},
		{"1w", qotcommonpb.KLType_KLType_Week},
		{"week", qotcommonpb.KLType_KLType_Week},
		{"1M", qotcommonpb.KLType_KLType_Month},
		{"month", qotcommonpb.KLType_KLType_Month},
		{"quarter", qotcommonpb.KLType_KLType_Quarter},
		{"1y", qotcommonpb.KLType_KLType_Year},
	}
	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			got, err := futuKLTypeFromIntervalString(tt.period)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestFutuKLTypeFromIntervalStringInvalid(t *testing.T) {
	_, err := futuKLTypeFromIntervalString("invalid")
	if err == nil {
		t.Fatal("expected error for invalid period")
	}
}

// --- Test: int64AsFloat64Ptr ---

func TestInt64AsFloat64Ptr(t *testing.T) {
	result := int64AsFloat64Ptr(new(int64(12345)))
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if *result != 12345.0 {
		t.Fatalf("expected 12345.0, got %f", *result)
	}
}

func TestInt64AsFloat64PtrNil(t *testing.T) {
	result := int64AsFloat64Ptr(nil)
	if result != nil {
		t.Fatal("expected nil")
	}
}

// --- Test: brokerFundsSnapshotFromProto with full margin fields ---

func TestBrokerFundsSnapshotFromProtoFullMargin(t *testing.T) {
	account := resolvedTradeAccount{
		AccountID:          "12345",
		TradingEnvironment: "REAL",
		Market:             "HK",
		AccountType:        "MARGIN",
		protoAccountID:     12345,
		protoTrdEnv:        int32(trdcommonpb.TrdEnv_TrdEnv_Real),
		protoTrdMarket:     int32(trdcommonpb.TrdMarket_TrdMarket_HK),
	}

	protoFunds := &trdcommonpb.Funds{
		Power:             new(float64(200000)),
		TotalAssets:       new(float64(500000)),
		Cash:              new(float64(100000)),
		MarketVal:         new(float64(350000)),
		FrozenCash:        new(float64(10000)),
		DebtCash:          new(float64(50000)),
		AvlWithdrawalCash: new(float64(80000)),
		MaxPowerShort:     new(float64(100000)),
		NetCashPower:      new(float64(120000)),
		LongMv:            new(float64(350000)),
		ShortMv:           new(float64(0)),
		MaxWithdrawal:     new(float64(150000)),
		InitialMargin:     new(float64(50000)),
		MaintenanceMargin: new(float64(25000)),
		MarginCallMargin:  new(float64(15000)),
		RiskStatus:        new(int32(trdcommonpb.CltRiskStatus_CltRiskStatus_Level1)),
		SecuritiesAssets:  new(float64(300000)),
		FundAssets:        new(float64(50000)),
		BondAssets:        new(float64(0)),
		// PDT fields
		IsPdt:         new(true),
		PdtSeq:        new("3/3"),
		BeginningDTBP: new(float64(100000)),
		RemainingDTBP: new(float64(75000)),
		DtCallAmount:  new(float64(5000)),
		DtStatus:      new(int32(trdcommonpb.DTStatus_DTStatus_Unlimited)),
		// Exposure fields
		ExposureLevel:  new(int32(trdcommonpb.ExposureLevel_ExposureLevel_Normal)),
		ExposureLimit:  new(float64(2000000)),
		UsedLimit:      new(float64(800000)),
		RemainingLimit: new(float64(1200000)),
	}

	result := brokerFundsSnapshotFromProto(account, protoFunds)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify all margin fields are mapped
	if result.DebtCash == nil || *result.DebtCash != 50000 {
		t.Fatal("expected debtCash=50000")
	}
	if result.IsPDT == nil || !*result.IsPDT {
		t.Fatal("expected isPdt=true")
	}
	if result.PDTSeq == nil || *result.PDTSeq != "3/3" {
		t.Fatal("expected pdtSeq=3/3")
	}
	if result.BeginningDTBP == nil || *result.BeginningDTBP != 100000 {
		t.Fatal("expected beginningDTBP=100000")
	}
	if result.RemainingDTBP == nil || *result.RemainingDTBP != 75000 {
		t.Fatal("expected remainingDTBP=75000")
	}
	if result.DTCallAmount == nil || *result.DTCallAmount != 5000 {
		t.Fatal("expected dtCallAmount=5000")
	}
	if result.DTStatus == nil || *result.DTStatus == "" {
		t.Fatal("expected non-empty dtStatus")
	}
	if result.ExposureLevel == nil || *result.ExposureLevel == "" {
		t.Fatal("expected non-empty exposureLevel")
	}
	if result.ExposureLimit == nil || *result.ExposureLimit != 2000000 {
		t.Fatal("expected exposureLimit=2000000")
	}
	if result.UsedLimit == nil || *result.UsedLimit != 800000 {
		t.Fatal("expected usedLimit=800000")
	}
	if result.RemainingLimit == nil || *result.RemainingLimit != 1200000 {
		t.Fatal("expected remainingLimit=1200000")
	}
}

func TestBrokerFundsSnapshotFromProtoNilFunds(t *testing.T) {
	account := resolvedTradeAccount{
		AccountID:          "0",
		TradingEnvironment: "REAL",
		Market:             "HK",
	}
	result := brokerFundsSnapshotFromProto(account, nil)
	if result == nil {
		t.Fatal("expected non-nil for nil funds")
	}
}

// --- Test: broker.FundsSnapshot zero-value (no margin data) conversion round-trip ---

func TestBrokerFundsSnapshotRoundTripNoMargin(t *testing.T) {
	account := resolvedTradeAccount{
		AccountID:          "1",
		TradingEnvironment: "SIMULATE",
		Market:             "HK",
		protoAccountID:     1,
		protoTrdEnv:        int32(trdcommonpb.TrdEnv_TrdEnv_Simulate),
		protoTrdMarket:     int32(trdcommonpb.TrdMarket_TrdMarket_HK),
	}
	protoFunds := &trdcommonpb.Funds{
		Power:     new(float64(1000000)),
		Cash:      new(float64(1000000)),
		MarketVal: new(float64(0)),
	}

	snapshot := brokerFundsSnapshotFromProto(account, protoFunds)
	brokerSnapshot := convertFundsSnapshot(snapshot)

	if brokerSnapshot.PurchasingPower == nil || *brokerSnapshot.PurchasingPower != 1000000 {
		t.Fatal("purchasingPower mismatch")
	}
	// No margin data → all should be nil
	if brokerSnapshot.DebtCash != nil {
		t.Fatal("expected nil debtCash for simulate")
	}
	if brokerSnapshot.IsPDT != nil {
		t.Fatal("expected nil isPdt for simulate")
	}
}

// --- Helpers ---

func assertFloatPtrEqual(t *testing.T, field string, expected, actual *float64) {
	t.Helper()
	if expected == nil && actual == nil {
		return
	}
	if expected == nil || actual == nil {
		t.Fatalf("%s: expected %v, got %v", field, expected, actual)
	}
	if *expected != *actual {
		t.Fatalf("%s: expected %f, got %f", field, *expected, *actual)
	}
}

func assertBoolPtrEqual(t *testing.T, field string, expected, actual *bool) {
	t.Helper()
	if expected == nil && actual == nil {
		return
	}
	if expected == nil || actual == nil {
		t.Fatalf("%s: expected %v, got %v", field, expected, actual)
	}
	if *expected != *actual {
		t.Fatalf("%s: expected %v, got %v", field, *expected, *actual)
	}
}

func assertStringPtrEqual(t *testing.T, field string, expected, actual *string) {
	t.Helper()
	if expected == nil && actual == nil {
		return
	}
	if expected == nil || actual == nil {
		t.Fatalf("%s: expected %v, got %v", field, expected, actual)
	}
	if *expected != *actual {
		t.Fatalf("%s: expected %q, got %q", field, *expected, *actual)
	}
}

// --- Order Book conversion tests ---

func TestOrderBookLevelFromPb(t *testing.T) {
	detail := &qotcommonpb.OrderBookDetail{
		OrderID: new(int64(12345)),
		Volume:  new(int64(1000)),
	}
	pb := &qotcommonpb.OrderBook{
		Price:       new(175.5),
		Volume:      new(int64(5000)),
		OrederCount: new(int32(3)),
		DetailList:  []*qotcommonpb.OrderBookDetail{detail},
	}

	level := orderBookLevelFromPb(pb)

	if level.Price != 175.5 {
		t.Errorf("Price = %v, want 175.5", level.Price)
	}
	if level.Volume != 5000 {
		t.Errorf("Volume = %v, want 5000", level.Volume)
	}
	if level.OrderCount != 3 {
		t.Errorf("OrderCount = %v, want 3", level.OrderCount)
	}
	if len(level.DetailList) != 1 {
		t.Fatalf("DetailList len = %v, want 1", len(level.DetailList))
	}
	if level.DetailList[0].OrderID != 12345 {
		t.Errorf("DetailList[0].OrderID = %v, want 12345", level.DetailList[0].OrderID)
	}
	if level.DetailList[0].Volume != 1000 {
		t.Errorf("DetailList[0].Volume = %v, want 1000", level.DetailList[0].Volume)
	}
}

func TestOrderBookLevelFromPbNil(t *testing.T) {
	level := orderBookLevelFromPb(nil)
	if level.Price != 0 || level.Volume != 0 || level.OrderCount != 0 {
		t.Error("expected zero value for nil input")
	}
	if len(level.DetailList) != 0 {
		t.Error("expected empty DetailList for nil input")
	}
}

func TestOrderBookLevelFromPbEmptyDetails(t *testing.T) {
	pb := &qotcommonpb.OrderBook{
		Price:       new(100.0),
		Volume:      new(int64(200)),
		OrederCount: new(int32(1)),
	}

	level := orderBookLevelFromPb(pb)

	if level.Price != 100.0 {
		t.Errorf("Price = %v, want 100.0", level.Price)
	}
	if len(level.DetailList) != 0 {
		t.Errorf("DetailList should be empty, got %d", len(level.DetailList))
	}
}

func TestOrderBookSnapshotFromOpendResult(t *testing.T) {
	res := &opend.OrderBookResult{
		Name:           "Tencent",
		SvrRecvTimeBid: "2025-01-01 10:00:00.000",
		SvrRecvTimeAsk: "2025-01-01 10:00:01.000",
		AskList: []*qotcommonpb.OrderBook{
			{Price: new(320.0), Volume: new(int64(100)), OrederCount: new(int32(1))},
			{Price: new(321.0), Volume: new(int64(200)), OrederCount: new(int32(2))},
		},
		BidList: []*qotcommonpb.OrderBook{
			{Price: new(319.0), Volume: new(int64(150)), OrederCount: new(int32(1))},
		},
	}

	query := &broker.OrderBookQuery{
		ReadQuery: broker.ReadQuery{AccountID: "test-account"},
		Symbol:    "HK.00700",
		Num:       10,
	}

	snapshot := orderBookSnapshotFromOpendResult(res, query)
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}

	if snapshot.AccountID != "test-account" {
		t.Errorf("AccountID = %q, want test-account", snapshot.AccountID)
	}
	if snapshot.Symbol != "HK.00700" {
		t.Errorf("Symbol = %q, want HK.00700", snapshot.Symbol)
	}
	if snapshot.Name == nil || *snapshot.Name != "Tencent" {
		t.Errorf("Name = %v, want Tencent", snapshot.Name)
	}
	if snapshot.SvrRecvTimeBid == nil || *snapshot.SvrRecvTimeBid != "2025-01-01 10:00:00.000" {
		t.Errorf("SvrRecvTimeBid = %v", snapshot.SvrRecvTimeBid)
	}
	if snapshot.SvrRecvTimeAsk == nil || *snapshot.SvrRecvTimeAsk != "2025-01-01 10:00:01.000" {
		t.Errorf("SvrRecvTimeAsk = %v", snapshot.SvrRecvTimeAsk)
	}

	if len(snapshot.Asks) != 2 {
		t.Fatalf("Asks len = %d, want 2", len(snapshot.Asks))
	}
	if snapshot.Asks[0].Price != 320.0 {
		t.Errorf("Asks[0].Price = %v, want 320.0", snapshot.Asks[0].Price)
	}
	if snapshot.Asks[1].Price != 321.0 {
		t.Errorf("Asks[1].Price = %v, want 321.0", snapshot.Asks[1].Price)
	}

	if len(snapshot.Bids) != 1 {
		t.Fatalf("Bids len = %d, want 1", len(snapshot.Bids))
	}
	if snapshot.Bids[0].Price != 319.0 {
		t.Errorf("Bids[0].Price = %v, want 319.0", snapshot.Bids[0].Price)
	}
}

func TestOrderBookSnapshotFromOpendResultNil(t *testing.T) {
	snapshot := orderBookSnapshotFromOpendResult(nil, &broker.OrderBookQuery{})
	if snapshot != nil {
		t.Error("expected nil snapshot for nil result")
	}
}

func TestOrderBookSnapshotFromOpendResultEmptyResult(t *testing.T) {
	res := &opend.OrderBookResult{}
	query := &broker.OrderBookQuery{Symbol: "HK.00700"}

	snapshot := orderBookSnapshotFromOpendResult(res, query)
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if len(snapshot.Bids) != 0 || len(snapshot.Asks) != 0 {
		t.Error("expected empty bids/asks for empty result")
	}
}
