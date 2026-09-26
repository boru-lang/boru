package native

import (
	"errors"
	"strings"
	"testing"
)

// Coverage for native_service.go (ADR-008): the in-process service
// words' guard branches, the serviceBehavior Format/Equal arms, the
// remote-endpoint (boru:net) hooks, and the handler-chain error paths.
// Handlers are driven directly (crafted args, unexported access) with
// positive twins alongside every negative, per lang/go/CLAUDE.md.

func svcCoverMap(k string, v Value) Value {
	om := NewOrderedMap()
	om.Set(k, v)
	return NewMap(om)
}

// svcCoverFn builds a Function value by evaluating a boru lambda.
func svcCoverFn(t *testing.T, r *Registry, src string) Value {
	t.Helper()
	out, err := seam5Run(r, src)
	if err != nil {
		t.Fatalf("building handler %q: %v", src, err)
	}
	if len(out) != 1 {
		t.Fatalf("building handler %q: expected 1 value, got %v", src, out)
	}
	return out[0]
}

// ---- registration ----

func TestServiceCoverRegisterServiceTypeDuplicate(t *testing.T) {
	saved := SwapTypeInitErrs(nil)
	t.Cleanup(func() { SwapTypeInitErrs(saved) })
	if tt := registerServiceType(); tt != nil {
		t.Fatalf("duplicate registration must return nil, got %v", tt)
	}
	err := TypeInitError()
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected already-registered init error, got %v", err)
	}
}

// ---- construction ----

func TestServiceCoverNewServiceValueFlexFallback(t *testing.T) {
	r := seam5Reg(t)

	// A RecordTypeInfo value has Parent=TMap yet no map payload, so
	// FlexDeepCopy declines it and NewServiceValue falls back to the
	// plain (non-flex) map state.
	rec := Value{Parent: TMap, Data: RecordTypeInfo{Fields: NewOrderedMap()}}
	om := NewOrderedMap()
	om.Set("rec", rec)
	svc := NewServiceValue(om)

	out, err := serviceStateHandler([]Value{svc}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("state-of on fallback service: %v / %v", out, err)
	}
	if !out[0].Parent.Equal(TMap) || out[0].Parent.Equal(TFlexMap) {
		t.Errorf("fallback state must be a plain Map, got %v", out[0].Parent)
	}
	m, mErr := AsMap(out[0])
	if mErr != nil || m == nil {
		t.Fatalf("fallback state must be map-readable: %v", mErr)
	}
	if _, ok := m.Get("rec"); !ok {
		t.Error("fallback state must retain the initial fields")
	}

	// Positive twin: a flexable initial map produces a flex state.
	fom := NewOrderedMap()
	fom.Set("count", NewInteger(1))
	fsvc := NewServiceValue(fom)
	fout, fErr := serviceStateHandler([]Value{fsvc}, nil, nil, r)
	if fErr != nil || len(fout) != 1 {
		t.Fatalf("state-of on flex service: %v / %v", fout, fErr)
	}
	if !fout[0].Parent.Equal(TFlexMap) {
		t.Errorf("flexable state must be a flex map, got %v", fout[0].Parent)
	}
}

func TestServiceCoverServiceNewHandlerNonConcreteMap(t *testing.T) {
	r := seam5Reg(t)
	if _, err := serviceNewHandler([]Value{NewCarrier(TMap)}, nil, nil, r); err == nil {
		t.Fatal("service with a non-concrete map must error")
	}
	// Positive twin.
	out, err := serviceNewHandler([]Value{svcCoverMap("n", NewInteger(0))}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("service {n:0} must construct, got %v / %v", out, err)
	}
	if _, ok := asService(out[0]); !ok {
		t.Fatalf("service must produce a Service value, got %v", out[0])
	}
}

// ---- ServiceCloser / asService guards ----

func TestServiceCoverServiceCloserArms(t *testing.T) {
	// Non-service (payload is not even an extension): nil.
	if ServiceCloser(NewInteger(1)) != nil {
		t.Error("ServiceCloser on an Integer must be nil")
	}
	// Extension payload that is not a serviceState: nil.
	if ServiceCloser(NewPatrun(TAny)) != nil {
		t.Error("ServiceCloser on a Patrun must be nil")
	}
	// Plain in-process service: closer is nil.
	if ServiceCloser(NewServiceValue(nil)) != nil {
		t.Error("ServiceCloser on a plain service must be nil")
	}
	// Positive twin: a remote endpoint carries its closer.
	closed := false
	ep := NewRemoteServiceValue(
		func(_ *Registry, _ Value, _ Value) ([]Value, error) { return nil, nil },
		func() error { closed = true; return nil },
	)
	closer := ServiceCloser(ep)
	if closer == nil {
		t.Fatal("ServiceCloser on an endpoint must return the closer")
	}
	if err := closer(); err != nil || !closed {
		t.Errorf("closer must run: err=%v closed=%v", err, closed)
	}
}

// ---- serviceBehavior ----

func TestServiceCoverBehaviorEqual(t *testing.T) {
	b := serviceBehavior{}
	s1 := NewServiceValue(nil)
	s2 := NewServiceValue(nil)
	if !b.Equal(s1, s1) {
		t.Error("a service must equal itself")
	}
	if b.Equal(s1, s2) {
		t.Error("distinct services must not compare equal")
	}
	// Non-service operand: falls back to DefaultBehavior.Equal.
	if b.Equal(s1, NewInteger(1)) {
		t.Error("a service must not equal an Integer")
	}
	if !b.Equal(NewInteger(1), NewInteger(1)) {
		t.Error("fallback Equal must still hold for identical scalars")
	}
	// Match delegates to the default conformance check.
	if !b.Match(s1, TService) {
		t.Error("a service must match TService")
	}
}

func TestServiceCoverBehaviorFormat(t *testing.T) {
	r := seam5Reg(t)
	b := serviceBehavior{}

	// Not a service: bare type name.
	if got := b.Format(NewInteger(1)); got != "Service" {
		t.Errorf("non-service Format = %q, want %q", got, "Service")
	}

	// Plain service with one handler and one wrap.
	svc := NewServiceValue(nil)
	fn := svcCoverFn(t, r, `([req:Map state:Any] => [ 1 ])`)
	if _, err := serviceAddHandler([]Value{svcCoverMap("op", NewString("x")), fn, svc}, nil, nil, r); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := serviceWrapHandler([]Value{fn, svc}, nil, nil, r); err != nil {
		t.Fatalf("wrap: %v", err)
	}
	if got := b.Format(svc); got != "Service(1 handlers, 1 wraps)" {
		t.Errorf("service Format = %q", got)
	}

	// Remote endpoint renders as Endpoint.
	ep := NewRemoteServiceValue(
		func(_ *Registry, _ Value, _ Value) ([]Value, error) { return nil, nil }, nil)
	if got := b.Format(ep); got != "Endpoint(0 handlers, 0 wraps)" {
		t.Errorf("endpoint Format = %q", got)
	}
}

// ---- receiver type guards on every word ----

func TestServiceCoverReceiverTypeErrors(t *testing.T) {
	r := seam5Reg(t)
	notSvc := NewInteger(9)
	req := svcCoverMap("op", NewString("x"))
	fn := svcCoverFn(t, r, `([req:Map state:Any] => [ 1 ])`)

	if _, err := serviceAddHandler([]Value{req, fn, notSvc}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "add: expected a Service") {
		t.Errorf("add receiver guard: got %v", err)
	}
	if _, err := serviceWrapHandler([]Value{fn, notSvc}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "wrap: expected a Service") {
		t.Errorf("wrap receiver guard: got %v", err)
	}
	if _, err := serviceStateHandler([]Value{notSvc}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "state-of: expected a Service") {
		t.Errorf("state-of receiver guard: got %v", err)
	}
	if _, err := serviceCallHandler([]Value{req, notSvc}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "call: expected a Service") {
		t.Errorf("call receiver guard: got %v", err)
	}
	if _, err := serviceSendHandler([]Value{req, notSvc}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "send: expected a Service") {
		t.Errorf("send receiver guard: got %v", err)
	}
}

// ---- add: pattern coercion ----

func TestServiceCoverAddRejectsNonScalarPattern(t *testing.T) {
	r := seam5Reg(t)
	svc := NewServiceValue(nil)
	fn := svcCoverFn(t, r, `([req:Map state:Any] => [ 1 ])`)

	badPat := svcCoverMap("a", NewList([]Value{NewInteger(1)}))
	if _, err := serviceAddHandler([]Value{badPat, fn, svc}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "must be a Scalar") {
		t.Fatalf("non-scalar pattern value must be rejected, got %v", err)
	}
	// Positive twin: a scalar pattern registers.
	if _, err := serviceAddHandler([]Value{svcCoverMap("a", NewInteger(1)), fn, svc}, nil, nil, r); err != nil {
		t.Fatalf("scalar pattern must register: %v", err)
	}
	if got := ServiceRoutePatterns(svc); len(got) != 1 {
		t.Errorf("expected 1 registered pattern, got %d", len(got))
	}
}

// ---- remote endpoints ----

func TestServiceCoverRemoteSendForwards(t *testing.T) {
	r := seam5Reg(t)
	var seen []Value
	ep := NewRemoteServiceValue(
		func(_ *Registry, req Value, _ Value) ([]Value, error) {
			seen = append(seen, req)
			return []Value{NewString("remote-reply")}, nil
		}, nil)

	out, err := serviceSendHandler([]Value{svcCoverMap("op", NewString("x")), ep}, nil, nil, r)
	if err != nil {
		t.Fatalf("remote send: %v", err)
	}
	if out != nil {
		t.Errorf("send must discard the reply, got %v", out)
	}
	if len(seen) != 1 {
		t.Fatalf("remote dispatch must receive the request once, got %d", len(seen))
	}

	// Negative twin: a transport failure surfaces from send.
	epErr := NewRemoteServiceValue(
		func(_ *Registry, _ Value, _ Value) ([]Value, error) {
			return nil, errors.New("wire down")
		}, nil)
	if _, err := serviceSendHandler([]Value{svcCoverMap("op", NewString("x")), epErr}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "wire down") {
		t.Errorf("remote send must propagate the transport error, got %v", err)
	}
}

// ---- dispatch error paths ----

func TestServiceCoverSendPropagatesDispatchError(t *testing.T) {
	r := seam5Reg(t)
	svc := NewServiceValue(nil)
	// No handlers registered: local dispatch raises no_match and send
	// must propagate it rather than swallow it.
	if _, err := serviceSendHandler([]Value{svcCoverMap("op", NewString("x")), svc}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "no_match") {
		t.Fatalf("send into an empty service must raise no_match, got %v", err)
	}
}

func TestServiceCoverDispatchSubjectCoercionError(t *testing.T) {
	r := seam5Reg(t)
	svc := NewServiceValue(nil)
	// A non-concrete request cannot be routed: coerceSubject declines it.
	if _, err := DispatchServiceValue(r, svc, NewCarrier(TMap)); err == nil {
		t.Fatal("dispatch with a non-concrete request must error")
	}
}

func TestServiceCoverDispatchServiceValueArms(t *testing.T) {
	r := seam5Reg(t)
	req := svcCoverMap("op", NewString("x"))
	if _, err := DispatchServiceValue(r, NewInteger(1), req); err == nil ||
		!strings.Contains(err.Error(), "dispatch: expected a Service") {
		t.Fatalf("dispatch on a non-service must error, got %v", err)
	}
	// Positive twin: a catch-all handler answers via the Go hook.
	r2 := seam5Reg(t)
	svc := NewServiceValue(nil)
	fn := svcCoverFn(t, r2, `([req:Map state:Any] => [ "fallback" ])`)
	if _, err := serviceAddHandler([]Value{NewMap(NewOrderedMap()), fn, svc}, nil, nil, r2); err != nil {
		t.Fatalf("add catch-all: %v", err)
	}
	out, err := DispatchServiceValue(r2, svc, req)
	if err != nil || len(out) != 1 {
		t.Fatalf("dispatch: %v / %v", out, err)
	}
	if s, _ := AsString(out[0]); s != "fallback" {
		t.Errorf("dispatch reply = %v, want fallback", out[0])
	}
}

func TestServiceCoverRoutePatternsNonService(t *testing.T) {
	if got := ServiceRoutePatterns(NewInteger(1)); got != nil {
		t.Errorf("ServiceRoutePatterns on a non-service must be nil, got %v", got)
	}
}

// ---- handler chain ----

func TestServiceCoverHandlerArityMismatch(t *testing.T) {
	r := seam5Reg(t)
	svc := NewServiceValue(nil)
	// A 1-param function passes requireHandlerFn but matches neither
	// [req state] nor [req state prior] at dispatch time.
	bad := svcCoverFn(t, r, `([a:Map] => [ 1 ])`)
	pat := svcCoverMap("op", NewString("bad"))
	if _, err := serviceAddHandler([]Value{pat, bad, svc}, nil, nil, r); err != nil {
		t.Fatalf("add: %v", err)
	}
	_, err := serviceCallHandler([]Value{pat, svc}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "handler signature must be [req state] or [req state prior]") {
		t.Fatalf("bad-arity handler must raise the signature error, got %v", err)
	}
}

func TestServiceCoverRunHandlerChainGuards(t *testing.T) {
	r := seam5Reg(t)
	state := NewMap(NewOrderedMap())
	req := svcCoverMap("op", NewString("x"))

	// Empty chain: a `prior` call past the bottom of the stack yields None.
	out, err := runHandlerChain(r, state, req, nil)
	if err != nil || len(out) != 1 || !IsNoneShape(out[0]) {
		t.Fatalf("empty chain must yield None, got %v / %v", out, err)
	}

	// A non-function chain entry is declined (defense-in-depth: add/wrap
	// validate handlers, but the chain runner must not trust them).
	if _, err := runHandlerChain(r, state, req, []chainLink{{handler: NewInteger(1)}}); err == nil ||
		!strings.Contains(err.Error(), "handler is not a function") {
		t.Fatalf("non-function chain entry must error, got %v", err)
	}
}

// Language-level twin of the empty-chain guard: the only handler for a
// pattern calls `prior`, which continues past the bottom and yields None.
func TestServiceCoverPriorPastBottomReturnsNone(t *testing.T) {
	r := seam5Reg(t)
	out, err := seam5Run(r, `def svc (service {})
add {op:"solo"} ([req:Map state:Any prior:Any] => [ prior req ]) svc
call {op:"solo"} svc`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) != 1 || !IsNoneShape(out[0]) {
		t.Errorf("prior past bottom must reply None, got %v", out)
	}
}

// topSlots reads the newest layer's binding slots; an empty stack has none
// (dispatch only asks of a routed pattern, which always has a layer).
func TestServiceCoverTopSlotsOfAnEmptyStack(t *testing.T) {
	if got := topSlots(nil); got != nil {
		t.Errorf("an empty stack has no slots, got %v", got)
	}
	layers := [][]recvBind{nil, {{name: "n", t: TInteger}}}
	if got := topSlots(layers); len(got) != 1 || got[0].name != "n" {
		t.Errorf("the newest layer's slots, got %v", got)
	}
}
