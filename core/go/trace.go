package core

import (
	"fmt"
	"io"
	"strings"
)

// ANSI color codes for trace output.
const (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cDim     = "\033[2m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cYellow  = "\033[33m"
	cBlue    = "\033[34m"
	cMagenta = "\033[35m"
	cCyan    = "\033[36m"
	cWhite   = "\033[37m"
	cGray    = "\033[90m"
)

// TraceColorize returns a colored string representation of a Value for trace output.
func TraceColorize(v Value) string {
	switch {
	case IsNop(v):
		// In-place compilation's claimed cell (inplace.go).
		return cDim + "·" + cReset
	case IsCall(v):
		// In-place compilation's call cell: the matched dispatch over its
		// operands in signature order.
		ci, _ := AsCall(v)
		parts := make([]string, 0, len(ci.Args())+1)
		parts = append(parts, ci.Name())
		for _, a := range ci.Args() {
			parts = append(parts, TraceColorize(a))
		}
		return cMagenta + "⟨" + cReset + strings.Join(parts, " ") + cMagenta + "⟩" + cReset
	case IsWord(v):
		w, _ := AsWord(v)
		if w.ForceStack {
			return cYellow + w.Name + "/s" + cReset
		}
		if w.ForceForward {
			return cYellow + w.Name + "/f" + cReset
		}
		return cYellow + w.Name + cReset
	case IsForward(v):
		f, _ := AsForward(v)
		// A speculative forward (a slot the plan filled with a word
		// that will dispatch rather than arrive) is marked spec@k so
		// the plan-vs-arrival divergence is visible in traces. See
		// design/FORWARD-COLLECTION-PHASES.10.md.
		if f.Speculative {
			return cMagenta + fmt.Sprintf("→%s(%d/%d spec@%d)",
				f.FuncName, f.CollectedArgs, f.ExpectedArgs, f.SpeculativeAt) + cReset
		}
		return cMagenta + fmt.Sprintf("→%s(%d/%d)", f.FuncName, f.CollectedArgs, f.ExpectedArgs) + cReset
	case IsOpenParen(v):
		return cDim + "(" + cReset
	case v.Data == nil:
		// Type literal — render the value's OWN type name (it IS its
		// lattice node), not its parent (which is the supertype).
		return cCyan + TypeNodeOf(v).String() + cReset
	case v.Parent.ConformsTo(TString):
		// Unwrap via the accessor: v.Data is the sealed StrPayload
		// struct, and %q on the struct renders `{"x"}` not `"x"`.
		s, err := AsString(v)
		if err != nil {
			return cGreen + v.String() + cReset
		}
		return cGreen + fmt.Sprintf("%q", s) + cReset
	case v.Parent.ConformsTo(TInteger):
		n, err := AsInteger(v)
		if err != nil {
			return cBlue + v.String() + cReset
		}
		return cBlue + fmt.Sprintf("%d", n) + cReset
	case v.Parent.ConformsTo(TBoolean):
		_as0, _ := AsBoolean(v)
		if _as0 {
			return cCyan + "true" + cReset
		}
		return cCyan + "false" + cReset
	case v.Parent.ConformsTo(TAtom):
		s, err := AsAtom(v)
		if err != nil {
			return cRed + fmt.Sprintf("%v", v.Data) + cReset
		}
		return cRed + s + cReset
	case v.Parent.ConformsTo(TList):
		_lst, _ := AsList(v)
		elems := _lst.Slice()
		parts := make([]string, len(elems))
		for i, e := range elems {
			parts[i] = TraceColorize(e)
		}
		return cDim + "[" + cReset + strings.Join(parts, " ") + cDim + "]" + cReset
	case v.Parent.ConformsTo(TMap):
		m, _ := AsMap(v)
		if m == nil {
			return cWhite + v.String() + cReset
		}
		parts := make([]string, 0, m.Len())
		for _, k := range m.Keys() {
			val, _ := m.Get(k)
			parts = append(parts, cWhite+k+cReset+cDim+":"+cReset+TraceColorize(val))
		}
		return cDim + "{" + cReset + strings.Join(parts, " ") + cDim + "}" + cReset
	default:
		return cWhite + v.String() + cReset
	}
}

// traceStripANSI returns the visible length of a string, excluding ANSI escape codes.
func TraceVisibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

// The `trace` word registration lives in
// lang/go/engine/native_trace.go. TraceHandler and RunTrace are
// exported algorithm primitives that lang's registration calls into.

func TraceHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return nil, fmt.Errorf("trace: argument must be a concrete list, got type literal")
	}
	_lst, _ := AsList(args[0])
	elems := _lst.Slice()
	return RunTrace(r, elems, r.Output)
}

// RunTrace executes tokens in a sub-engine with tracing enabled,
// printing the stack evolution to w.
func RunTrace(r *Registry, tokens []Value, w io.Writer) ([]Value, error) {
	const termWidth = 120
	const stepWidth = 4 // "NNN "

	type traceStep struct {
		step    int
		pointer int
		stack   []Value
		note    string
	}

	var steps []traceStep

	sub := New(r)
	sub.trace = func(step int, pointer int, stack []Value, note string) {
		steps = append(steps, traceStep{step, pointer, stack, note})
	}

	// C4 attribution (interp_entry.go): a TRACE run is interpretation the end
	// state must PERMIT, not debt. The word exists to print what the engine did
	// step by step, so compiling the body would not speed it up — it would leave
	// nothing to print. That is the same judgement boru:debug's step counter and
	// profiler get, and the opposite of the one `Log.with-span` and
	// `Assert.throws` get, whose answers do not depend on which engine ran the
	// body (test/go/langspec/interp_entry_census_test.go, columns (h)-(j)).
	restoreAtt := r.SetInterpAttribution("debug-observe")
	result, err := sub.Run(tokens)
	restoreAtt()

	// Print header.
	fmt.Fprintf(w, "\n%s%s─── trace ──────────────────────────────────────────────────%s\n", cBold, cCyan, cReset)

	for _, s := range steps {
		// Build left side: resolved stack with pointer marker.
		// Format: [ val val ^val val | pending pending ]
		var leftParts []string
		for i, v := range s.stack {
			tok := TraceColorize(v)
			if i == s.pointer {
				tok = cBold + cWhite + "^" + cReset + tok
			}
			leftParts = append(leftParts, tok)
		}

		// Split into resolved (before pointer) and pending (pointer onward).
		var resolved, pending []string
		for i, part := range leftParts {
			if i < s.pointer {
				resolved = append(resolved, part)
			} else {
				pending = append(pending, part)
			}
		}

		leftResolved := strings.Join(resolved, " ")
		leftPending := strings.Join(pending, " ")

		var leftStr string
		if len(resolved) > 0 && len(pending) > 0 {
			leftStr = leftResolved + " " + cDim + "│" + cReset + " " + leftPending
		} else if len(resolved) > 0 { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			leftStr = leftResolved
		} else {
			leftStr = leftPending
		}

		leftDisplay := cDim + "[" + cReset + " " + leftStr + " " + cDim + "]" + cReset
		leftVisLen := TraceVisibleLen(leftDisplay)

		// Step number: right-aligned, 3 digits.
		stepStr := fmt.Sprintf("%s%3d%s", cGray, s.step, cReset)

		// Format the annotation (right-aligned).
		noteStr := ""
		noteVisLen := 0
		if s.note != "" {
			noteStr = cGray + s.note + cReset
			noteVisLen = TraceVisibleLen(noteStr)
		}

		// Determine if we need wrapping.
		usedLen := stepWidth + leftVisLen
		if usedLen <= termWidth {
			if noteVisLen > 0 && usedLen+2+noteVisLen <= termWidth {
				// Everything fits — right-align the note.
				gap := termWidth - usedLen - noteVisLen
				if gap < 2 { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
					gap = 2
				}
				fmt.Fprintf(w, "%s %s%s%s\n", stepStr, leftDisplay,
					strings.Repeat(" ", gap), noteStr)
			} else {
				// Stack fits, note on next line (right-aligned).
				fmt.Fprintf(w, "%s %s\n", stepStr, leftDisplay)
				if noteVisLen > 0 {
					gap := termWidth - noteVisLen
					if gap < stepWidth+1 {
						gap = stepWidth + 1
					}
					fmt.Fprintf(w, "%s%s\n", strings.Repeat(" ", gap), noteStr)
				}
			}
		} else {
			// Multi-line: wrap the stack display.
			lines := TraceWrap(leftParts, s.pointer, termWidth-stepWidth-4)
			for i, line := range lines {
				if i == 0 {
					fmt.Fprintf(w, "%s %s\n", stepStr, line)
				} else {
					fmt.Fprintf(w, "%s %s\n", cGray+"   "+cReset, line)
				}
			}
			if noteVisLen > 0 {
				gap := termWidth - noteVisLen
				if gap < stepWidth+1 {
					gap = stepWidth + 1
				}
				fmt.Fprintf(w, "%s%s\n", strings.Repeat(" ", gap), noteStr)
			}
		}
	}

	// Print final state.
	if result != nil {
		var resParts []string
		for _, v := range result {
			resParts = append(resParts, TraceColorize(v))
		}
		fmt.Fprintf(w, "%s%s─── result: %s%s[ %s %s]%s\n\n",
			cBold, cCyan, cReset,
			cDim, strings.Join(resParts, " "), cDim, cReset)
	}

	if err != nil {
		fmt.Fprintf(w, "%s%s─── error: %s%s\n\n", cBold, cRed, err.Error(), cReset)
		return nil, err
	}

	return result, nil
}

// TraceWrap wraps a list of colored tokens into multiple display lines,
// each fitting within maxWidth visible characters. Maintains the [ ] and | framing.
func TraceWrap(parts []string, pointer int, maxWidth int) []string {
	if maxWidth < 20 {
		maxWidth = 20
	}

	var lines []string
	var curLine []string
	curLen := 4 // account for "[ " prefix and " ]" suffix
	isFirst := true

	flush := func() {
		if len(curLine) == 0 { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return
		}
		content := strings.Join(curLine, " ")
		if isFirst {
			lines = append(lines, cDim+"["+cReset+" "+content)
			isFirst = false
		} else {
			lines = append(lines, "  "+content)
		}
		curLine = nil
		curLen = 2 // indent for continuation
	}

	addedSep := false
	for i, part := range parts {
		// Insert separator between resolved and pending.
		if i == pointer && !addedSep && i > 0 {
			sep := cDim + "│" + cReset
			curLine = append(curLine, sep)
			curLen += 2
			addedSep = true
		}

		partLen := TraceVisibleLen(part)
		if curLen+partLen+1 > maxWidth && len(curLine) > 0 {
			flush()
		}
		curLine = append(curLine, part)
		curLen += partLen + 1
	}

	// Close the last line with ]
	if len(curLine) > 0 {
		content := strings.Join(curLine, " ")
		if isFirst {
			lines = append(lines, cDim+"["+cReset+" "+content+" "+cDim+"]"+cReset)
		} else {
			lines = append(lines, "  "+content+" "+cDim+"]"+cReset)
		}
	} else if len(lines) > 0 { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
		lines[len(lines)-1] += " " + cDim + "]" + cReset
	} else {
		lines = append(lines, cDim+"[ ]"+cReset)
	}

	return lines
}
