package parser

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestParserNeverInventsNames is the regression gate for ADR-012's
// 2026-08-04 amendment: the parser emits STRUCTURE, never names. Every
// Word the parser constructs must be spelled by the user — built from
// the token's own source text — and every piece of surface sugar must
// parse to a structural marker (`core.NewSugar`) that the engine lowers
// through the registry's sugar-role table. A parser that bakes a word
// name into a string literal re-couples the grammar to one language's
// vocabulary and breaks the "eng is mechanism" boundary.
//
// The gate scans the package's non-test sources for the two ways a
// name could sneak back in:
//
//  1. `core.NewWord("…")` / `core.NewAtom("…")` with a literal string —
//     a word the user never wrote. (Constructing from a variable is
//     fine: that's the token's own text.)
//  2. A `jsonic.Text{Str: "…"}` grammar substitution whose literal is
//     not one of the allowlisted STRUCTURAL spellings. The allowlist
//     covers punctuation the user actually typed being re-emitted as
//     its own text (`)`, `?`, `!`, `|`, `.`) plus `;` → "end", whose
//     conversion lands on the kernel End marker (`core.NewEnd()`), not
//     a name-dispatched word.
//
// If a new sugar needs a word, bind a sugar role (core/go/sugar.go,
// `BindSugarWord`) and emit a marker — do not extend the allowlist
// with a word name.
var namelessGateStructuralText = map[string]bool{
	")":   true, // close paren → core.NewCloseParen()
	"end": true, // `;` statement separator → core.NewEnd()
	"?":   true, // sig optional marker (user-typed punctuation)
	"!":   true, // strict-reach marker (user-typed punctuation)
	"|":   true, // sig barrier marker (user-typed punctuation)
	".":   true, // reach dot (user-typed punctuation)
}

var (
	namelessGateLiteralWord = regexp.MustCompile(`core\.New(?:Word|Atom)\(\s*"`)
	namelessGateTextSubst   = regexp.MustCompile(`jsonic\.Text\{Str:\s*"((?:[^"\\]|\\.)*)"`)
)

func TestParserNeverInventsNames(t *testing.T) {
	structuralText := namelessGateStructuralText
	literalWord := namelessGateLiteralWord
	textSubst := namelessGateTextSubst

	var offenders []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != "." {
				return filepath.SkipDir // gate this package only
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(data), "\n") {
			if literalWord.MatchString(line) {
				offenders = append(offenders,
					filepath.Base(path)+":"+strconv.Itoa(i+1)+": literal word name: "+strings.TrimSpace(line))
			}
			for _, m := range textSubst.FindAllStringSubmatch(line, -1) {
				if !structuralText[m[1]] {
					offenders = append(offenders,
						filepath.Base(path)+":"+strconv.Itoa(i+1)+": non-structural text substitution "+m[1]+": "+strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf("the parser must not invent word names (ADR-012: emit a sugar marker instead):\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// TestParserNeverInventsNamesCatches — negative pairing for the gate:
// the patterns must actually flag the shapes they forbid, and must not
// flag the legitimate ones, or the gate is protection that isn't there.
func TestParserNeverInventsNamesCatches(t *testing.T) {
	for _, bad := range []string{
		`return core.NewWord("afn"), nil`,
		`out = append(out, core.NewAtom("gen"))`,
		`v := core.NewWord( "of")`,
	} {
		if !namelessGateLiteralWord.MatchString(bad) {
			t.Errorf("gate must flag literal name construction: %s", bad)
		}
	}
	for _, good := range []string{
		`return core.NewWord(name), nil`,
		`op := core.NewWord(opTxt.Str)`,
	} {
		if namelessGateLiteralWord.MatchString(good) {
			t.Errorf("gate must not flag source-text construction: %s", good)
		}
	}

	m := namelessGateTextSubst.FindStringSubmatch(`r.Node = jsonic.Text{Str: "afn", Quote: ""}`)
	if m == nil || namelessGateStructuralText[m[1]] {
		t.Errorf("gate must flag a word-name text substitution (got %v)", m)
	}
	m = namelessGateTextSubst.FindStringSubmatch(`r.Node = jsonic.Text{Str: "end", Quote: ""}`)
	if m == nil || !namelessGateStructuralText[m[1]] {
		t.Errorf("structural `end` substitution must stay allowlisted (got %v)", m)
	}
	if namelessGateTextSubst.MatchString(`r.Node = jsonic.Text{Str: opTxt.Str}`) {
		t.Errorf("gate must not flag a source-text substitution")
	}
}
