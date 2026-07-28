package pineruntime

import (
	"testing"

	jftsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestManagerReconfigureRejectsNilManager(t *testing.T) {
	var manager *Manager
	if _, enabled, err := manager.Reconfigure(jftsettings.PineWorkerSettings{}, nil); err == nil || enabled {
		t.Fatalf("nil manager Reconfigure enabled=%v error=%v, want disabled error", enabled, err)
	}
}
