package lang

import (
	"fmt"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"

	"github.com/boru-lang/boru/lang/go/native"
)

// End-to-end tests for the detached-stamp primitive (compiler.StampFnValue) over
// REAL fn bodies — the compile path eng's own tests cannot drive (eng has no
// def/fn words). Positive paths pair with negatives per lang/go/CLAUDE.md.

// stampHarness runs src on a fresh boru, arms runtime stamping when armed is
// true, and returns the boru plus the fn value bound to name.
func stampHarness(t *testing.T, src, name string, armed bool) (*Boru, Value) {
	t.Helper()
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RunInterp(src); err != nil {
		t.Fatalf("setup run: %v", err)
	}
	if armed {
		a.registry.EnableRuntimeStamping()
	}
	v, ok := a.registry.Defs.Top(name)
	if !ok {
		t.Fatalf("binding %q not found after setup", name)
	}
	return a, v
}

// invokeFnValue dispatches a fn VALUE through the callback seam exactly as
// the codec / service words do: MatchFnSig then InvokeCallback.
func invokeFnValue(t *testing.T, a *Boru, fn Value, args ...Value) []Value {
	t.Helper()
	fd, ok := fn.Data.(core.FnDefInfo)
	if !ok {
		t.Fatalf("not a fn value: %v", fn)
	}
	sig := native.MatchFnSig(fn, args)
	if sig == nil {
		t.Fatalf("MatchFnSig found no signature for %d args", len(args))
	}
	out, err := core.InvokeCallback(a.registry, sig, args, fd.Captured)
	if err != nil {
		t.Fatalf("InvokeCallback: %v", err)
	}
	return out
}

// A runtime-constructed fn with a compilable body stamps, and the VM result
// is identical to the interpreter's over the same value. The negative twin:
// without arming (a plain interpreter run), the same value declines.
func TestStampFnValueRealBodyVMMatchesInterpreter(t *testing.T) {
	const src = `def h (fn [[x:Integer] [Integer] [x add 1]])`

	a, plain := stampHarness(t, src, "h", false)
	if _, ok := compiler.StampFnValue(a.registry, plain); ok {
		t.Fatalf("unarmed registry must not stamp")
	}
	interp := invokeFnValue(t, a, plain, core.NewInteger(41))

	a2, v2 := stampHarness(t, src, "h", true)
	stamped, ok := compiler.StampFnValue(a2.registry, v2)
	if !ok {
		t.Fatalf("armed registry must stamp the compilable body")
	}
	fd := stamped.Data.(core.FnDefInfo)
	ref := compiler.CompiledRef(&fd.Signatures[0])
	if ref == nil || ref.Prog == nil {
		t.Fatalf("stamped value must carry a finalized CompiledFnRef")
	}
	vm := invokeFnValue(t, a2, stamped, core.NewInteger(41))

	if len(vm) != len(interp) || len(vm) != 1 {
		t.Fatalf("result shape: vm=%v interp=%v", vm, interp)
	}
	vn, _ := vm[0].AsConcreteInteger()
	in, _ := interp[0].AsConcreteInteger()
	if vn != in || vn != 42 {
		t.Fatalf("vm=%d interp=%d, want both 42", vn, in)
	}

	// The ORIGINAL value is untouched by the clone-and-stamp.
	origFd := v2.Data.(core.FnDefInfo)
	if compiler.CompiledRef(&origFd.Signatures[0]) != nil {
		t.Fatalf("StampFnValue must not mutate the input value's shared impl")
	}

	// A DEP-FREE body (a literal — no module-level name reads) stamps with an
	// empty snapshot and stays vacuously fresh across invokes.
	a3, v3 := stampHarness(t, `def k (fn [[x:Integer] [Integer] [42]])`, "k", true)
	depFree, ok := compiler.StampFnValue(a3.registry, v3)
	if !ok {
		t.Fatalf("dep-free literal body must stamp")
	}
	for i := 0; i < 2; i++ {
		out := invokeFnValue(t, a3, depFree, core.NewInteger(1))
		if n, _ := out[len(out)-1].AsConcreteInteger(); n != 42 {
			t.Fatalf("dep-free stamped fn: got %d, want 42", n)
		}
	}
}

// A callback body whose trailing residual is a COMPUTED MAP (a field built
// from the param — the mini-redis catch-all shape) stamps: the detached
// stored-fn unit records the map's OpMakeMap assembly rather than declining
// "body result of unknown provenance". The VM re-assembles it per invoke,
// value-identical to the interpreter. Recording is enabled ONLY for callback
// bodies (isCallbackBodyName) — every callback is invoked in a live frame via
// InvokeCallback / CallBoru, so the in-frame assembly matches both engines.
func TestStampFnValueComputedMapBodyVMMatchesInterpreter(t *testing.T) {
	const src = `def h (fn [[req:Map] [Map] [ {message: (join "" ["hi " req.who])} ]])`

	a, plain := stampHarness(t, src, "h", false)
	arg := func() Value {
		om := core.NewOrderedMap()
		om.Set("who", core.NewString("bob"))
		return core.NewMap(om)
	}
	interp := invokeFnValue(t, a, plain, arg())

	a2, v2 := stampHarness(t, src, "h", true)
	stamped, ok := compiler.StampFnValue(a2.registry, v2)
	if !ok {
		t.Fatalf("armed registry must stamp the computed-map callback body")
	}
	ref := compiler.CompiledRef(&stamped.Data.(core.FnDefInfo).Signatures[0])
	if ref == nil || ref.Prog == nil {
		t.Fatalf("stamped value must carry a finalized CompiledFnRef")
	}
	vm := invokeFnValue(t, a2, stamped, arg())

	if len(vm) != 1 || len(interp) != 1 {
		t.Fatalf("result shape: vm=%v interp=%v", vm, interp)
	}
	if vm[0].String() != interp[0].String() || vm[0].String() != `{message:'hi bob'}` {
		t.Fatalf("vm=%s interp=%s, want both {message:'hi bob'}", vm[0].String(), interp[0].String())
	}
}

// A stamped body reading a module-level dep: after the dep is rebound the
// depSnap goes stale and the invoke falls back to the interpreter, which
// resolves the LIVE binding — never the frozen one.
func TestStampFnValueDepRebindFallsBackLive(t *testing.T) {
	a, v := stampHarness(t, `
def bump 10
def h (fn [[x:Integer] [Integer] [x add bump]])
`, "h", true)
	stamped, ok := compiler.StampFnValue(a.registry, v)
	if !ok {
		t.Fatalf("stamp declined for the dep-reading body")
	}
	out := invokeFnValue(t, a, stamped, core.NewInteger(1))
	if n, _ := out[0].AsConcreteInteger(); n != 11 {
		t.Fatalf("pre-rebind: got %d, want 11", n)
	}

	// Rebind the dep. The stamped unit froze bump=10; the interpreter (and
	// therefore the stale-dep fallback) must see 20.
	if _, err := a.RunInterp(`def bump 20`); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	out = invokeFnValue(t, a, stamped, core.NewInteger(1))
	if n, _ := out[0].AsConcreteInteger(); n != 21 {
		t.Fatalf("post-rebind: got %d, want the LIVE 21 (interpreter fallback)", n)
	}
}

// A body the compiler declines — a THREE-level curried inline apply threading
// the enclosing param capture (§9.2d compiles two levels; the third declines,
// its own inventory item) — declines the stamp and keeps interpreting
// unchanged. (The former paren-apply fixture graduated 2026-07-17, §9.2e.)
func TestStampFnValueFailingToCompileBodyInterpretsUnchanged(t *testing.T) {
	a, v := stampHarness(t, `
def h (fn [[x:Integer] [Integer] [ 3 2 1 (fn [[a:Integer] [Function] [(fn [[b:Integer] [Function] [(fn [[c:Integer] [Integer] [x add a add b add c]])]])]]) apply apply apply ]])
`, "h", true)
	stamped, ok := compiler.StampFnValue(a.registry, v)
	if ok {
		t.Fatalf("declining body must decline the stamp")
	}
	// The returned value is the input, and it still runs on the interpreter:
	// x + 1 + 2 + 3 = 7+6 = 13.
	out := invokeFnValue(t, a, stamped, core.NewInteger(7))
	if n, _ := out[len(out)-1].AsConcreteInteger(); n != 13 {
		t.Fatalf("declined value must interpret unchanged: got %d, want 13", n)
	}
}

// A body whose OUTER stored-fn unit compiles but whose NESTED fn SUB-UNIT
// declines at Finalize — the detached-stamp Finalize belt (stamp_runtime.go).
// compileStoredFnUnit RECORDS the inner fn as a sub-unit and returns ok (it
// lowers nothing); Finalize's per-unit lowering loop then declines the inner
// unit ("fn zzinner: consumes loop results" — the Stage-2 loop-result
// boundary), so StampDetachedFn leaves the value plain and it interprets
// unchanged. This is distinct from TestStampFnValueFailingToCompileBodyInterpretsUnchanged-
// Unchanged above, where compileStoredFnUnit itself declines (the earlier
// return); here the outer compile succeeds and only Finalize declines. It is
// the shape the variation sweep's module-body transform reaches over a sift
// row — the reason the belt graduated from //covergate:allow
// (design/COVERAGE-ALLOWLIST.10.md).
func TestDetachedStampSubUnitFinalizeCompileFailureDeclines(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	a.registry.EnableRuntimeStamping()
	// Module load detached-stamps every module-scope fn (StampFnValueInPlace).
	// zzouter's OUTER body compiles (compileStoredFnUnit ok — it records the
	// nested zzinner as a sub-unit but lowers nothing), then Finalize's per-unit
	// lowering loop declines zzinner ("fn zzinner: consumes loop results" — the
	// Stage-2 boundary: `size` over the `for 3 [1]` loop residual). So the belt
	// fires: zzouter stays plain and interprets. `[]` (no return contract) keeps
	// the three-value loop residual interpreter-valid. `M.run` returns the inner
	// fn value, which — a 0-arg fn at the top level — auto-invokes and yields
	// the loop's three 1s, proving the graceful fallback runs unchanged.
	out, err := a.RunInterp(`import module [ def zzouter fn [[] [] [def zzinner fn [[] [] [size (for 3 [1])]] zzinner]] export "M" {run: zzouter/v} ] end M.run`)
	if err != nil {
		t.Fatalf("belt is a graceful fallback — the module must load and run, got: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("M.run must yield the loop's three values, got %d: %v", len(out), out)
	}
	// The belt recorded its attribution event for zzouter.
	found := false
	for _, ev := range a.registry.StampEvents() {
		if ev.Name == "zzouter" && !ev.Stamped && ev.Reason == "finalize left the unit unstamped" {
			found = true
		}
	}
	if !found {
		t.Fatalf("belt must record the finalize-left-unstamped event for zzouter: %+v", a.registry.StampEvents())
	}
}

// The detached compile is fully isolated: the live registry's def depths,
// check-state identity, and diagnostics are untouched by a stamp performed
// mid-session.
func TestStampFnValueParentStateUntouched(t *testing.T) {
	a, v := stampHarness(t, `
def bump 10
def h (fn [[x:Integer] [Integer] [x add bump]])
`, "h", true)
	r := a.registry
	checkBefore := r.Check
	emitBefore := r.Check.Emit
	diagsBefore := len(r.Check.Diagnostics)
	depthBase, depthH := r.Defs.Depth("bump"), r.Defs.Depth("h")
	genBase := r.Defs.Gen("bump")

	if _, ok := compiler.StampFnValue(r, v); !ok {
		t.Fatalf("stamp declined")
	}

	if r.Check != checkBefore {
		t.Fatalf("stamp replaced the parent's CheckState")
	}
	if r.Check.Emit != emitBefore {
		t.Fatalf("stamp swapped the parent's Emit recorder")
	}
	if len(r.Check.Diagnostics) != diagsBefore {
		t.Fatalf("stamp leaked %d diagnostics into the parent", len(r.Check.Diagnostics)-diagsBefore)
	}
	if r.Defs.Depth("bump") != depthBase || r.Defs.Depth("h") != depthH {
		t.Fatalf("stamp changed parent def depths")
	}
	if r.Defs.Gen("bump") != genBase {
		t.Fatalf("stamp bumped a parent binding generation")
	}
	if r.Check.Mode {
		t.Fatalf("stamp left the parent in check mode")
	}
}

// stampModuleSrc constructs a module whose helper fn stamps in place at load
// when the importing registry is armed — the observable that proves runtime
// stamping was active during a request.
const stampModuleSrc = `module [ def helper (fn [[x:Integer] [Integer] [x add 1]]) export "M" {helper: helper/v} ]`

// countStamped returns how many report events stamped a fn of the given name.
func countStamped(events []core.StampEvent, name string) int {
	n := 0
	for _, ev := range events {
		if ev.Name == name && ev.Stamped {
			n++
		}
	}
	return n
}

// RunCompiled / RunCompiledStrict arm the policy FOR THE DURATION of the
// request and RESTORE the prior state on return; a plain Run never arms it —
// the mode contract that keeps -no-compile a pure interpreter path, and that
// keeps a compiled-mode request from leaking the armed flag into a later Run.
func TestCompiledEntriesArmRuntimeStamping(t *testing.T) {
	a, _ := New()
	if _, err := a.RunInterp(`import ` + stampModuleSrc); err != nil {
		t.Fatal(err)
	}
	if a.registry.RuntimeStampingEnabled() {
		t.Fatalf("plain Run must not arm runtime stamping")
	}
	if got := countStamped(a.StampReport(), "helper"); got != 0 {
		t.Fatalf("plain Run stamped the helper %d times; -no-compile must never stamp", got)
	}

	// RunCompiled: stamping is ACTIVE during the request (the module helper
	// stamps at load) but RESTORED to unarmed on return.
	b, _ := New()
	if _, _, err := b.RunCompiled(`import ` + stampModuleSrc); err != nil {
		t.Fatal(err)
	}
	if b.registry.RuntimeStampingEnabled() {
		t.Fatalf("RunCompiled must restore the prior (unarmed) flag on return")
	}
	if got := countStamped(b.StampReport(), "helper"); got != 1 {
		t.Fatalf("RunCompiled must stamp the helper once during the request, got %d", got)
	}

	c, _ := New()
	if _, err := c.RunCompiledStrict(`import ` + stampModuleSrc); err != nil {
		t.Fatal(err)
	}
	if c.registry.RuntimeStampingEnabled() {
		t.Fatalf("RunCompiledStrict must restore the prior (unarmed) flag on return")
	}
	if got := countStamped(c.StampReport(), "helper"); got != 1 {
		t.Fatalf("RunCompiledStrict must stamp the helper once during the request, got %d", got)
	}

	// A caller that armed the registry ITSELF keeps it armed: restore returns to
	// the prior state, it does not force-disarm.
	e, _ := New()
	e.registry.EnableRuntimeStamping()
	if _, _, err := e.RunCompiled(`1 add 1`); err != nil {
		t.Fatal(err)
	}
	if !e.registry.RuntimeStampingEnabled() {
		t.Fatalf("RunCompiled must not disarm a flag the caller armed")
	}
}

// The mode contract, end to end: after a compiled-mode request restores the
// flag, a subsequent plain Run must NOT stamp a newly-constructed callback —
// the armed flag never leaks across the RunCompiled boundary. (Already-stamped
// callbacks keep their VM path regardless — InvokeCallback gates on the stored
// ref — so restoring the flag is safe.)
func TestRunCompiledDoesNotLeakStampingIntoLaterRun(t *testing.T) {
	a, _ := New()
	// A service handler stamps at its store site during the compiled request.
	if _, _, err := a.RunCompiled(`def svc (service {}) add {cmd:"X"} ([req:Map state:Any] => [ 10 ]) svc`); err != nil {
		t.Fatal(err)
	}
	if a.registry.RuntimeStampingEnabled() {
		t.Fatalf("RunCompiled leaked the armed flag")
	}
	before := len(a.StampReport())

	// A later plain Run that constructs a fresh handler must add no stamps.
	if _, err := a.RunInterp(`def svc2 (service {}) add {cmd:"Y"} ([req:Map state:Any] => [ 20 ]) svc2`); err != nil {
		t.Fatal(err)
	}
	if after := len(a.StampReport()); after != before {
		t.Fatalf("plain Run after RunCompiled recorded %d new stamps; -no-compile must never stamp", after-before)
	}
}

// The interpreter-fallback path must not double-count the check pass's in-place
// module-load stamps: RestoreForCompile rolls them back, so ResetStampLog drops
// them and only the fallback re-run's authoritative stamps reach the report.
func TestRunCompiledFallbackNoDuplicateStampReport(t *testing.T) {
	// Legacy compile failure+fallback-parity contract: pins the one-release
	// A stampable module import followed by an uncompilable tail (a def
	// consuming a DYNAMIC-count variadic loop region — the stable S5 declining
	// fixture) so the program does not compile after the
	// check pass stamped the module helper in place. (The former paren-apply
	// tail graduated 2026-07-17, §9.2e.)
	src := `import ` + stampModuleSrc + ` def dm {n: 3} def zz (for (dm get "n") [1]) zz`
	a, _ := New()
	_, compiled, err := a.RunCompiled(src)
	if noteCompileDefect(t, src, nil, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if compiled {
		t.Fatalf("expected the compile failure for the declining program")
	}
	if got := countStamped(a.StampReport(), "helper"); got != 1 {
		t.Fatalf("helper stamped %d times in the report; the rolled-back check-pass stamp must not double-count", got)
	}
}

// Module sub-registries inherit the armed flag (RunModuleBody), so module
// bodies — where the real apps construct their callbacks — can stamp; and
// they do NOT inherit it when the parent is unarmed.
func TestModuleRegistryInheritsRuntimeStamping(t *testing.T) {
	src := `module [ def helper (fn [[x:Integer] [Integer] [x add 1]]) export "M" {helper: helper/v} ]`

	a, _ := New()
	a.registry.EnableRuntimeStamping()
	// Rebind the export's fn to a top-level name so the test reads it from
	// the def table without unwrapping the ModuleExport payload Go-side.
	if _, err := a.RunInterp(`import ` + src + ` def hh M.helper/v`); err != nil {
		t.Fatalf("armed import: %v", err)
	}
	helper, ok := a.registry.Defs.Top("hh")
	if !ok {
		t.Fatalf("hh not bound after armed import")
	}
	fd, isFn := helper.Data.(core.FnDefInfo)
	if !isFn || fd.Registry == nil {
		t.Fatalf("M.helper is not a module fn with a sub-registry")
	}
	if !fd.Registry.RuntimeStampingEnabled() {
		t.Fatalf("module sub-registry must inherit the armed flag")
	}

	b, _ := New()
	if _, err := b.RunInterp(`import ` + src + ` def hh M.helper/v`); err != nil {
		t.Fatalf("unarmed import: %v", err)
	}
	h2, ok := b.registry.Defs.Top("hh")
	if !ok {
		t.Fatalf("hh not bound after unarmed import")
	}
	fd2, isFn2 := h2.Data.(core.FnDefInfo)
	if !isFn2 {
		t.Fatalf("M.helper (unarmed) is not a fn value")
	}
	if fd2.Registry != nil && fd2.Registry.RuntimeStampingEnabled() {
		t.Fatalf("unarmed parent must not arm the module sub-registry")
	}
}

// The gradual-nesting mode (detached compiles only): a stored handler whose
// body calls an `Any`-param helper that reads a FIELD of that param compiles
// detached — the nested callee generalises the Any→Any arg as a GRADUAL
// carrier, where a strict Any declined "unmatched dispatch recovered at dot".
// Pins the probe-inherits-mode contract too (a strict probe would decline
// before the gradual real compile ran).
func TestStampFnValueGradualNestedCallee(t *testing.T) {
	const src = `
def helper (fn [[st:Any k:String] [Any] [ def kv2 st.kv  kv2 get k ]])
def h (fn [[req:Map state:Any] [Any] [ helper state "a" ]])
`
	a, v := stampHarness(t, src, "h", true)
	stamped, ok := compiler.StampFnValue(a.registry, v)
	if !ok {
		t.Fatalf("gradual nesting must let the Any-param dot callee compile")
	}

	om := core.NewOrderedMap()
	om.Set("a", core.NewInteger(42))
	stOm := core.NewOrderedMap()
	stOm.Set("kv", core.NewMap(om))
	req := core.NewMap(core.NewOrderedMap())
	state := core.NewMap(stOm)

	vm := invokeFnValue(t, a, stamped, req, state)
	interp := invokeFnValue(t, a, v, req, state)
	vn, _ := vm[len(vm)-1].AsConcreteInteger()
	in, _ := interp[len(interp)-1].AsConcreteInteger()
	if vn != in || vn != 42 {
		t.Fatalf("vm=%d interp=%d, want both 42", vn, in)
	}
}

// Module-load stamping + the module-export apply reroute (Phase 4): an
// armed import compiles each eligible module fn to a detached unit IN PLACE
// (def binding and export map share the impl), and execFnDefSig routes a
// stamped module-export application through InvokeCallback at runtime. The
// unarmed twin keeps everything plain.
func TestModuleFnStampedAtLoadAndRerouted(t *testing.T) {
	src := `module [
  def helper (fn [[x:Integer] [Integer] [x add 1]])
  def alias (helper/v)
  def decliner (fn [[x:Integer] [Integer] [ (((fn [[a:Integer] [Function] [(fn [[b:Integer] [Function] [(fn [[c:Integer] [Integer] [x add a add b add c]])]])]]) 1) 2) 3 ]])
  def fact (fn [[n:Integer] [Integer] [ if (n lte 1) [1] [n mul (fact (n sub 1))] ]])
  def tbl {k: 1}
  export "M" {helper: helper/v fact: fact/v decliner: decliner/v}
]`

	fetch := func(a *Boru, name string) Value {
		t.Helper()
		if _, err := a.RunInterp(`def got M.` + name + `/v`); err != nil {
			t.Fatalf("fetch %s: %v", name, err)
		}
		v, ok := a.registry.Defs.Top("got")
		if !ok {
			t.Fatalf("got not bound")
		}
		return v
	}
	refOf := func(v Value) *compiler.CompiledFnRef {
		fd, isFn := v.Data.(core.FnDefInfo)
		if !isFn {
			t.Fatalf("not a fn value")
		}
		for i := range fd.Signatures {
			if r := compiler.CompiledRef(&fd.Signatures[i]); r != nil {
				return r
			}
		}
		return nil
	}

	// The export map holds a module-fn WRAPPER; dispatch matches the INNER
	// def binding's signatures (execFnDefLiteral looks the name up in the
	// wrapper's Registry), so the stamp must be asserted on the inner value.
	inner := func(a *Boru, name string) Value {
		t.Helper()
		w := fetch(a, name)
		fd, isFn := w.Data.(core.FnDefInfo)
		if !isFn || fd.Registry == nil {
			t.Fatalf("%s: export is not a module fn wrapper", name)
		}
		v, ok := fd.Registry.Defs.Top(name)
		if !ok {
			t.Fatalf("%s: inner binding missing", name)
		}
		return v
	}

	armed, _ := New()
	armed.registry.EnableRuntimeStamping()
	if _, err := armed.RunInterp(`import ` + src); err != nil {
		t.Fatalf("armed import: %v", err)
	}
	if ref := refOf(inner(armed, "helper")); ref == nil || ref.Prog == nil {
		t.Fatalf("armed load must stamp the module fn's inner binding")
	}
	if refOf(inner(armed, "decliner")) != nil {
		t.Fatalf("a declining body must stay unstamped at load")
	}

	// Applications route through the runtime seam and agree with the
	// unarmed interpreter, including recursion through the module name.
	plain, _ := New()
	if _, err := plain.RunInterp(`import ` + src); err != nil {
		t.Fatalf("plain import: %v", err)
	}
	if refOf(inner(plain, "helper")) != nil {
		t.Fatalf("an unarmed load must not stamp module fns")
	}
	for _, probe := range []string{`M.helper 41`, `M.fact 5`, `M.decliner 7`} {
		gotA, errA := armed.RunInterp(probe)
		gotP, errP := plain.RunInterp(probe)
		if errA != nil || errP != nil {
			t.Fatalf("%s: errs armed=%v plain=%v", probe, errA, errP)
		}
		if fmt.Sprint(gotA) != fmt.Sprint(gotP) {
			t.Fatalf("%s: armed=%v plain=%v (must agree)", probe, gotA, gotP)
		}
	}
}
