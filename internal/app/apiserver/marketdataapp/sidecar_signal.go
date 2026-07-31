package marketdataapp

import "os"

type sidecarProcessStopper interface {
	Signal(os.Signal) error
	Kill() error
}

type sidecarStopPlan struct {
	kill   bool
	signal os.Signal
}

func terminateSidecarProcess(process *os.Process) (bool, error) {
	return stopSidecarProcess(process, platformSidecarStopPlan)
}

func stopSidecarProcess(process sidecarProcessStopper, plan sidecarStopPlan) (bool, error) {
	if plan.kill {
		return true, process.Kill()
	}
	return false, process.Signal(plan.signal)
}
