package backtest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const stage3ExecutionModel = "conservative-bar-v1"

type stage3Corpus struct {
	Version int          `json:"version"`
	Cases   []stage3Case `json:"cases"`
}

type stage3Case struct {
	ID                   string              `json:"id"`
	Symbol               string              `json:"symbol"`
	BaseCurrency         string              `json:"baseCurrency"`
	QuoteCurrency        string              `json:"quoteCurrency"`
	InitialBalance       string              `json:"initialBalance"`
	ProcessOrdersOnClose bool                `json:"processOrdersOnClose"`
	SlippageTicks        int                 `json:"slippageTicks"`
	Market               stage3Market        `json:"market"`
	FeeRules             []stage3FeeRule     `json:"feeRules"`
	IndicatorPeriods     []int               `json:"indicatorPeriods"`
	CancelBeforeBar      *int                `json:"cancelBeforeBar"`
	Candles              []stage3Candle      `json:"candles"`
	Intents              []stage3OrderIntent `json:"intents"`
}

type stage3Market struct {
	TickSize     string `json:"tickSize"`
	QuantityStep string `json:"quantityStep"`
	MinQuantity  string `json:"minQuantity"`
}

type stage3Candle struct {
	Start  string `json:"start"`
	End    string `json:"end"`
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

type stage3OrderIntent struct {
	BarIndex      int    `json:"barIndex"`
	Action        string `json:"action"`
	ID            string `json:"id"`
	TargetID      string `json:"targetId"`
	Side          string `json:"side"`
	OrderType     string `json:"orderType"`
	Quantity      string `json:"quantity"`
	LimitPrice    string `json:"limitPrice"`
	StopPrice     string `json:"stopPrice"`
	ReduceOnly    bool   `json:"reduceOnly"`
	ParentID      string `json:"parentId"`
	OCOGroupID    string `json:"ocoGroupId"`
	AtomicGroupID string `json:"atomicGroupId"`
}

type stage3FeeRule struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Group       string `json:"group"`
	Side        string `json:"side"`
	Basis       string `json:"basis"`
	Rate        string `json:"rate"`
	FixedAmount string `json:"fixedAmount"`
	MinAmount   string `json:"minAmount"`
	MaxAmount   string `json:"maxAmount"`
	MaxRate     string `json:"maxRate"`
	Rounding    string `json:"rounding"`
}

type stage3CorpusOutput struct {
	Version        int                `json:"version"`
	ExecutionModel string             `json:"executionModel"`
	Cases          []stage3CaseOutput `json:"cases"`
}

type stage3CaseOutput struct {
	ID              string                `json:"id"`
	Status          string                `json:"status"`
	ProcessedBars   int                   `json:"processedBars"`
	Cash            string                `json:"cash"`
	BasePosition    string                `json:"basePosition"`
	FinalEquity     string                `json:"finalEquity"`
	RealizedPnL     string                `json:"realizedPnl"`
	TotalBrokerFees string                `json:"totalBrokerFees"`
	TotalMarketFees string                `json:"totalMarketFees"`
	TotalFees       string                `json:"totalFees"`
	TotalFills      int                   `json:"totalFills"`
	TotalTrades     int                   `json:"totalTrades"`
	WinningTrades   int                   `json:"winningTrades"`
	WinRate         string                `json:"winRate"`
	MaxDrawdown     string                `json:"maxDrawdown"`
	CurrentDrawdown string                `json:"currentDrawdown"`
	Orders          []stage3OrderOutput   `json:"orders"`
	Fills           []stage3FillOutput    `json:"fills"`
	EquityCurve     []stage3EquityPoint   `json:"equityCurve"`
	DrawdownCurve   []stage3DrawdownPoint `json:"drawdownCurve"`
	FeeBreakdown    []stage3FeeBreakdown  `json:"feeBreakdown"`
	Indicators      []stage3Indicator     `json:"indicators"`
	Warnings        []string              `json:"warnings"`
	ResultHash      string                `json:"resultHash"`
}

type stage3OrderOutput struct {
	OrderID        string `json:"orderId"`
	ClientOrderID  string `json:"clientOrderId"`
	Side           string `json:"side"`
	OrderType      string `json:"orderType"`
	Quantity       string `json:"quantity"`
	Status         string `json:"status"`
	FilledQuantity string `json:"filledQuantity"`
	FilledPrice    string `json:"filledPrice"`
	SubmittedAt    string `json:"submittedAt"`
	FilledAt       string `json:"filledAt"`
	ReduceOnly     bool   `json:"reduceOnly"`
}

type stage3FillOutput struct {
	TradeID       string `json:"tradeId"`
	OrderID       string `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
	Side          string `json:"side"`
	Price         string `json:"price"`
	Quantity      string `json:"quantity"`
	QuoteQuantity string `json:"quoteQuantity"`
	Time          string `json:"time"`
	Maker         bool   `json:"maker"`
	BrokerFee     string `json:"brokerFee"`
	MarketFee     string `json:"marketFee"`
	TotalFee      string `json:"totalFee"`
	RealizedPnL   string `json:"realizedPnl"`
}

type stage3EquityPoint struct {
	Time   string `json:"time"`
	Equity string `json:"equity"`
}

type stage3DrawdownPoint struct {
	Time     string `json:"time"`
	Drawdown string `json:"drawdown"`
}

type stage3FeeBreakdown struct {
	RuleID string `json:"ruleId"`
	Label  string `json:"label"`
	Group  string `json:"group"`
	Amount string `json:"amount"`
	Count  int    `json:"count"`
}

type stage3Indicator struct {
	Kind   string    `json:"kind"`
	Period int       `json:"period"`
	Values []*string `json:"values"`
}

func TestRustMigrationStage3CorpusMatchesGolden(t *testing.T) {
	corpus := loadStage3Corpus(t)
	got := runStage3ReferenceCorpus(t, corpus)
	gotJSON := marshalStage3Output(t, got)
	if os.Getenv("JFTRADE_STAGE3_PRINT_REFERENCE") == "1" {
		t.Log(string(gotJSON))
		return
	}
	wantJSON, err := os.ReadFile(stage3FixturePath(t, "backtest-corpus.expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	assertStage3JSONEqual(t, gotJSON, wantJSON)
}

func TestRustMigrationStage3CorpusIsDeterministicAndCancellationRecovers(t *testing.T) {
	corpus := loadStage3Corpus(t)
	first := marshalStage3Output(t, runStage3ReferenceCorpus(t, corpus))
	second := marshalStage3Output(t, runStage3ReferenceCorpus(t, corpus))
	if !bytes.Equal(first, second) {
		t.Fatal("Go reference corpus is not byte-for-byte deterministic")
	}

	recovered := cloneStage3Corpus(t, corpus)
	found := false
	for index := range recovered.Cases {
		if recovered.Cases[index].ID == "cancelled-before-next-bar" {
			recovered.Cases[index].CancelBeforeBar = nil
			found = true
		}
	}
	if !found {
		t.Fatal("cancellation recovery case is missing")
	}
	output := runStage3ReferenceCorpus(t, recovered)
	for _, item := range output.Cases {
		if item.ID == "cancelled-before-next-bar" && (item.Status != "completed" || item.ProcessedBars != 3) {
			t.Fatalf("recovered case = %#v, want completed three-bar run", item)
		}
	}
}

func TestRustMigrationStage3ProcessProbe(t *testing.T) {
	repeatText := os.Getenv("JFTRADE_STAGE3_PROCESS_REPEAT")
	if repeatText == "" {
		t.Skip("process probe is only used by the stage 3 benchmark runner")
	}
	repeat, err := strconv.Atoi(repeatText)
	if err != nil || repeat <= 0 {
		t.Fatalf("invalid JFTRADE_STAGE3_PROCESS_REPEAT %q", repeatText)
	}
	corpus := loadStage3Corpus(t)
	previousLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(previousLogOutput) })
	started := time.Now()
	var output stage3CorpusOutput
	for range repeat {
		output = runStage3ReferenceCorpus(t, corpus)
	}
	payload := struct {
		DurationNanos int64  `json:"durationNanos"`
		ResultHash    string `json:"resultHash"`
	}{
		DurationNanos: time.Since(started).Nanoseconds(),
		ResultHash:    output.Cases[0].ResultHash,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("JFTRADE_STAGE3_PROBE=%s\n", encoded)
}

func BenchmarkRustMigrationStage3ReferenceCorpus(b *testing.B) {
	corpus := loadStage3Corpus(b)
	b.ResetTimer()
	for range b.N {
		_ = runStage3ReferenceCorpus(b, corpus)
	}
}

func loadStage3Corpus(t testing.TB) stage3Corpus {
	t.Helper()
	data, err := os.ReadFile(stage3FixturePath(t, "backtest-corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus stage3Corpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	return corpus
}

func cloneStage3Corpus(t testing.TB, corpus stage3Corpus) stage3Corpus {
	t.Helper()
	data, err := json.Marshal(corpus)
	if err != nil {
		t.Fatal(err)
	}
	var cloned stage3Corpus
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func stage3FixturePath(t testing.TB, name string) string {
	t.Helper()
	if root := os.Getenv("JFTRADE_STAGE3_FIXTURE_ROOT"); root != "" {
		return filepath.Join(root, name)
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve stage 3 fixture path")
	}
	return filepath.Join(filepath.Dir(source), "..", "..", "tests", "fixtures", "rust-migration", "stage3", name)
}

func marshalStage3Output(t testing.TB, output stage3CorpusOutput) []byte {
	t.Helper()
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertStage3JSONEqual(t testing.TB, got, want []byte) {
	t.Helper()
	var gotValue any
	var wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatal(err)
	}
	gotCanonical, _ := json.Marshal(gotValue)
	wantCanonical, _ := json.Marshal(wantValue)
	if !bytes.Equal(gotCanonical, wantCanonical) {
		t.Fatalf("stage 3 corpus mismatch\ngot:  %s\nwant: %s", gotCanonical, wantCanonical)
	}
}

func stage3MetricText(value float64) string {
	text := strconv.FormatFloat(value, 'f', 12, 64)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}
