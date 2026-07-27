package servercore

import (
	"errors"

	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	appstores "github.com/jftrade/jftrade-main/internal/app/apiserver/stores"
	backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"
	researchstore "github.com/jftrade/jftrade-main/internal/store/research"
	strategystore "github.com/jftrade/jftrade-main/internal/store/strategy"
	watchliststore "github.com/jftrade/jftrade-main/internal/store/watchlist"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

type serverPersistentState struct {
	resources        *appcomposition.Resources
	resourceSetupErr error
	stores           appstores.Handle
	auth             *webAuth
}

func (b *serverBootstrap) loadPersistentState(store SidecarSettingsStore) serverPersistentState {
	state := serverPersistentState{resources: &appcomposition.Resources{}}
	state.stores.Design = appstores.Open(
		&state.stores,
		"strategy design store",
		func() (strategystore.Resource, error) { return b.loadDesignStore(), nil },
		func(resource strategystore.Resource) error { return closeApplicationResource(resource) },
	)
	state.stores.StrategyCatalog = appstores.Open(
		&state.stores,
		"strategy catalog",
		func() (strategystore.CatalogResource, error) { return b.loadStrategyCatalog(), nil },
		func(resource strategystore.CatalogResource) error { return closeApplicationResource(resource) },
	)
	state.stores.BacktestRuns = appstores.Open(
		&state.stores,
		"backtest run store",
		func() (backtestRunStore, error) { return b.loadBacktestRunStore(), nil },
		func(resource backtestRunStore) error { return closeApplicationResource(resource) },
	)
	state.stores.ExecutionOrders = appstores.Open(
		&state.stores,
		"execution order store",
		func() (executionOrderStore, error) {
			return b.loadExecutionOrderStore(store.ExecutionSettings()), nil
		},
		func(resource executionOrderStore) error { return closeApplicationResource(resource) },
	)
	state.stores.Watchlist = appstores.Open(
		&state.stores,
		"watchlist store",
		func() (*watchliststore.Store, error) { return b.loadWatchlistStore(), nil },
		func(resource *watchliststore.Store) error { return closeApplicationResource(resource) },
	)
	state.stores.Research = appstores.Open(
		&state.stores,
		"research store",
		func() (*researchstore.Store, error) { return b.loadResearchStore(), nil },
		func(resource *researchstore.Store) error { return closeApplicationResource(resource) },
	)
	state.stores.BacktestTasks = backteststore.NewSyncTaskStore()
	state.resourceSetupErr = state.stores.SetupError()
	if err := state.resources.Register("persistent stores", state.stores.Close); err != nil {
		state.resourceSetupErr = errors.Join(state.resourceSetupErr, err)
	}
	state.auth = openPersistentResource(
		&state,
		"web authentication",
		func() (*webAuth, error) { return newWebAuth(store.SecuritySettings()), nil },
		func(auth *webAuth) error {
			if auth != nil {
				auth.close()
			}
			return nil
		},
	)
	return state
}

func openPersistentResource[T any](
	state *serverPersistentState,
	name string,
	openFn func() (T, error),
	closeFn func(T) error,
) T {
	if state.resourceSetupErr != nil {
		var zero T
		return zero
	}
	value, err := appcomposition.Open(state.resources, name, openFn, closeFn)
	if err == nil {
		return value
	}
	state.resourceSetupErr = errors.Join(state.resourceSetupErr, err)
	var zero T
	return zero
}

func (b *serverBootstrap) loadStrategyCatalog() strategystore.CatalogResource {
	path := strategystore.DeriveCatalogPath(b.settingsPath)
	store, err := strategystore.NewCatalog(path, strategystore.DerivePluginTargetDir(b.settingsPath))
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseStrategy, err)
		return strategystore.NewUnavailableCatalog(path, strategystore.DerivePluginTargetDir(b.settingsPath))
	}
	return store
}

func (b *serverBootstrap) loadDesignStore() strategystore.Resource {
	path := strategystore.DerivePath(b.settingsPath)
	store, err := strategystore.New(path)
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseStrategy, err)
		return strategystore.NewUnavailable(path)
	}
	return store
}

func (b *serverBootstrap) loadBacktestRunStore() backtestRunStore {
	store, err := backteststore.New(backteststore.DerivePath(b.settingsPath))
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseBacktestRuns, err)
		return backteststore.NewInMemory()
	}
	return store
}

func (b *serverBootstrap) loadExecutionOrderStore(settings jfsettings.ExecutionSettings) executionOrderStore {
	store, err := newExecutionOrderStoreWithDB(deriveExecutionOrderDBPath(b.settingsPath))
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseExecution, err)
		store = newExecutionOrderStore()
	}
	store.ConfigureSeenFillRetention(settings.SeenFillRetentionDays)
	return store
}
