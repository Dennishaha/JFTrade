package pineengine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func ExternalModeFromEnv() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("JFTRADE_PINETS_MODE"))) {
	case ModeShadow:
		return ModeShadow
	case ModeCommunityAGPL:
		return ModeCommunityAGPL
	default:
		return ModeOff
	}
}

func ShadowPayloadForScript(script string) ExternalEnginePayload {
	mode := ExternalModeFromEnv()
	if mode == ModeOff {
		return DisabledPayload()
	}
	if mode == ModeCommunityAGPL && !thirdPartyNoticeAvailable() {
		payload := DisabledPayload()
		payload.Enabled = true
		payload.Mode = mode
		payload.Status = "compliance_error"
		payload.Diagnostics = []Diagnostic{{
			Severity:  "error",
			Code:      "PINETS_AGPL_NOTICE_MISSING",
			Message:   "community-agpl mode requires docs/legal/third-party-notices.md to expose source and license obligations",
			Line:      1,
			Column:    1,
			EndLine:   1,
			EndColumn: 1,
		}}
		payload.DifferenceSummary = map[string]any{"evaluated": false, "reason": "AGPL notice/source-offer file is missing"}
		return payload
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := NewPinetsWorkerClient("", "")
	defer func() { _ = client.Close() }()
	result, err := client.RunIndicator(ctx, RunIndicatorRequest{
		Script:    script,
		Symbol:    "JFTRADE.SAMPLE",
		Timeframe: "1m",
		Mode:      mode,
		TimeoutMS: 10_000,
	})
	payload := ExternalEnginePayloadFromResult(mode, result, err)
	payload.Repository = "https://github.com/LuxAlgo/PineTS"
	return payload
}

func thirdPartyNoticeAvailable() bool {
	if _, err := os.Stat(filepath.Join("docs", "legal", "third-party-notices.md")); err == nil {
		return true
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return false
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "legal", "third-party-notices.md")
	_, err := os.Stat(filepath.Clean(path))
	return err == nil
}
