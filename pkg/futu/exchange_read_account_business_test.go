package futu

import (
	"errors"
	"io"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

func TestRecoverableOpenDErrClassifiesConnectionFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "closed sentinel", err: opend.ErrClosed, want: true},
		{name: "wrapped timeout sentinel", err: errors.Join(errors.New("retry"), opend.ErrRequestTimeout), want: true},
		{name: "broken pipe", err: errors.New("write tcp 127.0.0.1:1234: broken pipe"), want: true},
		{name: "connection reset", err: errors.New("read tcp: connection reset by peer"), want: true},
		{name: "eof", err: io.EOF, want: true},
		{name: "closed network", err: errors.New("use of closed network connection"), want: true},
		{name: "business denial", err: errors.New("permission denied"), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRecoverableOpenDErr(tc.err); got != tc.want {
				t.Fatalf("isRecoverableOpenDErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestResolveTradeMarketHonorsRequestedAuthorityAndFallbacks(t *testing.T) {
	account := &trdcommonpb.TrdAcc{
		TrdMarketAuthList: []int32{
			int32(trdcommonpb.TrdMarket_TrdMarket_HK),
			int32(trdcommonpb.TrdMarket_TrdMarket_US),
		},
	}

	market, code, ok, err := resolveTradeMarket(account, " us ")
	if err != nil || !ok {
		t.Fatalf("resolveTradeMarket requested US = err %v ok %v, want match", err, ok)
	}
	if market != "US" || code != int32(trdcommonpb.TrdMarket_TrdMarket_US) {
		t.Fatalf("requested US resolved to %q/%d", market, code)
	}

	if _, _, ok, err := resolveTradeMarket(account, "CN"); err != nil || ok {
		t.Fatalf("unauthorized CN = ok %v err %v, want no match without error", ok, err)
	}

	market, code, ok, err = resolveTradeMarket(&trdcommonpb.TrdAcc{}, "jp")
	if err != nil || !ok {
		t.Fatalf("resolveTradeMarket no-auth JP = err %v ok %v, want direct market mapping", err, ok)
	}
	if market != "JP" || code != int32(trdcommonpb.TrdMarket_TrdMarket_JP) {
		t.Fatalf("no-auth JP resolved to %q/%d", market, code)
	}

	if _, _, _, err := resolveTradeMarket(&trdcommonpb.TrdAcc{}, "MARS"); err == nil {
		t.Fatalf("unsupported no-auth market did not fail")
	}

	market, code, ok, err = resolveTradeMarket(&trdcommonpb.TrdAcc{
		TrdMarketAuthList: []int32{
			999999,
			int32(trdcommonpb.TrdMarket_TrdMarket_HK),
		},
	}, "")
	if err != nil || !ok || market != "HK" || code != int32(trdcommonpb.TrdMarket_TrdMarket_HK) {
		t.Fatalf("first valid authority resolved to %q/%d ok=%v err=%v", market, code, ok, err)
	}

	market, code, ok, err = resolveTradeMarket(nil, "")
	if err != nil || !ok || market != "HK" || code != int32(trdcommonpb.TrdMarket_TrdMarket_HK) {
		t.Fatalf("nil-account default resolved to %q/%d ok=%v err=%v", market, code, ok, err)
	}
}

func TestCandidateTradeAccountFromProtoFiltersAndBuildsHeader(t *testing.T) {
	if candidate, ok, err := candidateTradeAccountFromProto(nil, BrokerReadQuery{}); err != nil || ok || candidate.AccountID != "" {
		t.Fatalf("nil candidate = %#v ok=%v err=%v, want no match", candidate, ok, err)
	}

	account := &trdcommonpb.TrdAcc{
		TrdEnv: futuTestPtr(int32(trdcommonpb.TrdEnv_TrdEnv_Simulate)),
		AccID:  new(uint64(9001)),
		TrdMarketAuthList: []int32{
			int32(trdcommonpb.TrdMarket_TrdMarket_US),
			int32(trdcommonpb.TrdMarket_TrdMarket_HK),
		},
		AccType: futuTestPtr(int32(trdcommonpb.TrdAccType_TrdAccType_Margin)),
	}

	candidate, ok, err := candidateTradeAccountFromProto(account, BrokerReadQuery{
		AccountID:          "9001",
		TradingEnvironment: "simulate",
		Market:             "us",
	})
	if err != nil || !ok {
		t.Fatalf("candidateTradeAccountFromProto = err %v ok %v, want match", err, ok)
	}
	if candidate.AccountID != "9001" || candidate.TradingEnvironment != "SIMULATE" || candidate.Market != "US" || candidate.AccountType != "MARGIN" {
		t.Fatalf("candidate normalized fields = %#v", candidate)
	}
	header := candidate.header()
	if header.GetAccID() != 9001 ||
		header.GetTrdEnv() != int32(trdcommonpb.TrdEnv_TrdEnv_Simulate) ||
		header.GetTrdMarket() != int32(trdcommonpb.TrdMarket_TrdMarket_US) {
		t.Fatalf("trade header = %#v", header)
	}

	if _, ok, err := candidateTradeAccountFromProto(account, BrokerReadQuery{AccountID: "other"}); err != nil || ok {
		t.Fatalf("account-id mismatch = ok %v err %v, want filtered", ok, err)
	}
	if _, ok, err := candidateTradeAccountFromProto(account, BrokerReadQuery{TradingEnvironment: "REAL"}); err != nil || ok {
		t.Fatalf("environment mismatch = ok %v err %v, want filtered", ok, err)
	}
	if _, ok, err := candidateTradeAccountFromProto(account, BrokerReadQuery{Market: "CN"}); err != nil || ok {
		t.Fatalf("market mismatch = ok %v err %v, want filtered", ok, err)
	}

	cardAccount := &trdcommonpb.TrdAcc{
		TrdEnv:            futuTestPtr(int32(trdcommonpb.TrdEnv_TrdEnv_Real)),
		AccID:             new(uint64(0)),
		CardNum:           new("CARD-9002"),
		TrdMarketAuthList: []int32{int32(trdcommonpb.TrdMarket_TrdMarket_HK)},
	}
	candidate, ok, err = candidateTradeAccountFromProto(cardAccount, BrokerReadQuery{AccountID: "card-9002"})
	if err != nil || !ok || candidate.AccountID != "CARD-9002" || candidate.TradingEnvironment != "REAL" {
		t.Fatalf("card-number account candidate = %#v ok=%v err=%v", candidate, ok, err)
	}
}

func TestBrokerReadQueryNormalizationAndAccountPriority(t *testing.T) {
	normalized := normalizeBrokerReadQuery(BrokerReadQuery{
		AccountID:          " 9001 ",
		TradingEnvironment: " simulate ",
		Market:             " us ",
	})
	if normalized.AccountID != "9001" || normalized.TradingEnvironment != "SIMULATE" || normalized.Market != "US" {
		t.Fatalf("normalized query = %#v", normalized)
	}

	candidates := []resolvedTradeAccount{
		{AccountID: "1", TradingEnvironment: "REAL"},
		{AccountID: "2", TradingEnvironment: "SIMULATE"},
		{AccountID: "3", TradingEnvironment: "UNKNOWN"},
	}
	filtered := filterResolvedTradeAccountsByEnvironment(candidates, "simulate")
	if len(filtered) != 1 || filtered[0].AccountID != "2" {
		t.Fatalf("simulate filtered candidates = %#v", filtered)
	}
	if resolvedTradeAccountPriority(candidates[1]) >= resolvedTradeAccountPriority(candidates[0]) {
		t.Fatalf("simulate account should be preferred over real when no environment is requested")
	}
	if resolvedTradeAccountPriority(candidates[2]) <= resolvedTradeAccountPriority(candidates[0]) {
		t.Fatalf("unknown account should sort after real accounts")
	}
}

func TestTrdMarketFromNormalizedCoversSupportedMarkets(t *testing.T) {
	cases := map[string]trdcommonpb.TrdMarket{
		" hk ":    trdcommonpb.TrdMarket_TrdMarket_HK,
		"US":      trdcommonpb.TrdMarket_TrdMarket_US,
		"cn":      trdcommonpb.TrdMarket_TrdMarket_CN,
		"SG":      trdcommonpb.TrdMarket_TrdMarket_SG,
		"au":      trdcommonpb.TrdMarket_TrdMarket_AU,
		"JP":      trdcommonpb.TrdMarket_TrdMarket_JP,
		"my":      trdcommonpb.TrdMarket_TrdMarket_MY,
		"CA":      trdcommonpb.TrdMarket_TrdMarket_CA,
		"crypto":  trdcommonpb.TrdMarket_TrdMarket_Crypto,
		"futures": trdcommonpb.TrdMarket_TrdMarket_Futures,
	}
	for raw, want := range cases {
		got, ok := trdMarketFromNormalized(raw)
		if !ok || got != want {
			t.Fatalf("trdMarketFromNormalized(%q) = %v/%v, want %v/true", raw, got, ok, want)
		}
	}

	if got, ok := trdMarketFromNormalized("MARS"); ok || got != 0 {
		t.Fatalf("unknown market = %v/%v, want zero/false", got, ok)
	}
}
