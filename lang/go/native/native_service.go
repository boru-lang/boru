package native

import (
	"fmt"
	"sort"
	"sync"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// In-process services — the language surface of design/SERVICES.0.md
// phase 1: a `Service` value owns private state and answers
// pattern-matched requests through a patrun of handlers. Words:
//
//	service {state}                 -> Service
//	add {pattern} [handler] svc                  (patrun routing; handler stacks)
//	call {request} svc              -> reply     (no_match if nothing matches)
//	send {request} svc                           (same dispatch, reply discarded)
//	state-of svc                    -> Map       (the private state)
//	wrap [handler] svc                           (ambient middleware)
//
// Handlers are `[req state] -> reply` or, for layering/middleware,
// `[req state prior] -> reply` where `prior` is the continuation for the
// next handler down the chain (SERVICES.0.md §1 "prior"/"wrap").
//
// Divergences from the RFC (recorded in
// design/legacy/NETWORK-IMPLEMENTATION-PLAN.0.ignore):
//   - The state accessor is `state-of` (not `state`): handler params
//     cannot shadow a built-in word, and the RFC's handler contract
//     names its second param `state` in every example.
//   - The state is a FLEX map, not a Store: flex nodes are the one
//     container family whose `set` mutates IN PLACE through every
//     alias (lang/spec/edge-containers-2.tsv §"flex"), which is the
//     semantics the handler contract needs — a Store `set` is
//     COW/layered and a Map `set` is functional, so writes through a
//     captured alias would not stick with either. Handlers write
//     `state set count 9` and read `state.count`.
//   - Instead of running a served service as its own process, a
//     Service serializes dispatch with an internal mutex — the same
//     "one request at a time" gen_server guarantee, so one shared
//     service is safe under many connection actors. A handler that
//     `call`s its OWN service deadlocks (as in OTP).

// RemoteDispatch is the transport hook a `connect`ed Endpoint carries: it
// forwards an encoded request to the remote peer and returns the reply.
// Installed by boru:net (design/NETWORK-CLIENTS.0.md §6); nil for ordinary
// in-process services.
type RemoteDispatch func(r *Registry, req Value, opts Value) ([]Value, error)

// serviceState is the payload behind a Service value: the handler patrun,
// per-pattern handler stacks, wrap middleware, and the private state
// Store. The dispatch mutex serializes handling (one request at a time).
type serviceState struct {
	mu     sync.Mutex
	pm     *patrunMatcher
	stacks map[string][]Value // pattern sig → handler stack (newest last)
	wraps  []Value            // ambient middleware (newest last = outermost)
	state  Value              // the private state: a flex map, mutated in place

	// slots are each stacked handler's binding slots (its pattern's
	// `name:Type` fields), aligned with stacks: pattern sig → per-layer
	// slots, newest last (NUR064).
	slots map[string][][]recvBind

	// remote, when non-nil, makes this Service an Endpoint: call/send
	// forward over the wire instead of dispatching locally. `add`ed
	// handlers stay local (peer-push dispatch is a later phase).
	remote RemoteDispatch
	// closer tears down the endpoint's transport (the `close` overload).
	closer func() error
}

// NewServiceValue builds a fresh in-process Service with the given
// initial state fields (may be nil).
func NewServiceValue(initial *OrderedMap) Value {
	om := NewOrderedMap()
	if initial != nil {
		for _, k := range initial.Keys() {
			v, _ := initial.Get(k)
			om.Set(k, v)
		}
	}
	st, err := core.FlexDeepCopy(NewMap(om))
	if err != nil {
		// A plain map of already-constructed values always flexes; fall
		// back to the map itself rather than failing construction.
		st = NewMap(om)
	}
	s := &serviceState{
		pm:     newPatrunMatcher(TAny),
		stacks: map[string][]Value{},
		slots:  map[string][][]recvBind{},
		state:  st,
	}
	return core.NewExtension(TService, s)
}

// NewRemoteServiceValue builds an Endpoint: a Service whose call/send
// forward through the given transport dispatch. Used by boru:net connect.
func NewRemoteServiceValue(dispatch RemoteDispatch, closer func() error) Value {
	v := NewServiceValue(nil)
	s, _ := asService(v)
	s.remote = dispatch
	s.closer = closer
	return v
}

// ServiceCloser returns the endpoint's transport closer (nil for plain
// services). Used by boru:net's `close` overload.
func ServiceCloser(v Value) func() error {
	if s, ok := asService(v); ok {
		return s.closer
	}
	return nil
}

// ServiceStateOf returns the service's state value (the same value the
// `state-of` word yields) — the hook a Go driver uses to render from a
// service-shaped app's state. The bool is false for non-Service values.
func ServiceStateOf(v Value) (Value, bool) {
	s, ok := asService(v)
	if !ok {
		return Value{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, true
}

func asService(v Value) (*serviceState, bool) {
	ep, ok := v.Data.(core.ExtensionPayload)
	if !ok {
		return nil, false
	}
	s, ok := ep.Body.(*serviceState)
	return s, ok
}

// FormatService renders the state with its handler count — the
// delegation seam basic.ServiceBehavior formats through (the type
// identity and Behavior shell moved to basic/go with the Ideal/Service
// registration; the handler state stays here with the words). A
// connected remote renders as an Endpoint.
func (s *serviceState) FormatService() string {
	s.mu.Lock()
	n := 0
	for _, st := range s.stacks {
		n += len(st)
	}
	w := len(s.wraps)
	remote := s.remote != nil
	s.mu.Unlock()
	if remote {
		return fmt.Sprintf("Endpoint(%d handlers, %d wraps)", n, w)
	}
	return fmt.Sprintf("Service(%d handlers, %d wraps)", n, w)
}

// serviceNatives installs the service words. `add` and `send` carry
// Service overloads folded onto the existing words (upsertFnDef appends).
var serviceNatives = []NativeFunc{
	{
		Name: "service",
		Signatures: []Signature{
			// service {state} — construct a service with initial private state.
			{Args: []*Type{TMap}, Impl: Go(serviceNewHandler), Returns: []*Type{TService}, BarrierPos: -1},
		},
	},
	{
		Name: "add",
		Signatures: []Signature{
			// add {pattern} [handler] svc — register a handler; adding to an
			// already-registered pattern PUSHES a layering stack (prior).
			// The pattern is a clause pattern, as a `receive` clause's is:
			// scalar fields route, `name:Type` fields are binding slots the
			// handler's run sees by name (NUR064). Returns nothing
			// (statement form, like the Patrun overload).
			{Args: []*Type{TMap, TAny, TService}, Impl: Go(serviceAddHandler), Returns: []*Type{},
				ReturnsFn:  serviceAddCheck,
				BarrierPos: -1, CompileEffect: CompileStoresFn | CompileFnHandlerStrict},
		},
	},
	{
		Name: "call",
		Signatures: []Signature{
			// call {request} svc {opts} — opts (timeout:, …) honoured by remote
			// endpoints; advisory in-process.
			{Args: []*Type{TMap, TService, TMap}, Impl: Go(serviceCallHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			// call {request} svc — synchronous request → reply.
			{Args: []*Type{TMap, TService}, Impl: Go(serviceCallHandler), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "send",
		Signatures: []Signature{
			// send {request} svc — same dispatch as call, reply discarded.
			{Args: []*Type{TAny, TService}, Impl: Go(serviceSendHandler), Returns: []*Type{}, BarrierPos: -1},
		},
	},
	{
		Name: "state-of",
		Signatures: []Signature{
			// state-of svc — the service's private state (rarely needed
			// directly; handlers receive it as their second param).
			{Args: []*Type{TService}, Impl: Go(serviceStateHandler), Returns: []*Type{TMap}, BarrierPos: -1},
		},
	},
	{
		Name: "wrap",
		Signatures: []Signature{
			// wrap [handler] svc — ambient middleware around every dispatch.
			{Args: []*Type{TAny, TService}, Impl: Go(serviceWrapHandler), Returns: []*Type{},
				BarrierPos: -1, CompileEffect: CompileStoresFn | CompileFnHandlerStrict},
		},
	},
}

func serviceNewHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	mp, err := RequireConcreteMap(args[0], "service")
	if err != nil {
		return nil, err
	}
	om := NewOrderedMap()
	for _, k := range mp.Keys() {
		v, _ := mp.Get(k)
		om.Set(k, v)
	}
	return []Value{NewServiceValue(om)}, nil
}

// requireHandlerFn validates a handler argument is a function value.
func requireHandlerFn(r *Registry, v Value, word string) error {
	if _, ok := FnDefFromValue(v); !ok {
		return r.BoruErrorHint(word+"_error",
			word+": handler must be a function, got "+v.Parent.String(),
			word, "write `[ [req state] => [ … ] ]` (or `[req state prior]` for a layering handler)")
	}
	return nil
}

func serviceAddHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	s, ok := asService(args[2])
	if !ok {
		return nil, r.BoruError("service_error", "add: expected a Service, got "+args[2].Parent.String(), "add")
	}
	if err := requireHandlerFn(r, args[1], "add"); err != nil {
		return nil, err
	}
	clause, err := splitClausePattern(r, args[0], "add", "patrun_error")
	if err != nil {
		return nil, err
	}
	pat := clause.route
	keys := make([]string, 0, len(pat))
	for k := range pat {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sig := patrunSig(keys, pat)
	// Detached-stamp the handler at its store site so runHandlerChain's
	// InvokeCallback runs it on the VM: a handler added from an interpreted
	// context (a module fn's body — the real apps) is otherwise invisible to
	// every compile pass. StampFnValue declines silently (policy off,
	// already stamped, capturing, declining body) returning the input
	// unchanged, so behaviour is byte-identical on every decline; the stamp
	// runs BEFORE the lock (it forks the registry, no service state).
	handler, _ := compiler.StampFnValue(r, args[1])
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.stacks[sig]; !exists {
		h := sig
		s.pm.pm.Add(pat, &h)
		s.pm.side[sig] = patrunRule{raw: args[0], val: handler, disp: patrunDisp(keys, pat)}
		s.pm.order = append(s.pm.order, sig)
	}
	// Same-pattern add PUSHES (layering, newest outermost) — deliberately
	// different from raw patrun, which overwrites (SERVICES.0.md §1). A
	// pattern's identity is its routing tags; its slots ride with the layer.
	s.stacks[sig] = append(s.stacks[sig], handler)
	s.slots[sig] = append(s.slots[sig], clause.binds)
	return nil, nil
}

// serviceAddCheck is `add`'s check-mode half over a Service. The handler's
// body was analysed where its fn literal was built — before this call names
// the pattern's binding slots — so its reads of a slot's name reported
// undefined_word. Those reads are the slot's (the run binds the name around
// the handler, as a `receive` clause's body sees it), so the pass notes each
// such token for RescueForwardRefDiagnostics to excuse: that token, not the
// name (NUR064).
func serviceAddCheck(args []Value, r *Registry) []Value {
	// The check pass's unmatched-dispatch recovery calls a candidate's
	// check half over whatever operands the call had (`'x' add`).
	if len(args) < 2 {
		return []Value{}
	}
	slots := map[string]bool{}
	if mp, err := AsMap(args[0]); err == nil && mp != nil && IsConcrete(args[0]) {
		for _, k := range mp.Keys() {
			if v, _ := mp.Get(k); IsTypeLiteral(v) {
				slots[k] = true
			}
		}
	}
	if fd, ok := FnDefFromValue(args[1]); ok && len(slots) > 0 {
		for i := range fd.Signatures {
			core.WalkBodyWords(fd.Signatures[i].Body(), func(w core.WordInfo, tok Value) {
				if slots[w.Name] {
					r.Check.NoteSlotBoundRead(w.Name, tok.Pos())
				}
			})
		}
	}
	return []Value{}
}

func serviceWrapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	s, ok := asService(args[1])
	if !ok {
		return nil, r.BoruError("service_error", "wrap: expected a Service, got "+args[1].Parent.String(), "wrap")
	}
	if err := requireHandlerFn(r, args[0], "wrap"); err != nil {
		return nil, err
	}
	// Same store-site stamping as `add` (see serviceAddHandler).
	wrap, _ := compiler.StampFnValue(r, args[0])
	s.mu.Lock()
	s.wraps = append(s.wraps, wrap)
	s.mu.Unlock()
	return nil, nil
}

func serviceStateHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	s, ok := asService(args[0])
	if !ok {
		return nil, r.BoruError("service_error", "state-of: expected a Service, got "+args[0].Parent.String(), "state-of")
	}
	return []Value{s.state}, nil
}

func serviceCallHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	s, ok := asService(args[1])
	if !ok {
		return nil, r.BoruError("service_error", "call: expected a Service, got "+args[1].Parent.String(), "call")
	}
	var opts Value
	if len(args) >= 3 {
		opts = args[2]
	}
	if s.remote != nil {
		return s.remote(r, args[0], opts)
	}
	res, err := dispatchService(r, s, args[0])
	if err != nil {
		return nil, err
	}
	return res, nil
}

func serviceSendHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	s, ok := asService(args[1])
	if !ok {
		return nil, r.BoruError("service_error", "send: expected a Service, got "+args[1].Parent.String(), "send")
	}
	if s.remote != nil {
		_, err := s.remote(r, args[0], Value{})
		return nil, err
	}
	if _, err := dispatchService(r, s, args[0]); err != nil {
		return nil, err
	}
	return nil, nil // reply discarded
}

// ServiceRoutePatterns returns the raw pattern maps registered on a
// Service, in registration order. The boru:net HTTP router scans these
// for `path` templates carrying `:param` / `*rest` segments.
func ServiceRoutePatterns(svc Value) []Value {
	s, ok := asService(svc)
	if !ok {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Value, 0, len(s.pm.order))
	for _, sig := range s.pm.order {
		out = append(out, s.pm.side[sig].raw)
	}
	return out
}

// DispatchServiceValue routes one request into a Service from Go — the
// hook the boru:net transport loop uses to deliver decoded messages.
func DispatchServiceValue(r *Registry, svc Value, req Value) ([]Value, error) {
	s, ok := asService(svc)
	if !ok {
		return nil, r.BoruError("service_error", "dispatch: expected a Service", "call")
	}
	return dispatchService(r, s, req)
}

// dispatchService routes req through the wrap layers → patrun match →
// per-pattern prior stack → base handler, under the service mutex.
func dispatchService(r *Registry, s *serviceState, req Value) ([]Value, error) {
	s.mu.Lock()
	// Route on the request's scalar fields.
	subj, err := coerceSubject(req, "call")
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	h, found := s.pm.pm.Find(subj)
	var chain []chainLink
	// wraps run outermost, newest first.
	for i := len(s.wraps) - 1; i >= 0; i-- {
		chain = append(chain, chainLink{handler: s.wraps[i]})
	}
	slotsDecline := false
	if found && h != nil {
		sig := *h
		// Routing matched; the handler's binding slots decide whether it
		// takes the request — a declining one falls back to a slot-free
		// catch-all, as a `receive` clause's does (NUR064).
		if _, ok := bindSlots(topSlots(s.slots[sig]), req); !ok {
			if sig != "" && len(s.stacks[""]) > 0 && len(topSlots(s.slots[""])) == 0 {
				sig = "" // the `{}` route
			} else {
				slotsDecline = true
			}
		}
		if !slotsDecline {
			stack, slots := s.stacks[sig], s.slots[sig]
			for i := len(stack) - 1; i >= 0; i-- {
				chain = append(chain, chainLink{handler: stack[i], slots: slots[i]})
			}
		}
	}
	state := s.state
	s.mu.Unlock()

	if !found || h == nil {
		return nil, r.BoruErrorHint("no_match",
			"call: no handler matches request "+ValToString(req),
			"call", "register a handler with `add {pattern} [handler] svc` (a catch-all `add {} …` accepts anything)")
	}
	if slotsDecline {
		return nil, r.BoruErrorHint("no_match",
			"call: request routed to a handler but failed its typed binding slots: "+ValToString(req),
			"call", "binding slots ({reply: Pid}) require the field present and of the slot type")
	}

	// Serialize the actual handling: one request at a time (the
	// gen_server guarantee — handlers mutate `state` without locks).
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := runHandlerChain(r, state, req, chain)
	if err != nil {
		return nil, err
	}
	// The reply is the handler's LAST result: bodies routinely leave
	// residue (a flex `set` returns its receiver), and "the reply" is a
	// single value by contract.
	if len(res) > 1 {
		res = res[len(res)-1:]
	}
	return res, nil
}

// chainLink is one handler a dispatch runs: a wrap (no slots) or a stacked
// `add` handler with its pattern's binding slots.
type chainLink struct {
	handler Value
	slots   []recvBind
}

// topSlots is the newest layer's binding slots — the handler routing reaches
// first — or none for an empty stack.
func topSlots(layers [][]recvBind) []recvBind {
	if len(layers) == 0 {
		return nil
	}
	return layers[len(layers)-1]
}

// runHandlerChain invokes chain[0] with (req, state) or (req, state,
// prior) by handler arity; `prior` continues at chain[1:]. A stacked
// handler runs with its binding slots bound from the request it receives —
// a request `prior` hands on without a slot's field reaches no handler.
func runHandlerChain(r *Registry, state Value, req Value, chain []chainLink) ([]Value, error) {
	if len(chain) == 0 {
		// A layering handler called `prior` past the bottom of the stack.
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	binds, ok := bindSlots(chain[0].slots, req)
	if !ok {
		return nil, r.BoruErrorHint("no_match",
			"call: request passed on by prior fails the handler's typed binding slots: "+ValToString(req),
			"call", "binding slots ({reply: Pid}) require the field present and of the slot type")
	}
	return withSlotBindings(r, binds, func() ([]Value, error) {
		return invokeHandler(r, state, req, chain[0].handler, chain[1:])
	})
}

// invokeHandler runs one handler over (req, state) or (req, state, prior).
func invokeHandler(r *Registry, state Value, req Value, handler Value, rest []chainLink) ([]Value, error) {
	fnInfo, ok := FnDefFromValue(handler)
	if !ok {
		return nil, r.BoruError("service_error", "handler is not a function", "call")
	}

	// Try the layering arity first: [req state prior]. InvokeCallback runs the
	// handler on the VM when its body compiled to a unit (nested in the enclosing
	// run, or fresh on an idle actor registry) and falls back to CallBoru — the
	// interpreter — for a body that didn't compile (e.g. one that calls `prior`).
	priorFn := makePriorFn(r, state, rest)
	args3 := []Value{req, state, priorFn}
	if sig := MatchFnSig(handler, args3); sig != nil {
		return core.InvokeCallbackFn(r, fnInfo, sig, args3)
	}
	args2 := []Value{req, state}
	if sig := MatchFnSig(handler, args2); sig != nil {
		return core.InvokeCallbackFn(r, fnInfo, sig, args2)
	}
	return nil, r.BoruErrorHint("service_error",
		"handler signature must be [req state] or [req state prior]",
		"call", "declare handlers as `[ [req state] => [ … ] ]`")
}

// makePriorFn builds the `prior` continuation: a Function value whose Go
// handler resumes the chain at rest. Passing a (possibly modified)
// request re-dispatches the remaining layers with it.
func makePriorFn(r *Registry, state Value, rest []chainLink) Value {
	handler := func(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
		return runHandlerChain(reg, state, args[0], rest)
	}
	// Authored with Params (not the legacy Args field): a constructed
	// Function value dispatches through compileFnDef, which derives the
	// forward barrier from len(Params).
	sig := Signature{
		Params:     []core.FnParam{{Type: TAny}},
		Impl:       Go(handler),
		Returns:    []*Type{TAny},
		BarrierPos: 1,
	}
	return NewFunction(FnDefInfo{Name: "prior", Signatures: []Signature{sig}})
}
