package futu

import (
	"context"
	"maps"
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

func TestLiveOpenDHKDualCounterCurrencyResolution(t *testing.T) {
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

	market := int32(qotcommonpb.QotMarket_QotMarket_HK_Security)
	securities := make([]*qotcommonpb.Security, 0, 2)
	for _, code := range []string{"00700", "80700"} {
		securities = append(securities, &qotcommonpb.Security{
			Market: &market,
			Code:   &code,
		})
	}
	staticInfo, err := exchange.Client().GetStaticInfo(ctx, securities)
	if err != nil {
		t.Fatalf("GetStaticInfo dual counters: %v", err)
	}
	resolved := make(map[string]string, len(staticInfo))
	for _, info := range staticInfo {
		if info == nil || info.GetBasic() == nil || info.GetBasic().GetSecurity() == nil {
			continue
		}
		code := info.GetBasic().GetSecurity().GetCode()
		name := info.GetBasic().GetName()
		resolved[code] = researchScreenQuoteCurrency("HK", code, name)
		t.Logf("HK.%s name=%q quoteCurrency=%s", code, name, resolved[code])
	}
	if resolved["00700"] != "HKD" || resolved["80700"] != "CNY" {
		t.Fatalf("dual-counter quote currencies = %#v", resolved)
	}
}

func TestLiveOpenDQuoteRightDiscoveryDoesNotSubscribe(t *testing.T) {
	if os.Getenv("JFTRADE_FUTU_LIVE_TEST") != "1" {
		t.Skip("set JFTRADE_FUTU_LIVE_TEST=1 to run against local OpenD")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	exchange := NewExchange(DefaultOpenDAddr)
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()
	adapter := NewBrokerAdapter(exchange).(*futuAdapter)

	if err := exchange.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	client := exchange.Client()
	before, err := client.GetSubInfo(ctx, false)
	if err != nil {
		t.Fatalf("GetSubInfo before quote-right discovery: %v", err)
	}

	rights, err := client.GetQuoteRights(ctx)
	if err != nil {
		t.Fatalf("GetQuoteRights: %v", err)
	}
	if !userInfoHasQuoteRights(rights) {
		t.Fatal("GetQuoteRights returned no entitlement fields")
	}
	t.Logf(
		"quote rights HK=%s US=%s SH=%s SZ=%s US index=%s",
		qotcommonpb.QotRight(rights.GetHkQotRight()),
		qotcommonpb.QotRight(rights.GetUsQotRight()),
		qotcommonpb.QotRight(rights.GetShQotRight()),
		qotcommonpb.QotRight(rights.GetSzQotRight()),
		qotcommonpb.QotRight(rights.GetUsIndexQotRight()),
	)

	for _, market := range []string{"HK", "US", "SH", "SZ"} {
		evaluation, evaluationErr := adapter.EvaluateCapability(ctx, broker.CapabilityEvaluationRequest{
			Market: market,
			DeclaredCapability: broker.FeatureCapability{
				Access:             broker.FeatureAccessRead,
				RequiresConnection: true,
				RequiresQuoteRight: true,
			},
		})
		if evaluationErr != nil {
			t.Fatalf("market=%s EvaluateCapability: %v", market, evaluationErr)
		}
		switch evaluation.QuoteRight.Code {
		case "QUOTE_RIGHT_AVAILABLE", "QUOTE_RIGHT_POLLING_ONLY",
			"QUOTE_RIGHT_DENIED", "QUOTE_RIGHT_UNKNOWN":
		default:
			t.Fatalf("market=%s unresolved quote right: %#v", market, evaluation.QuoteRight)
		}
		t.Logf(
			"market=%s state=%s code=%s reason=%s",
			market, evaluation.State, evaluation.QuoteRight.Code,
			evaluation.QuoteRight.Reason,
		)
	}

	after, err := client.GetSubInfo(ctx, false)
	if err != nil {
		t.Fatalf("GetSubInfo after quote-right discovery: %v", err)
	}
	if len(before.GetConnSubInfoList()) != len(after.GetConnSubInfoList()) {
		t.Fatalf("quote-right discovery changed subscription count: before=%v after=%v",
			before.GetConnSubInfoList(), after.GetConnSubInfoList())
	}
	for index := range before.GetConnSubInfoList() {
		if !proto.Equal(before.GetConnSubInfoList()[index], after.GetConnSubInfoList()[index]) {
			t.Fatalf("quote-right discovery changed subscription state: before=%v after=%v",
				before.GetConnSubInfoList(), after.GetConnSubInfoList())
		}
	}
	if before.GetTotalUsedQuota() != after.GetTotalUsedQuota() ||
		before.GetRemainQuota() != after.GetRemainQuota() {
		t.Fatalf("quote-right discovery changed subscription quota: before used=%d remain=%d; after used=%d remain=%d",
			before.GetTotalUsedQuota(), before.GetRemainQuota(),
			after.GetTotalUsedQuota(), after.GetRemainQuota())
	}
}

func TestLiveOpenDResearchCatalogReadsDoNotSubscribe(t *testing.T) {
	if os.Getenv("JFTRADE_FUTU_LIVE_TEST") != "1" {
		t.Skip("set JFTRADE_FUTU_LIVE_TEST=1 to run against local OpenD")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	exchange := NewExchange(DefaultOpenDAddr)
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()
	if err := exchange.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	client := exchange.Client()
	before, err := client.GetSubInfo(ctx, false)
	if err != nil {
		t.Fatalf("GetSubInfo before research reads: %v", err)
	}
	adapter := NewBrokerAdapter(exchange).(*futuAdapter)
	plates, err := adapter.QueryMarketResearch(ctx, broker.FeatureQuery{
		Market: "HK", FeatureID: broker.FeatureResearchIndustry,
		Params: map[string]any{"operation": "plate_list", "plateType": "concept"},
	})
	if err != nil || plates == nil || len(plates.Entries) == 0 {
		t.Fatalf("concept plate list = %#v, %v", plates, err)
	}
	plateID := stringValue(plates.Entries[0]["instrumentId"])
	if plateID == "" {
		t.Fatalf("concept plate has no canonical instrumentId: %#v", plates.Entries[0])
	}
	members, err := adapter.QueryMarketResearch(ctx, broker.FeatureQuery{
		Market: "HK", InstrumentID: plateID, FeatureID: broker.FeatureResearchIndustry,
		Params: map[string]any{"operation": "plate_members"},
	})
	if err != nil || members == nil {
		t.Fatalf("plate members %s = %#v, %v", plateID, members, err)
	}
	funds, err := adapter.QueryMarketResearch(ctx, broker.FeatureQuery{
		Market: "HK", FeatureID: broker.FeatureResearchRankings,
		Params: map[string]any{"operation": "fund_catalog"},
	})
	if err != nil || funds == nil || len(funds.Entries) == 0 {
		t.Fatalf("fund catalog = %#v, %v", funds, err)
	}
	fundCounts := map[string]int{"HK": len(funds.Entries)}
	for _, market := range []string{"US", "SH", "SZ"} {
		catalog, catalogErr := adapter.QueryMarketResearch(ctx, broker.FeatureQuery{
			Market: market, FeatureID: broker.FeatureResearchRankings,
			Params: map[string]any{"operation": "fund_catalog"},
		})
		if catalogErr != nil || catalog == nil || len(catalog.Entries) == 0 {
			t.Fatalf("market=%s fund_catalog = %#v, %v", market, catalog, catalogErr)
		}
		fundCounts[market] = len(catalog.Entries)
	}
	for market, instrumentID := range map[string]string{
		"US": "US.AAPL", "HK": "HK.00700", "SH": "SH.600519", "SZ": "SZ.000858",
	} {
		snapshot, snapshotErr := adapter.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
			Symbols: []string{instrumentID},
		})
		if snapshotErr != nil || snapshot == nil || len(snapshot.Snapshots) == 0 {
			t.Fatalf("market=%s snapshot %s unavailable: result=%#v err=%v",
				market, instrumentID, snapshot, snapshotErr)
		}
		price := float64(0)
		if snapshot.Snapshots[0].LastPrice != nil {
			price = *snapshot.Snapshots[0].LastPrice
		}
		t.Logf("market=%s snapshot=%s price=%v", market, instrumentID, price)
	}
	_, usBenchmarkErr := adapter.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
		Symbols: []string{"US..DJI", "US..SPX", "US..IXIC"},
	})
	if usBenchmarkErr == nil || !strings.Contains(usBenchmarkErr.Error(), "暂不支持美股指数") {
		t.Fatalf("US index snapshot capability error = %v", usBenchmarkErr)
	}
	for market, benchmarkIDs := range map[string][]string{
		"HK": {"HK.800000", "HK.800100", "HK.800700"},
		"CN": {"SH.000001", "SZ.399001", "SZ.399006"},
	} {
		benchmarks, benchmarkErr := adapter.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
			Symbols: benchmarkIDs,
		})
		if benchmarkErr != nil || benchmarks == nil || len(benchmarks.Snapshots) != len(benchmarkIDs) {
			t.Fatalf("market=%s benchmark snapshots = %#v, %v", market, benchmarks, benchmarkErr)
		}
		t.Logf("market=%s benchmark snapshots=%d", market, len(benchmarks.Snapshots))
	}
	fundID := stringValue(funds.Entries[0]["instrumentId"])
	fundSnapshot, fundSnapshotErr := adapter.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
		Symbols: []string{fundID},
	})
	if fundID == "" || fundSnapshotErr != nil || fundSnapshot == nil || len(fundSnapshot.Snapshots) != 1 {
		t.Fatalf("fund snapshot %q = %#v, %v", fundID, fundSnapshot, fundSnapshotErr)
	}
	plateDetails, plateDetailsErr := exchange.QuerySecurityDetails(ctx, plateID)
	if plateDetailsErr != nil || plateDetails == nil {
		t.Fatalf("plate details %s = %#v, %v", plateID, plateDetails, plateDetailsErr)
	}
	plateSnapshot, plateSnapshotErr := adapter.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
		Symbols: []string{plateID},
	})
	if plateSnapshotErr != nil || plateSnapshot == nil || len(plateSnapshot.Snapshots) != 1 {
		t.Fatalf("plate snapshot %s = %#v, %v", plateID, plateSnapshot, plateSnapshotErr)
	}
	today := time.Now().Format("2006-01-02")
	for _, calendarQuery := range []broker.FeatureQuery{
		{
			Market: "US", FeatureID: broker.FeatureResearchCalendar,
			Params: map[string]any{"operation": "earnings", "beginDate": today, "endDate": today},
		},
		{
			Market: "US", FeatureID: broker.FeatureResearchCalendar,
			Params: map[string]any{"operation": "economic", "beginDate": today, "endDate": today},
		},
	} {
		calendar, calendarErr := adapter.QueryMarketResearch(ctx, calendarQuery)
		if calendarErr != nil || calendar == nil || len(calendar.Entries) == 0 {
			t.Fatalf("calendar operation=%s result=%#v error=%v",
				calendarQuery.Params["operation"], calendar, calendarErr)
		}
		t.Logf("calendar operation=%s date=%s entries=%d",
			calendarQuery.Params["operation"], today, len(calendar.Entries))
	}
	for _, market := range []string{"US", "HK"} {
		institutions, listErr := adapter.QueryMarketResearch(ctx, broker.FeatureQuery{
			Market: market, FeatureID: broker.FeatureResearchInstitutions,
			Params: map[string]any{"operation": "list"}, PageSize: 1,
		})
		if listErr != nil || institutions == nil || len(institutions.Entries) == 0 {
			t.Fatalf("market=%s institutions unavailable: result=%#v err=%v",
				market, institutions, listErr)
		}
		institutionID, ok := researchNumber(institutions.Entries[0]["institutionId"])
		if !ok || institutionID <= 0 {
			t.Fatalf("market=%s institution has no canonical id: %#v", market, institutions.Entries[0])
		}
		for _, operation := range []string{"profile", "holdings", "distribution"} {
			detail, detailErr := adapter.QueryMarketResearch(ctx, broker.FeatureQuery{
				Market: market, FeatureID: broker.FeatureResearchInstitutions, PageSize: 5,
				Params: map[string]any{"operation": operation, "institutionId": int64(institutionID)},
			})
			if detailErr != nil || detail == nil || len(detail.Entries) == 0 {
				t.Fatalf("market=%s institution=%d operation=%s result=%#v error=%v",
					market, int64(institutionID), operation, detail, detailErr)
			}
			t.Logf("market=%s institution=%d operation=%s entries=%d",
				market, int64(institutionID), operation, len(detail.Entries))
		}
	}
	for _, calendarQuery := range []broker.FeatureQuery{
		{
			Market: "US", FeatureID: broker.FeatureResearchCalendar,
			Params: map[string]any{"operation": "dividends", "date": today},
		},
		{
			Market: "US", FeatureID: broker.FeatureResearchCalendar,
			Params: map[string]any{"operation": "ipos"},
		},
	} {
		calendar, calendarErr := adapter.QueryMarketResearch(ctx, calendarQuery)
		if calendarErr != nil || calendar == nil {
			t.Fatalf("calendar operation=%s result=%#v error=%v",
				calendarQuery.Params["operation"], calendar, calendarErr)
		}
		t.Logf("calendar operation=%s entries=%d", calendarQuery.Params["operation"], len(calendar.Entries))
	}
	for _, rankingQuery := range []broker.FeatureQuery{
		{
			Market: "US", FeatureID: broker.FeatureResearchRankings,
			Params: map[string]any{"operation": "top_movers", "direction": "up"}, PageSize: 5,
		},
		{
			Market: "US", FeatureID: broker.FeatureResearchRankings,
			Params: map[string]any{"operation": "top_movers", "direction": "down"}, PageSize: 5,
		},
		{
			Market: "US", FeatureID: broker.FeatureResearchRankings,
			Params: map[string]any{"operation": "hot"}, PageSize: 5,
		},
		{
			Market: "HK", FeatureID: broker.FeatureResearchRankings,
			Params: map[string]any{"operation": "high_dividend_state"}, PageSize: 5,
		},
		{
			Market: "HK", FeatureID: broker.FeatureResearchRankings,
			Params: map[string]any{"operation": "heatmap", "plateType": "industry"}, PageSize: 5,
		},
	} {
		ranking, rankingErr := adapter.QueryMarketResearch(ctx, rankingQuery)
		if rankingErr != nil || ranking == nil || len(ranking.Entries) == 0 {
			t.Fatalf("ranking operation=%s result=%#v error=%v",
				rankingQuery.Params["operation"], ranking, rankingErr)
		}
		if rankingQuery.Params["operation"] == "high_dividend_state" {
			for _, entry := range ranking.Entries {
				if market := stringValue(entry["market"]); market != "HK" {
					t.Fatalf("high_dividend_state returned non-HK instrument: %#v", entry)
				}
			}
		}
		t.Logf("ranking operation=%s entries=%d", rankingQuery.Params["operation"], len(ranking.Entries))
	}
	history, historyErr := adapter.MarketData().QueryKLines(ctx, broker.KLineQuery{
		ReadQuery: broker.ReadQuery{BrokerID: "futu", Market: "US"},
		Symbol:    "US.AAPL", Period: "1d",
		BeforeTime: time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano), Limit: 5,
	})
	if historyErr != nil || history == nil || len(history.KLines) == 0 {
		t.Fatalf("completed historical K = %#v, %v", history, historyErr)
	}
	t.Logf("completed historical K symbol=US.AAPL entries=%d", len(history.KLines))
	after, err := client.GetSubInfo(ctx, false)
	if err != nil {
		t.Fatalf("GetSubInfo after research reads: %v", err)
	}
	if len(before.GetConnSubInfoList()) != len(after.GetConnSubInfoList()) {
		t.Fatalf("research reads changed connection subscriptions: before=%v after=%v",
			before.GetConnSubInfoList(), after.GetConnSubInfoList())
	}
	for index := range before.GetConnSubInfoList() {
		if !proto.Equal(before.GetConnSubInfoList()[index], after.GetConnSubInfoList()[index]) {
			t.Fatalf("research reads changed subscription state: before=%v after=%v",
				before.GetConnSubInfoList(), after.GetConnSubInfoList())
		}
	}
	if before.GetTotalUsedQuota() != after.GetTotalUsedQuota() ||
		before.GetRemainQuota() != after.GetRemainQuota() {
		t.Fatalf("research reads changed subscription quota: before used=%d remain=%d; after used=%d remain=%d",
			before.GetTotalUsedQuota(), before.GetRemainQuota(),
			after.GetTotalUsedQuota(), after.GetRemainQuota())
	}
	t.Logf("plate=%s members=%d funds=%v usedQuota=%d remainQuota=%d",
		plateID, len(members.Entries), fundCounts, after.GetTotalUsedQuota(), after.GetRemainQuota())
}

func TestLiveOpenDEarningsCalendarDayWeekMonthSortAndFilter(t *testing.T) {
	if os.Getenv("JFTRADE_FUTU_LIVE_TEST") != "1" {
		t.Skip("set JFTRADE_FUTU_LIVE_TEST=1 to run against local OpenD")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	exchange := NewExchange(DefaultOpenDAddr)
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()
	if err := exchange.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	adapter := NewBrokerAdapter(exchange).(*futuAdapter)

	today := time.Now()
	dayKey := today.Format(time.DateOnly)
	weekBegin := today.AddDate(0, 0, -int(today.Weekday()))
	weekEnd := weekBegin.AddDate(0, 0, 6)
	monthFirst := time.Date(today.Year(), today.Month(), 1, 12, 0, 0, 0, time.Local)
	monthBegin := monthFirst.AddDate(0, 0, -int(monthFirst.Weekday()))
	monthLast := time.Date(today.Year(), today.Month()+1, 0, 12, 0, 0, 0, time.Local)
	monthEnd := monthLast.AddDate(0, 0, 6-int(monthLast.Weekday()))
	if int(monthEnd.Sub(monthBegin).Hours()/24)+1 < 35 {
		monthEnd = monthEnd.AddDate(0, 0, 7)
	}

	queries := []struct {
		name       string
		beginDate  string
		endDate    string
		params     map[string]any
		wantChunks int
	}{
		{name: "day", beginDate: dayKey, endDate: dayKey, wantChunks: 1},
		{
			name: "week", beginDate: weekBegin.Format(time.DateOnly),
			endDate: weekEnd.Format(time.DateOnly), wantChunks: 1,
		},
		{
			name: "month", beginDate: monthBegin.Format(time.DateOnly),
			endDate:    monthEnd.Format(time.DateOnly),
			wantChunks: int(monthEnd.Sub(monthBegin).Hours()/24+1) / earningsCalendarChunkDays,
		},
		{
			name: "sort_and_range_filter", beginDate: weekBegin.Format(time.DateOnly),
			endDate: weekEnd.Format(time.DateOnly), wantChunks: 1,
			params: map[string]any{"sort": "market_cap", "marketCapMin": 0},
		},
	}

	totalEntries := 0
	for _, test := range queries {
		params := map[string]any{
			"operation": "earnings",
			"beginDate": test.beginDate,
			"endDate":   test.endDate,
		}
		maps.Copy(params, test.params)
		result, err := adapter.QueryMarketResearch(ctx, broker.FeatureQuery{
			Market: "US", FeatureID: broker.FeatureResearchCalendar, Params: params,
		})
		if err != nil || result == nil {
			t.Fatalf("%s earnings calendar = %#v, %v", test.name, result, err)
		}
		if result.Metadata["rangeChunks"] != test.wantChunks {
			t.Fatalf("%s rangeChunks=%v want=%d metadata=%#v",
				test.name, result.Metadata["rangeChunks"], test.wantChunks, result.Metadata)
		}
		totalEntries += len(result.Entries)
		t.Logf("%s range=%s..%s chunks=%d entries=%d",
			test.name, test.beginDate, test.endDate, test.wantChunks, len(result.Entries))
	}
	if totalEntries == 0 {
		t.Fatal("live earnings calendar day/week/month/filter queries all returned no entries")
	}
}

func TestLiveOpenDSZ000858DeclaredCandlePeriods(t *testing.T) {
	if os.Getenv("JFTRADE_FUTU_LIVE_TEST") != "1" {
		t.Skip("set JFTRADE_FUTU_LIVE_TEST=1 to run against local OpenD")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	exchange := NewExchange(DefaultOpenDAddr)
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()
	reader := NewBrokerAdapter(exchange).MarketData()

	for _, period := range futuCandlePeriods() {
		t.Run(period, func(t *testing.T) {
			latest, err := reader.QueryKLines(ctx, broker.KLineQuery{
				Symbol: "SZ.000858",
				Period: period,
				Limit:  500,
			})
			if err != nil {
				t.Fatalf("latest page: %v", err)
			}
			if len(latest.KLines) == 0 || len(latest.KLines) > 500 {
				t.Fatalf("latest page candles=%d", len(latest.KLines))
			}
			assertLiveKLinePage(t, latest.KLines, "")
			latestAt, parseErr := time.Parse(time.RFC3339Nano, latest.KLines[len(latest.KLines)-1].Time)
			if parseErr != nil || time.Since(latestAt) > 90*24*time.Hour {
				t.Fatalf("latest candle time = %q, parse=%v", latest.KLines[len(latest.KLines)-1].Time, parseErr)
			}
			if !latest.Pagination.HasMore {
				t.Logf("latest=%s total=%d reached listing boundary", latestAt, len(latest.KLines))
				return
			}
			if len(latest.KLines) != 500 || latest.Pagination.NextBefore == "" {
				t.Fatalf("latest pagination = %#v, candles=%d", latest.Pagination, len(latest.KLines))
			}

			older, olderErr := reader.QueryKLines(ctx, broker.KLineQuery{
				Symbol:     "SZ.000858",
				Period:     period,
				BeforeTime: latest.Pagination.NextBefore,
				Limit:      500,
			})
			if olderErr != nil || len(older.KLines) == 0 {
				t.Fatalf("older page candles=%d error=%v", len(older.KLines), olderErr)
			}
			assertLiveKLinePage(t, older.KLines, latest.Pagination.NextBefore)
			t.Logf(
				"latest=%s first=%s older=%d nextBefore=%s",
				latest.KLines[len(latest.KLines)-1].Time,
				latest.KLines[0].Time,
				len(older.KLines),
				older.Pagination.NextBefore,
			)
		})
	}
}

func assertLiveKLinePage(t *testing.T, klines []broker.KLineItem, before string) {
	t.Helper()
	seen := make(map[string]struct{}, len(klines))
	for index, kline := range klines {
		if _, ok := seen[kline.Time]; ok {
			t.Fatalf("duplicate candle time %q", kline.Time)
		}
		seen[kline.Time] = struct{}{}
		if before != "" && kline.Time >= before {
			t.Fatalf("candle %q is not strictly before %q", kline.Time, before)
		}
		if index > 0 && klines[index-1].Time >= kline.Time {
			t.Fatalf("candles are not strictly ascending at %q -> %q", klines[index-1].Time, kline.Time)
		}
	}
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
