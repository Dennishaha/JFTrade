package rustmigration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/researchscreen"
)

type stage9ResearchScreenCatalogFixture struct {
	Version  string                            `json:"version"`
	Catalogs map[string]researchscreen.Catalog `json:"catalogs"`
}

func TestStage9ResearchScreenCatalogFixtureMatchesCurrentGoOwner(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve stage 9 research screen catalog fixture source")
	}
	path := filepath.Join(
		filepath.Dir(source),
		"../../tests/fixtures/rust-migration/stage9/research-screen-catalogs.json",
	)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stage 9 research screen catalog fixture: %v", err)
	}
	var got stage9ResearchScreenCatalogFixture
	if err := json.Unmarshal(contents, &got); err != nil {
		t.Fatalf("decode stage 9 research screen catalog fixture: %v", err)
	}
	want := stage9ResearchScreenCatalogFixture{
		Version:  "stage9.research-screen-catalogs.v1",
		Catalogs: map[string]researchscreen.Catalog{},
	}
	for _, query := range []struct{ broker, market string }{
		{broker: "futu"}, {broker: "futu", market: "HK"}, {broker: "futu", market: "US"},
		{broker: "futu", market: "SH"}, {broker: "futu", market: "SZ"},
		{broker: "yfinance"}, {broker: "yfinance", market: "US"},
		{broker: "akshare"}, {broker: "akshare", market: "SH"}, {broker: "akshare", market: "SZ"},
		{broker: "akshare", market: "CN"}, {broker: "akshare", market: "HK"}, {broker: "akshare", market: "US"},
	} {
		key := query.broker + "|" + query.market
		if query.broker == "futu" {
			want.Catalogs[key] = researchscreen.CatalogForMarket(query.market)
		} else {
			want.Catalogs[key] = researchscreen.EmbeddedCatalog(query.broker, query.market)
		}
	}
	if got.Version != want.Version || len(got.Catalogs) != len(want.Catalogs) {
		t.Fatalf("stage 9 research screen catalog fixture header drifted from the Go owner")
	}
	for key, expected := range want.Catalogs {
		actual, ok := got.Catalogs[key]
		if !ok {
			t.Fatalf("stage 9 research screen catalog fixture is missing %s", key)
		}
		actualJSON, err := json.Marshal(actual)
		if err != nil {
			t.Fatalf("encode actual catalog %s: %v", key, err)
		}
		expectedJSON, err := json.Marshal(expected)
		if err != nil {
			t.Fatalf("encode expected catalog %s: %v", key, err)
		}
		if string(actualJSON) != string(expectedJSON) {
			t.Fatalf("stage 9 research screen catalog %s drifted from the Go owner", key)
		}
	}
}
