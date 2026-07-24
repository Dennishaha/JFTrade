package research

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	domain "github.com/jftrade/jftrade-main/internal/research"
)

type presetURI struct {
	PresetID string `uri:"presetId" binding:"required"`
}

// RegisterRoutes registers instance-owned research APIs under /api/v1/research.
func RegisterRoutes(api *gin.RouterGroup, service *domain.Service) {
	referenceOpenAPIDocumentation()
	if service == nil {
		service = domain.NewService(nil)
	}
	presets := api.Group("/research/screens/presets")
	presets.GET("", func(c *gin.Context) {
		items, err := service.ListScreenPresets(c.Request.Context())
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, map[string]any{"presets": items})
	})
	presets.POST("", func(c *gin.Context) {
		var input domain.CreateScreenPresetInput
		if err := httpserver.BindStrictJSON(c, &input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "RESEARCH_PRESET_INVALID", "invalid research screen preset payload")
			return
		}
		preset, err := service.CreateScreenPreset(c.Request.Context(), input)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, preset)
	})
	presets.GET("/:presetId", func(c *gin.Context) {
		presetID, ok := bindPresetID(c)
		if !ok {
			return
		}
		preset, err := service.GetScreenPreset(c.Request.Context(), presetID)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, preset)
	})
	presets.PATCH("/:presetId", func(c *gin.Context) {
		presetID, ok := bindPresetID(c)
		if !ok {
			return
		}
		var input domain.UpdateScreenPresetInput
		if err := httpserver.BindStrictJSON(c, &input); err != nil {
			httpserver.WriteError(c, http.StatusBadRequest, "RESEARCH_PRESET_INVALID", "invalid research screen preset payload")
			return
		}
		preset, err := service.UpdateScreenPreset(c.Request.Context(), presetID, input)
		if err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, preset)
	})
	presets.DELETE("/:presetId", func(c *gin.Context) {
		presetID, ok := bindPresetID(c)
		if !ok {
			return
		}
		if err := service.DeleteScreenPreset(c.Request.Context(), presetID); err != nil {
			writeError(c, err)
			return
		}
		httpserver.WriteOK(c, map[string]any{"deleted": true})
	})
}

func bindPresetID(c *gin.Context) (string, bool) {
	var uri presetURI
	if err := httpserver.BindURI(c, &uri); err != nil {
		httpserver.WriteError(c, http.StatusBadRequest, "RESEARCH_PRESET_INVALID", "invalid research screen preset id")
		return "", false
	}
	return uri.PresetID, true
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrUnavailable):
		httpserver.WriteError(c, http.StatusServiceUnavailable, "RESEARCH_PRESET_UNAVAILABLE", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		httpserver.WriteError(c, http.StatusNotFound, "RESEARCH_PRESET_NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrValidation):
		httpserver.WriteError(c, http.StatusBadRequest, "RESEARCH_PRESET_INVALID", err.Error())
	case errors.Is(err, domain.ErrConflict):
		httpserver.WriteError(c, http.StatusConflict, "RESEARCH_PRESET_CONFLICT", err.Error())
	default:
		httpserver.WriteError(c, http.StatusInternalServerError, "RESEARCH_PRESET_FAILED", err.Error())
	}
}
