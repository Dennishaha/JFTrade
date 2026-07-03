package storage

import (
	"database/sql"
	"errors"
	"unsafe"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

const (
	storedFixedScale  = int64(fixedpoint.DefaultPow)
	storedFixedMaxInt = int64(^uint64(0) >> 1)
)

func scanKLine(row *sql.Row, symbol string, interval types.Interval) (*types.KLine, error) {
	var startTimeMillis, endTimeMillis int64
	var open, high, low, close, volume string

	if err := row.Scan(&startTimeMillis, &endTimeMillis,
		&open, &high, &low, &close, &volume,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	kline, err := newStoredKLine(startTimeMillis, endTimeMillis, symbol, interval, open, high, low, close, volume)
	if err != nil {
		return nil, err
	}
	return &kline, nil
}

func scanKLines(rows *sql.Rows, symbol string, interval types.Interval) ([]types.KLine, error) {
	return scanKLinesWithCapacity(rows, symbol, interval, 0)
}

func scanKLinesWithCapacity(rows *sql.Rows, symbol string, interval types.Interval, capacity int) ([]types.KLine, error) {
	klines := make([]types.KLine, 0, capacity)
	if err := streamKLines(rows, symbol, interval, func(kline types.KLine) {
		klines = append(klines, kline)
	}); err != nil {
		return nil, err
	}
	return klines, nil
}

func streamKLines(rows *sql.Rows, symbol string, interval types.Interval, emit func(types.KLine)) error {
	for rows.Next() {
		kline, err := scanStoredKLineRow(rows, symbol, interval)
		if err != nil {
			return err
		}
		emit(kline)
	}
	return rows.Err()
}

func scanStoredKLineRow(rows *sql.Rows, symbol string, interval types.Interval) (types.KLine, error) {
	var startTimeMillis, endTimeMillis int64
	var open, high, low, close, volume sql.RawBytes

	if err := rows.Scan(&startTimeMillis, &endTimeMillis,
		&open, &high, &low, &close, &volume,
	); err != nil {
		return types.KLine{}, err
	}

	return newStoredKLine(
		startTimeMillis,
		endTimeMillis,
		symbol,
		interval,
		rawBytesToString(open),
		rawBytesToString(high),
		rawBytesToString(low),
		rawBytesToString(close),
		rawBytesToString(volume),
	)
}

func newStoredKLine(startTimeMillis, endTimeMillis int64, symbol string, interval types.Interval, open, high, low, close, volume string) (types.KLine, error) {
	openValue, err := parseStoredFixed(open)
	if err != nil {
		return types.KLine{}, err
	}
	highValue, err := parseStoredFixed(high)
	if err != nil {
		return types.KLine{}, err
	}
	lowValue, err := parseStoredFixed(low)
	if err != nil {
		return types.KLine{}, err
	}
	closeValue, err := parseStoredFixed(close)
	if err != nil {
		return types.KLine{}, err
	}
	volumeValue, err := parseStoredFixed(volume)
	if err != nil {
		return types.KLine{}, err
	}

	return types.KLine{
		StartTime:      types.Time(timeFromUnixMillis(startTimeMillis)),
		EndTime:        types.Time(timeFromUnixMillis(endTimeMillis)),
		Interval:       interval,
		Symbol:         symbol,
		Open:           openValue,
		High:           highValue,
		Low:            lowValue,
		Close:          closeValue,
		Volume:         volumeValue,
		Closed:         true,
		LastTradeID:    0,
		NumberOfTrades: 0,
	}, nil
}

func reverseKLines(klines []types.KLine) {
	for i, j := 0, len(klines)-1; i < j; i, j = i+1, j-1 {
		klines[i], klines[j] = klines[j], klines[i]
	}
}

func parseStoredFixed(value string) (fixedpoint.Value, error) {
	if parsed, ok := parseStoredFixedDecimal(value); ok {
		return parsed, nil
	}
	return fixedpoint.NewFromString(value)
}

func parseStoredFixedDecimal(value string) (fixedpoint.Value, bool) {
	if len(value) == 0 {
		return 0, true
	}

	index := 0
	sign := int64(1)
	switch value[0] {
	case '-':
		sign = -1
		index = 1
	case '+':
		index = 1
	}
	if index >= len(value) {
		return 0, false
	}

	var whole int64
	var fraction int64
	fractionDigits := 0
	seenDot := false

	for ; index < len(value); index++ {
		char := value[index]
		switch {
		case char >= '0' && char <= '9':
			digit := int64(char - '0')
			if !seenDot {
				if whole > (storedFixedMaxInt-digit)/10 {
					return 0, false
				}
				whole = whole*10 + digit
				continue
			}
			if fractionDigits >= fixedpoint.DefaultPrecision {
				return 0, false
			}
			fraction = fraction*10 + digit
			fractionDigits++
		case char == '.':
			if seenDot {
				return 0, false
			}
			seenDot = true
		default:
			return 0, false
		}
	}

	for ; fractionDigits < fixedpoint.DefaultPrecision; fractionDigits++ {
		fraction *= 10
	}
	if whole > (storedFixedMaxInt-fraction)/storedFixedScale {
		return 0, false
	}

	total := whole*storedFixedScale + fraction
	if sign < 0 {
		total = -total
	}
	return fixedpoint.Value(total), true
}

func rawBytesToString(value sql.RawBytes) string {
	if len(value) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(value), len(value))
}
