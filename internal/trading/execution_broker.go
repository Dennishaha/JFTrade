package trading

import (
	"strings"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *Service) resolveExecutionBroker(rawBrokerID string) (string, broker.Broker, error) {
	brokerID := strings.ToLower(strings.TrimSpace(rawBrokerID))
	selected := s.brokerRuntime.ActiveBroker()
	if brokerID == "" {
		if selected == nil {
			return "", nil, requestErrorf("no default broker is available")
		}
		brokerID = strings.ToLower(strings.TrimSpace(selected.ID()))
	}
	if resolver, ok := s.brokerRuntime.(interface{ ResolveBroker(string) broker.Broker }); ok {
		selected = resolver.ResolveBroker(brokerID)
	} else if selected != nil && !strings.EqualFold(selected.ID(), brokerID) {
		selected = nil
	}
	if selected == nil {
		return "", nil, requestErrorf("brokerId %q is not available", brokerID)
	}
	return brokerID, selected, nil
}
