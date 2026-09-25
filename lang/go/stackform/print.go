package stackform

import (
	"fmt"
	"strconv"
	"strings"

	core "github.com/boru-lang/boru/core/go"
)

// Pretty renders a StackForm back to readable boru source. The output
// uses pure post-fix form (args appear on the stack before the word
// that consumes them) — no forward-collection sugar. This is the
// canonical surface representation for inspecting what the reducer
// arrived at.
//
// Quote bodies render as bracketed lists. Calls render as their
// word name. Literals use the engine's canonical string form via
// core.CanonValue when available, falling back to Value.String().
func Pretty(form *StackForm) string {
	if form == nil {
		return ""
	}
	var sb strings.Builder
	pretty(&sb, form, 0)
	return sb.String()
}

func pretty(sb *strings.Builder, form *StackForm, depth int) {
	for i, op := range form.Ops {
		if i > 0 {
			sb.WriteByte(' ')
		}
		switch o := op.(type) {
		case PushLit:
			writeLiteral(sb, o.V)
		case Call:
			sb.WriteString(o.Name)
		case Apply:
			sb.WriteString("apply/" + strconv.Itoa(o.Arity))
		case Quote:
			sb.WriteByte('[')
			if o.Body != nil {
				pretty(sb, o.Body, depth+1)
			}
			sb.WriteByte(']')
		case DoEval:
			sb.WriteString("do")
		default:
			fmt.Fprintf(sb, "<?op:%T>", op)
		}
	}
}

func writeLiteral(sb *strings.Builder, v core.Value) {
	// Strings need quoting; everything else relies on Value.String().
	if v.Parent != nil && v.Parent.ConformsTo(core.TString) {
		s, err := core.AsString(v)
		if err == nil {
			fmt.Fprintf(sb, "%q", s)
			return
		}
	}
	sb.WriteString(v.String())
}
