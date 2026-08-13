package system

import (
	"encoding/json"

	sys "github.com/jftrade/jftrade-main/internal/system"
)

func toSystemStatusResponse(status sys.Status) SystemStatusResponse {
	resources := make([]SystemRuntimeResourceDescriptor, 0, len(status.RuntimeResources.Items))
	for _, resource := range status.RuntimeResources.Items {
		resources = append(resources, SystemRuntimeResourceDescriptor{
			ID: resource.ID, Owner: resource.Owner, Kind: resource.Kind, Path: resource.Path,
			InitializedBy: resource.InitializedBy, SchemaOwner: resource.SchemaOwner,
			CloseOwner: resource.CloseOwner, HealthProvider: resource.HealthProvider,
			EnvironmentOverride: resource.EnvironmentOverride, Critical: resource.Critical,
		})
	}
	return SystemStatusResponse{
		Name: status.Name, APIPort: status.APIPort, DefaultBroker: status.DefaultBroker,
		DefaultTradingEnvironment: status.DefaultTradingEnvironment,
		RealTradingEnabled:        status.RealTradingEnabled,
		RealTradingKillSwitch: SystemRealTradingKillSwitch{
			Active:            status.RealTradingKillSwitch.Active,
			RuntimeActive:     status.RealTradingKillSwitch.RuntimeActive,
			BlockedOperations: status.RealTradingKillSwitch.BlockedOperations,
			AllowsCancel:      status.RealTradingKillSwitch.AllowsCancel,
		},
		RealTradingRisk: SystemRealTradingRisk{
			Enabled:                           status.RealTradingRisk.Enabled,
			MaxOrderQuantity:                  status.RealTradingRisk.MaxOrderQuantity,
			MaxOrderNotional:                  status.RealTradingRisk.MaxOrderNotional,
			RuntimeConfiguredMaxOrderQuantity: status.RealTradingRisk.RuntimeConfiguredMaxOrderQuantity,
			RuntimeConfiguredMaxOrderNotional: status.RealTradingRisk.RuntimeConfiguredMaxOrderNotional,
			RuntimeRiskConfigured:             status.RealTradingRisk.RuntimeRiskConfigured,
		},
		RealTradeAccess: SystemRealTradeAccess{
			ApproverAllowlistEnabled: status.RealTradeAccess.ApproverAllowlistEnabled,
			ApproverCount:            status.RealTradeAccess.ApproverCount,
			AdminAllowlistEnabled:    status.RealTradeAccess.AdminAllowlistEnabled,
			AdminCount:               status.RealTradeAccess.AdminCount,
		},
		Build: SystemBuildInformation{
			Version: status.Build.Version, Commit: status.Build.Commit, BuildTime: status.Build.BuildTime,
			GOOS: status.Build.GOOS, GOARCH: status.Build.GOARCH,
		},
		Persistence: SystemPersistence{
			Engine: status.Persistence.Engine, DatabasePath: status.Persistence.DatabasePath,
			Status: status.Persistence.Status, Migrated: status.Persistence.Migrated,
			PendingMigrations: status.Persistence.PendingMigrations, Tables: status.Persistence.Tables,
			CheckedAt: status.Persistence.CheckedAt,
		},
		Observability: SystemObservability{
			API: projectionMap(status.Observability.API), Live: projectionMap(status.Observability.Live),
			MarketData:        projectionMap(status.Observability.MarketData),
			ExchangeCalendars: projectionMap(status.Observability.ExchangeCalendars),
			Broker:            projectionMap(status.Observability.Broker),
			StrategyRuntime:   projectionMap(status.Observability.StrategyRuntime),
			Requests:          status.Observability.Requests,
		},
		RuntimeResources: SystemRuntimeResources{
			CheckedAt: status.RuntimeResources.CheckedAt, Count: status.RuntimeResources.Count, Items: resources,
		},
		Broker: status.Broker, StrategyRuntime: status.StrategyRuntime, Message: status.Message,
	}
}

func projectionMap(value any) map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var projection map[string]any
	if err := json.Unmarshal(encoded, &projection); err != nil {
		return nil
	}
	return projection
}
