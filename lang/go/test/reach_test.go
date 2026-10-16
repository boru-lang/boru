package test

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// Reach Phase D: the `reach` constructor (an inert lens value) and
// inspection via convert Map / convert List. See design/REACH.10.md §7.

func TestReachConstructorIsInert(t *testing.T) {
	// `reach` builds a Reach value that does NOT auto-evaluate.
	res, err := runNativeSteps(t, nil, []string{`reach 5 [a/q b/q]`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 || !core.IsReach(&res[0]) {
		t.Fatalf("reach should yield an inert Reach value, got %v", res)
	}
}

// The reach constructor's key list (NOT evaluated) encodes segment ops:
// bare key = get, `!` marks the next key getr, `(expr)` = a computed key.
func TestReachConstructorEncodesOps(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`reach 0 [a !b (k)]`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	info, err := core.AsReach(&res[0])
	if err != nil {
		t.Fatalf("AsReach: %v", err)
	}
	if len(info.Segments) != 3 {
		t.Fatalf("want 3 segments (a, !b, (k)), got %+v", info.Segments)
	}
	// a = get, b = getr (the `!` was consumed as a marker), (k) = computed.
	if info.Segments[0].Getr {
		t.Error("segment a should be get")
	}
	if !info.Segments[1].Getr {
		t.Error("segment b should be getr (preceded by !)")
	}
	if !info.Segments[2].Computed {
		t.Error("segment (k) should be computed")
	}
	// Negative: a trailing `!` with no following key is rejected.
	if _, err := runNativeSteps(t, nil, []string{`reach 0 [a !]`}); err == nil {
		t.Error("reach 0 [a !] should error (dangling !), got nil")
	}
}

func TestReachConvertMapInspects(t *testing.T) {
	// convert Map on a codequote'd reach exposes receiver + segments.
	res, err := runNativeSteps(t, nil, []string{`convert Map (codequote m.a.b)`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	m, err := native.AsMap(res[0])
	if err != nil || m == nil {
		t.Fatalf("convert Map should yield a map, got %v", res[0])
	}
	if _, ok := m.Get("receiver"); !ok {
		t.Error("inspected reach map missing 'receiver'")
	}
	segs, ok := m.Get("segments")
	if !ok {
		t.Fatal("inspected reach map missing 'segments'")
	}
	sl, slErr := native.AsList(segs)
	if slErr != nil || sl.Len() != 2 {
		t.Errorf("expected 2 segments for m.a.b, got %v", segs)
	}
}

func TestReachConvertListKeys(t *testing.T) {
	// convert List on a reach yields its segment keys.
	res, err := runNativeSteps(t, nil, []string{`convert List (codequote m.a.b)`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	l, err := native.AsList(res[0])
	if err != nil || l.Len() != 2 {
		t.Fatalf("expected 2 keys [a b] for m.a.b, got %v", res[0])
	}
}

// A receiverless reach ($ sentinel) is an inert lens value, not eager access.
func TestReceiverlessReachIsInertLens(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`$.name`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 || !core.IsReach(&res[0]) {
		t.Fatalf("$.name should be an inert Reach value, got %v", res)
	}
	info, _ := core.AsReach(&res[0])
	if len(info.Receiver) != 0 || info.Eval {
		t.Fatalf("$.name must be receiverless + inert, got %+v", info)
	}
}

// apply applies a receiverless lens to a receiver (the lens "get"); rebind
// swaps the receiver and stays inert data.
func TestReachApplyAndRebind(t *testing.T) {
	// apply reads the field.
	res, err := runNativeSteps(t, nil, []string{`def p {a: {b: 7}}`, `p $.a.b apply`})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if n, _ := core.AsInteger(res[0]); n != 7 {
		t.Errorf("p $.a.b apply = %v, want 7", res[0])
	}
	// getr strictness survives apply: a missing strict key errors.
	if _, err := runNativeSteps(t, nil, []string{`def p {a: 1}`, `p $!.missing apply`}); err == nil {
		t.Error("p $!.missing apply should error (dotr strict), got nil")
	}
	// rebind returns an inert bound lens, not the value.
	res, err = runNativeSteps(t, nil, []string{`def p {name: "ada"}`, `rebind $.name p`})
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if len(res) != 1 || !core.IsReach(&res[0]) {
		t.Fatalf("rebind should yield a Reach value, got %v", res)
	}
}

// A receiverless reach acts as an arity-1 accessor function in higher-order
// positions: each plucks it, filter keeps truthy, sortby derives sort keys.
func TestReachAsFunction(t *testing.T) {
	// each plucks the lens from every element.
	res, err := runNativeSteps(t, nil, []string{
		`def people [{name: "ada" age: 36} {name: "bob" age: 20}]`,
		`each $.name people`,
	})
	if err != nil {
		t.Fatalf("each lens: %v", err)
	}
	l, err := core.AsList(res[0])
	if err != nil || l.Len() != 2 {
		t.Fatalf("each $.name should yield 2 names, got %v", res[0])
	}
	if s, _ := core.AsString(l.Get(0)); s != "ada" {
		t.Errorf("each $.name[0] = %v, want ada", l.Get(0))
	}
	// filter keeps elements whose lens is truthy.
	res, err = runNativeSteps(t, nil, []string{
		`def xs [{n: "a" on: true} {n: "b" on: false}]`,
		`filter $.on xs`,
	})
	if err != nil {
		t.Fatalf("filter lens: %v", err)
	}
	l, err = core.AsList(res[0])
	if err != nil || l.Len() != 1 {
		t.Fatalf("filter $.on should keep 1 element, got %v", res[0])
	}
	// Negative: the each lens form needs a concrete data list.
	if _, err := runNativeSteps(t, nil, []string{`each $.name 5`}); err == nil {
		t.Error("each $.name 5 should error (not a list), got nil")
	}
}

// getpath / setpath accepting a Reach lens (struct-util module words) are
// covered end-to-end in lang/spec/reach.tsv §5 (the langspec runner wires
// up the module resolver; runNativeSteps here does not).

// Parity guard: a parsed dot-access still evaluates eagerly (not a Reach value).
func TestParsedDotAccessStaysEager(t *testing.T) {
	res, err := runNativeSteps(t, nil, []string{`def m {a: {b: 7}}`, `m.a.b`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if n, _ := core.AsInteger(res[0]); n != 7 {
		t.Errorf("m.a.b = %v, want 7 (eager)", res[0])
	}
}
