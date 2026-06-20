package broker

// Descriptor describes a broker's static capabilities.
// This is exposed to the frontend so the UI can adapt to each broker's feature set.
type Descriptor struct {
	ID           string             `json:"id"`
	DisplayName  string             `json:"displayName"`
	Environments []string           `json:"environments"`
	Capabilities []MarketCapability `json:"capabilities"`
	Notes        []string           `json:"notes,omitempty"`
}

// MarketCapability describes what a broker can do for a specific market.
type MarketCapability struct {
	Market        string         `json:"market"`
	SupportsQuote bool           `json:"supportsQuote"`
	SupportsTrade bool           `json:"supportsTrade"`
	ReadFeatures  map[string]any `json:"readFeatures,omitempty"`
}
