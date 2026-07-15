package futu

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	qotgetsubinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetsubinfo"
	qotsubpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotsub"
)

func TestExchangeSubscriptionMethodsArePairedExactAndIdempotent(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()

	if err := exchange.SubscribeBasicQuote(t.Context(), "HK.00700", true); err != nil {
		t.Fatalf("SubscribeBasicQuote: %v", err)
	}
	if err := exchange.SubscribeBasicQuote(t.Context(), "hk.00700", true); err != nil {
		t.Fatalf("duplicate SubscribeBasicQuote: %v", err)
	}
	if err := exchange.UnsubscribeBasicQuote(t.Context(), "HK.00700"); err != nil {
		t.Fatalf("UnsubscribeBasicQuote: %v", err)
	}
	if err := exchange.UnsubscribeBasicQuote(t.Context(), "HK.00700"); err != nil {
		t.Fatalf("duplicate UnsubscribeBasicQuote: %v", err)
	}

	if err := exchange.SubscribeKLine(t.Context(), "US.AAPL", types.Interval1m); err != nil {
		t.Fatalf("SubscribeKLine: %v", err)
	}
	if err := exchange.SubscribeKLine(t.Context(), "us.aapl", types.Interval1m); err != nil {
		t.Fatalf("duplicate SubscribeKLine: %v", err)
	}
	if err := exchange.UnsubscribeKLine(t.Context(), "US.AAPL", types.Interval1m); err != nil {
		t.Fatalf("UnsubscribeKLine: %v", err)
	}
	if err := exchange.UnsubscribeKLine(t.Context(), "US.AAPL", types.Interval1m); err != nil {
		t.Fatalf("duplicate UnsubscribeKLine: %v", err)
	}

	requests := server.capturedQotSubRequests()
	if len(requests) != 4 {
		t.Fatalf("Qot_Sub requests = %d, want four paired calls", len(requests))
	}
	assertSubscriptionPair(t, requests[0], requests[1], qotcommonpb.SubType_SubType_Basic, false)
	assertSubscriptionPair(t, requests[2], requests[3], qotcommonpb.SubType_SubType_KL_1Min, true)
	if requests[0].IsRegOrUnRegPush == nil || !requests[0].GetIsRegOrUnRegPush() || requests[1].IsRegOrUnRegPush == nil || requests[1].GetIsRegOrUnRegPush() {
		t.Fatalf("Basic push register/unregister flags = %#v / %#v", requests[0], requests[1])
	}
	if requests[2].GetSession() != int32(commonpb.Session_Session_ALL) || requests[3].GetSession() != int32(commonpb.Session_Session_ALL) {
		t.Fatalf("US K-line sessions differ: %#v / %#v", requests[2], requests[3])
	}
}

func assertSubscriptionPair(t *testing.T, subscribe, unsubscribe *qotsubpb.C2S, subtype qotcommonpb.SubType, extended bool) {
	t.Helper()
	if !subscribe.GetIsSubOrUnSub() || unsubscribe.GetIsSubOrUnSub() {
		t.Fatalf("subscribe flags = %v/%v", subscribe.GetIsSubOrUnSub(), unsubscribe.GetIsSubOrUnSub())
	}
	if !reflect.DeepEqual(subscribe.GetSecurityList(), unsubscribe.GetSecurityList()) ||
		!reflect.DeepEqual(subscribe.GetSubTypeList(), unsubscribe.GetSubTypeList()) ||
		len(subscribe.GetSubTypeList()) != 1 || subscribe.GetSubTypeList()[0] != int32(subtype) {
		t.Fatalf("subscription pair parameters differ: %#v / %#v", subscribe, unsubscribe)
	}
	if subscribe.GetExtendedTime() != extended || unsubscribe.GetExtendedTime() != extended {
		t.Fatalf("extended flags = %v/%v, want %v", subscribe.GetExtendedTime(), unsubscribe.GetExtendedTime(), extended)
	}
}

func TestExchangeSubscriptionCacheUpdatesOnlyAfterOpenDConfirmation(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()
	failure := &qotsubpb.Response{RetType: new(int32(1)), ErrCode: new(int32(429)), RetMsg: new("quota exceeded")}
	success := &qotsubpb.Response{RetType: new(int32(0))}

	server.setQotSubResponses(failure, success, failure, success)
	if err := exchange.SubscribeKLine(t.Context(), "HK.00700", types.Interval5m); err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("failed SubscribeKLine error = %v", err)
	}
	if err := exchange.SubscribeKLine(t.Context(), "HK.00700", types.Interval5m); err != nil {
		t.Fatalf("retry SubscribeKLine: %v", err)
	}
	if err := exchange.UnsubscribeKLine(t.Context(), "HK.00700", types.Interval5m); err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("failed UnsubscribeKLine error = %v", err)
	}
	if err := exchange.UnsubscribeKLine(t.Context(), "HK.00700", types.Interval5m); err != nil {
		t.Fatalf("retry UnsubscribeKLine: %v", err)
	}
	if server.subCallCount() != 4 {
		t.Fatalf("cache incorrectly suppressed retry, Qot_Sub calls = %d", server.subCallCount())
	}

	server.setQotSubResponses(failure, success, failure, success)
	if err := exchange.SubscribeBasicQuote(t.Context(), "US.NVDA", false); err == nil {
		t.Fatal("failed SubscribeBasicQuote error = nil")
	}
	if err := exchange.SubscribeBasicQuote(t.Context(), "US.NVDA", false); err != nil {
		t.Fatalf("retry SubscribeBasicQuote: %v", err)
	}
	if err := exchange.UnsubscribeBasicQuote(t.Context(), "US.NVDA"); err == nil {
		t.Fatal("failed UnsubscribeBasicQuote error = nil")
	}
	if err := exchange.UnsubscribeBasicQuote(t.Context(), "US.NVDA"); err != nil {
		t.Fatalf("retry UnsubscribeBasicQuote: %v", err)
	}
}

func TestQueryKLinesWithoutLeaseReturnsHistoryWithoutRealtimeSubscription(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	labelAt := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	server.setHistoryPages([][]*qotcommonpb.KLine{{testHistoryKLine(labelAt, 100)}})
	exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()

	klines, err := exchange.QueryKLines(t.Context(), "HK.00700", types.Interval1m, types.KLineQueryOptions{
		Limit: 2, StartTime: new(labelAt.Add(-time.Hour)), EndTime: new(labelAt.Add(time.Hour)),
	})
	if err != nil || len(klines) != 1 {
		t.Fatalf("QueryKLines historical-only = %#v, %v", klines, err)
	}
	if server.subCallCount() != 0 || server.currentKLCallCount() != 0 {
		t.Fatalf("historical query leaked realtime calls: sub=%d current=%d", server.subCallCount(), server.currentKLCallCount())
	}
}

func TestExchangeQuerySubscriptionQuotaSeparatesOwnAndOtherConnections(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	server.setSubInfoResponse(&qotgetsubinfopb.Response{RetType: new(int32(0)), S2C: &qotgetsubinfopb.S2C{
		TotalUsedQuota: new(int32(12)), RemainQuota: new(int32(88)),
		ConnSubInfoList: []*qotcommonpb.ConnSubInfo{
			{UsedQuota: new(int32(3)), IsOwnConnData: new(true)},
			{UsedQuota: new(int32(2)), IsOwnConnData: new(true)},
			{UsedQuota: new(int32(7)), IsOwnConnData: new(false)},
		},
	}})
	exchange := NewExchangeWithConfig(opend.Config{Addr: server.addr, RequestTimeout: 2 * time.Second})
	defer func() { jftradeCheckTestError(t, exchange.Close()) }()
	quota, err := exchange.QuerySubscriptionQuota(t.Context())
	if err != nil || quota != (SubscriptionQuota{TotalUsed: 12, Remaining: 88, OwnUsed: 5}) {
		t.Fatalf("QuerySubscriptionQuota = %#v, %v", quota, err)
	}

	server.setSubInfoResponse(&qotgetsubinfopb.Response{RetType: new(int32(1)), RetMsg: new("quota unavailable")})
	if _, err := exchange.QuerySubscriptionQuota(t.Context()); err == nil || !strings.Contains(err.Error(), "quota unavailable") {
		t.Fatalf("QuerySubscriptionQuota error = %v", err)
	}
}

func TestSubscriptionMethodsValidateSymbolsAndIntervals(t *testing.T) {
	exchange := NewExchange("127.0.0.1:1")
	if err := exchange.SubscribeBasicQuote(t.Context(), "BAD", true); err == nil {
		t.Fatal("SubscribeBasicQuote invalid symbol error = nil")
	}
	if err := exchange.UnsubscribeBasicQuote(t.Context(), "BAD"); err == nil {
		t.Fatal("UnsubscribeBasicQuote invalid symbol error = nil")
	}
	if err := exchange.SubscribeKLine(t.Context(), "HK.00700", types.Interval("bad")); err == nil {
		t.Fatal("SubscribeKLine invalid interval error = nil")
	}
	if err := exchange.UnsubscribeKLine(t.Context(), "HK.00700", types.Interval("bad")); err == nil {
		t.Fatal("UnsubscribeKLine invalid interval error = nil")
	}
}
