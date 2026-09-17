package native

import (
	"fmt"
	"strings"

	parser "github.com/boru-lang/boru/parser/go"
)

// parseTextHandler implements `StructUtil.parse` — jsonic/JSON text →
// data, the decode complement of jsonify (design/legacy/PARSING.10.ignore §2).
// The input parses in DATA context, the way a map literal's interior
// parses: unquoted text becomes strings, numbers become numbers,
// true/false booleans. Nothing is evaluated and nothing dispatches —
// `StructUtil.parse "{val:'if'}"` yields a map holding the STRING
// 'if'. The jsonic superset is accepted (unquoted keys, optional
// commas), so strict JSON parses too. Malformed input raises
// [boru/parse_error] — loud, never a silent none.
func parseTextHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	text, err := AsString(args[0])
	if err != nil {
		return nil, r.BoruError("parse_error",
			fmt.Sprintf("parse: argument must be a string, got %s", args[0].String()), "parse")
	}
	// Empty input is "no value to decode" and raises (never a silent none).
	// Gate on the raw text BEFORE parsing: jsonic returns a Go nil for BOTH
	// empty input AND a valid top-level `null`, so a post-parse nil check
	// would wrongly reject `null` — which must hydrate to the concrete none
	// (jsonicToValue maps nil → None, matching how `{a: null}` decodes).
	if strings.TrimSpace(text) == "" {
		return nil, r.BoruError("parse_error",
			"parse: input is empty (no value to decode)", "parse")
	}
	// SafeParseData (not SafeParse) so number tokens preserve their
	// int/float distinction: "42.0" decodes to Float, "42" to Integer.
	result, perr := parser.SafeParseData(text)
	if perr != nil {
		return nil, r.BoruError("parse_error",
			fmt.Sprintf("parse: invalid jsonic/JSON: %v", perr), "parse")
	}
	v, cerr := jsonicToValue(result)
	if cerr != nil {
		return nil, r.BoruError("parse_error",
			fmt.Sprintf("parse: %v", cerr), "parse")
	}
	return []Value{v}, nil
}
