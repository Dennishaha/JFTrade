package settings

import (
	"errors"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestServiceCreateManagedAccountNormalizesClientFields(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)

	account, err := svc.CreateManagedAccount(jfsettings.ManagedBrokerAccount{
		ID:                 "client-id",
		AccountID:          "acc-1",
		CreatedAt:          "client-created",
		UpdatedAt:          "client-updated",
		TradingEnvironment: "SIMULATE",
	})
	if err != nil {
		t.Fatalf("CreateManagedAccount: %v", err)
	}
	if account.ID != "" || account.CreatedAt != "" || account.UpdatedAt != "" {
		t.Fatalf("created account = %#v", account)
	}
	if len(store.managedAccounts) != 1 || store.managedAccounts[0].AccountID != "acc-1" {
		t.Fatalf("stored accounts = %#v", store.managedAccounts)
	}
}

func TestServiceCreateManagedAccountRejectsBlankAccountID(t *testing.T) {
	svc := NewService(&fakeStore{})
	if _, err := svc.CreateManagedAccount(jfsettings.ManagedBrokerAccount{}); err == nil {
		t.Fatal("CreateManagedAccount error = nil, want bad request")
	} else if !errors.Is(err, ErrBadRequest) || err.Error() != "accountId is required" {
		t.Fatalf("CreateManagedAccount error = %v, want ErrBadRequest with field message", err)
	}
}

func TestServiceOptionsCaptureBrokerDescriptorAndDefaultTradingEnvironment(t *testing.T) {
	descriptor := map[string]any{"brokerId": "futu", "markets": []string{"HK", "US"}}
	svc := NewService(
		&fakeStore{},
		WithBrokerDescriptor(func() map[string]any { return descriptor }),
		WithDefaultTradingEnvironment("REAL"),
	)

	if svc.brokerDescriptor == nil {
		t.Fatal("broker descriptor option was not installed")
	}
	got := svc.brokerDescriptor()
	if got["brokerId"] != "futu" || len(got["markets"].([]string)) != 2 {
		t.Fatalf("broker descriptor = %#v", got)
	}
	if svc.defaultTradingEnv != "REAL" {
		t.Fatalf("default trading env = %q, want REAL", svc.defaultTradingEnv)
	}
}
