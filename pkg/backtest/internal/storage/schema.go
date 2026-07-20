package storage

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

// KLineTable is the SQLite table-name prefix for Futu historical K-lines.
const KLineTable = "local_klines"

const selectKLineColumns = "start_time, end_time, open, high, low, close, volume"

const (
	rehabTypeNoneCode int64 = iota
	rehabTypeForwardCode
	rehabTypeBackwardCode
)

const (
	klineSessionScopeLegacy   = "legacy"
	klineSessionScopeRegular  = "regular"
	klineSessionScopeExtended = "extended"
	klineReadSessionScopeAuto = "auto"
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
	case types.Interval10m:
		return 600
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
		if duration, ok := safeIntervalDuration(interval); ok {
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
	case 600:
		return types.Interval10m, nil
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

func klineTableName(symbol string, interval types.Interval, rehabType string) string {
	return klineTableNameForSessionScope(symbol, interval, rehabType, klineSessionScopeLegacy)
}

func klineTableNameForSessionScope(symbol string, interval types.Interval, rehabType string, sessionScope string) string {
	normalizedSymbol := strings.ToLower(strings.TrimSpace(symbol))
	normalizedInterval := strings.ToLower(strings.TrimSpace(string(interval)))
	normalizedRehabType := normalizeRehabTypeName(rehabType)
	normalizedSessionScope := normalizeKLineSessionScopeName(sessionScope)

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(normalizedSymbol))
	// Keep the suffix deterministic (not random): it avoids table-name collisions
	// when different symbols normalize to the same sanitized identifier.
	if normalizedSessionScope == klineSessionScopeLegacy {
		return fmt.Sprintf(
			"%s__%s__%s__%s__%08x",
			KLineTable,
			sanitizeIdentifierComponent(normalizedSymbol),
			sanitizeIdentifierComponent(normalizedInterval),
			normalizedRehabType,
			hasher.Sum32(),
		)
	}

	return fmt.Sprintf(
		"%s__%s__%s__%s__%s__%08x",
		KLineTable,
		sanitizeIdentifierComponent(normalizedSymbol),
		sanitizeIdentifierComponent(normalizedInterval),
		normalizedRehabType,
		klineSessionScopeStorageTag(normalizedSessionScope),
		hasher.Sum32(),
	)
}

func normalizeKLineSessionScopeName(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case klineSessionScopeRegular:
		return klineSessionScopeRegular
	case klineSessionScopeExtended:
		return klineSessionScopeExtended
	default:
		return klineSessionScopeLegacy
	}
}

func normalizeReadSessionScopeName(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case klineSessionScopeLegacy:
		return klineSessionScopeLegacy
	case klineSessionScopeRegular:
		return klineSessionScopeRegular
	case klineSessionScopeExtended:
		return klineSessionScopeExtended
	default:
		return klineReadSessionScopeAuto
	}
}

func klineSessionScopeStorageTag(scope string) string {
	switch normalizeKLineSessionScopeName(scope) {
	case klineSessionScopeRegular:
		return "r"
	case klineSessionScopeExtended:
		return "x"
	default:
		return "l"
	}
}

func sanitizeIdentifierComponent(value string) string {
	if value == "" {
		return "value"
	}
	builder := strings.Builder{}
	builder.Grow(len(value))
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	cleaned := strings.Trim(builder.String(), "_")
	if cleaned == "" {
		return "value"
	}
	return cleaned
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func aggregationBaseIntervals(interval types.Interval) []types.Interval {
	targetDuration, ok := safeIntervalDuration(interval)
	if !ok || targetDuration <= time.Minute || targetDuration%time.Minute != 0 {
		return nil
	}

	candidates := make([]types.Interval, 0)
	for candidate := range types.SupportedIntervals {
		candidateDuration, ok := safeIntervalDuration(candidate)
		if !ok || candidateDuration < time.Minute || candidateDuration >= targetDuration {
			continue
		}
		if targetDuration%candidateDuration != 0 {
			continue
		}
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(i, j int) bool {
		left, _ := safeIntervalDuration(candidates[i])
		right, _ := safeIntervalDuration(candidates[j])
		return left > right
	})
	return candidates
}

func canAggregateFromLowerInterval(interval types.Interval) bool {
	return len(aggregationBaseIntervals(interval)) > 0
}

func alignTimeToIntervalStart(at time.Time, interval types.Interval) time.Time {
	duration, ok := safeIntervalDuration(interval)
	if !ok {
		return at.UTC()
	}
	return at.UTC().Truncate(duration)
}

func firstClosedKLineEndAtOrAfter(at time.Time, interval types.Interval) time.Time {
	duration, ok := safeIntervalDuration(interval)
	if !ok {
		return at.UTC()
	}
	return alignTimeToIntervalStart(at, interval).Add(duration).Add(-time.Millisecond)
}

func latestClosedKLineEndAtOrBefore(at time.Time, interval types.Interval) time.Time {
	duration, ok := safeIntervalDuration(interval)
	if !ok {
		return at.UTC()
	}
	bucketStart := alignTimeToIntervalStart(at, interval)
	bucketEnd := bucketStart.Add(duration).Add(-time.Millisecond)
	if !at.Before(bucketEnd) {
		return bucketEnd
	}
	return bucketStart.Add(-time.Millisecond)
}

func safeIntervalDuration(interval types.Interval) (duration time.Duration, ok bool) {
	defer func() {
		if recover() != nil {
			duration = 0
			ok = false
		}
	}()
	duration = interval.Duration()
	return duration, duration > 0
}

func expectedKLineSchemaColumns() []string {
	return []string{
		"end_time:INTEGER:1",
		"start_time:INTEGER:0",
		"open:TEXT:0",
		"high:TEXT:0",
		"low:TEXT:0",
		"close:TEXT:0",
		"volume:TEXT:0",
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

func KLineTableName(symbol string, interval types.Interval, rehabType string) string {
	return klineTableName(symbol, interval, rehabType)
}

func KLineTableNameForSessionScope(symbol string, interval types.Interval, rehabType string, sessionScope string) string {
	return klineTableNameForSessionScope(symbol, interval, rehabType, sessionScope)
}

func NormalizeKLineSessionScopeName(scope string) string {
	return normalizeKLineSessionScopeName(scope)
}

const (
	KLineSessionScopeLegacy   = klineSessionScopeLegacy
	KLineSessionScopeRegular  = klineSessionScopeRegular
	KLineSessionScopeExtended = klineSessionScopeExtended
	KLineReadSessionScopeAuto = klineReadSessionScopeAuto
)
