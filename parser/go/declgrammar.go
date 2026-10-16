package parser

// The declarative grammar loader. grammar.json is the single source of
// the boru grammar's STRUCTURE — the token table and the rule-spec
// amendments — shared verbatim with the TS twin (eng/ts reads the same
// file; this side embeds it). Behavior that data cannot express stays
// in the host language as NAMED hooks: `a` (action) and `c` (condition)
// names bind against the tables the caller passes to applyDeclEdits,
// and an unknown name is a panic at parser construction — the grammar
// file and the hook tables must agree exactly, on both engines.
//
// Migration is rule-by-rule: edits move from the setup* functions in
// grammar.go into grammar.json batch by batch, with the full parser
// suites (streamdump, corpora, the cross-engine differential) green
// after every batch. See design/legacy/ENG-COVERAGE-PARITY.0.ignore.

import (
	_ "embed"
	"encoding/json"
	"fmt"

	jsonic "github.com/tabnas/jsonic/go"
)

//go:embed grammar.json
var declGrammarJSON []byte

type declGrammar struct {
	Schema int         `json:"schema"`
	Tokens []declToken `json:"tokens"`
	Edits  []declEdit  `json:"edits"`
}

type declToken struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

type declEdit struct {
	Rule string    `json:"rule"`
	Op   string    `json:"op"`
	Alts []declAlt `json:"alts"`
}

type declAlt struct {
	S    [][]string `json:"s"`
	P    string     `json:"p"`
	R    string     `json:"r"`
	B    int        `json:"b"`
	Text *string    `json:"text"`
	A    string     `json:"a"`
	C    string     `json:"c"`
}

// declHooks are the host-language bindings for the named behavior the
// grammar file references.
type declHooks struct {
	Actions map[string]func(*jsonic.Rule, *jsonic.Context)
	Conds   map[string]func(*jsonic.Rule, *jsonic.Context) bool
}

// loadDeclGrammar decodes the embedded artifact once per parse setup.
func loadDeclGrammar() declGrammar {
	var g declGrammar
	if err := json.Unmarshal(declGrammarJSON, &g); err != nil { //covergate:allow the embedded artifact is compile-time-constant JSON that TestDeclGrammarLoads proves valid
		panic(fmt.Sprintf("parser: grammar.json: %v", err))
	}
	if g.Schema != 1 { //covergate:allow the embedded artifact pins schema 1 and TestDeclGrammarLoads asserts it
		panic(fmt.Sprintf("parser: grammar.json: unsupported schema %d", g.Schema))
	}
	return g
}

// registerDeclTokens registers the artifact's token table in order —
// fixed tokens with their source text, matcher-produced tokens by name
// — and returns the name → Tin map the edits resolve against.
func registerDeclTokens(j *jsonic.Jsonic, g declGrammar) map[string]jsonic.Tin {
	tins := make(map[string]jsonic.Tin, len(g.Tokens))
	for _, tok := range g.Tokens {
		if tok.Text != "" {
			tins[tok.Name] = j.Token("#"+tok.Name, tok.Text)
		} else {
			tins[tok.Name] = j.Token("#" + tok.Name)
		}
	}
	return tins
}

// applyDeclEdits applies every rule amendment in the artifact, binding
// named hooks from the tables. Unknown rule ops, token names, or hook
// names panic: the artifact and the host tables are one contract.
func applyDeclEdits(j *jsonic.Jsonic, g declGrammar, tins map[string]jsonic.Tin, hooks declHooks) {
	for _, edit := range g.Edits {
		alts := make([]*jsonic.AltSpec, 0, len(edit.Alts))
		for _, da := range edit.Alts {
			alts = append(alts, buildDeclAlt(edit, da, tins, hooks))
		}
		op := edit.Op
		j.Rule(edit.Rule, func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
			switch op {
			case "prependOpen":
				prependOpen(rs, alts)
			case "appendOpen":
				rs.AddOpen(alts...)
			case "setOpen":
				setOpen(rs, alts)
			case "prependClose":
				prependClose(rs, alts)
			case "appendClose":
				rs.AddClose(alts...)
			case "setClose":
				setClose(rs, alts)
			default:
				panic(fmt.Sprintf("parser: grammar.json: unknown edit op %q on rule %q", op, edit.Rule))
			}
		})
	}
}

func buildDeclAlt(edit declEdit, da declAlt, tins map[string]jsonic.Tin, hooks declHooks) *jsonic.AltSpec {
	alt := &jsonic.AltSpec{P: da.P, R: da.R, B: da.B}
	if len(da.S) > 0 {
		s := make([][]jsonic.Tin, len(da.S))
		for i, group := range da.S {
			row := make([]jsonic.Tin, len(group))
			for k, name := range group {
				tin, ok := tins[name]
				if !ok {
					panic(fmt.Sprintf("parser: grammar.json: unknown token %q in rule %q", name, edit.Rule))
				}
				row[k] = tin
			}
			s[i] = row
		}
		alt.S = s
	}
	switch {
	case da.Text != nil:
		marker := *da.Text
		alt.A = func(r *jsonic.Rule, _ *jsonic.Context) {
			r.Node = jsonic.Text{Str: marker, Quote: ""}
		}
	case da.A != "":
		fn, ok := hooks.Actions[da.A]
		if !ok {
			panic(fmt.Sprintf("parser: grammar.json: unknown action hook %q in rule %q", da.A, edit.Rule))
		}
		alt.A = fn
	}
	if da.C != "" {
		fn, ok := hooks.Conds[da.C]
		if !ok {
			panic(fmt.Sprintf("parser: grammar.json: unknown condition hook %q in rule %q", da.C, edit.Rule))
		}
		alt.C = fn
	}
	return alt
}
