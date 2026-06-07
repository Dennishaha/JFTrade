package jftradeapi

import (
	"encoding"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var _ encoding.TextUnmarshaler = (*optionalIntValue)(nil)
var _ encoding.TextUnmarshaler = (*optionalBoolValue)(nil)
var _ encoding.TextUnmarshaler = (*optionalTimeValue)(nil)
var _ encoding.TextUnmarshaler = (*candlePeriodValue)(nil)

type brokerResourceURI struct {
	BrokerID string `uri:"brokerId" binding:"required"`
	Resource string `uri:"resource" binding:"required"`
}

type marketInstrumentURI struct {
	Market string `uri:"market" binding:"required"`
	Symbol string `uri:"symbol" binding:"required"`
}

type accountRecordURI struct {
	AccountRecordID string `uri:"accountRecordId" binding:"required"`
}

type operationURI struct {
	OperationID string `uri:"operationId" binding:"required"`
}

type pluginURI struct {
	PluginID string `uri:"pluginId" binding:"required"`
}

type definitionURI struct {
	DefinitionID string `uri:"definitionId" binding:"required"`
}

type instanceURI struct {
	InstanceID string `uri:"instanceId" binding:"required"`
}

type backtestRunURI struct {
	RunID string `uri:"runId" binding:"required"`
}

type taskURI struct {
	TaskID string `uri:"taskId" binding:"required"`
}

type sessionURI struct {
	SessionID string `uri:"sessionId" binding:"required"`
}

type runURI struct {
	RunID string `uri:"runId" binding:"required"`
}

type approvalURI struct {
	ApprovalID string `uri:"approvalId" binding:"required"`
}

type providerURI struct {
	ProviderID string `uri:"providerId" binding:"required"`
}

type agentURI struct {
	AgentID string `uri:"agentId" binding:"required"`
}

type skillURI struct {
	SkillID string `uri:"skillId" binding:"required"`
}

type internalOrderURI struct {
	InternalOrderID string `uri:"internalOrderId" binding:"required"`
}

type instrumentSearchQuery struct {
	Query string `form:"query"`
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

type strategyDefinitionPreviewQuery struct {
	Interval         string `form:"interval"`
	Symbol           string `form:"symbol"`
	UseExtendedHours bool   `form:"useExtendedHours"`
}

type strategyActivityPageQuery struct {
	Limit    optionalIntValue  `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset   optionalIntValue  `form:"offset,parser=encoding.TextUnmarshaler"`
	Level    string            `form:"level"`
	Kind     string            `form:"kind"`
	FromTime optionalTimeValue `form:"fromTime,parser=encoding.TextUnmarshaler"`
	ToTime   optionalTimeValue `form:"toTime,parser=encoding.TextUnmarshaler"`
}

type executionOrdersQuery struct {
	Scope              string `form:"scope"`
	BrokerID           string `form:"brokerId"`
	TradingEnvironment string `form:"tradingEnvironment"`
	AccountID          string `form:"accountId"`
	Market             string `form:"market"`
}

type brokerBaseReadQuery struct {
	TradingEnvironment string `form:"tradingEnvironment"`
	AccountID          string `form:"accountId"`
	Market             string `form:"market"`
}

type brokerOrdersReadQuery struct {
	brokerBaseReadQuery
	Scope     string   `form:"scope"`
	Symbol    string   `form:"symbol"`
	StartTime string   `form:"startTime"`
	EndTime   string   `form:"endTime"`
	Status    []string `form:"status"`
	Statuses  []string `form:"statuses"`
}

type brokerFillsReadQuery struct {
	brokerBaseReadQuery
	Scope     string `form:"scope"`
	Symbol    string `form:"symbol"`
	StartTime string `form:"startTime"`
	EndTime   string `form:"endTime"`
}

type brokerCashFlowsReadQuery struct {
	brokerBaseReadQuery
	ClearingDate string `form:"clearingDate"`
	Direction    string `form:"direction"`
}

type brokerOrderFeesReadQuery struct {
	brokerBaseReadQuery
	OrderIDEx     []string `form:"orderIdEx"`
	OrderIDExList []string `form:"orderIdExList"`
}

type brokerMarginRatiosReadQuery struct {
	brokerBaseReadQuery
	Symbol  []string `form:"symbol"`
	Symbols []string `form:"symbols"`
}

type brokerMaxTradeQuantityReadQuery struct {
	brokerBaseReadQuery
	Symbol             string `form:"symbol"`
	OrderType          string `form:"orderType"`
	Price              string `form:"price"`
	OrderIDEx          string `form:"orderIdEx"`
	AdjustSideAndLimit string `form:"adjustSideAndLimit"`
	Session            string `form:"session"`
	PositionID         string `form:"positionId"`
}

type brokerQuoteReadQuery struct {
	brokerBaseReadQuery
	Symbol  []string `form:"symbol"`
	Symbols []string `form:"symbols"`
}

type brokerKLinesReadQuery struct {
	brokerBaseReadQuery
	Symbol   string `form:"symbol"`
	Period   string `form:"period"`
	FromTime string `form:"fromTime"`
	ToTime   string `form:"toTime"`
	Limit    string `form:"limit"`
}

type brokerSecuritiesReadQuery struct {
	brokerBaseReadQuery
	Symbol  []string `form:"symbol"`
	Symbols []string `form:"symbols"`
}

type brokerCancelOrdersRequest struct {
	Orders []brokerCancelOrderItem `json:"orders"`
}

type brokerCancelOrderItem struct {
	OrderID       uint64 `json:"orderId"`
	BrokerOrderID string `json:"brokerOrderId"`
	Symbol        string `json:"symbol"`
}

type brokerPlaceOrderRequest struct {
	Symbol         string   `json:"symbol"`
	Side           string   `json:"side"`
	OrderType      string   `json:"orderType"`
	Price          *float64 `json:"price,omitempty"`
	Quantity       float64  `json:"quantity"`
	TimeInForce    *string  `json:"timeInForce,omitempty"`
	ClientOrderID  string   `json:"clientOrderId,omitempty"`
	Remark         *string  `json:"remark,omitempty"`
	Session        *string  `json:"session,omitempty"`
	FillOutsideRTH *bool    `json:"fillOutsideRTH,omitempty"`
}

type brokerUnlockTradeRequest struct {
	Unlock      bool   `json:"unlock"`
	PasswordMD5 string `json:"passwordMd5,omitempty"`
}

type adkPageQuery struct {
	Limit  optionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset optionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
}

type adkSessionsQuery struct {
	Limit   optionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset  optionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
	AgentID string           `form:"agentId"`
	Query   string           `form:"query"`
}

type adkRunsQuery struct {
	Limit     optionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset    optionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
	Status    string           `form:"status"`
	AgentID   string           `form:"agentId"`
	SessionID string           `form:"sessionId"`
}

type adkApprovalsQuery struct {
	Limit   optionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset  optionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
	Status  string           `form:"status"`
	AgentID string           `form:"agentId"`
}

type adkAgentsQuery struct {
	Status string `form:"status"`
}

type adkAuditQuery struct {
	Kind      string `form:"kind"`
	SubjectID string `form:"subjectId"`
}

type marketSubscriptionDeleteQuery struct {
	ConsumerID string `form:"consumerId"`
}

type optionalIntValue struct {
	Value int
	Set   bool
	Valid bool
}

func (v *optionalIntValue) UnmarshalText(text []byte) error {
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
		return nil
	}
	v.Value = parsed
	v.Valid = true
	return nil
}

func (v optionalIntValue) Int() int {
	return v.Value
}

func newOptionalIntValue(value int) optionalIntValue {
	return optionalIntValue{Value: value, Set: true, Valid: true}
}

type optionalBoolValue struct {
	Value bool
	Set   bool
}

func (v *optionalBoolValue) UnmarshalText(text []byte) error {
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

func (v optionalBoolValue) Bool() bool {
	return v.Value
}

func newOptionalBoolValue(value bool) optionalBoolValue {
	return optionalBoolValue{Value: value, Set: true}
}

type optionalTimeValue struct {
	time.Time
}

func (v *optionalTimeValue) UnmarshalText(text []byte) error {
	v.Time = parseQueryTime(string(text), time.Time{})
	return nil
}

func (v optionalTimeValue) PtrUTC() *time.Time {
	if v.Time.IsZero() {
		return nil
	}
	result := v.Time.UTC()
	return &result
}

func (v optionalTimeValue) StringValue() string {
	if v.Time.IsZero() {
		return ""
	}
	return v.Time.Format(time.RFC3339Nano)
}

func newOptionalTimeValue(value time.Time) optionalTimeValue {
	return optionalTimeValue{Time: value}
}

type candlePeriodValue string

func (v *candlePeriodValue) UnmarshalText(text []byte) error {
	raw := strings.TrimSpace(string(text))
	if raw == "" {
		*v = ""
		return nil
	}
	normalized, err := normalizeCandlePeriod(raw)
	if err != nil {
		return err
	}
	*v = candlePeriodValue(normalized)
	return nil
}

func (v candlePeriodValue) String() string {
	return string(v)
}

func bindURI(c *gin.Context, target any) error {
	if err := c.ShouldBindUri(target); err != nil {
		return err
	}
	if c.Request == nil || c.Request.RequestURI != "" {
		return nil
	}
	for _, value := range c.Params {
		if hasInvalidPercentEscape(value.Value) {
			return fmt.Errorf("invalid URL escape")
		}
	}
	return nil
}

func normalizeBoundPage(limit int, offset int, defaultLimit int, maxLimit int) (int, int) {
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

func (q marketCandlesQuery) values() map[string][]string {
	values := map[string][]string{}
	if q.Period != "" {
		values["period"] = []string{q.Period.String()}
	}
	if q.Limit.Set && q.Limit.Valid {
		values["limit"] = []string{strconv.Itoa(q.Limit.Int())}
	}
	if value := q.FromTime.StringValue(); value != "" {
		values["fromTime"] = []string{value}
	}
	if value := q.ToTime.StringValue(); value != "" {
		values["toTime"] = []string{value}
	}
	if value := q.From.StringValue(); value != "" {
		values["from"] = []string{value}
	}
	if value := q.To.StringValue(); value != "" {
		values["to"] = []string{value}
	}
	return values
}

func (q marketDepthQuery) values() map[string][]string {
	values := map[string][]string{}
	if q.Num.Set && q.Num.Valid {
		values["num"] = []string{strconv.Itoa(q.Num.Int())}
	}
	return values
}

func (q marketSnapshotQuery) values() map[string][]string {
	values := map[string][]string{}
	if q.Refresh.Set {
		values["refresh"] = []string{strconv.FormatBool(q.Refresh.Bool())}
	}
	return values
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
