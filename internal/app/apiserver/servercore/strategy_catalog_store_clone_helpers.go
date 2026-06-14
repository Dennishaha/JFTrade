package servercore

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

func cloneManagedStrategyPlugin(input managedStrategyPlugin) managedStrategyPlugin {
	if input.Descriptor.Keywords != nil {
		input.Descriptor.Keywords = append([]string(nil), input.Descriptor.Keywords...)
	}
	if input.Artifact != nil {
		artifactCopy := *input.Artifact
		artifactCopy.Build.BuildTags = append([]string(nil), artifactCopy.Build.BuildTags...)
		input.Artifact = &artifactCopy
	}
	if input.Installation.CurrentOperation != nil {
		operationCopy := *input.Installation.CurrentOperation
		input.Installation.CurrentOperation = &operationCopy
	}
	if input.Installation.LastOperation != nil {
		operationCopy := *input.Installation.LastOperation
		input.Installation.LastOperation = &operationCopy
	}
	return input
}

func cloneManagedStrategyInstance(input managedStrategyInstance) managedStrategyInstance {
	input.Params = copyMap(input.Params)
	input.Binding.Symbols = append([]string(nil), input.Binding.Symbols...)
	if input.Binding.BrokerAccount != nil {
		bindingCopy := *input.Binding.BrokerAccount
		input.Binding.BrokerAccount = &bindingCopy
	}
	return input
}
