//go:build windows

package marketdataapp

// Windows does not implement os.Interrupt for child processes. Kill avoids a
// fixed graceful-shutdown timeout while the shared Close path still Waits and
// reaps the process.
var platformSidecarStopPlan = sidecarStopPlan{kill: true}
