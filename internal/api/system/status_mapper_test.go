package system

import (
	"encoding/json"
	"reflect"
	"testing"

	sys "github.com/jftrade/jftrade-main/internal/system"
)

func TestSystemStatusTransportMapperPreservesDomainJSON(t *testing.T) {
	status := sys.NewService(
		sys.WithAPIPort(3900),
		sys.WithSettingsPath("/tmp/jftrade/settings.json"),
		sys.WithLiveStats(func() *sys.LiveStats {
			return &sys.LiveStats{Connected: 2, ActiveInstruments: []string{"US.AAPL"}}
		}),
		sys.WithRuntimeResources(func() sys.RuntimeResources {
			return sys.RuntimeResources{
				CheckedAt: "2026-08-13T00:00:00Z",
				Count:     1,
				Items:     []sys.RuntimeResourceDescriptor{{ID: "settings-file", Owner: "settings"}},
			}
		}),
	).Status()

	var domainProjection any
	decodeStatusJSON(t, status, &domainProjection)
	var transportProjection any
	decodeStatusJSON(t, toSystemStatusResponse(status), &transportProjection)
	if !reflect.DeepEqual(transportProjection, domainProjection) {
		t.Fatalf("transport projection changed status JSON\ntransport=%#v\ndomain=%#v", transportProjection, domainProjection)
	}
}

func decodeStatusJSON(t *testing.T, value any, target any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatal(err)
	}
}
