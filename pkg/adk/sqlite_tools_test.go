package adk

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type sqliteDialectBoundaryModel struct {
	ID         uint `gorm:"primaryKey;autoIncrement"`
	Active     bool
	Count      int
	Unsigned   uint
	Price      float64
	Name       string
	CreatedAt  time.Time `gorm:"not null"`
	OptionalAt time.Time
	Raw        []byte
}

func (sqliteDialectBoundaryModel) TableName() string {
	return "adk_sqlite_dialect_boundary_models"
}

func TestSQLiteDialectorMigratesAndPersistsWithGORM(t *testing.T) {
	db := openTestSQLiteGORM(t)

	if db.Name() != "sqlite" {
		t.Fatalf("dialector name = %q, want sqlite", db.Name())
	}
	if db.Migrator() == nil {
		t.Fatal("migrator is nil")
	}
	if err := db.AutoMigrate(&sqliteDialectBoundaryModel{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	record := sqliteDialectBoundaryModel{
		Active:    true,
		Count:     7,
		Unsigned:  9,
		Price:     12.5,
		Name:      "alpha",
		CreatedAt: time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC),
		Raw:       []byte("payload"),
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("Create: %v", err)
	}
	if record.ID == 0 {
		t.Fatal("auto increment id was not assigned")
	}

	var got sqliteDialectBoundaryModel
	if err := db.First(&got, "name = ?", "alpha").Error; err != nil {
		t.Fatalf("First: %v", err)
	}
	if !got.Active || got.Count != 7 || got.Unsigned != 9 || got.Price != 12.5 || string(got.Raw) != "payload" {
		t.Fatalf("persisted model = %+v, want original scalar values", got)
	}
	if err := db.Delete(&got).Error; err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := db.First(&sqliteDialectBoundaryModel{}, got.ID).Error; err == nil {
		t.Fatal("deleted record was still readable")
	}
}

func TestSQLiteDialectorDataTypesDefaultsAndQuoting(t *testing.T) {
	db := openTestSQLiteGORM(t)
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&sqliteDialectBoundaryModel{}); err != nil {
		t.Fatalf("Parse model: %v", err)
	}
	dialector := sqliteDialector{}
	for _, tc := range []struct {
		field string
		want  string
	}{
		{field: "ID", want: "integer PRIMARY KEY AUTOINCREMENT"},
		{field: "Active", want: "numeric"},
		{field: "Count", want: "integer"},
		{field: "Unsigned", want: "integer"},
		{field: "Price", want: "real"},
		{field: "Name", want: "text"},
		{field: "CreatedAt", want: "datetime"},
		{field: "OptionalAt", want: "timestamp"},
		{field: "Raw", want: "blob"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			field := stmt.Schema.LookUpField(tc.field)
			if field == nil {
				t.Fatalf("field %s not found", tc.field)
			}
			if got := dialector.DataTypeOf(field); got != tc.want {
				t.Fatalf("DataTypeOf(%s) = %q, want %q", tc.field, got, tc.want)
			}
		})
	}

	idField := stmt.Schema.LookUpField("ID")
	nameField := stmt.Schema.LookUpField("Name")
	if got := dialector.DefaultValueOf(idField).(clause.Expr).SQL; got != "NULL" {
		t.Fatalf("auto increment default = %q, want NULL", got)
	}
	if got := dialector.DefaultValueOf(nameField).(clause.Expr).SQL; got != "DEFAULT" {
		t.Fatalf("regular default = %q, want DEFAULT", got)
	}

	var bind strings.Builder
	dialector.BindVarTo(&bind, stmt, "ignored")
	if got := bind.String(); got != "?" {
		t.Fatalf("BindVarTo wrote %q, want ?", got)
	}

	for _, tc := range []struct {
		input string
		want  string
	}{
		{input: "adk_runs.name", want: "`adk_runs`.`name`"},
		{input: "order", want: "`order`"},
	} {
		var quoted strings.Builder
		dialector.QuoteTo(&quoted, tc.input)
		if got := quoted.String(); got != tc.want {
			t.Fatalf("QuoteTo(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	if got := dialector.Explain("select * from adk_runs where id = ? and name = ?", 12, "alpha"); !strings.Contains(got, `12`) || !strings.Contains(got, `"alpha"`) {
		t.Fatalf("Explain output = %q, want interpolated vars", got)
	}
}

func TestSQLiteDialectorClauseBuildersAndVersionCompare(t *testing.T) {
	db := openTestSQLiteGORM(t)
	dialector := sqliteDialector{}
	builders := dialector.ClauseBuilders()

	insertStmt := &gorm.Statement{DB: db}
	insertStmt.Table = "adk_runs"
	builders["INSERT"](clause.Clause{Expression: clause.Insert{Modifier: "OR IGNORE"}}, insertStmt)
	if got := insertStmt.SQL.String(); got != "INSERT OR IGNORE INTO `adk_runs`" {
		t.Fatalf("INSERT builder = %q, want INSERT OR IGNORE INTO table", got)
	}

	limitStmt := &gorm.Statement{DB: db}
	limit := 10
	builders["LIMIT"](clause.Clause{Expression: clause.Limit{Limit: &limit, Offset: 5}}, limitStmt)
	if got := limitStmt.SQL.String(); got != "LIMIT 10 OFFSET 5" {
		t.Fatalf("LIMIT builder = %q, want LIMIT 10 OFFSET 5", got)
	}

	offsetOnlyStmt := &gorm.Statement{DB: db}
	builders["LIMIT"](clause.Clause{Expression: clause.Limit{Offset: 20}}, offsetOnlyStmt)
	if got := offsetOnlyStmt.SQL.String(); got != "LIMIT -1 OFFSET 20" {
		t.Fatalf("offset-only LIMIT builder = %q, want LIMIT -1 OFFSET 20", got)
	}

	lockingStmt := &gorm.Statement{DB: db}
	builders["FOR"](clause.Clause{Expression: clause.Locking{Strength: "UPDATE"}}, lockingStmt)
	if got := lockingStmt.SQL.String(); got != "" {
		t.Fatalf("FOR locking builder = %q, want empty SQLite locking clause", got)
	}

	for _, tc := range []struct {
		version  string
		required string
		want     int
	}{
		{version: "3.35.0", required: "3.35.0", want: 0},
		{version: "3.44.1", required: "3.35.0", want: 1},
		{version: "3.7.17", required: "3.35.0", want: -1},
		{version: "3..35-alpha", required: "3.0.35", want: 0},
	} {
		t.Run(fmt.Sprintf("%s_vs_%s", tc.version, tc.required), func(t *testing.T) {
			if got := compareSQLiteVersion(tc.version, tc.required); got != tc.want {
				t.Fatalf("compareSQLiteVersion(%q, %q) = %d, want %d", tc.version, tc.required, got, tc.want)
			}
		})
	}
}

func TestOpenAIToolInvocationParsingRestoresNamesAndCapsProviderOutput(t *testing.T) {
	calls := []openAIToolCall{
		openAIToolCallForBoundaryTest("   ", `{"ignored":true}`),
		openAIToolCallForBoundaryTest("market-snapshot", `{"market":"HK","limit":3}`),
		openAIToolCallForBoundaryTest("http-fetch", `not-json`),
		openAIToolCallForBoundaryTest("workflow-wait", ""),
		openAIToolCallForBoundaryTest("tools-search", `{"query":"strategy","limit":2}`),
		openAIToolCallForBoundaryTest("strategy-research_backtest", `{"script":"strategy('x')"}`),
		openAIToolCallForBoundaryTest("tasks-create", `{"title":"overflow should be capped"}`),
	}

	invocations := toolInvocationsFromOpenAI(calls)
	if len(invocations) != 5 {
		t.Fatalf("invocations len = %d, want cap of 5", len(invocations))
	}
	wantNames := []string{"market.snapshot", "http.fetch", "workflow.wait", "tools.search", "strategy.research_backtest"}
	for i, want := range wantNames {
		if invocations[i].Name != want {
			t.Fatalf("invocation[%d].Name = %q, want %q", i, invocations[i].Name, want)
		}
	}
	if got := invocations[0].Input["limit"]; got != float64(3) {
		t.Fatalf("parsed numeric JSON arg = %#v, want float64(3)", got)
	}
	if invocations[1].Input["rawParameters"] != "not-json" || !strings.Contains(fmt.Sprint(invocations[1].Input["parseError"]), "invalid character") {
		t.Fatalf("invalid JSON input = %#v, want rawParameters and parseError", invocations[1].Input)
	}
	if len(invocations[2].Input) != 0 {
		t.Fatalf("empty arguments input = %#v, want empty map", invocations[2].Input)
	}
}

func TestOpenAIToolsFromDescriptorsSanitizesProviderContract(t *testing.T) {
	tools := openAIToolsFromDescriptors([]ToolDescriptor{
		{Name: "   "},
		{
			Name:          "market.snapshot",
			Description:   "Fetch snapshot",
			OutputSummary: "latest quote",
			RiskLevel:     "low",
			InputSchema: map[string]any{
				"type":                 "object",
				"additionalProperties": true,
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "additionalProperties": true},
				},
			},
		},
		{Name: "workflow.wait"},
	})
	if len(tools) != 2 {
		t.Fatalf("tools len = %d, want blank descriptor skipped", len(tools))
	}
	first := tools[0]
	if first.Type != "function" || first.Function.Name != "market-snapshot" {
		t.Fatalf("first tool = %+v, want sanitized function name", first)
	}
	if !strings.Contains(first.Function.Description, "Output: latest quote") || !strings.Contains(first.Function.Description, "Risk: low") {
		t.Fatalf("description = %q, want output and risk metadata", first.Function.Description)
	}
	if _, ok := first.Function.Parameters["additionalProperties"]; ok {
		t.Fatalf("schema kept additionalProperties:true: %#v", first.Function.Parameters)
	}
	properties := schemaPropertiesForBoundaryTest(t, first.Function.Parameters)
	querySchema, ok := properties["query"].(map[string]any)
	if !ok {
		t.Fatalf("query schema = %#v, want object", properties["query"])
	}
	if _, ok := querySchema["additionalProperties"]; ok {
		t.Fatalf("nested schema kept additionalProperties:true: %#v", querySchema)
	}
	if tools[1].Function.Parameters == nil || tools[1].Function.Parameters["type"] != "object" {
		t.Fatalf("workflow.wait default schema = %#v, want generated object schema", tools[1].Function.Parameters)
	}
}

func TestDefaultToolSchemasCoverBusinessCriticalToolPayloads(t *testing.T) {
	for _, name := range []string{
		"http.fetch",
		"tools.search",
		"models.list",
		"workflow.wait",
		"tasks.create",
		"tasks.update",
		"tasks.delete",
		"tasks.list",
		"memory.remember",
		"memory.list",
		"memory.forget",
		"broker.orders",
		"broker.fills",
		"broker.cash_flows",
		"broker.fees",
		"broker.margin_ratios",
		"market.depth",
		"execution.order_events",
		"market.snapshot",
		"market.candles",
		"watchlist.list",
		"portfolio.summary",
		"strategy.optimize",
		"strategy.research_backtest",
		"backtest.result_view",
		"backtest.kline_sync_status",
		"strategy.pine_spec",
		"strategy.validate_pine",
		"strategy.save_draft",
		"strategy.save_definition",
		"strategy.update_instance_mode",
		"unknown.tool",
	} {
		t.Run(name, func(t *testing.T) {
			schema := defaultToolInputSchema(name)
			if schema["type"] != "object" {
				t.Fatalf("%s schema type = %#v, want object", name, schema["type"])
			}
			properties := schemaPropertiesForBoundaryTest(t, schema)
			if len(properties) == 0 {
				t.Fatalf("%s schema has no properties", name)
			}
			if schema["additionalProperties"] != false {
				t.Fatalf("%s additionalProperties = %#v, want false", name, schema["additionalProperties"])
			}
		})
	}

	for _, tc := range []struct {
		name     string
		required []string
	}{
		{name: "http.fetch", required: []string{"url"}},
		{name: "tasks.create", required: []string{"title"}},
		{name: "tasks.update", required: []string{"id"}},
		{name: "tasks.delete", required: []string{"id"}},
		{name: "memory.remember", required: []string{"key", "value"}},
		{name: "memory.forget", required: []string{"id"}},
		{name: "broker.cash_flows", required: []string{"clearingDate"}},
		{name: "market.depth", required: []string{"market", "symbol"}},
		{name: "market.snapshot", required: []string{"market", "symbol"}},
		{name: "market.candles", required: []string{"market", "symbol"}},
		{name: "strategy.optimize", required: []string{"definitionIds", "market", "symbol", "startTime", "endTime"}},
		{name: "strategy.research_backtest", required: []string{"script", "market", "startTime", "endTime"}},
		{name: "backtest.result_view", required: []string{"runId"}},
		{name: "backtest.kline_sync_status", required: []string{"taskId"}},
		{name: "strategy.validate_pine", required: []string{"script"}},
		{name: "strategy.save_definition", required: []string{"name", "script"}},
		{name: "strategy.update_instance_mode", required: []string{"instanceId", "executionMode"}},
	} {
		t.Run(tc.name+"_required", func(t *testing.T) {
			got, ok := defaultToolInputSchema(tc.name)["required"].([]string)
			if !ok {
				t.Fatalf("%s required = %#v, want []string", tc.name, defaultToolInputSchema(tc.name)["required"])
			}
			for _, required := range tc.required {
				if !slices.Contains(got, required) {
					t.Fatalf("%s required = %#v, missing %q", tc.name, got, required)
				}
			}
		})
	}

	candles := schemaPropertiesForBoundaryTest(t, defaultToolInputSchema("market.candles"))
	if _, ok := candles["period"]; !ok {
		t.Fatalf("market.candles missing period: %#v", candles)
	}
	if limit := candles["limit"].(map[string]any); limit["maximum"] != 500 {
		t.Fatalf("market.candles limit schema = %#v, want maximum 500", limit)
	}

	wait := schemaPropertiesForBoundaryTest(t, defaultToolInputSchema("workflow.wait"))
	if seconds := wait["seconds"].(map[string]any); seconds["maximum"] != 25 {
		t.Fatalf("workflow.wait seconds schema = %#v, want max 25", seconds)
	}
	if durationMs := wait["durationMs"].(map[string]any); durationMs["maximum"] != 25000 {
		t.Fatalf("workflow.wait durationMs schema = %#v, want max 25000", durationMs)
	}

	research := schemaPropertiesForBoundaryTest(t, defaultToolInputSchema("strategy.research_backtest"))
	resultView := research["resultView"].(map[string]any)
	if resultView["additionalProperties"] != false {
		t.Fatalf("research resultView schema = %#v, want closed nested schema", resultView)
	}
	if waitForCompletion := research["waitForCompletionMs"].(map[string]any); waitForCompletion["maximum"] != 25000 {
		t.Fatalf("research waitForCompletionMs schema = %#v, want max 25000", waitForCompletion)
	}

	modelsList := fmt.Sprint(defaultToolInputSchema("models.list"))
	if strings.Contains(modelsList, "apiKey") {
		t.Fatalf("models.list schema leaks api key fields: %s", modelsList)
	}

	watchlist := schemaPropertiesForBoundaryTest(t, defaultToolInputSchema("watchlist.list"))
	if includeQuotes := watchlist["includeQuotes"].(map[string]any); includeQuotes["default"] != false {
		t.Fatalf("watchlist.list includeQuotes schema = %#v, want default false", includeQuotes)
	}
	if limit := watchlist["limit"].(map[string]any); limit["maximum"] != 200 {
		t.Fatalf("watchlist.list limit schema = %#v, want maximum 200", limit)
	}
}

func TestToolRegistryAliasesModesAndNumericInputs(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{Name: "market.snapshot", DisplayName: "Market Snapshot", Permission: "read_internal"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	registry.Register(ToolDescriptor{Name: "orders.place", Permission: "live_trading"}, func(context.Context, map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})

	for _, alias := range []string{" market-snapshot ", "@jftrade market snapshot", "jftrade:market/snapshot", "Market Snapshot"} {
		if got, ok := registry.CanonicalName(alias); !ok || got != "market.snapshot" {
			t.Fatalf("CanonicalName(%q) = %q/%v, want market.snapshot", alias, got, ok)
		}
	}
	if got, ok := registry.CanonicalName("unknown"); ok || got != "" {
		t.Fatalf("CanonicalName unknown = %q/%v, want miss", got, ok)
	}
	if names := registry.AvailableNames(); !slices.Contains(names, "market.snapshot") || !slices.Contains(names, "workflow.wait") {
		t.Fatalf("available names = %#v, want built-in and registered tools", names)
	}

	live, ok := registry.Get("orders.place")
	if !ok {
		t.Fatal("orders.place not registered")
	}
	if live.Descriptor.RiskLevel != "critical" {
		t.Fatalf("live trading risk = %q, want critical", live.Descriptor.RiskLevel)
	}
	if ToolAllowedInMode(live.Descriptor, PermissionModeApproval) || !ToolAllowedInMode(live.Descriptor, PermissionModeAll) {
		t.Fatalf("live trading allowed modes not enforced: %+v", live.Descriptor)
	}

	input := map[string]any{
		"float":   float64(12.9),
		"int":     7,
		"string":  " 42 ",
		"invalid": "soon",
	}
	if got := toolIntValue(input, "float", 1); got != 12 {
		t.Fatalf("float toolIntValue = %d, want 12", got)
	}
	if got := toolIntValue(input, "int", 1); got != 7 {
		t.Fatalf("int toolIntValue = %d, want 7", got)
	}
	if got := toolIntValue(input, "string", 1); got != 42 {
		t.Fatalf("string toolIntValue = %d, want 42", got)
	}
	if got := toolIntValue(input, "invalid", 99); got != 99 {
		t.Fatalf("invalid toolIntValue = %d, want default 99", got)
	}
	if got := toolStringValue(map[string]any{"name": "agent"}, "name"); got != "agent" {
		t.Fatalf("toolStringValue = %q, want agent", got)
	}
}

func openTestSQLiteGORM(t *testing.T) *gorm.DB {
	t.Helper()
	managed, err := sqliteconn.Open(filepath.Join(t.TempDir(), "adk-gorm.db"))
	if err != nil {
		t.Fatalf("sqliteconn.Open: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, managed.Close()) })
	db, err := gorm.Open(sqliteDialector{Conn: newSQLiteGormPool(managed)}, &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return db
}

func openAIToolCallForBoundaryTest(name string, arguments string) openAIToolCall {
	var call openAIToolCall
	call.Function.Name = name
	call.Function.Arguments = arguments
	return call
}

func schemaPropertiesForBoundaryTest(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %#v, want map[string]any", schema["properties"])
	}
	return properties
}
