// Package calc is a small CLI calculator built directly on the core kernel.
//
// It serves two roles:
//
//   - A useful tool — type expressions like `add 2 mul 3 4` and see the
//     result; carry a stack across REPL lines; use named constants like
//     pi and e.
//
//   - A test case for the kernel boundary — calc imports only
//     github.com/boru-lang/boru/core/go and …/parser/go (kernel + parser)
//     and defines every word it needs (see words.go). If calc compiles,
//     runs, and tests pass, the core module is a self-sufficient algorithm
//     library.
//
// The vocabulary is deliberately small (arithmetic, stack ops, two
// constants, print/show) — enough to be useful in a REPL, narrow
// enough to read end-to-end. The richer language layer lives in lang/.
package calc

import (
	"fmt"
	"io"
	"strings"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/parser/go"
)

// Calc bundles the registry + engine plus the configured output writer
// so a REPL or test can keep a single instance across many Eval calls.
//
// The stack persists across Eval calls — `1 2` leaves [1 2] on the
// stack and a subsequent `add` collapses them to [3]. This matches the
// REPL's "carry the work-in-progress between lines" intuition.
type Calc struct {
	Registry *core.Registry
	Out      io.Writer
	stack    []core.Value
}

// Test seams (design/TEST-SEAMS.10.md): registry construction and the
// registration-error read cannot fail on a healthy build, so tests
// swap these to drive New's error arms.
var (
	newRegistry = core.NewRegistry
	registryErr = (*core.Registry).Err
)

// New constructs a Calc with a fresh registry and the calculator
// vocabulary installed. out is where `print` and `show` write; pass
// nil to discard (useful when the caller only cares about the
// returned stack).
func New(out io.Writer) (*Calc, error) {
	r, err := newRegistry()
	if err != nil {
		return nil, fmt.Errorf("calc: registry: %w", err)
	}
	RegisterWords(r, out)
	if err := registryErr(r); err != nil {
		return nil, fmt.Errorf("calc: word registration: %w", err)
	}
	r.SetParseFunc(parser.Parse)
	r.InitRootContext()
	r.MarkReady()
	return &Calc{Registry: r, Out: out}, nil
}

// Eval parses src, prepends the current stack as literal seed values,
// runs the program, and stores the resulting stack. The same stack is
// returned to the caller for inspection.
//
// A parse or runtime error leaves the previous stack intact — failed
// evaluations don't corrupt the REPL state.
func (c *Calc) Eval(src string) ([]core.Value, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	seed := make([]core.Value, 0, len(c.stack)+len(values))
	seed = append(seed, c.stack...)
	seed = append(seed, values...)

	result, err := core.NewTop(c.Registry).Run(seed)
	if err != nil {
		return nil, err
	}
	c.stack = result
	return result, nil
}

// Stack returns a copy of the current stack so the caller can inspect
// it without risk of mutating the engine's view.
func (c *Calc) Stack() []core.Value {
	out := make([]core.Value, len(c.stack))
	copy(out, c.stack)
	return out
}

// Reset clears the stack. Equivalent to evaluating `clear` but does
// not parse or dispatch.
func (c *Calc) Reset() {
	c.stack = nil
}

// FormatStack renders a stack as a single space-separated line — the
// same form `show` writes — for callers that want to print the stack
// outside the engine's word machinery.
func FormatStack(stk []core.Value) string {
	if len(stk) == 0 {
		return "(empty)"
	}
	parts := make([]string, len(stk))
	for i, v := range stk {
		parts[i] = v.String()
	}
	return strings.Join(parts, " ")
}
