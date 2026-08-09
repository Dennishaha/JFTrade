package providers

import "strings"

// SanitizeToolNameForOpenAI replaces characters that OpenAI-compatible
// providers reject in function names. The API requires names matching
// ^[a-zA-Z0-9_-]+$.
func SanitizeToolNameForOpenAI(name string) string {
	return strings.ReplaceAll(name, ".", "-")
}

// RestoreToolNameFromOpenAI reverses the sanitization applied by
// SanitizeToolNameForOpenAI.
func RestoreToolNameFromOpenAI(name string) string {
	return strings.ReplaceAll(name, "-", ".")
}
