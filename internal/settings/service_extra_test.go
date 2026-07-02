package settings

import (
	"context"
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

func TestDataMigrationFallbackAndDelegation(t *testing.T) {
	svc := NewService(&fakeStore{})

	status, err := svc.DataMigrationStatus(context.Background())
	if err != nil {
		t.Fatalf("DataMigrationStatus default: %v", err)
	}
	got, ok := status.(map[string]any)
	if !ok || len(got["databases"].([]any)) != 0 {
		t.Fatalf("default status = %#v", status)
	}
	if _, err := svc.ScheduleDatabaseRebuild(context.Background(), map[string]any{"mode": "all"}); err == nil {
		t.Fatal("ScheduleDatabaseRebuild error = nil, want unavailable")
	}

	wantErr := errors.New("rebuild failed")
	svc = NewService(&fakeStore{}, WithDataMigration(
		func(context.Context) (any, error) { return map[string]any{"databases": []any{"adk"}}, nil },
		func(context.Context, any) (any, error) { return nil, wantErr },
	))
	status, err = svc.DataMigrationStatus(context.Background())
	if err != nil {
		t.Fatalf("delegated DataMigrationStatus: %v", err)
	}
	got, ok = status.(map[string]any)
	if !ok || len(got["databases"].([]any)) != 1 {
		t.Fatalf("delegated status = %#v", status)
	}
	if _, err := svc.ScheduleDatabaseRebuild(context.Background(), map[string]any{"mode": "single"}); !errors.Is(err, wantErr) {
		t.Fatalf("ScheduleDatabaseRebuild error = %v, want %v", err, wantErr)
	}
}
