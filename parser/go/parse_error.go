package parser

import (
	"errors"
	"fmt"

	core "github.com/boru-lang/boru/core/go"
	jsonic "github.com/tabnas/jsonic/go"
)

// This file translates tabnas/jsonic lexer-and-grammar failures into
// boru-voice BoruErrors (design/DIAGNOSTICS.0.md, phase 2). The library's
// own rendering is disabled at the source (parseErrMsgOptions /
// parseColorOff below kill the always-on ANSI palette, the
// `--internal:` suffix block, and the library docs link), and every
// *JsonicError is rebuilt as an [boru/syntax_error] carrying the same
// position, a boru-phrased detail line, and a plain-language note — so
// parse failures render through the shared diagnostic renderer exactly
// like every other boru error, on every surface (CLI, REPL, LSP, wasm).

// parseColorOff pins the library renderer's color off; the translated
// BoruError owns presentation (color is caller-resolved there).
var parseColorOff = false

// parseErrMsgOptions brands and quiets the library error format for the
// rare failure that escapes translation untyped.
var parseErrMsgOptions = jsonic.ErrMsgOptions{
	Name:   "boru",
	Suffix: false,
}

// translateParseError rebuilds a jsonic parse failure as a boru
// syntax_error at the same source position. Errors of any other type
// keep the historical `parse error: %w` wrap.
func translateParseError(err error, src string) error {
	var te *jsonic.JsonicError
	if !errors.As(err, &te) {
		return fmt.Errorf("parse error: %w", err)
	}
	detail, note, help := parseErrText(te)
	ae := core.MakeBoruErrorAt("syntax_error", detail, te.Src, src, "",
		core.SrcPos{Row: te.Row, Col: te.Col, Src: te.Src})
	if note != "" {
		ae.Notes = append(ae.Notes, note)
	}
	if help != "" {
		ae.Suggestions = append(ae.Suggestions, core.DiagSuggestion{Message: help})
	}
	return ae
}

// parseErrText maps a tabnas error code to the boru-voice detail line,
// an explanatory note, and an actionable help suggestion (either may be
// empty). The code set is the library's errorMessages table; unknown
// codes keep the library detail with an honest parser-internal note.
func parseErrText(te *jsonic.JsonicError) (detail, note, help string) {
	q := func(s string) string { return "`" + s + "`" }
	switch te.Code {
	case "unexpected":
		return "unexpected " + q(te.Src) + " — nothing valid can appear here",
			"boru's grammar allows no continuation with " + q(te.Src) + " at this position",
			"check for a missing bracket, quote, or value just before it"
	case "empty_child":
		return emptyElementDetail,
			"a typed list's `:` child needs a value after the colon",
			"write the element type after the colon, e.g. `[:Integer]`"
	case "arrow_no_body":
		return "`=>` has no body",
			"a lambda's body follows the arrow: nothing does here",
			"write the body after the arrow, e.g. `x => [ x mul 2 ]`"
	case "unterminated_string":
		return "this string is never closed: " + te.Src,
			"",
			"add the closing quote"
	case "unterminated_comment":
		return "this comment is never closed",
			"",
			"close the comment before the end of the source"
	case "end_of_source":
		return "the source ends in the middle of an expression",
			"an unclosed `[` list, `{` map, `(` group, or string earlier is the usual cause",
			""
	case "invalid_unicode":
		return "invalid unicode escape: " + te.Src,
			"the escape sequence " + q(te.Src) + " does not encode a valid unicode code point",
			""
	case "invalid_ascii":
		return "invalid ascii escape: " + te.Src,
			"the escape sequence " + q(te.Src) + " does not encode a valid ASCII character",
			""
	case "unprintable":
		return "unprintable character inside a string",
			"a raw control character (code point below 32) is not allowed in a string literal",
			"use an escape sequence instead"
	}
	// unknown_rule / internal / unknown / future codes: the failure is in
	// the parser, not the program — keep the library detail and say so.
	return te.Detail,
		"this is a parser-internal failure — likely a bug in boru's grammar, not in your program",
		""
}
