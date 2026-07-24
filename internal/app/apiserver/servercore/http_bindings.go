package servercore

import (
	"time"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
)

func newOptionalIntValue(value int) httpserver.OptionalIntValue {
	return httpserver.OptionalIntValue{Value: value, Set: true, Valid: true}
}

func newOptionalBoolValue(value bool) httpserver.OptionalBoolValue {
	return httpserver.OptionalBoolValue{Value: value, Set: true}
}

func newOptionalTimeValue(value time.Time) httpserver.OptionalTimeValue {
	return httpserver.OptionalTimeValue{Time: value}
}

type marketSnapshotQuery struct {
	Refresh httpserver.OptionalBoolValue `form:"refresh,parser=encoding.TextUnmarshaler"`
}

type marketCandlesQuery struct {
	Period   httpserver.CandlePeriodValue `form:"period,parser=encoding.TextUnmarshaler"`
	Limit    httpserver.OptionalIntValue  `form:"limit,parser=encoding.TextUnmarshaler"`
	FromTime httpserver.OptionalTimeValue `form:"fromTime,parser=encoding.TextUnmarshaler"`
	ToTime   httpserver.OptionalTimeValue `form:"toTime,parser=encoding.TextUnmarshaler"`
	From     httpserver.OptionalTimeValue `form:"from,parser=encoding.TextUnmarshaler"`
	To       httpserver.OptionalTimeValue `form:"to,parser=encoding.TextUnmarshaler"`
}

type marketDepthQuery struct {
	Num httpserver.OptionalIntValue `form:"num,parser=encoding.TextUnmarshaler"`
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
