package pineworker

import (
	"encoding/binary"
	"maps"
	"math"
	"time"

	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker/pineworkerpb"
)

const (
	candleBatchEncodingVersion = uint32(1)
	candleBatchRecordBytes     = 56
)

func requestToProto(request RunScriptRequest) *pineworkerpb.RunScriptRequest {
	return &pineworkerpb.RunScriptRequest{
		JobId:        request.JobID,
		ScriptId:     request.ScriptID,
		Source:       request.Source,
		Symbol:       request.Symbol,
		Timeframe:    request.Timeframe,
		Mode:         request.Mode,
		Candles:      candlesToProto(request.Candles),
		Params:       copyStringMap(request.Params),
		IncludePlots: includePlotsForRequest(request),
	}
}

func responseFromProto(response *pineworkerpb.RunScriptResponse) RunScriptResponse {
	if response == nil {
		return RunScriptResponse{}
	}
	plots := plotsFromProto(response.GetPlots())
	return RunScriptResponse{
		JobID:           response.GetJobId(),
		Outputs:         seriesOutputsFromPlots(plots),
		Plots:           plots,
		OrderIntents:    orderIntentsFromProto(response.GetOrderIntents()),
		Alerts:          alertsFromProto(response.GetAlerts()),
		VisualOutputs:   visualOutputsFromProto(response.GetVisualOutputs()),
		Logs:            append([]string(nil), response.GetLogs()...),
		Warnings:        append([]string(nil), response.GetWarnings()...),
		Diagnostics:     diagnosticsFromProto(response.GetDiagnostics()),
		Metadata:        metadataFromProto(response.GetMetadata()),
		Error:           response.GetError(),
		StrategyMetrics: strategyMetricsFromProto(response.GetStrategyMetrics()),
	}
}

func healthFromProto(response *pineworkerpb.HealthCheckResponse) HealthStatus {
	if response == nil {
		return HealthStatus{}
	}
	return HealthStatus{
		OK:            response.GetOk(),
		WorkerID:      response.GetWorkerId(),
		Version:       response.GetVersion(),
		PineTSVersion: response.GetPinetsVersion(),
		Capabilities:  append([]string(nil), response.GetCapabilities()...),
	}
}

func includePlotsForRequest(request RunScriptRequest) bool {
	return normalizeMode(request.Mode) != ModeBacktest
}

func candlesToProto(candles []Candle) *pineworkerpb.CandleBatch {
	payload := make([]byte, len(candles)*candleBatchRecordBytes)
	for index, candle := range candles {
		offset := index * candleBatchRecordBytes
		binary.LittleEndian.PutUint64(payload[offset:], uint64(candle.OpenTime))
		binary.LittleEndian.PutUint64(payload[offset+8:], uint64(candle.CloseTime))
		binary.LittleEndian.PutUint64(payload[offset+16:], math.Float64bits(candle.Open))
		binary.LittleEndian.PutUint64(payload[offset+24:], math.Float64bits(candle.High))
		binary.LittleEndian.PutUint64(payload[offset+32:], math.Float64bits(candle.Low))
		binary.LittleEndian.PutUint64(payload[offset+40:], math.Float64bits(candle.Close))
		binary.LittleEndian.PutUint64(payload[offset+48:], math.Float64bits(candle.Volume))
	}
	return &pineworkerpb.CandleBatch{
		EncodingVersion: candleBatchEncodingVersion,
		Payload:         payload,
	}
}

func seriesOutputsFromPlots(plots []PlotOutput) []SeriesOutput {
	result := make([]SeriesOutput, 0, len(plots))
	for _, plot := range plots {
		result = append(result, SeriesOutput{
			Name:   plot.Name,
			Kind:   "plot",
			Values: plot.Values,
		})
	}
	return result
}

func plotsFromProto(plots []*pineworkerpb.PlotOutput) []PlotOutput {
	result := make([]PlotOutput, 0, len(plots))
	for _, plot := range plots {
		result = append(result, PlotOutput{
			Name:   plot.GetName(),
			Values: append([]float64(nil), plot.GetValues()...),
		})
	}
	return result
}

func alertsFromProto(alerts []*pineworkerpb.AlertEvent) []AlertEvent {
	result := make([]AlertEvent, 0, len(alerts))
	for _, alert := range alerts {
		result = append(result, AlertEvent{
			Type:      alert.GetType(),
			ID:        alert.GetId(),
			Message:   alert.GetMessage(),
			Title:     alert.GetTitle(),
			Frequency: alert.GetFrequency(),
			BarIndex:  int(alert.GetBarIndex()),
			Time:      alert.GetTime(),
		})
	}
	return result
}

func visualOutputsFromProto(outputs []*pineworkerpb.VisualOutput) []VisualOutput {
	result := make([]VisualOutput, 0, len(outputs))
	for _, output := range outputs {
		result = append(result, VisualOutput{
			Kind:        output.GetKind(),
			Name:        output.GetName(),
			PayloadJSON: output.GetPayloadJson(),
		})
	}
	return result
}

func strategyMetricsFromProto(metrics *pineworkerpb.StrategyMetrics) *StrategyMetrics {
	if metrics == nil {
		return nil
	}
	return &StrategyMetrics{
		BuyAndHoldPnL:             metrics.GetBuyAndHoldPnl(),
		BuyAndHoldPerGain:         metrics.GetBuyAndHoldPerGain(),
		StrategyOutperformance:    metrics.GetStrategyOutperformance(),
		HasBuyAndHoldPnL:          metrics.GetHasBuyAndHoldPnl(),
		HasBuyAndHoldPerGain:      metrics.GetHasBuyAndHoldPerGain(),
		HasStrategyOutperformance: metrics.GetHasStrategyOutperformance(),
	}
}

func diagnosticsFromProto(diagnostics []*pineworkerpb.Diagnostic) []Diagnostic {
	result := make([]Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result = append(result, Diagnostic{
			Severity: diagnostic.GetSeverity(),
			Code:     diagnostic.GetCode(),
			Message:  diagnostic.GetMessage(),
			Line:     int(diagnostic.GetLine()),
			Column:   int(diagnostic.GetColumn()),
		})
	}
	return result
}

func orderIntentsFromProto(intents []*pineworkerpb.OrderIntent) []OrderIntent {
	result := make([]OrderIntent, 0, len(intents))
	for _, intent := range intents {
		result = append(result, OrderIntent{
			Kind:           intent.GetKind(),
			ID:             intent.GetId(),
			FromEntry:      intent.GetFromEntry(),
			Direction:      intent.GetDirection(),
			Quantity:       intent.GetQuantity(),
			QuantityPct:    intent.GetQuantityPct(),
			LimitPrice:     intent.GetLimitPrice(),
			StopPrice:      intent.GetStopPrice(),
			Comment:        intent.GetComment(),
			AlertMessage:   intent.GetAlertMessage(),
			DisableAlert:   intent.GetDisableAlert(),
			BarIndex:       int(intent.GetBarIndex()),
			Time:           intent.GetTime(),
			HasQuantity:    intent.GetHasQuantity(),
			HasQuantityPct: intent.GetHasQuantityPct(),
			HasLimitPrice:  intent.GetHasLimitPrice(),
			HasStopPrice:   intent.GetHasStopPrice(),
		})
	}
	return result
}

func metadataFromProto(metadata *pineworkerpb.WorkerMetadata) WorkerMetadata {
	if metadata == nil {
		return WorkerMetadata{}
	}
	return WorkerMetadata{
		WorkerID:      metadata.GetWorkerId(),
		Version:       metadata.GetVersion(),
		PineTSVersion: metadata.GetPinetsVersion(),
		ScriptHash:    metadata.GetScriptHash(),
		DataHash:      metadata.GetDataHash(),
		Duration:      time.Duration(metadata.GetDurationMs()) * time.Millisecond,
		RequestBytes:  int(metadata.GetRequestBytes()),
		ResponseBytes: int(metadata.GetResponseBytes()),
		PeakRSSBytes:  metadata.GetPeakRssBytes(),
	}
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	maps.Copy(result, values)
	return result
}
