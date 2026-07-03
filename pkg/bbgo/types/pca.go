package types

import (
	"fmt"

	"gonum.org/v1/gonum/mat"
)

type PCA struct {
	svd *mat.SVD
}

func (pca *PCA) FitTransform(x []SeriesExtend, lookback, feature int) ([]SeriesExtend, error) {
	if err := pca.Fit(x, lookback); err != nil {
		return nil, err
	}
	return pca.Transform(x, lookback, feature), nil
}

func (pca *PCA) Fit(x []SeriesExtend, lookback int) error {
	vec := make([]float64, lookback*len(x))
	for i, xx := range x {
		mean := xx.Mean(lookback)
		for j := range lookback {
			vec[i+j*i] = xx.Last(j) - mean
		}
	}
	pca.svd = &mat.SVD{}
	diffMatrix := mat.NewDense(lookback, len(x), vec)
	if ok := pca.svd.Factorize(diffMatrix, mat.SVDThin); !ok {
		return fmt.Errorf("unable to factorize")
	}
	return nil
}

func (pca *PCA) Transform(x []SeriesExtend, lookback int, features int) (result []SeriesExtend) {
	result = make([]SeriesExtend, features)
	vTemp := new(mat.Dense)
	pca.svd.VTo(vTemp)
	var ret mat.Dense
	vec := make([]float64, lookback*len(x))
	for i, xx := range x {
		for j := range lookback {
			vec[i+j*i] = xx.Last(j)
		}
	}
	newX := mat.NewDense(lookback, len(x), vec)
	ret.Mul(newX, vTemp)
	newMatrix := mat.NewDense(lookback, features, nil)
	newMatrix.Copy(&ret)
	for i := range features {
		queue := NewQueue(lookback)
		for j := range lookback {
			queue.Update(newMatrix.At(lookback-j-1, i))
		}
		result[i] = queue
	}
	return result
}
