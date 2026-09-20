package basic

// Micron literal grammars — each builtin Micron leaf owns a tabnas
// grammar (github.com/tabnas/parser/go) that recognizes its literal
// string form and, via the alternate's action, constructs the
// appropriate boru value. The three grammars MERGE (the tabnas
// (*Tabnas).Merge operation — commutative, alternates interleaved
// deterministically) into the ONE grammar MicronFromString parses
// with, so the micron minilang (`+m:…`, boru:minilang) dispatches on
// literal SHAPE and returns the appropriate type.
//
// Division of labour per leaf:
//
//   - the grammar's match token (an anchored regexp) RECOGNIZES the
//     literal shape — which token lexes decides which type wins;
//   - the alternate's action calls the leaf's string CONSTRUCTOR
//     (emailonFromString / urlonFromString / makePathon), which stays
//     the one semantic validator and field extractor — the same code
//     `make Emailon '…'` runs.
//
// Precedence is Emailon, then Urlon, then Pathon: micronTokenOrder
// pins the lexer's match-token order (the same MatchOptions.TokenOrder
// value on every grammar, so the commutative merge carries it without
// conflict), and Pathon's catch-all pattern accepts any whitespace-free
// source, so a micron literal never fails to parse. A shape the
// gate accepts but the constructor rejects (e.g. an RFC edge net/mail
// declines) falls back to Pathon — exactly the old try-each-constructor
// cascade's behaviour.

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	tabnas "github.com/tabnas/parser/go"
)

// micronLiteralGrammar builds one leaf's literal grammar: a match
// token gating the shape, and a val alternate whose action runs the
// leaf's constructor. build returns the constructed Value or an error;
// on error the action falls back to a Pathon (the family catch-all),
// preserving first-match-wins across the merge.
func micronLiteralGrammar(tag, token string, pattern *regexp.Regexp, order []string, build func(s string) (Value, error)) *tabnas.Tabnas {
	j := tabnas.Make(tabnas.Options{
		Tag: tag,
		Match: &tabnas.MatchOptions{
			Token:      map[string]*regexp.Regexp{token: pattern},
			TokenOrder: order,
		},
	})
	tin := j.Token(token)
	j.Rule("val", func(rs *tabnas.RuleSpec, _ *tabnas.Parser) {
		rs.AddOpen(&tabnas.AltSpec{
			S: [][]tabnas.Tin{{tin}},
			A: func(r *tabnas.Rule, ctx *tabnas.Context) {
				s := fmt.Sprintf("%v", r.O0.ResolveVal(r, ctx))
				v, err := build(s)
				if err != nil {
					// The gate matched but the constructor declined —
					// fall to the family catch-all, like the cascade.
					if out, perr := MakePathon(NewString(s), false); perr == nil {
						v = out[0]
					}
				}
				r.Node = v
			},
		})
	})
	return j
}

// micronEmailonGrammar is Emailon's literal grammar: a plain
// user@host span (no whitespace, no display-name angle brackets, one
// unquoted @) — the same shapes emailonFromString accepts.
func micronEmailonGrammar(order []string) *tabnas.Tabnas {
	return micronLiteralGrammar("Emailon", "#EMAILON",
		regexp.MustCompile(`\A[^@\s<>]+@[^@\s<>]+\z`), order,
		func(s string) (Value, error) {
			out, err := makeEmailon(NewString(s))
			if err != nil {
				return Value{}, err
			}
			return out[0], nil
		})
}

// micronUrlonGrammar is Urlon's literal grammar: an absolute URL
// (scheme://…), matching urlonFromString's absolute-only rule.
func micronUrlonGrammar(order []string) *tabnas.Tabnas {
	return micronLiteralGrammar("Urlon", "#URLON",
		regexp.MustCompile(`\A[A-Za-z][A-Za-z0-9+.-]*://\S+\z`), order,
		func(s string) (Value, error) {
			out, err := makeUrlon(NewString(s))
			if err != nil {
				return Value{}, err
			}
			return out[0], nil
		})
}

// micronPathonGrammar is Pathon's literal grammar: any whitespace-free
// span — the family catch-all, so it merges LAST in micronTokenOrder.
func micronPathonGrammar(order []string) *tabnas.Tabnas {
	return micronLiteralGrammar("Pathon", "#PATHON",
		regexp.MustCompile(`\A\S+\z`), order,
		func(s string) (Value, error) {
			out, err := MakePathon(NewString(s), false)
			if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
				return Value{}, err
			}
			return out[0], nil
		})
}

// micronBuildMergedGrammar builds THE merged Micron literal grammar:
// Emailon ~ Urlon ~ Pathon. The leaf set is FIXED — the `+m` literal is
// NOT extensible (the former MicronLiteralSpec/MicronGrammarWith extras
// hook was removed with the frozen kind namespaces): a custom Micron
// source is parsed by an ordinary parser fn value and constructed with
// `make`, never by joining this merge.
func micronBuildMergedGrammar() (*tabnas.Tabnas, error) {
	order := []string{"#EMAILON", "#URLON", "#PATHON"}
	m, err := micronEmailonGrammar(order).Merge(micronUrlonGrammar(order))
	if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
		return nil, err
	}
	return m.Merge(micronPathonGrammar(order))
}

// micronMergedGrammar memoizes the ONE builtin merged literal grammar:
// Emailon ~ Urlon ~ Pathon (no extras). Built on first use; the build
// cannot fail (distinct tags, identical options), but any error is
// kept and surfaced per parse rather than panicking (ADR-005).
var micronMergedGrammar = sync.OnceValues(micronBuildMergedGrammar)

// micronParseMu serializes merged-grammar parses — the tabnas instance
// is shared process-wide and a parse is not documented reentrant.
// Micron literal parsing is short and rare, so a mutex is cheap.
var micronParseMu sync.Mutex

// MicronFromString parses a Micron literal with the merged grammar:
// each builtin leaf contributes its own tabnas literal grammar
// (micronEmailonGrammar / micronUrlonGrammar / micronPathonGrammar)
// and the merge dispatches on shape — Emailon, then Urlon, then the
// Pathon catch-all — returning the appropriate type. The `+m:…`
// minilang literal (boru:minilang, kind micron / short form m) routes
// here.
func MicronFromString(s string) (Value, error) {
	if s == "" {
		// An empty span never reaches the lexer's match tokens —
		// preserve the family rule directly (the empty relative path).
		out, err := MakePathon(NewString(s), false)
		if err != nil { //covergate:allow shared-assertion / gate-guaranteed kernel guard (§kernel)
			return Value{}, err
		}
		return out[0], nil
	}
	m, err := micronMergedGrammar()
	if err != nil {
		return Value{}, &BoruError{Code: "type_error",
			Detail: fmt.Sprintf("micron: literal grammar unavailable: %v", err)}
	}
	micronParseMu.Lock()
	node, perr := m.Parse(s)
	micronParseMu.Unlock()
	if perr != nil {
		msg := perr.Error()
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		return Value{}, &BoruError{Code: "type_error",
			Detail: fmt.Sprintf("micron: cannot parse literal %q: %s", s, msg)}
	}
	v, ok := node.(Value)
	if !ok {
		return Value{}, &BoruError{Code: "type_error",
			Detail: fmt.Sprintf("micron: literal %q did not produce a value", s)}
	}
	return v, nil
}
