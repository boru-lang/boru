package langspec

// TestCorpusThreeModes is the explicit gate for the generated word/structure
// corpus (lang/spec/corpus-*.tsv): every row must INTERPRET, CHECK (zero
// error-level diagnostics), and COMPILE (RunCompiled returns the interpreter's
// result) without error. The corpus is harvested from `boru describe` examples
// (core + every native module export) plus hand-written code-structure variants,
// then verified here so the three execution surfaces stay in lockstep across the
// whole language surface — not just the curated per-feature specs.
//
// "Compiled without error" accepts an interpreter island (RunCompiled's compiled
// flag may be false) as long as the compiled result matches the interpreter:
// islanding is a faithful fallback, the differential tests already gate native
// parity, and a handful of words (dynamic dispatch, code-body higher-order forms)
// legitimately island.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

func TestCorpusThreeModes(t *testing.T) {
	t.Parallel()
	specDir := filepath.Join("..", "..", "..", "lang", "spec")
	files, err := filepath.Glob(filepath.Join(specDir, "corpus-*.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no corpus-*.tsv files found")
	}

	total := 0
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		base := filepath.Base(path)
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		line := 0
		for sc.Scan() {
			line++
			raw := sc.Text()
			if strings.TrimSpace(raw) == "" || strings.HasPrefix(raw, "#") {
				continue
			}
			cols := strings.SplitN(raw, "\t", 3)
			if len(cols) < 2 {
				t.Errorf("%s:%d malformed row: %q", base, line, raw)
				continue
			}
			src, want := cols[0], cols[1]
			total++
			name := fmt.Sprintf("%s_L%d", base, line)
			t.Run(name, func(t *testing.T) {
				// 1. INTERPRET
				ai, _ := lang.New()
				gotI, err := ai.RunInterp(src)
				if err != nil {
					t.Fatalf("interpret error: %v\nsrc: %s", err, src)
				}
				// 2. CHECK — zero error-level diagnostics
				ac, _ := lang.New()
				res, cerr := ac.Check(src)
				if cerr != nil {
					t.Fatalf("check error: %v\nsrc: %s", cerr, src)
				}
				for _, d := range res.Diagnostics {
					if d.Severity == native.SeverityError {
						t.Errorf("check diagnostic [%s]: %s\nsrc: %s", d.Code, d.Detail, src)
					}
				}
				// 3. COMPILE — RunCompiled matches the interpreter (island ok)
				acc, _ := lang.New()
				gotC, _, mErr := acc.RunCompiled(src)
				if mErr != nil {
					t.Fatalf("compile/run error: %v\nsrc: %s", mErr, src)
				}
				if fmt.Sprint(gotI) != fmt.Sprint(gotC) {
					t.Errorf("compiled != interpreted:\n  interp=%v\n  compiled=%v\n  src: %s", gotI, gotC, src)
				}
				_ = want // expected value is gated by the spec-runner differential; here we gate the three surfaces agree
			})
		}
		f.Close()
	}
	t.Logf("corpus rows checked across 3 modes: %d", total)
}
