package lang

import (
	"strings"
	"testing"
)

// TestUnmaskNoSigCheckVsCompile pins the resolution of the check-vs-compile wart
// Leaf-5 Part 1 targeted: a get over a union (Disjunct) receiver bound by an
// if-branch (the tst_unit shape) is CLEAN under a plain `boru check` (the fn body
// is analysed once at its def site, not re-entered per call). It used to decline
// force-compile as the GENERIC "check diagnostics" — a checkModeAssumeSig
// no_signature ERROR diagnostic, spuriously added on the compile pass, tripped
// CompileCheck's diagnostic gate (boru.go:297) and masked the real reason. Two
// fixes converge here: Part 1 gates that diagnostic on !Check.Compiling so it is
// never spuriously added on the compile pass, and the Stage-D poly widening
// makes this shape COMPILE outright. So now: plain check clean AND it compiles
// AND the reason is never the masked "check diagnostics". (A still-declining
// dynamic dispatch — the decision_smoke `raise`, the trie find-kid suspended
// `get` — surfaces its specific "unmatched dispatch recovered at <w>" reason in
// the voxgig force-compile sweep; the plain-check half is pinned by
// TestNoSigStillReportedInPlainCheck below.)
func TestUnmaskNoSigCheckVsCompile(t *testing.T) {
	const src = `def pluck fn [[nd:Any] [Any] [(nd "ch" get)]] ` +
		`def insert fn [[x:Any] [Any] [ def nd (if (x eq none) [{ch:1}] [x]) (nd pluck) ]] ` +
		`(none insert)`
	// Plain check is clean — no error diagnostics.
	a, _ := New()
	cr, _ := a.Check(src)
	for _, d := range cr.Diagnostics {
		if d.Severity == SeverityError {
			t.Errorf("plain check should be clean, got error: %s %s", d.Code, d.Detail)
		}
	}
	// CompileCheck no longer masks as "check diagnostics" — it now compiles.
	b, _ := New()
	prog, reason, _, _ := b.CompileCheck(src)
	if strings.Contains(reason, "check diagnostics") {
		t.Errorf("compile reason masked as %q (the wart Part 1 removes)", reason)
	}
	if prog == nil {
		t.Errorf("expected compile (Stage-D get-over-union widening), declined: %q", reason)
	}
}

// TestNoSigStillReportedInPlainCheck guards the unmask above: suppressing the
// no_signature diagnostic on a REAL compile pass must NOT suppress it during a
// plain `boru check`, where a genuine unmatched dispatch is a real static error
// the user must still see. The discriminator MUST be Check.Compiling, NOT
// Emit==nil / es.active(): a plain check arms a fresh ACTIVE Emit while
// analysing each fn body (IsolateEmit), so the fn-body-nested case below — a
// genuine type error inside `def f`'s body — is the one that pins it. (An
// earlier Emit==nil gate silently dropped exactly this error.)
func TestNoSigStillReportedInPlainCheck(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"top-level concrete type error", `({a:1} sub 3)`},
		{"top-level boolean+integer", `(true add 1)`},
		// Nested inside a fn body → analysed under an armed Emit (IsolateEmit);
		// the Compiling discriminator is what keeps this reported.
		{"fn-body-nested type error", `def use fn [[s:String] [Integer] [ 1 ]] ` +
			`def f fn [[] [Integer] [ [1 2 3] use ]]`},
	} {
		a, _ := New()
		cr, _ := a.Check(c.src)
		found := false
		for _, d := range cr.Diagnostics {
			if d.Severity == SeverityError && d.Code == "no_signature" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s (%q): plain check must still report a no_signature error (over-suppression regression)", c.name, c.src)
		}
	}
}
