package persistence

import (
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"testing"
)

func TestNormalizeDefaultProviderSelection(t *testing.T) {
	rawProviders := []assistantmodel.Provider{
		{ID: "p1", DisplayName: "P1", BaseURL: "https://p1.example/v1", Default: false},
		{ID: "p2", DisplayName: "P2", BaseURL: "https://p2.example/v1", Default: true},
		{ID: "p3", DisplayName: "P3", BaseURL: "https://p3.example/v1", Default: true},
	}
	if !NormalizeDefaultProviderSelection(rawProviders) {
		t.Fatal("NormalizeDefaultProviderSelection with duplicate defaults changed=false, want true")
	}
	if !rawProviders[1].Default || rawProviders[2].Default {
		t.Fatalf("duplicate default normalization = %#v", rawProviders)
	}
	noDefaultProviders := []assistantmodel.Provider{{ID: "p1", DisplayName: "P1", BaseURL: "https://p1.example/v1"}}
	if !NormalizeDefaultProviderSelection(noDefaultProviders) || !noDefaultProviders[0].Default {
		t.Fatalf("missing default normalization = %#v", noDefaultProviders)
	}
	if NormalizeDefaultProviderSelection(nil) {
		t.Fatal("empty provider normalization changed=true, want false")
	}
}

func TestSortProvidersDefaultFirst(t *testing.T) {
	providers := []assistantmodel.Provider{
		{ID: "b", CreatedAt: "same", Default: false},
		{ID: "a", CreatedAt: "same", Default: false},
		{ID: "default", CreatedAt: "later", Default: true},
	}
	SortProvidersDefaultFirst(providers)
	if providers[0].ID != "default" || providers[1].ID != "a" || providers[2].ID != "b" {
		t.Fatalf("SortProvidersDefaultFirst id tie = %+v", providers)
	}
}
