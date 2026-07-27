package servercore

import (
	"context"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/research"
	researchstore "github.com/jftrade/jftrade-main/internal/store/research"
)

func (b *serverBootstrap) loadResearchStore() *researchstore.Store {
	store, err := researchstore.Open(context.Background(), apiruntime.DeriveResearchDBPath(b.settingsPath))
	if err != nil {
		b.recordUnavailable(datamigration.DatabaseResearch, err)
		return nil
	}
	return store
}

func (s *serverApplication) initializeResearchService() {
	if s == nil || s.stores.Research == nil {
		return
	}
	s.researchSvc = research.NewService(s.stores.Research)
}
