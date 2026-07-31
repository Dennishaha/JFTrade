package marketdataapp

import (
	"errors"
	"os"
	"testing"
)

func TestStopSidecarProcessAppliesSignalPlan(t *testing.T) {
	stopErr := errors.New("signal failed")
	process := &sidecarProcessStopperStub{signalErr: stopErr}
	signal := os.Interrupt

	killed, err := stopSidecarProcess(process, sidecarStopPlan{signal: signal})

	if killed || !errors.Is(err, stopErr) {
		t.Fatalf("signal stop result = killed %v, error %v", killed, err)
	}
	if process.killCalls != 0 || len(process.signals) != 1 || process.signals[0] != signal {
		t.Fatalf("signal stop calls = kills %d, signals %#v", process.killCalls, process.signals)
	}
}

func TestStopSidecarProcessAppliesImmediateKillPlan(t *testing.T) {
	stopErr := errors.New("kill failed")
	process := &sidecarProcessStopperStub{killErr: stopErr}

	killed, err := stopSidecarProcess(process, sidecarStopPlan{kill: true, signal: os.Interrupt})

	if !killed || !errors.Is(err, stopErr) {
		t.Fatalf("kill stop result = killed %v, error %v", killed, err)
	}
	if process.killCalls != 1 || len(process.signals) != 0 {
		t.Fatalf("kill stop calls = kills %d, signals %#v", process.killCalls, process.signals)
	}
}

type sidecarProcessStopperStub struct {
	signals   []os.Signal
	killCalls int
	signalErr error
	killErr   error
}

func (s *sidecarProcessStopperStub) Signal(signal os.Signal) error {
	s.signals = append(s.signals, signal)
	return s.signalErr
}

func (s *sidecarProcessStopperStub) Kill() error {
	s.killCalls++
	return s.killErr
}
