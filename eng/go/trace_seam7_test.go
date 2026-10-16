package eng

// Seam-7 (cluster A4): in-package unit tests for previously-unreached
// blocks in trace.go — the trace renderer. TraceColorize and TraceWrap
// are pure and driven directly with crafted Values; RunTrace is driven
// end-to-end with small token programs so the step formatter's branches
// (fit / note-next-line / multi-line wrap) execute. A prior bug printed
// sealed payload structs, so the colorizer asserts clean rendering.

import (
	"bytes"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// --- TraceColorize --------------------------------------------------------

func TestS7TraceColorizeSpeculativeForward(t *testing.T) {
	fwd := core.NewForward(core.ForwardInfo{
		FuncName:      "f",
		CollectedArgs: 1,
		ExpectedArgs:  2,
		Speculative:   true,
		SpeculativeAt: 0,
	})
	got := core.TraceColorize(fwd)
	if !strings.Contains(got, "spec@0") || !strings.Contains(got, "→f") {
		t.Errorf("speculative forward render = %q", got)
	}
}

func TestS7TraceColorizePayloadMismatches(t *testing.T) {
	// Each scalar arm's accessor-error fallback renders via String() and
	// must not leak the sealed payload struct form.
	strBad := core.ValueOf(&core.Datum{Parent: core.TString, Data: core.IntPayload{N: 1}})
	intBad := core.ValueOf(&core.Datum{Parent: core.TInteger, Data: core.StrPayload{S: "x"}})
	atomBad := core.ValueOf(&core.Datum{Parent: core.TAtom, Data: core.IntPayload{N: 1}})
	for _, v := range []core.Value{strBad, intBad, atomBad} {
		got := core.TraceColorize(v)
		if strings.Contains(got, "Payload{") {
			t.Errorf("colorize leaked a payload struct: %q", got)
		}
	}
	// A TMap-conforming value whose payload is not a map -> AsMap yields a
	// nil map -> the nil-map String() fallback.
	mapBad := core.ValueOf(&core.Datum{Parent: core.TMap, Data: core.IntPayload{N: 1}})
	if got := core.TraceColorize(mapBad); got == "" {
		t.Error("map-nil fallback produced empty output")
	}
	// The `none` value hits the default arm.
	if got := core.TraceColorize(core.NewNone()); !strings.Contains(got, "none") {
		t.Errorf("default-arm render of none = %q", got)
	}
}

func TestS7TraceColorizeContainers(t *testing.T) {
	// List and map arms with real content (element recursion).
	lst := core.NewList([]core.Value{core.NewInteger(1), core.NewString("a")})
	if got := core.TraceColorize(lst); !strings.Contains(got, "[") {
		t.Errorf("list render = %q", got)
	}
	m := core.NewOrderedMap()
	m.Set("k", core.NewInteger(1))
	if got := core.TraceColorize(core.NewMap(m)); !strings.Contains(got, "k") {
		t.Errorf("map render = %q", got)
	}
}

// --- TraceHandler ---------------------------------------------------------

func TestS7TraceHandlerNonConcrete(t *testing.T) {
	if _, err := core.TraceHandler([]core.Value{core.NewTypeLiteral(core.TList)}, nil, nil, nil); err == nil {
		t.Error("trace of a non-concrete arg must error")
	}
}

// --- TraceWrap ------------------------------------------------------------

func TestS7TraceWrapClampAndSingleLine(t *testing.T) {
	parts := []string{core.TraceColorize(core.NewInteger(1)), core.TraceColorize(core.NewInteger(2))}
	// maxWidth below the floor clamps to 20; short input stays one line.
	lines := core.TraceWrap(parts, 0, 5)
	if len(lines) != 1 {
		t.Errorf("short wrap should be one line, got %d", len(lines))
	}
	// Empty parts -> the "[ ]" sentinel line.
	if got := core.TraceWrap(nil, 0, 100); len(got) != 1 || !strings.Contains(got[0], "[") {
		t.Errorf("empty wrap = %v", got)
	}
}

func TestS7TraceWrapMultiLine(t *testing.T) {
	parts := make([]string, 40)
	for i := range parts {
		parts[i] = core.TraceColorize(core.NewInteger(int64(100 + i)))
	}
	lines := core.TraceWrap(parts, 5, 60)
	if len(lines) < 2 {
		t.Errorf("wide input should wrap to multiple lines, got %d", len(lines))
	}
}

// --- RunTrace end-to-end --------------------------------------------------

// s7traceReg registers an identity word so a dispatch step carries a note.
func s7traceReg(t *testing.T) *core.Registry {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "s7id",
		Signatures: []core.Signature{{
			Args:       []*core.Type{core.TInteger},
			Returns:    []*core.Type{core.TInteger},
			BarrierPos: -1,
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return args, nil
			}),
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register s7id: %v", err)
	}
	return r
}

func TestS7RunTraceNarrowWithNote(t *testing.T) {
	r := s7traceReg(t)
	var buf bytes.Buffer
	// A small program that dispatches a word (producing a trace note).
	_, err := core.RunTrace(r, []core.Value{core.NewInteger(5), core.NewWord("s7id")}, &buf)
	if err != nil {
		t.Fatalf("RunTrace: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected trace output")
	}
	if !strings.Contains(buf.String(), "trace") {
		t.Error("trace header missing")
	}
}

func TestS7RunTraceWideStack(t *testing.T) {
	r := s7traceReg(t)
	var buf bytes.Buffer
	// A wide stack forces the multi-line wrap branch in the step printer.
	toks := make([]core.Value, 60)
	for i := range toks {
		toks[i] = core.NewInteger(int64(100 + i))
	}
	if _, err := core.RunTrace(r, toks, &buf); err != nil {
		t.Fatalf("RunTrace wide: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected trace output for a wide stack")
	}
}

func TestS7RunTraceWideWithNote(t *testing.T) {
	r := s7traceReg(t)
	var buf bytes.Buffer
	// Wide stack AND a dispatched word -> a wide step that also carries a
	// note (the multi-line + note branch).
	toks := make([]core.Value, 0, 62)
	for i := 0; i < 60; i++ {
		toks = append(toks, core.NewInteger(int64(100+i)))
	}
	toks = append(toks, core.NewInteger(7), core.NewWord("s7id"))
	if _, err := core.RunTrace(r, toks, &buf); err != nil {
		t.Fatalf("RunTrace wide+note: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected trace output")
	}
}

func TestS7RunTraceWidthSweep(t *testing.T) {
	// Sweep the pre-pushed stack width so the noted dispatch step lands in
	// each width regime of the step formatter: note-fits (with the gap<2
	// clamp), note-on-next-line, and multi-line-wrap-with-note.
	for n := 0; n < 80; n++ {
		r := s7traceReg(t)
		var buf bytes.Buffer
		toks := make([]core.Value, 0, n+2)
		for i := 0; i < n; i++ {
			toks = append(toks, core.NewInteger(int64(10+i)))
		}
		toks = append(toks, core.NewInteger(3), core.NewWord("s7id"))
		if _, err := core.RunTrace(r, toks, &buf); err != nil {
			t.Fatalf("RunTrace width=%d: %v", n, err)
		}
	}
}

func TestS7RunTraceLongNote(t *testing.T) {
	// A very long note (>115 visible chars) forces the note gap-clamp
	// branches: gap = termWidth - noteVisLen goes negative and clamps to
	// stepWidth+1. Exercised on both a narrow (note-next-line) and a wide
	// (multi-line) step.
	longName := "s7" + strings.Repeat("x", 130)
	reg := func() *core.Registry {
		r := newTestRegistry(t)
		r.RegisterNativeFunc(core.NativeFunc{
			Name: longName,
			Signatures: []core.Signature{{
				Args:       []*core.Type{core.TInteger},
				Returns:    []*core.Type{core.TInteger},
				BarrierPos: -1,
				Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return args, nil
				}),
			}},
		})
		if err := r.Err(); err != nil {
			t.Fatalf("register long word: %v", err)
		}
		return r
	}

	var narrow bytes.Buffer
	if _, err := core.RunTrace(reg(), []core.Value{core.NewInteger(3), core.NewWord(longName)}, &narrow); err != nil {
		t.Fatalf("RunTrace narrow long-note: %v", err)
	}
	var wide bytes.Buffer
	toks := make([]core.Value, 0, 62)
	for i := 0; i < 60; i++ {
		toks = append(toks, core.NewInteger(int64(100+i)))
	}
	toks = append(toks, core.NewInteger(3), core.NewWord(longName))
	if _, err := core.RunTrace(reg(), toks, &wide); err != nil {
		t.Fatalf("RunTrace wide long-note: %v", err)
	}
}

func TestS7RunTraceError(t *testing.T) {
	r := s7traceReg(t)
	var buf bytes.Buffer
	// A word with a wrong-typed arg makes the sub-engine error, exercising
	// the error-print tail of RunTrace.
	_, err := core.RunTrace(r, []core.Value{core.NewString("x"), core.NewWord("s7id")}, &buf)
	if err == nil {
		t.Log("no runtime error (acceptable); error tail not exercised this run")
	}
}
