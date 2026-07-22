package futu

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/shopspring/decimal"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotupdateorderbookpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotupdateorderbook"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestBalanceMapFromBrokerFundsUsesCurrencyRowsBeforeAccountFallback(t *testing.T) {
	hkdCash := 1000.0
	hkdAvailable := 920.0
	usdCash := 250.0
	snapshot := &BrokerFundsSnapshot{
		Market: "US",
		Cash:   new(9999.0),
		CurrencyBalances: []BrokerCurrencyBalanceSnapshot{
			{Currency: "HKD", Cash: &hkdCash, AvailableWithdrawalCash: &hkdAvailable},
			{Currency: "USD", Cash: &usdCash},
		},
	}

	balances := balanceMapFromBrokerFunds(snapshot)
	if len(balances) != 2 {
		t.Fatalf("balance count = %d, want 2", len(balances))
	}
	if got := balances["HKD"].Available.Float64(); got != hkdAvailable {
		t.Fatalf("HKD available = %v, want %v", got, hkdAvailable)
	}
	if got := balances["HKD"].NetAsset.Float64(); got != hkdCash {
		t.Fatalf("HKD net asset = %v, want %v", got, hkdCash)
	}
	if got := balances["USD"].Available.Float64(); got != usdCash {
		t.Fatalf("USD available fallback = %v, want cash %v", got, usdCash)
	}
}

func TestBalanceMapFromBrokerFundsFallsBackToMarketCurrencyAndLockedCash(t *testing.T) {
	cash := 1000.0
	available := 750.0
	availableFunds := 840.0
	maxWithdrawal := 700.0
	balances := balanceMapFromBrokerFunds(&BrokerFundsSnapshot{
		Market:                  "US",
		Cash:                    &cash,
		AvailableWithdrawalCash: &available,
		AvailableFunds:          &availableFunds,
		MaxWithdrawal:           &maxWithdrawal,
	})

	balance, ok := balances["USD"]
	if !ok {
		t.Fatalf("balances = %#v, want USD default for US market", balances)
	}
	if got := balance.Available.Float64(); got != availableFunds {
		t.Fatalf("available cash = %v, want broker available funds %v", got, availableFunds)
	}
	if got := balance.Locked.Float64(); got != 160 {
		t.Fatalf("locked cash = %v, want cash - available funds = 160", got)
	}
	if got := balance.MaxWithdrawAmount.Float64(); got != maxWithdrawal {
		t.Fatalf("max withdrawal = %v, want %v", got, maxWithdrawal)
	}
	for marketCode, currency := range map[string]string{"CN": "CNH", "MY": "MYR"} {
		fallback := balanceMapFromBrokerFunds(&BrokerFundsSnapshot{
			Market: marketCode,
			Cash:   &cash,
		})
		if _, ok := fallback[currency]; !ok {
			t.Fatalf("balances = %#v, want %s fallback for %s market", fallback, currency, marketCode)
		}
	}

	negativeDifference := balanceMapFromBrokerFunds(&BrokerFundsSnapshot{
		Cash:                    new(100.0),
		AvailableWithdrawalCash: new(120.0),
		FrozenCash:              new(8.0),
	})
	if got := negativeDifference["HKD"].Locked.Float64(); got != 0 {
		t.Fatalf("negative locked cash = %v, want clamped zero", got)
	}
}

func TestBalanceMapFromFundsAndBrokerOrderSortBoundaries(t *testing.T) {
	balances := balanceMapFromFunds(&trdcommonpb.Funds{
		Currency:          futuTestPtr(int32(trdcommonpb.Currency_Currency_USD)),
		Cash:              new(1000.0),
		AvlWithdrawalCash: new(820.0),
		FrozenCash:        new(180.0),
		CashInfoList: []*trdcommonpb.AccCashInfo{{
			Currency:         futuTestPtr(int32(trdcommonpb.Currency_Currency_HKD)),
			Cash:             new(500.0),
			AvailableBalance: new(460.0),
		}},
	}, "US")
	if len(balances) != 1 {
		t.Fatalf("balanceMapFromFunds count = %d, want currency rows to win", len(balances))
	}
	if got := balances["HKD"].Available.Float64(); got != 460 {
		t.Fatalf("HKD available = %v, want 460", got)
	}

	updatedAt := brokerOrderSortKey(BrokerOrderSnapshot{
		SubmittedAt: "2026-06-20T13:30:00Z",
		UpdatedAt:   "2026-06-20T13:35:00Z",
	})
	if updatedAt != time.Date(2026, time.June, 20, 13, 35, 0, 0, time.UTC) {
		t.Fatalf("brokerOrderSortKey updated = %s", updatedAt)
	}
	submittedAt := brokerOrderSortKey(BrokerOrderSnapshot{SubmittedAt: "2026-06-20 09:30:00"})
	if submittedAt != time.Date(2026, time.June, 20, 9, 30, 0, 0, time.UTC) {
		t.Fatalf("brokerOrderSortKey submitted fallback = %s", submittedAt)
	}
	fillAt := brokerOrderFillSortKey(BrokerOrderFillSnapshot{FilledAt: "2026-06-20T13:31:00Z"})
	if fillAt != time.Date(2026, time.June, 20, 13, 31, 0, 0, time.UTC) {
		t.Fatalf("brokerOrderFillSortKey = %s", fillAt)
	}
}

func TestBrokerOrderMappingCoversOrderLifecycleEnums(t *testing.T) {
	gtt := "GTT"
	price := 178.25
	filled := 3.0
	order := bbgoOrderFromBrokerOrder(BrokerOrderSnapshot{
		Market:         "US",
		Symbol:         "US.AAPL",
		BrokerOrderID:  "12345",
		Side:           "SELLSHORT",
		OrderType:      "STOPLIMIT",
		Status:         "FILLED_PART",
		Quantity:       10,
		FilledQuantity: &filled,
		Price:          &price,
		TimeInForce:    &gtt,
		SubmittedAt:    "2026-06-20T13:30:00Z",
		UpdatedAt:      "2026-06-20T13:35:00Z",
	})

	if order.Side != types.SideTypeSell {
		t.Fatalf("side = %s, want sell for SELLSHORT", order.Side)
	}
	if order.Type != types.OrderTypeStopLimit {
		t.Fatalf("order type = %s, want stop limit", order.Type)
	}
	if order.TimeInForce != types.TimeInForceGTT {
		t.Fatalf("time in force = %s, want GTT", order.TimeInForce)
	}
	if order.Status != types.OrderStatusPartiallyFilled || !order.IsWorking {
		t.Fatalf("status/working = %s/%v, want partially-filled working order", order.Status, order.IsWorking)
	}
	if got := order.OrderID; got != 12345 {
		t.Fatalf("OrderID = %d, want 12345", got)
	}
	if got := order.ExecutedQuantity.Float64(); got != filled {
		t.Fatalf("executed quantity = %v, want %v", got, filled)
	}

	statusCases := map[string]types.OrderStatus{
		"FILLED_ALL":    types.OrderStatusFilled,
		"CANCELLED_ALL": types.OrderStatusCanceled,
		"SUBMITFAILED":  types.OrderStatusRejected,
		"TIMEOUT":       types.OrderStatusNew,
	}
	for raw, want := range statusCases {
		if got := bbgoOrderStatusFromBrokerOrderStatus(raw); got != want {
			t.Fatalf("status %s = %s, want %s", raw, got, want)
		}
	}
}

func TestBrokerOrderTypeAndTimeInForceMappingsCoverTradingVariants(t *testing.T) {
	orderTypes := map[string]types.OrderType{
		"MARKET":            types.OrderTypeMarket,
		"TWAP_MARKET":       types.OrderTypeMarket,
		"STOP":              types.OrderTypeStopMarket,
		"TRAILINGSTOP":      types.OrderTypeStopMarket,
		"STOPLIMIT":         types.OrderTypeStopLimit,
		"TRAILINGSTOPLIMIT": types.OrderTypeStopLimit,
		"MARKETIFTOUCHED":   types.OrderTypeTakeProfitMarket,
		"LIMITIFTOUCHED":    types.OrderTypeTakeProfit,
		"LIMIT":             types.OrderTypeLimit,
	}
	for raw, want := range orderTypes {
		if got := bbgoOrderTypeFromBrokerOrderType(raw); got != want {
			t.Fatalf("bbgoOrderTypeFromBrokerOrderType(%q) = %s, want %s", raw, got, want)
		}
	}

	for _, tc := range []struct {
		raw  *string
		want types.TimeInForce
	}{
		{raw: nil, want: ""},
		{raw: new("IOC"), want: types.TimeInForceIOC},
		{raw: new("FOK"), want: types.TimeInForceFOK},
		{raw: new("GTC"), want: types.TimeInForceGTC},
		{raw: new("DAY"), want: ""},
	} {
		if got := bbgoTimeInForceFromBrokerOrder(tc.raw); got != tc.want {
			t.Fatalf("bbgoTimeInForceFromBrokerOrder(%v) = %s, want %s", tc.raw, got, tc.want)
		}
	}
	if got := bbgoAccountTypeFromRuntimeAccountType(" margin "); got != types.AccountTypeMargin {
		t.Fatalf("account type = %s, want margin", got)
	}
}

func TestBrokerOrderMarketAndCurrencyBoundaries(t *testing.T) {
	marketCases := map[string]string{
		"hk.00700":   "HK",
		"us.aapl":    "US",
		"sh.600519":  "CN",
		"sz.000001":  "CN",
		"sg.d05":     "SG",
		"jp.7203":    "JP",
		"au.bhp":     "AU",
		"my.1155":    "MY",
		"ca.shop":    "CA",
		"UNKNOWN.X":  "FALLBACK",
		"bareSymbol": "HK",
	}
	for symbol, want := range marketCases {
		fallback := ""
		if symbol == "UNKNOWN.X" {
			fallback = "FALLBACK"
		}
		if got := marketFromSymbol(symbol, fallback); got != want {
			t.Fatalf("marketFromSymbol(%q) = %q, want %q", symbol, got, want)
		}
	}

	currencyCases := map[string]string{
		"US": "USD",
		"CN": "CNH",
		"SG": "SGD",
		"JP": "JPY",
		"MY": "MYR",
		"CA": "CAD",
		"AU": "AUD",
		"HK": "HKD",
	}
	for marketCode, want := range currencyCases {
		if got := defaultFundsCurrencyForMarket(marketCode); got != want {
			t.Fatalf("defaultFundsCurrencyForMarket(%q) = %q, want %q", marketCode, got, want)
		}
	}
	fundsCurrencyCases := map[string]trdcommonpb.Currency{
		"HK": trdcommonpb.Currency_Currency_HKD,
		"US": trdcommonpb.Currency_Currency_USD,
		"CN": trdcommonpb.Currency_Currency_CNH,
		"SG": trdcommonpb.Currency_Currency_SGD,
		"AU": trdcommonpb.Currency_Currency_AUD,
		"JP": trdcommonpb.Currency_Currency_JPY,
		"MY": trdcommonpb.Currency_Currency_MYR,
		"CA": trdcommonpb.Currency_Currency_CAD,
		"":   trdcommonpb.Currency_Currency_HKD,
	}
	for marketCode, want := range fundsCurrencyCases {
		if got := fundsCurrencyForMarket(marketCode); got != want {
			t.Fatalf("fundsCurrencyForMarket(%q) = %s, want %s", marketCode, got, want)
		}
	}

	if got := bbgoAccountTypeFromRuntimeAccountType(" derivatives "); got != types.AccountTypeFutures {
		t.Fatalf("account type = %s, want futures for derivatives", got)
	}
}

func TestExchangeLocalMarketAndOrderBookHandlerBoundaries(t *testing.T) {
	ex := NewExchange("")
	if ex.Name() != Name {
		t.Fatalf("Name() = %s, want %s", ex.Name(), Name)
	}
	if ex.PlatformFeeCurrency() != "HKD" {
		t.Fatalf("PlatformFeeCurrency() = %s, want HKD", ex.PlatformFeeCurrency())
	}
	if stream := ex.NewStream(); stream == nil {
		t.Fatal("NewStream() = nil")
	}

	ex.EnsureMarket(" us.aapl ")
	ex.EnsureMarket("US.AAPL")
	markets, err := ex.QueryMarkets(context.TODO())
	if err != nil {
		t.Fatalf("QueryMarkets: %v", err)
	}
	if got := markets["US.AAPL"].QuoteCurrency; got != "USD" {
		t.Fatalf("US.AAPL quote currency = %q, want USD", got)
	}

	var updates []string
	cancel := ex.OnOrderBookUpdate(func(symbol string) {
		updates = append(updates, symbol)
	})
	ex.dispatchOrderBookNotify(nil)
	ex.dispatchOrderBookNotify(&qotupdateorderbookpb.S2C{
		Security: &qotcommonpb.Security{
			Market: futuTestPtr(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
			Code:   new("00700"),
		},
	})
	if len(updates) != 1 || updates[0] != "HK.00700" {
		t.Fatalf("order book updates = %#v, want HK.00700", updates)
	}
	cancel()
	ex.dispatchOrderBookNotify(&qotupdateorderbookpb.S2C{
		Security: &qotcommonpb.Security{
			Market: futuTestPtr(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
			Code:   new("00005"),
		},
	})
	if len(updates) != 1 {
		t.Fatalf("order book handler was not removed, updates=%#v", updates)
	}
	ex.OnOrderBookUpdate(nil)()
}

func TestExchangeInvalidateClientClearsReadyStateAndSubscriptions(t *testing.T) {
	ex := NewExchange("127.0.0.1:11110")
	ex.ready = true
	ex.subscriptions.markBasicQot("HK.00700")
	ex.subscriptions.markBasicQotPush("US.AAPL")
	ex.subscriptions.markKLine("US.AAPL|1m")
	ex.subscriptions.markOrderBook("HK.00700")
	ex.subscriptions.markOrderBookPush("HK.00700")

	ex.invalidateClient()

	if ex.ready {
		t.Fatal("ready = true after invalidateClient")
	}
	if ex.subscriptions.hasBasicQot("HK.00700") ||
		ex.subscriptions.hasBasicQotPush("US.AAPL") ||
		ex.subscriptions.hasKLine("US.AAPL|1m") ||
		ex.subscriptions.hasOrderBook("HK.00700") ||
		ex.subscriptions.hasOrderBookPush("HK.00700") {
		t.Fatalf("subscriptions were not reset: %#v", ex.subscriptions)
	}
}

func TestTradeSecurityInfoAndRuntimeMarketAuthorityBoundaries(t *testing.T) {
	tradeMarkets := map[string]trdcommonpb.TrdSecMarket{
		"HK.00700":  trdcommonpb.TrdSecMarket_TrdSecMarket_HK,
		"US:AAPL":   trdcommonpb.TrdSecMarket_TrdSecMarket_US,
		"SH.600519": trdcommonpb.TrdSecMarket_TrdSecMarket_CN_SH,
		"SZ.000001": trdcommonpb.TrdSecMarket_TrdSecMarket_CN_SZ,
		"SG.D05":    trdcommonpb.TrdSecMarket_TrdSecMarket_SG,
		"JP.7203":   trdcommonpb.TrdSecMarket_TrdSecMarket_JP,
		"AU.BHP":    trdcommonpb.TrdSecMarket_TrdSecMarket_AU,
		"MY.1155":   trdcommonpb.TrdSecMarket_TrdSecMarket_MY,
		"CA.SHOP":   trdcommonpb.TrdSecMarket_TrdSecMarket_CA,
	}
	for symbol, wantMarket := range tradeMarkets {
		code, gotMarket, err := tradeSecurityInfoFromSymbol(symbol)
		if err != nil {
			t.Fatalf("tradeSecurityInfoFromSymbol(%q): %v", symbol, err)
		}
		if gotMarket != wantMarket || code == "" || strings.Contains(code, ".") || strings.Contains(code, ":") {
			t.Fatalf("tradeSecurityInfoFromSymbol(%q) = code=%q market=%s, want market %s and bare code", symbol, code, gotMarket, wantMarket)
		}
	}
	for _, symbol := range []string{"", "AAPL", "EU.SAP"} {
		if _, _, err := tradeSecurityInfoFromSymbol(symbol); err == nil {
			t.Fatalf("tradeSecurityInfoFromSymbol(%q) error = nil, want validation error", symbol)
		}
	}

	runtimeMarkets := map[trdcommonpb.TrdMarket]string{
		trdcommonpb.TrdMarket_TrdMarket_HKCC:                "HK",
		trdcommonpb.TrdMarket_TrdMarket_HK_Fund:             "HK",
		trdcommonpb.TrdMarket_TrdMarket_Futures_Simulate_HK: "HK",
		trdcommonpb.TrdMarket_TrdMarket_US_Fund:             "US",
		trdcommonpb.TrdMarket_TrdMarket_Futures_Simulate_US: "US",
		trdcommonpb.TrdMarket_TrdMarket_CN:                  "CN",
		trdcommonpb.TrdMarket_TrdMarket_SG_Fund:             "SG",
		trdcommonpb.TrdMarket_TrdMarket_Futures_Simulate_SG: "SG",
		trdcommonpb.TrdMarket_TrdMarket_AU:                  "AU",
		trdcommonpb.TrdMarket_TrdMarket_JP_Fund:             "JP",
		trdcommonpb.TrdMarket_TrdMarket_Futures_Simulate_JP: "JP",
		trdcommonpb.TrdMarket_TrdMarket_MY_Fund:             "MY",
		trdcommonpb.TrdMarket_TrdMarket_CA:                  "CA",
		trdcommonpb.TrdMarket_TrdMarket_Crypto:              "CRYPTO",
		trdcommonpb.TrdMarket_TrdMarket_Futures:             "FUTURES",
	}
	for raw, want := range runtimeMarkets {
		if got := runtimeMarketAuthority(int32(raw)); got != want {
			t.Fatalf("runtimeMarketAuthority(%s) = %q, want %q", raw, got, want)
		}
	}
	authorities := runtimeMarketAuthorities([]int32{
		int32(trdcommonpb.TrdMarket_TrdMarket_US),
		int32(trdcommonpb.TrdMarket_TrdMarket_US_Fund),
		999999,
		int32(trdcommonpb.TrdMarket_TrdMarket_HK),
	})
	if len(authorities) != 2 || authorities[0] != "US" || authorities[1] != "HK" {
		t.Fatalf("runtimeMarketAuthorities = %#v, want unique US/HK and unknown skipped", authorities)
	}
	orderAuthorities := runtimeOrderMarketAuthorities([]int32{
		int32(trdcommonpb.TrdMarket_TrdMarket_US_Fund),
		int32(trdcommonpb.TrdMarket_TrdMarket_US),
		int32(trdcommonpb.TrdMarket_TrdMarket_HK_Fund),
		int32(trdcommonpb.TrdMarket_TrdMarket_HK),
	})
	if len(orderAuthorities) != 2 || orderAuthorities[0] != "US" || orderAuthorities[1] != "HK" {
		t.Fatalf("runtimeOrderMarketAuthorities = %#v, want stock-capable US/HK", orderAuthorities)
	}
	fundOnlyAuthorities := runtimeOrderMarketAuthorities([]int32{
		int32(trdcommonpb.TrdMarket_TrdMarket_HK_Fund),
		int32(trdcommonpb.TrdMarket_TrdMarket_US_Fund),
	})
	if fundOnlyAuthorities == nil || len(fundOnlyAuthorities) != 0 {
		t.Fatalf("fund-only order authorities = %#v, want explicit empty slice", fundOnlyAuthorities)
	}
	if unspecified := runtimeOrderMarketAuthorities(nil); unspecified != nil {
		t.Fatalf("unspecified order authorities = %#v, want nil fallback", unspecified)
	}

	historicalErr := (&historicalKLineRequestError{retType: 1, errCode: 42, retMsg: "session unsupported"}).Error()
	if !strings.Contains(historicalErr, "retType=1") || !strings.Contains(historicalErr, "errCode=42") || !strings.Contains(historicalErr, "session unsupported") {
		t.Fatalf("historical K-line error = %q", historicalErr)
	}
}

func TestKLineSessionRegistryResolvesExactRecordAndQuoteSamples(t *testing.T) {
	ex := NewExchange("")
	start := time.Date(2026, time.June, 20, 13, 30, 0, 0, time.UTC)
	kline := types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval5m,
		StartTime: types.Time(start),
		EndTime:   types.Time(start.Add(5 * time.Minute)),
	}
	ex.RegisterKLineSession(kline, market.SessionRegular)

	if got, ok := ex.ResolveKLineSession(kline); !ok || got != market.SessionRegular {
		t.Fatalf("ResolveKLineSession exact = %s/%v, want regular/true", got, ok)
	}

	sampleKLine := types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval5m,
		StartTime: types.Time(start.Add(30 * time.Minute)),
		EndTime:   types.Time(start.Add(35 * time.Minute)),
	}
	ex.RecordMarketSessionSample(" us.aapl ", market.SessionPre, start.Add(31*time.Minute))
	ex.RecordMarketSessionSample("US.AAPL", market.SessionRegular, start.Add(34*time.Minute))
	if got, ok := ex.ResolveKLineSession(sampleKLine); !ok || got != market.SessionRegular {
		t.Fatalf("ResolveKLineSession sampled = %s/%v, want latest in-window regular/true", got, ok)
	}
}

func TestKLineSessionSamplePruningAndWindowFallback(t *testing.T) {
	now := time.Date(2026, time.June, 20, 14, 0, 0, 0, time.UTC)
	samples := pruneMarketSessionSamples([]marketSessionSample{
		{at: now.Add(-13 * time.Hour), session: market.SessionPre},
		{at: now.Add(-2 * time.Minute), session: market.SessionUnknown},
		{at: now.Add(-time.Minute), session: market.SessionRegular},
	}, now)
	if len(samples) != 1 || samples[0].session != market.SessionRegular {
		t.Fatalf("pruned samples = %#v, want only recent known regular sample", samples)
	}

	session, ok := resolveSessionFromSamples(
		[]marketSessionSample{
			{at: now.Add(-20 * time.Minute), session: market.SessionPre},
			{at: now.Add(-2 * time.Minute), session: market.SessionRegular},
			{at: now.Add(20 * time.Minute), session: market.SessionAfter},
		},
		now.Add(-5*time.Minute),
		now,
		5*time.Minute,
	)
	if !ok || session != market.SessionRegular {
		t.Fatalf("resolveSessionFromSamples = %s/%v, want regular/true", session, ok)
	}
}

func TestMergeStaticInfoIntoSecurityDetailsFillsMissingFieldsWithoutClobberingSnapshot(t *testing.T) {
	details := &SecurityDetails{
		Name:         "Snapshot Name",
		SecurityType: "Eqty",
		LotSize:      100,
		Option: &OptionSecurityDetails{
			OptionType:  "SnapshotCall",
			StrikePrice: decimal.NewFromInt(100),
		},
	}
	info := &qotcommonpb.SecurityStaticInfo{
		Basic: &qotcommonpb.SecurityStaticBasic{
			Security:      &qotcommonpb.Security{Market: futuTestPtr(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
			Id:            new(int64(700)),
			Name:          new("Static Name"),
			SecType:       futuTestPtr(int32(qotcommonpb.SecurityType_SecurityType_Eqty)),
			ExchType:      futuTestPtr(int32(qotcommonpb.ExchType_ExchType_HK_MainBoard)),
			ListTime:      new("2004-06-16"),
			ListTimestamp: new(float64(1087324800)),
			Delisting:     new(false),
			LotSize:       new(int32(500)),
		},
		WarrantExData: &qotcommonpb.WarrantStaticExData{
			Type:  futuTestPtr(int32(qotcommonpb.WarrantType_WarrantType_Bull)),
			Owner: &qotcommonpb.Security{Market: futuTestPtr(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)), Code: new("00700")},
		},
		OptionExData: &qotcommonpb.OptionStaticExData{
			Type:            futuTestPtr(int32(qotcommonpb.OptionType_OptionType_Put)),
			Owner:           &qotcommonpb.Security{Market: futuTestPtr(int32(qotcommonpb.QotMarket_QotMarket_US_Security)), Code: new("AAPL")},
			StrikeTime:      new("2026-12-18"),
			StrikePrice:     new(380.0),
			StrikeTimestamp: new(1797552000.0),
			IndexOptionType: futuTestPtr(int32(qotcommonpb.IndexOptionType_IndexOptionType_Small)),
		},
		FutureExData: &qotcommonpb.FutureStaticExData{
			LastTradeTime:      new("2026-09-18"),
			LastTradeTimestamp: new(1789689600.0),
			IsMainContract:     new(true),
		},
	}

	mergeStaticInfoIntoSecurityDetails(details, info)

	if details.SecurityID == nil || *details.SecurityID != 700 {
		t.Fatalf("SecurityID = %#v, want 700", details.SecurityID)
	}
	if details.Name != "Snapshot Name" || details.SecurityType != "Eqty" || details.LotSize != 100 {
		t.Fatalf("snapshot-owned fields were clobbered: %#v", details)
	}
	if details.ExchangeType != "HK_MainBoard" || details.ListTime != "2004-06-16" {
		t.Fatalf("static basic fields not merged: exchange=%q listTime=%q", details.ExchangeType, details.ListTime)
	}
	if details.Warrant == nil || details.Warrant.WarrantType != "Bull" || details.Warrant.Owner.Symbol != "00700" {
		t.Fatalf("warrant static fields = %#v", details.Warrant)
	}
	if details.Option.OptionType != "SnapshotCall" {
		t.Fatalf("option type was clobbered: %#v", details.Option)
	}
	if details.Option.StrikeTime != "2026-12-18" || !details.Option.StrikePrice.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("option fields = %#v, want strike time filled and snapshot strike price preserved", details.Option)
	}
	if details.Option.Owner == nil || details.Option.Owner.InstrumentID != "US.AAPL" || details.Option.IndexOptionType != "Small" {
		t.Fatalf("option owner/index type = %#v", details.Option)
	}
	if details.Future == nil || details.Future.LastTradeTime != "2026-09-18" || !details.Future.IsMainContract {
		t.Fatalf("future static fields = %#v", details.Future)
	}
}

func futuTestPtr[T any](value T) *T {
	return new(value)
}
