package sweep

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/test/go/vary"
)

func TestParseSeeds(t *testing.T) {
	good := "# a comment\n\neach\tliteral\teach [add 1] [1 2 3]\r\napply\tliteral\t5 [add 1] apply\tn/a\n"
	seeds, err := ParseSeeds("s.tsv", good)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 2 || seeds[0] != (Seed{Word: "each", Kind: Literal, Src: "each [add 1] [1 2 3]"}) ||
		seeds[1] != (Seed{Word: "apply", Kind: Literal, Src: "5 [add 1] apply", NA: true}) || seeds[1].Key() != "apply/literal" {
		t.Errorf("seeds = %+v", seeds)
	}
	bad := []struct{ name, text, want string }{
		{"two cells", "each\tliteral\n", "want <word>"},
		{"five cells", "a\tliteral\tp\tn/a\tx\n", "want <word>"},
		{"a fourth cell that is not n/a", "a\tliteral\tp\tno\n", "want <word>"},
		{"unknown kind", "a\tbogus\tp\n", "unknown kind"},
		{"empty word", "\tliteral\tp\n", "must both be present"},
		{"empty program", "a\tliteral\t \tn/a\n", "must both be present"},
		{"duplicate", "a\tliteral\tp\na\tliteral\tq\n", "listed twice"},
	}
	for _, c := range bad {
		if _, err := ParseSeeds("s.tsv", c.text); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one containing %q", c.name, err, c.want)
		}
	}
}

func TestSeedsEmbedded(t *testing.T) {
	seeds, err := Seeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) == 0 {
		t.Error("the embedded table is empty")
	}
}

func TestKinds(t *testing.T) {
	if !KnownKind(Literal) || KnownKind("bogus") {
		t.Error("KnownKind")
	}
	if ks := KindsOf(Quoted); len(ks) != 1 || ks[0] != Literal {
		t.Errorf("KindsOf(Quoted) = %v", ks)
	}
	if ks := KindsOf(Body); len(ks) != len(Kinds) {
		t.Errorf("KindsOf(Body) = %v", ks)
	}
}

func TestOrphans(t *testing.T) {
	inv := map[string]Class{"each": Body, "dot": Quoted}
	seeds := []Seed{
		{Word: "each", Kind: Lambda, Src: "p"},
		{Word: "dot", Kind: Literal, Src: "p"},
		{Word: "dot", Kind: Lambda, Src: "p"},
		{Word: "nope", Kind: Literal, Src: "p"},
	}
	got := Orphans(inv, seeds)
	if len(got) != 2 || got[0].Key() != "dot/lambda" || got[1].Key() != "nope/literal" {
		t.Errorf("orphans = %+v", got)
	}
}

func TestCellStatusAndGlyph(t *testing.T) {
	seed := &Seed{Word: "w", Kind: Literal, Src: "p"}
	probe := &Seed{Word: "w", Kind: Literal, Src: "p", NA: true}
	res := func(o vary.Outcome) vary.Result { return vary.Result{Outcome: o} }
	cases := []struct {
		cell   Cell
		status Status
		glyph  string
	}{
		{Cell{Word: "w", Kind: Literal}, Empty, "·"},
		{Cell{Seed: probe, Base: res(vary.InterpReject)}, NotApplicable, "n/a"},
		{Cell{Seed: probe, Base: res(vary.Pass)}, StaleNA, "n/a?!"},
		{Cell{Seed: seed, Base: res(vary.InterpReject)}, Invalid, "✗"},
		{Cell{Seed: seed, Base: res(vary.CheckReject)}, CheckReject, "C"},
		{Cell{Seed: seed, Base: res(vary.Declined)}, Failed, "F"},
		{Cell{Seed: seed, Base: res(vary.Islanded)}, Islanded, "I"},
		{Cell{Seed: seed, Base: res(vary.Diverged)}, Diverged, "D!"},
		{Cell{Seed: seed, Base: res(vary.Panicked)}, Panicked, "P!"},
		{Cell{Seed: seed, Base: res(vary.Hung)}, Hung, "H!"},
		{Cell{Seed: seed, Base: res(vary.Pass), Variants: []vary.Variant{{Res: res(vary.Pass)}, {Res: res(vary.Declined)}}}, Pass, "✓ 1/2"},
	}
	for i, c := range cases {
		if s := c.cell.Status(); s != c.status {
			t.Errorf("case %d: status %q, want %q", i, s, c.status)
		}
		if g := glyph(c.cell); g != c.glyph {
			t.Errorf("case %d: glyph %q, want %q", i, g, c.glyph)
		}
	}
	if cases[0].cell.Key() != "w/literal" {
		t.Errorf("key = %q", cases[0].cell.Key())
	}
}

// TestRunRenderCount runs a three-word matrix through the real pipeline:
// a passing seed, an n/a probe, a quoted-only word, an invalid seed, an
// orphan, and the empty cells around them.
func TestRunRenderCount(t *testing.T) {
	inv := map[string]Class{"each": Body, "dot": Quoted, "zz": Body}
	seeds := []Seed{
		{Word: "each", Kind: Literal, Src: "each [add 1] [1 2 3]"},
		{Word: "each", Kind: Lambda, Src: "each 5 5", NA: true},
		{Word: "dot", Kind: Literal, Src: "{k:9} dot k"},
		{Word: "zz", Kind: Literal, Src: "zzundefinedword 1"},
		{Word: "nope", Kind: Literal, Src: "1"},
	}
	progress := 0
	cells := Run(inv, seeds, func(done, total int) { progress = done })
	if progress != len(seeds) {
		t.Errorf("progress reported %d of %d seeds", progress, len(seeds))
	}
	if len(cells) != 1+len(Kinds)*2 || cells[0].Key() != "dot/literal" || cells[1].Key() != "each/literal" {
		t.Fatalf("cells = %d, first %s then %s", len(cells), cells[0].Key(), cells[1].Key())
	}
	want := map[string]Status{"dot/literal": Pass, "each/literal": Pass, "each/lambda": NotApplicable, "each/named-fn": Empty, "zz/literal": Invalid, "zz/computed": Empty}
	for _, c := range cells {
		if s, ok := want[c.Key()]; ok && c.Status() != s {
			t.Errorf("%s: status %q (%s), want %q", c.Key(), c.Status(), c.Base.Detail, s)
		}
	}
	counts := Count(cells)
	if counts.Cells[Pass] != 2 || counts.Cells[NotApplicable] != 1 || counts.Cells[Invalid] != 1 || counts.Cells[Empty] != len(cells)-4 {
		t.Errorf("counts = %+v", counts.Cells)
	}
	total := 0
	for _, n := range counts.Variants {
		total += n
	}
	if total != 2*len(vary.Transforms()) {
		t.Errorf("variants = %d, want every transform of the two passing seeds (%d)", total, 2*len(vary.Transforms()))
	}
	out := Render(cells)
	for _, w := range []string{"| `dot` | ✓ ", "| `each` | ✓ ", "| n/a | · |", "| `zz` | ✗ |", "## Cells that are not green", "`zz` literal — **invalid**: `zzundefinedword 1` — "} {
		if !strings.Contains(out, w) {
			t.Errorf("rendering lacks %q:\n%s", w, out)
		}
	}
}

// TestRenderDefectArms renders every non-green status and a failing
// call-form variant without running anything.
func TestRenderDefectArms(t *testing.T) {
	seed := &Seed{Src: "p"}
	cells := []Cell{
		{Word: "a", Kind: Literal, Seed: seed, Base: vary.Result{Outcome: vary.Declined, Detail: "why"}},
		{Word: "a", Kind: Lambda, Seed: seed, Base: vary.Result{Outcome: vary.Islanded}},
		{Word: "a", Kind: NamedFn, Seed: seed, Base: vary.Result{Outcome: vary.Diverged, Detail: strings.Repeat("x", 200)}},
		{Word: "a", Kind: Factory, Seed: seed, Base: vary.Result{Outcome: vary.CheckReject}},
		{Word: "b", Kind: Literal, Seed: seed, Base: vary.Result{Outcome: vary.Panicked, Detail: "PANIC in run: x"}},
		{Word: "b", Kind: Lambda, Seed: seed, Base: vary.Result{Outcome: vary.Hung, Detail: "HUNG: y"}},
		{Word: "a", Kind: Container, Seed: &Seed{Src: "p", NA: true}, Base: vary.Result{Outcome: vary.Pass}},
		{Word: "a", Kind: ModuleExport, Seed: seed, Base: vary.Result{Outcome: vary.Pass}, Variants: []vary.Variant{
			{Transform: "fn-body", Res: vary.Result{Outcome: vary.Declined, Detail: "r"}},
			{Transform: "do-body", Res: vary.Result{Outcome: vary.Pass}},
		}},
	}
	out := Render(cells)
	for _, w := range []string{
		"| `a` | F | I | D! | C | n/a?! | ✓ 1/2 | — |",
		"- `a` literal — **failed**: `p` — why",
		"- `a` lambda — **islanded**: `p`",
		"…", "- `a` factory — **check-reject**", "- `a` container — **n/a-STALE**",
		"- `a` module-export · fn-body — **declined** — r",
		"| `b` | P! | H! |", "- `b` literal — **PANIC**: `p` — PANIC in run: x", "- `b` lambda — **HUNG**: `p` — HUNG: y",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("rendering lacks %q:\n%s", w, out)
		}
	}
	green := Render([]Cell{{Word: "a", Kind: Literal, Seed: seed, Base: vary.Result{Outcome: vary.Pass}}})
	if strings.Count(green, "_None._") != 2 {
		t.Errorf("an all-green matrix should list no defects:\n%s", green)
	}
}
