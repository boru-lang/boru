package eng_test

// Unit probes for the specfix fixture words' guard arms — the branches
// no eng/spec corpus row reaches (fixture-internal error guards, the
// storage handlers the corpus only inspects, and the inspection arms
// for type kinds the corpus never constructs standalone). The corpus
// lanes in corpus_standalone_test.go carry the fixtures' happy paths;
// these probes pin the rest so specfix meets ADR-008 as production
// code. Class types, dep scalars, and table types are constructed
// through the kernel's exported constructors and bound via the def
// table — the same shapes lang's words mint, minus the lang registry.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	parser "github.com/boru-lang/boru/parser/go"
	"github.com/boru-lang/boru/test/specfix"
)

// specfixProbeRegistry is standaloneRegistry plus Go-constructed
// bindings the probe rows reference by name:
//
//	P     class type {x:Integer} rooted at Class
//	p     instance of P with x=1
//	pm    mutable-map value whose Parent is P's def (getObjectH's
//	      AsMutableMap arm — the pre-class object-instance shape)
//	dslo  dep scalar Integer >= 1 (inspect's lo-bound arm)
//	dshi  dep scalar Integer <= 5 (inspect's hi-bound arm)
//	tbl   table type [{c:Integer}] (inspect's table arm)
//	t_    a lowercase TYPE binding (typedDef's name-clash arm)
func specfixProbeRegistry(t *testing.T) *core.Registry {
	t.Helper()
	r, err := standaloneRegistry()
	if err != nil {
		t.Fatal(err)
	}
	specfix.RegisterCheckExtras(r)
	specfix.RegisterControlWords(r)

	def := r.Types.MintType("Class/P", core.TClass)
	fields := core.NewOrderedMap()
	fields.Set("x", core.NewTypeLiteral(core.TInteger))
	pinfo := core.ClassTypeInfo{Fields: fields, ID: "P1", Name: "Class/P"}
	pbody := core.NewClassType(def, pinfo)
	// Mirror installTypeBinding: the node records its declared content
	// (design/legacy/TYPE-REPRESENTATION.1.ignore §N2) so the probes that evaluate
	// `P` — which now denotes the node — recover the class schema.
	def.SetTypeBody(pbody)
	r.Defs.PushType("P", def, pbody)

	ifields := core.NewOrderedMap()
	ifields.Set("x", core.NewInteger(1))
	r.Defs.Push("p", core.NewClassInstance(def, core.ClassInstanceInfo{TypeRef: &pinfo, Fields: ifields}))

	mm := core.NewOrderedMap()
	mm.Set("y", core.NewInteger(2))
	r.Defs.Push("pm", core.NewValueRaw(def, core.MapPayload{M: mm}))

	r.Defs.Push("dslo", core.NewDepScalar(core.DepGTE, core.NewInteger(1)))
	r.Defs.Push("dshi", core.NewDepScalar(core.DepLTE, core.NewInteger(5)))

	// dd: a CONCRETE disjunct binding — the loop-capture models see a
	// def-resolved Disjunct on the body stack (source can only reach a
	// computed enum there, which check mode models as a carrier).
	r.Defs.Push("dd", core.NewEnum([]core.Value{core.NewAtom("a"), core.NewAtom("b")}))

	tf := core.NewOrderedMap()
	tf.Set("c", core.NewTypeLiteral(core.TInteger))
	r.Defs.Push("tbl", core.NewValueRaw(core.TList, core.TableTypeInfo{Record: core.RecordTypeInfo{Fields: tf}}))

	r.Defs.PushType("t_", core.TInteger, core.NewTypeLiteral(core.TInteger))

	// P0: a class type whose info carries no Type back-reference (the
	// raw-ClassTypeInfo shape), so objectWithParentH's TClass fallback
	// for the minted child's parent def is exercised.
	def0 := r.Types.MintType("Class/P0", core.TClass)
	f0 := core.NewOrderedMap()
	f0.Set("x", core.NewTypeLiteral(core.TInteger))
	p0body := core.NewValueRaw(def0, core.ClassTypeInfo{Fields: f0, ID: "P0", Name: "Class/P0"})
	def0.SetTypeBody(p0body)
	r.Defs.PushType("P0", def0, p0body)

	// nosig: a Function binding whose FnDefInfo declares no signatures,
	// so buildInspection's empty-signature normalisation is exercised.
	r.Defs.Push("nosig", core.NewFunction(core.FnDefInfo{Name: "nosig"}))
	return r
}

func TestSpecfixGuardProbes(t *testing.T) {
	rows := []struct {
		input   string
		want    string   // exact Canon of the result stack
		wantSub []string // substrings of the Canon (generated type IDs vary)
		wantErr string   // substring of the error
	}{
		// def — the universal binder's fixture arms.
		{input: "def Big Integer 3 is Big", want: "true"},
		{input: "def $$ 1", wantErr: "is reserved"},
		{input: "def {f:Function} (fn [Integer Any [5]]) f 7", want: "5"},
		{input: "def {a:Integer b:Integer} 3", wantErr: "typed-name map must have exactly one key"},
		{input: "def {Foo:Integer} 3", wantErr: `word "Foo" must begin with`},
		{input: "def {t_:Integer} 1", wantErr: "name clash — already a type"},
		{input: "def {x:String} 3", wantErr: "does not satisfy declared type String"},
		{input: "def Foo word 42", wantErr: `word "Foo" must begin with`},
		{input: "def Foo fn [1 2 3]", wantErr: `word "Foo" must begin with`},
		{input: "def f fn [1 2]", wantErr: "list length must be a non-zero multiple of 3"},
		{input: "def f fn [x Integer [1]]", wantErr: `invalid type "x"`},
		{input: "def f fn [1 2 3] inspect f",
			want: "{name:'f' kind:defined signatures:[{args:['Integer']} {args:[]}]}"},

		// word / fn — bare forms.
		{input: "word 5", want: "5"},
		{input: "fn [1 2]", wantErr: "list length must be a non-zero multiple of 3"},
		{input: "fn [x Integer [1]]", wantErr: `invalid type "x"`},

		// get — the kernel-container fixture arms the corpus only inspects.
		{input: "[10 20 30] get 1", want: "20"},
		{input: "[10 20 30] get 5", want: "None"},
		{input: "{a:1} get a", want: "1"},
		{input: "{a:1} get b", want: "None"},
		{input: "[1 2] get a", want: "None"},
		{input: "{a:1} get b get 1", want: "None"},
		{input: "p get x", want: "1"},
		{input: "p get z", want: "None"},
		{input: "pm get y", want: "2"},
		{input: "pm get z", want: "None"},

		// set — object-instance mutation and its guards.
		{input: "set x 5 p p get x", want: "5"},
		// Stage 2 flip: `P` denotes its minted node, so the type-literal
		// guard fires ahead of the instance-shape assert — matching the
		// production storage word's post-flip compile failure.
		{input: "set x 5 P", wantErr: "cannot set field on type literal"},
		// …while a concrete non-instance at the Class slot still reaches
		// the instance-shape assert (pm is P-typed but map-shaped).
		{input: "set x 5 pm", wantErr: "expected an Object instance"},

		// refine — the object/record constructor arms.
		{input: "refine P {y:String}", wantSub: []string{"object<", "x:Integer y:String}"}},
		// A NAMED argument (the Stage 2 flip): M2's node records the
		// field map, and refine's arg-resolution recovers it.
		{input: "def M2 {y:String} refine P M2", wantSub: []string{"object<", "x:Integer y:String}"}},
		{input: "refine P0 {y:String}", wantSub: []string{"object<", "x:Integer y:String}"}},
		{input: "refine P {x:String}", wantErr: "cannot expand parent type"},
		{input: "refine P [1]", wantErr: "must be a map of field definitions"},
		{input: "refine P {:Integer}", wantErr: "must be a concrete map"},
		{input: "refine Class {x:Integer}", wantErr: "base must be Record or an object type"},
		{input: "refine Record {a:Integer}", wantErr: "a record takes a list of field pairs"},
		{input: "refine Record []", wantErr: "must have at least one field"},
		{input: "refine Record [5]", wantErr: "each element must be a pair (map)"},
		{input: "refine Record [ {:Integer} ]", wantErr: "must be a concrete pair"},
		{input: "refine Integer", want: "Integer"},
		{input: "refine 5", wantErr: "argument must be a type"},

		// inspect — the type-kind arms the corpus never constructs.
		{input: "inspect P",
			want: "{name:'P' type:'Type' struct:'Class/P' kind:object fields:{x:'Integer'}}"},
		{input: "def C (refine P {y:String}) inspect C",
			wantSub: []string{"kind:object", "parent:'Class/P'", "y:'String'"}},
		{input: "inspect tbl",
			want: "{name:'tbl' type:'Type' struct:'List' kind:table fields:{c:'Integer'}}"},
		{input: "inspect dslo",
			want: "{name:'dslo' type:'Type' struct:'Integer' kind:dependent_scalar leaf:'Integer' lo:{kind:'gte' value:1}}"},
		{input: "inspect dshi",
			want: "{name:'dshi' type:'Type' struct:'Integer' kind:dependent_scalar leaf:'Integer' hi:{kind:'lte' value:5}}"},
		{input: "def tt Integer inspect tt",
			want: "{name:'tt' type:'Type' struct:'Integer' kind:literal}"},
		{input: "inspect nosig", want: "{name:'nosig' kind:native signatures:[]}"},
		{input: "bakefnq nosig/v", wantErr: "bakefnq: argument must be a fn"},

		// fnsig / do — the remaining reachable guard arms.
		{input: "fnsig [x y]", wantErr: `invalid type "x"`},
		{input: "do [notaword_xyz]", want: "error(undefined word: notaword_xyz)"},
		// The embedded code list errors during dispatch's map auto-eval
		// and the error must surface (the walk's own error arms are
		// unit-tested on doEvalMapValue directly in specfix).
		{input: "do {a:[refine 5]}", wantErr: "argument must be a type"},
		{input: "do {a:{b:[refine 5]}}", wantErr: "argument must be a type"},

		// noretq — the check-extras fixture's impl, run at runtime.
		{input: "noretq 1", want: "1"},
	}
	for _, row := range rows {
		t.Run(row.input, func(t *testing.T) {
			values, err := parser.Parse(row.input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			out, runErr := core.NewTop(specfixProbeRegistry(t)).Run(values)
			if row.wantErr != "" {
				if runErr == nil {
					t.Fatalf("want error containing %q, got %s", row.wantErr, core.Canon(out))
				}
				if !strings.Contains(runErr.Error(), row.wantErr) {
					t.Errorf("error %q does not contain %q", runErr.Error(), row.wantErr)
				}
				return
			}
			if runErr != nil {
				t.Fatalf("unexpected error: %v", runErr)
			}
			got := core.Canon(out)
			if row.want != "" && got != row.want {
				t.Errorf("got %s, want %s", got, row.want)
			}
			for _, sub := range row.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("got %s, missing %q", got, sub)
				}
			}
		})
	}
}

// TestSpecfixCheckModeGuards drives the guards only a check-mode
// carrier can reach: `refine` runs in check (RunInCheck), so a hole
// evaluating to a bare List carrier (`args` declares Returns List)
// lands in recordH non-concrete.
func TestSpecfixCheckModeGuards(t *testing.T) {
	const input = "refine Record (args)"
	values, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := specfixProbeRegistry(t)
	r.Source = input
	done := r.Check.Begin()
	_, runErr := core.NewTop(r).Run(values)
	done()
	if runErr == nil || !strings.Contains(runErr.Error(), "must be a concrete list, got type literal") {
		t.Errorf("want the record concrete-list guard, got %v", runErr)
	}
}

// TestSpecfixLoopDisjunctCapture drives the loop-capture models with a
// def-resolved CONCRETE Disjunct on the body stack (the `dd` binding):
// both loop forms type the residual as a typed list OF the disjunct.
func TestSpecfixLoopDisjunctCapture(t *testing.T) {
	for _, input := range []string{"for 2 [dd]", "for [2] [dd]"} {
		values, err := parser.Parse(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		r := specfixProbeRegistry(t)
		r.Source = input
		done := r.Check.Begin()
		stack, runErr := core.NewTop(r).Run(values)
		done()
		if runErr != nil {
			t.Fatalf("%q: %v", input, runErr)
		}
		got := specfix.RenderCheck(stack, r.Check.Diagnostics)
		t.Logf("%q => %q", input, got)
		if !strings.Contains(got, "Enum") {
			t.Errorf("%q: want an Enum-typed residual, got %q", input, got)
		}
	}
}

// TestSpecfixNilArgsStack pins the args/__pa error arms: the ArgsStack
// API contract errors only on a nil receiver (a registry not built by
// NewRegistry), so the probes run the words against exactly that state.
func TestSpecfixNilArgsStack(t *testing.T) {
	for _, input := range []string{"args", "__pa"} {
		t.Run(input, func(t *testing.T) {
			values, err := parser.Parse(input)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			r := specfixProbeRegistry(t)
			r.Args = nil
			_, runErr := core.NewTop(r).Run(values)
			if runErr == nil || !strings.Contains(runErr.Error(), "argsstack: nil stack") {
				t.Errorf("want the nil args-stack error, got %v", runErr)
			}
		})
	}
}
