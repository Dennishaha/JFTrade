package servercore

import (
	"strconv"
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

type marketInstrumentURI struct {
	Market string `uri:"market" binding:"required"`
	Symbol string `uri:"symbol" binding:"required"`
}

type accountRecordURI struct {
	AccountRecordID string `uri:"accountRecordId" binding:"required"`
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

type instrumentSearchQuery struct {
	Query string `form:"query"`
}

type normalizeMarketInstrumentRequest struct {
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	Code         string `json:"code"`
	InstrumentID string `json:"instrumentId"`
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

type adkTasksQuery struct {
	Limit   optionalIntValue `form:"limit,parser=encoding.TextUnmarshaler"`
	Offset  optionalIntValue `form:"offset,parser=encoding.TextUnmarshaler"`
	Status  string           `form:"status"`
	AgentID string           `form:"agentId"`
	RunID   string           `form:"runId"`
}

type adkMemoryQuery struct {
	Scope   string `form:"scope"`
	AgentID string `form:"agentId"`
	Key     string `form:"key"`
}

type memoryURI struct {
	MemoryID string `uri:"memoryId" binding:"required"`
}

type marketSubscriptionDeleteQuery struct {
	ConsumerID string `form:"consumerId"`
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
