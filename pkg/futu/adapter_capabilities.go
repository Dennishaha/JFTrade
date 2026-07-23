package futu

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	getuserinfopb "github.com/jftrade/jftrade-main/pkg/futu/pb/getuserinfo"
	notifypb "github.com/jftrade/jftrade-main/pkg/futu/pb/notify"
	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

const quoteRightsFailureRetryInterval = 3 * time.Second

type futuConnectCapabilityStatus struct {
	quoteLoggedIn bool
	tradeLoggedIn bool
	observedAt    time.Time
	generation    uint64
}

type futuQuoteCapabilityRights struct {
	value      *notifypb.QotRight
	observedAt time.Time
	generation uint64
}

type futuQuoteCapabilityFailure struct {
	reason     string
	observedAt time.Time
	retryAt    time.Time
	generation uint64
}

func (a *futuAdapter) captureCapabilityNotification(response *notifypb.Response) {
	generation := uint64(0)
	if a != nil && a.exchange != nil {
		generation = a.exchange.activeConnectionGeneration()
	}
	a.captureCapabilityNotificationAt(response, generation)
}

func (a *futuAdapter) captureCapabilityNotificationAt(
	response *notifypb.Response,
	generation uint64,
) {
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
			if a.lastConnectStatus != nil &&
				a.lastConnectStatus.generation > generation {
				return
			}
			a.lastConnectStatus = &futuConnectCapabilityStatus{
				quoteLoggedIn: status.GetQotLogined(),
				tradeLoggedIn: status.GetTrdLogined(),
				observedAt:    now,
				generation:    generation,
			}
		}
	case notifypb.NotifyType_NotifyType_QotRight:
		rights := s2c.GetQotRight()
		if rights != nil {
			if a.lastQuoteRights != nil &&
				a.lastQuoteRights.generation > generation {
				return
			}
			a.lastQuoteRights = &futuQuoteCapabilityRights{
				value: proto.Clone(rights).(*notifypb.QotRight), observedAt: now,
				generation: generation,
			}
			if a.lastQuoteRightsFailure != nil &&
				a.lastQuoteRightsFailure.generation == generation {
				a.lastQuoteRightsFailure = nil
			}
			a.quoteRightsRevision++
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
		if connectStatus != nil &&
			connectStatus.generation == a.exchange.activeConnectionGeneration() {
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
		if err := a.ensureQuoteRights(ctx, now); err != nil {
			evaluation.QuoteRight = capabilityDegraded(
				now,
				"QUOTE_RIGHT_QUERY_FAILED",
				"OpenD quote entitlement verification failed: "+err.Error(),
			)
		} else {
			evaluation.QuoteRight = a.evaluateQuoteCapability(request, now)
		}
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
	expectedGeneration := uint64(0)
	hasExchange := a != nil && a.exchange != nil
	if hasExchange {
		expectedGeneration = a.exchange.activeConnectionGeneration()
	}
	a.capabilityMu.RLock()
	snapshot := a.lastQuoteRights
	a.capabilityMu.RUnlock()
	if snapshot == nil ||
		snapshot.value == nil ||
		(hasExchange && expectedGeneration == 0) ||
		(expectedGeneration != 0 && snapshot.generation != expectedGeneration) ||
		(expectedGeneration != 0 &&
			a.exchange.activeConnectionGeneration() != expectedGeneration) {
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

func (a *futuAdapter) ensureQuoteRights(ctx context.Context, now time.Time) error {
	if a == nil || a.exchange == nil {
		return errors.New("OpenD exchange is not configured")
	}
	for range 2 {
		generation := a.exchange.activeConnectionGeneration()
		if generation == 0 {
			if err := a.exchange.withRetryingClient(ctx, func(*opend.Client) error {
				return nil
			}); err != nil {
				return err
			}
			generation = a.exchange.activeConnectionGeneration()
		}
		if err := a.cachedQuoteRightsStatus(generation, now); err == nil {
			return nil
		} else if !errors.Is(err, errQuoteRightsRefreshRequired) {
			return err
		}

		key := fmt.Sprintf("session-%d", generation)
		resultChannel := a.quoteRightsFlight.DoChan(key, func() (any, error) {
			return nil, a.refreshQuoteRights(ctx, generation)
		})
		var err error
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-resultChannel:
			err = result.Err
		}
		if errors.Is(err, errQuoteRightsRefreshRequired) {
			now = time.Now().UTC()
			continue
		}
		return err
	}
	return errors.New("OpenD connection changed while verifying quote entitlements")
}

func (a *futuAdapter) refreshQuoteRights(ctx context.Context, generation uint64) error {
	queryContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if generation == 0 || a.exchange.activeConnectionGeneration() != generation {
		return errQuoteRightsRefreshRequired
	}
	if err := a.cachedQuoteRightsStatus(generation, time.Now().UTC()); err == nil {
		return nil
	} else if !errors.Is(err, errQuoteRightsRefreshRequired) {
		return err
	}

	a.capabilityMu.RLock()
	startRevision := a.quoteRightsRevision
	a.capabilityMu.RUnlock()

	info, fetchedGeneration, fetchErr := a.exchange.queryQuoteRights(queryContext, generation)
	observedAt := time.Now().UTC()
	if errors.Is(fetchErr, context.Canceled) ||
		errors.Is(fetchErr, context.DeadlineExceeded) {
		return fetchErr
	}
	if fetchedGeneration == 0 &&
		a.exchange.activeConnectionGeneration() == 0 &&
		fetchErr != nil {
		return fetchErr
	}
	if fetchedGeneration == 0 ||
		fetchedGeneration != generation ||
		a.exchange.activeConnectionGeneration() != generation {
		return errQuoteRightsRefreshRequired
	}
	if fetchErr != nil {
		return a.handleQuoteRightsFetchFailure(generation, observedAt, fetchErr)
	}
	rights := quoteRightsFromUserInfo(info)
	if rights == nil {
		return a.handleQuoteRightsFetchFailure(
			generation,
			observedAt,
			errors.New("OpenD GetUserInfo returned no quote entitlement fields"),
		)
	}
	return a.storeQuoteRights(generation, startRevision, observedAt, rights)
}

func (a *futuAdapter) handleQuoteRightsFetchFailure(
	generation uint64,
	observedAt time.Time,
	err error,
) error {
	if a.resolveOrRememberQuoteRightsFailure(generation, observedAt, err) {
		return nil
	}
	if a.exchange.activeConnectionGeneration() != generation {
		return errQuoteRightsRefreshRequired
	}
	return err
}

func (a *futuAdapter) storeQuoteRights(
	generation uint64,
	startRevision uint64,
	observedAt time.Time,
	rights *notifypb.QotRight,
) error {
	a.capabilityMu.Lock()
	defer a.capabilityMu.Unlock()
	if a.quoteRightsRevision != startRevision &&
		a.lastQuoteRights != nil &&
		a.lastQuoteRights.value != nil &&
		a.lastQuoteRights.generation == generation {
		if a.lastQuoteRightsFailure != nil &&
			a.lastQuoteRightsFailure.generation == generation {
			a.lastQuoteRightsFailure = nil
		}
		return nil
	}
	if a.lastQuoteRights != nil &&
		a.lastQuoteRights.generation > generation {
		return errQuoteRightsRefreshRequired
	}
	a.lastQuoteRights = &futuQuoteCapabilityRights{
		value: rights, observedAt: observedAt, generation: generation,
	}
	if a.lastQuoteRightsFailure != nil &&
		a.lastQuoteRightsFailure.generation == generation {
		a.lastQuoteRightsFailure = nil
	}
	return nil
}

var errQuoteRightsRefreshRequired = errors.New("quote rights refresh required")

func (a *futuAdapter) cachedQuoteRightsStatus(
	generation uint64,
	now time.Time,
) error {
	a.capabilityMu.RLock()
	defer a.capabilityMu.RUnlock()
	if snapshot := a.lastQuoteRights; snapshot != nil &&
		snapshot.value != nil &&
		snapshot.generation == generation &&
		generation != 0 {
		return nil
	}
	if failure := a.lastQuoteRightsFailure; failure != nil &&
		failure.generation == generation &&
		now.Before(failure.retryAt) {
		return errors.New(failure.reason)
	}
	return errQuoteRightsRefreshRequired
}

func (a *futuAdapter) resolveOrRememberQuoteRightsFailure(
	generation uint64,
	observedAt time.Time,
	err error,
) bool {
	a.capabilityMu.Lock()
	defer a.capabilityMu.Unlock()
	if generation == 0 ||
		a.exchange.activeConnectionGeneration() != generation {
		return false
	}
	if a.lastQuoteRights != nil &&
		a.lastQuoteRights.value != nil &&
		a.lastQuoteRights.generation == generation {
		if a.lastQuoteRightsFailure != nil &&
			a.lastQuoteRightsFailure.generation == generation {
			a.lastQuoteRightsFailure = nil
		}
		return true
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if a.lastQuoteRightsFailure != nil &&
		a.lastQuoteRightsFailure.generation > generation {
		return false
	}
	a.lastQuoteRightsFailure = &futuQuoteCapabilityFailure{
		reason: err.Error(), observedAt: observedAt,
		retryAt:    observedAt.Add(quoteRightsFailureRetryInterval),
		generation: generation,
	}
	return false
}

func quoteRightsFromUserInfo(info *getuserinfopb.S2C) *notifypb.QotRight {
	if info == nil || !userInfoHasQuoteRights(info) {
		return nil
	}
	shRight := cloneOptionalInt32(info.ShQotRight)
	if shRight == nil {
		shRight = cloneOptionalInt32(info.CnQotRight)
	}
	szRight := cloneOptionalInt32(info.SzQotRight)
	if szRight == nil {
		szRight = cloneOptionalInt32(info.CnQotRight)
	}
	usOptionRight := cloneOptionalInt32(info.UsOptionQotRight)
	if usOptionRight == nil && info.GetHasUSOptionQotRight() {
		usOptionRight = new(int32(qotcommonpb.QotRight_QotRight_Level1))
	}
	return &notifypb.QotRight{
		HkQotRight:            cloneOptionalInt32(info.HkQotRight),
		UsQotRight:            cloneOptionalInt32(info.UsQotRight),
		CnQotRight:            cloneOptionalInt32(info.CnQotRight),
		HkOptionQotRight:      cloneOptionalInt32(info.HkOptionQotRight),
		HasUSOptionQotRight:   cloneOptionalBool(info.HasUSOptionQotRight),
		HkFutureQotRight:      cloneOptionalInt32(info.HkFutureQotRight),
		UsFutureQotRight:      cloneOptionalInt32(info.UsFutureQotRight),
		UsOptionQotRight:      usOptionRight,
		UsIndexQotRight:       cloneOptionalInt32(info.UsIndexQotRight),
		UsOtcQotRight:         cloneOptionalInt32(info.UsOtcQotRight),
		SgFutureQotRight:      cloneOptionalInt32(info.SgFutureQotRight),
		JpFutureQotRight:      cloneOptionalInt32(info.JpFutureQotRight),
		UsCMEFutureQotRight:   cloneOptionalInt32(info.UsCMEFutureQotRight),
		UsCBOTFutureQotRight:  cloneOptionalInt32(info.UsCBOTFutureQotRight),
		UsNYMEXFutureQotRight: cloneOptionalInt32(info.UsNYMEXFutureQotRight),
		UsCOMEXFutureQotRight: cloneOptionalInt32(info.UsCOMEXFutureQotRight),
		UsCBOEFutureQotRight:  cloneOptionalInt32(info.UsCBOEFutureQotRight),
		ShQotRight:            shRight,
		SzQotRight:            szRight,
		CcQotRight:            cloneOptionalInt32(info.CcQotRight),
		SgStockQotRight:       cloneOptionalInt32(info.SgStockQotRight),
		MyStockQotRight:       cloneOptionalInt32(info.MyStockQotRight),
		JpStockQotRight:       cloneOptionalInt32(info.JpStockQotRight),
		EcQotRight:            cloneOptionalInt32(info.EcQotRight),
	}
}

func userInfoHasQuoteRights(info *getuserinfopb.S2C) bool {
	return info.HkQotRight != nil ||
		info.UsQotRight != nil ||
		info.CnQotRight != nil ||
		info.ShQotRight != nil ||
		info.SzQotRight != nil ||
		info.HkOptionQotRight != nil ||
		info.HasUSOptionQotRight != nil ||
		info.UsOptionQotRight != nil ||
		info.HkFutureQotRight != nil ||
		info.UsFutureQotRight != nil ||
		info.UsIndexQotRight != nil ||
		info.UsOtcQotRight != nil ||
		info.UsCMEFutureQotRight != nil ||
		info.UsCBOTFutureQotRight != nil ||
		info.UsNYMEXFutureQotRight != nil ||
		info.UsCOMEXFutureQotRight != nil ||
		info.UsCBOEFutureQotRight != nil ||
		info.SgFutureQotRight != nil ||
		info.JpFutureQotRight != nil ||
		info.CcQotRight != nil ||
		info.SgStockQotRight != nil ||
		info.MyStockQotRight != nil ||
		info.JpStockQotRight != nil ||
		info.EcQotRight != nil
}

func cloneOptionalInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func cloneOptionalBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	return new(*value)
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
	if request.ProductClass == broker.ProductClassIndex &&
		strings.EqualFold(request.Market, "US") {
		return rights.GetUsIndexQotRight()
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
