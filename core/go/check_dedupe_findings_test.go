package core

import "testing"

// DedupeFindings (the end-of-pass collapse RescueForwardRefDiagnostics runs):
// an exact repeat of a finding is dropped, and everything that could make
// two lines mean different things keeps both.

func TestDedupeFindingsDropsExactRepeats(t *testing.T) {
	undef := CheckDiagnostic{Code: "undefined_word", Detail: "undefined word: nosuch", Word: "nosuch",
		Row: 1, Col: 23, Severity: SeverityError, FnBody: true, FnName: "f"}
	dead := CheckDiagnostic{Code: "unreachable_branch", Detail: "if condition is a constant true; else-branch is unreachable",
		Word: "if", Row: 1, Col: 25, Severity: SeverityWarning}
	// A position-less finding carrying its position in the detail (the
	// fn_body_error shape) dedupes like any other.
	bodyErr := CheckDiagnostic{Code: "fn_body_error", Detail: "fn body analysis error for f: … --> 1:76", Word: "f",
		Severity: SeverityError}
	c := &CheckState{Diagnostics: []CheckDiagnostic{undef, dead, undef, bodyErr, dead, bodyErr}}
	c.DedupeFindings()
	if len(c.Diagnostics) != 3 {
		t.Fatalf("each repeated finding must survive once, in first-seen order; got %+v", c.Diagnostics)
	}
	if c.Diagnostics[0].Code != "undefined_word" || c.Diagnostics[1].Code != "unreachable_branch" || c.Diagnostics[2].Code != "fn_body_error" {
		t.Errorf("first-seen order lost: %+v", c.Diagnostics)
	}
}

func TestDedupeFindingsKeepsDistinctLines(t *testing.T) {
	base := CheckDiagnostic{Code: "no_signature", Detail: "cannot call `keys`", Word: "keys",
		Row: 1, Col: 1, Severity: SeverityError}
	other := func(mut func(*CheckDiagnostic)) CheckDiagnostic {
		d := base
		mut(&d)
		return d
	}
	info := CheckDiagnostic{Code: "module_body_executed_in_check", Detail: "check executed a module body",
		Word: "import", Severity: SeverityInfo}
	ds := []CheckDiagnostic{
		base,
		// The flags are part of the identity: a mirror never absorbs a
		// model-undermining twin (the compile gate declines on the latter).
		other(func(d *CheckDiagnostic) { d.RuntimeMirror = true }),
		other(func(d *CheckDiagnostic) { d.CaughtAtRuntime = true }),
		other(func(d *CheckDiagnostic) { d.FnBody = true }),
		other(func(d *CheckDiagnostic) { d.FnBody, d.FnName = true, "g" }),
		// So is every visible field.
		other(func(d *CheckDiagnostic) { d.Col = 9 }),
		other(func(d *CheckDiagnostic) { d.Row = 2 }),
		other(func(d *CheckDiagnostic) { d.Detail = "cannot call `keys`; got (Integer)" }),
		other(func(d *CheckDiagnostic) { d.Word = "vals" }),
		other(func(d *CheckDiagnostic) { d.Src = "keys" }),
		other(func(d *CheckDiagnostic) { d.Code = "type_error" }),
		other(func(d *CheckDiagnostic) { d.Severity = SeverityWarning }),
		// So is the structured payload: a did-you-mean one analysis gained is
		// kept beside the bare twin, and a replacement's absence differs from
		// an empty one.
		other(func(d *CheckDiagnostic) { d.Notes = []string{"the note"} }),
		other(func(d *CheckDiagnostic) { d.Suggestions = []DiagSuggestion{{Message: "did you mean `vals`?"}} }),
		other(func(d *CheckDiagnostic) {
			empty := ""
			d.Suggestions = []DiagSuggestion{{Message: "did you mean `vals`?", Replacement: &empty}}
		}),
		// Info advisories are never collapsed: one per body execution is
		// the finding.
		info, info,
	}
	c := &CheckState{Diagnostics: append([]CheckDiagnostic(nil), ds...)}
	c.DedupeFindings()
	if len(c.Diagnostics) != len(ds) {
		t.Fatalf("distinct lines must all survive: kept %d of %d: %+v", len(c.Diagnostics), len(ds), c.Diagnostics)
	}
}

// RescueForwardRefDiagnostics is the end-of-pass site that runs the collapse.
func TestRescueForwardRefDiagnosticsDedupes(t *testing.T) {
	r := newTestRegistry(t)
	d := CheckDiagnostic{Code: "unreachable_branch", Detail: "dead", Word: "if", Row: 3, Col: 4, Severity: SeverityWarning}
	r.Check.Diagnostics = []CheckDiagnostic{d, d}
	r.RescueForwardRefDiagnostics()
	if len(r.Check.Diagnostics) != 1 {
		t.Fatalf("the end-of-pass rescue must collapse the repeat; got %+v", r.Check.Diagnostics)
	}
}

// Two findings with the same payload are one: the payload joins the
// identity, it does not split every repeat.
func TestDedupeFindingsSamePayloadCollapses(t *testing.T) {
	fix := "vals"
	d := CheckDiagnostic{Code: "undefined_word", Detail: "undefined word: vasl", Word: "vasl", Row: 1, Col: 1,
		Severity: SeverityError, Notes: []string{"n"}, Suggestions: []DiagSuggestion{{Message: "did you mean `vals`?", Replacement: &fix}}}
	c := &CheckState{Diagnostics: []CheckDiagnostic{d, d}}
	c.DedupeFindings()
	if len(c.Diagnostics) != 1 {
		t.Errorf("an exact repeat with its payload collapses to one, got %+v", c.Diagnostics)
	}
}
