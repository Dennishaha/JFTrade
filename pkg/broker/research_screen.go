package broker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// FactorRef identifies a configured factor instance. factorKey is stable
// across catalog refreshes; instanceId distinguishes e.g. MA(20) from
// MA(60), even when both use the same factor key.
type FactorRef struct {
	InstanceID string                     `json:"instanceId"`
	FactorKey  string                     `json:"factorKey"`
	Params     ResearchScreenFactorParams `json:"params"`
}

const ScreenQuerySchemaVersionV2 = 2

func (ref FactorRef) Identity() string {
	if strings.TrimSpace(ref.InstanceID) != "" {
		return strings.TrimSpace(ref.InstanceID)
	}
	return strings.TrimSpace(ref.FactorKey)
}

// ScreenCondition is the V2, editor-facing condition model. Value is kept as
// JSON so scalar, set, interval and provider-specific union values can all be
// represented by one executable contract.
type ScreenCondition struct {
	ID           string     `json:"id"`
	Factor       FactorRef  `json:"factor"`
	Operator     string     `json:"operator"`
	Value        any        `json:"value,omitempty"`
	SecondFactor *FactorRef `json:"secondFactor,omitempty"`
}

type ScreenColumn struct {
	ID     string    `json:"columnId"`
	Factor FactorRef `json:"factor"`
	Label  string    `json:"label,omitempty"`
}

type ScreenSort struct {
	ID        string    `json:"sortId,omitempty"`
	ColumnID  string    `json:"columnId,omitempty"`
	Factor    FactorRef `json:"factor"`
	Direction string    `json:"direction" enums:"asc,desc,abs_asc,abs_desc"`
}

// ScreenDefinitionV2 is the persisted stock-screener definition. Conditions
// are ANDed in the first stock implementation; the explicit model leaves
// room for derivative-specific logic without changing this contract.
type ScreenDefinitionV2 struct {
	BrokerID           string             `json:"brokerId,omitempty"`
	Market             string             `json:"market"`
	Pool               ResearchScreenPool `json:"pool"`
	Conditions         []ScreenCondition  `json:"conditions,omitempty"`
	Columns            []ScreenColumn     `json:"columns,omitempty"`
	Sorts              []ScreenSort       `json:"sorts,omitempty"`
	CatalogVersion     string             `json:"catalogVersion"`
	QuerySchemaVersion int                `json:"querySchemaVersion"`
}

type ScreenQueryV2 struct {
	ScreenDefinitionV2
	AccountID          string                   `json:"accountId,omitempty"`
	TradingEnvironment string                   `json:"tradingEnvironment,omitempty"`
	Page               ResearchScreenPagination `json:"page"`
}

type ScreenResultColumn struct {
	ColumnID   string `json:"columnId"`
	InstanceID string `json:"instanceId"`
	FactorKey  string `json:"factorKey"`
	Label      string `json:"label,omitempty"`
	Unit       string `json:"unit,omitempty"`
}

type ScreenResultCell struct {
	ColumnID   string              `json:"columnId"`
	InstanceID string              `json:"instanceId"`
	FactorKey  string              `json:"factorKey,omitempty"`
	Value      ResearchScreenValue `json:"value"`
}

// NewFactorRef creates a stable instance identity when the caller does not
// provide one. Hashing the normalized factor and params keeps preset reloads
// deterministic while allowing same-key instances with different params.
func NewFactorRef(factorKey string, params any, instanceID string) FactorRef {
	factorKey = strings.ToLower(strings.TrimSpace(factorKey))
	instanceID = strings.TrimSpace(instanceID)
	normalizedParams := normalizeFactorParams(params)
	if instanceID == "" {
		body, _ := json.Marshal(struct {
			Key    string                     `json:"key"`
			Params ResearchScreenFactorParams `json:"params"`
		}{Key: factorKey, Params: normalizedParams})
		digest := sha256.Sum256(body)
		instanceID = "factor_" + hex.EncodeToString(digest[:8])
	}
	return FactorRef{InstanceID: instanceID, FactorKey: factorKey, Params: normalizedParams}
}

func normalizeFactorParams(value any) ResearchScreenFactorParams {
	if params, ok := value.(ResearchScreenFactorParams); ok {
		return params
	}
	content, err := json.Marshal(value)
	if err != nil || string(content) == "null" || len(content) == 0 {
		return ResearchScreenFactorParams{}
	}
	var params ResearchScreenFactorParams
	if err := json.Unmarshal(content, &params); err != nil {
		return ResearchScreenFactorParams{}
	}
	return params
}

type ResearchScreenPool struct {
	WatchlistStockIDs []string              `json:"watchlistStockIds,omitempty"`
	Plates            []ResearchScreenPlate `json:"plates,omitempty"`
}

type ResearchScreenPlate struct {
	ParentPlateID string   `json:"parentPlateId,omitempty"`
	PlateIDs      []string `json:"plateIds"`
}

// ResearchScreenFactorParams covers the provider-neutral parameter union used
// by cumulative, financial, technical, featured, broker and option factors.
// Fields not used by the selected factor family must be omitted.
type ResearchScreenFactorParams struct {
	Days                int32   `json:"days,omitempty"`
	PeriodAverage       int32   `json:"periodAverage,omitempty"`
	Term                int32   `json:"term,omitempty"`
	Duration            int64   `json:"duration,omitempty"`
	Year                int32   `json:"year,omitempty"`
	FutureDuration      int32   `json:"futureDuration,omitempty"`
	Period              int32   `json:"period,omitempty"`
	RangePeriod         int32   `json:"rangePeriod,omitempty"`
	FirstCustomParam    int64   `json:"firstCustomParam,omitempty"`
	IndicatorParams     []int64 `json:"indicatorParams,omitempty"`
	BrokerParam         string  `json:"brokerParam,omitempty"`
	OptionParamType     int32   `json:"optionParamType,omitempty"`
	OptionParamString   string  `json:"optionParamString,omitempty"`
	OptionParamInteger  int64   `json:"optionParamInteger,omitempty"`
	OptionParamIntegers []int64 `json:"optionParamIntegers,omitempty"`
	OptionHVPeriod      int32   `json:"optionHvPeriod,omitempty"`
}

type ResearchScreenPagination struct {
	Offset int `json:"offset,omitempty" minimum:"0"`
	Limit  int `json:"limit,omitempty" minimum:"1" maximum:"100"`
}

type ResearchScreenValue struct {
	Type     string   `json:"type" enums:"string,integer,integer_array,number,missing"`
	String   *string  `json:"string,omitempty"`
	Integer  *int64   `json:"integer,omitempty"`
	Integers []int64  `json:"integers,omitempty"`
	Number   *float64 `json:"number,omitempty"`
	EnumType string   `json:"enumType,omitempty"`
	EnumName string   `json:"enumName,omitempty"`
	EndTime  *int64   `json:"endTime,omitempty"`
	Unit     string   `json:"unit,omitempty"`
}

type ResearchScreenRow struct {
	StockID       string                      `json:"stockId"`
	InstrumentID  string                      `json:"instrumentId,omitempty"`
	Market        string                      `json:"market,omitempty"`
	Symbol        string                      `json:"symbol,omitempty"`
	Name          string                      `json:"name,omitempty"`
	Industry      string                      `json:"industry,omitempty"`
	QuoteCurrency string                      `json:"quoteCurrency,omitempty"`
	ProductClass  ProductClass                `json:"productClass"`
	Cells         map[string]ScreenResultCell `json:"cells"`
}

type ResearchScreenResult struct {
	Provider       ProviderAttribution   `json:"provider"`
	AsOf           time.Time             `json:"asOf"`
	CatalogVersion string                `json:"catalogVersion,omitempty"`
	Columns        []ScreenResultColumn  `json:"columns,omitempty"`
	Entries        []ResearchScreenRow   `json:"entries"`
	NextOffset     *int                  `json:"nextOffset,omitempty"`
	HasMore        bool                  `json:"hasMore"`
	Total          *int                  `json:"total,omitempty"`
	Warnings       []string              `json:"warnings,omitempty"`
	PartialErrors  []FeaturePartialError `json:"partialErrors,omitempty"`
}

var ErrResearchScreenRateLimited = errors.New("research stock screen rate limited")

type ResearchScreenRateLimitError struct {
	retryAfter time.Duration
}

func (e *ResearchScreenRateLimitError) Error() string {
	if e == nil {
		return ErrResearchScreenRateLimited.Error()
	}
	return fmt.Sprintf("%s; retry after %s", ErrResearchScreenRateLimited, e.retryAfter.Round(time.Millisecond))
}

func (e *ResearchScreenRateLimitError) Unwrap() error { return ErrResearchScreenRateLimited }

func NewResearchScreenRateLimitError(retryAfter time.Duration) error {
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	return &ResearchScreenRateLimitError{retryAfter: retryAfter}
}

func ResearchScreenRetryAfter(err error) (time.Duration, bool) {
	var target *ResearchScreenRateLimitError
	if !errors.As(err, &target) || target == nil {
		return 0, false
	}
	return target.retryAfter, true
}
