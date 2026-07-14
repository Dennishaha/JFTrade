package futu

import (
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestKLineHistoryAndSubscriptionBoundaryPaths(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	client, err := exchange.ensureClient(t.Context())
	if err != nil {
		t.Fatalf("ensureClient() error = %v", err)
	}
	security, canonical, err := futuSecurityFromSymbol("HK.00700")
	if err != nil {
		t.Fatalf("futuSecurityFromSymbol() error = %v", err)
	}
	if _, err := exchange.queryCurrentKLines(t.Context(), security, canonical, types.Interval("invalid"), qotcommonpb.KLType_KLType_5Min); err == nil {
		t.Fatal("queryCurrentKLines(invalid interval) error = nil")
	}

	if err := subscribeKLine(t.Context(), client, klineSubscriptionRequest{
		canonical:    canonical,
		security:     security,
		subType:      qotcommonpb.SubType_SubType_KL_5Min,
		extendedTime: true,
		session:      commonpb.Session_Session_ALL,
	}); err != nil {
		t.Fatalf("subscribeKLine(extended) error = %v", err)
	}

	start := time.Date(2026, time.June, 22, 13, 30, 0, 0, time.UTC)
	blank := testHistoryKLine(start, 100)
	blank.IsBlank = new(true)
	server.setHistoryPages([][]*qotcommonpb.KLine{{blank, testHistoryKLine(start, 101)}, {testHistoryKLine(start.Add(5*time.Minute), 102)}})
	if _, err := exchange.queryHistoricalKLinesForPlan(
		t.Context(), security, canonical, types.Interval5m, qotcommonpb.KLType_KLType_5Min,
		start, start.Add(time.Hour), qotcommonpb.RehabType_RehabType_None,
		1, 1, historicalKLineRequestPlan{},
	); err == nil || !strings.Contains(err.Error(), "pagination exceeded") {
		t.Fatalf("single-page historical request error = %v", err)
	}

	server.setHistorySessionError(0, 1, "history unavailable")
	if _, err := exchange.QueryKLines(t.Context(), "HK.00700", types.Interval5m, types.KLineQueryOptions{}); err == nil {
		t.Fatal("QueryKLines(OpenD error) error = nil")
	}
}
