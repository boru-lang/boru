package native

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// Store-site stamping (Phase 2 of design/legacy/RUNTIME-STAMPING.0.ignore): `add` and
// `wrap` detached-stamp an eligible handler when runtime stamping is armed,
// so a service built from an INTERPRETED context (the RunCompiled fallback,
// a module fn's body the compile pass could not stamp) still dispatches its
// handlers on the VM. Positive + negative per lang/go/CLAUDE.md.

func stampSvcReg(t *testing.T, armed bool) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if armed {
		r.EnableRuntimeStamping()
	}
	return r
}

// storedHandlerRefs returns the CompiledFnRef (or nil) of every handler
// stored on the service bound to name, in stack order.
func storedHandlerRefs(t *testing.T, r *Registry, name string) []*compiler.CompiledFnRef {
	t.Helper()
	v, ok := r.Defs.Top(name)
	if !ok {
		t.Fatalf("service %q not bound", name)
	}
	s, isSvc := asService(v)
	if !isSvc {
		t.Fatalf("%q is not a service", name)
	}
	var refs []*compiler.CompiledFnRef
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sig := range s.pm.order {
		for _, h := range s.stacks[sig] {
			fd, isFn := FnDefFromValue(h)
			if !isFn {
				t.Fatalf("stored handler is not a fn")
			}
			var ref *compiler.CompiledFnRef
			for i := range fd.Signatures {
				if r := compiler.CompiledRef(&fd.Signatures[i]); r != nil {
					ref = r
				}
			}
			refs = append(refs, ref)
		}
	}
	return refs
}

// Armed: `add` stores a stamped clone (finalized Prog) and the service
// answers identically to the unarmed (interpreted) build. Unarmed: the
// stored handler stays plain — the -no-compile contract.
func TestServiceAddStampsHandlerWhenArmed(t *testing.T) {
	const src = `
def svc (service {kv: {}})
add {cmd:"GET"} ([req:Map state:Any] => [ def kv2 state.kv  kv2 get "a" ]) svc
(state-of svc).kv set "a" 42;
`
	probe := `(call {cmd:"GET"} svc)`

	armed := stampSvcReg(t, true)
	if _, err := seam5Run(armed, src); err != nil {
		t.Fatalf("armed setup: %v", err)
	}
	refs := storedHandlerRefs(t, armed, "svc")
	if len(refs) != 1 || refs[0] == nil || refs[0].Prog == nil {
		t.Fatalf("armed add must store a stamped handler, got %v", refs)
	}
	outA, err := seam5Run(armed, probe)
	if err != nil {
		t.Fatalf("armed call: %v", err)
	}

	plain := stampSvcReg(t, false)
	if _, err := seam5Run(plain, src); err != nil {
		t.Fatalf("unarmed setup: %v", err)
	}
	refsP := storedHandlerRefs(t, plain, "svc")
	if len(refsP) != 1 || refsP[0] != nil {
		t.Fatalf("unarmed add must store a plain handler, got %v", refsP)
	}
	outP, err := seam5Run(plain, probe)
	if err != nil {
		t.Fatalf("unarmed call: %v", err)
	}

	nA, _ := core.AsInteger(outA[len(outA)-1])
	nP, _ := core.AsInteger(outP[len(outP)-1])
	if nA != nP || nA != 42 {
		t.Fatalf("stamped=%d interpreted=%d, want both 42", nA, nP)
	}
}

// A CAPTURING handler (a lambda closing over an enclosing-fn local) STAMPS
// post the capture-slot landing (plan Phase 6.3): the ref carries the
// captured value and the VM run answers with it — dispatch identical.
func TestServiceAddStampsCapturingHandler(t *testing.T) {
	const src = `
def mk (fn [[n:Integer] [Any] [
  def svc (service {})
  add {cmd:"N"} ([req:Map state:Any] => [ n ]) svc
  svc
]])
def svc (mk 7)
`
	r := stampSvcReg(t, true)
	if _, err := seam5Run(r, src); err != nil {
		t.Fatalf("setup: %v", err)
	}
	refs := storedHandlerRefs(t, r, "svc")
	if len(refs) != 1 || refs[0] == nil {
		t.Fatalf("capturing handler must stamp (capture-slot unit), got %v", refs)
	}
	out, err := seam5Run(r, `(call {cmd:"N"} svc)`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if n, _ := core.AsInteger(out[len(out)-1]); n != 7 {
		t.Fatalf("captured handler answered %d, want 7", n)
	}
}

// `wrap` stamps through the same seam; a wrap that calls `prior` keeps the
// chain semantics whether or not the base handler is stamped.
func TestServiceWrapStampsWhenArmed(t *testing.T) {
	const src = `
def svc (service {})
add {cmd:"X"} ([req:Map state:Any] => [ 10 ]) svc
wrap ([req:Map state:Any prior:Any] => [ add 1 (prior req) ]) svc
`
	r := stampSvcReg(t, true)
	if _, err := seam5Run(r, src); err != nil {
		t.Fatalf("setup: %v", err)
	}
	out, err := seam5Run(r, `(call {cmd:"X"} svc)`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if n, _ := core.AsInteger(out[len(out)-1]); n != 11 {
		t.Fatalf("wrapped chain answered %d, want 11", n)
	}
}

// The mini-redis CATCH-ALL shape — a handler whose whole body is a COMPUTED
// MAP literal (`{message: (join …)}`, a field computed from the request) —
// stamps at the store site. The map is the body's TRAILING residual, so the
// stored-fn unit must record its OpMakeMap assembly instead of refusing "body
// result of unknown provenance". runFnBodyOnce enables that recording only
// for CALLBACK bodies (isCallbackBodyName), where both engines evaluate the
// residual in the live frame via InvokeCallback / CallBoru — so the recorded
// assembly matches the interpreter exactly. Armed stamps; unarmed stays plain
// and answers identically (the -no-compile contract).
func TestServiceAddStampsComputedMapHandler(t *testing.T) {
	const src = `
def svc (service {})
add {} ([req:Map state:Any] => [ {message: (join "" ["unknown '" req.cmd "'"])} ]) svc
`
	probe := `((call {cmd:"BOGUS"} svc) get "message")`

	armed := stampSvcReg(t, true)
	if _, err := seam5Run(armed, src); err != nil {
		t.Fatalf("armed setup: %v", err)
	}
	refs := storedHandlerRefs(t, armed, "svc")
	if len(refs) != 1 || refs[0] == nil || refs[0].Prog == nil {
		t.Fatalf("the computed-map catch-all handler must stamp, got %v", refs)
	}
	outA, err := seam5Run(armed, probe)
	if err != nil {
		t.Fatalf("armed call: %v", err)
	}

	plain := stampSvcReg(t, false)
	if _, err := seam5Run(plain, src); err != nil {
		t.Fatalf("unarmed setup: %v", err)
	}
	refsP := storedHandlerRefs(t, plain, "svc")
	if len(refsP) != 1 || refsP[0] != nil {
		t.Fatalf("unarmed add must store a plain handler, got %v", refsP)
	}
	outP, err := seam5Run(plain, probe)
	if err != nil {
		t.Fatalf("unarmed call: %v", err)
	}

	sA, _ := core.AsString(outA[len(outA)-1])
	sP, _ := core.AsString(outP[len(outP)-1])
	if sA != sP || sA != "unknown 'BOGUS'" {
		t.Fatalf("stamped=%q interpreted=%q, want both \"unknown 'BOGUS'\"", sA, sP)
	}
}

// The mini-redis KEYS shape — a handler whose body runs a CAPTURING filter
// lambda over a COMPUTED collection — stamps at the store site (Phase 3
// closed both closure gates: lexical captures on the body lambda, typed
// carrier data operands).
func TestServiceAddStampsFilterLambdaHandler(t *testing.T) {
	const src = `
def svc (service {kv: {}})
add {cmd:"KEYS"} ([req:Map state:Any] => [
  def kv state.kv
  filter ([e:Any] => [ ((kv get e.value) eq None) not ]) (keys kv)
]) svc
(state-of svc).kv set "a" 1;
`
	r := stampSvcReg(t, true)
	if _, err := seam5Run(r, src); err != nil {
		t.Fatalf("setup: %v", err)
	}
	refs := storedHandlerRefs(t, r, "svc")
	if len(refs) != 1 || refs[0] == nil || refs[0].Prog == nil {
		t.Fatalf("the KEYS-shape handler must stamp, got %v", refs)
	}
	out, err := seam5Run(r, `(call {cmd:"KEYS"} svc)`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := out[len(out)-1].String()
	if got != `['a']` && got != `[a]` {
		t.Fatalf("KEYS reply = %s, want the ['a'] key list", got)
	}
}
