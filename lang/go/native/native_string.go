package native

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Case-insensitive matching that preserves original byte offsets.
//
// strings.ToLower can change a string's byte length (e.g. Turkish İ
// U+0130 lowercases to the two-rune "i̇"), so an index found in a
// lowercased copy does NOT line up with the original string. Slicing the
// original with such an index cuts through a multi-byte rune and yields
// invalid UTF-8. These helpers match rune-by-rune under simple (1:1)
// case folding and return offsets into the ORIGINAL string, so every
// slice lands on a rune boundary.

// foldEqualRune reports whether a and b are equal under simple Unicode
// case folding (rune-for-rune, never changing byte length).
func foldEqualRune(a, b rune) bool {
	return a == b || unicode.ToLower(a) == unicode.ToLower(b)
}

// The string words expose ONE character unit — runes (NUR007). The
// search machinery below works in byte offsets internally (that is
// what strings.Index and the fold helpers return); these two helpers
// convert at the word boundary: user-supplied indices ({from}) arrive
// as rune indices and convert IN, result indices convert OUT.

// byteToRuneIdx converts a byte offset in s to its character (rune)
// index; the -1 not-found sentinel passes through unchanged.
func byteToRuneIdx(s string, byteIdx int) int {
	if byteIdx < 0 {
		return byteIdx
	}
	return utf8.RuneCountInString(s[:byteIdx])
}

// runeToByteIdx returns the byte offset of the runeIdx-th rune in s,
// or len(s) when runeIdx is at (or beyond) the rune count. Callers
// bound runeIdx to [0, rune count] first.
func runeToByteIdx(s string, runeIdx int) int {
	seen := 0
	for i := range s {
		if seen == runeIdx {
			return i
		}
		seen++
	}
	return len(s)
}

// foldMatchAt reports whether s begins with a case-insensitive match of
// substr, returning the number of bytes of s the match spans (which can
// differ from len(substr) when casing changes byte length).
func foldMatchAt(s, substr string) (int, bool) {
	si := 0
	for _, pr := range substr {
		if si >= len(s) {
			return 0, false
		}
		sr, size := utf8.DecodeRuneInString(s[si:])
		if !foldEqualRune(sr, pr) {
			return 0, false
		}
		si += size
	}
	return si, true
}

// foldIndex returns the byte offset in s (at or after from) of the first
// case-insensitive match of substr, plus the number of bytes of s the
// match spans. Returns (-1, 0) when absent. Both values are offsets into
// s, so slicing s with them never produces invalid UTF-8.
func foldIndex(s, substr string, from int) (start, span int) {
	if from < 0 {
		from = 0
	}
	if substr == "" {
		return from, 0
	}
	for i := from; i <= len(s); {
		if sp, ok := foldMatchAt(s[i:], substr); ok {
			return i, sp
		}
		if i >= len(s) {
			break
		}
		// s[i:] is non-empty here (i < len(s)), so DecodeRuneInString always
		// returns size >= 1 — a "size == 0" guard would be dead
		// (design/TEST-SEAMS.10.md, ADR-005).
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return -1, 0
}

// foldLastIndex is the case-insensitive analogue of strings.LastIndex,
// returning the original-string byte offset of the last match, or -1.
func foldLastIndex(s, substr string) int {
	last := -1
	from := 0
	for {
		idx, span := foldIndex(s, substr, from)
		if idx < 0 {
			return last
		}
		last = idx
		if span == 0 {
			_, size := utf8.DecodeRuneInString(s[idx:])
			if size == 0 {
				return last
			}
			from = idx + size
		} else {
			from = idx + span
		}
	}
}

// StringModuleNatives covers the string-manipulation words, moved OUT of core
// into the boru:string-util module (StringUtil namespace). Formerly stringNatives. Each entry uses
// the standard NativeFunc shape: forward-collecting words whose
// signatures fan out across [TString], [TString, TMap], etc.
//
// Most handlers are package-level named functions defined below; the
// trivial upper/lower handlers are produced by the unaryStringNative
// builder which returns a NativeFunc carrying both [TString] and
// [TAtom] signatures.
var StringModuleNatives = []NativeFunc{
	unaryStringNative("upper", strings.ToUpper),
	unaryStringNative("lower", strings.ToLower),
	{
		Name: "concat",

		Signatures: []Signature{
			{Args: []*Type{TMap, TList}, Impl: Go(concatOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TList}, Impl: Go(concatHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		Name: "split",

		Signatures: []Signature{
			{Args: []*Type{TString, TString, TMap}, Impl: Go(splitOptsHandler), Returns: []*Type{TList}, BarrierPos: -1},
			{Args: []*Type{TString, TString}, Impl: Go(splitHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "trim",

		Signatures: []Signature{
			{Args: []*Type{TString, TMap}, Impl: Go(trimOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TString}, Impl: Go(trimHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TAtom, TMap}, Impl: Go(trimOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TAtom}, Impl: Go(trimHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		Name: "contains",

		Signatures: []Signature{
			{Args: []*Type{TString, TString, TMap}, Impl: Go(containsOptsHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
			{Args: []*Type{TString, TString}, Impl: Go(containsHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
	{
		Name: "indexof",

		// String-only: substring position. Argument order is
		// HAYSTACK-LAST (data-last grain): forward form
		// `indexof needle haystack` returns the index of needle in
		// haystack, -1 if absent; the pipeline form is
		// `haystack needle indexof`. The opts form takes a trailing
		// options map: `indexof needle haystack {…}`. The list-membership
		// form (for each needle, its index in the haystack) is a separate
		// word, ArrayUtil.indices, in the boru:array-util module — see
		// native_array.go and ADR-001.
		Signatures: []Signature{
			{Args: []*Type{TString, TString, TMap}, Impl: Go(indexOfOptsHandler), Returns: []*Type{TInteger}, BarrierPos: -1},
			{Args: []*Type{TString, TString}, Impl: Go(indexOfHandler), Returns: []*Type{TInteger}, BarrierPos: -1},
		},
	},
	{
		Name: "replace",

		Signatures: []Signature{
			// The dry pass surfaces validateStrOpts' rejections (an unknown
			// option key, an out-of-domain enum value) at check time when the
			// option map is a top-level literal — the handler is a pure
			// string transform.
			{Args: []*Type{TString, TString, TString, TMap}, Impl: Go(replaceOptsHandler), Returns: []*Type{TString}, ReturnsFn: DryPassReturns(replaceOptsHandler, TString), BarrierPos: -1},
			{Args: []*Type{TString, TString, TString}, Impl: Go(replaceHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		Name: "changecase",

		Signatures: []Signature{
			// Same dry pass as replace: the option validation is decidable
			// over a literal option map.
			{Args: []*Type{TString, TMap}, Impl: Go(changeCaseOptsHandler), Returns: []*Type{TString}, ReturnsFn: DryPassReturns(changeCaseOptsHandler, TString), BarrierPos: -1},
			{Args: []*Type{TString}, Impl: Go(changeCaseHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TAtom, TMap}, Impl: Go(changeCaseOptsHandler), Returns: []*Type{TString}, ReturnsFn: DryPassReturns(changeCaseOptsHandler, TString), BarrierPos: -1},
			{Args: []*Type{TAtom}, Impl: Go(changeCaseHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		Name: "normalize",

		Signatures: []Signature{
			{Args: []*Type{TString, TMap}, Impl: Go(normalizeOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TString}, Impl: Go(normalizeHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		Name: "repeat",

		// Subject-last: `repeat count input` (count then the string).
		Signatures: []Signature{
			{Args: []*Type{TInteger, TString, TMap}, Impl: Go(repeatOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TInteger, TString}, Impl: Go(repeatHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		Name: "pad",

		Signatures: []Signature{
			{Args: []*Type{TInteger, TMap, TString}, Impl: Go(padOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TInteger, TString}, Impl: Go(padHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TInteger, TAny}, Impl: Go(padWidthHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TAny}, Impl: Go(padDefaultHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		Name: "match",

		Signatures: []Signature{
			{Args: []*Type{TString, TString, TMap}, Impl: Go(matchOptsHandler), Returns: []*Type{TMap}, BarrierPos: -1},
			{Args: []*Type{TString, TString}, Impl: Go(matchHandler), Returns: []*Type{TMap}, BarrierPos: -1},
		},
	},
	{
		Name: "escape",

		Signatures: []Signature{
			{Args: []*Type{TString, TMap}, Impl: Go(escapeOptsHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TString}, Impl: Go(escapeHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
}

// unaryStringNative builds a NativeFunc with two signatures —
// [TString] and [TAtom] — both routed through a handler that applies
// fn to the input string. Used for upper, lower, and similar
// transforms that share the same shape.
func unaryStringNative(name string, fn func(string) string) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		// Two sigs route here — [TString] and [TAtom]. Read via the
		// type-specific accessor so the post-Step-5 payload variants
		// (StrPayload / AtomPayload) are surfaced correctly.
		var s string
		if IsAtom(args[0]) {
			s, _ = AsAtom(args[0])
		} else if as, err := AsString(args[0]); err == nil {
			s = as
		} else {
			return nil, fmt.Errorf("%s: expected string, got %s", name, args[0].String())
		}
		return []Value{NewString(fn(s))}, nil
	}
	return NativeFunc{
		Name: name,

		Signatures: []Signature{
			{Args: []*Type{TString}, Impl: Go(handler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TAtom}, Impl: Go(handler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	}
}

// ---- concat ----

func concatHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return doConcat(args[0], strOpts{})
}

func concatOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[0], "concat"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[0])
	return doConcat(args[1], opts)
}

func doConcat(listVal Value, o strOpts) ([]Value, error) {
	if !IsConcrete(&listVal) {
		return nil, fmt.Errorf("concat: argument must be a concrete list, got type literal")
	}
	elems, _ := AsList(listVal)
	var parts []string
	for _, e := range elems.Slice() {
		if e.Parent.Equal(TNone) {
			if o.skipNullish {
				continue
			}
			parts = append(parts, "")
			continue
		}
		s := ValToString(e)
		if s == "" && o.skipEmpty {
			continue
		}
		parts = append(parts, s)
	}
	result := strings.Join(parts, o.sep)
	return []Value{NewString(result)}, nil
}

// ---- split ----

func splitHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// Subject (input) is the LAST argument: forward `split sep input`.
	sep, _ := args[0].AsConcreteString()
	input, _ := args[1].AsConcreteString()
	return doSplit(input, sep, strOpts{cs: "sensitive", mode: "literal"})
}

func splitOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[2], "split"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[2])
	sep, _ := args[0].AsConcreteString()
	input, _ := args[1].AsConcreteString()
	return doSplit(input, sep, opts)
}

func doSplit(input, sep string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
	}

	var parts []string

	if o.mode == "shell" {
		parts = splitByShell(input, sep, o.cs == "insensitive")
	} else {
		// literal split
		if o.cs == "insensitive" {
			parts = splitInsensitive(input, sep)
		} else {
			lim := -1
			if o.hasLim {
				lim = int(o.lim)
			}
			if lim > 0 {
				parts = strings.SplitN(input, sep, lim)
			} else {
				parts = strings.Split(input, sep)
			}
		}
	}

	// Apply lim for non-literal modes (literal already handled above)
	if o.hasLim && o.mode != "literal" {
		lim := int(o.lim)
		if lim > 0 && len(parts) > lim {
			// Join the excess parts back with the separator
			last := strings.Join(parts[lim-1:], sep)
			parts = append(parts[:lim-1], last)
		}
	}

	// Apply keepEmpty and trimParts
	var result []Value
	for _, p := range parts {
		if o.trimParts {
			p = strings.TrimSpace(p)
		}
		if !o.keepEmpty && p == "" {
			continue
		}
		result = append(result, NewString(p))
	}

	// If keepEmpty is true, include all parts (even empty ones)
	if o.keepEmpty {
		result = nil
		for _, p := range parts {
			if o.trimParts {
				p = strings.TrimSpace(p)
			}
			result = append(result, NewString(p))
		}
	}

	if result == nil {
		result = []Value{}
	}
	return []Value{NewList(result)}, nil
}

// splitInsensitive splits a string by separator case-insensitively.
func splitInsensitive(input, sep string) []string {
	if sep == "" {
		return strings.Split(input, "")
	}
	var parts []string
	start := 0
	for {
		idx, span := foldIndex(input, sep, start)
		if idx < 0 {
			parts = append(parts, input[start:])
			break
		}
		parts = append(parts, input[start:idx])
		start = idx + span
	}
	return parts
}

// splitByShell splits using shell pattern matching to find separators.
func splitByShell(input, pattern string, caseInsensitive bool) []string {
	var parts []string
	remaining := input
	for {
		idx, length := shellFind(remaining, pattern, caseInsensitive)
		if idx < 0 || length == 0 {
			parts = append(parts, remaining)
			break
		}
		parts = append(parts, remaining[:idx])
		remaining = remaining[idx+length:]
	}
	return parts
}

// ---- trim ----

func trimHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteString()
	return []Value{NewString(strings.TrimSpace(_as0))}, nil
}

func trimOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[1], "trim"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[1])
	_as1, _ := args[0].AsConcreteString()
	return doTrim(_as1, opts)
}

func doTrim(input string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
	}

	chars := o.fill // chars field reused from fill via parseStrOpts
	if chars != "" {
		cutset := chars
		if o.cs == "insensitive" {
			// For case-insensitive char matching, include both cases in cutset
			cutset = strings.ToLower(chars) + strings.ToUpper(chars)
		}
		switch o.side {
		case "left":
			input = strings.TrimLeft(input, cutset)
		case "right":
			input = strings.TrimRight(input, cutset)
		default: // "both"
			input = strings.Trim(input, cutset)
		}
	} else {
		switch o.side {
		case "left":
			input = strings.TrimLeftFunc(input, unicode.IsSpace)
		case "right":
			input = strings.TrimRightFunc(input, unicode.IsSpace)
		default: // "both"
			input = strings.TrimSpace(input)
		}
	}

	return []Value{NewString(input)}, nil
}

// ---- contains ----

func containsHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// Haystack (input) is the LAST argument: forward `contains needle haystack`.
	needle, _ := args[0].AsConcreteString()
	haystack, _ := args[1].AsConcreteString()
	return doContains(haystack, needle, strOpts{cs: "sensitive", mode: "literal"})
}

func containsOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[2], "contains"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[2])
	needle, _ := args[0].AsConcreteString()
	haystack, _ := args[1].AsConcreteString()
	return doContains(haystack, needle, opts)
}

func doContains(input, search string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
		search = applyNorm(search, o.normForm)
	}

	ci := o.cs == "insensitive"

	if o.mode == "shell" {
		// Shell pattern matching
		switch o.anchored {
		case "both", "true":
			return []Value{NewBoolean(shellMatch(search, input, ci))}, nil
		case "start":
			// Check if any prefix matches the pattern
			for i := 1; i <= len(input); i++ {
				if shellMatch(search, input[:i], ci) {
					return []Value{NewBoolean(true)}, nil
				}
			}
			return []Value{NewBoolean(false)}, nil
		case "end":
			// Check if any suffix matches the pattern
			for i := 0; i < len(input); i++ {
				if shellMatch(search, input[i:], ci) {
					return []Value{NewBoolean(true)}, nil
				}
			}
			return []Value{NewBoolean(false)}, nil
		default:
			idx, _ := shellFind(input, search, ci)
			return []Value{NewBoolean(idx >= 0)}, nil
		}
	}

	// Literal matching
	haystack := input
	needle := search
	if ci {
		haystack = strings.ToLower(haystack)
		needle = strings.ToLower(needle)
	}

	switch o.anchored {
	case "start":
		found := strings.HasPrefix(haystack, needle)
		if found && o.wholeWord {
			found = isWordBoundary(input, len(search))
		}
		return []Value{NewBoolean(found)}, nil
	case "end":
		found := strings.HasSuffix(haystack, needle)
		if found && o.wholeWord {
			found = isWordBoundary(input, len(input)-len(search))
		}
		return []Value{NewBoolean(found)}, nil
	case "both", "true":
		return []Value{NewBoolean(haystack == needle)}, nil
	default:
		if o.wholeWord {
			return []Value{NewBoolean(containsWholeWord(input, search, ci))}, nil
		}
		return []Value{NewBoolean(strings.Contains(haystack, needle))}, nil
	}
}

// containsWholeWord checks if search appears as a whole word in input.
func containsWholeWord(input, search string, ci bool) bool {
	haystack := input
	needle := search
	if ci {
		haystack = strings.ToLower(haystack)
		needle = strings.ToLower(needle)
	}
	idx := 0
	for {
		pos := strings.Index(haystack[idx:], needle)
		if pos < 0 {
			return false
		}
		absPos := idx + pos
		endPos := absPos + len(needle)
		if isWordBoundary(input, absPos) && isWordBoundary(input, endPos) {
			return true
		}
		idx = absPos + 1
	}
}

// ---- indexof ----

func indexOfHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// Haystack is the LAST argument (data-last): forward form is
	// `indexof needle haystack`, so args[0]=needle, args[1]=haystack. The
	// equivalent stack/pipeline form is `haystack needle indexof`.
	needle, _ := args[0].AsConcreteString()
	haystack, _ := args[1].AsConcreteString()
	return doIndexOf(haystack, needle, strOpts{cs: "sensitive", mode: "literal", occ: "first"})
}

func indexOfOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[2], "indexof"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[2])
	needle, _ := args[0].AsConcreteString()
	haystack, _ := args[1].AsConcreteString()
	return doIndexOf(haystack, needle, opts)
}

func doIndexOf(input, search string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
		search = applyNorm(search, o.normForm)
	}

	ci := o.cs == "insensitive"
	// {from} is a CHARACTER (rune) index — converted to the byte offset
	// the search machinery works in; results convert back on the way
	// out (NUR007: one character unit across the string words).
	from := 0
	if o.hasFrom {
		fromRunes := int(o.from)
		if fromRunes < 0 {
			fromRunes = 0
		}
		if fromRunes > utf8.RuneCountInString(input) {
			return []Value{NewInteger(-1)}, nil
		}
		from = runeToByteIdx(input, fromRunes)
	}

	if o.mode == "shell" {
		if o.occ == "last" {
			return []Value{NewInteger(int64(byteToRuneIdx(input, shellFindLast(input, search, ci))))}, nil
		}
		idx, _ := shellFind(input[from:], search, ci)
		if idx >= 0 {
			idx += from
		}
		return []Value{NewInteger(int64(byteToRuneIdx(input, idx)))}, nil
	}

	// Literal matching. Case-insensitive search goes through foldIndex so
	// the returned offset is a position in the ORIGINAL string (not a
	// lowercased copy whose byte offsets can drift).
	if ci {
		if o.occ == "last" {
			return []Value{NewInteger(int64(byteToRuneIdx(input, foldLastIndex(input, search))))}, nil
		}
		idx, _ := foldIndex(input, search, from)
		return []Value{NewInteger(int64(byteToRuneIdx(input, idx)))}, nil
	}

	if o.occ == "last" {
		idx := strings.LastIndex(input, search)
		return []Value{NewInteger(int64(byteToRuneIdx(input, idx)))}, nil
	}

	idx := strings.Index(input[from:], search)
	if idx >= 0 {
		idx += from
	}
	return []Value{NewInteger(int64(byteToRuneIdx(input, idx)))}, nil
}

// shellFindLast finds the last occurrence of a shell pattern.
func shellFindLast(s, pattern string, caseInsensitive bool) int {
	matches := shellFindAll(s, pattern, caseInsensitive)
	if len(matches) == 0 {
		return -1
	}
	return matches[len(matches)-1][0]
}

// ---- replace ----

func replaceHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// Subject (input) is the LAST argument: forward `replace search repl input`.
	search, _ := args[0].AsConcreteString()
	repl, _ := args[1].AsConcreteString()
	input, _ := args[2].AsConcreteString()
	return doReplace(input, search, repl,
		strOpts{cs: "sensitive", mode: "literal", scope: "first"})
}

func replaceOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[3], "replace"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[3])
	search, _ := args[0].AsConcreteString()
	repl, _ := args[1].AsConcreteString()
	input, _ := args[2].AsConcreteString()
	return doReplace(input, search, repl, opts)
}

func doReplace(input, search, repl string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
		search = applyNorm(search, o.normForm)
	}

	ci := o.cs == "insensitive"
	// {from} is a rune index (NUR007) — convert to the byte offset the
	// replacement machinery slices with, clamped to the string's end.
	from := 0
	if o.hasFrom {
		fromRunes := int(o.from)
		if fromRunes < 0 {
			fromRunes = 0
		}
		if rc := utf8.RuneCountInString(input); fromRunes > rc {
			fromRunes = rc
		}
		from = runeToByteIdx(input, fromRunes)
	}

	maxCount := -1
	if o.hasCount {
		maxCount = int(o.count)
	}

	if o.mode == "shell" {
		return []Value{NewString(replaceShell(input, search, repl, ci, o.scope, from, maxCount))}, nil
	}

	// Literal replacement
	if o.scope == "all" {
		if ci {
			return []Value{NewString(replaceAllInsensitive(input, search, repl, from, maxCount))}, nil
		}
		if maxCount >= 0 {
			return []Value{NewString(replaceN(input, search, repl, from, maxCount))}, nil
		}
		if from > 0 {
			prefix := input[:from]
			rest := strings.ReplaceAll(input[from:], search, repl)
			return []Value{NewString(prefix + rest)}, nil
		}
		return []Value{NewString(strings.ReplaceAll(input, search, repl))}, nil
	}

	// scope == "first" (default)
	if ci {
		return []Value{NewString(replaceFirstInsensitive(input, search, repl, from))}, nil
	}
	if from > 0 {
		idx := strings.Index(input[from:], search)
		if idx < 0 {
			return []Value{NewString(input)}, nil
		}
		absIdx := from + idx
		return []Value{NewString(input[:absIdx] + repl + input[absIdx+len(search):])}, nil
	}
	return []Value{NewString(strings.Replace(input, search, repl, 1))}, nil
}

func replaceFirstInsensitive(input, search, repl string, from int) string {
	idx, span := foldIndex(input, search, from)
	if idx < 0 {
		return input
	}
	return input[:idx] + repl + input[idx+span:]
}

func replaceAllInsensitive(input, search, repl string, from, maxCount int) string {
	var b strings.Builder
	b.WriteString(input[:from])
	pos := from
	count := 0
	for {
		if maxCount >= 0 && count >= maxCount {
			b.WriteString(input[pos:])
			break
		}
		idx, span := foldIndex(input, search, pos)
		if idx < 0 || span == 0 {
			// span == 0 only for an empty search; stop rather than spin.
			b.WriteString(input[pos:])
			break
		}
		b.WriteString(input[pos:idx])
		b.WriteString(repl)
		pos = idx + span
		count++
	}
	return b.String()
}

func replaceN(input, search, repl string, from, maxCount int) string {
	var b strings.Builder
	b.WriteString(input[:from])
	remaining := input[from:]
	count := 0
	for {
		if maxCount >= 0 && count >= maxCount {
			b.WriteString(remaining)
			break
		}
		idx := strings.Index(remaining, search)
		if idx < 0 {
			b.WriteString(remaining)
			break
		}
		b.WriteString(remaining[:idx])
		b.WriteString(repl)
		remaining = remaining[idx+len(search):]
		count++
	}
	return b.String()
}

func replaceShell(input, pattern, repl string, ci bool, scope string, from, maxCount int) string {
	matches := shellFindAll(input[from:], pattern, ci)
	if len(matches) == 0 {
		return input
	}

	// Adjust indices for the from offset
	for i := range matches {
		matches[i][0] += from
		matches[i][1] += from
	}

	if scope == "first" {
		matches = matches[:1]
	}
	if maxCount >= 0 && len(matches) > maxCount {
		matches = matches[:maxCount]
	}

	var b strings.Builder
	prev := 0
	for _, m := range matches {
		b.WriteString(input[prev:m[0]])
		b.WriteString(repl)
		prev = m[1]
	}
	b.WriteString(input[prev:])
	return b.String()
}

// ---- changecase ----

func changeCaseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteString()
	return doChangeCase(_as0, strOpts{style: "lower"})
}

func changeCaseOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[1], "changecase"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[1])
	if opts.style == "" {
		opts.style = "lower"
	}
	_as1, _ := args[0].AsConcreteString()
	return doChangeCase(_as1, opts)
}

func doChangeCase(input string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
	}

	var result string
	switch o.style {
	case "upper":
		result = strings.ToUpper(input)
	case "capitalize":
		result = capitalize(input)
	case "title":
		result = titleCase(input)
	case "sentence":
		result = sentenceCase(input)
	case "fold":
		result = strings.ToLower(input) // fold approximation using toLower
	default: // "lower"
		result = strings.ToLower(input)
	}

	return []Value{NewString(result)}, nil
}

// capitalize uppercases the first character only.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// titleCase uppercases the first letter of each word.
func titleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(prev) || unicode.IsPunct(prev) {
			prev = r
			return unicode.ToUpper(r)
		}
		prev = r
		return unicode.ToLower(r)
	}, s)
}

// sentenceCase lowercases everything, then uppercases the first letter.
func sentenceCase(s string) string {
	lower := strings.ToLower(s)
	return capitalize(lower)
}

// ---- normalize ----

func normalizeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteString()
	return doNormalize(_as0, strOpts{form: "NFC", eol: "preserve"})
}

func normalizeOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[1], "normalize"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[1])
	_as1, _ := args[0].AsConcreteString()
	return doNormalize(_as1, opts)
}

func doNormalize(input string, o strOpts) ([]Value, error) {
	// Apply Unicode normalization
	result := applyNorm(input, o.form)

	// Normalize line endings
	switch o.eol {
	case "lf":
		result = strings.ReplaceAll(result, "\r\n", "\n")
		result = strings.ReplaceAll(result, "\r", "\n")
	case "crlf":
		result = strings.ReplaceAll(result, "\r\n", "\n")
		result = strings.ReplaceAll(result, "\r", "\n")
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}

	// Collapse whitespace
	if o.collapseWs {
		var b strings.Builder
		prevWs := false
		for _, r := range result {
			if unicode.IsSpace(r) && r != '\n' && r != '\r' {
				if !prevWs {
					b.WriteRune(' ')
					prevWs = true
				}
			} else {
				b.WriteRune(r)
				prevWs = false
			}
		}
		result = b.String()
	}

	// Trim surrounding whitespace
	if o.trim {
		result = strings.TrimSpace(result)
	}

	return []Value{NewString(result)}, nil
}

// ---- repeat ----

func repeatHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// Subject (input) is the LAST argument: forward `repeat count input`.
	count, _ := args[0].AsConcreteInteger()
	input, _ := args[1].AsConcreteString()
	return doRepeat(input, count, strOpts{})
}

func repeatOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[2], "repeat"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[2])
	count, _ := args[0].AsConcreteInteger()
	input, _ := args[1].AsConcreteString()
	return doRepeat(input, count, opts)
}

// maxStringResultBytes caps the size of a string a single builder word
// (repeat / pad) will allocate from a user-supplied count or length.
// Without it, strings.Repeat overflows (panic) or the allocation OOMs
// the process. 256 MiB is far beyond any legitimate use. See ADR-005.
const maxStringResultBytes = 1 << 28

func doRepeat(input string, count int64, o strOpts) ([]Value, error) {
	if count < 0 {
		return nil, fmt.Errorf("repeat: count must be non-negative, got %d", count)
	}
	// Bound the result size (and the per-copy slice in the separator
	// branch) without overflowing the multiplication. perRep is at least
	// 1 so this also caps the number of repetitions.
	perRep := len(input) + len(o.sep)
	if perRep < 1 {
		perRep = 1
	}
	if count > int64(maxStringResultBytes)/int64(perRep) {
		return nil, fmt.Errorf("repeat: result would exceed %d bytes", maxStringResultBytes)
	}

	if !o.hasSep || o.sep == "" {
		return []Value{NewString(strings.Repeat(input, int(count)))}, nil
	}

	// With separator: join count copies with sep
	if count == 0 {
		return []Value{NewString("")}, nil
	}
	parts := make([]string, count)
	for i := range parts {
		parts[i] = input
	}
	return []Value{NewString(strings.Join(parts, o.sep))}, nil
}

// ---- pad ----

// pad: forward-first: args[0]=width (forward), args[1]=string (stack).
// Usage: "ab" pad 5 → "ab   "
func padHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as1, _ := args[1].AsConcreteString()
	_as0, _ := args[0].AsConcreteInteger()
	return doPad(_as1, _as0, strOpts{side: "right", fill: " "})
}

// pad: forward-first: args[0]=width (forward), args[1]=opts (forward), args[2]=string (stack).
// Usage: "ab" pad 5 {side:"left" fill:"0"} → "000ab"
func padOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if !IsConcrete(&args[1]) {
		return nil, r.BoruError("pad_error", "pad: options must be a concrete map, got type literal", "pad")
	}
	if err := validateStrOpts(r, args[1], "pad"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[1])
	if opts.fill == "" {
		opts.fill = " "
	}
	// Default side for pad is "right", not "both" from parseStrOpts.
	if m, _ := AsMap(args[1]); m != nil {
		if _, ok := m.Get("side"); !ok {
			opts.side = "right"
		}
	}
	_as3, _ := args[2].AsConcreteString()
	_as2, _ := args[0].AsConcreteInteger()
	return doPad(_as3, _as2, opts)
}

func doPad(input string, targetLen int64, o strOpts) ([]Value, error) {
	if targetLen < 0 {
		return nil, fmt.Errorf("pad: target length must be non-negative, got %d", targetLen)
	}
	if targetLen > int64(maxStringResultBytes) {
		return nil, fmt.Errorf("pad: target length %d exceeds %d bytes", targetLen, maxStringResultBytes)
	}
	// Width and truncation count CHARACTERS (runes), the string words'
	// one unit (NUR007) — byte-based padding both mis-sized multi-byte
	// input and could split a multi-byte fill rune.
	current := utf8.RuneCountInString(input)
	target := int(targetLen)

	if current >= target {
		if o.trunc {
			return []Value{NewString(string([]rune(input)[:target]))}, nil
		}
		return []Value{NewString(input)}, nil
	}

	needed := target - current
	fill := o.fill
	if fill == "" {
		fill = " "
	}

	// The cap is a BYTE bound (the builder-allocation/OOM safeguard),
	// and a multi-byte fill multiplies the per-character width — so
	// re-check the PROJECTED byte size before allocating anything
	// (flagged in PR #306 review: a 4-byte emoji fill could pass the
	// character-count check yet allocate ~4x the cap).
	fillRunes := utf8.RuneCountInString(fill)
	reps := (needed / fillRunes) + 1
	if projected := len(input) + reps*len(fill); projected > maxStringResultBytes {
		return nil, fmt.Errorf("pad: projected result size %d exceeds %d bytes", projected, maxStringResultBytes)
	}

	// Generate enough fill characters (runes, so a multi-rune or
	// multi-byte fill never gets cut mid-character).
	padding := []rune(strings.Repeat(fill, reps))

	switch o.side {
	case "left":
		result := string(padding[:needed]) + input
		return []Value{NewString(result)}, nil
	case "both":
		left := needed / 2
		right := needed - left
		result := string(padding[:left]) + input + string(padding[:right])
		return []Value{NewString(result)}, nil
	default: // "right"
		result := input + string(padding[:needed])
		return []Value{NewString(result)}, nil
	}
}

// ---- match ----

func matchHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// Subject (input) is the LAST argument: forward `match pattern input`.
	pattern, _ := args[0].AsConcreteString()
	input, _ := args[1].AsConcreteString()
	return doMatch(input, pattern,
		strOpts{cs: "sensitive", mode: "literal", scope: "first"})
}

func matchOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[2], "match"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[2])
	pattern, _ := args[0].AsConcreteString()
	input, _ := args[1].AsConcreteString()
	return doMatch(input, pattern, opts)
}

func doMatch(input, pattern string, o strOpts) ([]Value, error) {
	if o.normForm != "" {
		input = applyNorm(input, o.normForm)
		pattern = applyNorm(pattern, o.normForm)
	}

	ci := o.cs == "insensitive"

	var matches []matchEntry

	if o.mode == "shell" {
		matches = findShellMatches(input, pattern, ci, o.scope == "all")
	} else {
		matches = findLiteralMatches(input, pattern, ci, o.scope == "all")
	}

	return []Value{buildMatchResult(matches)}, nil
}

type matchEntry struct {
	m string
	i int
	e int
}

func findLiteralMatches(input, pattern string, ci bool, all bool) []matchEntry {
	if ci {
		// Case-insensitive: match through foldIndex so offsets and the
		// captured substring come from the ORIGINAL string (a lowercased
		// copy can have different byte offsets and corrupt the slice).
		var matches []matchEntry
		start := 0
		for {
			idx, span := foldIndex(input, pattern, start)
			if idx < 0 || span == 0 {
				break
			}
			matches = append(matches, matchEntry{
				m: input[idx : idx+span],
				i: byteToRuneIdx(input, idx),
				e: byteToRuneIdx(input, idx+span),
			})
			if !all {
				break
			}
			// input[idx:idx+span] was just sliced above without panicking, so
			// idx+span <= len(input): a "start > len(input)" guard would be
			// dead (design/TEST-SEAMS.10.md, ADR-005). foldIndex tolerates
			// start == len(input) on the next pass.
			start = idx + span
		}
		return matches
	}

	// An empty pattern matches at every CHARACTER boundary, end
	// included — one match per boundary. The generic loop's one-BYTE
	// advance produced duplicate character indices inside multi-byte
	// runes once indices became rune-based (flagged in PR #306 review).
	if pattern == "" {
		if !all {
			return []matchEntry{{m: "", i: 0, e: 0}}
		}
		n := utf8.RuneCountInString(input)
		matches := make([]matchEntry, 0, n+1)
		for k := 0; k <= n; k++ {
			matches = append(matches, matchEntry{m: "", i: k, e: k})
		}
		return matches
	}

	var matches []matchEntry
	start := 0
	for {
		idx := strings.Index(input[start:], pattern)
		if idx < 0 {
			break
		}
		absIdx := start + idx
		end := absIdx + len(pattern)
		matches = append(matches, matchEntry{
			m: input[absIdx:end],
			i: byteToRuneIdx(input, absIdx),
			e: byteToRuneIdx(input, end),
		})
		if !all {
			break
		}
		// A valid non-empty UTF-8 pattern can only match at rune
		// boundaries (lead bytes never equal continuation bytes), so
		// the byte-wise advance keeps the overlapping-match contract
		// without ever landing mid-rune.
		start = absIdx + 1
		if start >= len(input) {
			break
		}
	}
	return matches
}

func findShellMatches(input, pattern string, ci bool, all bool) []matchEntry {
	results := shellFindAll(input, pattern, ci)
	var matches []matchEntry
	for _, r := range results {
		matches = append(matches, matchEntry{
			m: input[r[0]:r[1]],
			i: byteToRuneIdx(input, r[0]),
			e: byteToRuneIdx(input, r[1]),
		})
		if !all {
			break
		}
	}
	return matches
}

func buildMatchResult(matches []matchEntry) Value {
	result := NewOrderedMap()

	ok := len(matches) > 0
	result.Set("ok", NewBoolean(ok))

	// Build ms array
	msElems := make([]Value, len(matches))
	for i, m := range matches {
		entry := NewOrderedMap()
		entry.Set("m", NewString(m.m))
		entry.Set("i", NewInteger(int64(m.i)))
		entry.Set("e", NewInteger(int64(m.e)))
		msElems[i] = NewMap(entry)
	}
	result.Set("ms", NewList(msElems))

	// Set fst and lst convenience views
	if len(matches) > 0 {
		fst := NewOrderedMap()
		fst.Set("m", NewString(matches[0].m))
		fst.Set("i", NewInteger(int64(matches[0].i)))
		fst.Set("e", NewInteger(int64(matches[0].e)))
		result.Set("fst", NewMap(fst))

		last := matches[len(matches)-1]
		lst := NewOrderedMap()
		lst.Set("m", NewString(last.m))
		lst.Set("i", NewInteger(int64(last.i)))
		lst.Set("e", NewInteger(int64(last.e)))
		result.Set("lst", NewMap(lst))
	}

	result.Set("n", NewInteger(int64(len(matches))))

	return NewMap(result)
}

// ---- escape ----

func escapeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteString()
	return doEscape(_as0, strOpts{tgt: "sh", quote: "none"})
}

func escapeOptsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	if err := validateStrOpts(r, args[1], "escape"); err != nil {
		return nil, err
	}
	opts := parseStrOpts(args[1])
	_as1, _ := args[0].AsConcreteString()
	return doEscape(_as1, opts)
}

func doEscape(input string, o strOpts) ([]Value, error) {
	var result string

	switch o.tgt {
	case "bash":
		result = escapeBash(input)
	case "sed":
		result = escapeSed(input)
	case "awk":
		result = escapeAwk(input)
	case "grep":
		result = escapeGrep(input)
	default: // "sh"
		result = escapeSh(input)
	}

	// Apply quoting
	switch o.quote {
	case "single":
		result = "'" + result + "'"
	case "double":
		result = "\"" + result + "\""
	}

	return []Value{NewString(result)}, nil
}

// escapeSh escapes for POSIX sh: backslash-escape shell metacharacters.
func escapeSh(s string) string {
	meta := `\'"` + "`$!#&|;(){}[]<>?*~ \t\n"
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeBash escapes for bash: similar to sh but includes additional chars.
func escapeBash(s string) string {
	return escapeSh(s) // same treatment for simplicity
}

// escapeSed escapes for sed regex: escape BRE metacharacters.
func escapeSed(s string) string {
	meta := `\/.^$*[]`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeAwk escapes for awk regex: escape ERE metacharacters.
func escapeAwk(s string) string {
	meta := `\/.^$*+?()[]{}|`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeGrep escapes for grep BRE: escape BRE metacharacters.
func escapeGrep(s string) string {
	meta := `\/.^$*[]`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(meta, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
