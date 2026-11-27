package core

import (
	"strconv"
	"strings"
)

// This file owns diagnostic RENDERING — the one place BoruError and
// CheckDiagnostic turn into text (design/DIAGNOSTICS.0.md). The layout
// is a strict superset of the historical four-part shape
// ([boru/<code>]: detail / --> pos / excerpt+caret / = hint), extended
// with secondary labeled spans (--- underlines), `= note:` lines, and
// `= help:` suggestions. BoruError.Error() is Render(RenderOpts{}) —
// the plain rendering — so string-matching consumers never see ANSI;
// color is caller-resolved and opt-in per call. Resolving it needs the
// process environment and a stat() on the destination, which is ambient
// I/O this kernel does not do — native.ResolveColor owns that decision
// and hands the verdict in through RenderOpts.

// RenderOpts controls diagnostic rendering. The zero value is the
// plain rendering Error() uses.
type RenderOpts struct {
	// Color enables the ANSI palette. Callers resolve it (native.ResolveColor
	// or a --color flag); the renderer itself never sniffs a terminal.
	Color bool
}

// paint wraps s in an ANSI style when color is on; identity otherwise.
func (o RenderOpts) paint(style, s string) string {
	if !o.Color || s == "" {
		return s
	}
	return style + s + cReset
}

// Render produces the full diagnostic report for e. With the zero
// RenderOpts and no structured payload it reproduces the historical
// Error() output byte-for-byte (the 872 spec ERROR rows substring-match
// it); spans, notes, and suggestions extend the report additively.
func (e *BoruError) Render(o RenderOpts) string {
	var b strings.Builder

	// Line 1: [boru/<code>]: <detail>
	b.WriteString(o.paint(cBold+cRed, "[boru/"+e.Code+"]:"))
	b.WriteString(" ")
	b.WriteString(e.Detail)

	// Line 2: --> <row>:<col>, or an explicit unknown marker. The parser
	// stamps real positions on values; an error with no position is one the
	// engine could not attribute to a source token. We say so rather than
	// guess a location by text-searching the source (which is wrong whenever
	// a word appears more than once).
	b.WriteString("\n")
	b.WriteString(o.paint(cBlue, "  --> "+e.posLabel()))

	// Primary source site extract: ^^^ underline captioned with the
	// detail, 2 context lines each side.
	if e.FullSource != "" && e.Row > 0 {
		site := renderSite(e.FullSource, e.Src, e.Detail, e.Row, e.Col, o, primarySiteStyle)
		if site != "" {
			b.WriteString("\n")
			b.WriteString(site)
		}
	}

	// Secondary labeled spans. A span with a real position renders its
	// own --> header (when it names a different location) plus a
	// one-line excerpt with a --- underline captioned by the label; a
	// span without a position degrades to a note carrying the label —
	// same no-guessing rule as the primary position.
	for _, sp := range e.Spans {
		if sp.Pos.Row <= 0 {
			writeMarker(&b, o, cBold+cBlue, "note", sp.Label)
			continue
		}
		file := sp.File
		if file == "" {
			file = e.File
		}
		source := sp.Source
		if source == "" {
			source = e.FullSource
		}
		b.WriteString("\n")
		b.WriteString(o.paint(cBlue, "  --> "+posLabel(file, sp.Pos.Row, sp.Pos.Col)))
		if source != "" {
			site := renderSite(source, sp.Pos.Src, sp.Label, sp.Pos.Row, sp.Pos.Col, o, siteStyle{underline: "-", caretStyle: cBlue, context: 0})
			if site != "" {
				b.WriteString("\n")
				b.WriteString(site)
			}
		} else if sp.Label != "" {
			writeMarker(&b, o, cBold+cBlue, "note", sp.Label)
		}
	}

	// Legacy hint — first, unchanged, until every call site migrates to
	// Notes/Suggestions.
	if e.Hint != "" {
		b.WriteString("\n  ")
		b.WriteString(o.paint(cBold, "="))
		b.WriteString(" ")
		b.WriteString(e.Hint)
	}

	for _, n := range e.Notes {
		writeMarker(&b, o, cBold+cBlue, "note", n)
	}
	for _, s := range e.Suggestions {
		writeMarker(&b, o, cBold+cCyan, "help", s.Message)
		if s.Replacement != nil {
			b.WriteString("\n          ")
			b.WriteString(o.paint(cGreen, *s.Replacement))
		}
	}

	return b.String()
}

// posLabel formats a `file:row:col` position label; the col falls back
// to 1 when unknown so the label is always clickable.
func posLabel(file string, row, col int) string {
	var b strings.Builder
	if file != "" {
		b.WriteString(file)
		b.WriteString(":")
	}
	b.WriteString(strconv.Itoa(row))
	b.WriteString(":")
	if col > 0 {
		b.WriteString(strconv.Itoa(col))
	} else {
		b.WriteString("1")
	}
	return b.String()
}

// posLabel renders e's primary position, or the honest unknown marker.
func (e *BoruError) posLabel() string {
	if e.Row <= 0 {
		return "source position unknown"
	}
	return posLabel(e.File, e.Row, e.Col)
}

// writeMarker appends one `  = <kind>: <text>` line, painting the
// marker in the given style.
func writeMarker(b *strings.Builder, o RenderOpts, style, kind, text string) {
	b.WriteString("\n  ")
	b.WriteString(o.paint(style, "= "+kind+":"))
	b.WriteString(" ")
	b.WriteString(text)
}

// siteStyle parameterizes renderSite per span role: the primary span
// underlines with ^ in the error style and shows 2 context lines; a
// secondary span underlines with - in the secondary style and shows
// just its own line.
type siteStyle struct {
	underline  string // "^" or "-"
	caretStyle string // ANSI style for the underline + caption
	context    int    // context lines each side (0 or 2)
}

// primarySiteStyle is the primary-span excerpt style — the historical
// boruErrSite shape.
var primarySiteStyle = siteStyle{underline: "^", caretStyle: cBold + cRed, context: 2}

// renderSite generates a source code extract showing one location — a
// gutter-numbered excerpt with an underline captioned by msg. With the
// primary style and plain opts it reproduces the historical boruErrSite
// output byte-for-byte.
func renderSite(src, sub, msg string, row, col int, o RenderOpts, st siteStyle) string {
	if row < 1 {
		row = 1
	}
	if col < 1 {
		col = 1
	}

	lines := strings.Split(src, "\n")

	// row is 1-based, convert to 0-based index
	lineIdx := row - 1
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}

	// Determine padding width based on largest line number shown
	maxLineNum := row + st.context
	pad := len(strconv.Itoa(maxLineNum)) + 2

	var result []string

	ln := func(num int, text string) string {
		numStr := strconv.Itoa(num)
		gutter := strings.Repeat(" ", pad-len(numStr)) + numStr + " |"
		return o.paint(cBlue, gutter) + " " + text
	}

	// Context lines before
	for d := st.context; d >= 1; d-- {
		if lineIdx-d >= 0 {
			result = append(result, ln(row-d, lines[lineIdx-d]))
		}
	}

	// The located line
	if lineIdx >= 0 && lineIdx < len(lines) {
		result = append(result, ln(row, lines[lineIdx]))
	}

	// Underline line
	caretCount := len(sub)
	if caretCount < 1 {
		caretCount = 1
	}
	indent := strings.Repeat(" ", pad) + "   " + strings.Repeat(" ", col-1)
	caption := strings.Repeat(st.underline, caretCount)
	if msg != "" {
		caption += " " + msg
	}
	result = append(result, indent+o.paint(st.caretStyle, caption))

	// Context lines after
	for d := 1; d <= st.context; d++ {
		if lineIdx+d < len(lines) {
			result = append(result, ln(row+d, lines[lineIdx+d]))
		}
	}

	return strings.Join(result, "\n")
}

// RenderCheckDiagnostic renders the RICH block for a check diagnostic —
// the source excerpt, notes, and suggestions that sit UNDER the stable
// one-line `check: r:c: [sev] code: detail` header the check command
// prints (that header is a parsing contract; this block is additive).
// source/file describe the program the diagnostic points into. Returns
// "" when there is nothing beyond the one-liner to show.
func RenderCheckDiagnostic(d CheckDiagnostic, source, file string, o RenderOpts) string {
	var b strings.Builder

	if source != "" && d.Row > 0 {
		sub := d.Src
		if sub == "" {
			sub = d.Word
		}
		site := renderSite(source, sub, "", d.Row, d.Col, o, siteStyle{underline: "^", caretStyle: severityStyle(d.Severity), context: 1})
		if site != "" {
			b.WriteString(site)
		}
	}

	for _, n := range d.Notes {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  ")
		b.WriteString(o.paint(cBold+cBlue, "= note:"))
		b.WriteString(" ")
		b.WriteString(n)
	}
	for _, s := range d.Suggestions {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  ")
		b.WriteString(o.paint(cBold+cCyan, "= help:"))
		b.WriteString(" ")
		b.WriteString(s.Message)
		if s.Replacement != nil {
			b.WriteString("\n          ")
			b.WriteString(o.paint(cGreen, *s.Replacement))
		}
	}

	_ = file // reserved: cross-file check spans render their own header in a later phase
	return b.String()
}

// severityStyle maps a check severity to its underline style.
func severityStyle(s CheckSeverity) string {
	switch s {
	case SeverityError:
		return cBold + cRed
	case SeverityWarning:
		return cBold + cYellow
	}
	return cBold + cBlue
}
