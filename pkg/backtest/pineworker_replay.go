package backtest

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type PineWorkerReplayPlanRequest struct {
	JobID     string
	ScriptID  string
	Source    string
	Symbol    string
	Timeframe string
	Params    map[string]string
	KLines    []types.KLine
}

type PineWorkerReplayPlan struct {
	Request     pineworker.RunScriptRequest
	Commands    []WorkerOrderCommand
	ByBarIndex  map[int][]WorkerOrderCommand
	ByOpenTime  map[int64][]WorkerOrderCommand
	Metadata    pineworker.WorkerMetadata
	CandleCount int
}

type PineWorkerReplayPlanner struct {
	Adapter PineWorkerBacktestAdapter
}

type pineWorkerCompactReplayPlan struct {
	Commands []WorkerOrderCommand
	Metadata pineworker.WorkerMetadata
}

func (planner PineWorkerReplayPlanner) Plan(ctx context.Context, request PineWorkerReplayPlanRequest) (PineWorkerReplayPlan, error) {
	workerRequest, err := BuildPineWorkerBacktestRequest(request)
	if err != nil {
		return PineWorkerReplayPlan{}, err
	}
	commands, metadata, err := planner.Adapter.Run(ctx, workerRequest)
	if err != nil {
		return PineWorkerReplayPlan{}, err
	}
	plan, err := NewPineWorkerReplayPlan(workerRequest, commands, metadata)
	if err != nil {
		return PineWorkerReplayPlan{}, err
	}
	return plan, nil
}

func BuildPineWorkerBacktestRequest(request PineWorkerReplayPlanRequest) (pineworker.RunScriptRequest, error) {
	return buildPineWorkerBacktestRequest(request, CandlesFromKLines(request.KLines))
}

func buildPineWorkerBacktestRequest(request PineWorkerReplayPlanRequest, candles []pineworker.Candle) (pineworker.RunScriptRequest, error) {
	if strings.TrimSpace(request.Source) == "" {
		return pineworker.RunScriptRequest{}, fmt.Errorf("pine worker replay source is required")
	}
	if strings.TrimSpace(request.Symbol) == "" {
		return pineworker.RunScriptRequest{}, fmt.Errorf("pine worker replay symbol is required")
	}
	if strings.TrimSpace(request.Timeframe) == "" {
		return pineworker.RunScriptRequest{}, fmt.Errorf("pine worker replay timeframe is required")
	}
	if len(candles) == 0 {
		return pineworker.RunScriptRequest{}, fmt.Errorf("pine worker replay candles are required")
	}
	jobID := strings.TrimSpace(request.JobID)
	if jobID == "" {
		jobID = defaultPineWorkerReplayJobID(request.Symbol, request.Timeframe)
	}
	return pineworker.RunScriptRequest{
		JobID:     jobID,
		ScriptID:  request.ScriptID,
		Source:    request.Source,
		Symbol:    request.Symbol,
		Timeframe: request.Timeframe,
		Mode:      pineworker.ModeBacktest,
		Candles:   candles,
		Params:    copyReplayParams(request.Params),
	}, nil
}

func CandlesFromKLines(klines []types.KLine) []pineworker.Candle {
	candles := make([]pineworker.Candle, 0, len(klines))
	for _, kline := range klines {
		candles = append(candles, CandleFromKLine(kline))
	}
	return candles
}

func candlesFromReplayKLineBatch(batch *pineWorkerReplayKLineBatch) []pineworker.Candle {
	candles := make([]pineworker.Candle, batch.Len())
	index := 0
	batch.forEach(func(kline types.KLine) bool {
		candles[index] = CandleFromKLine(kline)
		index++
		return true
	})
	return candles
}

func planPineWorkerCompactReplay(
	ctx context.Context,
	adapter PineWorkerBacktestAdapter,
	request PineWorkerReplayPlanRequest,
	batch *pineWorkerReplayKLineBatch,
) (pineWorkerCompactReplayPlan, error) {
	workerRequest, err := buildPineWorkerBacktestRequest(request, candlesFromReplayKLineBatch(batch))
	if err != nil {
		return pineWorkerCompactReplayPlan{}, err
	}
	commands, metadata, err := adapter.Run(ctx, workerRequest)
	if err != nil {
		return pineWorkerCompactReplayPlan{}, err
	}
	normalizedCommands, err := normalizeReplayCommands(workerRequest.Candles, commands)
	if err != nil {
		return pineWorkerCompactReplayPlan{}, err
	}
	return pineWorkerCompactReplayPlan{
		Commands: normalizedCommands,
		Metadata: metadata,
	}, nil
}

func CandleFromKLine(kline types.KLine) pineworker.Candle {
	return pineworker.Candle{
		OpenTime:  kline.StartTime.Time().UnixMilli(),
		CloseTime: kline.EndTime.Time().UnixMilli(),
		Open:      kline.Open.Float64(),
		High:      kline.High.Float64(),
		Low:       kline.Low.Float64(),
		Close:     kline.Close.Float64(),
		Volume:    kline.Volume.Float64(),
	}
}

func NewPineWorkerReplayPlan(
	request pineworker.RunScriptRequest,
	commands []WorkerOrderCommand,
	metadata pineworker.WorkerMetadata,
) (PineWorkerReplayPlan, error) {
	normalizedCommands, err := normalizeReplayCommands(request.Candles, commands)
	if err != nil {
		return PineWorkerReplayPlan{}, err
	}
	byBarIndex := make(map[int][]WorkerOrderCommand)
	byOpenTime := make(map[int64][]WorkerOrderCommand)
	for _, command := range normalizedCommands {
		byBarIndex[command.BarIndex] = append(byBarIndex[command.BarIndex], command)
		byOpenTime[command.Time] = append(byOpenTime[command.Time], command)
	}
	return PineWorkerReplayPlan{
		Request:     request,
		Commands:    normalizedCommands,
		ByBarIndex:  byBarIndex,
		ByOpenTime:  byOpenTime,
		Metadata:    metadata,
		CandleCount: len(request.Candles),
	}, nil
}

func normalizeReplayCommands(candles []pineworker.Candle, commands []WorkerOrderCommand) ([]WorkerOrderCommand, error) {
	result := make([]WorkerOrderCommand, 0, len(commands))
	for index, command := range commands {
		if command.BarIndex < 0 || command.BarIndex >= len(candles) {
			return nil, fmt.Errorf("pine worker command %d has bar index %d outside candle range 0..%d", index, command.BarIndex, len(candles)-1)
		}
		if command.Time == 0 {
			command.Time = candles[command.BarIndex].OpenTime
		}
		result = append(result, command)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].BarIndex != result[j].BarIndex {
			return result[i].BarIndex < result[j].BarIndex
		}
		return result[i].Kind < result[j].Kind
	})
	return result, nil
}

func defaultPineWorkerReplayJobID(symbol string, timeframe string) string {
	return "backtest:" + strings.TrimSpace(symbol) + ":" + strings.TrimSpace(timeframe)
}

func copyReplayParams(params map[string]string) map[string]string {
	if len(params) == 0 {
		return nil
	}
	copied := make(map[string]string, len(params))
	maps.Copy(copied, params)
	return copied
}
