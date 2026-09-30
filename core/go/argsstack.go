package core

import "errors"

// ArgsStack is the per-call args list stack. Each fn-body invocation
// (CallBoru, execFnDefSig) pushes the caller's args list before
// executing the body; the body retrieves the current list via the
// `args` word. Nested calls push their own list so each level sees
// only its own args.
//
// Extracted from Registry to match the DefTable / TypeTable /
// ContextStack pattern.
//
// An entry may be LAZY (PushLazy): the call's argument values, held
// without the list a reader sees until one reads it — the leaf frame's
// per-call copy costs nothing where no code the frame runs reads `args`,
// and every read still answers the call's real list (NUR350).
type ArgsStack struct {
	stack []Value
	// lazy parallels stack: a non-nil entry is the unmaterialised values
	// of a PushLazy entry, whose stack slot Top fills on first read.
	lazy [][]Value
}

// errArgsStackNil is returned by every method when the receiver is
// nil. The production path constructs the stack through NewRegistry,
// so a nil receiver indicates a misconfigured (zero-initialised or
// partially built) Registry — a programming error that should
// surface rather than be silently ignored.
var errArgsStackNil = errors.New("argsstack: nil stack (registry was not initialised via NewRegistry)")

// NewArgsStack returns an empty args stack.
func NewArgsStack() *ArgsStack {
	return &ArgsStack{}
}

// Push pushes an args list onto the stack. Returns an error only if
// the receiver is nil; empty pushes are not possible.
func (as *ArgsStack) Push(args Value) error {
	if as == nil {
		return errArgsStackNil
	}
	as.stack = append(as.stack, args)
	as.lazy = append(as.lazy, nil)
	return nil
}

// PushLazy pushes the args list made of vals, built only when a reader
// reads it (Top). vals must stay unchanged while the entry is on the stack:
// the caller hands over a copy it owns.
func (as *ArgsStack) PushLazy(vals []Value) error {
	if as == nil {
		return errArgsStackNil
	}
	if vals == nil {
		vals = []Value{} // a nil entry marks an eager one
	}
	as.stack = append(as.stack, Value{})
	as.lazy = append(as.lazy, vals)
	return nil
}

// Pop pops the top args entry. Returns (true, nil) on success,
// (false, nil) when the stack is empty (a normal flow-control signal
// for callers that expect Pop to be a best-effort cleanup), or
// (false, error) when the receiver is nil.
func (as *ArgsStack) Pop() (bool, error) {
	if as == nil {
		return false, errArgsStackNil
	}
	if len(as.stack) == 0 {
		return false, nil
	}
	as.stack = as.stack[:len(as.stack)-1]
	as.lazy[len(as.lazy)-1] = nil
	as.lazy = as.lazy[:len(as.lazy)-1]
	return true, nil
}

// Depth returns the current stack depth. A nil receiver reports 0 (the
// callers pair it with Truncate, which no-ops on nil).
func (as *ArgsStack) Depth() int {
	if as == nil {
		return 0
	}
	return len(as.stack)
}

// Truncate drops entries above depth n — the frame-exit / error-unwind
// rebalance for the compiled args bracket (Program.DynEnv): every VM frame
// records its entry depth and truncates back, mirroring the interpreter's
// per-call push/pop discipline without threading pops through every exit
// path. A no-op when n is at or above the current depth, or on nil.
func (as *ArgsStack) Truncate(n int) {
	if as == nil || n < 0 || n >= len(as.stack) {
		return
	}
	as.stack = as.stack[:n]
	clear(as.lazy[n:])
	as.lazy = as.lazy[:n]
}

// Top returns the current top args entry. Returns (value, true, nil)
// on success, (zero Value, false, nil) when the stack is empty (the
// `args` word relies on this to return an empty list outside any fn
// call), or (zero Value, false, error) when the receiver is nil.
func (as *ArgsStack) Top() (Value, bool, error) {
	if as == nil {
		return Value{}, false, errArgsStackNil
	}
	if len(as.stack) == 0 {
		return Value{}, false, nil
	}
	top := len(as.stack) - 1
	if vals := as.lazy[top]; vals != nil {
		// First read of a lazy entry: build the list once, so every
		// read in the frame answers the one value.
		as.stack[top] = NewList(vals)
		as.lazy[top] = nil
	}
	return as.stack[top], true, nil
}
