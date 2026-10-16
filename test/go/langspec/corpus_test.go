package langspec

// TestCorpusTwoModes is the explicit gate for the generated word/structure
// corpus (lang/spec/corpus-*.tsv): every row must INTERPRET and CHECK (zero
// error-level diagnostics) without error. The corpus is harvested from `boru
// describe` examples (core + every native module export) plus hand-written
// code-structure variants, then verified here so the two surfaces stay in
// lockstep across the whole language surface — not just the curated
// per-feature specs.

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

func TestCorpusTwoModes(t *testing.T) {
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
				if _, err := ai.Run(src); err != nil {
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
				_ = want // the expected value is the spec runner's to gate; here the two surfaces must agree
			})
		}
		f.Close()
	}
	t.Logf("corpus rows checked across 2 modes: %d", total)
}
