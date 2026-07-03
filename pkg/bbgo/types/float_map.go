package types

import "github.com/jftrade/jftrade-main/pkg/bbgo/datatype/floats"

var _ Series = floats.Slice([]float64{}).Addr()
