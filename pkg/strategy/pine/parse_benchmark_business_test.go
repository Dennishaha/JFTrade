package pine

import "testing"

func TestBenchmarkBusinessScriptsCompileAsRegressionCases(t *testing.T) {
	for _, benchmarkCase := range pineBenchmarkCases() {
		t.Run(benchmarkCase.name, func(t *testing.T) {
			compilation, err := Compile(benchmarkCase.script)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			if compilation.Program == nil {
				t.Fatal("Compile() returned nil program")
			}
			if compilation.Program.Metadata.Name == "" {
				t.Fatalf("compiled program missing strategy name: %#v", compilation.Program.Metadata)
			}
			if len(compilation.Program.Hooks) == 0 {
				t.Fatalf("compiled program missing executable hooks: %#v", compilation.Program)
			}
			analysis := AnalyzeScript(benchmarkCase.script, AnalysisOptions{IncludeAST: true})
			if !analysis.OK {
				t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
			}
			if analysis.AST == nil || analysis.Semantic.Declarations == nil {
				t.Fatalf("AnalyzeScript() missing AST/semantic payload: %#v", analysis)
			}
		})
	}
}
