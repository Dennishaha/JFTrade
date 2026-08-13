package servercore

import (
	"context"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	"github.com/jftrade/jftrade-main/internal/research"
	researchstore "github.com/jftrade/jftrade-main/internal/store/research"
)

func (b *serverBootstrap) loadResearchStore() *researchstore.Store {
	store, err := researchstore.Open(context.Background(), apiruntime.DeriveResearchDBPath(b.settingsPath))
	if err != nil {
		b.recordUnavailable(dmsrv.DatabaseResearch, err)
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
