package stackform

// Seam-5b wave E: unit tests for the stackform blocks no suite reached —
// nil-form entries, the DoEval / Quote arms of Cost / Flatten / Pretty,
// the opMarker methods, Walk early-stop, and opEqual mismatch arms.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// s5beFakeOp is an in-package Op variant unknown to the switches, driving
// the default arms of pretty and opEqual.
type s5beFakeOp struct{}

func (s5beFakeOp) opMarker() {}

func TestS5bEOpMarkers(t *testing.T) {
	// The marker methods are empty by design; invoke each directly so the
	// sealed-interface seam itself is executed.
	PushLit{}.opMarker()
	Call{}.opMarker()
	Quote{}.opMarker()
	DoEval{}.opMarker()
}

func TestS5bENilFormEntryPoints(t *testing.T) {
	if got := Flatten(nil); got != nil {
		t.Errorf("Flatten(nil) = %v, want nil", got)
	}
	if got := Pretty(nil); got != "" {
		t.Errorf("Pretty(nil) = %q, want empty", got)
	}
	var f *StackForm
	if got := f.Len(); got != 0 {
		t.Errorf("(nil).Len() = %d, want 0", got)
	}
	Walk(nil, func([]int, Op) bool { t.Error("visit must not run on a nil form"); return true })
	if !Equal(nil, nil) {
		t.Error("Equal(nil, nil) must be true")
	}
	if Equal(nil, &StackForm{}) {
		t.Error("Equal(nil, non-nil) must be false")
	}
}

func TestS5bECostDoEval(t *testing.T) {
	f := &StackForm{Ops: []Op{DoEval{}, Quote{Body: &StackForm{Ops: []Op{DoEval{}}}}}}
	// DoEval = 1, Quote = 1 + body cost (1) → 3.
	if got := Cost(f); got != 3 {
		t.Errorf("Cost = %d, want 3", got)
	}
}

func TestS5bEFlattenQuoteAndDoEval(t *testing.T) {
	inner := &StackForm{Ops: []Op{PushLit{V: core.NewInteger(1)}}}
	f := &StackForm{Ops: []Op{Quote{Body: inner}, DoEval{}}}
	out := Flatten(f)
	if len(out) != 2 {
		t.Fatalf("Flatten produced %d tokens, want 2", len(out))
	}
	if !core.IsQuoted(&out[0]) || !out[0].Parent.Equal(core.TList) {
		t.Errorf("Quote must flatten to a quoted list, got %v", out[0])
	}
	if !core.IsWord(&out[1]) {
		t.Fatalf("DoEval must flatten to the do word, got %v", out[1])
	}
	if w, _ := core.AsWord(&out[1]); w.Name != "do" {
		t.Errorf("DoEval word = %q, want do", w.Name)
	}
}

func TestS5bEPrettyDoEvalAndUnknownOp(t *testing.T) {
	f := &StackForm{Ops: []Op{DoEval{}, s5beFakeOp{}}}
	got := Pretty(f)
	if !strings.Contains(got, "do") {
		t.Errorf("Pretty should render do, got %q", got)
	}
	if !strings.Contains(got, "<?op:") {
		t.Errorf("Pretty should render the unknown-op marker, got %q", got)
	}
}

func TestS5bEWalkEarlyStop(t *testing.T) {
	inner := &StackForm{Ops: []Op{PushLit{V: core.NewInteger(1)}, PushLit{V: core.NewInteger(2)}}}
	f := &StackForm{Ops: []Op{Quote{Body: inner}, DoEval{}}}

	// Stop inside the Quote body: the nested walk returns false and the
	// outer walk must not visit DoEval.
	var seen []string
	Walk(f, func(path []int, op Op) bool {
		seen = append(seen, Pretty(&StackForm{Ops: []Op{op}}))
		if len(path) == 2 { // first op inside the quote body
			return false
		}
		return true
	})
	if len(seen) != 2 {
		t.Errorf("early stop inside quote: visited %d ops (%v), want 2", len(seen), seen)
	}

	// Stop at the top level.
	count := 0
	Walk(f, func(path []int, op Op) bool {
		count++
		return false
	})
	if count != 1 {
		t.Errorf("top-level early stop: visited %d ops, want 1", count)
	}
}

func TestS5bEOpEqualMismatchArms(t *testing.T) {
	if opEqual(PushLit{V: core.NewInteger(1)}, DoEval{}) {
		t.Error("PushLit vs DoEval must not be equal")
	}
	if !opEqual(DoEval{}, DoEval{}) {
		t.Error("DoEval vs DoEval must be equal")
	}
	if opEqual(DoEval{}, Call{Name: "x"}) {
		t.Error("DoEval vs Call must not be equal")
	}
	if opEqual(s5beFakeOp{}, s5beFakeOp{}) {
		t.Error("unknown op kinds fall through to false")
	}
	// Length mismatch short-circuits Equal.
	a := &StackForm{Ops: []Op{DoEval{}}}
	b := &StackForm{Ops: []Op{DoEval{}, DoEval{}}}
	if Equal(a, b) {
		t.Error("forms of different length must not be Equal")
	}
	// Quote bodies compare recursively.
	q1 := Quote{Body: &StackForm{Ops: []Op{PushLit{V: core.NewInteger(1)}}}}
	q2 := Quote{Body: &StackForm{Ops: []Op{PushLit{V: core.NewInteger(2)}}}}
	if opEqual(q1, q2) {
		t.Error("quotes with different bodies must not be equal")
	}
}
