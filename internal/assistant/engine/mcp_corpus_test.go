package adk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpCorpusRoundTripper struct {
	base    http.RoundTripper
	entries []mcpCorpusExchange
}

type mcpCorpusExchange struct {
	method   string
	path     string
	headers  http.Header
	status   int
	request  map[string]any
	response map[string]any
}

func (r *mcpCorpusRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	resp, err := r.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(responseBody))
	var request map[string]any
	var response map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, err
	}
	r.entries = append(r.entries, mcpCorpusExchange{
		method: req.Method, path: req.URL.Path, headers: req.Header.Clone(), status: resp.StatusCode,
		request: request, response: response,
	})
	return resp, nil
}

func TestLocalMCPHandlerMatchesGoSDKV17Corpus(t *testing.T) {
	fixtureBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "mcp_go_sdk_v1_7_corpus.json"))
	if err != nil {
		t.Fatalf("read MCP corpus: %v", err)
	}
	var fixture struct {
		Entries []struct {
			Method        string              `json:"method"`
			Path          string              `json:"path"`
			RequestMethod string              `json:"requestMethod"`
			Headers       map[string][]string `json:"headers"`
			Status        int                 `json:"status"`
			Response      map[string]any      `json:"response"`
		} `json:"entries"`
		LegacyEntries []struct {
			Method        string              `json:"method"`
			Path          string              `json:"path"`
			RequestMethod string              `json:"requestMethod"`
			Headers       map[string][]string `json:"headers"`
			Status        int                 `json:"status"`
			Response      map[string]any      `json:"response"`
		} `json:"legacyEntries"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("decode MCP corpus: %v", err)
	}

	registry := NewToolRegistry()
	for _, toolName := range LocalMCPReadOnlyToolNames {
		name := toolName
		registry.Register(assistantmodel.ToolDescriptor{Name: name, DisplayName: name, Description: "fixture", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
			return map[string]any{"ok": true, "tool": name}, nil
		})
	}
	handler, err := NewLocalMCPHandler(NewRuntime(nil, registry))
	if err != nil {
		t.Fatalf("NewLocalMCPHandler: %v", err)
	}
	t.Cleanup(handler.Close)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	recorder := &mcpCorpusRoundTripper{base: server.Client().Transport}
	client := mcp.NewClient(&mcp.Implementation{Name: "jftrade-corpus", Version: "1.0"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: recorder},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("MCP initialize: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if _, err := session.ListTools(t.Context(), nil); err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "system.status"})
	if err != nil || result.IsError {
		t.Fatalf("tools/call: result=%#v err=%v", result, err)
	}
	if len(recorder.entries) != len(fixture.Entries) {
		t.Fatalf("recorded exchanges = %d, want %d", len(recorder.entries), len(fixture.Entries))
	}
	for index, expected := range fixture.Entries {
		got := recorder.entries[index]
		if got.method != expected.Method || got.path != expected.Path || got.status != expected.Status {
			t.Fatalf("exchange[%d] transport = %s %s %d, want %s %s %d", index, got.method, got.path, got.status, expected.Method, expected.Path, expected.Status)
		}
		if got.request["method"] != expected.RequestMethod {
			t.Fatalf("exchange[%d] request method = %#v, want %q", index, got.request["method"], expected.RequestMethod)
		}
		for key, values := range expected.Headers {
			if !reflect.DeepEqual(got.headers.Values(key), values) {
				t.Fatalf("exchange[%d] %s = %#v, want %#v", index, key, got.headers.Values(key), values)
			}
		}
		resultBody, ok := got.response["result"].(map[string]any)
		if !ok {
			t.Fatalf("exchange[%d] response result = %#v", index, got.response)
		}
		for key, want := range expected.Response {
			if key == "toolNames" {
				tools, _ := resultBody["tools"].([]any)
				names := make([]string, 0, len(tools))
				for _, item := range tools {
					if object, ok := item.(map[string]any); ok {
						if name, ok := object["name"].(string); ok {
							names = append(names, name)
						}
					}
				}
				wantValues, ok := want.([]any)
				if !ok {
					t.Fatalf("exchange[%d] corpus toolNames = %#v", index, want)
				}
				wantNames := make([]string, 0, len(wantValues))
				for _, item := range wantValues {
					name, ok := item.(string)
					if !ok {
						t.Fatalf("exchange[%d] corpus tool name = %#v", index, item)
					}
					wantNames = append(wantNames, name)
				}
				if !reflect.DeepEqual(names, wantNames) {
					t.Fatalf("exchange[%d] tool names = %#v, want %#v", index, names, wantNames)
				}
				continue
			}
			if key == "isError" && want == false {
				// Go omits the false-valued field; absence is the frozen success shape.
				if _, present := resultBody[key]; present {
					t.Fatalf("exchange[%d] success unexpectedly includes isError", index)
				}
				continue
			}
			if !reflect.DeepEqual(resultBody[key], want) {
				t.Fatalf("exchange[%d] result.%s = %#v, want %#v", index, key, resultBody[key], want)
			}
		}
	}
}

func TestLocalMCPHandlerMatchesGoSDKV17LegacyInitializeCorpus(t *testing.T) {
	fixtureBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "mcp_go_sdk_v1_7_corpus.json"))
	if err != nil {
		t.Fatalf("read MCP corpus: %v", err)
	}
	var fixture struct {
		LegacyEntries []struct {
			Method        string              `json:"method"`
			Path          string              `json:"path"`
			RequestMethod string              `json:"requestMethod"`
			Headers       map[string][]string `json:"headers"`
			Status        int                 `json:"status"`
			Response      map[string]any      `json:"response"`
		} `json:"legacyEntries"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("decode MCP corpus: %v", err)
	}
	registry := NewToolRegistry()
	registry.Register(assistantmodel.ToolDescriptor{Name: "system.status", DisplayName: "System Status", Description: "status", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true, "tool": "system.status"}, nil
	})
	handler, err := NewLocalMCPHandler(NewRuntime(nil, registry))
	if err != nil {
		t.Fatalf("NewLocalMCPHandler: %v", err)
	}
	t.Cleanup(handler.Close)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	recorder := &mcpCorpusRoundTripper{base: server.Client().Transport}
	client := &http.Client{Transport: recorder}
	payloads := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"jftrade-corpus","version":"1.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"system.status","arguments":{}}}`,
	}
	for _, payload := range payloads {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/mcp", bytes.NewReader([]byte(payload)))
		if err != nil {
			t.Fatalf("create legacy request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("Mcp-Protocol-Version", "2025-11-25")
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("legacy request: %v", err)
		}
		_ = response.Body.Close()
	}
	if len(recorder.entries) != len(fixture.LegacyEntries) {
		t.Fatalf("recorded legacy exchanges = %d, want %d", len(recorder.entries), len(fixture.LegacyEntries))
	}
	for index, expected := range fixture.LegacyEntries {
		got := recorder.entries[index]
		if got.method != expected.Method || got.path != expected.Path || got.status != expected.Status || got.request["method"] != expected.RequestMethod {
			t.Fatalf("legacy exchange[%d] = %#v, want method=%s path=%s status=%d rpc=%s", index, got, expected.Method, expected.Path, expected.Status, expected.RequestMethod)
		}
		for key, values := range expected.Headers {
			if !reflect.DeepEqual(got.headers.Values(key), values) {
				t.Fatalf("legacy exchange[%d] %s = %#v, want %#v", index, key, got.headers.Values(key), values)
			}
		}
		resultBody, ok := got.response["result"].(map[string]any)
		if !ok {
			t.Fatalf("legacy exchange[%d] result = %#v", index, got.response)
		}
		for key, want := range expected.Response {
			if key == "toolNames" {
				tools, _ := resultBody["tools"].([]any)
				if len(tools) != len(want.([]any)) {
					t.Fatalf("legacy exchange[%d] tool count = %d, want %d", index, len(tools), len(want.([]any)))
				}
				continue
			}
			if key == "isError" && want == false {
				if _, present := resultBody[key]; present {
					t.Fatalf("legacy exchange[%d] success unexpectedly includes isError", index)
				}
				continue
			}
			if !reflect.DeepEqual(resultBody[key], want) {
				t.Fatalf("legacy exchange[%d] result.%s = %#v, want %#v", index, key, resultBody[key], want)
			}
		}
	}
}
