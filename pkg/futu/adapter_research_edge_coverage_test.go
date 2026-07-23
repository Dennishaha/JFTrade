package futu

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestAdvancedResearchDefaultsRejectIncompleteQueries(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		params   map[string]any
		query    broker.FeatureQuery
	}{
		{"plate list market", "Qot_GetPlateSet", map[string]any{"plateType": "industry"}, broker.FeatureQuery{}},
		{"plate members instrument", "Qot_GetPlateSecurity", map[string]any{}, broker.FeatureQuery{}},
		{"fund catalog market", "Qot_GetStaticInfo", map[string]any{}, broker.FeatureQuery{}},
		{"economic date", "Qot_GetEconomicCalendar", map[string]any{}, broker.FeatureQuery{}},
		{
			"economic market",
			"Qot_GetEconomicCalendar",
			map[string]any{"beginDate": "2026-07-23"},
			broker.FeatureQuery{Market: "XX"},
		},
		{"dividend date", "Qot_GetDividendCalendar", map[string]any{}, broker.FeatureQuery{}},
		{
			"institution id",
			"Qot_GetInstitutionProfile",
			map[string]any{"institutionId": 1.5},
			broker.FeatureQuery{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := injectAdvancedProtocolDefaults(
				test.params,
				test.protocol,
				test.query,
			); err == nil {
				t.Fatalf("%s accepted incomplete params %#v", test.protocol, test.params)
			}
		})
	}
}

func TestAdvancedResearchDefaultsTranslatePublicInputs(t *testing.T) {
	plateParams := map[string]any{"market": 11, "plateType": "region"}
	if err := injectAdvancedProtocolDefaults(
		plateParams,
		"Qot_GetPlateSet",
		broker.FeatureQuery{Market: "US"},
	); err != nil || plateParams["plateSetType"] != 2 || plateParams["plateType"] != nil {
		t.Fatalf("plate defaults = %#v, %v", plateParams, err)
	}
	economicParams := map[string]any{"beginDate": "2026-07-23"}
	if err := injectAdvancedProtocolDefaults(
		economicParams,
		"Qot_GetEconomicCalendar",
		broker.FeatureQuery{Market: "HK"},
	); err != nil || economicParams["marketList"] == nil {
		t.Fatalf("economic defaults = %#v, %v", economicParams, err)
	}
	institutionParams := map[string]any{"institutionId": float64(7)}
	if err := injectAdvancedProtocolDefaults(
		institutionParams,
		"Qot_GetInstitutionHoldingList",
		broker.FeatureQuery{},
	); err != nil || institutionParams["institutionId"] != int32(7) {
		t.Fatalf("institution defaults = %#v, %v", institutionParams, err)
	}
	newsParams := map[string]any{}
	if err := injectAdvancedProtocolDefaults(
		newsParams,
		"Qot_GetSearchNews",
		broker.FeatureQuery{Market: "US", InstrumentID: "US.AAPL"},
	); err != nil || newsParams["keyword"] != "AAPL" {
		t.Fatalf("news defaults = %#v, %v", newsParams, err)
	}
	if err := injectAdvancedProtocolDefaults(
		map[string]any{},
		"unknown",
		broker.FeatureQuery{},
	); err != nil {
		t.Fatalf("unknown protocol defaults: %v", err)
	}
}

func TestAdvancedResearchEnumTranslations(t *testing.T) {
	topMoverCases := []struct {
		value string
		want  any
	}{
		{"", nil},
		{"gainers", 0},
		{"ascending", 1},
	}
	for _, test := range topMoverCases {
		params := map[string]any{"direction": test.value}
		if err := translateTopMoversDirection(params); err != nil ||
			params["sortDir"] != test.want {
			t.Errorf("top mover %q = %#v, %v", test.value, params, err)
		}
	}
	if err := translateTopMoversDirection(
		map[string]any{"direction": "sideways"},
	); err == nil {
		t.Fatal("unsupported top mover direction succeeded")
	}

	heatMapCases := []struct {
		value any
		want  any
	}{
		{float64(2), 2},
		{"industry", 0},
		{"concept", 1},
		{"theme", 2},
	}
	for _, test := range heatMapCases {
		params := map[string]any{"plateType": test.value}
		if err := translateHeatMapPlateType(params); err != nil ||
			params["plateType"] != test.want {
			t.Errorf("heatmap %v = %#v, %v", test.value, params, err)
		}
	}
	if err := translateHeatMapPlateType(map[string]any{}); err != nil {
		t.Fatalf("missing heatmap plate type: %v", err)
	}
	for _, value := range []any{float64(3), "unsupported"} {
		if err := translateHeatMapPlateType(
			map[string]any{"plateType": value},
		); err == nil {
			t.Errorf("unsupported heatmap type %v succeeded", value)
		}
	}

	plateSetCases := []struct {
		value any
		want  any
	}{
		{float64(3), 3},
		{"all", 0},
		{"industry", 1},
		{"region", 2},
		{"concept", 3},
	}
	for _, test := range plateSetCases {
		params := map[string]any{"plateSetType": test.value}
		if err := translatePlateSetType(params); err != nil ||
			params["plateSetType"] != test.want {
			t.Errorf("plate set %v = %#v, %v", test.value, params, err)
		}
	}
	for _, params := range []map[string]any{
		{},
		{"plateSetType": math.NaN()},
		{"plateSetType": "unsupported"},
	} {
		if err := translatePlateSetType(params); err == nil {
			t.Errorf("unsupported plate set params %#v succeeded", params)
		}
	}
	if _, ok := boundedResearchEnum(float64(1.5), 0, 3); ok {
		t.Fatal("fractional research enum succeeded")
	}
}

func TestResearchNormalizationCoversAlternateWireShapes(t *testing.T) {
	payload := map[string]any{
		"list": []any{
			"preserve",
			map[string]any{
				"marketVal":        float64(100),
				"dividendYieldTTM": float64(2),
			},
		},
	}
	normalized := normalizeResearchProtocolPayload("Qot_GetHighDividendSOERank", payload)
	values := normalized["list"].([]any)
	if values[0] != "preserve" {
		t.Fatalf("scalar research row = %#v", values[0])
	}
	row := values[1].(map[string]any)
	if row["marketValue"] != float64(100) || row["dividendYield"] != float64(2) {
		t.Fatalf("alternate research fields = %#v", row)
	}

	result := &broker.FeatureResult{Entries: []map[string]any{{"id": 1}, {"id": 2}}}
	if err := applyResearchLocalPagination(
		result,
		broker.FeatureQuery{PageSize: 1, Cursor: "local:99"},
		"Qot_GetPlateSet",
	); err != nil || len(result.Entries) != 0 || result.Total == nil || *result.Total != 2 {
		t.Fatalf("out-of-range local pagination = %#v, %v", result, err)
	}
}

func TestResearchNormalizationCoversProductAndCalendarVariants(t *testing.T) {
	productCases := map[any]string{
		"eqty":   "equity",
		"trust":  "fund",
		"drvt":   "option",
		"bwrt":   "warrant",
		"future": "future",
		int32(1): "bond",
	}
	for value, want := range productCases {
		if got := researchSecurityType(value); got != want {
			t.Errorf("researchSecurityType(%v) = %q, want %q", value, got, want)
		}
	}
	if got := researchSecurityType(struct{}{}); got != "" {
		t.Fatalf("unsupported research security type = %q", got)
	}
	if got := researchProductClass(
		"Qot_GetStaticInfo",
		map[string]any{},
		"",
		map[string]any{},
	); got != "fund" {
		t.Fatalf("static-info product class = %q", got)
	}
	if got := researchProductClass(
		"Qot_GetStaticInfo",
		map[string]any{"productClass": "OPTION"},
		"",
		map[string]any{},
	); got != "option" {
		t.Fatalf("explicit product class = %q", got)
	}

	ipo := map[string]any{
		"listPrice":     float64(10),
		"ipoPriceMin":   float64(9),
		"ipoPriceMax":   float64(11),
		"listTime":      "2026-07-23",
		"issueSize":     int64(1000),
		"listTimestamp": float64(1_753_228_800),
	}
	flattenResearchIPO(ipo)
	if ipo["issuePrice"] != float64(10) ||
		ipo["listingDate"] != "2026-07-23" ||
		ipo["issueVolume"] != int64(1000) {
		t.Fatalf("IPO fallback fields = %#v", ipo)
	}
	ipoPrice := map[string]any{"ipoPrice": float64(12)}
	flattenResearchIPO(ipoPrice)
	if ipoPrice["issuePrice"] != float64(12) {
		t.Fatalf("IPO primary price field = %#v", ipoPrice)
	}
	earnings := map[string]any{
		"earningsDate":      "2026-07-25",
		"earningsTimestamp": float64(1_753_401_600),
	}
	normalizeResearchCalendarFields("Qot_GetEarningsCalendar", earnings)
	if earnings["calendarType"] != "earnings" {
		t.Fatalf("earnings calendar fields = %#v", earnings)
	}
	normalizeResearchCalendarFields("Qot_GetDividendCalendar", map[string]any{
		"exDate": "2026-07-24",
	})
	institution := map[string]any{
		"institutionId":       int64(7),
		"positionValueChange": float64(2),
		"positionCountChange": int64(3),
	}
	normalizeResearchInstitutionFields("Qot_GetInstitutionHoldingChange", institution)
	if institution["marketValueChange"] != float64(2) ||
		institution["holdingCountChange"] != int64(3) {
		t.Fatalf("institution aliases = %#v", institution)
	}
}

func TestResearchNumberAcceptsSupportedScalarTypes(t *testing.T) {
	values := []any{
		float64(1),
		float32(1),
		int(1),
		int32(1),
		int64(1),
		uint(1),
		uint32(1),
		uint64(1),
		" 1 ",
	}
	for _, value := range values {
		if number, ok := researchNumber(value); !ok || number != 1 {
			t.Errorf("researchNumber(%T(%v)) = %v, %v", value, value, number, ok)
		}
	}
	if _, ok := researchNumber(true); ok {
		t.Fatal("boolean research number succeeded")
	}
}

func TestQuoteRightsRefreshStateEdges(t *testing.T) {
	if err := (*futuAdapter)(nil).ensureQuoteRights(t.Context(), time.Now()); err == nil {
		t.Fatal("nil adapter quote-right refresh succeeded")
	}
	(*futuAdapter)(nil).captureCapabilityNotification(nil)

	adapter := NewBrokerAdapter(nil).(*futuAdapter)
	adapter.lastConnectStatus = &futuConnectCapabilityStatus{generation: 3}
	adapter.captureCapabilityNotificationAt(&notifypb.Response{S2C: &notifypb.S2C{
		Type: new(int32(notifypb.NotifyType_NotifyType_ConnStatus)),
		ConnectStatus: &notifypb.ConnectStatus{
			QotLogined: new(true),
		},
	}}, 2)
	if adapter.lastConnectStatus.generation != 3 {
		t.Fatalf("stale connect notification replaced generation: %#v", adapter.lastConnectStatus)
	}
	adapter.lastQuoteRights = &futuQuoteCapabilityRights{
		value:      completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level1),
		generation: 3,
	}
	adapter.captureCapabilityNotificationAt(&notifypb.Response{S2C: &notifypb.S2C{
		Type:     new(int32(notifypb.NotifyType_NotifyType_QotRight)),
		QotRight: completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level2),
	}}, 2)
	if adapter.lastQuoteRights.generation != 3 {
		t.Fatalf("stale quote-right notification replaced generation: %#v", adapter.lastQuoteRights)
	}

	now := time.Now().UTC()
	adapter.quoteRightsRevision = 2
	adapter.lastQuoteRightsFailure = &futuQuoteCapabilityFailure{generation: 3}
	if err := adapter.storeQuoteRights(
		3,
		1,
		now,
		completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level2),
	); err != nil || adapter.lastQuoteRightsFailure != nil {
		t.Fatalf("notification-won quote-right store = %#v, %v", adapter, err)
	}
	adapter.lastQuoteRights = &futuQuoteCapabilityRights{generation: 4}
	if err := adapter.storeQuoteRights(
		3,
		2,
		now,
		completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level2),
	); !errors.Is(err, errQuoteRightsRefreshRequired) {
		t.Fatalf("older quote-right store error = %v", err)
	}
	fresh := NewBrokerAdapter(nil).(*futuAdapter)
	fresh.lastQuoteRightsFailure = &futuQuoteCapabilityFailure{generation: 1}
	if err := fresh.storeQuoteRights(
		1,
		0,
		now,
		completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level2),
	); err != nil || fresh.lastQuoteRightsFailure != nil ||
		fresh.lastQuoteRights.generation != 1 {
		t.Fatalf("fresh quote-right store = %#v, %v", fresh, err)
	}
}

func TestQuoteRightsFailureAndExchangeGenerationEdges(t *testing.T) {
	exchange := NewExchangeWithConfig(opend.Config{
		Addr: "127.0.0.1:1", RequestTimeout: 20 * time.Millisecond,
	})
	exchange.installClientLocked(opend.New(opend.Config{}))
	generation := exchange.activeConnectionGeneration()
	adapter := NewBrokerAdapter(exchange).(*futuAdapter)
	now := time.Now().UTC()
	adapter.captureCapabilityNotification(nil)

	if err := adapter.refreshQuoteRights(t.Context(), 0); !errors.Is(
		err,
		errQuoteRightsRefreshRequired,
	) {
		t.Fatalf("zero-generation refresh error = %v", err)
	}
	adapter.lastQuoteRightsFailure = &futuQuoteCapabilityFailure{
		reason: "cached failure", retryAt: now.Add(time.Minute), generation: generation,
	}
	if err := adapter.refreshQuoteRights(t.Context(), generation); err == nil ||
		err.Error() != "cached failure" {
		t.Fatalf("cached quote-right failure = %v", err)
	}

	adapter.lastQuoteRightsFailure = nil
	adapter.lastQuoteRights = &futuQuoteCapabilityRights{
		value:      completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level1),
		generation: generation,
	}
	if err := adapter.handleQuoteRightsFetchFailure(
		generation,
		now,
		errors.New("late query failure"),
	); err != nil {
		t.Fatalf("notification-resolved query failure = %v", err)
	}
	if err := adapter.handleQuoteRightsFetchFailure(
		generation+1,
		now,
		errors.New("stale query failure"),
	); !errors.Is(err, errQuoteRightsRefreshRequired) {
		t.Fatalf("stale query failure = %v", err)
	}
	adapter.lastQuoteRights = nil
	queryErr := errors.New("query failed")
	if err := adapter.handleQuoteRightsFetchFailure(
		generation,
		now,
		queryErr,
	); !errors.Is(err, queryErr) {
		t.Fatalf("current query failure = %v", err)
	}
	adapter.lastQuoteRightsFailure = &futuQuoteCapabilityFailure{generation: generation}
	adapter.captureCapabilityNotificationAt(&notifypb.Response{S2C: &notifypb.S2C{
		Type:     new(int32(notifypb.NotifyType_NotifyType_QotRight)),
		QotRight: completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level2),
	}}, generation)
	if adapter.lastQuoteRightsFailure != nil {
		t.Fatalf("quote-right notification did not clear failure: %#v", adapter.lastQuoteRightsFailure)
	}

	exchange.activeClient.Store(opend.New(opend.Config{}))
	exchange.activeGeneration.Store(0)
	exchange.dispatchSystemNotifyFrom(exchange.activeClient.Load(), &notifypb.Response{})
	calls := 0
	exchange.systemNotifyGenHandlers = []func(*notifypb.Response, uint64){
		func(*notifypb.Response, uint64) { calls++ },
	}
	exchange.dispatchSystemNotifyAt(&notifypb.Response{}, generation)
	if calls != 1 {
		t.Fatalf("generation notification calls = %d", calls)
	}
	exchange.onSystemNotifyWithGeneration(nil)

	exchange.installClientLocked(nil)
	if exchange.activeConnectionGeneration() != 0 {
		t.Fatalf("nil client generation = %d", exchange.activeConnectionGeneration())
	}
	if (*Exchange)(nil).activeConnectionGeneration() != 0 {
		t.Fatal("nil exchange reported an active generation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, usedGeneration, err := exchange.queryQuoteRights(ctx, 1); err == nil ||
		usedGeneration != 0 {
		t.Fatalf("unconnected quote-right query generation=%d err=%v", usedGeneration, err)
	}
}
