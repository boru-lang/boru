package lang

import (
	"fmt"
	"strings"
	"testing"

	eng "github.com/boru-lang/boru/eng/go"
)

// A stored service handler reads its module-scope dependencies LIVE (the
// seventy-first increment — the stored-handler latch's lookup half): a
// bare read of a data def is seated at its token (OpLookupDynScope), a
// word slot routes with its dispatch, and a fn with declared signatures
// dispatched by name routes with a live lead — the op resolves the lead in
// the registry at the call and runs the live signature's own unit. So a
// rebind of the dep between the store and the call, which the latch
// declined as a whole-program hammer, compiles and answers the interpreter's
// call-time binding. What still declines: a LAMBDA helper as the original
// (no declaration site to locate a unit by — the latch's own text), and a
// rebind of a live lead TO a lambda or a data value (the routed op could
// not run it — declined through the undef site).
func TestStoredHandlerReadsLiveBinding(t *testing.T) {
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
		// A live lead rebound to another arity: the live plan claims what
		// the live signature takes, as the interpreter's dispatch does.
		{named + "def helper fn [[x:Integer y:Integer][Integer][x add y]] end call {} svc", "[6]"},
		// The admission rides on the stored unit's descriptor, not on the
		// word (review of #467): a body-local `helper` in another fn keeps
		// its committed call and answers its own body.
		{named + "def f fn [[x:Integer][Integer][def helper fn [[y:Integer][Integer][y add 10]] end helper x]] end f 5", "[15]"},
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
		// The lead routes (`helper 5`) — once, for the stored unit's own
		// dispatch; the read of k is a live lookup at its token — bare, or
		// the stack operand `k add 1` pushes before its committed native
		// call.
		dis := prog.Disassemble()
		if strings.Contains(c.src, "[helper 5]") != (strings.Count(dis, "DISPATCH_GENERIC") == 1) || strings.Contains(c.src, "=> [k") != strings.Contains(dis, "LOOKUP_DYN_SCOPE") {
			t.Errorf("%q: the lead routes once, the read of k is a live lookup:\n%s", c.src, dis)
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
	declined := []struct{ src, reason string }{
		{"def helper ([x:Integer] => [x add 1]) def svc (service {}) add {} ([r:Map state:Any] => [helper 5]) svc def helper ([x:Integer] => [x add 2]) call {} svc", "rebound after a stored handler captured it as a dep"},
		{named + "def helper ([x:Integer] => [x add 2]) call {} svc", "module binding helper rebound to a value with no declared signature after a stored handler dispatched it live"},
		{named + "def helper 9 call {} svc", "module binding helper rebound to a value with no declared signature after a stored handler dispatched it live"},
		// Review of #467: a name read BOTH ways — the lead routed, its value
		// baked by `/v` — is the latch's still; a live READ rebound to a fn,
		// which the lookup op would defer on past the handler's print where
		// the interpreter dispatches it, declines.
		{"def helper fn [[x:Integer][Integer][x add 1]] end def svc (service {}) add {} ([r:Map state:Any] => [helper 5 drop def h helper/v 5 h/v apply]) svc def helper fn [[x:Integer][Integer][x add 2]] end call {} svc", "rebound after a stored handler captured it as a dep"},
		{"def k 6 def svc (service {}) add {} ([r:Map state:Any] => [print \"x\" k]) svc undef k def k fn [[][Integer][11]] end call {} svc", "module binding k rebound to a dispatching value after a stored handler read it live"},
	}
	for _, c := range declined {
		a, err := New()
		if err != nil {
			t.Fatal(err)
		}
		prog, reason, _, cerr := a.CompileCheck(c.src)
		if cerr != nil {
			t.Fatalf("%q: %v", c.src, cerr)
		}
		if prog != nil || !strings.Contains(reason, c.reason) {
			t.Errorf("%q: want the compile failure %q…, got compiled=%v reason=%q", c.src, c.reason, prog != nil, reason)
		}
		gotC, _, errC, gotI, errI := runBothEngines(t, c.src)
		requireParity(t, c.src, gotC, errC, gotI, errI)
	}
}

// A program that reads a stored handler's deps live but makes no binding
// transition of its own carries LiveLeadNames and no twins; running it
// again must NOT roll the registry back to its ReplayBase (review of
// #467: the restore predicate had read the live sets, so a rebind made by
// a later request was undone by re-running the earlier program — `helper
// 5` back to 6 from 7 — and the registry left rolled back). The restore is
// the twin regime's, owed to a transition the program replays, and a live
// read implies none.
func TestStoredHandlerLiveNamesDoNotRestoreBase(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	reg := a.NativeRegistry()
	if _, err := a.RunInterp("def helper fn [[x:Integer][Integer][x add 1]] end"); err != nil {
		t.Fatal(err)
	}
	// A spawned body is a stored-ref unit: its `helper 5` is a live lead,
	// and the program makes no transition of its own.
	prog, reason, _, err := a.CompileCheck("spawn [helper 5 drop] drop  helper 5")
	if err != nil || prog == nil {
		t.Fatalf("the spawn program compiles: err=%v reason=%q", err, reason)
	}
	if len(prog.BindTwins) != 0 || !prog.LiveLeadNames["helper"] {
		t.Fatalf("no twins, a live lead: twins=%d live=%v", len(prog.BindTwins), prog.LiveLeadNames)
	}
	if out, err := eng.RunProgram(prog, reg); err != nil || fmt.Sprint(out) != "[6]" {
		t.Fatalf("first run: %v %v", out, err)
	}
	if _, err := a.RunInterp("def helper fn [[x:Integer][Integer][x add 2]] end"); err != nil {
		t.Fatal(err)
	}
	depth := reg.Defs.Depth("helper")
	if _, err := eng.RunProgram(prog, reg); err != nil {
		t.Fatalf("the re-run: %v", err)
	}
	if d := reg.Defs.Depth("helper"); d != depth {
		t.Fatalf("the re-run rolled the registry back: helper depth %d, want %d", d, depth)
	}
	if got, err := a.RunInterp("helper 5"); err != nil || fmt.Sprint(got) != "[7]" {
		t.Fatalf("the registry keeps the later request's binding after the re-run: %v %v", got, err)
	}
}
