package futu

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

// RuntimeAccount is a normalized OpenD trading-account snapshot suitable for
// bbgo-side consumers and compatibility routes.
type RuntimeAccount struct {
	AccountID              string   `json:"accountId"`
	TradingEnvironment     string   `json:"tradingEnvironment"`
	AccountType            string   `json:"accountType"`
	AccountRole            *string  `json:"accountRole"`
	SecurityFirm           *string  `json:"securityFirm"`
	MarketAuthorities      []string `json:"marketAuthorities"`
	OrderMarketAuthorities []string `json:"-"`
	SimulatedAccountType   *string  `json:"simulatedAccountType"`
}

// DiscoverAccounts returns the trading accounts currently exposed by OpenD.
// The query reuses the exchange's managed OpenD session so bbgo-facing callers
// stay on the same connection lifecycle as quote/trade operations.
func (e *Exchange) DiscoverAccounts(ctx context.Context) ([]RuntimeAccount, error) {
	var protoAccounts []*trdcommonpb.TrdAcc
	if err := e.withRetryingClient(ctx, func(client *opend.Client) error {
		accounts, err := client.GetAccountList(ctx)
		if err != nil {
			return err
		}
		protoAccounts = accounts
		return nil
	}); err != nil {
		return nil, err
	}

	return runtimeAccountsFromProto(protoAccounts), nil
}

func runtimeAccountsFromProto(protoAccounts []*trdcommonpb.TrdAcc) []RuntimeAccount {
	accounts := make([]RuntimeAccount, 0, len(protoAccounts))
	seen := make(map[string]struct{}, len(protoAccounts))
	for _, account := range protoAccounts {
		if account == nil {
			continue
		}
		normalized := runtimeAccountFromProto(account)
		key := normalized.AccountID + "|" + normalized.TradingEnvironment
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		accounts = append(accounts, normalized)
	}

	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].TradingEnvironment != accounts[j].TradingEnvironment {
			return accounts[i].TradingEnvironment < accounts[j].TradingEnvironment
		}
		return accounts[i].AccountID < accounts[j].AccountID
	})
	return accounts
}

func runtimeAccountFromProto(account *trdcommonpb.TrdAcc) RuntimeAccount {
	accountID := strconv.FormatUint(account.GetAccID(), 10)
	if accountID == "0" {
		accountID = strings.TrimSpace(account.GetCardNum())
		if accountID == "" {
			accountID = strings.TrimSpace(account.GetUniCardNum())
		}
	}

	return RuntimeAccount{
		AccountID:              accountID,
		TradingEnvironment:     runtimeTradingEnvironment(account.GetTrdEnv()),
		AccountType:            requiredRuntimeEnum(account.GetAccType(), trdcommonpb.TrdAccType_name),
		AccountRole:            optionalRuntimeEnum(account.GetAccRole(), trdcommonpb.TrdAccRole_name),
		SecurityFirm:           optionalRuntimeEnum(account.GetSecurityFirm(), trdcommonpb.SecurityFirm_name),
		MarketAuthorities:      runtimeMarketAuthorities(account.GetTrdMarketAuthList()),
		OrderMarketAuthorities: runtimeOrderMarketAuthorities(account.GetTrdMarketAuthList()),
		SimulatedAccountType:   optionalRuntimeEnum(account.GetSimAccType(), trdcommonpb.SimAccType_name),
	}
}

func runtimeTradingEnvironment(value int32) string {
	switch trdcommonpb.TrdEnv(value) {
	case trdcommonpb.TrdEnv_TrdEnv_Real:
		return "REAL"
	case trdcommonpb.TrdEnv_TrdEnv_Simulate:
		return "SIMULATE"
	default:
		return "UNKNOWN"
	}
}

func requiredRuntimeEnum(value int32, names map[int32]string) string {
	if normalized := normalizeRuntimeEnum(enumName(value, names)); normalized != "" {
		return normalized
	}
	return "UNKNOWN"
}

func optionalRuntimeEnum(value int32, names map[int32]string) *string {
	normalized := normalizeRuntimeEnum(enumName(value, names))
	if normalized == "" || normalized == "UNKNOWN" {
		return nil
	}
	return &normalized
}

func normalizeRuntimeEnum(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToUpper(trimmed)
}

func runtimeMarketAuthorities(values []int32) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		market := runtimeMarketAuthority(value)
		if market == "" {
			continue
		}
		if _, exists := seen[market]; exists {
			continue
		}
		seen[market] = struct{}{}
		result = append(result, market)
	}
	return result
}

func runtimeOrderMarketAuthorities(values []int32) []string {
	if values == nil {
		return nil
	}
	filtered := make([]int32, 0, len(values))
	for _, value := range values {
		if isFundOnlyTradeMarket(value) {
			continue
		}
		filtered = append(filtered, value)
	}
	return runtimeMarketAuthorities(filtered)
}

func isFundOnlyTradeMarket(value int32) bool {
	switch trdcommonpb.TrdMarket(value) {
	case trdcommonpb.TrdMarket_TrdMarket_HK_Fund,
		trdcommonpb.TrdMarket_TrdMarket_US_Fund,
		trdcommonpb.TrdMarket_TrdMarket_SG_Fund,
		trdcommonpb.TrdMarket_TrdMarket_JP_Fund,
		trdcommonpb.TrdMarket_TrdMarket_MY_Fund:
		return true
	default:
		return false
	}
}

func runtimeMarketAuthority(value int32) string {
	switch trdcommonpb.TrdMarket(value) {
	case trdcommonpb.TrdMarket_TrdMarket_HK,
		trdcommonpb.TrdMarket_TrdMarket_HKCC,
		trdcommonpb.TrdMarket_TrdMarket_HK_Fund,
		trdcommonpb.TrdMarket_TrdMarket_Futures_Simulate_HK:
		return "HK"
	case trdcommonpb.TrdMarket_TrdMarket_US,
		trdcommonpb.TrdMarket_TrdMarket_US_Fund,
		trdcommonpb.TrdMarket_TrdMarket_Futures_Simulate_US,
		trdcommonpb.TrdMarket_TrdMarket_Prediction:
		return "US"
	case trdcommonpb.TrdMarket_TrdMarket_CN:
		return "CN"
	case trdcommonpb.TrdMarket_TrdMarket_SG,
		trdcommonpb.TrdMarket_TrdMarket_SG_Fund,
		trdcommonpb.TrdMarket_TrdMarket_Futures_Simulate_SG:
		return "SG"
	case trdcommonpb.TrdMarket_TrdMarket_AU:
		return "AU"
	case trdcommonpb.TrdMarket_TrdMarket_JP,
		trdcommonpb.TrdMarket_TrdMarket_JP_Fund,
		trdcommonpb.TrdMarket_TrdMarket_Futures_Simulate_JP:
		return "JP"
	case trdcommonpb.TrdMarket_TrdMarket_MY,
		trdcommonpb.TrdMarket_TrdMarket_MY_Fund:
		return "MY"
	case trdcommonpb.TrdMarket_TrdMarket_CA:
		return "CA"
	case trdcommonpb.TrdMarket_TrdMarket_Crypto:
		return "CRYPTO"
	case trdcommonpb.TrdMarket_TrdMarket_Futures:
		return "FUTURES"
	default:
		return normalizeRuntimeEnum(enumName(value, trdcommonpb.TrdMarket_name))
	}
}
