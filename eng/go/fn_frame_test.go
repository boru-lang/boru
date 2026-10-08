package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// The fn-frame tape protocol (fn_frame.go) is what every body-splice
// path builds frames from; these tests pin the marked open paren's
// predicates and the canonical tail shape so a probe scanning frames
// (design/legacy/TCO-STAGED.10.ignore Stage 2) has an asserted contract to match.

func TestFrameOpenPredicates(t *testing.T) {
	meta := &core.FnFrameMeta{Name: "f"}
	fo := core.NewFrameOpen(meta)

	if !core.IsOpenParen(fo) {
		t.Error("NewFrameOpen must remain an ordinary OpenParen structurally")
	}
	if !core.IsFrameOpen(fo) {
		t.Error("NewFrameOpen not recognised by IsFrameOpen")
	}
	if fo.String() != "(" {
		t.Errorf("frame open renders %q, want \"(\" (payload must not leak into rendering)", fo.String())
	}

	info, err := core.AsFrameOpen(fo)
	if err != nil {
		t.Fatalf("AsFrameOpen: %v", err)
	}
	if info.Meta != meta {
		t.Error("AsFrameOpen did not round-trip the meta pointer")
	}

	// Negative cases: plain parens and non-parens are NOT frame opens.
	if core.IsFrameOpen(core.NewOpenParen()) {
		t.Error("a plain grouping paren must not read as a frame open")
	}
	if core.IsFrameOpen(core.NewCloseParen()) {
		t.Error("a close paren must not read as a frame open")
	}
	if core.IsFrameOpen(core.NewWord("(")) {
		t.Error("a word must not read as a frame open")
	}
	if _, err := core.AsFrameOpen(core.NewOpenParen()); err == nil {
		t.Error("AsFrameOpen on a plain paren must error")
	}
}

func TestAppendFrameTailShape(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	snap := map[string]int{"x": 1}

	tail := core.AppendFrameTail(nil, core.FrameTailSpec{
		Registry:     r,
		Snapshot:     snap,
		Names:        []string{"a", "b"}, // install order; undefs must reverse
		Returns:      []*core.Type{core.TInteger},
		UnnamedCount: 1,
		FuncName:     "f",
	})

	// Expected: __DC  __RC — the marker carries the frame's pop and the
	// names to tear down (install order; the marker reverses them).
	if len(tail) != 2 {
		t.Fatalf("tail has %d tokens, want 2: %v", len(tail), tail)
	}
	if !core.IsDefCleanup(tail[0]) {
		t.Errorf("tail[0] = %v, want DefCleanup", tail[0])
	}
	dc, _ := core.AsDefCleanup(tail[0])
	if dc.Snapshot["x"] != 1 || dc.Registry != r {
		t.Error("DefCleanup does not carry the supplied snapshot/registry")
	}
	if !dc.PopFrame {
		t.Error("a frame tail's DefCleanup must pop the frame (PopFrame)")
	}
	if len(dc.Names) != 2 || dc.Names[0] != "a" || dc.Names[1] != "b" {
		t.Errorf("DefCleanup names = %v, want [a b] (install order)", dc.Names)
	}
	if !core.IsReturnCheck(tail[1]) {
		t.Fatalf("tail[1] = %v, want ReturnCheck", tail[1])
	}
	rc, _ := core.AsReturnCheck(tail[1])
	if rc.FuncName != "f" || rc.UnnamedCount != 1 || len(rc.Returns) != 1 || !rc.Returns[0].Equal(core.TInteger) {
		t.Errorf("ReturnCheck fields wrong: %+v", rc)
	}
	if rc.Pos.Row != 0 {
		t.Errorf("unstamped tail must leave Pos zero for execMatch's call-site stamping, got %+v", rc.Pos)
	}

	// No declared returns ⇒ no ReturnCheck: the marker alone, still a
	// frame marker with nothing to tear down.
	bare := core.AppendFrameTail(nil, core.FrameTailSpec{Registry: r, Snapshot: snap})
	if len(bare) != 1 || !core.IsDefCleanup(bare[0]) {
		t.Fatalf("bare tail = %v, want exactly [__DC]", bare)
	}
	if bdc, _ := core.AsDefCleanup(bare[0]); !bdc.PopFrame || len(bdc.Names) != 0 {
		t.Fatalf("bare tail marker = %+v, want a PopFrame marker with no names", bdc)
	}
}

func TestPopFrameArgsPairsArgsAndBaseline(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.PushFnBaseline(r.Defs.Snapshot())
	if err := r.Args.Push(core.NewList(nil)); err != nil {
		t.Fatal(err)
	}
	if r.TopFnBaseline() == nil {
		t.Fatal("baseline not pushed")
	}
	if err := core.PopFrameArgs(r); err != nil {
		t.Fatalf("PopFrameArgs: %v", err)
	}
	if r.TopFnBaseline() != nil {
		t.Error("PopFrameArgs did not pop the FnBaseline alongside the Args entry")
	}
	if _, ok, _ := r.Args.Top(); ok {
		t.Error("PopFrameArgs did not pop the Args entry")
	}
}
