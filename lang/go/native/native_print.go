package native

import (
	core "github.com/boru-lang/boru/core/go"
)

// printNatives covers `print` — the one output word that stays in core so
// basic output needs no import. `print` writes its argument's formatted
// representation to the registry's Output writer followed by a newline.
// `printstr` (no newline) moved to the boru:io module (see io_module.go).
//
// Algorithms (FormatForPrint and the rest of print.go) live in eng;
// this file owns the word name and dispatch wiring.
var printNatives = []NativeFunc{
	{
		Name: "print",

		Signatures: []Signature{{
			Args:    []*Type{TAny},
			Impl:    Go(core.PrintHandler),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	},
}
