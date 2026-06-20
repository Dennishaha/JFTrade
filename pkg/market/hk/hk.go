package hk

import (
	"sync"
	"time"
)

const (
	Code            = "HK"
	ResolvedMarket  = "HK"
	PreferredPrefix = "HK"
	LocationName    = "Asia/Hong_Kong"
)

var RegularWindows = [][2]int{
	{9*60 + 30, 12 * 60},
	{13 * 60, 16 * 60},
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
