package us

import "time"

const (
	Code            = "US"
	ResolvedMarket  = "US"
	PreferredPrefix = "US"
	LocationName    = "America/New_York"
)

var RegularWindows = [][2]int{
	{9*60 + 30, 16 * 60},
}

func Location() *time.Location {
	loc, err := time.LoadLocation(LocationName)
	if err != nil {
		return time.UTC
	}
	return loc
}
