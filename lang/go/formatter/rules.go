package formatter

// Rules is the declarative layout rule table the formatter interprets — the
// stylesheet of the XSLT-style split the design settled on
// (design/legacy/fmt-module-and-xslt.0.ignore, Phase 3): the RULES are data (this
// table, exposed to boru as `Fmt.rules` and consumed by `Fmt.format-with`),
// and the emitter is the generic PROCESSOR that interprets them, exactly as
// an XSLT processor interprets templates. Every layout decision the
// formatter makes — where whitespace goes, what glues to what, how overflow
// wraps — reads from this table; none is hardcoded in the walk. DefaultRules
// reproduces the canonical boru style byte-for-byte.
//
// Whitespace is owned BY the rules, not by an output algebra: attach classes
// say which tokens glue (no space), Indent says how container bodies and
// continuations shift, Width drives the fit checks, and Strategies names the
// overflow templates in the order they are tried. That is how the XSLT-style
// algorithm reproduces the exact current output — templates emit text,
// including its whitespace.
type Rules struct {
	// Width is the maximum line width; layout templates try to fit within
	// it and wrap when they cannot.
	Width int

	// Indent is the indentation step for container bodies, fn bodies, and
	// wrap continuations.
	Indent int

	// StmtStartWords are the words that open a new logical group inside a
	// multi-line list or fn body — each group starts a fresh line.
	StmtStartWords []string

	// FnWord is the function-definition trigger: a statement ending with
	// `<FnWord> [wrapper]` is eligible for the fn layout template.
	FnWord string

	// AttachPrev lists node kinds that glue to the PREVIOUS token (no space
	// before them): by default comma, colon, question, dot.
	AttachPrev []NodeKind

	// AttachNext lists node kinds the FOLLOWING token glues to (no space
	// after them): by default colon and dot.
	AttachNext []NodeKind

	// AttachDotSuffix glues the next token to a word ending in `.`
	// (`input.(foo)` stays solid).
	AttachDotSuffix bool

	// Container brackets — the COMPILED form of the container templates
	// (`list:['[' apply/q ']']` etc. in the stylesheet's `templates`
	// section): the literals before the recursion op become Open, the
	// literals after it become Close.
	ListOpen, ListClose   string
	MapOpen, MapClose     string
	ParenOpen, ParenClose string

	// Glyphs is the compiled form of the punctuation templates
	// (`comma:[',']` etc.): the literal each punctuation kind emits. A
	// missing kind emits nothing.
	Glyphs map[NodeKind]string

	// Strategies is the ordered list of statement layout templates tried
	// for an overflowing statement. Known names (see StrategyNames):
	//
	//	comment-only      — a lone comment renders as itself
	//	inline            — keep the statement on one line when it fits
	//	trailing-comment  — a statement ending in a line comment stays on
	//	                    one line even when it overflows (the comment
	//	                    annotates the whole statement)
	//	fn                — the fn-definition multi-line template
	//	trailing-container— a trailing map/list opens on the header line
	//	wrap              — greedy fill-wrap with a hanging continuation
	//
	// An unknown name is inert (skipped). If no strategy claims the
	// statement it stays inline regardless of width.
	Strategies []string
}

// The canonical rule table is NOT defined here: it is EXPRESSED IN boru in
// fmt-rules.boru (the stylesheet, embedded and parsed at first use — see
// rules_boru.go::DefaultRules). This Go struct is the stylesheet's compiled
// form; the processor below interprets whichever table it is given.

// StrategyNames lists the statement layout templates the processor knows,
// in the canonical order. `Fmt.format-with` validates a rule table's
// `strategies` entries against this set.
func StrategyNames() []string {
	return []string{
		"comment-only", "inline", "trailing-comment",
		"fn", "trailing-container", "wrap",
	}
}

// KindByName resolves a node-kind dispatch name (the NodeKindName
// vocabulary: `word`, `list`, `comma`, …) back to its NodeKind. It is the
// boundary a data-expressed rule table (attach classes keyed by kind name)
// crosses into the processor.
func KindByName(name string) (NodeKind, bool) {
	for k := NdRoot; k <= NdNewline; k++ {
		if NodeKindName(k) == name {
			return k, true
		}
	}
	return 0, false
}

// renderer is the compiled form of a Rules table: the lookup sets the
// processor consults on every node. Built once per FormatRules call.
type renderer struct {
	ru         Rules
	stmtStart  map[string]bool
	attachPrev map[NodeKind]bool
	attachNext map[NodeKind]bool
	// memo caches emitNode by (node, indent). Rendering is a pure function
	// of that pair — the renderer is immutable once built and the node tree
	// is not mutated after parsing — and caching it is what keeps
	// formatting linear. See emitNode for why it is load-bearing rather
	// than an optimisation.
	memo map[emitKey]string
}

// emitKey identifies one memoised render: a node laid out at one indent.
// The indent is part of the key because a container's broken form embeds
// its own indentation.
type emitKey struct {
	n      *Node
	indent int
}

// glyph returns the punctuation template's compiled literal for kind — the
// text the kind emits. A kind absent from the table emits nothing (total on
// a zero-value Rules).
func (r *renderer) glyph(k NodeKind) string {
	return r.ru.Glyphs[k]
}

func newRenderer(ru Rules) *renderer {
	r := &renderer{
		ru:         ru,
		stmtStart:  map[string]bool{},
		attachPrev: map[NodeKind]bool{},
		attachNext: map[NodeKind]bool{},
		memo:       map[emitKey]string{},
	}
	for _, w := range ru.StmtStartWords {
		r.stmtStart[w] = true
	}
	for _, k := range ru.AttachPrev {
		r.attachPrev[k] = true
	}
	for _, k := range ru.AttachNext {
		r.attachNext[k] = true
	}
	return r
}

// maxPad caps a single padding run. Indentation deeper than this is
// nonsensical for source layout; the cap keeps a hostile rule table (a
// huge Indent reaching pad via Fmt.format-with) from forcing an absurd
// allocation. The boru boundary additionally range-validates width/indent.
const maxPad = 1 << 16

// pad returns n spaces, clamping a negative n to zero and an absurd n to
// maxPad, so no rule table — hostile, zero-value, or buggy — can panic
// the processor or force an unbounded allocation.
func pad(n int) string {
	if n < 0 {
		n = 0
	}
	if n > maxPad {
		n = maxPad
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}
