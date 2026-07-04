package runtimecontrol

type LiveExecutionLimitation struct {
	Code    string
	Message string
}

func LiveExecutionLimitations(script string) []LiveExecutionLimitation {
	_ = script
	return nil
}
