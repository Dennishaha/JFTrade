package types

import "math"

// Softmax returns the input value in the range of 0 to 1
// with sum of all the probabilities being equal to one.
// It is commonly used in machine learning neural networks.
// Will return Softmax SeriesExtend result based in latest [window] numbers from [a] Series
func Softmax(a Series, window int) SeriesExtend {
	s := 0.0
	max := Highest(a, window)
	for i := range window {
		s += math.Exp(a.Last(i) - max)
	}
	out := NewQueue(window)
	for i := window - 1; i >= 0; i-- {
		out.Update(math.Exp(a.Last(i)-max) / s)
	}
	return out
}

// Entropy computes the Shannon entropy of a distribution or the distance between
// two distributions. The natural logarithm is used.
// - sum(v * ln(v))
func Entropy(a Series, window int) (e float64) {
	for i := range window {
		v := a.Last(i)
		if v != 0 {
			e -= v * math.Log(v)
		}
	}
	return e
}

// CrossEntropy computes the cross-entropy between the two distributions
func CrossEntropy(a, b Series, window int) (e float64) {
	for i := range window {
		v := a.Last(i)
		if v != 0 {
			e -= v * math.Log(b.Last(i))
		}
	}
	return e
}

func sigmoid(z float64) float64 {
	return 1. / (1. + math.Exp(-z))
}

func propagate(w []float64, gradient float64, x [][]float64, y []float64) (float64, []float64, float64) {
	loglossEpoch := 0.0
	var activations []float64
	var dw []float64
	m := len(y)
	db := 0.0
	for i, xx := range x {
		result := 0.0
		for j, ww := range w {
			result += ww * xx[j]
		}
		a := sigmoid(result + gradient)
		activations = append(activations, a)
		logloss := a*math.Log1p(y[i]) + (1.-a)*math.Log1p(1-y[i])
		loglossEpoch += logloss

		db += a - y[i]
	}
	for j := range w {
		err := 0.0
		for i, xx := range x {
			errI := activations[i] - y[i]
			err += errI * xx[j]
		}
		err /= float64(m)
		dw = append(dw, err)
	}

	cost := -(loglossEpoch / float64(len(x)))
	db /= float64(m)
	return cost, dw, db
}

func LogisticRegression(x []Series, y Series, lookback, iterations int, learningRate float64) *LogisticRegressionModel {
	features := len(x)
	if features == 0 {
		panic("no feature to train")
	}
	w := make([]float64, features)
	if lookback > x[0].Length() {
		lookback = x[0].Length()
	}
	xx := make([][]float64, lookback)
	for i := 0; i < lookback; i++ {
		for j := range features {
			xx[i] = append(xx[i], x[j].Last(lookback-i-1))
		}
	}
	yy := Reverse(y, lookback)

	b := 0.
	for range iterations {
		_, dw, db := propagate(w, b, xx, yy)
		for j := range w {
			w[j] = w[j] - (learningRate * dw[j])
		}
		b -= learningRate * db
	}
	return &LogisticRegressionModel{
		Weight:       w,
		Gradient:     b,
		LearningRate: learningRate,
	}
}

type LogisticRegressionModel struct {
	Weight       []float64
	Gradient     float64
	LearningRate float64
}

/*
// Might not be correct.
// Please double check before uncomment this
func (l *LogisticRegressionModel) Update(x []float64, y float64) {
	z := 0.0
	for i, w := l.Weight {
		z += w * x[i]
	}
	a := sigmoid(z + l.Gradient)
	//logloss := a * math.Log1p(y) + (1.-a)*math.Log1p(1-y)
	db = a - y
	var dw []float64
	for j, ww := range l.Weight {
		err := db * x[j]
		dw = append(dw, err)
	}
	for i := range l.Weight {
		l.Weight[i] -= l.LearningRate * dw[i]
	}
	l.Gradient -= l.LearningRate * db
}
*/

func (l *LogisticRegressionModel) Predict(x []float64) float64 {
	z := 0.0
	for i, w := range l.Weight {
		z += w * x[i]
	}
	return sigmoid(z + l.Gradient)
}
