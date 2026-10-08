// errorcodes_test.go gates REFERENCE.md's "Common codes" table against the
// codes the engine can actually mint.
//
// That table is a DISPATCH CONTRACT, not prose: readers write
//
//	do [risky] error [dot code case [not_found/q "…" bad_input/q "…"]]
//
// against it, so a documented code no site mints is not a typo — it is a
// `case` arm that can never fire, and the author has no way to discover
// that short of provoking the condition and printing the result. Seven of
// the table's twenty-three rows were exactly that when this gate was
// written (`type_mismatch`, `out_of_range`, `unify_fail`,
// `extend_user_type`, `io_error`, `cap_denied`, `cancelled`), plus one
// duplicated row. See design/verse-report-defects-investigation.0.md §E.
//
// There are now TWO truth sets, and the gate cross-checks all three
// artefacts against each other:
//
//   - MINTED — extracted from the construction sites themselves (codes are
//     string literals at ~700 of them), which is what a program can actually
//     observe.
//   - REGISTERED — the engine-owned enumeration, core.ErrorCodes()
//     (eng/go/errorcodes.go plus each layer's registration). This did not
//     exist when the gate was first written, which is why the gate was
//     one-directional then.
//   - DOCUMENTED — REFERENCE.md's "Common codes" table.
//
// The checks, and what each one is for:
//
//   - documented ⊆ minted — the original gate. A documented code nothing
//     mints is a `case` arm that can never fire.
//   - minted == registered, BOTH WAYS. A new code cannot reach users without
//     a deliberate entry in the enumeration, which is the review point the
//     naming and stability rules need in order to mean anything; and an
//     entry for a code no site mints is a phantom in the enumeration, the
//     same defect as a phantom in the table.
//   - documented ⊆ registered — so tooling that reads the enumeration
//     (`boru explain <code>`) can resolve every code a reader can look up.
//
// Two limits remain, stated because they bound what this proves:
//
//   - The table is still allowed to omit codes: it is "common codes", not a
//     census, so `registered ⊆ documented` is NOT checked.
//   - A code being minted and registered does NOT prove the row's
//     DESCRIPTION is right, only that the name is real. `type_mismatch`
//     would have failed here; a row that named `signature_error` but
//     described the wrong condition would not. That part still needs a
//     reader.
package docexamples

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	_ "github.com/boru-lang/boru/lang/go/native"
)

// codeSourceRoots are the trees searched for code-minting sites, relative to
// this package (repo root is three dirs up from test/go/docexamples).
//
// `cmd/go` is deliberately NOT scanned. Its only two matches are
// `Code: "boru/init"` and `Code: "boru/check"` in the LSP server
// (cmd/go/internal/lsp/diagnostics.go) — fields of an LSP `Diagnostic`, not
// of a BoruError. No boru program can dispatch on them, and they are not boru
// error codes; including them would force two protocol strings into the
// language's enumeration to keep the bidirectional check green. The one-way
// gate tolerated them because extra entries only made it more permissive.
// The blank import of lang/go/native is what registers the language layer's
// codes — the enumeration is per-build by design (see eng/go/errorcodes.go),
// so the gate has to link the layers it means to check. `basic/go` carries
// the fundamental words' minting sites (def / if / case / fn / var / gen …)
// since the ADR-013 layering split; `core/go` carries the kernel sites that
// moved with the interpreter-core cut (design/legacy/ENG-FOUR-PIECE.0.ignore Stage 4);
// `parser/go` carries the syntax sites (float_overflow, integer_overflow,
// syntax_error …) that moved with the parser cut.
//
// This list is COUPLED TO THE MODULE LAYOUT. Extracting a module without
// adding it here does not fail at the extraction — it fails later as
// "registered but attached by no site", which is how the parser cut was
// caught (float_overflow, minted in parse.go, fell out of every root).
var codeSourceRoots = []string{"core/go", "check/go", "compiler/go", "parser/go", "eng/go", "basic/go", "lang/go"}

// codeMintPatterns match every shape that ATTACHES an error code to an
// error or a diagnostic. Capture group 1 is the code.
//
// Deliberately NOT matched: a package-level constant that merely spells a
// code (`CodePermissionDenied = "boru/permission_denied"`). Counting
// declarations would have made this gate vacuous exactly where it is most
// needed — `policy` declares four such constants, its header says "the
// engine adapter copies these onto the produced BoruError", and until that
// adapter was written not one of them ever reached a user. A code is real
// when a site attaches it, not when a site names it.
//
// The `boru/` prefix stays optional in the struct-literal pattern because a
// code can be assigned from such a constant; BoruError stores the bare name
// and renders the prefix.
var codeMintPatterns = []*regexp.Regexp{
	// r.BoruError("x", …) / r.BoruErrorHint("x", …) / MakeBoruError("x", …)
	// / MakeBoruErrorAt / makeBoruError / makeBoruErrorAt.
	regexp.MustCompile(`\b(?:M|m)akeBoruError(?:At)?\(\s*"([a-z][a-z0-9_-]*)"`),
	// …and the POSITIONED constructors r.BoruErrorAt / r.BoruErrorHintAt. They
	// arrived with the boru:io positionless-error pilot, after this gate was
	// written, and without them every code minted only through a positioned
	// site was invisible here — neither counted as mintable nor required to
	// be in the enumeration, which is precisely the drift the gate exists to
	// catch. `exit_error` was the first such code.
	regexp.MustCompile(`\.BoruError(?:Hint)?(?:At)?\(\s*"([a-z][a-z0-9_-]*)"`),
	// BoruError / Diagnostic struct literals: Code: "x".
	regexp.MustCompile(`\bCode:\s*"(?:boru/)?([a-z][a-z0-9_-]*)"`),
	// Check-mode diagnostics: CheckAddUniqueDiagnostic(r, "x", …).
	regexp.MustCompile(`\bCheckAdd\w*Diagnostic\([^,)]*,\s*"([a-z][a-z0-9_-]*)"`),
}

// codeLiteralPatterns match the same attaching calls as codeMintPatterns but
// capture ANY string literal, not just a well-formed code. They exist because
// the strict patterns have a blind spot that cost real user-visible breakage:
// a code that does not match the naming rule simply does not match the regex,
// so it is INVISIBLE to every check here — not reported as malformed, not
// counted as mintable, and not required to be in the enumeration.
//
// A live example was `r.BoruError("unknown key_error", …)` in the Store `get`
// handler: the message's leading word glued onto the code. A code containing a
// SPACE cannot be matched by any `case` arm, so the failure was undispatchable,
// and the same call passed "unknown key" as the WORD, which the compiled path
// renders as an 11-caret span against the interpreter's 3. One typo, two
// user-visible defects, and every gate silent.
var codeLiteralPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:M|m)akeBoruError(?:At)?\(\s*"([^"]*)"`),
	regexp.MustCompile(`\.BoruError(?:Hint)?(?:At)?\(\s*"([^"]*)"`),
}

// TestEveryAttachedCodeLiteralIsWellFormed is the blind-spot check: every
// string literal handed to an error constructor as its CODE must satisfy the
// naming rule the enumeration enforces at registration
// (eng/go/errorcodes.go's errorCodeNamePattern). Without it the rule only
// binds codes that already conform, which is the set that never needed it.
func TestEveryAttachedCodeLiteralIsWellFormed(t *testing.T) {
	// The rule is DISPATCHABILITY, not house style. A code has to be
	// spellable as a `case` arm, so a hyphen is fine — boru word names use
	// them freely (`for-each`, `with-decimal`) and four codes follow suit
	// (`expected-byte`, `bad-encoding`, `cancel-timeout_error`,
	// `cancel-interval_error`). A SPACE or a capital is not spellable, and
	// that is what this catches.
	wellFormed := regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	seen := 0
	for _, root := range codeSourceRoots {
		dir := filepath.Join("..", "..", "..", root)
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// eng/go/specfix is TEST SCAFFOLDING that happens to live as a
			// real package (the standalone corpus lanes and the engspec
			// harness share it): its fixture words mint harness-local codes
			// (fn_invalid_spec — mirrored verbatim by the TS fixture,
			// eng/ts/src/spec-fixture.ts) that are not language dispatch
			// contracts, so they stay out of the enumeration the same way
			// cmd/go's LSP protocol strings do (see codeSourceRoots).
			if strings.Contains(filepath.ToSlash(path), "/specfix/") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, line := range strings.Split(string(src), "\n") {
				// Skip comment lines: this file's own prose quotes example
				// calls (`r.BoruError("…", …)`), and a doc comment is not a
				// construction site.
				if strings.HasPrefix(strings.TrimSpace(line), "//") {
					continue
				}
				for _, re := range codeLiteralPatterns {
					for _, m := range re.FindAllStringSubmatch(line, -1) {
						seen++
						if code := m[1]; !wellFormed.MatchString(code) {
							t.Errorf("%s attaches the code %q, which no `case` arm can "+
								"match — a code is a dispatch contract, and a name with a "+
								"space or a capital is not spellable as an arm, so the "+
								"failure is undispatchable.", path, code)
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	// A floor, so a regex that stops matching fails as a broken gate rather
	// than as a clean bill of health.
	if seen < 200 {
		t.Fatalf("only %d code literals found across %v — codeLiteralPatterns "+
			"have stopped matching", seen, codeSourceRoots)
	}
}

// mintableCodes walks the source roots and returns every code any site
// can attach to an error or diagnostic.
func mintableCodes(t *testing.T) map[string]string {
	t.Helper()
	found := map[string]string{}
	for _, root := range codeSourceRoots {
		dir := filepath.Join("..", "..", "..", root)
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return nil
			}
			// eng/go/specfix is TEST SCAFFOLDING that happens to live as a
			// real package (the standalone corpus lanes and the engspec
			// harness share it): its fixture words mint harness-local codes
			// (fn_invalid_spec — mirrored verbatim by the TS fixture,
			// eng/ts/src/spec-fixture.ts) that are not language dispatch
			// contracts, so they stay out of the enumeration the same way
			// cmd/go's LSP protocol strings do (see codeSourceRoots).
			if strings.Contains(filepath.ToSlash(path), "/specfix/") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, re := range codeMintPatterns {
				for _, m := range re.FindAllSubmatch(src, -1) {
					code := string(m[1])
					if _, seen := found[code]; !seen {
						found[code] = path
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	return found
}

// documentedCodes parses the "Common codes" table out of REFERENCE.md,
// in document order and WITH duplicates, so the caller can flag both
// phantoms and repeated rows.
func documentedCodes(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "REFERENCE.md"))
	if err != nil {
		t.Fatalf("reading REFERENCE.md: %v", err)
	}
	lines := strings.Split(string(src), "\n")
	// Anchor on the prose that introduces the table rather than on a
	// line number or on "| Code | Meaning |" — REFERENCE.md has two
	// other tables with that exact header, both of them EXIT codes.
	start := -1
	for i, ln := range lines {
		if strings.Contains(ln, "Common codes:") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("REFERENCE.md: no `Common codes:` table found — if the table " +
			"moved or was renamed, retarget this gate rather than deleting it")
	}
	// One code per row, by convention — a cell spelling two codes
	// (`read_error` / `write_error`) would be skipped silently, which is
	// the one way a row could slip past this gate. Split such rows.
	cell := regexp.MustCompile("^\\|\\s*`([a-z][a-z0-9_]*)`\\s*\\|")
	suspicious := regexp.MustCompile("^\\|\\s*`[a-z][a-z0-9_]*`[^|]")
	var out []string
	for _, ln := range lines[start:] {
		if m := cell.FindStringSubmatch(ln); m != nil {
			out = append(out, m[1])
			continue
		}
		if suspicious.MatchString(ln) {
			t.Errorf("codes-table row names more than one code in its first "+
				"cell, so this gate cannot read it — give each code its own "+
				"row:\n  %s", strings.TrimSpace(ln))
			continue
		}
		// Stop at the first blank line AFTER at least one row, so the
		// gap between the prose anchor and the table header doesn't end
		// the scan.
		if len(out) > 0 && strings.TrimSpace(ln) == "" {
			break
		}
	}
	return out
}

func TestReferenceErrorCodesAreMintable(t *testing.T) {
	mintable := mintableCodes(t)
	// A floor, so a regex that silently stops matching fails as a broken
	// gate rather than as ~20 simultaneous "phantom" reports pointing at
	// innocent rows.
	if len(mintable) < 100 {
		t.Fatalf("extracted only %d codes from %v — the mint patterns have "+
			"stopped matching; fix codeMintPatterns before reading any "+
			"failure below as a documentation bug", len(mintable), codeSourceRoots)
	}

	documented := documentedCodes(t)
	if len(documented) < 10 {
		t.Fatalf("parsed only %d rows out of REFERENCE.md's codes table — "+
			"the table's shape changed and this gate is no longer reading it",
			len(documented))
	}

	for _, code := range documented {
		if _, ok := mintable[code]; !ok {
			t.Errorf("REFERENCE.md documents `%s`, which NO site mints — a "+
				"`case %s/q` arm against it can never fire. Either the code "+
				"was renamed (find the real one and fix the row) or the "+
				"condition raises code-lessly (give it a code).", code, code)
		}
	}
}

// A duplicated row is its own small defect: the table is read as a
// checklist, and the same code described twice reads as two conditions.
func TestReferenceErrorCodeTableHasNoDuplicates(t *testing.T) {
	seen := map[string]bool{}
	for _, code := range documentedCodes(t) {
		if seen[code] {
			t.Errorf("REFERENCE.md's codes table lists `%s` twice", code)
		}
		seen[code] = true
	}
}

// registeredCodes returns the engine-owned enumeration as a set.
func registeredCodes() map[string]string {
	out := map[string]string{}
	for _, ec := range core.ErrorCodes() {
		out[ec.Code] = ec.Owner
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestEveryMintedCodeIsRegistered is the direction that did not exist before
// the enumeration did, and it is the one that makes the enumeration worth
// having: a code cannot reach users without a deliberate entry.
//
// That is the whole enforcement mechanism behind the naming rule and the
// stability contract in eng/go/errorcodes.go. Neither can be enforced by
// inspection — 232 codes across ~700 sites is more than anyone re-reads — but
// both are enforced by a failing build the first time a new literal appears.
func TestEveryMintedCodeIsRegistered(t *testing.T) {
	minted := mintableCodes(t)
	if len(minted) < 100 {
		t.Fatalf("extracted only %d codes from %v — the mint patterns have "+
			"stopped matching; fix codeMintPatterns before reading any failure "+
			"below as a missing registration", len(minted), codeSourceRoots)
	}
	registered := registeredCodes()

	var unregistered []string
	for code := range minted {
		if _, ok := registered[code]; !ok {
			unregistered = append(unregistered, code+" ("+minted[code]+")")
		}
	}
	sort.Strings(unregistered)
	if len(unregistered) > 0 {
		t.Errorf("%d code(s) are attached by a site but not registered in the "+
			"enumeration:\n  %s\n\nAdd each to kernelErrorCodes "+
			"(eng/go/errorcodes.go) or langErrorCodes "+
			"(lang/go/native/errorcodes.go), whichever layer owns it. A code is "+
			"a dispatch contract — `do [...] error [dot code case [...]]` — so "+
			"this is the moment to check the NAME reads like its neighbours, "+
			"because renaming it later silently stops every handler that "+
			"matched it.", len(unregistered), strings.Join(unregistered, "\n  "))
	}
}

// TestEveryRegisteredCodeIsMinted is the other direction: an entry for a code
// no site attaches. It is the same defect as a phantom row in REFERENCE.md,
// one level down — tooling built on the enumeration (`boru explain <code>`, an
// editor completion list) would offer a code that can never appear.
//
// It also catches the likelier bookkeeping error: a code deleted from the
// source but left in the list, which would otherwise keep the rename that
// caused it invisible.
func TestEveryRegisteredCodeIsMinted(t *testing.T) {
	minted := mintableCodes(t)
	if len(minted) < 100 {
		t.Fatalf("extracted only %d codes — fix codeMintPatterns first", len(minted))
	}
	registered := registeredCodes()

	var phantom []string
	for _, code := range sortedKeys(registered) {
		if _, ok := minted[code]; !ok {
			phantom = append(phantom, code+" [owner "+registered[code]+"]")
		}
	}
	if len(phantom) > 0 {
		t.Errorf("%d code(s) are registered in the enumeration but attached by "+
			"NO site:\n  %s\n\nEither the site was removed (drop the entry) or "+
			"the code was renamed (fix the entry AND check whether REFERENCE.md "+
			"and any user handler named the old spelling).",
			len(phantom), strings.Join(phantom, "\n  "))
	}
}

// TestDocumentedCodesAreRegistered closes the triangle. A code a reader can
// look up in REFERENCE.md must be resolvable through the enumeration, because
// that is what a `boru explain <code>` would consult — a documented code the
// enumeration cannot resolve would report "no such code" for something the
// manual describes.
func TestDocumentedCodesAreRegistered(t *testing.T) {
	registered := registeredCodes()
	for _, code := range documentedCodes(t) {
		if _, ok := registered[code]; !ok {
			t.Errorf("REFERENCE.md documents `%s`, which is not in the "+
				"engine-owned enumeration — tooling that reads core.ErrorCodes() "+
				"cannot resolve a code the manual tells readers to handle", code)
		}
	}
}
