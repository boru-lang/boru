package lang

// Store-identity context typing, stage 1 (design/checker-precision-
// fronts.0.md §2): store-class containers — context Store layers,
// `flex` maps, patrun dispatch tables — carry an abstract
// core.StoreShapeInfo minted per creation site / live layer in CHECK
// MODE, and set/get/add/find over a shaped carrier read/write ITS
// shape instead of the flat CheckState.ContextTypes namespace. The
// flat map remains the compatibility fallback for any store the
// minting misses, so precision only increases; every shape path is
// gated to plain (non-Compiling) passes so the compiled stream is
// byte-identical (store/flex/patrun rows compile natively today).

import (
	"fmt"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// shapeCheckStack runs a plain Check and returns the residual stack
// strings plus whether any error-severity diagnostic was reported.
func shapeCheckStack(t *testing.T, src string) ([]string, bool) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, cerr := a.Check(src)
	if cerr != nil {
		t.Fatalf("Check(%q): %v", src, cerr)
	}
	return res.Stack, res.Summary.Errors > 0
}

// TestStoreShapeTwoStoresNoJoin — the spec's HEADLINE: two DIFFERENT
// stores' same-named keys no longer join. Each creation site mints its
// own shape, so a's key stays Integer-bounded although b wrote a
// String under the same name (the flat map would join them to a
// Disjunct for both readers).
func TestStoreShapeTwoStoresNoJoin(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`def a (flex {}) def b (flex {}) set k 1 a end set k 'x' b end a.k`,
			"FlexMap FlexMap dynamic(Integer)"},
		{`def a (flex {}) def b (flex {}) set k 1 a end set k 'x' b end b.k`,
			"FlexMap FlexMap dynamic(ProperString)"},
		// Two patrun instances: each find is bounded by ITS OWN table's
		// DECLARED value type (∪ None), not the other's.
		{`def r (patrun Integer) def s (patrun String) add {a:1} 7 r add {a:1} 'x' s find {a:1} r is Integer`,
			"dynamic(Boolean)"},
	}
	for _, c := range cases {
		stack, flagged := shapeCheckStack(t, c.src)
		if flagged {
			t.Errorf("%q: checker flagged an error; want clean", c.src)
			continue
		}
		if got := strings.Join(stack, " "); got != c.want {
			t.Errorf("%q: residual %q, want %q", c.src, got, c.want)
		}
	}
}

// TestStoreShapeContextLayerTyping — context Store typing keyed by the
// LIVE layer: every `context` call in one scope shares one shape (the
// set-then-get across two `context` calls keeps its concrete-fold
// parity with the flat map), and a sub-run's pushed layer (`do` body)
// is a DIFFERENT store — its same-named write no longer contaminates
// the outer read (the flat map alone joined them; runtime agrees with
// the shape, because a scoped layer's writes vanish on pop).
func TestStoreShapeContextLayerTyping(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		// single-scope parity (storage.tsv semantics, unchanged)
		{`context set 'x' 42 end context get 'x'`, "Integer"},
		{`context set 'n' 7 end context get 'n' add 3`, "Integer"},
		// cross-layer isolation: the do-body layer's String write does not
		// widen the outer layer's Integer claim. (The leading Error carrier
		// is do's caught-error modelling of its value-less body.)
		{`context set 'k' 1 end do [context set 'k' 'str' end] context get 'k'`,
			"Error Integer"},
		// same-layer read inside the do body sees the body's own write.
		{`context set 'k' 1 end do [context set 'k' 'str' end context get 'k']`,
			"ProperString"},
	}
	for _, c := range cases {
		stack, flagged := shapeCheckStack(t, c.src)
		if flagged {
			t.Errorf("%q: checker flagged an error; want clean", c.src)
			continue
		}
		if got := strings.Join(stack, " "); got != c.want {
			t.Errorf("%q: residual %q, want %q", c.src, got, c.want)
		}
	}
	// Runtime agreement for the isolation row: the scoped write vanishes.
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, rerr := a.RunInterp(`context set 'k' 1 end do [context set 'k' 'str' end] context get 'k'`)
	if rerr != nil || fmt.Sprint(got) != "[1]" {
		t.Errorf("layer-isolation runtime: got %v err=%v, want [1]", got, rerr)
	}
}

// TestStoreShapeFlexTyping — `flex` is a store-creating word: a
// concrete map source mints the shape (nested maps recursively,
// mirroring FlexDeepCopy), set-writes thread through it, reads surface
// GRADUAL bounds (never strict/concrete — flex trees have writers the
// shape cannot see), aliases share the shape the way runtime handles
// share the tree, and a flex-of-flex CLONES it (runtime deep copy
// disconnects aliasing).
func TestStoreShapeFlexTyping(t *testing.T) {
	cases := []struct {
		src  string
		want string
		why  string
	}{
		{`def f (flex {a:7}) f.a`, "dynamic(Integer)", "creation-site field"},
		{`def f (flex {a:7}) f.a add 1`, "dynamic(Integer)", "bound feeds downstream dispatch"},
		{`def f (flex {}) set a "s" f end f.a`, "FlexMap dynamic(ProperString)", "set-then-read"},
		{`def m {a:{b:1}} def f (flex m) set b/q 9 f.a drop f.a.b`, "dynamic(Integer)",
			"nested shape: chained reads narrow, writes through a read alias land"},
		{`def h (flex {z:1}) def f (flex {}) set a h f end set z 9 f.a end h.z`,
			"FlexMap dynamic(Disjunct) dynamic(Integer)",
			"handle stored in another flex: shape pointer shared, write visible via h"},
		{`def f (flex {a:1}) def g (flex f) set a 'x' g end f.a`,
			"FlexMap dynamic(Integer)",
			"flex-of-flex clones: g's write does not widen f's claim"},
	}
	for _, c := range cases {
		stack, flagged := shapeCheckStack(t, c.src)
		if flagged {
			t.Errorf("%q (%s): checker flagged an error; want clean", c.src, c.why)
			continue
		}
		if got := strings.Join(stack, " "); got != c.want {
			t.Errorf("%q (%s): residual %q, want %q", c.src, c.why, got, c.want)
		}
	}
}

// TestStoreShapePatrunTyping — a patrun's find is bounded by the
// DECLARED value type of THAT instance (`patrun T`), ∪ None (the miss
// result): a gradual bound over the dynamic-dispatch container, never a
// proof of which rule matches.
func TestStoreShapePatrunTyping(t *testing.T) {
	cases := []struct {
		src     string
		want    string
		wantRun string
	}{
		{`def r (patrun String)  add {a:1} "A" r  add {a:1 b:1} "B" r  find {a:1} r`,
			"dynamic(Disjunct)", "[A]"},
		{`def r (patrun String)  add {a:1} "A" r  remove {a:1} r  find {a:1} r`,
			"dynamic(Disjunct)", "[None]"}, // the declared bound is String ∪ None; a miss reads None
		{`def r (patrun Integer)  add {a:1} 7 r  add {b:1} 9 r  find {a:1} r`,
			"dynamic(Disjunct)", "[7]"},
	}
	for _, c := range cases {
		stack, flagged := shapeCheckStack(t, c.src)
		if flagged {
			t.Errorf("%q: checker flagged an error; want clean", c.src)
			continue
		}
		if got := strings.Join(stack, " "); got != c.want {
			t.Errorf("%q: residual %q, want %q", c.src, got, c.want)
		}
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, rerr := a.RunInterp(c.src)
		if rerr != nil || fmt.Sprint(got) != c.wantRun {
			t.Errorf("%q: runtime got %v err=%v, want %s", c.src, got, rerr, c.wantRun)
		}
	}
}

// TestStoreShapeNegatives — the compat/decline contract: a store the
// minting missed keeps the flat-map path entirely; a shaped container's
// UNSEEN key stays dynamic(Any) (untracked writers exist — never None);
// a typed patrun surfaces its DECLARED value type ∪ None (a
// Function-valued table reads dynamic(Function|None), no longer a
// poisoned dynamic(Any) — the typed refactor cleared patrun.tsv:40).
func TestStoreShapeNegatives(t *testing.T) {
	cases := []struct {
		src  string
		want string
		why  string
	}{
		// A fn-param Store is an UNSHAPED carrier at body-analysis time:
		// set/get inside the body run today's flat-map path — set-then-get
		// still types (nothing regresses where the minting doesn't reach).
		{`def f fn [[s:Store] [Integer] [set 'zq9' 42 s end s get 'zq9']]  f (context)`,
			"Integer", "unshaped store keeps the flat-map set-then-get typing"},
		{`def f fn [[s:Store] [Any] [s get 'zq_never']]  f (context)`,
			"dynamic(Any)", "unshaped store, unseen key: today's dynamic(Any)"},
		{`def f (flex {a:1}) f.b`, "dynamic(Any)",
			"shaped flex, unseen key: dynamic(Any), never None (untracked writers exist)"},
		{`def f (flex {a:([x:Any] => [x])}) f.a`, "dynamic(Any)",
			"dispatch-bearing field omitted from the shape: the fn-value hatch stands"},
		{`def r (patrun Function)  add {a:1} ([m:Map] => [m.x]) r  find {a:1} r`, "dynamic(Disjunct)",
			"a Function-valued table surfaces dynamic(Function|None), not a poisoned Any"},
		{`def r (patrun String)  find {a:1} r`, "dynamic(Disjunct)",
			"empty table: the declared bound String ∪ None still applies (a miss reads None)"},
	}
	for _, c := range cases {
		stack, flagged := shapeCheckStack(t, c.src)
		if flagged {
			t.Errorf("%q (%s): checker flagged an error; want clean", c.src, c.why)
			continue
		}
		if got := strings.Join(stack, " "); got != c.want {
			t.Errorf("%q (%s): residual %q, want %q", c.src, c.why, got, c.want)
		}
	}
}

// TestStoreShapeObservationFree — the §2 hazard pinned as an
// assertion: the check pass mutates NO real store. The shape is a new
// abstract payload, the real *StoreInstanceInfo never flows through
// check mode (toCarrier strips it; contextReturns mints the abstract
// twin), and the check-mode set intercept never runs CowSet — so the
// live context layer's Data and the stack itself are untouched by a
// check pass full of ctx writes.
func TestStoreShapeObservationFree(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	root := reg.Contexts.Top()
	if root == nil {
		t.Fatal("no root context")
	}
	beforeKeys := len(root.Data)
	beforeDepth := reg.Contexts.Depth()

	values, perr := Parse(`context set 'zzob' 99 end context get 'zzob' drop ` +
		`def f (flex {a:1}) set b/q 2 f end ` +
		`def r (patrun String) add {a:1} 'A' r find {a:1} r`)
	if perr != nil {
		t.Fatalf("parse: %v", perr)
	}
	done := reg.Check.Begin()
	_, runErr := core.NewTop(reg).Run(values)
	done()
	if runErr != nil {
		t.Fatalf("check run: %v", runErr)
	}

	if got := reg.Contexts.Top(); got != root {
		t.Errorf("check pass replaced the live context layer (COW ran): %p != %p", got, root)
	}
	if reg.Contexts.Depth() != beforeDepth {
		t.Errorf("check pass left the context stack at depth %d, want %d", reg.Contexts.Depth(), beforeDepth)
	}
	if len(root.Data) != beforeKeys {
		t.Errorf("check pass mutated the real context store: %d keys, want %d (%v)",
			len(root.Data), beforeKeys, root.Data)
	}
	if _, leaked := root.Data["zzob"]; leaked {
		t.Error("check-mode ctx-set wrote through to the REAL store — the pass must be observation-free")
	}
}

// TestStoreShapeCompileDiscipline — checker precision must NOT change
// the compile pass. Store/flex/patrun rows compile natively today
// (through the flat-map typing and the dynamic hatches); with every
// shape path gated to !Compiling they must keep compiling to the same
// behavior — and the one compile failure in the family keeps declining with the
// same reason.
func TestStoreShapeCompileDiscipline(t *testing.T) {
	compiles := []struct {
		src string
	}{
		{`context set 'x' 42 end context get 'x' add 3`},
		{`def f (flex {a:7}) f.a add 1`},
		{`def f (flex {}) set a "s" f end f.a`},
		{`def r (patrun String)  add {a:1} "A" r  find {a:1} r`},
	}
	for _, c := range compiles {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: expected native compile (as before the shape work); reason=%q err=%v",
				c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: compiled with a FALLBACK island; the shape work must not change lowering", c.src)
		}
		b, _ := New()
		gotC, compiled, errC := b.RunCompiled(c.src)
		if noteCompileDefect(t, c.src, gotC, errC) {
			continue
		}
		d, _ := New()
		gotI, errI := d.RunInterp(c.src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: compiled/interpreted parity broke: compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
				c.src, compiled, gotC, errC, gotI, errI)
		}
	}

	// The family's historical compile failure (`set` over the dynamic flex read
	// committed the 0-return Store overload and starved the drop) now
	// COMPILES: the compile pass narrows through the same shape bound the
	// plain check uses, so `set` commits a container overload with the
	// right result arity, and the dispatch stays a runtime-re-matched poly
	// whose NOut claim the VM enforces (flex.tsv:88/95).
	src := `def m {a:{b:1}} def f (flex m) set b/q 9 f.a drop f.a.b`
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("%q: CompileCheck error %v", src, cerr)
	}
	if prog == nil {
		t.Errorf("%q: expected a native compile after the flex-shape narrowing; declined: %q", src, reason)
	}
	b, _ := New()
	got, compiled, rerr := b.RunCompiled(src)
	if noteCompileDefect(t, src, got, rerr) {
		return
	}
	if !compiled || rerr != nil || fmt.Sprint(got) != "[9]" {
		t.Errorf("%q: compiled run got %v compiled=%v err=%v, want [9] compiled", src, got, compiled, rerr)
	}
}
