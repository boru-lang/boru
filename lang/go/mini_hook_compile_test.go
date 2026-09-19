package lang

import (
	"fmt"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/native"
)

// zzBfHookInstance installs a contract-exercising Go compile hook on the
// built-in `bf` kind: the hook splices an uppercased src literal at
// expansion — the 2026-07-15 flip finding's fixture, rebuilt on the
// builtin-hook surface (RegisterMiniCompileGoHook) after the custom-kind
// registration APIs were removed with the frozen namespace. The hook is
// authoritative at the call site, so a compile pass must either mirror it
// or refuse.
func zzBfHookInstance(t *testing.T) *Boru {
	t.Helper()
	a := mustNew(t)
	native.RegisterMiniCompileGoHook(a.registry, "bf",
		func(src string, _ native.Value, _ *native.Registry) ([]native.Value, error) {
			return []native.Value{native.NewString(strings.ToUpper(src))}, nil
		})
	return a
}

// The Go compile hook runs IN the compile pass over a concrete src, so the
// compiled program splices the hook's expansion exactly as the interpreter
// does — the flip finding's positive twin (compiled [hi] vs interpreted [HI]
// before the fix).
func TestMiniGoHookCompilesIdentically(t *testing.T) {
	const src = `import "boru:minilang" end mini bf 'hi'`
	gotC, ran, errC := zzBfHookInstance(t).RunCompiled(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if !ran || errC != nil {
		t.Fatalf("hooked mini must run compiled: ran=%v err=%v", ran, errC)
	}
	gotI, errI := zzBfHookInstance(t).RunInterp(src)
	if errI != nil || fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Fatalf("hook parity: compiled=%v interp=%v (errI=%v)", gotC, gotI, errI)
	}
	if fmt.Sprint(gotC) != "[HI]" {
		t.Fatalf("hook result = %v, want [HI] (the compile hook ran)", gotC)
	}
}

// A hook over a NON-CONCRETE src (a fn param) refuses the same way — the
// hook cannot run at compile time, and baking the transducer instead would
// miscompile a semantics-bearing hook.
func TestMiniGoHookNonConcreteSrcRefuses(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	const src = `import "boru:minilang" end def f fn [[s:String][String][mini bf s]] f 'hi'`
	a := zzBfHookInstance(t)
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("CompileCheck: %v", cerr)
	}
	if prog != nil || !strings.Contains(reason, "concrete src") {
		t.Fatalf("prog=%v reason=%q — a non-concrete src must refuse the hook bake", prog, reason)
	}
	// Parity via the (transitional-default) fallback.
	gotC, ran, errC := zzBfHookInstance(t).RunCompiled(src)
	gotI, errI := zzBfHookInstance(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if ran || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Fatalf("fallback parity: C=%v/%v I=%v/%v ran=%v", gotC, errC, gotI, errI, ran)
	}
}

// A hook whose expansion the compile pass cannot mirror REFUSES — never the
// transducer bake. Non-concrete opts (a fn param) is the exercised shape.
func TestMiniGoHookNonConcreteOptsRefuses(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	const src = `import "boru:minilang" end def f fn [[m:Map][String][mini bf 'hi' m]] f {x:1}`
	a := zzBfHookInstance(t)
	prog, reason, _, cerr := a.CompileCheck(src)
	if cerr != nil {
		t.Fatalf("CompileCheck: %v", cerr)
	}
	if prog != nil || !strings.Contains(reason, "concrete opts") {
		t.Fatalf("prog=%v reason=%q — non-concrete opts must refuse the hook bake", prog, reason)
	}
	// Parity via the (transitional-default) fallback.
	gotC, ran, errC := zzBfHookInstance(t).RunCompiled(src)
	gotI, errI := zzBfHookInstance(t).RunInterp(src)
	if noteCompileDefect(t, src, gotC, errC) {
		return
	}
	if ran || fmt.Sprint(gotC) != fmt.Sprint(gotI) || fmt.Sprint(errC) != fmt.Sprint(errI) {
		t.Fatalf("fallback parity: C=%v/%v I=%v/%v ran=%v", gotC, errC, gotI, errI, ran)
	}
}

// (TestMiniBoruHookRefuses died with the boru compile-hook surface —
// MiniLang.register-compiled is a tombstone now; the frozen-registry raise
// is pinned in module-minilang.tsv and TestMiniCovRegisterTombstones.)
