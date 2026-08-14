package marketdataapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	settingssrv "github.com/jftrade/jftrade-main/internal/settings"
)

type AssistantProviderPorts struct {
	MarketProviders      func(context.Context) (any, error)
	SelectMarketProvider func(context.Context, string, string) (any, error)
}

func NewAssistantProviderPorts(service *mdsrv.Service, settingsService *settingssrv.Service) AssistantProviderPorts {
	return AssistantProviderPorts{
		MarketProviders: func(ctx context.Context) (any, error) {
			return AssistantMarketProviders(ctx, service, settingsService)
		},
		SelectMarketProvider: func(ctx context.Context, scope, providerID string) (any, error) {
			return SelectAssistantMarketProvider(ctx, service, settingsService, scope, providerID)
		},
	}
}

// AssistantMarketProviders reports provider selection and active-provider
// health without acquiring or starting an inactive sidecar.
func AssistantMarketProviders(
	ctx context.Context,
	service *mdsrv.Service,
	settingsService *settingssrv.Service,
) (any, error) {
	if service == nil || settingsService == nil {
		return nil, fmt.Errorf("market provider services are unavailable")
	}
	backtest, err := settingsService.GetBacktestMarketDataProvider(ctx)
	if err != nil {
		return nil, err
	}
	runtime := RuntimeFromService(service)
	liveProvider := ""
	if runtime != nil {
		liveProvider = runtime.ActiveProviderID()
	}
	liveStatus, statusErr := service.ProviderStatus(ctx)
	providers := make([]any, 0, len(backtest.AvailableProviders))
	for _, descriptor := range backtest.AvailableProviders {
		providers = append(providers, descriptor)
	}
	result := map[string]any{
		"liveProvider":     liveProvider,
		"backtestProvider": string(backtest.ActiveProvider),
		"providers":        providers,
		"checkedAt":        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if statusErr != nil {
		result["liveHealth"] = map[string]any{"status": "unknown", "error": statusErr.Error()}
	} else {
		result["liveHealth"] = liveStatus.Health
		result["liveRuntime"] = liveStatus.Runtime
	}
	return result, nil
}

// SelectAssistantMarketProvider persists a global live or backtest selection;
// the settings service owns activation and rollback for each scope.
func SelectAssistantMarketProvider(
	ctx context.Context,
	service *mdsrv.Service,
	settingsService *settingssrv.Service,
	scope string,
	providerID string,
) (any, error) {
	if service == nil || settingsService == nil {
		return nil, fmt.Errorf("market provider settings are unavailable")
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if providerID == "" {
		return nil, fmt.Errorf("providerId is required")
	}
	before, err := AssistantMarketProviders(ctx, service, settingsService)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "live":
		selected, saveErr := settingsService.SaveActiveMarketDataProvider(jfsettings.ActiveMarketDataProvider(providerID))
		if saveErr != nil {
			return map[string]any{"scope": "live", "providerId": providerID, "before": before, "error": saveErr.Error(), "rolledBack": true}, saveErr
		}
		after, err := AssistantMarketProviders(ctx, service, settingsService)
		if err != nil {
			return nil, err
		}
		return map[string]any{"scope": "live", "providerId": string(selected), "before": before, "after": after}, nil
	case "backtest":
		selected, saveErr := settingsService.SaveBacktestMarketDataProvider(jfsettings.ActiveMarketDataProvider(providerID))
		if saveErr != nil {
			return map[string]any{"scope": "backtest", "providerId": providerID, "before": before, "error": saveErr.Error(), "rolledBack": true}, saveErr
		}
		after, err := AssistantMarketProviders(ctx, service, settingsService)
		if err != nil {
			return nil, err
		}
		return map[string]any{"scope": "backtest", "providerId": string(selected.ActiveProvider), "before": before, "after": after}, nil
	default:
		return nil, fmt.Errorf("scope must be live or backtest")
	}
}
