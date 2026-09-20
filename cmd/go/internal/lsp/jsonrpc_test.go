// Unit tests for the JSON-RPC/LSP base-protocol transport plus the
// pure helpers in diagnostics.go, hover.go, and formatting.go that
// the round-trip tests cannot steer into their edge branches.

package lsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/formatter"
)

// failWriter fails every Write, for driving the transport error paths.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write declined") }

// --- readMessage ---

func TestReadMessageParsesFrame(t *testing.T) {
	c := newConn(strings.NewReader("Content-Type: application/json\r\nContent-Length: 2\r\n\r\n{}"), io.Discard)
	got, err := c.readMessage()
	if err != nil {
		t.Fatalf("readMessage: %s", err)
	}
	if string(got) != "{}" {
		t.Errorf("readMessage = %q, want {}", got)
	}
}

func TestReadMessageMissingContentLength(t *testing.T) {
	c := newConn(strings.NewReader("\r\n"), io.Discard)
	if _, err := c.readMessage(); err == nil || !strings.Contains(err.Error(), "missing Content-Length") {
		t.Errorf("err = %v, want missing Content-Length", err)
	}
}

func TestReadMessageInvalidContentLength(t *testing.T) {
	c := newConn(strings.NewReader("Content-Length: banana\r\n\r\n"), io.Discard)
	if _, err := c.readMessage(); err == nil || !strings.Contains(err.Error(), "invalid Content-Length") {
		t.Errorf("err = %v, want invalid Content-Length", err)
	}
}

func TestReadMessageRejectsOversizeDeclaration(t *testing.T) {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", maxMessageBytes+1)
	c := newConn(strings.NewReader(header), io.Discard)
	if _, err := c.readMessage(); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Errorf("err = %v, want exceeds limit", err)
	}
}

func TestReadMessageTruncatedBody(t *testing.T) {
	c := newConn(strings.NewReader("Content-Length: 10\r\n\r\n{}"), io.Discard)
	if _, err := c.readMessage(); err == nil {
		t.Error("expected error for body shorter than Content-Length")
	}
}

func TestReadMessageHeaderReadError(t *testing.T) {
	// No trailing newline: ReadString hits EOF mid-header.
	c := newConn(strings.NewReader("Content-Length"), io.Discard)
	if _, err := c.readMessage(); err == nil {
		t.Error("expected error when the header stream ends mid-line")
	}
}

// --- write paths ---

func TestWriteMessageWriterError(t *testing.T) {
	c := newConn(strings.NewReader(""), failWriter{})
	if err := c.writeMessage([]byte("{}")); err == nil {
		t.Error("expected write error to propagate")
	}
}

func TestRespondMarshalError(t *testing.T) {
	c := newConn(strings.NewReader(""), io.Discard)
	if err := c.respond(json.RawMessage("1"), func() {}); err == nil {
		t.Error("expected marshal error for unencodable result")
	}
}

func TestNotifyMarshalError(t *testing.T) {
	c := newConn(strings.NewReader(""), io.Discard)
	if err := c.notify("m", func() {}); err == nil {
		t.Error("expected marshal error for unencodable params")
	}
}

func TestRespondErrorWritesFrame(t *testing.T) {
	var out bytes.Buffer
	c := newConn(strings.NewReader(""), &out)
	if err := c.respondError(json.RawMessage("7"), errMethodNotFound, "nope"); err != nil {
		t.Fatalf("respondError: %s", err)
	}
	if !strings.Contains(out.String(), `"nope"`) {
		t.Errorf("output = %q, want error message", out.String())
	}
	if err := c.respondError(json.RawMessage("8"), errMethodNotFound, "x"); err != nil {
		t.Errorf("second respondError: %s", err)
	}
}

func TestHasID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"", false},
		{"null", false},
		{" null ", false},
		{"0", true},
		{"42", true},
		{`"abc"`, true},
	}
	for _, tc := range tests {
		m := rawMessage{ID: json.RawMessage(tc.id)}
		if got := m.hasID(); got != tc.want {
			t.Errorf("hasID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// --- diagnostics helpers ---

func TestPublishDiagnosticsUnknownURIIsNoop(t *testing.T) {
	var out, stderr bytes.Buffer
	s := newServer(strings.NewReader(""), &out, &stderr)
	s.publishDiagnostics("file:///never-opened")
	if out.Len() != 0 {
		t.Errorf("expected no notification for unknown URI, got %q", out.String())
	}
}

func TestPublishDiagnosticsNotifyErrorLogged(t *testing.T) {
	var stderr bytes.Buffer
	s := newServer(strings.NewReader(""), failWriter{}, &stderr)
	s.buffers["file:///u"] = "1 add 2"
	s.publishDiagnostics("file:///u")
	if !strings.Contains(stderr.String(), "publishDiagnostics") {
		t.Errorf("stderr = %q, want a publishDiagnostics failure note", stderr.String())
	}
}

func TestToLSPDiagnosticTranslations(t *testing.T) {
	tests := []struct {
		name string
		in   lang.CheckDiagnostic
		want Diagnostic
	}{
		{
			name: "error severity with word range",
			in:   lang.CheckDiagnostic{Code: "c1", Detail: "boom", Word: "add", Row: 2, Col: 3, Severity: lang.SeverityError},
			want: Diagnostic{
				Range:    Range{Start: Position{Line: 1, Character: 2}, End: Position{Line: 1, Character: 5}},
				Severity: severityError, Code: "c1", Source: "boru", Message: "boom",
			},
		},
		{
			name: "warning severity",
			in:   lang.CheckDiagnostic{Code: "c2", Detail: "meh", Word: "x", Row: 1, Col: 1, Severity: lang.SeverityWarning},
			want: Diagnostic{
				Range:    Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 1}},
				Severity: severityWarning, Code: "c2", Source: "boru", Message: "meh",
			},
		},
		{
			name: "unknown position clamps to origin, empty word gets 1-char range",
			in:   lang.CheckDiagnostic{Code: "c3", Detail: "?", Row: 0, Col: 0},
			want: Diagnostic{
				Range:    Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 1}},
				Severity: severityInformation, Code: "c3", Source: "boru", Message: "?",
			},
		},
		{
			name: "non-BMP word counts UTF-16 units",
			in:   lang.CheckDiagnostic{Code: "c4", Detail: "e", Word: "\U0001F30Dx", Row: 1, Col: 1, Severity: lang.SeverityError},
			want: Diagnostic{
				Range:    Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 3}},
				Severity: severityError, Code: "c4", Source: "boru", Message: "e",
			},
		},
	}
	for _, tc := range tests {
		if got := toLSPDiagnostic(tc.in); got != tc.want {
			t.Errorf("%s: toLSPDiagnostic = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// --- hover helpers ---

func TestWordAt(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		pos      Position
		want     string
		wantOK   bool
		wantFrom int
		wantTo   int
	}{
		{"middle of word", "1 add 2", Position{0, 3}, "add", true, 2, 5},
		{"second line", "abc\ndef", Position{1, 1}, "def", true, 0, 3},
		{"dotted namespaced word", "Color.hex2rgb x", Position{0, 7}, "Color.hex2rgb", true, 0, 13},
		{"underscore and dash", "my_word-9 y", Position{0, 4}, "my_word-9", true, 0, 9},
		{"line out of range", "abc", Position{5, 0}, "", false, 0, 0},
		{"character beyond line", "abc", Position{0, 99}, "", false, 0, 0},
		{"negative character", "abc", Position{0, -1}, "", false, 0, 0},
		{"cursor on whitespace gap", "a  b", Position{0, 2}, "", false, 0, 0},
		{"empty source", "", Position{0, 0}, "", false, 0, 0},
	}
	for _, tc := range tests {
		word, r, ok := wordAt(tc.src, tc.pos)
		if ok != tc.wantOK {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if word != tc.want {
			t.Errorf("%s: word = %q, want %q", tc.name, word, tc.want)
		}
		if r.Start.Character != tc.wantFrom || r.End.Character != tc.wantTo {
			t.Errorf("%s: range = [%d,%d), want [%d,%d)", tc.name, r.Start.Character, r.End.Character, tc.wantFrom, tc.wantTo)
		}
	}
}

func TestBuildHoverFallbacks(t *testing.T) {
	var stderr bytes.Buffer
	s := newServer(strings.NewReader(""), io.Discard, &stderr)

	// Registered word: dynamic registry info.
	if h := s.buildHover("1 add 2", Position{Line: 0, Character: 3}); h == nil {
		t.Error("hover on add should produce content")
	}
	// "sqrt" has a static help entry but no registry function: the
	// help.Lookup fallback must kick in.
	if h := s.buildHover("9 sqrt", Position{Line: 0, Character: 3}); h == nil {
		t.Error("hover on sqrt should fall back to static help")
	}
	// A word with neither registry info nor help entry → nil.
	if h := s.buildHover("zzzqqqxx", Position{Line: 0, Character: 2}); h != nil {
		t.Errorf("hover on unknown word = %+v, want nil", h)
	}
	// No word at position → nil.
	if h := s.buildHover("a  b", Position{Line: 0, Character: 2}); h != nil {
		t.Errorf("hover on whitespace = %+v, want nil", h)
	}
}

// --- formatting helpers ---

func TestBuildFormattingEditsNoChange(t *testing.T) {
	s := newServer(strings.NewReader(""), io.Discard, io.Discard)
	clean := formatter.Format("1   add    2")
	if edits := s.buildFormattingEdits(clean); len(edits) != 0 {
		t.Errorf("expected no edits for already-formatted source, got %d", len(edits))
	}
	if edits := s.buildFormattingEdits("1   add    2"); len(edits) != 1 {
		t.Errorf("expected 1 edit for unformatted source, got %d", len(edits))
	}
}
