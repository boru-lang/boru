package compiler

import core "github.com/boru-lang/boru/core/go"

func strictDisjunct(alts ...core.Value) core.Value {
	d := core.NewDisjunct(alts)
	core.SetCarrier(&d, true)
	return d
}
