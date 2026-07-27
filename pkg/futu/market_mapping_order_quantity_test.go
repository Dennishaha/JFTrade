package futu

import (
	"context"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	qotcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon"
)

func TestFutuMarketMappingsCoverEverySupportedQuotePrefix(t *testing.T) {
	cases := []struct {
		prefix string
		market qotcommonpb.QotMarket
		code   string
	}{
		{prefix: "HK", market: qotcommonpb.QotMarket_QotMarket_HK_Security, code: "HK"},
		{prefix: "US", market: qotcommonpb.QotMarket_QotMarket_US_Security, code: "US"},
		{prefix: "SH", market: qotcommonpb.QotMarket_QotMarket_CNSH_Security, code: "SH"},
		{prefix: "CNSH", market: qotcommonpb.QotMarket_QotMarket_CNSH_Security, code: "SH"},
		{prefix: "SZ", market: qotcommonpb.QotMarket_QotMarket_CNSZ_Security, code: "SZ"},
		{prefix: "CNSZ", market: qotcommonpb.QotMarket_QotMarket_CNSZ_Security, code: "SZ"},
		{prefix: "SG", market: qotcommonpb.QotMarket_QotMarket_SG_Security, code: "SG"},
		{prefix: "JP", market: qotcommonpb.QotMarket_QotMarket_JP_Security, code: "JP"},
		{prefix: "AU", market: qotcommonpb.QotMarket_QotMarket_AU_Security, code: "AU"},
		{prefix: "MY", market: qotcommonpb.QotMarket_QotMarket_MY_Security, code: "MY"},
		{prefix: "CA", market: qotcommonpb.QotMarket_QotMarket_CA_Security, code: "CA"},
	}
	for _, tc := range cases {
		t.Run(tc.prefix, func(t *testing.T) {
			gotMarket, err := futuQotMarketForCode(" " + strings.ToLower(tc.prefix) + " ")
			if err != nil || gotMarket != tc.market {
				t.Fatalf("futuQotMarketForCode(%q) = %s, %v; want %s, nil", tc.prefix, gotMarket, err, tc.market)
			}
			gotCode, err := futuMarketCodeFromQotMarket(tc.market)
			if err != nil || gotCode != tc.code {
				t.Fatalf("futuMarketCodeFromQotMarket(%s) = %q, %v; want %q, nil", tc.market, gotCode, err, tc.code)
			}
		})
	}
	if _, err := futuQotMarketForCode("EU"); err == nil {
		t.Fatal("unsupported quote market error = nil")
	}
	if _, err := futuSymbolFromSecurity(&qotcommonpb.Security{
		Market: new(int32(qotcommonpb.QotMarket_QotMarket_Unknown)),
		Code:   new("AAPL"),
	}); err == nil {
		t.Fatal("unknown security market error = nil")
	}
	if _, _, err := futuSecurityFromSymbol("EU.SAP"); err == nil {
		t.Fatal("unsupported symbol prefix error = nil")
	}
}

func TestBrokerOrderQuantityValidationAndEmptyCancellationAreSafe(t *testing.T) {
	market := types.Market{
		Symbol:      "HK.00700",
		MinQuantity: fixedpoint.NewFromInt(100),
		StepSize:    fixedpoint.NewFromInt(100),
	}
	cases := []struct {
		name     string
		quantity fixedpoint.Value
		wantErr  string
	}{
		{name: "non-positive", quantity: fixedpoint.Zero, wantErr: "positive"},
		{name: "below market minimum", quantity: fixedpoint.NewFromInt(10), wantErr: "less than"},
		{name: "wrong lot step", quantity: fixedpoint.NewFromInt(150), wantErr: "step"},
		{name: "valid lot", quantity: fixedpoint.NewFromInt(200)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSubmitOrderQuantityAgainstMarket(types.SubmitOrder{Symbol: market.Symbol, Quantity: tc.quantity}, market)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSubmitOrderQuantityAgainstMarket() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateSubmitOrderQuantityAgainstMarket() error = %v, want %q", err, tc.wantErr)
			}
		})
	}

	exchange := NewExchange("")
	if err := exchange.CancelBrokerOrders(context.Background(), BrokerReadQuery{}); err != nil {
		t.Fatalf("CancelBrokerOrders(empty) error = %v", err)
	}

	placed := placedOrderFromSubmitOrder(types.SubmitOrder{
		Symbol:   "US.AAPL",
		Side:     types.SideTypeBuy,
		Type:     types.OrderTypeLimit,
		Quantity: fixedpoint.NewFromInt(1),
	}, 123)
	if placed.Market.Symbol != "US.AAPL" || placed.Market.Exchange != Name || placed.OrderID != 123 {
		t.Fatalf("placed order fallback market = %#v", placed)
	}
}
