package opend

import (
	"strings"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	if got := FormatVersion(1009, 6908); got != "10.9.6908" {
		t.Fatalf("FormatVersion = %q", got)
	}
	if got := FormatVersion(504, 0); got != "5.4" {
		t.Fatalf("FormatVersion without build = %q", got)
	}
}

func TestValidateMinimumVersion(t *testing.T) {
	currentBuild := int32(6908)
	newerBuild := int32(7000)
	oldBuild := int32(6808)

	for _, test := range []struct {
		name      string
		serverVer int32
		buildNo   *int32
		wantError bool
	}{
		{name: "current exact", serverVer: 1009, buildNo: &currentBuild},
		{name: "newer build", serverVer: 1009, buildNo: &newerBuild},
		{name: "newer minor", serverVer: 1010, buildNo: &currentBuild},
		{name: "init connect current line", serverVer: 1009},
		{name: "old build", serverVer: 1009, buildNo: &oldBuild, wantError: true},
		{name: "old minor", serverVer: 1008, buildNo: &newerBuild, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateMinimumVersion(test.serverVer, test.buildNo)
			if test.wantError && (err == nil || !strings.Contains(err.Error(), MinimumOpenDVersion)) {
				t.Fatalf("error = %v", err)
			}
			if !test.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
