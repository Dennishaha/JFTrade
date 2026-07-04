package runtimecontrol

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

const (
	ModeOff     = "off"
	ModeMonitor = "monitor"
	ModeEnforce = "enforce"
)

type RiskSettings struct {
	Mode             string
	CloseOnly        bool
	MaxOrderQuantity *float64
	MaxOrderNotional *float64
	DailyMaxOrders   *int
	PauseOnReject    bool
}

type OrderIntent struct {
	Symbol   string
	Side     string
	Quantity float64
	Price    *float64
}

type RiskContext struct {
	CurrentPrice             float64
	SellableQuantity         float64
	TodaySubmittedOrderCount int
}

type RiskDecision struct {
	Matched       bool
	Rejected      bool
	PauseOnReject bool
	Reason        string
	Detail        string
}

func NormalizeRiskSettings(input RiskSettings) RiskSettings {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	switch mode {
	case ModeMonitor, ModeEnforce:
	default:
		mode = ModeOff
	}
	input.Mode = mode
	input.MaxOrderQuantity = normalizeOptionalPositiveFloat(input.MaxOrderQuantity)
	input.MaxOrderNotional = normalizeOptionalPositiveFloat(input.MaxOrderNotional)
	input.DailyMaxOrders = normalizeOptionalPositiveInt(input.DailyMaxOrders)
	if input.Mode == ModeOff {
		input.CloseOnly = false
		input.MaxOrderQuantity = nil
		input.MaxOrderNotional = nil
		input.DailyMaxOrders = nil
		input.PauseOnReject = false
	}
	return input
}

func EvaluateRisk(settings RiskSettings, order OrderIntent, context RiskContext) RiskDecision {
	settings = NormalizeRiskSettings(settings)
	if settings.Mode == ModeOff {
		return RiskDecision{}
	}
	reason := RejectReason(settings, order, context)
	if reason == "" {
		return RiskDecision{}
	}
	return RiskDecision{
		Matched:       true,
		Rejected:      settings.Mode == ModeEnforce,
		PauseOnReject: settings.PauseOnReject,
		Reason:        reason,
		Detail:        RiskDecisionDetail(settings, order, reason),
	}
}

func RejectReason(settings RiskSettings, order OrderIntent, context RiskContext) string {
	side := strings.ToUpper(strings.TrimSpace(order.Side))
	if settings.CloseOnly {
		if side != "SELL" {
			return "close_only"
		}
		if order.Quantity > context.SellableQuantity+0.0000001 {
			return "close_only_insufficient_position"
		}
	}
	if settings.MaxOrderQuantity != nil && order.Quantity > *settings.MaxOrderQuantity {
		return "max_order_quantity"
	}
	if settings.MaxOrderNotional != nil {
		price := 0.0
		if order.Price != nil {
			price = *order.Price
		}
		if price <= 0 {
			price = context.CurrentPrice
		}
		if price <= 0 {
			return "max_order_notional_missing_price"
		}
		if order.Quantity*price > *settings.MaxOrderNotional {
			return "max_order_notional"
		}
	}
	if settings.DailyMaxOrders != nil && context.TodaySubmittedOrderCount >= *settings.DailyMaxOrders {
		return "daily_max_orders"
	}
	return ""
}

func RiskDecisionDetail(settings RiskSettings, order OrderIntent, reason string) string {
	settings = NormalizeRiskSettings(settings)
	return fmt.Sprintf(
		"rule=%s symbol=%s side=%s qty=%s mode=%s closeOnly=%t maxQty=%s maxNotional=%s dailyMaxOrders=%s",
		strings.TrimSpace(reason),
		order.Symbol,
		order.Side,
		FormatNumber(order.Quantity),
		settings.Mode,
		settings.CloseOnly,
		OptionalFloatLabel(settings.MaxOrderQuantity),
		OptionalFloatLabel(settings.MaxOrderNotional),
		OptionalIntLabel(settings.DailyMaxOrders),
	)
}

func MarketDayStartUTC(symbol string, now time.Time) time.Time {
	if start, ok := market.TradingDayBoundaryStart(symbol, now, true); ok {
		return start
	}
	location := time.UTC
	if profile, ok := market.ProfileForSymbol(symbol); ok && profile.Location != nil {
		location = profile.Location
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).UTC()
}

func OptionalFloatLabel(value *float64) string {
	if value == nil {
		return "none"
	}
	return FormatNumber(*value)
}

func OptionalIntLabel(value *int) string {
	if value == nil {
		return "none"
	}
	return fmt.Sprintf("%d", *value)
}

func FormatNumber(value float64) string {
	text := strconv.FormatFloat(value, 'f', 4, 64)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func OptionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func TimePointerToString(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	text := value.UTC().Format(time.RFC3339Nano)
	return &text
}

func OptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func MaxTime(left time.Time, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

func normalizeOptionalPositiveFloat(input *float64) *float64 {
	if input == nil || *input <= 0 || math.IsNaN(*input) || math.IsInf(*input, 0) {
		return nil
	}
	value := *input
	return &value
}

func normalizeOptionalPositiveInt(input *int) *int {
	if input == nil || *input <= 0 {
		return nil
	}
	value := *input
	return &value
}
