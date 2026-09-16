package lang

import (
	"fmt"
	"strings"
	"testing"
)

// A stored service handler reads its module-scope dependencies LIVE (the
// seventy-first increment — the stored-handler latch's lookup half): a
// bare read of a data def is seated at its token (OpLookupDynScope), a
// word slot routes with its dispatch, and a fn with declared signatures
// dispatched by name routes with a live lead — the op resolves the lead in
// the registry at the call and runs the live signature's own unit. So a
// rebind of the dep between the store and the call, which the latch
// refused as a whole-program hammer, compiles and answers the interpreter's
// call-time binding. What still refuses: a LAMBDA helper as the original
// (no declaration site to locate a unit by — the latch's own text), and a
// rebind of a live lead TO a lambda or a data value (the routed op could
// not run it — refused through the undef site).
func TestStoredHandlerReadsLiveBinding(t *testing.T) {
	t.Setenv("BORU_COMPILE_FALLBACK", "1")
	const named = "def helper fn [[x:Integer][Integer][x add 1]] end def svc (service {}) add {} ([r:Map state:Any] => [helper 5]) svc "
	compiled := []struct{ src, want string }{
		// A named helper rebound after the store: the routed lead runs the
		// live signature's unit.
		{named + "def helper fn [[x:Integer][Integer][x add 2]] end call {} svc", "[7]"},
		{named + "undef helper def helper fn [[x:Integer][Integer][x add 3]] end call {} svc", "[8]"},
		// Never rebound: the stable shape of every real handler.
		{named + "call {} svc", "[6]"},
		// A data dep read bare, and in a slot.
		{"def k 6 def svc (service {}) add {} ([r:Map state:Any] => [k]) svc def k 11 call {} svc", "[11]"},
		{"def k 6 def svc (service {}) add {} ([r:Map state:Any] => [k add 1]) svc def k 11 call {} svc", "[12]"},
		{"def k 6 def svc (service {}) add {} ([r:Map state:Any] => [k]) svc undef k def k 11 call {} svc", "[11]"},
	}
	for _, c := range compiled {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil || prog == nil {
			t.Errorf("%q: must compile, got reason=%q err=%v", c.src, reason, cerr)
			continue
		}
		if got := prog.StoredRefStampedCount(); got != 1 {
			t.Errorf("%q: the handler's ref is stamped (live reads leave no bake to poison): %d", c.src, got)
		}
		// The lead routes (`helper 5`); the read of k is a live lookup at
		// its token — bare, or the stack operand `k add 1` pushes before its
		// committed native call.
		dis := prog.Disassemble()
		if strings.Contains(c.src, "[helper 5]") != strings.Contains(dis, "DISPATCH_GENERIC") || strings.Contains(c.src, "=> [k") != strings.Contains(dis, "LOOKUP_DYN_SCOPE") {
			t.Errorf("%q: the lead routes, the read of k is a live lookup:\n%s", c.src, dis)
		}
		gotC, compiledRun, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
		if !compiledRun {
			t.Errorf("%q: the compiled run stays compiled", c.src)
		}
		if got := fmt.Sprint(gotC); got != c.want {
			t.Errorf("%q: got %s, want %s", c.src, got, c.want)
		}
	}
	refused := []struct{ src, reason string }{
		{"def helper ([x:Integer] => [x add 1]) def svc (service {}) add {} ([r:Map state:Any] => [helper 5]) svc def helper ([x:Integer] => [x add 2]) call {} svc", "rebound after a stored handler captured it as a dep"},
		{named + "def helper ([x:Integer] => [x add 2]) call {} svc", "module binding helper rebound to a value with no declared signature after a stored handler dispatched it live"},
		{named + "def helper 9 call {} svc", "module binding helper rebound to a value with no declared signature after a stored handler dispatched it live"},
	}
	for _, c := range refused {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: %v", c.src, cerr)
		}
		if prog != nil || !strings.Contains(reason, c.reason) {
			t.Errorf("%q: want the refusal %q…, got compiled=%v reason=%q", c.src, c.reason, prog != nil, reason)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}
