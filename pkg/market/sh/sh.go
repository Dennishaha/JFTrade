package sh

import "time"

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

func Location() *time.Location {
	loc, err := time.LoadLocation(LocationName)
	if err != nil {
		return time.UTC
	}
	return loc
}
