package basic

import (
	"testing"
)

// micronTestOrder is the FIXED token order the standalone-grammar tests
// build with — the same order micronBuildMergedGrammar uses (the +m
// literal grammar is not extensible).
var micronTestOrder = []string{"#EMAILON", "#URLON", "#PATHON"}

// TestMicronMergedGrammarDispatch pins the merged literal grammar's
// type dispatch: each builtin leaf contributes its own tabnas grammar,
// the merge combines them, and the shape decides the type — Emailon,
// then Urlon, then the Pathon catch-all.
func TestMicronMergedGrammarDispatch(t *testing.T) {
	cases := []struct {
		src  string
		kind *Type
	}{
		{"alice@example.com", TEmailon},
		{"bob@x.io", TEmailon},
		{"https://x.com/a?q=1", TUrlon},
		{"http://h.example:8080/p", TUrlon},
		{"a/b", TPathon},
		{"/abs/path", TPathon},
		{"9000", TPathon},         // any whitespace-free span is a valid single-segment path
		{"a@b@c", TPathon},        // two @s — not an Emailon shape, falls to the catch-all
		{"Alice<a@b.c>", TPathon}, // display-name form — excluded from the Emailon gate
		{"", TPathon},             // the empty relative path (handled before the lexer)
	}
	for _, c := range cases {
		v, err := MicronFromString(c.src)
		if err != nil {
			t.Errorf("MicronFromString(%q): unexpected error: %v", c.src, err)
			continue
		}
		if !v.Parent().Equal(c.kind) {
			t.Errorf("MicronFromString(%q) = %s, want %s", c.src, v.Parent().Name(), c.kind.Name())
		}
	}
}

// TestMicronMergedGrammarValues pins that the merged grammar's actions
// run the SAME constructors `make` uses — the parsed value is
// content-equal to the make-built one.
func TestMicronMergedGrammarValues(t *testing.T) {
	v, err := MicronFromString("alice@example.com")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	made, err := makeEmailon(NewString("alice@example.com"))
	if err != nil {
		t.Fatalf("make: %v", err)
	}
	if !ValuesEqual(v, made[0]) {
		t.Fatalf("grammar value %s != constructor value %s", v.String(), made[0].String())
	}
	if got := v.String(); got != "alice@example.com" {
		t.Fatalf("render = %q", got)
	}
}

// TestMicronLeafGrammarsIndependent pins that each leaf's grammar
// stands alone — it accepts its own literal shape and REJECTS the
// other kinds' (the negative half of the merge contract: the merged
// dispatch comes from combining single-shape grammars, not from one
// grammar that happened to accept everything).
func TestMicronLeafGrammarsIndependent(t *testing.T) {
	em := micronEmailonGrammar(micronTestOrder)
	if _, err := em.Parse("alice@example.com"); err != nil {
		t.Errorf("emailon grammar rejected its own literal: %v", err)
	}
	if _, err := em.Parse("a/b"); err == nil {
		t.Error("emailon grammar accepted a path literal")
	}
	if _, err := em.Parse("https://x.com/a"); err == nil {
		t.Error("emailon grammar accepted a URL literal")
	}

	ur := micronUrlonGrammar(micronTestOrder)
	if _, err := ur.Parse("https://x.com/a"); err != nil {
		t.Errorf("urlon grammar rejected its own literal: %v", err)
	}
	if _, err := ur.Parse("alice@example.com"); err == nil {
		t.Error("urlon grammar accepted an email literal")
	}

	pt := micronPathonGrammar(micronTestOrder)
	if _, err := pt.Parse("a/b"); err != nil {
		t.Errorf("pathon grammar rejected its own literal: %v", err)
	}
}

// TestMicronMergeCommutative pins the tabnas merge contract for the
// micron grammars: merging in either direction dispatches identically
// (the shared micronTokenOrder carries the lexer precedence through
// the commutative option merge).
func TestMicronMergeCommutative(t *testing.T) {
	ab, err := micronEmailonGrammar(micronTestOrder).Merge(micronUrlonGrammar(micronTestOrder))
	if err != nil {
		t.Fatalf("merge e~u: %v", err)
	}
	abc, err := ab.Merge(micronPathonGrammar(micronTestOrder))
	if err != nil {
		t.Fatalf("merge (e~u)~p: %v", err)
	}
	cb, err := micronPathonGrammar(micronTestOrder).Merge(micronUrlonGrammar(micronTestOrder))
	if err != nil {
		t.Fatalf("merge p~u: %v", err)
	}
	cba, err := cb.Merge(micronEmailonGrammar(micronTestOrder))
	if err != nil {
		t.Fatalf("merge (p~u)~e: %v", err)
	}
	for _, m := range []interface {
		Parse(string) (any, error)
	}{abc, cba} {
		for src, kind := range map[string]*Type{
			"alice@example.com": TEmailon,
			"https://x.com/a":   TUrlon,
			"just/a/path":       TPathon,
		} {
			node, err := m.Parse(src)
			if err != nil {
				t.Errorf("merged parse %q: %v", src, err)
				continue
			}
			v, ok := node.(Value)
			if !ok || !v.Parent().Equal(kind) {
				t.Errorf("merged parse %q → %v, want %s", src, node, kind.Name())
			}
		}
	}
}

// (The extras hook — MicronLiteralSpec / MicronGrammarWith — was removed
// with the frozen kind namespaces: the +m literal grammar is fixed to the
// builtin leaves, so the former testExtraGrammar / extras / token-collision
// pins died with it. micronBuildMergedGrammar is pinned below.)

// TestMicronBuildMergedGrammar pins the fixed builtin merge directly: each
// leaf claims its span and the Pathon catch-all takes the rest.
func TestMicronBuildMergedGrammar(t *testing.T) {
	m, err := micronBuildMergedGrammar()
	if err != nil {
		t.Fatalf("micronBuildMergedGrammar: %v", err)
	}
	for src, kind := range map[string]*Type{
		"alice@example.com": TEmailon,
		"https://x.com/a":   TUrlon,
		"plain/path":        TPathon,
		"T-123":             TPathon, // no user shapes — everything else is a Pathon
	} {
		node, err := m.Parse(src)
		if err != nil {
			t.Errorf("parse %q: %v", src, err)
			continue
		}
		v, ok := node.(Value)
		if !ok || !v.Parent().Equal(kind) {
			t.Errorf("parse %q → %v, want %s", src, node, kind.Name())
		}
	}
}
