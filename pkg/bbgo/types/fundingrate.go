package types

import (
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
)

type FundingRate struct {
	Symbol      string
	FundingRate fixedpoint.Value
	FundingTime time.Time
	Time        time.Time
}
