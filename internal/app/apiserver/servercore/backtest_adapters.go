package servercore

import (
	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

func toSrvStrategyDef(definition stratsrv.Definition) btsrv.StrategyDef {
	return btsrv.StrategyDef{
		ID:           definition.ID,
		Version:      definition.Version,
		SourceFormat: definition.SourceFormat,
		Script:       definition.Script,
	}
}

// strategyProviderAdapter narrows the strategy domain store to the fields a
// backtest needs to compile one definition.
type strategyProviderAdapter struct {
	store stratsrv.DesignStore
}

func (a *strategyProviderAdapter) Definition(id string) (btsrv.StrategyDef, bool, error) {
	definition, ok, err := a.store.GetDefinition(id)
	if err != nil || !ok {
		return btsrv.StrategyDef{}, ok, err
	}
	return toSrvStrategyDef(definition), true, nil
}
