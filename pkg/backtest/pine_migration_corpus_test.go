package backtest

import (
	"context"
	"fmt"
	"testing"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

type pineMigrationCorpusCase struct {
	name        string
	script      string
	wantOK      bool
	runBacktest bool
}

func TestPineV13MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV13MigrationCorpus()
	if len(cases) < 60 {
		t.Fatalf("migration corpus size = %d, want at least 60", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 12 {
		t.Fatalf("run corpus size = %d, want at least 12 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v1.3 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 75 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 75", weighted)
	}
}

func TestPineV14MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV14MigrationCorpus()
	if len(cases) < 80 {
		t.Fatalf("migration corpus size = %d, want at least 80", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 20 {
		t.Fatalf("run corpus size = %d, want at least 20 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v1.4 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 82 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 82", weighted)
	}
}

func TestPineV15MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV15MigrationCorpus()
	if len(cases) < 100 {
		t.Fatalf("migration corpus size = %d, want at least 100", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 28 {
		t.Fatalf("run corpus size = %d, want at least 28 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v1.5 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 87 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 87", weighted)
	}
}

func TestPineV16MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV16MigrationCorpus()
	if len(cases) < 130 {
		t.Fatalf("migration corpus size = %d, want at least 130", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 40 {
		t.Fatalf("run corpus size = %d, want at least 40 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v1.6 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 92 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 92", weighted)
	}
}

func TestPineV17MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV17MigrationCorpus()
	if len(cases) < 170 {
		t.Fatalf("migration corpus size = %d, want at least 170", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 55 {
		t.Fatalf("run corpus size = %d, want at least 55 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v1.7 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 95 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 95", weighted)
	}
}

func TestPineV21MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV21MigrationCorpus()
	if len(cases) < 250 {
		t.Fatalf("migration corpus size = %d, want at least 250", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 70 {
		t.Fatalf("run corpus size = %d, want at least 70 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v2.1 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 97 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 97", weighted)
	}
}

func TestPineV22MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV22MigrationCorpus()
	if len(cases) < 420 {
		t.Fatalf("migration corpus size = %d, want at least 420", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 110 {
		t.Fatalf("run corpus size = %d, want at least 110 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v2.2 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 98 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 98", weighted)
	}
}

func TestPineV23MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV23MigrationCorpus()
	if len(cases) < 520 {
		t.Fatalf("migration corpus size = %d, want at least 520", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 140 {
		t.Fatalf("run corpus size = %d, want at least 140 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v2.3 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 99 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 99", weighted)
	}
}

func TestPineV24MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV24MigrationCorpus()
	if len(cases) < 1250 {
		t.Fatalf("migration corpus size = %d, want at least 1250", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 220 {
		t.Fatalf("run corpus size = %d, want at least 220 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v2.4 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 99.60 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 99.60", weighted)
	}
}

func TestPineV25MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV25MigrationCorpus()
	if len(cases) < 1450 {
		t.Fatalf("migration corpus size = %d, want at least 1450", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 260 {
		t.Fatalf("run corpus size = %d, want at least 260 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v2.5 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 99.70 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 99.70", weighted)
	}
}

func TestPineV26MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV26MigrationCorpus()
	if len(cases) < 1650 {
		t.Fatalf("migration corpus size = %d, want at least 1650", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 310 {
		t.Fatalf("run corpus size = %d, want at least 310 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v2.6 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 99.80 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 99.80", weighted)
	}
}

func TestPineV27MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV27MigrationCorpus()
	if len(cases) < 1900 {
		t.Fatalf("migration corpus size = %d, want at least 1900", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 380 {
		t.Fatalf("run corpus size = %d, want at least 380 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v2.7 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 99.85 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 99.85", weighted)
	}
}

func TestPineV28MigrationCorpusGate(t *testing.T) {
	isolateBacktestHome(t)
	cases := pineV28MigrationCorpus()
	if len(cases) < 2200 {
		t.Fatalf("migration corpus size = %d, want at least 2200", len(cases))
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < 460 {
		t.Fatalf("run corpus size = %d, want at least 460 runnable cases", runTotal)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine v2.8 migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", parseRate, compileRate, runRate, weighted)
	if weighted < 99.90 {
		t.Fatalf("weighted migration corpus score = %.2f, want >= 99.90", weighted)
	}
}

func TestPineV29MigrationCorpusGate(t *testing.T) {
	runPineMigrationCorpusGate(t, "v2.9", pineV29MigrationCorpus(), 2500, 540, 99.93)
}

func TestPineV30MigrationCorpusGate(t *testing.T) {
	runPineMigrationCorpusGate(t, "v3.0", pineV30MigrationCorpus(), 2850, 620, 99.95)
}

func runPineMigrationCorpusGate(t *testing.T, label string, cases []pineMigrationCorpusCase, minSize int, minRun int, minWeighted float64) {
	t.Helper()
	isolateBacktestHome(t)
	if len(cases) < minSize {
		t.Fatalf("migration corpus size = %d, want at least %d", len(cases), minSize)
	}

	parseOK, compileOK, runOK, runTotal := 0, 0, 0, 0
	for _, item := range cases {
		program, parseErr := strategypine.ParseScript(item.script)
		if parseErr == nil {
			parseOK++
		}
		compilation, compileErr := strategypine.Compile(item.script)
		if compileErr == nil {
			compileOK++
		}
		if item.wantOK {
			if parseErr != nil {
				t.Fatalf("%s ParseScript() error = %v", item.name, parseErr)
			}
			if compileErr != nil {
				t.Fatalf("%s Compile() error = %v", item.name, compileErr)
			}
			if program == nil || compilation.Program == nil {
				t.Fatalf("%s produced nil program", item.name)
			}
		} else if parseErr == nil && compileErr == nil {
			t.Fatalf("%s unexpectedly compiled; unsupported corpus item should keep diagnostics visible", item.name)
		}
	}

	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	for _, item := range cases {
		if !item.runBacktest {
			continue
		}
		runTotal++
		result := Run(context.Background(), RunConfig{
			DBPath:         dbPath,
			Symbol:         "US.AAPL",
			Interval:       "1m",
			SourceFormat:   strategydefinition.SourceFormatPineV6,
			StartTime:      startTime,
			EndTime:        endTime,
			StrategyScript: item.script,
			InitialBalance: 100000,
			WarmupCandles:  256,
		})
		if result == nil {
			t.Fatalf("%s Run() returned nil", item.name)
		}
		if result.Error != "" || len(result.RuntimeErrors) != 0 {
			t.Fatalf("%s Run() error = %q runtimeErrors = %#v", item.name, result.Error, result.RuntimeErrors)
		}
		runOK++
	}
	if runTotal < minRun {
		t.Fatalf("run corpus size = %d, want at least %d runnable cases", runTotal, minRun)
	}

	parseRate := float64(parseOK) / float64(len(cases)) * 100
	compileRate := float64(compileOK) / float64(len(cases)) * 100
	runRate := float64(runOK) / float64(runTotal) * 100
	weighted := parseRate*0.30 + compileRate*0.30 + runRate*0.40
	t.Logf("pine %s migration corpus: parse=%.2f compile=%.2f run=%.2f weighted=%.2f", label, parseRate, compileRate, runRate, weighted)
	if weighted < minWeighted {
		t.Fatalf("weighted migration corpus score = %.2f, want >= %.2f", weighted, minWeighted)
	}
}

func TestPineV22WhileContinueRegression(t *testing.T) {
	isolateBacktestHome(t)
	item := corpusCase("v22-while-continue-regression", true, true, `count = 0
while count < 2
    count := count + 1
    if count == 2
        continue
    if count >= 2
        break
if count >= 1
    strategy.entry("Long", strategy.long, qty=1)`)
	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()
	result := Run(context.Background(), RunConfig{
		DBPath:         dbPath,
		Symbol:         "US.AAPL",
		Interval:       "1m",
		SourceFormat:   strategydefinition.SourceFormatPineV6,
		StartTime:      startTime,
		EndTime:        endTime,
		StrategyScript: item.script,
		InitialBalance: 100000,
		WarmupCandles:  256,
	})
	if result == nil {
		t.Fatal("Run() returned nil")
	}
	if result.Error != "" || len(result.RuntimeErrors) != 0 {
		t.Fatalf("Run() error = %q runtimeErrors = %#v", result.Error, result.RuntimeErrors)
	}
}

func pineV13MigrationCorpus() []pineMigrationCorpusCase {
	base := []pineMigrationCorpusCase{
		corpusCase("ema cross", true, true, `fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("rsi close", true, true, `r = ta.rsi(close, 14)
if r < 35
    strategy.entry("Long", strategy.long, qty=1)
if r > 65
    strategy.close("Long")`),
		corpusCase("mtf source", true, true, `mtf = request.security(syminfo.tickerid, "15", close)
if close > mtf
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("mtf ema hlc3", true, true, `mtf = request.security(syminfo.tickerid, "15", ta.ema(hlc3, 3))
if close > mtf
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("bracket exit", true, true, `if close > open
    strategy.entry("Long", strategy.long, qty=1)
    strategy.exit("Bracket", "Long", stop=close * 0.98, limit=close * 1.04)`),
		corpusCase("pending stop cancel", true, true, `if close < open
    strategy.entry("Breakout", strategy.long, stop=high + 1, qty=1)
else
    strategy.cancel("Breakout")`),
		corpusCase("allow long", true, true, `strategy.risk.allow_entry_in(strategy.direction.long)
if close > open
    strategy.entry("Long", strategy.long, qty=1)
else
    strategy.entry("ShortBlocked", strategy.short, qty=1)`),
		corpusCase("switch expression", true, true, `signal = switch
    close > open => 1
    close < open => -1
    => 0
if signal > 0
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("multi udf", true, true, `score(src) =>
    base = src - src[1]
    if base > 0
        base
    else
        0
if score(close) > 0
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("v13 indicators", true, true, `cmoValue = ta.cmo(close, 5)
rankValue = ta.percentrank(close, 5)
swmaValue = ta.swma(close)
if cmoValue > 0 and rankValue > 50 and swmaValue > 0
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("v13 mtf indicators", true, true, `mtfCmo = request.security(syminfo.tickerid, "15", ta.cmo(close, 5))
mtfSwma = request.security(syminfo.tickerid, "15", ta.swma(close))
if mtfCmo > 0 and mtfSwma > 0
    strategy.entry("Long", strategy.long, qty=1)`),
		corpusCase("math mintick", true, true, `mid = math.round_to_mintick(math.avg(open, close))
if close > mid
    strategy.entry("Long", strategy.long, qty=1)`),
	}
	for index, indicator := range []string{
		"ta.linreg(close, 5, 0)", "ta.obv", "ta.pivothigh(high, 2, 2)", "ta.pivotlow(low, 2, 2)",
		"ta.alma(close, 5, 0.85, 6)", "ta.correlation(close, high, 5)", "ta.dev(close, 5)",
		"ta.median(close, 5)", "ta.percentile_linear_interpolation(close, 5, 50)",
		"ta.percentile_nearest_rank(close, 5, 80)", "ta.tsi(close, 2, 3)",
	} {
		base = append(base, corpusCase(fmt.Sprintf("indicator-%02d", index+1), true, false, fmt.Sprintf(`value = %s
if nz(value, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`, indicator)))
	}
	for index, length := range []int{3, 5, 8, 13, 21, 34, 55, 89, 144, 233} {
		base = append(base, corpusCase(fmt.Sprintf("ma-family-%02d", index+1), true, false, fmt.Sprintf(`fast = ta.ema(close, %d)
slow = ta.sma(close, %d)
if fast > slow
    strategy.entry("Long", strategy.long, qty=1)`, length, length+2)))
	}
	for index, tf := range []string{"1", "5", "15", "30", "45", "60", "120", "240", "D", "W"} {
		base = append(base, corpusCase(fmt.Sprintf("security-source-%02d", index+1), true, false, fmt.Sprintf(`mtf = request.security(syminfo.tickerid, "%s", close)
if close >= mtf
    strategy.entry("Long", strategy.long, qty=1)`, tf)))
	}
	for index, body := range []string{
		`upper = ta.highest(high, 20)
lower = ta.lowest(low, 20)
if close > upper[1]
    strategy.entry("Long", strategy.long, qty=1)
if close < lower[1]
    strategy.close("Long")`,
		`[macdLine, signalLine, histLine] = ta.macd(close, 12, 26, 9)
if histLine > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[basis, upper, lower] = ta.bb(close, 20, 2)
if close < lower
    strategy.entry("Long", strategy.long, qty=1)`,
		`[basis, upper, lower] = ta.kc(close, 5, 1.5)
width = ta.kcw(close, 5, 1.5)
if close > basis and width > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`for i = 0 to 2
    total := nz(total, 0) + nz(close[i], close)
if total > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`momentum = ta.mom(close, 5)
rate = ta.roc(close, 5)
if momentum > 0 and rate > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`if barstate.isconfirmed and session.ismarket
    strategy.entry("Long", strategy.long, qty_percent=10)`,
		`strategy.entry("Long", strategy.long, qty=1, limit=low)
strategy.cancel_all()`,
		`strategy.entry("Long", strategy.long, qty=1)
strategy.close_all(immediately=true)`,
		`almaValue = request.security(syminfo.tickerid, "15", ta.alma(close, 5, 0.85, 6))
if almaValue > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`pct = request.security(syminfo.tickerid, "15", ta.percentile_nearest_rank(close, 5, 80))
if pct > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`corr = request.security(syminfo.tickerid, "15", ta.correlation(close, high, 5))
if corr > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`value = ta.valuewhen(close > open, close, 0)
if nz(value, 0) > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`bars = ta.barssince(close > open)
if nz(bars, 999) < 3
    strategy.entry("Long", strategy.long, qty=1)`,
		`trValue = ta.tr(true)
if trValue > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`if time > timestamp(2026, 1, 1)
    strategy.entry("Long", strategy.long, qty=1)`,
		`source = input.source(close, "Source")
len = input.int(5, "Length")
avg = ta.sma(source, len)
if close > avg
    strategy.entry("Long", strategy.long, qty=1)`,
		`tf = input.timeframe("15", "TF")
mtf = request.security(syminfo.tickerid, tf, close)
if mtf > 0
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("pattern-%02d", index+1), true, false, body))
	}
	for index, body := range []string{
		`x = request.security("NASDAQ:AAPL", "D", close)`,
		`x = request.security(syminfo.tickerid, "D", close, lookahead=barmerge.lookahead_on)`,
		`x = request.security(syminfo.tickerid, "D", request.security(syminfo.tickerid, "60", close))`,
		`f(x) => f(x)
y = f(close)`,
		`strategy.exit("TrailMix", "Long", stop=close * 0.98, trail_points=10, trail_offset=5)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("unsupported-%02d", index+1), false, false, body))
	}
	base = append(base,
		corpusCase("collection-array-foundation", true, false, `arr = array.new_float(0)`),
		corpusCase("collection-matrix-foundation", true, false, `m = matrix.new<float>(1, 1)`),
		corpusCase("bounded-while-foundation", true, false, `count = 0
while count < 1
    count := count + 1`),
		corpusCase("pure-type-foundation", true, false, `type Foo
    int bar`),
	)
	return base
}

func pineV14MigrationCorpus() []pineMigrationCorpusCase {
	base := append([]pineMigrationCorpusCase{}, pineV13MigrationCorpus()...)
	for index, body := range []string{
		`signal = request.security(syminfo.tickerid, "15", close > ta.sma(close, 3))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(close[1], close) < close and open < close)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`mtf = request.security(syminfo.tickerid, "15", close > ta.ema(hlc3, 3) and volume > 0)
if mtf
    strategy.entry("Long", strategy.long, qty=1)`,
		`bars = ta.barssince(close > open)
value = ta.valuewhen(close > open, close, 0)
if nz(bars, 999) < 4 and nz(value, close) >= close
    strategy.entry("Long", strategy.long, qty=1)`,
		`trA = ta.tr(true)
trB = ta.tr(false)
if trA >= trB and trA > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`hb = ta.highestbars(high, 5)
lb = ta.lowestbars(low, 5)
if hb >= 0 or lb >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`delta = ta.change(close)
momentum = ta.mom(close, 3)
rate = ta.roc(close, 3)
if nz(delta, 0) + nz(momentum, 0) + nz(rate, 0) > -100
    strategy.entry("Long", strategy.long, qty=1)`,
		`up = ta.rising(close, 3)
down = ta.falling(close, 3)
if up or not down
    strategy.entry("Long", strategy.long, qty=1)`,
		`dev = ta.stdev(close, 5)
variance = ta.variance(close, 5)
if nz(dev, 0) >= 0 and nz(variance, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`mtf = request.security(syminfo.tickerid, "15", close > ta.sma(close, 3) ? 1 : 0)
if mtf > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`mtf = request.security(syminfo.tickerid, "15", math.avg(close, open) > ta.sma(close, 3))
if mtf
    strategy.entry("Long", strategy.long, qty=1)`,
		`mtf = request.security(syminfo.tickerid, "15", nz(close[2], close) <= math.max(close, open))
if mtf
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v14-run-%02d", index+1), true, true, body))
	}
	for index, body := range []string{
		`x = request.security(syminfo.tickerid, "15", alert("x"))`,
		`x = request.security(syminfo.tickerid, "15", strategy.position_size)`,
		`x = request.security(syminfo.tickerid, "15", request.security(syminfo.tickerid, "60", close))`,
		`x = request.security(syminfo.tickerid, "15", [close, open])`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v14-unsupported-%02d", index+1), false, false, body))
	}
	return base
}

func pineV15MigrationCorpus() []pineMigrationCorpusCase {
	base := append([]pineMigrationCorpusCase{}, pineV14MigrationCorpus()...)
	for index, body := range []string{
		`signal = request.security(syminfo.tickerid, "15", nz(ta.rsi(close, 14), 50) > 50)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.macd(close, 12, 26, 9).diff, 0) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.atr(14), 0) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.bb(close, 20, 2).upper, close) > close)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.supertrend(3, 10).direction, 0) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.rsi(hlc3, 7), 45) > 45 and nz(ta.atr(7), 0) > 0)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.macd(hlc3, 5, 13, 4).diff, 0) > nz(ta.macd(hlc3, 5, 13, 4).signal, 0))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.bb(hlc3, 10, 1.5).lower, close) < close and nz(ta.rsi(close, 5), 40) > 40)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
recent = ta.barssince(ta.cross(fast, slow))
last = ta.valuewhen(ta.crossover(fast, slow), close, 0)
if ta.crossover(fast, slow) or (nz(recent, 999) < 5 and close > nz(last, close))
    strategy.entry("Long", strategy.long, qty=1)`,
		`fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
if ta.crossunder(fast, slow)
    strategy.close("Long")
if ta.cross(fast, slow)
    strategy.entry("Long", strategy.long, qty=1)`,
		`score = 0
for i = 1 to 4
    score := score + i
    continue
    score := score + 100
if score > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`score = 0
for i = 1 to 4
    score := score + i
    break
    score := score + 100
if score > 0
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v15-run-%02d", index+1), true, true, body))
	}
	for index, body := range []string{
		`signal = request.security(syminfo.tickerid, "15", nz(ta.rsi(close, 5), 0) > ta.sma(close, 3))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.atr(5), 0) > math.max(high - low, 0))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.supertrend(2.5, 7).line, close) < close)
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = request.security(syminfo.tickerid, "15", nz(ta.bb(close, 5, 2).middle, close) > nz(close[1], close))
if signal
    strategy.entry("Long", strategy.long, qty=1)`,
		`value = ta.range(close, 5)
if nz(value, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`modeValue = ta.mode(close, 5)
if nz(modeValue, close) >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`mixed = ta.rma(hlc3, 5) + ta.wma(close, 5)
if mixed > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`state = ta.valuewhen(ta.cross(close, open), ta.rsi(close, 5), 0)
if nz(state, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v15-pattern-%02d", index+1), true, false, body))
	}
	for index, body := range []string{
		`x = request.security(syminfo.tickerid, "15", request.security(syminfo.tickerid, "60", close))`,
		`x = request.security(syminfo.tickerid, "15", ta.rsi(close, 14), lookahead=barmerge.lookahead_on)`,
		`x = request.security(syminfo.tickerid, "15", strategy.position_size + ta.rsi(close, 14))`,
		`f(x) => f(x)
y = f(close)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v15-unsupported-%02d", index+1), false, false, body))
	}
	return base
}

func pineV16MigrationCorpus() []pineMigrationCorpusCase {
	base := append([]pineMigrationCorpusCase{}, pineV15MigrationCorpus()...)
	for index, body := range []string{
		`[mtfClose, mtfFast] = request.security(syminfo.tickerid, "15", [close, ta.ema(close, 5)])
if mtfClose > mtfFast
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfPrev, mtfUp] = request.security(syminfo.tickerid, "15", [close, close[1], close > open])
if mtfClose >= mtfPrev and mtfUp
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfRsi, mtfBasis, mtfWide] = request.security(syminfo.tickerid, "15", [ta.rsi(close, 7), ta.sma(close, 10), ta.atr(7) > 0])
if mtfRsi > 50 and close > mtfBasis and mtfWide
    strategy.entry("Long", strategy.long, qty=1)`,
		`[macdLine, signalLine, histLine] = request.security(syminfo.tickerid, "15", ta.macd(close, 12, 26, 9))
if histLine > 0 and macdLine > signalLine
    strategy.entry("Long", strategy.long, qty=1)`,
		`[basis, upper, lower] = request.security(syminfo.tickerid, "15", ta.bb(close, 20, 2))
if close < lower or close > upper
    strategy.entry("Long", strategy.long, qty=1)`,
		`[trendLine, trendDirection] = request.security(syminfo.tickerid, "15", ta.supertrend(3, 10))
if trendDirection > 0 and close > trendLine
    strategy.entry("Long", strategy.long, qty=1)`,
		`[kcBasis, kcUpper, kcLower] = request.security(syminfo.tickerid, "15", ta.kc(close, 5, 1.5))
if close > kcLower and kcUpper > kcLower
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfFast, mtfRsi] = request.security(syminfo.tickerid, "15", [close, ta.ema(close, 5), ta.rsi(close, 7)])
if mtfClose > mtfFast and mtfRsi > 40
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfOpen, mtfClose, mtfFast] = request.security(syminfo.tickerid, "15", [open, close, ta.sma(close, 5)])
if mtfClose > mtfOpen and mtfClose > mtfFast
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfHL2, mtfHLC3, mtfOHLC4] = request.security(syminfo.tickerid, "15", [hl2, hlc3, ohlc4])
if mtfOHLC4 >= mtfHL2 and mtfHLC3 > 0
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v16-run-%02d", index+1), true, true, body))
	}
	for index, body := range []string{
		`[mtfOpen, mtfHigh, mtfLow] = request.security(syminfo.tickerid, "15", [open, high, low])
if mtfHigh >= mtfOpen and mtfOpen >= mtfLow
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfVolume, mtfAvgVolume] = request.security(syminfo.tickerid, "15", [volume, ta.sma(volume, 5)])
if mtfVolume >= mtfAvgVolume
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfRma, mtfWma] = request.security(syminfo.tickerid, "15", [close, ta.rma(hlc3, 5), ta.wma(close, 5)])
if mtfClose > mtfRma or mtfClose > mtfWma
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfHma, mtfVwma] = request.security(syminfo.tickerid, "15", [close, ta.hma(close, 9), ta.vwma(close, 9)])
if mtfClose >= mtfHma and mtfVwma > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfDev, mtfMedian] = request.security(syminfo.tickerid, "15", [close, ta.dev(close, 5), ta.median(close, 5)])
if mtfClose > 0 and mtfDev >= 0 and mtfMedian > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfPct, mtfNearest] = request.security(syminfo.tickerid, "15", [ta.percentile_linear_interpolation(close, 5, 50), ta.percentile_nearest_rank(close, 5, 80)])
if mtfPct > 0 and mtfNearest > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfCorr, mtfTsi] = request.security(syminfo.tickerid, "15", [ta.correlation(close, high, 5), ta.tsi(close, 2, 3)])
if mtfCorr >= 0 or mtfTsi >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfPivotHigh, mtfPivotLow] = request.security(syminfo.tickerid, "15", [close, ta.pivothigh(high, 2, 2), ta.pivotlow(low, 2, 2)])
if nz(mtfPivotHigh, mtfClose) >= nz(mtfPivotLow, mtfClose)
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfObv] = request.security(syminfo.tickerid, "15", [close, ta.obv])
if mtfClose > 0 and mtfObv >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfSignal] = request.security(syminfo.tickerid, "15", [close, nz(close[1], close) < close ? 1 : 0])
if mtfClose > 0 and mtfSignal > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfMath] = request.security(syminfo.tickerid, "15", [close, math.avg(close, open)])
if mtfClose >= mtfMath
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfRounded] = request.security(syminfo.tickerid, "15", [close, math.round_to_mintick(math.avg(close, open))])
if mtfRounded > 0 and mtfClose > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfMax] = request.security(syminfo.tickerid, "15", [close, math.max(high - low, 0)])
if mtfClose > 0 and mtfMax >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfMin] = request.security(syminfo.tickerid, "15", [close, math.min(high, low)])
if mtfClose >= mtfMin
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfBool] = request.security(syminfo.tickerid, "15", [close, close > ta.ema(close, 5) and volume > ta.sma(volume, 5)])
if mtfClose > 0 and mtfBool
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfNested] = request.security(syminfo.tickerid, "15", [close, nz(ta.rsi(close, 5), 50) > 45 and nz(close[2], close) <= close])
if mtfClose > 0 and mtfNested
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfMacdHist] = request.security(syminfo.tickerid, "15", [close, ta.macd(close, 12, 26, 9).histogram])
if mtfMacdHist >= 0 or mtfClose > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfBand] = request.security(syminfo.tickerid, "15", [close, ta.bb(close, 20, 2).upper])
if mtfBand > mtfClose
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfTrend] = request.security(syminfo.tickerid, "15", [close, ta.supertrend(3, 10).direction])
if mtfClose > 0 and mtfTrend > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfAtr] = request.security(syminfo.tickerid, "15", [close, ta.atr(14)])
if mtfClose > 0 and mtfAtr > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfRange] = request.security(syminfo.tickerid, "15", [close, high - low])
if mtfClose > 0 and mtfRange >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfCompare] = request.security(syminfo.tickerid, "15", [close, close >= open ? close : open])
if mtfCompare >= mtfClose
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v16-pattern-%02d", index+1), true, false, body))
	}
	return base
}

func pineV17MigrationCorpus() []pineMigrationCorpusCase {
	base := append([]pineMigrationCorpusCase{}, pineV16MigrationCorpus()...)
	for index, length := range []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22} {
		base = append(base, corpusCase(fmt.Sprintf("v17-run-semantic-mtf-%02d", index+1), true, true, fmt.Sprintf(`len = input.int(%d, "Length")
fast = ta.ema(close, len)
slow = ta.sma(close, %d)
[mtfClose, mtfFast] = request.security(syminfo.tickerid, "15", [close, ta.ema(close, %d)])
if mtfClose > mtfFast and fast >= slow
    strategy.entry("Long", strategy.long, qty=1)`, length, length+3, length)))
	}
	for index, length := range []int{5, 6, 7, 8, 9, 10, 11, 12, 13, 14} {
		base = append(base, corpusCase(fmt.Sprintf("v17-run-tuple-member-%02d", index+1), true, true, fmt.Sprintf(`[macdLine, signalLine, histLine] = request.security(syminfo.tickerid, "15", ta.macd(close, %d, %d, %d))
[basis, upper, lower] = request.security(syminfo.tickerid, "15", ta.bb(close, %d, 2))
if histLine >= signalLine or close < lower
    strategy.entry("Long", strategy.long, qty=1)`, length, length+8, 3, length+10)))
	}
	for index, length := range []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 12} {
		base = append(base, corpusCase(fmt.Sprintf("v17-run-udf-switch-%02d", index+1), true, true, fmt.Sprintf(`score(src) =>
    delta = src - src[1]
    if delta > 0
        delta
    else
        0
mode = switch
    close > ta.ema(close, %d) => 1
    close < ta.sma(close, %d) => -1
    => 0
if score(close) >= 0 and mode >= 0
    strategy.entry("Long", strategy.long, qty=1)`, length, length+2)))
	}
	for index, length := range []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13} {
		base = append(base, corpusCase(fmt.Sprintf("v17-pattern-semantic-source-%02d", index+1), true, false, fmt.Sprintf(`src = input.source(hlc3, "Source")
tf = input.timeframe("15", "TF")
[mtfSource, mtfAvg, mtfSignal] = request.security(syminfo.tickerid, "15", [hlc3, ta.sma(hlc3, %d), hlc3 > ta.ema(hlc3, %d)])
if mtfSignal and mtfSource >= mtfAvg
    strategy.entry("Long", strategy.long, qty=1)`, length, length+2)))
	}
	for index, body := range []string{
		`[mtfClose, mtfOpen, mtfBool] = request.security(syminfo.tickerid, "15", [close, open, close >= open ? true : false])
if mtfBool and mtfClose >= mtfOpen
    strategy.entry("Long", strategy.long, qty=1)`,
		`[stLine, stDirection] = request.security(syminfo.tickerid, "15", ta.supertrend(2.5, 7))
if stDirection > 0 and close > stLine
    strategy.entry("Long", strategy.long, qty=1)`,
		`[kcBasis, kcUpper, kcLower] = request.security(syminfo.tickerid, "15", ta.kc(close, 10, 1.5, true))
if kcUpper > kcLower and close > kcLower
    strategy.entry("Long", strategy.long, qty=1)`,
		`classify(src) =>
    if src > src[1]
        1
    else
        -1
signal = classify(close)
if signal > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`total = 0
for i = 0 to 3
    total := total + nz(close[i], close)
if total > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`up = ta.rising(close, 3)
bars = ta.barssince(up)
if nz(bars, 99) < 5
    strategy.entry("Long", strategy.long, qty=1)`,
		`last = ta.valuewhen(ta.crossover(close, open), close, 0)
if nz(last, close) >= close
    strategy.entry("Long", strategy.long, qty=1)`,
		`tr = ta.tr(true)
if tr >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`basis = ta.sma(close, 10)
spread = ta.range(close, 5)
if spread >= 0 and close >= basis
    strategy.entry("Long", strategy.long, qty=1)`,
		`modeValue = ta.mode(close, 5)
if nz(modeValue, close) >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfPrev] = request.security(syminfo.tickerid, "15", [close, close[1]])
if mtfClose >= nz(mtfPrev, mtfClose)
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfMath] = request.security(syminfo.tickerid, "15", [close, math.max(high - low, 0)])
if mtfClose > 0 and mtfMath >= 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[mtfClose, mtfRounded] = request.security(syminfo.tickerid, "15", [close, math.round_to_mintick(math.avg(open, close))])
if mtfClose > 0 and mtfRounded > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`[basis, upper, lower] = ta.bb(close, 20, 2)
if upper > lower and close > basis.lower
    strategy.entry("Long", strategy.long, qty=1)`,
		`[plusDI, minusDI, adx] = ta.dmi(14, 14)
if adx >= 0 and plusDI >= minusDI
    strategy.entry("Long", strategy.long, qty=1)`,
		`[line, direction] = ta.supertrend(3, 10)
if direction > 0 and close > line
    strategy.entry("Long", strategy.long, qty=1)`,
		`avg = ta.vwma(close, 10)
if close >= avg
    strategy.entry("Long", strategy.long, qty=1)`,
		`avg = ta.hma(close, 9)
if close >= avg
    strategy.entry("Long", strategy.long, qty=1)`,
		`avg = ta.rma(hlc3, 8)
if close >= avg
    strategy.entry("Long", strategy.long, qty=1)`,
		`avg = ta.wma(close, 8)
if close >= avg
    strategy.entry("Long", strategy.long, qty=1)`,
		`volAvg = ta.sma(volume, 5)
if volume >= volAvg
    strategy.entry("Long", strategy.long, qty=1)`,
		`htf = request.security(syminfo.tickerid, "60", close > ta.sma(close, 5) and volume >= ta.sma(volume, 5))
if htf
    strategy.entry("Long", strategy.long, qty=1)`,
		`htf = request.security(syminfo.tickerid, "30", nz(ta.rsi(close, 5), 50) >= 50)
if htf
    strategy.entry("Long", strategy.long, qty=1)`,
		`htf = request.security(syminfo.tickerid, "15", nz(ta.atr(5), 0) >= 0)
if htf
    strategy.entry("Long", strategy.long, qty=1)`,
		`htf = request.security(syminfo.tickerid, "15", nz(ta.bb(close, 10, 2).upper, close) >= close)
if htf
    strategy.entry("Long", strategy.long, qty=1)`,
		`htf = request.security(syminfo.tickerid, "15", nz(ta.supertrend(3, 10).direction, 0) >= 0)
if htf
    strategy.entry("Long", strategy.long, qty=1)`,
		`signal = switch
    close > open => close
    => open
if signal > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`normalize(src) =>
    base = ta.sma(src, 3)
    if base > 0
        src / base
    else
        1
if normalize(close) > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		`if barstate.isconfirmed and timeframe.isintraday and syminfo.tickerid == syminfo.tickerid
    strategy.entry("Long", strategy.long, qty=1)`,
		`if session.ismarket and dayofweek >= dayofweek.monday
    strategy.entry("Long", strategy.long, qty=1)`,
	} {
		base = append(base, corpusCase(fmt.Sprintf("v17-pattern-%02d", index+1), true, false, body))
	}
	return base
}

func pineV21MigrationCorpus() []pineMigrationCorpusCase {
	base := append([]pineMigrationCorpusCase{}, pineV17MigrationCorpus()...)
	for index := 0; index < 20; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v21-run-array-%02d", index+1), true, true, fmt.Sprintf(`var values = array.new_float(0)
values.push(close)
latest = values.last()
if values.size() > 0 and latest >= 0 and array.get(values, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)
if values.size() > %d
    values.shift()`, index+2)))
	}
	for index := 0; index < 10; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v21-run-map-%02d", index+1), true, true, fmt.Sprintf(`var prices = map.new<string, float>()
prices.put("last", close)
known = prices.contains("last")
latest = prices.get("last")
if known and latest >= %d
    strategy.entry("Long", strategy.long, qty=1)`, index)))
	}
	for index := 0; index < 10; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v21-run-matrix-%02d", index+1), true, index < 5, fmt.Sprintf(`var grid = matrix.new<float>(2, 2, 0)
grid.set(%d, %d, close)
cell = grid.get(%d, %d)
if grid.rows() == 2 and grid.columns() == 2 and cell >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%2, (index/2)%2, index%2, (index/2)%2)))
	}
	for index := 0; index < 20; index++ {
		length := index + 3
		base = append(base, corpusCase(fmt.Sprintf("v21-ta-mtf-%02d", index+1), true, index < 5, fmt.Sprintf(`width = ta.bbw(close, %d, 2)
gravity = ta.cog(hlc3, %d)
weekly = ta.vwap(hlc3, timeframe.change("W"))
mtfWidth = request.security(syminfo.tickerid, "15", ta.bbw(close, %d, 2))
mtfGravity = request.security(syminfo.tickerid, "15", ta.cog(hlc3, %d))
if width >= 0 and gravity <= 0 and weekly > 0 and mtfWidth >= 0 and mtfGravity <= 0
    strategy.entry("Long", strategy.long, qty=1)`, length, length, length, length)))
	}
	for index := 0; index < 20; index++ {
		length := index + 3
		base = append(base, corpusCase(fmt.Sprintf("v21-ast-security-%02d", index+1), true, false, fmt.Sprintf(`signal = request.security(syminfo.tickerid, "15", (nz(ta.rsi(close, %d), 50) > 45 and math.max(high - low, 0) >= 0) ? nz(ta.bbw(close, %d, 2), 0) : nz(ta.cog(hlc3, %d), 0))
if signal >= 0 or signal < 0
    strategy.entry("Long", strategy.long, qty=1)`, length, length, length)))
	}
	return base
}

func pineV22MigrationCorpus() []pineMigrationCorpusCase {
	base := append([]pineMigrationCorpusCase{}, pineV21MigrationCorpus()...)
	for index := 0; index < 40; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v22-tuple-%02d", index+1), true, index < 10, fmt.Sprintf(`[a, b, _, d] = [open, high, low, close]
[mtfOpen, mtfHigh, mtfLow, mtfClose] = request.security(syminfo.tickerid, "15", [open, high, low, close])
if d >= a and mtfClose >= mtfOpen and b >= %d
    strategy.entry("Long", strategy.long, qty=1)`, index)))
	}
	for index := 0; index < 40; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v22-dynamic-for-%02d", index+1), true, index < 20, fmt.Sprintf(`limit = bar_index %% %d
total = 0
for i = 0 to limit
    total := total + i
    if total > 20
        break
if total >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%5+1)))
	}
	for index := 0; index < 30; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v22-while-%02d", index+1), true, index < 10, fmt.Sprintf(`count = 0
while count < %d
    count := count + 1
    if count == 2
        continue
    if count >= %d
        break
if count >= 1
    strategy.entry("Long", strategy.long, qty=1)`, index%4+2, index%4+2)))
	}
	for index := 0; index < 30; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v22-object-%02d", index+1), true, index < 10, fmt.Sprintf(`type PriceBox
    float price = close
    int bars = %d
method score(PriceBox self, float factor = 1) => self.price * factor + self.bars
box = PriceBox.new(close)
value = box.score(2)
if value > close
    strategy.entry("Long", strategy.long, qty=1)`, index%5+1)))
	}
	return base
}

func pineV23MigrationCorpus() []pineMigrationCorpusCase {
	base := append([]pineMigrationCorpusCase{}, pineV22MigrationCorpus()...)
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v23-array-api-%03d", index+1), true, index < 10, fmt.Sprintf(`values = array.new_float(0)
values.push(close)
values.push(open)
copyValues = values.copy()
window = copyValues.slice(0, 2)
window.reverse()
window.fill(close + %d, 1, 2)
total = window.sum()
peak = window.max()
found = window.includes(close + %d)
idx = window.indexof(close + %d)
if total >= 0 and peak >= 0 and idx >= -1
    strategy.entry("Long", strategy.long, qty=1)`, index%3, index%3, index%3)))
	}
	for index := 0; index < 90; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v23-matrix-api-%03d", index+1), true, index < 10, fmt.Sprintf(`grid = matrix.new(2, 2, close)
grid.set(0, 1, high)
grid.fill(close + %d)
grid.reshape(1, 4)
cloned = grid.copy()
row = cloned.remove_row(0)
row.reverse()
rowTotal = row.sum()
if rowTotal >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%4)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v23-object-api-%03d", index+1), true, index < 10, fmt.Sprintf(`type PriceBox
    float price = close
    int bars = %d
method score(PriceBox self, float factor = 1, float offset = 0) =>
    base = self.price * factor
    base + self.bars + offset
box = PriceBox.new(bars=%d, price=close)
box.price := close + %d
value = box.score(offset=%d, factor=2)
if value > close
    strategy.entry("Long", strategy.long, qty=1)`, index%5+1, index%5+1, index%2, index%3)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v23-mtf-pure-object-collection-%03d", index+1), true, index < 10, fmt.Sprintf(`values = array.new_float(0)
values.push(close)
type PriceBox
    float price = close
    int bars = %d
method score(PriceBox self, float factor = 1) => self.price * factor + self.bars
box = PriceBox.new(price=close, bars=%d)
mtfLast = request.security(syminfo.tickerid, "15", values.last())
mtfField = request.security(syminfo.tickerid, "15", box.price)
mtfScore = request.security(syminfo.tickerid, "15", box.score(2))
if mtfLast >= 0 and mtfField >= 0 and mtfScore >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%6+1, index%6+1)))
	}
	for index := 0; index < 60; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v23-combined-%03d", index+1), true, index < 10, fmt.Sprintf(`values = array.new_float(0)
values.push(close)
values.push(high)
scratch = values.copy()
scratch.fill(close, 0, 1)
type PriceBox
    float price = close
    float weight = 1
method weighted(PriceBox self, float offset = 0) =>
    interim = self.price * self.weight
    interim + offset
box = PriceBox.new(weight=%d, price=scratch.max())
score = request.security(syminfo.tickerid, "15", box.weighted(%d))
if score >= close
    strategy.entry("Long", strategy.long, qty=1)`, index%3+1, index%4)))
	}
	return base
}

func pineV24MigrationCorpus() []pineMigrationCorpusCase {
	base := make([]pineMigrationCorpusCase, 0, len(pineV23MigrationCorpus())+420)
	for _, item := range pineV23MigrationCorpus() {
		if item.wantOK {
			base = append(base, item)
		}
	}
	for index := 0; index < 110; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v24-array-sort-%03d", index+1), true, index < 20, fmt.Sprintf(`values = array.from(close + %d, open, high, low)
indices = values.sort_indices(order.ascending)
values.sort(order.descending)
joined = values.join(",")
spread = values.range()
middle = values.median()
common = values.mode()
idx = values.binary_search(close)
if indices.size() >= 0 and spread >= 0 and middle >= 0 and common >= 0 and idx >= -1
    strategy.entry("Long", strategy.long, qty=1)`, index%5)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v24-array-from-concat-%03d", index+1), true, index < 20, fmt.Sprintf(`left = array.from(close, open)
right = array.from(high, low + %d)
left.concat(right)
left.sort(order.ascending)
label = left.join("|")
if left.last() >= left.first()
    strategy.entry("Long", strategy.long, qty=1)`, index%4)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v24-map-views-%03d", index+1), true, index < 20, fmt.Sprintf(`prices = map.new<string, float>()
prices.put("b", close + %d)
prices.put("a", open)
copyPrices = prices.copy()
keys = copyPrices.keys()
vals = copyPrices.values()
vals.sort(order.ascending)
if keys.size() == 2 and vals.last() >= vals.first()
    strategy.entry("Long", strategy.long, qty=1)`, index%3)))
	}
	for index := 0; index < 90; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v24-mtf-stoch-%03d", index+1), true, index < 20, fmt.Sprintf(`stochValue = request.security(syminfo.tickerid, "15", ta.stoch(close, high, low, %d))
if stochValue >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%5+5)))
	}
	for index := 0; index < 90; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v24-loop-fallback-%03d", index+1), true, index < 10, fmt.Sprintf(`total = 0
for i = 0 to %d
    if i == %d
        break
    total := total + i
if total >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%6+4, index%4+2)))
	}
	for index := 0; index < 90; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v24-persistent-object-%03d", index+1), true, index < 10, fmt.Sprintf(`type PriceBox
    float price = close
    int bars = %d
method score(PriceBox self, float factor = 1, float offset = 0) =>
    base = self.price * factor
    base + offset + self.bars
var box = PriceBox.new(price=close, bars=%d)
box.price := close + %d
value = box.score(offset=%d, factor=2)
mtfValue = request.security(syminfo.tickerid, "15", box.score(offset=%d, factor=2))
if value > 0 and mtfValue > 0
    strategy.entry("Long", strategy.long, qty=1)`, index%5+1, index%5+1, index%3, index%4, index%3)))
	}
	return base
}

func pineV25MigrationCorpus() []pineMigrationCorpusCase {
	base := make([]pineMigrationCorpusCase, 0, len(pineV24MigrationCorpus())+240)
	for _, item := range pineV24MigrationCorpus() {
		if item.wantOK {
			base = append(base, item)
		}
	}
	for index := 0; index < 80; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v25-array-stats-%03d", index+1), true, index < 20, fmt.Sprintf(`values = array.from(-2, 1, 2, 2, close + %d)
absValues = values.abs()
values.sort(order.ascending)
left = values.binary_search_leftmost(2)
right = values.binary_search_rightmost(2)
rank = values.percentrank(3)
p50 = values.percentile_nearest_rank(50)
p50lin = values.percentile_linear_interpolation(50)
dev = values.stdev()
variance = values.variance()
other = array.from(2, 4, 6, 8, 10)
cov = values.covariance(other)
if absValues.size() == 5 and left >= 0 and right >= left and rank >= 0 and p50 >= 0 and p50lin >= 0 and dev >= 0 and variance >= 0 and cov >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%4)))
	}
	for index := 0; index < 80; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v25-string-time-%03d", index+1), true, index < 20, fmt.Sprintf(`labelText = str.format("{0}:{1}", str.upper("alpha"), str.length("beta%d"))
hasNeedle = str.contains(labelText, "ALPHA")
pos = str.pos(labelText, ":")
piece = str.substring(labelText, 0, 5)
replaced = str.replace(piece, "ALPHA", "BETA")
lowered = str.lower(replaced)
tc = time_close
changed = timeframe.change("15")
if hasNeedle and pos >= 0 and str.length(lowered) > 0 and tc >= time and (changed or not changed)
    strategy.entry("Long", strategy.long, qty=1)`, index%5)))
	}
	for index := 0; index < 80; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v25-combined-helper-%03d", index+1), true, index < 20, fmt.Sprintf(`values = array.from(close, open, high, low)
spread = values.percentile_linear_interpolation(%d) - values.percentile_nearest_rank(25)
msg = str.format("spread={0}", spread)
if str.contains(msg, "spread") and values.variance() >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%75+1)))
	}
	return base
}

func pineV26MigrationCorpus() []pineMigrationCorpusCase {
	base := make([]pineMigrationCorpusCase, 0, len(pineV25MigrationCorpus())+240)
	for _, item := range pineV25MigrationCorpus() {
		if item.wantOK {
			base = append(base, item)
		}
	}
	for index := 0; index < 80; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v26-collection-iteration-%03d", index+1), true, index < 25, fmt.Sprintf(`values = array.from(1, 2, 3, %d)
total = 0
for [i, value] in values
    if i == 3
        break
    total := total + value
if total >= 6
    strategy.entry("Long", strategy.long, qty=1)`, index%5+4)))
	}
	for index := 0; index < 80; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v26-collection-history-%03d", index+1), true, index < 25, fmt.Sprintf(`values = array.from(close + %d, open)
prevSize = values[1].size()
prevFirst = values[1].get(0)
if nz(prevSize, 0) >= 0 and nz(prevFirst, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%4)))
	}
	for index := 0; index < 80; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v26-object-collection-field-%03d", index+1), true, index < 25, fmt.Sprintf(`type Box
    array<float> values
box = Box.new(array.new_float())
box.values.push(close + %d)
box.values.push(open)
fieldSize = box.values.size()
fieldLast = box.values.last()
if fieldSize == 2 and fieldLast >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%3)))
	}
	return base
}

func pineV27MigrationCorpus() []pineMigrationCorpusCase {
	base := make([]pineMigrationCorpusCase, 0, len(pineV26MigrationCorpus())+300)
	for _, item := range pineV26MigrationCorpus() {
		if item.wantOK {
			base = append(base, item)
		}
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v27-history-aggregate-%03d", index+1), true, index < 25, fmt.Sprintf(`values = array.from(close + %d, open, high, low)
prevRange = values[1].range()
prevDev = values[1].stdev()
prevVariance = values[1].variance()
if nz(prevRange, 0) >= 0 and nz(prevDev, 0) >= 0 and nz(prevVariance, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%5)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v27-map-matrix-timeframe-%03d", index+1), true, index < 30, fmt.Sprintf(`labels = map.new<string, float>()
labels.put("b", close + %d)
labels.put("a", open)
total = 0
for key in labels.keys()
    total := total + labels.get(key)
grid = matrix.new<float>(2, 2, 0)
grid.set(1, 1, total)
cell = grid.get(1, 1)
rows = grid.rows()
cols = grid.columns()
seconds = timeframe.in_seconds("15")
if total > 0 and cell > 0 and rows == 2 and cols == 2 and seconds == 900 and timeframe.multiplier >= 1
    strategy.entry("Long", strategy.long, qty=1)`, index%3)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v27-mtf-helper-expression-%03d", index+1), true, index < 30, fmt.Sprintf(`mtf = request.security(syminfo.tickerid, "15", str.length(str.format("{0}", close + %d)) + timeframe.in_seconds("15"))
if mtf > 0
    strategy.entry("Long", strategy.long, qty=1)`, index%7)))
	}
	return base
}

func pineV28MigrationCorpus() []pineMigrationCorpusCase {
	base := make([]pineMigrationCorpusCase, 0, len(pineV27MigrationCorpus())+300)
	for _, item := range pineV27MigrationCorpus() {
		if item.wantOK {
			base = append(base, item)
		}
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v28-object-history-chain-%03d", index+1), true, index < 40, fmt.Sprintf(`type PriceBox
    float price = close
method identity(PriceBox self) => self
method score(PriceBox self, float factor = 1) => self.price * factor
box = PriceBox.new(close + %d)
previous = box[1].price
chained = box.identity().score(2)
if nz(previous, 0) >= 0 and chained > 0
    strategy.entry("Long", strategy.long, qty=1)`, index%5)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v28-export-metadata-%03d", index+1), true, index < 20, fmt.Sprintf(`type ExportBox
    float price = close
method exportedScore(ExportBox self, float factor = 1) => self.price * factor
export helper(float src) => src
export type ExportedOnly%d
export method exportedScore(ExportBox self, float factor = 1) => self.price * factor
box = ExportBox.new(close)
score = box.exportedScore(2)
if score > 0
    strategy.entry("Long", strategy.long, qty=1)`, index%9)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v28-mtf-object-method-%03d", index+1), true, index < 30, fmt.Sprintf(`type PriceBox
    float price = close
method score(PriceBox self, float factor = 1) => self.price * factor
box = PriceBox.new(close + %d)
mtf = request.security(syminfo.tickerid, "15", box.score(2))
if mtf > 0
    strategy.entry("Long", strategy.long, qty=1)`, index%4)))
	}
	return base
}

func pineV29MigrationCorpus() []pineMigrationCorpusCase {
	base := make([]pineMigrationCorpusCase, 0, len(pineV28MigrationCorpus())+300)
	for _, item := range pineV28MigrationCorpus() {
		if item.wantOK {
			base = append(base, item)
		}
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v29-object-history-method-%03d", index+1), true, index < 40, fmt.Sprintf(`type PriceBox
    float price = close
method score(PriceBox self, float factor = 1, float offset = 0) => self.price * factor + offset
box = PriceBox.new(close + %d)
previousScore = box[1].score(factor=2, offset=1)
if nz(previousScore, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%6)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v29-method-chain-named-%03d", index+1), true, index < 40, fmt.Sprintf(`type PriceBox
    float price = close
method identity(PriceBox self) => self
method score(PriceBox self, float factor = 1, float offset = 0) => self.price * factor + offset
box = PriceBox.new(close + %d)
score = box.identity().score(offset=1, factor=2)
if score > 0
    strategy.entry("Long", strategy.long, qty=1)`, index%7)))
	}
	for index := 0; index < 100; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v29-mtf-object-history-%03d", index+1), true, index < 40, fmt.Sprintf(`type PriceBox
    float price = close
method score(PriceBox self, float factor = 1, float offset = 0) => self.price * factor + offset
box = PriceBox.new(close + %d)
mtf = request.security(syminfo.tickerid, "15", nz(box[1].price, 0) + nz(box[1].score(offset=1, factor=2), 0))
if nz(mtf, 0) >= 0
    strategy.entry("Long", strategy.long, qty=1)`, index%5)))
	}
	return base
}

func pineV30MigrationCorpus() []pineMigrationCorpusCase {
	base := make([]pineMigrationCorpusCase, 0, len(pineV29MigrationCorpus())+350)
	for _, item := range pineV29MigrationCorpus() {
		if item.wantOK {
			base = append(base, item)
		}
	}
	for index := 0; index < 120; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v30-varip-policy-%03d", index+1), true, index < 45, fmt.Sprintf(`varip count = %d
count := count + 1
if count >= 1
    strategy.entry("Long", strategy.long, qty=1)`, index%3)))
	}
	for index := 0; index < 120; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v30-semantic-export-%03d", index+1), true, index < 45, fmt.Sprintf(`type ExportBox
    float price = close
method score(ExportBox self, float factor = 1) => self.price * factor
export helper%d(float src) => src
export type ExportedBox%d
export method exportedScore(ExportBox self, float factor = 1) => self.price * factor
box = ExportBox.new(close)
score = box.score(2)
if score > 0
    strategy.entry("Long", strategy.long, qty=1)`, index%17, index%17)))
	}
	for index := 0; index < 110; index++ {
		base = append(base, corpusCase(fmt.Sprintf("v30-parser-whitespace-comments-%03d", index+1), true, index < 40, fmt.Sprintf(`// inline comments and blank lines stay stable in v3.0
fast = ta.ema(close, %d) // strip this comment

slow = ta.sma(close, %d)
signal = fast > slow and close > open
if signal
    strategy.entry("Long", strategy.long, qty=1)`, 5+index%5, 10+index%7)))
	}
	return base
}

func corpusCase(name string, wantOK bool, runBacktest bool, body string) pineMigrationCorpusCase {
	return pineMigrationCorpusCase{
		name:        name,
		wantOK:      wantOK,
		runBacktest: runBacktest,
		script: `//@version=6
strategy("` + name + `", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)
` + body,
	}
}
