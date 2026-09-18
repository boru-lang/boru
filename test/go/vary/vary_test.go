package vary

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lang "github.com/boru-lang/boru/lang/go"
)

// TestLoadSeeds — value rows load with file/line provenance; comments,
// blanks, ERROR rows, tab-less prose lines, non-.tsv files, and
// subdirectories are all skipped.
func TestLoadSeeds(t *testing.T) {
	dir := t.TempDir()
	tsv := "# comment\n\n1 add 2\t3\n" +
		"prose line without a tab\n" +
		"bad row\tERROR:undefined_word\tnote\n" +
		"5 mul 5\t25\ttrailing note\n"
	if err := os.WriteFile(filepath.Join(dir, "a.tsv"), []byte(tsv), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("x\ty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.tsv"), []byte("9\t9\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	seeds, err := LoadSeeds(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []Seed{
		{File: "a.tsv", Line: 3, Input: "1 add 2"},
		{File: "a.tsv", Line: 6, Input: "5 mul 5"},
	}
	if len(seeds) != len(want) {
		t.Fatalf("seeds = %+v, want %+v", seeds, want)
	}
	for i := range want {
		if seeds[i] != want[i] {
			t.Errorf("seed[%d] = %+v, want %+v", i, seeds[i], want[i])
		}
	}
}

func TestLoadSeedsDirMissing(t *testing.T) {
	if _, err := LoadSeeds(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error for a missing dir")
	}
}

func TestLoadSeedsUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	// A dangling symlink with a .tsv name: ReadDir lists it (not a dir),
	// ReadFile fails — the per-file error arm.
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "dangling.tsv")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := LoadSeeds(dir); err == nil {
		t.Fatal("want error for an unreadable .tsv")
	}
}

// TestSampleNesting — hash-priority order is deterministic, n<=0 / n>=len
// return everything, and smaller samples are strict prefixes of larger ones
// (the property the CI ledger's stale arm relies on).
func TestSampleNesting(t *testing.T) {
	var seeds []Seed
	for _, in := range []string{"1", "2 add 2", "true not", "'x' size", "[1 2] size", "none"} {
		seeds = append(seeds, Seed{File: "f.tsv", Line: len(seeds) + 1, Input: in})
	}
	all := Sample(seeds, 0)
	if len(all) != len(seeds) {
		t.Fatalf("Sample(0) len = %d, want %d", len(all), len(seeds))
	}
	if got := Sample(seeds, len(seeds)+5); len(got) != len(seeds) {
		t.Fatalf("Sample(len+5) len = %d, want %d", len(got), len(seeds))
	}
	three := Sample(seeds, 3)
	five := Sample(seeds, 5)
	if len(three) != 3 || len(five) != 5 {
		t.Fatalf("sample sizes: %d, %d", len(three), len(five))
	}
	for i := range three {
		if three[i] != five[i] {
			t.Errorf("nesting violated at %d: %+v vs %+v", i, three[i], five[i])
		}
	}
	again := Sample(seeds, 3)
	for i := range three {
		if three[i] != again[i] {
			t.Errorf("determinism violated at %d", i)
		}
	}
}

// TestSampleTieBreaks — duplicate inputs share a hash, so ordering falls to
// file then line.
func TestSampleTieBreaks(t *testing.T) {
	seeds := []Seed{
		{File: "b.tsv", Line: 9, Input: "1 add 2"},
		{File: "a.tsv", Line: 5, Input: "1 add 2"},
		{File: "a.tsv", Line: 2, Input: "1 add 2"},
	}
	got := Sample(seeds, 0)
	want := []Seed{
		{File: "a.tsv", Line: 2, Input: "1 add 2"},
		{File: "a.tsv", Line: 5, Input: "1 add 2"},
		{File: "b.tsv", Line: 9, Input: "1 add 2"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("order[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestTransformsEmbedSeed — every transform's output contains the seed
// verbatim, and names are unique.
func TestTransformsEmbedSeed(t *testing.T) {
	const seed = "1 add 2"
	names := map[string]bool{}
	for _, tr := range Transforms() {
		if names[tr.Name] {
			t.Errorf("duplicate transform name %q", tr.Name)
		}
		names[tr.Name] = true
		wrapped := tr.Wrap(seed)
		if !strings.Contains(wrapped, seed) {
			t.Errorf("%s: wrapped %q does not embed the seed", tr.Name, wrapped)
		}
	}
}

func TestOutcomeString(t *testing.T) {
	cases := map[Outcome]string{
		Pass:         "pass",
		InterpReject: "interp-reject",
		CheckReject:  "check-reject",
		Refused:      "refused",
		Islanded:     "islanded",
		Diverged:     "DIVERGED",
	}
	for o, want := range cases {
		if got := o.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", o, got, want)
		}
	}
}

// TestClassifyRealArms — the classifier arms a healthy build CAN reach:
// pass, interpreter rejection, and a genuine compile refusal.
func TestClassifyRealArms(t *testing.T) {
	if r := Classify("1 add 2"); r.Outcome != Pass || r.Detail != "" {
		t.Errorf("1 add 2: %v %q, want pass", r.Outcome, r.Detail)
	}
	if r := Classify("zzvnosuchword"); r.Outcome != InterpReject {
		t.Errorf("undefined word: %v %q, want interp-reject", r.Outcome, r.Detail)
	}
	// A def-PROMOTED do-catch read — a stable refusal (the result leaves the
	// stack for a frame slot, so neither the mark window nor the paren apply
	// reproduces it). Earlier fixtures graduated to corpus-native: `for 3
	// [1 2]` at net drivers, the bare do-catch region `do [(zf 5) 2] error
	// [dot code]` at the mark-window island (L-DO part 2b).
	r := Classify(`def zf fn [[x:Any] [Any] [raise bad_input 'no']]  def msg (do [(zf 5) 2] error [dot code])  msg`)
	if r.Outcome != Refused || r.Detail == "" {
		t.Errorf("promoted do-catch read: %v %q, want a refusal", r.Outcome, r.Detail)
	}
}

// TestClassifySeamArms — the arms reachable only through the seams: harness
// construction failures, a check hard-error, an islanded program, a runtime
// bail, and both divergence flavours.
func TestClassifySeamArms(t *testing.T) {
	swap := func(t *testing.T) {
		prevNew, prevCC, prevRC, prevDis := langNew, compileCheck, runCompiled, disasm
		t.Cleanup(func() { langNew, compileCheck, runCompiled, disasm = prevNew, prevCC, prevRC, prevDis })
	}

	t.Run("langNew fails on nth call", func(t *testing.T) {
		for fail := 1; fail <= 3; fail++ {
			swap(t)
			calls := 0
			real := lang.New
			langNew = func() (*lang.Boru, error) {
				calls++
				if calls == fail {
					return nil, errors.New("boom")
				}
				return real()
			}
			r := Classify("1 add 2")
			if r.Outcome != CheckReject || !strings.Contains(r.Detail, "harness: boom") {
				t.Errorf("fail@%d: %v %q, want harness check-reject", fail, r.Outcome, r.Detail)
			}
			langNew = func() (*lang.Boru, error) { return lang.New() }
		}
	})

	t.Run("compileCheck hard error", func(t *testing.T) {
		swap(t)
		compileCheck = func(*lang.Boru, string) (*lang.Program, string, lang.CheckResult, error) {
			return nil, "check error", lang.CheckResult{}, errors.New("kaput")
		}
		r := Classify("1 add 2")
		if r.Outcome != CheckReject || !strings.Contains(r.Detail, "kaput") {
			t.Errorf("%v %q, want check-reject kaput", r.Outcome, r.Detail)
		}
	})

	t.Run("islanded program", func(t *testing.T) {
		swap(t)
		disasm = func(*lang.Program) string { return "0000 FALLBACK 3" }
		r := Classify("1 add 2")
		if r.Outcome != Islanded {
			t.Errorf("%v %q, want islanded", r.Outcome, r.Detail)
		}
	})

	t.Run("a program that does not answer by the deadline is hung", func(t *testing.T) {
		swap(t)
		prev := Deadline
		Deadline = 10 * time.Millisecond
		t.Cleanup(func() { Deadline = prev })
		block := make(chan struct{})
		disasm = func(*lang.Program) string { <-block; return "" }
		r := Classify("1 add 2")
		// The abandoned classification still reads the seams: let it run to
		// its end before the swap's cleanup restores them.
		close(block)
		inflight.Wait()
		if r.Outcome != Hung || !strings.Contains(r.Detail, "HUNG: no answer within 10ms") || r.Outcome.String() != "HUNG" {
			t.Errorf("%v %q, want hung", r.Outcome, r.Detail)
		}
	})

	t.Run("a panic in an engine is recovered, named by its phase", func(t *testing.T) {
		swap(t)
		disasm = func(*lang.Program) string { panic("boom") }
		r := Classify("1 add 2")
		if r.Outcome != Panicked || r.Detail != "PANIC in disassemble: boom" || r.Outcome.String() != "PANIC" {
			t.Errorf("%v %q, want the recovered panic", r.Outcome, r.Detail)
		}
	})

	t.Run("runtime bail", func(t *testing.T) {
		swap(t)
		runCompiled = func(*lang.Boru, string) ([]any, bool, error) {
			return nil, false, errors.New("bailed")
		}
		r := Classify("1 add 2")
		if r.Outcome != Refused || !strings.Contains(r.Detail, "runtime bail") {
			t.Errorf("%v %q, want runtime-bail refusal", r.Outcome, r.Detail)
		}
	})

	t.Run("error divergence", func(t *testing.T) {
		swap(t)
		runCompiled = func(*lang.Boru, string) ([]any, bool, error) {
			return nil, true, errors.New("phantom")
		}
		r := Classify("1 add 2")
		if r.Outcome != Diverged || !strings.Contains(r.Detail, "error divergence") {
			t.Errorf("%v %q, want error divergence", r.Outcome, r.Detail)
		}
	})

	t.Run("value divergence", func(t *testing.T) {
		swap(t)
		runCompiled = func(*lang.Boru, string) ([]any, bool, error) {
			return []any{int64(99)}, true, nil
		}
		r := Classify("1 add 2")
		if r.Outcome != Diverged || !strings.Contains(r.Detail, "value divergence") {
			t.Errorf("%v %q, want value divergence", r.Outcome, r.Detail)
		}
	})
}

func TestDiscardWriter(t *testing.T) {
	n, err := discard{}.Write([]byte("abc"))
	if n != 3 || err != nil {
		t.Errorf("discard.Write = %d, %v", n, err)
	}
}

// TestSweepSeeds — a passing base fans out into every transform; a
// non-passing base contributes only its own classification; order is
// deterministic and progress is monotonic. Runs single-worker (seam) to pin
// the worker floor too.
func TestSweepSeeds(t *testing.T) {
	prev := numWorkers
	t.Cleanup(func() { numWorkers = prev })
	numWorkers = func() int { return 0 } // exercises the <1 floor

	seeds := []Seed{
		{File: "f.tsv", Line: 1, Input: "1 add 2"},
		{File: "f.tsv", Line: 2, Input: "zzvnosuchword"},
	}
	var progress []int
	got := SweepSeeds(seeds, func(done, total int) {
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
		progress = append(progress, done)
	})

	wantLen := 1 + len(Transforms()) + 1
	if len(got) != wantLen {
		t.Fatalf("variants = %d, want %d", len(got), wantLen)
	}
	if got[0].Transform != "seed" || got[0].Res.Outcome != Pass {
		t.Errorf("first = %+v, want passing base", got[0])
	}
	for i, tr := range Transforms() {
		v := got[1+i]
		if v.Transform != tr.Name || v.Seed.Line != 1 {
			t.Errorf("variant[%d] = %s of line %d, want %s of line 1", i, v.Transform, v.Seed.Line, tr.Name)
		}
	}
	last := got[len(got)-1]
	if last.Transform != "seed" || last.Res.Outcome != InterpReject {
		t.Errorf("last = %+v, want interp-rejected base", last)
	}
	if len(progress) != 2 || progress[len(progress)-1] != 2 {
		t.Errorf("progress = %v, want monotonic to 2", progress)
	}
}

// TestSweepSeedsParallel — the multi-worker path with a nil report.
func TestSweepSeedsParallel(t *testing.T) {
	prev := numWorkers
	t.Cleanup(func() { numWorkers = prev })
	numWorkers = func() int { return 4 }

	seeds := []Seed{
		{File: "f.tsv", Line: 1, Input: "1"},
		{File: "f.tsv", Line: 2, Input: "2"},
		{File: "f.tsv", Line: 3, Input: "zzvnosuchword"},
	}
	got := SweepSeeds(seeds, nil)
	want := 2*(1+len(Transforms())) + 1
	if len(got) != want {
		t.Fatalf("variants = %d, want %d", len(got), want)
	}
	// Deterministic order despite parallel execution.
	if got[0].Seed.Line != 1 || got[len(got)-1].Seed.Line != 3 {
		t.Errorf("order: first line %d, last line %d", got[0].Seed.Line, got[len(got)-1].Seed.Line)
	}
}
