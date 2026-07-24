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

func (s *Server) initializeResearchService() {
	if s == nil || s.researchStore == nil {
		return
	}
	s.researchSvc = research.NewService(s.researchStore)
}

func (s *Server) closeResearchStore() error {
	if s == nil || s.researchStore == nil {
		return nil
	}
	return s.researchStore.Close()
}
