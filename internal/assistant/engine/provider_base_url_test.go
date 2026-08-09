package adk

import "testing"

func TestValidateProviderBaseURLRejectsMetadataTargets(t *testing.T) {
	for _, rawURL := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://100.100.100.200/latest/meta-data",
		"http://metadata.google.internal/computeMetadata/v1",
		"http://[fd00:ec2::254]/latest/meta-data",
	} {
		if err := validateProviderBaseURL(rawURL); err == nil {
			t.Errorf("validateProviderBaseURL(%q) succeeded, want metadata rejection", rawURL)
		}
	}
}
