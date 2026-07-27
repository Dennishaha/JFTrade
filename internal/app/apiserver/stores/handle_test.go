package stores

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestHandleClosesStoresInReverseOpenOrder(t *testing.T) {
	var handle Handle
	var closed []string
	open := func(name string) string {
		return Open(
			&handle,
			name,
			func() (string, error) { return name, nil },
			func(string) error {
				closed = append(closed, name)
				return nil
			},
		)
	}

	if open("design") == "" || open("catalog") == "" {
		t.Fatal("stores did not open")
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"catalog", "design"}; !reflect.DeepEqual(closed, want) {
		t.Fatalf("close order = %v, want %v", closed, want)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"catalog", "design"}; !reflect.DeepEqual(closed, want) {
		t.Fatalf("repeat Close ran stores again: %v", closed)
	}
}

func TestHandleRollsBackAndStopsAfterOpenFailure(t *testing.T) {
	startupErr := errors.New("catalog unavailable")
	closeErr := errors.New("design close failed")
	var handle Handle
	var opened []string
	var closed []string
	open := func(name string, openErr error, resourceCloseErr error) string {
		return Open(
			&handle,
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

	if open("design", nil, closeErr) == "" {
		t.Fatal("design store did not open")
	}
	if got := open("catalog", startupErr, nil); got != "" {
		t.Fatalf("failed store = %q, want zero value", got)
	}
	if got := open("execution", nil, nil); got != "" {
		t.Fatalf("store opened after rollback = %q", got)
	}

	err := handle.SetupError()
	if !errors.Is(err, startupErr) || !errors.Is(err, closeErr) {
		t.Fatalf("setup error = %v, want startup and rollback failures", err)
	}
	if !reflect.DeepEqual(opened, []string{"design", "catalog"}) {
		t.Fatalf("open order = %v", opened)
	}
	if !reflect.DeepEqual(closed, []string{"design"}) {
		t.Fatalf("rollback order = %v", closed)
	}
	if !strings.Contains(err.Error(), "open catalog") ||
		!strings.Contains(err.Error(), "close design") {
		t.Fatalf("setup error lacks resource names: %v", err)
	}
}
