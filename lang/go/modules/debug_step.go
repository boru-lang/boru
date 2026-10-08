package modules

import (
	"fmt"
	"strings"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
)

// This file implements the interactive-stepping surface of boru:debug
// (design/DEBUG-MODULE.0.md §3 B, Phase 3): Debug.step / break / break-when
// / run-stepped. Stepping is driven by the engine's per-step TraceCallback
// (installed via Engine.SetTrace) and a host StepController obtained from
// the DebugOps capability. With no DebugOps installed, Debug.step falls
// back to printing each frame (a trace) and breakpoints are no-ops — so
// leaving a Debug.break in committed code is harmless in production.
//
// StepQuit detaches the stepper (no further pauses) rather than preempting
// the run: the engine offers no mid-Run interruption seam, and the
// codebase forbids control-flow panics, so "quit" means "stop interacting,
// let it finish" — the conventional debugger detach.

const debugBreakWord = "debug-break"

// stepNatives returns the interactive-stepping primitives. They are added
// to debugNatives()'s slice (see debug.go).
func stepNatives() []native.NativeFunc {
	return native.SideEffecting([]native.NativeFunc{
		{
			// Run a quoted body under interactive single-step control.
			Name: "debug-step",
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TList},
				Returns:    []*native.Type{native.TAny},
				NoEvalArgs: map[int]bool{0: true},
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					body, err := native.RequireConcreteList(args[0], "Debug.step")
					if err != nil {
						return nil, err
					}
					return runStepped(r, append([]native.Value(nil), body.Slice()...))
				}),
			}},
		},
		{
			// A breakpoint: pause here when a controller is attached; else a no-op.
			Name: debugBreakWord,
			Signatures: []native.Signature{{
				Args:       []*native.Type{},
				Returns:    []*native.Type{},
				BarrierPos: -1,
				Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					pauseAtBreak(r)
					return nil, nil
				}),
			}},
		},
		{
			// A conditional breakpoint: pause only when the condition is true.
			Name: "debug-break-when",
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TBoolean},
				Returns:    []*native.Type{},
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					cond, err := args[0].AsConcreteBoolean()
					if err != nil {
						return nil, err
					}
					if cond {
						pauseAtBreak(r)
					}
					return nil, nil
				}),
			}},
		},
		{
			// Parse a source string and step it (the REPL/CLI entry point).
			Name: "debug-run-stepped",
			Signatures: []native.Signature{{
				Args:       []*native.Type{native.TString},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
				Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
					src, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					if r.ParseFunc == nil {
						return nil, r.BoruError("debug_error", "Debug.run-stepped: parser not configured", "Debug.run-stepped")
					}
					tokens, perr := r.ParseFunc(src)
					if perr != nil {
						return nil, r.BoruError("parse_error", fmt.Sprintf("Debug.run-stepped: %v", perr), "Debug.run-stepped")
					}
					return runStepped(r, tokens)
				}),
			}},
		},
	})
}

// runStepped runs tokens in a sub-engine under single-step control. Each
// step renders the frame and consults the controller; with no DebugOps it
// prints the frame and continues (a trace fallback).
func runStepped(parent *native.Registry, tokens []native.Value) ([]native.Value, error) {
	ops, hasOps := native.EffectiveDebugOps(parent)
	sub := native.New(parent)
	single := true    // start paused at the first step
	detached := false // set by StepQuit — stop pausing, let it finish

	sub.SetTrace(func(step int, pointer int, stack []native.Value, _ string) {
		if detached {
			return
		}
		atBreak := isBreakAt(stack, pointer)
		if !single && !atBreak {
			return
		}
		frame := capabilities.StepFrame{
			Step:    step,
			Pointer: pointer,
			Stack:   renderTape(stack),
			AtBreak: atBreak,
			File:    parent.BaseFile,
		}
		// Locate the value about to execute in its source (0 = unknown for
		// synthetic tokens) so source-aware controllers can render the pause.
		if pointer >= 0 && pointer < len(stack) {
			pos := stack[pointer].Pos()
			frame.Row, frame.Col = pos.Row, pos.Col
		}
		if !hasOps {
			fmt.Fprintln(parent.Output, renderFrameLine(frame))
			return
		}
		switch ops.Controller().OnStep(frame) {
		case capabilities.StepInto:
			single = true
		case capabilities.StepContinue:
			single = false
		case capabilities.StepQuit:
			detached = true
		}
	})

	// C4 attribution: a single-stepping debugger observes the interpreter by
	// definition — the trace hook above is what pauses at each step and each
	// breakpoint. There is no compiled equivalent to step through.
	restoreAtt := parent.SetInterpAttribution("debug-observe")
	res, err := sub.Run(tokens)
	restoreAtt()
	if err != nil {
		return nil, err
	}
	return []native.Value{lastOrNone(res)}, nil
}

// pauseAtBreak handles a Debug.break / Debug.break-when hit during ordinary
// execution: if a controller is attached, render the current data stack and
// consult it (a one-shot pause); otherwise it is a no-op (production-safe).
func pauseAtBreak(r *native.Registry) {
	ops, ok := native.EffectiveDebugOps(r)
	if !ok {
		return
	}
	stack, _ := r.CurrentStack()
	ops.Controller().OnStep(capabilities.StepFrame{
		Step:    -1,
		Pointer: len(stack),
		Stack:   renderValues(stack),
		AtBreak: true,
		// A one-shot pause fires inside the handler, with no tape pointer in
		// hand — Row/Col stay 0 (unknown); the file is still the registry's.
		File: r.BaseFile,
	})
}

// isBreakAt reports whether the tape value about to execute is a Debug.break
// marker word.
func isBreakAt(stack []native.Value, pointer int) bool {
	if pointer < 0 || pointer >= len(stack) {
		return false
	}
	v := stack[pointer]
	if !native.IsWord(v) {
		return false
	}
	w, err := native.AsWord(v)
	return err == nil && w.Name == debugBreakWord
}

// renderTape renders a tape snapshot one entry per slot for a StepFrame.
func renderTape(stack []native.Value) []string {
	return renderValues(stack)
}

func renderValues(vals []native.Value) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = native.FormatForPrint(v)
	}
	return out
}

// renderFrameLine formats a frame as a single trace line for the no-controller
// fallback: "  <step>  [ a b ^c d ]".
func renderFrameLine(f capabilities.StepFrame) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%4d  [ ", f.Step)
	for i, s := range f.Stack {
		if i == f.Pointer {
			b.WriteString("^")
		}
		b.WriteString(s)
		if i < len(f.Stack)-1 {
			b.WriteString(" ")
		}
	}
	b.WriteString(" ]")
	return b.String()
}
