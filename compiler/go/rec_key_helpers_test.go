package compiler

import core "github.com/boru-lang/boru/core/go"

// rec_key_helpers_test.go holds the fixtures' recorder-key helpers: a value
// a test treats as an event's result carries the (event, index) key the
// recorder would have stamped on it (rec_key.go).

// producedAs is v under the identity id, stamped as result idx of event seq
// of es — a fixture value an event produced.
func producedAs(es *EmitState, v core.Value, id string, seq, idx int) core.Value {
	core.SetID(&v, id)
	es.stampProduced(&v, seq, idx)
	return v
}

// read is a fixture read value: the identity id and nothing else.
func read(id string) *core.Value {
	v := core.IdentityOnly(id)
	return &v
}

// stamped is v stamped as result idx of event seq of es, under the identity
// it already has.
func stamped(es *EmitState, v core.Value, seq, idx int) core.Value {
	es.stampProduced(&v, seq, idx)
	return v
}
