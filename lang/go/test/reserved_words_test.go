package test

import (
	"strings"
	"testing"
)

// TestReservedCoreWordsCannotBeRedefined pins WAT-audit Exhibit M as
// refined by design/OPEN-WORDS.0.md: a built-in word's VALUE binding
// and the reserved literals true/false/none may not be redefined or
// undefined (reserved_word); a LOCKED signature tuple may not be
// replaced by a def-merge (locked_signature); and the sealed words
// (def / make / word) cannot be extended at all. `def <builtin> fn`
// with a NEW tuple is the open-words merge — pinned in
// lang/spec/open-words.tsv, not here.
func TestReservedCoreWordsCannotBeRedefined(t *testing.T) {
	reserved := []string{
		`def if 7`,     // control word bound to a value
		`def each [1]`, // higher-order word bound to a value
		`def dup 5`,    // stack word bound to a value
		`def true 99`,  // reserved literal
		`def false 0`,  // reserved literal
		`undef add`,    // can't undef a native (no extension present)
		`undef true`,   // can't undef a literal
		// sealed words: even the fn-merge form declines
		`def def fn [[x:Number] [Number] [x]]`,
		`def make fn [[x:Number] [Number] [x]]`,
		`def word fn [[x:Number] [Number] [x]]`,
	}
	for _, src := range reserved {
		_, err := runNativeSteps(t, nil, []string{src})
		if err == nil {
			t.Errorf("%q: expected [boru/reserved_word], got no error", src)
			continue
		}
		if !strings.Contains(err.Error(), "reserved_word") {
			t.Errorf("%q: expected reserved_word error, got %v", src, err)
		}
	}

	// An attempt to claim a locked kernel tuple dies at ADMISSION under
	// rev 2 (design/OPEN-WORDS.1.md): the all-kernel tuple has no owned
	// nominal anchor, so the compile failure is extend_owner — the replacement
	// question is never even reached (the locked_signature arm survives
	// for non-core locked-bearing words; eng pins it directly in
	// TestMergeExtensionSigsLockedCollision).
	if _, err := runNativeSteps(t, nil, []string{`def add fn [[x:Number y:Number] [Number] [x sub y]] 5 3 add`}); err == nil {
		t.Errorf("locked-tuple merge: expected [boru/extend_owner], got no error")
	} else if !strings.Contains(err.Error(), "extend_owner") {
		t.Errorf("locked-tuple merge: expected extend_owner error, got %v", err)
	}

	// `none` is the None value literal, so it is rejected one layer
	// earlier (it is not a nameable token) — still illegal to redefine,
	// just via a signature error rather than the reserved-word guard.
	if _, err := runNativeSteps(t, nil, []string{`def none 1`}); err == nil {
		t.Errorf("def none 1: expected an error, got none")
	}
}

// TestUserWordsRemainRedefinable confirms the guard is scoped to core
// words: user words define, shadow (re-def), and undef freely.
func TestUserWordsRemainRedefinable(t *testing.T) {
	cases := []struct{ src, want string }{
		{`def myadd fn [[x:Number y:Number] [Number] [x add y]] 5 3 myadd`, "8"},
		{`def foo 1 def foo 2 foo`, "2"},           // re-def shadows
		{`def bar 5 def bar 6 undef bar bar`, "5"}, // undef pops the shadow
	}
	for _, c := range cases {
		out, err := runNativeSteps(t, nil, []string{c.src})
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.src, err)
			continue
		}
		if len(out) != 1 || out[0].String() != c.want {
			t.Errorf("%q = %v, want %s", c.src, out, c.want)
		}
	}
}
