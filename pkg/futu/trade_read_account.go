package futu

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func (e *Exchange) resolveTradeAccountWithClient(ctx context.Context, client *opend.Client, query BrokerReadQuery) (resolvedTradeAccount, error) {
	accounts, err := client.GetAccountList(ctx)
	if err != nil {
		return resolvedTradeAccount{}, err
	}
	if len(accounts) == 0 {
		return resolvedTradeAccount{}, fmt.Errorf("futu exchange: no trading accounts discovered")
	}

	normalized := normalizeBrokerReadQuery(query)
	candidates := make([]resolvedTradeAccount, 0, len(accounts))
	for _, account := range accounts {
		candidate, ok, err := candidateTradeAccountFromProto(account, normalized)
		if err != nil {
			return resolvedTradeAccount{}, err
		}
		if ok {
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) == 0 {
		if normalized.AccountID != "" {
			return resolvedTradeAccount{}, fmt.Errorf("futu exchange: account %s not found for tradingEnvironment=%s market=%s", normalized.AccountID, normalized.TradingEnvironment, normalized.Market)
		}
		return resolvedTradeAccount{}, fmt.Errorf("futu exchange: no trading account matched tradingEnvironment=%s market=%s", normalized.TradingEnvironment, normalized.Market)
	}

	if normalized.TradingEnvironment == "" {
		if preferred := filterResolvedTradeAccountsByEnvironment(candidates, "SIMULATE"); len(preferred) > 0 {
			candidates = preferred
		}
	}

	sortResolvedTradeAccounts(candidates)

	return candidates[0], nil
}

func sortResolvedTradeAccounts(candidates []resolvedTradeAccount) {
	sort.Slice(candidates, func(i, j int) bool {
		leftPriority := resolvedTradeAccountPriority(candidates[i])
		rightPriority := resolvedTradeAccountPriority(candidates[j])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if candidates[i].AccountID != candidates[j].AccountID {
			return candidates[i].AccountID < candidates[j].AccountID
		}
		return candidates[i].Market < candidates[j].Market
	})
}

func candidateTradeAccountFromProto(account *trdcommonpb.TrdAcc, query BrokerReadQuery) (resolvedTradeAccount, bool, error) {
	if account == nil {
		return resolvedTradeAccount{}, false, nil
	}

	runtimeAccount := runtimeAccountFromProto(account)
	accountID := runtimeAccount.AccountID
	protoAccountID := strconv.FormatUint(account.GetAccID(), 10)
	if query.AccountID != "" && !strings.EqualFold(query.AccountID, accountID) && !strings.EqualFold(query.AccountID, protoAccountID) {
		return resolvedTradeAccount{}, false, nil
	}
	if query.TradingEnvironment != "" && !strings.EqualFold(query.TradingEnvironment, runtimeAccount.TradingEnvironment) {
		return resolvedTradeAccount{}, false, nil
	}

	selectedMarket, selectedMarketCode, ok, err := resolveTradeMarket(account, query.Market)
	if err != nil {
		return resolvedTradeAccount{}, false, err
	}
	if !ok {
		return resolvedTradeAccount{}, false, nil
	}

	return resolvedTradeAccount{
		AccountID:          accountID,
		TradingEnvironment: runtimeAccount.TradingEnvironment,
		Market:             selectedMarket,
		AccountType:        runtimeAccount.AccountType,
		protoAccountID:     account.GetAccID(),
		protoTrdEnv:        account.GetTrdEnv(),
		protoTrdMarket:     selectedMarketCode,
	}, true, nil
}

func resolveTradeMarket(account *trdcommonpb.TrdAcc, requested string) (string, int32, bool, error) {
	normalizedRequested := strings.ToUpper(strings.TrimSpace(requested))
	authList := account.GetTrdMarketAuthList()
	if normalizedRequested != "" {
		if len(authList) > 0 {
			for _, rawMarket := range authList {
				if runtimeMarketAuthority(rawMarket) == normalizedRequested {
					return normalizedRequested, rawMarket, true, nil
				}
			}
			return "", 0, false, nil
		}
		rawMarket, ok := trdMarketFromNormalized(normalizedRequested)
		if !ok {
			return "", 0, false, fmt.Errorf("futu exchange: unsupported market %q", requested)
		}
		return normalizedRequested, int32(rawMarket), true, nil
	}

	for _, rawMarket := range authList {
		normalizedMarket := runtimeMarketAuthority(rawMarket)
		if normalizedMarket == "" {
			continue
		}
		return normalizedMarket, rawMarket, true, nil
	}

	return "HK", int32(trdcommonpb.TrdMarket_TrdMarket_HK), true, nil
}

func trdMarketFromNormalized(market string) (trdcommonpb.TrdMarket, bool) {
	switch strings.ToUpper(strings.TrimSpace(market)) {
	case "HK":
		return trdcommonpb.TrdMarket_TrdMarket_HK, true
	case "US":
		return trdcommonpb.TrdMarket_TrdMarket_US, true
	case "CN":
		return trdcommonpb.TrdMarket_TrdMarket_CN, true
	case "SG":
		return trdcommonpb.TrdMarket_TrdMarket_SG, true
	case "AU":
		return trdcommonpb.TrdMarket_TrdMarket_AU, true
	case "JP":
		return trdcommonpb.TrdMarket_TrdMarket_JP, true
	case "MY":
		return trdcommonpb.TrdMarket_TrdMarket_MY, true
	case "CA":
		return trdcommonpb.TrdMarket_TrdMarket_CA, true
	case "CRYPTO":
		return trdcommonpb.TrdMarket_TrdMarket_Crypto, true
	case "FUTURES":
		return trdcommonpb.TrdMarket_TrdMarket_Futures, true
	default:
		return 0, false
	}
}

func normalizeBrokerReadQuery(query BrokerReadQuery) BrokerReadQuery {
	return BrokerReadQuery{
		AccountID:          strings.TrimSpace(query.AccountID),
		TradingEnvironment: strings.ToUpper(strings.TrimSpace(query.TradingEnvironment)),
		Market:             strings.ToUpper(strings.TrimSpace(query.Market)),
	}
}

func filterResolvedTradeAccountsByEnvironment(candidates []resolvedTradeAccount, tradingEnvironment string) []resolvedTradeAccount {
	filtered := make([]resolvedTradeAccount, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.TradingEnvironment, tradingEnvironment) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func resolvedTradeAccountPriority(candidate resolvedTradeAccount) int {
	switch candidate.TradingEnvironment {
	case "SIMULATE":
		return 0
	case "REAL":
		return 1
	default:
		return 2
	}
}

func (account resolvedTradeAccount) header() *trdcommonpb.TrdHeader {
	return &trdcommonpb.TrdHeader{
		TrdEnv:    new(account.protoTrdEnv),
		AccID:     new(account.protoAccountID),
		TrdMarket: new(account.protoTrdMarket),
	}
}
