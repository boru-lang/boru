package test

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// Language-level coverage for the BEAM-style process words
// (design/PROCESSES.0.md phase 1) and the in-process service model
// (design/SERVICES.0.md phase 1). Every positive behaviour is paired
// with the negative contract it implies (no_match, not_sendable,
// unknown names, bad handler shapes) per lang/go/CLAUDE.md.

func oneResult(t *testing.T, vals []native.Value) native.Value {
	t.Helper()
	if len(vals) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(vals), vals)
	}
	return vals[0]
}

func wantString(t *testing.T, vals []native.Value, want string) {
	t.Helper()
	got, err := core.AsString(oneResult(t, vals))
	if err != nil {
		t.Fatalf("expected String, got %v: %v", vals, err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func wantInt(t *testing.T, vals []native.Value, want int64) {
	t.Helper()
	got, err := core.AsInteger(oneResult(t, vals))
	if err != nil {
		t.Fatalf("expected Integer, got %v: %v", vals, err)
	}
	if got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

// ---- processes ----

// The PROCESSES.0.md §10 worked example: spawn → register → tagged-map
// messages → pattern-matched dispatch → typed binding slots → reply
// correlation, with `after` deadlines guarding every blocking receive.
func TestProcessCounterWorkedExample(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def counter-loop fn [[n:Integer] [Any] [
		   receive [
		     {cmd: "inc"}            [ counter-loop (add 1 n) ]
		     {cmd: "get" reply: Pid} [ send {tag: "count" count: n} reply  counter-loop n ]
		     {cmd: "stop"}           [ 0 ]
		     after 8000 [ 0 ]
		   ]
		 ]]`,
		`def pid (spawn [ counter-loop 0 ])`,
		`register "counter" pid`,
		`send {cmd: "inc"} "counter"`,
		`send {cmd: "inc"} "counter"`,
		`send {cmd: "get" reply: self} "counter"`,
		`def got (receive [ {tag: "count" count: Integer} [ count ] after 5000 [ -1 ] ])`,
		`send {cmd: "stop"} "counter"`,
		`got`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantInt(t, out, 2)
}

func TestProcessReceiveAfterFiresOnEmptyMailbox(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`receive [ {never: 1} [ "msg" ] after 50 [ "timed-out" ] ]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantString(t, out, "timed-out")
}

func TestProcessReceiveCatchAll(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`send {weird: "shape"} (self)`,
		`receive [ {cmd: "x"} [ "cmd" ] {} [ "caught" ] after 3000 [ "timeout" ] ]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantString(t, out, "caught")
}

func TestProcessReceiveNoMatchRaises(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`send {a: 1} (self)`,
		`receive [ {b: 2} [ 0 ] ]`,
	})
	if err == nil || !strings.Contains(err.Error(), "no_match") {
		t.Fatalf("unmatched message must raise no_match, got %v", err)
	}
}

func TestProcessReceiveBindingTypeMismatchRaises(t *testing.T) {
	// Routed by tag, but the binding slot requires reply: Pid and the
	// message carries an Integer — with no catch-all this is no_match.
	_, err := runNativeSteps(t, nil, []string{
		`send {cmd: "get" reply: 42} (self)`,
		`receive [ {cmd: "get" reply: Pid} [ 0 ] ]`,
	})
	if err == nil || !strings.Contains(err.Error(), "no_match") {
		t.Fatalf("binding type mismatch must raise no_match, got %v", err)
	}
}

func TestProcessReceiveUnknownTypeInPatternRaises(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`receive [ {reply: NoSuchType} [ 0 ] after 10 [ 1 ] ]`,
	})
	if err == nil || !strings.Contains(err.Error(), "receive_error") {
		t.Fatalf("unknown slot type must raise receive_error, got %v", err)
	}
}

func TestProcessSendMutableIsNotSendable(t *testing.T) {
	// `context` is a Store — stateful containers are declined at the
	// process boundary (PROCESSES.0.md §6).
	_, err := runNativeSteps(t, nil, []string{
		`send {payload: context} (self)`,
	})
	if err == nil || !strings.Contains(err.Error(), "not_sendable") {
		t.Fatalf("sending a Store must raise not_sendable, got %v", err)
	}
}

func TestProcessSendToUnknownNameIsDropped(t *testing.T) {
	// Fire-and-forget: sends to unknown names vanish silently.
	out, err := runNativeSteps(t, nil, []string{
		`send {a: 1} "nobody-home"`,
		`"still-alive"`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantString(t, out, "still-alive")
}

func TestProcessWhereisMissReturnsNone(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`(whereis "ghost") eq None`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	b, bErr := core.AsBoolean(oneResult(t, out))
	if bErr != nil || !b {
		t.Errorf("whereis miss must be None, got %v", out)
	}
}

func TestProcessRegisterUnregisterRoundTrip(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def pid (spawn [ receive [ {stop: 1} [ 0 ] after 5000 [ 0 ] ] ])`,
		`register "w" pid`,
		`def before ((whereis "w") eq None)`,
		`unregister "w"`,
		`def after2 ((whereis "w") eq None)`,
		`send {stop: 1} pid`,
		`[before after2]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := oneResult(t, out).String()
	if !strings.Contains(got, "false") || !strings.Contains(got, "true") {
		t.Errorf("register/unregister round trip = %s, want [false true]", got)
	}
}

func TestProcessSpawnBadOptsRejected(t *testing.T) {
	for _, src := range []string{
		`spawn [ 0 ] {mailbox: -1}`,
		`spawn [ 0 ] {overflow: "explode"}`,
	} {
		_, err := runNativeSteps(t, nil, []string{src})
		if err == nil || !strings.Contains(err.Error(), "spawn_error") {
			t.Errorf("%s: want spawn_error, got %v", src, err)
		}
	}
}

func TestProcessSelfIsAPid(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{`(typeof (self)) eq Pid`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	b, bErr := core.AsBoolean(oneResult(t, out))
	if bErr != nil || !b {
		t.Errorf("typeof self must be Pid, got %v", out)
	}
}

// ---- services ----

func TestServiceCounter(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def svc (service {count: 10})`,
		`add {op:"inc"} ([req:Map state:Any] => [ state set count (add 1 state.count) drop state.count ]) svc`,
		`add {op:"get"} ([req:Map state:Any] => [ state.count ]) svc`,
		`call {op:"inc"} svc drop`,
		`call {op:"inc"} svc drop`,
		`call {op:"get"} svc`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantInt(t, out, 12)
}

func TestServiceNoMatchRaises(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def svc (service {})`,
		`add {op:"known"} ([req:Map state:Any] => [ 1 ]) svc`,
		`call {op:"unknown"} svc`,
	})
	if err == nil || !strings.Contains(err.Error(), "no_match") {
		t.Fatalf("unmatched call must raise no_match, got %v", err)
	}
}

func TestServiceCatchAllHandler(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def svc (service {})`,
		`add {} ([req:Map state:Any] => [ "fallback" ]) svc`,
		`add {op:"hi"} ([req:Map state:Any] => [ "specific" ]) svc`,
		`[ (call {op:"hi"} svc) (call {op:"other"} svc) ]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := oneResult(t, out).String()
	if !strings.Contains(got, "specific") || !strings.Contains(got, "fallback") {
		t.Errorf("catch-all dispatch = %s", got)
	}
}

// Layering: a second `add` for the same pattern receives `prior` and
// decorates the shadowed base handler (newest outermost).
func TestServicePriorLayering(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def svc (service {})`,
		`add {op:"greet"} ([req:Map state:Any] => [ "hello" ]) svc`,
		`add {op:"greet"} ([req:Map state:Any prior:Any] => [ join "" ["<" (prior req) ">"] ]) svc`,
		`call {op:"greet"} svc`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantString(t, out, "<hello>")
}

// wrap decorates whatever pattern matched; prior at one signature does not.
func TestServiceWrapDecoratesEveryPattern(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def svc (service {hits: 0})`,
		`add {op:"a"} ([req:Map state:Any] => [ "A" ]) svc`,
		`add {op:"b"} ([req:Map state:Any] => [ "B" ]) svc`,
		`wrap ([req:Map state:Any prior:Any] => [ state set hits (add 1 state.hits) drop (prior req) ]) svc`,
		`call {op:"a"} svc drop`,
		`call {op:"b"} svc drop`,
		`(state-of svc).hits`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantInt(t, out, 2)
}

// A wrap may short-circuit (never call prior) or pre-process the request.
func TestServiceWrapShortCircuitAndRewrite(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def svc (service {})`,
		`add {op:"echo"} ([req:Map state:Any] => [ req.text ]) svc`,
		`add {op:"blocked"} ([req:Map state:Any] => [ "should-not-run" ]) svc`,
		`wrap ([req:Map state:Any prior:Any] => [
		   if (req.op eq "blocked") [ "denied" ] [ prior req ]
		 ]) svc`,
		`[ (call {op:"echo" text:"hi"} svc) (call {op:"blocked"} svc) ]`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := oneResult(t, out).String()
	if !strings.Contains(got, "hi") || !strings.Contains(got, "denied") {
		t.Errorf("wrap short-circuit = %s", got)
	}
}

func TestServiceSendDiscardsReply(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def svc (service {n: 0})`,
		`add {op:"bump"} ([req:Map state:Any] => [ state set n (add 1 state.n) drop state.n ]) svc`,
		`send {op:"bump"} svc`,
		`(state-of svc).n`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantInt(t, out, 1)
}

func TestServiceAddRejectsNonFunction(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def svc (service {})`,
		`add {op:"x"} "not-a-fn" svc`,
	})
	if err == nil || !strings.Contains(err.Error(), "add_error") {
		t.Fatalf("add with a non-function handler must raise add_error, got %v", err)
	}
}

func TestServiceWrapRejectsNonFunction(t *testing.T) {
	_, err := runNativeSteps(t, nil, []string{
		`def svc (service {})`,
		`wrap 42 svc`,
	})
	if err == nil || !strings.Contains(err.Error(), "wrap_error") {
		t.Fatalf("wrap with a non-function must raise wrap_error, got %v", err)
	}
}

// A service is safe to share across processes: dispatch is serialized,
// so concurrent bumps from actors never lose updates.
func TestServiceSharedAcrossProcesses(t *testing.T) {
	out, err := runNativeSteps(t, nil, []string{
		`def svc (service {n: 0})`,
		`add {op:"bump"} ([req:Map state:Any] => [ state set n (add 1 state.n) drop None ]) svc`,
		`add {op:"read"} ([req:Map state:Any] => [ state.n ]) svc`,
		// worker returns nothing: `send` leaves no value, and a named call's
		// declared count is the frame's contract on every path (NUR191).
		`def worker fn [[main:Pid] [] [
		   send {op:"bump"} svc
		   send {op:"bump"} svc
		   send {done: 1} main
		 ]]`,
		`def me (self)`,
		`spawn [ worker me ] drop`,
		`spawn [ worker me ] drop`,
		`receive [ {done: 1} [ 1 ] after 5000 [ -1 ] ] drop`,
		`receive [ {done: 1} [ 1 ] after 5000 [ -1 ] ] drop`,
		`call {op:"read"} svc`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	wantInt(t, out, 4)
}
