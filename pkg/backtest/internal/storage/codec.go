package storage

import (
	"database/sql"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func scanKLine(row *sql.Row, symbol string, interval types.Interval) (*types.KLine, error) {
	var startTimeMillis, endTimeMillis int64
	var open, high, low, close, volume float64

	if err := row.Scan(&startTimeMillis, &endTimeMillis,
		&open, &high, &low, &close, &volume,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	kline := newStoredKLine(startTimeMillis, endTimeMillis, symbol, interval, open, high, low, close, volume)
	return &kline, nil
}

func scanKLines(rows *sql.Rows, symbol string, interval types.Interval) ([]types.KLine, error) {
	var klines []types.KLine
	for rows.Next() {
		var startTimeMillis, endTimeMillis int64
		var open, high, low, close, volume float64

		if err := rows.Scan(&startTimeMillis, &endTimeMillis,
			&open, &high, &low, &close, &volume,
		); err != nil {
			return nil, err
		}

		kline := newStoredKLine(startTimeMillis, endTimeMillis, symbol, interval, open, high, low, close, volume)
		klines = append(klines, kline)
	}
	return klines, rows.Err()
}

func newStoredKLine(startTimeMillis, endTimeMillis int64, symbol string, interval types.Interval, open, high, low, close, volume float64) types.KLine {
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
	}
}

func reverseKLines(klines []types.KLine) {
	for i, j := 0, len(klines)-1; i < j; i, j = i+1, j-1 {
		klines[i], klines[j] = klines[j], klines[i]
	}
}

func floatToFixed(f float64) fixedpoint.Value {
	return fixedpoint.NewFromFloat(f)
}
