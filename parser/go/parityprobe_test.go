package parser_test

// The Go half of scripts/parity-probe.sh — a developer probe, not a gate.
//
// It renders each candidate source exactly as parserspec_test's renderSpec
// does (canon, or "ERR " and the first line of the error), so a probe result
// can be pasted straight into parse.tsv. Without PARITY_PROBE_IN set it is a
// no-op, so `go test ./...` is unaffected.
//
// Why a probe at all: a corpus row is only honest if BOTH engines were asked
// before it was written down. Writing the expected column by hand from one
// engine's behaviour is how a divergence gets baselined as a contract.

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestParityProbe(t *testing.T) {
	in := os.Getenv("PARITY_PROBE_IN")
	out := os.Getenv("PARITY_PROBE_OUT")
	if in == "" || out == "" {
		t.Skip("PARITY_PROBE_IN / PARITY_PROBE_OUT unset — probe is opt-in")
	}
	f, err := os.Open(in)
	if err != nil {
		t.Fatalf("open %s: %v", in, err)
	}
	defer func() { _ = f.Close() }()

	var b strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		b.WriteString(encodeProbe(renderSpec(decodeSpecEscapes(line))))
		b.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", in, err)
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write %s: %v", out, err)
	}
}

// encodeProbe is decodeSpecEscapes' inverse, so a render containing a
// newline stays one output line and lines up with its input.
func encodeProbe(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return strings.ReplaceAll(s, "\t", "\\t")
}
