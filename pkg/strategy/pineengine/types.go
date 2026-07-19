package pineengine

import "github.com/jftrade/jftrade-main/pkg/strategy/pineworker"

const (
	GoPineEngineID       = "go-pine"
	PinetsShadowEngineID = "pinets-shadow"

	ModeOff           = "off"
	ModeShadow        = "shadow"
	ModeCommunityAGPL = "community-agpl"
)

type EngineInfo struct {
	Engine        string `json:"engine"`
	EngineVersion string `json:"engineVersion"`
	PackageName   string `json:"packageName,omitempty"`
	License       string `json:"license,omitempty"`
	Repository    string `json:"repository,omitempty"`
	Runtime       string `json:"runtime,omitempty"`
}

// Candle is the candle element of RunIndicatorRequest, the JSON-RPC payload
// sent to the PineTS shadow worker (scripts/pinets-worker.mjs), which reads the
// camelCase openTime/closeTime fields. It aliases the canonical numeric wire
// candle from pkg/strategy/pineworker; the JSON tags that define this
// protocol's wire shape live on that shared type. Do not reintroduce a local
// copy — extend pineworker.Candle instead.
type Candle = pineworker.Candle

type Diagnostic struct {
	Severity  string `json:"severity"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"endLine"`
	EndColumn int    `json:"endColumn"`
}

type RunIndicatorRequest struct {
	Script     string   `json:"script"`
	Symbol     string   `json:"symbol"`
	Timeframe  string   `json:"timeframe"`
	Candles    []Candle `json:"candles"`
	WarmupBars int      `json:"warmupBars,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	TimeoutMS  int      `json:"timeoutMs,omitempty"`
}

type Plot struct {
	Title string `json:"title"`
	Data  []any  `json:"data"`
}

type RunIndicatorResponse struct {
	OK            bool            `json:"ok"`
	Engine        string          `json:"engine"`
	EngineVersion string          `json:"engineVersion"`
	License       string          `json:"license"`
	Diagnostics   []Diagnostic    `json:"diagnostics"`
	Plots         map[string]Plot `json:"plots"`
	Signals       map[string]any  `json:"signals"`
	Metadata      map[string]any  `json:"metadata"`
	RuntimeMS     int             `json:"runtimeMs"`
}

type ExternalEnginePayload struct {
	Enabled           bool           `json:"enabled"`
	Mode              string         `json:"mode"`
	Engine            string         `json:"engine"`
	EngineVersion     string         `json:"engineVersion,omitempty"`
	License           string         `json:"license,omitempty"`
	Repository        string         `json:"repository,omitempty"`
	OK                bool           `json:"ok"`
	Status            string         `json:"status"`
	Diagnostics       []Diagnostic   `json:"diagnostics"`
	DifferenceSummary map[string]any `json:"differenceSummary"`
	Compliance        map[string]any `json:"compliance"`
}

func DisabledPayload() ExternalEnginePayload {
	return ExternalEnginePayload{
		Enabled:     false,
		Mode:        ModeOff,
		Engine:      PinetsShadowEngineID,
		Status:      "disabled",
		Diagnostics: []Diagnostic{},
		DifferenceSummary: map[string]any{
			"evaluated": false,
			"reason":    "external PineTS shadow engine is disabled by default",
		},
		Compliance: CommunityAGPLCompliance(),
	}
}

func CommunityAGPLCompliance() map[string]any {
	return map[string]any{
		"license":           "AGPL-3.0-only",
		"commercialLicense": false,
		"sourceOffer":       "docs/legal/third-party-notices.md",
		"networkUseNotice":  "If PineTS functionality is exposed over a network, provide corresponding source and license notices for the AGPL-covered integration.",
	}
}

func PayloadMap(payload ExternalEnginePayload) map[string]any {
	return map[string]any{
		"enabled":           payload.Enabled,
		"mode":              payload.Mode,
		"engine":            payload.Engine,
		"engineVersion":     payload.EngineVersion,
		"license":           payload.License,
		"repository":        payload.Repository,
		"ok":                payload.OK,
		"status":            payload.Status,
		"diagnostics":       payload.Diagnostics,
		"differenceSummary": payload.DifferenceSummary,
		"compliance":        payload.Compliance,
	}
}
