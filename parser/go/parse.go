package parser

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"

	core "github.com/boru-lang/boru/core/go"
	"github.com/cockroachdb/apd/v3"
	jsonic "github.com/tabnas/jsonic/go"
)

func boolPtr(b bool) *bool { return &b }

// parenGroup represents items collected between ( and ) by the jsonic grammar.
// At the top level, these are expanded to engine paren markers. In data context
// (map values), they become ParenExpr values for inline evaluation.
type parenGroup []any

// unclosedParen wraps a parenGroup that was auto-closed at EOF (no matching `)`).
// The converter produces an error for these.
type unclosedParen struct{ items []any }

// angleGroup represents a generics angle-bracket sugar group
// (design/legacy/GENERICS.10.ignore Phase 6): `Box<Integer>` folds the receiver
// name and the collected items into one node. The conversion emits ONE
// structural sugar marker (ADR-012 amendment) carrying both
// precomputed forms — the generic-def head params and the use-site
// args — and the ENGINE picks which to lower at dispatch. The parser
// recognises no binder word; the angle group converts identically in
// every position.
type angleGroup struct {
	Name  string
	Items []any
}

// unclosedAngle marks an angle group auto-closed at EOF (no matching
// `>`). The converter produces an error for these.
type unclosedAngle struct{ Name string }

// interpGroup represents the parts collected between backticks by the interp
// grammar rule. Each element is either a jsonic.Text{Quote:"tl"} (literal
// segment) or an iexprGroup (interpolated expression).
type interpGroup []any

// iexprGroup represents the values collected between ${ and } by the iexpr
// grammar rule. Contains raw jsonic values that will be converted to engine
// values by the converter.
type iexprGroup []any

// sited wraps a converted jsonic node with the source position of its
// opening token, captured by the val-rule before-close callback (see
// grammar.go setupValRule). deSite unwraps it.
//
// Pos here is a pure diagnostic: an unknown (zero) SrcPos renders as
// "source position unknown" at error time rather than a guessed location,
// so the 0=unknown convention is deliberate and is NOT a "No Zero-Value
// Overload" violation (see eng/go/CLAUDE.md) — no decision reads a zero Pos.
type sited struct {
	Node any
	Pos  core.SrcPos
}

// deSite unwraps a sited wrapper, returning the inner node and its
// source position. A non-sited node is returned unchanged with a zero
// (unknown) Pos. Idempotent and safe on any node — call it wherever a
// raw jsonic node is type-switched so the wrapper never leaks past the
// converter's dispatch points.
func deSite(v any) (any, core.SrcPos) {
	if s, ok := v.(sited); ok {
		return s.Node, s.Pos
	}
	return v, core.SrcPos{}
}

// withPos stamps a source position onto v, unless pos is unknown (zero
// Row), in which case v is returned untouched. The guard avoids both
// storing the unknown and clobbering a position a deeper converter
// already set on a nested value.
func withPos(v core.Value, pos core.SrcPos) core.Value {
	if pos.Row != 0 {
		v.SetPos(pos)
	}
	return v
}

// maxParseNestingDepth bounds how deeply lists, maps, and paren groups may
// nest in one source program. The converters below (convertTopLevelItems,
// convertDataValue, and their mutual callees) recurse once per nesting level;
// without a bound a pathologically deep input — e.g. `if true [` repeated
// hundreds of thousands of times — overflows the Go stack. A stack overflow is
// a FATAL runtime error that no recover() can catch: it crashes the whole host
// process, defeating the interpreter/VM fallback the rest of the pipeline relies
// on. The limit sits far above any realistic hand-written or generated program
// yet far below the overflow point, so it turns a crash into a clean
// evaluation_limit error. It also caps the super-linear conversion cost deep
// nesting incurs.
const maxParseNestingDepth = 10000

// parseDepth tracks the live container-nesting depth of the single conversion
// walk. It is threaded through the recursive converters (not a separate
// pre-pass — the bound is enforced IN the one pass that already builds the
// values): each container converter calls enter on the way in and leave on the
// way out, so cur mirrors the Go recursion depth. One *parseDepth is created
// per Parse call, so the counter is per-parse and concurrency-safe.
type parseDepth struct {
	cur int
	src string
}

// enter descends one container level, failing with a clean evaluation_limit
// error (never a stack overflow) once nesting passes maxParseNestingDepth.
// Pair every enter with a deferred leave.
func (d *parseDepth) enter() error {
	d.cur++
	if d.cur > maxParseNestingDepth {
		return core.MakeBoruError("evaluation_limit",
			fmt.Sprintf("source nesting exceeds the depth limit of %d — lists, maps, and parentheses are nested too deeply", maxParseNestingDepth),
			"", d.src,
			"flatten the structure or split it across definitions; nesting this deep is almost always generated, not intended")
	}
	return nil
}

// leave ascends one container level. Always deferred immediately after enter.
func (d *parseDepth) leave() { d.cur-- }

// Parse tokenizes the boru source string into a slice of core.Value.
// The input is treated as a top-level implicit list: jsonic.Parse handles
// the entire source. The TextInfo option distinguishes quoted strings from
// unquoted text (words).
//
// Custom tokens are registered for (, ), and . so that they are lexed as
// separate tokens by jsonic. This replaces the earlier preprocessParens
// approach and string-based dot expansion, making the parser cleaner.
//
// Context rules:
//   - Top level: unquoted text → words, quoted text → strings.
//   - Inside maps (including implicit): all text → scalar data.
//   - Inside lists at the top level: unquoted text → words (quotation).
//   - Inside lists inside maps: all text → scalar data.
func Parse(src string) ([]core.Value, error) {
	// Program tokens must carry compile identities even though parsing
	// runs outside any check/compile pass — the emitter's provenance
	// tracking keys on the IDs of the parsed literals themselves. Runtime
	// value mints elide IDs (eng mode-gated elision); this scope re-arms
	// minting for the parse.
	defer core.BeginIDMintScope()()
	j := SafeMake(jsonic.Options{
		// tabnas groups the output-shaping flags under Info: Text wraps
		// scalars in Text{Str,Quote} (quoted/unquoted distinction), List/Map
		// return ListRef/MapRef carriers with structural metadata. These were
		// the top-level TextInfo/ListRef/MapRef flags in the legacy jsonicjs.
		Info:  &jsonic.InfoOptions{Text: boolPtr(true), List: boolPtr(true), Map: boolPtr(true)},
		List:  &jsonic.ListOptions{Pair: boolPtr(true), Child: boolPtr(true)},
		Map:   &jsonic.MapOptions{Child: boolPtr(true)},
		Value: &jsonic.ValueOptions{Lex: boolPtr(false)},
		// The library's own error rendering is silenced at the source:
		// parse failures are translated into boru syntax_errors
		// (translateParseError), so the always-on ANSI palette, the
		// `--internal:` suffix, and the library docs link must never
		// reach a user (they used to leak raw into the REPL and the
		// wasm playground DOM).
		ErrMsg: &parseErrMsgOptions,
		Color:  &jsonic.ColorOptions{Active: &parseColorOff},
	})

	// Stage 1: Lex setup — register tokens and custom matchers, the
	// token table coming from the declarative grammar artifact.
	g := loadDeclGrammar()
	t, tins := setupBaseTokens(j, g)
	setupTemplateLiteralMatcher(j, t)
	setupStringEscapeMatcher(j)
	setupBigNumberMatcher(j, t)
	setupDecimalUnderscoreMatcher(j, t)
	setupMiniLitMatcher(j, t)
	setupXmlMatcher(j, t)

	// Stage 2: Grammar setup — the declarative edits first (they were
	// the head of the val amendments), then the remaining imperative
	// rule extensions, batch-migrating into grammar.json.
	applyDeclEdits(j, g, tins, valDeclHooks())
	setupValRule(j, t)
	setupPairGrammar(j, t)
	setupParenGrammar(j, t)
	setupAngleGrammar(j, t)
	setupInterpGrammar(j, t)
	setupXmlGrammar(j, t)
	setupNumberSub(j)

	// Stage 3: Parse and convert to engine values.
	result, err := j.Parse(src)
	if err != nil {
		return nil, translateParseError(err, src)
	}

	if result == nil {
		return nil, nil
	}

	// A single top-level scalar/paren/interp value arrives wrapped by the
	// val-rule BC; unwrap it so the cases below see a bare jsonic node, but
	// keep its position to stamp the single produced value. (Root containers
	// come from the list/map rule and are not sited; their elements carry
	// positions individually.)
	result, rootPos := deSite(result)

	// One depth tracker for this parse, threaded through the recursive
	// converters so pathologically deep nesting fails with a clean
	// evaluation_limit error instead of overflowing the Go stack — enforced
	// inside the single conversion walk, not as a separate pre-pass.
	d := &parseDepth{src: src}

	// With ListRef and MapRef enabled, jsonic returns ListRef/MapRef for
	// all lists and maps. ListRef.Implicit and MapRef.Implicit distinguish
	// implicit structures from explicit ones.
	switch val := result.(type) {
	case jsonic.ListRef:
		if val.Child != nil {
			tv, err := convertTypedList(val, d)
			if err != nil {
				return nil, err
			}
			return []core.Value{tv}, nil
		}
		if !val.Implicit {
			// Explicit list [...]  — a single list value (quotation).
			lv, err := convertWordList(val.Val, d)
			if err != nil {
				return nil, err
			}
			return []core.Value{lv}, nil
		}
		// Implicit list — top-level stack values.
		return convertTopLevel(val.Val, d)
	case jsonic.MapRef:
		if hasMapChild(val.Val) {
			tv, err := convertTypedMap(val.Val, d, val.Meta)
			if err != nil {
				return nil, err
			}
			return []core.Value{tv}, nil
		}
		mv, err := convertMapData(val.Val, val.Implicit, d, val.Meta)
		if err != nil {
			return nil, err
		}
		// Top-level implicit maps (e.g. entire input is "a:x") must be
		// auto-evaluated so expressions in values resolve.
		if val.Implicit && !mv.Eval {
			mv.Eval = true
		}
		return []core.Value{mv}, nil
	case unclosedParen:
		return nil, core.MakeBoruError("syntax_error", "unmatched opening parenthesis", "(", src, "")

	case parenGroup:
		// Single paren group at top level: expand to paren markers.
		return convertTopLevelItems([]any{val}, d)

	case interpGroup:
		// Single template string at top level.
		iv, err := convertInterpGroup(val, d)
		if err != nil {
			return nil, err
		}
		return []core.Value{withPos(iv, rootPos)}, nil

	default:
		// Single top-level scalar/word: stamp the position the root deSite
		// recovered (convertTopLevelValue sees a bare node, so it cannot).
		v, err := convertTopLevelValue(val, d)
		if err != nil {
			return nil, err
		}
		return []core.Value{withPos(v, rootPos)}, nil
	}
}

// isToken checks if item is an unquoted text marker matching the given string.
// Quoted text (e.g. "." or "!") has Quote != "" and is handled as a string
// by convertTopLevelValue, so it never reaches the token checks.
func isToken(item any, tok string) bool {
	node, _ := deSite(item)
	text, ok := node.(jsonic.Text)
	return ok && text.Str == tok && text.Quote == ""
}

// convertTopLevelItems converts a list of jsonic items in word context,
// handling parenthesis markers and token sequences.
//
// Dotted access is sugar for `get`/`getr` and is GROUPED so it binds to
// its immediate receiver: a chain `recv.k1.k2` (or `recv!.k1`) wraps as a
// single paren group `( recv get k1 get k2 … )`. Grouping isolates the
// access from surrounding forward-collection, so `size m.x` means
// `size (m.x)`, not `(size m).x`. `get`/`getr` left-compose on the stack,
// so one flat group covers any chain length. The receiver and each key may
// themselves be paren groups, which covers `(expr).k` and the computed-key
// form `m.(expr)`.
//
//   - "." → get      "!" "." → getr
//
// A lone "." / "!." with no receiver still emits the bare word (and errors
// at runtime, as before). All other items convert to engine values
// directly.
func convertTopLevelItems(items []any, d *parseDepth) ([]core.Value, error) {
	if err := d.enter(); err != nil {
		return nil, err
	}
	defer d.leave()
	// Strip sited wrappers once into a bare-node slice plus a parallel
	// position slice. The token helpers (isToken/startsDot/isChainReceiver)
	// then see bare jsonic nodes, while the emitted operator-marker words
	// (get/getr) and primaries are still stamped from poss[i].
	poss := make([]core.SrcPos, len(items))
	if anySited(items) {
		bare := make([]any, len(items))
		for i, it := range items {
			bare[i], poss[i] = deSite(it)
		}
		items = bare
	}

	values := make([]core.Value, 0, len(items))
	for i := 0; i < len(items); i++ {
		// Minilang literal `+name<delim>src<delim>`: emit the three tokens
		// `mini name 'src'` INLINE in word context (top level, list elements,
		// paren contents) — byte-for-byte the same value stream as if the
		// user had typed `mini name 'src'`. This keeps full parity with the
		// explicit form (a trailing opts Map collected by mini's 3-arg sig, a
		// stack subject, the expansion-time unknown-kind error, and check
		// mode) instead of wrapping in an outer splice (which would
		// double-splice mini's own splice and trip check mode at the bare top
		// level). Data-context map values fall back to the splice form in
		// convertTopLevelValueInner.
		if ml, ok := items[i].(miniLitVal); ok {
			values = append(values,
				withPos(core.NewSugar(core.SugarInfo{Kind: core.SugarMini, Name: ml.Name, Src: ml.Src}), poss[i]))
			continue
		}
		// Dotted-access chain: a receiver primary followed by one or more
		// `.key` / `!.key` segments → one group `( recv get k … )`.
		if isChainReceiver(items[i]) && i+1 < len(items) && startsDot(items, i+1) {
			// A reach receiver must be something `get` can index into (a
			// Map, List, Store, Object, module namespace, …). A number has no
			// members, so a numeric literal before a `.` can never be a
			// valid reach — `1.2.3` (a malformed numeric literal), `1 . 2`,
			// or `5 . foo`. Reject it here with a clear message rather than
			// the runtime "no matching signature for get".
			if isNumberLiteral(items[i]) {
				return nil, numberReceiverError(poss[i])
			}
			// Build the access group into a temp slice so a trailing
			// `/`-modifier can be placed correctly: a WORD modifier
			// (usurp / stack-args / forward-args) is emitted BEFORE the
			// group so it forward-collects the result (the result must not
			// auto-dispatch first), while the Word/__DM marker (/N /v /q)
			// is emitted AFTER for execFnDefLiteral to peek.
			// Build a first-class Reach node (design/REACH.10.md Phase B):
			// receiver tokens + per-segment {op, literal-or-computed key}.
			var recv []core.Value
			if err := emitPrimary(&recv, items[i], poss[i], d); err != nil {
				return nil, err
			}
			var segs []core.ReachSeg
			j := i + 1
			var pfx, sfx []core.Value
		chain:
			for j < len(items) {
				var getr bool
				var keyItem any
				var kpos core.SrcPos
				switch {
				case isToken(items[j], "!") && j+2 < len(items) && isToken(items[j+1], "."):
					getr, keyItem, kpos = true, items[j+2], poss[j+2]
					j += 3
				case isToken(items[j], ".") && j+1 < len(items):
					getr, keyItem, kpos = false, items[j+1], poss[j+1]
					j += 2
				default:
					break chain
				}
				// A `/`-modifier on the final key applies to the whole reach.
				if base, pre, suf, ok := groupModifier(keyItem); ok && base != "" {
					keyItem, pfx, sfx = jsonic.Text{Str: base}, pre, suf
				}
				seg := core.ReachSeg{Getr: getr}
				if pg, ok := keyItem.(parenGroup); ok {
					// Computed key: m.(expr) — store the paren's tokens.
					inner, err := convertTopLevelItems([]any(pg), d)
					if err != nil {
						return nil, err
					}
					seg.Computed = true
					seg.KeyExpr = inner
				} else {
					// Literal key (word → atom-via-get/q, string, number).
					var tmp []core.Value
					if err := emitPrimary(&tmp, keyItem, kpos, d); err != nil {
						return nil, err
					}
					// emitPrimary appends exactly one value on success.
					seg.KeyLit = reachSegmentName(keyItem, tmp[0], kpos)
				}
				segs = append(segs, seg)
			}
			// A `$` receiver marks a RECEIVERLESS reach — a detached lens
			// (`$.name`), not a `$ get name` chain. It is inert (Eval=false):
			// it evaluates to itself (a Reach value) so it can be passed as a
			// reusable accessor (`people each $.name`) and applied/rebound to a
			// receiver later. `$` is reserved as a user word (see
			// ValidateWordName), so this never shadows a real binding.
			reachInfo := core.ReachInfo{Receiver: recv, Segments: segs, Eval: true}
			if isDollarReceiver(recv) {
				reachInfo.Receiver, reachInfo.Eval = nil, false
			}
			pe := withPos(core.NewReach(reachInfo), poss[i])
			// A standalone `/mod` token immediately after the path also
			// applies to the result (`(a.b)/mod`).
			if pfx == nil && sfx == nil && j < len(items) {
				if base, pre, suf, ok := groupModifier(items[j]); ok && base == "" {
					pfx, sfx = pre, suf
					j++
				}
			}
			for _, p := range pfx {
				values = append(values, withPos(p, poss[i]))
			}
			values = append(values, pe)
			for _, s := range sfx {
				values = append(values, withPos(s, poss[i]))
			}
			i = j - 1
			continue
		}

		// A "!." or "." with no receiver before it is the leading / prefix
		// dot form (e.g. `.5`, `.name`, `!.x`). It has nothing to access,
		// so reject it at parse time with a clear message instead of
		// emitting a receiverless get/getr that fails later as an obscure
		// signature error. The receiverless reach `$.name` is unaffected:
		// it carries an explicit `$` receiver and is built in the chain
		// branch above.
		if isToken(items[i], "!") && i+1 < len(items) && isToken(items[i+1], ".") {
			return nil, danglingDotError(poss[i])
		}
		if isToken(items[i], ".") {
			return nil, danglingDotError(poss[i])
		}

		// Unclosed paren: error at parse time.
		if up, ok := items[i].(unclosedParen); ok {
			_ = up
			return nil, core.MakeBoruError("syntax_error", "unmatched opening parenthesis", "(", "", "")
		}

		// A standalone `/mod` token right after a primary (e.g. `(expr)/s`)
		// applies the modifier to that primary's result. Word modifiers are
		// emitted BEFORE the primary (so they forward-collect the result
		// before any auto-dispatch); the /v /q marker is emitted AFTER.
		if i+1 < len(items) {
			if base, pre, suf, ok := groupModifier(items[i+1]); ok && base == "" {
				for _, p := range pre {
					values = append(values, withPos(p, poss[i]))
				}
				if err := emitPrimary(&values, items[i], poss[i], d); err != nil {
					return nil, err
				}
				for _, s := range suf {
					values = append(values, withPos(s, poss[i]))
				}
				i++
				continue
			}
		}

		// A value, or a paren group expanded to ( … ) markers.
		if err := emitPrimary(&values, items[i], poss[i], d); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// anySited reports whether any item is a sited wrapper, so the common
// data-context path (where items are never sited) skips re-allocation.
func anySited(items []any) bool {
	for _, it := range items {
		if _, ok := it.(sited); ok {
			return true
		}
	}
	return false
}

// startsDot reports whether items[j] begins a dot-access segment: a "."
// token, or a "!" immediately followed by ".".
func startsDot(items []any, j int) bool {
	if isToken(items[j], ".") {
		return true
	}
	return isToken(items[j], "!") && j+1 < len(items) && isToken(items[j+1], ".")
}

// isDollarReceiver reports whether a dot-chain receiver is the lone reserved
// `$` sentinel — the marker for a receiverless reach (`$.name` → a lens). The
// receiver emits as a single Word("$") (see emitPrimary / convertTopLevelValue).
func isDollarReceiver(recv []core.Value) bool {
	if len(recv) != 1 || !core.IsWord(recv[0]) {
		return false
	}
	w, _ := core.AsWord(recv[0])
	return w.Name == "$"
}

// isChainReceiver reports whether an item can be the receiver of a dot
// chain — anything except the "." / "!" operator markers and an unclosed
// paren (i.e. a real value, word, or paren group).
func isChainReceiver(item any) bool {
	if _, ok := item.(unclosedParen); ok {
		return false
	}
	return !isToken(item, ".") && !isToken(item, "!")
}

// groupModifier decodes a `/`-modifier on a word token that should apply to
// a parenthesised / dotted-path RESULT. It returns the base text (empty for
// a standalone `/mod` token following the group) plus tokens to place around
// the group:
//
//	/u  → prefix `usurp`            /s  → prefix `stack-args`
//	/f  → prefix `forward-args`     /N  → prefix `force-arity N`
//	/v /q → suffix Word/__DM marker (leave the result inert/data)
//
// Word modifiers are PREFIXED (emitted before the group) so they
// forward-collect the result before it can auto-dispatch; the marker is
// SUFFIXED for execFnDefLiteral to peek. So `a.b/u` parses as
// `usurp (a get b)` and `a.b/2` as `force-arity 2 (a get b)`. ok=false for
// a plain word (no modifier).
func groupModifier(item any) (base string, prefix, suffix []core.Value, ok bool) {
	node, _ := deSite(item)
	t, isText := node.(jsonic.Text)
	if !isText || t.Quote != "" {
		return "", nil, nil, false
	}
	b, argCount, fs, ff, q, v, u, _, valid := scanWordModifier(t.Str)
	if !valid {
		return "", nil, nil, false
	}
	switch {
	case u:
		return b, []core.Value{core.NewSugar(core.SugarInfo{Kind: core.SugarUsurp})}, nil, true
	case fs:
		return b, []core.Value{core.NewSugar(core.SugarInfo{Kind: core.SugarStackArgs})}, nil, true
	case ff:
		return b, []core.Value{core.NewSugar(core.SugarInfo{Kind: core.SugarForwardArgs})}, nil, true
	case argCount >= 0:
		return b, []core.Value{core.NewSugar(core.SugarInfo{Kind: core.SugarForceArity, N: int64(argCount)})}, nil, true
	case v || q:
		return b, nil, []core.Value{core.NewDispatchMod(core.DispatchModInfo{Val: v, Quote: q})}, true
	}
	return "", nil, nil, false
}

// emitPrimary appends the converted form of a single primary item — a
// value, or a parenthesised group as a nested ParenExpr value — to dst. Used
// for the receiver, the keys of a dot chain, and ordinary top-level items.
// pos is the source position of item (caller has already deSited it).
//
// Word-context paren groups become a single ParenExpr value (paren-nesting
// Step 1, design/legacy/PAREN-REPRESENTATION.9.ignore), the same representation data
// context already uses. The engine evaluates it via evalParenExprResults
// (Step 2 at the pointer, Step 3 in a forward window).
func emitPrimary(dst *[]core.Value, item any, pos core.SrcPos, d *parseDepth) error {
	if pg, ok := item.(parenGroup); ok {
		inner, err := convertTopLevelItems([]any(pg), d)
		if err != nil {
			return err
		}
		*dst = append(*dst, withPos(core.NewParenExpr(inner), pos))
		return nil
	}
	v, err := convertTopLevelValue(item, d)
	if err != nil {
		return err
	}
	// convertTopLevelValue deSites internally, but item arrived bare from
	// the caller's normalization, so re-stamp from the caller's pos.
	*dst = append(*dst, withPos(v, pos))
	return nil
}

// convertTopLevel converts a top-level implicit list from jsonic into
// a slice of core.Value using word context.
func convertTopLevel(items []any, d *parseDepth) ([]core.Value, error) {
	return convertTopLevelItems(items, d)
}

// convertTopLevelValue converts a single value in word context.
// Unquoted text → word, quoted text → string. The value is stamped with
// the source position captured by the val-rule BC (if any).
func convertTopLevelValue(v any, d *parseDepth) (core.Value, error) {
	node, pos := deSite(v)
	res, err := convertTopLevelValueInner(node, d)
	if err != nil {
		return res, err
	}
	return withPos(res, pos), nil
}

func convertTopLevelValueInner(v any, d *parseDepth) (core.Value, error) {
	switch val := v.(type) {
	case arrowTag:
		// `=>` — the lambda sugar marker; the engine lowers it to the
		// role-bound constructor word (ADR-012 rule 3 amendment).
		return core.NewSugar(core.SugarInfo{Kind: core.SugarLambda}), nil
	case jsonic.Text:
		if val.Quote == "" {
			return parseWord(val.Str)
		}
		return core.NewString(val.Str), nil

	case interpGroup:
		return convertInterpGroup(val, d)

	case miniLitVal:
		// `+name<delim>src<delim>` desugars to the splice `mini name 'src'`
		// (the `word` mechanism): one value that re-steps as the standard
		// mini call, so a stack subject, a trailing opts Map, the
		// unknown-kind error, and check mode all behave exactly as if the
		// user had typed `mini name 'src'`.
		return core.NewSugar(core.SugarInfo{Kind: core.SugarMini, Name: val.Name, Src: val.Src}), nil

	case xmlElemVal:
		// Embedded XML literal `<tag>…</tag>`: the matcher already built
		// the immutable Node/Xml value; surface any build error here.
		if val.Err != nil {
			return core.Value{}, val.Err
		}
		return val.V, nil

	case numberVal:
		return numberValToValue(val)

	case float64:
		return floatToValue(val), nil

	case jsonic.MapRef:
		if hasMapChild(val.Val) {
			return convertTypedMap(val.Val, d, val.Meta)
		}
		mv, err := convertMapData(val.Val, val.Implicit, d, val.Meta)
		if err != nil {
			return mv, err
		}
		// In word context (top level), implicit maps from pair syntax
		// (e.g. a:x) must be auto-evaluated so expressions resolve.
		if val.Implicit && !mv.Eval {
			mv.Eval = true
		}
		return mv, nil

	case map[string]any:
		// Raw map from list.pair syntax (e.g., [x:number] produces
		// map[string]any{"x": Text("number")} inside the list).
		if hasMapChild(val) {
			return convertTypedMap(val, d)
		}
		mv, err := convertMapData(val, true, d)
		if err != nil {
			return mv, err
		}
		// In word context (top level), implicit maps from pair syntax
		// must be auto-evaluated so expressions resolve.
		mv.Eval = true
		return mv, nil

	case jsonic.ListRef:
		if val.Child != nil {
			return convertTypedList(val, d)
		}
		return convertWordList(val.Val, d)

	case parenGroup:
		// Parentheses remain an expression when they appear directly inside
		// angle arguments (for example, Box<(Integer)>).  Angle conversion
		// feeds each argument through this word-context path, so handle the
		// same intermediate value that is already supported in data context.
		items, err := convertTopLevelItems([]any(val), d)
		if err != nil {
			return core.Value{}, err
		}
		return core.NewParenExpr(items), nil

	case angleGroup:
		// Use-site sugar: `Box<Integer>` → the canonical paren span
		// `( Box of [Integer] )` (a ParenExpr, exactly what the
		// canonical source produces in word context).
		return angleUseSite(val, d)

	case unclosedAngle:
		return core.Value{}, unclosedAngleError(val)

	case unclosedParen:
		// An unclosed group in a member or operand position (`a.(`,
		// `quote . ( =`): the item loop's refusal, never the internal
		// marker's type name (NUR060).
		return core.Value{}, core.MakeBoruError("syntax_error", "unmatched opening parenthesis", "(", "", "")

	case bool:
		return core.NewBoolean(val), nil

	case nil:
		// An untyped-nil element is an EMPTY list slot — `[1,,2]`, `[,1]`,
		// a leading/repeated comma. (An explicit `null` arrives as the
		// jsonic.Text token "null", handled above, so this case only fires
		// for a genuinely empty element.) A repeated or leading comma is a
		// typo, not a value — reject it rather than fabricating a `null`.
		return core.Value{}, emptyElementError()

	default:
		return core.Value{}, fmt.Errorf("unsupported value type %T", v)
	}
}

// emptyElementError is raised when a list literal contains an empty
// element produced by a leading or repeated comma (`[1,,2]`, `[,1]`).
// boru has no implicit "hole" value; commas are optional separators, so a
// missing element is a typo. Use `none` for an explicit empty value.
func emptyElementError() error {
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: emptyElementDetail,
	}
}

// emptyElementDetail is the empty-element refusal's detail, shared by the
// converter's nil-slot check and the grammar's empty list child (NUR060).
const emptyElementDetail = "empty list element: remove the leading/repeated comma (write `none` for an explicit empty value)"

// isNumberLiteral reports whether a (deSited) jsonic item is a numeric
// literal: an integer arrives from jsonic as a float64, a decimal as a
// numberVal (dot-bearing source wrapped by setupNumberSub).
func isNumberLiteral(item any) bool {
	switch item.(type) {
	case float64, numberVal:
		return true
	default:
		return false
	}
}

// numberReceiverError rejects a `.`-access whose receiver is a numeric
// literal. `get`'s receiver is always a container (Map / List / Store /
// Object / module namespace / …) — a number has no members — so `1.2.3` (a
// malformed numeric literal), `1 . 2`, or `5 . foo` can never be a valid
// reach. Caught at parse time instead of surfacing the runtime
// "no matching signature for get".
func numberReceiverError(pos core.SrcPos) error {
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: "a number has no members to access with `.`",
		Hint:   "this looks like a malformed numeric literal (e.g. `1.2.3`) or `.`-access on a number — numbers have no fields or keys",
		Row:    pos.Row,
		Col:    pos.Col,
		Src:    pos.Src,
	}
}

// danglingDotError rejects a leading / receiverless `.` or `!.` — the
// prefix dot form (e.g. `.5`, `.name`, `!.x`). Member access must follow
// a value (`m.key`); a leading `.` is never valid. In particular `.5` is
// not a fraction — write `0.5`. The receiverless reach `$.name` is the
// supported way to write a detached accessor.
func danglingDotError(pos core.SrcPos) error {
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: "`.` member access has no receiver",
		Hint:   "a `.`/`!.` access must follow a value (e.g. `m.key`); a leading `.` is not valid — write `0.5` for a fraction, or `$.key` for a detached accessor",
		Row:    pos.Row,
		Col:    pos.Col,
		Src:    pos.Src,
	}
}

// convertWordList converts a list in word context (top-level list).
// The resulting list is marked for auto-evaluation: its contents will
// be executed at the end of Run unless quoted or consumed by a word.
func convertWordList(items []any, d *parseDepth) (core.Value, error) {
	elems, err := convertTopLevelItems(items, d)
	if err != nil {
		return core.Value{}, err
	}
	return core.NewEvalList(elems), nil
}

// convertMapData converts a map in data context. All text values are
// scalar data regardless of quoting. When implicit is true the
// resulting OrderedMap is marked as coming from pair syntax (e.g.,
// [x:Integer] rather than {x:Integer}).
// Explicit maps are marked for auto-evaluation (Eval=true).
// The optional meta parameter receives MapRef.Meta for optional field detection.
func convertMapData(m map[string]any, implicit bool, d *parseDepth, meta ...map[string]any) (core.Value, error) {
	if err := d.enter(); err != nil {
		return core.Value{}, err
	}
	defer d.leave()
	om := core.NewOrderedMap()
	if implicit {
		om.Implicit = true
	}
	// Extract metadata from MapRef.Meta.
	var qmSet map[string]bool // optional keys (? syntax)
	var ckSet map[string]bool // computed keys ([key] syntax)
	var qkSet map[string]bool // quoted keys ({'k': v} syntax)
	var shList []string       // shorthand keys ({foo} / {foo/v} syntax)
	var ko []string           // source key order (D1: Meta["ko"] channel)
	if len(meta) > 0 && meta[0] != nil {
		qmSet, _ = meta[0]["qm"].(map[string]bool)
		ckSet, _ = meta[0]["ck"].(map[string]bool)
		qkSet, _ = meta[0]["qk"].(map[string]bool)
		shList, _ = meta[0]["sh"].([]string)
		ko, _ = meta[0]["ko"].([]string)
	}

	// Word modifiers (`/v`, `/q`, `/f`, `/s`, `/N`) are legal only on
	// shorthand entries — `{foo/v}` ≡ `{foo: foo/v}` — where the token is
	// also the value, so the modifier qualifies that value. On a bare
	// `key: value` pair the modifier would only ever land on the KEY,
	// which is meaningless (a map key is a plain name). Reject it rather
	// than silently keeping `f/v` as a literal key. A quoted key
	// (`{'f/v': …}`) or computed key (`{[f/v]: …}`) is an explicit string
	// literal — the slash is data, not a modifier — so those are exempt.
	// Bare explicit keys (with and without an optional `?`) arrive in m;
	// shorthand (sh) and value-less optional (qm) tokens are handled below
	// where the modifier legitimately rides along on the synthesized value.
	for key := range m {
		if qkSet[key] || ckSet[key] {
			continue
		}
		if _, _, _, _, _, _, _, _, hasMod := scanWordModifier(key); hasMod {
			return core.Value{}, &core.BoruError{
				Code: "illegal_key",
				Detail: fmt.Sprintf(
					"word modifier not allowed on map key %q (modifiers qualify values, not keys; quote the key as '%s' for a literal slash)",
					key, key),
			}
		}
	}

	// Shorthand entries: `{foo}` ≡ `{foo: foo}`, `{foo/v}` ≡ `{foo: foo/v}`,
	// `{foo?}` ≡ `{foo?: foo}`, `{foo/v?}` ≡ `{foo?: foo/v}`. The key is
	// always the base name (modifiers stay on the value only); the value
	// is the full word token. synth maps each synthesized base key to that
	// token. Two sources: explicit shorthand tokens (Meta["sh"]) and
	// optional keys (Meta["qm"]) that never received an explicit value.
	//
	// optBase collects the base name of every optional key so optionality
	// is looked up by base name below — qmSet is keyed by the raw token
	// (e.g. `f/v`), which no longer matches the synthesized base key.
	synth := make(map[string]string)
	optBase := make(map[string]bool, len(qmSet))
	for _, raw := range shList {
		synth[wordBaseName(raw)] = raw
	}
	for key := range qmSet {
		base := wordBaseName(strings.TrimSuffix(key, "?"))
		optBase[base] = true
		if _, ok := m[key]; !ok {
			synth[base] = key
		}
	}

	// Iterate the sorted union of value-map keys and synthesized keys.
	keySet := make(map[string]bool, len(m)+len(synth))
	union := make(map[string]any, len(m)+len(synth))
	for k, v := range m {
		union[k] = v
		keySet[k] = true
	}
	for k := range synth {
		if !keySet[k] {
			union[k] = nil
			keySet[k] = true
		}
	}

	for _, key := range orderedKeys(union, ko) {
		var child core.Value
		var err error
		if _, inMap := m[key]; inMap {
			child, err = convertDataValue(m[key], d)
		} else {
			child, err = parseWord(synth[key])
		}
		if err != nil {
			return core.Value{}, err
		}
		// Optional field: `?:T` means "None or absent, universally" —
		// desugared to `disjunct(T, None, Absent)`. The Absent
		// alternative carries the "may be missing" half of the rule
		// (the map unifier synthesises Absent when a key is absent);
		// the None alternative carries the "may be explicitly None"
		// half. No separate metadata needed — the optionality lives
		// entirely in the type. For an EXPLICIT record/map (`{a?:T}`), route
		// through SimplifyDisjunctAlts — the same boundary user-facing `tor`
		// unions pass through — so `{a?:T}` and the explicit
		// `{a:(T tor None tor Absent)}` reduce to the SAME canonically-ordered
		// value (otherwise they build order-divergent disjuncts and record
		// equality compares false). An IMPLICIT map is the fn-param list
		// (`[a?:T]`), whose optional-param expansion reads the base type off
		// the FIRST alternative — keep the parser order there.
		optional := qmSet[key] || optBase[key]
		realKey := key
		if strings.HasSuffix(key, "?") {
			realKey = strings.TrimSuffix(key, "?")
			optional = true
		}
		if optional {
			alts := []core.Value{
				child,
				core.NewTypeLiteral(core.TNone),
				core.NewTypeLiteral(core.TAbsent),
			}
			// An unresolved type-name child (a Word, post-ADR-012) cannot
			// be reasoned about here; the engine's resolve prepass
			// re-simplifies the disjunct after resolving the name, which
			// restores the `{a?:T}` ≡ `{a:(T tor None tor Absent)}`
			// canonical equality at every consumption boundary.
			if !implicit && !core.IsWord(child) {
				alts = core.SimplifyDisjunctAlts(alts)
			}
			child = core.NewDisjunct(alts)
		}
		om.Set(realKey, child)
	}
	// Propagate computed keys to OrderedMap.Meta for autoEvalMap.
	if len(ckSet) > 0 {
		if om.Meta == nil {
			om.Meta = make(map[string]any)
		}
		om.Meta["ck"] = ckSet
	}
	// Explicit maps (from {...} syntax) are marked for auto-evaluation.
	// Implicit maps (from pair syntax [x:Integer]) are structural and not evaluated.
	if !implicit {
		return core.NewEvalMap(om), nil
	}
	return core.NewMap(om), nil
}

// convertDataValue converts a value in data context (inside maps).
// Quoted text → strings, unquoted text → words (executable). The value is
// stamped with the source position captured by the val-rule BC (if any).
func convertDataValue(v any, d *parseDepth) (core.Value, error) {
	node, pos := deSite(v)
	res, err := convertDataValueInner(node, d)
	if err != nil {
		return res, err
	}
	return withPos(res, pos), nil
}

func convertDataValueInner(v any, d *parseDepth) (core.Value, error) {
	switch val := v.(type) {
	case arrowTag:
		// `=>` in data context — the same lambda sugar marker.
		return core.NewSugar(core.SugarInfo{Kind: core.SugarLambda}), nil
	case jsonic.Text:
		if val.Quote != "" {
			// Quoted text (e.g. "hello") → string
			return core.NewString(val.Str), nil
		}
		// Unquoted text → word (same as top-level word context).
		// This allows map values like {r:rv} to evaluate rv.
		return parseWord(val.Str)

	case interpGroup:
		return convertInterpGroup(val, d)

	case miniLitVal:
		// `+name<delim>src<delim>` desugars to the splice `mini name 'src'`
		// (the `word` mechanism): one value that re-steps as the standard
		// mini call, so a stack subject, a trailing opts Map, the
		// unknown-kind error, and check mode all behave exactly as if the
		// user had typed `mini name 'src'`.
		return core.NewSugar(core.SugarInfo{Kind: core.SugarMini, Name: val.Name, Src: val.Src}), nil

	case xmlElemVal:
		// Embedded XML literal in data context (a map value): same
		// immutable Node/Xml value the matcher built.
		if val.Err != nil {
			return core.Value{}, val.Err
		}
		return val.V, nil

	case numberVal:
		return numberValToValue(val)

	case float64:
		return floatToValue(val), nil

	case jsonic.MapRef:
		if hasMapChild(val.Val) {
			return convertTypedMap(val.Val, d, val.Meta)
		}
		return convertMapData(val.Val, val.Implicit, d, val.Meta)

	case map[string]any:
		if hasMapChild(val) {
			return convertTypedMap(val, d)
		}
		return convertMapData(val, true, d)

	case jsonic.ListRef:
		if val.Child != nil {
			return convertTypedList(val, d)
		}
		return convertDataList(val.Val, d)

	case unclosedParen:
		return core.Value{}, core.MakeBoruError("syntax_error", "unmatched opening parenthesis", "(", "", "")

	case parenGroup:
		// Paren group in data context: convert items in word context
		// and wrap as a ParenExpr for inline evaluation by autoEvalMap.
		items, err := convertTopLevelItems([]any(val), d)
		if err != nil {
			return core.Value{}, err
		}
		return core.NewParenExpr(items), nil

	case angleGroup:
		// Use-site sugar in data context — `{x: Box<Integer>}`, the
		// `[:Box<Integer>]` child constraint, `b:Box<Integer>` typed
		// annotations — becomes the same ParenExpr the canonical
		// `(Box of [Integer])` produces here.
		return angleUseSite(val, d)

	case unclosedAngle:
		return core.Value{}, unclosedAngleError(val)

	case bool:
		return core.NewBoolean(val), nil

	case nil:
		// An untyped-nil element is an EMPTY list slot — `[1,,2]`, `[,1]`,
		// a leading/repeated comma. (An explicit `null` arrives as the
		// jsonic.Text token "null", handled above, so this case only fires
		// for a genuinely empty element.) A repeated or leading comma is a
		// typo, not a value — reject it rather than fabricating a `null`.
		return core.Value{}, emptyElementError()

	default:
		return core.Value{}, fmt.Errorf("unsupported value type %T", v)
	}
}

// convertTypedList converts a ListRef with a Child into a typed list value.
// The child value is converted in data context (type names resolve to type literals).
//
// When lr.Val carries concrete elements alongside the child constraint
// (`[v0 :T v1]`), each element is converted in data context and
// retained on the resulting Value's ChildTypeInfo.Elements; the
// runtime `is` validates them against Child on demand.
func convertTypedList(lr jsonic.ListRef, d *parseDepth) (core.Value, error) {
	if err := d.enter(); err != nil {
		return core.Value{}, err
	}
	defer d.leave()
	childVal, err := convertDataValue(lr.Child, d)
	if err != nil {
		return core.Value{}, err
	}
	if len(lr.Val) == 0 {
		return core.NewTypedList(childVal), nil
	}
	elems := make([]core.Value, 0, len(lr.Val))
	for _, item := range lr.Val {
		ev, err := convertDataValue(item, d)
		if err != nil {
			return core.Value{}, err
		}
		elems = append(elems, ev)
	}
	return core.NewTypedListWithElements(childVal, elems), nil
}

// hasMapChild reports whether a jsonic map contains the "child$" key
// set by the map.child option (bare colon syntax {:value}).
func hasMapChild(m map[string]any) bool {
	_, ok := m["child$"]
	return ok
}

// convertTypedMap converts a map with a "child$" key into a typed
// map value. The child value is converted in data context (type
// names resolve to type literals).
//
// When the source carries concrete entries alongside the child
// constraint (`{k:v :T}`), each entry is converted in data context
// and retained on the resulting Value's ChildTypeInfo.Entries; the
// runtime `is` validates each entry's value against Child on demand.
func convertTypedMap(m map[string]any, d *parseDepth, meta ...map[string]any) (core.Value, error) {
	if err := d.enter(); err != nil {
		return core.Value{}, err
	}
	defer d.leave()
	childVal, err := convertDataValue(m["child$"], d)
	if err != nil {
		return core.Value{}, err
	}
	// Collect non-`child$` entries as concrete values, in SOURCE order
	// (D1 — design/FLEX-ATTRS.1.md §3). The `child$` constraint is not a
	// pair, so the order channel never records it; drop it from the union
	// before ordering.
	var ko []string
	if len(meta) > 0 && meta[0] != nil {
		ko, _ = meta[0]["ko"].([]string)
	}
	union := make(map[string]any, len(m))
	for k, v := range m {
		if k == "child$" {
			continue
		}
		union[k] = v
	}
	if len(union) == 0 {
		return core.NewTypedMap(childVal), nil
	}
	entries := make([]core.ChildEntry, 0, len(union))
	for _, k := range orderedKeys(union, ko) {
		ev, err := convertDataValue(m[k], d)
		if err != nil {
			return core.Value{}, err
		}
		entries = append(entries, core.ChildEntry{Key: k, Value: ev})
	}
	return core.NewTypedMapWithEntries(childVal, entries), nil
}

// angleUseSite converts an angle group to ONE structural sugar marker
// (ADR-012 amendment) carrying both precomputed forms: the use-site
// type args (Items) and the generic-def head params (Head, or HeadErr
// when the items are not a valid param list). The engine picks the
// form at dispatch. Each item converts in word context; nested sugar
// (`Box<Pair<A, B>>`) recurses through the angleGroup case.
func angleUseSite(ag angleGroup, d *parseDepth) (core.Value, error) {
	args := make([]core.Value, 0, len(ag.Items))
	for _, it := range ag.Items {
		v, err := convertTopLevelValue(it, d)
		if err != nil {
			return core.Value{}, err
		}
		args = append(args, v)
	}
	// Both forms are precomputed; the ENGINE picks (ADR-012 rule 3
	// amendment): a marker arriving at a binder's /q name slot lowers
	// to the generic-def head (`Name gen [params]`), anywhere else to
	// the use-site apply. A head form whose items are not a valid
	// param list carries the error text instead — it surfaces only if
	// the head form is actually selected.
	info := core.SugarInfo{Kind: core.SugarAngle, Name: ag.Name, Items: args}
	if head, herr := angleGenList(ag.Items, core.SrcPos{}, d); herr == nil {
		info.Head = head
	} else {
		info.HeadErr = herr.Error()
	}
	return core.NewSugar(info), nil
}

// angleGenList desugars an angle group's items to the canonical
// generic-def head parameter list (the marker's precomputed Head,
// used only if the engine selects the head form):
//
//	T                 → Word(T)
//	T extends C       → ParenExpr[ Word(T) Word(extends) C ]
//	T = D             → ParenExpr[ Word(T) sugar(gen-default) D ]
//
// `extends` rides as the user-written word itself; `=` is not a legal
// word name, so it rides as the gen-default sugar marker.
//
// Items arrive as a flat val sequence (commas are optional separators,
// like list elements), so entries are recognised by the grammar above:
// a capitalised name, optionally followed by an `extends`/`=` operator
// and its operand.
func angleGenList(items []any, pos core.SrcPos, d *parseDepth) (core.Value, error) {
	entries := make([]core.Value, 0, len(items))
	i := 0
	for i < len(items) {
		node, epos := deSite(items[i])
		txt, ok := node.(jsonic.Text)
		if !ok || txt.Quote != "" || txt.Str == "" || !(txt.Str[0] >= 'A' && txt.Str[0] <= 'Z') {
			return core.Value{}, &core.BoruError{
				Code:   "syntax_error",
				Detail: "generic parameter must be a capitalised name",
				Hint:   "write def Name<T> / def Name<T extends Bound> / def Name<T = Default>",
				Row:    epos.Row, Col: epos.Col, Src: epos.Src,
			}
		}
		name := txt.Str
		if i+1 < len(items) {
			op, _ := deSite(items[i+1])
			if opTxt, isTxt := op.(jsonic.Text); isTxt && opTxt.Quote == "" {
				var kw string
				switch opTxt.Str {
				case "extends":
					kw = "extends"
				case "=":
					kw = "default"
				}
				if kw != "" {
					if i+2 >= len(items) {
						return core.Value{}, &core.BoruError{
							Code:   "syntax_error",
							Detail: "generic parameter " + name + ": " + opTxt.Str + " needs a value",
							Row:    epos.Row, Col: epos.Col, Src: epos.Src,
						}
					}
					operand, err := convertTopLevelValue(items[i+2], d)
					if err != nil { //covergate:allow angleUseSite's Items loop converts every raw item with the same converter before angleGenList runs, so a failing operand errors there first
						return core.Value{}, err
					}
					op := core.NewWord(opTxt.Str)
					if kw == "default" {
						// `=` is not a legal word name — it rides as
						// the gen-default sugar marker; `extends` is
						// the user-written word itself.
						op = core.NewSugar(core.SugarInfo{Kind: core.SugarGenDefault})
					}
					entries = append(entries, withPos(core.NewParenExpr([]core.Value{
						core.NewWord(name),
						op,
						operand,
					}), epos))
					i += 3
					continue
				}
			}
		}
		entries = append(entries, withPos(core.NewWord(name), epos))
		i++
	}
	return withPos(core.NewEvalList(entries), pos), nil
}

// unclosedAngleError is raised for an angle group auto-closed at EOF.
func unclosedAngleError(ua unclosedAngle) error {
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: "unclosed angle bracket: " + ua.Name + "<… has no matching `>`",
	}
}

// convertDataList converts a list in data context (inside maps).
// Lists use word context and are marked for auto-evaluation.
func convertDataList(items []any, d *parseDepth) (core.Value, error) {
	elems, err := convertTopLevelItems(items, d)
	if err != nil {
		return core.Value{}, err
	}
	return core.NewEvalList(elems), nil
}

// resolveTextValue converts a bare text string into the appropriate
// boru value — type literal, boolean, or atom.
// Unquoted text is never a string; only quoted text produces strings.
func resolveTextValue(text string) core.Value {
	if text == "true" {
		return core.NewBoolean(true)
	}
	if text == "false" {
		return core.NewBoolean(false)
	}
	switch text {
	case "inf":
		return core.NewFloat(math.Inf(1))
	case "-inf":
		return core.NewFloat(math.Inf(-1))
	case "nan":
		return core.NewFloat(math.NaN())
	}
	// Type names are not resolved (ADR-012 rule 4 — parseWord has the
	// same rule); a capitalised name is data here, an Atom.
	return core.NewAtom(text)
}

// orderedKeys returns the keys of a map in SOURCE order — the D1
// insertion-order default (design/FLEX-ATTRS.1.md §3). `ko` is the source
// key order captured by the grammar's `Meta["ko"]` channel
// (grammar.go): keys present in `ko` are emitted in that order (first-
// position/last-value — a repeated literal key keeps its first slot,
// matching OrderedMap.Set), then any remaining union keys the channel did
// not cover are appended in sorted order.
//
// The sorted tail is the determinism fallback for map values that reach
// this path WITHOUT an order channel — a map assembled off the pair
// grammar (e.g. the synthesized `{child$:…}` split, or any future
// non-pair producer) hands an UNORDERED Go map, where source order is
// unrecoverable and sorted is the only stable choice. When `ko` covers
// every key (the common literal case) the tail is empty and output is
// pure source order.
func orderedKeys(union map[string]any, ko []string) []string {
	out := make([]string, 0, len(union))
	seen := make(map[string]bool, len(union))
	for _, k := range ko {
		if seen[k] {
			continue
		}
		if _, ok := union[k]; ok {
			out = append(out, k)
			seen[k] = true
		}
	}
	rest := make([]string, 0, len(union))
	for k := range union {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	// sort.Strings, not a hand-rolled insertion sort: `rest` is filled
	// from a Go map range, so whether an insertion sort's inner swap
	// ever executed depended on the randomised iteration order — the
	// statement was covered or not from run to run, making the 100%
	// gate (ADR-008) nondeterministically red. One library call has no
	// such conditional interior. Same ascending order as before.
	sort.Strings(rest)
	return append(out, rest...)
}

// scanWordModifier parses the optional `/...` modifier suffix of an
// unquoted word token: name/f (forceForward), name/s (forceStack),
// name/N (argCount), name/q (quote → Atom), name/v (ref → bound value),
// name/u (usurp → reversed-sig wrapper), and combinations like name/1f,
// name/qs, name/f2, name/uv. Modifiers stack in any order; f and s are
// mutually exclusive; q is mutually exclusive with both r and u (an atom
// has no binding to reference or usurp); u may combine with r (name/uv);
// the argCount digits form a single number. When the token has no `/` or a
// malformed modifier suffix, valid is false, base is the whole text, and
// every flag is at its zero value (argCount -1).
func scanWordModifier(text string) (base string, argCount int, forceStack, forceForward, quoteFlag, valFlag, usurpFlag, typeFlag, valid bool) {
	argCount = -1

	idx := strings.LastIndex(text, "/")
	if idx < 0 || idx >= len(text)-1 {
		return text, argCount, false, false, false, false, false, false, false
	}
	mod := text[idx+1:]
	baseName := text[:idx]

	// Scan modifier chars in any order: digits, 'f', 's', 'q', 'v', 'u',
	// 't'. Each letter appears at most once; f/s are mutually exclusive;
	// q is mutually exclusive with v and u; t (the type-bound sugar,
	// `Map/t` ≡ `(Type of [Map])`) combines with nothing — it produces a
	// type expression, not a word; digits run contiguously and form a
	// single argCount value.
	valid = true
	seenDigits := false
	i := 0
	for i < len(mod) {
		c := mod[i]
		switch {
		case c >= '0' && c <= '9':
			if seenDigits {
				valid = false
			} else {
				j := i
				for j < len(mod) && mod[j] >= '0' && mod[j] <= '9' {
					j++
				}
				n, err := strconv.Atoi(mod[i:j])
				if err != nil || n < 0 {
					valid = false
				} else {
					argCount = n
					seenDigits = true
				}
				i = j
				continue
			}
		case c == 'f':
			if forceForward || forceStack {
				valid = false
			} else {
				forceForward = true
			}
		case c == 's':
			if forceForward || forceStack {
				valid = false
			} else {
				forceStack = true
			}
		case c == 'q':
			if quoteFlag || valFlag || usurpFlag {
				valid = false
			} else {
				quoteFlag = true
			}
		case c == 'v':
			if valFlag || quoteFlag {
				valid = false
			} else {
				valFlag = true
			}
		case c == 'u':
			if usurpFlag || quoteFlag {
				valid = false
			} else {
				usurpFlag = true
			}
		case c == 't':
			if typeFlag {
				valid = false
			} else {
				typeFlag = true
			}
		default:
			valid = false
		}
		if !valid {
			break
		}
		i++
	}

	// /t combines with nothing — any companion flag invalidates, in
	// either order.
	if typeFlag && (quoteFlag || valFlag || usurpFlag || forceStack || forceForward || argCount >= 0) {
		valid = false
	}
	if !valid {
		// Unrecognized / malformed modifier — treat entire token as plain word.
		// parseWord upgrades the all-modifier-alphabet subset of this case
		// (a botched modifier like foo/fs, never a legitimate name) to a
		// loud parse error — see isModifierAlphabet (NUR027).
		return text, -1, false, false, false, false, false, false, false
	}
	return baseName, argCount, forceStack, forceForward, quoteFlag, valFlag, usurpFlag, typeFlag, true
}

// isModifierAlphabet reports whether every character of a candidate
// modifier suffix is drawn from the modifier alphabet (digits plus
// f s q v u t). It is the line between a BOTCHED MODIFIER (`foo/fs`,
// `foo/qv`, `foo/1f2` — all-alphabet but an invalid combination),
// which errors loudly, and a slash-bearing plain name (`foo/bar`,
// `add/x`, the builtin type paths `Scalar/Number/Integer` — the
// suffix contains a non-modifier character, uppercase included),
// which stays a single plain word exactly as before. Valid modifier
// combinations never reach this: scanWordModifier claims them first.
func isModifierAlphabet(s string) bool {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
		case c == 'f', c == 's', c == 'q', c == 'v', c == 'u', c == 't':
		default:
			return false
		}
	}
	return true
}

// wordBaseName returns the base name of an unquoted word token, stripping
// a valid `/...` modifier suffix (foo/v → foo, foo → foo). Used to derive
// the key of a shorthand map entry, whose value keeps the full token.
func wordBaseName(text string) string {
	base, _, _, _, _, _, _, _, _ := scanWordModifier(text)
	return base
}

// parseWord interprets an unquoted text token as a boru word, handling
// the modifier syntax decoded by scanWordModifier. q produces an Atom and
// overrides the other modifiers; u emits a usurp-word and r emits a
// ref-word, both of which short-circuit the rest.
// reachSegmentName restores the dot-access lowering identity for a segment
// spelled as a BARE NAME. Dot access IS a `get`/`getr` chain
// (design/REACH.10.md is the single source of truth for the lowering), so
// `m.k` and `m get k/q` are meant to differ in spelling only. They did not:
// the reserved VALUE literals — `none`, `end`, `inf`, `-inf`, `nan` —
// resolve to their values in parseWord before the chain sees them, so the
// segment arrived as a marker or a Float and no `dot` signature matched
// (NUR066). The `/q` form never had the problem, because a modifier suffix
// returns an atom before the literal switch is reached, which is why
// `m get none/q` read the field all along.
//
// The rule is DERIVED rather than a list of names: a segment whose source
// token is an unquoted valid word name that parseWord turned into
// something OTHER than a Word is exactly a reserved literal, and as a
// field name it means the name. A new literal joins parseWord and this
// stays in step with no second edit.
//
// Non-names are untouched and keep their literal readings: `m.1` indexes,
// `m.'k'` is a string key, `m.(expr)` is computed (handled by the caller),
// and the punctuation-spelled markers fail ValidateWordName — `;` among
// them, which is the reason the grammar hands the semicolon its own text
// instead of rewriting it to `end`. `m.;` therefore stays the error it was
// rather than silently reading a field called `end`.
func reachSegmentName(keyItem any, key core.Value, pos core.SrcPos) core.Value {
	if core.IsWord(key) {
		return key
	}
	node, _ := deSite(keyItem)
	text, ok := node.(jsonic.Text)
	if !ok || text.Quote != "" {
		return key
	}
	if core.ValidateWordName(text.Str) != nil {
		return key
	}
	return withPos(core.NewWord(text.Str), pos)
}

// bareModifierError refuses a `/` modifier with nothing before it (`/s`,
// `/v`, `/2`): a modifier follows the word or group it modifies. It used to
// leave the parser as a plain `empty word` error — the one parse failure
// that was no syntax_error, in both ports (NUR060).
func bareModifierError(text string) error {
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: "`" + text + "` modifies nothing: a `/` modifier follows the word or group it modifies",
	}
}

func parseWord(text string) (core.Value, error) {
	name, argCount, forceStack, forceForward, quoteFlag, valFlag, usurpFlag, typeFlag, valid := scanWordModifier(text)

	if name == "" {
		return core.Value{}, bareModifierError(text)
	}

	// An invalid modifier combination spelled entirely from the modifier
	// alphabet is a PARSE ERROR (NUR027) — previously the whole token
	// silently became a slash-bearing word that surfaced later as an
	// obscure undefined_word. A suffix containing any non-modifier
	// character keeps the historical plain-word reading (slash-bearing
	// names, builtin type paths).
	if !valid {
		if idx := strings.LastIndex(text, "/"); idx >= 0 && idx < len(text)-1 && isModifierAlphabet(text[idx+1:]) {
			return core.Value{}, &core.BoruError{
				Code:   "syntax_error",
				Detail: fmt.Sprintf("invalid word modifier /%s on %q", text[idx+1:], text[:idx]),
				Src:    text,
				Hint:   "modifier letters stack in any order, each at most once; f|s are exclusive; q excludes v and u; t combines with nothing; digits form one contiguous run within int range",
			}
		}
	}

	// `X/t` — the type-bound sugar: desugars to the paren group
	// `(Type of [X/q])`, the bounded-Type application (≡ `Type<X>`).
	// X rides as an ATOM inside the argument list: it survives list
	// auto-eval untouched, so `of` sees the NAME and can bind a named
	// structural type (a disjunct, a record) to its MINTED lattice
	// node — a word would resolve to the body first. Resolution
	// happens entirely inside `of`; a non-type X fails there with
	// of's error — the parser owns no semantics.
	if typeFlag {
		bound := core.NewList([]core.Value{core.NewAtom(name)})
		return core.NewSugar(core.SugarInfo{Kind: core.SugarTypeBound, Items: []core.Value{bound}}), nil
	}

	// `0d…` arbitrary-precision literals (BigInteger / BigDecimal). The
	// lexer matcher claims the whole run as one text token (so an embedded
	// `.` isn't split off); here we build the value. Checked early — these
	// never take modifiers and must not fall through to word/number paths.
	if isBigNumberLiteral(name) {
		if hasUppercaseNumericPrefix(name) {
			return core.Value{}, uppercaseNumericPrefixError(name, 0, 0)
		}
		return parseBigNumber(name)
	}

	// /q produces an Atom: equivalent to (quote name). Other modifiers
	// in the same suffix are accepted but ignored — the atom is data,
	// not a function call.
	if quoteFlag {
		return core.NewAtom(name), nil
	}

	// /u emits a usurp-word that resolves the name to its bound Function
	// value and wraps it with reversed signature arg order. Legal only for
	// function words (illegal_ref at run time otherwise). It may combine
	// with /v: /u alone dispatches the wrapper, /uv leaves it as data.
	// Argument-shape modifiers don't apply (the wrapper supplies its own).
	if usurpFlag {
		return core.NewWordUsurp(name, valFlag), nil
	}

	// /v emits a ref-word that, when reached at the pointer, resolves the
	// name to its bound Function value without invoking. /v is legal only
	// for function words; a non-fn binding raises illegal_ref at run time
	// (the parser accepts the syntax — the binding kind isn't known until
	// resolution). Argument-shape modifiers don't apply because ref
	// bypasses dispatch entirely; they're accepted syntactically but
	// ignored.
	if valFlag {
		return core.NewWordRef(name), nil
	}

	if forceStack || forceForward || argCount >= 0 {
		return core.NewWordModified(name, argCount, forceStack, forceForward), nil
	}

	// `none` is the unique inhabitant of None — a value, not a type.
	// `null` is the JSON-null atom (handled in data context via `case
	// nil:`; here in word context bare `null` falls through as a Word
	// for engine-side resolution).
	if name == "none" {
		return core.NewNone(), nil
	}

	// IEEE-754 special-value literals, following the boru reserved-literal
	// convention (lowercase, parser-emitted, like true / false / none):
	// `inf` / `-inf` / `nan` produce the corresponding Float. They render
	// back to these same tokens (see FormatFloat), so print∘parse is
	// identity. `inf negate` is the long form for -inf.
	switch name {
	case "inf":
		return core.NewFloat(math.Inf(1)), nil
	case "-inf":
		return core.NewFloat(math.Inf(-1)), nil
	case "nan":
		return core.NewFloat(math.NaN()), nil
	}

	// Reserved tape-syntax tokens emit typed marker values so the
	// engine recognises them by Parent identity (parens, end / ';').
	// These would otherwise become plain Word values that the engine
	// would have to name-dispatch in stepWord.
	//
	// `;` carries its OWN text rather than being rewritten to `end` in
	// the grammar (as `)` already does): the two spell the same value but
	// are not the same source, and a dot-path segment has to tell them
	// apart — `m.end` names a field, `m.;` is a stray terminator
	// (reachSegmentName, NUR066).
	switch name {
	case "end", ";":
		return core.NewEnd(), nil
	case ")":
		return core.NewCloseParen(), nil
	}

	// Type names are NOT resolved here — the parser is type-name-opaque
	// (ADR-012 rule 4): a capitalised name is an ordinary Word in every
	// context, and the engine's canonical cascade (eng/go/resolve.go)
	// resolves it at consumption, exactly as user-defined type names
	// have always resolved. Quotation meaning is consumption-time.

	// A base-prefixed integer token whose magnitude jsonic could not lex
	// as a number (>= 2^63) arrives here as bare text. Parse it exactly:
	// `-0x8000000000000000` (= int64 min) succeeds, while a truly
	// out-of-range magnitude becomes a clean [boru/integer_overflow]
	// instead of an opaque undefined_word. (In-range base-prefixed
	// literals never reach this path — jsonic lexes them as numbers.)
	if isBasePrefixedInteger(name) {
		// No uppercase check here: both lexers CLAIM a base-prefixed run as
		// #NR whatever its magnitude, so an uppercase one is declined in
		// numberValToValue and can never reach this text path.
		if strings.IndexByte(name, '_') >= 0 && !validUnderscores(name) {
			return core.Value{}, &core.BoruError{Code: "syntax_error", Src: name,
				Detail: "misplaced `_` in numeric literal: " + name,
				Hint:   "`_` is a single digit-separator — use one between digits"}
		}
		n, err := strconv.ParseInt(stripUnderscores(name), 0, 64)
		if err == nil {
			return core.NewInteger(n), nil
		}
		if isRangeError(err) {
			return core.Value{}, integerLiteralOverflowError(name, 0, 0)
		}
		// Other parse errors (e.g. an invalid digit) fall through to the
		// numeric-shape classification below.
	}

	// A digit-led token reached here only because jsonic could not lex it
	// as a number — and no word may start with a digit (see
	// ValidateWordName), so it can only be a malformed or out-of-range
	// numeric literal. Classify it for a clear diagnostic instead of an
	// opaque "undefined word": a value ParseFloat recognises but reports
	// out of range overflowed binary64; anything else is malformed (e.g.
	// `1e`, `2dup` — the renamed-away stack words are now `dup2` etc.).
	if isDigitLed(name) {
		f, err := strconv.ParseFloat(stripUnderscores(name), 64)
		if isRangeError(err) || math.IsInf(f, 0) {
			return core.Value{}, floatLiteralOverflowError(name)
		}
		return core.Value{}, malformedNumberError(name)
	}

	return core.NewWord(name), nil
}

// isDigitLed reports whether s starts (after an optional sign) with a
// decimal digit. Because no word may begin with a digit
// (ValidateWordName), a digit-led token is always a numeric literal —
// never an identifier.
func isDigitLed(s string) bool {
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	return len(s) > 0 && s[0] >= '0' && s[0] <= '9'
}

// isRangeError reports whether err is a strconv range (overflow) error.
func isRangeError(err error) bool {
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

// floatLiteralOverflowError reports a floating-point literal whose
// magnitude overflows binary64 to ±infinity (e.g. 1e309).
func floatLiteralOverflowError(src string) error {
	return &core.BoruError{
		Code:   "float_overflow",
		Detail: "floating-point literal out of range: " + src + " overflows to infinity",
		Src:    src,
		Hint:   "the Float range is about ±1.8e308; write the `inf` literal for infinity",
	}
}

// malformedNumberError reports a digit-led token that is not a valid
// number. Since no word may start with a digit, such a token can only be
// a botched numeric literal (`1e`, `0x1p4`) or a digit-first name (`2dup`
// — the stack words are now `dup2`/`swap2`/`drop2`/`over2`).
func malformedNumberError(src string) error {
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: "invalid numeric literal: " + src,
		Src:    src,
		Hint:   "a number is digits with an optional sign, base prefix (0x/0o/0b), `.`, exponent (`e`), and single `_` separators — and a name cannot start with a digit",
	}
}

// convertInterpGroup converts an interpGroup (produced by the interp/ielem/iexpr
// jsonic rules) into an engine InterpString value, or a plain string if there
// are no expression parts.
func convertInterpGroup(grp interpGroup, d *parseDepth) (core.Value, error) {
	if len(grp) == 0 {
		return core.NewString(""), nil
	}
	var parts []core.InterpPart
	hasExpr := false
	for _, item := range grp {
		item, _ := deSite(item) // interp parts are not normally sited; guard anyway
		switch v := item.(type) {
		case jsonic.Text:
			// Template literal segment (Quote="tl").
			parts = append(parts, core.InterpPart{Lit: v.Str})
		case iexprGroup:
			if len(v) == 0 {
				// An empty hole (`${}`, `${ }`) holds no expression and
				// contributes nothing: a template whose holes are all empty
				// is the plain string it spells, as `abc` in backticks is
				// (NUR060) — and as an XML attribute's empty hole folds.
				continue
			}
			hasExpr = true
			exprVals, err := convertTopLevelItems([]any(v), d)
			if err != nil {
				return core.Value{}, fmt.Errorf("interpolation expression error: %w", err)
			}
			parts = append(parts, core.InterpPart{Expr: exprVals})
		default:
			return core.Value{}, fmt.Errorf("unexpected interp part type %T", item)
		}
	}
	if !hasExpr {
		// No interpolations — just concatenate literals into a plain string.
		var buf strings.Builder
		for _, p := range parts {
			buf.WriteString(p.Lit)
		}
		return core.NewString(buf.String()), nil
	}
	return core.NewInterpString(parts), nil
}

// processTemplateEscapes processes escape sequences in template literal text.
func processTemplateEscapes(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i += writeStringEscape(&buf, s, i+1)
		} else {
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}

// writeStringEscape decodes ONE escape sequence — the character(s) after
// a backslash at s[at:] — into buf, and returns how many bytes of s it
// consumed (always >= 1, counting the escape character itself).
//
// This is the single escape vocabulary for boru string literals
// (NUR026). Templates used to carry a hand-rolled six-case switch
// (`\n \t \r \\ ` $`) that kept everything else LITERAL, while quoted
// strings rode jsonic's native handling and got the full set — so
// `size "z\x41z"` was 3 and its template spelling 6. That was an
// implementation accident, not a design choice: the backtick was
// removed from jsonic's StringChars so templates could carry `${…}`
// interpolation, and the replacement escape handler was never brought
// to parity. A reader should not have to know which quoting form they
// are in to know what `\x41` means.
//
// The vocabulary matches what jsonic accepts for `"…"` / `'…'`,
// measured 2026-08-15:
//
//	\n \t \r \b \f \v   the control characters
//	\xNN                one byte, two hex digits
//	\uNNNN              one rune, four hex digits
//	anything else       the character itself, backslash DROPPED
//
// That last rule is the behaviour change to watch, and it is the one
// the record asked to pin: `\z` is `z` and `\0` is `0`, in a template
// exactly as in a quoted string. The template-only spellings need no
// case of their own — a backslash before a backtick or a `$` falls into
// the default arm and yields the bare character, which is what they
// always meant.
//
// One asymmetry SURVIVES here, deliberately and recorded: a MALFORMED
// \x / \u (too few digits, or a non-hex digit) falls through to the
// default arm, so a template's `\xZZ` is `xZZ`, while jsonic RAISES on
// the same sequence in a quoted string ("does not encode a valid ASCII
// character" — measured 2026-08-15, not assumed). Matching that needs
// an error channel this call site does not have: the caller is a
// jsonic LexMatcher returning a *Token, so raising means changing the
// lexer seam, which is the unified-lexer work NUR026's earlier verdict
// sketched and this one did not ask for. The vocabulary — what a
// well-formed escape MEANS — is now identical in both forms, which is
// the divergence the record was opened for; the malformed-input
// REPORTING difference is narrower and stays on the record.
func writeStringEscape(buf *strings.Builder, s string, at int) int {
	switch c := s[at]; c {
	case 'n':
		buf.WriteByte('\n')
	case 't':
		buf.WriteByte('\t')
	case 'r':
		buf.WriteByte('\r')
	case 'b':
		buf.WriteByte('\b')
	case 'f':
		buf.WriteByte('\f')
	case 'v':
		buf.WriteByte('\v')
	case 'x':
		if r, ok := parseHexEscape(s, at+1, 2); ok {
			buf.WriteByte(byte(r))
			return 3
		}
		buf.WriteByte(c)
	case 'u':
		if at+1 < len(s) && s[at+1] == '{' {
			// The braced form, `\u{1F600}`: 1-6 hex digits, any code point.
			if r, n, ok := parseBracedEscape(s, at+2); ok {
				buf.WriteRune(rune(r))
				return n + 3
			}
			buf.WriteByte(c)
			break
		}
		if r, ok := parseHexEscape(s, at+1, 4); ok {
			// A UTF-16 surrogate pair split across two escapes is one code
			// point, as jsonic reads it in a quoted string (a lone surrogate
			// becomes U+FFFD through WriteRune).
			if r >= 0xD800 && r <= 0xDBFF && at+11 <= len(s) && s[at+5] == '\\' && s[at+6] == 'u' {
				if lo, ok := parseHexEscape(s, at+7, 4); ok && lo >= 0xDC00 && lo <= 0xDFFF {
					buf.WriteRune(rune(0x10000 + (r-0xD800)<<10 + (lo - 0xDC00)))
					return 11
				}
			}
			buf.WriteRune(rune(r))
			return 5
		}
		buf.WriteByte(c)
	default:
		// Unknown escape: the character itself, backslash dropped —
		// jsonic's rule for a quoted string, now the template's too.
		buf.WriteByte(c)
	}
	return 1
}

// parseBracedEscape reads a braced code point, `{` already consumed: 1-6 hex
// digits at s[at:] and a closing `}`, at most U+10FFFF. n is the digit count.
func parseBracedEscape(s string, at int) (r uint32, n int, ok bool) {
	end := strings.IndexByte(s[min(at, len(s)):], '}')
	if end < 1 || end > 6 {
		return 0, 0, false
	}
	v, ok := parseHexEscape(s, at, end)
	if !ok || v > 0x10FFFF {
		return 0, 0, false
	}
	return v, end, true
}

// escapeFault reports a malformed `\x` / `\u` escape — the one definition a
// template's text and a quoted string's body both answer to (NUR026). at
// indexes the character after the backslash; stop is the form's closing
// delimiter, which a reported span never crosses. code is jsonic's
// (invalid_ascii / invalid_unicode) and end bounds the offending span, which
// runs from the backslash; code is "" for a well-formed or other escape.
func escapeFault(s string, at int, stop byte) (code string, end int) {
	span := func(n int) int {
		for i := at; i < at-1+n; i++ {
			if i >= len(s) || s[i] == stop {
				return i
			}
		}
		return at - 1 + n
	}
	switch s[at] {
	case 'x':
		if _, ok := parseHexEscape(s, at+1, 2); !ok {
			return "invalid_ascii", span(4)
		}
	case 'u':
		if at+1 < len(s) && s[at+1] == '{' {
			if _, _, ok := parseBracedEscape(s, at+2); !ok {
				// The span runs through the closing `}` when one comes
				// before the delimiter, else to the delimiter or the end.
				rest := s[at:]
				c, d := strings.IndexByte(rest, '}'), strings.IndexByte(rest, stop)
				if c >= 0 && (d < 0 || c < d) {
					return "invalid_unicode", at + c + 1
				}
				return "invalid_unicode", span(len(rest) + 1)
			}
		} else if _, ok := parseHexEscape(s, at+1, 4); !ok {
			return "invalid_unicode", span(6)
		}
	}
	return "", 0
}

// badEscapeToken is the lexer's refusal of a malformed escape: a bad token
// over the escape itself, carrying jsonic's code, so a template and a quoted
// string report it alike — `invalid ascii escape: \xZZ` (NUR026).
func badEscapeToken(lex *jsonic.Lex, code, src string) *jsonic.Token {
	tkn := lex.Token("#BD", jsonic.TinBD, nil, src)
	tkn.Why = code
	return tkn
}

// parseHexEscape reads exactly n hex digits at s[at:] and returns their
// value. ok=false when the run is short or contains a non-hex digit, so
// the caller can fall back to the literal-character reading.
func parseHexEscape(s string, at, n int) (uint32, bool) {
	if at+n > len(s) {
		return 0, false
	}
	var v uint32
	for i := at; i < at+n; i++ {
		var d uint32
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint32(c-'A') + 10
		default:
			return 0, false
		}
		v = v<<4 | d
	}
	return v, true
}

// numberVal wraps a float64 with its source text and source position so
// we can (1) distinguish integer literals (e.g. "5") from decimal
// literals (e.g. "5.0"), (2) parse integers from their exact digits, and
// (3) locate an out-of-range integer literal in the source. Injected by
// the jsonic Sub callback for every number token (setupNumberSub).
type numberVal struct {
	Val      float64
	Src      string
	Row, Col int // 1-based source position of the literal (0 = unknown)
}

// floatToValue converts a JSON float64 to the appropriate boru numeric value.
// Whole numbers become integers; fractional values become decimals.
func floatToValue(f float64) core.Value {
	if f == float64(int64(f)) && !math.IsInf(f, 0) && !math.IsNaN(f) {
		return core.NewInteger(int64(f))
	}
	return core.NewFloat(f)
}

// numberValToValue converts a numberVal (float64 + source) to the
// appropriate boru numeric value.
//
//   - A source containing "." is always a Float (even whole-valued, e.g.
//     5.0), using the float64 jsonic produced.
//   - A *plain decimal integer* source (optional leading '-', then digits
//     and '_' separators only — no base prefix, no exponent) is parsed
//     from its exact digits with strconv.ParseInt. This is the fix for
//     WAT Exhibit K: routing through float64 silently corrupts any
//     integer above 2^53 (e.g. 9007199254740993 → ...992) and turns
//     near-int64-max literals into Floats. An out-of-int64-range literal
//     now raises [boru/integer_overflow] instead of silently degrading.
//   - Everything else (scientific notation like 1e3, or a base prefix
//     like 0x10 / 0o17 / 0b101) keeps the existing float64-derived path,
//     which already matches jsonic's own base interpretation.
//
// See design/INTEGER-OVERFLOW-STRATEGY.5.md.
func numberValToValue(nv numberVal) (core.Value, error) {
	// `_` is a single digit-separator only: it must sit between two
	// digits (no leading, trailing, or repeated underscores). `1__0` and
	// `1_` are rejected.
	if strings.IndexByte(nv.Src, '_') >= 0 && !validUnderscores(nv.Src) {
		return core.Value{}, underscoreError(nv)
	}
	if strings.Contains(nv.Src, ".") {
		// A leading `.` (with or without a sign) is not a valid number —
		// `.5`, `-.5`, `+.5` are rejected; write `0.5` / `-0.5`. The bare
		// `.5` is caught earlier as a stray `.` token; the signed forms
		// reach here because jsonic lexes them as one number token.
		if isLeadingDotFloat(nv.Src) {
			return core.Value{}, leadingDotNumberError(nv)
		}
		// Parse the Float from its exact source digits for a guaranteed
		// correctly-rounded (round-ties-to-even) decimal→binary64
		// conversion, rather than trusting the float64 jsonic produced.
		// strconv.ParseFloat is IEEE-correct; fall back to jsonic's value
		// only if the (jsonic-validated) token somehow fails to reparse.
		if f, err := strconv.ParseFloat(stripUnderscores(nv.Src), 64); err == nil {
			return core.NewFloat(f), nil
		} else if isRangeError(err) || math.IsInf(f, 0) {
			return core.Value{}, floatLiteralOverflowError(nv.Src)
		}
		return core.NewFloat(nv.Val), nil
	}
	if isPlainDecimalInteger(nv.Src) {
		n, err := strconv.ParseInt(stripUnderscores(nv.Src), 10, 64)
		if err != nil {
			return core.Value{}, integerLiteralOverflowError(nv.Src, nv.Row, nv.Col)
		}
		return core.NewInteger(n), nil
	}
	if hasUppercaseNumericPrefix(nv.Src) {
		return core.Value{}, uppercaseNumericPrefixError(nv.Src, nv.Row, nv.Col)
	}
	if isBasePrefixedInteger(nv.Src) {
		// base 0 auto-detects the 0x / 0o / 0b prefix; parsing the exact
		// digits keeps full precision above 2^53 and range-checks like
		// the decimal path (out of int64 range → integer_overflow).
		n, err := strconv.ParseInt(stripUnderscores(nv.Src), 0, 64)
		if err != nil {
			return core.Value{}, integerLiteralOverflowError(nv.Src, nv.Row, nv.Col)
		}
		return core.NewInteger(n), nil
	}
	// Scientific notation without a dot (`1e400`) reaches this tail. Keep
	// an overflowing literal an error rather than silently manufacturing
	// the special `inf` value; oversized plain integers were classified by
	// the exact ParseInt branch above and remain integer_overflow.
	if math.IsInf(nv.Val, 0) {
		return core.Value{}, floatLiteralOverflowError(nv.Src)
	}
	return floatToValue(nv.Val), nil
}

// ConvertParsedNumber converts a jsonic parse-result element that the
// number-Sub wrapped in a numberVal (see SafeParseData / setupNumberSub)
// into its boru numeric Value, preserving the int/float distinction carried
// by the source text. Returns ok=false for any non-numberVal input so the
// caller falls through to its default conversion. The numberVal type stays
// unexported; this is the single public seam data-decode paths use.
func ConvertParsedNumber(v any) (core.Value, bool, error) {
	if nv, ok := v.(numberVal); ok {
		ev, err := numberValToValue(nv)
		return ev, true, err
	}
	return core.Value{}, false, nil
}

// isBigNumberLiteral reports whether src is a `0d`/`0D`-prefixed literal
// (optional leading sign). The lexer matcher claims these as one text
// token; parseWord routes them here.
func isBigNumberLiteral(src string) bool {
	s := src
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	return len(s) >= 3 && s[0] == '0' && (s[1] == 'd' || s[1] == 'D')
}

// parseBigNumber converts a `0d…` literal string to a BigInteger or
// BigDecimal. A `.` or exponent makes it a BigDecimal (apd-parsed, exact);
// otherwise a BigInteger (math/big, unbounded). Sign and `_` separators
// are handled like the int/float paths. Errors carry no source position
// (parseWord has none — mirrors the other parseWord numeric errors).
func parseBigNumber(src string) (core.Value, error) {
	if strings.IndexByte(src, '_') >= 0 && !validUnderscores(src) {
		return core.Value{}, bigLiteralError(src)
	}
	body := src
	sign := ""
	if len(body) > 0 && (body[0] == '-' || body[0] == '+') {
		sign = string(body[0])
		body = body[1:]
	}
	digits := stripUnderscores(body[2:]) // body[0:2] is the 0d / 0D prefix
	if digits == "" {
		return core.Value{}, bigLiteralError(src)
	}
	text := sign + digits
	if strings.ContainsAny(digits, ".eE") {
		d, _, err := apd.NewFromString(text)
		if err != nil {
			return core.Value{}, bigLiteralError(src)
		}
		return core.NewBigDecimal(d), nil
	}
	n, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return core.Value{}, bigLiteralError(src)
	}
	return core.NewBigInteger(n), nil
}

func bigLiteralError(src string) error {
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: "invalid 0d numeric literal: " + src,
		Src:    src,
		Hint:   "a 0d literal is `0d` then digits, with an optional `.fraction`, exponent, and `_` separators",
	}
}

// isLiteralDigit reports whether c is an actual digit in base. Separators
// beside a prefix/exponent marker or a digit the literal's base cannot spell
// are not digit separators (`0x_1`, `1e_4`, `0b1_2`).
func isLiteralDigit(c byte, base int) bool {
	if c >= '0' && c <= '9' {
		return int(c-'0') < base
	}
	return base == 16 && ((c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'))
}

// validUnderscores reports whether every `_` in src is a single separator
// flanked by actual digits for the literal's base. Decimal fractions and
// exponents use decimal digits; 0x/0o/0b bodies use their named base.
// `1_000` and `0xFF_FF` pass; `1e_4`, `0x_1`, `1__0`, and `1_` fail.
func validUnderscores(src string) bool {
	s := src
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	base := 10
	if len(s) >= 2 && s[0] == '0' {
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
	}
	for i := 0; i < len(src); i++ {
		if src[i] != '_' {
			continue
		}
		if i == 0 || i == len(src)-1 || !isLiteralDigit(src[i-1], base) || !isLiteralDigit(src[i+1], base) {
			return false
		}
	}
	return true
}

// isLeadingDotFloat reports whether src is a `.`-leading number (after an
// optional sign): `.5`, `-.5`, `+.5`. These need a digit before the dot.
func isLeadingDotFloat(src string) bool {
	s := src
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	return len(s) > 0 && s[0] == '.'
}

// isPlainDecimalInteger reports whether src is a base-10 integer literal:
// an optional leading sign ('-' or '+') followed by one or more decimal
// digits, with '_' permitted as a digit separator. It deliberately
// rejects base prefixes (0x/0o/0b — handled by isBasePrefixedInteger) and
// exponents (1e3). Leading zeros stay decimal (jsonic treats 010 as 10,
// not octal), which base-10 ParseInt also does.
func isPlainDecimalInteger(src string) bool {
	if src == "" {
		return false
	}
	i := 0
	if src[0] == '-' || src[0] == '+' {
		i = 1
	}
	if i == len(src) {
		return false
	}
	sawDigit := false
	for ; i < len(src); i++ {
		c := src[i]
		switch {
		case c >= '0' && c <= '9':
			sawDigit = true
		case c == '_':
			// separator; allowed between digits
		default:
			return false
		}
	}
	return sawDigit
}

// hasUppercaseNumericPrefix reports whether src carries an UPPERCASE base or
// big-number prefix — `0X`, `0O`, `0B`, `0D` — after an optional sign.
//
// Boru's numeric syntax prefixes are lowercase only. Both lexers still CLAIM
// an uppercase run as a numeric token (declining would split the ports: Go's
// stock scanner reads `0XFF` as 255 while the TS one reads it as text), so the
// compile failure belongs here in the converter, where one diagnostic serves both
// ports and every context.
func hasUppercaseNumericPrefix(src string) bool {
	s := src
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	if len(s) < 3 || s[0] != '0' {
		return false
	}
	switch s[1] {
	case 'X', 'O', 'B', 'D':
		return true
	}
	return false
}

// uppercaseNumericPrefixError declines an uppercase-prefixed numeric literal.
// Loud in EVERY context, data decode included: `0XFF` is a typo for `0xFF`
// far more often than it is text, and a silent string would be the kind of
// wrong answer this parser exists to decline.
func uppercaseNumericPrefixError(src string, row, col int) *core.BoruError {
	// Callers reach here only via hasUppercaseNumericPrefix, so the prefix
	// letter is always present — index it directly rather than carrying an
	// arm no input can take.
	i := strings.IndexAny(src, "XOBD")
	lower := src[:i] + strings.ToLower(src[i:i+1]) + src[i+1:]
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: "numeric prefix must be lowercase: " + src,
		Row:    row,
		Col:    col,
		Src:    src,
		Hint:   "write `" + lower + "` — boru's numeric prefixes are lowercase (`0x`, `0o`, `0b`, `0d`)",
	}
}

// isBasePrefixedInteger reports whether src is a hex / octal / binary
// integer literal — an optional sign then a 0x / 0o / 0b prefix. These are
// parsed exactly (strconv.ParseInt base 0) rather than via float64, so
// values above 2^53 keep full precision instead of silently rounding.
func isBasePrefixedInteger(src string) bool {
	s := src
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	if len(s) < 3 || s[0] != '0' {
		return false
	}
	switch s[1] {
	case 'x', 'X', 'o', 'O', 'b', 'B':
		return true
	}
	return false
}

// stripUnderscores removes '_' digit separators so strconv.ParseInt
// (base 10) accepts a source jsonic lexed with separators (e.g. 1_000).
func stripUnderscores(src string) string {
	if !strings.Contains(src, "_") {
		return src
	}
	return strings.ReplaceAll(src, "_", "")
}

// integerLiteralOverflowError reports an integer literal (decimal or
// base-prefixed) that does not fit in int64, located at the literal's
// source position when known (row/col 0 = unknown). Until Integer becomes
// arbitrary-precision (Phase 1 of design/INTEGER-OVERFLOW-STRATEGY.5.md),
// this is a hard parse error rather than a silent fall-back to Float.
func integerLiteralOverflowError(src string, row, col int) error {
	return &core.BoruError{
		Code:   "integer_overflow",
		Detail: "integer literal out of range: " + src + " exceeds the Integer range (-9223372036854775808..9223372036854775807)",
		Row:    row,
		Col:    col,
		Src:    src,
		Hint:   "this value exceeds the 64-bit signed Integer range; use a Float (e.g. add a decimal point) for an approximate magnitude",
	}
}

// underscoreError reports a misused `_` digit-separator (leading, trailing,
// or repeated) in a numeric literal.
func underscoreError(nv numberVal) error {
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: "misplaced `_` in numeric literal: " + nv.Src,
		Row:    nv.Row,
		Col:    nv.Col,
		Src:    nv.Src,
		Hint:   "`_` is a single digit-separator — use one between digits, e.g. `1_000` (not `1__0` or `1_`)",
	}
}

// leadingDotNumberError reports a `.`-leading numeric literal (`-.5`,
// `+.5`); a number needs a digit before the decimal point.
func leadingDotNumberError(nv numberVal) error {
	return &core.BoruError{
		Code:   "syntax_error",
		Detail: "numeric literal has no digit before `.`: " + nv.Src,
		Row:    nv.Row,
		Col:    nv.Col,
		Src:    nv.Src,
		Hint:   "write a leading zero, e.g. `-0.5` instead of `-.5`",
	}
}
