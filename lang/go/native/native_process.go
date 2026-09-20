package native

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
	eng "github.com/boru-lang/boru/eng/go"

	"github.com/boru-lang/boru/lang/go/native/internal/patrun"
	"github.com/boru-lang/boru/lang/go/policy"
)

// BEAM-style process words — the language surface of design/PROCESSES.0.md
// phase 1: `spawn` / `self` / `send` / `receive` / `register` / `whereis` /
// `unregister`, plus the opaque `Pid` handle type. The runtime substrate
// (ProcessRuntime, Process, the bounded mailbox) lives in eng/go/process.go;
// this file owns dispatch: patrun clause routing for `receive`, the
// immutable-only `send` rule, and the `process`-scope capability gate on
// `spawn`.
//
// Message-passing semantics (deliberate divergences noted):
//   - Messages are DEEP-COPIED at `send` (core.CloneValue): plain List/Map
//     payloads ARE mutated in place by `set` in today's boru, so zero-copy
//     sharing across goroutines (the PROCESSES.0.md §6 aspiration) is not
//     yet safe for containers. Scalars, Bytes, and handles still share
//     (CloneValue shares immutable payloads), so the common cases stay
//     cheap. Stateful containers (Store / Object / Table / Flex / Array)
//     are REJECTED (`not_sendable`) rather than silently snapshotted.
//   - Mailboxes are BOUNDED (default 1024) with a "block" / "fail" /
//     "drop" overflow policy per spawn (SERVICES.0.md §8.1).
//   - `receive` is consume-front + patrun dispatch (no selective receive).

// processNatives installs the process words.
var processNatives = []NativeFunc{
	{
		Name: "spawn",
		Signatures: []Signature{
			// spawn [body] {opts} — opts: {mailbox: Integer, overflow: "block"|"fail"|"drop"}.
			{Args: []*Type{TList, TMap}, Impl: Go(spawnHandler), Returns: []*Type{TPid},
				BarrierPos: -1, NoEvalArgs: map[int]bool{0: true}, CompileEffect: CompileStoresBody},
			// spawn [body] — start a process running body; returns its Pid.
			{Args: []*Type{TList}, Impl: Go(spawnHandler), Returns: []*Type{TPid},
				BarrierPos: -1, NoEvalArgs: map[int]bool{0: true}, CompileEffect: CompileStoresBody},
		},
	},
	{
		Name: "self",
		Signatures: []Signature{
			// self — the current process's pid (the implicit main process at top level).
			{Args: []*Type{}, Impl: Go(selfHandler), Returns: []*Type{TPid}, BarrierPos: -1},
		},
	},
	{
		Name: "send",
		Signatures: []Signature{
			// send <msg> <pid> — async enqueue. Statement form: returns
			// nothing (divergence from the RFC's "returns the message" —
			// sends overwhelmingly stand alone, and a returned message
			// strands on the stack).
			{Args: []*Type{TAny, TPid}, Impl: Go(sendHandler), Returns: []*Type{}, BarrierPos: -1},
			// send <msg> <name> — to a registered process name (String or Atom).
			{Args: []*Type{TAny, TString}, Impl: Go(sendHandler), Returns: []*Type{}, BarrierPos: -1},
			{Args: []*Type{TAny, TAtom}, Impl: Go(sendHandler), Returns: []*Type{}, BarrierPos: -1},
		},
	},
	{
		Name: "receive",
		Signatures: []Signature{
			// receive [ {pat} [body] … (after <ms> [body]) ] — take the front
			// mailbox message and dispatch it by clause.
			{Args: []*Type{TList}, Impl: Go(receiveHandler), Returns: []*Type{TAny},
				BarrierPos: -1, NoEvalArgs: map[int]bool{0: true}},
		},
	},
	{
		Name: "register",
		Signatures: []Signature{
			// register <name> <pid> — bind a name in the shared process
			// registry. Returns nothing (`spawn […] register "name"` leaves
			// the stack clean).
			{Args: []*Type{TString, TPid}, Impl: Go(registerHandler), Returns: []*Type{}, BarrierPos: -1},
			{Args: []*Type{TAtom, TPid}, Impl: Go(registerHandler), Returns: []*Type{}, BarrierPos: -1},
		},
	},
	{
		Name: "whereis",
		Signatures: []Signature{
			// whereis <name> — the registered Pid, or None.
			{Args: []*Type{TString}, Impl: Go(whereisHandler), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAtom}, Impl: Go(whereisHandler), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "unregister",
		Signatures: []Signature{
			// unregister <name> — remove a name binding (missing names are a no-op).
			{Args: []*Type{TString}, Impl: Go(unregisterHandler), Returns: []*Type{}, BarrierPos: -1},
			{Args: []*Type{TAtom}, Impl: Go(unregisterHandler), Returns: []*Type{}, BarrierPos: -1},
		},
	},
}

// checkProcessPolicy gates `spawn` behind the `process` capability scope
// (PROCESSES.0.md §7), mirroring fetch.go::checkFetchPolicy. No policy
// installed → allow (the historical default).
func checkProcessPolicy(r *Registry) error {
	if r == nil {
		return nil
	}
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	if !pol.Installed("process") {
		return PolicyRefusal(r, "spawn", &policy.Denied{
			Code:    policy.CodeCapabilityNotInstalled,
			Scope:   "process",
			Op:      "spawn",
			Profile: pol.Name(),
			Blame:   "process.install=false",
		})
	}
	return PolicyRefusal(r, "spawn", pol.Check("process", "spawn", policy.Args{}))
}

// procRuntime returns the registry's shared process runtime, creating one
// defensively for registries not built by core.NewRegistry (test fixtures).
func procRuntime(r *Registry) *core.ProcessRuntime {
	if r.Procs == nil {
		r.Procs = core.NewProcessRuntime()
	}
	return r.Procs
}

// ensureSelfProc returns the process this registry's goroutine runs as,
// lazily creating the implicit main process for a top-level registry (so a
// top-level program can `self`/`receive` replies from actors it spawned —
// the PROCESSES.0.md §10 shape).
func ensureSelfProc(r *Registry) (*core.Process, error) {
	if r.Proc != nil {
		return r.Proc, nil
	}
	rt := procRuntime(r)
	p := core.NewProcess(rt, core.DefaultMailboxBound, core.OverflowBlock)
	if err := rt.Insert(p); err != nil {
		return nil, r.BoruError("process_error", "process runtime is shut down", "self")
	}
	r.Proc = p
	return p, nil
}

func spawnHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := checkProcessPolicy(r); err != nil {
		return nil, err
	}
	// A COMPILED spawn body arrives as a synthetic fn-value carrier with a
	// CompiledFnRef (CompileStoresBody); an interpreted / declined body arrives as
	// a raw code-list. Run the former via RunUnit on the fork, the latter via a
	// fresh interpreter sub-engine.
	var compiledRef *compiler.CompiledFnRef
	if fd, ok := args[0].Data.(core.FnDefInfo); ok {
		for i := range fd.Signatures {
			if ref := compiler.CompiledRef(&fd.Signatures[i]); ref != nil && ref.Prog != nil {
				compiledRef = ref
				break
			}
		}
	}
	var tokens []Value
	if compiledRef == nil {
		bodyList, err := RequireConcreteList(args[0], "spawn")
		if err != nil {
			return nil, err
		}
		tokens = make([]Value, bodyList.Len())
		copy(tokens, bodyList.Slice())
	}
	bound := core.DefaultMailboxBound
	overflow := core.OverflowBlock
	if len(args) >= 2 {
		if opts, _ := AsMap(args[1]); opts != nil {
			if v, ok := opts.Get("mailbox"); ok {
				n, cErr := v.AsConcreteInteger()
				if cErr != nil || n < 0 {
					return nil, r.BoruError("spawn_error", "spawn: opts.mailbox must be a non-negative Integer (0 = unbounded)", "spawn")
				}
				bound = int(n)
			}
			if v, ok := opts.Get("overflow"); ok {
				s := ValToString(v)
				switch s {
				case core.OverflowBlock, core.OverflowFail, core.OverflowDrop:
					overflow = s
				default:
					return nil, r.BoruError("spawn_error",
						fmt.Sprintf("spawn: opts.overflow must be %q, %q or %q, got %q",
							core.OverflowBlock, core.OverflowFail, core.OverflowDrop, s), "spawn")
				}
			}
		}
	}

	rt := procRuntime(r)
	// Fork now, on the caller's goroutine (the ForkConcurrent contract),
	// so the body runs on an isolated registry.
	fork := r.ForkConcurrent()
	fork.Output = rt.SyncOutput(r.Output)
	fork.ErrOutput = rt.SyncOutput(r.ErrOutput)
	p := core.NewProcess(rt, bound, overflow)
	fork.Proc = p
	if err := rt.Insert(p); err != nil {
		return nil, r.BoruError("spawn_error", "spawn: process runtime is shut down", "spawn")
	}

	go func() {
		defer func() {
			// "Let it crash" per process: a panic or error terminates only
			// this process; the host and sibling processes are untouched.
			if rec := recover(); rec != nil {
				fmt.Fprintf(fork.ErrOutput, "[boru/process] %s crashed: %v\n", p.ID, rec)
			}
			rt.Remove(p)
			p.Close()
		}()
		var runErr error
		if compiledRef != nil {
			_, runErr = eng.RunUnit(compiledRef, fork, nil)
		} else {
			_, runErr = New(fork).Run(tokens)
		}
		if runErr != nil {
			fmt.Fprintf(fork.ErrOutput, "[boru/process] %s exited with error: %v\n", p.ID, runErr)
		}
	}()

	return []Value{NewPid(p)}, nil
}

func selfHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	p, err := ensureSelfProc(r)
	if err != nil {
		return nil, err
	}
	return []Value{NewPid(p)}, nil
}

// resolveSendTarget resolves a Pid / registered-name target. A dead or
// unknown target returns nil — `send` is fire-and-forget (BEAM semantics).
func resolveSendTarget(r *Registry, v Value) *core.Process {
	if p, ok := asPid(v); ok {
		return p
	}
	name := ValToString(v)
	if p, ok := procRuntime(r).Whereis(name); ok {
		return p
	}
	return nil
}

// sendableViolation walks a message value and returns the offending type
// name when it contains a stateful mutable container, or "" when the
// message is sendable. Plain List/Map are sendable (they are deep-copied
// at the boundary); Store / Object / Table / Flex nodes are declined.
func sendableViolation(v Value) string {
	switch d := v.Data.(type) {
	case *core.StoreInstanceInfo:
		return "Store"
	case core.ClassInstanceInfo:
		return "Object"
	case core.TableData:
		return "Table"
	case *core.FlexListData:
		return "Flex"
	case core.ListPayload:
		for _, e := range d.Elems {
			if s := sendableViolation(e); s != "" {
				return s
			}
		}
	case core.MapPayload:
		if d.M != nil {
			for _, k := range d.M.Keys() {
				e, _ := d.M.Get(k)
				if s := sendableViolation(e); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func sendHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	msg := args[0]
	if bad := sendableViolation(msg); bad != "" {
		return nil, r.BoruErrorHint("not_sendable",
			fmt.Sprintf("send: message contains a mutable %s, which cannot cross a process boundary", bad),
			"send", "messages must be immutable values (scalars, Bytes, plain List/Map, Pid); convert stateful containers first")
	}
	target := resolveSendTarget(r, args[1])
	if target == nil {
		return nil, nil // dead/unknown target: silently dropped
	}
	// Deep-copy at the boundary so the receiver can never observe the
	// sender mutating a shared container (see the file comment).
	if err := target.Send(CloneValue(msg)); err != nil {
		if err == core.ErrMailboxOverload {
			return nil, r.BoruError("overload", "send: target mailbox is full", "send")
		}
		return nil, r.BoruError("process_error", "send: "+err.Error(), "send")
	}
	return nil, nil
}

func registerHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := ValToString(args[0])
	p, ok := asPid(args[1])
	if !ok {
		return nil, r.BoruError("process_error", "register: expected a Pid, got "+args[1].Parent.String(), "register")
	}
	if name == "" {
		return nil, r.BoruError("process_error", "register: name must be non-empty", "register")
	}
	if err := procRuntime(r).RegisterName(name, p); err != nil {
		return nil, r.BoruError("process_error", "register: "+err.Error(), "register")
	}
	return nil, nil
}

func whereisHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	name := ValToString(args[0])
	if p, ok := procRuntime(r).Whereis(name); ok {
		return []Value{NewPid(p)}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

func unregisterHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	procRuntime(r).UnregisterName(ValToString(args[0]))
	return nil, nil
}

// ---- receive: clause parsing + two-layer dispatch ----
//
// A clause pattern is a Map whose CONCRETE-SCALAR fields are patrun
// routing tags and whose TYPE-LITERAL fields (`reply: Pid`, `n: Integer`)
// are binding slots — once patrun routes to a clause, the message's named
// fields are type-checked against the slots and bound into the clause
// body's scope (PROCESSES.0.md §3 "clause matching: routing vs. binding").

type recvBind struct {
	name string
	t    *core.Type
}

type recvClause struct {
	route   map[string]string // scalar routing tags (patrun subject keys)
	binds   []recvBind        // name:Type binding slots
	body    []Value
	isCatch bool // zero routing tags (matches any message)
}

type recvAfter struct {
	ms   int64
	body []Value
}

// parseReceiveClauses walks the raw clause list: `{pat} [body]` pairs plus
// an optional trailing `after <ms> [body]`.
func parseReceiveClauses(r *Registry, list Value) ([]recvClause, *recvAfter, error) {
	ll, err := RequireConcreteList(list, "receive")
	if err != nil {
		return nil, nil, err
	}
	elems := ll.Slice()
	var clauses []recvClause
	var after *recvAfter
	for i := 0; i < len(elems); i++ {
		e := elems[i]
		// `after <ms> [body]` — the timeout clause.
		if w, ok := e.Data.(core.WordInfo); ok && w.Name == "after" {
			if i+2 >= len(elems) {
				return nil, nil, r.BoruError("receive_error", "receive: `after` needs <ms> and [body]", "receive")
			}
			ms, cErr := elems[i+1].AsConcreteInteger()
			if cErr != nil || ms < 0 {
				return nil, nil, r.BoruError("receive_error", "receive: `after` deadline must be a non-negative Integer literal", "receive")
			}
			bodyList, bErr := RequireConcreteList(elems[i+2], "receive")
			if bErr != nil {
				return nil, nil, r.BoruError("receive_error", "receive: `after` body must be a list", "receive")
			}
			body := make([]Value, bodyList.Len())
			copy(body, bodyList.Slice())
			after = &recvAfter{ms: ms, body: body}
			i += 2
			continue
		}
		// `{pattern} [body]` pair.
		if !IsConcrete(e) || !e.Parent.ConformsTo(TMap) {
			return nil, nil, r.BoruError("receive_error",
				"receive: expected a {pattern} map (or `after`), got "+e.Parent.String(), "receive")
		}
		if i+1 >= len(elems) {
			return nil, nil, r.BoruError("receive_error", "receive: pattern has no [body]", "receive")
		}
		bodyList, bErr := RequireConcreteList(elems[i+1], "receive")
		if bErr != nil {
			return nil, nil, r.BoruError("receive_error", "receive: clause body must be a list", "receive")
		}
		body := make([]Value, bodyList.Len())
		copy(body, bodyList.Slice())
		clause, cErr := splitClausePattern(r, e)
		if cErr != nil {
			return nil, nil, cErr
		}
		clause.body = body
		clauses = append(clauses, clause)
		i++
	}
	if len(clauses) == 0 && after == nil {
		return nil, nil, r.BoruError("receive_error", "receive: needs at least one clause", "receive")
	}
	return clauses, after, nil
}

// splitClausePattern splits a clause pattern map into routing tags
// (concrete Scalars) and binding slots (type literals). Anything else is
// a loud error, matching patrun's pattern strictness.
func splitClausePattern(r *Registry, pat Value) (recvClause, error) {
	mp, err := RequireConcreteMap(pat, "receive")
	if err != nil {
		return recvClause{}, err
	}
	c := recvClause{route: map[string]string{}}
	for _, k := range mp.Keys() {
		v, _ := mp.Get(k)
		// The clause list is NoEvalArgs (raw), so a type name in the
		// pattern map arrives as an unevaluated Word — resolve it against
		// the active type bindings/builtins (`reply: Pid`, `n: Integer`).
		if w, ok := v.Data.(core.WordInfo); ok {
			t := r.LookupTypeName(w.Name) // user `def Foo …` type bindings
			if t == nil {
				t = core.TypeNameTable()[w.Name] // builtin / external names (Pid, Integer, …)
			}
			if t != nil {
				c.binds = append(c.binds, recvBind{name: k, t: CanonicalType(r, t)})
				continue
			}
			return recvClause{}, r.BoruErrorHint("receive_error",
				fmt.Sprintf("receive: pattern field %q names unknown type %q", k, w.Name),
				"receive", "route on top-level scalar tags ({cmd: \"inc\"}); bind typed fields ({reply: Pid})")
		}
		switch {
		case IsConcrete(v) && v.Parent.ConformsTo(TScalar):
			c.route[k] = ValToString(v)
		case IsTypeLiteral(v):
			t := core.ValueType(v)
			if t == nil { //covergate:allow IsTypeLiteral(v) implies a bare type node, so core.ValueType returns a non-nil pointer; the t==nil default cannot fire (§native)
				t = TAny
			}
			c.binds = append(c.binds, recvBind{name: k, t: CanonicalType(r, t)})
		default:
			return recvClause{}, r.BoruErrorHint("receive_error",
				fmt.Sprintf("receive: pattern field %q must be a Scalar routing tag or a Type binding slot, got %s", k, v.Parent.String()),
				"receive", "route on top-level scalar tags ({cmd: \"inc\"}); bind typed fields ({reply: Pid})")
		}
	}
	c.isCatch = len(c.route) == 0
	return c, nil
}

// routeMessage picks the clause for msg: patrun most-specific-wins over
// the scalar routing tags. Returns the clause index, or -1.
func routeMessage(clauses []recvClause, msg Value) int {
	pm := patrun.New()
	seen := map[string]bool{}
	for idx, c := range clauses {
		sig := routeSig(c.route)
		if seen[sig] {
			continue // first clause with identical tags wins
		}
		seen[sig] = true
		h := strconv.Itoa(idx)
		pm.Add(c.route, &h)
	}
	subj := map[string]string{}
	if IsConcrete(msg) && msg.Parent.ConformsTo(TMap) {
		if mp, _ := AsMap(msg); mp != nil {
			for _, k := range mp.Keys() {
				v, _ := mp.Get(k)
				if IsConcrete(v) && v.Parent.ConformsTo(TScalar) {
					subj[k] = ValToString(v)
				}
			}
		}
	}
	h, found := pm.Find(subj)
	if !found || h == nil {
		return -1
	}
	idx, err := strconv.Atoi(*h)
	if err != nil { //covergate:allow patrun handles are strconv.Itoa(idx) stored by routeMessage itself, so Atoi on a returned handle cannot fail (§native)
		return -1
	}
	return idx
}

func routeSig(route map[string]string) string {
	keys := make([]string, 0, len(route))
	for k := range route {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s := ""
	for _, k := range keys {
		s += k + "\x00" + route[k] + "\x00"
	}
	return s
}

// bindClause type-checks and collects the clause's binding slots against
// the message. ok=false means the message does not satisfy the slots.
func bindClause(c recvClause, msg Value) ([]recvBinding, bool) {
	if len(c.binds) == 0 {
		return nil, true
	}
	if !IsConcrete(msg) || !msg.Parent.ConformsTo(TMap) {
		return nil, false
	}
	mp, _ := AsMap(msg)
	if mp == nil {
		return nil, false
	}
	out := make([]recvBinding, 0, len(c.binds))
	for _, b := range c.binds {
		v, ok := mp.Get(b.name)
		if !ok {
			return nil, false
		}
		if !b.t.Equal(TAny) && !v.Is(b.t) {
			return nil, false
		}
		out = append(out, recvBinding{name: b.name, val: v})
	}
	return out, true
}

type recvBinding struct {
	name string
	val  Value
}

// runClauseBody runs a clause body with the bound fields installed as
// frame bindings (shadowing, torn down afterwards).
func runClauseBody(r *Registry, binds []recvBinding, body []Value) ([]Value, error) {
	names := make([]string, 0, len(binds))
	for _, b := range binds {
		core.InstallFrameBinding(r, b.name, b.val)
		names = append(names, b.name)
	}
	tokens := make([]Value, len(body))
	copy(tokens, body)
	sub := New(r)
	res, err := sub.Run(tokens)
	for i := len(names) - 1; i >= 0; i-- {
		UninstallDef(r, names[i])
	}
	return res, err
}

func receiveHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	clauses, after, err := parseReceiveClauses(r, args[0])
	if err != nil {
		return nil, err
	}
	p, err := ensureSelfProc(r)
	if err != nil {
		return nil, err
	}
	var msg Value
	var got bool
	if after != nil {
		msg, got, err = p.PopFront(time.Duration(after.ms)*time.Millisecond, true)
	} else {
		msg, got, err = p.PopFront(0, false)
	}
	if err != nil {
		return nil, r.BoruError("process_error", "receive: "+err.Error(), "receive")
	}
	if !got {
		// Deadline passed with an empty mailbox → the `after` body.
		return runClauseBody(r, nil, after.body)
	}

	idx := routeMessage(clauses, msg)
	if idx < 0 {
		return nil, r.BoruErrorHint("no_match",
			"receive: message matches no clause: "+ValToString(msg),
			"receive", "add a catch-all {} clause to accept unmatched messages")
	}
	binds, ok := bindClause(clauses[idx], msg)
	if !ok {
		// Routing matched but a binding slot declined (missing field or
		// type mismatch) — fall back to a catch-all clause if one exists.
		fell := false
		for i, c := range clauses {
			if c.isCatch && len(c.binds) == 0 && i != idx {
				idx, binds, fell = i, nil, true
				break
			}
		}
		if !fell {
			return nil, r.BoruErrorHint("no_match",
				"receive: message routed to a clause but failed its typed binding slots: "+ValToString(msg),
				"receive", "binding slots ({reply: Pid}) require the field present and of the slot type")
		}
	}
	return runClauseBody(r, binds, clauses[idx].body)
}
