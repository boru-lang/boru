package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// geoModule is a boru module, defined inline and imported, that exports
// two USER-DEFINED TYPES and two functions whose signatures accept those
// types:
//
//   - Point   — a structural record type (refine Record).
//   - Celsius — a nominal scalar newtype (refine Integer).
//   - manhattan : (Point)   -> Integer
//   - freezing  : (Celsius) -> Boolean
//
// The exported functions are referenced as data with `/v`; the types are
// exported as their type literals. After import the namespace `Geo` makes
// both the types (`Geo.Point`, `Geo.Celsius`) and the functions
// (`Geo.manhattan`, `Geo.freezing`) available to the caller.
const geoModule = `import module [
  def Point   refine Record [x:Integer y:Integer]
  def Celsius refine Integer
  def manhattan fn [[p:Point]   [Integer] [(p.x) add (p.y)]]
  def freezing  fn [[c:Celsius] [Boolean] [(c) lte 0]]
  export "Geo" {Point: Point  Celsius: Celsius  manhattan: manhattan/v  freezing: freezing/v}
]
`

// TestModuleExportedTypesAcrossEngines defines a module that exports types
// and functions accepting those types, calls the functions, and confirms
// the result is identical and correct across all three execution engines:
// the interpreter (Run), the static type checker (Check), and the bytecode
// compiler (RunCompiled).
func TestModuleExportedTypesAcrossEngines(t *testing.T) {
	cases := []struct {
		name string
		// call is the expression appended after the module preamble.
		call string
		// want is fmt.Sprint of the result stack from interpreter / compiler.
		want string
		// checkType is the residual stack type the checker must infer.
		checkType string
		// mustCompile requires the bytecode compiler to lower the program
		// natively (not not compile). Left false for
		// programs the emitter may legitimately decline; parity is still
		// asserted in every case.
		mustCompile bool
	}{
		{
			name:        "manhattan accepts a record conforming to Point",
			call:        `Geo.manhattan {x:3 y:4}`,
			want:        `[7]`,
			checkType:   "Integer",
			mustCompile: true,
		},
		{
			name:      "freezing accepts a value made of the nominal Celsius type",
			call:      `Geo.freezing (make Geo.Celsius 0)`,
			want:      `[true]`,
			checkType: "Boolean",
		},
		{
			name:        "manhattan composes with the exported type's fields",
			call:        `Geo.manhattan {x:10 y:-3}`,
			want:        `[7]`,
			checkType:   "Integer",
			mustCompile: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := geoModule + c.call

			// Interpreter.
			ai, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			interp, err := ai.RunInterp(src)
			if err != nil {
				t.Fatalf("interpreter: %v", err)
			}
			if got := fmt.Sprint(interp); got != c.want {
				t.Errorf("interpreter = %s, want %s", got, c.want)
			}

			// Checker — the call must type-check cleanly (the value
			// satisfies the function's exported-type parameter), and the
			// residual stack type must be the function's declared return.
			ac, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			res, err := ac.Check(src)
			if err != nil {
				t.Fatalf("checker: %v", err)
			}
			if res.Summary.Errors != 0 {
				t.Errorf("checker reported %d error(s): %v", res.Summary.Errors, res.Diagnostics)
			}
			if n := len(res.Stack); n != 1 || res.Stack[n-1] != c.checkType {
				t.Errorf("checker residual stack = %v, want [%s]", res.Stack, c.checkType)
			}

			// Compiler — the bytecode result must match the interpreter.
			acc, err := lang.New()
			if err != nil {
				t.Fatalf("lang.New: %v", err)
			}
			comp, compiled, err := acc.RunCompiled(src)
			if err != nil {
				t.Fatalf("compiler: %v", err)
			}
			if got := fmt.Sprint(comp); got != c.want {
				t.Errorf("compiled = %s, want %s (compiled natively = %v)", got, c.want, compiled)
			}
			if c.mustCompile && !compiled {
				t.Errorf("expected the compiler to lower the program natively, but it did not compile")
			}
		})
	}
}

// TestModuleExportedTypesRejectWrongType confirms the exported type is a
// real constraint, not decoration: passing a value that does not inhabit
// the parameter's exported type is rejected by both the checker (a type
// error) and the interpreter (no signature matches). This is the negative
// half of the contract the positive test above relies on.
func TestModuleExportedTypesRejectWrongType(t *testing.T) {
	// A plain Integer is NOT a (nominal) Geo.Celsius, so `freezing` must
	// reject it everywhere.
	src := geoModule + `Geo.freezing 5`

	// Checker rejects it.
	ac, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	res, err := ac.Check(src)
	if err != nil {
		t.Fatalf("checker: %v", err)
	}
	if res.Summary.Errors == 0 {
		t.Errorf("checker accepted a plain Integer for a Celsius parameter; want a type error (stack=%v)", res.Stack)
	}

	// Interpreter rejects it: no signature matches, so the call does not
	// dispatch and surfaces an error.
	ai, err := lang.New()
	if err != nil {
		t.Fatalf("lang.New: %v", err)
	}
	if _, err := ai.Run(src); err == nil {
		t.Errorf("interpreter accepted a plain Integer for a Celsius parameter; want a dispatch error")
	} else if !strings.Contains(err.Error(), "signature") && !strings.Contains(err.Error(), "freezing") {
		t.Errorf("interpreter error = %v, want a signature/dispatch failure for `freezing`", err)
	}
}
