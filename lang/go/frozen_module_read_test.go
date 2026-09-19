package lang

// Frozen module reads (the unit-bake freeze discipline): a CONCRETE
// module-scope binding read inside a fn/closure UNIT analysis bakes into the
// unit (a const, or splice-fired tokens) that re-runs on every call, while
// the interpreter re-resolves the name per call (the documented module-level
// dynamic semantics, lang/go/CLAUDE.md "Closures and Capture"). A later
// module-scope rebind therefore made the compiled program diverge:
//
//	def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  def x 2  f 0
//	interpreter: 1 2      compiled (before the fix): 1 1   ← MISCOMPILE
//
// The first fix (2026-09-03/04) REFUSED such programs: NoteFrozenRead
// recorded the read and NotifyNameRebound marked the program uncompilable on
// a later module-scope rebind — interpreter fallback, correct result. Stage
// 4b (compiler/go/unit_memo.go) replaced the refusal with a binding-sensitive
// unit memo: the note now records the binding's generation on the unit, and
// the call after the rebind re-records the unit instead of reusing the stale
// one. The refusal survives only for a unit whose reference ESCAPES into a
// value (a returned closure, a stamped fn value), which no later call site
// can refresh. STORED-REF units (service handlers, spawn bodies) are exempt
// either way: their rebind safety is the precise per-ref poisoning
// (TestCompiledStoredHandlerFreezeRedefine), which keeps the rest of the
// program compiled.

import (
	"fmt"
	"strings"
	"testing"
)

// The latch's own text — "module binding <name> rebound after a fn unit
// baked its <bake>" — is pinned EXACTLY where it can still fire, in
// compiler/go/frozen_read_test.go (NotifyNameRebound's escaping-unit arm).
// No end-to-end row pins it here, and that is a measured gap, not an
// oversight: every returned-closure shape that would reach the latch at top
// level refuses earlier on its own residual shape (`(mk 1) 2` → "call result
// above a literal"; `def h (mk 1)  h 2` → "unconsumed fn-value carrier"), so
// the first such shape to compile is the one that owes this file its row.
// MarkUncompilable is FIRST-REASON-WINS, which is why that row must assert
// the text and not merely `reason != ""`.

// TestModuleReadRebindCompilesWithParity — the rows that REFUSED under the
// frozen-read latch until Stage 4b, and now compile: the unit memo is
// binding-sensitive (compiler/go/unit_memo.go), so the call after the rebind
// re-records the unit against the new binding instead of reusing the one
// baked before it. Each row was a measured miscompile before its arm of the
// discipline existed (the "before" column), then a refusal, and is now a
// compiled program that agrees with the interpreter. bound / bake name what
// the row rebinds and what the unit had baked — the three artifacts the
// three arms defend, all repaired by the one mechanism.
func TestModuleReadRebindCompilesWithParity(t *testing.T) {
	cases := []struct{ src, bound, bake, before string }{
		// The documented module-dynamic case (scalar read baked as a const).
		{`def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  def x 2  f 0`, "x", "value", "1 1 for 1 2"},
		// The TYPE twin: a type name lives in the SAME single binding store
		// (lang/go/CLAUDE.md "Registry Bindings"), so a module-scope type
		// read inside a unit froze exactly as a value read did — the unit
		// lowered to OpPushType carrying the node's compile-time IDENTITY.
		// Measured on the DEFAULT lane before its arm existed:
		//
		//	interpreted -> true false      compiled -> true true
		{`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  def T String  f`, "T", "type", "true true for true false"},
		// The CALL-TARGET family, and the plainest shape in this file: define
		// a helper, define a caller, call it, redefine the helper. That is the
		// documented late-binding half of NUR097 and it is what a REPL session
		// or a hot reload does, so it is not an edge case the corpus merely
		// happens to lack. The unit lowered to `TAIL_CALL_USER f1` naming a
		// SPECIFIC compiled unit; the bind twin replayed `def-replace helper`
		// correctly beside it and the call target did not move (§6.5's
		// lesson: the twins fix where the registry is, the memo fixes what
		// is in the bytecode). Measured on the default lane before its arm:
		//
		//	interpreted -> 2 101      compiled -> 2 2
		{`def helper fn [[x:Integer] [Integer] [x add 1]]  def use fn [[x:Integer] [Integer] [helper x]]  ` +
			`use 1  def helper fn [[x:Integer] [Integer] [x add 100]]  use 1`, "helper", "call target", "2 2 for 2 101"},
		// Transitively: the rebind is two call levels below the unit that
		// froze it — the memo's staleness walks the unit-reference graph.
		// `1 9` interpreted, `1 1` compiled before the arm.
		{`def a fn [[][Integer][1]]  def b fn [[][Integer][a]]  def c fn [[][Integer][b]]  c  ` +
			`def a fn [[][Integer][9]]  c`, "a", "call target", "1 1 for 1 9"},
		// The ZERO-OUTPUT call site: a 0-output CALL_USER recorded through a
		// SECOND RecordUserCall site, whose divergence is INVISIBLE to the
		// differential — a 0-output fn leaves no value to compare, so both
		// lanes return `[]` and only stdout differs (`1 1` compiled against
		// the interpreter's `1 99` before the arm). What this row can assert
		// is that it compiles and agrees on the (empty) residual.
		{`def g fn [[] [] [print 1]]  def f fn [[] [] [g]]  f  def g fn [[] [] [print 99]]  f`,
			"g", "call target", "prints 1 1 for 1 99"},
		// The `/v` SPELLING of a frozen read, both bakes. `T/v` and `k/v`
		// resolve through stepWordVal, which reaches NEITHER of stepWord's
		// substitution branches — so both walked straight past the latch while
		// `resolveOperand` baked them exactly as it bakes the plain spelling.
		// The TYPE row was found by review on the type arm's own PR; the VALUE
		// row is OLDER than that arm. Measured on the default lane:
		//
		//	def T Integer  def f fn [[] [Boolean] [5 is T/v]]  f  def T String  f
		//	  interpreted -> true false      compiled -> true true
		//	def k 5  def f fn [[] [Integer] [k/v add 2]]  f  def k 9  f
		//	  interpreted -> 7 11            compiled -> 7 7
		{`def T Integer  def f fn [[] [Boolean] [5 is T/v]]  f  def T String  f`, "T", "type", "true true for true false"},
		{`def k 5  def f fn [[] [Integer] [k/v add 2]]  f  def k 9  f`, "k", "value", "7 7 for 7 11"},
		// The REBIND SITE axis — NUR117. A rebind written inside a `do` body
		// reached NotifyNameRebound with the recorder SUSPENDED, so the latch
		// was never consulted and the program compiled against a unit still
		// holding the old bake (`7 7` for `7 11`). A `do` body's defs LEAK to
		// the enclosing scope, so the rebind is as real as a top-level one —
		// and the memo sees it exactly as it sees a top-level one: the leaked
		// binding's generation moved.
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  do [def k 9]  f`, "k", "value", "7 7 for 7 11"},
		{`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  do [def T String]  f`, "T", "type", "true true for true false"},
		{`def g fn [[][Integer][1]]  def f fn [[] [Integer] [g]]  f  ` +
			`do [def g fn [[][Integer][2]]]  f`, "g", "call target", "1 1 for 1 2"},
		// The SPLICE twin: the macro payload fires into the unit's tokens at
		// analysis time, and the re-recorded unit fires the NEW payload. Its
		// two Any-typed call results were a residual the lowering refused
		// ("dynamic value precedes residual args") until a user call's single
		// result was recognised as PARKED data (callResultPlaced,
		// design/PAREN-RESTEP-RULE.0.md §2.1); it compiles with parity now.
		{`def op (quote [1 add 2])  def f fn [[n:Integer] [Any] [do [n drop word op]]]  f 0  def op (quote [10 mul 4])  f 0`,
			"op", "splice payload", "refused (fn-value-call boundary) for 3 40"},
	}
	for _, c := range cases {
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled {
			t.Errorf("%q: the rebind of %s (%s baked) must compile under the binding-sensitive memo "+
				"(before its arm: %s); compiled=false err=%v", c.src, c.bound, c.bake, c.before, errC)
			continue
		}
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// TestModuleReadRebindSoundFallbacks — the rows the memo hands to the
// interpreter rather than compiling, each pinned with its reason. The UNDEF
// rows re-analyse the unit against a name that is gone, so the check pass
// reports undefined_word and the program refuses on check diagnostics — the
// interpreter then raises the same undefined_word at run time. The MULTI-RUN
// rows reach the frozen unit through a top-level read after an each body
// bound the name, which the arm-residency gate refuses (the runtime binding
// is per-element, or absent at zero iterations). Each refusal's fallback is
// parity with the interpreter; a row that starts compiling has graduated and
// moves to the parity test with its interpreter answer.
//
// The `5 is T` rows are ROUTED forward slots since the sixty-fifth
// increment (`is` dispatches through its descriptor, region_route.go), and
// they still refuse here: routing retires the escaping latch's note, not
// the memo's key, so the undef re-records the unit and the check pass
// reports the undefined name exactly as before (review of #461). The
// routed op meets a live rebind only where no call site can re-record.
func TestModuleReadRebindSoundFallbacks(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_failed; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	const armRead = "twin regime: read of `k` after a multi-run body binds it"
	// defer names the op's defer site for a row that compiles and hands the
	// RUN to the interpreter; empty for a row that refuses at check.
	cases := []struct{ src, reason, defer_ string }{
		// The UNDEF twins: `undef` reaches the same rebind notification as
		// `def`, and the re-recorded unit reads a name the pass no longer
		// binds. Before the discipline's arms: `7 7` where the interpreter
		// raises undefined_word; `true true` for the type; `1 1` for the
		// sig-undef of a call target.
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  undef k  f`, "check diagnostics", ""},
		{`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  undef T  f`, "check diagnostics", ""},
		{`def g fn [[][Integer][1]]  def f fn [[] [Integer] [g]]  f  undef g (fnsig [[] [Integer]])  f`, "check diagnostics", ""},
		{`def T Integer  def f fn [[] [Boolean] [5 is T/v]]  f  undef T  f`, "check diagnostics", ""},
		{`def k 5  def f fn [[] [Integer] [k/v add 2]]  f  undef k  f`, "check diagnostics", ""},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  do [undef k]  f`, "check diagnostics", ""},
		{`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  do [undef T]  f`, "check diagnostics", ""},
		// The MULTI-RUN twin of the rebind-site axis: an each body leaks its
		// LAST iteration's def to module scope, and the top-level read after
		// it is the arm-residency gate's. Measured `7 [9] 11` interpreted
		// against `7 [9] 7` compiled before the arm.
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  [1] each [def k 9  k]  f`, armRead, ""},
		{`def k 5  def f fn [[] [Integer] [k add 2]]  f  [1 2] each [def k 9  k]  f`, armRead, ""},
		{`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  [1] each [def T String  1]  f`,
			"twin regime: a bind transition has no stream placement", ""},
	}
	for _, c := range cases {
		src := c.src
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil {
			t.Fatalf("CompileCheck(%q): %v", src, cerr)
		}
		if c.defer_ != "" {
			if prog == nil {
				t.Errorf("%q: the routed read compiles; refused with %q", src, reason)
				continue
			}
			b, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			var bails []string
			disarm := b.ArmRuntimeBailHook(func(ev BailEvent) { bails = append(bails, ev.Site) })
			_, compiled, _ := b.RunCompiled(src)
			disarm()
			if compiled || len(bails) != 1 || bails[0] != c.defer_ {
				t.Errorf("%q: want the run deferred at %s, got compiled=%v bails=%v", src, c.defer_, compiled, bails)
			}
		} else {
			if prog != nil {
				t.Errorf("%q: compiled — this shape has graduated; move it to the parity rows", src)
				continue
			}
			if !strings.Contains(reason, c.reason) {
				t.Errorf("%q: refusal drifted: want %q in %q", src, c.reason, reason)
			}
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if compiled {
			t.Errorf("%q: expected the interpreter fallback", src)
		}
		if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
			t.Errorf("%q: engine divergence: compiled=%v/%v interp=%v/%v", src, gotC, errC, gotI, errI)
		}
	}
}

// TestModuleReadCrossFamilyRebindCompiles — the CROSS-FAMILY twin, the row
// that once said a live operand alone could not repair the bake: the unit had
// lowered to two PUSH_CONSTs feeding a MONO `CALL_NATIVE add (Number, Number)`
// chosen from the frozen value, so making the read live without re-deciding
// the signature could not reproduce the interpreter. The memo re-records the
// unit against the String binding instead, so the second call's unit selects
// add's String arm at analysis, concatenates to 'x2', and f's declared
// [Integer] return raises — the interpreter's own type_error, same code, same
// message.
//
// What still differs is the BLAME POSITION: the compiled RET check blames the
// body's first token where the interpreter blames the call site (NUR118 —
// pre-existing for every compiled return-contract error, measured on the
// tree before this row could reach it). The row therefore compares the
// error's first line, not its rendering.
func TestModuleReadCrossFamilyRebindCompiles(t *testing.T) {
	src := "def k 5  def f fn [[] [Integer] [k add 2]]  f  def k \"x\"  f"
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled {
		t.Fatalf("%q: must compile under the binding-sensitive memo; err=%v", src, errC)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("%q: value divergence: compiled=%v interp=%v", src, gotC, gotI)
	}
	if errC == nil || errI == nil {
		t.Fatalf("%q: both lanes must raise f's return type_error; compiled=%v interp=%v", src, errC, errI)
	}
	firstLine := func(err error) string { return strings.SplitN(err.Error(), "\n", 2)[0] }
	if firstLine(errC) != firstLine(errI) {
		t.Errorf("%q: error divergence beyond the NUR118 position: compiled=%q interp=%q", src, firstLine(errC), firstLine(errI))
	}
}

// TestModuleReadNoRebindStillCompiles — the positive pair: the same fn over a
// NEVER-rebound module binding keeps compiling (the freeze is keyed on actual
// rebinds, not on "reads a module ref").
func TestModuleReadNoRebindStillCompiles(t *testing.T) {
	srcs := []string{
		`def x 1  def f fn [[y:Integer] [Integer] [x add y]]  f 0  f 10`,
		// The TYPE control, and the one that bounds the 2026-09-04 fix. The
		// new note fires on EVERY module-scope type read inside a unit, so
		// this row is what says the latch still needs a REBIND to trip: a
		// type read in a unit, called twice, must keep compiling.
		`def T Integer  def f fn [[] [Boolean] [5 is T]]  f  f`,
		// And the type read that is REBOUND but never read inside a unit —
		// the other side of the same bound. Nothing froze, so nothing refuses.
		`def T Integer  5 is T  def T String  5 is T`,
		// The CALL-TARGET controls, bounding the third arm the same way. A fn
		// called from a unit and never rebound keeps compiling —
		`def g fn [[][Integer][1]]  def f fn [[] [Integer] [g]]  f  f`,
		// so does SELF-recursion, which reads its own name inside its own unit
		// (the shape that would break every recursive fn if the note fired on
		// a name the analysis merely resolves) —
		`def fact fn [[n:Integer] [Integer] [if (n lte 1) [1] [n mul (fact (n sub 1))]]]  fact 5`,
		// and so does a rebind with no unit between it and the call.
		`def g fn [[][Integer][1]]  g  def g fn [[][Integer][2]]  g`,
		// and the 0-output twin of the same control.
		`def g fn [[] [] [print 1]]  def f fn [[] [] [g]]  f  f`,
		// The `/v` controls: the same reads with no rebind keep compiling, so
		// the new note is keyed on an actual rebind rather than on the
		// spelling.
		`def k 5  def f fn [[] [Integer] [k/v add 2]]  f  f`,
		`def T Integer  def f fn [[] [Boolean] [5 is T/v]]  f  f`,
		// The NUR117 controls. A `do` body that rebinds NOTHING the unit
		// froze must still compile — the latch stays keyed on an actual
		// rebind of a baked name, not on "a `do` body is open" —
		`def k 5  def f fn [[] [Integer] [k add 2]]  f  do [1 add 2]  f`,
		// and so must a rebind with no frozen read anywhere.
		`def k 5  do [def k 9]  k`,
		// A `do` INSIDE a fn body binds a frame-local, not a module binding,
		// so it must not refuse: both publication gates test FnBodyDepth, and
		// this row is what fails if either stops.
		`def k 5  def f fn [[] [Integer] [k add 2]]  f  ` +
			`def g fn [[] [Integer] [do [def z 1]  z]]  g  f`,
	}
	for _, src := range srcs {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(src)
		if cerr != nil || prog == nil {
			t.Fatalf("%q: must compile; reason=%q err=%v", src, reason, cerr)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
		if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
			t.Errorf("%q: want compiled parity; compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
				src, compiled, gotC, errC, gotI, errI)
		}
	}
}

// TestStaticSpliceBodiesCompile — closure bodies containing `word` splices
// over compile-time-known payloads compile NATIVELY: the check engine fires
// the splice during body analysis (`word` is a non-emitting projection) and
// the post-splice dispatches record into the unit. Unblocked by the
// multi-out/out-of-order residual work; pinned here so the shapes never
// regress to "code-body word do (Stage 2)".
func TestStaticSpliceBodiesCompile(t *testing.T) {
	cases := []struct{ src, want string }{
		{`def xs (quote [1 add 2])  do [word xs]`, "[3]"},
		{`def op (quote [mul 2])  do [5 word op]`, "[10]"},
		{`def twice word [dup add]  do [5 twice]`, "[10]"},
		{`def op (quote [mul 2])  [1 2 3] each [word op]`, "[[2 4 6]]"},
		// a computed payload that CONST-FOLDS (literal index) composes.
		{`def i 0  def ops [quote [1 add 2]]  do [word (ops get i)]`, "[3]"},
	}
	for _, c := range cases {
		a, err := New()
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: splice body must compile; reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if strings.Contains(prog.Disassemble(), "FALLBACK") {
			t.Errorf("%q: must compile natively, not island", c.src)
		}
		gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
		if !compiled || errC != nil || errI != nil ||
			fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != c.want {
			t.Errorf("%q: want %s compiled with parity; compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
				c.src, c.want, compiled, gotC, errC, gotI, errI)
		}
	}
	// A RUNTIME-computed payload (the def evaluates its list body) compiles
	// via the dyn-body backstop: the def's OpBindDynScope twin re-binds the
	// RUNTIME value (its computed source event promotes to a frame local
	// under DynEnv — the OpMakeList shape), and the body's sub-run splices
	// the live binding exactly as the interpreter does.
	src := `def xs [add 1 2]  do [word xs]`
	b, _ := New()
	prog, reason, _, _ := b.CompileCheck(src)
	if prog == nil {
		t.Errorf("%q: the dyn-body backstop must compile a computed splice payload; reason=%q", src, reason)
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[3]" {
		t.Errorf("%q: want [3] compiled with parity: compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
			src, compiled, gotC, errC, gotI, errI)
	}
}

// TestComputedEachBodyCompiles — a COMPUTED body on each compiles with parity
// through the dyn-body backstop: since S1a (2026-09-19,
// design/FULL-COMPILATION-REPLAN.0.md) each declares CompileDynBody, so the
// carrier code body no island could bake (its tokens carry the island's
// program) is handed to the handler at run time, exactly as `do`'s is.
// Until S1a the shape refused with fallback parity; this pinned the
// island's non-bakeable NoEvalArgs decline, which the backstop now bypasses.
func TestComputedEachBodyCompiles(t *testing.T) {
	src := `def op (quote [mul 2])  def f fn [[b:List] [List] [[1 2 3] each b]]  f op`
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("CompileCheck(%q): %v", src, cerr)
	}
	if prog == nil {
		t.Errorf("%q: the dyn-body backstop must compile a computed each body; reason=%q", src, reason)
	}
	gotC, compiled, errC, gotI, errI := runBothEngines(t, src)
	if !compiled || errC != nil || errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[[2 4 6]]" {
		t.Errorf("%q: want [[2 4 6]] compiled with parity: compiled=%v gotC=%v errC=%v gotI=%v errI=%v",
			src, compiled, gotC, errC, gotI, errI)
	}
}
