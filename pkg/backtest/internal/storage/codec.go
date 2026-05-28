package storage

import (
	"database/sql"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func scanKLine(row *sql.Row, symbol string, interval types.Interval) (*types.KLine, error) {
	var startTimeMillis, endTimeMillis int64
	var open, high, low, close, volume string

	if err := row.Scan(&startTimeMillis, &endTimeMillis,
		&open, &high, &low, &close, &volume,
	); err != nil {
		if err == sql.ErrNoRows {
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
	var klines []types.KLine
	for rows.Next() {
		var startTimeMillis, endTimeMillis int64
		var open, high, low, close, volume string

		if err := rows.Scan(&startTimeMillis, &endTimeMillis,
			&open, &high, &low, &close, &volume,
		); err != nil {
			return nil, err
		}

		kline, err := newStoredKLine(startTimeMillis, endTimeMillis, symbol, interval, open, high, low, close, volume)
		if err != nil {
			return nil, err
		}
		klines = append(klines, kline)
	}
	return klines, rows.Err()
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
	return fixedpoint.NewFromString(value)
}
