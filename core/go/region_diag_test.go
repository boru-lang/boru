package core

import (
	"strings"
	"testing"
)

func TestUndefinedWordDiagWithExtraCandidates(t *testing.T) {
	r := covRegistry(t, nil)
	pos := SrcPos{Row: 2, Col: 4}

	// A frame local no registry names: the one did-you-mean line.
	ae := UndefinedWordDiagWith(r, "src", "frobnicatorx", pos, []string{"frobnicatorz"})
	if ae.Code != "undefined_word" || ae.Row != 2 || ae.Col != 4 || ae.Src != "frobnicatorx" {
		t.Fatalf("diagnostic shape: %+v", ae)
	}
	if len(ae.Suggestions) != 1 || !strings.Contains(ae.Suggestions[0].Message, "`frobnicatorz`") {
		t.Errorf("a near-miss local yields the one did-you-mean line, got %+v", ae.Suggestions)
	}

	// The nearest miss is a builtin (a reserved literal): the describe
	// pointer arrives with the fix.
	ae = UndefinedWordDiagWith(r, "src", "TRUE", pos, []string{"true"})
	if len(ae.Suggestions) != 2 || !strings.Contains(ae.Suggestions[0].Message, "`true`") ||
		!strings.Contains(ae.Suggestions[1].Message, "boru describe true") {
		t.Errorf("a builtin near-miss adds the describe pointer, got %+v", ae.Suggestions)
	}

	// A local the registry ALSO holds (a compiled frame's dynamic-scope
	// bind), or one listed twice, is offered once — the interpreter's one
	// def stack names it once.
	r.Defs.Push("frobnicatorq", NewInteger(1))
	ae = UndefinedWordDiagWith(r, "src", "frobnicatorx", pos, []string{"frobnicatorq", "frobnicatorz", "frobnicatorz"})
	if len(ae.Suggestions) != 1 || strings.Count(ae.Suggestions[0].Message, "`frobnicatorq`") != 1 ||
		strings.Count(ae.Suggestions[0].Message, "`frobnicatorz`") != 1 {
		t.Errorf("each candidate is offered once, got %+v", ae.Suggestions)
	}
	r.Defs.Pop("frobnicatorq")

	// No extras: the plain UndefinedWordDiag path.
	if plain := UndefinedWordDiag(r, "src", "frobnicatorx", pos); len(plain.Suggestions) != 0 {
		t.Errorf("without the local the registry alone has no near miss, got %+v", plain.Suggestions)
	}
}
