package research

import (
	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	domain "github.com/jftrade/jftrade-main/internal/research"
)

type ResearchScreenPreset domain.ScreenPreset

type ResearchScreenPresetsData struct {
	Presets []ResearchScreenPreset `json:"presets"`
}

type ResearchScreenPresetDeleteData struct {
	Deleted bool `json:"deleted"`
}

func referenceOpenAPIDocumentation() {
	_ = [...]func(){
		documentListScreenPresets,
		documentCreateScreenPreset,
		documentGetScreenPreset,
		documentUpdateScreenPreset,
		documentDeleteScreenPreset,
	}
}

// documentListScreenPresets godoc
// @Summary 查询股票筛选预设
// @Tags research
// @Produce json
// @Success 200 {object} httpserver.Envelope{data=ResearchScreenPresetsData}
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/research/screens/presets [get]
func documentListScreenPresets() { _ = httpserver.Envelope{} }

// documentCreateScreenPreset godoc
// @Summary 创建股票筛选预设
// @Tags research
// @Accept json
// @Produce json
// @Param input body domain.CreateScreenPresetInput true "筛选预设"
// @Success 200 {object} httpserver.Envelope{data=ResearchScreenPreset}
// @Failure 400 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/research/screens/presets [post]
func documentCreateScreenPreset() { _ = httpserver.Envelope{} }

// documentGetScreenPreset godoc
// @Summary 查询股票筛选预设详情
// @Tags research
// @Produce json
// @Param presetId path string true "预设 ID"
// @Success 200 {object} httpserver.Envelope{data=ResearchScreenPreset}
// @Failure 404 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/research/screens/presets/{presetId} [get]
func documentGetScreenPreset() { _ = httpserver.Envelope{} }

// documentUpdateScreenPreset godoc
// @Summary 修改股票筛选预设
// @Tags research
// @Accept json
// @Produce json
// @Param presetId path string true "预设 ID"
// @Param input body domain.UpdateScreenPresetInput true "筛选预设变更"
// @Success 200 {object} httpserver.Envelope{data=ResearchScreenPreset}
// @Failure 400 {object} httpserver.Envelope
// @Failure 404 {object} httpserver.Envelope
// @Failure 409 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/research/screens/presets/{presetId} [patch]
func documentUpdateScreenPreset() { _ = httpserver.Envelope{} }

// documentDeleteScreenPreset godoc
// @Summary 删除股票筛选预设
// @Tags research
// @Produce json
// @Param presetId path string true "预设 ID"
// @Success 200 {object} httpserver.Envelope{data=ResearchScreenPresetDeleteData}
// @Failure 404 {object} httpserver.Envelope
// @Failure 503 {object} httpserver.Envelope
// @Router /api/v1/research/screens/presets/{presetId} [delete]
func documentDeleteScreenPreset() { _ = httpserver.Envelope{} }
