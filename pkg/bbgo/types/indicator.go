package types

import (
	"fmt"
	"gonum.org/v1/gonum/stat"
	"math"
	"reflect"

	"github.com/jftrade/jftrade-main/pkg/bbgo/datatype/floats"
)

// Float64Indicator is the indicators (SMA and EWMA) that we want to use are returning float64 data.
type Float64Indicator interface {
	Last(i int) float64
}

type AbsResult struct {
	a Series
}

func (a *AbsResult) Last(i int) float64 {
	return math.Abs(a.a.Last(i))
}

func (a *AbsResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *AbsResult) Length() int {
	return a.a.Length()
}

// Return series that having all the elements positive
func Abs(a Series) SeriesExtend {
	return NewSeries(&AbsResult{a})
}

var _ Series = &AbsResult{}

func LinearRegression(a Series, lookback int) (alpha float64, beta float64) {
	if a.Length() < lookback {
		lookback = a.Length()
	}
	x := make([]float64, lookback)
	y := make([]float64, lookback)
	var weights []float64
	for i := 0; i < lookback; i++ {
		x[i] = float64(i)
		y[i] = a.Last(i)
	}
	alpha, beta = stat.LinearRegression(x, y, weights, false)
	return
}

func Predict(a Series, lookback int, offset ...int) float64 {
	alpha, beta := LinearRegression(a, lookback)
	o := -1.0
	if len(offset) > 0 {
		o = -float64(offset[0])
	}
	return alpha + beta*o
}

// This will make prediction using Linear Regression to get the next cross point
// Return (offset from latest, crossed value, could cross)
// offset from latest should always be positive
// lookback param is to use at most `lookback` points to determine linear regression functions
//
// You may also refer to excel's FORECAST function
func NextCross(a Series, b Series, lookback int) (int, float64, bool) {
	if a.Length() < lookback {
		lookback = a.Length()
	}
	if b.Length() < lookback {
		lookback = b.Length()
	}
	x := make([]float64, lookback)
	y1 := make([]float64, lookback)
	y2 := make([]float64, lookback)
	var weights []float64
	for i := 0; i < lookback; i++ {
		x[i] = float64(i)
		y1[i] = a.Last(i)
		y2[i] = b.Last(i)
	}
	alpha1, beta1 := stat.LinearRegression(x, y1, weights, false)
	alpha2, beta2 := stat.LinearRegression(x, y2, weights, false)
	if beta2 == beta1 {
		return 0, 0, false
	}
	indexf := (alpha1 - alpha2) / (beta2 - beta1)

	// crossed in different direction
	if indexf >= 0 {
		return 0, 0, false
	}
	return int(math.Ceil(-indexf)), alpha1 + beta1*indexf, true
}

func Highest(a Series, lookback int) float64 {
	if lookback > a.Length() {
		lookback = a.Length()
	}
	highest := a.Last(0)
	for i := 1; i < lookback; i++ {
		current := a.Last(i)
		if highest < current {
			highest = current
		}
	}
	return highest
}

func Lowest(a Series, lookback int) float64 {
	if lookback > a.Length() {
		lookback = a.Length()
	}
	lowest := a.Last(0)
	for i := 1; i < lookback; i++ {
		current := a.Last(i)
		if lowest > current {
			lowest = current
		}
	}
	return lowest
}

type NumberSeries float64

func (a NumberSeries) Last(_ int) float64 {
	return float64(a)
}

func (a NumberSeries) Index(_ int) float64 {
	return float64(a)
}

func (a NumberSeries) Length() int {
	return math.MaxInt32
}

func (a NumberSeries) Clone() NumberSeries {
	return a
}

var _ Series = NumberSeries(0)

type AddSeriesResult struct {
	a Series
	b Series
}

// Add two series, result[i] = a[i] + b[i]
func Add(a any, b any) SeriesExtend {
	aa := switchIface(a)
	bb := switchIface(b)
	return NewSeries(&AddSeriesResult{aa, bb})
}

func (a *AddSeriesResult) Last(i int) float64 {
	return a.a.Last(i) + a.b.Last(i)
}

func (a *AddSeriesResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *AddSeriesResult) Length() int {
	lengtha := a.a.Length()
	lengthb := a.b.Length()
	if lengtha < lengthb {
		return lengtha
	}
	return lengthb
}

var _ Series = &AddSeriesResult{}

type MinusSeriesResult struct {
	a Series
	b Series
}

// Sub two series, result[i] = a[i] - b[i]
func Sub(a any, b any) SeriesExtend {
	aa := switchIface(a)
	bb := switchIface(b)
	return NewSeries(&MinusSeriesResult{aa, bb})
}

func (a *MinusSeriesResult) Last(i int) float64 {
	return a.a.Last(i) - a.b.Last(i)
}

func (a *MinusSeriesResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *MinusSeriesResult) Length() int {
	lengtha := a.a.Length()
	lengthb := a.b.Length()
	if lengtha < lengthb {
		return lengtha
	}
	return lengthb
}

var _ Series = &MinusSeriesResult{}

func switchIface(b any) Series {
	switch tp := b.(type) {
	case float64:
		return NumberSeries(tp)
	case int32:
		return NumberSeries(float64(tp))
	case int64:
		return NumberSeries(float64(tp))
	case float32:
		return NumberSeries(float64(tp))
	case int:
		return NumberSeries(float64(tp))
	case Series:
		return tp
	default:
		fmt.Println(reflect.TypeOf(b))
		panic("input should be either *Series or numbers")

	}
}

// Divid two series, result[i] = a[i] / b[i]
func Div(a any, b any) SeriesExtend {
	aa := switchIface(a)
	if b == 0 {
		panic("Divid by zero exception")
	}
	bb := switchIface(b)
	return NewSeries(&DivSeriesResult{aa, bb})

}

type DivSeriesResult struct {
	a Series
	b Series
}

func (a *DivSeriesResult) Last(i int) float64 {
	return a.a.Last(i) / a.b.Last(i)
}

func (a *DivSeriesResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *DivSeriesResult) Length() int {
	lengtha := a.a.Length()
	lengthb := a.b.Length()
	if lengtha < lengthb {
		return lengtha
	}
	return lengthb
}

var _ Series = &DivSeriesResult{}

// Multiple two series, result[i] = a[i] * b[i]
func Mul(a any, b any) SeriesExtend {
	aa := switchIface(a)
	bb := switchIface(b)
	return NewSeries(&MulSeriesResult{aa, bb})
}

type MulSeriesResult struct {
	a Series
	b Series
}

func (a *MulSeriesResult) Last(i int) float64 {
	return a.a.Last(i) * a.b.Last(i)
}

func (a *MulSeriesResult) Index(i int) float64 {
	return a.Last(i)
}

func (a *MulSeriesResult) Length() int {
	lengtha := a.a.Length()
	lengthb := a.b.Length()
	if lengtha < lengthb {
		return lengtha
	}
	return lengthb
}

var _ Series = &MulSeriesResult{}

// Calculate (a dot b).
// if limit is given, will only calculate the first limit numbers (a.Index[0..limit])
// otherwise will operate on all elements
func Dot(a any, b any, limit ...int) float64 {
	left := dotOperandFromAny(a)
	right := dotOperandFromAny(b)
	length := dotLength(left, right, limit)
	return dotProduct(left, right, length)
}

type dotOperand struct {
	scalar   float64
	series   Series
	isScalar bool
}

func dotOperandFromAny(value any) dotOperand {
	switch tp := value.(type) {
	case float64:
		return dotOperand{scalar: tp, isScalar: true}
	case int32:
		return dotOperand{scalar: float64(tp), isScalar: true}
	case int64:
		return dotOperand{scalar: float64(tp), isScalar: true}
	case float32:
		return dotOperand{scalar: float64(tp), isScalar: true}
	case int:
		return dotOperand{scalar: float64(tp), isScalar: true}
	case Series:
		return dotOperand{series: tp}
	default:
		panic("input should be either *Series or numbers")
	}
}

func dotLength(left dotOperand, right dotOperand, limit []int) int {
	if len(limit) > 0 {
		return limit[0]
	}
	if left.isScalar && right.isScalar {
		return 1
	}
	length := dotSeriesLength(left)
	if otherLength := dotSeriesLength(right); otherLength != 0 && (length == 0 || otherLength < length) {
		return otherLength
	}
	return length
}

func dotSeriesLength(operand dotOperand) int {
	if operand.isScalar || operand.series == nil {
		return 0
	}
	return operand.series.Length()
}

func dotProduct(left dotOperand, right dotOperand, length int) float64 {
	if left.isScalar && right.isScalar {
		return left.scalar * right.scalar * float64(length)
	}
	sum := 0.
	for i := range length {
		sum += dotAt(left, right, i)
	}
	return sum
}

func dotAt(left dotOperand, right dotOperand, index int) float64 {
	switch {
	case left.isScalar:
		return left.scalar * right.series.Last(index)
	case right.isScalar:
		return left.series.Last(index) * right.scalar
	default:
		return left.series.Last(index) * right.series.Index(index)
	}
}

// Array extracts elements from the Series to a float64 array, following the order of Index(0..limit)
// if limit is given, will only take the first limit numbers (a.Index[0..limit])
// otherwise will operate on all elements
func Array(a Series, limit ...int) (result []float64) {
	l := a.Length()
	if len(limit) > 0 && l > limit[0] {
		l = limit[0]
	}
	if l > a.Length() {
		l = a.Length()
	}
	result = make([]float64, l)
	for i := 0; i < l; i++ {
		result[i] = a.Last(i)
	}
	return
}

// Ordinary Least Squares fit result, only support 1d array
func OLS(a SeriesExtend, b SeriesExtend, n int) (float64, float64) {
	if a.Length() < n {
		n = a.Length()
	}
	if b.Length() < n {
		n = b.Length()
	}
	numerator := 0.0
	denominator := 0.0
	meana := a.Mean(n)
	meanb := b.Mean(n)
	for i := 0; i < n; i++ {
		x := a.Last(i)
		y := b.Last(i)
		numerator += (x - meana) * (y - meanb)
		denominator += (x - meana) * (x - meana)
	}
	if denominator == 0 {
		return 0, 0
	}
	beta := numerator / denominator
	alpha := meanb - beta*meana
	return alpha, beta
}

// Similar to Array but in reverse order.
// Useful when you want to cache series' calculated result as float64 array
// the then reuse the result in multiple places (so that no recalculation will be triggered)
//
// notice that the return type is a Float64Slice, which implements the Series interface
func Reverse(a Series, limit ...int) (result floats.Slice) {
	l := a.Length()
	if len(limit) > 0 && l > limit[0] {
		l = limit[0]
	}
	result = make([]float64, l)
	for i := 0; i < l; i++ {
		result[l-i-1] = a.Last(i)
	}
	return
}

type ChangeResult struct {
	a      Series
	offset int
}

func (c *ChangeResult) Last(i int) float64 {
	if i+c.offset >= c.a.Length() {
		return 0
	}
	return c.a.Last(i) - c.a.Last(i+c.offset)
}

func (c *ChangeResult) Index(i int) float64 {
	return c.Last(i)
}

func (c *ChangeResult) Length() int {
	length := c.a.Length()
	if length >= c.offset {
		return length - c.offset
	}
	return 0
}

// Difference between current value and previous, a - a[offset]
// offset: if not given, offset is 1.
func Change(a Series, offset ...int) SeriesExtend {
	o := 1
	if len(offset) > 0 {
		o = offset[0]
	}

	return NewSeries(&ChangeResult{a, o})
}

type PercentageChangeResult struct {
	a      Series
	offset int
}

func (c *PercentageChangeResult) Last(i int) float64 {
	if i+c.offset >= c.a.Length() {
		return 0
	}
	return c.a.Last(i)/c.a.Last(i+c.offset) - 1
}

func (c *PercentageChangeResult) Index(i int) float64 {
	return c.Last(i)
}

func (c *PercentageChangeResult) Length() int {
	length := c.a.Length()
	if length >= c.offset {
		return length - c.offset
	}
	return 0
}

// Percentage change between current and a prior element, a / a[offset] - 1.
// offset: if not give, offset is 1.
func PercentageChange(a Series, offset ...int) SeriesExtend {
	o := 1
	if len(offset) > 0 {
		o = offset[0]
	}

	return NewSeries(&PercentageChangeResult{a, o})
}

func Stdev(a Series, params ...int) float64 {
	length := a.Length()
	if length == 0 {
		return 0
	}
	if len(params) > 0 && params[0] < length {
		length = params[0]
	}
	ddof := 0
	if len(params) > 1 {
		ddof = params[1]
	}
	avg := Mean(a, length)
	s := .0
	for i := 0; i < length; i++ {
		diff := a.Last(i) - avg
		s += diff * diff
	}
	if length-ddof == 0 {
		return 0
	}
	return math.Sqrt(s / float64(length-ddof))
}

type CorrFunc func(Series, Series, int) float64

func Kendall(a, b Series, length int) float64 {
	if a.Length() < length {
		length = a.Length()
	}
	if b.Length() < length {
		length = b.Length()
	}
	aRanks := Rank(a, length)
	bRanks := Rank(b, length)
	concordant, discordant := 0, 0
	for i := 0; i < length; i++ {
		for j := i + 1; j < length; j++ {
			value := (aRanks.Last(i) - aRanks.Last(j)) * (bRanks.Last(i) - bRanks.Last(j))
			if value > 0 {
				concordant++
			} else {
				discordant++
			}
		}
	}
	return float64(concordant-discordant) * 2.0 / float64(length*(length-1))
}

func Rank(a Series, length int) SeriesExtend {
	if length > a.Length() {
		length = a.Length()
	}
	rank := make([]float64, length)
	mapper := make([]float64, length+1)
	for i := length - 1; i >= 0; i-- {
		ii := a.Last(i)
		counter := 0.
		for j := 0; j < length; j++ {
			if a.Last(j) <= ii {
				counter += 1.
			}
		}
		rank[i] = counter
		mapper[int(counter)] += 1.
	}
	output := NewQueue(length)
	for i := length - 1; i >= 0; i-- {
		output.Update(rank[i] - (mapper[int(rank[i])]-1.)/2)
	}
	return output
}

func Pearson(a, b Series, length int) float64 {
	if a.Length() < length {
		length = a.Length()
	}
	if b.Length() < length {
		length = b.Length()
	}
	x := make([]float64, length)
	y := make([]float64, length)
	for i := 0; i < length; i++ {
		x[i] = a.Last(i)
		y[i] = b.Last(i)
	}
	return stat.Correlation(x, y, nil)
}

func Spearman(a, b Series, length int) float64 {
	if a.Length() < length {
		length = a.Length()
	}
	if b.Length() < length {
		length = b.Length()
	}
	aRank := Rank(a, length)
	bRank := Rank(b, length)
	return Pearson(aRank, bRank, length)
}

// similar to pandas.Series.corr() function.
//
// method could either be `types.Pearson`, `types.Spearman` or `types.Kendall`
func Correlation(a Series, b Series, length int, method ...CorrFunc) float64 {
	var runner CorrFunc
	if len(method) == 0 {
		runner = Pearson
	} else {
		runner = method[0]
	}
	return runner(a, b, length)
}

// similar to pandas.Series.autocorr() function.
//
// The method computes the Pearson correlation between Series and shifted itself
func AutoCorrelation(a Series, length int, lags ...int) float64 {
	lag := 1
	if len(lags) > 0 {
		lag = lags[0]
	}
	return Pearson(a, Shift(a, lag), length)
}

// similar to pandas.Series.cov() function with ddof=0
//
// Compute covariance with Series
func Covariance(a Series, b Series, length int) float64 {
	if a.Length() < length {
		length = a.Length()
	}
	if b.Length() < length {
		length = b.Length()
	}

	meana := Mean(a, length)
	meanb := Mean(b, length)
	sum := 0.0
	for i := 0; i < length; i++ {
		sum += (a.Last(i) - meana) * (b.Last(i) - meanb)
	}
	sum /= float64(length)
	return sum
}

func Variance(a Series, length int) float64 {
	return Covariance(a, a, length)
}

// similar to pandas.Series.skew() function.
//
// Return unbiased skew over input series
func Skew(a Series, length int) float64 {
	if length > a.Length() {
		length = a.Length()
	}
	mean := Mean(a, length)
	sum2 := 0.0
	sum3 := 0.0
	for i := 0; i < length; i++ {
		diff := a.Last(i) - mean
		sum2 += diff * diff
		sum3 += diff * diff * diff
	}
	if length <= 2 || sum2 == 0 {
		return math.NaN()
	}
	l := float64(length)
	return l * math.Sqrt(l-1) / (l - 2) * sum3 / math.Pow(sum2, 1.5)
}

type ShiftResult struct {
	a      Series
	offset int
}

func (inc *ShiftResult) Last(i int) float64 {
	if inc.offset+i < 0 {
		return 0
	}
	if inc.offset+i >= inc.a.Length() {
		return 0
	}

	return inc.a.Last(inc.offset + i)
}

func (inc *ShiftResult) Index(i int) float64 {
	return inc.Last(i)
}

func (inc *ShiftResult) Length() int {
	return inc.a.Length() - inc.offset
}

func Shift(a Series, offset int) SeriesExtend {
	return NewSeries(&ShiftResult{a, offset})
}

type RollingResult struct {
	a      Series
	window int
}

type SliceView struct {
	a      Series
	start  int
	length int
}

func (s *SliceView) Last(i int) float64 {
	if i >= s.length {
		return 0
	}

	return s.a.Last(i + s.start)
}

func (s *SliceView) Index(i int) float64 {
	return s.Last(i)
}

func (s *SliceView) Length() int {
	return s.length
}

var _ Series = &SliceView{}

func (r *RollingResult) Last() SeriesExtend {
	return NewSeries(&SliceView{r.a, 0, r.window})
}

func (r *RollingResult) Index(i int) SeriesExtend {
	if i*r.window > r.a.Length() {
		return nil
	}
	return NewSeries(&SliceView{r.a, i * r.window, r.window})
}

func (r *RollingResult) Length() int {
	mod := r.a.Length() % r.window
	if mod > 0 {
		return r.a.Length()/r.window + 1
	} else {
		return r.a.Length() / r.window
	}
}

func Rolling(a Series, window int) *RollingResult {
	return &RollingResult{a, window}
}

// TODO: ta.linreg
