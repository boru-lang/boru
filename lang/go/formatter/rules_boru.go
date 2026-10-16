package formatter

import (
	_ "embed"
	"fmt"
	"sync"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
)

// The canonical rule table is EXPRESSED IN boru — fmt-rules.boru is the
// stylesheet, exactly as an XSLT stylesheet is expressed in XML. This file
// is the processor's loader: it parses the embedded stylesheet into the
// compiled Rules form the renderer interprets. The Go struct is not the
// definition; the boru source is.
//
//go:embed fmt-rules.boru
var rulesSource string

var (
	defaultRulesOnce sync.Once
	defaultRules     Rules
	defaultRulesErr  error
)

func loadDefaultRules() {
	defaultRules, defaultRulesErr = parseRulesSource(rulesSource)
}

// DefaultRules is the canonical boru layout: the compiled form of the
// fmt-rules.boru stylesheet (parsed once, at first use). If the embedded
// stylesheet failed to parse — a build defect, pinned against by
// TestDefaultRulesFromBoru — this returns the zero Rules and
// DefaultRulesInitError reports the failure; BuildFmtModule surfaces it at
// module construction (the ADR-005 no-panic pattern).
func DefaultRules() Rules {
	defaultRulesOnce.Do(loadDefaultRules)
	return defaultRules
}

// DefaultRulesInitError returns the error from parsing the embedded
// fmt-rules.boru stylesheet, or nil. The boru:fmt module surfaces it at
// build time rather than formatting with a zero table.
func DefaultRulesInitError() error {
	defaultRulesOnce.Do(loadDefaultRules)
	return defaultRulesErr
}

// SwapDefaultRulesErr swaps the recorded stylesheet init error and returns
// the previous value — a TEST SEAM (mirroring native.SwapTypeInitErrs) so
// the surfaced-at-construction path is coverable; never call it outside a
// test.
func SwapDefaultRulesErr(err error) error {
	defaultRulesOnce.Do(loadDefaultRules)
	prev := defaultRulesErr
	defaultRulesErr = err
	return prev
}

// parseRulesSource parses a boru stylesheet — comments, then one map
// literal carrying ALL rule keys — into a complete Rules table. Used for
// the embedded canonical stylesheet; strictness (every key required)
// guarantees the file fully defines the style, with no silent Go-side
// defaulting.
func parseRulesSource(src string) (Rules, error) {
	vals, err := parser.Parse(src)
	if err != nil {
		return Rules{}, fmt.Errorf("fmt rules stylesheet: %w", err)
	}
	var table *core.Value
	for i := range vals {
		v := vals[i]
		if _, merr := core.AsMap(v); merr == nil {
			if table != nil {
				return Rules{}, fmt.Errorf("fmt rules stylesheet: more than one rule table in the source")
			}
			table = &vals[i]
		}
	}
	if table == nil {
		return Rules{}, fmt.Errorf("fmt rules stylesheet: no rule table (a map literal) in the source")
	}
	return mergeRulesValue(Rules{}, *table, true)
}

// MergeRulesValue reads a (partial) boru rule table over base: keys the
// table names are overridden, everything else keeps base's value. This is
// the read side of `Fmt.format-with` (modules/fmtrules.go wraps it with
// DefaultRules as the base). Every key and enum value is validated — an
// unknown rule key, node kind, attach class, or strategy name is an error,
// not a silent no-op.
func MergeRulesValue(base Rules, v core.Value) (Rules, error) {
	return mergeRulesValue(base, v, false)
}

// maxRuleMagnitude bounds width/indent values from a boru rule table.
// Layout dimensions beyond ±4096 columns are nonsensical and a huge indent
// would otherwise force absurd padding allocations (the processor also
// clamps defensively — this boundary rejects loudly instead of clamping
// silently).
const maxRuleMagnitude = 4096

// requiredRuleKeys is the complete top-level key set a strict (stylesheet)
// table must define.
func requiredRuleKeys() []string {
	return []string{
		"width", "indent", "fn-word", "statement-starts",
		"attach", "attach-dot-suffix", "templates", "strategies",
	}
}

func mergeRulesValue(base Rules, v core.Value, requireAll bool) (Rules, error) {
	ru := base
	m, err := core.RequireConcreteMap(v, "fmt format-with")
	if err != nil {
		return ru, err
	}
	seen := map[string]bool{}
	for _, key := range m.Keys() {
		val, _ := m.Get(key)
		seen[key] = true
		switch key {
		case "width":
			n, nerr := intRuleField(val, "width")
			if nerr != nil {
				return ru, nerr
			}
			ru.Width = n
		case "indent":
			n, nerr := intRuleField(val, "indent")
			if nerr != nil {
				return ru, nerr
			}
			ru.Indent = n
		case "fn-word":
			s, serr := val.AsConcreteString()
			if serr != nil {
				return ru, fmt.Errorf("fmt rules: fn-word must be a String: %w", serr)
			}
			ru.FnWord = s
		case "statement-starts":
			words, lerr := stringListField(val, "statement-starts")
			if lerr != nil {
				return ru, lerr
			}
			ru.StmtStartWords = words
		case "attach":
			if aerr := readAttach(val, &ru); aerr != nil {
				return ru, aerr
			}
		case "attach-dot-suffix":
			b, berr := boolRuleField(val, "attach-dot-suffix")
			if berr != nil {
				return ru, berr
			}
			ru.AttachDotSuffix = b
		case "templates":
			if terr := readTemplates(val, &ru, requireAll); terr != nil {
				return ru, terr
			}
		case "strategies":
			names, lerr := stringListField(val, "strategies")
			if lerr != nil {
				return ru, lerr
			}
			known := map[string]bool{}
			for _, s := range StrategyNames() {
				known[s] = true
			}
			for _, s := range names {
				if !known[s] {
					return ru, fmt.Errorf("fmt rules: unknown strategy %q (known: %v)", s, StrategyNames())
				}
			}
			ru.Strategies = names
		default:
			return ru, fmt.Errorf("fmt rules: unknown rule key %q", key)
		}
	}
	if requireAll {
		for _, key := range requiredRuleKeys() {
			if !seen[key] {
				return ru, fmt.Errorf("fmt rules stylesheet: missing rule key %q", key)
			}
		}
	}
	return ru, nil
}

// intRuleField reads a bounded integer rule-table field (width / indent).
func intRuleField(v core.Value, field string) (int, error) {
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, fmt.Errorf("fmt rules: %s must be an Integer: %w", field, err)
	}
	if n < -maxRuleMagnitude || n > maxRuleMagnitude {
		return 0, fmt.Errorf("fmt rules: %s %d out of range (|%s| must be <= %d)", field, n, field, maxRuleMagnitude)
	}
	return int(n), nil
}

// boolRuleField reads a Boolean rule-table field. The words `true` /
// `false` are accepted alongside a concrete Boolean: the raw parser (the
// stylesheet path) leaves an unevaluated map's `true` as a WORD — it only
// becomes a Boolean under engine evaluation — and the two paths must read
// the same table the same way.
func boolRuleField(v core.Value, field string) (bool, error) {
	if b, err := v.AsConcreteBoolean(); err == nil {
		return b, nil
	}
	if core.IsWord(&v) {
		if info, werr := core.AsWord(&v); werr == nil {
			switch info.Name {
			case "true":
				return true, nil
			case "false":
				return false, nil
			}
		}
	}
	return false, fmt.Errorf("fmt rules: %s must be a Boolean", field)
}

// stringListField reads a rule-table field that must be a List of Strings.
func stringListField(v core.Value, field string) ([]string, error) {
	lst, err := core.RequireConcreteList(v, "fmt rules")
	if err != nil {
		return nil, fmt.Errorf("fmt rules: %s must be a List: %w", field, err)
	}
	out := make([]string, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		el := lst.Get(i)
		s, serr := el.AsConcreteString()
		if serr != nil {
			return nil, fmt.Errorf("fmt rules: %s[%d] must be a String: %w", field, i, serr)
		}
		out[i] = s
	}
	return out, nil
}

// readAttach reads the attach-class map ({comma:'prev' colon:'both' …})
// into the Rules' AttachPrev/AttachNext kind sets. Setting the key REPLACES
// the whole policy: kinds absent from the map attach nothing.
func readAttach(v core.Value, ru *Rules) error {
	m, err := core.RequireConcreteMap(v, "fmt rules")
	if err != nil {
		return fmt.Errorf("fmt rules: attach must be a Map: %w", err)
	}
	ru.AttachPrev = nil
	ru.AttachNext = nil
	for _, name := range m.Keys() {
		kind, ok := KindByName(name)
		if !ok {
			return fmt.Errorf("fmt rules: attach: unknown node kind %q", name)
		}
		cv, _ := m.Get(name)
		cls, cerr := cv.AsConcreteString()
		if cerr != nil {
			return fmt.Errorf("fmt rules: attach.%s must be a String class: %w", name, cerr)
		}
		switch cls {
		case "prev":
			ru.AttachPrev = append(ru.AttachPrev, kind)
		case "next":
			ru.AttachNext = append(ru.AttachNext, kind)
		case "both":
			ru.AttachPrev = append(ru.AttachPrev, kind)
			ru.AttachNext = append(ru.AttachNext, kind)
		case "none":
			// explicit no-attachment: named for clarity, adds nothing.
		default:
			return fmt.Errorf("fmt rules: attach.%s: unknown class %q (want prev/next/both/none)", name, cls)
		}
	}
	return nil
}

// templateOp names each kind's recursion op — the point in its template
// body where the processor recurses (XSLT's <xsl:apply-templates/>):
//
//	text        the node's own source text (leaf value kinds)
//	apply       the rendered children (list / paren)
//	entries     the canonicalised key:value entries (map)
//	statements  the statement sequence (root)
//
// Punctuation kinds have NO op — their body is the literal they emit
// (newline's is empty). The map is the closed vocabulary readTemplates
// validates against.
var templateOp = map[string]string{
	"root": "statements",
	"word": "text", "string": "text", "number": "text", "comment": "text",
	"list": "apply", "paren": "apply",
	"map": "entries",
}

// readTemplates reads the per-kind template bodies — the stylesheet's
// `templates` section, the XSLT-template analogue: each node kind maps to
// a body of String LITERALS and at most one recursion-op ATOM
// (`list:['[' apply/q ']']`). Containers compile their surrounding
// literals into the open/close glyphs; punctuation kinds compile their
// literal into the glyph they emit; value kinds and root must be exactly
// their op. In merge mode a named kind's body replaces that kind's
// compiled form; in strict (stylesheet) mode every kind is required, so
// the file fully defines every template.
func readTemplates(v core.Value, ru *Rules, requireAll bool) error {
	m, err := core.RequireConcreteMap(v, "fmt rules")
	if err != nil {
		return fmt.Errorf("fmt rules: templates must be a Map: %w", err)
	}
	// The Glyphs map may be shared with the base Rules (DefaultRules) —
	// clone before writing so an override never mutates the canonical table.
	glyphs := map[NodeKind]string{}
	for k, g := range ru.Glyphs {
		glyphs[k] = g
	}
	ru.Glyphs = glyphs

	seen := map[string]bool{}
	for _, name := range m.Keys() {
		kind, ok := KindByName(name)
		if !ok {
			return fmt.Errorf("fmt rules: templates: unknown node kind %q", name)
		}
		seen[name] = true
		bv, _ := m.Get(name)
		body, berr := core.RequireConcreteList(bv, "fmt rules")
		if berr != nil {
			return fmt.Errorf("fmt rules: templates.%s must be a List: %w", name, berr)
		}

		// Split the body into literals around at most one op atom.
		pre, post := "", ""
		op := ""
		for i := 0; i < body.Len(); i++ {
			part := body.Get(i)
			if part.Is(core.TAtom) {
				a, aerr := part.AsConcreteAtom()
				if aerr != nil {
					return fmt.Errorf("fmt rules: templates.%s[%d]: %w", name, i, aerr)
				}
				if op != "" {
					return fmt.Errorf("fmt rules: templates.%s has more than one op (%s, %s)", name, op, a)
				}
				op = a
				continue
			}
			s, serr := part.AsConcreteString()
			if serr != nil {
				return fmt.Errorf("fmt rules: templates.%s[%d] must be a String literal or an op atom: %w", name, i, serr)
			}
			if op == "" {
				pre += s
			} else {
				post += s
			}
		}

		want := templateOp[name]
		if want == "" {
			// Punctuation kind: literal-only body compiles to its glyph.
			if op != "" {
				return fmt.Errorf("fmt rules: templates.%s takes no op, got %s/q", name, op)
			}
			glyphs[kind] = pre
			continue
		}
		if op != want {
			return fmt.Errorf("fmt rules: templates.%s needs the %s/q op, got %q", name, want, op)
		}
		switch name {
		case "list":
			ru.ListOpen, ru.ListClose = pre, post
		case "map":
			ru.MapOpen, ru.MapClose = pre, post
		case "paren":
			ru.ParenOpen, ru.ParenClose = pre, post
		default:
			// root and the value kinds carry no glyphs: their body is
			// exactly the op.
			if pre != "" || post != "" {
				return fmt.Errorf("fmt rules: templates.%s must be exactly [%s/q]", name, want)
			}
		}
	}
	if requireAll {
		for k := NdRoot; k <= NdNewline; k++ {
			if !seen[NodeKindName(k)] {
				return fmt.Errorf("fmt rules stylesheet: templates missing kind %q", NodeKindName(k))
			}
		}
	}
	return nil
}
