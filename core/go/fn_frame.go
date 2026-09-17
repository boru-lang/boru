package core

import "fmt"

// This file is the single home of the fn-frame tape protocol: the
// marked open paren that starts a spliced fn-body frame, the canonical
// cleanup tail that ends it, and the Go-side expression of what the
// tail's markers do. Every site that splices a frame onto the tape
// (buildFnBodyHandler, compileFnDef via buildFnBodyHandler, and
// execFnDefSig's no-captured-registry branch) builds its frame from
// these pieces, so the on-tape frame shape cannot diverge between
// dispatch paths. See design/legacy/TCO-STAGED.10.ignore Stage 1.
//
// A frame on the tape is:
//
//	(ₘ  unnamed-args…  body…  __DC  __pa  undef n₁ … undef nₖ  [__RC]  )
//
// where (ₘ is a FrameOpenInfo-marked open paren, __DC is the
// DefCleanup marker (truncates body-local defs to the entry snapshot),
// __pa pops the per-call Args list and FnBaseline, the undef pairs
// tear down captures+params (reverse install order), and __RC is the
// ReturnCheck when returns are declared.

// FnFrameMeta identifies one compiled fn overload across the frames it
// splices. The SAME pointer is attached to the compiled Signature
// (Signature.FnFrame) and stamped on the open paren of every frame
// that signature splices, so "is this dispatch a self-recursive call
// of the enclosing frame's fn?" is a pointer comparison — no name
// lookup, no structural sig equality.
type FnFrameMeta struct {
	// Name is the fn's registered name; "<fn>" for frames spliced from
	// an unregistered Function value (execFnDefSig's splice branch).
	Name string
	// HasGen marks a generic fn (the handler installs inferred
	// type-parameter bindings per call). The eager-teardown gate
	// declines generic frames until the bind/teardown/Retire
	// interaction is separately proven (design/legacy/TCO-STAGED.10.ignore
	// Stage 4).
	HasGen bool
	// InstallNames are the binding names this overload's handler
	// installs at every call: capture names plus named params — the
	// same list the synthesized undef tail tears down. The
	// eager-teardown gate requires the CALLER frame's removed names to
	// be a subset of the CALLEE's InstallNames: bindings are then
	// rebound before the callee body runs, so a dynamic read anywhere
	// in the callee chain sees exactly what it would have seen under
	// nesting (where the callee's bindings shadow the caller's).
	// Without this, tearing down a frame whose body-locals or params
	// the callee reads dynamically — the recursive-local-fn idiom
	// (`def go fn […] go 3`), a loop-carried base-branch read —
	// breaks with undefined_word.
	InstallNames []string
}

// fnInstallNames computes the binding names a fn-body handler installs
// per call — captures first, then named params, mirroring
// buildFnBodyHandler's install order — for FnFrameMeta.InstallNames.
func fnInstallNames(s FnSig, captured []CapturedBinding) []string {
	names := make([]string, 0, len(captured)+len(s.Params))
	for _, cb := range captured {
		names = append(names, cb.Name)
	}
	for _, p := range s.Params {
		if p.Name != "" {
			names = append(names, p.Name)
		}
	}
	return names
}

// fnValueFrameMeta marks frames spliced by execFnDefSig for a Function
// value dispatched straight off the tape. No registered Signature
// carries this meta, so such frames anchor probe scans but never
// satisfy a sig-identity gate.
var fnValueFrameMeta = &FnFrameMeta{Name: "<fn>"}

// FrameOpenInfo is the payload on a fn frame's open paren. The token
// remains an ordinary OpenParen for every structural purpose (IsOpenParen
// is Parent-based, rendering is "(", collapse removes it like any paren);
// the payload adds provenance — which fn overload opened this frame —
// and the frame's resolved-argument span.
type FrameOpenInfo struct {
	Meta *FnFrameMeta
	// ArgSpan is the number of unnamed-argument values spliced at the
	// frame head, immediately after this paren. They were RESOLVED at
	// the call site (forward collection / auto-eval), so the step loop
	// skips the pointer past them — they enter the frame as stack data,
	// exactly like a named binding, and are never re-stepped. Without
	// the skip, an argument with active step semantics (a Function
	// value, an __SP marker) fires on PLACEMENT, making its behaviour
	// depend on which siblings happen to sit beside it — the
	// named/unnamed asymmetry of design/legacy/ARG-SEMANTICS-UNIFICATION.0.ignore.
	ArgSpan int
}

// NewFrameOpen creates the open paren of a spliced fn-body frame,
// carrying the splicing overload's identity.
func NewFrameOpen(meta *FnFrameMeta) Value {
	return NewValueRaw(TOpenParen, FrameOpenInfo{Meta: meta})
}

// NewFrameOpenSpan is NewFrameOpen with the frame's resolved-argument
// span: the count of unnamed args the builder splices directly after
// the paren, which the step loop skips (see FrameOpenInfo.ArgSpan).
func NewFrameOpenSpan(meta *FnFrameMeta, argSpan int) Value {
	return NewValueRaw(TOpenParen, FrameOpenInfo{Meta: meta, ArgSpan: argSpan})
}

// IsFrameOpen reports whether v is a fn frame's open paren (as opposed
// to a plain grouping paren).
func IsFrameOpen(v Value) bool {
	if !IsOpenParen(v) {
		return false
	}
	_, ok := v.Data.(FrameOpenInfo)
	return ok
}

// AsFrameOpen returns the FrameOpenInfo carried by a frame-open paren.
func AsFrameOpen(v Value) (FrameOpenInfo, error) {
	info, ok := v.Data.(FrameOpenInfo)
	if !ok {
		return FrameOpenInfo{}, fmt.Errorf("AsFrameOpen: not a frame-open paren (got %T)", v.Data)
	}
	return info, nil
}

// FrameTailSpec describes one frame's synthesized cleanup tail.
type FrameTailSpec struct {
	// Registry receives the DefCleanup truncation and __pa pops.
	Registry *Registry
	// Snapshot is the def-depth snapshot taken AFTER captures+params
	// were installed (so DefCleanup tears down only body-local defs —
	// and the generic type-param bindings installed after it, see
	// buildFnBodyHandler). Nil when SkipCleanup is set.
	Snapshot map[string]int
	// SkipCleanup marks a body that provably installs no body-local defs:
	// the DefCleanup marker is still emitted (frame shape unchanged) but
	// carries SkipCleanup so stepDefCleanup and TCO short-circuit without a
	// Snapshot map. See buildFnBodyHandler's bodyNeedsFrameState.
	SkipCleanup bool
	// Names are the installed captures+params in install order; the
	// tail undefs them in reverse.
	Names []string
	// Returns / UnnamedCount / FuncName / Pos populate the ReturnCheck;
	// no ReturnCheck is emitted when Returns is empty. A zero Pos means
	// "stamp later": execMatch stamps handler-produced ReturnChecks
	// with the call-site position (stampResultPos keys on Pos.Row == 0).
	Returns []*Type
	// ReturnPatterns mirrors Returns positionally: the structural
	// constraint for a declared return whose *Type degraded to Any.
	ReturnPatterns []*Value
	UnnamedCount   int
	FuncName       string
	Pos            SrcPos
	// Decl is the return contract's declaration site (FnSig.Decl),
	// forwarded onto the ReturnCheck for two-span return errors.
	Decl DeclSite
	// EvalResidual marks a COMPUTING body (BodyEvalsResidual) — the
	// DefCleanup marker evaluates the frame's residual pending
	// containers in-frame before the truncation (see
	// DefCleanupInfo.EvalResidual).
	EvalResidual bool
}

// BodyEvalsResidual reports whether a fn body's residual pending
// containers evaluate IN-frame at the DefCleanup marker: any
// multi-token body, and a single paren-expression token — a
// COMPUTATION whose grouping must not change where residual names
// resolve (`[(def z 0 {a: c1})]` binds the param exactly like
// `[def z 0 {a: c1}]`). A single bare literal stays consumer-evaluated:
// that is the pinned no-closures node-binding transparency
// (def-node-binding.tsv §3). Every frame-tail builder and the
// compile-side residual-recording admission route through this ONE
// predicate so the engines cannot disagree about the timing.
func BodyEvalsResidual(body []Value) bool {
	return len(body) > 1 || (len(body) == 1 && IsParenExpr(body[0]))
}

// AppendFrameTail appends the canonical frame cleanup tail to tokens:
// the DefCleanup marker, the __pa word, the undef pairs for
// captures+params (reverse install order, force-forward so undef takes
// the name word that follows rather than a same-typed value from the
// prefix stack), and the ReturnCheck when returns are declared. The
// frame's close paren is NOT appended — callers own the (ₘ … ) pair.
func AppendFrameTail(tokens []Value, spec FrameTailSpec) []Value {
	tokens = append(tokens, NewDefCleanup(DefCleanupInfo{
		Snapshot:     spec.Snapshot,
		Registry:     spec.Registry,
		SkipCleanup:  spec.SkipCleanup,
		EvalResidual: spec.EvalResidual,
	}))
	tokens = append(tokens, NewWord("__pa"))
	for i := len(spec.Names) - 1; i >= 0; i-- {
		tokens = append(tokens,
			NewWordModified("undef", -1, false, true),
			NewWord(spec.Names[i]),
		)
	}
	if len(spec.Returns) > 0 {
		tokens = append(tokens, NewReturnCheck(ReturnCheckInfo{
			FuncName:       spec.FuncName,
			Returns:        spec.Returns,
			ReturnPatterns: spec.ReturnPatterns,
			UnnamedCount:   spec.UnnamedCount,
			Pos:            spec.Pos,
			Decl:           spec.Decl,
		}))
	}
	return tokens
}

// PopFrameArgs is the Go-side expression of the synthesized __pa
// token: pop the per-call Args list and, in lockstep, the enclosing-fn
// baseline pushed at fn entry (closure-capture detection on subsequent
// fn constructions reads the baseline, so the two must move together —
// see eng/go/CLAUDE.md "Per-Call Stacks"). The __pa word's handler
// delegates here; an eager frame teardown can call it directly.
func PopFrameArgs(r *Registry) error {
	if _, err := r.Args.Pop(); err != nil {
		return err
	}
	r.PopFnBaseline()
	return nil
}

// unwindLiveFrames executes the cleanup tails of fn frames that are OPEN
// inside the tape region [from, to) — their frame-open paren already
// stepped (below the pointer) but their synthesized tail not yet reached —
// before a flow-control rewrite (break/continue) discards the region.
// Without this, a break/continue escaping a spliced fn body discards the
// frame's `__DC __pa undef…` tail unexecuted, LEAKING the per-call Args
// list, FnBaseline, and param/capture bindings: the enclosing loop's next
// iteration then reads the dead callee's `args` and bindings (found via
// the module-fnvalue-boundary continue row — the callee's leaked args
// list shadowed the caller's for the rest of the loop).
//
// Frames unwind innermost-first so the per-call stacks pop in the same
// LIFO order normal tail execution produces.
func (e *Engine) unwindLiveFrames(from, to int) {
	if to > e.Tape.Len() {
		to = e.Tape.Len()
	}
	var opens []int
	for i := from; i < to && i < e.Pointer; i++ {
		if !IsFrameOpen(e.Tape.At(i)) {
			continue
		}
		// Live iff the frame's matching close paren is at/after the
		// pointer — the tail between them has not executed.
		depth := 0
		closed := -1
		for j := i; j < to; j++ {
			v := e.Tape.At(j)
			if IsOpenParen(v) {
				depth++
			} else if IsCloseParen(v) {
				depth--
				if depth == 0 {
					closed = j
					break
				}
			}
		}
		if closed < 0 || closed >= e.Pointer {
			opens = append(opens, i)
		}
	}
	for k := len(opens) - 1; k >= 0; k-- {
		e.unwindFrameTail(opens[k], to)
	}
}

// unwindFrameTail replays the canonical cleanup tail of the frame opened
// at openIdx: the __DC marker (body-local def truncation), the __pa word
// (Args + FnBaseline pop), and the force-forward `undef name` pairs
// (capture/param teardown), exactly as AppendFrameTail laid them out.
// Only DIRECT children of the frame paren are executed (depth 1) — a
// nested live frame's tail is unwound by its own unwindLiveFrames entry,
// innermost-first. The __DC anchor is unambiguous: DefCleanupInfo is a
// machine-generated payload no user source can produce, and the tail's
// undef words carry ForceForward, distinguishing them from user-written
// undef tokens in the (skipped) body region. The ReturnCheck, if any, is
// deliberately NOT executed — an aborted body has no returns to check.
func (e *Engine) unwindFrameTail(openIdx, to int) {
	depth := 0
	for j := openIdx; j < to; j++ {
		v := e.Tape.At(j)
		switch {
		case IsOpenParen(v):
			depth++
		case IsCloseParen(v):
			depth--
			if depth == 0 {
				return
			}
		case depth == 1 && IsDefCleanup(v):
			// Registry-balance replay of a DISCARDED frame: the region's
			// values (any pending residual included) are dropped by the
			// flow resolver, so the in-frame residual eval is skipped —
			// markerIdx -1 disables the scan and no error is possible.
			_ = e.stepDefCleanup(v, -1)
		case depth == 1 && IsWord(v):
			w, _ := AsWord(v)
			switch {
			case w.Name == "__pa":
				_ = PopFrameArgs(e.Registry)
			case w.Name == "undef" && w.ForceForward && j+1 < to && IsWord(e.Tape.At(j+1)):
				nw, _ := AsWord(e.Tape.At(j + 1))
				UninstallDef(e.Registry, nw.Name)
				j++
			}
		}
	}
}
