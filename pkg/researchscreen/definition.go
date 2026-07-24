package researchscreen

// This file contains the broker-neutral V2 normalizer and validator. Keeping
// these contracts in the catalog package means the HTTP API, preset store, and
// broker adapter cannot silently disagree about which fields are executable.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

const defaultBroker = "futu"

// FieldError points callers at the exact draft field that failed validation.
// It intentionally has a small stable JSON-like shape so API layers can map
// it to their existing error envelope without depending on this package.
type FieldError struct {
	Path    string
	Code    string
	Message string
}

func (e *FieldError) Error() string {
	if e == nil {
		return "research screen validation failed"
	}
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

func issue(path, code, message string) error {
	return &FieldError{Path: path, Code: code, Message: message}
}

// ValidateDefinitionV2 validates all fields that affect serialization and
// execution. It does not mutate the supplied definition.
func ValidateDefinitionV2(def broker.ScreenDefinitionV2) error {
	_, err := NormalizeDefinitionV2(def)
	return err
}

// NormalizeDefinitionV2 canonicalizes case, fills stable IDs, and validates
// the V2-only document. Callers must send explicit schema and catalog versions.
func NormalizeDefinitionV2(input broker.ScreenDefinitionV2) (broker.ScreenDefinitionV2, error) {
	def := input
	if err := normalizeDefinitionHeaderAndPool(&def); err != nil {
		return def, err
	}
	if err := normalizeDefinitionConditions(&def); err != nil {
		return def, err
	}
	if err := normalizeDefinitionColumns(&def); err != nil {
		return def, err
	}
	if err := normalizeDefinitionSorts(&def); err != nil {
		return def, err
	}
	return def, nil
}

func normalizeDefinitionHeaderAndPool(def *broker.ScreenDefinitionV2) error {
	def.BrokerID = strings.ToLower(strings.TrimSpace(def.BrokerID))
	if def.BrokerID == "" {
		def.BrokerID = defaultBroker
	}
	def.Market = strings.ToUpper(strings.TrimSpace(def.Market))
	if def.QuerySchemaVersion != broker.ScreenQuerySchemaVersionV2 {
		return issue("querySchemaVersion", "unsupported_schema", fmt.Sprintf("must be %d", broker.ScreenQuerySchemaVersionV2))
	}
	if def.CatalogVersion != CatalogVersion {
		return issue("catalogVersion", "unsupported_catalog", fmt.Sprintf("catalog %q is not executable", def.CatalogVersion))
	}
	if def.Market != "HK" && def.Market != "US" && def.Market != "SH" && def.Market != "SZ" {
		return issue("market", "unsupported_market", "must be one of HK, US, SH or SZ")
	}
	for index := range def.Pool.Plates {
		def.Pool.Plates[index].ParentPlateID = strings.TrimSpace(def.Pool.Plates[index].ParentPlateID)
		def.Pool.Plates[index].PlateIDs = cleanIDs(def.Pool.Plates[index].PlateIDs)
		if len(def.Pool.Plates[index].PlateIDs) == 0 {
			return issue(fmt.Sprintf("pool.plates[%d].plateIds", index), "required", "at least one plate id is required")
		}
	}
	for index, id := range def.Pool.WatchlistStockIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return issue(fmt.Sprintf("pool.watchlistStockIds[%d]", index), "required", "stock id is required")
		}
		if _, err := strconv.ParseUint(id, 10, 64); err != nil {
			return issue(fmt.Sprintf("pool.watchlistStockIds[%d]", index), "invalid_id", "must be an unsigned integer string")
		}
		def.Pool.WatchlistStockIDs[index] = id
	}
	return nil
}

func normalizeDefinitionConditions(def *broker.ScreenDefinitionV2) error {
	seenConditions := make(map[string]struct{}, len(def.Conditions))
	seenConditionFactors := make(map[string]struct{}, len(def.Conditions))
	for index := range def.Conditions {
		condition := &def.Conditions[index]
		path := fmt.Sprintf("conditions[%d]", index)
		condition.ID = strings.TrimSpace(condition.ID)
		if condition.ID == "" {
			condition.ID = stableInstanceID("condition", condition.Factor, index)
		}
		if _, exists := seenConditions[condition.ID]; exists {
			return issue(path+".id", "duplicate", "condition id must be unique")
		}
		seenConditions[condition.ID] = struct{}{}
		if err := validateRef(path+".factor", condition.Factor, def.Market, true, false, false); err != nil {
			return err
		}
		condition.Factor.Params = normalizeFactorParams(condition.Factor)
		condition.Factor.InstanceID = normalizedInstanceID(condition.Factor.InstanceID, condition.Factor, index)
		configurationKey := factorConfigurationKey(condition.Factor)
		if _, exists := seenConditionFactors[configurationKey]; exists {
			return issue(path+".factor", "duplicate_factor", "the same factor and parameters already exist")
		}
		seenConditionFactors[configurationKey] = struct{}{}
		condition.Operator = strings.ToLower(strings.TrimSpace(condition.Operator))
		if condition.Operator == "" {
			condition.Operator = inferOperator(condition.Value)
		}
		if err := validateConditionValue(path, condition); err != nil {
			return err
		}
		if condition.SecondFactor != nil {
			if err := validateRef(path+".secondFactor", *condition.SecondFactor, def.Market, true, false, false); err != nil {
				return err
			}
			firstFactor, firstOK := Lookup(condition.Factor.FactorKey)
			secondFactor, secondOK := Lookup(condition.SecondFactor.FactorKey)
			if !firstOK || !secondOK || firstFactor.Category != "indicator" || secondFactor.Category != "indicator" {
				return issue(path+".secondFactor.factorKey", "unsupported_factor", "second factor comparisons require two indicators")
			}
			copy := *condition.SecondFactor
			copy.Params = normalizeFactorParams(copy)
			copy.InstanceID = normalizedInstanceID(copy.InstanceID, copy, index+1)
			condition.SecondFactor = &copy
		}
	}
	return nil
}

func normalizeDefinitionColumns(def *broker.ScreenDefinitionV2) error {
	seenColumns := make(map[string]struct{}, len(def.Columns))
	seenColumnFactors := make(map[string]struct{}, len(def.Columns))
	for index := range def.Columns {
		column := &def.Columns[index]
		path := fmt.Sprintf("columns[%d]", index)
		column.ID = strings.TrimSpace(column.ID)
		if column.ID == "" {
			column.ID = fmt.Sprintf("column-%d", index+1)
		}
		if _, exists := seenColumns[column.ID]; exists {
			return issue(path+".id", "duplicate", "column id must be unique")
		}
		seenColumns[column.ID] = struct{}{}
		if err := validateRef(path+".factor", column.Factor, def.Market, false, true, false); err != nil {
			return err
		}
		column.Factor.Params = normalizeFactorParams(column.Factor)
		column.Factor.InstanceID = normalizedInstanceID(column.Factor.InstanceID, column.Factor, index)
		configurationKey := factorConfigurationKey(column.Factor)
		if _, exists := seenColumnFactors[configurationKey]; exists {
			return issue(path+".factor", "duplicate_factor", "the same factor and parameters already exist")
		}
		seenColumnFactors[configurationKey] = struct{}{}
	}
	return nil
}

func normalizeDefinitionSorts(def *broker.ScreenDefinitionV2) error {
	for index := range def.Sorts {
		sortValue := &def.Sorts[index]
		path := fmt.Sprintf("sorts[%d]", index)
		sortValue.ID = strings.TrimSpace(sortValue.ID)
		if sortValue.ID == "" {
			sortValue.ID = fmt.Sprintf("sort-%d", index+1)
		}
		sortValue.Direction = strings.ToLower(strings.TrimSpace(sortValue.Direction))
		if sortValue.Direction == "" {
			sortValue.Direction = "desc"
		}
		switch sortValue.Direction {
		case "asc", "desc", "abs_asc", "abs_desc":
		default:
			return issue(path+".direction", "invalid_operator", "must be asc, desc, abs_asc or abs_desc")
		}
		if err := validateRef(path+".factor", sortValue.Factor, def.Market, false, false, true); err != nil {
			return err
		}
		sortValue.Factor.Params = normalizeFactorParams(sortValue.Factor)
		sortValue.Factor.InstanceID = normalizedInstanceID(sortValue.Factor.InstanceID, sortValue.Factor, index)
	}
	return nil
}

func validateRef(path string, ref broker.FactorRef, market string, filter, retrieve, sort bool) error {
	ref.FactorKey = strings.ToLower(strings.TrimSpace(ref.FactorKey))
	if ref.FactorKey == "" {
		return issue(path+".factorKey", "required", "factor key is required")
	}
	if _, err := ValidateFactorForMarket(ref.FactorKey, market, filter, retrieve, sort); err != nil {
		return issue(path+".factorKey", "unsupported_factor", err.Error())
	}
	if err := validateParams(path+".params", ref); err != nil {
		return err
	}
	return nil
}

func validateParams(path string, ref broker.FactorRef) error {
	factor, ok := Lookup(ref.FactorKey)
	if !ok {
		return issue(path, "unsupported_factor", "unknown factor")
	}
	values := typedParamsMap(ref.Params)
	for _, parameter := range factor.Parameters {
		if parameter.Type == "union" {
			if err := validateUnionParameter(path+"."+parameter.Name, parameter.Name, values); err != nil {
				return err
			}
			continue
		}
		value, exists := values[parameter.Name]
		if !exists || isMissingJSONValue(value) {
			if parameter.Required && parameter.Default == nil {
				return issue(path+"."+parameter.Name, "required", "parameter is required")
			}
			continue
		}
		if err := validateParameterValue(path+"."+parameter.Name, parameter, value); err != nil {
			return err
		}
	}
	return nil
}

func validateParameterValue(path string, parameter ParameterDescriptor, value any) error {
	if parameter.Type == "string" {
		if _, ok := value.(string); !ok {
			return issue(path, "invalid_type", "must be a string")
		}
		return nil
	}
	values := []any{value}
	if parameter.Type == "integer_array" || parameter.Type == "number_array" {
		array, ok := value.([]any)
		if !ok {
			return issue(path, "invalid_type", "must be an array")
		}
		values = array
	}
	for _, item := range values {
		number, ok := numericJSONValue(item)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			return issue(path, "invalid_type", "must contain only finite numbers")
		}
		if (parameter.Type == "integer" || parameter.Type == "integer_array" || parameter.Enum != "") &&
			number != math.Trunc(number) {
			return issue(path, "invalid_type", "must contain only integers")
		}
		if parameter.Minimum != nil && number < float64(*parameter.Minimum) {
			return issue(path, "minimum", fmt.Sprintf("must be at least %d", *parameter.Minimum))
		}
		if parameter.Maximum != nil && number > float64(*parameter.Maximum) {
			return issue(path, "maximum", fmt.Sprintf("must be at most %d", *parameter.Maximum))
		}
		if parameter.Step != nil && *parameter.Step > 0 {
			base := 0.0
			if parameter.Minimum != nil {
				base = float64(*parameter.Minimum)
			}
			remainder := math.Mod(math.Abs(number-base), *parameter.Step)
			if remainder > 1e-9 && math.Abs(remainder-*parameter.Step) > 1e-9 {
				return issue(path, "step", fmt.Sprintf("must use step %g", *parameter.Step))
			}
		}
		if parameter.Enum != "" && !enumContains(parameter.Enum, int64(number)) {
			return issue(path, "invalid_enum", fmt.Sprintf("must be a valid %s value", parameter.Enum))
		}
	}
	return nil
}

func validateUnionParameter(path, name string, values map[string]any) error {
	if name != "optionParam" {
		return issue(path, "unsupported_union", "unsupported union parameter")
	}
	rawType, hasType := values["optionParamType"]
	_, hasString := values["optionParamString"]
	_, hasInteger := values["optionParamInteger"]
	_, hasIntegers := values["optionParamIntegers"]
	if !hasType {
		if hasString || hasInteger || hasIntegers {
			return issue(path+".type", "required", "union type is required")
		}
		return nil
	}
	parameterType, ok := numericJSONValue(rawType)
	if !ok || parameterType != math.Trunc(parameterType) {
		return issue(path+".type", "invalid_type", "union type must be an integer")
	}
	switch int64(parameterType) {
	case 1:
		if value, ok := values["optionParamString"].(string); !ok || strings.TrimSpace(value) == "" {
			return issue(path+".string", "required", "string value is required for union type 1")
		}
	case 2:
		if _, ok := numericJSONValue(values["optionParamInteger"]); !ok {
			return issue(path+".integer", "required", "integer value is required for union type 2")
		}
	case 3:
		array, ok := values["optionParamIntegers"].([]any)
		if !ok || len(array) == 0 {
			return issue(path+".integers", "required", "integer array is required for union type 3")
		}
		for _, item := range array {
			number, ok := numericJSONValue(item)
			if !ok || number != math.Trunc(number) {
				return issue(path+".integers", "invalid_type", "integer array must contain only integers")
			}
		}
	default:
		return issue(path+".type", "invalid_enum", "union type must be 1, 2 or 3")
	}
	return nil
}

func enumContains(name string, value int64) bool {
	for _, option := range generatedEnums[name] {
		if option.Value == value {
			return true
		}
	}
	return false
}

// normalizeFactorParams fills catalog defaults before serialization. The
// adapter therefore receives the same explicit values the editor displayed,
// while omitted optional fields remain omitted.
func normalizeFactorParams(ref broker.FactorRef) broker.ResearchScreenFactorParams {
	factor, ok := Lookup(ref.FactorKey)
	if !ok {
		return ref.Params
	}
	content, _ := json.Marshal(ref.Params)
	values := map[string]any{}
	_ = json.Unmarshal(content, &values)
	for _, parameter := range factor.Parameters {
		if _, exists := values[parameter.Name]; exists || parameter.Default == nil {
			continue
		}
		values[parameter.Name] = parameter.Default
	}
	content, _ = json.Marshal(values)
	var result broker.ResearchScreenFactorParams
	_ = json.Unmarshal(content, &result)
	return result
}

func validateConditionValue(path string, condition *broker.ScreenCondition) error {
	if err := validateConditionOperator(path, condition); err != nil {
		return err
	}
	if condition.Value == nil {
		return issue(path+".value", "required", "condition value is required")
	}
	switch condition.Operator {
	case "in":
		return validateSetConditionValue(path, condition.Value)
	case "between":
		return validateBetweenConditionValue(path, condition.Value)
	case "position":
		return validatePositionConditionValue(path, condition)
	case "pattern":
		return validatePatternConditionValue(path, condition.Value)
	default:
		return nil
	}
}

func validateConditionOperator(path string, condition *broker.ScreenCondition) error {
	switch condition.Operator {
	case "", "is", "eq", "ne", "gt", "gte", "lt", "lte", "between", "in", "contains", "crosses", "position", "pattern":
	default:
		return issue(path+".operator", "unsupported_operator", "unsupported condition operator")
	}
	if factor, ok := Lookup(condition.Factor.FactorKey); ok {
		switch factor.FilterKind {
		case "enum", "set":
			if condition.Operator != "in" && condition.Operator != "eq" && condition.Operator != "is" {
				return issue(path+".operator", "unsupported_operator", "set factor requires in, eq or is")
			}
		case "interval":
			if condition.Operator != "between" {
				return issue(path+".operator", "unsupported_operator", "interval factor requires between")
			}
		case "interval_or_set":
			if condition.Operator != "between" && condition.Operator != "in" {
				return issue(path+".operator", "unsupported_operator", "factor requires an interval or a value set")
			}
		case "position":
			if condition.Operator != "position" {
				return issue(path+".operator", "unsupported_operator", "indicator factor requires position")
			}
		case "pattern":
			if condition.Operator != "pattern" {
				return issue(path+".operator", "unsupported_operator", "pattern factor requires pattern")
			}
		}
	}
	return nil
}

func validateSetConditionValue(path string, value any) error {
	values, ok := numericSlice(value)
	if !ok || len(values) == 0 {
		return issue(path+".value", "invalid_set", "value set must contain at least one integer")
	}
	return nil
}

func numericSlice(value any) ([]int64, bool) {
	values, ok := value.([]any)
	if !ok {
		if typed, typedOK := value.([]int64); typedOK {
			return append([]int64(nil), typed...), true
		}
		return nil, false
	}
	result := make([]int64, len(values))
	for index, item := range values {
		number, itemOK := numericJSONValue(item)
		if !itemOK {
			return nil, false
		}
		result[index] = int64(number)
	}
	return result, true
}

func typedParamsMap(params broker.ResearchScreenFactorParams) map[string]any {
	content, _ := json.Marshal(params)
	result := map[string]any{}
	_ = json.Unmarshal(content, &result)
	return result
}

func validateBetweenConditionValue(path string, value any) error {
	rangeValue, ok := value.(map[string]any)
	if !ok {
		return issue(path+".value", "invalid_range", "between requires an object with min/max")
	}
	_, hasMinimum := numericJSONValue(rangeValue["min"])
	_, hasMaximum := numericJSONValue(rangeValue["max"])
	if hasMinimum || hasMaximum {
		if err := validateRangeValue(path+".value", rangeValue); err != nil {
			return err
		}
	}
	hasIntervals := false
	if rawIntervals, exists := rangeValue["intervals"]; exists {
		intervals, ok := rawIntervals.([]any)
		if !ok {
			return issue(path+".value.intervals", "invalid_range", "intervals must be an array")
		}
		hasIntervals = len(intervals) > 0
		for index, raw := range intervals {
			interval, ok := raw.(map[string]any)
			if !ok {
				return issue(fmt.Sprintf("%s.value.intervals[%d]", path, index), "invalid_range", "interval must be an object")
			}
			if err := validateRangeValue(fmt.Sprintf("%s.value.intervals[%d]", path, index), interval); err != nil {
				return err
			}
		}
	}
	if !hasMinimum && !hasMaximum && !hasIntervals {
		return issue(path+".value", "invalid_range", "at least one of min, max or intervals is required")
	}
	return nil
}

func validatePositionConditionValue(path string, condition *broker.ScreenCondition) error {
	value, ok := condition.Value.(map[string]any)
	if !ok {
		return issue(path+".value", "invalid_position", "position requires an object")
	}
	position, ok := numericJSONValue(value["position"])
	if !ok || position != math.Trunc(position) || position < 1 || position > 4 {
		return issue(path+".value.position", "invalid_position", "position must be an integer from 1 to 4")
	}
	if condition.SecondFactor == nil {
		if _, ok := numericJSONValue(value["secondValue"]); !ok {
			return issue(path+".value.secondValue", "required", "secondValue or secondFactor is required")
		}
	}
	return nil
}

func validatePatternConditionValue(path string, value any) error {
	patternValue, ok := value.(map[string]any)
	if !ok {
		return issue(path+".value", "invalid_pattern", "pattern requires an object")
	}
	if raw, exists := patternValue["match"]; exists {
		if _, ok := raw.(bool); !ok {
			return issue(path+".value.match", "invalid_pattern", "match must be boolean")
		}
	}
	return nil
}

func validateRangeValue(path string, value map[string]any) error {
	minimum, minOK := numericJSONValue(value["min"])
	maximum, maxOK := numericJSONValue(value["max"])
	if !minOK && !maxOK {
		return issue(path, "invalid_range", "at least one of min or max is required")
	}
	if (minOK && math.IsNaN(minimum)) || (maxOK && math.IsNaN(maximum)) ||
		(minOK && maxOK && minimum > maximum) {
		return issue(path, "invalid_range", "min must not exceed max")
	}
	return nil
}

func inferOperator(value any) string {
	if _, ok := value.(map[string]any); ok {
		return "between"
	}
	if _, ok := value.([]any); ok {
		return "in"
	}
	return "eq"
}

func normalizedInstanceID(value string, ref broker.FactorRef, index int) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return stableInstanceID("factor", ref, index)
}

func stableInstanceID(prefix string, ref broker.FactorRef, index int) string {
	content, _ := json.Marshal(struct {
		Key    string                            `json:"key"`
		Params broker.ResearchScreenFactorParams `json:"params"`
	}{ref.FactorKey, ref.Params})
	hash := sha256.Sum256(append(content, []byte(strconv.Itoa(index))...))
	return prefix + "-" + strings.Trim(strings.ToLower(ref.FactorKey), ".") + "-" + hex.EncodeToString(hash[:])[:10]
}

func factorConfigurationKey(ref broker.FactorRef) string {
	content, _ := json.Marshal(ref.Params)
	return strings.ToLower(strings.TrimSpace(ref.FactorKey)) + ":" + string(content)
}

func cleanIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isMissingJSONValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}

func numericJSONValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
