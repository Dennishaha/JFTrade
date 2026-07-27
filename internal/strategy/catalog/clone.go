package catalog

import stratsrv "github.com/jftrade/jftrade-main/internal/strategy"

func cloneSnapshot(input Snapshot) Snapshot {
	output := Snapshot{
		TargetDir:  input.TargetDir,
		Plugins:    make([]ManagedPlugin, len(input.Plugins)),
		Strategies: make([]stratsrv.ManagedInstance, len(input.Strategies)),
		Operations: append([]stratsrv.PluginOperation(nil), input.Operations...),
	}
	for index := range input.Plugins {
		output.Plugins[index] = clonePlugin(input.Plugins[index])
	}
	for index := range input.Strategies {
		output.Strategies[index] = cloneInstance(input.Strategies[index])
	}
	return output
}

func copyMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = copyDynamicValue(value)
	}
	return output
}

func copyDynamicValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return copyMap(typed)
	case []string:
		return append([]string(nil), typed...)
	case []any:
		output := make([]any, len(typed))
		for index, entry := range typed {
			output[index] = copyDynamicValue(entry)
		}
		return output
	case []map[string]any:
		output := make([]map[string]any, len(typed))
		for index, entry := range typed {
			output[index] = copyMap(entry)
		}
		return output
	default:
		return value
	}
}

func clonePlugin(input ManagedPlugin) ManagedPlugin {
	input.Descriptor.Keywords = append([]string(nil), input.Descriptor.Keywords...)
	if input.Artifact != nil {
		artifact := *input.Artifact
		artifact.Build.BuildTags = append([]string(nil), artifact.Build.BuildTags...)
		input.Artifact = &artifact
	}
	if input.Installation.CurrentOperation != nil {
		input.Installation.CurrentOperation = new(*input.Installation.CurrentOperation)
	}
	if input.Installation.LastOperation != nil {
		input.Installation.LastOperation = new(*input.Installation.LastOperation)
	}
	return input
}

func cloneInstance(input stratsrv.ManagedInstance) stratsrv.ManagedInstance {
	input.Params = copyMap(input.Params)
	input.Binding.Symbols = append([]string(nil), input.Binding.Symbols...)
	input.Binding.Instruments = append([]stratsrv.BindingInstrument(nil), input.Binding.Instruments...)
	if input.Binding.BrokerAccount != nil {
		input.Binding.BrokerAccount = new(*input.Binding.BrokerAccount)
	}
	if input.Binding.RuntimeRisk.MaxOrderQuantity != nil {
		input.Binding.RuntimeRisk.MaxOrderQuantity = new(*input.Binding.RuntimeRisk.MaxOrderQuantity)
	}
	if input.Binding.RuntimeRisk.MaxOrderNotional != nil {
		input.Binding.RuntimeRisk.MaxOrderNotional = new(*input.Binding.RuntimeRisk.MaxOrderNotional)
	}
	if input.Binding.RuntimeRisk.DailyMaxOrders != nil {
		input.Binding.RuntimeRisk.DailyMaxOrders = new(*input.Binding.RuntimeRisk.DailyMaxOrders)
	}
	input.Logs = append([]string(nil), input.Logs...)
	input.AuditEntries = append([]stratsrv.AuditEntry(nil), input.AuditEntries...)
	return input
}
