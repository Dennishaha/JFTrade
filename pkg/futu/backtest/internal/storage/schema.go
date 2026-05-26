package storage

import (
	"fmt"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

// KLineTable is the SQLite table name for Futu historical K-lines.
const KLineTable = "futu_klines"

const selectKLineColumns = "start_time, end_time, interval, symbol, open, high, low, close, volume"

const (
	rehabTypeNoneCode int64 = iota
	rehabTypeForwardCode
	rehabTypeBackwardCode
)

func normalizeRehabTypeName(rehabType string) string {
	switch strings.ToLower(strings.TrimSpace(rehabType)) {
	case "backward":
		return "backward"
	case "none":
		return "none"
	default:
		return "forward"
	}
}

func rehabTypeCode(rehabType string) int64 {
	switch normalizeRehabTypeName(rehabType) {
	case "backward":
		return rehabTypeBackwardCode
	case "none":
		return rehabTypeNoneCode
	default:
		return rehabTypeForwardCode
	}
}

// RehabTypeName converts a qotcommonpb.RehabType enum to the store's string
// representation: "forward", "backward", or "none".
func RehabTypeName(rehabType int32) string {
	switch rehabType {
	case 1:
		return "forward"
	case 2:
		return "backward"
	default:
		return "none"
	}
}

func intervalStorageValue(interval types.Interval) int64 {
	switch interval {
	case types.Interval1s:
		return 1
	case types.Interval1m:
		return 60
	case types.Interval3m:
		return 180
	case types.Interval5m:
		return 300
	case types.Interval15m:
		return 900
	case types.Interval30m:
		return 1800
	case types.Interval1h:
		return 3600
	case types.Interval2h:
		return 7200
	case types.Interval4h:
		return 14400
	case types.Interval6h:
		return 21600
	case types.Interval12h:
		return 43200
	case types.Interval1d:
		return 86400
	case types.Interval3d:
		return 259200
	case types.Interval1w:
		return 604800
	case types.Interval2w:
		return 1209600
	case types.Interval1mo:
		return 2592000
	default:
		if duration := interval.Duration(); duration > 0 {
			return int64(duration / time.Second)
		}
		return 0
	}
}

func intervalFromStorageValue(value int64) (types.Interval, error) {
	switch value {
	case 1:
		return types.Interval1s, nil
	case 60:
		return types.Interval1m, nil
	case 180:
		return types.Interval3m, nil
	case 300:
		return types.Interval5m, nil
	case 900:
		return types.Interval15m, nil
	case 1800:
		return types.Interval30m, nil
	case 3600:
		return types.Interval1h, nil
	case 7200:
		return types.Interval2h, nil
	case 14400:
		return types.Interval4h, nil
	case 21600:
		return types.Interval6h, nil
	case 43200:
		return types.Interval12h, nil
	case 86400:
		return types.Interval1d, nil
	case 259200:
		return types.Interval3d, nil
	case 604800:
		return types.Interval1w, nil
	case 1209600:
		return types.Interval2w, nil
	case 2592000:
		return types.Interval1mo, nil
	default:
		return "", fmt.Errorf("unsupported stored interval value %d", value)
	}
}

func timeToUnixMillis(at time.Time) int64 {
	return at.UTC().UnixMilli()
}

func timeFromUnixMillis(value int64) time.Time {
	return time.UnixMilli(value).UTC()
}

func expectedKLineSchemaColumns() []string {
	return []string{
		"symbol:TEXT:1",
		"interval:INTEGER:2",
		"rehab_type:INTEGER:3",
		"end_time:INTEGER:4",
		"start_time:INTEGER:0",
		"open:REAL:0",
		"high:REAL:0",
		"low:REAL:0",
		"close:REAL:0",
		"volume:REAL:0",
	}
}

const (
	RehabTypeNoneCode     = rehabTypeNoneCode
	RehabTypeForwardCode  = rehabTypeForwardCode
	RehabTypeBackwardCode = rehabTypeBackwardCode
)

func ExpectedKLineSchemaColumns() []string {
	return expectedKLineSchemaColumns()
}

func IntervalStorageValue(interval types.Interval) int64 {
	return intervalStorageValue(interval)
}

func IntervalFromStorageValue(value int64) (types.Interval, error) {
	return intervalFromStorageValue(value)
}
