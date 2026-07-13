package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

const (
	defaultBusinessThreshold = 90.0
	defaultCriticalThreshold = 95.0
	defaultModuleThreshold   = 85.0
	defaultTestTimeout       = 300 * time.Second
)

type config struct {
	businessThreshold float64
	criticalThreshold float64
	moduleThreshold   float64
	testTimeout       time.Duration
	packages          packageList
	tempDir           string
}

type packageList []string

func (p *packageList) String() string {
	return fmt.Sprint([]string(*p))
}

func (p *packageList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("package pattern must not be empty")
	}
	*p = append(*p, value)
	return nil
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}

	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "get working directory: %v\n", err)
		return 1
	}
	violations, err := executeCoverageCheck(cfg, workDir, stdout, stderr, execGoRunner{})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(violations) == 0 {
		return 0
	}

	fmt.Fprintln(stderr, "Coverage gate failed:")
	for _, violation := range violations {
		fmt.Fprintf(stderr, "- %s\n", violation)
	}
	return 1
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	cfg := config{}
	flags := flag.NewFlagSet("check-go-coverage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Float64Var(&cfg.businessThreshold, "business-threshold", defaultBusinessThreshold, "minimum business coverage percentage")
	flags.Float64Var(&cfg.criticalThreshold, "critical-threshold", defaultCriticalThreshold, "minimum critical package coverage percentage")
	flags.Float64Var(&cfg.moduleThreshold, "module-threshold", defaultModuleThreshold, "minimum ordinary package coverage percentage")
	flags.DurationVar(&cfg.testTimeout, "timeout", defaultTestTimeout, "timeout passed to go test")
	flags.Var(&cfg.packages, "package", "Go package pattern to test; may be repeated")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	if len(cfg.packages) == 0 {
		cfg.packages = packageList{"./..."}
	}
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintln(stderr, err)
		return config{}, err
	}
	return cfg, nil
}

func validateConfig(cfg config) error {
	thresholds := []struct {
		name  string
		value float64
	}{
		{"business-threshold", cfg.businessThreshold},
		{"critical-threshold", cfg.criticalThreshold},
		{"module-threshold", cfg.moduleThreshold},
	}
	for _, threshold := range thresholds {
		if math.IsNaN(threshold.value) || math.IsInf(threshold.value, 0) || threshold.value < 0 || threshold.value > 100 {
			return fmt.Errorf("-%s must be between 0 and 100", threshold.name)
		}
	}
	if cfg.testTimeout <= 0 {
		return errors.New("-timeout must be greater than zero")
	}
	return nil
}
