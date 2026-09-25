package basic

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// RunForLoop builds the mark+body+move tokens for a for loop and returns
// them. The engine splices these onto the stack and processes them; the
// move's ForCont drives subsequent iterations via stepMoveCont.
//
// Break and continue use sentinel errors caught by the engine's Run loop,
// which delegates to handleLoopBreak/handleLoopContinue.
func RunForLoop(r *Registry, start, end, step int64, iterName string, body Value) ([]Value, error) {
	if step == 0 {
		return nil, r.BoruError("for_error", "for: step cannot be zero", "for")
	}
	if step > 0 && start >= end {
		return nil, nil
	}
	if step < 0 && start <= end {
		return nil, nil
	}

	if !IsConcrete(body) {
		return nil, r.BoruError("for_error", "for: body must be a concrete list, got type literal", "for")
	}
	// Under a compiled run (the registry carries the VM's invoker) the
	// handler is reached as a plain CALL_NATIVE for a COMPUTED body the
	// recorder could not lower (`for 3 (mk 0)`, code-bodies.tsv L141; the
	// dyn-body path, 2026-09-25): a tape-coupled result has no tape to run
	// on there, so the loop is HOSTED — each iteration runs the body through
	// the InvokeBody seam (the run-time token-body host) with the index
	// installed as the interpreter installs it, the completed iterations'
	// values accumulate, and an escaped body (break / continue, the flag
	// InvokeBody leaves) discards that iteration's partial values exactly as
	// handleLoopBreak / handleLoopContinue splice them away.
	if r.Invoker != nil {
		return runHostedForLoop(r, start, end, step, iterName, body)
	}
	_lst, _ := AsList(body)
	bodySlice := _lst.Slice()

	// Install the iterator variable for the first iteration.
	InstallDef(r, iterName, NewInteger(start))

	// Create the continuation state.
	bodyCopy := make([]Value, len(bodySlice))
	copy(bodyCopy, bodySlice)

	cont := &ForCont{
		Registry: r,
		IterName: iterName,
		Current:  start,
		End:      end,
		Step:     step,
		Body:     bodyCopy,
	}

	// Build the stack segment: mark + body + move.
	id := NextMarkID()
	tokens := make([]Value, 0, len(bodySlice)+2)
	tokens = append(tokens, NewMark(id, bodySlice...))
	bodyTokens := make([]Value, len(bodySlice))
	copy(bodyTokens, bodySlice)
	tokens = append(tokens, bodyTokens...)
	tokens = append(tokens, NewMoveCont(id, "for loop", cont))

	return tokens, nil
}

// LoopIterations reports how many times a `for` with the given decoded range
// runs its body. It mirrors RunForLoop's empty-span guards exactly (a
// non-advancing or wrong-direction span runs zero times) so the static
// zero-count pruning in forCarrierAnalyse agrees with the interpreter. A zero
// step is treated as "non-static" (returns -1): RunForLoop raises on it, so it
// must NOT be pruned as empty.
func LoopIterations(start, end, step int64) int64 {
	if step == 0 {
		return -1
	}
	if step > 0 {
		if start >= end {
			return 0
		}
		return (end - start + step - 1) / step
	}
	if start <= end {
		return 0
	}
	return (start - end + (-step) - 1) / (-step)
}

// ParseRange parses a range specification list into start, end, step.
//
//	[end]              — 0 to end, step 1
//	[start, end]       — start to end, step 1
//	[start, end, step] — start to end, step
func ParseRange(elems []Value) (start, end, step int64, err error) {
	// AsConcreteInteger rejects non-Integer values AND DepScalar/carrier
	// payloads — matching the VM's OpForSetup (eng/go/vm.go), which reads each
	// bound with the same concrete accessor. Using it (rather than a
	// ConformsTo check plus an error-swallowing AsInteger) means a non-concrete
	// bound errors in the interpreter exactly as it does in the VM, instead of
	// being silently coerced to zero.
	intAt := func(v Value) (int64, error) {
		n, e := v.AsConcreteInteger()
		if e != nil {
			return 0, fmt.Errorf("range: expected a concrete integer, got %s", v.Parent)
		}
		return n, nil
	}
	switch len(elems) {
	case 1:
		if end, err = intAt(elems[0]); err != nil {
			return 0, 0, 0, err
		}
		return 0, end, 1, nil
	case 2:
		if start, err = intAt(elems[0]); err != nil {
			return 0, 0, 0, err
		}
		if end, err = intAt(elems[1]); err != nil {
			return 0, 0, 0, err
		}
		return start, end, 1, nil
	case 3:
		if start, err = intAt(elems[0]); err != nil {
			return 0, 0, 0, err
		}
		if end, err = intAt(elems[1]); err != nil {
			return 0, 0, 0, err
		}
		if step, err = intAt(elems[2]); err != nil {
			return 0, 0, 0, err
		}
		return start, end, step, nil
	default:
		return 0, 0, 0, fmt.Errorf("range: expected 1-3 elements, got %d", len(elems))
	}
}

// break and continue words now live in eng/go/flowctrl.go; they raise
// a FlowCtrl signal on the Registry rather than returning a sentinel
// error. FlowCtrl / FlowBreak / FlowContinue are re-exported via
// aliases.go.

// runHostedForLoop is RunForLoop's compiled-run twin: the counted loop driven
// in Go over the InvokeBody seam (see RunForLoop). The index binding follows
// the interpreter's discipline — installed for the first iteration, popped
// and re-installed per iteration (DefStacks depth 1 throughout), uninstalled
// at the end or at a break.
func runHostedForLoop(r *Registry, start, end, step int64, iterName string, body Value) ([]Value, error) {
	var out []Value
	InstallDef(r, iterName, NewInteger(start))
	for cur := start; (step > 0 && cur < end) || (step < 0 && cur > end); cur += step {
		if cur != start {
			UninstallDef(r, iterName)
			InstallDef(r, iterName, NewInteger(cur))
		}
		res, err := InvokeBody(r, body, nil)
		if err != nil {
			UninstallDef(r, iterName)
			return nil, err
		}
		if core.BodyEscaped(r) {
			fc := r.FlowCtrl
			r.FlowCtrl = FlowNone
			if fc == FlowBreak {
				break
			}
			continue
		}
		out = append(out, res...)
	}
	UninstallDef(r, iterName)
	return out, nil
}
