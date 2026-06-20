package sz

import (
	"sync"
	"time"
)

const (
	Code            = "SZ"
	ResolvedMarket  = "CN"
	PreferredPrefix = "SZ"
	LocationName    = "Asia/Shanghai"
)

var RegularWindows = [][2]int{
	{9*60 + 30, 11*60 + 30},
	{13 * 60, 15 * 60},
}

var location = sync.OnceValue(func() *time.Location {
	loc, err := time.LoadLocation(LocationName)
	if err != nil {
		return time.UTC
	}
	return loc
})

func Location() *time.Location {
	return location()
}
