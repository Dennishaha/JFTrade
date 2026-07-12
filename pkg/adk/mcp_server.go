package adk

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LocalMCPReadOnlyToolNames is the explicit security boundary for the local
// MCP server. Newly registered ADK tools are intentionally not exposed until
// they are reviewed and added here.
var LocalMCPReadOnlyToolNames = []string{
	"system.status",
	"system.futu_opend",
	"plugins.catalog",
	"market.subscriptions",
	"market.snapshot",
	"market.candles",
	"market.depth",
	"watchlist.list",
	"portfolio.summary",
	"account.orders",
	"broker.orders",
	"broker.fills",
	"broker.cash_flows",
	"broker.fees",
	"broker.margin_ratios",
	"execution.order_events",
	"risk.state",
	"risk.events",
	"strategy.definitions",
	"strategy.pine_spec",
	"strategy.validate_pine",
	"backtest.runs",
	"backtest.result_view",
	"backtest.kline_sync_status",
}

// NewLocalMCPHandler bridges the reviewed read-only ADK tools to MCP's
// Streamable HTTP transport. Authentication and loopback enforcement belong to
// the listener owner because they are deployment settings, not tool concerns.
func NewLocalMCPHandler(runtime *Runtime) (http.Handler, error) {
	if runtime == nil || runtime.Tools() == nil {
		return nil, errors.New("ADK runtime is unavailable")
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "jftrade", Version: "1.0"}, &mcp.ServerOptions{
		Instructions: "JFTrade local read-only market, portfolio, risk, strategy, and backtest tools.",
	})
	registeredCount := 0
	for _, name := range LocalMCPReadOnlyToolNames {
		registered, ok := runtime.Tools().Get(name)
		if !ok {
			continue
		}
		descriptor := registered.Descriptor
		if descriptor.Permission != "read_internal" {
			// The name allowlist is intentionally not the only boundary: a later
			// registration must not be able to replace a reviewed read tool with a
			// write-capable implementation under the same name.
			continue
		}
		registeredTool := registered
		toolDescriptor := descriptor
		inputSchema := descriptor.InputSchema
		if inputSchema == nil {
			inputSchema = map[string]any{"type": "object"}
		}
		mcp.AddTool[map[string]any, any](server, &mcp.Tool{
			Name:        toolDescriptor.Name,
			Title:       toolDescriptor.DisplayName,
			Description: toolDescriptor.Description,
			InputSchema: inputSchema,
		}, func(ctx context.Context, _ *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
			runtime.RecordAudit(ctx, "mcp.tool.called", toolDescriptor.Name, "local MCP read-only tool call", map[string]any{"transport": "streamable_http"})
			output, err := executeRegisteredTool(ctx, registeredTool, input)
			if err != nil {
				runtime.RecordAudit(ctx, "mcp.tool.failed", toolDescriptor.Name, "local MCP read-only tool call failed", map[string]any{"transport": "streamable_http"})
				return nil, nil, err
			}
			return nil, limitToolOutput(output), nil
		})
		registeredCount++
	}
	if registeredCount == 0 {
		return nil, errors.New("no reviewed MCP tools are registered")
	}

	// Leave DisableLocalhostProtection unset so the SDK's DNS rebinding defense
	// remains active even though the listener itself is loopback-only.
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		JSONResponse:   true,
		SessionTimeout: 5 * time.Minute,
	}), nil
}
