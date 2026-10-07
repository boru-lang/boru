package core

// Block scopes (design/IMMUTABLE-DEF.1.md §2.1, phase 2): a code body a word
// runs — a branch arm, a loop body, a callback body — is a scope of its own,
// live for ONE execution of the body. A `def` or a declaring `var` inside it
// binds a block local that ends with the block; a `var NAME v` over a var
// visible in the frame assigns that cell (bindVar) and binds nothing. The
// enclosing scope's bindings are untouched when the block ends: `def n 0
// for 3 [def n (n add 1)] n` answers 0 (the body's `def n` is a shadow that
// read the enclosing n, the shadow_rebind warning), and `if b [def z 9] []
// end z` is an undefined_word (NUR204 generalised from the loop index to
// every name a body binds). `do`'s body is the one exception, transparent by
// ruling (#11): InvokeBodyKeepDefs.
//
// The seams: InvokeBody for every handler-driven body, stepMoveIf for an
// inline branch arm (a BlockEnd marker closes it), the loop drivers for a
// for/while body region (one scope per iteration, closed when the
// iteration's values are collected or a break abandons it), and the case
// arm runners. Each is gated on BodyBindsLocals — a body that binds nothing
// costs nothing, as a frame that binds nothing skips its cleanup
// (DefCleanupInfo.SkipCleanup).

// BodyBindsLocals reports whether a code body may bind a name of its own: a
// `def` or `var` word at any code depth, through nested code lists and
// groups but not quoted data or an already-built fn value (WalkBodyWords's
// descent). A conservative over-approximation, as bodyNeedsFrameState is: a
// `var` that turns out to assign an enclosing cell opens a block that pops
// nothing, and a def inside a nested lambda literal counts for the body
// that holds the literal.
func BodyBindsLocals(toks []Value) bool {
	binds := false
	WalkBodyWords(toks, func(w WordInfo, _ Value) {
		if w.Name == "def" || w.Name == "var" {
			binds = true
		}
	})
	return binds
}

// bodyBindsLocals is BodyBindsLocals over a body VALUE, memoised per body
// identity on the registry: a callback body runs once per element, and the
// scan is the same every time. A body with no identity (a value built by
// hand) is scanned each time; the memo is dropped when it grows past
// blockBindsMemoCap, so a run minting bodies forever stays bounded.
func (r *Registry) bodyBindsLocals(body Value) bool {
	if body.ID == "" {
		return BodyBindsLocals(BodyTokens(body))
	}
	if r.blockBinds != nil {
		if binds, ok := r.blockBinds[body.ID]; ok {
			return binds
		}
	}
	binds := BodyBindsLocals(BodyTokens(body))
	if r.blockBinds == nil || len(r.blockBinds) >= blockBindsMemoCap {
		r.blockBinds = map[string]bool{}
	}
	r.blockBinds[body.ID] = binds
	return binds
}

// blockBindsMemoCap bounds the per-registry body-binds memo.
const blockBindsMemoCap = 4096

// EnterBlock opens a block scope for one execution of a body and returns
// its id, the handle LeaveBlock closes it by.
func EnterBlock(r *Registry) int32 {
	return r.Defs.EnterScope(ScopeBlock)
}

// LeaveBlock closes the block scope id — and every scope opened inside it
// that an unwinding left open — and pops every binding those scopes made:
// the block's locals end with it. A binding whose scope is still open
// stands (an assignment replaced a cell in place and pushed nothing; a
// frame binding belongs to its frame). Pops run through UninstallDef and
// UninstallType, so the bind ledger, the rebind notification and a minted
// type's retirement follow as they do for an `undef`. A no-op when id is
// not open: a BlockEnd marker a frame's eager tail-call teardown left on
// the tape finds its scope already closed by the frame's PopFnBaseline.
func LeaveBlock(r *Registry, id int32) {
	names, ok := r.Defs.LeaveScopeTo(id)
	if !ok {
		return
	}
	census := r.Check.IsActive()
	for _, name := range names {
		for {
			e, has := r.Defs.TopEntry(name)
			if !has || r.Defs.ScopeOpen(e.Scope) {
				break
			}
			if census {
				// The census's leak-read question, for a read after the
				// block (def_census.go noteUnboundReadCensus).
				r.Defs.NoteBlockBound(name, BlockBoundNote{Site: e.Site, Note: bindingKind(e)})
			}
			uninstallBinding(r, name)
		}
	}
}

// uninstallBinding pops name's top binding as its kind requires: a TYPE
// binding through UninstallType — a node the binding minted is retired, so
// a block or frame that defs a type per run (`each [def ZB (Integer gt e) 7]
// …`) leaves no node behind to conflict with the next run's — and any other
// binding through UninstallDef.
func uninstallBinding(r *Registry, name string) {
	if r.Defs.IsType(name) {
		UninstallType(r, name)
		return
	}
	UninstallDef(r, name)
}

// RunBlockResolved runs `inputs… tokens…` as one execution of a BLOCK: the
// inputs enter as resolved stack data (RunResolved), the body's own bindings
// end with the run. The interpreter half of InvokeBody; the case arm
// runners take it directly.
func RunBlockResolved(r *Registry, inputs, tokens []Value) ([]Value, error) {
	id := EnterBlock(r)
	res, err := RunResolved(r, inputs, tokens)
	LeaveBlock(r, id)
	return res, err
}

// RunBodyResolved is RunBlockResolved gated on the body binding anything:
// a body that binds nothing runs on the plain seam at no cost.
func RunBodyResolved(r *Registry, body Value, inputs []Value) ([]Value, error) {
	toks := BodyTokens(body)
	if r.bodyBindsLocals(body) {
		return RunBlockResolved(r, inputs, toks)
	}
	return RunResolved(r, inputs, toks)
}

// RunBlockTokens runs tokens on a fresh engine as one execution of a block
// (gated as RunBodyResolved is): the scrutinee body of `case`, a test's
// body.
func RunBlockTokens(r *Registry, tokens []Value) ([]Value, error) {
	if !BodyBindsLocals(tokens) {
		return New(r).Run(tokens)
	}
	id := EnterBlock(r)
	res, err := New(r).Run(tokens)
	LeaveBlock(r, id)
	return res, err
}
