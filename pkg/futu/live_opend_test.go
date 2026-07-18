package futu

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestLiveOpenDProto108Contract(t *testing.T) {
	if os.Getenv("JFTRADE_FUTU_LIVE_TEST") != "1" {
		t.Skip("set JFTRADE_FUTU_LIVE_TEST=1 to run against local OpenD")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	exchange := NewExchange(DefaultOpenDAddr)
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()
	if err := exchange.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	client := exchange.Client()
	state, err := client.GetGlobalState(ctx)
	if err != nil {
		t.Fatalf("GetGlobalState: %v", err)
	}
	if err := opend.ValidateMinimumVersion(state.ServerVer, &state.ServerBuildNo); err != nil {
		t.Fatal(err)
	}
	t.Logf("OpenD version=%s quoteLoggedIn=%v tradeLoggedIn=%v", opend.FormatVersion(state.ServerVer, state.ServerBuildNo), state.QotLogined, state.TrdLogined)

	beforeSearch, err := client.GetSubInfo(ctx, false)
	if err != nil {
		t.Fatalf("GetSubInfo before search: %v", err)
	}
	aaplResults, err := client.GetSearchQuote(ctx, "AAPL", 100)
	if err != nil {
		t.Fatalf("GetSearchQuote AAPL: %v", err)
	}
	for index, candidate := range aaplResults {
		if index >= 5 {
			break
		}
		t.Logf("AAPL candidate[%d] market=%s(%d) code=%q name=%q type=%s",
			index,
			qotcommonpb.QotMarket(candidate.GetMarket()).String(),
			candidate.GetMarket(),
			candidate.GetCode(),
			candidate.GetName(),
			qotcommonpb.SecurityType(candidate.GetSecType()).String(),
		)
	}
	foundNamedAAPL := false
	for _, candidate := range aaplResults {
		if qotcommonpb.QotMarket(candidate.GetMarket()) == qotcommonpb.QotMarket_QotMarket_US_Security &&
			canonicalSearchQuoteSymbol("US", candidate.GetCode()) == "US.AAPL" && candidate.GetName() != "" {
			foundNamedAAPL = true
			break
		}
	}
	if !foundNamedAAPL {
		t.Fatalf("GetSearchQuote AAPL returned no named US.AAPL candidate: %#v", aaplResults)
	}
	chineseResults, err := client.GetSearchQuote(ctx, "腾讯", 100)
	if err != nil {
		t.Fatalf("GetSearchQuote Chinese name: %v", err)
	}
	foundNamedChineseResult := false
	for _, candidate := range chineseResults {
		if candidate.GetName() != "" {
			foundNamedChineseResult = true
			break
		}
	}
	if !foundNamedChineseResult {
		t.Fatalf("GetSearchQuote Chinese name returned no named candidates: %#v", chineseResults)
	}
	afterSearch, err := client.GetSubInfo(ctx, false)
	if err != nil {
		t.Fatalf("GetSubInfo after search: %v", err)
	}
	if len(beforeSearch.GetConnSubInfoList()) != len(afterSearch.GetConnSubInfoList()) {
		t.Fatalf("search changed current-connection subscription count: before=%d after=%d",
			len(beforeSearch.GetConnSubInfoList()), len(afterSearch.GetConnSubInfoList()))
	}
	for index := range beforeSearch.GetConnSubInfoList() {
		if !proto.Equal(beforeSearch.GetConnSubInfoList()[index], afterSearch.GetConnSubInfoList()[index]) {
			t.Fatalf("search changed current-connection subscription state: before=%v after=%v",
				beforeSearch.GetConnSubInfoList(), afterSearch.GetConnSubInfoList())
		}
	}
	t.Logf("search AAPL=%d Chinese=%d currentConnectionSubscriptions=%d usedQuota=%d remainQuota=%d",
		len(aaplResults),
		len(chineseResults),
		liveCurrentConnectionSubscriptionCount(afterSearch),
		afterSearch.GetTotalUsedQuota(),
		afterSearch.GetRemainQuota(),
	)

	security := &qotcommonpb.Security{
		Market: new(int32(qotcommonpb.QotMarket_QotMarket_HK_Security)),
		Code:   new("00700"),
	}
	snapshots, err := client.GetSecuritySnapshot(ctx, []*qotcommonpb.Security{security})
	if err != nil || len(snapshots) != 1 || snapshots[0].GetBasic().GetSecurity().GetCode() != "00700" {
		t.Fatalf("GetSecuritySnapshot = (%d, %v)", len(snapshots), err)
	}

	book, err := exchange.QueryOrderBook(ctx, "HK.00700", 5)
	if err != nil {
		t.Fatalf("QueryOrderBook: %v", err)
	}
	if book.Security == nil || book.Security.GetCode() != "00700" || len(book.BidList)+len(book.AskList) == 0 {
		t.Fatalf("QueryOrderBook returned no HK.00700 levels: %#v", book)
	}
	t.Logf("HK.00700 snapshotPrice=%v orderBookBids=%d asks=%d", snapshots[0].GetBasic().GetCurPrice(), len(book.BidList), len(book.AskList))
}

func TestLiveOpenDOptionBABAReadClosure(t *testing.T) {
	if os.Getenv("JFTRADE_FUTU_LIVE_TEST") != "1" {
		t.Skip("set JFTRADE_FUTU_LIVE_TEST=1 to run against local OpenD")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	exchange := NewExchange(DefaultOpenDAddr)
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()
	adapter := NewBrokerAdapter(exchange).(*futuAdapter)

	snapshot, err := adapter.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
		Symbols: []string{"US.BABA"},
	})
	if err != nil || snapshot == nil || len(snapshot.Snapshots) != 1 {
		t.Fatalf("BABA root batch snapshot = %#v, %v", snapshot, err)
	}
	chain, err := adapter.QueryDerivativeCatalog(ctx, broker.FeatureQuery{
		Market: "US", InstrumentID: "US.BABA",
		FeatureID: broker.FeatureOptionChain,
		Params:    map[string]any{"operation": "chain"},
	})
	if err != nil || len(chain.Entries) == 0 {
		t.Fatalf("BABA option chain = %#v, %v", chain, err)
	}
	contractID := firstLiveOptionInstrumentID(chain.Entries)
	if contractID == "" {
		t.Fatalf("BABA option chain returned no contract: %#v", chain.Entries[0])
	}

	for _, operation := range []string{
		"underlying_overview", "historical_volatility",
	} {
		result, queryErr := adapter.QueryOptionAnalytics(ctx, broker.FeatureQuery{
			Market: "US", InstrumentID: "US.BABA",
			FeatureID: broker.FeatureOptionAnalysis,
			Params:    map[string]any{"operation": operation},
		})
		if queryErr != nil || result == nil {
			t.Fatalf("BABA underlying %s = %#v, %v", operation, result, queryErr)
		}
	}
	for _, operation := range []string{"quote", "volatility", "exercise_probability"} {
		result, queryErr := adapter.QueryOptionAnalytics(ctx, broker.FeatureQuery{
			Market: "US", InstrumentID: contractID,
			FeatureID: broker.FeatureOptionAnalysis,
			Params:    map[string]any{"operation": operation},
		})
		if queryErr != nil || result == nil {
			t.Fatalf("BABA contract %s %s = %#v, %v", contractID, operation, result, queryErr)
		}
	}

	eventResults := make(map[string]*broker.FeatureResult)
	for _, operation := range []string{"unusual", "zero_dte", "earnings"} {
		result, queryErr := adapter.QueryOptionAnalytics(ctx, broker.FeatureQuery{
			Market: "US", InstrumentID: "US.BABA",
			FeatureID: broker.FeatureOptionEvents,
			Params: map[string]any{
				"operation": operation, "underlyingProductClass": "equity",
			},
		})
		if queryErr != nil || result == nil {
			t.Fatalf("BABA event %s = %#v, %v", operation, result, queryErr)
		}
		eventResults[operation] = result
	}
	for _, strategy := range []string{"covered_call", "cash_secured_put"} {
		result, queryErr := adapter.QueryOptionAnalytics(ctx, broker.FeatureQuery{
			Market: "US", InstrumentID: "US.BABA",
			FeatureID: broker.FeatureOptionEvents,
			Params: map[string]any{
				"operation": "seller", "sellerStrategy": strategy,
				"underlyingProductClass": "equity",
			},
		})
		if queryErr != nil || result == nil {
			t.Fatalf("BABA seller %s = %#v, %v", strategy, result, queryErr)
		}
	}

	zeroDte := eventResults["zero_dte"]
	if zeroDte != nil && len(zeroDte.Entries) > 0 {
		contextValue, ok := zeroDte.Entries[0]["drilldownContext"].(map[string]any)
		if !ok {
			t.Fatalf("BABA 0DTE context = %#v", zeroDte.Entries[0])
		}
		result, queryErr := adapter.QueryOptionAnalytics(ctx, broker.FeatureQuery{
			Market: "US", InstrumentID: "US.BABA",
			FeatureID: broker.FeatureOptionEvents,
			Params: map[string]any{
				"operation":       "zero_dte_contract",
				"expiryTimestamp": contextValue["expiryTimestamp"],
				"chainLocator":    contextValue["chain"],
				"sort":            "volume",
				"optionType":      "all",
			},
		})
		if queryErr != nil || result == nil {
			t.Fatalf("BABA 0DTE contracts = %#v, %v", result, queryErr)
		}
	}
	t.Logf(
		"BABA snapshot=%d expiries=%d contract=%s unusual=%d zeroDTE=%d earnings=%d",
		len(snapshot.Snapshots), len(chain.Entries), contractID,
		len(eventResults["unusual"].Entries), len(eventResults["zero_dte"].Entries),
		len(eventResults["earnings"].Entries),
	)
}

func firstLiveOptionInstrumentID(entries []map[string]any) string {
	for _, expiry := range entries {
		rows, ok := expiry["option"].([]any)
		if !ok {
			continue
		}
		for _, rawRow := range rows {
			row, ok := rawRow.(map[string]any)
			if !ok {
				continue
			}
			for _, sideName := range []string{"call", "put"} {
				side, _ := row[sideName].(map[string]any)
				basic, _ := side["basic"].(map[string]any)
				security, _ := basic["security"].(map[string]any)
				instrumentID := strings.ToUpper(stringValue(security["instrumentId"]))
				if strings.HasPrefix(instrumentID, "US.") {
					return instrumentID
				}
			}
		}
	}
	return ""
}

func liveCurrentConnectionSubscriptionCount(info interface {
	GetConnSubInfoList() []*qotcommonpb.ConnSubInfo
}) int {
	total := 0
	for _, connection := range info.GetConnSubInfoList() {
		if connection != nil && connection.GetIsOwnConnData() {
			total += len(connection.GetSubInfoList())
		}
	}
	return total
}
