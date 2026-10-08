package native

import (
	"testing"

	parser "github.com/boru-lang/boru/parser/go"
)

// Wave-9 (cluster nativeA) shared drivers for the *_seam9_test.go
// coverage suites (design/TEST-SEAMS.10.md). Uniquely W9-prefixed so
// they never collide with any other wave's shared helpers when patches
// are merged. Most nativeA error/edge arms are reached by calling the
// handler directly with crafted args; the boru entry points cover arms
// that only fire through real dispatch.

// w9Reg returns a fresh default registry with the parser installed.
func w9Reg(t *testing.T) *Registry {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	return r
}

// w9Run parses and runs src on r, returning the residual stack.
func w9Run(t *testing.T, r *Registry, src string) ([]Value, error) {
	t.Helper()
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	r.Source = src
	e := NewTop(r)
	e.SetSource(src)
	return e.Run(toks)
}

// w9Map builds a concrete Map value from alternating string keys and
// Value payloads.
func w9Map(pairs ...any) Value {
	om := NewOrderedMap()
	for i := 0; i+1 < len(pairs); i += 2 {
		om.Set(pairs[i].(string), pairs[i+1].(Value))
	}
	return NewMap(om)
}

// w9List wraps elements into a concrete list value.
func w9List(elems ...Value) Value { return NewList(elems) }

// w9Driven returns a runner for a DIRECT handler call's result: a looping
// word's handler now exits with its loop's first region (core loop.go), so a
// test that calls the handler itself steps that region to the loop's results
// on an engine over r, exactly as the dispatching run would; plain results
// and errors pass through. Use as `drive(eachMapHandler(args, nil, nil, r))`.
func w9Driven(r *Registry) func([]Value, error) ([]Value, error) {
	return func(out []Value, err error) ([]Value, error) {
		if err != nil || !IsLoopRegion(out) {
			return out, err
		}
		return NewTop(r).Run(out)
	}
}
