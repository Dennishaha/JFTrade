package application

import (
	"context"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/internal/watchlist"
)

// AssistantSettings is the persisted settings surface consumed while
// composing Assistant tools and runtime limits.
type AssistantSettings interface {
	ADKSettings() jfsettings.ADKRuntimeSettings
	ManagedAccounts() []jfsettings.ManagedBrokerAccount
	Integration() jfsettings.BrokerIntegration
}

// AssistantHealth projects the broker runtime health needed by Assistant
// tools without coupling composition to a concrete integration coordinator.
type AssistantHealth interface {
	OpenDHealth(context.Context) map[string]any
}

// AssistantOptions contains the application-owned dependencies exposed to
// Assistant assembly. Services are stable after bootstrap; their internal
// runtime providers remain dynamic.
type AssistantOptions struct {
	SettingsPath string
	Settings     AssistantSettings
	Runtime      assistantassembly.Runtime
	Health       AssistantHealth

	System          *system.Service
	MarketData      *mdsrv.Service
	Strategy        *stratsrv.Service
	Trading         *trdsrv.Service
	Backtest        *btsrv.Service
	ProductFeatures *productsrv.Service
	Watchlist       *watchlist.Service
}

// OpenAssistant creates the Assistant runtime at the application composition
// boundary. Assistant assembly remains independent of internal/app packages.
func OpenAssistant(options AssistantOptions) (assistantassembly.Runtime, error) {
	return assistantassembly.OpenApplication(assistantassembly.ApplicationOptions{
		Paths: AssistantPaths(options.SettingsPath),
		Ports: AssistantPorts(options),
	})
}

// AssistantPaths derives every Assistant-owned persistent path from the
// sidecar settings location.
func AssistantPaths(settingsPath string) assistantassembly.Paths {
	return assistantassembly.Paths{
		Database: apiruntime.DeriveADKDBPath(settingsPath),
		Session:  apiruntime.DeriveADKSessionDBPath(settingsPath),
		Secrets:  apiruntime.DeriveADKSecretsPath(settingsPath),
		Skills:   apiruntime.DeriveADKSkillsDir(settingsPath),
	}
}

// InspectAssistantRuntimeDatabase checks whether the Assistant configuration
// database can be opened without transferring ownership to the caller.
func InspectAssistantRuntimeDatabase(settingsPath string) assistantassembly.DatabaseProbe {
	return assistantassembly.InspectRuntimeDatabase(AssistantPaths(settingsPath))
}

// InspectAssistantSessionDatabase checks whether the Assistant session
// database can be opened without transferring ownership to the caller.
func InspectAssistantSessionDatabase(settingsPath string) assistantassembly.DatabaseProbe {
	return assistantassembly.InspectSessionDatabase(AssistantPaths(settingsPath))
}

// AssistantPorts adapts application services to the provider functions used
// by Assistant assembly. It is exported so application-level contract tests
// can exercise the same boundary without constructing the persistent runtime.
func AssistantPorts(options AssistantOptions) assistantassembly.ApplicationPorts {
	return assistantassembly.ApplicationPorts{
		Runtime:         func() assistantassembly.Runtime { return options.Runtime },
		System:          func() *system.Service { return options.System },
		MarketData:      func() *mdsrv.Service { return options.MarketData },
		Strategy:        func() *stratsrv.Service { return options.Strategy },
		Trading:         func() *trdsrv.Service { return options.Trading },
		Backtest:        func() *btsrv.Service { return options.Backtest },
		ProductFeatures: func() *productsrv.Service { return options.ProductFeatures },
		Watchlist:       func() *watchlist.Service { return options.Watchlist },
		RuntimeSettings: func() jfsettings.ADKRuntimeSettings {
			if options.Settings == nil {
				return jfsettings.ADKRuntimeSettings{}
			}
			return options.Settings.ADKSettings()
		},
		ManagedAccounts: func() []jfsettings.ManagedBrokerAccount {
			if options.Settings == nil {
				return nil
			}
			return options.Settings.ManagedAccounts()
		},
		BrokerIntegration: func() jfsettings.BrokerIntegration {
			if options.Settings == nil {
				return jfsettings.BrokerIntegration{}
			}
			return options.Settings.Integration()
		},
		FutuOpenDHealth: func(ctx context.Context) (any, error) {
			if options.Health == nil {
				return map[string]any{"status": "unavailable"}, nil
			}
			return options.Health.OpenDHealth(ctx), nil
		},
	}
}
