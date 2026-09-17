package lang

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
)

// Stage M2c landing tests — shaped-instance-method dispatch
// (design/legacy/STAGE3-INLINING-DESIGN-ROUND.0.ignore §6 M2c; eng/go/method_shape.go).
//
// A module instance (logger / instrument / span / rand handle) is a Map of
// trivial-delegation method wrappers closing over per-instance state. The
// method read stays DYNAMIC (the check-mode shape instance must never bake —
// the freeze-gate), but the read is ANNOTATED with the resolved member's
// shape, the compile pass MODELS the interpreter's auto-dispatch at the exact
// point it happens, and a guarded mid-stream OpCallDynMethod applies the
// RUNTIME value with a claimed arity + result count. A failed claim defers to
// the interpreter via internal_error (RunCompiled's runtimeShouldFallback).

// --- positives: the eight formerly-stalled module-log rows ----------------

func TestShapedMethodDispatchCompiles(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"module-log.tsv:53 — statement-position l.info stamps the logger name",
			`import "boru:log" ; def l (Log.with "http" {svc:"api"}) ; Log.add-sink memory/q ; Log.remove-sink console/q ; l.info "req" ; Log.dump 0 get "logger" get`,
			"[http]"},
		{"module-log.tsv:54 — 2-arg l.info merges defaults under call fields",
			`import "boru:log" ; def l (Log.with "http" {svc:"api"}) ; Log.add-sink memory/q ; Log.remove-sink console/q ; l.info "req" {path:"/x"} ; Log.dump 0 get "attributes" get`,
			"[{svc:'api' path:'/x'}]"},
		{"module-log.tsv:55 — paren-bounded l.child derives a shaped child logger",
			`import "boru:log" ; def l (Log.with "a" {x:1}) ; def c (l.child {y:2}) ; Log.add-sink memory/q ; Log.remove-sink console/q ; c.info "m" ; Log.dump 0 get "attributes" get`,
			"[{x:1 y:2}]"},
		{"module-log.tsv:56 — call field overrides a logger default",
			`import "boru:log" ; def l (Log.with "a" {x:1}) ; Log.add-sink memory/q ; Log.remove-sink console/q ; l.error "boom" {x:2} ; Log.dump 0 get "attributes" get`,
			"[{x:2}]"},
		{"module-log.tsv:77 — two counter adds record two measurements",
			`import "boru:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; def c (Log.counter "requests") ; c.add 1 ; c.add 2 ; Log.measurements size`,
			"[2]"},
		{"module-log.tsv:78 — a counter records its added value",
			`import "boru:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; def c (Log.counter "requests") ; c.add 3 ; Log.measurements 0 get "value" get`,
			"[3]"},
		{"module-log.tsv:79 — a gauge records gauge-kind measurements",
			`import "boru:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; def g (Log.gauge "temp") ; g.record 21 ; Log.measurements 0 get "kind" get`,
			"[gauge]"},
		{"module-log.tsv:80 — a histogram records measurement attributes",
			`import "boru:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; def h (Log.histogram "lat") ; h.record 12 {ep:"/x"} ; Log.measurements 0 get "attributes" get`,
			"[{ep:'/x'}]"},
	} {
		fnValueM2Native(t, c.name, c.src, c.want)
	}
}

// Side-effect ORDERING: a statement-position shaped method call WRITES (the
// console sink prints the record), and the compiled op must run at exactly
// the interpreter's point in the output stream — between the surrounding
// prints, byte-identical under a frozen clock. This test also pins the
// tryNativeFnApply registry-parity fix: the compiled fast path hands the
// handler the DISPATCHING registry (as execMatch does), so the frozen clock
// stamps the record identically in both engines — the prior sub-registry
// pass-through printed wall time on the compiled path only.
func TestShapedMethodEffectOrdering(t *testing.T) {
	src := `import "boru:log" ; def l (Log.with "ord" {svc:"api"}) ; print "a" ; l.info "req" ; print "b" ; 42`
	clk := capabilities.FixedClock{T: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)}
	// One capture stream for BOTH effect channels: print writes through the
	// registry Output and the console sink writes the record line through the
	// registry ErrOutput — pointing both at one pipe makes the full effect
	// order observable as a single byte stream.
	capture := func(run func(a *Boru)) string {
		rd, w, _ := os.Pipe()
		a := mustNew(t)
		a.SetOutput(w)
		a.registry.ErrOutput = w
		a.SetClock(clk)
		run(a)
		w.Close()
		out, _ := io.ReadAll(rd)
		return string(out)
	}
	var gotC, gotI []any
	outC := capture(func(a *Boru) {
		var compiled bool
		var err error
		gotC, compiled, err = a.RunCompiled(src)
		if !compiled || err != nil {
			t.Errorf("effect ordering: compiled=%v err=%v", compiled, err)
		}
	})
	outI := capture(func(a *Boru) {
		var err error
		gotI, err = a.RunInterp(src)
		if err != nil {
			t.Errorf("effect ordering: interp err=%v", err)
		}
	})
	if outC != outI {
		t.Errorf("effect ordering: compiled output %q != interpreted %q", outC, outI)
	}
	if !strings.Contains(outC, "2021-01-01") {
		t.Errorf("effect ordering: frozen clock not honoured on the compiled path: %q", outC)
	}
	ai, ri, bi := strings.Index(outC, "a\n"), strings.Index(outC, "req"), strings.Index(outC, "b\n")
	if ai < 0 || ri < 0 || bi < 0 || ai > ri || ri > bi {
		t.Errorf("effect ordering: log record not between the prints: %q", outC)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotC) != "[42]" {
		t.Errorf("effect ordering: compiled=%v interp=%v want [42]", gotC, gotI)
	}
}

// --- the former guard rows: 0-arg landings now compile ---------------------

// The 4 miscompile-E rows (module-log 72/73, module-rand 14/15) used to sit
// on a read-side refusal: a member with a genuine 0-arg overload
// (Span.finish, Rand.bool/float) auto-dispatches the moment it lands, which
// the read guard blocked wholesale. The guard is now RE-HOMED onto the
// landing: NoteMethodShape ANNOTATES the 0-arg class, the read guards skip
// the annotated read (EmitState.noteShapedRead), and the landing model claims
// it as an explicit arity-0 OpCallDynMethod (shapedMethodApplyWindow's 0-arg
// path) — or refuses via tryShapedMethodDispatch's guard-owned decline. So
// the rows compile with parity, and the VM's poly-get returns the member
// VALUE for the opcode to consume (no runtime auto-apply).
func TestShapedMethodZeroArgLandingsCompile(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"module-log.tsv:72 — span set-attr/finish",
			`import "boru:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; def s (Log.span "m") ; s.set-attr region/q "eu" ; s.finish ; Log.traces 0 get "attributes" get`,
			"[{region:'eu'}]"},
		{"module-log.tsv:73 — span record-error/finish",
			`import "boru:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; def s (Log.span "m") ; s.record-error "boom" ; s.finish ; Log.traces 0 get "status" get`,
			"[error]"},
		{"module-rand.tsv:14 — r.bool (0-arg draw)",
			`import "boru:rand"  def r (Rand.with-seed 7)  r.bool`,
			"[false]"},
		{"module-rand.tsv:15 — r.float (0-arg draw)",
			`import "boru:rand"  def r (Rand.with-seed 1)  r.float`,
			"[0.6046602879796196]"},
	} {
		mustCompileWithParity(t, c.src, c.want)
	}
}

// A CAPTURING method fn (a map member that is a closure over an enclosing
// fn's state, not a delegation wrapper) is outside the shaped-method class:
// NoteMethodShape declines it, the statement-position call keeps refusing,
// and the fallback stays faithful.
func TestShapedMethodCapturingMemberStaysRefused(t *testing.T) {
	fnValueM2Refusal(t, "capturing member fn in statement position",
		`def mk fn [[x:Integer] [Map] [{f: (([y:Integer] => [x add y]))/v}]] def m (mk 5) m.f 1 ; 42`,
		"")
}

// A window token the model cannot bake (a paren to evaluate) declines the
// model — and the ANNOTATED carrier then also declines the residual
// leading-apply window (methodShapeAnnotated), because that window absorbs
// values across statement boundaries: this exact source compiled to the
// WRONG value pre-M2c (the leading OpCallDynamic applied c.add to the add
// result AND the next statement's size result — probe-confirmed divergence
// on the committed tree). It must now refuse, with faithful fallback.
//
// Re-diagnosed 2026-09-05 (NUR121): the collection-hazard mark names the
// same fact one stage earlier — the next statement's `size` stack-collected
// past the unapplied `c.add` lead, so any apply of that lead over the
// residual would run over `size`'s result. Same refusal, earlier and
// truer diagnosis; the methodShapeAnnotated decline still stands behind it.
func TestShapedMethodComputedArgStaysRefused(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	// BORU_COMPILE_FALLBACK=1 hatch behavior (Stage J flipped the default
	// to compile_refused; migrate this contract or retire it with the hatch).
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	fnValueM2Refusal(t, "computed arg in the statement window",
		`import "boru:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; def c (Log.counter "n") ; c.add (1 add 2) ; Log.measurements size`,
		"fn-value lead's argument was collected by a later dispatch (NUR121)")
}

// --- the shape-claim violation: internal_error → interpreter re-run --------

// zzShapedInstance registers `zz-inst`, a word returning an instance whose
// method `m` DECLARES Returns [] (the shape claim the model compiles) but
// whose runtime handler RETURNS a value — a deliberate shape-claim violation.
// The compiled OpCallDynMethod must detect the count mismatch and defer via
// internal_error: RunCompiled silently re-runs the interpreter (correct
// result, fallback), RunCompiledStrict surfaces the internal_error loudly.
func zzShapedInstance(t *testing.T) *Boru {
	t.Helper()
	a := mustNew(t)
	if err := zzInstallShapedInstance(a); err != nil {
		t.Fatalf("zz-inst fixture: %v", err)
	}
	return a
}

// zzInstallShapedInstance registers the zz-inst fixture on an existing
// instance without a *testing.T, so data-driven suites (the frontier cases)
// can build it too.
func zzInstallShapedInstance(a *Boru) error {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return err
	}
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "zz-m",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny},
			Returns:    []*native.Type{}, // the CLAIM: no results
			BarrierPos: -1,
			Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{native.NewInteger(7)}, nil // the LIE: one result
			}),
		}},
	})
	wrapper := native.NewFunction(native.FnDefInfo{
		Name: "zz-m",
		Signatures: []native.FnSig{{
			Params:     []native.FnParam{{Type: native.TAny}},
			Returns:    []*native.Type{},
			Impl:       native.Boru([]native.Value{native.NewWord("zz-m")}),
			BarrierPos: -1,
		}},
		Registry: subReg,
	})
	inst := func() native.Value {
		m := native.NewOrderedMap()
		m.Set("m", wrapper)
		return native.NewMap(m)
	}
	a.Register("zz-inst", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{native.TMap},
		BarrierPos: -1,
		ReturnsFn: func(_ []native.Value, _ *native.Registry) []native.Value {
			return []native.Value{inst()}
		},
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			return []native.Value{inst()}, nil
		}),
	})
	return nil
}

func TestShapedMethodClaimViolationDefers(t *testing.T) {
	src := `def i (zz-inst) ; i.m 5 ; 42`

	// The claim compiles (the model believed the declared Returns []).
	a := zzShapedInstance(t)
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("claim violation: prog=%v reason=%q cerr=%v (the claim itself must compile)", prog != nil, reason, cerr)
	}
	if !strings.Contains(prog.Disassemble(), "CALL_DYN_METHOD") {
		t.Fatalf("claim violation: expected a CALL_DYN_METHOD, got:\n%s", prog.Disassemble())
	}

	// RunCompiled: the claim fails at run time (1 result vs 0 claimed) →
	// internal_error → silent interpreter re-run with the CORRECT result.
	a2 := zzShapedInstance(t)
	gotC, compiled, errC := a2.RunCompiled(src)
	a3 := zzShapedInstance(t)
	gotI, errI := a3.RunInterp(src)
	if compiled {
		t.Errorf("claim violation: ran compiled; want the interpreter fallback")
	}
	if errC != nil || errI != nil {
		t.Fatalf("claim violation: errs compiled=%v interp=%v", errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(gotI) != "[7 42]" {
		t.Errorf("claim violation: fallback=%v interp=%v want [7 42]", gotC, gotI)
	}

	// Force mode surfaces the deferral loudly instead of masking it.
	a4 := zzShapedInstance(t)
	if _, err := a4.RunCompiledStrict(src); codeOf(err) != "internal_error" {
		t.Errorf("claim violation: force-compile err=[%s] %v; want internal_error", codeOf(err), err)
	}
}

// Observation-free (the TestStoreShapeObservationFree discipline): a compile
// pass over shaped-method rows models the dispatches abstractly — nothing may
// reach the RUNTIME sink registry (the check-mode shape instances write
// nowhere; the model records, never runs, the method).
func TestShapedMethodCompilePassObservationFree(t *testing.T) {
	a := mustNew(t)
	src := `import "boru:log" ; Log.add-sink memory/q ; Log.remove-sink console/q ; def c (Log.counter "n") ; c.add 1 ; Log.measurements size`
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("observation-free: prog=%v reason=%q cerr=%v", prog != nil, reason, cerr)
	}
	got, err := a.RunInterp(`import "boru:log" ; Log.measurements size`)
	if err != nil || fmt.Sprint(got) != "[0]" {
		t.Errorf("compile pass leaked into the sink registry: got %v err %v", got, err)
	}
}

// A truthful registered shape compiles AND runs compiled — the same
// construction with an honest claim is the model's positive twin, so the
// violation test above fails for the right reason (the lie, not the shape).
func TestShapedMethodRegisteredShapeCompiles(t *testing.T) {
	a := mustNew(t)
	subReg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatalf("sub-registry: %v", err)
	}
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "zz-h",
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TInteger},
			Returns:    []*native.Type{native.TInteger},
			BarrierPos: -1,
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, _ := args[0].AsConcreteInteger()
				return []native.Value{native.NewInteger(n * 2)}, nil
			}),
		}},
	})
	wrapper := native.NewFunction(native.FnDefInfo{
		Name: "zz-h",
		Signatures: []native.FnSig{{
			Params:     []native.FnParam{{Type: native.TInteger}},
			Returns:    []*native.Type{native.TInteger},
			Impl:       native.Boru([]native.Value{native.NewWord("zz-h")}),
			BarrierPos: -1,
		}},
		Registry: subReg,
	})
	inst := func() native.Value {
		m := native.NewOrderedMap()
		m.Set("h", wrapper)
		return native.NewMap(m)
	}
	a.Register("zz-hinst", native.Signature{
		Args:       []*native.Type{},
		Returns:    []*native.Type{native.TMap},
		BarrierPos: -1,
		ReturnsFn: func(_ []native.Value, _ *native.Registry) []native.Value {
			return []native.Value{inst()}
		},
		Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
			return []native.Value{inst()}, nil
		}),
	})
	src := `def i (zz-hinst) ; i.h 5 ; 42`
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil || prog == nil {
		t.Fatalf("honest shape: prog=%v reason=%q cerr=%v", prog != nil, reason, cerr)
	}
	if strings.Contains(prog.Disassemble(), "FALLBACK") {
		t.Errorf("honest shape: expected native, got island:\n%s", prog.Disassemble())
	}
	got, compiled, err := a.RunCompiled(src)
	if !compiled || err != nil {
		t.Fatalf("honest shape: compiled=%v err=%v", compiled, err)
	}
	if fmt.Sprint(got) != "[10 42]" {
		t.Errorf("honest shape: got %v want [10 42]", got)
	}
}
