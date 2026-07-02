package servercore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
)

func TestOpenAPIRouteDocumentationHelpersAreCallable(t *testing.T) {
	for name, fn := range map[string]func(){
		"data migration":        documentDataMigrationRoutes,
		"assistant catalog":     documentAssistantCatalogRoutes,
		"assistant task memory": documentAssistantTaskMemoryRoutes,
		"assistant session run": documentAssistantSessionRunRoutes,
		"assistant chat":        documentAssistantChatApprovalSkillRoutes,
		"assistant optimize":    documentAssistantOptimizationRoutes,
		"assistant workflow":    documentAssistantWorkflowRoutes,
		"backtest sync":         documentBacktestSyncTaskRoutes,
		"market utility":        documentMarketUtilityRoutes,
		"plugins":               documentPluginRoutes,
		"portfolio":             documentPortfolioRoutes,
		"broker funds":          documentBrokerFundsRoute,
		"broker positions":      documentBrokerPositionsRoute,
		"broker orders":         documentBrokerOrdersRoute,
		"broker fills":          documentBrokerFillsRoute,
		"broker cash flows":     documentBrokerCashFlowsRoute,
		"broker order fees":     documentBrokerOrderFeesRoute,
		"broker margin ratios":  documentBrokerMarginRatiosRoute,
		"broker max trade qty":  documentBrokerMaxTradeQuantityRoute,
		"broker quote":          documentBrokerQuoteRoute,
		"broker klines":         documentBrokerKLinesRoute,
		"broker securities":     documentBrokerSecuritiesRoute,
		"execution orders":      documentExecutionOrdersRoute,
		"execution place":       documentExecutionPlaceRoute,
		"execution cancel":      documentExecutionCancelRoute,
		"execution events":      documentExecutionEventsRoute,
		"system operational":    documentSystemOperationalRoutes,
		"execution preview":     documentExecutionPreviewRoute,
	} {
		t.Run(name, func(t *testing.T) {
			fn()
		})
	}
}

func TestRuntimeDefaultsAndLayoutBoundaries(t *testing.T) {
	development := ResolveLaunchDefaults(false)
	if development.APIBind != defaultDevelopmentAPIBind || development.GUIBind != "" {
		t.Fatalf("development defaults = %#v", development)
	}
	release := ResolveLaunchDefaults(true)
	if release.APIBind != defaultReleaseAPIBind || release.GUIBind != defaultReleaseGUIBind {
		t.Fatalf("release defaults = %#v", release)
	}
	if got := APIBaseURLForBind(":3000"); got != "http://127.0.0.1:3000" {
		t.Fatalf("APIBaseURLForBind(:3000) = %q", got)
	}
	if got := PortFromBind("127.0.0.1:5173", 3000); got != 5173 {
		t.Fatalf("PortFromBind = %d, want 5173", got)
	}
	if got := PortFromBind("invalid", 3000); got != 3000 {
		t.Fatalf("PortFromBind invalid = %d, want default", got)
	}

	root := t.TempDir()
	settingsPath := filepath.Join(root, "runtime", "settings.json")
	backtestPath := filepath.Join(root, "data", "backtest.db")
	if err := EnsureRuntimeLayout(settingsPath, backtestPath); err != nil {
		t.Fatalf("EnsureRuntimeLayout: %v", err)
	}
	for _, dir := range []string{filepath.Dir(settingsPath), filepath.Dir(backtestPath)} {
		if !directoryExists(dir) {
			t.Fatalf("runtime directory %s was not created", dir)
		}
	}
}

func TestWorkflowAndMarketRuntimeBoundaryHelpers(t *testing.T) {
	market, symbol, ok := splitWorkflowInstrumentID(" us.aapl ")
	if !ok || market != "US" || symbol != "AAPL" {
		t.Fatalf("splitWorkflowInstrumentID = %q/%q/%v", market, symbol, ok)
	}
	for _, raw := range []string{"", "US", ".AAPL", "US."} {
		if market, symbol, ok := splitWorkflowInstrumentID(raw); ok {
			t.Fatalf("splitWorkflowInstrumentID(%q) = %q/%q/true, want invalid", raw, market, symbol)
		}
	}

	if got := strategyRuntimeMarketFromSymbol("hk.00700", "US"); got != "HK" {
		t.Fatalf("strategyRuntimeMarketFromSymbol dotted = %q", got)
	}
	if got := strategyRuntimeMarketFromSymbol("us:aapl", "HK"); got != "US" {
		t.Fatalf("strategyRuntimeMarketFromSymbol colon = %q", got)
	}
	if got := strategyRuntimeMarketFromSymbol("AAPL", " us "); got != "US" {
		t.Fatalf("strategyRuntimeMarketFromSymbol fallback = %q", got)
	}

	if code, label := strategyRuntimeStartError(errors.New("missing provider")); code != 400 || label != "BAD_REQUEST" {
		t.Fatalf("missing provider start error = %d/%s", code, label)
	}
	if code, label := strategyRuntimeStartError(errors.New("broker gateway down")); code != 502 || label != "STRATEGY_RUNTIME_START_FAILED" {
		t.Fatalf("gateway start error = %d/%s", code, label)
	}
}

func TestTimeStatusAndDefaultScriptBoundaries(t *testing.T) {
	parsed := httpTime("2026-06-20T13:30:00.123456789+08:00")
	if parsed.IsZero() || parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != "2026-06-20T05:30:00.123456789Z" {
		t.Fatalf("httpTime parsed = %s", parsed.Format(time.RFC3339Nano))
	}
	if !httpTime("not-time").IsZero() {
		t.Fatalf("invalid httpTime should be zero")
	}

	cutoff := time.Date(2026, time.June, 20, 0, 0, 0, 0, time.UTC)
	if !executionTimestampBefore("2026-06-19T23:59:59Z", cutoff) {
		t.Fatalf("timestamp before cutoff not detected")
	}
	if executionTimestampBefore("", cutoff) || executionTimestampBefore("bad", cutoff) || executionTimestampBefore("2026-06-20T00:00:00Z", cutoff) {
		t.Fatalf("executionTimestampBefore accepted empty/bad/equal timestamp")
	}

	if boolValue(nil) {
		t.Fatalf("nil boolValue = true")
	}
	if value := true; !boolValue(&value) {
		t.Fatalf("true boolValue = false")
	}
	if got := programStatusString(nil); got != "Unavailable" {
		t.Fatalf("nil programStatusString = %q", got)
	}
	statusType := commonpb.ProgramStatusType_ProgramStatusType_NeedPhoneVerifyCode
	status := &commonpb.ProgramStatus{Type: &statusType, StrExtDesc: new("scan QR code")}
	if got := programStatusString(status); !strings.Contains(got, "NeedPhoneVerifyCode: scan QR code") {
		t.Fatalf("programStatusString = %q", got)
	}

	script := defaultStrategyDesignScript(`Quote "Name"`, "pine")
	if !strings.Contains(script, `strategy("Quote \"Name\""`) || !strings.Contains(script, "ta.crossover") {
		t.Fatalf("default strategy script = %q", script)
	}
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

//go:fix inline
func servercoreBoundaryPtr[T any](value T) *T {
	return new(value)
}
