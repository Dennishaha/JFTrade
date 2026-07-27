package pine

import (
	"errors"
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestRequestSecurityLoweringRejectsUnrepresentableExecutionBoundaries(t *testing.T) {
	if values, ok := lowerSupportedRequestSecurityTupleGeneral([]string{
		"syminfo.tickerid", `"15"`, "[open, close]", "gaps=barmerge.gaps_on",
	}); ok || values != nil {
		t.Fatalf("tuple with gaps_on lowered to %#v/%v", values, ok)
	}
	if !supportedRequestSecurityMergeArgs([]string{"barmerge.gaps_off", "barmerge.lookahead_off"}) {
		t.Fatal("default request.security merge semantics were rejected")
	}
	if supportedRequestSecurityMergeArgs([]string{"barmerge.gaps_off", "barmerge.lookahead_off", "barmerge.gaps_off"}) {
		t.Fatal("third positional merge argument was accepted")
	}

	for _, expression := range []string{
		"ta.sma(strategy.position_size, 2)",
		"ta.rsi(strategy.position_size, 14)",
		"ta.ema(close, 14",
	} {
		if lowered, ok := lowerSupportedRequestSecurityInner(expression, `"15m"`); ok || lowered != "" {
			t.Fatalf("unsafe request.security expression %q lowered to %q/%v", expression, lowered, ok)
		}
	}

	placeholders := make([]string, 0)
	addPlaceholder := func(value string) string {
		placeholders = append(placeholders, value)
		return "placeholder"
	}
	if lowered, ok := maskPureRequestSecurityTACalls("ta.obv", `"15m"`, addPlaceholder); !ok || lowered != "placeholder" || len(placeholders) != 1 || placeholders[0] != `obv(close, "15m")` {
		t.Fatalf("ta.obv property masking = %q/%v/%#v", lowered, ok, placeholders)
	}
	for _, expression := range []string{"ta.bad(close)", "ta.ema(close, 14"} {
		if _, ok := maskPureRequestSecurityTACalls(expression, `"15m"`, addPlaceholder); ok {
			t.Fatalf("unsupported TA masking accepted %q", expression)
		}
	}

	if lowered, ok := maskPureRequestSecuritySourceHistory("volume[2] + unsupported[1]", `"15m"`, addPlaceholder); !ok || !strings.Contains(lowered, "placeholder") {
		t.Fatalf("source history masking = %q/%v", lowered, ok)
	}
	if _, ok := maskPureRequestSecuritySourceHistory("close[501]", `"15m"`, addPlaceholder); ok {
		t.Fatal("out-of-range request.security history was accepted")
	}

	for _, tc := range []struct {
		call string
		args []string
	}{
		{call: "bbw", args: []string{"close", "20"}},
		{call: "cog", args: []string{"close"}},
		{call: "cmo", args: []string{"close"}},
		{call: "correlation", args: []string{"close", "bad", "20"}},
		{call: "percentile_nearest_rank", args: []string{"close", "20"}},
	} {
		if lowered, ok := lowerAdvancedRequestSecurity(tc.call, tc.args, `"15m"`); ok || lowered != "" {
			t.Fatalf("invalid advanced request.security call %s(%#v) lowered to %q/%v", tc.call, tc.args, lowered, ok)
		}
	}
	for _, tc := range []struct {
		call string
		args []string
	}{
		{call: "ema", args: []string{"unsupported_source", "20"}},
		{call: "rsi", args: []string{"unsupported_source", "14"}},
		{call: "macd", args: []string{"unsupported_source", "12", "26", "9"}},
		{call: "bb", args: []string{"unsupported_source", "20", "2"}},
		{call: "stoch", args: []string{"close", "low", "high", "14"}},
		{call: "correlation", args: []string{"close", "high"}},
	} {
		if lowered, ok := lowerRequestSecurityTACall(tc.call, tc.args, `"15m"`); ok || lowered != "" {
			t.Fatalf("unsupported request.security source in %s(%#v) lowered to %q/%v", tc.call, tc.args, lowered, ok)
		}
	}
}

func TestObjectDefinitionParserProtectsTypeAndMethodContracts(t *testing.T) {
	newState := func(lines ...parsedLine) *parseState {
		return newParseState("", lines, nil)
	}
	valid := newState(
		parsedLine{number: 1, trimmed: "type PriceBox"},
		parsedLine{number: 2, trimmed: "float price", indent: 4},
		parsedLine{number: 3, trimmed: "int bars = 0", indent: 4},
		parsedLine{number: 4, trimmed: "method score(PriceBox self, float factor=1) => self.price * factor"},
	)
	next, err := valid.parseExecutableTypeDefinition(0)
	if err != nil || next != 3 || valid.udtTypes["pricebox"].Fields[0].Default != "na" {
		t.Fatalf("parseExecutableTypeDefinition = next=%d err=%v types=%#v", next, err, valid.udtTypes)
	}
	next, err = valid.parseExecutableMethodDefinition(3)
	if err != nil || next != 4 || len(valid.udtMethods["pricebox.score"]) != 1 {
		t.Fatalf("parseExecutableMethodDefinition = next=%d err=%v methods=%#v", next, err, valid.udtMethods)
	}

	for _, tc := range []struct {
		name  string
		state *parseState
		parse func(*parseState) error
		want  string
	}{
		{
			name:  "duplicate type",
			state: valid,
			parse: func(state *parseState) error {
				state.lines = append(state.lines, parsedLine{number: 5, trimmed: "type PriceBox"}, parsedLine{number: 6, trimmed: "float other", indent: 4})
				_, err := state.parseExecutableTypeDefinition(4)
				return err
			},
			want: "already declared",
		},
		{
			name: "duplicate field",
			state: newState(
				parsedLine{number: 10, trimmed: "type Duplicate"},
				parsedLine{number: 11, trimmed: "float value", indent: 4},
				parsedLine{number: 12, trimmed: "float value", indent: 4},
			),
			parse: func(state *parseState) error { _, err := state.parseExecutableTypeDefinition(0); return err },
			want:  "repeats field",
		},
		{
			name: "invalid field default",
			state: newState(
				parsedLine{number: 20, trimmed: "type Invalid"},
				parsedLine{number: 21, trimmed: "float value = close >", indent: 4},
			),
			parse: func(state *parseState) error { _, err := state.parseExecutableTypeDefinition(0); return err },
			want:  "invalid type field default",
		},
		{
			name:  "unknown method receiver",
			state: newState(parsedLine{number: 30, trimmed: "method score(Missing self) => self.price"}),
			parse: func(state *parseState) error { _, err := state.parseExecutableMethodDefinition(0); return err },
			want:  "receiver type Missing is not declared",
		},
		{
			name: "method block must be pure",
			state: newState(
				parsedLine{number: 40, trimmed: "type Pure"},
				parsedLine{number: 41, trimmed: "float price", indent: 4},
				parsedLine{number: 42, trimmed: "method score(Pure self) => strategy.position_size"},
			),
			parse: func(state *parseState) error {
				if _, err := state.parseExecutableTypeDefinition(0); err != nil {
					return err
				}
				_, err := state.parseExecutableMethodDefinition(2)
				return err
			},
			want: "side-effect-free",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.parse(tc.state)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parser error = %v, want %q", err, tc.want)
			}
		})
	}

	state := newObjectCollectionBoundaryParseState()
	state.normalizationErr = errors.New("constructor normalization failure")
	if _, err := state.normalizeObjectConstructorArguments(50, []string{"price=close"}, state.udtTypes["pricebox"].Fields); err == nil || !strings.Contains(err.Error(), "constructor normalization failure") {
		t.Fatalf("constructor normalization error = %v", err)
	}
	state = newObjectCollectionBoundaryParseState()
	fields := state.udtTypes["pricebox"].Fields
	if _, err := state.normalizeObjectConstructorArguments(51, []string{"price=close", "bars=1", "values=array.new_float()", "extra=2"}, fields); err == nil || !strings.Contains(err.Error(), "unknown constructor field") {
		t.Fatalf("constructor unknown field error = %v", err)
	}
	parameters := []strategyir.ObjectParameter{{Name: "required"}, {Name: "optional", Default: "0"}}
	if _, err := state.normalizeObjectMethodArguments(52, []string{"optional=2"}, parameters); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("method missing required argument error = %v", err)
	}
}
