package main

import "testing"

func TestValidateArgsAllowsNoArgs(t *testing.T) {
	if err := validateArgs(nil); err != nil {
		t.Fatalf("validateArgs(nil) = %v, want nil", err)
	}
}

func TestValidateArgsRejectsLegacySubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"api"},
		{"serve-api"},
		{"run"},
	} {
		if err := validateArgs(args); err == nil {
			t.Fatalf("validateArgs(%v) = nil, want error", args)
		}
	}
}

func TestIsHelpArgs(t *testing.T) {
	for _, args := range [][]string{
		{"help"},
		{"--help"},
		{"-h"},
	} {
		if !isHelpArgs(args) {
			t.Fatalf("isHelpArgs(%v) = false, want true", args)
		}
	}
	if isHelpArgs(nil) || isHelpArgs([]string{"api"}) || isHelpArgs([]string{"help", "extra"}) {
		t.Fatal("isHelpArgs accepted non-help args")
	}
}
