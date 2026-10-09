package native

import (
	"strings"
	"testing"
)

// Coverage for the error/edge arms of the map overloads of each /
// for-each / fold / scan and the keys / vals projections
// (native_map_iter.go). Each handler is driven directly with crafted
// args so the individual reject arms fire:
//   - a non-concrete Map arg (bare Map type literal) → requireConcreteMap
//   - a non-concrete body (bare List type literal) → newMapBody
//   - a body that raises at runtime (undefined word) → per-entry error
//   - a body that leaves the stack empty (drop) → "produced no result"
// Positive rows accompany each so the handler's happy path stays pinned.

func w9BadBody() Value { return NewTypeLiteral(TList) }               // non-concrete → newMapBody error
func w9ErrBody() Value { return w9List(NewWord("no_such_word_xyz")) } // runtime error
func w9Drop2Body() Value {
	return w9List(NewWord("drop"), NewWord("drop")) // empties two slots
}

func w9wantErr(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", sub)
	}
	if sub != "" && !strings.Contains(err.Error(), sub) {
		t.Fatalf("error %q does not contain %q", err.Error(), sub)
	}
}

func TestW9EachMapHandlerArms(t *testing.T) {
	r := w9Reg(t)
	drive := w9Driven(r)
	m := w9Map("a", NewInteger(1), "b", NewInteger(2))

	// requireConcreteMap: bare Map literal is not concrete.
	_, err := drive(eachMapHandler([]Value{w9List(), NewTypeLiteral(TMap)}, nil, nil, r))
	w9wantErr(t, err, "expected a concrete map")

	// newMapBody: bare List literal body is not concrete/lambda/closure.
	_, err = drive(eachMapHandler([]Value{w9BadBody(), m}, nil, nil, r))
	w9wantErr(t, err, "must be a quotation list or a lambda")

	// per-entry body error.
	_, err = drive(eachMapHandler([]Value{w9ErrBody(), m}, nil, nil, r))
	w9wantErr(t, err, "key")

	// positive: empty body returns the value unchanged, keeping shape.
	out, err := drive(eachMapHandler([]Value{w9List(), m}, nil, nil, r))
	if err != nil {
		t.Fatalf("positive each: %v", err)
	}
	rm, _ := AsMap(out[0])
	if rm == nil || rm.Len() != 2 {
		t.Fatalf("positive each: expected 2-entry map, got %v", out[0])
	}
}

func TestW9ForEachMapHandlerArms(t *testing.T) {
	r := w9Reg(t)
	drive := w9Driven(r)
	m := w9Map("a", NewInteger(1))

	_, err := drive(forEachMapHandler([]Value{w9List(), NewTypeLiteral(TMap)}, nil, nil, r))
	w9wantErr(t, err, "expected a concrete map")

	_, err = drive(forEachMapHandler([]Value{w9BadBody(), m}, nil, nil, r))
	w9wantErr(t, err, "must be a quotation list or a lambda")

	_, err = drive(forEachMapHandler([]Value{w9ErrBody(), m}, nil, nil, r))
	w9wantErr(t, err, "key")

	// positive: side-effect-only run over a valid map returns nothing.
	out, err := drive(forEachMapHandler([]Value{w9List(), m}, nil, nil, r))
	if err != nil || len(out) != 0 {
		t.Fatalf("positive for-each: out=%v err=%v", out, err)
	}
}

func TestW9FoldMapHandlerArms(t *testing.T) {
	r := w9Reg(t)
	drive := w9Driven(r)
	m := w9Map("a", NewInteger(1), "b", NewInteger(2))
	seed := NewInteger(0)

	// foldMapInitHandler requireConcreteMap.
	_, err := drive(foldMapInitHandler([]Value{w9List(), NewTypeLiteral(TMap), seed}, nil, nil, r))
	w9wantErr(t, err, "expected a concrete map")

	// foldMapNoInitHandler requireConcreteMap.
	_, err = drive(foldMapNoInitHandler([]Value{w9List(), NewTypeLiteral(TMap)}, nil, nil, r))
	w9wantErr(t, err, "expected a concrete map")

	// doFoldMap newMapBody error (reached via no-init with a concrete map).
	_, err = drive(foldMapNoInitHandler([]Value{w9BadBody(), m}, nil, nil, r))
	w9wantErr(t, err, "must be a quotation list or a lambda")

	// doFoldMap per-entry body error.
	_, err = drive(foldMapInitHandler([]Value{w9ErrBody(), m, seed}, nil, nil, r))
	w9wantErr(t, err, "key")

	// doFoldMap "produced no result": drop both acc and value.
	_, err = drive(foldMapInitHandler([]Value{w9Drop2Body(), m, seed}, nil, nil, r))
	w9wantErr(t, err, "produced no result")

	// positive: empty body threads the value as the new accumulator.
	out, err := drive(foldMapNoInitHandler([]Value{w9List(), m}, nil, nil, r))
	if err != nil {
		t.Fatalf("positive fold: %v", err)
	}
	if got, _ := AsInteger(out[0]); got != 2 {
		t.Fatalf("positive fold: expected last value 2, got %d", got)
	}
}

func TestW9ScanMapHandlerArms(t *testing.T) {
	r := w9Reg(t)
	drive := w9Driven(r)
	m := w9Map("a", NewInteger(1), "b", NewInteger(2))

	_, err := drive(scanMapHandler([]Value{w9List(), NewTypeLiteral(TMap)}, nil, nil, r))
	w9wantErr(t, err, "expected a concrete map")

	_, err = drive(scanMapHandler([]Value{w9BadBody(), m}, nil, nil, r))
	w9wantErr(t, err, "must be a quotation list or a lambda")

	// per-entry body error (needs >=2 keys to enter the scan loop).
	_, err = drive(scanMapHandler([]Value{w9ErrBody(), m}, nil, nil, r))
	w9wantErr(t, err, "key")

	// positive: empty body keeps the map shape.
	out, err := drive(scanMapHandler([]Value{w9List(), m}, nil, nil, r))
	if err != nil {
		t.Fatalf("positive scan: %v", err)
	}
	rm, _ := AsMap(out[0])
	if rm == nil || rm.Len() != 2 {
		t.Fatalf("positive scan: expected 2-entry map, got %v", out[0])
	}
}

func TestW9KeysValsHandlerArms(t *testing.T) {
	r := w9Reg(t)
	m := w9Map("a", NewInteger(1), "b", NewInteger(2))

	_, err := keysHandler([]Value{NewTypeLiteral(TMap)}, nil, nil, r)
	w9wantErr(t, err, "expected a concrete map")

	_, err = valsHandler([]Value{NewTypeLiteral(TMap)}, nil, nil, r)
	w9wantErr(t, err, "expected a concrete map")

	// positive: keys and vals over a real map.
	out, err := keysHandler([]Value{m}, nil, nil, r)
	if err != nil {
		t.Fatalf("positive keys: %v", err)
	}
	if kl, _ := AsList(out[0]); kl.Len() != 2 {
		t.Fatalf("positive keys: expected 2 keys, got %v", out[0])
	}
	out, err = valsHandler([]Value{m}, nil, nil, r)
	if err != nil {
		t.Fatalf("positive vals: %v", err)
	}
	if vl, _ := AsList(out[0]); vl.Len() != 2 {
		t.Fatalf("positive vals: expected 2 vals, got %v", out[0])
	}
}
