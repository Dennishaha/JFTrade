package storage

import (
	"database/sql"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func scanKLine(row *sql.Row) (*types.KLine, error) {
	var startTimeMillis, endTimeMillis, intervalValue int64
	var symbol string
	var open, high, low, close, volume float64

	if err := row.Scan(&startTimeMillis, &endTimeMillis, &intervalValue, &symbol,
		&open, &high, &low, &close, &volume,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	kline, err := newStoredKLine(startTimeMillis, endTimeMillis, intervalValue, symbol, open, high, low, close, volume)
	if err != nil {
		return nil, err
	}
	return &kline, nil
}

func scanKLines(rows *sql.Rows) ([]types.KLine, error) {
	var klines []types.KLine
	for rows.Next() {
		var startTimeMillis, endTimeMillis, intervalValue int64
		var symbol string
		var open, high, low, close, volume float64

		if err := rows.Scan(&startTimeMillis, &endTimeMillis, &intervalValue, &symbol,
			&open, &high, &low, &close, &volume,
		); err != nil {
			return nil, err
		}

		kline, err := newStoredKLine(startTimeMillis, endTimeMillis, intervalValue, symbol, open, high, low, close, volume)
		if err != nil {
			return nil, err
		}

		klines = append(klines, kline)
	}
	return klines, rows.Err()
}

func newStoredKLine(startTimeMillis, endTimeMillis, intervalValue int64, symbol string, open, high, low, close, volume float64) (types.KLine, error) {
	interval, err := intervalFromStorageValue(intervalValue)
	if err != nil {
		return types.KLine{}, err
	}

	return types.KLine{
		StartTime:      types.Time(timeFromUnixMillis(startTimeMillis)),
		EndTime:        types.Time(timeFromUnixMillis(endTimeMillis)),
		Interval:       interval,
		Symbol:         symbol,
		Open:           floatToFixed(open),
		High:           floatToFixed(high),
		Low:            floatToFixed(low),
		Close:          floatToFixed(close),
		Volume:         floatToFixed(volume),
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

func floatToFixed(f float64) fixedpoint.Value {
	return fixedpoint.NewFromFloat(f)
}
