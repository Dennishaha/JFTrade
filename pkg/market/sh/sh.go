package sh

import (
	"sync"
	"time"
)

const (
	Code            = "SH"
	ResolvedMarket  = "CN"
	PreferredPrefix = "SH"
	LocationName    = "Asia/Shanghai"
)

var RegularWindows = [][2]int{
	{9*60 + 30, 11*60 + 30},
	{13 * 60, 15 * 60},
}

var location = sync.OnceValue(func() *time.Location {
	return loadLocation(LocationName)
})

func loadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

func Location() *time.Location {
	return location()
}
