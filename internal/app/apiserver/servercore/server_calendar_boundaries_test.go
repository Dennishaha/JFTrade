package servercore

import (
	"context"
	"testing"

	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/system"
)

func TestServerCalendarOptionsAndOperationsRemainingBoundaries(t *testing.T) {
	//nolint:staticcheck // Exercise the helper's explicit nil-parent fallback.
	ctx, cancel := exchangeCalendarOperationContext(nil)
	if ctx == nil {
		t.Fatal("nil parent did not produce operation context")
	}
	cancel()

	emptyServer := &Server{}
	emptyService := system.NewService(emptyServer.systemCalendarOptions()...)
	if got := emptyService.ExchangeCalendarStatus(); got.Markets != nil || got.Sources != nil {
		t.Fatalf("empty calendar status = %#v", got)
	}
	if got := emptyService.ExchangeCalendarSources(); got != nil {
		t.Fatalf("empty calendar sources = %#v", got)
	}
	if got := emptyService.RefreshExchangeCalendars(context.Background(), "US"); got["accepted"] != false {
		t.Fatalf("empty calendar refresh = %#v", got)
	}
	if got := emptyService.ProbeExchangeCalendars(context.Background(), "US"); got["accepted"] != false {
		t.Fatalf("empty calendar probe = %#v", got)
	}

	manager := exchangecalendar.NewManager(nil, func() jfsettings.ExchangeCalendarSettings { return jfsettings.ExchangeCalendarSettings{} }, exchangecalendar.WithRegistry(exchangecalendar.NewSourceRegistry()))
	server := &Server{}
	server.runtimes.SetExchangeCalendars(manager, nil)
	service := system.NewService(server.systemCalendarOptions()...)
	if got := service.ExchangeCalendarStatus(); len(got.Markets) == 0 {
		t.Fatal("configured calendar status has no markets")
	}
	if got := service.ExchangeCalendarSources(); got == nil {
		t.Fatal("configured calendar sources is nil")
	}
	for _, tc := range []struct {
		name    string
		market  string
		refresh bool
	}{
		{name: "refresh all", refresh: true},
		{name: "probe all"},
		{name: "refresh market", market: "US", refresh: true},
		{name: "probe market", market: "HK"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]any
			if tc.refresh {
				got = service.RefreshExchangeCalendars(context.Background(), tc.market)
			} else {
				got = service.ProbeExchangeCalendars(context.Background(), tc.market)
			}
			if got["accepted"] != true {
				t.Fatalf("calendar operation = %#v", got)
			}
		})
	}
}
