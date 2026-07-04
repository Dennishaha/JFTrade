package instanceview

import (
	"maps"
	"strings"
	"time"

	strategy "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

const DefaultPluginID = pineworker.RuntimeID

func PluginIDForDefinition(strategy.Definition) string {
	return DefaultPluginID
}

func RuntimeFromParams(params map[string]any) string {
	if runtime, ok := params["runtime"].(string); ok {
		normalized := pineworker.NormalizeRuntime(runtime)
		if normalized != "" {
			return normalized
		}
	}
	return DefaultPluginID
}

func SourceFormatFromParams(params map[string]any) string {
	if sourceFormat, ok := params["sourceFormat"].(string); ok {
		return strategydefinition.NormalizeSourceFormat(sourceFormat)
	}
	return strategydefinition.SourceFormatPineV6
}

func Startable(instance strategy.ManagedInstance) bool {
	sourceFormat := SourceFormatFromParams(instance.Params)
	runtime := RuntimeFromParams(instance.Params)
	return sourceFormat == strategydefinition.SourceFormatPineV6 && runtime == DefaultPluginID
}

func ToInstanceView(instance strategy.ManagedInstance) strategy.InstanceView {
	instance = NormalizeManagedInstance(instance)
	return strategy.InstanceView{
		ID:           instance.ID,
		PluginID:     instance.PluginID,
		Definition:   instance.Definition,
		Runtime:      RuntimeFromParams(instance.Params),
		SourceFormat: SourceFormatFromParams(instance.Params),
		Startable:    Startable(instance),
		Binding:      instance.Binding,
		Params:       copyMap(instance.Params),
		Status:       instance.Status,
		CreatedAt:    instance.CreatedAt,
		Logs:         []string{},
	}
}

func NormalizeManagedInstance(input strategy.ManagedInstance) strategy.ManagedInstance {
	if input.Params == nil {
		input.Params = map[string]any{}
	}
	instancebinding.ApplyParams(&input)
	return input
}

func BuildInstanceID(definitionID string, at time.Time) string {
	definitionID = strings.TrimSpace(definitionID)
	if definitionID == "" {
		definitionID = DefaultPluginID
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return definitionID + "-" + at.UTC().Format("20060102150405.000000000")
}

func copyMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(input))
	maps.Copy(result, input)
	return result
}
