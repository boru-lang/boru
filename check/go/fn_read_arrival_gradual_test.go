package check

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// fn_read_arrival_gradual_test.go pins the GRADUAL claim of the def-read
// model (NUR207, 2026-09-26): a def-bound carrier NOT typed Function — an
// `Any`-returning factory's result, a pinpointed member read — whose shape
// the recorder claimed at the def. The whole fitting window is the word
// dispatch it always was; of the windows the model cannot claim, only the
// two the program-level paths got wrong decline (a bare read with nothing
// beneath it, a written function word the forward phase stops at), and
// every other shape stands aside SILENTLY to the paths it had.

// fragFix is fraFix over a dynamic(Any) carrier — the gradual stand-in.
func fragFix(t *testing.T, arity int) (*core.Registry, *zzmsRecorder, core.Value, func()) {
	t.Helper()
	r := zzmsReg(t)
	done := r.Check.Begin()
	rec := newZZMSRecorder()
	r.Check.Emit = rec
	carrier := core.NewDynamicCarrier(core.TAny)
	rec.defReads = map[string]string{carrier.ID: "j"}
	r.Check.NoteFnShape(carrier, core.FnShape{Arity: arity, Params: make([]*core.Type, arity)})
	return r, rec, carrier, done
}

func TestGradualReadArrivalWholeWindow(t *testing.T) {
	r, rec, carrier, done := fragFix(t, 1)
	defer done()
	e := zzmsEngine(r, []core.Value{carrier, core.NewInteger(5), core.NewEnd()})
	if !tryMemberFnArrivalDispatch(e, 0) {
		t.Fatal("a claimed gradual read dispatches over its whole window")
	}
	if len(rec.dynCalls) != 1 || rec.dynCalls[0].word != "j" || len(rec.dynCalls[0].args) != 1 {
		t.Fatalf("one RecordDynMethod over the window, got %v", rec.dynCalls)
	}
	if len(rec.reasons) != 0 {
		t.Errorf("nothing declines, got %v", rec.reasons)
	}
}

func TestGradualReadArrivalDeclines(t *testing.T) {
	for name, tape := range map[string]func(c core.Value) []core.Value{
		"bare read, nothing beneath": func(c core.Value) []core.Value { return []core.Value{c, core.NewEnd()} },
		"past the tape end":          func(c core.Value) []core.Value { return []core.Value{c} },
		"a function word follows": func(c core.Value) []core.Value {
			return []core.Value{c, core.NewWord("zzfra-fn"), core.NewInteger(1), core.NewEnd()}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, rec, carrier, done := fragFix(t, 1)
			defer done()
			zzmsRegisterInner(t, r, "zzfra-fn", core.Signature{Args: []*core.Type{core.TAny}, Impl: zzmsGoImpl(), BarrierPos: core.BarrierAllForward})
			e := zzmsEngine(r, tape(carrier))
			if tryMemberFnArrivalDispatch(e, 0) {
				t.Fatal("the window is not the model's")
			}
			if len(rec.reasons) != 1 || !strings.Contains(rec.reasons[0], "`j`") {
				t.Errorf("one compile failure naming `j`, got %v", rec.reasons)
			}
		})
	}
}

func TestGradualReadArrivalStandsAside(t *testing.T) {
	type fix struct {
		tape   func(c core.Value) []core.Value
		at     int
		dynOK  bool
		fnBody bool
		nested bool
	}
	for name, c := range map[string]fix{
		"values beneath in the frame": {tape: func(c core.Value) []core.Value { return []core.Value{core.NewInteger(3), c, core.NewEnd()} }, at: 1, dynOK: true},
		"a value-bound word follows":  {tape: func(c core.Value) []core.Value { return []core.Value{c, core.NewWord("zzfra-unbound"), core.NewEnd()} }, dynOK: true},
		"a paren group follows":       {tape: func(c core.Value) []core.Value { return []core.Value{c, core.NewOpenParen(), core.NewEnd()} }, dynOK: true},
		"inside a fn body":            {tape: func(c core.Value) []core.Value { return []core.Value{c, core.NewEnd()} }, dynOK: true, fnBody: true},
		"inside a nested body":        {tape: func(c core.Value) []core.Value { return []core.Value{c, core.NewEnd()} }, dynOK: true, nested: true},
		"an operand with no home":     {tape: func(c core.Value) []core.Value { return []core.Value{c, core.NewInteger(5), core.NewEnd()} }},
	} {
		t.Run(name, func(t *testing.T) {
			r, rec, carrier, done := fragFix(t, 1)
			defer done()
			rec.dynOK = c.dynOK
			if c.fnBody {
				r.Check.FnBodyDepth++
				defer func() { r.Check.FnBodyDepth-- }()
			}
			if c.nested {
				r.Check.NestedBodyDepth++
				defer func() { r.Check.NestedBodyDepth-- }()
			}
			tape := c.tape(carrier)
			e := zzmsEngine(r, tape)
			e.Pointer = c.at
			if tryMemberFnArrivalDispatch(e, c.at) {
				t.Fatal("the model must not consume this window")
			}
			if e.Tape.Len() != len(tape) || len(rec.reasons) != 0 {
				t.Errorf("a gradual read the model cannot claim keeps its paths: tape %d/%d, reasons %v", e.Tape.Len(), len(tape), rec.reasons)
			}
		})
	}
}

// A carrier whose bound excludes every fn is no gradual claim at all, and a
// FUNCTION-typed carrier keeps its strict declines (fraFix's rows).
func TestGradualReadArrivalAdmission(t *testing.T) {
	r, rec, _, done := fragFix(t, 1)
	defer done()
	data := core.NewDynamicCarrier(core.TInteger)
	rec.defReads[data.ID] = "d"
	r.Check.NoteFnShape(data, core.FnShape{Arity: 1})
	e := zzmsEngine(r, []core.Value{data, core.NewEnd()})
	if tryMemberFnArrivalDispatch(e, 0) || len(rec.reasons) != 0 {
		t.Errorf("an Integer-bounded read is data: reasons %v", rec.reasons)
	}
}

func TestFunctionWordInWindow(t *testing.T) {
	r := zzmsReg(t)
	zzmsRegisterInner(t, r, "zzfra-fn", core.Signature{Args: []*core.Type{core.TAny}, Impl: zzmsGoImpl(), BarrierPos: core.BarrierAllForward})
	c := core.NewDynamicCarrier(core.TAny)
	for _, row := range []struct {
		name string
		tape []core.Value
		n    int
		want bool
	}{
		{"fixed tokens then the word", []core.Value{c, core.NewInteger(1), core.NewWord("zzfra-fn")}, 2, true},
		{"the word first", []core.Value{c, core.NewWord("zzfra-fn")}, 1, true},
		{"a value word", []core.Value{c, core.NewWord("zzfra-unbound")}, 1, false},
		{"a dispatch modifier", []core.Value{c, core.NewDispatchMod(core.DispatchModInfo{Val: true})}, 1, false},
		{"a paren before the word", []core.Value{c, core.NewOpenParen(), core.NewWord("zzfra-fn")}, 2, false},
		{"all fixed", []core.Value{c, core.NewInteger(1)}, 1, false},
		{"the window runs off the tape", []core.Value{c}, 2, false},
	} {
		if got := functionWordInWindow(zzmsEngine(r, row.tape), 0, row.n); got != row.want {
			t.Errorf("%s: functionWordInWindow = %v, want %v", row.name, got, row.want)
		}
	}
}
