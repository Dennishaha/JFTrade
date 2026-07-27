package catalog

import (
	"context"
	"log"
	"strings"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/runtimecontrol"
)

const listLogsTailSize = 20

func (s *Service) enrichInstance(item stratsrv.InstanceView) stratsrv.InstanceView {
	s.mu.RLock()
	definitions := s.definitions
	observationSource := s.observationSource
	s.mu.RUnlock()
	item.DefinitionSync = buildDefinitionSyncStatus(item, definitions)
	if observationSource != nil {
		if observation, ok := observationSource.GetObservation(item.ID); ok {
			item.RuntimeObservation = &observation
		}
	}
	if item.RuntimeObservation == nil && s.activity != nil {
		snapshot, ok, err := s.activity.GetObservation(context.Background(), item.ID)
		if err != nil {
			log.Printf("JFTrade load persisted strategy runtime observation degraded: %v", err)
		} else if ok {
			observation := runtimecontrol.ObservationFromSnapshot(snapshot, item.Status, StatusStopped)
			item.RuntimeObservation = &stratsrv.RuntimeObservation{
				ActualStatus:      observation.ActualStatus,
				ActiveSymbols:     observation.ActiveSymbols,
				LastClosedKLineAt: observation.LastClosedKLineAt,
				LastSignalAt:      observation.LastSignalAt,
				LastOrderAt:       observation.LastOrderAt,
				LastErrorAt:       observation.LastErrorAt,
				LastError:         observation.LastError,
				UpdatedAt:         observation.UpdatedAt,
			}
		}
	}
	if s.activity != nil {
		persisted, err := s.activity.ListRecentLogsTail(context.Background(), item.ID, listLogsTailSize)
		if err != nil {
			log.Printf("JFTrade load persisted strategy list logs degraded: %v", err)
		} else if len(persisted) > 0 {
			item.Logs = make([]string, 0, len(persisted))
			for _, event := range persisted {
				item.Logs = append(item.Logs, event.Raw)
			}
		}
	}
	return item
}

func buildDefinitionSyncStatus(item stratsrv.InstanceView, definitions DefinitionStore) *stratsrv.DefinitionSyncStatus {
	definitionID := strings.TrimSpace(item.Definition.StrategyID)
	if definitionID == "" {
		definitionID = definitionIDFromParams(item.Params)
	}
	if definitionID == "" {
		return nil
	}
	appliedVersion := strings.TrimSpace(item.Definition.Version)
	status := &stratsrv.DefinitionSyncStatus{
		DefinitionID:   definitionID,
		AppliedVersion: appliedVersion,
		LatestVersion:  appliedVersion,
		IsLatest:       true,
	}
	if definitions == nil {
		return status
	}
	definition, ok, err := definitions.GetDefinition(definitionID)
	if err != nil || !ok {
		return status
	}
	status.LatestVersion = strings.TrimSpace(definition.Version)
	status.IsLatest = status.AppliedVersion == status.LatestVersion
	if status.IsLatest {
		return status
	}
	status.CanApplyLatest = item.Status == StatusStopped
	if !status.CanApplyLatest {
		status.BlockedReason = new("当前实例不是 STOPPED，先停止后才能刷新到最新策略。")
	}
	return status
}
