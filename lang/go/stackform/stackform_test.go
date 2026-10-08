package stackform

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Unit tests for stackform — structural operations on StackForm
// values (Append, Len, Equal, Walk, Cost) that don't need a live
// engine. Compile-Eval round-trip and equivalence-with-direct-run
// tests live in lang/go/test/stackform_equivalence_test.go where
// the language-layer registry is available.

func TestStackFormAppendAndLen(t *testing.T) {
	f := &StackForm{}
	if f.Len() != 0 {
		t.Errorf("empty form len = %d, want 0", f.Len())
	}
	f.Append(PushLit{V: core.NewInteger(1)})
	f.Append(Call{Name: "add", Arity: 2})
	if f.Len() != 2 {
		t.Errorf("after 2 appends len = %d, want 2", f.Len())
	}
}

func TestStackFormEqual(t *testing.T) {
	a := &StackForm{Ops: []Op{
		PushLit{V: core.NewInteger(1)},
		PushLit{V: core.NewInteger(2)},
		Call{Name: "add", Arity: 2},
	}}
	b := &StackForm{Ops: []Op{
		PushLit{V: core.NewInteger(1)},
		PushLit{V: core.NewInteger(2)},
		Call{Name: "add", Arity: 2},
	}}
	if !Equal(a, b) {
		t.Error("identical forms should be Equal")
	}
	c := &StackForm{Ops: []Op{
		PushLit{V: core.NewInteger(1)},
		PushLit{V: core.NewInteger(3)},
		Call{Name: "add", Arity: 2},
	}}
	if Equal(a, c) {
		t.Error("forms differing in a literal should NOT be Equal")
	}
	d := &StackForm{Ops: []Op{
		PushLit{V: core.NewInteger(1)},
		PushLit{V: core.NewInteger(2)},
		Call{Name: "sub", Arity: 2},
	}}
	if Equal(a, d) {
		t.Error("forms differing in a Call name should NOT be Equal")
	}
}

func TestStackFormEqualNestedQuote(t *testing.T) {
	q1 := &StackForm{Ops: []Op{PushLit{V: core.NewInteger(42)}}}
	q2 := &StackForm{Ops: []Op{PushLit{V: core.NewInteger(42)}}}
	q3 := &StackForm{Ops: []Op{PushLit{V: core.NewInteger(43)}}}
	a := &StackForm{Ops: []Op{Quote{Body: q1}}}
	b := &StackForm{Ops: []Op{Quote{Body: q2}}}
	c := &StackForm{Ops: []Op{Quote{Body: q3}}}
	if !Equal(a, b) {
		t.Error("Quote bodies with identical Ops should be Equal")
	}
	if Equal(a, c) {
		t.Error("Quote bodies with different Ops should NOT be Equal")
	}
}

func TestStackFormWalk(t *testing.T) {
	inner := &StackForm{Ops: []Op{
		PushLit{V: core.NewInteger(99)},
		Call{Name: "neg", Arity: 1},
	}}
	form := &StackForm{Ops: []Op{
		PushLit{V: core.NewInteger(1)},
		Quote{Body: inner},
		Call{Name: "wrap", Arity: 1},
	}}
	var seen []string
	Walk(form, func(_ []int, op Op) bool {
		switch o := op.(type) {
		case PushLit:
			n, _ := core.AsInteger(o.V)
			seen = append(seen, "lit:"+itoa(n))
		case Call:
			seen = append(seen, "call:"+o.Name)
		case Quote:
			seen = append(seen, "quote")
		}
		return true
	})
	// Pre-order traversal: outer first, then descend into Quote bodies.
	want := []string{"lit:1", "quote", "lit:99", "call:neg", "call:wrap"}
	if len(seen) != len(want) {
		t.Fatalf("Walk visited %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("Walk[%d]=%q, want %q", i, seen[i], want[i])
		}
	}
}

func TestStackFormCost(t *testing.T) {
	form := &StackForm{Ops: []Op{
		PushLit{V: core.NewInteger(1)}, // 1
		PushLit{V: core.NewInteger(2)}, // 1
		Call{Name: "add", Arity: 2},    // 2
	}}
	if got := Cost(form); got != 4 {
		t.Errorf("Cost = %d, want 4 (1+1+2)", got)
	}
	nested := &StackForm{Ops: []Op{
		Quote{Body: form}, // 1 + Cost(form) = 1 + 4 = 5
	}}
	if got := Cost(nested); got != 5 {
		t.Errorf("Cost(nested) = %d, want 5", got)
	}
	if got := Cost(nil); got != 0 {
		t.Errorf("Cost(nil) = %d, want 0", got)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Replayable is the guard that stops a form the recorder could not capture
// faithfully from being replayed to a different answer than the program it
// came from (NUR077). Its arms are unit-tested here because two of them are
// unreachable from Compile: a nil form, and a Quote body — which the recorder
// never emits today (see the package doc), so only a hand-built form reaches
// the recursion.
func TestReplayable(t *testing.T) {
	named := Call{Name: "add", Arity: 2}
	unnamed := Call{Name: "", Arity: 1}

	t.Run("nil form is replayable", func(t *testing.T) {
		if err := Replayable(nil); err != nil {
			t.Errorf("Replayable(nil) = %v, want nil — an empty program replays as itself", err)
		}
	})

	t.Run("named calls are replayable", func(t *testing.T) {
		f := &StackForm{Ops: []Op{PushLit{V: core.NewInteger(1)}, named}}
		if err := Replayable(f); err != nil {
			t.Errorf("Replayable = %v, want nil — a word call re-invokes by name", err)
		}
	})

	t.Run("an unnamed call is declined", func(t *testing.T) {
		f := &StackForm{Ops: []Op{PushLit{V: core.NewInteger(1)}, unnamed}}
		if err := Replayable(f); err != ErrUnnamedApply {
			t.Errorf("Replayable = %v, want ErrUnnamedApply — an applied fn value has no name to re-invoke", err)
		}
	})

	t.Run("an unnamed call NESTED in a Quote is declined", func(t *testing.T) {
		// The recursion arm: a quoted sub-program is still replayed, so an
		// unreplayable op inside one is just as fatal as at the top level.
		f := &StackForm{Ops: []Op{Quote{Body: &StackForm{Ops: []Op{unnamed}}}}}
		if err := Replayable(f); err != ErrUnnamedApply {
			t.Errorf("Replayable = %v, want ErrUnnamedApply — a Quote body is not exempt", err)
		}
	})

	t.Run("a Quote of replayable ops is fine", func(t *testing.T) {
		f := &StackForm{Ops: []Op{Quote{Body: &StackForm{Ops: []Op{named}}}}}
		if err := Replayable(f); err != nil {
			t.Errorf("Replayable = %v, want nil — the recursion must not decline everything it walks", err)
		}
	})
}

// A Function pushed inside a Quote body needs the same inert-then-restored
// treatment as one at the top level, so flattenStamped merges the nested
// body's stamped set into its own. Without the merge, Eval would leave a
// nested reference permanently Quoted — the P1 defect, one level down.
//
// Hand-built because the recorder never emits Quote today (see the package
// doc); the vocabulary reserves it, and this keeps the recursion honest for
// when it does.
func TestFlattenStampsThroughQuote(t *testing.T) {
	fn := core.NewFunction(core.FnDefInfo{Name: "held"})
	if fn.Quoted {
		t.Fatal("fixture must start unquoted")
	}

	form := &StackForm{Ops: []Op{Quote{Body: &StackForm{Ops: []Op{PushLit{V: fn}}}}}}
	tokens, stamped := flattenStamped(form)

	if len(stamped) != 1 {
		t.Errorf("stamped = %v, want the nested Function — Eval cannot restore what it is not told about", stamped)
	}
	if len(tokens) != 1 {
		t.Fatalf("tokens = %v, want one quoted list", tokens)
	}
	inner, err := core.AsList(tokens[0])
	if err != nil || inner.Len() != 1 {
		t.Fatalf("nested body did not flatten to a one-element list: %v", tokens[0])
	}
	if !inner.Get(0).Quoted {
		t.Error("the nested Function was not marked inert — it would dispatch on replay")
	}
}
