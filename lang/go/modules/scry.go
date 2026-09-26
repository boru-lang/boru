package modules

import (
	"fmt"
	"sort"

	"github.com/boru-lang/boru/lang/go/native"
)

// BuildScryModule creates the "boru:scry" native module — a boru system's
// knowledge of itself, as plain data (design/BORU-SCRY.0.md). Scry is the
// canonical home of the seven self-knowledge words boru:debug shipped first
// (`words`, `defs`, `modules`, `sig`, `body`, `deps`, `shape`): both modules
// build them from ONE constructor (selfKnowledge), so the two surfaces cannot
// fork, and the debug copies are frozen and deprecated (NUR063). New
// self-knowledge lands here only.
//
//	import "boru:scry"
//	Scry.words                        # every dispatchable word
//	"add" Scry.sig                    # [{args returns} …]
//	[1 add 2 mul 3] Scry.deps         # ['add' 'mul']
func BuildScryModule(parent *native.Registry) (native.ModuleDesc, error) {
	sk := selfKnowledge{prefix: "scry", ns: "Scry", code: "scry_unknown_word"}
	natives := sk.natives()
	subReg, err := newModuleRegistry("boru:scry", natives)
	if err != nil {
		return native.ModuleDesc{}, err
	}
	exports := native.NewOrderedMap()
	for _, n := range natives {
		exports.Set(sk.exportName(n.Name), makeModuleFnDef(n, subReg))
	}
	return moduleDesc(parent, "Scry", subReg, exports), nil
}

// selfKnowledge builds the seven self-knowledge natives for one module
// surface: inner names `<prefix>-<word>`, errors naming `<ns>.<word>` (the
// word the user wrote) and raising code for an unknown word. The handlers are
// the same Go code on every surface — boru:scry's canonical ones and
// boru:debug's frozen copies (NUR063).
type selfKnowledge struct {
	prefix, ns, code string
}

func (s selfKnowledge) name(word string) string { return s.prefix + "-" + word }

func (s selfKnowledge) op(word string) string { return s.ns + "." + word }

// exportName strips the inner prefix off a native's name (scry-sig -> "sig").
func (s selfKnowledge) exportName(internal string) string {
	return internal[len(s.prefix)+1:]
}

// natives is the seven, in the census → per-word → value order of
// design/BORU-SCRY.0.md §4.
func (s selfKnowledge) natives() []native.NativeFunc {
	return []native.NativeFunc{s.words(), s.defs(), s.modules(), s.sig(), s.body(), s.deps(), s.shape()}
}

// deps: the distinct word names a quoted body references.
func (s selfKnowledge) deps() native.NativeFunc {
	return native.NativeFunc{
		Name: s.name("deps"),
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TList},
			Returns:    []*native.Type{native.TList},
			NoEvalArgs: map[int]bool{0: true},
			BarrierPos: -1,
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				body, err := native.RequireConcreteList(args[0], s.op("deps"))
				if err != nil {
					return nil, err
				}
				seen := map[string]bool{}
				var names []string
				native.WalkBodyWords(body.Slice(), func(w native.WordInfo, _ native.Value) {
					if w.Name == "" || seen[w.Name] {
						return
					}
					seen[w.Name] = true
					names = append(names, w.Name)
				})
				sort.Strings(names)
				return []native.Value{stringsToList(names)}, nil
			}),
		}},
	}
}

// words: every word actually dispatchable in this registry (live
// natives/host words + def-bound names) — not the static help catalog, which
// can list words that are documented but not registered (e.g. moved to an
// unimported module) and would fail with undefined_word.
func (s selfKnowledge) words() native.NativeFunc {
	return native.NativeFunc{
		Name: s.name("words"),
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				return []native.Value{stringsToList(r.RegisteredWordNames())}, nil
			}),
		}},
	}
}

// defs: current def-bound names mapped to their active top binding.
func (s selfKnowledge) defs() native.NativeFunc {
	return native.NativeFunc{
		Name: s.name("defs"),
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				names := append([]string(nil), r.Defs.Names()...)
				sort.Strings(names)
				om := native.NewOrderedMap()
				for _, name := range names {
					if v, ok := r.Defs.Top(name); ok {
						om.Set(name, v)
					}
				}
				return []native.Value{native.NewMap(om)}, nil
			}),
		}},
	}
}

// modules: the native modules available to import.
func (s selfKnowledge) modules() native.NativeFunc {
	return native.NativeFunc{
		Name: s.name("modules"),
		Signatures: []native.Signature{{
			Args:       []*native.Type{},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl: native.Go(func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				names := moduleNamesFn()
				sort.Strings(names)
				out := make([]string, len(names))
				for i, n := range names {
					out[i] = "boru:" + n
				}
				return []native.Value{stringsToList(out)}, nil
			}),
		}},
	}
}

// shape: a structural census of a value — counts by kind, depth, node count.
func (s selfKnowledge) shape() native.NativeFunc {
	return native.NativeFunc{
		Name: s.name("shape"),
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TAny},
			Returns:    []*native.Type{native.TMap},
			BarrierPos: -1,
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				var c shapeCensus
				c.walk(args[0], 0)
				om := native.NewOrderedMap()
				om.Set("nodes", native.NewInteger(int64(c.nodes)))
				om.Set("lists", native.NewInteger(int64(c.lists)))
				om.Set("maps", native.NewInteger(int64(c.maps)))
				om.Set("strings", native.NewInteger(int64(c.strings)))
				om.Set("scalars", native.NewInteger(int64(c.scalars)))
				om.Set("max-depth", native.NewInteger(int64(c.maxDepth)))
				return []native.Value{native.NewMap(om)}, nil
			}),
		}},
	}
}

// sig: the signatures of a word, as structured data.
func (s selfKnowledge) sig() native.NativeFunc {
	return native.NativeFunc{
		Name: s.name("sig"),
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString},
			Returns:    []*native.Type{native.TList},
			BarrierPos: -1,
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				name, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				fn := r.Lookup(name)
				if fn == nil {
					return nil, r.BoruError(s.code, fmt.Sprintf("%s: no such word %q", s.op("sig"), name), s.op("sig"))
				}
				var sigs []native.Value
				for _, sig := range fn.Signatures {
					sm := native.NewOrderedMap()
					sm.Set("args", typeLeavesToList(sig.ArgTypes()))
					sm.Set("returns", typeLeavesToList(sig.Returns))
					sigs = append(sigs, native.NewMap(sm))
				}
				return []native.Value{native.NewList(sigs)}, nil
			}),
		}},
	}
}

// body: the quoted body of a boru-defined word; `native/q` for a host word.
func (s selfKnowledge) body() native.NativeFunc {
	return native.NativeFunc{
		Name: s.name("body"),
		Signatures: []native.Signature{{
			Args:       []*native.Type{native.TString},
			Returns:    []*native.Type{native.TAny},
			BarrierPos: -1,
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				name, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				fn := r.Lookup(name)
				if fn == nil {
					return nil, r.BoruError(s.code, fmt.Sprintf("%s: no such word %q", s.op("body"), name), s.op("body"))
				}
				for _, sig := range fn.OwnSigs() {
					if len(sig.Body()) > 0 {
						body := native.NewList(append([]native.Value(nil), sig.Body()...))
						body.Quoted = true
						return []native.Value{body}, nil
					}
				}
				return []native.Value{native.NewAtom("native")}, nil
			}),
		}},
	}
}
