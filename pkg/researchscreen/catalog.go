// Package researchscreen owns the versioned broker-neutral stock-screen
// factor catalog. Provider numeric IDs stay server-side and are never emitted
// by the HTTP catalog.
package researchscreen

import (
	"fmt"
	"slices"
	"strings"
)

const (
	CatalogVersion = "futu-stock-screen-v1"
	// CatalogSchemaVersion identifies semantic metadata layered over the
	// generated provider factor-set revision named by CatalogVersion.
	CatalogSchemaVersion = 2
	QuerySchemaVersion   = 2
	ProviderVersion      = "10.9.6908"
)

type ParameterDescriptor struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	EditorType  string         `json:"editorType,omitempty"`
	Enum        string         `json:"enum,omitempty"`
	Required    bool           `json:"required"`
	Default     any            `json:"default"`
	Minimum     *int64         `json:"minimum,omitempty"`
	Maximum     *int64         `json:"maximum,omitempty"`
	Step        *float64       `json:"step,omitempty"`
	Unit        string         `json:"unit,omitempty"`
	VisibleWhen map[string]any `json:"visibleWhen,omitempty"`
	Help        string         `json:"help,omitempty"`
}

type EnumOption struct {
	Key   string `json:"key"`
	Value int64  `json:"value"`
	Label string `json:"label"`
}

type FactorDescriptor struct {
	Key             string                `json:"key"`
	Category        string                `json:"category"`
	Label           string                `json:"label"`
	ValueType       string                `json:"valueType"`
	Unit            string                `json:"unit,omitempty"`
	CurrencyBasis   string                `json:"currencyBasis,omitempty" enums:"quote,reporting"`
	DisplayFormat   string                `json:"displayFormat,omitempty" enums:"price,compact_amount,percent,integer,timestamp,number"`
	FilterKind      string                `json:"filterKind,omitempty"`
	ConditionEditor string                `json:"conditionEditor,omitempty"`
	ValueEnum       string                `json:"valueEnum,omitempty"`
	Operators       []string              `json:"operators,omitempty"`
	Filter          bool                  `json:"filter"`
	Retrieve        bool                  `json:"retrieve"`
	Sort            bool                  `json:"sort"`
	Roles           []string              `json:"roles,omitempty"`
	Parameters      []ParameterDescriptor `json:"parameters,omitempty"`
	Markets         []string              `json:"markets,omitempty"`
	Availability    string                `json:"availability" enums:"available,experimental,unsupported"`
	Reason          string                `json:"reason,omitempty"`
	Help            string                `json:"help,omitempty"`
	SearchKeywords  []string              `json:"searchKeywords,omitempty"`

	ProviderID int32 `json:"-"`
}

type Category struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type RateLimit struct {
	Requests int `json:"requests"`
	Window   int `json:"windowSeconds"`
}

type Catalog struct {
	Version            string                  `json:"version"`
	SchemaVersion      int                     `json:"schemaVersion"`
	QuerySchemaVersion int                     `json:"querySchemaVersion"`
	Provider           string                  `json:"provider"`
	ProviderVersion    string                  `json:"providerVersion"`
	Market             string                  `json:"market,omitempty"`
	Markets            []string                `json:"markets"`
	Categories         []Category              `json:"categories"`
	Factors            []FactorDescriptor      `json:"factors"`
	Enums              map[string][]EnumOption `json:"enums"`
	RateLimit          RateLimit               `json:"rateLimit"`
}

var categoryLabels = map[string]string{
	"field":       "股票范围",
	"basic":       "基础信息",
	"simple":      "基础行情",
	"cumulative":  "累计行情",
	"financial":   "财务指标",
	"indicator":   "技术指标",
	"pattern":     "技术形态",
	"featured":    "特色数据",
	"broker":      "经纪商持仓",
	"option":      "期权指标",
	"kline_shape": "K 线形态",
}

var factorByKey map[string]FactorDescriptor
var factorByProviderKey map[string]FactorDescriptor

func init() {
	factorByKey = make(map[string]FactorDescriptor, len(generatedFactors))
	factorByProviderKey = make(map[string]FactorDescriptor, len(generatedFactors))
	for index, factor := range generatedFactors {
		factor = decorateFactorSemantics(factor)
		generatedFactors[index] = factor
		if factor.Key == "" || factor.ProviderID <= 0 {
			panic("research screen catalog contains an invalid factor")
		}
		if _, exists := factorByKey[factor.Key]; exists {
			panic("research screen catalog contains duplicate factor " + factor.Key)
		}
		factorByKey[factor.Key] = factor
		factorByProviderKey[providerKey(factor.Category, factor.ProviderID)] = factor
	}
	if err := ValidateCatalog(); err != nil {
		panic(err)
	}
}

var factorUnitOverrides = map[string]string{
	"simple.long_margin_allowed":                "",
	"simple.short_margin_allowed":               "",
	"simple.price_to_52w_high":                  "percent",
	"simple.price_to_52w_low":                   "percent",
	"simple.high_to_52w_high":                   "percent",
	"simple.low_to_52w_low":                     "percent",
	"simple.volume_ratio":                       "",
	"simple.pe_annual":                          "",
	"simple.pe_ttm":                             "",
	"simple.pb":                                 "",
	"financial.equity_multiplier":               "",
	"financial.money_turnover_cycle":            "days",
	"financial.stockholder_profit_cagr":         "percent",
	"financial.operating_profit_cagr":           "percent",
	"financial.free_cash_cagr":                  "percent",
	"financial.total_assets_cagr":               "percent",
	"financial.operating_revenue_cash_cover":    "percent",
	"financial.surprise_revenue_date":           "timestamp",
	"financial.surprise_revenue_term":           "",
	"financial.surprise_revenue_post_period":    "",
	"financial.surprise_revenue_date_v2":        "timestamp",
	"financial.surprise_revenue_post_period_v2": "",
	"featured.cash_flow_net_in_count":           "count",
	"featured.cash_flow_net_out_count":          "count",
	"featured.cash_flow_main_in_count":          "count",
	"featured.cash_flow_main_out_count":         "count",
}

var explicitQuotePriceFactors = map[string]struct{}{
	"simple.last_close":               {},
	"simple.high":                     {},
	"simple.low":                      {},
	"simple.last_close_hp":            {},
	"simple.high_hp":                  {},
	"simple.low_hp":                   {},
	"featured.morningstar_fair_value": {},
	"kline_shape.support_level":       {},
	"kline_shape.pressure_level":      {},
}

func decorateFactorSemantics(factor FactorDescriptor) FactorDescriptor {
	if unit, exists := factorUnitOverrides[factor.Key]; exists {
		factor.Unit = unit
	}
	if isQuotePriceFactor(factor.Key) {
		factor.Unit = "currency"
	}
	switch factor.Unit {
	case "currency":
		factor.DisplayFormat = "compact_amount"
		if isPriceDisplayFactor(factor.Key) {
			factor.DisplayFormat = "price"
		}
		if factor.Category == "financial" && factor.Key != "financial.float_market_cap" {
			factor.CurrencyBasis = "reporting"
		} else {
			factor.CurrencyBasis = "quote"
		}
	case "percent":
		factor.DisplayFormat = "percent"
	case "timestamp":
		factor.DisplayFormat = "timestamp"
	case "shares", "count", "days":
		factor.DisplayFormat = "integer"
	default:
		switch factor.ValueType {
		case "integer":
			factor.DisplayFormat = "integer"
		case "number":
			factor.DisplayFormat = "number"
		}
	}
	// Generated rows intentionally contain only the provider's parameter
	// names. Keep the generated file stable and attach the UI contract here so
	// semantic changes do not get overwritten by the next provider refresh.
	factor.Parameters = decorateParameters(factor.Parameters)
	factor.ConditionEditor, factor.ValueEnum, factor.Operators = conditionContract(factor)
	roles := factor.Roles[:0]
	if factor.Filter {
		roles = append(roles, "condition")
	}
	if factor.Retrieve {
		roles = append(roles, "column")
	}
	if factor.Sort {
		roles = append(roles, "sort")
	}
	factor.Roles = roles
	if strings.TrimSpace(factor.Help) == "" {
		factor.Help = factor.Label
	}
	if len(factor.SearchKeywords) == 0 {
		factor.SearchKeywords = factorSearchKeywords(factor)
	}
	return factor
}

func conditionContract(factor FactorDescriptor) (string, string, []string) {
	if !factor.Filter {
		return "", "", nil
	}
	switch factor.FilterKind {
	case "enum":
		valueEnum := factorValueEnum(factor)
		if valueEnum == "" {
			return "integer", "", []string{"in"}
		}
		return "singleSelect", valueEnum, []string{"in"}
	case "set":
		valueEnum := factorValueEnum(factor)
		if valueEnum == "" {
			return "integerSet", "", []string{"in"}
		}
		return "multiSelect", valueEnum, []string{"in"}
	case "interval":
		return "range", "", []string{"between"}
	case "interval_or_set":
		return "rangeOrSet", factorValueEnum(factor), []string{"between", "in"}
	case "position":
		return "indicatorCompare", "", []string{"position"}
	case "pattern":
		return "pattern", factorValueEnum(factor), []string{"pattern"}
	default:
		return "range", "", []string{"between"}
	}
}

func factorValueEnum(factor FactorDescriptor) string {
	switch {
	case factor.Key == "field.market":
		return "market"
	case factor.Category == "kline_shape":
		return "kline_shape_type"
	case strings.Contains(factor.Key, "cash_flow"):
		return "cash_flow_period"
	default:
		return ""
	}
}

func decorateParameters(parameters []ParameterDescriptor) []ParameterDescriptor {
	if len(parameters) == 0 {
		return nil
	}
	result := make([]ParameterDescriptor, len(parameters))
	for index, parameter := range parameters {
		parameter.EditorType = parameterEditorType(parameter)
		parameter.Required, parameter.Default, parameter.Minimum, parameter.Maximum, parameter.Step = parameterContract(parameter)
		if parameter.Enum != "" {
			parameter.VisibleWhen = nil
		}
		if parameter.Help == "" {
			parameter.Help = parameterHelp(parameter)
		}
		result[index] = parameter
	}
	return result
}

func parameterEditorType(parameter ParameterDescriptor) string {
	if parameter.EditorType != "" {
		return parameter.EditorType
	}
	if parameter.Enum != "" {
		return "select"
	}
	switch parameter.Type {
	case "integer_array", "number_array":
		return "multiNumber"
	case "union":
		return "union"
	case "string":
		return "text"
	case "date", "timestamp":
		return "date"
	default:
		return "number"
	}
}

func parameterContract(parameter ParameterDescriptor) (bool, any, *int64, *int64, *float64) {
	minimum := parameter.Minimum
	maximum := parameter.Maximum
	step := parameter.Step
	if step == nil {
		value := float64(1)
		step = &value
	}
	if minimum == nil {
		value := int64(0)
		minimum = &value
	}
	var required bool
	var defaultValue any
	switch parameter.Name {
	case "days":
		required, defaultValue = true, int64(1)
		minimum = int64PointerAtLeast(minimum, 1)
		maximum = int64PointerAtMost(maximum, 3650)
	case "period":
		required, defaultValue = true, int64(11)
	case "term":
		// Latest quarter is a useful, provider-supported default. A zero term
		// remains valid for callers that explicitly request provider defaults.
		required, defaultValue = false, int64(10)
	case "optionHvPeriod":
		required, defaultValue = false, int64(0)
	case "futureDuration":
		required, defaultValue = false, int64(0)
	case "rangePeriod":
		required, defaultValue = false, int64(1)
	case "periodAverage":
		required, defaultValue = false, int64(0)
	case "duration", "year", "firstCustomParam":
		required, defaultValue = false, int64(0)
	case "indicatorParams":
		required, defaultValue = false, []int64{}
	case "brokerParam", "optionParam":
		required, defaultValue = false, ""
	default:
		required, defaultValue = parameter.Required, parameter.Default
	}
	if parameter.Required {
		required = true
	}
	if parameter.Default != nil {
		defaultValue = parameter.Default
	}
	return required, defaultValue, minimum, maximum, step
}

func int64PointerAtLeast(value *int64, minimum int64) *int64 {
	if value == nil || *value < minimum {
		return &minimum
	}
	return value
}

func int64PointerAtMost(value *int64, maximum int64) *int64 {
	if value == nil || *value > maximum {
		return &maximum
	}
	return value
}

func parameterHelp(parameter ParameterDescriptor) string {
	if parameter.Enum != "" {
		return "从目录枚举中选择"
	}
	switch parameter.Name {
	case "days":
		return "统计窗口，单位为交易日"
	case "period":
		return "K 线周期"
	case "term":
		return "财务数据周期"
	case "duration":
		return "累计周期或持续时长"
	case "indicatorParams":
		return "指标专用参数，按目录顺序填写"
	case "optionParam":
		return "期权参数联合类型"
	default:
		return "可选参数"
	}
}

func factorSearchKeywords(factor FactorDescriptor) []string {
	keywords := []string{factor.Key, factor.Label}
	for _, part := range strings.FieldsFunc(factor.Key, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	}) {
		if part != "" {
			keywords = append(keywords, part)
		}
	}
	return keywords
}

func isQuotePriceFactor(key string) bool {
	if _, ok := explicitQuotePriceFactors[key]; ok {
		return true
	}
	return strings.HasPrefix(key, "indicator.ma") ||
		strings.HasPrefix(key, "indicator.ema") ||
		strings.HasPrefix(key, "indicator.boll")
}

func isPriceDisplayFactor(key string) bool {
	if isQuotePriceFactor(key) {
		return true
	}
	switch key {
	case "simple.price", "simple.open_price", "simple.bid_price", "simple.ask_price",
		"simple.lot_price", "simple.price_hp", "simple.open_price_hp",
		"simple.bid_price_hp", "simple.ask_price_hp", "simple.lot_price_hp",
		"simple.before_price", "simple.before_price_change", "simple.after_price",
		"simple.after_price_change", "simple.before_price_hp",
		"simple.before_price_change_hp", "simple.after_price_hp",
		"simple.after_price_change_hp", "simple.overnight_price",
		"simple.overnight_price_change", "cumulative.price_change",
		"cumulative.amplitude", "cumulative.price_change_hp", "indicator.price",
		"featured.analyst_target_price":
		return true
	default:
		return false
	}
}

func Lookup(key string) (FactorDescriptor, bool) {
	factor, ok := factorByKey[strings.ToLower(strings.TrimSpace(key))]
	return factor, ok
}

func LookupProvider(category string, providerID int32) (FactorDescriptor, bool) {
	factor, ok := factorByProviderKey[providerKey(category, providerID)]
	return factor, ok
}

func providerKey(category string, providerID int32) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(category), providerID)
}

func FullCatalog() Catalog {
	return CatalogForMarket("")
}

func CatalogForMarket(market string) Catalog {
	market = strings.ToUpper(strings.TrimSpace(market))
	factors := append([]FactorDescriptor(nil), generatedFactors...)
	counts := make(map[string]int)
	for index, factor := range factors {
		factor = factorAvailability(factor, market)
		factors[index] = factor
		counts[factor.Category]++
	}
	categories := make([]Category, 0, len(counts))
	for key, count := range counts {
		categories = append(categories, Category{Key: key, Label: categoryLabels[key], Count: count})
	}
	slices.SortFunc(categories, func(a, b Category) int {
		return strings.Compare(a.Key, b.Key)
	})
	return Catalog{
		Version:            CatalogVersion,
		SchemaVersion:      CatalogSchemaVersion,
		QuerySchemaVersion: QuerySchemaVersion,
		Provider:           "futu",
		ProviderVersion:    ProviderVersion,
		Market:             market,
		Markets:            []string{"HK", "US", "SH", "SZ"},
		Categories:         categories,
		Factors:            factors,
		Enums:              cloneEnums(generatedEnums),
		RateLimit:          RateLimit{Requests: 10, Window: 30},
	}
}

func cloneEnums(source map[string][]EnumOption) map[string][]EnumOption {
	result := make(map[string][]EnumOption, len(source))
	for key, values := range source {
		result[key] = append([]EnumOption(nil), values...)
	}
	return result
}

func factorAvailability(factor FactorDescriptor, market string) FactorDescriptor {
	factor.Availability = "available"
	factor.Markets = []string{"HK", "US", "SH", "SZ"}
	switch factor.Category {
	case "broker":
		factor.Markets = []string{"HK"}
		switch factor.ProviderID {
		case 6102, 6104, 6105:
			factor.Availability = "unsupported"
			factor.Reason = "OpenD 10.9 documents this broker-holdings factor as unsupported"
		}
	case "option":
		factor.Markets = []string{"HK", "US"}
	}
	if market != "" && !slices.Contains(factor.Markets, market) {
		factor.Availability = "unsupported"
		factor.Reason = "factor is unavailable in " + market
	}
	return factor
}

func ValidateFactorUse(key string, filter, retrieve, sort bool) (FactorDescriptor, error) {
	factor, ok := Lookup(key)
	if !ok {
		return FactorDescriptor{}, fmt.Errorf("unknown research screen factor %q", key)
	}
	switch {
	case filter && !factor.Filter:
		return FactorDescriptor{}, fmt.Errorf("research screen factor %q cannot be filtered", key)
	case retrieve && !factor.Retrieve:
		return FactorDescriptor{}, fmt.Errorf("research screen factor %q cannot be retrieved", key)
	case sort && !factor.Sort:
		return FactorDescriptor{}, fmt.Errorf("research screen factor %q cannot be sorted", key)
	default:
		return factor, nil
	}
}

func ValidateFactorForMarket(key, market string, filter, retrieve, sort bool) (FactorDescriptor, error) {
	factor, err := ValidateFactorUse(key, filter, retrieve, sort)
	if err != nil {
		return FactorDescriptor{}, err
	}
	factor = factorAvailability(factor, strings.ToUpper(strings.TrimSpace(market)))
	if factor.Availability == "unsupported" {
		return FactorDescriptor{}, fmt.Errorf("research screen factor %q is unavailable: %s", key, factor.Reason)
	}
	return factor, nil
}

// ValidateCatalog checks the generated provider rows against the semantic
// contract required by the editor and serializer. It is exported so CI and
// provider refresh jobs can fail before an incomplete catalog is published.
func ValidateCatalog() error {
	if len(generatedFactors) == 0 {
		return fmt.Errorf("research screen catalog is empty")
	}
	for _, factor := range generatedFactors {
		if factor.Key == "" || factor.Category == "" || factor.Label == "" {
			return fmt.Errorf("factor %q has incomplete identity", factor.Key)
		}
		if !factor.Filter && !factor.Retrieve && !factor.Sort {
			return fmt.Errorf("factor %q has no supported role", factor.Key)
		}
		if len(factor.Roles) == 0 {
			return fmt.Errorf("factor %q has no semantic roles", factor.Key)
		}
		if factor.Filter && (factor.ConditionEditor == "" || len(factor.Operators) == 0) {
			return fmt.Errorf("factor %q has no condition editor contract", factor.Key)
		}
		if factor.ValueEnum != "" {
			if _, ok := generatedEnums[factor.ValueEnum]; !ok {
				return fmt.Errorf("factor %q references unknown value enum %q", factor.Key, factor.ValueEnum)
			}
		}
		for _, parameter := range factor.Parameters {
			if parameter.Name == "" || parameter.Type == "" || parameter.EditorType == "" {
				return fmt.Errorf("factor %q has incomplete parameter %q", factor.Key, parameter.Name)
			}
			if parameter.Enum != "" {
				if _, ok := generatedEnums[parameter.Enum]; !ok {
					return fmt.Errorf("factor %q parameter %q references unknown enum %q", factor.Key, parameter.Name, parameter.Enum)
				}
			}
			if parameter.Default == nil && parameter.Required {
				return fmt.Errorf("factor %q required parameter %q has no default", factor.Key, parameter.Name)
			}
		}
	}
	return nil
}
