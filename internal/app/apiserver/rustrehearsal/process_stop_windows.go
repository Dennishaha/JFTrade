//go:build windows

package rustrehearsal

import "os"

// Windows has no portable graceful signal for an arbitrary child. Kill still
// feeds the shared Wait path so the process is always reaped.
func requestProcessStop(process *os.Process) (bool, error) {
	return true, normalizeProcessStopError(process.Kill())
}
