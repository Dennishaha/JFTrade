//go:build !windows

package rustrehearsal

import (
	"errors"
	"os"
	"syscall"
)

func requestProcessStop(process *os.Process) (bool, error) {
	err := process.Signal(syscall.SIGTERM)
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return false, nil
	}
	return true, normalizeProcessStopError(process.Kill())
}
