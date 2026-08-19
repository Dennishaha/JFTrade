package futu

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/futu/opend"
)

type stage5FutuCorpus struct {
	Version   string   `json:"version"`
	Protocols []string `json:"protocols"`
}

type stage5FutuExpected struct {
	Protocols []struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Value *struct {
			Dispatch   bool   `json:"dispatch"`
			Protocol   string `json:"protocol"`
			ProtocolID uint32 `json:"protocolId"`
			ReadOnly   bool   `json:"readOnly"`
		} `json:"value"`
	} `json:"protocols"`
}

func TestRustMigrationStage5OpenDTradeProtocolGateMatchesCorpus(t *testing.T) {
	var corpus stage5FutuCorpus
	var expected stage5FutuExpected
	readStage5FutuFixture(t, "trading-strategy-corpus.json", &corpus)
	readStage5FutuFixture(t, "trading-strategy-corpus.expected.json", &expected)
	if corpus.Version != "stage5.v1" || len(corpus.Protocols) != len(expected.Protocols) {
		t.Fatalf("stage 5 Futu corpus shape = version %q protocols %d/%d", corpus.Version, len(corpus.Protocols), len(expected.Protocols))
	}
	for index, protocol := range corpus.Protocols {
		protocolID, write, ok := stage5GoTradeProtocol(protocol)
		if !ok {
			t.Fatalf("unknown protocol fixture %q", protocol)
		}
		want := expected.Protocols[index]
		if write {
			if want.OK || want.Error == "" || want.Value != nil {
				t.Fatalf("write protocol %q is not rejected by shadow contract: %#v", protocol, want)
			}
			continue
		}
		if !want.OK || want.Value == nil || want.Value.Protocol != protocol || want.Value.ProtocolID != protocolID || !want.Value.ReadOnly || want.Value.Dispatch {
			t.Fatalf("read protocol %q drifted: %#v", protocol, want)
		}
	}
}

func stage5GoTradeProtocol(name string) (uint32, bool, bool) {
	switch name {
	case "get_account_list":
		return opend.ProtoTrdGetAccList, false, true
	case "get_position_list":
		return opend.ProtoTrdGetPositionList, false, true
	case "get_order_list":
		return opend.ProtoTrdGetOrderList, false, true
	case "update_order":
		return opend.ProtoTrdUpdateOrder, false, true
	case "update_order_fill":
		return opend.ProtoTrdUpdateOrderFill, false, true
	case "place_order":
		return opend.ProtoTrdPlaceOrder, true, true
	case "modify_order":
		return opend.ProtoTrdModifyOrder, true, true
	case "unlock_trade":
		return opend.ProtoTrdUnlockTrade, true, true
	default:
		return 0, false, false
	}
}

func readStage5FutuFixture(t *testing.T, name string, target any) {
	t.Helper()
	directory := os.Getenv("JFTRADE_STAGE5_FIXTURE_ROOT")
	if directory == "" {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("resolve stage 5 Futu test source")
		}
		directory = filepath.Join(filepath.Dir(source), "..", "..", "tests", "fixtures", "rust-migration", "stage5")
	}
	content, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		t.Fatal(err)
	}
}
