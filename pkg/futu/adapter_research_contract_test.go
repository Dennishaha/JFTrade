package futu

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

func TestResearchCatalogOperationsBuildStrictOpenDRequests(t *testing.T) {
	tests := []struct {
		name       string
		protocol   string
		query      broker.FeatureQuery
		params     map[string]any
		wantFields map[string]any
	}{
		{
			name: "concept plate list", protocol: "Qot_GetPlateSet",
			query:      broker.FeatureQuery{Market: "HK"},
			params:     map[string]any{"plateType": "concept"},
			wantFields: map[string]any{"plateSetType": 3},
		},
		{
			name: "exact plate members", protocol: "Qot_GetPlateSecurity",
			query:  broker.FeatureQuery{Market: "HK", InstrumentID: "HK.BK1001"},
			params: map[string]any{}, wantFields: map[string]any{"plate": map[string]any{
				"market": int32(1), "code": "BK1001",
			}},
		},
		{
			name: "fund catalog", protocol: "Qot_GetStaticInfo",
			query:  broker.FeatureQuery{Market: "US"},
			params: map[string]any{"secType": 3}, wantFields: map[string]any{"secType": 4},
		},
		{
			name: "down movers", protocol: "Qot_GetTopMoversRank",
			query:  broker.FeatureQuery{Market: "US"},
			params: map[string]any{"direction": "down"}, wantFields: map[string]any{"sortDir": 1},
		},
		{
			name: "concept heatmap", protocol: "Qot_GetHeatMapData",
			query:  broker.FeatureQuery{Market: "US"},
			params: map[string]any{"plateType": "concept"}, wantFields: map[string]any{"plateType": 1},
		},
		{
			name: "economic calendar market", protocol: "Qot_GetEconomicCalendar",
			query:      broker.FeatureQuery{Market: "SH"},
			params:     map[string]any{"beginDate": "2026-07-23"},
			wantFields: map[string]any{"marketList": []any{int32(21)}},
		},
		{
			name: "institution holding changes", protocol: "Qot_GetInstitutionHoldingChange",
			query:      broker.FeatureQuery{Market: "US"},
			params:     map[string]any{"institutionId": int64(7)},
			wantFields: map[string]any{"institutionId": int32(7)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := injectFeatureInstrument(test.params, test.protocol, test.query.InstrumentID); err != nil {
				t.Fatalf("inject instrument: %v", err)
			}
			if err := injectAdvancedDefaults(test.params, test.protocol, test.query); err != nil {
				t.Fatalf("inject defaults: %v", err)
			}
			for key, want := range test.wantFields {
				if got := test.params[key]; !researchContractEqual(got, want) {
					t.Errorf("%s = %#v, want %#v; params=%#v", key, got, want, test.params)
				}
			}
			if test.params["direction"] != nil {
				t.Fatalf("business direction leaked into OpenD params: %#v", test.params)
			}
			if err := opend.ValidateAdvancedC2S(test.protocol, test.params); err != nil {
				t.Fatalf("strict request validation: %v; params=%#v", err, test.params)
			}
		})
	}
}

func TestResearchCatalogOperationsRejectMissingOrInvalidParameters(t *testing.T) {
	tests := []struct {
		protocol string
		query    broker.FeatureQuery
		params   map[string]any
		want     string
	}{
		{"Qot_GetPlateSet", broker.FeatureQuery{Market: "HK"}, map[string]any{}, "requires plateType"},
		{"Qot_GetPlateSet", broker.FeatureQuery{Market: "HK"}, map[string]any{"plateType": "theme"}, "unsupported plateType"},
		{"Qot_GetPlateSet", broker.FeatureQuery{Market: "HK"}, map[string]any{"plateType": float64(4)}, "unsupported plateType"},
		{"Qot_GetPlateSecurity", broker.FeatureQuery{Market: "HK"}, map[string]any{}, "exact plate instrumentId"},
		{"Qot_GetStaticInfo", broker.FeatureQuery{}, map[string]any{}, "requires market"},
		{"Qot_GetEconomicCalendar", broker.FeatureQuery{}, map[string]any{}, "requires beginDate"},
		{"Qot_GetDividendCalendar", broker.FeatureQuery{Market: "HK"}, map[string]any{}, "requires date"},
		{"Qot_GetInstitutionHoldingChange", broker.FeatureQuery{Market: "US"}, map[string]any{}, "requires a positive integer institutionId"},
		{"Qot_GetInstitutionHoldingChange", broker.FeatureQuery{Market: "US"}, map[string]any{"institutionId": 1.5}, "requires a positive integer institutionId"},
		{"Qot_GetTopMoversRank", broker.FeatureQuery{Market: "US"}, map[string]any{"direction": "flat"}, "unsupported top movers direction"},
	}
	for _, test := range tests {
		t.Run(test.protocol+"/"+test.want, func(t *testing.T) {
			if err := injectFeatureInstrument(test.params, test.protocol, test.query.InstrumentID); err != nil {
				t.Fatalf("inject instrument: %v", err)
			}
			err := injectAdvancedDefaults(test.params, test.protocol, test.query)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResearchProtocolPayloadAddsCanonicalFieldsWithoutDroppingOpenDFields(t *testing.T) {
	t.Run("ranking instrument", func(t *testing.T) {
		result := featureResultFromProtocolPayload(broker.FeatureQuery{
			FeatureID: broker.FeatureResearchRankings,
		}, "Qot_GetTopMoversRank", map[string]any{
			"dataList": []any{map[string]any{
				"security": map[string]any{"market": "QotMarket_US_Security", "code": "aapl"},
				"name":     "Apple", "curPrice": float64(210), "changeRatio": float64(2.5),
				"dividendYieldTTM": float64(0.55),
			}},
			"allCount": float64(17),
		})
		entry := onlyResearchEntry(t, result)
		assertResearchIdentity(t, entry, "US.AAPL", "US", "AAPL", "Apple", "equity")
		if entry["changeRate"] != float64(2.5) || entry["changeRatio"] != float64(2.5) ||
			entry["price"] != float64(210) || entry["dividendYield"] != float64(0.55) {
			t.Fatalf("ranking aliases = %#v", entry)
		}
		if result.Total == nil || *result.Total != 17 || result.Metadata["allCount"] != float64(17) {
			t.Fatalf("ranking envelope = %#v", result)
		}
	})

	t.Run("plate", func(t *testing.T) {
		result := featureResultFromProtocolPayload(broker.FeatureQuery{
			FeatureID: broker.FeatureResearchIndustry,
		}, "Qot_GetPlateSet", map[string]any{
			"plateInfoList": []any{map[string]any{
				"plate": map[string]any{"market": "QotMarket_HK_Security", "code": "BK1001"},
				"name":  "Semiconductors", "plateType": float64(1),
			}},
		})
		entry := onlyResearchEntry(t, result)
		assertResearchIdentity(t, entry, "HK.BK1001", "HK", "BK1001", "Semiconductors", "plate")
		if entry["plateType"] != float64(1) {
			t.Fatalf("original plate field was dropped: %#v", entry)
		}
	})

	t.Run("fund static info", func(t *testing.T) {
		result := featureResultFromProtocolPayload(broker.FeatureQuery{
			FeatureID: broker.FeatureResearchRankings,
		}, "Qot_GetStaticInfo", map[string]any{
			"staticInfoList": []any{map[string]any{"basic": map[string]any{
				"security": map[string]any{"market": "QotMarket_US_Security", "code": "SPY"},
				"name":     "SPDR S&P 500 ETF", "secType": float64(4), "lotSize": float64(1),
			}}},
		})
		entry := onlyResearchEntry(t, result)
		assertResearchIdentity(t, entry, "US.SPY", "US", "SPY", "SPDR S&P 500 ETF", "fund")
		if entry["basic"] == nil {
			t.Fatalf("original static basic was dropped: %#v", entry)
		}
	})

	t.Run("ipo", func(t *testing.T) {
		result := featureResultFromProtocolPayload(broker.FeatureQuery{
			FeatureID: broker.FeatureResearchCalendar,
		}, "Qot_GetIpoList", map[string]any{
			"ipoList": []any{map[string]any{
				"basic": map[string]any{
					"security": map[string]any{"market": "QotMarket_US_Security", "code": "IPOX"},
					"name":     "Example IPO", "listTime": "2026-08-01", "listTimestamp": float64(1785513600),
				},
				"usExData": map[string]any{
					"ipoPriceMin": float64(10), "ipoPriceMax": float64(12), "issueSize": float64(5_000_000),
				},
			}},
		})
		entry := onlyResearchEntry(t, result)
		assertResearchIdentity(t, entry, "US.IPOX", "US", "IPOX", "Example IPO", "equity")
		if entry["issuePriceMin"] != float64(10) || entry["issuePriceMax"] != float64(12) ||
			entry["issueVolume"] != float64(5_000_000) || entry["listingDate"] != "2026-08-01" ||
			entry["eventDate"] != "2026-08-01" || entry["calendarType"] != "ipo" || entry["usExData"] == nil {
			t.Fatalf("IPO canonical fields = %#v", entry)
		}
	})

	t.Run("calendar and institution", func(t *testing.T) {
		economic := featureResultFromProtocolPayload(broker.FeatureQuery{
			FeatureID: broker.FeatureResearchCalendar,
		}, "Qot_GetEconomicCalendar", map[string]any{
			"itemList": []any{map[string]any{
				"title": "CPI", "timestamp": float64(1784764800), "country": "US", "star": float64(3),
				"previous": "2.8%", "consensus": "2.7%", "actual": "2.6%",
			}},
		})
		economicEntry := onlyResearchEntry(t, economic)
		if economicEntry["eventTimestamp"] != float64(1784764800) || economicEntry["eventDate"] != "2026-07-23" ||
			economicEntry["eventTime"] != "2026-07-23T00:00:00Z" || economicEntry["calendarType"] != "economic" ||
			economicEntry["region"] != "US" || economicEntry["importance"] != float64(3) ||
			economicEntry["previousValue"] != "2.8%" || economicEntry["forecastValue"] != "2.7%" ||
			economicEntry["actualValue"] != "2.6%" {
			t.Fatalf("economic canonical fields = %#v", economicEntry)
		}
		for _, timestamp := range []float64{1784764800, 1784764800000} {
			seconds, at, ok := canonicalResearchTimestamp(timestamp)
			if !ok || seconds != float64(1784764800) || at.Format("2006-01-02T15:04:05Z07:00") != "2026-07-23T00:00:00Z" {
				t.Fatalf("canonical timestamp(%v) = %v %v %v", timestamp, seconds, at, ok)
			}
		}

		institutions := featureResultFromProtocolPayload(broker.FeatureQuery{
			FeatureID: broker.FeatureResearchInstitutions,
		}, "Qot_GetInstitutionList", map[string]any{
			"dataList": []any{map[string]any{
				"institutionId": float64(7), "institutionName": "Fund Seven",
				"positionValue": float64(1000), "positionCount": float64(42), "disclosureDate": "2026-06-30",
			}},
			"nextPage": "institution-page-2",
		})
		institution := onlyResearchEntry(t, institutions)
		if institution["name"] != "Fund Seven" || institution["marketValue"] != float64(1000) ||
			institution["holdingCount"] != float64(42) || institution["asOfDate"] != "2026-06-30" {
			t.Fatalf("institution canonical fields = %#v", institution)
		}
		if institutions.NextCursor != "institution-page-2" || institutions.HasMore == nil || !*institutions.HasMore {
			t.Fatalf("institution pagination = %#v", institutions)
		}
	})
}

func TestResearchCatalogLocalPaginationKeepsOpenDRequestUnchanged(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		listKey  string
	}{
		{name: "plate list", protocol: "Qot_GetPlateSet", listKey: "plateInfoList"},
		{name: "plate members", protocol: "Qot_GetPlateSecurity", listKey: "staticInfoList"},
		{name: "fund catalog", protocol: "Qot_GetStaticInfo", listKey: "staticInfoList"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := map[string]any{}
			injectAdvancedCursor(params, test.protocol, "local:2")
			injectAdvancedPageSize(params, test.protocol, 2)
			if len(params) != 0 {
				t.Fatalf("local pagination leaked into %s request: %#v", test.protocol, params)
			}

			rows := make([]any, 5)
			for index := range rows {
				rows[index] = map[string]any{"row": float64(index)}
			}
			first := featureResultFromProtocolPayload(
				broker.FeatureQuery{FeatureID: broker.FeatureResearchRankings},
				test.protocol,
				map[string]any{test.listKey: rows},
			)
			if err := applyResearchLocalPagination(first, broker.FeatureQuery{PageSize: 2}, test.protocol); err != nil {
				t.Fatalf("first page: %v", err)
			}
			assertResearchLocalPage(t, first, 0, 2, 5, "local:2", true)

			second := featureResultFromProtocolPayload(
				broker.FeatureQuery{FeatureID: broker.FeatureResearchRankings},
				test.protocol,
				map[string]any{test.listKey: rows},
			)
			if err := applyResearchLocalPagination(
				second,
				broker.FeatureQuery{PageSize: 2, Cursor: first.NextCursor},
				test.protocol,
			); err != nil {
				t.Fatalf("second page: %v", err)
			}
			assertResearchLocalPage(t, second, 2, 2, 5, "local:4", true)

			last := featureResultFromProtocolPayload(
				broker.FeatureQuery{FeatureID: broker.FeatureResearchRankings},
				test.protocol,
				map[string]any{test.listKey: rows},
			)
			if err := applyResearchLocalPagination(
				last,
				broker.FeatureQuery{PageSize: 2, Cursor: second.NextCursor},
				test.protocol,
			); err != nil {
				t.Fatalf("last page: %v", err)
			}
			assertResearchLocalPage(t, last, 4, 1, 5, "", false)
		})
	}

	result := featureResultFromProtocolPayload(
		broker.FeatureQuery{FeatureID: broker.FeatureResearchRankings},
		"Qot_GetStaticInfo",
		map[string]any{"staticInfoList": []any{}},
	)
	if err := applyResearchLocalPagination(
		result,
		broker.FeatureQuery{PageSize: 2, Cursor: "server-cursor"},
		"Qot_GetStaticInfo",
	); err == nil || !strings.Contains(err.Error(), "invalid local research cursor") {
		t.Fatalf("invalid cursor error = %v", err)
	}
}

func TestEconomicCalendarPaginationHonorsExplicitHasMoreAndEmptyRows(t *testing.T) {
	result := featureResultFromProtocolPayload(
		broker.FeatureQuery{FeatureID: broker.FeatureResearchCalendar},
		"Qot_GetEconomicCalendar",
		map[string]any{
			"nextPage": "stale-next-page",
			"hasMore":  false,
		},
	)
	if len(result.Entries) != 0 {
		t.Fatalf("pagination metadata became an economic event: %#v", result.Entries)
	}
	if result.HasMore == nil || *result.HasMore || result.NextCursor != "" {
		t.Fatalf("economic pagination = hasMore %#v nextCursor %q", result.HasMore, result.NextCursor)
	}
	if result.Total == nil || *result.Total != 0 ||
		result.Metadata["nextPage"] != "stale-next-page" ||
		result.Metadata["hasMore"] != false {
		t.Fatalf("economic envelope = %#v", result)
	}
}

func assertResearchLocalPage(
	t *testing.T,
	result *broker.FeatureResult,
	firstRow, count, total int,
	nextCursor string,
	hasMore bool,
) {
	t.Helper()
	if len(result.Entries) != count ||
		result.Entries[0]["row"] != float64(firstRow) ||
		result.Total == nil || *result.Total != total ||
		result.NextCursor != nextCursor ||
		result.HasMore == nil || *result.HasMore != hasMore {
		t.Fatalf("local page = %#v", result)
	}
}

func researchContractEqual(got, want any) bool {
	gotMap, gotOK := got.(map[string]any)
	wantMap, wantOK := want.(map[string]any)
	if gotOK && wantOK {
		for key, value := range wantMap {
			if !researchContractEqual(gotMap[key], value) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(got, want)
}

func onlyResearchEntry(t *testing.T, result *broker.FeatureResult) map[string]any {
	t.Helper()
	if result == nil || len(result.Entries) != 1 {
		t.Fatalf("result entries = %#v", result)
	}
	return result.Entries[0]
}

func assertResearchIdentity(
	t *testing.T,
	entry map[string]any,
	instrumentID, market, symbol, name, productClass string,
) {
	t.Helper()
	if entry["instrumentId"] != instrumentID || entry["market"] != market ||
		entry["symbol"] != symbol || entry["name"] != name || entry["productClass"] != productClass {
		t.Fatalf("canonical identity = %#v", entry)
	}
}
