package core

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// BoruError is the structured error type for boru engine errors.
// It mirrors the jsonic JsonicError format, providing error codes,
// source location, source extracts, and detailed descriptions.
//
// Format:
//
//	[boru/<code>]: <detail>
//	  --> <row>:<col>
//	  <line> | <source>
//	           ^^^^ <detail>
type BoruError struct {
	Code   string // Error code: "signature_error", "type_error", "syntax_error", etc.
	Detail string // Human-readable detail message
	Row    int    // 1-based line number (0 = unknown)
	Col    int    // 1-based column number (0 = unknown)
	Src    string // Source fragment at the error (the token/word text)
	Hint   string // Additional explanatory text
	// File is the path of the source file the Row/Col point into, when
	// known — set by the executing engine (stampErrPos) from the
	// registry's BaseFile so an error raised inside an IMPORTED module
	// renders `--> /path/mod.boru:130:9` instead of a bare position
	// into an unnamed file (decision DX report finding 4). Empty for
	// -e / REPL input.
	File string
	// Data carries the extra keys of a `raise {code:… message:… …}`
	// spec map, so a catching handler can read them off the Error
	// value (ErrorInfo.Data via NewError). Nil for every other error.
	Data *OrderedMap

	// --- Structured diagnostic payload (design/DIAGNOSTICS.0.md). ---
	// The Row/Col/Src trio above remains the PRIMARY span; the fields
	// below are the additive Rust/Elm-style layer. All render through
	// Render (Error() is the plain-color rendering); message builders
	// (diag_msg.go) populate them so the interpreter, the VM, and the
	// checker share one text source.

	// Spans are SECONDARY labeled source locations — the declaration a
	// constraint came from, the operand that failed a slot, the other
	// half of an expected-vs-found pair. Order is render order.
	Spans []DiagSpan
	// Notes are freestanding explanatory lines (`= note: …`). They
	// supersede the legacy convention of embedding "\n  = " separators
	// inside Hint; Hint still renders (first, unchanged) while call
	// sites migrate.
	Notes []string
	// Suggestions are actionable fixes (`= help: …`), optionally with a
	// concrete replacement snippet.
	Suggestions []DiagSuggestion

	// DeferAlt, set only on a DESIGNED VM defer (internal_error, vmDeferAlt),
	// is the best-effort user-facing raise the defer site prepared for the
	// moment it bails: a no-match defer whose site proved the interpreter would
	// also fail this dispatch builds the rich signature_error over the live
	// window, so the caller surfaces a real diagnostic instead of an internal
	// error telling the user to report a compiler bug. It was the consolation
	// prize for a fence-blocked re-run; with no re-run left it is the ANSWER —
	// the "trap that raises the interpreter's own error at the same moment"
	// disposition (design/SESSION-HANDOVER.0.md). Best-effort: the Detail and candidate
	// verdicts are canonical, but the rendered argument tuple may be wider
	// than the tape-derived tuple the interpreter would show (the fallback
	// arm, when open, stays byte-identical by re-running). Nil everywhere
	// else.
	DeferAlt *BoruError

	// VMDefer marks a DESIGNED VM defer-to-interpreter (vmErrAt / the panic
	// guards), as opposed to a user `raise internal_error …` — the two share
	// the public `internal_error` code, so a consumer that must catch the
	// user error but re-raise the defer (the `do` escape hatch,
	// bodyErrorPropagates) keys on this marker, not the code. Set only by the
	// eng VM's internal_error mints; false on every user-raised error.
	VMDefer bool

	// fullSource is the complete source text for generating context extracts.
	FullSource string
}

// DiagSpan is one secondary labeled source location attached to a
// BoruError. The error's own Row/Col/Src is the primary span (rendered
// with a ^^^ underline); secondaries render with a --- underline and
// their Label. A span whose Pos.Row is 0 has no usable location and
// renders its Label as a plain note instead — the same honesty rule as
// the primary position (no guessed locations).
type DiagSpan struct {
	Pos   SrcPos // Row/Col/Src of the labeled location
	Label string // e.g. "the declaration says `f` returns Integer"
	// File names the source file Pos points into when it differs from
	// the error's own File. Empty = same file as the error.
	File string
	// Source is the full source text Pos indexes into when it differs
	// from the error's own source (a declaration inside an imported
	// module). Empty = same source as the error.
	Source string
}

// DeclSite records where a contract was DECLARED — the fn output
// signature a return check enforces — so the enforcing error can label
// that location as a secondary span (design/DIAGNOSTICS.0.md, phase 5).
// Source/File pin the text Pos indexes into, captured at declaration
// time, because the declaring program (an imported module) may differ
// from the one executing when the check fires. A zero DeclSite means
// the declaration site is unknown (Go-registered sigs, tests).
type DeclSite struct {
	Pos    SrcPos
	Source string
	File   string
}

// DiagSuggestion is one actionable fix attached to a BoruError.
type DiagSuggestion struct {
	Message string `json:"message"` // "did you mean `upper`?"
	// Replacement is a concrete snippet the user can apply verbatim
	// (rendered on its own line; the seed for editor code actions).
	// nil = the suggestion has no mechanical replacement — distinct
	// from an empty string per the no-zero-value-overload rule.
	Replacement *string `json:"replacement,omitempty"`
}

// Error implements the error interface with the PLAIN (color-free)
// rendering — exactly Render(RenderOpts{}). Every string-comparing
// consumer (spec ERROR rows, %w chains, logs, the wasm playground)
// reads this path, so it stays ANSI-free forever; interactive surfaces
// opt into color by calling Render with ResolveColor's verdict.
func (e *BoruError) Error() string {
	return e.Render(RenderOpts{})
}

// SrcPos holds source position information for a value.
// Embedded in Value to enable error messages with source extracts.
type SrcPos struct {
	Row int    // 1-based line number (0 = unknown)
	Col int    // 1-based column number (0 = unknown)
	Src string // source text of the token
}

// MakeBoruError creates a BoruError with no source position. The caller
// has no source token to attribute the error to; the rendered error states
// the position is unknown. Use MakeBoruErrorAt (or thread a Value's .Pos)
// whenever the offending token is in hand.
func MakeBoruError(code, detail, word, fullSource, hint string) *BoruError {
	return makeBoruError(code, detail, word, fullSource, hint)
}

// MakeBoruErrorAt creates a BoruError at an explicit source position — the
// exported twin of makeBoruErrorAt for packages outside eng that hold a
// real position (the parser translating a lexer failure, hosts embedding
// the engine). When pos is unknown (Row 0) the error renders "source
// position unknown"; there is no text-search fallback.
func MakeBoruErrorAt(code, detail, word, fullSource, hint string, pos SrcPos) *BoruError {
	return makeBoruErrorAt(code, detail, word, fullSource, hint, pos)
}

// makeBoruError creates a BoruError with no source position (Row 0).
func makeBoruError(code, detail, word, fullSource, hint string) *BoruError {
	return makeBoruErrorAt(code, detail, word, fullSource, hint, SrcPos{})
}

// makeBoruErrorAt creates a BoruError at an explicit source position. When
// pos is unknown (Row 0) the error carries no location and renders as
// "source position unknown" — there is no text-search fallback, because a
// guessed location is wrong whenever the word appears more than once.
func makeBoruErrorAt(code, detail, word, fullSource, hint string, pos SrcPos) *BoruError {
	src := word
	if pos.Src != "" {
		src = pos.Src
	}
	return &BoruError{
		Code:       code,
		Detail:     detail,
		Row:        pos.Row,
		Col:        pos.Col,
		Src:        src,
		Hint:       hint,
		FullSource: fullSource,
	}
}

// diagMaxListHead is the number of leading elements diagValue shows from
// a container before collapsing the rest into a `… (N more)` marker.
// Keeps a large quoted fn body (often hundreds of tokens) from burying
// an error.
const diagMaxListHead = 8

// diagMaxDepth is how far diagValue descends into nested containers
// before rendering the rest as an elision. Depth needs its own bound
// because a head limit is per-LEVEL: `[[0 1 … 10000]]` has an outer
// length of one, so no head limit anywhere fires, and the whole nested
// run renders. Three levels is enough to recognise a value while
// keeping the worst case bounded. Every level is rendered by the same
// rule, so the bound holds whatever shape the value takes rather than
// depending on where the big part happens to sit.
const diagMaxDepth = 4

// diagMaxRendered is a total character backstop, applied once to the
// finished string. The structural limits above bound the common cases
// legibly, and this catches what they cannot: a single scalar (one
// enormous string) is not a container, so no head or depth limit
// applies to it. Cutting mid-value is not legible, which is why it is a
// backstop rather than the mechanism — it fires only where the
// structural limits have already failed to bound the value.
const diagMaxRendered = 400

// diagValue renders v for inclusion in an error message, bounding
// containers so a quoted code body or other big literal does not flood
// the output. Only the diagnostic surface uses this — ValToString and
// the normal value renderers are untouched, so it never alters real
// output or round-tripping.
//
// A container longer than diagMaxListHead shows its first few entries
// followed by `… (N more)`, and each entry shown is rendered under the
// same rule, down to diagMaxDepth. Bounding only the outer level was
// the original defect: the limit fired on the LENGTH of the value
// handed in, so one short list of enormous elements slipped every
// check. Everything else (including function values, which already
// render compactly via formatFnDef) is shown by its normal String().
func diagValue(v Value) string {
	return diagClamp(diagValueDepth(v, 0))
}

// diagClamp applies the character backstop, marking the cut so a
// shortened rendering never reads as a complete one.
func diagClamp(s string) string {
	if len(s) <= diagMaxRendered {
		return s
	}
	// Cut on a rune boundary so the marker cannot be preceded by half
	// a multi-byte character.
	cut := diagMaxRendered
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func diagValueDepth(v Value, depth int) string {
	if v.Parent == nil || !IsConcrete(v) {
		return v.String()
	}
	isList := v.Parent.ConformsTo(TList)
	isMap := v.Parent.ConformsTo(TMap)
	if !isList && !isMap {
		return v.String()
	}
	if depth >= diagMaxDepth {
		// Deep enough that the shape is all that is still useful.
		if isList {
			return "[…]"
		}
		return "{…}"
	}
	if isList {
		lst, err := AsList(v)
		if err != nil {
			// Conforming to TList and being concrete is not the same as
			// carrying a ListPayload: a TYPED list (`List of [Integer]`)
			// is Parent=TList with a ChildTypeInfo payload, so it lands
			// here. It has no elements to walk and renders itself.
			return v.String()
		}
		elems := lst.Slice()
		parts, _ := diagHead(len(elems), func(i int) string {
			return diagValueDepth(elems[i], depth+1)
		})
		return "[" + strings.Join(parts, " ") + "]"
	}
	m, err := AsMap(v)
	if err != nil {
		// The typed-MAP twin of the branch above.
		return v.String()
	}
	keys := m.Keys()
	parts, _ := diagHead(len(keys), func(i int) string {
		val, _ := m.Get(keys[i])
		return keys[i] + ":" + diagValueDepth(val, depth+1)
	})
	return "{" + strings.Join(parts, " ") + "}"
}

// diagHead renders the first diagMaxListHead of n entries through render
// and appends the elision marker, returning the parts and how many were
// elided. Shared so a list and a map cannot drift apart on the limit.
func diagHead(n int, render func(i int) string) ([]string, int) {
	shown := n
	more := 0
	if shown > diagMaxListHead {
		more = shown - diagMaxListHead
		shown = diagMaxListHead
	}
	parts := make([]string, 0, shown+1)
	for i := 0; i < shown; i++ {
		parts = append(parts, render(i))
	}
	if more > 0 {
		parts = append(parts, "… ("+strconv.Itoa(more)+" more)")
	}
	return parts, more
}

// diagValueList renders the run of values a diagnostic is about, for the
// errors whose subject is a SEQUENCE rather than one value
// (design/DIAGNOSTIC-VALUES.0.md).
// Abbreviated twice over: at most diagMaxListHead values, each through
// diagValue — so one enormous list element cannot swamp the line, and
// neither can a hundred of them.
func diagValueList(vals []Value) string {
	if len(vals) == 0 {
		return "[]"
	}
	parts, _ := diagHead(len(vals), func(i int) string {
		return diagValue(vals[i])
	})
	return "[" + strings.Join(parts, " ") + "]"
}

// describeStackTypes returns a human-readable description of the types
// on the stack around a given position, for inclusion in error messages.
func describeStackTypes(tape *Tape, pointer int) string {
	if tape.Len() == 0 {
		return "stack is empty"
	}
	// Show types of up to 3 values before and after the pointer.
	var parts []string
	start := pointer - 3
	if start < 0 {
		start = 0
	}
	end := pointer + 4
	if end > tape.Len() {
		end = tape.Len()
	}
	for i := start; i < end; i++ {
		v := tape.At(i)
		label := "?"
		if t := ValueType(v); t != nil {
			label = t.String()
		}
		if IsWord(v) {
			w, _ := AsWord(v)
			label = "word(" + w.Name + ")"
		} else if IsAtom(v) {
			a, _ := AsAtom(v)
			label = "atom(" + a + ")"
		} else if s := renderDepScalar(v); s != "" {
			// Render the constraint payload rather than falling
			// into a ConformsTo(TString)/AsString path that would
			// silently produce an empty label.
			label = s
		} else if v.Parent.ConformsTo(TString) {
			s, _ := AsString(v)
			if len(s) > 20 {
				s = s[:20] + "..."
			}
			label = "'" + s + "'"
		} else if v.Parent.ConformsTo(TInteger) {
			n, _ := AsInteger(v)
			label = strconv.FormatInt(n, 10)
		} else if v.Parent.ConformsTo(TFloat) {
			f, _ := AsFloat(v)
			label = formatFloat(f)
		}
		if i == pointer {
			label = ">>>" + label + "<<<"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

// describeSigArgs returns a human-readable description of a signature's
// expected argument types.
func describeSigArgs(sig *Signature) string {
	if sig == nil || sig.TotalArgs() == 0 {
		return "(no args)"
	}
	argTypes := sig.ArgTypes()
	parts := make([]string, len(argTypes))
	for i, t := range argTypes {
		parts[i] = t.String()
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// describeAllSigs returns a summary of all available signatures for a word.
func describeAllSigs(fn *FnDefInfo) string {
	if fn == nil || len(fn.Signatures) == 0 {
		return ""
	}
	var parts []string
	for _, sig := range fn.Signatures {
		parts = append(parts, describeSigArgs(&sig))
	}
	return strings.Join(parts, " or ")
}
