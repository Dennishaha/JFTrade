package futu

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	getuserinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/getuserinfo"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestFutuCapabilityNotificationsAndRuntimeAggregation(t *testing.T) {
	adapter := NewBrokerAdapter(nil).(*futuAdapter)
	adapter.captureCapabilityNotification(nil)
	adapter.captureCapabilityNotification(&notifypb.Response{})
	adapter.captureCapabilityNotification(&notifypb.Response{S2C: &notifypb.S2C{
		Type: new(int32(notifypb.NotifyType_NotifyType_ConnStatus)),
	}})
	adapter.captureCapabilityNotification(&notifypb.Response{S2C: &notifypb.S2C{
		Type: new(int32(notifypb.NotifyType_NotifyType_QotRight)),
	}})

	adapter.captureCapabilityNotification(&notifypb.Response{S2C: &notifypb.S2C{
		Type: new(int32(notifypb.NotifyType_NotifyType_ConnStatus)),
		ConnectStatus: &notifypb.ConnectStatus{
			QotLogined: new(false), TrdLogined: new(true),
		},
	}})
	rights := completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level1)
	adapter.captureCapabilityNotification(&notifypb.Response{S2C: &notifypb.S2C{
		Type:     new(int32(notifypb.NotifyType_NotifyType_QotRight)),
		QotRight: rights,
	}})
	rights.UsQotRight = new(int32(qotcommonpb.QotRight_QotRight_No))
	if adapter.lastQuoteRights.value.GetUsQotRight() !=
		int32(qotcommonpb.QotRight_QotRight_Level1) {
		t.Fatal("quote entitlement notification was not cloned")
	}

	request := broker.CapabilityEvaluationRequest{
		DeclaredCapability: broker.FeatureCapability{RequiresConnection: true},
	}
	evaluation, err := adapter.EvaluateCapability(t.Context(), request)
	if err != nil || evaluation.State != broker.CapabilityUnavailable ||
		evaluation.Code != "OPEND_UNCONFIGURED" {
		t.Fatalf("unconfigured evaluation = %#v, %v", evaluation, err)
	}
	evaluation, err = (*futuAdapter)(nil).EvaluateCapability(t.Context(), request)
	if err != nil || evaluation.Code != "OPEND_UNCONFIGURED" {
		t.Fatalf("nil evaluation = %#v, %v", evaluation, err)
	}

	at := time.Now().UTC()
	available := capabilityAvailable(at, "OK", "available")
	degraded := capabilityDegraded(at, "SLOW", "degraded")
	unavailable := capabilityUnavailable(at, "NO", "unavailable")
	notRequired := capabilityNotRequired(at)
	if available.State != broker.CapabilityAvailable ||
		degraded.State != broker.CapabilityDegraded ||
		unavailable.State != broker.CapabilityUnavailable ||
		notRequired.Code != "NOT_REQUIRED" {
		t.Fatal("capability check constructors changed")
	}
	aggregated := aggregateCapabilityEvaluation(broker.CapabilityEvaluation{
		Connection: degraded, Account: capabilityDegraded(at, "OTHER", "other"),
		QuoteRight: available,
	})
	if aggregated.State != broker.CapabilityDegraded || aggregated.Code != "SLOW" {
		t.Fatalf("degraded aggregation = %#v", aggregated)
	}
	aggregated = aggregateCapabilityEvaluation(broker.CapabilityEvaluation{
		Connection: available, Account: unavailable, QuoteRight: degraded,
	})
	if aggregated.State != broker.CapabilityUnavailable || aggregated.Code != "NO" {
		t.Fatalf("unavailable aggregation = %#v", aggregated)
	}
}

func TestFutuCapabilityConnectionAccountAndEntitlementDecisions(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	account := testSimulateHKCashAccount()
	account.TrdMarketAuthList = []int32{int32(trdcommonpb.TrdMarket_TrdMarket_US)}
	account.SecurityFirm = new(int32(trdcommonpb.SecurityFirm_SecurityFirm_FutuInc))
	server.setAccounts([]*trdcommonpb.TrdAcc{account})
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	ctx := t.Context()
	if err := adapter.exchange.EnsureSystemNotifications(ctx); err != nil {
		t.Fatalf("EnsureSystemNotifications() error = %v", err)
	}

	adapter.lastConnectStatus = &futuConnectCapabilityStatus{
		quoteLoggedIn: false, tradeLoggedIn: true, observedAt: time.Now().UTC(),
		generation: adapter.exchange.activeConnectionGeneration(),
	}
	evaluation, err := adapter.EvaluateCapability(ctx, broker.CapabilityEvaluationRequest{
		DeclaredCapability: broker.FeatureCapability{
			Access: broker.FeatureAccessRead, RequiresConnection: true,
		},
	})
	if err != nil || evaluation.Code != "OPEND_NOT_LOGGED_IN" {
		t.Fatalf("quote login evaluation = %#v, %v", evaluation, err)
	}
	evaluation, err = adapter.EvaluateCapability(ctx, broker.CapabilityEvaluationRequest{
		DeclaredCapability: broker.FeatureCapability{
			Access: broker.FeatureAccessTrade, RequiresConnection: true,
		},
	})
	if err != nil || evaluation.State != broker.CapabilityAvailable {
		t.Fatalf("trade login evaluation = %#v, %v", evaluation, err)
	}

	adapter.lastConnectStatus = nil
	adapter.lastQuoteRights = nil
	evaluation, err = adapter.EvaluateCapability(ctx, broker.CapabilityEvaluationRequest{
		Market: "US",
		DeclaredCapability: broker.FeatureCapability{
			Access: broker.FeatureAccessRead, RequiresConnection: true,
			RequiresAccount: true, RequiresQuoteRight: true,
		},
	})
	if err != nil || evaluation.State != broker.CapabilityDegraded ||
		evaluation.Account.Code != "ACCOUNT_CONTEXT_REQUIRED" ||
		evaluation.QuoteRight.Code != "QUOTE_RIGHT_AVAILABLE" {
		t.Fatalf("partial runtime evaluation = %#v, %v", evaluation, err)
	}
	if server.userInfoCalls.Load() != 1 {
		t.Fatalf("GetUserInfo calls = %d, want 1", server.userInfoCalls.Load())
	}

	request := broker.CapabilityEvaluationRequest{
		FeatureID: broker.FeaturePredictionDiscover,
		AccountID: "1001", TradingEnvironment: "SIMULATE", Market: "US",
		MarketSegment: broker.MarketSegmentPrediction,
		ProductClass:  broker.ProductClassEventContract,
		DeclaredCapability: broker.FeatureCapability{
			Access: broker.FeatureAccessTrade, RequiresAccount: true,
		},
	}
	evaluation, err = adapter.EvaluateCapability(ctx, request)
	if err != nil || evaluation.Account.Code != "ACCOUNT_ELIGIBLE" {
		t.Fatalf("eligible prediction account = %#v, %v", evaluation, err)
	}
	if cached, err := adapter.capabilityAccountSnapshot(ctx, time.Now().UTC()); err != nil ||
		len(cached) != 1 {
		t.Fatalf("cached account snapshot = %#v, %v", cached, err)
	}

	request.TradingEnvironment = "REAL"
	check := adapter.evaluateAccountCapability(ctx, request, time.Now().UTC())
	if check.Code != "ACCOUNT_NOT_FOUND" {
		t.Fatalf("environment mismatch = %#v", check)
	}
	request.TradingEnvironment = "SIMULATE"
	request.Market = "HK"
	check = adapter.evaluateAccountCapability(ctx, request, time.Now().UTC())
	if check.Code != "ACCOUNT_NOT_FOUND" {
		t.Fatalf("market mismatch = %#v", check)
	}
	request.Market = "US"
	request.AccountID = "missing"
	check = adapter.evaluateAccountCapability(ctx, request, time.Now().UTC())
	if check.Code != "ACCOUNT_NOT_FOUND" {
		t.Fatalf("missing account = %#v", check)
	}

	nonUSFirm := "FUTUSECURITIES"
	adapter.capabilityAccounts = []broker.Account{{
		ID: "ineligible", TradingEnvironment: "SIMULATE",
		MarketAuthorities: []string{"US"}, SecurityFirm: &nonUSFirm,
	}}
	adapter.capabilityAccountsExpiresAt = time.Now().Add(time.Minute)
	request.AccountID = "ineligible"
	check = adapter.evaluateAccountCapability(ctx, request, time.Now().UTC())
	if check.Code != "PREDICTION_ACCOUNT_INELIGIBLE" {
		t.Fatalf("ineligible prediction account = %#v", check)
	}

	dead := NewBrokerAdapter(NewExchangeWithConfig(opend.Config{
		Addr: "127.0.0.1:1", RequestTimeout: 30 * time.Millisecond,
	})).(*futuAdapter)
	deadRequest := request
	deadRequest.AccountID = "1001"
	check = dead.evaluateAccountCapability(context.Background(), deadRequest, time.Now().UTC())
	if check.Code != "ACCOUNT_DISCOVERY_FAILED" {
		t.Fatalf("account discovery failure = %#v", check)
	}
	evaluation, err = dead.EvaluateCapability(context.Background(), broker.CapabilityEvaluationRequest{
		DeclaredCapability: broker.FeatureCapability{RequiresConnection: true},
	})
	if err != nil || evaluation.Code != "OPEND_CONNECTION_UNAVAILABLE" {
		t.Fatalf("connection failure = %#v, %v", evaluation, err)
	}
}

func TestFutuCapabilityQuoteRightProductsAndStates(t *testing.T) {
	rights := completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level2)
	requests := []broker.CapabilityEvaluationRequest{
		{FeatureID: broker.FeaturePredictionSnapshot, Market: "US"},
		{MarketSegment: broker.MarketSegmentPrediction, Market: "US"},
		{ProductClass: broker.ProductClassEventContract, Market: "US"},
		{FeatureID: broker.FeatureOptionAnalysis, Market: "HK"},
		{ProductClass: broker.ProductClassOption, Market: "US"},
		{FeatureID: broker.FeatureFutures, Market: "HK"},
		{ProductClass: broker.ProductClassFuture, Market: "US"},
		{Market: "HK"}, {Market: "SH"}, {Market: "SZ"}, {Market: "US"},
	}
	for _, request := range requests {
		if got := quoteRightForCapability(rights, request); got !=
			int32(qotcommonpb.QotRight_QotRight_Level2) {
			t.Errorf("quoteRightForCapability(%#v) = %d", request, got)
		}
	}
	if maximumQuoteRight(
		int32(qotcommonpb.QotRight_QotRight_No),
		int32(qotcommonpb.QotRight_QotRight_Level1),
		int32(qotcommonpb.QotRight_QotRight_Level3),
	) != int32(qotcommonpb.QotRight_QotRight_Level3) {
		t.Fatal("maximum quote right did not select the highest entitlement")
	}
	if maximumQuoteRight(0, int32(qotcommonpb.QotRight_QotRight_No)) !=
		int32(qotcommonpb.QotRight_QotRight_No) {
		t.Fatal("maximum quote right did not preserve explicit denial")
	}
	if maximumQuoteRight() != 0 {
		t.Fatal("empty quote-right maximum changed")
	}

	adapter := NewBrokerAdapter(nil).(*futuAdapter)
	request := broker.CapabilityEvaluationRequest{Market: "US"}
	cases := []struct {
		right qotcommonpb.QotRight
		code  string
	}{
		{qotcommonpb.QotRight_QotRight_Level1, "QUOTE_RIGHT_AVAILABLE"},
		{qotcommonpb.QotRight_QotRight_Bmp, "QUOTE_RIGHT_POLLING_ONLY"},
		{qotcommonpb.QotRight_QotRight_No, "QUOTE_RIGHT_DENIED"},
		{qotcommonpb.QotRight_QotRight_Unknow, "QUOTE_RIGHT_UNKNOWN"},
	}
	for _, test := range cases {
		value := completeCapabilityRights(test.right)
		adapter.lastQuoteRights = &futuQuoteCapabilityRights{
			value: value, observedAt: time.Now().UTC(),
		}
		if check := adapter.evaluateQuoteCapability(request, time.Now().UTC()); check.Code != test.code {
			t.Errorf("right %s = %#v", test.right, check)
		}
	}
	adapter.lastQuoteRights = &futuQuoteCapabilityRights{}
	if check := adapter.evaluateQuoteCapability(request, time.Now().UTC()); check.Code != "QUOTE_RIGHT_UNVERIFIED" {
		t.Fatalf("nil rights = %#v", check)
	}
	if !containsFoldValue([]string{" hk ", "US"}, "HK") ||
		containsFoldValue([]string{"HK"}, "US") {
		t.Fatal("case-insensitive capability membership changed")
	}
}

func TestFutuCapabilityLoadsQuoteRightsOncePerConnection(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	level2 := int32(qotcommonpb.QotRight_QotRight_Level2)
	server.setUserInfoResponse(&getuserinfopb.Response{
		RetType: new(int32(0)),
		S2C: &getuserinfopb.S2C{
			HkQotRight: &level2, UsQotRight: &level2,
			ShQotRight: &level2, SzQotRight: &level2,
		},
	})
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	request := broker.CapabilityEvaluationRequest{
		Market: "HK",
		DeclaredCapability: broker.FeatureCapability{
			Access: broker.FeatureAccessRead, RequiresConnection: true,
			RequiresQuoteRight: true,
		},
	}

	const callers = 20
	var wait sync.WaitGroup
	wait.Add(callers)
	errorsSeen := make(chan error, callers)
	for range callers {
		go func() {
			defer wait.Done()
			evaluation, err := adapter.EvaluateCapability(t.Context(), request)
			if err != nil {
				errorsSeen <- err
				return
			}
			if evaluation.State != broker.CapabilityAvailable ||
				evaluation.QuoteRight.Code != "QUOTE_RIGHT_AVAILABLE" {
				errorsSeen <- errors.New("quote right was not available")
			}
		}()
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Error(err)
	}
	if calls := server.userInfoCalls.Load(); calls != 1 {
		t.Fatalf("GetUserInfo calls = %d, want 1", calls)
	}
}

func TestFutuCapabilityCachesQuoteRightFailuresAndRefreshesAfterReconnect(t *testing.T) {
	server := startQuoteOpenDServer(t)
	defer server.stop()
	server.setUserInfoResponse(&getuserinfopb.Response{
		RetType: new(int32(-1)),
		RetMsg:  new("permission query failed"),
	})
	adapter := newTestBrokerAdapter(t, server).(*futuAdapter)
	request := broker.CapabilityEvaluationRequest{
		Market: "US",
		DeclaredCapability: broker.FeatureCapability{
			Access: broker.FeatureAccessRead, RequiresConnection: true,
			RequiresQuoteRight: true,
		},
	}

	for range 2 {
		evaluation, err := adapter.EvaluateCapability(t.Context(), request)
		if err != nil || evaluation.State != broker.CapabilityDegraded ||
			evaluation.QuoteRight.Code != "QUOTE_RIGHT_QUERY_FAILED" {
			t.Fatalf("failed entitlement query = %#v, %v", evaluation, err)
		}
	}
	if calls := server.userInfoCalls.Load(); calls != 1 {
		t.Fatalf("failed GetUserInfo calls = %d, want 1", calls)
	}

	level3 := int32(qotcommonpb.QotRight_QotRight_Level3)
	server.setUserInfoResponse(&getuserinfopb.Response{
		RetType: new(int32(0)),
		S2C:     &getuserinfopb.S2C{UsQotRight: &level3},
	})
	client := adapter.exchange.Client()
	if client == nil {
		t.Fatal("OpenD client is nil")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close() error = %v", err)
	}

	evaluation, err := adapter.EvaluateCapability(t.Context(), request)
	if err != nil || evaluation.State != broker.CapabilityAvailable {
		t.Fatalf("refreshed entitlement = %#v, %v", evaluation, err)
	}
	if calls := server.userInfoCalls.Load(); calls != 2 {
		t.Fatalf("GetUserInfo calls after reconnect = %d, want 2", calls)
	}
	if adapter.lastQuoteRights.value.GetUsQotRight() != level3 ||
		adapter.lastQuoteRights.generation != adapter.exchange.activeConnectionGeneration() {
		t.Fatalf("refreshed quote rights = %#v", adapter.lastQuoteRights)
	}
}

func TestQuoteRightsFromUserInfoPreservesProductRightsAndCNFallback(t *testing.T) {
	cn := int32(qotcommonpb.QotRight_QotRight_Level1)
	usIndex := int32(qotcommonpb.QotRight_QotRight_No)
	hasUSOption := true
	rights := quoteRightsFromUserInfo(&getuserinfopb.S2C{
		CnQotRight: &cn, UsIndexQotRight: &usIndex,
		HasUSOptionQotRight: &hasUSOption,
	})
	if rights == nil ||
		rights.GetShQotRight() != cn ||
		rights.GetSzQotRight() != cn ||
		rights.GetUsOptionQotRight() != int32(qotcommonpb.QotRight_QotRight_Level1) {
		t.Fatalf("converted quote rights = %#v", rights)
	}
	indexRequest := broker.CapabilityEvaluationRequest{
		Market: "US", ProductClass: broker.ProductClassIndex,
	}
	if got := quoteRightForCapability(rights, indexRequest); got != usIndex {
		t.Fatalf("US index quote right = %d, want %d", got, usIndex)
	}
	if quoteRightsFromUserInfo(&getuserinfopb.S2C{}) != nil {
		t.Fatal("empty user info produced quote rights")
	}
}

func TestFutuCapabilityRejectsStaleNotificationsAndFailureWrites(t *testing.T) {
	exchange := NewExchange("")
	exchange.activeGeneration.Store(2)
	adapter := NewBrokerAdapter(exchange).(*futuAdapter)
	level1 := completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level1)
	level3 := completeCapabilityRights(qotcommonpb.QotRight_QotRight_Level3)
	notification := func(rights *notifypb.QotRight) *notifypb.Response {
		return &notifypb.Response{S2C: &notifypb.S2C{
			Type:     new(int32(notifypb.NotifyType_NotifyType_QotRight)),
			QotRight: rights,
		}}
	}

	adapter.captureCapabilityNotificationAt(notification(level3), 2)
	adapter.lastQuoteRightsFailure = &futuQuoteCapabilityFailure{
		generation: 2, retryAt: time.Now().Add(time.Minute),
	}
	adapter.captureCapabilityNotificationAt(notification(level1), 1)
	if adapter.lastQuoteRights.generation != 2 ||
		adapter.lastQuoteRights.value.GetUsQotRight() !=
			int32(qotcommonpb.QotRight_QotRight_Level3) ||
		adapter.lastQuoteRightsFailure == nil {
		t.Fatalf("stale notification changed current rights: %#v", adapter.lastQuoteRights)
	}

	if !adapter.resolveOrRememberQuoteRightsFailure(
		2, time.Now().UTC(), errors.New("stale query error"),
	) {
		t.Fatal("current quote-right notification did not resolve the query failure")
	}
	if adapter.lastQuoteRightsFailure != nil {
		t.Fatal("current rights did not clear the same-generation failure")
	}
	if check := adapter.evaluateQuoteCapability(
		broker.CapabilityEvaluationRequest{Market: "US"}, time.Now().UTC(),
	); check.Code != "QUOTE_RIGHT_AVAILABLE" {
		t.Fatalf("current generation evaluation = %#v", check)
	}

	exchange.activeGeneration.Store(3)
	if check := adapter.evaluateQuoteCapability(
		broker.CapabilityEvaluationRequest{Market: "US"}, time.Now().UTC(),
	); check.Code != "QUOTE_RIGHT_UNVERIFIED" {
		t.Fatalf("stale generation evaluation = %#v", check)
	}
	if adapter.resolveOrRememberQuoteRightsFailure(
		2, time.Now().UTC(), errors.New("old connection error"),
	) {
		t.Fatal("stale generation error was resolved by unrelated rights")
	}
	if adapter.lastQuoteRightsFailure != nil {
		t.Fatalf("stale generation failure was cached: %#v", adapter.lastQuoteRightsFailure)
	}
	exchange.activeGeneration.Store(0)
	if check := adapter.evaluateQuoteCapability(
		broker.CapabilityEvaluationRequest{Market: "US"}, time.Now().UTC(),
	); check.Code != "QUOTE_RIGHT_UNVERIFIED" {
		t.Fatalf("disconnected generation evaluation = %#v", check)
	}
	exchange.activeGeneration.Store(3)
	if adapter.resolveOrRememberQuoteRightsFailure(
		3, time.Now().UTC(), context.Canceled,
	) || adapter.lastQuoteRightsFailure != nil {
		t.Fatal("caller cancellation was cached as a quote-right failure")
	}
}

func completeCapabilityRights(value qotcommonpb.QotRight) *notifypb.QotRight {
	right := int32(value)
	return &notifypb.QotRight{
		HkQotRight: &right, UsQotRight: &right,
		HkOptionQotRight: &right, UsOptionQotRight: &right,
		HkFutureQotRight: &right, UsFutureQotRight: &right,
		UsIndexQotRight:     &right,
		UsCMEFutureQotRight: &right, UsCBOTFutureQotRight: &right,
		UsNYMEXFutureQotRight: &right, UsCOMEXFutureQotRight: &right,
		UsCBOEFutureQotRight: &right, ShQotRight: &right, SzQotRight: &right,
		EcQotRight: &right,
	}
}
