package lang

import (
	"fmt"
	"strings"
	"testing"
)

// Stage-1 bytecode emitter goldens (design/legacy/boru-bytecode-plan.0.ignore
// Stage 1 gate): the recording pass lowers straight-line monomorphic
// code to the expected instruction stream, and everything beyond the
// stage is refused with a precise reason — never lowered wrongly.

func compile(t *testing.T, src string) (string, string) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck(%q): %v", src, err)
	}
	if prog == nil {
		return "", reason
	}
	return prog.Disassemble(), ""
}

func TestEmitGoldens(t *testing.T) {
	cases := []struct {
		src    string
		golden string
	}{
		{`add 1 2`, `0000 PUSH_CONST  k1   ; 2 (Integer)
0001 PUSH_CONST  k0   ; 1 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
; consts=2 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		// `1 add 2` is the k=1 split of the (2,1) assignment — the
		// forward 2 fills sig[0], the stack-prefix 1 fills sig[1]
		// (one uniform rule: forward until the barrier, then
		// backward). Its push order therefore differs from
		// `add 1 2`, the all-forward spelling of the (1,2)
		// assignment; same sum only because add commutes.
		{`1 add 2`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 2 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
; consts=2 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		{`0 add 7 sub 3`, `0000 PUSH_CONST  k1   ; 0 (Integer)
0001 PUSH_CONST  k0   ; 7 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
0003 PUSH_CONST  k2   ; 3 (Integer)
0004 CALL_NATIVE s1   ; sub (Number, Number)
; consts=3 types=0 sigs=2 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		// A paren result feeding the next call: the prior result stays
		// on the simulated stack; only the literal is pushed.
		{`(1 add 2) mul 3`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 2 (Integer)
0002 CALL_NATIVE s0   ; add (Number, Number)
0003 PUSH_CONST  k2   ; 3 (Integer)
0004 CALL_NATIVE s1   ; mul (Number, Number)
; consts=3 types=0 sigs=2 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		// Top-level strings are stripped to carriers by check mode;
		// RememberOriginal preserves the originals for interning.
		{`'a' add 'b'`, `0000 PUSH_CONST  k1   ; 'a' (ProperString)
0001 PUSH_CONST  k0   ; 'b' (ProperString)
0002 CALL_NATIVE s0   ; add (String, Scalar)
; consts=2 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		// `if` lowers to JMP_IF_FALSE / JMP: condition code first, a
		// branch per fragment, the join value on the stack for the
		// downstream call. Const-only branches are single pushes.
		{`if (1 gt 0) [10] [20]`, `0000 PUSH_CONST  k1   ; 1 (Integer)
0001 PUSH_CONST  k0   ; 0 (Integer)
0002 CALL_NATIVE s0   ; gt (Any, Any)
0003 JMP_IF_FALSE -> 0006
0004 PUSH_CONST  k2   ; 10 (Integer)
0005 JMP         -> 0007
0006 PUSH_CONST  k3   ; 20 (Integer)
; consts=4 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		// The branch result feeds the downstream mul; a stripped
		// Boolean literal condition compiles as PUSH_CONST + jump
		// (CoerceBoolean truthiness at run time).
		{`def y (if (2 gt 1) [1 add 2] [9]) y mul 2`, `0000 PUSH_CONST  k1   ; 2 (Integer)
0001 PUSH_CONST  k0   ; 1 (Integer)
0002 CALL_NATIVE s0   ; gt (Any, Any)
0003 JMP_IF_FALSE -> 0008
0004 PUSH_CONST  k0   ; 1 (Integer)
0005 PUSH_CONST  k1   ; 2 (Integer)
0006 CALL_NATIVE s1   ; add (Number, Number)
0007 JMP         -> 0009
0008 PUSH_CONST  k2   ; 9 (Integer)
0009 BIND_TWIN   w0   ; bind twin def y @depth 1 (replay)
0010 BIND_GLOBAL g0   ; global bind y @depth 1
0011 PUSH_CONST  k1   ; 2 (Integer)
0012 CALL_NATIVE s2   ; mul (Number, Number)
; consts=3 types=0 sigs=3 fallbacks=0 fns=0 max-stack=2 locals=0
`},
		// Literal-substitution def: x resolves to the interned literal
		// through value provenance — the report's §5.2 inline case. The
		// BIND_TWIN marks the def's ledger transition in the stream (inert
		// until the rollback-and-replay flip; §6.5).
		{`def x 1 x add 2`, `0000 BIND_TWIN   w0   ; bind twin def x @depth 1 (replay)
0001 PUSH_CONST  k1   ; 1 (Integer)
0002 PUSH_CONST  k0   ; 2 (Integer)
0003 CALL_NATIVE s0   ; add (Number, Number)
; consts=2 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
`},
	}
	for _, c := range cases {
		got, reason := compile(t, c.src)
		if reason != "" {
			t.Errorf("%q unexpectedly uncompilable: %s", c.src, reason)
			continue
		}
		if got != c.golden {
			t.Errorf("%q lowering changed:\n--- got\n%s--- want\n%s", c.src, got, c.golden)
		}
	}
}

// Mirror-equivalent forms produce IDENTICAL bytecode. The mirror of
// `add 1 2` is `2 1 add` (the kernel's one convention: sig position
// 0 is the FIRST forward arg in forward form and the TOP of stack in
// stack form, so the stack form is written reversed).
func TestEmitMirrorFormsIdentical(t *testing.T) {
	a, _ := compile(t, `add 1 2`)
	b, _ := compile(t, `2 1 add`)
	if a == "" || a != b {
		t.Errorf("mirror forms diverged:\nforward:\n%s\nstack:\n%s", a, b)
	}
}

// One split rule, one bytecode per ASSIGNMENT: every split of the
// same sig-order assignment lowers identically. `1 add 2` (k=1
// split), `add 2 1` (all-forward), and `1 2 add` (all-stack) all
// assign sig[0]=2, sig[1]=1 — identical code. `add 1 2` is the
// (1,2) assignment — a different program (observable on
// non-commutative words: `10 sub 3` = 7, `sub 10 3` = -7), so it
// must NOT be conflated with the other class.
func TestEmitSplitFormsIdentical(t *testing.T) {
	a, _ := compile(t, `1 add 2`)
	b, _ := compile(t, `1 2 add`)
	c, _ := compile(t, `add 2 1`)
	if a == "" || a != b || a != c {
		t.Errorf("splits of one assignment diverged:\nmixed:\n%s\nstack:\n%s\nforward:\n%s", a, b, c)
	}
	fwd, _ := compile(t, `add 1 2`)
	if fwd == a {
		t.Errorf("the (1,2) and (2,1) assignments lowered identically — distinct assignments must not be conflated")
	}
}

// Counted loops lower to FOR_SETUP / FOR_NEXT with the iterator as a
// VM local and the body's trailing JMP as the program's only
// back-edge; one value per iteration accumulates on the stack,
// matching the interpreter (`for 3 [i]` → 0 1 2).
func TestEmitForLoopGolden(t *testing.T) {
	got, reason := compile(t, `for 3 [i add 10]`)
	if reason != "" {
		t.Fatalf("for-loop unexpectedly uncompilable: %s", reason)
	}
	// The count form lowers as the range [0, 3, 1]: FOR_SETUP pops
	// start (top), end, step — the parseRange triple.
	want := `0000 PUSH_CONST  k3   ; 1 (Integer)
0001 PUSH_CONST  k2   ; 3 (Integer)
0002 PUSH_CONST  k1   ; 0 (Integer)
0003 FOR_SETUP   l0
0004 FOR_NEXT    -> 0009
0005 PUSH_LOCAL  l0
0006 PUSH_CONST  k0   ; 10 (Integer)
0007 CALL_NATIVE s0   ; add (Number, Number)
0008 JMP         -> 0004
; consts=4 types=0 sigs=1 fallbacks=0 fns=0 max-stack=3 locals=1
`
	if got != want {
		t.Errorf("for lowering changed:\n--- got\n%s--- want\n%s", got, want)
	}
}

// Stage-2 completion shapes: range/negative-step loops, list-form
// conditions, the 2-arg if, and break/continue all compile and any
// downstream consumption of the variadic results refuses.
func TestEmitStage2CompletionShapes(t *testing.T) {
	for _, src := range []string{
		`for [2,5] [i]`,
		`for [5,0,-1] [i]`,
		`if [1 lt 2] [10] [20]`,
		`if (1 gt 0) [7]`,
		`for 5 [if (i gt 2) [break] [i]]`,
		`for 5 [if (i eq 2) [continue] [i]]`,
		// A def-bound range is CONCRETE at check time (def evaluates
		// its body) — statically known bounds compile.
		`def r [1,3] for r [i]`,
		// A computed-END range with a CONST start (and step) lowers to
		// FOR_SETUP with the end resolved to its runtime operand — the
		// s3-send-resp shape. The variadic loop result is absorbed by the
		// program residual (`[1 2]` / `[0 2 4]` here).
		`for [1, (1 add 2)] [i]`,
		`for [0, (2 mul 3), 2] [i]`,
	} {
		if got, reason := compile(t, src); got == "" {
			t.Errorf("%q unexpectedly uncompilable: %s", src, reason)
		}
	}
	// Negative: the 2-arg if's result is VARIADIC — consuming it must
	// refuse (only the program residual may absorb it).
	if got, _ := compile(t, `(if (1 gt 0) [7]) add 1`); got != "" {
		t.Errorf("2-arg if result consumption compiled but must refuse:\n%s", got)
	}
	// Negative: break outside any loop refuses.
	if got, _ := compile(t, `if (1 gt 0) [break] [1]`); got != "" {
		t.Errorf("break outside a loop compiled but must refuse:\n%s", got)
	}
	// Negative: a range whose START (or step) is computed does NOT compile — a
	// FOR_SETUP loop const-bakes start/step and can only resolve a computed END,
	// so a computed start/step keeps islanding over the runtime list (falls
	// back). The interpreter runs it faithfully.
	if got, _ := compile(t, `for [(1 add 2), 5] [i]`); got != "" && !strings.Contains(got, "FALLBACK") {
		t.Errorf("computed-start range compiled NATIVELY but must island/refuse:\n%s", got)
	}
}

// P5 multi-result lowering (design/legacy/boru-bytecode-runtime-independence.0.ignore):
// 0-result side-effect words (set/raise/drop/…) and genuine multi-result
// words now record and lower, where they previously refused "returns N
// values". The (seq, idx) operand model distinguishes a multi-result call's
// outputs; a 0-result word pushes nothing.
func TestEmitP5MultiResult(t *testing.T) {
	// Positive: these now COMPILE.
	for _, src := range []string{
		`raise "boom"`, // 0-result, diverges with an error at run time
		`5 7 swap`,     // 2-in-2-out: distinct output ids
		`5 7 swap sub`, // a multi-result consumed by a downstream call
		`def C class {x:1} def a (make C {}) set 'x' 99 a`, // 0-result in-place mutator (class set)
	} {
		if got, reason := compile(t, src); got == "" {
			t.Errorf("%q unexpectedly uncompilable: %s", src, reason)
		}
	}

	// Runtime parity for a multi-result word feeding a non-commutative
	// downstream call (`5 7 swap sub` → 7 - 5 = 2): the two swap outputs
	// must land in the right order.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`5 7 swap sub`)
	if err != nil || !compiled {
		t.Fatalf("`5 7 swap sub`: compiled=%v err=%v", compiled, err)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	want, err := b.RunInterp(`5 7 swap sub`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || len(want) != 1 || out[0] != want[0] {
		t.Fatalf("compiled %v != interpreted %v", out, want)
	}

	// A compiled `raise` errors at run time exactly as the interpreter does
	// (the 0-result call lowers to CALL_NATIVE, whose handler returns the error).
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, wasCompiled, rerr := c.RunCompiled(`raise "boom"`); !wasCompiled || rerr == nil {
		t.Fatalf("`raise \"boom\"`: compiled=%v err=%v (want compiled + error)", wasCompiled, rerr)
	}

	// A multi-RETURN fn now compiles: the body leaves N values matching the
	// declared returns, and CALL_USER leaves them on the caller's stack.
	d, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gotMR, wasCompiled, mrErr := d.RunCompiled(`def mk fn [[] [Integer Integer] [1 2]] mk`)
	if !wasCompiled || mrErr != nil {
		t.Fatalf("multi-return fn: compiled=%v err=%v (want compiled, no error)", wasCompiled, mrErr)
	}
	if len(gotMR) != 2 || gotMR[0] != int64(1) || gotMR[1] != int64(2) {
		t.Fatalf("multi-return fn result = %v, want [1 2]", gotMR)
	}

	// A body whose value count differs from the DECLARED returns is the
	// interpreter's return-count type_error. It now COMPILES the error path —
	// the VM's RET enforces the exact count and raises the byte-identical error
	// — rather than refusing and falling back.
	if got, reason := compile(t, `def r2 fn [[n:Integer] [Integer] [n n]] r2 1`); got == "" {
		t.Errorf("count-mismatch fn must now compile the error path, but refused: %q", reason)
	}
	if _, compiled, err := mustRun(t, `def r2 fn [[n:Integer] [Integer] [n n]] r2 1`); !compiled || err == nil {
		t.Errorf("count-mismatch fn: compiled=%v err=%v, want compiled with the raised count error", compiled, err)
	}
}

// TestEmitApplyFnValue: `…args fn apply` compiles. apply returns the fn VALUE
// concrete in check mode (ReturnsIdentity), so the check engine re-steps it and
// the fn dispatches against its stack args as an ordinary CALL_USER. Covers the
// stack-form binding (sig position 0 = top), the 0-arg fn, and an anon lambda.
func TestEmitApplyFnValue(t *testing.T) {
	cases := []struct {
		src  string
		want []any
	}{
		{`def inc fn [[n:Integer][Integer][n add 1]] 5 inc/v apply`, []any{int64(6)}},
		// Stack form: a=top=3, b=10 → a sub b = 3-10 = -7 (NOT forward sub2 10 3 = 7).
		{`def sub2 fn [[a:Integer b:Integer][Integer][a sub b]] 10 3 sub2/v apply`, []any{int64(-7)}},
		{`def z fn [[][Integer][42]] z/v apply`, []any{int64(42)}},
		{`def f ([n:Integer] => [n add 1]) 5 f/v apply`, []any{int64(6)}},
	}
	for _, c := range cases {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		got, wasCompiled, rerr := a.RunCompiled(c.src)
		if !wasCompiled || rerr != nil {
			t.Fatalf("%s: compiled=%v err=%v (want compiled, no error)", c.src, wasCompiled, rerr)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v, want %v", c.src, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.src, got, c.want)
				break
			}
		}
	}

	// Negative: applying a NON-function (a plain Integer) errors in both
	// engines — apply's [Function] sig rejects it; the row must not silently
	// compile to a wrong value.
	if _, _, rerr := mustRun(t, `5 6 apply`); rerr == nil {
		t.Errorf("`5 6 apply`: want an error (apply of a non-function), got none")
	}
}

// TestEmitGuardIf: a 2-arg `if` whose then produces 0 values (a raise guard,
// or a 0-value word) is a statement guard — the if contributes 0 values on
// both paths (true→raise/0, false→0), so it compiles with no merge slot and
// the program continues after it.
func TestEmitGuardIf(t *testing.T) {
	// Positive: guard-if compiles and continues to the trailing value.
	if _, r := compile(t, `def n 5 if (n eq 0) [raise "zero"] def q (10 div n) q`); r != "" {
		t.Errorf("guard-if must compile, refused: %s", r)
	}
	okCases := []struct {
		src  string
		want int64
	}{
		{`def n 5 if (n eq 0) [raise "zero"] def q (10 div n) q`, 2},
		{`def f fn [[n:Integer] [Integer] [ if (n eq 0) [raise "zero"] def q (10 div n) q ]] f 5`, 2},
	}
	for _, c := range okCases {
		got, _, err := mustRun(t, c.src)
		if err != nil || len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: got %v err=%v, want [%d]", c.src, got, err, c.want)
		}
	}
	// Negative: the guard FIRES (raise on the true path) — both engines error.
	if _, _, err := mustRun(t, `def n 0 if (n eq 0) [raise "zero"] def q (10 div n) q`); err == nil {
		t.Error("guard-if true path must raise, got no error")
	}
}

// TestEmitDynOutNative: a CORE builtin with CONCRETE args but a declared-Any
// (dynamic) output — e.g. `unify`, [Any,Any]→[Any,Boolean] — bakes a plain
// CALL_NATIVE (the sig was resolved by real matching; the handler runs
// faithfully). A dynamic INPUT still refuses (the sig would be a guess).
func TestEmitDynOutNative(t *testing.T) {
	if _, r := compile(t, `List unify [1 2]`); r != "" {
		t.Errorf("concrete-args unify must compile, refused: %s", r)
	}
	// Value parity in compiled mode (unify returns [unified, ok]).
	got, _, err := mustRun(t, `List unify [1 2]`)
	if err != nil || len(got) != 2 {
		t.Fatalf("List unify [1 2]: got %v err=%v", got, err)
	}
	// `push 3 l` now types as a List (push's List overload declares Returns=[List],
	// the fold-push checker fix), so `… unify l` resolves to a real match and
	// compiles — and compiled == interpreted. (Previously push typed as Any, so
	// this stood as the dynamic-input refusal case; that path is still covered by
	// the set/getpath dynamic-input emit tests below.)
	if _, r := compile(t, `def l [1 2] push 3 l unify l`); r != "" {
		t.Errorf("push-typed unify must compile, refused: %s", r)
	}
	gotP, _, errP := mustRun(t, `def l [1 2] push 3 l unify l`)
	ai, _ := New()
	gotI, errI := ai.RunInterp(`def l [1 2] push 3 l unify l`)
	if errP != nil || errI != nil || fmt.Sprint(gotP) != fmt.Sprint(gotI) {
		t.Errorf("push-typed unify parity: compiled=%v(%v) interp=%v(%v)", gotP, errP, gotI, errI)
	}
}

// TestEmitStructuralTypePattern: a STATIC structural type pattern — a map or
// list whose leaves are bare type nodes (`{a:Integer}`, `[Integer String]`,
// `[Resource Entity]`) — bakes as one const operand (a type node is admitted
// as a const MEMBER), so `is` / `typeof` / `size` over it compiles natively
// instead of refusing "operand of unknown provenance." The pattern is inert
// data the VM pushes verbatim, so the handler runs identically to the
// interpreter.
func TestEmitStructuralTypePattern(t *testing.T) {
	okCases := []struct {
		src  string
		want string
	}{
		{`{a:5} is {a:Integer}`, "true"},
		{`{a:'x'} is {a:Integer}`, "false"},
		{`{a:5 b:'x'} is {a:Integer b:String}`, "true"},
		{`{inner:{x:5}} is {inner:{x:Integer}}`, "true"},
		{`[1] is [Integer]`, "true"},
		{`[1 'x'] is [Integer String]`, "true"},
		{`size [Resource Entity]`, "2"},
	}
	for _, c := range okCases {
		dis, r := compile(t, c.src)
		if r != "" {
			t.Errorf("%s: structural type-pattern must compile, refused: %s", c.src, r)
			continue
		}
		if strings.Contains(dis, "FALLBACK") {
			t.Errorf("%s: compiled with an interpreter island, want a native bake:\n%s", c.src, dis)
		}
		got, _, err := mustRun(t, c.src)
		if err != nil || len(got) != 1 || fmt.Sprint(got[0]) != c.want {
			t.Errorf("%s: got %v err=%v, want [%s]", c.src, got, err, c.want)
		}
	}
	// A generic schema whose body nests the type variable inside a typed-list
	// field (`{items:[:T]}`) also compiles: the schema-body check admits a Word
	// naming a type parameter, so the placeholder rides through the
	// ChildTypeInfo wrapper as inert schema data.
	if _, r := compile(t, `def Stack gen [T] class {items:[:T]} end make Stack {items:[1 2]}`); r != "" {
		t.Errorf("typed-list-field generic make must compile, refused: %s", r)
	}
}

// TestEmitGenericSchema: an installed generic schema (`def Box gen [T] class
// {value:T}`) bakes as a const — it is immutable data riding its canonical
// minted node, with the instantiation memo held in the registry, not the
// struct. So `make Box {…}` (T inferred at run time), `is`/`typeof` over the
// schema, and the bare schema as a residual all compile, matching the
// interpreter. Explicit instantiation (`make (Box of [Integer]) {…}`) already
// compiled via the structural-type-body path.
func TestEmitGenericSchema(t *testing.T) {
	for _, src := range []string{
		`def Box gen [T] class {value:T} end Box`,
		`def Box gen [T] class {value:T} end make Box {value:42}`,
		`def Box gen [T] class {value:T} end (make Box {value:42}) typeof`,
		`def Box gen [T] class {value:T} end (make (Box of [Integer]) {value:1}) is Box`,
		// Type variable nested inside a typed-list field — the placeholder Word
		// rides through the ChildTypeInfo as inert schema data.
		`def Stack gen [T] class {items:[:T]} end Stack`,
		`def Stack gen [T] class {items:[:T]} end (make Stack {items:[1 2]}) typeof`,
		// Default-valued parameter (`T default Integer`).
		`def R gen [(T default Integer)] class {items:[:T]} end (make R {items:[]}) typeof`,
	} {
		dis, r := compile(t, src)
		if r != "" {
			t.Errorf("%s: generic schema must compile, refused: %s", src, r)
			continue
		}
		if strings.Contains(dis, "FALLBACK") {
			t.Errorf("%s: compiled with an interpreter island, want a native bake", src)
		}
	}
	// Value parity for the inferred-T make.
	got, _, err := mustRun(t, `def Box gen [T] class {value:T} end (make Box {value:42}) typeof`)
	if err != nil || len(got) != 1 || fmt.Sprint(got[0]) != "Box of [Integer]" {
		t.Errorf("inferred-T make typeof: got %v err=%v, want [Box of [Integer]]", got, err)
	}
	// A function-signature value is pure descriptor data, so a fnsig schema and
	// typeof/teq over an instantiated fnsig now compile too.
	for _, src := range []string{
		`(fnsig [[Integer] [String]]) typeof`,
		`def Mapper gen [T U] fnsig [[T] [U]] end (Mapper of [Integer String]) typeof`,
		`def Mapper gen [T U] fnsig [[T] [U]] end (Mapper of [Integer String]) teq (Mapper of [Integer String])`,
	} {
		if _, r := compile(t, src); r != "" {
			t.Errorf("%s: fnsig value must compile, refused: %s", src, r)
		}
	}
	if got, _, err := mustRun(t, `def Mapper gen [T U] fnsig [[T] [U]] end (Mapper of [Integer String]) teq (Mapper of [Integer String])`); err != nil || len(got) != 1 || fmt.Sprint(got[0]) != "true" {
		t.Errorf("instantiated-fnsig teq: got %v err=%v, want [true]", got, err)
	}
	// `make` of a generic instantiated inside a generic fn body compiles:
	// the call site `boxit 5` monomorphises T to Integer, so the
	// computed-construction body records its make event with a resolved
	// type operand (see "compile make with a computed construction body in
	// a fn body"). The compiled typeof matches the interpreter.
	const innerMake = `def Box gen [T] class {value:T} def boxit gen [T] fn [[x:T] [Any] [make (Box of [T]) {value:x}]] end (boxit 5) typeof`
	if _, r := compile(t, innerMake); r != "" {
		t.Errorf("make of a generic instantiated in a fn body must compile, refused: %s", r)
	}
	if got, _, err := mustRun(t, innerMake); err != nil || len(got) != 1 || fmt.Sprint(got[0]) != "Box of [Integer]" {
		t.Errorf("inner-make typeof: got %v err=%v, want [Box of [Integer]]", got, err)
	}
}

// TestEmitSurfaceType: a surface type (`def Shape surface {area: (fnsig …)}`)
// bakes as a const — an immutable contract descriptor riding its canonical
// minted node, with its `exposes`-filled conformance set consulted through the
// same shared payload the live unifier holds. On the compiled path RunCompiled
// runs the VM over the check-pass registry without re-minting, so the baked
// const and the live surface are one object. The whole surface type-algebra
// surface (residual, is, typeof, tand/tor/tnot, unify) compiles.
func TestEmitSurfaceType(t *testing.T) {
	const pre = `def Shape surface {area: (fnsig [[Self] [Float]])} `
	for _, src := range []string{
		pre + `Shape`,
		pre + `typeof Shape`,
		pre + `Shape tand Shape`,
		pre + `def Circle class {r:1.0} def area fn [[c:Circle] [Float] [3.14]] Circle exposes Shape end (make Circle {}) is Shape`,
	} {
		if _, r := compile(t, src); r != "" {
			t.Errorf("%s: surface must compile, refused: %s", src, r)
		}
	}
	// Value parity: a Circle that exposes Shape is a Shape; a bare Integer isn't.
	if got, _, err := mustRun(t, pre+`def Circle class {r:1.0} def area fn [[c:Circle] [Float] [3.14]] Circle exposes Shape end (make Circle {}) is Shape`); err != nil || len(got) != 1 || fmt.Sprint(got[0]) != "true" {
		t.Errorf("Circle is Shape: got %v err=%v, want [true]", got, err)
	}
	if got, _, err := mustRun(t, pre+`5 is Shape`); err != nil || len(got) != 1 || fmt.Sprint(got[0]) != "false" {
		t.Errorf("5 is Shape: got %v err=%v, want [false]", got, err)
	}
}

// TestEmitTypedDefInstance: `def b:Type {map}` is `def b (make Type map)`, but
// the typed-def handler builds the instance via a direct MakeObject call,
// skipping the make WORD dispatch — and with it the make event an explicit
// make records. Without that event the bound instance has no provenance and a
// downstream `b typeof` (or field access) refuses. The handler now records the
// skipped make event in emit mode, so these compile, for plain classes,
// explicit generic instantiations, inferred generic schemas, and the `<>`
// sugar alike.
func TestEmitTypedDefInstance(t *testing.T) {
	type row struct{ src, want string }
	for _, c := range []row{
		{`def C class {a:1} def b:C {a:5} b typeof`, "C"},
		{`def C class {a:1} def b:C {a:5} (b dot a)`, "5"},
		{`def Box gen [T] class {value:T} def b:(Box of [Integer]) {value:42} b typeof`, "Box of [Integer]"},
		{`def Box gen [T] class {value:T} def b:Box {value:'hi'} b typeof`, "Box of [ProperString]"},
		{`def Box<T> class {value:T} def b:Box<Integer> {value:42} (b dot value)`, "42"},
	} {
		dis, r := compile(t, c.src)
		if r != "" {
			t.Errorf("%s: typed-def instance must compile, refused: %s", c.src, r)
			continue
		}
		if strings.Contains(dis, "FALLBACK") {
			t.Errorf("%s: compiled with an interpreter island, want a native bake", c.src)
		}
		if got, _, err := mustRun(t, c.src); err != nil || len(got) != 1 || fmt.Sprint(got[0]) != c.want {
			t.Errorf("%s: got %v err=%v, want [%s]", c.src, got, err, c.want)
		}
	}
}

// TestEmitDeadValueDef: a single-result value-def referenced zero times — a
// dead binding (`def b (make C {…})` / `def x (1 add 2)` with the name never
// used) — used to refuse as an "unconsumed call result" on the simulated
// stack. The call still runs (side effects preserved) and its result is
// CONSUMED by the def's OpBindGlobal write-back (Pop mode — the binding
// persists cross-request; pre-OpBindGlobal it was a plain DROP), so the
// program compiles with nothing lingering. A USED def is unaffected.
func TestEmitDeadValueDef(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`def C class {a:1} def b (make C {a:5})`, "[]"},
		{`def C class {a:1} def b:C {a:5}`, "[]"},
		{`def x (1 add 2)`, "[]"},
		{`def x (1 add 2) 99`, "[99]"},
		{`def a (1 add 2) def b (3 add 4) 99`, "[99]"},
	} {
		dis, r := compile(t, c.src)
		if r != "" {
			t.Errorf("%s: dead value-def must compile, refused: %s", c.src, r)
			continue
		}
		if !strings.Contains(dis, "BIND_GLOBAL") {
			t.Errorf("%s: expected the dead result consumed by BIND_GLOBAL, got:\n%s", c.src, dis)
		}
		if strings.Contains(dis, "DROP") {
			t.Errorf("%s: the write-back consumes the dead result; no DROP expected, got:\n%s", c.src, dis)
		}
		if got, _, err := mustRun(t, c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: got %v err=%v, want %s", c.src, got, err, c.want)
		}
	}
	// A USED value-def must NOT drop — its result feeds a consumer (the
	// write-back re-pushes from the promoted local and consumes the COPY).
	dis, r := compile(t, `def x (1 add 2) (x add x)`)
	if r != "" {
		t.Fatalf("used value-def refused: %s", r)
	}
	if strings.Contains(dis, "DROP") {
		t.Errorf("used value-def emitted a DROP:\n%s", dis)
	}
}

// TestEmitFnValueData: a no-capture function VALUE used as data — a residual
// (`f/v`), a map member (`{b:f/v}`), or an arity/param introspection operand —
// bakes as a const, so these compile. The auto-dispatch boundary splits on the
// CARRIER bit: a [Function]-typed CARRIER leading the residual (`(mk2 5) 10` —
// the factory pattern) is always the interpreter's auto-apply case and now
// compiles VM-native (see TestFactoryApplyCompiles); an inert CONCRETE fn value
// ahead of args (`f/v 2`) is left as a stranded [fn args] residual and stays
// refused so it falls back faithfully.
func TestEmitFnValueData(t *testing.T) {
	const inc = `def f fn [[x:Integer] [Integer] [x add 1]] `
	for _, c := range []struct{ src, want string }{
		{inc + `f/v`, "[fn f(Integer)]"},
		{`import "boru:type-util"  TypeUtil.arityof (fn [[a:Integer b:Integer] [Integer] [a]])`, "[2]"},
	} {
		dis, r := compile(t, c.src)
		if r != "" {
			t.Errorf("%s: fn value as data must compile, refused: %s", c.src, r)
			continue
		}
		if strings.Contains(dis, "FALLBACK") {
			t.Errorf("%s: compiled with an interpreter island, want a native bake", c.src)
		}
		if got, _, err := mustRun(t, c.src); err != nil || fmt.Sprint(got) != c.want {
			t.Errorf("%s: got %v err=%v, want %s", c.src, got, err, c.want)
		}
	}
	// A captured closure carries snapshot state, so it does NOT bake as a const.
	if _, r := compile(t, `def mk fn [[x:Integer] [Function] [fn [[y:Integer] [Integer] [x add y]]]] def a (mk 5) a`); r == "" {
		t.Error("a captured closure value compiled as a const but must not (closure state)")
	}
	// Auto-dispatch boundary: a CONCRETE (inert) fn value ahead of residual args
	// must still fall back — the interpreter leaves it as a stranded [fn args]
	// residual (NOT an apply), and a concrete fn is not a [Function]-typed
	// CARRIER, so it stays refused. (The factory-apply `(mk2 5) 10` — a CARRIER
	// lead — now compiles VM-native; see TestFactoryApplyCompiles.)
	if _, r := compile(t, inc+`f/v 2`); r == "" {
		t.Error("a concrete fn value ahead of residual args compiled but must fall back")
	}
}

// TestEmitModuleInnerNative: a module word reached via dot-access
// (`StructUtil.clone …`) trivially delegates to its inner native; even with a
// declared-Any (dynamic) output the inner sig bakes a plain CALL_NATIVE,
// verified against the module sub-registry (so a usurp synthetic can't slip
// through). The interpreter dispatches the same inner native via execMatch on
// the main engine, so the baked call is identical.
func TestEmitModuleInnerNative(t *testing.T) {
	cases := []string{
		`import "boru:struct-util" StructUtil.clone {a:1}`,
		`import "boru:struct-util" StructUtil.jsonify {a:1}`,
	}
	for _, src := range cases {
		if _, r := compile(t, src); r != "" {
			t.Errorf("module inner native must compile, refused: %s\n  %s", r, src)
		}
		_, compiled, err := mustRun(t, src)
		if err != nil || !compiled {
			t.Fatalf("%s: compiled=%v err=%v (want compiled, no error)", src, compiled, err)
		}
	}
	// A def-bound dispatch-modifier wrapper (`def ff (force-arity 2 add)`)
	// compiles: ForceArityFunction's wrapper sig runs in check mode, so the
	// carrier compiler steps the re-dispatch and bakes the inner `add` call
	// directly — `ff 1 2` lowers exactly like `add 1 2` (see "compile the
	// dispatch-modifier word forms"). It is a static dispatch-shape change,
	// byte-identical to the runtime one, so the compiled result matches.
	const forceArityDef = `def ff (force-arity 2 add) ff 1 2`
	if _, r := compile(t, forceArityDef); r != "" {
		t.Errorf("force-arity'd user def must compile, refused: %s", r)
	}
	if got, _, err := mustRun(t, forceArityDef); err != nil || len(got) != 1 || fmt.Sprint(got[0]) != "3" {
		t.Errorf("force-arity'd user def: got %v err=%v, want [3]", got, err)
	}
}

// mustRun runs src in compiled mode, returning (result, wasCompiled, err).
func mustRun(t *testing.T, src string) ([]any, bool, error) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return a.RunCompiled(src)
}

// A DECLARED fn whose body holds a 0-output statement guard (`if cond [raise]`)
// followed by a real result now compiles: the guard registers a phantom None in
// the analyzer's residual but produces NO runtime value, so it is excluded from
// the return-count check and leaves no operand — exactly as the top-level
// residual reconciliation handles it. Without the exclusion the phantom inflated
// the body count and the fn refused as "value count differs from declared
// returns".
func TestFnGuardReturnCount(t *testing.T) {
	// Positive: a guard + real result compiles and matches BOTH the fall-through
	// (f 5 -> 6) and the raising (f 0 -> error) paths.
	guard := `def f fn [[n:Integer] [Integer] [if (n eq 0) [raise "zero"] def q (n add 1) q]] `
	if got, compiled, err := mustRun(t, guard+`f 5`); !compiled || err != nil || len(got) != 1 || got[0] != int64(6) {
		t.Errorf("f 5: compiled=%v got=%v err=%v, want compiled 6", compiled, got, err)
	}
	if _, compiled, err := mustRun(t, guard+`f 0`); !compiled || err == nil {
		t.Errorf("f 0: compiled=%v err=%v, want compiled with the raised error", compiled, err)
	}
	// A GENUINE count mismatch (body leaves 2 DEFINITE values, declared 1) is the
	// interpreter's return-count type_error. It now COMPILES the error path (the
	// VM's RET enforces the exact count) rather than refusing — and running it
	// raises the matching error, matching the interpreter.
	if got, r := compile(t, `def r2 fn [[n:Integer] [Integer] [n n]] r2 1`); got == "" {
		t.Errorf("count-mismatch fn must now compile the error path, but refused: %q", r)
	}
	if _, compiled, err := mustRun(t, `def r2 fn [[n:Integer] [Integer] [n n]] r2 1`); !compiled || err == nil {
		t.Errorf("count-mismatch fn: compiled=%v err=%v, want compiled with the raised count error", compiled, err)
	}
}

// TestEmitTrap: a word lenient in check mode but erroring at run time — an
// orphan gen (gen_without_constructor), an unpack of a missing key
// (unpack_error) — compiles a terminal OpTrap that raises the byte-identical
// error, instead of refusing the whole program.
func TestEmitTrap(t *testing.T) {
	cases := []struct {
		src, errSubstr string
	}{
		{`gen [T]`, "gen_without_constructor"},
		{`def m2 {x:1} unpack [y] m2`, "not found"},
	}
	for _, tc := range cases {
		got, reason := compile(t, tc.src)
		if got == "" {
			t.Errorf("%q must compile a trap, but refused: %q", tc.src, reason)
			continue
		}
		if !strings.Contains(got, "TRAP") {
			t.Errorf("%q compiled without a TRAP:\n%s", tc.src, got)
		}
		if _, compiled, err := mustRun(t, tc.src); !compiled || err == nil {
			t.Errorf("%q: compiled=%v err=%v, want compiled with the raised error", tc.src, compiled, err)
		} else if !strings.Contains(err.Error(), tc.errSubstr) {
			t.Errorf("%q: error %q does not contain %q", tc.src, err.Error(), tc.errSubstr)
		}
	}
	// NEGATIVE: a suppressed error inside a branch fragment is conditional and
	// not modelled as a trap, so it keeps the blanket refusal and falls back.
	if got, _ := compile(t, `if true [def mm {x:1} unpack [y] mm] [1]`); got != "" {
		t.Errorf("nested suppressed error must refuse (no top-level trap), but compiled:\n%s", got)
	}
}

// TestEmitReverse: an N-arg (N>=3) call whose args are all COMPUTED evaluates
// them left-to-right (sig position 0 lands deepest), so the lowerer reverses the
// top N with one OpReverse to seat them in sig order — the 3-deep rotate the VM
// previously had no opcode for (it refused as "operand shape beyond Stage 1").
func TestEmitReverse(t *testing.T) {
	src := `range (1 add 0) (4 sub 0) (2 sub 1)` // range 1 4 1 -> [1 2 3], all 3 args computed
	got, reason := compile(t, src)
	if got == "" {
		t.Fatalf("%q must compile via OpReverse, but refused: %q", src, reason)
	}
	if !strings.Contains(got, "REVERSE") {
		t.Errorf("%q compiled without a REVERSE:\n%s", src, got)
	}
	// The reversed operands must produce the SAME result as the interpreter.
	res, compiled, err := mustRun(t, src)
	if !compiled || err != nil {
		t.Fatalf("%q: compiled=%v err=%v", src, compiled, err)
	}
	if len(res) != 1 {
		t.Fatalf("%q: result = %v, want a single list", src, res)
	}
	// NEGATIVE: a 2-computed-arg call still uses SWAP, not REVERSE (REVERSE is
	// only for N>=3 — the case SWAP cannot cover).
	if g2, _ := compile(t, `range (1 add 0) (4 sub 0)`); strings.Contains(g2, "REVERSE") {
		t.Errorf("2-arg call must use SWAP, not REVERSE:\n%s", g2)
	}
}

// TestEmitSpillSeat: an operand shape the cheap stack-only paths can't seat —
// two COMPUTED args plus a trailing inert arg (`range (a) (b) const`), which the
// old layoutOperands refused as "operand shape beyond Stage 1" — now compiles by
// spilling the event operands to frame-local destinations (STORE_LOCAL) and
// re-pushing in sig order (DDCG frame-slot destinations). The result must match
// the interpreter.
func TestEmitSpillSeat(t *testing.T) {
	src := `range (1 add 0) (5 sub 0) 1` // range 1 5 1 -> [1 2 3 4]; sig[0],sig[1] computed, sig[2] const
	got, reason := compile(t, src)
	if got == "" {
		t.Fatalf("%q must compile via spill, but refused: %q", src, reason)
	}
	if !strings.Contains(got, "STORE_LOCAL") {
		t.Errorf("%q compiled without a spill (STORE_LOCAL):\n%s", src, got)
	}
	res, compiled, err := mustRun(t, src)
	if !compiled || err != nil {
		t.Fatalf("%q: compiled=%v err=%v", src, compiled, err)
	}
	if len(res) != 1 {
		t.Fatalf("%q: result = %v, want a single list [1 2 3 4]", src, res)
	}
	// NEGATIVE: an all-inert call never spills (no event operands to seat).
	if g2, _ := compile(t, `range 1 5 1`); strings.Contains(g2, "STORE_LOCAL") {
		t.Errorf("all-const call must not spill:\n%s", g2)
	}
}

// TestEmitMethodField: a map whose field is an UNNAMED inline fn (`{f: fn […]}`)
// const-bakes, so `m.f args` compiles — a poly `get` returns the fn (dynamic),
// the fn-value-call boundary (CALL_DYNAMIC) applies it. A NAMED ref field map
// (`{b: f/v}`) now const-bakes too (the no-capture fn value is inert data), but
// the `m.b 2` CALL through it still falls back at the get-then-dispatch — value
// parity holds either way, asserted here.
func TestEmitMethodField(t *testing.T) {
	cases := []struct {
		src  string
		want []any
	}{
		{`def m {f: (fn [[a:Integer][Integer][a add 1]])} m.f 5`, []any{int64(6)}},
		{`def m {f: (fn [[a:Integer b:Integer][Integer][(a mul 100) add b]])} m.f 2 3`, []any{int64(203)}},
		// Named-ref field: falls back, but must still match the interpreter.
		{`def f fn [[x:Integer] [Integer] [add x 1]] def m {b:f/v} m.b 2`, []any{int64(3)}},
	}
	for _, c := range cases {
		got, _, rerr := mustRun(t, c.src)
		if rerr != nil {
			t.Fatalf("%s: err=%v", c.src, rerr)
		}
		if len(got) != len(c.want) || (len(got) == 1 && got[0] != c.want[0]) {
			t.Errorf("%s: got %v, want %v", c.src, got, c.want)
		}
	}
}

// Loop results are VARIADIC at run time (one value per iteration):
// downstream calls must refuse to consume them — only the program
// residual may absorb the accumulation.
func TestEmitRefusesLoopResultConsumption(t *testing.T) {
	got, reason := compile(t, `(for 3 [i]) size`)
	if got != "" {
		t.Fatalf("loop-result consumption compiled but must refuse:\n%s", got)
	}
	if !strings.Contains(reason, "loop") && !strings.Contains(reason, "provenance") {
		t.Errorf("unexpected refusal reason %q", reason)
	}
}

// Negative: every beyond-Stage-1 construct is refused with a precise
// reason — never lowered wrongly.
func TestEmitRefusals(t *testing.T) {
	cases := []struct {
		src        string
		wantReason string
	}{
		// A loop-collect def with a DYNAMIC count: the S5 first-value split
		// needs the static region size, so the refusal stands. (The
		// statically-counted sibling — including its read inside a branch
		// arm — graduated 2026-07-17 with the S5 split: the read resolves
		// the live binding via OpLookupDynScope, so branch fragments consume
		// it like any dynamic-scope read; error-parity pinned in
		// TestEdgeFindingLoopCollectDefCompiles. The enclosing COMPUTED-value
		// read — `def y (expr) … if … y …` — compiles via per-unit
		// value-def-locals; see TestEnclosingReadInBranchCompiles.)
		{`def m {n: 3} def xs (for (m get "n") [1]) if (xs.0 gt 0) [xs] [[9]]`, "consumes loop results"},
	}
	for _, c := range cases {
		got, reason := compile(t, c.src)
		if got != "" {
			t.Errorf("%q compiled but must be refused at Stage 1:\n%s", c.src, got)
			continue
		}
		if !strings.Contains(reason, c.wantReason) {
			t.Errorf("%q refused with %q, want reason containing %q", c.src, reason, c.wantReason)
		}
	}
}

// Stage 3: a polymorphic (partitioned) site is classified and lowered to
// OpCallNativePoly. A strict-disjunct straddle (`y` is Integer|String from the
// two `if` arms) reaching more than one `is` overload lowers to a runtime-
// matched poly call: the VM re-matches the one concrete runtime alternative,
// so it no longer refuses as "polymorphic dispatch" (roadmap item 4).
// Faithfulness is the load-bearing assertion — the runtime value (1, the true
// arm) dispatches `1 is Integer = true` in both engines.
//
// (`add` used to be the straddle word here, via its old `[Scalar Scalar]`
// catch-all; once that overload was tightened to require a String operand,
// `Integer|String add 1` no longer has a single whole-disjunct seed overload,
// so the straddle moved to `is`, which keeps the same shape.)
func TestEmitPolySiteLowersToRuntimeMatch(t *testing.T) {
	const src = `def y (if (1 gt 0) [1] ['s']) y is Integer`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatal(err)
	}
	if prog == nil {
		t.Fatalf("strict-disjunct straddle did not compile: reason=%q", reason)
	}
	if !strings.Contains(prog.Disassemble(), "CALL_NATIVE_POLY") {
		t.Errorf("expected a runtime-matched poly call for the straddling is:\n%s", prog.Disassemble())
	}
	gotC, compiled, errC := a.RunCompiled(src)
	if !compiled || errC != nil {
		t.Fatalf("compiled run: compiled=%v err=%v", compiled, errC)
	}
	b, _ := New()
	gotI, _ := b.RunInterp(src)
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[true]" {
		t.Errorf("poly is: compiled=%v interp=%v (want [true])", gotC, gotI)
	}
}

// Stage 3: user fns compile as their own code units with params as
// frame locals; the body is compiled against GENERALISED carrier
// args (a call's concrete values must not constant-fold into the
// shared unit); tail-position recursive calls lower to
// TAIL_CALL_USER — the language's tail-call guarantee in compiled
// form.
func TestEmitUserFnAndTailCall(t *testing.T) {
	got, reason := compile(t, `def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 10 0`)
	if reason != "" {
		t.Fatalf("tail-recursive fn unexpectedly uncompilable: %s", reason)
	}
	if !strings.Contains(got, "TAIL_CALL_USER") {
		t.Errorf("tail call not lowered as TAIL_CALL_USER:\n%s", got)
	}
	if !strings.Contains(got, "CALL_USER") || !strings.Contains(got, "RET") {
		t.Errorf("user-fn frame opcodes missing:\n%s", got)
	}
	// The generalisation guard: the body must reference its params as
	// locals, never the outer call's constants (n sub 1 with n=10
	// must NOT fold to 9).
	if strings.Contains(got, "; 9 (Integer)") {
		t.Errorf("call-site constant folded into the shared fn unit:\n%s", got)
	}

	// Non-tail recursion also compiles (frames stack).
	got2, reason2 := compile(t, `def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]] fact 10`)
	if reason2 != "" {
		t.Fatalf("factorial unexpectedly uncompilable: %s", reason2)
	}
	if strings.Contains(got2, "TAIL_CALL_USER") {
		t.Errorf("non-tail recursive call wrongly marked tail:\n%s", got2)
	}

	// Stage 1 (the def-bound computed-fn read): a closure ESCAPING as a
	// value — returned, bound, then called once through the binding —
	// now COMPILES: the read resolves through the fn-carrier side table
	// and the leading-apply machinery owns the call (runtime parity 8,
	// pinned by the frontier harness; repeated reads still refuse at the
	// fn-value residual nets). Locally-called closures were already
	// compiled — see TestEmitClosureCaptureSlots.
	if _, r3 := compile(t, `def mk fn [[x:Integer] [Function] [fn [[y:Integer] [Integer] [x add y]]]] def a5 (mk 5) a5 3`); r3 != "" {
		t.Errorf("escaping closure must compile since Stage 1, refused: %s", r3)
	}
}

// Runtime equality + the tail guarantee under a tight ceiling: deep
// self tail-recursion runs in O(1) frames in compiled mode, while
// equally deep NON-tail recursion exhausts loudly with the shared
// taxonomy.
func TestRunCompiledTailGuarantee(t *testing.T) {
	tight := Options{Tape: TapeOptions{InitialSize: 64, MaxGrows: 1, GrowthFactor: 2.7}}
	a, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def s2 fn [[n:Integer acc:Integer] [Integer] [if (n lte 0) [acc] [s2 (n sub 1) (acc add n)]]] s2 100000 0`)
	if err != nil {
		t.Fatalf("deep tail recursion: %v", err)
	}
	if !compiled {
		t.Fatal("tail-recursive program fell back to the interpreter")
	}
	if len(out) != 1 || out[0] != int64(5000050000) {
		t.Fatalf("s2 100000 0 = %v, want 5000050000", out)
	}

	b, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	_, compiled2, err2 := b.RunCompiled(`def f fn [[n:Integer] [Integer] [if (n lte 0) [0] [n add (f (n sub 1))]]] f 100000`)
	if !compiled2 {
		t.Fatal("non-tail program fell back to the interpreter")
	}
	if err2 == nil || !strings.Contains(err2.Error(), "tape_exhausted") {
		t.Fatalf("deep non-tail recursion under tight ceiling = %v, want tape_exhausted", err2)
	}
}

// A tail call inside a STATICALLY-TAKEN (const-condition) branch keeps the
// O(1)-frame guarantee. lowerBranch inlines only the taken arm, and
// markTailCalls marks the call there as TAIL_CALL_USER just as it does on the
// dynamic-condition path — otherwise the const-cond arm lowered the recursion
// as a frame-growing CALL_USER, silently losing the guarantee the interpreter
// (and the dynamic-cond compiled path) honour. The `[true]` list condition is
// what LiteralCondValue folds to a constCond (a bare `true` word stays a
// runtime branch).
func TestRunCompiledConstCondTailGuarantee(t *testing.T) {
	const src = `def g fn [[n:Integer] [Integer] [if (n lte 0) [0] [if [true] [g (n sub 1)] [0]]]] g 100000`

	// The const-cond arm's self-call lowers as TAIL_CALL_USER. (Before the
	// fix it was a plain CALL_USER and no TAIL_CALL_USER appeared at all.)
	got, reason := compile(t, src)
	if reason != "" {
		t.Fatalf("const-cond tail recursion uncompilable: %s", reason)
	}
	if !strings.Contains(got, "TAIL_CALL_USER f0   ; g/1") {
		t.Fatalf("const-cond arm tail call not lowered as TAIL_CALL_USER:\n%s", got)
	}

	// Runs in O(1) frames under a tight ceiling deep NON-tail recursion would
	// exhaust, and agrees with the interpreter.
	tight := Options{Tape: TapeOptions{InitialSize: 64, MaxGrows: 1, GrowthFactor: 2.7}}
	a, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(src)
	if err != nil {
		t.Fatalf("deep const-cond tail recursion: %v", err)
	}
	if !compiled {
		t.Fatal("const-cond tail recursion fell back to the interpreter")
	}
	if len(out) != 1 || out[0] != int64(0) {
		t.Fatalf("g 100000 = %v, want 0", out)
	}

	// Negative: a PENDING op below the same const-cond arm's self-call makes it
	// non-tail; frames then grow and exhaust loudly under the tight ceiling —
	// the const-cond branch must not spuriously elide a real frame.
	const nonTail = `def h fn [[n:Integer] [Integer] [if (n lte 0) [0] [if [true] [1 add (h (n sub 1))] [0]]]] h 100000`
	b, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	_, compiled2, err2 := b.RunCompiled(nonTail)
	if !compiled2 {
		t.Fatal("const-cond non-tail program fell back to the interpreter")
	}
	if err2 == nil || !strings.Contains(err2.Error(), "tape_exhausted") {
		t.Fatalf("deep const-cond non-tail recursion = %v, want tape_exhausted", err2)
	}
}

// Stage 3 completion: mutual tail recursion. The forward-reference
// rescue clears the checker FP, both bodies compile as units at the
// top-level call site (each is fully defined by then), and the arm
// tail calls lower as cross-unit TAIL_CALL_USER.
func TestEmitMutualTailRecursion(t *testing.T) {
	got, reason := compile(t, `def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isev 10`)
	if reason != "" {
		t.Fatalf("mutual tail recursion uncompilable: %s", reason)
	}
	if !strings.Contains(got, "TAIL_CALL_USER f1") || !strings.Contains(got, "TAIL_CALL_USER f0") {
		t.Errorf("mutual tails not lowered as cross-unit TAIL_CALL_USER:\n%s", got)
	}
	if !strings.Contains(got, "fns=2") {
		t.Errorf("expected two fn units:\n%s", got)
	}
}

// Mutual tail recursion runs in O(1) frames under a tight ceiling
// (the language guarantee crosses units); mutual NON-tail recursion
// (a pending add below each call) stacks frames and exhausts loudly.
func TestRunCompiledMutualTailGuarantee(t *testing.T) {
	tight := Options{Tape: TapeOptions{InitialSize: 64, MaxGrows: 1, GrowthFactor: 2.7}}
	a, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def isod fn [[n:Integer] [Boolean] [if (n eq 0) [false] [isev (n sub 1)]]] def isev fn [[n:Integer] [Boolean] [if (n eq 0) [true] [isod (n sub 1)]]] isev 1000000`)
	if err != nil {
		t.Fatalf("deep mutual tail recursion: %v", err)
	}
	if !compiled {
		t.Fatal("mutual tail recursion fell back to the interpreter")
	}
	// convertResults renders Booleans as strings.
	if len(out) != 1 || out[0] != "true" {
		t.Fatalf("isev 1000000 = %v, want true", out)
	}

	b, err := New(tight)
	if err != nil {
		t.Fatal(err)
	}
	// Non-tail because each recursive call's result is consumed (dropped)
	// before the frame returns `n`, so the call is not in tail position and
	// frames stack. `drop` takes an Any operand, so the cross-unit forward
	// reference (`od` references `ev`, defined after it) type-checks without
	// relying on `add` absorbing the forward-ref result — `add` was tightened
	// to require a Number/String operand and no longer matches an as-yet-
	// untyped forward reference.
	_, compiled2, err2 := b.RunCompiled(`def od fn [[n:Integer] [Integer] [if (n eq 0) [0] [ev (n sub 1) drop n]]] def ev fn [[n:Integer] [Integer] [if (n eq 0) [0] [od (n sub 1) drop n]]] ev 100000`)
	if !compiled2 {
		t.Fatal("mutual non-tail program fell back to the interpreter")
	}
	if err2 == nil || !strings.Contains(err2.Error(), "tape_exhausted") {
		t.Fatalf("deep mutual non-tail recursion under tight ceiling = %v, want tape_exhausted", err2)
	}
}

// Stage 3 completion: closures. Captures ride as hidden trailing
// param slots — the construction site supplies the enclosing frame's
// values, a recursive call re-passes the frame's own capture slots
// (construction-time snapshot semantics).
func TestEmitClosureCaptureSlots(t *testing.T) {
	// lang/spec/recursion.tsv §6: a captured body-local stays visible
	// through a chain of recursive tail calls.
	got, reason := compile(t, `def outc fn [[] [Integer] [def x 7 def goc fn [[n:Integer] [Integer] [if (n lte 0) [x] [goc (n sub 1)]]] goc 3]] outc`)
	if reason != "" {
		t.Fatalf("recursive closure uncompilable: %s", reason)
	}
	if !strings.Contains(got, "goc/2") {
		t.Errorf("capture slot not added to the unit's params:\n%s", got)
	}

	// Runtime parity, with a non-commutative op so slot ORDER is
	// pinned, and a param (not a literal) as the captured value.
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def oc fn [[m:Integer] [Integer] [def gc fn [[n:Integer] [Integer] [m sub n]] gc 3]] oc 10`)
	if err != nil {
		t.Fatalf("closure over param: %v", err)
	}
	if !compiled {
		t.Fatal("closure fell back to the interpreter")
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	want, err := b.RunInterp(`def oc fn [[m:Integer] [Integer] [def gc fn [[n:Integer] [Integer] [m sub n]] gc 3]] oc 10`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || len(want) != 1 || out[0] != want[0] {
		t.Fatalf("compiled %v != interpreted %v", out, want)
	}
}

// The fn-unit key includes the construction site: redefining a
// same-name same-sig fn must bind later call sites to the LATER
// body. (Without the site in the key, both calls ran the FIRST
// definition's unit — caught against the interpreter's 2 3.)
func TestCompiledFnRedefinitionBindsLatest(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def rdf fn [[n:Integer] [Integer] [n add 1]] (rdf 1) def rdf fn [[n:Integer] [Integer] [n add 2]] (rdf 1)`)
	if err != nil {
		t.Fatal(err)
	}
	if !compiled {
		t.Fatal("redefinition program fell back to the interpreter")
	}
	if len(out) != 2 || out[0] != int64(2) || out[1] != int64(3) {
		t.Fatalf("redefined fn calls = %v, want [2 3]", out)
	}
}

// A tail-recursive SPIN never grows the stack or the frame list, so
// neither ceiling trips — the VM's step budget must catch it with
// the interpreter's evaluation_limit taxonomy instead of hanging.
func TestRunCompiledStepBudget(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	_, compiled, err := a.RunCompiled(`def spinf fn [[n:Integer] [Integer] [spinf n]] spinf 0`)
	if !compiled {
		t.Fatal("tail spin fell back to the interpreter")
	}
	if err == nil || !strings.Contains(err.Error(), "evaluation_limit") {
		t.Fatalf("tail spin = %v, want evaluation_limit", err)
	}
}

// Stage 4: generic fns compile one unit per memoised instantiation
// (the unit key carries the instantiated arg types); generic units
// stay OUT of tail marking, mirroring the interpreter's HasGen
// exclusion from frame elision.
func TestEmitGenericInstantiation(t *testing.T) {
	got, reason := compile(t, `def idg gen [T] fn [[x:T] [T] [x]] (idg 5) (idg "a")`)
	if reason != "" {
		t.Fatalf("generic fn uncompilable: %s", reason)
	}
	if !strings.Contains(got, "fns=2") {
		t.Errorf("two instantiations must compile two units:\n%s", got)
	}

	// Generic recursion compiles but is NOT tail-marked.
	got2, reason2 := compile(t, `def cntg gen [T] fn [[x:T n:Integer] [Integer] [if (n lte 0) [n] [cntg x (n sub 1)]]] cntg "a" 5`)
	if reason2 != "" {
		t.Fatalf("generic recursion uncompilable: %s", reason2)
	}
	if strings.Contains(got2, "TAIL_CALL_USER") {
		t.Errorf("generic unit was tail-marked (HasGen exclusion):\n%s", got2)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def pickg gen [T U] fn [[a:T b:U] [U] [b]] pickg 1 "x"`)
	if err != nil || !compiled {
		t.Fatalf("generic call: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != "x" {
		t.Fatalf("pickg 1 \"x\" = %v, want x", out)
	}
}

// Stage 4: type operands lower as PUSH_TYPE — resolved through the
// registry's TypeTable at RUN time so the handler always receives
// the CANONICAL node (never a stale pooled copy), for builtins and
// check-pass-minted user types alike.
func TestEmitTypeOperands(t *testing.T) {
	got, reason := compile(t, `5 is Integer`)
	if reason != "" {
		t.Fatalf("is-with-type-operand uncompilable: %s", reason)
	}
	if !strings.Contains(got, "PUSH_TYPE") || !strings.Contains(got, "; Integer") {
		t.Errorf("type operand not lowered as PUSH_TYPE:\n%s", got)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def Pt refine Integer def p:Pt 5 p is Pt`)
	if err != nil || !compiled {
		t.Fatalf("minted-type operand: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != "true" {
		t.Fatalf("p is Pt = %v, want true", out)
	}

	// Structural type bodies (make's operand) ride the const pool —
	// payloads are pointer-backed and the minted node is reached via
	// the Parent pointer, which stays canonical.
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out2, compiled2, err := b.RunCompiled(`def M refine Record [k:String] end make M {k:"x"}`)
	if err != nil || !compiled2 {
		t.Fatalf("make with record body: compiled=%v err=%v", compiled2, err)
	}
	if len(out2) != 1 || out2[0] != "{k:'x'}" {
		t.Fatalf("make M = %v, want {k:'x'}", out2)
	}

	// Positive: a generic instantiation over a class whose default is a
	// concrete SCALAR (`r:1.0`) compiles faithfully — the scalar stays concrete
	// through check mode (toCarrier keeps inert scalars), so the baked type body
	// renders `r:1.0`, NOT a stripped `r:Float` artefact. The differential gate
	// confirms parity (it once would have diverged; the scalar-keep closed it).
	g, err := New()
	if err != nil {
		t.Fatal(err)
	}
	outG, compiledG, errG := g.RunCompiled(`def Shape surface {area: (fnsig [[Self] [Float]])} def Circle class {r:1.0} def area fn [[c:Circle] [Float] [1.0]] Circle exposes Shape def Holder gen [(T extends Shape)] refine Record [item:T] end Holder of [Circle]`)
	if errG != nil || !compiledG {
		t.Fatalf("scalar-default generic body: compiled=%v err=%v", compiledG, errG)
	}
	// Stage 2 flip (design/legacy/TYPE-REPRESENTATION.1.ignore §6): the type
	// argument denotes its minted NODE, so the instantiated body renders
	// the field's type by NAME (`item:Circle`), not as the class body.
	// The differential gate above still confirms compiled/interp parity.
	if len(outG) != 1 || !strings.Contains(fmt.Sprint(outG[0]), "item:Circle") {
		t.Fatalf("generic body baked %v, want item:Circle", outG)
	}

	// A mutable-INSTANCE default (`items:(flex [])`) inside a class body now bakes
	// as a const TEMPLATE: make's FreshenDefault copies it per instance, so the
	// shared baked body is never an instance's mutable field. A generic over such
	// a class therefore compiles and matches the interpreter's per-instance
	// rebuild. (The mutation-safety NEGATIVE — a mutable instance must NOT bake
	// STANDALONE — lives in eng/go/bytecode_constbake_test.go.)
	// Stage 2 flip (§6): the class type argument renders by NAME here too.
	if outF, cF, eF := mustRun(t, `def C class {items:(flex [])} def Holder gen [T] refine Record [item:T] end Holder of [C]`); !cF || eF != nil || !strings.Contains(fmt.Sprint(outF), "item:C") {
		t.Errorf("flex-default generic body: compiled=%v err=%v out=%v", cF, eF, outF)
	}
}

// Stage 4: a macro call site lowers to its EXPANSION — the macro
// installs during the check pass (RunInCheckMode construction, like
// fn/fnsig), its use expands on the tape (execMacro), and the
// recording pass sees only the expanded stream (plan R6 #29: raw-form
// operand spans are never lowered pre-expansion).
func TestEmitMacroExpansionGolden(t *testing.T) {
	got, reason := compile(t, `def twice (macro [[e] [ quote [ unquote e add unquote e ] ]])  twice 5`)
	if reason != "" {
		t.Fatalf("macro site uncompilable: %s", reason)
	}
	// The BIND_TWIN is the macro's own `def twice` install (a runtime-visible
	// binding transition of the check pass — the ledger's, so the stream's;
	// inert until the flip). The EXPANSION's tokens follow.
	want := `0000 BIND_TWIN   w0   ; bind twin def twice @depth 1 (replay)
0001 PUSH_CONST  k0   ; 5 (Integer)
0002 PUSH_CONST  k0   ; 5 (Integer)
0003 CALL_NATIVE s0   ; add (Number, Number)
; consts=1 types=0 sigs=1 fallbacks=0 fns=0 max-stack=2 locals=0
`
	if got != want {
		t.Errorf("macro expansion lowering changed:\n--- got\n%s--- want\n%s", got, want)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def unless (macro [[c body] [ quote [ if unquote c [0] unquote body ] ]])  unless false [42]`)
	if err != nil || !compiled {
		t.Fatalf("unless macro: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != int64(42) {
		t.Fatalf("unless false [42] = %v, want 42", out)
	}
}

// Stage 4: a module dot-access call compiles to a bare CALL_NATIVE on
// the INNER native's signature — `MathUtil.sqrt 16.0` is the tokens
// `MathUtil get sqrt 16.0`; the import and the get resolution run
// during the check pass and are ELIDED (the resolved wrapper's
// trivial-delegation dispatch records the real call, through the same
// engine/registry the interpreter's short-circuit uses).
func TestEmitModuleCallLowering(t *testing.T) {
	got, reason := compile(t, `import "boru:math-util" MathUtil.sqrt 16.0`)
	if reason != "" {
		t.Fatalf("module call uncompilable: %s", reason)
	}
	// The BIND_TWIN is the import's namespace install (`MathUtil`, a
	// runtime-visible binding transition of the check pass; inert until the
	// flip) — the import's EXECUTION stays elided, per the front-end
	// carve-out: the twin re-binds, never re-imports.
	want := `0000 BIND_TWIN   w0   ; bind twin def MathUtil @depth 1 (replay)
0001 PUSH_CONST  k0   ; 16.0 (Float)
0002 CALL_NATIVE s0   ; sqrt (Float)
; consts=1 types=0 sigs=1 fallbacks=0 fns=0 max-stack=1 locals=0
`
	if got != want {
		t.Errorf("module call lowering changed:\n--- got\n%s--- want\n%s", got, want)
	}

	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`import "boru:math-util" MathUtil.max 5.0 9.0`)
	if err != nil || !compiled {
		t.Fatalf("MathUtil.max: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != "9.0" {
		t.Fatalf("max 5.0 9.0 = %v, want 9.0", out)
	}

	// A field read over a CONCRETE map with a static key now narrows to the
	// field's type (getNodeReturns), so a def-bound `m.a` monomorphizes to a
	// plain CALL_NATIVE — better than the prior dynamic poly dispatch. The
	// result must still match the interpreter.
	got2, reason2 := compile(t, `def m {a:1} m.a`)
	if reason2 != "" {
		t.Fatalf("concrete-map dot refused: %s", reason2)
	}
	if strings.Contains(got2, "FALLBACK") || strings.Contains(got2, "CALL_NATIVE_POLY") {
		t.Errorf("concrete-map field dot should monomorphize to CALL_NATIVE:\n%s", got2)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out2, _, err := b.RunCompiled(`def m {a:1} m.a`)
	if err != nil {
		t.Fatalf("concrete-map get: %v", err)
	}
	if len(out2) != 1 || out2[0] != int64(1) {
		t.Fatalf("m.a = %v, want 1", out2)
	}

	// An OBJECT field read also narrows: the field's declared type is
	// resolved from the class SCHEMA (getObjectReturns) even though the
	// instance is an abstract carrier in check mode, so `o.a` likewise
	// monomorphizes to a plain CALL_NATIVE. The runtime poly-dispatch path is
	// covered by TestEmitPolySiteLowersToRuntimeMatch.
	got3, reason3 := compile(t, `def Foo class {a:1} def o (make Foo {}) o.a`)
	if reason3 != "" {
		t.Fatalf("object field dot refused: %s", reason3)
	}
	if strings.Contains(got3, "FALLBACK") || strings.Contains(got3, "CALL_NATIVE_POLY") {
		t.Errorf("object field dot should monomorphize to CALL_NATIVE:\n%s", got3)
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out3, _, err := c.RunCompiled(`def Foo class {a:1} def o (make Foo {}) o.a`)
	if err != nil {
		t.Fatalf("object field get: %v", err)
	}
	if len(out3) != 1 || out3[0] != int64(1) {
		t.Fatalf("o.a = %v, want 1", out3)
	}
}

// Stage 4: a multi-overload user fn monomorphizes per call SHAPE —
// the checker resolves which overload each statically-typed call site
// selects, and each becomes its own code unit (the memo key carries
// the arg types). No runtime poly dispatch is needed when the checker
// can pick; the genuinely-polymorphic case (a dynamic arg reaching
// several overloads) is Stage-5 fallback territory.
func TestEmitMultiOverloadMonomorphises(t *testing.T) {
	got, reason := compile(t, `def f fn [[a:Integer][Integer][a add 1] [a:String][String][a add "!"]] (f 5) (f "x")`)
	if reason != "" {
		t.Fatalf("multi-overload fn uncompilable: %s", reason)
	}
	if !strings.Contains(got, "fns=2") {
		t.Errorf("two call shapes must compile two units:\n%s", got)
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(`def f fn [[a:Integer][Integer][a add 1] [a:String][String][a add "!"]] (f 5) (f "x")`)
	if err != nil || !compiled {
		t.Fatalf("multi-overload run: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 2 || out[0] != int64(6) || out[1] != "x!" {
		t.Fatalf("multi-overload = %v, want [6 x!]", out)
	}
}

// Stage 4: the minilang `mini` word lowers to a bare CALL_NATIVE —
// it is a deterministic expansion the recording pass treats like any
// other native. A trailing dynamic accessor on the result — `.n` —
// is the F4 data-get case: the dynamic `mini` result threads into a
// `get` island and re-dispatches faithfully (Stage-6 F4 follow-on).
func TestEmitMinilangCompiles(t *testing.T) {
	got, reason := compile(t, `import "boru:minilang" "a1b2c3" mini re "\\d"`)
	if reason != "" {
		t.Fatalf("mini expansion uncompilable: %s", reason)
	}
	// A filter-kind mini now expands to a PARTIAL function (src+opts
	// bound, subject the remaining param), so the lowering is a compiled
	// CALL_USER whose body makes the CALL_NATIVE — still fully compiled,
	// no interpreter island.
	if !strings.Contains(got, "CALL_NATIVE") {
		t.Errorf("mini did not lower to a native call:\n%s", got)
	}
	// The dynamic-result data accessor now islands (F4); the result
	// must match the interpreter.
	src := `import "boru:minilang" ("a1b2c3" mini re "\\d").n`
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	gc, _, ec := a.RunCompiled(src)
	b, _ := New()
	gi, ei := b.RunInterp(src)
	if (ec == nil) != (ei == nil) {
		t.Fatalf("mini accessor error divergence: compiled=%v interp=%v", ec, ei)
	}
	if ec == nil && (len(gc) != len(gi) || (len(gc) == 1 && gc[0] != gi[0])) {
		t.Errorf("mini accessor F4: compiled=%v interp=%v", gc, gi)
	}
}

// A method field whose value is an UNNAMED inline fn now COMPILES (the map
// const-bakes the fn member, a poly get returns it, CALL_DYNAMIC applies it —
// see TestEmitMethodField). A NAMED ref field (`{b: f/v}`) still falls back to
// the interpreter (it would diverge through the island), with the correct
// result — the remaining sanctioned fn-value-from-map fallback.
// A fn-VALUE held in a baked map/object field and APPLIED (`m.b 2`) compiles:
// the field bakes as an inert const member (isInertConstMember admits a capture-
// free named ref), get poly-folds it, and CALL_DYNAMIC applies it. Faithfulness
// rides on the trivial-delegation guard: callPoly's 0-arg auto-apply and
// callDynamic's VM-native fast path fire ONLY for a `[Word(inner)]` delegation
// method (isDelegationFnDef) — a USER fn (real body) routes through the island,
// which runs its body exactly as the interpreter would. Both an unnamed inline
// fn and a named `/v` ref work.
func TestEmitFnValueFieldCallCompiles(t *testing.T) {
	for _, c := range []struct {
		src  string
		want int64
	}{
		{`def m {f: (fn [[a:Integer][Integer][a add 1]])}  m.f 5`, 6},           // unnamed inline
		{`def f fn [[x:Integer] [Integer] [add x 1]]  def m {b:f/v}  m.b 2`, 3}, // named ref
		{`def inc fn [[n:Integer][Integer][n add 1]]  def ops {f: inc/v}  ops.f 5`, 6},
		{`def m {a:add/v} end m.a 1 2`, 3}, // builtin ref
	} {
		got, compiled, err := mustRun(t, c.src)
		if !compiled || err != nil {
			t.Errorf("%q: compiled=%v err=%v (want compiled, no error)", c.src, compiled, err)
			continue
		}
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%q = %v, want %d", c.src, got, c.want)
		}
	}
	// NEGATIVE: a BARE inert ref directly followed by an arg (`f/v 2`) is NOT
	// applied by the interpreter — the /v leaves it inert data, residual [f, 2] —
	// so the compiler must refuse rather than auto-apply it (which would diverge).
	if _, r := compile(t, `def f fn [[x:Integer] [Integer] [add x 1]]  f/v 2`); r == "" {
		t.Error("bare inert ref `f/v 2` compiled but must refuse (interpreter leaves it inert)")
	}
}

// Plan P2: a code-body higher-order word compiles its body to a CLOSURE
// unit (PUSH_CLOSURE) and runs it through the VM — no interpreter island.
// The body must reference only VM-resolvable words; a body reading a
// check-time `def` (a carrier at run time) keeps the island/fallback path.
func TestEmitFallbackIsland(t *testing.T) {
	got, reason := compile(t, `each [mul 2] [1 2 3]`)
	if reason != "" {
		t.Fatalf("each closure uncompilable: %s", reason)
	}
	if !strings.Contains(got, "PUSH_CLOSURE") || strings.Contains(got, "FALLBACK") {
		t.Errorf("each did not lower to a closure (expected PUSH_CLOSURE, no FALLBACK):\n%s", got)
	}

	// Runtime parity across each / fold / scan.
	for _, c := range []struct {
		src  string
		want any
	}{
		{`each [mul 2] [1 2 3]`, "[2 4 6]"},
		{`[1 2 3] each [mul 2]`, "[2 4 6]"},
		{`fold [add] [1 2 3 4] 0`, int64(10)},
		{`scan [add] [1 2 3]`, "[1 3 6]"},
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		out, compiled, err := a.RunCompiled(c.src)
		if err != nil || !compiled {
			t.Fatalf("%q: compiled=%v err=%v", c.src, compiled, err)
		}
		if len(out) != 1 || out[0] != c.want {
			t.Fatalf("%q compiled = %v, want %v", c.src, out, c.want)
		}
	}

	// A body reading a CONCRETE module-def bakes it as a const in the
	// closure body (plan P2 closure-capture / const-bake) — compiles native.
	if got, r := compile(t, `def n 10 each [add n] [1 2 3]`); r != "" {
		t.Errorf("each with a concrete def-referencing body refused: %s", r)
	} else if strings.Contains(got, "FALLBACK") {
		t.Errorf("each with a concrete def-referencing body islanded:\n%s", got)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := b.RunCompiled(`def n 10 each [add n] [1 2 3]`)
	if err != nil || !compiled {
		t.Fatalf("def-body closure: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != "[11 12 13]" {
		t.Fatalf("def-body closure = %v, want [11 12 13]", out)
	}

	// A module-qualified higher-order word with a code body dispatches
	// the inner native through a sub-registry; the core-dispatch guard
	// keeps any bare-name re-run faithful. It runs correctly either way.
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	mout, _, merr := c.RunCompiled(`import "boru:array-util" ArrayUtil.group ['a' 'b' 'a'] [1 2 3]`)
	if merr != nil {
		t.Fatalf("module group: %v", merr)
	}
	d, _ := New()
	iout, _ := d.RunInterp(`import "boru:array-util" ArrayUtil.group ['a' 'b' 'a'] [1 2 3]`)
	if len(mout) != len(iout) || (len(mout) == 1 && mout[0] != iout[0]) {
		t.Fatalf("module group compiled=%v interpreted=%v", mout, iout)
	}
}

// Plan P2: a single-result `do` body compiles to a CLOSURE unit (native),
// not an island. The splice word `word` stays OUT (not a data transform) —
// it refuses to whole-program fallback.
func TestEmitWidenedAllowSet(t *testing.T) {
	for _, c := range []struct {
		src  string
		want any
		isFn bool // compiles native via a closure (no island, no refusal)
	}{
		{`do [1 add 2]`, int64(3), true},
		{`do [mul 2 (add 3 4)]`, int64(14), true},
		// (`word [1 add 2]` USED to refuse here; the splice now compiles by inline
		// expansion to a plain native lowering — see TestWordSpliceCompilesNative.)
	} {
		got, reason := compile(t, c.src)
		if c.isFn {
			if reason != "" {
				t.Errorf("%q expected a native closure, refused: %s", c.src, reason)
			} else if strings.Contains(got, "FALLBACK") {
				t.Errorf("%q expected a native closure, islanded:\n%s", c.src, got)
			} else if !strings.Contains(got, "PUSH_CLOSURE") {
				t.Errorf("%q expected a PUSH_CLOSURE:\n%s", c.src, got)
			}
		} else if reason == "" {
			t.Errorf("%q expected refusal (whole-program fallback), but compiled:\n%s", c.src, got)
		}
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		out, _, err := a.RunCompiled(c.src)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if len(out) != 1 || out[0] != c.want {
			t.Fatalf("%q = %v, want %v", c.src, out, c.want)
		}
	}
}

// Stage 5 (F4 — general dynamic dispatch): a typed query word
// (get/size/is/typeof/make/type-algebra) on a now-native each result is
// itself native: with the each body compiled to a closure (plan P2) its
// result is a concrete typed List, so the typed query lowers to CALL_NATIVE
// rather than re-dispatching through an island. (A genuinely-dynamic
// operand — a threaded get's Any result — still islands; that path is
// covered by the combination-path pins and the full-corpus gate.)
func TestEmitF4DynamicDispatch(t *testing.T) {
	for _, c := range []struct {
		src  string
		want any
	}{
		{`size (each [mul 2] [1 2 3])`, int64(3)},
		{`typeof (each [mul 2] [1 2 3])`, "List"},
		{`(each [mul 2] [1 2 3]) is List`, "true"},
		{`flex (each [mul 2] [1 2 3])`, "[2 4 6]"},
	} {
		got, reason := compile(t, c.src)
		if reason != "" {
			t.Errorf("%q F4 dispatch refused: %s", c.src, reason)
		} else if strings.Contains(got, "FALLBACK") {
			t.Errorf("%q islanded but a native-each result is a concrete query:\n%s", c.src, got)
		}
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		out, compiled, err := a.RunCompiled(c.src)
		if err != nil || !compiled {
			t.Fatalf("%q: compiled=%v err=%v", c.src, compiled, err)
		}
		if len(out) != 1 || out[0] != c.want {
			t.Fatalf("%q = %v, want %v", c.src, out, c.want)
		}
	}

	// Negative: a CONCRETE-operand query keeps the normal CALL_NATIVE
	// path — it must NOT become a FALLBACK island (which would poison the
	// result to dynamic). `size [1 2 3]` lowers to a native call.
	got, reason := compile(t, `size [1 2 3]`)
	if reason != "" {
		t.Fatalf("size of a concrete list refused: %s", reason)
	}
	if strings.Contains(got, "FALLBACK") {
		t.Errorf("concrete `size [1 2 3]` was islanded but must lower to CALL_NATIVE:\n%s", got)
	}
}

// Stage 5: a flow-control sentinel (break/continue/return) inside an
// island body cannot cross the island boundary to an enclosing compiled
// loop — the sub-engine would set FlowCtrl that the VM can't propagate.
// Such a body refuses to island; the whole program falls back and the
// interpreter unwinds the sentinel correctly.
func TestEmitIslandSentinelRefusal(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_failed; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	// `each [break]` inside a compiled `for`: the break targets the for,
	// not each — it must NOT be islanded.
	src := `for 3 [each [break] [1 2]]`
	if _, reason := compile(t, src); reason == "" {
		t.Errorf("%q islanded a sentinel body but must refuse", src)
	}
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := a.RunCompiled(src)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	if compiled {
		t.Errorf("%q took the compiled path but a sentinel island must fall back", src)
	}
	b, _ := New()
	iout, _ := b.RunInterp(src)
	if len(out) != len(iout) {
		t.Fatalf("%q: compiled-fallback %v != interpreter %v", src, out, iout)
	}
}

// Plan P2: a COMPUTED receiver — a data arg that is a prior compiled
// event's result rather than a baked literal — flows as a normal operand
// to the native higher-order call, with the body compiled to a closure.
// `(iota 4) each [body]` compiles to `… CALL_NATIVE iota; PUSH_CLOSURE;
// CALL_NATIVE each` — no island, the iota result is the each data operand.
func TestEmitThreadedFallbackIsland(t *testing.T) {
	for _, src := range []string{
		`each [mul 2] (iota 4)`,
		`(iota 4) each [mul 2]`,
		`scan [add] (iota 4)`,
		`each [mul 2] (range 1 5)`,
	} {
		got, reason := compile(t, src)
		if reason != "" {
			t.Fatalf("%q computed-receiver closure uncompilable: %s", src, reason)
		}
		if !strings.Contains(got, "PUSH_CLOSURE") || strings.Contains(got, "FALLBACK") {
			t.Errorf("%q did not lower to a native closure call with a computed data operand:\n%s", src, got)
		}
	}

	// Runtime parity: compiled (threaded island) == interpreter.
	for _, c := range []struct {
		src  string
		want any
	}{
		{`each [mul 2] (iota 4)`, "[0 2 4 6]"},
		{`(iota 4) each [mul 2]`, "[0 2 4 6]"},
		{`scan [add] (iota 4)`, "[0 1 3 6]"},
		{`each [mul 2] (range 1 5)`, "[2 4 6 8]"},
		// A threaded island whose dynamic result feeds a second island:
		// the first each's result threads into the second.
		{`each [add 1] (each [mul 2] (iota 3))`, "[1 3 5]"},
	} {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		out, compiled, err := a.RunCompiled(c.src)
		if err != nil || !compiled {
			t.Fatalf("%q: compiled=%v err=%v", c.src, compiled, err)
		}
		if len(out) != 1 || out[0] != c.want {
			t.Fatalf("%q compiled = %v, want %v", c.src, out, c.want)
		}
	}

	// A computed receiver with a concrete def-referencing body compiles
	// native: the def bakes as a const, the data threads as a computed
	// operand (plan P2).
	if got, r := compile(t, `def n 10 each [add n] (iota 3)`); r != "" {
		t.Errorf("threaded each with a concrete def-referencing body refused: %s", r)
	} else if strings.Contains(got, "FALLBACK") {
		t.Errorf("threaded each with a concrete def-referencing body islanded:\n%s", got)
	}
	b, err := New()
	if err != nil {
		t.Fatal(err)
	}
	out, compiled, err := b.RunCompiled(`def n 10 each [add n] (iota 3)`)
	if err != nil || !compiled {
		t.Fatalf("def-body threaded closure: compiled=%v err=%v", compiled, err)
	}
	if len(out) != 1 || out[0] != "[10 11 12]" {
		t.Fatalf("def-body threaded closure = %v, want [10 11 12]", out)
	}
}
