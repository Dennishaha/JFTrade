//go:build !windows

package marketdataapp

import "syscall"

var platformSidecarStopPlan = sidecarStopPlan{signal: syscall.SIGTERM}
