package catalog

import (
	"fmt"
	"strings"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	instancebinding "github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
	instanceview "github.com/jftrade/jftrade-main/internal/strategy/instanceview"
)

func (s *Service) TransitionInstance(instanceID, nextStatus string) (stratsrv.InstanceView, error) {
	return s.TransitionRuntime(instanceID, nextStatus, statusKind(nextStatus), statusDetail(nextStatus))
}

func (s *Service) TransitionRuntime(instanceID, nextStatus, kind, detail string) (stratsrv.InstanceView, error) {
	now := time.Now().UTC()
	s.mu.Lock()
	item, err := s.updateInstanceLocked(instanceID, func(instance *stratsrv.ManagedInstance) error {
		instance.Status = nextStatus
		s.recordEventsLocked(
			instance,
			now,
			fmt.Sprintf("%s strategy %s", strings.ToLower(kind), instance.Definition.StrategyID),
			logLevelForKind(kind, detail),
			"control",
			kind,
			detail,
		)
		return nil
	})
	s.mu.Unlock()
	if err != nil {
		return stratsrv.InstanceView{}, err
	}
	return s.enrichInstance(item), nil
}

func (s *Service) AppendRuntimeEvent(instanceID, message, kind, detail string) error {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		instance := s.normalizeStrategy(s.data.Strategies[index])
		if instance.ID != instanceID {
			continue
		}
		s.recordEventsLocked(
			&instance,
			now,
			message,
			logLevelForKind(kind, message),
			"runtime",
			kind,
			detail,
		)
		return nil
	}
	return stratsrv.NotFoundError("strategy resource not found")
}

func (s *Service) ReconcileRuntimeFailure(instanceID, detail string) error {
	now := time.Now().UTC()
	detail = strings.TrimSpace(detail)
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		instance := s.normalizeStrategy(s.data.Strategies[index])
		if instance.ID != instanceID {
			continue
		}
		if instance.Status != StatusRunning {
			return nil
		}
		instance.Status = StatusStopped
		s.recordEventsLocked(
			&instance,
			now,
			fmt.Sprintf("strategy runtime exited unexpectedly: %s", detail),
			"error",
			"runtime",
			"runtime_exited",
			detail,
		)
		s.data.Strategies[index] = instance
		return s.persistLocked()
	}
	return stratsrv.NotFoundError("strategy resource not found")
}

func (s *Service) ReconcileOnStartup() (int, error) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := 0
	for index := range s.data.Strategies {
		instance := s.normalizeStrategy(s.data.Strategies[index])
		if instance.Status != StatusRunning && instance.Status != StatusPaused {
			continue
		}
		previous := instance.Status
		instance.Status = StatusStopped
		s.recordEventsLocked(
			&instance,
			now,
			fmt.Sprintf("reconciled strategy state from %s to %s after server startup", previous, StatusStopped),
			"warning",
			"startup",
			"reconciled",
			fmt.Sprintf("server startup reset stale %s state to %s", strings.ToLower(previous), StatusStopped),
		)
		s.data.Strategies[index] = instance
		changed++
	}
	if changed == 0 {
		return 0, nil
	}
	if err := s.persistLocked(); err != nil {
		return 0, err
	}
	return changed, nil
}

func (s *Service) RefreshDefinition(instanceID string, definition stratsrv.Definition) (stratsrv.InstanceView, error) {
	now := time.Now().UTC()
	params, err := buildInstanceParams(definition, now.Format(time.RFC3339Nano))
	if err != nil {
		return stratsrv.InstanceView{}, err
	}
	s.mu.Lock()
	item, err := s.updateInstanceLocked(instanceID, func(instance *stratsrv.ManagedInstance) error {
		_, refreshErr := s.refreshDefinitionLocked(instance, definition, params, now)
		return refreshErr
	})
	s.mu.Unlock()
	if err != nil {
		return stratsrv.InstanceView{}, err
	}
	return s.enrichInstance(item), nil
}

func (s *Service) RefreshInstanceDefinition(instanceID string) (stratsrv.InstanceView, error) {
	instance, ok := s.GetInstance(instanceID)
	if !ok {
		return stratsrv.InstanceView{}, stratsrv.NotFoundError("strategy instance not found")
	}
	s.mu.RLock()
	definitions := s.definitions
	s.mu.RUnlock()
	if definitions == nil {
		return stratsrv.InstanceView{}, stratsrv.NotFoundError("strategy definition not found")
	}
	definitionID := strings.TrimSpace(instance.Definition.StrategyID)
	definition, exists, err := definitions.GetDefinition(definitionID)
	if err != nil {
		return stratsrv.InstanceView{}, fmt.Errorf("get definition %s: %w", definitionID, err)
	}
	if !exists {
		return stratsrv.InstanceView{}, stratsrv.NotFoundError("strategy definition not found")
	}
	return s.RefreshDefinition(instanceID, definition)
}

func (s *Service) ApplyDefinitionToLinked(definition stratsrv.Definition) (stratsrv.ApplyLinkedInstancesResult, error) {
	now := time.Now().UTC()
	params, err := buildInstanceParams(definition, now.Format(time.RFC3339Nano))
	if err != nil {
		return stratsrv.ApplyLinkedInstancesResult{}, err
	}
	result := stratsrv.ApplyLinkedInstancesResult{
		DefinitionID:  strings.TrimSpace(definition.ID),
		LatestVersion: strings.TrimSpace(definition.Version),
		Applied:       []string{},
		AlreadyLatest: []string{},
		SkippedBusy:   []string{},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for index := range s.data.Strategies {
		instance := s.normalizeStrategy(s.data.Strategies[index])
		if !instanceUsesDefinition(instance, definition.ID) {
			continue
		}
		result.TotalLinked++
		if instance.Status != StatusStopped {
			result.SkippedBusy = append(result.SkippedBusy, instance.ID)
			continue
		}
		if strings.TrimSpace(instance.Definition.Version) == strings.TrimSpace(definition.Version) {
			result.AlreadyLatest = append(result.AlreadyLatest, instance.ID)
			continue
		}
		_, _ = s.refreshDefinitionLocked(&instance, definition, params, now)
		s.data.Strategies[index] = instance
		result.Applied = append(result.Applied, instance.ID)
		changed = true
	}
	if changed {
		if err := s.persistLocked(); err != nil {
			return stratsrv.ApplyLinkedInstancesResult{}, err
		}
	}
	return result, nil
}

func (s *Service) refreshDefinitionLocked(
	instance *stratsrv.ManagedInstance,
	definition stratsrv.Definition,
	params map[string]any,
	at time.Time,
) (bool, error) {
	if instance == nil {
		return false, nil
	}
	if instance.Status != StatusStopped {
		return false, stratsrv.BusyError("strategy instance must be stopped before modification")
	}
	if strings.TrimSpace(instance.Definition.Version) == strings.TrimSpace(definition.Version) {
		return false, nil
	}
	previousVersion := strings.TrimSpace(instance.Definition.Version)
	instance.PluginID = instanceview.PluginIDForDefinition(definition)
	instance.Definition = stratsrv.DefinitionSummary{
		StrategyID: strings.TrimSpace(definition.ID),
		Name:       strings.TrimSpace(definition.Name),
		Version:    strings.TrimSpace(definition.Version),
	}
	instance.Params = copyMap(params)
	instance.Binding = instancebinding.NormalizeBinding(instance.Binding, instance.Params)
	instancebinding.ApplyParams(instance)
	s.recordEventsLocked(
		instance,
		at,
		fmt.Sprintf("refreshed strategy definition %s to v%s", definition.ID, definition.Version),
		"info",
		"control",
		"definition.refreshed",
		fmt.Sprintf("%s | %s -> %s", strings.TrimSpace(definition.ID), previousVersion, strings.TrimSpace(definition.Version)),
	)
	return true, nil
}

func statusKind(status string) string {
	switch status {
	case StatusRunning:
		return "started"
	case StatusPaused:
		return "paused"
	case StatusStopped:
		return "stopped"
	default:
		return status
	}
}

func statusDetail(status string) string {
	switch status {
	case StatusRunning:
		return "strategy runtime requested start"
	case StatusPaused:
		return "manual pause"
	case StatusStopped:
		return "manual stop"
	default:
		return "status transition"
	}
}
