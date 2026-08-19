package futu

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/futu/codec"
)

type stage4FutuCorpus struct {
	Version string `json:"version"`
	Futu    struct {
		Refs          []marketdata.InstrumentRef `json:"refs"`
		FrameProtoID  uint32                     `json:"frameProtoId"`
		FrameSerialNo uint32                     `json:"frameSerialNo"`
		FrameBody     []byte                     `json:"frameBody"`
	} `json:"futu"`
}

func TestRustMigrationStage4OpenDFrameAndSubscriptionPlanMatchesCorpus(t *testing.T) {
	directory := os.Getenv("JFTRADE_STAGE4_FIXTURE_ROOT")
	if directory == "" {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("resolve stage 4 Futu test source")
		}
		directory = filepath.Join(filepath.Dir(source), "..", "..", "..", "tests", "fixtures", "rust-migration", "stage4")
	}
	path := filepath.Join(directory, "provider-lifecycle-corpus.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var corpus stage4FutuCorpus
	if err := json.Unmarshal(content, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Version != "stage4.v1" {
		t.Fatalf("stage 4 corpus version = %q", corpus.Version)
	}

	physical, logical := desiredPhysicalSubscriptions(corpus.Futu.Refs)
	if logical != 3 {
		t.Fatalf("logical subscriptions = %d, want 3", logical)
	}
	wantKeys := []string{"BASIC:US.AAPL", "KLINE:US.AAPL:1m", "ORDER_BOOK:HK.00700"}
	if keys := sortedPhysicalKeys(physical); !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("physical subscriptions = %#v", keys)
	}

	packet, err := codec.Encode(corpus.Futu.FrameProtoID, corpus.Futu.FrameSerialNo, corpus.Futu.FrameBody)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := codec.Decode(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != 49 || frame.Header.ProtoID != 3004 || frame.Header.SerialNo != 42 ||
		!reflect.DeepEqual(frame.Body, []byte{1, 2, 3, 4, 5}) {
		t.Fatalf("OpenD frame = len:%d header:%#v body:%v", len(packet), frame.Header, frame.Body)
	}
	if subscriptionRetryDelay(0).Seconds() != 5 || subscriptionRetryDelay(99).Seconds() != 30 {
		t.Fatal("OpenD subscription retry schedule drifted from stage 4 contract")
	}
}
