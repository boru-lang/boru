// Package docexamples extracts and runs the `expr  # returns result`
// examples embedded in boru's prose documentation (README, REFERENCE,
// TUTORIAL, HOWTO, EXPLANATION) so the docs can't silently drift from
// real engine behavior. The extractor (this file) is pure
// text→[]Example with no engine dependency, so it is unit-testable in
// isolation; the runner (docexamples_test.go) evaluates each Example
// through the production language layer and compares the rendered stack
// to the documented result.
//
// The documentation convention is a trailing `# returns …` comment
// (never the `=>` arrow, which is real syntax — the anonymous-function
// word `afn`). Two surface forms are recognised inside fenced code
// blocks:
//
//   - inline:        `expr            # returns result`   (all docs)
//   - REPL prompt:    `boru> expr       # returns result`   (TUTORIAL), and
//     the bare `boru> expr` continuation lines that set up state (e.g.
//     `def`) consumed by a later `# returns` line in the same block.
//
// The expected value is the text after `returns`. An optional human
// description may follow it after an em-dash, which the extractor
// ignores: `# returns 5 5 — duplicate top` asserts `5 5`. A documented
// result of `error` / `build error` / `error: …` / `[boru/…]` means the
// example is expected to fail (loose substring match), not to equal a
// rendered value.
package docexamples

import "strings"

// Example is one extracted documentation example.
type Example struct {
	File string // source file basename, e.g. "REFERENCE.md"
	Line int    // 1-based line number of the `# returns` line in File

	// Program is the full boru source to evaluate: the block's preceding
	// setup lines (everything before this line that was not itself a
	// `# returns` line) joined by newlines, then the expression on this
	// line. Setup-only result lines are deliberately excluded so a block
	// with several result lines doesn't pile multiple values on the stack.
	Program string

	// Expr is just this line's left-hand expression (for subtest names
	// and diagnostics).
	Expr string

	// Expected is the documented value (the text after `# returns`, with
	// any ` — description` suffix removed). Meaningful only when WantErr
	// is false.
	Expected string

	// WantErr is true when the documented result denotes a failure
	// (`error`, `build error`, `error: …`). ErrSubstr, when non-empty,
	// is the text after `error:` that the actual error must contain.
	WantErr   bool
	ErrSubstr string
}

// skipMarker on the line directly above a fence opts that whole block
// out of extraction (used for the handful of non-runnable examples:
// fetch/network, time/random, file/import, ellipsis output, multi-value
// stacks, and syntax-template fragments).
const skipMarker = "<!-- boru-test: skip -->"

// returnsKeyword introduces an asserted result inside a trailing
// comment. A comment that does not begin with it is treated as an
// ordinary description and the line is not asserted.
const returnsKeyword = "returns"

// descSep separates the asserted value from an optional human-readable
// description in a `# returns` comment (`# returns 5 5 — duplicate top`).
// Em-dash never appears in rendered boru values, so splitting on it is
// unambiguous.
const descSep = "—"

// Extract pulls every checkable `# returns` example out of one markdown
// file. file is the basename recorded on each Example; src is the body.
func Extract(file, src string) []Example {
	var out []Example
	lines := strings.Split(src, "\n")

	inFence := false
	skipBlock := false
	prevNonBlank := "" // last non-blank line seen before the current fence

	// setup accumulates the binding (`def`/`undef`) lines of the current
	// block, in order, so a later `# returns` line can prepend them as
	// shared state. setupOpen tracks unbalanced `[`/`(` from a multi-line
	// `def` so its continuation lines are captured too.
	var setup []string
	setupOpen := 0

	// prevCode/prevLine hold the most recent non-result expression line so
	// a following bare `# returns result` (result on its own line) can
	// attach to it.
	prevCode := ""
	prevLine := 0

	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)

		// Fence boundary: a line whose first non-space content is ```.
		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				// Opening fence. Only plain (untagged) or ```boru blocks
				// carry boru examples; ```bash / ```text etc. are skipped.
				info := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				skipBlock = (info != "" && info != "boru") ||
					strings.Contains(prevNonBlank, skipMarker)
				inFence = true
				setup = setup[:0]
				setupOpen = 0
				prevCode, prevLine = "", 0
			} else {
				// Closing fence.
				inFence = false
				skipBlock = false
			}
			prevNonBlank = trimmed
			continue
		}

		if !inFence {
			if trimmed != "" {
				prevNonBlank = trimmed
			}
			continue
		}
		if skipBlock {
			continue
		}

		// Inside a runnable fence. Strip a `boru> ` / `boru>` prompt.
		line := stripPrompt(raw)
		code := strings.TrimSpace(line)
		if code == "" {
			continue
		}

		exprPart, rhs, isResult := splitReturns(code)
		if !isResult {
			// A non-result line is shared state for later `# returns`
			// lines ONLY when it's a binding statement (`def`/`undef`/
			// import) or a continuation of one still open across physical
			// lines (multi-line `def … fn [ … ]`). Other bare expressions
			// are illustrative — their result isn't asserted and they must
			// NOT pile onto a following example's stack.
			if isSetupLine(code, setupOpen) {
				setup = append(setup, code)
				setupOpen += bracketDelta(code)
				prevCode, prevLine = "", 0
			} else {
				// Remember it: a following bare `# returns result` line
				// (result on its own line) attaches to this expression.
				prevCode, prevLine = code, i+1
			}
			continue
		}

		expr := strings.TrimSpace(exprPart)
		exprLine := i + 1
		if expr == "" {
			// Result-on-own-line form: `# returns result` whose expression
			// was the preceding code line in this block.
			if prevCode == "" {
				continue // stray result with nothing to evaluate
			}
			expr, exprLine = prevCode, prevLine
		}
		prevCode, prevLine = "", 0

		expected, wantErr, errSub := classifyRHS(rhs)

		program := expr
		if len(setup) > 0 {
			program = strings.Join(setup, "\n") + "\n" + expr
		}

		out = append(out, Example{
			File:      file,
			Line:      exprLine,
			Program:   program,
			Expr:      expr,
			Expected:  expected,
			WantErr:   wantErr,
			ErrSubstr: errSub,
		})
	}

	return out
}

// isSetupLine reports whether a non-result code line is shared state for
// later examples in the same block. True for binding / side-effecting
// statements (`def`, `var`, `undef`, and the `… import end` / `use` module
// forms) and for continuation lines of a multi-line statement still open
// (setupOpen > 0). Other bare expressions are unasserted illustrations
// and must not pile onto a following example's stack.
func isSetupLine(code string, setupOpen int) bool {
	if setupOpen > 0 {
		return true
	}
	first := code
	if sp := strings.IndexAny(first, " \t"); sp >= 0 {
		first = first[:sp]
	}
	switch first {
	case "def", "var", "undef", "use", "context", "set", "ctx-set", "import":
		// `import "mod"` (forward form) and the binding statements (`var`
		// declares the one binding whose value changes in place).
		return true
	}
	// A module-import statement in prefix order — `import "mod"` or
	// `import "mod"` — whose effect (bringing `pkg.word` into scope)
	// later lines rely on. Match `import` as a whole token, not a
	// substring (so `important` / a quoted "import" don't false-match).
	for i, f := range strings.Fields(code) {
		if i > 0 && f == "import" {
			return true
		}
	}
	return false
}

// bracketDelta returns the net change in open `[`/`(` brackets on a line,
// ignoring brackets inside single-quoted strings. Used to keep capturing
// the continuation lines of a multi-line `def … fn [ … ]`.
func bracketDelta(code string) int {
	delta, inStr := 0, false
	for i := 0; i < len(code); i++ {
		switch code[i] {
		case '\'':
			inStr = !inStr
		case '[', '(':
			if !inStr {
				delta++
			}
		case ']', ')':
			if !inStr {
				delta--
			}
		}
	}
	return delta
}

// stripPrompt removes a leading `boru> ` (or bare `boru>`) REPL prompt,
// preserving the rest of the line verbatim.
func stripPrompt(line string) string {
	t := strings.TrimLeft(line, " \t")
	if rest, ok := strings.CutPrefix(t, "boru>"); ok {
		return strings.TrimPrefix(rest, " ")
	}
	return line
}

// splitReturns splits a code line on its trailing `# returns …` result
// comment. Returns (expr, value, true) when the line's first unquoted
// `#` introduces a comment beginning with the `returns` keyword; the
// `value` is the comment text after `returns`, with any ` — description`
// suffix stripped. Otherwise returns ("", "", false) — the line is a
// setup statement or an unasserted illustration.
func splitReturns(code string) (expr string, value string, isResult bool) {
	hash := firstUnquotedHash(code)
	if hash < 0 {
		return "", "", false
	}
	comment := strings.TrimSpace(code[hash+1:])
	rest, ok := strings.CutPrefix(comment, returnsKeyword)
	if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
		// Comment is an ordinary description, not a `returns` assertion.
		return "", "", false
	}
	value = strings.TrimSpace(rest)
	if idx := strings.Index(value, descSep); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return strings.TrimSpace(code[:hash]), value, true
}

// firstUnquotedHash returns the byte index of the first `#` that is not
// inside a single- or double-quoted string, or -1 if there is none.
func firstUnquotedHash(s string) int {
	var quote byte // 0 when outside a string, else the open quote char
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '#':
			return i
		}
	}
	return -1
}

// classifyRHS interprets the documented value. It decides whether the
// example expects an error (`error`, `build error`, `error: …`, or an
// `[boru/code]` notation) or an exact rendered value.
func classifyRHS(rhs string) (expected string, wantErr bool, errSubstr string) {
	rhs = strings.TrimSpace(rhs)

	low := strings.ToLower(rhs)
	switch {
	case strings.HasPrefix(low, "error:"):
		return "", true, strings.TrimSpace(rhs[len("error:"):])
	case low == "error" || low == "build error":
		return "", true, ""
	case strings.HasPrefix(rhs, "[boru/"):
		// Documented error-code notation, e.g.
		// `[boru/type_error] return value 1: expected Integer got …`.
		// Match on the bracketed code (`boru/type_error`) — the prose
		// after it is illustrative and not the engine's exact wording.
		if end := strings.IndexByte(rhs, ']'); end > 0 {
			return "", true, rhs[1:end]
		}
		return "", true, ""
	default:
		return rhs, false, ""
	}
}
