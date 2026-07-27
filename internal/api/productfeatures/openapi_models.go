package productfeatures

import (
	service "github.com/jftrade/jftrade-main/internal/productfeatures"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// ProductFeatureQueryRequest documents the deliberately open parameter bag
// accepted by broker-neutral product feature POST queries.
type ProductFeatureQueryRequest map[string]any

// CustomizationRequest documents broker customization payloads whose fields
// depend on the selected feature family.
type CustomizationRequest map[string]any

// PredictionSubscriptionReleaseData documents a released subscription lease.
type PredictionSubscriptionReleaseData struct {
	Released bool `json:"released"`
}

// BrokerCapabilitiesData documents the machine-readable broker capability view.
type BrokerCapabilitiesData struct {
	Catalog broker.CapabilityCatalog          `json:"catalog"`
	Brokers []broker.Descriptor               `json:"brokers"`
	Runtime []service.RuntimeCapabilityStatus `json:"runtime"`
}
