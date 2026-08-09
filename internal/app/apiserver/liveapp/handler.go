// Package liveapp owns the application-level construction of the live stream
// transport. The websocket protocol itself remains in internal/api/live; this
// package only joins that transport to an application-provided backend.
package liveapp

import (
	"time"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
)

// Options is the small application-owned subset of live stream timing knobs.
type Options struct {
	DataInterval            time.Duration
	SecurityDetailsInterval time.Duration
	DepthRefreshInterval    time.Duration
}

func (o Options) transportOptions() apilive.Options {
	return apilive.Options{
		DataInterval:            o.DataInterval,
		SecurityDetailsInterval: o.SecurityDetailsInterval,
		DepthRefreshInterval:    o.DepthRefreshInterval,
	}
}

// NewHandler constructs the live websocket transport around a narrow backend.
func NewHandler(backend apilive.Backend, options Options) *apilive.Handler {
	return apilive.NewHandler(backend, options.transportOptions())
}
