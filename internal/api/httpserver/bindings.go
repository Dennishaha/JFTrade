package httpserver

import (
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"

	"github.com/gin-gonic/gin"
)

// ---- Compile-time interface checks ----

var _ encoding.TextUnmarshaler = (*OptionalIntValue)(nil)
var _ encoding.TextUnmarshaler = (*OptionalBoolValue)(nil)
var _ encoding.TextUnmarshaler = (*OptionalTimeValue)(nil)
var _ encoding.TextUnmarshaler = (*CandlePeriodValue)(nil)

// ---- Optional Int ----

// OptionalIntValue 是一个可选的整型查询参数。Set 指示参数是否出现在请求中；
// Valid 指示解析是否成功。空字符串视为有效（值为 0）。
type OptionalIntValue struct {
	Value int
	Set   bool
	Valid bool
}

func (v *OptionalIntValue) UnmarshalText(text []byte) error {
	v.Set = true
	raw := strings.TrimSpace(string(text))
	if raw == "" {
		v.Value = 0
		v.Valid = true
		return nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		v.Value = 0
		v.Valid = false
		return nil // 不返回 error，保持 Valid=false
	}
	v.Value = parsed
	v.Valid = true
	return nil
}

func (v OptionalIntValue) Int() int {
	return v.Value
}

// ---- Optional Bool ----

// OptionalBoolValue 是一个可选的布尔查询参数。
type OptionalBoolValue struct {
	Value bool
	Set   bool
}

func (v *OptionalBoolValue) UnmarshalText(text []byte) error {
	v.Set = true
	switch strings.ToLower(strings.TrimSpace(string(text))) {
	case "1", "true", "yes", "y", "on":
		v.Value = true
	case "0", "false", "no", "n", "off", "":
		v.Value = false
	default:
		v.Value = false
	}
	return nil
}

func (v OptionalBoolValue) Bool() bool {
	return v.Value
}

// ---- Optional Time ----

// OptionalTimeValue 是一个可选的时间查询参数，支持 RFC3339、RFC3339Nano、
// "2006-01-02 15:04:05" 和 "2006-01-02" 格式。
type OptionalTimeValue struct {
	time.Time
}

func (v *OptionalTimeValue) UnmarshalText(text []byte) error {
	v.Time = ParseQueryTime(string(text), time.Time{})
	return nil
}

func (v OptionalTimeValue) PtrUTC() *time.Time {
	if v.IsZero() {
		return nil
	}
	return new(v.UTC())
}

func (v OptionalTimeValue) StringValue() string {
	if v.IsZero() {
		return ""
	}
	return v.UTC().Format(time.RFC3339Nano)
}

// ---- Candle Period ----

// CandlePeriodValue 是一个 K 线周期查询参数，支持 "1m"、"5m"、"1h"、"1d" 等别名。
type CandlePeriodValue string

func (v *CandlePeriodValue) UnmarshalText(text []byte) error {
	raw := strings.TrimSpace(string(text))
	if raw == "" {
		*v = ""
		return nil
	}
	normalized, err := NormalizeCandlePeriod(raw)
	if err != nil {
		return err
	}
	*v = CandlePeriodValue(normalized)
	return nil
}

func (v CandlePeriodValue) String() string {
	return string(v)
}

// ---- URI Binding ----

// BindURI 将 Gin URI 参数绑定到目标结构体，同时校验百分号转义合法性。
func BindURI(c *gin.Context, target any) error {
	if err := c.ShouldBindUri(target); err != nil {
		return err
	}
	if rawPath := requestEscapedPath(c); rawPath != "" {
		if hasInvalidPercentEscape(rawPath) {
			return fmt.Errorf("invalid URL escape")
		}
		return nil
	}
	for _, value := range c.Params {
		if hasInvalidPercentEscape(value.Value) {
			return fmt.Errorf("invalid URL escape")
		}
	}
	return nil
}

// BindStrictJSON decodes exactly one JSON value and rejects unknown object
// fields. Use it for versioned contracts where silently ignoring an older or
// misspelled field could change execution semantics.
func BindStrictJSON(c *gin.Context, target any) error {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return io.EOF
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

// ---- Pagination ----

// NormalizeBoundPage 归一化分页参数。
func NormalizeBoundPage(limit int, offset int, defaultLimit int, maxLimit int) (int, int) {
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ---- Time Parsing ----

// ParseQueryTime 解析查询参数中的时间字符串，支持多种常见格式。
// 解析失败时返回 fallback。
func ParseQueryTime(value string, fallback time.Time) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		parsed, err := time.ParseInLocation(layout, value, time.UTC)
		if err == nil {
			return parsed.UTC()
		}
	}
	return fallback
}

// ---- Candle Period Normalization ----

// NormalizeCandlePeriod 将 K 线周期别名规范化为标准形式。
func NormalizeCandlePeriod(period string) (string, error) {
	return broker.NormalizeCandlePeriod(period)
}

// ---- Internal helpers ----

func hasInvalidPercentEscape(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			continue
		}
		if i+2 >= len(value) || !isHex(value[i+1]) || !isHex(value[i+2]) {
			return true
		}
		i += 2
	}
	return false
}

func isHex(value byte) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'f') || (value >= 'A' && value <= 'F')
}

func requestEscapedPath(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if c.Request.RequestURI != "" {
		path, _, _ := strings.Cut(c.Request.RequestURI, "?")
		return path
	}
	if c.Request.URL != nil && c.Request.URL.RawPath != "" {
		return c.Request.URL.EscapedPath()
	}
	return ""
}
