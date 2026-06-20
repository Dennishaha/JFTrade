package servercore

import (
	"time"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
)

// Type aliases to httpserver — these keep all existing jftradeapi code
// compatible while the backing types live in the HTTP infrastructure layer.
type optionalIntValue = httpserver.OptionalIntValue
type optionalBoolValue = httpserver.OptionalBoolValue
type optionalTimeValue = httpserver.OptionalTimeValue
type candlePeriodValue = httpserver.CandlePeriodValue

// Thin constructors — kept for internal jftradeapi convenience.
func newOptionalIntValue(value int) optionalIntValue {
	return optionalIntValue{Value: value, Set: true, Valid: true}
}

func newOptionalBoolValue(value bool) optionalBoolValue {
	return optionalBoolValue{Value: value, Set: true}
}

func newOptionalTimeValue(value time.Time) optionalTimeValue {
	return optionalTimeValue{Time: value}
}

type marketSnapshotQuery struct {
	Refresh optionalBoolValue `form:"refresh,parser=encoding.TextUnmarshaler"`
}

type marketCandlesQuery struct {
	Period   candlePeriodValue `form:"period,parser=encoding.TextUnmarshaler"`
	Limit    optionalIntValue  `form:"limit,parser=encoding.TextUnmarshaler"`
	FromTime optionalTimeValue `form:"fromTime,parser=encoding.TextUnmarshaler"`
	ToTime   optionalTimeValue `form:"toTime,parser=encoding.TextUnmarshaler"`
	From     optionalTimeValue `form:"from,parser=encoding.TextUnmarshaler"`
	To       optionalTimeValue `form:"to,parser=encoding.TextUnmarshaler"`
}

type marketDepthQuery struct {
	Num optionalIntValue `form:"num,parser=encoding.TextUnmarshaler"`
}

func (q marketSnapshotQuery) forceRefresh() bool {
	return q.Refresh.Bool()
}

func (q marketCandlesQuery) normalizedPeriod() string {
	if q.Period == "" {
		return "1m"
	}
	return q.Period.String()
}

func (q marketCandlesQuery) limitOrDefault(defaultLimit int, maxLimit int) int {
	limit := defaultLimit
	if q.Limit.Set && q.Limit.Valid {
		limit = q.Limit.Int()
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return limit
}

func (q marketDepthQuery) numOrDefault(defaultNum int32, maxNum int32) int32 {
	num := defaultNum
	if q.Num.Set && q.Num.Valid {
		num = int32(q.Num.Int())
	}
	if num < 1 {
		num = 1
	}
	if num > maxNum {
		num = maxNum
	}
	return num
}
