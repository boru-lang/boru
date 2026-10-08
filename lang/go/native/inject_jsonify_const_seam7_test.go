package native

import (
	"errors"
	"strings"
	"testing"
)

// Seam-7 coverage for inject.go, jsonify.go and native_const.go
// (design/TEST-SEAMS.10.md).

func TestSeam7InjectHandler(t *testing.T) {
	r := seam5Reg(t)
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	val := NewMap(om)
	store := NewMap(NewOrderedMap())

	// Positive: a plain map injects and converts back.
	if _, err := injectHandler([]Value{val, store}, nil, nil, r); err != nil {
		t.Fatalf("injectHandler positive: %v", err)
	}

	// Negative: force the convert-back arm via the structConvert seam.
	saved := structConvert
	t.Cleanup(func() { structConvert = saved })
	structConvert = func(any) (Value, error) { return Value{}, errors.New("boom") }
	if _, err := injectHandler([]Value{val, store}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "inject:") {
		t.Fatalf("expected inject convert error, got %v", err)
	}
}

func TestSeam7JsonifyDefaultHandler(t *testing.T) {
	r := seam5Reg(t)

	// Positive: plain map serialises.
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	out, err := jsonifyDefaultHandler([]Value{NewMap(om)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("jsonifyDefaultHandler positive: %v / %v", out, err)
	}

	// Negative: a value whose nodify behavior errors propagates.
	res, err := seam5Run(r, `def E class {x:Integer}
behave nodify/q (fn [[E] [Any] [quux]])
make E {x:1}`)
	if err != nil || len(res) == 0 {
		t.Fatalf("setup nodify-erroring instance: %v / %v", res, err)
	}
	inst := res[len(res)-1]
	if _, err := jsonifyDefaultHandler([]Value{inst}, nil, nil, r); err == nil {
		t.Fatal("expected nodify error to propagate through jsonifyDefaultHandler")
	}
}

func TestSeam7JsonifyFlagsHandler(t *testing.T) {
	r := seam5Reg(t)
	flags := NewMap(NewOrderedMap())
	om := NewOrderedMap()
	om.Set("a", NewInteger(1))

	// Positive: flags map + data.
	if _, err := jsonifyFlagsHandler([]Value{flags, NewMap(om)}, nil, nil, r); err != nil {
		t.Fatalf("jsonifyFlagsHandler positive: %v", err)
	}

	// Negative: flags arg not a map.
	if _, err := jsonifyFlagsHandler([]Value{NewInteger(1), NewMap(om)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected map") {
		t.Fatalf("expected flags map error, got %v", err)
	}

	// Negative: nodify body error on the data arg.
	res, err := seam5Run(r, `def E2 class {x:Integer}
behave nodify/q (fn [[E2] [Any] [quux]])
make E2 {x:1}`)
	if err != nil || len(res) == 0 {
		t.Fatalf("setup: %v / %v", res, err)
	}
	if _, err := jsonifyFlagsHandler([]Value{flags, res[len(res)-1]}, nil, nil, r); err == nil {
		t.Fatal("expected nodify error via jsonifyFlagsHandler")
	}
}

func TestSeam7ValueToAnySerInstances(t *testing.T) {
	r := seam5Reg(t)

	// Class instance: serialises with a $class metadata key.
	res, err := seam5Run(r, `def Point class {x:Integer y:Integer}
make Point {x:1 y:2}`)
	if err != nil || len(res) == 0 {
		t.Fatalf("class setup: %v / %v", res, err)
	}
	out := valueToAnySer(res[len(res)-1])
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("class instance must serialise to a map, got %T", out)
	}
	if _, ok := m["$class"]; !ok {
		t.Fatalf("expected $class key, got %v", m)
	}

	// Resource instance: serialises as a flat field map, no $class.
	inst := entityInst6()
	rout := valueToAnySer(inst)
	rm, ok := rout.(map[string]any)
	if !ok {
		t.Fatalf("resource instance must serialise to a map, got %T", rout)
	}
	if _, bad := rm["$class"]; bad {
		t.Fatalf("resource instance must not carry $class, got %v", rm)
	}

	// A user map key beginning with "$" escapes to "$$".
	esc := NewOrderedMap()
	esc.Set("$mongo", NewInteger(7))
	mout := valueToAnySer(NewMap(esc)).(map[string]any)
	if _, ok := mout["$$mongo"]; !ok {
		t.Fatalf("expected escaped $$mongo key, got %v", mout)
	}

	// Non-concrete input serialises to nil.
	if valueToAnySer(NewCarrier(TMap)) != nil {
		t.Fatal("non-concrete value must serialise to nil")
	}
}

func TestSeam7ConstHandler(t *testing.T) {
	r := seam5Reg(t)

	// Positive: a scalar mints a singleton and returns tagged value.
	out, err := constHandler([]Value{NewInteger(1)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("constHandler positive: %v / %v", out, err)
	}

	// Negative: a mutable Store cannot pin a value.
	if _, err := constHandler([]Value{NewStore(TStore)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "Store") {
		t.Fatalf("expected mutable-Store error, got %v", err)
	}

	// Negative: a non-concrete (type-literal) value is declined.
	if _, err := constHandler([]Value{NewTypeLiteral(TInteger)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "concrete") {
		t.Fatalf("expected concrete-value error, got %v", err)
	}
}
