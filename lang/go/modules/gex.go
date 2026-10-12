package modules

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/boru-lang/boru/lang/go/native"
)

// gex — glob-expression matching, vendored from github.com/rjrodger/gex
// (MIT) and exposed as the `gex` mini-language kind. A glob is a simpler
// alternative to a regular expression:
//
//	*   any run of characters, including newlines    → [\s\S]*
//	?   exactly one character, including a newline    → [\s\S]
//	**  a literal '*'
//	*?  a literal '?'
//
// A pattern compiles to an ANCHORED regexp (^…$), so it matches the WHOLE
// subject, never a substring. Unlike `re` (a match-extractor that returns
// {ok ms fst lst n}), `gex` is a SELECTOR — gex's `.on`: it returns the
// subject when it matches and filters the subject when it is a collection.
//
//	import "boru:minilang"
//	"ax" mini gex 'a*'                 # → 'ax'   (subject matches → returned)
//	"ax" +gex/a*/                      # → 'ax'   (the terse +literal form)
//	"bx" +gex/a*/                      # → None   (no match)
//	["ab" "zz" "ac"] +gex/a*/          # → ['ab' 'ac']        (filter a list)
//	{ab:1 zz:2 ac:3} +gex/a*/ {}       # → {ab:1 ac:3}        (filter a map by key)
//
// None and empty collections are falsy (CoerceBoolean), so the scalar
// value-or-None result drops straight into `if` / filters.
//
// A MAP subject needs an explicit trailing `{}` opts (as above): `mini`'s
// optional opts argument is itself a Map, so a BARE map on the stack is
// claimed as opts and never reaches the subject slot. Scalars and lists
// have no such clash and need no opts.

// gexCache memoizes compiled glob patterns by source — a +gex/…/ in a loop
// compiles once per process, mirroring miniCompiledPattern for `re`.
var (
	gexCacheMu sync.Mutex
	gexCache   = map[string]*regexp.Regexp{}
)

// gexCompile turns one glob spec into an anchored regexp, memoized by source.
func gexCompile(spec string) (*regexp.Regexp, error) {
	gexCacheMu.Lock()
	defer gexCacheMu.Unlock()
	if re, ok := gexCache[spec]; ok {
		return re, nil
	}
	re, err := regexpCompile(gexSpecToRegexp(spec))
	if err != nil {
		return nil, err
	}
	gexCache[spec] = re
	return re, nil
}

// gexSpecToRegexp performs the glob→regexp translation in one left-to-right
// pass over runes, so multi-byte characters survive QuoteMeta intact. The
// digraphs ** and *? are tested before the bare * / ? so a literal star or
// question mark is recognised first.
func gexSpecToRegexp(spec string) string {
	var b strings.Builder
	b.WriteByte('^')
	r := []rune(spec)
	for i := 0; i < len(r); {
		switch {
		case r[i] == '*' && i+1 < len(r) && r[i+1] == '*': // ** → literal *
			b.WriteString(regexp.QuoteMeta("*"))
			i += 2
		case r[i] == '*' && i+1 < len(r) && r[i+1] == '?': // *? → literal ?
			b.WriteString(regexp.QuoteMeta("?"))
			i += 2
		case r[i] == '*':
			b.WriteString(`[\s\S]*`)
			i++
		case r[i] == '?':
			b.WriteString(`[\s\S]`)
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(r[i])))
			i++
		}
	}
	b.WriteByte('$')
	return b.String()
}

// miniGexHandler implements the `gex` kind — args[0]=src (glob), args[1]=opts
// (reserved), args[2]=subject (Any). Polymorphic select (gex's .on): a List
// is filtered to its matching elements, a Map to the entries whose KEY
// matches, and a scalar returns itself when it matches or None otherwise.
func miniGexHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	src, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.BoruError("mini_error", fmt.Sprintf("gex: src: %v", err), "lang_gex")
	}
	re, cerr := gexCompile(src)
	if cerr != nil {
		return nil, r.BoruErrorHint("mini_parse_error",
			fmt.Sprintf("gex: %v", cerr), "lang_gex",
			"use ** for a literal * and *? for a literal ?")
	}
	subject := args[2]

	switch {
	case native.IsConcrete(&subject) && subject.Parent.ConformsTo(native.TList):
		elems, _ := native.AsList(subject)
		var out []native.Value
		for _, e := range elems.Elems() {
			if re.MatchString(native.ValToString(e)) {
				out = append(out, e)
			}
		}
		return []native.Value{native.NewList(out)}, nil

	case native.IsConcrete(&subject) && subject.Parent.ConformsTo(native.TMap):
		m, _ := native.AsMap(subject)
		out := native.NewOrderedMap()
		for _, k := range m.Keys() {
			if re.MatchString(k) {
				v, _ := m.Get(k)
				out.Set(k, v)
			}
		}
		return []native.Value{native.NewMap(out)}, nil

	default: // scalar → the subject if it matches, else None ("no value")
		if re.MatchString(native.ValToString(subject)) {
			return []native.Value{subject}, nil
		}
		return []native.Value{native.NewTypeLiteral(native.TNone)}, nil
	}
}
