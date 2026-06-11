package jftradeapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
	strategypinespec "github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

type strategyPineAnalyzeRequest struct {
	Script       string `json:"script"`
	SourceFormat string `json:"sourceFormat"`
	IncludeAST   bool   `json:"includeAst"`
}

func (s *Server) handleAnalyzeStrategyPine(c *gin.Context) {
	var payload strategyPineAnalyzeRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid strategy pine analyze payload")
		return
	}
	sourceFormat := strategydefinition.NormalizeSourceFormat(payload.SourceFormat)
	if sourceFormat == "" {
		sourceFormat = strategydefinition.SourceFormatPineV6
	}
	if sourceFormat != strategydefinition.SourceFormatPineV6 {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "strategy-pine analyze supports pine-v6 only")
		return
	}

	analysis := strategypine.AnalyzeScript(payload.Script, strategypine.AnalysisOptions{IncludeAST: payload.IncludeAST})
	response := map[string]any{
		"ok":               analysis.OK,
		"sourceFormat":     strategypinespec.SourceFormat,
		"runtime":          strategypinespec.Runtime,
		"normalizedScript": analysis.NormalizedScript,
		"diagnostics":      analysis.Diagnostics,
		"warnings":         analysis.Warnings,
		"metadata":         strategyMetadataPayload(analysis.Program),
		"hooks":            buildCompiledHookKinds(analysis.Program),
		"requirements":     buildCompiledRequirementsPayload(analysis.Requirements),
		"features":         analysis.Features,
	}
	if payload.IncludeAST {
		response["ast"] = analysis.AST
	}
	s.writeOK(c, response)
}
