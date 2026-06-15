package pineruntime

import (
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestExecuteCollectionStatementsPersistAcrossBars(t *testing.T) {
	runtime := &strategyRuntime{persistentValues: map[string]any{}}
	statements := []strategyir.Statement{
		&strategyir.CollectionStmt{
			Range:      strategyir.SourceRange{StartLine: 1, EndLine: 1},
			Namespace:  "array",
			Operation:  "new_float",
			ResultName: "values",
			Arguments:  []string{"0"},
			Mode:       strategyir.AssignmentModeVar,
		},
		&strategyir.CollectionStmt{
			Range:     strategyir.SourceRange{StartLine: 2, EndLine: 2},
			Namespace: "array",
			Operation: "push",
			Target:    "values",
			Arguments: []string{"close"},
		},
		&strategyir.CollectionStmt{
			Range:      strategyir.SourceRange{StartLine: 3, EndLine: 3},
			Namespace:  "array",
			Operation:  "size",
			Target:     "values",
			ResultName: "count",
		},
	}

	first := collectionTestScope(runtime, 101)
	if _, err := runtime.executeStatements(statements, first); err != nil {
		t.Fatalf("first executeStatements: %v", err)
	}
	if got, _ := coerceFloatValue(first.variables["count"]); got != 1 {
		t.Fatalf("first count = %v, want 1", got)
	}

	second := collectionTestScope(runtime, 102)
	if _, err := runtime.executeStatements(statements, second); err != nil {
		t.Fatalf("second executeStatements: %v", err)
	}
	if got, _ := coerceFloatValue(second.variables["count"]); got != 2 {
		t.Fatalf("second count = %v, want 2", got)
	}
	values, ok := second.variables["values"].(*pineArray)
	if !ok || len(values.values) != 2 || values.values[0] != float64(101) || values.values[1] != float64(102) {
		t.Fatalf("values = %#v", second.variables["values"])
	}
}

func TestExecuteCollectionStatementsSupportMapAndMatrix(t *testing.T) {
	runtime := &strategyRuntime{persistentValues: map[string]any{}}
	scope := collectionTestScope(runtime, 103)
	statements := []strategyir.Statement{
		&strategyir.CollectionStmt{Range: strategyir.SourceRange{StartLine: 1}, Namespace: "map", Operation: "new", ResultName: "prices", TypeArgs: "string, float", Mode: strategyir.AssignmentModeVar},
		&strategyir.CollectionStmt{Range: strategyir.SourceRange{StartLine: 2}, Namespace: "map", Operation: "put", Target: "prices", Arguments: []string{`"last"`, "close"}},
		&strategyir.CollectionStmt{Range: strategyir.SourceRange{StartLine: 3}, Namespace: "map", Operation: "get", Target: "prices", ResultName: "latest", Arguments: []string{`"last"`}},
		&strategyir.CollectionStmt{Range: strategyir.SourceRange{StartLine: 4}, Namespace: "matrix", Operation: "new", ResultName: "grid", TypeArgs: "float", Arguments: []string{"2", "2", "0"}},
		&strategyir.CollectionStmt{Range: strategyir.SourceRange{StartLine: 5}, Namespace: "matrix", Operation: "set", Target: "grid", Arguments: []string{"1", "1", "close"}},
		&strategyir.CollectionStmt{Range: strategyir.SourceRange{StartLine: 6}, Namespace: "matrix", Operation: "get", Target: "grid", ResultName: "cell", Arguments: []string{"1", "1"}},
	}
	if _, err := runtime.executeStatements(statements, scope); err != nil {
		t.Fatalf("executeStatements: %v", err)
	}
	if got, _ := coerceFloatValue(scope.variables["latest"]); got != 103 {
		t.Fatalf("latest = %v, want 103", got)
	}
	if got, _ := coerceFloatValue(scope.variables["cell"]); got != 103 {
		t.Fatalf("cell = %v, want 103", got)
	}
}

func TestCollectionRuntimeRejectsBoundsAndTypeErrors(t *testing.T) {
	array := &pineArray{elementType: "int", values: []any{float64(1)}}
	if _, err := array.operation("get", []any{float64(2)}); err == nil {
		t.Fatal("array.get out of bounds error = nil")
	}
	if _, err := array.operation("push", []any{1.5}); err == nil {
		t.Fatal("array.push type error = nil")
	}
}

func TestExecuteV23ArrayCollectionOperations(t *testing.T) {
	array := &pineArray{elementType: "float", values: []any{float64(1), float64(2), float64(3), float64(2)}}
	if value, err := array.operation("copy", nil); err != nil {
		t.Fatalf("array.copy error = %v", err)
	} else if copied, ok := value.(*pineArray); !ok || copied == array || len(copied.values) != 4 || copied.values[2] != float64(3) {
		t.Fatalf("array.copy = %#v", value)
	}
	if value, err := array.operation("slice", []any{float64(1), float64(3)}); err != nil {
		t.Fatalf("array.slice error = %v", err)
	} else if sliced, ok := value.(*pineArray); !ok || len(sliced.values) != 2 || sliced.values[0] != float64(2) || sliced.values[1] != float64(3) {
		t.Fatalf("array.slice = %#v", value)
	}
	if value, _ := array.operation("includes", []any{float64(3)}); value != true {
		t.Fatalf("array.includes = %#v, want true", value)
	}
	if value, _ := array.operation("indexof", []any{float64(2)}); value != float64(1) {
		t.Fatalf("array.indexof = %#v, want 1", value)
	}
	if value, _ := array.operation("lastindexof", []any{float64(2)}); value != float64(3) {
		t.Fatalf("array.lastindexof = %#v, want 3", value)
	}
	for operation, want := range map[string]float64{"min": 1, "max": 3, "avg": 2, "sum": 8} {
		value, err := array.operation(operation, nil)
		if err != nil {
			t.Fatalf("array.%s error = %v", operation, err)
		}
		if got, _ := coerceFloatValue(value); got != want {
			t.Fatalf("array.%s = %v, want %v", operation, got, want)
		}
	}
	if _, err := array.operation("fill", []any{float64(5), float64(1), float64(3)}); err != nil {
		t.Fatalf("array.fill error = %v", err)
	}
	if array.values[0] != float64(1) || array.values[1] != float64(5) || array.values[2] != float64(5) || array.values[3] != float64(2) {
		t.Fatalf("array after fill = %#v", array.values)
	}
	if _, err := array.operation("reverse", nil); err != nil {
		t.Fatalf("array.reverse error = %v", err)
	}
	if array.values[0] != float64(2) || array.values[3] != float64(1) {
		t.Fatalf("array after reverse = %#v", array.values)
	}
}

func TestExecuteV23MatrixCollectionOperations(t *testing.T) {
	matrix := &pineMatrix{elementType: "float", rows: 2, columns: 2, values: []any{float64(1), float64(2), float64(3), float64(4)}}
	if value, err := matrix.operation("copy", nil); err != nil {
		t.Fatalf("matrix.copy error = %v", err)
	} else if copied, ok := value.(*pineMatrix); !ok || copied == matrix || copied.values[3] != float64(4) {
		t.Fatalf("matrix.copy = %#v", value)
	}
	if _, err := matrix.operation("reshape", []any{float64(1), float64(4)}); err != nil {
		t.Fatalf("matrix.reshape error = %v", err)
	}
	if matrix.rows != 1 || matrix.columns != 4 {
		t.Fatalf("matrix shape = %dx%d, want 1x4", matrix.rows, matrix.columns)
	}
	if _, err := matrix.operation("reshape", []any{float64(2), float64(2)}); err != nil {
		t.Fatalf("matrix.reshape back error = %v", err)
	}
	row := &pineArray{elementType: "float", values: []any{float64(7), float64(8)}}
	if _, err := matrix.operation("add_row", []any{float64(1), row}); err != nil {
		t.Fatalf("matrix.add_row error = %v", err)
	}
	if matrix.rows != 3 || matrix.columns != 2 || matrix.values[2] != float64(7) || matrix.values[3] != float64(8) {
		t.Fatalf("matrix after add_row = %#v shape %dx%d", matrix.values, matrix.rows, matrix.columns)
	}
	column := &pineArray{elementType: "float", values: []any{float64(9), float64(10), float64(11)}}
	if _, err := matrix.operation("add_col", []any{float64(1), column}); err != nil {
		t.Fatalf("matrix.add_col error = %v", err)
	}
	if matrix.columns != 3 || matrix.values[1] != float64(9) || matrix.values[4] != float64(10) || matrix.values[7] != float64(11) {
		t.Fatalf("matrix after add_col = %#v shape %dx%d", matrix.values, matrix.rows, matrix.columns)
	}
	removedColumn, err := matrix.operation("remove_col", []any{float64(1)})
	if err != nil {
		t.Fatalf("matrix.remove_col error = %v", err)
	}
	if removed, ok := removedColumn.(*pineArray); !ok || len(removed.values) != 3 || removed.values[0] != float64(9) {
		t.Fatalf("removed column = %#v", removedColumn)
	}
	removedRow, err := matrix.operation("remove_row", []any{float64(1)})
	if err != nil {
		t.Fatalf("matrix.remove_row error = %v", err)
	}
	if removed, ok := removedRow.(*pineArray); !ok || len(removed.values) != 2 || removed.values[0] != float64(7) {
		t.Fatalf("removed row = %#v", removedRow)
	}
	if _, err := matrix.operation("fill", []any{float64(6)}); err != nil {
		t.Fatalf("matrix.fill error = %v", err)
	}
	for _, value := range matrix.values {
		if value != float64(6) {
			t.Fatalf("matrix after fill = %#v", matrix.values)
		}
	}
}

func TestExecuteV24ArrayCollectionOperations(t *testing.T) {
	constructed, err := constructPineCollection(&strategyir.CollectionStmt{Namespace: "array", Operation: "from"}, []any{float64(3), float64(1), float64(2), float64(2)})
	if err != nil {
		t.Fatalf("array.from error = %v", err)
	}
	array, ok := constructed.(*pineArray)
	if !ok || len(array.values) != 4 {
		t.Fatalf("array.from = %#v", constructed)
	}
	if value, err := array.operation("sort_indices", []any{"ascending"}); err != nil {
		t.Fatalf("array.sort_indices error = %v", err)
	} else if indices, ok := value.(*pineArray); !ok || len(indices.values) != 4 || indices.values[0] != float64(1) || indices.values[3] != float64(0) {
		t.Fatalf("array.sort_indices = %#v", value)
	}
	if _, err := array.operation("sort", []any{"descending"}); err != nil {
		t.Fatalf("array.sort error = %v", err)
	}
	if array.values[0] != float64(3) || array.values[3] != float64(1) {
		t.Fatalf("array after descending sort = %#v", array.values)
	}
	if value, err := array.operation("binary_search", []any{float64(2)}); err != nil {
		t.Fatalf("array.binary_search error = %v", err)
	} else if value != float64(-1) {
		t.Fatalf("descending binary_search = %#v, want -1 for ascending-only search", value)
	}
	if _, err := array.operation("sort", []any{"ascending"}); err != nil {
		t.Fatalf("array.sort ascending error = %v", err)
	}
	if value, err := array.operation("binary_search", []any{float64(2)}); err != nil {
		t.Fatalf("array.binary_search ascending error = %v", err)
	} else if value != float64(1) {
		t.Fatalf("array.binary_search = %#v, want 1", value)
	}
	for operation, want := range map[string]float64{"median": 2, "mode": 2, "range": 2} {
		value, err := array.operation(operation, nil)
		if err != nil {
			t.Fatalf("array.%s error = %v", operation, err)
		}
		if got, _ := coerceFloatValue(value); got != want {
			t.Fatalf("array.%s = %v, want %v", operation, got, want)
		}
	}
	other := &pineArray{values: []any{float64(4)}}
	if _, err := array.operation("concat", []any{other}); err != nil {
		t.Fatalf("array.concat error = %v", err)
	}
	if value, err := array.operation("join", []any{","}); err != nil {
		t.Fatalf("array.join error = %v", err)
	} else if value != "1,2,2,3,4" {
		t.Fatalf("array.join = %#v", value)
	}
}

func TestExecuteV24MapCollectionOperations(t *testing.T) {
	values := &pineMap{keyType: "string", valueType: "float", values: map[any]any{}}
	for _, item := range []struct {
		key   string
		value float64
	}{{"b", 2}, {"a", 1}, {"c", 3}} {
		if _, err := values.operation("put", []any{item.key, item.value}); err != nil {
			t.Fatalf("map.put(%s) error = %v", item.key, err)
		}
	}
	copied, err := values.operation("copy", nil)
	if err != nil {
		t.Fatalf("map.copy error = %v", err)
	}
	if copiedMap, ok := copied.(*pineMap); !ok || copiedMap == values || len(copiedMap.values) != 3 {
		t.Fatalf("map.copy = %#v", copied)
	}
	keys, err := values.operation("keys", nil)
	if err != nil {
		t.Fatalf("map.keys error = %v", err)
	}
	keyArray, ok := keys.(*pineArray)
	if !ok || len(keyArray.values) != 3 || keyArray.values[0] != "a" || keyArray.values[2] != "c" {
		t.Fatalf("map.keys = %#v", keys)
	}
	rawValues, err := values.operation("values", nil)
	if err != nil {
		t.Fatalf("map.values error = %v", err)
	}
	valueArray, ok := rawValues.(*pineArray)
	if !ok || len(valueArray.values) != 3 || valueArray.values[0] != float64(1) || valueArray.values[2] != float64(3) {
		t.Fatalf("map.values = %#v", rawValues)
	}
}

func TestExecuteV25ArrayCollectionStatistics(t *testing.T) {
	array := &pineArray{elementType: "float", values: []any{float64(-2), float64(1), float64(2), float64(2), float64(5)}}
	absValue, err := array.operation("abs", nil)
	if err != nil {
		t.Fatalf("array.abs error = %v", err)
	}
	absArray, ok := absValue.(*pineArray)
	if !ok || absArray == array || absArray.values[0] != float64(2) || array.values[0] != float64(-2) {
		t.Fatalf("array.abs = %#v original %#v", absValue, array.values)
	}
	if _, err := array.operation("sort", []any{"ascending"}); err != nil {
		t.Fatalf("array.sort error = %v", err)
	}
	for operation, want := range map[string]float64{
		"binary_search_leftmost":  2,
		"binary_search_rightmost": 3,
	} {
		value, err := array.operation(operation, []any{float64(2)})
		if err != nil {
			t.Fatalf("array.%s error = %v", operation, err)
		}
		if got, _ := coerceFloatValue(value); got != want {
			t.Fatalf("array.%s = %v, want %v", operation, got, want)
		}
	}
	if value, err := array.operation("percentrank", []any{float64(3)}); err != nil {
		t.Fatalf("array.percentrank error = %v", err)
	} else if got, _ := coerceFloatValue(value); got != 75 {
		t.Fatalf("array.percentrank = %v, want 75", got)
	}
	for operation, want := range map[string]float64{
		"percentile_nearest_rank":         2,
		"percentile_linear_interpolation": 2,
		"variance":                        5.04,
	} {
		args := []any{float64(50)}
		if operation == "variance" {
			args = nil
		}
		value, err := array.operation(operation, args)
		if err != nil {
			t.Fatalf("array.%s error = %v", operation, err)
		}
		got, _ := coerceFloatValue(value)
		if got != want {
			t.Fatalf("array.%s = %v, want %v", operation, got, want)
		}
	}
	if value, err := array.operation("stdev", nil); err != nil {
		t.Fatalf("array.stdev error = %v", err)
	} else if got, _ := coerceFloatValue(value); got < 2.244 || got > 2.245 {
		t.Fatalf("array.stdev = %v, want sqrt(5.04)", got)
	}
	other := &pineArray{values: []any{float64(2), float64(4), float64(6), float64(8), float64(10)}}
	if value, err := array.operation("covariance", []any{other}); err != nil {
		t.Fatalf("array.covariance error = %v", err)
	} else if got, _ := coerceFloatValue(value); got != 6.0 {
		t.Fatalf("array.covariance = %v, want 6.0", got)
	}
}

func collectionTestScope(runtime *strategyRuntime, close float64) *evaluationScope {
	return &evaluationScope{
		runtime:      runtime,
		variables:    map[string]any{},
		hasBarData:   true,
		closeSeries:  seriesNumber{Current: close, HasCurrent: true},
		openSeries:   seriesNumber{Current: close, HasCurrent: true},
		highSeries:   seriesNumber{Current: close, HasCurrent: true},
		lowSeries:    seriesNumber{Current: close, HasCurrent: true},
		volumeSeries: seriesNumber{Current: 1, HasCurrent: true},
	}
}
