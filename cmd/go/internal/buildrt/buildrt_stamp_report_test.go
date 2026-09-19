package buildrt

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	lang "github.com/boru-lang/boru/lang/go"
)

// PrintStampReport rendering: stamped lines, refusal lines (explicit and
// DEFAULTED empty reason), anonymous-name fallback, position suffix, and the
// empty-report header.
func TestPrintStampReport(t *testing.T) {
	var b strings.Builder
	PrintStampReport(&b, nil)
	if !strings.Contains(b.String(), "no runtime-stamp attempts") {
		t.Fatalf("empty report must print the header, got %q", b.String())
	}

	b.Reset()
	PrintStampReport(&b, []lang.StampEvent{
		{Name: "kv-read", Pos: core.SrcPos{Row: 73, Col: 5}, Stamped: true},
		{Name: "", Reason: "lexical captures (interpreter keeps the body)"},
		{Name: "redis-serve", Reason: ""}, // empty reason → generic text
	})
	out := b.String()
	for _, want := range []string{
		"compile-report: stamped kv-read @ 73:5",
		"compile-report: did not compile (anonymous fn) — lexical captures",
		"compile-report: did not compile redis-serve — the body did not compile as a stored fn",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
