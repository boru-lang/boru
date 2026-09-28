package native

// Coverage for lang/go/native/native_process.go (design/TEST-SEAMS.10.md
// discipline): direct handler calls with crafted args drive the error arms
// the boru surface cannot reach deterministically (runtime shutdown, full
// mailboxes, crafted payloads), and seam5Run drives the clause-parsing and
// routing arms from source. Per lang/go/CLAUDE.md every negative case is
// paired with the positive behaviour it protects.

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
)

func wantProcErr(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), sub) {
		t.Fatalf("want error containing %q, got %v", sub, err)
	}
}

// procCoverClauses builds a well-formed `receive` clause list —
// [{cmd:"x"} [1]] — for driving receiveHandler past the parse phase.
func procCoverClauses() Value {
	om := NewOrderedMap()
	om.Set("cmd", NewString("x"))
	return NewList([]Value{NewMap(om), NewList([]Value{NewInteger(1)})})
}

// ---- Pid type registration + behavior ----

func TestProcessCoverRegisterPidTypeDuplicate(t *testing.T) {
	saved := SwapTypeInitErrs(nil)
	t.Cleanup(func() { SwapTypeInitErrs(saved) })
	if tt := registerPidType(); tt != nil {
		t.Fatalf("duplicate Ideal/Pid registration must return nil, got %v", tt)
	}
	err := TypeInitError()
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected already-registered init error, got %v", err)
	}
}

func TestProcessCoverPidBehaviorFormatEqual(t *testing.T) {
	rt := core.NewProcessRuntime()
	p1 := core.NewProcess(rt, 4, core.OverflowBlock)
	p2 := core.NewProcess(rt, 4, core.OverflowBlock)
	b := pidBehavior{}

	if got := b.Format(NewPid(p1)); !strings.HasPrefix(got, "Pid<P_") {
		t.Errorf("Format(pid) = %q, want a Pid<P_…> rendering", got)
	}
	// A Pid-typed value with no process handle (the bare type literal)
	// falls back to the plain type name.
	if got := b.Format(NewTypeLiteral(TPid)); got != "Pid" {
		t.Errorf("Format(type literal) = %q, want \"Pid\"", got)
	}

	if !b.Equal(NewPid(p1), NewPid(p1)) {
		t.Error("two handles to the same process must be Equal")
	}
	if b.Equal(NewPid(p1), NewPid(p2)) {
		t.Error("handles to distinct processes must not be Equal")
	}
	if b.Equal(NewPid(p1), NewInteger(1)) {
		t.Error("pid vs non-pid must fall to the default Equal (false)")
	}
}

// ---- policy / runtime plumbing ----

func TestProcessCoverPolicyNilRegistry(t *testing.T) {
	if err := checkProcessPolicy(nil); err != nil {
		t.Fatalf("nil registry must allow spawn: %v", err)
	}
}

func TestProcessCoverProcRuntimeLazyCreate(t *testing.T) {
	r := &Registry{} // not built by core.NewRegistry: Procs starts nil
	rt := procRuntime(r)
	if rt == nil || r.Procs != rt {
		t.Fatalf("procRuntime must create and pin a runtime, got %v (Procs %v)", rt, r.Procs)
	}
	if procRuntime(r) != rt {
		t.Fatal("second procRuntime call must return the same runtime")
	}
}

func TestProcessCoverSelfLazyMainAndShutdown(t *testing.T) {
	// Positive: `self` lazily creates the implicit main process and pins it.
	r := seam5Reg(t)
	out, err := selfHandler(nil, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("self: %v / %v", out, err)
	}
	p, ok := asPid(out[0])
	if !ok || r.Proc != p {
		t.Fatalf("self must return the registry's own process, got %v", out[0])
	}
	out2, err := selfHandler(nil, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if p2, _ := asPid(out2[0]); p2 != p {
		t.Error("repeated self must return the same process")
	}

	// Negative: a shut-down runtime refuses to create the main process.
	r2 := seam5Reg(t)
	r2.Procs.Shutdown()
	_, err = selfHandler(nil, nil, nil, r2)
	wantProcErr(t, err, "shut down")
}

// ---- spawn ----

func TestProcessCoverSpawnDeniedByPolicy(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = spawnHandler([]Value{NewList([]Value{NewInteger(0)})}, nil, nil, r)
	if err == nil {
		t.Fatal("sandbox policy must deny spawn")
	}
	// The gate codes the refusal (policy_error.go), so what escapes is a
	// dispatchable BoruError rather than the internal *policy.Denied.
	var ae *BoruError
	if !errors.As(err, &ae) {
		t.Fatalf("expected a coded BoruError, got %T (%v)", err, err)
	}
}

func TestProcessCoverSpawnNonConcreteBody(t *testing.T) {
	r := seam5Reg(t)
	_, err := spawnHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r)
	wantProcErr(t, err, "expected a concrete list")
}

func TestProcessCoverSpawnValidOpts(t *testing.T) {
	// The positive twin of TestProcessSpawnBadOptsRejected: in-range
	// mailbox and overflow opts are accepted and the body runs.
	r := seam5Reg(t)
	opts := NewOrderedMap()
	opts.Set("mailbox", NewInteger(2))
	opts.Set("overflow", NewString(core.OverflowDrop))
	out, err := spawnHandler([]Value{NewList([]Value{NewInteger(0)}), NewMap(opts)}, nil, nil, r)
	if err != nil {
		t.Fatalf("spawn with valid opts: %v", err)
	}
	p, ok := asPid(out[0])
	if !ok {
		t.Fatalf("spawn must return a Pid, got %v", out)
	}
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("spawned body did not finish")
	}
}

func TestProcessCoverSpawnAfterShutdown(t *testing.T) {
	r := seam5Reg(t)
	r.Procs.Shutdown()
	_, err := spawnHandler([]Value{NewList([]Value{NewInteger(0)})}, nil, nil, r)
	wantProcErr(t, err, "shut down")
}

func TestProcessCoverSpawnCrashIsolation(t *testing.T) {
	// "Let it crash": a panic inside a spawned body terminates only that
	// process, reports on ErrOutput, and never reaches the host goroutine.
	r := seam5Reg(t)
	var buf bytes.Buffer
	r.ErrOutput = &buf
	r.RegisterNativeFunc(NativeFunc{
		Name: "cover-proc-boom",
		Signatures: []Signature{{
			Args: []*Type{},
			Impl: Go(func([]Value, map[string]Value, []Value, *Registry) ([]Value, error) {
				panic("cover-proc-boom detonated")
			}),
			Returns:    []*Type{},
			BarrierPos: -1,
		}},
	})
	out, err := spawnHandler([]Value{NewList([]Value{NewWord("cover-proc-boom")})}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := asPid(out[0])
	if !ok {
		t.Fatalf("spawn must return a Pid, got %v", out)
	}
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("crashed process did not close")
	}
	if got := buf.String(); !strings.Contains(got, "crashed") {
		t.Errorf("crash must be reported on ErrOutput, got %q", got)
	}
}

func TestProcessCoverSpawnBodyErrorReported(t *testing.T) {
	r := seam5Reg(t)
	var buf bytes.Buffer
	r.ErrOutput = &buf
	out, err := spawnHandler([]Value{NewList([]Value{NewWord("cover-no-such-word")})}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := asPid(out[0])
	if !ok {
		t.Fatalf("spawn must return a Pid, got %v", out)
	}
	select {
	case <-p.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("failing process did not close")
	}
	if got := buf.String(); !strings.Contains(got, "exited with error") {
		t.Errorf("body error must be reported on ErrOutput, got %q", got)
	}
}

// ---- send ----

func TestProcessCoverSendableViolations(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"object", Value{Parent: TMap, Data: core.ClassInstanceInfo{}}, "Object"},
		{"table", Value{Parent: TMap, Data: core.TableData{}}, "Table"},
		{"flex", Value{Parent: TList, Data: &core.FlexListData{}}, "Flex"},
		{"store-in-list", NewList([]Value{
			NewInteger(1),
			{Parent: TStore, Data: &core.StoreInstanceInfo{}},
		}), "Store"},
		{"clean-list", NewList([]Value{NewInteger(1), NewString("a")}), ""},
	}
	for _, c := range cases {
		if got := sendableViolation(c.v); got != c.want {
			t.Errorf("%s: sendableViolation = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestProcessCoverSendOverloadAndShutdown(t *testing.T) {
	r := seam5Reg(t)
	rt := core.NewProcessRuntime()

	// "fail" policy on a full mailbox surfaces as the overload error.
	full := core.NewProcess(rt, 1, core.OverflowFail)
	if err := full.Send(NewInteger(1)); err != nil {
		t.Fatal(err)
	}
	_, err := sendHandler([]Value{NewInteger(2), NewPid(full)}, nil, nil, r)
	wantProcErr(t, err, "mailbox is full")

	// A blocked send that cannot make progress after runtime shutdown
	// surfaces as a generic process_error.
	blocked := core.NewProcess(rt, 1, core.OverflowBlock)
	if err := blocked.Send(NewInteger(1)); err != nil {
		t.Fatal(err)
	}
	rt.Shutdown()
	_, err = sendHandler([]Value{NewInteger(2), NewPid(blocked)}, nil, nil, r)
	wantProcErr(t, err, "shut down")
}

// ---- register / whereis ----

func TestProcessCoverRegisterErrors(t *testing.T) {
	r := seam5Reg(t)

	// A Pid-typed value with no process handle (the bare literal) is refused.
	_, err := registerHandler([]Value{NewString("cover-name"), NewTypeLiteral(TPid)}, nil, nil, r)
	wantProcErr(t, err, "expected a Pid")

	p := core.NewProcess(procRuntime(r), 4, core.OverflowBlock)
	_, err = registerHandler([]Value{NewString(""), NewPid(p)}, nil, nil, r)
	wantProcErr(t, err, "non-empty")

	// Positive twin: a valid registration resolves via whereis.
	if _, err := registerHandler([]Value{NewString("cover-reg"), NewPid(p)}, nil, nil, r); err != nil {
		t.Fatalf("valid register: %v", err)
	}
	out, err := whereisHandler([]Value{NewString("cover-reg")}, nil, nil, r)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := asPid(out[0]); !ok || got != p {
		t.Fatalf("whereis must return the registered pid, got %v", out)
	}

	// RegisterName refuses after runtime shutdown.
	r2 := seam5Reg(t)
	p2 := core.NewProcess(procRuntime(r2), 4, core.OverflowBlock)
	r2.Procs.Shutdown()
	_, err = registerHandler([]Value{NewString("late"), NewPid(p2)}, nil, nil, r2)
	wantProcErr(t, err, "shut down")
}

// ---- receive: clause parsing ----

func TestProcessCoverReceiveClauseParseErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{`receive [ after 5 ]`, "needs <ms> and [body]"},
		{`receive [ after "soon" [ 0 ] ]`, "non-negative Integer"},
		{`receive [ after 5 6 ]`, "`after` body must be a list"},
		{`receive [ 5 [ 0 ] ]`, "expected a {pattern} map"},
		{`receive [ {a: 1} ]`, "pattern has no [body]"},
		{`receive [ {a: 1} 5 ]`, "clause body must be a list"},
		{`receive []`, "needs at least one clause"},
		{`receive [ {a: [1 2]} [ 0 ] ]`, "Scalar routing tag or a Type binding slot"},
	}
	for _, c := range cases {
		r := seam5Reg(t)
		_, err := seam5Run(r, c.src)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", c.src, c.want, err)
		}
	}
}

func TestProcessCoverReceiveNonConcreteClauseList(t *testing.T) {
	r := seam5Reg(t)
	_, err := receiveHandler([]Value{NewTypeLiteral(TList)}, nil, nil, r)
	wantProcErr(t, err, "expected a concrete list")
}

func TestProcessCoverSplitClausePattern(t *testing.T) {
	r := seam5Reg(t)

	// Non-concrete pattern map is refused.
	if _, err := splitClausePattern(r, NewTypeLiteral(TMap), "receive", "receive_error"); err == nil {
		t.Fatal("type-literal pattern must be refused")
	}

	// Positive twin: scalar tags route, type literals bind.
	om := NewOrderedMap()
	om.Set("cmd", NewString("go"))
	om.Set("reply", NewTypeLiteral(TPid))
	c, err := splitClausePattern(r, NewMap(om), "receive", "receive_error")
	if err != nil {
		t.Fatal(err)
	}
	if c.route["cmd"] != "go" || len(c.binds) != 1 || c.binds[0].name != "reply" {
		t.Fatalf("unexpected split: %+v", c)
	}
	if c.isCatch {
		t.Error("a clause with routing tags must not be catch-all")
	}
}

// ---- receive: routing + binding ----

func TestProcessCoverReceiveDuplicateRouteFirstWins(t *testing.T) {
	r := seam5Reg(t)
	if _, err := seam5Run(r, `send {cmd: "a"} (self)`); err != nil {
		t.Fatal(err)
	}
	out, err := seam5Run(r, `receive [ {cmd: "a"} [ "one" ] {cmd: "a"} [ "two" ] ]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %v", out)
	}
	s, sErr := AsString(out[0])
	if sErr != nil || s != "one" {
		t.Errorf("first clause with identical tags must win, got %v (%v)", out[0], sErr)
	}
}

func TestProcessCoverReceiveBindFailureFallsToCatchAll(t *testing.T) {
	// Routing matches the typed clause, but the message is MISSING the
	// bound field — receive must fall back to the catch-all clause.
	r := seam5Reg(t)
	if _, err := seam5Run(r, `send {cmd: "get"} (self)`); err != nil {
		t.Fatal(err)
	}
	out, err := seam5Run(r, `receive [ {cmd: "get" reply: Pid} [ "typed" ] {} [ "fallback" ] ]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %v", out)
	}
	s, sErr := AsString(out[0])
	if sErr != nil || s != "fallback" {
		t.Errorf("binding failure must fall to the catch-all, got %v (%v)", out[0], sErr)
	}
}

func TestProcessCoverBindClauseRefusals(t *testing.T) {
	c := recvClause{binds: []recvBind{{name: "n", t: TInteger}}}

	// A non-map message cannot satisfy binding slots.
	if _, ok := bindSlots(c.binds, NewInteger(7)); ok {
		t.Error("non-map message must refuse binding slots")
	}
	// Map-conforming value with no OrderedMap payload (AsMap returns nil).
	if _, ok := bindSlots(c.binds, Value{Parent: TMap, Data: core.TableData{}}); ok {
		t.Error("map-typed value without an OrderedMap payload must refuse")
	}
	// Field missing.
	missing := NewOrderedMap()
	missing.Set("other", NewInteger(1))
	if _, ok := bindSlots(c.binds, NewMap(missing)); ok {
		t.Error("message missing the bound field must refuse")
	}
	// Field present with the wrong type.
	wrong := NewOrderedMap()
	wrong.Set("n", NewString("x"))
	if _, ok := bindSlots(c.binds, NewMap(wrong)); ok {
		t.Error("field of the wrong type must refuse")
	}
	// Positive twin: matching message binds the field.
	good := NewOrderedMap()
	good.Set("n", NewInteger(3))
	binds, ok := bindSlots(c.binds, NewMap(good))
	if !ok || len(binds) != 1 || binds[0].name != "n" {
		t.Fatalf("matching message must bind, got %v ok=%v", binds, ok)
	}
}

// ---- receive: runtime failure arms ----

func TestProcessCoverReceiveRuntimeErrors(t *testing.T) {
	// ensureSelfProc failure: runtime already down, no main process yet.
	r1 := seam5Reg(t)
	r1.Procs.Shutdown()
	_, err := receiveHandler([]Value{procCoverClauses()}, nil, nil, r1)
	wantProcErr(t, err, "shut down")

	// PopFront failure: live process, but the runtime shuts down under it.
	r2 := seam5Reg(t)
	p := core.NewProcess(r2.Procs, 4, core.OverflowBlock)
	if err := r2.Procs.Insert(p); err != nil {
		t.Fatal(err)
	}
	r2.Proc = p
	r2.Procs.Shutdown()
	_, err = receiveHandler([]Value{procCoverClauses()}, nil, nil, r2)
	wantProcErr(t, err, "receive: ")
}
