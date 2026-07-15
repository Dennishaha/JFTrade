package servercore

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func (s *strategyCatalogStore) instantiateStrategy(definition strategyDesignDefinition, binding strategyInstanceBinding) (strategyListItem, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	params, err := buildStrategyInstanceParams(definition, now)
	if err != nil {
		return strategyListItem{}, err
	}
	binding = normalizeStrategyInstanceBinding(binding, params)
	instance := managedStrategyInstance{
		ID:       buildStrategyInstanceID(definition.ID),
		PluginID: strategyPluginIDForDefinition(definition),
		Definition: strategyDefinitionSummary{
			StrategyID: definition.ID,
			Name:       definition.Name,
			Version:    definition.Version,
		},
		Binding:   binding,
		Params:    params,
		Status:    strategyStatusStopped,
		CreatedAt: now,
	}
	s.recordStrategyEventsLocked(&instance, time.Now().UTC(), fmt.Sprintf("instantiated strategy from definition %s", definition.ID), "info", "control", "instantiated", strategyBindingAuditDetail(definition.ID, binding))
	if err := s.saveStrategy(instance); err != nil {
		return strategyListItem{}, err
	}
	return strategyToListItem(s.normalizeStrategy(instance)), nil
}

func (s *strategyCatalogStore) updateStrategyBinding(instanceID string, binding strategyInstanceBinding) (strategyListItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		if strategy.Status != strategyStatusStopped {
			return strategyListItem{}, errStrategyInstanceBusy
		}
		strategy.Binding = normalizeStrategyInstanceBinding(binding, strategy.Params)
		applyStrategyBindingParams(&strategy)
		s.recordStrategyEventsLocked(&strategy, time.Now().UTC(), "updated strategy binding", "info", "control", "binding.updated", strategyBindingAuditDetail(strategy.Definition.StrategyID, strategy.Binding))
		s.data.Strategies[index] = strategy
		if err := s.persistLocked(); err != nil {
			return strategyListItem{}, err
		}
		return strategyToListItem(strategy), nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) updateStrategyRuntimeRisk(instanceID string, risk strategyRuntimeRiskSettings) (strategyListItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		strategy.Binding.RuntimeRisk = normalizeStrategyRuntimeRiskSettings(risk)
		applyStrategyBindingParams(&strategy)
		s.recordStrategyEventsLocked(&strategy, time.Now().UTC(), "updated strategy runtime risk", "info", "control", "runtime_risk.updated", strategyRuntimeRiskAuditDetail(strategy.Binding.RuntimeRisk))
		s.data.Strategies[index] = strategy
		if err := s.persistLocked(); err != nil {
			return strategyListItem{}, err
		}
		return strategyToListItem(strategy), nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) refreshStrategyDefinition(instanceID string, definition strategyDesignDefinition) (strategyListItem, error) {
	now := time.Now().UTC()
	params, err := buildStrategyInstanceParams(definition, now.Format(time.RFC3339Nano))
	if err != nil {
		return strategyListItem{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		changed, refreshErr := s.refreshStrategyDefinitionLocked(&strategy, definition, params, now)
		if refreshErr != nil {
			return strategyListItem{}, refreshErr
		}
		if changed {
			s.data.Strategies[index] = strategy
			if err := s.persistLocked(); err != nil {
				return strategyListItem{}, err
			}
		}
		return strategyToListItem(strategy), nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) applyDefinitionToLinkedStrategies(definition strategyDesignDefinition) (strategyApplyLinkedInstancesResponse, error) {
	now := time.Now().UTC()
	params, err := buildStrategyInstanceParams(definition, now.Format(time.RFC3339Nano))
	if err != nil {
		return strategyApplyLinkedInstancesResponse{}, err
	}

	result := strategyApplyLinkedInstancesResponse{
		DefinitionID:  strings.TrimSpace(definition.ID),
		LatestVersion: strings.TrimSpace(definition.Version),
		Applied:       []string{},
		AlreadyLatest: []string{},
		SkippedBusy:   []string{},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	persistChanged := false
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if !strategyInstanceUsesDefinition(strategy, definition.ID) {
			continue
		}
		result.TotalLinked++
		if strategy.Status != strategyStatusStopped {
			result.SkippedBusy = append(result.SkippedBusy, strategy.ID)
			continue
		}
		if strings.TrimSpace(strategy.Definition.Version) == strings.TrimSpace(definition.Version) {
			result.AlreadyLatest = append(result.AlreadyLatest, strategy.ID)
			continue
		}
		_, _ = s.refreshStrategyDefinitionLocked(&strategy, definition, params, now)
		s.data.Strategies[index] = strategy
		persistChanged = true
		result.Applied = append(result.Applied, strategy.ID)
	}
	if persistChanged {
		if err := s.persistLocked(); err != nil {
			return strategyApplyLinkedInstancesResponse{}, err
		}
	}
	return result, nil
}

func (s *strategyCatalogStore) deleteStrategy(instanceID string) (strategyListItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		if strategy.Status != strategyStatusStopped {
			return strategyListItem{}, errStrategyInstanceBusy
		}
		removed := strategyToListItem(strategy)
		s.data.Strategies = append(s.data.Strategies[:index], s.data.Strategies[index+1:]...)
		if err := s.persistLocked(); err != nil {
			return strategyListItem{}, err
		}
		return removed, nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) transitionStrategy(instanceID string, nextStatus string, kind string, detail string) (strategyListItem, error) {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		strategy.Status = nextStatus
		s.recordStrategyEventsLocked(&strategy, now, fmt.Sprintf("%s strategy %s", strings.ToLower(kind), strategy.Definition.StrategyID), strategyLogLevelForKind(kind, detail), "control", kind, detail)
		s.data.Strategies[index] = strategy
		if err := s.persistLocked(); err != nil {
			return strategyListItem{}, err
		}
		return strategyToListItem(strategy), nil
	}

	return strategyListItem{}, os.ErrNotExist
}

func (s *strategyCatalogStore) appendStrategyRuntimeEvent(instanceID string, logMessage string, kind string, detail string) error {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		s.recordStrategyEventsLocked(&strategy, now, logMessage, strategyLogLevelForKind(kind, logMessage), "runtime", kind, detail)
		return nil
	}

	return os.ErrNotExist
}

func (s *strategyCatalogStore) reconcileStrategyRuntimeFailure(instanceID string, detail string) error {
	now := time.Now().UTC()
	detail = strings.TrimSpace(detail)

	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.ID != instanceID {
			continue
		}
		if strategy.Status != strategyStatusRunning {
			return nil
		}
		strategy.Status = strategyStatusStopped
		s.recordStrategyEventsLocked(&strategy, now, fmt.Sprintf("strategy runtime exited unexpectedly: %s", detail), "error", "runtime", "runtime_exited", detail)
		s.data.Strategies[index] = strategy
		return s.persistLocked()
	}

	return os.ErrNotExist
}

func (s *strategyCatalogStore) reconcileRuntimeStatesOnStartup() (int, error) {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	changed := 0
	for index := range s.data.Strategies {
		strategy := s.normalizeStrategy(s.data.Strategies[index])
		if strategy.Status != strategyStatusRunning && strategy.Status != strategyStatusPaused {
			continue
		}
		previousStatus := strategy.Status
		strategy.Status = strategyStatusStopped
		s.recordStrategyEventsLocked(&strategy, now, fmt.Sprintf("reconciled strategy state from %s to %s after server startup", previousStatus, strategyStatusStopped), "warning", "startup", "reconciled", fmt.Sprintf("server startup reset stale %s state to %s", strings.ToLower(previousStatus), strategyStatusStopped))
		s.data.Strategies[index] = strategy
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

func (s *strategyCatalogStore) refreshStrategyDefinitionLocked(strategy *managedStrategyInstance, definition strategyDesignDefinition, params map[string]any, at time.Time) (bool, error) {
	if strategy == nil {
		return false, nil
	}
	if strategy.Status != strategyStatusStopped {
		return false, errStrategyInstanceBusy
	}
	if strings.TrimSpace(strategy.Definition.Version) == strings.TrimSpace(definition.Version) {
		return false, nil
	}
	previousVersion := strings.TrimSpace(strategy.Definition.Version)
	strategy.PluginID = strategyPluginIDForDefinition(definition)
	strategy.Definition = strategyDefinitionSummary{
		StrategyID: strings.TrimSpace(definition.ID),
		Name:       strings.TrimSpace(definition.Name),
		Version:    strings.TrimSpace(definition.Version),
	}
	strategy.Params = copyMap(params)
	strategy.Binding = normalizeStrategyInstanceBinding(strategy.Binding, strategy.Params)
	applyStrategyBindingParams(strategy)
	s.recordStrategyEventsLocked(
		strategy,
		at,
		fmt.Sprintf("refreshed strategy definition %s to v%s", strings.TrimSpace(definition.ID), strings.TrimSpace(definition.Version)),
		"info",
		"control",
		"definition.refreshed",
		fmt.Sprintf("%s | %s -> %s", strings.TrimSpace(definition.ID), previousVersion, strings.TrimSpace(definition.Version)),
	)
	return true, nil
}
