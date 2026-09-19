package lang

import (
	"fmt"
	"strings"
	"testing"
)

// TestCompiledStoredHandlerFreezeRedefine pins the discipline for PR #243 comment #2,
// REVISED by design/RELOAD-INVALIDATION.0.md §3 F1, and narrowed by the
// seventy-first increment. A compiled stored service handler FROZE its
// module-level dependencies at the `add` (registration) source point, while
// the interpreter resolves those same names at CALL time. The original fix
// POISONED the ref (NotifyNameRebound → CallBoru fallback) and kept the rest
// of the program compiled — which is sound only for calls sequenced AFTER
// the last rebind: module-scope def sites execute in the compile pass
// (RunInCheck), so the fallback's "live" def table holds the PASS-FINAL
// binding for every call, including calls BEFORE the rebind in program
// order (the F1 miscompile, pinned by
// TestStoredHandlerMidProgramRebindCompilesAndMatches below). The revised
// discipline: a module-scope rebind of a name an already-created stored ref
// BAKED refuses the whole program — interpreter fallback, correct values —
// exactly like the frozen-module-read hammer. Since the seventy-first
// increment the unit bakes less: a module-scope value it reads bare is
// seated live (OpLookupDynScope), a word slot routes, and a fn with
// DECLARED signatures dispatched by name routes with a live lead — so the
// data case compiles and answers the live binding, while a LAMBDA helper
// (no declaration site for the routed op to locate a unit by) keeps the
// latch.
func TestCompiledStoredHandlerFreezeRedefine(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	cases := []struct {
		name, src, want string
		compiles        bool
	}{
		// A USER FN dep is undef'd then redefined between add and call.
		// Live helper=x+2 at the call → 7. A lambda helper: the latch.
		{"user fn undef+redef",
			`def helper ([x:Integer] => [x add 1])
def svc (service {})
add {} ([r:Map state:Any] => [helper 5]) svc
undef helper
def helper ([x:Integer] => [x add 2])
call {} svc`, "[7]", false},
		// A bare redefinition (no undef) of a user fn dep.
		{"user fn bare redef",
			`def helper ([x:Integer] => [x add 1])
def svc (service {})
add {} ([r:Map state:Any] => [helper 5]) svc
def helper ([x:Integer] => [x add 2])
call {} svc`, "[7]", false},
		// A DATA def dep is undef'd then redefined between add and call:
		// the read is live, the program compiles (the seventy-first
		// increment).
		{"data def undef+redef",
			`def k 6
def svc (service {})
add {} ([r:Map state:Any] => [k]) svc
undef k
def k 11
call {} svc`, "[11]", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, _ := New()
			prog, reason, _, err := a.CompileCheck(c.src)
			if err != nil {
				t.Fatalf("CompileCheck error: %v", err)
			}
			if (prog != nil) != c.compiles {
				t.Fatalf("compiled=%v, want %v (reason %q)", prog != nil, c.compiles, reason)
			}
			if !c.compiles && !strings.Contains(reason, "rebound after a stored handler") {
				t.Errorf("refusal reason = %q, want the stored-handler rebind hammer", reason)
			}
			gotC, compiled, errC, gotI, errI := runBothEngines(t, c.src)
			if noteCompileDefect(t, c.src, gotC, errC) {
				if c.compiles {
					t.Errorf("this row is meant to compile and run")
				}
				return
			}
			if compiled != c.compiles {
				t.Errorf("compiled run = %v, want %v", compiled, c.compiles)
			}
			if errC != nil || errI != nil {
				t.Fatalf("run errors: compiled=%v interp=%v", errC, errI)
			}
			if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
				t.Errorf("compiled %v != interpreter %v (MISCOMPILE)", gotC, gotI)
			}
			if fmt.Sprint(gotC) != c.want {
				t.Errorf("got %v, want %s", gotC, c.want)
			}
		})
	}
}

// TestStoredHandlerMidProgramRebindCompilesAndMatches is the F1 pin
// (design/RELOAD-INVALIDATION.0.md §3): calls BEFORE a rebind must see the
// point-in-program binding. Under the pre-revision per-ref poisoning this
// program compiled and printed the pass-final value for every call
// (12 12 12); the interpreter's documented call-time-binding semantics give
// 6, then 105, then 12. The latch refused it until the seventy-first
// increment; now the handler's `bonus add 5` routes its slot live, the
// program compiles, and the twin regime replays each `def bonus` in
// program order at VM time. The compiled RUN still falls back to the
// interpreter, at the SECOND `call`: a pre-existing bail of the poly
// native seat (`vm:poly-no-match` in the defer census — the check pass
// commits `call` to its three-operand overload over the first call's
// gradual residual, and the run refutes the count), measured on main with
// no dep read at all (`def svc (service {}) add {} ([r:Map state:Any] =>
// [1]) svc call {} svc call {} svc`). The fallback matches the interpreter
// exactly; the bail is that seat's to retire, and this pin flips knowingly
// when it does.
func TestStoredHandlerMidProgramRebindCompilesAndMatches(t *testing.T) {
	// Legacy refusal+fallback-parity contract (see note above).
	src := `def bonus 1
def svc (service {})
add {op:"go"} ([req:Map state:Any] => [bonus add 5]) svc
call {op:"go"} svc
def bonus 100
call {op:"go"} svc
def bonus 7
call {op:"go"} svc`
	a, _ := New()
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck error: %v", err)
	}
	if prog == nil {
		t.Fatalf("a mid-program rebind of a dep the handler reads live compiles; refused: %q", reason)
	}
	gotC, _, errC, gotI, errI := runBothEngines(t, src)
	// The second `call` bails at the poly native seat (vm:poly-no-match). It
	// used to be resolved by re-running the source; it is booked as the
	// defect it is, and the meaning of the program is pinned below on the
	// reference engine.
	if noteCompileDefect(t, src, gotC, errC) {
		if errI != nil {
			t.Fatalf("interpreted: %v", errI)
		}
		if fmt.Sprint(gotI) != "[6 105 12]" {
			t.Errorf("interpreted = %v, want [6 105 12]", gotI)
		}
		return
	}
	if errC != nil || errI != nil {
		t.Fatalf("run errors: compiled=%v interp=%v", errC, errI)
	}
	if fmt.Sprint(gotC) != fmt.Sprint(gotI) {
		t.Errorf("compiled %v != interpreter %v (the F1 MISCOMPILE)", gotC, gotI)
	}
	if want := "[6 105 12]"; fmt.Sprint(gotI) != want {
		t.Errorf("interpreter baseline drifted: got %v, want %s", gotI, want)
	}
}

// TestCompiledStoredHandlerStableDepCompiles is the POSITIVE guard: a stored handler over
// a module dependency that is NEVER redefined (the shape of every real boru:net
// app handler — todo-api's live-todos, mini-redis's arg-at/kv-read) MUST still
// compile its unit and be stamped. Proves the hammer is PRECISE — keyed on
// actual redefinition, not "reads a module ref" — so the apps keep their
// compiled speedup. compile == interpret, and the ref IS stamped
// (StoredRefStampedCount 1).
func TestCompiledStoredHandlerStableDepCompiles(t *testing.T) {
	src := `def helper ([x:Integer] => [x add 1])
def svc (service {})
add {} ([r:Map state:Any] => [helper 5]) svc
call {} svc`
	a, _ := New()
	prog, reason, _, err := a.CompileCheck(src)
	if err != nil {
		t.Fatalf("CompileCheck error: %v", err)
	}
	if prog == nil {
		t.Fatalf("must compile, refused: %q", reason)
	}
	if got := prog.StoredRefCount(); got != 1 {
		t.Fatalf("StoredRefCount = %d, want 1", got)
	}
	if got := prog.StoredRefStampedCount(); got != 1 {
		t.Errorf("StoredRefStampedCount = %d, want 1 (stable dep → compiled unit)", got)
	}
	got, err := a.RunCompiledStrict(src)
	if err != nil {
		t.Fatalf("RunCompiledStrict: %v", err)
	}
	b, _ := New()
	want, _ := b.RunInterp(src)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("compiled %v != interpreter %v", got, want)
	}
	if fmt.Sprint(got) != "[6]" {
		t.Errorf("got %v, want [6] (frozen == live == helper 5 = 6)", got)
	}
}
