package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeStrategyPineRouteReturnsDiagnosticsAndRequirements(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"sourceFormat":"pine-v6","includeAst":true,"script":"//@version=6\nstrategy(\"Analyze\", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10, pyramiding=2)\nstart = input.time(timestamp(2026, 1, 1), \"Start\")\nsignalColor = input.color(color.green, \"Signal\")\nfast = ta.ema(close, 8)\navgVol = ta.sma(volume, 20)\nsar = ta.sar(0.02, 0.02, 0.2)\nif barstate.isconfirmed and session.ismarket and dayofweek == dayofweek.monday and time >= start and close > close[1] and volume > avgVol and close > sar\n    strategy.entry(\"Long\", strategy.long)"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST analyze status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			OK           bool             `json:"ok"`
			Diagnostics  []map[string]any `json:"diagnostics"`
			Features     []string         `json:"features"`
			Requirements struct {
				Indicators []map[string]any `json:"indicators"`
			} `json:"requirements"`
			Metadata map[string]any `json:"metadata"`
			AST      map[string]any `json:"ast"`
			Semantic map[string]any `json:"semantic"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if !envelope.OK || !envelope.Data.OK {
		t.Fatalf("analyze envelope = %#v", envelope)
	}
	if len(envelope.Data.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want empty", envelope.Data.Diagnostics)
	}
	if len(envelope.Data.Features) == 0 {
		t.Fatal("features is empty")
	}
	if len(envelope.Data.Requirements.Indicators) != 3 {
		t.Fatalf("indicators = %#v, want three", envelope.Data.Requirements.Indicators)
	}
	if !stringSliceContains(envelope.Data.Features, "expression.input_defaults") ||
		!stringSliceContains(envelope.Data.Features, "indicator.ma_source_aware") ||
		!stringSliceContains(envelope.Data.Features, "expression.barstate_session") ||
		!stringSliceContains(envelope.Data.Features, "expression.pine_constants") ||
		!stringSliceContains(envelope.Data.Features, "indicator.sar") ||
		!stringSliceContains(envelope.Data.Features, "order.strategy_order_net") ||
		!stringSliceContains(envelope.Data.Features, "order.qty_percent") ||
		!stringSliceContains(envelope.Data.Features, "order.close_all") ||
		!stringSliceContains(envelope.Data.Features, "order.exit_quantity") {
		t.Fatalf("features = %#v", envelope.Data.Features)
	}
	if envelope.Data.Requirements.Indicators[1]["key"] != "ma:SMA:20:volume" {
		t.Fatalf("indicators = %#v, want source-aware volume MA", envelope.Data.Requirements.Indicators)
	}
	if envelope.Data.Requirements.Indicators[2]["key"] != "sar:0.02:0.02:0.2" {
		t.Fatalf("indicators = %#v, want SAR requirement", envelope.Data.Requirements.Indicators)
	}
	if envelope.Data.Metadata["defaultQtyMode"] != "percent_of_equity" || envelope.Data.Metadata["defaultQtyValue"] != "10" || envelope.Data.Metadata["pyramiding"] != float64(2) {
		t.Fatalf("metadata = %#v", envelope.Data.Metadata)
	}
	if envelope.Data.AST == nil {
		t.Fatal("ast missing")
	}
	if envelope.Data.Semantic == nil {
		t.Fatal("semantic missing")
	}
	if symbols, ok := envelope.Data.Semantic["symbols"].([]any); !ok || len(symbols) == 0 {
		t.Fatalf("semantic symbols = %#v, want non-empty", envelope.Data.Semantic["symbols"])
	}
}

func TestAnalyzeStrategyPineRouteOmitsASTByDefault(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"script":"//@version=6\nstrategy(\"Analyze\", overlay=true)\nstrategy.entry(\"Long\", strategy.long)"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST analyze status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("analyze envelope = %#v", envelope)
	}
	if _, exists := envelope.Data["ast"]; exists {
		t.Fatalf("ast key exists with includeAst=false: %#v", envelope.Data["ast"])
	}
	if envelope.Data["sourceFormat"] != "pine-v6" {
		t.Fatalf("sourceFormat = %#v, want pine-v6", envelope.Data["sourceFormat"])
	}
}

func TestAnalyzeStrategyPineRouteRejectsUnsupportedSourceFormat(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"sourceFormat":"legacy","script":"//@version=6\nstrategy(\"Analyze\", overlay=true)"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST analyze status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze error: %v", err)
	}
	if envelope.OK || envelope.Error.Code != "BAD_REQUEST" || envelope.Error.Message != "strategy-pine analyze supports pine-v6 only" {
		t.Fatalf("unexpected error envelope: %#v", envelope)
	}
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestAnalyzeStrategyPineRouteReportsUnsupportedSyntax(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"script":"//@version=6\nstrategy(\"Analyze\", overlay=true)\nimport TradingView/ta/7"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST analyze status = %d", resp.StatusCode)
	}
	var envelope struct {
		Data struct {
			OK          bool `json:"ok"`
			Diagnostics []struct {
				Code string `json:"code"`
				Line int    `json:"line"`
			} `json:"diagnostics"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if envelope.Data.OK {
		t.Fatal("analyze ok = true, want false")
	}
	if len(envelope.Data.Diagnostics) == 0 || envelope.Data.Diagnostics[0].Code != "PINE_DECLARATION_UNSUPPORTED" || envelope.Data.Diagnostics[0].Line != 3 {
		t.Fatalf("diagnostics = %#v", envelope.Data.Diagnostics)
	}
}

func TestAnalyzeStrategyPineRouteReturnsV20ParseOnlyMetadata(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"includeAst":true,"script":"//@version=6\nstrategy(\"v2 metadata\", overlay=true)\nvar array<int> arr = array.new_float(0)\narray.push(arr, close)\narr.push(open)\ntype TradeBox\n    float price = close\nmethod reset(TradeBox box, float limit = 0) =>\n    box\nbox = TradeBox.new(close)\nresetBox = box.reset(10)\nimport TradingView/ta/7 as tav7\nlbl = label.new(bar_index, close, \"Entry\")\nplot(close, title=\"Close\")"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST analyze status = %d", resp.StatusCode)
	}
	var envelope struct {
		Data struct {
			OK          bool `json:"ok"`
			Diagnostics []struct {
				Code string `json:"code"`
			} `json:"diagnostics"`
			Declarations []struct {
				Kind      string `json:"kind"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
				Alias     string `json:"alias"`
				Call      string `json:"call"`
				TypeArgs  string `json:"typeArgs"`
				Receiver  *struct {
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"receiver"`
				Parameters []struct {
					Name    string `json:"name"`
					Type    string `json:"type"`
					Default string `json:"default"`
				} `json:"parameters"`
				Fields []struct {
					Name    string `json:"name"`
					Type    string `json:"type"`
					Default string `json:"default"`
				} `json:"fields"`
				ImportPath string `json:"importPath"`
				Version    string `json:"version"`
				Executable bool   `json:"executable"`
			} `json:"declarations"`
			CollectionOperations []struct {
				Namespace string   `json:"namespace"`
				Operation string   `json:"operation"`
				Call      string   `json:"call"`
				Signature string   `json:"signature"`
				Target    string   `json:"target"`
				Arguments []string `json:"arguments"`
				Mutates   bool     `json:"mutates"`
				Supported bool     `json:"supported"`
			} `json:"collectionOperations"`
			ObjectOperations []struct {
				Kind      string   `json:"kind"`
				Type      string   `json:"type"`
				Method    string   `json:"method"`
				Call      string   `json:"call"`
				Signature string   `json:"signature"`
				Target    string   `json:"target"`
				Arguments []string `json:"arguments"`
				Supported bool     `json:"supported"`
			} `json:"objectOperations"`
			Visuals []struct {
				Kind     string `json:"kind"`
				Call     string `json:"call"`
				Variable string `json:"variable"`
				Target   string `json:"target"`
				Title    string `json:"title"`
			} `json:"visuals"`
			Semantic map[string]any `json:"semantic"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if envelope.Data.OK {
		t.Fatal("analyze ok = true, want false for parse-only collection/declaration")
	}
	hasCollectionTypeDiagnostic := false
	for _, diagnostic := range envelope.Data.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_COLLECTION_TYPE" {
			hasCollectionTypeDiagnostic = true
		}
	}
	if !hasCollectionTypeDiagnostic {
		t.Fatalf("diagnostics = %#v, want collection type diagnostic", envelope.Data.Diagnostics)
	}
	if len(envelope.Data.Declarations) != 4 {
		t.Fatalf("declarations = %#v, want collection, type, method, and import", envelope.Data.Declarations)
	}
	if envelope.Data.Declarations[0].Kind != "collection" || envelope.Data.Declarations[0].Namespace != "array" || envelope.Data.Declarations[0].Call != "array.new_float" || envelope.Data.Declarations[0].TypeArgs != "int" || envelope.Data.Declarations[0].Name != "arr" || !envelope.Data.Declarations[0].Executable {
		t.Fatalf("collection declaration = %#v", envelope.Data.Declarations[0])
	}
	if envelope.Data.Declarations[1].Kind != "type" || envelope.Data.Declarations[1].Name != "TradeBox" || envelope.Data.Declarations[1].Executable {
		t.Fatalf("type declaration = %#v", envelope.Data.Declarations[1])
	}
	if len(envelope.Data.Declarations[1].Fields) != 1 || envelope.Data.Declarations[1].Fields[0].Type != "float" || envelope.Data.Declarations[1].Fields[0].Name != "price" || envelope.Data.Declarations[1].Fields[0].Default != "close" {
		t.Fatalf("type fields = %#v", envelope.Data.Declarations[1].Fields)
	}
	if envelope.Data.Declarations[2].Kind != "method" || envelope.Data.Declarations[2].Name != "reset" || envelope.Data.Declarations[2].Receiver == nil || envelope.Data.Declarations[2].Receiver.Type != "TradeBox" || envelope.Data.Declarations[2].Receiver.Name != "box" || len(envelope.Data.Declarations[2].Parameters) != 2 {
		t.Fatalf("method declaration = %#v", envelope.Data.Declarations[2])
	}
	if envelope.Data.Declarations[3].Kind != "import" || envelope.Data.Declarations[3].ImportPath != "TradingView/ta/7" || envelope.Data.Declarations[3].Version != "7" || envelope.Data.Declarations[3].Alias != "tav7" {
		t.Fatalf("import declaration = %#v", envelope.Data.Declarations[3])
	}
	if len(envelope.Data.CollectionOperations) != 3 {
		t.Fatalf("collection operations = %#v", envelope.Data.CollectionOperations)
	}
	if envelope.Data.CollectionOperations[0].Call != "array.new_float" || envelope.Data.CollectionOperations[0].Target != "arr" || !envelope.Data.CollectionOperations[0].Mutates {
		t.Fatalf("constructor operation = %#v", envelope.Data.CollectionOperations[0])
	}
	if envelope.Data.CollectionOperations[1].Call != "array.push" || envelope.Data.CollectionOperations[1].Signature != "array.push(id, value)" || envelope.Data.CollectionOperations[1].Target != "arr" || !envelope.Data.CollectionOperations[1].Mutates || !envelope.Data.CollectionOperations[1].Supported || len(envelope.Data.CollectionOperations[1].Arguments) != 2 {
		t.Fatalf("push operation = %#v", envelope.Data.CollectionOperations[1])
	}
	if envelope.Data.CollectionOperations[2].Call != "array.push" || envelope.Data.CollectionOperations[2].Signature != "array.push(id, value)" || envelope.Data.CollectionOperations[2].Target != "arr" || !envelope.Data.CollectionOperations[2].Mutates || !envelope.Data.CollectionOperations[2].Supported || len(envelope.Data.CollectionOperations[2].Arguments) != 2 || envelope.Data.CollectionOperations[2].Arguments[0] != "arr" {
		t.Fatalf("method-style push operation = %#v", envelope.Data.CollectionOperations[2])
	}
	if len(envelope.Data.ObjectOperations) != 2 {
		t.Fatalf("object operations = %#v", envelope.Data.ObjectOperations)
	}
	if envelope.Data.ObjectOperations[0].Kind != "constructor" || envelope.Data.ObjectOperations[0].Type != "TradeBox" || envelope.Data.ObjectOperations[0].Call != "TradeBox.new" || envelope.Data.ObjectOperations[0].Signature != "TradeBox.new(float price = close)" || envelope.Data.ObjectOperations[0].Target != "box" || !envelope.Data.ObjectOperations[0].Supported || len(envelope.Data.ObjectOperations[0].Arguments) != 1 {
		t.Fatalf("constructor object operation = %#v", envelope.Data.ObjectOperations[0])
	}
	if envelope.Data.ObjectOperations[1].Kind != "method" || envelope.Data.ObjectOperations[1].Type != "TradeBox" || envelope.Data.ObjectOperations[1].Method != "reset" || envelope.Data.ObjectOperations[1].Call != "box.reset" || envelope.Data.ObjectOperations[1].Signature != "reset(TradeBox box, float limit = 0)" || envelope.Data.ObjectOperations[1].Target != "box" || !envelope.Data.ObjectOperations[1].Supported || len(envelope.Data.ObjectOperations[1].Arguments) != 1 {
		t.Fatalf("method object operation = %#v", envelope.Data.ObjectOperations[1])
	}
	if len(envelope.Data.Visuals) != 2 {
		t.Fatalf("visuals = %#v", envelope.Data.Visuals)
	}
	if envelope.Data.Visuals[0].Kind != "drawing" || envelope.Data.Visuals[0].Call != "label.new" || envelope.Data.Visuals[0].Variable != "lbl" || envelope.Data.Visuals[0].Title != "Entry" {
		t.Fatalf("assigned visual = %#v", envelope.Data.Visuals[0])
	}
	if envelope.Data.Visuals[1].Kind != "plot" || envelope.Data.Visuals[1].Call != "plot" || envelope.Data.Visuals[1].Target != "close" || envelope.Data.Visuals[1].Title != "Close" {
		t.Fatalf("plot visual = %#v", envelope.Data.Visuals[1])
	}
	if envelope.Data.Semantic == nil {
		t.Fatal("semantic missing")
	}
}

func TestAnalyzeStrategyPineRouteReturnsObjectSignatureDiagnostics(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"includeAst":true,"script":"//@version=6\nstrategy(\"bad object\", overlay=true)\ntype TradeBox\n    float price\nmethod reset(TradeBox box, float limit) =>\n    box\nbox = TradeBox.new()\nresetBox = box.reset()"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST analyze status = %d", resp.StatusCode)
	}
	var envelope struct {
		Data struct {
			OK          bool `json:"ok"`
			Diagnostics []struct {
				Code string `json:"code"`
			} `json:"diagnostics"`
			ObjectOperations []struct {
				Kind      string   `json:"kind"`
				Signature string   `json:"signature"`
				Arguments []string `json:"arguments"`
			} `json:"objectOperations"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if envelope.Data.OK {
		t.Fatal("analyze ok = true, want false for object signature diagnostics")
	}
	if len(envelope.Data.ObjectOperations) != 2 {
		t.Fatalf("object operations = %#v", envelope.Data.ObjectOperations)
	}
	objectSignatureDiagnostics := 0
	for _, diagnostic := range envelope.Data.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_OBJECT_SIGNATURE" {
			objectSignatureDiagnostics++
		}
	}
	if objectSignatureDiagnostics != 2 {
		t.Fatalf("diagnostics = %#v, want two object signature diagnostics", envelope.Data.Diagnostics)
	}
	if envelope.Data.ObjectOperations[0].Kind != "constructor" || envelope.Data.ObjectOperations[0].Signature != "TradeBox.new(float price)" || len(envelope.Data.ObjectOperations[0].Arguments) != 0 {
		t.Fatalf("constructor operation = %#v", envelope.Data.ObjectOperations[0])
	}
	if envelope.Data.ObjectOperations[1].Kind != "method" || envelope.Data.ObjectOperations[1].Signature != "reset(TradeBox box, float limit)" || len(envelope.Data.ObjectOperations[1].Arguments) != 0 {
		t.Fatalf("method operation = %#v", envelope.Data.ObjectOperations[1])
	}
}

func TestAnalyzeStrategyPineRouteReturnsImportAliasDiagnostics(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"includeAst":true,"script":"//@version=6\nstrategy(\"bad import\", overlay=true)\nimport TradingView/ta/7 as tools\nimport TradingView/math/1 as tools"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST analyze status = %d", resp.StatusCode)
	}
	var envelope struct {
		Data struct {
			OK          bool `json:"ok"`
			Diagnostics []struct {
				Code string `json:"code"`
			} `json:"diagnostics"`
			Declarations []struct {
				Kind       string `json:"kind"`
				ImportPath string `json:"importPath"`
				Alias      string `json:"alias"`
			} `json:"declarations"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if envelope.Data.OK {
		t.Fatal("analyze ok = true, want false for import alias diagnostics")
	}
	if len(envelope.Data.Declarations) != 2 || envelope.Data.Declarations[0].Alias != "tools" || envelope.Data.Declarations[1].Alias != "tools" {
		t.Fatalf("declarations = %#v", envelope.Data.Declarations)
	}
	aliasDiagnostics := 0
	for _, diagnostic := range envelope.Data.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_DECLARATION" {
			aliasDiagnostics++
		}
	}
	if aliasDiagnostics != 1 {
		t.Fatalf("diagnostics = %#v, want one import alias diagnostic", envelope.Data.Diagnostics)
	}
}

func TestAnalyzeStrategyPineRouteReturnsTypeMethodRegistryDiagnostics(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	srv := httptest.NewServer(server)
	t.Cleanup(srv.Close)

	body := []byte(`{"includeAst":true,"script":"//@version=6\nstrategy(\"declaration registry\", overlay=true)\ntype TradeBox\n    float price\ntype TradeBox\n    int bars\nmethod reset(TradeBox box, float limit) =>\n    box\nmethod reset(TradeBox target, float threshold = 0) =>\n    target\nmethod haunt(Ghost ghost) =>\n    ghost"}`)
	resp, err := http.Post(srv.URL+"/api/v1/strategy-pine/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST analyze: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST analyze status = %d", resp.StatusCode)
	}
	var envelope struct {
		Data struct {
			OK          bool `json:"ok"`
			Diagnostics []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"diagnostics"`
			Declarations []struct {
				Kind     string `json:"kind"`
				Name     string `json:"name"`
				Receiver *struct {
					Name string `json:"name"`
					Type string `json:"type"`
				} `json:"receiver"`
			} `json:"declarations"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if envelope.Data.OK {
		t.Fatal("analyze ok = true, want false for declaration registry diagnostics")
	}
	if len(envelope.Data.Declarations) != 5 {
		t.Fatalf("declarations = %#v", envelope.Data.Declarations)
	}
	declarationDiagnostics := 0
	messages := make([]string, 0)
	for _, diagnostic := range envelope.Data.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_DECLARATION" {
			declarationDiagnostics++
			messages = append(messages, diagnostic.Message)
		}
	}
	if declarationDiagnostics != 3 {
		t.Fatalf("diagnostics = %#v, want duplicate type, duplicate method, and unknown receiver diagnostics", envelope.Data.Diagnostics)
	}
	joined := strings.Join(messages, "\n")
	for _, fragment := range []string{"type TradeBox is already declared", "method reset is already declared", "receiver type Ghost is not declared"} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("diagnostic messages = %q, missing %q", joined, fragment)
		}
	}
	if envelope.Data.Declarations[2].Receiver == nil || envelope.Data.Declarations[2].Receiver.Type != "TradeBox" {
		t.Fatalf("method declaration = %#v", envelope.Data.Declarations[2])
	}
}
