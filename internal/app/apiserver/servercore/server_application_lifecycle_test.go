package servercore

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
)

func TestServerCloseUsesApplicationResourceOrderAndStableAggregation(t *testing.T) {
	server := &Server{}
	server.registerOwnedResources()
	firstErr := errors.New("first resource failed")
	secondErr := errors.New("second resource failed")
	var closed []string
	if err := server.lifecycle.Resources().Register("first test resource", func() error {
		closed = append(closed, "first")
		return firstErr
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.lifecycle.Resources().Register("second test resource", func() error {
		closed = append(closed, "second")
		return secondErr
	}); err != nil {
		t.Fatal(err)
	}

	err := server.Close()
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("Close error = %v, want both resource failures", err)
	}
	if !strings.Contains(err.Error(), "close first test resource") ||
		!strings.Contains(err.Error(), "close second test resource") {
		t.Fatalf("Close error lacks resource names: %v", err)
	}
	if !reflect.DeepEqual(closed, []string{"second", "first"}) {
		t.Fatalf("close order = %v, want reverse registration order", closed)
	}
	if second := server.Close(); second == nil || second.Error() != err.Error() {
		t.Fatalf("second Close = %v, want stable %v", second, err)
	}
	if !reflect.DeepEqual(closed, []string{"second", "first"}) {
		t.Fatalf("repeat Close ran resources again: %v", closed)
	}
}

func TestPersistentAssemblyStopsAndRollsBackInReverseOpenOrder(t *testing.T) {
	startupErr := errors.New("open execution database")
	closeErr := errors.New("close design database")
	state := serverPersistentState{resources: &appcomposition.Resources{}}
	var opened []string
	var closed []string

	open := func(name string, openErr error, resourceCloseErr error) string {
		return openPersistentResource(
			&state,
			name,
			func() (string, error) {
				opened = append(opened, name)
				return name, openErr
			},
			func(string) error {
				closed = append(closed, name)
				return resourceCloseErr
			},
		)
	}

	if got := open("strategy design store", nil, closeErr); got == "" {
		t.Fatal("strategy design store was not opened")
	}
	if got := open("strategy catalog", nil, nil); got == "" {
		t.Fatal("strategy catalog was not opened")
	}
	if got := open("execution order store", startupErr, nil); got != "" {
		t.Fatalf("failed resource = %q, want zero value", got)
	}
	if got := open("watchlist store", nil, nil); got != "" {
		t.Fatalf("resource opened after rollback = %q", got)
	}

	if !errors.Is(state.resourceSetupErr, startupErr) || !errors.Is(state.resourceSetupErr, closeErr) {
		t.Fatalf("assembly error = %v, want startup and rollback failures", state.resourceSetupErr)
	}
	if !reflect.DeepEqual(opened, []string{"strategy design store", "strategy catalog", "execution order store"}) {
		t.Fatalf("open order = %v", opened)
	}
	if !reflect.DeepEqual(closed, []string{"strategy catalog", "strategy design store"}) {
		t.Fatalf("rollback order = %v", closed)
	}
	if !strings.Contains(state.resourceSetupErr.Error(), "open execution order store") ||
		!strings.Contains(state.resourceSetupErr.Error(), "close strategy design store") {
		t.Fatalf("assembly error lacks resource names: %v", state.resourceSetupErr)
	}
}
