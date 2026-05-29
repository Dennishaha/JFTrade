package main

import "testing"

func TestShouldRunAPIOnlyWithoutArgs(t *testing.T) {
	if !shouldRunAPIOnly(nil) {
		t.Fatal("expected no-arg launch to default to API mode")
	}
}

func TestShouldRunAPIOnlyForExplicitAPIArgs(t *testing.T) {
	if !shouldRunAPIOnly([]string{"api"}) {
		t.Fatal("expected api arg to enable API mode")
	}
	if !shouldRunAPIOnly([]string{"serve-api"}) {
		t.Fatal("expected serve-api arg to enable API mode")
	}
	if shouldRunAPIOnly([]string{"run", "--config", "./config/jftrade.yaml"}) {
		t.Fatal("expected explicit bbgo run command to bypass API-only mode")
	}
}
