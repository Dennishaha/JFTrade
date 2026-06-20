package pineruntime

import (
	"fmt"
	"math"
	"sort"
	"strings"

	exprast "github.com/expr-lang/expr/ast"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

const maxPineCollectionElements = 100000
const maxPineMapPairs = 50000

type pineArray struct {
	elementType string
	values      []any
}

type pineMap struct {
	keyType   string
	valueType string
	values    map[any]any
}

type pineMatrix struct {
	elementType string
	rows        int
	columns     int
	values      []any
}

func (r *strategyRuntime) executeCollectionStatement(statement *strategyir.CollectionStmt, scope *evaluationScope) error {
	if statement == nil {
		return nil
	}
	if statement.Mode == strategyir.AssignmentModeVar && statement.ResultName != "" && r != nil && r.persistentValues != nil {
		if value, ok := r.persistentValues[statement.ResultName]; ok {
			scope.setVariable(statement.ResultName, value)
			return nil
		}
	}
	arguments := make([]any, 0, len(statement.Arguments))
	for _, expression := range statement.Arguments {
		value, err := evaluateExpression(expression, scope)
		if err != nil {
			return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
		}
		arguments = append(arguments, collectionScalarValue(value))
	}
	var (
		value any
		err   error
	)
	if collectionRuntimeConstructorOperation(statement.Namespace, statement.Operation) {
		value, err = constructPineCollection(statement, arguments)
	} else {
		target, ok := scope.variable(statement.Target)
		if !ok {
			var evalErr error
			target, evalErr = evaluateExpression(statement.Target, scope)
			if evalErr != nil {
				return fmt.Errorf("pine line %d: unknown collection %q", statement.Range.StartLine, statement.Target)
			}
		}
		value, err = executePineCollectionOperation(statement.Namespace, statement.Operation, target, arguments)
	}
	if err != nil {
		return fmt.Errorf("pine line %d: %w", statement.Range.StartLine, err)
	}
	if statement.ResultName == "" {
		return nil
	}
	switch statement.Mode {
	case strategyir.AssignmentModeVar:
		if r != nil && r.persistentValues != nil {
			r.persistentValues[statement.ResultName] = value
		}
		scope.setVariable(statement.ResultName, value)
	case strategyir.AssignmentModeReassign:
		if r != nil && r.persistentValues != nil {
			if _, ok := r.persistentValues[statement.ResultName]; ok {
				r.persistentValues[statement.ResultName] = value
			}
		}
		scope.assignVariable(statement.ResultName, value)
	default:
		scope.setVariable(statement.ResultName, value)
	}
	return nil
}

func constructPineCollection(statement *strategyir.CollectionStmt, arguments []any) (any, error) {
	switch statement.Namespace {
	case "array":
		if statement.Operation == "from" {
			values := append([]any(nil), arguments...)
			if len(values) > maxPineCollectionElements {
				return nil, fmt.Errorf("array size exceeds limit %d", maxPineCollectionElements)
			}
			return &pineArray{elementType: collectionElementType(statement.Operation, statement.TypeArgs), values: values}, nil
		}
		elementType := collectionElementType(statement.Operation, statement.TypeArgs)
		size := 0
		if len(arguments) > 0 {
			parsed, err := collectionIndex(arguments[0])
			if err != nil {
				return nil, fmt.Errorf("array size: %w", err)
			}
			size = parsed
		}
		if size > maxPineCollectionElements {
			return nil, fmt.Errorf("array size %d exceeds limit %d", size, maxPineCollectionElements)
		}
		initial := any(nil)
		if len(arguments) > 1 {
			initial = arguments[1]
		}
		if err := validateCollectionValue(elementType, initial); err != nil {
			return nil, err
		}
		values := make([]any, size)
		for index := range values {
			values[index] = initial
		}
		return &pineArray{elementType: elementType, values: values}, nil
	case "map":
		keyType, valueType := collectionMapTypes(statement.TypeArgs)
		return &pineMap{keyType: keyType, valueType: valueType, values: map[any]any{}}, nil
	case "matrix":
		if len(arguments) < 2 {
			return nil, fmt.Errorf("matrix.new requires rows and columns")
		}
		rows, err := collectionIndex(arguments[0])
		if err != nil {
			return nil, fmt.Errorf("matrix rows: %w", err)
		}
		columns, err := collectionIndex(arguments[1])
		if err != nil {
			return nil, fmt.Errorf("matrix columns: %w", err)
		}
		if rows > 0 && columns > maxPineCollectionElements/rows {
			return nil, fmt.Errorf("matrix size exceeds limit %d", maxPineCollectionElements)
		}
		initial := any(nil)
		if len(arguments) > 2 {
			initial = arguments[2]
		}
		elementType := strings.TrimSpace(statement.TypeArgs)
		if err := validateCollectionValue(elementType, initial); err != nil {
			return nil, err
		}
		values := make([]any, rows*columns)
		for index := range values {
			values[index] = initial
		}
		return &pineMatrix{elementType: elementType, rows: rows, columns: columns, values: values}, nil
	default:
		return nil, fmt.Errorf("unsupported collection namespace %q", statement.Namespace)
	}
}

func collectionRuntimeConstructorOperation(namespace string, operation string) bool {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if strings.HasPrefix(operation, "new") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(namespace), "array") && operation == "from"
}

func executePineCollectionOperation(namespace string, operation string, target any, arguments []any) (any, error) {
	switch namespace {
	case "array":
		array, ok := target.(*pineArray)
		if !ok || array == nil {
			return nil, fmt.Errorf("array operation requires an array")
		}
		return array.operation(operation, arguments)
	case "map":
		values, ok := target.(*pineMap)
		if !ok || values == nil {
			return nil, fmt.Errorf("map operation requires a map")
		}
		return values.operation(operation, arguments)
	case "matrix":
		matrix, ok := target.(*pineMatrix)
		if !ok || matrix == nil {
			return nil, fmt.Errorf("matrix operation requires a matrix")
		}
		return matrix.operation(operation, arguments)
	default:
		return nil, fmt.Errorf("unsupported collection namespace %q", namespace)
	}
}

func (a *pineArray) operation(operation string, arguments []any) (any, error) {
	switch operation {
	case "size":
		return float64(len(a.values)), nil
	case "clear":
		a.values = a.values[:0]
		return nil, nil
	case "copy":
		values := append([]any(nil), a.values...)
		return &pineArray{elementType: a.elementType, values: values}, nil
	case "slice":
		if len(arguments) < 2 {
			return nil, fmt.Errorf("array.slice requires from and to")
		}
		from, err := collectionIndex(arguments[0])
		if err != nil {
			return nil, err
		}
		to, err := collectionIndex(arguments[1])
		if err != nil {
			return nil, err
		}
		if from > to || to > len(a.values) {
			return nil, fmt.Errorf("array slice [%d,%d) is out of bounds for size %d", from, to, len(a.values))
		}
		return &pineArray{elementType: a.elementType, values: append([]any(nil), a.values[from:to]...)}, nil
	case "reverse":
		for left, right := 0, len(a.values)-1; left < right; left, right = left+1, right-1 {
			a.values[left], a.values[right] = a.values[right], a.values[left]
		}
		return nil, nil
	case "fill":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.fill requires a value")
		}
		if err := validateCollectionValue(a.elementType, arguments[0]); err != nil {
			return nil, err
		}
		from, to, err := collectionRange(arguments, 1, len(a.values))
		if err != nil {
			return nil, err
		}
		for index := from; index < to; index++ {
			a.values[index] = arguments[0]
		}
		return nil, nil
	case "concat":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.concat requires an array")
		}
		other, ok := arguments[0].(*pineArray)
		if !ok || other == nil {
			return nil, fmt.Errorf("array.concat requires an array")
		}
		if len(a.values)+len(other.values) > maxPineCollectionElements {
			return nil, fmt.Errorf("array size exceeds limit %d", maxPineCollectionElements)
		}
		if err := validateCollectionValues(a.elementType, other.values); err != nil {
			return nil, err
		}
		a.values = append(a.values, other.values...)
		return a, nil
	case "join":
		separator := ""
		if len(arguments) > 0 {
			separator = fmt.Sprintf("%v", arguments[0])
		}
		parts := make([]string, len(a.values))
		for index, value := range a.values {
			if value == nil {
				parts[index] = "na"
			} else {
				parts[index] = fmt.Sprintf("%v", value)
			}
		}
		return strings.Join(parts, separator), nil
	case "sort":
		descending := collectionSortDescending(arguments)
		return nil, sortCollectionValues(a.values, descending)
	case "sort_indices":
		descending := collectionSortDescending(arguments)
		indices := make([]int, len(a.values))
		for index := range indices {
			indices[index] = index
		}
		if err := validateSortableCollectionValues(a.values); err != nil {
			return nil, err
		}
		sort.SliceStable(indices, func(left, right int) bool {
			return collectionSortLess(a.values[indices[left]], a.values[indices[right]], descending)
		})
		values := make([]any, len(indices))
		for index, value := range indices {
			values[index] = float64(value)
		}
		return &pineArray{elementType: "int", values: values}, nil
	case "binary_search":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.binary_search requires a value")
		}
		index, ok, err := a.binarySearchIndex(arguments[0], "any")
		if err != nil {
			return nil, err
		}
		if ok {
			return float64(index), nil
		}
		return float64(-1), nil
	case "binary_search_leftmost", "binary_search_rightmost":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.%s requires a value", operation)
		}
		side := "left"
		if operation == "binary_search_rightmost" {
			side = "right"
		}
		index, ok, err := a.binarySearchIndex(arguments[0], side)
		if err != nil {
			return nil, err
		}
		if ok {
			return float64(index), nil
		}
		return float64(-1), nil
	case "abs":
		values := make([]any, len(a.values))
		for index, value := range a.values {
			numeric, ok := coerceFloatValue(value)
			if !ok {
				return nil, fmt.Errorf("array.abs requires numeric values")
			}
			values[index] = math.Abs(numeric)
		}
		return &pineArray{elementType: a.elementType, values: values}, nil
	case "percentrank":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.percentrank requires an index")
		}
		index, err := collectionIndex(arguments[0])
		if err != nil {
			return nil, err
		}
		return a.percentRank(index)
	case "percentile_nearest_rank", "percentile_linear_interpolation":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.%s requires a percentage", operation)
		}
		percentage, ok := coerceFloatValue(arguments[0])
		if !ok {
			return nil, fmt.Errorf("array.%s percentage must be numeric", operation)
		}
		return a.percentile(percentage, operation == "percentile_linear_interpolation")
	case "stdev", "variance":
		return a.varianceOrStdev(operation == "stdev")
	case "covariance":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.covariance requires another array")
		}
		other, ok := arguments[0].(*pineArray)
		if !ok || other == nil {
			return nil, fmt.Errorf("array.covariance requires another array")
		}
		return a.covariance(other)
	case "get":
		index, err := requiredCollectionIndex(arguments, 0)
		if err != nil {
			return nil, err
		}
		return a.at(index)
	case "set":
		index, err := requiredCollectionIndex(arguments, 0)
		if err != nil {
			return nil, err
		}
		if len(arguments) < 2 {
			return nil, fmt.Errorf("array.set requires a value")
		}
		if _, err := a.at(index); err != nil {
			return nil, err
		}
		if err := validateCollectionValue(a.elementType, arguments[1]); err != nil {
			return nil, err
		}
		a.values[index] = arguments[1]
		return nil, nil
	case "push":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.push requires a value")
		}
		if len(a.values) >= maxPineCollectionElements {
			return nil, fmt.Errorf("array size exceeds limit %d", maxPineCollectionElements)
		}
		if err := validateCollectionValue(a.elementType, arguments[0]); err != nil {
			return nil, err
		}
		a.values = append(a.values, arguments[0])
		return nil, nil
	case "pop":
		if len(a.values) == 0 {
			return nil, fmt.Errorf("array.pop cannot read an empty array")
		}
		last := len(a.values) - 1
		value := a.values[last]
		a.values = a.values[:last]
		return value, nil
	case "shift":
		if len(a.values) == 0 {
			return nil, fmt.Errorf("array.shift cannot read an empty array")
		}
		value := a.values[0]
		a.values = append(a.values[:0], a.values[1:]...)
		return value, nil
	case "unshift":
		return a.insert(0, arguments)
	case "insert":
		if len(arguments) < 2 {
			return nil, fmt.Errorf("array.insert requires index and value")
		}
		index, err := collectionIndex(arguments[0])
		if err != nil {
			return nil, err
		}
		return a.insert(index, arguments[1:])
	case "remove":
		index, err := requiredCollectionIndex(arguments, 0)
		if err != nil {
			return nil, err
		}
		value, err := a.at(index)
		if err != nil {
			return nil, err
		}
		a.values = append(a.values[:index], a.values[index+1:]...)
		return value, nil
	case "first":
		return a.at(0)
	case "last":
		return a.at(len(a.values) - 1)
	case "includes":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.includes requires a value")
		}
		return a.indexOf(arguments[0], false) >= 0, nil
	case "indexof":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.indexof requires a value")
		}
		return float64(a.indexOf(arguments[0], false)), nil
	case "lastindexof":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("array.lastindexof requires a value")
		}
		return float64(a.indexOf(arguments[0], true)), nil
	case "min", "max", "avg", "sum", "median", "mode", "range":
		return a.aggregate(operation)
	default:
		return nil, fmt.Errorf("unsupported array operation %q", operation)
	}
}

func (a *pineArray) indexOf(value any, reverse bool) int {
	if reverse {
		for index := len(a.values) - 1; index >= 0; index-- {
			if collectionValuesEqual(a.values[index], value) {
				return index
			}
		}
		return -1
	}
	for index, current := range a.values {
		if collectionValuesEqual(current, value) {
			return index
		}
	}
	return -1
}

func (a *pineArray) aggregate(operation string) (any, error) {
	if len(a.values) == 0 {
		if operation == "sum" {
			return float64(0), nil
		}
		return nil, nil
	}
	total := 0.0
	minimum := 0.0
	maximum := 0.0
	for index, value := range a.values {
		numeric, ok := coerceFloatValue(value)
		if !ok {
			return nil, fmt.Errorf("array.%s requires numeric values", operation)
		}
		if index == 0 || numeric < minimum {
			minimum = numeric
		}
		if index == 0 || numeric > maximum {
			maximum = numeric
		}
		total += numeric
	}
	switch operation {
	case "min":
		return minimum, nil
	case "max":
		return maximum, nil
	case "avg":
		return total / float64(len(a.values)), nil
	case "median":
		numbers := make([]float64, len(a.values))
		for index, value := range a.values {
			numeric, _ := coerceFloatValue(value)
			numbers[index] = numeric
		}
		sort.Float64s(numbers)
		mid := len(numbers) / 2
		if len(numbers)%2 == 1 {
			return numbers[mid], nil
		}
		return (numbers[mid-1] + numbers[mid]) / 2, nil
	case "mode":
		counts := map[float64]int{}
		best := 0.0
		bestCount := 0
		for _, value := range a.values {
			numeric, _ := coerceFloatValue(value)
			counts[numeric]++
			if counts[numeric] > bestCount || (counts[numeric] == bestCount && numeric < best) {
				best = numeric
				bestCount = counts[numeric]
			}
		}
		return best, nil
	case "range":
		return maximum - minimum, nil
	default:
		return total, nil
	}
}

func (a *pineArray) binarySearchIndex(value any, side string) (int, bool, error) {
	if err := validateSortableCollectionValues(a.values); err != nil {
		return 0, false, err
	}
	if err := validateSortableCollectionValues([]any{value}); err != nil {
		return 0, false, err
	}
	switch side {
	case "left":
		index := sort.Search(len(a.values), func(index int) bool {
			return compareCollectionValues(a.values[index], value) >= 0
		})
		if index < len(a.values) && compareCollectionValues(a.values[index], value) == 0 {
			return index, true, nil
		}
	case "right":
		index := sort.Search(len(a.values), func(index int) bool {
			return compareCollectionValues(a.values[index], value) > 0
		}) - 1
		if index >= 0 && compareCollectionValues(a.values[index], value) == 0 {
			return index, true, nil
		}
	default:
		index := sort.Search(len(a.values), func(index int) bool {
			return compareCollectionValues(a.values[index], value) >= 0
		})
		if index < len(a.values) && compareCollectionValues(a.values[index], value) == 0 {
			return index, true, nil
		}
	}
	return -1, false, nil
}

func (a *pineArray) numericValues(operation string) ([]float64, error) {
	values := make([]float64, len(a.values))
	for index, value := range a.values {
		numeric, ok := coerceFloatValue(value)
		if !ok {
			return nil, fmt.Errorf("array.%s requires numeric values", operation)
		}
		values[index] = numeric
	}
	return values, nil
}

func (a *pineArray) percentRank(index int) (any, error) {
	values, err := a.numericValues("percentrank")
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	if index < 0 || index >= len(values) {
		return nil, fmt.Errorf("array index %d is out of bounds for size %d", index, len(values))
	}
	if len(values) == 1 {
		return float64(100), nil
	}
	current := values[index]
	lessOrEqual := 0
	for _, value := range values {
		if value <= current {
			lessOrEqual++
		}
	}
	return float64(lessOrEqual-1) / float64(len(values)-1) * 100, nil
}

func (a *pineArray) percentile(percentage float64, linear bool) (any, error) {
	values, err := a.numericValues("percentile")
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	if percentage < 0 || percentage > 100 {
		return nil, fmt.Errorf("array percentile must be between 0 and 100")
	}
	sort.Float64s(values)
	if len(values) == 1 {
		return values[0], nil
	}
	if !linear {
		rank := int(math.Ceil(percentage / 100 * float64(len(values))))
		if rank < 1 {
			rank = 1
		}
		if rank > len(values) {
			rank = len(values)
		}
		return values[rank-1], nil
	}
	position := percentage / 100 * float64(len(values)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return values[lower], nil
	}
	weight := position - float64(lower)
	return values[lower] + (values[upper]-values[lower])*weight, nil
}

func (a *pineArray) varianceOrStdev(stdev bool) (any, error) {
	values, err := a.numericValues("variance")
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}
	mean := 0.0
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))
	variance := 0.0
	for _, value := range values {
		delta := value - mean
		variance += delta * delta
	}
	variance /= float64(len(values))
	if stdev {
		return math.Sqrt(variance), nil
	}
	return variance, nil
}

func (a *pineArray) covariance(other *pineArray) (any, error) {
	left, err := a.numericValues("covariance")
	if err != nil {
		return nil, err
	}
	right, err := other.numericValues("covariance")
	if err != nil {
		return nil, err
	}
	if len(left) == 0 || len(right) == 0 {
		return nil, nil
	}
	if len(left) != len(right) {
		return nil, fmt.Errorf("array.covariance requires arrays with the same size")
	}
	leftMean := 0.0
	rightMean := 0.0
	for index := range left {
		leftMean += left[index]
		rightMean += right[index]
	}
	leftMean /= float64(len(left))
	rightMean /= float64(len(right))
	covariance := 0.0
	for index := range left {
		covariance += (left[index] - leftMean) * (right[index] - rightMean)
	}
	return covariance / float64(len(left)), nil
}

func (a *pineArray) insert(index int, arguments []any) (any, error) {
	if len(arguments) < 1 {
		return nil, fmt.Errorf("array insertion requires a value")
	}
	if index < 0 || index > len(a.values) {
		return nil, fmt.Errorf("array index %d is out of bounds for size %d", index, len(a.values))
	}
	if len(a.values) >= maxPineCollectionElements {
		return nil, fmt.Errorf("array size exceeds limit %d", maxPineCollectionElements)
	}
	if err := validateCollectionValue(a.elementType, arguments[0]); err != nil {
		return nil, err
	}
	a.values = append(a.values, nil)
	copy(a.values[index+1:], a.values[index:])
	a.values[index] = arguments[0]
	return nil, nil
}

func (a *pineArray) at(index int) (any, error) {
	if index < 0 || index >= len(a.values) {
		return nil, fmt.Errorf("array index %d is out of bounds for size %d", index, len(a.values))
	}
	return a.values[index], nil
}

func (m *pineMap) operation(operation string, arguments []any) (any, error) {
	switch operation {
	case "size":
		return float64(len(m.values)), nil
	case "clear":
		clear(m.values)
		return nil, nil
	case "copy":
		values := make(map[any]any, len(m.values))
		for key, value := range m.values {
			values[key] = value
		}
		return &pineMap{keyType: m.keyType, valueType: m.valueType, values: values}, nil
	case "keys", "values":
		keys := sortedMapKeys(m.values)
		if operation == "keys" {
			values := make([]any, len(keys))
			copy(values, keys)
			return &pineArray{elementType: m.keyType, values: values}, nil
		}
		values := make([]any, len(keys))
		for index, key := range keys {
			values[index] = m.values[key]
		}
		return &pineArray{elementType: m.valueType, values: values}, nil
	case "get", "contains", "remove":
		key, err := m.key(arguments)
		if err != nil {
			return nil, err
		}
		value, exists := m.values[key]
		switch operation {
		case "contains":
			return exists, nil
		case "remove":
			delete(m.values, key)
		}
		return value, nil
	case "put":
		key, err := m.key(arguments)
		if err != nil {
			return nil, err
		}
		if len(arguments) < 2 {
			return nil, fmt.Errorf("map.put requires a value")
		}
		if _, exists := m.values[key]; !exists && len(m.values) >= maxPineMapPairs {
			return nil, fmt.Errorf("map size exceeds limit %d", maxPineMapPairs)
		}
		if err := validateCollectionValue(m.valueType, arguments[1]); err != nil {
			return nil, err
		}
		m.values[key] = arguments[1]
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported map operation %q", operation)
	}
}

func collectionSortDescending(arguments []any) bool {
	if len(arguments) == 0 {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", arguments[0])))
	return value == "descending" || value == "order.descending"
}

func sortCollectionValues(values []any, descending bool) error {
	if err := validateSortableCollectionValues(values); err != nil {
		return err
	}
	sort.SliceStable(values, func(left, right int) bool {
		return collectionSortLess(values[left], values[right], descending)
	})
	return nil
}

func validateSortableCollectionValues(values []any) error {
	for _, value := range values {
		switch value.(type) {
		case nil, bool, string, float64:
			continue
		default:
			if _, ok := coerceFloatValue(value); ok {
				continue
			}
			return fmt.Errorf("collection sort supports number, string, bool, and nil values only")
		}
	}
	return nil
}

func compareCollectionValues(left any, right any) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return 1
	}
	if right == nil {
		return -1
	}
	leftFloat, leftFloatOK := coerceFloatValue(left)
	rightFloat, rightFloatOK := coerceFloatValue(right)
	if leftFloatOK && rightFloatOK {
		switch {
		case leftFloat < rightFloat:
			return -1
		case leftFloat > rightFloat:
			return 1
		default:
			return 0
		}
	}
	leftString := collectionStableString(left)
	rightString := collectionStableString(right)
	switch {
	case leftString < rightString:
		return -1
	case leftString > rightString:
		return 1
	default:
		return 0
	}
}

func collectionSortLess(left any, right any, descending bool) bool {
	if left == nil && right == nil {
		return false
	}
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	cmp := compareCollectionValues(left, right)
	if descending {
		return cmp > 0
	}
	return cmp < 0
}

func sortedMapKeys(values map[any]any) []any {
	keys := make([]any, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(left, right int) bool {
		return collectionStableString(keys[left]) < collectionStableString(keys[right])
	})
	return keys
}

func collectionStableString(value any) string {
	if value == nil {
		return "4:nil:"
	}
	if numeric, ok := coerceFloatValue(value); ok {
		return fmt.Sprintf("1:number:%020.10f", numeric)
	}
	switch typed := value.(type) {
	case bool:
		if typed {
			return "2:bool:true"
		}
		return "2:bool:false"
	case string:
		return "3:string:" + typed
	default:
		return fmt.Sprintf("9:%T:%v", value, value)
	}
}

func (m *pineMap) key(arguments []any) (any, error) {
	if len(arguments) == 0 {
		return nil, fmt.Errorf("map operation requires a key")
	}
	if err := validateCollectionValue(m.keyType, arguments[0]); err != nil {
		return nil, err
	}
	switch typed := arguments[0].(type) {
	case nil, bool, string, float64:
		return typed, nil
	default:
		return nil, fmt.Errorf("map key type %T is not supported", arguments[0])
	}
}

func (m *pineMatrix) operation(operation string, arguments []any) (any, error) {
	switch operation {
	case "rows":
		return float64(m.rows), nil
	case "columns":
		return float64(m.columns), nil
	case "copy":
		return &pineMatrix{elementType: m.elementType, rows: m.rows, columns: m.columns, values: append([]any(nil), m.values...)}, nil
	case "fill":
		if len(arguments) < 1 {
			return nil, fmt.Errorf("matrix.fill requires a value")
		}
		if err := validateCollectionValue(m.elementType, arguments[0]); err != nil {
			return nil, err
		}
		for index := range m.values {
			m.values[index] = arguments[0]
		}
		return nil, nil
	case "reshape":
		if len(arguments) < 2 {
			return nil, fmt.Errorf("matrix.reshape requires rows and columns")
		}
		rows, err := collectionIndex(arguments[0])
		if err != nil {
			return nil, err
		}
		columns, err := collectionIndex(arguments[1])
		if err != nil {
			return nil, err
		}
		if rows*columns != len(m.values) {
			return nil, fmt.Errorf("matrix.reshape size %dx%d does not match existing %d cells", rows, columns, len(m.values))
		}
		m.rows = rows
		m.columns = columns
		return nil, nil
	case "add_row":
		return m.addRow(arguments)
	case "add_col":
		return m.addColumn(arguments)
	case "remove_row":
		return m.removeRow(arguments)
	case "remove_col":
		return m.removeColumn(arguments)
	case "get", "set":
		if len(arguments) < 2 {
			return nil, fmt.Errorf("matrix.%s requires row and column", operation)
		}
		row, err := collectionIndex(arguments[0])
		if err != nil {
			return nil, err
		}
		column, err := collectionIndex(arguments[1])
		if err != nil {
			return nil, err
		}
		if row < 0 || row >= m.rows || column < 0 || column >= m.columns {
			return nil, fmt.Errorf("matrix index [%d,%d] is out of bounds for %dx%d", row, column, m.rows, m.columns)
		}
		index := row*m.columns + column
		if operation == "get" {
			return m.values[index], nil
		}
		if len(arguments) < 3 {
			return nil, fmt.Errorf("matrix.set requires a value")
		}
		if err := validateCollectionValue(m.elementType, arguments[2]); err != nil {
			return nil, err
		}
		m.values[index] = arguments[2]
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported matrix operation %q", operation)
	}
}

func (m *pineMatrix) addRow(arguments []any) (any, error) {
	index, values, err := matrixInsertionArgs(arguments, m.rows, m.columns)
	if err != nil {
		return nil, err
	}
	if err := validateCollectionValues(m.elementType, values); err != nil {
		return nil, err
	}
	next := make([]any, 0, len(m.values)+m.columns)
	for row := 0; row <= m.rows; row++ {
		if row == index {
			next = append(next, values...)
		}
		if row < m.rows {
			start := row * m.columns
			next = append(next, m.values[start:start+m.columns]...)
		}
	}
	m.rows++
	m.values = next
	return nil, nil
}

func (m *pineMatrix) addColumn(arguments []any) (any, error) {
	index, values, err := matrixInsertionArgs(arguments, m.columns, m.rows)
	if err != nil {
		return nil, err
	}
	if err := validateCollectionValues(m.elementType, values); err != nil {
		return nil, err
	}
	next := make([]any, 0, len(m.values)+m.rows)
	for row := 0; row < m.rows; row++ {
		for column := 0; column <= m.columns; column++ {
			if column == index {
				next = append(next, values[row])
			}
			if column < m.columns {
				next = append(next, m.values[row*m.columns+column])
			}
		}
	}
	m.columns++
	m.values = next
	return nil, nil
}

func (m *pineMatrix) removeRow(arguments []any) (any, error) {
	index, err := requiredCollectionIndex(arguments, 0)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= m.rows {
		return nil, fmt.Errorf("matrix row %d is out of bounds for %d rows", index, m.rows)
	}
	removed := &pineArray{elementType: m.elementType, values: append([]any(nil), m.values[index*m.columns:index*m.columns+m.columns]...)}
	next := make([]any, 0, len(m.values)-m.columns)
	for row := 0; row < m.rows; row++ {
		if row == index {
			continue
		}
		start := row * m.columns
		next = append(next, m.values[start:start+m.columns]...)
	}
	m.rows--
	m.values = next
	return removed, nil
}

func (m *pineMatrix) removeColumn(arguments []any) (any, error) {
	index, err := requiredCollectionIndex(arguments, 0)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= m.columns {
		return nil, fmt.Errorf("matrix column %d is out of bounds for %d columns", index, m.columns)
	}
	removed := &pineArray{elementType: m.elementType, values: make([]any, 0, m.rows)}
	next := make([]any, 0, len(m.values)-m.rows)
	for row := 0; row < m.rows; row++ {
		for column := 0; column < m.columns; column++ {
			value := m.values[row*m.columns+column]
			if column == index {
				removed.values = append(removed.values, value)
				continue
			}
			next = append(next, value)
		}
	}
	m.columns--
	m.values = next
	return removed, nil
}

func requiredCollectionIndex(arguments []any, index int) (int, error) {
	if len(arguments) <= index {
		return 0, fmt.Errorf("collection operation requires an index")
	}
	return collectionIndex(arguments[index])
}

func collectionIndex(value any) (int, error) {
	numeric, ok := coerceFloatValue(value)
	if !ok || numeric < 0 || numeric != math.Trunc(numeric) {
		return 0, fmt.Errorf("collection index must be a non-negative integer")
	}
	return int(numeric), nil
}

func collectionRange(arguments []any, startIndex int, defaultEnd int) (int, int, error) {
	from := 0
	to := defaultEnd
	var err error
	if len(arguments) > startIndex {
		from, err = collectionIndex(arguments[startIndex])
		if err != nil {
			return 0, 0, err
		}
	}
	if len(arguments) > startIndex+1 {
		to, err = collectionIndex(arguments[startIndex+1])
		if err != nil {
			return 0, 0, err
		}
	}
	if from > to || to > defaultEnd {
		return 0, 0, fmt.Errorf("collection range [%d,%d) is out of bounds for size %d", from, to, defaultEnd)
	}
	return from, to, nil
}

func collectionValuesEqual(left any, right any) bool {
	leftFloat, leftFloatOK := coerceFloatValue(left)
	rightFloat, rightFloatOK := coerceFloatValue(right)
	if leftFloatOK && rightFloatOK {
		return leftFloat == rightFloat
	}
	return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right)
}

func matrixInsertionArgs(arguments []any, limit int, valueCount int) (int, []any, error) {
	index := limit
	valueArgIndex := 0
	if len(arguments) > 0 {
		if _, ok := arguments[0].(*pineArray); !ok {
			parsed, err := collectionIndex(arguments[0])
			if err != nil {
				return 0, nil, err
			}
			index = parsed
			valueArgIndex = 1
		}
	}
	if index < 0 || index > limit {
		return 0, nil, fmt.Errorf("matrix insertion index %d is out of bounds for size %d", index, limit)
	}
	values := make([]any, valueCount)
	if len(arguments) > valueArgIndex {
		array, ok := arguments[valueArgIndex].(*pineArray)
		if !ok {
			return 0, nil, fmt.Errorf("matrix insertion values must be an array")
		}
		if len(array.values) != valueCount {
			return 0, nil, fmt.Errorf("matrix insertion array size %d must equal %d", len(array.values), valueCount)
		}
		copy(values, array.values)
	}
	return index, values, nil
}

func collectionElementType(operation string, typeArgs string) string {
	if strings.TrimSpace(typeArgs) != "" {
		return strings.TrimSpace(typeArgs)
	}
	if strings.EqualFold(strings.TrimSpace(operation), "from") {
		return ""
	}
	return strings.TrimPrefix(operation, "new_")
}

func collectionMapTypes(typeArgs string) (string, string) {
	parts := strings.SplitN(typeArgs, ",", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func validateCollectionValue(typeName string, value any) error {
	if value == nil || strings.TrimSpace(typeName) == "" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(typeName)) {
	case "float":
		if _, ok := coerceFloatValue(value); ok {
			return nil
		}
	case "int":
		if numeric, ok := coerceFloatValue(value); ok && numeric == math.Trunc(numeric) {
			return nil
		}
	case "bool":
		if _, ok := value.(bool); ok {
			return nil
		}
	case "string", "color":
		if _, ok := value.(string); ok {
			return nil
		}
	default:
		lower := strings.ToLower(strings.TrimSpace(typeName))
		if (lower == "array" || strings.HasPrefix(lower, "array<")) && isPineArrayValue(value) {
			return nil
		}
		if (lower == "map" || strings.HasPrefix(lower, "map<")) && isPineMapValue(value) {
			return nil
		}
		if (lower == "matrix" || strings.HasPrefix(lower, "matrix<")) && isPineMatrixValue(value) {
			return nil
		}
		return fmt.Errorf("collection element type %q is not executable", typeName)
	}
	return fmt.Errorf("collection value %T does not match %s", value, typeName)
}

func isPineArrayValue(value any) bool {
	array, ok := value.(*pineArray)
	return ok && array != nil
}

func isPineMapValue(value any) bool {
	values, ok := value.(*pineMap)
	return ok && values != nil
}

func isPineMatrixValue(value any) bool {
	matrix, ok := value.(*pineMatrix)
	return ok && matrix != nil
}

func validateCollectionValues(typeName string, values []any) error {
	for _, value := range values {
		if err := validateCollectionValue(typeName, value); err != nil {
			return err
		}
	}
	return nil
}

func collectionScalarValue(value any) any {
	if numeric, ok := coerceFloatValue(value); ok {
		return numeric
	}
	return value
}

func evaluateCollectionReadExpression(functionName string, arguments []exprast.Node, scope *evaluationScope) (any, error) {
	if !strings.HasPrefix(functionName, "collection_") {
		return nil, fmt.Errorf("invalid collection function %q", functionName)
	}
	rest := strings.TrimPrefix(functionName, "collection_")
	dot := strings.Index(rest, "_")
	if dot <= 0 || dot == len(rest)-1 {
		return nil, fmt.Errorf("invalid collection function %q", functionName)
	}
	namespace := rest[:dot]
	operation := rest[dot+1:]
	if collectionRuntimeConstructorOperation(namespace, operation) {
		values := make([]any, 0, len(arguments))
		for _, argument := range arguments {
			value, err := evaluateAST(argument, scope)
			if err != nil {
				return nil, err
			}
			values = append(values, collectionScalarValue(value))
		}
		return constructPineCollection(&strategyir.CollectionStmt{Namespace: namespace, Operation: operation}, values)
	}
	if len(arguments) == 0 {
		return nil, fmt.Errorf("%s requires a collection", functionName)
	}
	target, err := evaluateAST(arguments[0], scope)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, nil
	}
	values := make([]any, 0, len(arguments)-1)
	for _, argument := range arguments[1:] {
		value, err := evaluateAST(argument, scope)
		if err != nil {
			return nil, err
		}
		values = append(values, collectionScalarValue(value))
	}
	return executePineCollectionOperation(namespace, operation, target, values)
}
