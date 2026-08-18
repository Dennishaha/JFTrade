package marketdataapp

import (
	"context"
	"errors"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestRuntimeReusesSharedSidecarAcrossPythonProviders(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: "futu"}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	runtime.sidecar = sidecar
	runtime.healthCheck = func(_ context.Context, provider marketdata.Provider, _ bool) error {
		descriptor, descriptorErr := provider.Descriptor(t.Context())
		if descriptorErr != nil {
			return descriptorErr
		}
		if descriptor.ProviderID != "yahoo-finance" && descriptor.ProviderID != ProviderAKShare {
			t.Fatalf("unexpected Python provider descriptor: %#v", descriptor)
		}
		return nil
	}

	for _, providerID := range []string{ProviderYFinance, ProviderAKShare, ProviderYFinance} {
		if err := runtime.Activate(t.Context(), Activation{ProviderID: providerID}); err != nil {
			t.Fatalf("Activate(%s): %v", providerID, err)
		}
		if runtime.ActiveProviderID() != providerID || !sidecar.running || sidecar.stopCalls != 0 {
			t.Fatalf("activation %s = active %s, sidecar %#v", providerID, runtime.ActiveProviderID(), sidecar)
		}
	}
	if sidecar.ensureCalls != 3 {
		t.Fatalf("idempotent EnsureStarted calls = %d, want one per activation", sidecar.ensureCalls)
	}
	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderFutu}); err != nil {
		t.Fatalf("Activate(futu): %v", err)
	}
	if runtime.ActiveProviderID() != ProviderFutu || sidecar.stopCalls != 1 || sidecar.running {
		t.Fatalf("Futu activation = active %s, sidecar %#v", runtime.ActiveProviderID(), sidecar)
	}
}

func TestRuntimeKeepsSharedSidecarOnCrossPythonActivationFailure(t *testing.T) {
	healthErr := errors.New("AKShare runtime failed")
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: "futu"}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	runtime.sidecar = sidecar
	runtime.healthCheck = func(_ context.Context, provider marketdata.Provider, _ bool) error {
		descriptor, descriptorErr := provider.Descriptor(t.Context())
		if descriptorErr != nil {
			return descriptorErr
		}
		if descriptor.ProviderID == ProviderAKShare {
			return healthErr
		}
		return nil
	}
	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderYFinance}); err != nil {
		t.Fatalf("Activate(yfinance): %v", err)
	}
	err = runtime.Activate(t.Context(), Activation{ProviderID: ProviderAKShare, RequireHealthy: true})
	if !errors.Is(err, healthErr) || runtime.ActiveProviderID() != ProviderYFinance {
		t.Fatalf("failed AKShare switch = active %s, err=%v", runtime.ActiveProviderID(), err)
	}
	if sidecar.ensureCalls != 2 || sidecar.stopCalls != 0 || !sidecar.running {
		t.Fatalf("shared sidecar after rollback = %#v", sidecar)
	}
}

func TestRuntimeStopsNewSidecarWhenInitialAKShareActivationFails(t *testing.T) {
	healthErr := errors.New("AKShare import failed")
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: "futu"}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	runtime.sidecar = sidecar
	runtime.healthCheck = func(context.Context, marketdata.Provider, bool) error { return healthErr }

	err = runtime.Activate(t.Context(), Activation{ProviderID: ProviderAKShare, RequireHealthy: true})
	if !errors.Is(err, healthErr) || runtime.ActiveProviderID() != ProviderFutu {
		t.Fatalf("failed initial AKShare activation = active %s, err=%v", runtime.ActiveProviderID(), err)
	}
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 1 || sidecar.running {
		t.Fatalf("initial AKShare rollback sidecar = %#v", sidecar)
	}
}

func TestRuntimeRetriesAProviderMarkedUnavailable(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: ProviderFutu}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	runtime.sidecar = sidecar
	runtime.healthCheck = func(context.Context, marketdata.Provider, bool) error { return nil }
	activationErr := errors.New("AKShare helper unavailable")
	runtime.MarkProviderUnavailable(ProviderAKShare, activationErr)

	if runtime.ActiveProviderID() != ProviderAKShare ||
		!runtime.NeedsProviderActivation(ProviderAKShare) {
		t.Fatalf("marked unavailable state = provider %q, needs retry=%v",
			runtime.ActiveProviderID(), runtime.NeedsProviderActivation(ProviderAKShare))
	}
	if err := runtime.Activate(t.Context(), Activation{
		ProviderID:     ProviderAKShare,
		RequireHealthy: true,
	}); err != nil {
		t.Fatalf("retry AKShare activation: %v", err)
	}
	if runtime.ActiveProviderID() != ProviderAKShare ||
		runtime.NeedsProviderActivation(ProviderAKShare) || sidecar.ensureCalls != 1 {
		t.Fatalf("retried provider state = provider %q, needs retry=%v, sidecar=%#v",
			runtime.ActiveProviderID(), runtime.NeedsProviderActivation(ProviderAKShare), sidecar)
	}
}

func TestRuntimeUsesGenericCacheDirectoryWithLegacyFallback(t *testing.T) {
	if got := runtimeSidecarCacheDir(RuntimeOptions{
		MarketDataCacheDir: " /new/cache ", YFinanceCacheDir: "/old/cache",
	}); got != "/new/cache" {
		t.Fatalf("generic cache directory = %q", got)
	}
	if got := runtimeSidecarCacheDir(RuntimeOptions{YFinanceCacheDir: " /old/cache "}); got != "/old/cache" {
		t.Fatalf("legacy cache directory = %q", got)
	}
	for _, providerID := range []string{" YFINANCE ", "AKSHARE"} {
		if !isPythonProvider(providerID) {
			t.Fatalf("isPythonProvider(%q) = false", providerID)
		}
	}
	if isPythonProvider(ProviderFutu) {
		t.Fatal("Futu classified as Python provider")
	}
}
