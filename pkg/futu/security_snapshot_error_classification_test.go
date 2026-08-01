package futu

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

func TestMarketDataReaderClassifiesSymbolScopedSecuritySnapshotProtocolErrors(t *testing.T) {
	server, exchange := coverageMarginExchange(t)
	reader := &futuMarketDataReader{exchange: exchange}
	tests := []struct {
		name         string
		retType      int32
		errCode      int32
		message      string
		symbolScoped bool
	}{
		{name: "Chinese unknown stock", retType: -1, message: "未知股票 BBKCF", symbolScoped: true},
		{name: "Chinese unknown security", retType: -1, message: "未知证券 BBKCF", symbolScoped: true},
		{name: "English unknown stock", retType: -1, message: "unknown stock BBKCF", symbolScoped: true},
		{name: "English unknown security", retType: -1, message: "unknown security BBKCF", symbolScoped: true},
		{name: "Chinese US OTC unavailable", retType: -1, message: "暂不提供美股 OTC 市场行情 KXIAY", symbolScoped: true},
		{name: "English US OTC unavailable", retType: -1, message: "US OTC market quote is unavailable for KXIAY", symbolScoped: true},
		{name: "English US OTC market unavailable", retType: -1, message: "US OTC market is unavailable for KXIAY", symbolScoped: true},
		{name: "unknown security entitlement", retType: -1, message: "unknown security because entitlement is unavailable"},
		{name: "entitlement", retType: -1, errCode: 403, message: "snapshot entitlement denied"},
		{name: "entitlement without error code", retType: -1, message: "US OTC market quote is unavailable because entitlement"},
		{name: "generic service", retType: -1, message: "snapshot service unavailable"},
		{name: "session", retType: -1, message: "OpenD session unavailable"},
		{name: "rate limit", retType: -1, message: "frequency too high"},
		{name: "unexpected ret type", retType: 1, message: "unknown stock BBKCF"},
		{name: "unexpected error code", retType: -1, errCode: 1, message: "unknown stock BBKCF"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server.setSecuritySnapshotError(test.retType, test.errCode, test.message)
			_, err := reader.QuerySecuritySnapshot(t.Context(), broker.SecuritySnapshotQuery{Symbols: []string{"US.AAPL"}})
			if err == nil {
				t.Fatal("QuerySecuritySnapshot error = nil")
			}
			want := fmt.Sprintf(
				"opend GetSecuritySnapshot retType=%d errCode=%d retMsg=%s",
				test.retType,
				test.errCode,
				test.message,
			)
			if err.Error() != want {
				t.Fatalf("QuerySecuritySnapshot error = %q, want %q", err, want)
			}
			if got := broker.IsSymbolScopedSnapshotError(err); got != test.symbolScoped {
				t.Fatalf("IsSymbolScopedSnapshotError(%v) = %v, want %v", err, got, test.symbolScoped)
			}
		})
	}
}

func TestSecuritySnapshotTransportAndCanceledErrorsAreNotSymbolScoped(t *testing.T) {
	exchange := NewExchangeWithConfig(opend.Config{Addr: "127.0.0.1:1", RequestTimeout: 50 * time.Millisecond})
	t.Cleanup(func() { jftradeCheckTestError(t, exchange.Close()) })

	_, err := exchange.querySecuritySnapshotListDirect(t.Context(), []string{"US.AAPL"})
	if err == nil {
		t.Fatal("unreachable OpenD error = nil")
	}
	if broker.IsSymbolScopedSnapshotError(err) {
		t.Fatalf("transport error marked symbol scoped: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = exchange.querySecuritySnapshotListDirect(ctx, []string{"US.AAPL"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request error = %v", err)
	}
	if broker.IsSymbolScopedSnapshotError(err) {
		t.Fatalf("canceled error marked symbol scoped: %v", err)
	}
}
