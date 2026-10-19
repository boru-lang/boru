// Cross-engine differential gate. This is the Go half of the "each engine
// validates against the other" contract: it runs the shared corpus
// (eng/spec value mode + eng/spec/check check mode) through the Go kernel and
// compares each row's result to the TypeScript engine's result for the SAME
// row, obtained by executing the TS dumper (eng/ts/src/crossdump.ts).
//
// The TS half is the mirror: eng/ts/src/crossdiff.test.ts execs THIS file's
// Go dumper (TestCrossDump, gated on the CROSSDUMP_FILE env var) and compares
// in the same way — so each engine's test suite uses the other.
//
// Until now the two engines were compared only INDIRECTLY — each asserted its
// own result against the golden `expected` column, so agreement was
// transitive. This gate compares the engines DIRECTLY, so a divergence is
// caught even on a row whose golden is wrong or absent, and functionality
// gaps are surfaced explicitly.
//
// Dispositions per row (joined on mode:file:line):
//   - agree        — both produce the same value, or both error (parity).
//   - codeDiff     — both error but with different taxonomy codes (reported).
//   - gap          — one engine produces a value where the other errors: a
//     functionality gap. PERMITTED — reported, never failed.
//   - divergence   — both produce a value but the values differ: a real
//     miscompilation/misinterpretation. HARD FAIL.
//
// The diff test SKIPS (rather than fails) when `node` is unavailable, so the
// Go module stays buildable/testable in environments without the TS toolchain.
package engspec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	basic "github.com/boru-lang/boru/basic/go"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/specfix"
)

// crossRec is one engine's result for one corpus row.
type crossRec struct {
	Mode  string `json:"mode"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Input string `json:"input"`
	OK    bool   `json:"ok"`
	Out   string `json:"out"`
}

func crossKey(mode, file string, line int) string {
	return mode + "|" + file + ":" + strconv.Itoa(line)
}

// goValueResult runs one row through the Go kernel exactly as TestSpec does,
// rendering success via core.Canon and failure as the BoruError taxonomy code
// (so error-parity is compared by code, not message text — the same shape the
// TS dumper emits).
func goValueResult(input string) (bool, string) {
	values, err := parser.Parse(input)
	if err != nil {
		// A parse error that carries a taxonomy code (e.g. a malformed
		// numeric literal → syntax_error) compares by code, so it matches the
		// TS engine regardless of which stage rejected it; a plain parser error
		// (xml syntax) keeps the TOKENIZE: message form.
		return false, errTag(err, "TOKENIZE:")
	}
	r, err := core.NewRegistry()
	if err != nil {
		return false, "UNEXPECTED:newRegistry"
	}
	specfix.RegisterSpecWords(r)
	basic.InstallMicronIdeals(r)
	r.InitRootContext()
	out, runErr := core.NewTop(r).Run(values)
	if runErr != nil {
		return false, errTag(runErr, "UNEXPECTED:")
	}
	return true, core.Canon(out)
}

// goCheckResult runs one row through the Go checker (mirrors runCheckRow),
// returning the rendered carrier+diagnostics string on success.
func goCheckResult(input string) (bool, string) {
	rendered, err := runCheckRow(input)
	if err != nil {
		return false, errTag(err, "ERR:")
	}
	return true, rendered
}

// errTag renders an error as its BoruError taxonomy code, or as
// <fallbackPrefix><message> when it is not a BoruError — matching the TS
// dumper's classification so error-parity compares by code.
func errTag(err error, fallbackPrefix string) string {
	var ae *core.BoruError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return fallbackPrefix + err.Error()
}

// walkCorpus invokes fn for every value row (eng/spec/*.tsv) and every check
// row (eng/spec/check/*.tsv), in directory+line order.
func walkCorpus(t *testing.T, fn func(mode, file string, line int, input string)) {
	t.Helper()
	root := filepath.Join("..", "..", "..", "eng", "spec")
	walkDir(t, root, "value", fn)
	walkDir(t, filepath.Join(root, "check"), "check", fn)
}

func walkDir(t *testing.T, dir, mode string, fn func(mode, file string, line int, input string)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tsv") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		for sc.Scan() {
			lineNum++
			line := strings.TrimRight(sc.Text(), " \t")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			fn(mode, e.Name(), lineNum, strings.TrimSpace(parts[0]))
		}
		f.Close()
	}
}

// goResult dispatches to the value- or check-mode runner.
func goResult(mode, input string) (bool, string) {
	if mode == "check" {
		return goCheckResult(input)
	}
	return goValueResult(input)
}

// TestCrossDump is the Go dumper: when CROSSDUMP_FILE is set it writes one
// JSONL record per corpus row (value + check) to that file and returns. The TS
// cross-diff test (crossdiff.test.ts) invokes it this way to compare against
// Go. With the env var unset it is a no-op skip, so it costs nothing in normal
// `go test` runs.
func TestCrossDump(t *testing.T) {
	outPath := os.Getenv("CROSSDUMP_FILE")
	if outPath == "" {
		t.Skip("CROSSDUMP_FILE unset — Go cross-dumper inactive")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	walkCorpus(t, func(mode, file string, line int, input string) {
		ok, out := goResult(mode, input)
		_ = enc.Encode(crossRec{Mode: mode, File: file, Line: line, Input: input, OK: ok, Out: out})
	})
	if err := os.WriteFile(outPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", outPath, err)
	}
}

// tsResults runs the TS dumper once and returns its records keyed by crossKey.
// Returns ok=false when node is unavailable so the caller can skip.
func tsResults(t *testing.T) (map[string]crossRec, bool) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, false
	}
	script := filepath.Join("..", "..", "..", "eng", "ts", "src", "crossdump.ts")
	if _, err := os.Stat(script); err != nil {
		return nil, false
	}
	cmd := exec.Command(node, "--experimental-strip-types", "--no-warnings", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Logf("TS dumper failed (%v); stderr:\n%s", err, stderr.String())
		return nil, false
	}
	recs := map[string]crossRec{}
	sc := bufio.NewScanner(&stdout)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r crossRec
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("bad TS dumper line %q: %v", line, err)
		}
		recs[crossKey(r.Mode, r.File, r.Line)] = r
	}
	return recs, true
}

// knownGoAheadDivergence reports value-mode rows where the Go engine
// intentionally leads the TS port and a differing (both-succeed) output is
// expected, not a defect. EMPTY since the TS port adopted the jsonic-based
// parser (type-name opacity + the canonical cascade + the def keyword
// forms): the two engines agree on every shared-corpus row. Add entries
// here ONLY while a documented Go-side change is landing ahead of its TS
// twin, and remove them when the port catches up.
func knownGoAheadDivergence(mode, input string) bool {
	_ = mode
	_ = input
	return false
}

func TestCrossEngineDifferential(t *testing.T) {
	ts, ok := tsResults(t)
	if !ok {
		// A skip is right where the TS engine was never installed (a plain
		// `go test`), and a lie where it was meant to run: the daily build
		// installs and builds eng/ts and sets BORU_CROSSDIFF_REQUIRED, so a
		// dumper that cannot start there fails rather than passing silently.
		if os.Getenv("BORU_CROSSDIFF_REQUIRED") != "" {
			t.Fatal("BORU_CROSSDIFF_REQUIRED is set but the TS dumper (eng/ts/src/crossdump.ts) did not run — see its stderr above")
		}
		t.Skip("node/crossdump.ts unavailable — skipping cross-engine differential")
	}

	var agree, codeDiff, gaps, divergences, goOnly int
	var gapMsgs, divMsgs, codeMsgs []string
	seenTS := map[string]bool{}
	perMode := map[string]int{}

	walkCorpus(t, func(mode, file string, line int, input string) {
		perMode[mode]++
		gOK, gOut := goResult(mode, input)
		key := crossKey(mode, file, line)
		tr, present := ts[key]
		if !present {
			goOnly++
			return
		}
		seenTS[key] = true
		loc := mode + " " + file + ":L" + strconv.Itoa(line) + " " + input

		switch {
		case gOK && tr.OK:
			switch {
			case gOut == tr.Out:
				agree++
			case knownGoAheadDivergence(mode, input):
				// A documented Go-ahead-of-TS divergence: the Go `def`
				// fixture gained the strict-barrier KEYWORD FORMS (`def name
				// fn […]`, `def name word […]`) so those forms don't strand
				// under the strict forward-barrier, which the eng kernel
				// adopted first. `inspect def` therefore lists more
				// signatures in Go than in the (still-lenient) TS port. Not
				// a value divergence to fix here — it resolves when the TS
				// port adopts the same keyword forms.
				agree++
			default:
				divergences++
				if len(divMsgs) < 25 {
					divMsgs = append(divMsgs, loc+"\n    go=  "+gOut+"\n    ts=  "+tr.Out)
				}
			}
		case !gOK && !tr.OK:
			if gOut == tr.Out {
				agree++
			} else {
				codeDiff++
				if len(codeMsgs) < 40 {
					codeMsgs = append(codeMsgs, loc+"  (go="+gOut+" ts="+tr.Out+")")
				}
			}
		default:
			gaps++
			which := "go=ok ts=err"
			if !gOK {
				which = "go=err ts=ok"
			}
			if len(gapMsgs) < 50 {
				gapMsgs = append(gapMsgs, loc+"  ("+which+"; go="+gOut+" ts="+tr.Out+")")
			}
		}
	})

	tsOnly := 0
	for k := range ts {
		if !seenTS[k] {
			tsOnly++
		}
	}

	total := agree + codeDiff + gaps + divergences
	t.Logf("cross-engine differential over shared corpus: %d rows compared (value=%d check=%d)",
		total, perMode["value"], perMode["check"])
	t.Logf("  agree:         %d", agree)
	t.Logf("  error codeDiff: %d", codeDiff)
	t.Logf("  GAPS:          %d  (one engine errors where the other succeeds — permitted)", gaps)
	t.Logf("  divergences:   %d  (both succeed, values differ — fail)", divergences)
	t.Logf("  corpus skew:   go-only=%d ts-only=%d", goOnly, tsOnly)
	for _, m := range codeMsgs {
		t.Logf("  codeDiff: %s", m)
	}
	for _, m := range gapMsgs {
		t.Logf("  gap: %s", m)
	}

	if divergences > 0 {
		sort.Strings(divMsgs)
		t.Errorf("%d cross-engine divergences (both engines produced a value, but they differ):\n%s",
			divergences, strings.Join(divMsgs, "\n"))
	}
}
