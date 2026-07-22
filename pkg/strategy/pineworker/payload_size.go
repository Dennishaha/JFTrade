package pineworker

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
)

var runScriptRequestJSONFields = struct {
	JobID            int
	ScriptID         int
	Source           int
	Symbol           int
	Timeframe        int
	Mode             int
	Candles          int
	Params           int
	SessionID        int
	SessionOperation int
	ExpectedRevision int
}{
	JobID:            jsonFieldNameBytes("JobID"),
	ScriptID:         jsonFieldNameBytes("ScriptID"),
	Source:           jsonFieldNameBytes("Source"),
	Symbol:           jsonFieldNameBytes("Symbol"),
	Timeframe:        jsonFieldNameBytes("Timeframe"),
	Mode:             jsonFieldNameBytes("Mode"),
	Candles:          jsonFieldNameBytes("Candles"),
	Params:           jsonFieldNameBytes("Params"),
	SessionID:        jsonFieldNameBytes("SessionID"),
	SessionOperation: jsonFieldNameBytes("SessionOperation"),
	ExpectedRevision: jsonFieldNameBytes("ExpectedRevision"),
}

var candleJSONFields = struct {
	OpenTime  int
	CloseTime int
	Open      int
	High      int
	Low       int
	Close     int
	Volume    int
}{
	OpenTime:  jsonFieldNameBytes("openTime"),
	CloseTime: jsonFieldNameBytes("closeTime"),
	Open:      jsonFieldNameBytes("open"),
	High:      jsonFieldNameBytes("high"),
	Low:       jsonFieldNameBytes("low"),
	Close:     jsonFieldNameBytes("close"),
	Volume:    jsonFieldNameBytes("volume"),
}

func jsonSize(value any) (int, error) {
	if request, ok := value.(RunScriptRequest); ok {
		return estimateRunScriptRequestJSONSize(request)
	}
	counter := jsonByteCounter(0)
	encoder := json.NewEncoder(&counter)
	if err := encoder.Encode(value); err != nil {
		return 0, fmt.Errorf("encode pine worker payload: %w", err)
	}
	if counter > 0 {
		counter--
	}
	return int(counter), nil
}

type jsonByteCounter int

func (counter *jsonByteCounter) Write(data []byte) (int, error) {
	*counter += jsonByteCounter(len(data))
	return len(data), nil
}

func estimateRunScriptRequestJSONSize(request RunScriptRequest) (int, error) {
	candlesBytes, err := estimateCandlesJSONSize(request.Candles)
	if err != nil {
		return 0, err
	}
	paramsBytes := estimateStringMapJSONSize(request.Params)
	return estimateKnownJSONObjectSize([]knownJSONField{
		{nameBytes: runScriptRequestJSONFields.JobID, valueBytes: jsonStringValueBytes(request.JobID)},
		{nameBytes: runScriptRequestJSONFields.ScriptID, valueBytes: jsonStringValueBytes(request.ScriptID)},
		{nameBytes: runScriptRequestJSONFields.Source, valueBytes: jsonStringValueBytes(request.Source)},
		{nameBytes: runScriptRequestJSONFields.Symbol, valueBytes: jsonStringValueBytes(request.Symbol)},
		{nameBytes: runScriptRequestJSONFields.Timeframe, valueBytes: jsonStringValueBytes(request.Timeframe)},
		{nameBytes: runScriptRequestJSONFields.Mode, valueBytes: jsonStringValueBytes(request.Mode)},
		{nameBytes: runScriptRequestJSONFields.Candles, valueBytes: candlesBytes},
		{nameBytes: runScriptRequestJSONFields.Params, valueBytes: paramsBytes},
		{nameBytes: runScriptRequestJSONFields.SessionID, valueBytes: jsonStringValueBytes(request.SessionID)},
		{nameBytes: runScriptRequestJSONFields.SessionOperation, valueBytes: jsonStringValueBytes(request.SessionOperation)},
		{nameBytes: runScriptRequestJSONFields.ExpectedRevision, valueBytes: jsonUintValueBytes(request.ExpectedRevision)},
	}), nil
}

func validateAndMeasureRunScriptRequest(request RunScriptRequest, config WorkerConfig) (int, error) {
	if err := validateRunScriptRequestBasics(request, config); err != nil {
		return 0, err
	}
	candlesBytes := 2
	if request.Candles == nil {
		candlesBytes = 4
	} else if len(request.Candles) > 0 {
		candlesBytes += len(request.Candles) - 1
	}
	for index, candle := range request.Candles {
		if err := validateCandle(candle); err != nil {
			return 0, fmt.Errorf("candle %d: %w", index, err)
		}
		candleBytes, err := estimateCandleJSONSize(candle)
		if err != nil {
			return 0, err
		}
		candlesBytes += candleBytes
	}
	paramsBytes := estimateStringMapJSONSize(request.Params)
	return estimateKnownJSONObjectSize([]knownJSONField{
		{nameBytes: runScriptRequestJSONFields.JobID, valueBytes: jsonStringValueBytes(request.JobID)},
		{nameBytes: runScriptRequestJSONFields.ScriptID, valueBytes: jsonStringValueBytes(request.ScriptID)},
		{nameBytes: runScriptRequestJSONFields.Source, valueBytes: jsonStringValueBytes(request.Source)},
		{nameBytes: runScriptRequestJSONFields.Symbol, valueBytes: jsonStringValueBytes(request.Symbol)},
		{nameBytes: runScriptRequestJSONFields.Timeframe, valueBytes: jsonStringValueBytes(request.Timeframe)},
		{nameBytes: runScriptRequestJSONFields.Mode, valueBytes: jsonStringValueBytes(request.Mode)},
		{nameBytes: runScriptRequestJSONFields.Candles, valueBytes: candlesBytes},
		{nameBytes: runScriptRequestJSONFields.Params, valueBytes: paramsBytes},
		{nameBytes: runScriptRequestJSONFields.SessionID, valueBytes: jsonStringValueBytes(request.SessionID)},
		{nameBytes: runScriptRequestJSONFields.SessionOperation, valueBytes: jsonStringValueBytes(request.SessionOperation)},
		{nameBytes: runScriptRequestJSONFields.ExpectedRevision, valueBytes: jsonUintValueBytes(request.ExpectedRevision)},
	}), nil
}

type knownJSONField struct {
	nameBytes  int
	valueBytes int
}

func estimateKnownJSONObjectSize(fields []knownJSONField) int {
	if len(fields) == 0 {
		return 2
	}
	size := 2 + len(fields) - 1
	for _, field := range fields {
		size += field.nameBytes + field.valueBytes
	}
	return size
}

func estimateCandlesJSONSize(candles []Candle) (int, error) {
	if candles == nil {
		return 4, nil
	}
	if len(candles) == 0 {
		return 2, nil
	}
	size := 2 + len(candles) - 1
	for _, candle := range candles {
		candleBytes, err := estimateCandleJSONSize(candle)
		if err != nil {
			return 0, err
		}
		size += candleBytes
	}
	return size, nil
}

func estimateCandleJSONSize(candle Candle) (int, error) {
	openBytes, err := jsonNumberValueBytes(candle.Open)
	if err != nil {
		return 0, err
	}
	highBytes, err := jsonNumberValueBytes(candle.High)
	if err != nil {
		return 0, err
	}
	lowBytes, err := jsonNumberValueBytes(candle.Low)
	if err != nil {
		return 0, err
	}
	closeBytes, err := jsonNumberValueBytes(candle.Close)
	if err != nil {
		return 0, err
	}
	volumeBytes, err := jsonNumberValueBytes(candle.Volume)
	if err != nil {
		return 0, err
	}
	return estimateKnownJSONObjectSize([]knownJSONField{
		{nameBytes: candleJSONFields.OpenTime, valueBytes: jsonIntValueBytes(candle.OpenTime)},
		{nameBytes: candleJSONFields.CloseTime, valueBytes: jsonIntValueBytes(candle.CloseTime)},
		{nameBytes: candleJSONFields.Open, valueBytes: openBytes},
		{nameBytes: candleJSONFields.High, valueBytes: highBytes},
		{nameBytes: candleJSONFields.Low, valueBytes: lowBytes},
		{nameBytes: candleJSONFields.Close, valueBytes: closeBytes},
		{nameBytes: candleJSONFields.Volume, valueBytes: volumeBytes},
	}), nil
}

func estimateStringMapJSONSize(values map[string]string) int {
	if values == nil {
		return 4
	}
	if len(values) == 0 {
		return 2
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	size := 2 + len(keys) - 1
	for _, key := range keys {
		size += jsonStringValueBytes(key) + 1 + jsonStringValueBytes(values[key])
	}
	return size
}

func jsonFieldNameBytes(name string) int {
	return jsonStringValueBytes(name) + 1
}

func jsonStringValueBytes(value string) int {
	data, err := json.Marshal(value)
	if err != nil {
		return 2
	}
	return len(data)
}

func jsonIntValueBytes(value int64) int {
	var buffer [24]byte
	return len(strconv.AppendInt(buffer[:0], value, 10))
}

func jsonUintValueBytes(value uint64) int {
	var buffer [24]byte
	return len(strconv.AppendUint(buffer[:0], value, 10))
}

func jsonNumberValueBytes(value float64) (int, error) {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, fmt.Errorf("encode pine worker payload: json: unsupported value: %v", value)
	}
	var buffer [32]byte
	return len(strconv.AppendFloat(buffer[:0], value, 'g', -1, 64)), nil
}
