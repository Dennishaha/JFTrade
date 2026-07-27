package catalog

import (
	"fmt"
	"sort"
	"strings"
	"time"

	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	instancebinding "github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
	instanceview "github.com/jftrade/jftrade-main/internal/strategy/instanceview"
)

func (s *Service) ListInstances() []stratsrv.InstanceView {
	s.mu.RLock()
	items := make([]stratsrv.InstanceView, 0, len(s.data.Strategies))
	for _, instance := range s.data.Strategies {
		items = append(items, instanceview.ToInstanceView(s.normalizeStrategy(instance)))
	}
	s.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt < items[j].CreatedAt
	})
	for index := range items {
		items[index] = s.enrichInstance(items[index])
	}
	return items
}

func (s *Service) GetInstance(instanceID string) (stratsrv.ManagedInstance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, instance := range s.data.Strategies {
		normalized := s.normalizeStrategy(instance)
		if normalized.ID == instanceID {
			return cloneInstance(normalized), true
		}
	}
	return stratsrv.ManagedInstance{}, false
}

func (s *Service) ValidateStartable(instance stratsrv.ManagedInstance) error {
	if !instanceview.Startable(instance) {
		return stratsrv.BadRequestError(fmt.Sprintf(
			"strategy runtime %s is not startable yet",
			instanceview.RuntimeFromParams(instance.Params),
		))
	}
	return nil
}

func (s *Service) CreateInstance(definition stratsrv.Definition, binding stratsrv.InstanceBinding) (stratsrv.InstanceView, error) {
	now := time.Now().UTC()
	params, err := buildInstanceParams(definition, now.Format(time.RFC3339Nano))
	if err != nil {
		return stratsrv.InstanceView{}, err
	}
	binding = instancebinding.NormalizeBinding(binding, params)
	instance := stratsrv.ManagedInstance{
		ID:       instanceview.BuildInstanceID(definition.ID, now),
		PluginID: instanceview.PluginIDForDefinition(definition),
		Definition: stratsrv.DefinitionSummary{
			StrategyID: definition.ID,
			Name:       definition.Name,
			Version:    definition.Version,
		},
		Binding:   binding,
		Params:    params,
		Status:    StatusStopped,
		CreatedAt: now.Format(time.RFC3339Nano),
	}

	s.mu.Lock()
	instance = s.normalizeStrategy(instance)
	s.recordEventsLocked(
		&instance,
		now,
		fmt.Sprintf("instantiated strategy from definition %s", definition.ID),
		"info",
		"control",
		"instantiated",
		instancebinding.BindingAuditDetail(definition.ID, binding),
	)
	s.data.Strategies = append(s.data.Strategies, instance)
	err = s.persistLocked()
	s.mu.Unlock()
	if err != nil {
		return stratsrv.InstanceView{}, err
	}
	return s.enrichInstance(instanceview.ToInstanceView(instance)), nil
}

func (s *Service) UpdateInstance(instanceID string, binding stratsrv.InstanceBinding) (stratsrv.InstanceView, error) {
	s.mu.Lock()
	item, err := s.updateInstanceLocked(instanceID, func(instance *stratsrv.ManagedInstance) error {
		if instance.Status != StatusStopped {
			return stratsrv.BusyError("strategy instance must be stopped before modification")
		}
		instance.Binding = instancebinding.NormalizeBinding(binding, instance.Params)
		instancebinding.ApplyParams(instance)
		s.recordEventsLocked(
			instance,
			time.Now().UTC(),
			"updated strategy binding",
			"info",
			"control",
			"binding.updated",
			instancebinding.BindingAuditDetail(instance.Definition.StrategyID, instance.Binding),
		)
		return nil
	})
	s.mu.Unlock()
	if err != nil {
		return stratsrv.InstanceView{}, err
	}
	return s.enrichInstance(item), nil
}

func (s *Service) UpdateInstanceRuntimeRisk(instanceID string, risk stratsrv.RuntimeRiskSettings) (stratsrv.InstanceView, error) {
	s.mu.Lock()
	item, err := s.updateInstanceLocked(instanceID, func(instance *stratsrv.ManagedInstance) error {
		instance.Binding.RuntimeRisk = instancebinding.NormalizeRiskSettings(risk)
		instancebinding.ApplyParams(instance)
		s.recordEventsLocked(
			instance,
			time.Now().UTC(),
			"updated strategy runtime risk",
			"info",
			"control",
			"runtime_risk.updated",
			instancebinding.RiskAuditDetail(instance.Binding.RuntimeRisk),
		)
		return nil
	})
	s.mu.Unlock()
	if err != nil {
		return stratsrv.InstanceView{}, err
	}
	return s.enrichInstance(item), nil
}

func (s *Service) DeleteInstance(instanceID string) (stratsrv.InstanceView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.data.Strategies {
		instance := s.normalizeStrategy(s.data.Strategies[index])
		if instance.ID != instanceID {
			continue
		}
		if instance.Status != StatusStopped {
			return stratsrv.InstanceView{}, stratsrv.BusyError("strategy instance must be stopped before modification")
		}
		removed := instanceview.ToInstanceView(instance)
		s.data.Strategies = append(s.data.Strategies[:index], s.data.Strategies[index+1:]...)
		if err := s.persistLocked(); err != nil {
			return stratsrv.InstanceView{}, err
		}
		return removed, nil
	}
	return stratsrv.InstanceView{}, stratsrv.NotFoundError("strategy resource not found")
}

func (s *Service) GetLinkedInstanceIDs(definitionID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	definitionID = strings.TrimSpace(definitionID)
	linked := make([]string, 0)
	for _, instance := range s.data.Strategies {
		normalized := s.normalizeStrategy(instance)
		if instanceUsesDefinition(normalized, definitionID) {
			linked = append(linked, normalized.ID)
		}
	}
	sort.Strings(linked)
	return linked
}

func (s *Service) updateInstanceLocked(
	instanceID string,
	update func(*stratsrv.ManagedInstance) error,
) (stratsrv.InstanceView, error) {
	for index := range s.data.Strategies {
		instance := s.normalizeStrategy(s.data.Strategies[index])
		if instance.ID != instanceID {
			continue
		}
		if err := update(&instance); err != nil {
			return stratsrv.InstanceView{}, err
		}
		s.data.Strategies[index] = instance
		if err := s.persistLocked(); err != nil {
			return stratsrv.InstanceView{}, err
		}
		return instanceview.ToInstanceView(instance), nil
	}
	return stratsrv.InstanceView{}, stratsrv.NotFoundError("strategy resource not found")
}
