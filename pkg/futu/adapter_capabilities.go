package futu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

type futuConnectCapabilityStatus struct {
	quoteLoggedIn bool
	tradeLoggedIn bool
	observedAt    time.Time
}

type futuQuoteCapabilityRights struct {
	value      *notifypb.QotRight
	observedAt time.Time
}

func (a *futuAdapter) captureCapabilityNotification(response *notifypb.Response) {
	if a == nil || response == nil || response.GetS2C() == nil {
		return
	}
	now := time.Now().UTC()
	s2c := response.GetS2C()
	a.capabilityMu.Lock()
	defer a.capabilityMu.Unlock()
	switch notifypb.NotifyType(s2c.GetType()) {
	case notifypb.NotifyType_NotifyType_ConnStatus:
		status := s2c.GetConnectStatus()
		if status != nil {
			a.lastConnectStatus = &futuConnectCapabilityStatus{
				quoteLoggedIn: status.GetQotLogined(),
				tradeLoggedIn: status.GetTrdLogined(),
				observedAt:    now,
			}
		}
	case notifypb.NotifyType_NotifyType_QotRight:
		rights := s2c.GetQotRight()
		if rights != nil {
			a.lastQuoteRights = &futuQuoteCapabilityRights{
				value: proto.Clone(rights).(*notifypb.QotRight), observedAt: now,
			}
		}
	}
}

func (a *futuAdapter) EvaluateCapability(
	ctx context.Context,
	request broker.CapabilityEvaluationRequest,
) (broker.CapabilityEvaluation, error) {
	now := time.Now().UTC()
	evaluation := broker.CapabilityEvaluation{
		State:      broker.CapabilityAvailable,
		Connection: capabilityNotRequired(now),
		Account:    capabilityNotRequired(now),
		QuoteRight: capabilityNotRequired(now),
		CheckedAt:  now,
	}
	if a == nil || a.exchange == nil {
		evaluation.Connection = capabilityUnavailable(
			now, "OPEND_UNCONFIGURED", "OpenD exchange is not configured.",
		)
		return aggregateCapabilityEvaluation(evaluation), nil
	}
	if request.DeclaredCapability.RequiresConnection {
		err := a.exchange.withRetryingClient(ctx, func(_ *opend.Client) error {
			return nil
		})
		if err != nil {
			evaluation.Connection = capabilityUnavailable(
				now, "OPEND_CONNECTION_UNAVAILABLE", err.Error(),
			)
			return aggregateCapabilityEvaluation(evaluation), nil
		}
		evaluation.Connection = capabilityAvailable(now, "OPEND_CONNECTED", "OpenD session is connected.")
		a.capabilityMu.RLock()
		connectStatus := a.lastConnectStatus
		a.capabilityMu.RUnlock()
		if connectStatus != nil {
			loggedIn := connectStatus.quoteLoggedIn
			if request.DeclaredCapability.Access != broker.FeatureAccessRead {
				loggedIn = connectStatus.tradeLoggedIn
			}
			if !loggedIn {
				evaluation.Connection = capabilityUnavailable(
					connectStatus.observedAt, "OPEND_NOT_LOGGED_IN",
					"OpenD is connected but the required quote or trade session is not logged in.",
				)
			}
		}
	}
	if request.DeclaredCapability.RequiresAccount {
		evaluation.Account = a.evaluateAccountCapability(ctx, request, now)
	}
	if request.DeclaredCapability.RequiresQuoteRight {
		evaluation.QuoteRight = a.evaluateQuoteCapability(request, now)
	}
	return aggregateCapabilityEvaluation(evaluation), nil
}

func (a *futuAdapter) evaluateAccountCapability(
	ctx context.Context,
	request broker.CapabilityEvaluationRequest,
	now time.Time,
) broker.CapabilityCheck {
	accountID := strings.TrimSpace(request.AccountID)
	if accountID == "" {
		return capabilityDegraded(
			now, "ACCOUNT_CONTEXT_REQUIRED",
			"Select an account to verify product and trading eligibility.",
		)
	}
	accounts, err := a.capabilityAccountSnapshot(ctx, now)
	if err != nil {
		return capabilityUnavailable(now, "ACCOUNT_DISCOVERY_FAILED", err.Error())
	}
	for _, account := range accounts {
		if account.ID != accountID {
			continue
		}
		if environment := strings.TrimSpace(request.TradingEnvironment); environment != "" &&
			!strings.EqualFold(environment, account.TradingEnvironment) {
			continue
		}
		if market := strings.TrimSpace(request.Market); market != "" &&
			len(account.MarketAuthorities) > 0 &&
			!containsFoldValue(account.MarketAuthorities, market) {
			continue
		}
		if request.MarketSegment == broker.MarketSegmentPrediction ||
			request.ProductClass == broker.ProductClassEventContract ||
			strings.HasPrefix(string(request.FeatureID), "prediction.") {
			firm := ""
			if account.SecurityFirm != nil {
				firm = strings.ToUpper(strings.TrimSpace(*account.SecurityFirm))
			}
			if firm != "FUTUINC" {
				return capabilityUnavailable(
					now, "PREDICTION_ACCOUNT_INELIGIBLE",
					"Prediction markets require an eligible Moomoo US (FUTUINC) account.",
				)
			}
		}
		return capabilityAvailable(now, "ACCOUNT_ELIGIBLE", "The selected account is eligible.")
	}
	return capabilityUnavailable(
		now, "ACCOUNT_NOT_FOUND",
		fmt.Sprintf("Account %q is not available for the requested environment and market.", accountID),
	)
}

func (a *futuAdapter) capabilityAccountSnapshot(
	ctx context.Context,
	now time.Time,
) ([]broker.Account, error) {
	a.capabilityMu.RLock()
	if now.Before(a.capabilityAccountsExpiresAt) {
		accounts := append([]broker.Account(nil), a.capabilityAccounts...)
		a.capabilityMu.RUnlock()
		return accounts, nil
	}
	a.capabilityMu.RUnlock()
	accounts, err := a.DiscoverAccounts(ctx)
	if err != nil {
		return nil, err
	}
	a.capabilityMu.Lock()
	a.capabilityAccounts = append([]broker.Account(nil), accounts...)
	a.capabilityAccountsExpiresAt = now.Add(3 * time.Second)
	a.capabilityMu.Unlock()
	return accounts, nil
}

func (a *futuAdapter) evaluateQuoteCapability(
	request broker.CapabilityEvaluationRequest,
	now time.Time,
) broker.CapabilityCheck {
	a.capabilityMu.RLock()
	snapshot := a.lastQuoteRights
	a.capabilityMu.RUnlock()
	if snapshot == nil || snapshot.value == nil {
		return capabilityDegraded(
			now, "QUOTE_RIGHT_UNVERIFIED",
			"OpenD has not reported quote entitlements for this session yet.",
		)
	}
	right := quoteRightForCapability(snapshot.value, request)
	switch qotcommonpb.QotRight(right) {
	case qotcommonpb.QotRight_QotRight_Level1,
		qotcommonpb.QotRight_QotRight_Level2,
		qotcommonpb.QotRight_QotRight_SF,
		qotcommonpb.QotRight_QotRight_Level3:
		return capabilityAvailable(snapshot.observedAt, "QUOTE_RIGHT_AVAILABLE", "Quote entitlement is available.")
	case qotcommonpb.QotRight_QotRight_Bmp:
		return capabilityDegraded(
			snapshot.observedAt, "QUOTE_RIGHT_POLLING_ONLY",
			"BMP quote access is available for polling but does not permit streaming subscriptions.",
		)
	case qotcommonpb.QotRight_QotRight_No:
		return capabilityUnavailable(
			snapshot.observedAt, "QUOTE_RIGHT_DENIED",
			"The selected OpenD session has no quote entitlement for this product.",
		)
	default:
		return capabilityDegraded(
			snapshot.observedAt, "QUOTE_RIGHT_UNKNOWN",
			"OpenD reported an unknown quote entitlement for this product.",
		)
	}
}

func quoteRightForCapability(
	rights *notifypb.QotRight,
	request broker.CapabilityEvaluationRequest,
) int32 {
	if request.MarketSegment == broker.MarketSegmentPrediction ||
		request.ProductClass == broker.ProductClassEventContract ||
		strings.HasPrefix(string(request.FeatureID), "prediction.") {
		return rights.GetEcQotRight()
	}
	if request.ProductClass == broker.ProductClassOption ||
		strings.Contains(string(request.FeatureID), "option") {
		if strings.EqualFold(request.Market, "HK") {
			return rights.GetHkOptionQotRight()
		}
		return rights.GetUsOptionQotRight()
	}
	if request.ProductClass == broker.ProductClassFuture ||
		request.FeatureID == broker.FeatureFutures {
		if strings.EqualFold(request.Market, "HK") {
			return rights.GetHkFutureQotRight()
		}
		return maximumQuoteRight(
			rights.GetUsFutureQotRight(), rights.GetUsCMEFutureQotRight(),
			rights.GetUsCBOTFutureQotRight(), rights.GetUsNYMEXFutureQotRight(),
			rights.GetUsCOMEXFutureQotRight(), rights.GetUsCBOEFutureQotRight(),
		)
	}
	switch strings.ToUpper(strings.TrimSpace(request.Market)) {
	case "HK":
		return rights.GetHkQotRight()
	case "SH":
		return rights.GetShQotRight()
	case "SZ":
		return rights.GetSzQotRight()
	default:
		return rights.GetUsQotRight()
	}
}

func maximumQuoteRight(values ...int32) int32 {
	var result int32
	for _, value := range values {
		if value == int32(qotcommonpb.QotRight_QotRight_No) {
			continue
		}
		if value > result {
			result = value
		}
	}
	if result == 0 {
		for _, value := range values {
			if value == int32(qotcommonpb.QotRight_QotRight_No) {
				return value
			}
		}
	}
	return result
}

func aggregateCapabilityEvaluation(
	evaluation broker.CapabilityEvaluation,
) broker.CapabilityEvaluation {
	evaluation.State = broker.CapabilityAvailable
	for _, check := range []broker.CapabilityCheck{
		evaluation.Connection, evaluation.Account, evaluation.QuoteRight,
	} {
		if check.State == broker.CapabilityUnavailable {
			evaluation.State = broker.CapabilityUnavailable
			evaluation.Code = check.Code
			evaluation.Reason = check.Reason
			return evaluation
		}
		if check.State == broker.CapabilityDegraded {
			evaluation.State = broker.CapabilityDegraded
			if evaluation.Code == "" {
				evaluation.Code = check.Code
				evaluation.Reason = check.Reason
			}
		}
	}
	return evaluation
}

func capabilityAvailable(at time.Time, code, reason string) broker.CapabilityCheck {
	return broker.CapabilityCheck{State: broker.CapabilityAvailable, Code: code, Reason: reason, CheckedAt: at}
}

func capabilityDegraded(at time.Time, code, reason string) broker.CapabilityCheck {
	return broker.CapabilityCheck{State: broker.CapabilityDegraded, Code: code, Reason: reason, CheckedAt: at}
}

func capabilityUnavailable(at time.Time, code, reason string) broker.CapabilityCheck {
	return broker.CapabilityCheck{State: broker.CapabilityUnavailable, Code: code, Reason: reason, CheckedAt: at}
}

func capabilityNotRequired(at time.Time) broker.CapabilityCheck {
	return capabilityAvailable(at, "NOT_REQUIRED", "This runtime dimension is not required.")
}

func containsFoldValue(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}
