package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"

	adkartifact "google.golang.org/adk/v2/artifact"
	"google.golang.org/genai"
)

type toolArtifactRef struct {
	Name      string `json:"name"`
	Version   int64  `json:"version"`
	URI       string `json:"uri"`
	MimeType  string `json:"mimeType"`
	Truncated bool   `json:"truncated"`
}

var artifactToolNames = map[string]struct{}{
	"strategy.research_backtest": {},
	"backtest.result_view":       {},
	"strategy.optimize":          {},
}

var artifactFileNameRe = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func (e *googleADKExecution) materializeToolOutput(toolName string, callID string, output any) any {
	raw, err := json.Marshal(output)
	if err != nil || len(raw) <= MaxToolOutputBytes || !toolOutputShouldUseArtifact(toolName) || e == nil || e.artifactService == nil {
		return limitToolOutput(output)
	}
	name := artifactFileName(toolName, e.runID, callID)
	save, err := e.artifactService.Save(context.Background(), &adkartifact.SaveRequest{
		AppName:   e.appName,
		UserID:    googleADKUserID,
		SessionID: e.sessionID,
		FileName:  name,
		Part:      genai.NewPartFromBytes(raw, "application/json"),
	})
	if err != nil || save == nil {
		return limitToolOutput(output)
	}
	ref := toolArtifactRef{
		Name:      name,
		Version:   save.Version,
		URI:       fmt.Sprintf("adk://artifacts/%s?version=%d", name, save.Version),
		MimeType:  "application/json",
		Truncated: true,
	}
	preview := string(raw[:MaxToolOutputBytes])
	if typed, ok := output.(map[string]any); ok {
		out := make(map[string]any, len(typed)+3)
		maps.Copy(out, typed)
		out["truncated"] = true
		out["preview"] = preview
		out["artifactRef"] = ref
		return out
	}
	return map[string]any{
		"truncated":   true,
		"preview":     preview,
		"artifactRef": ref,
	}
}

func toolOutputShouldUseArtifact(toolName string) bool {
	_, ok := artifactToolNames[strings.TrimSpace(toolName)]
	return ok
}

func artifactFileName(toolName string, runID string, callID string) string {
	base := strings.Trim(artifactFileNameRe.ReplaceAllString(strings.TrimSpace(toolName), "-"), "-")
	if base == "" {
		base = "tool"
	}
	runID = strings.Trim(artifactFileNameRe.ReplaceAllString(strings.TrimSpace(runID), "-"), "-")
	callID = strings.Trim(artifactFileNameRe.ReplaceAllString(strings.TrimSpace(callID), "-"), "-")
	if callID == "" {
		callID = "output"
	}
	if runID != "" {
		return base + "-" + runID + "-" + callID + ".json"
	}
	return base + "-" + callID + ".json"
}
