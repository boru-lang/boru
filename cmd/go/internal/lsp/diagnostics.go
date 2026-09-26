// Diagnostics: lang.Check → []lsp.Diagnostic → publishDiagnostics.

package lsp

import (
	"fmt"

	lang "github.com/boru-lang/boru/lang/go"
)

// publishDiagnostics runs the static checker over the cached buffer
// and emits a textDocument/publishDiagnostics notification with the
// translated findings.
func (s *server) publishDiagnostics(uri string) {
	src, ok := s.buffers[uri]
	if !ok {
		return
	}

	diags := s.computeDiagnostics(src)

	// Always publish (even an empty list) so the editor clears any
	// previously reported diagnostics for the URI.
	if err := s.conn.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	}); err != nil {
		fmt.Fprintf(s.stderrLog, "lsp: publishDiagnostics: %s\n", err)
	}
}

// computeDiagnostics runs lang.Check on src and converts each
// CheckDiagnostic to an LSP Diagnostic. lang.Check returns a non-nil
// error when the source fails to parse (or some other top-level
// failure occurs); we synthesise a single diagnostic for that case
// when the per-word diagnostic list is empty, so the editor shows
// a marker for malformed buffers instead of clearing existing ones.
func (s *server) computeDiagnostics(src string) []Diagnostic {
	// The check RUNS an imported module's body, so the documented
	// environment policy governs it as it governs a run (NUR079); a policy
	// that cannot be resolved is the init failure below.
	pol, err := envPolicy()
	var a *lang.Boru
	if err == nil {
		a, err = langNew(lang.Options{Policy: pol})
	}
	if err != nil {
		return []Diagnostic{{
			Range:    Range{},
			Severity: severityError,
			Code:     "boru/init",
			Source:   "boru",
			Message:  err.Error(),
		}}
	}

	res, checkErr := a.Check(src)
	out := make([]Diagnostic, 0, len(res.Diagnostics)+1)
	for _, d := range res.Diagnostics {
		out = append(out, toLSPDiagnostic(d))
	}
	if checkErr != nil && len(res.Diagnostics) == 0 {
		out = append(out, Diagnostic{
			Range:    Range{},
			Severity: severityError,
			Code:     "boru/check",
			Source:   "boru",
			Message:  checkErr.Error(),
		})
	}
	return out
}

// toLSPDiagnostic translates a boru CheckDiagnostic to LSP shape.
// boru Row/Col are 1-based; LSP is 0-based. The diagnostic range
// covers the offending word; when Word is empty, fall back to a
// single-character range at (Row, Col) so the editor still has
// somewhere to draw a marker.
func toLSPDiagnostic(d lang.CheckDiagnostic) Diagnostic {
	row := d.Row - 1
	col := d.Col - 1
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	endCol := col + utf16Len(d.Word)
	if d.Word == "" {
		endCol = col + 1
	}

	sev := severityInformation
	switch d.Severity {
	case lang.SeverityError:
		sev = severityError
	case lang.SeverityWarning:
		sev = severityWarning
	}

	// Notes and suggestions (did-you-mean, candidate verdicts —
	// design/DIAGNOSTICS.0.md) append as extra Message lines: LSP has no
	// dedicated field for them, and editors render multi-line messages.
	msg := d.Detail
	for _, n := range d.Notes {
		msg += "\nnote: " + n
	}
	for _, s := range d.Suggestions {
		msg += "\nhelp: " + s.Message
	}

	return Diagnostic{
		Range: Range{
			Start: Position{Line: row, Character: col},
			End:   Position{Line: row, Character: endCol},
		},
		Severity: sev,
		Code:     d.Code,
		Source:   "boru",
		Message:  msg,
	}
}
