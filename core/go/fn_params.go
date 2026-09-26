package core

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file owns the canonical fn-signature parser. Both the bare
// borueng `fn` (in core_words.go) and the production boru `def`/`fn`
// in lang/go/engine/native_definition_fn.go call into these
// functions. Do NOT duplicate the parser logic anywhere else; the
// optional-arg `?` rule, the barrier `|` rule, and the type-name
// resolution rule must have a single source of truth.
//
// Public surface:
//
//   ParseFnParams(r, inputSig)  ([]FnParam, int, error)
//      — walks an `[ p1 p2 … ]` list and returns the FnParams plus
//        the BarrierPos (`|` position).
//   ParseFnReturns(r, outputSig)   ([]*Type, error)
//      — walks an `[ T1 T2 … ]` return-type list (or a single type
//        value) and returns the Types.
//   ResolveSigType(r, v)        (*Type, *Value, error)
//      — converts a Value (from a param's type slot) into a *Type
//        plus an optional pattern Value for structural matching.
//   ResolveTypeName(name)       (*Type, error)
//      — maps a bare type-name string to its *Type. Defers to NewType
//        for non-builtin paths.
//   LookupDefType(r, name)      *Value
//      — resolves a name to a type-body value via the type stack
//        first, then the def stack. Returns nil if neither layer
//        carries a type-body for that name.
//   ResolveDefType(v)           (*Type, *Value, error)
//      — converts a def'd type value (record, options, plain type
//        literal) into a sig type + pattern.
//
// All five functions are byte-identical ports of the production
// helpers that previously lived in lang/go/engine/native_definition_fn.go.

// ParseFnParams extracts parameters from an input signature list.
//
// The input is a List containing some mix of:
//   - Bare type-name Words: `Integer`, `String`, …
//   - *Type-literal values
//   - Implicit-pair maps `{name:*Type}` — the standard typed-param
//     form. The name may have a trailing `?` to mark the param
//     optional, and the type slot may be a paren expression that
//     evaluates to a type or a disjunct that includes None (which
//     is also auto-treated as optional).
//   - Concrete-value patterns (Integer / Boolean / String literals)
//     that anchor the param to that exact value.
//   - /q params — `name:Atom/q` (or bare `Atom/q` for unnamed): the
//     slot captures an upcoming bare Word as its Atom name during
//     collection, exactly like a native QuoteArgs slot, binding-
//     agnostic. The /q word suffix parses the type name to a concrete
//     atom, so an atom in type position IS the declaration; an atom
//     that names no type is an invalid parameter (the value-pattern
//     space stays reserved).
//   - The Word `?` — marks the PRECEDING param as optional. This
//     is the canonical post-name optionality marker.
//   - The Word `|` — sets the BarrierPos to the current param count.
//
// Returns the FnParam list, the BarrierPos, or a parse error.
func ParseFnParams(r *Registry, inputSig Value) ([]FnParam, int, error) {
	if !inputSig.Parent.Equal(TList) {
		return nil, 0, fmt.Errorf("function spec: input signature must be a list")
	}
	if !IsConcrete(inputSig) {
		return nil, 0, fmt.Errorf("function spec: input signature must be a concrete list, got type literal")
	}
	elems, _ := AsList(inputSig)
	var params []FnParam
	// -1 is the "no barrier seen" sentinel — consumers default
	// it to len(params) so boru fns without a `|` are all-forward,
	// matching the convention native registrations follow. A
	// leading `|` overwrites with 0 (explicit all-stack).
	barrierPos := -1

	for i := 0; i < elems.Len(); i++ {
		elem := elems.Get(i)

		_as1, _ := AsWord(elem)
		if IsWord(elem) && _as1.Name == "?" {
			if len(params) > 0 {
				params[len(params)-1].Optional = true
			}
			continue
		}

		// Bare `T/q` — an unnamed /q param. The /q word suffix parses
		// `Atom/q` to the concrete atom "Atom"; an atom in sig position
		// whose name is a type therefore IS the quote declaration: the
		// slot captures an upcoming bare Word as its Atom name during
		// collection (presented as if quoted), like a native QuoteArgs
		// slot. An atom that does NOT name a type is a KEYWORD slot —
		// it matches exactly the literal word `tn` (see keywordParam and
		// patternsOk's keyword branch). DepScalars are excluded (a
		// dependent atom type like `(Atom gt foo/q)` shares Parent=TAtom
		// but is a predicate constraint, handled by ResolveSigType's
		// pattern path).
		if elem.Parent.Equal(TAtom) && IsConcrete(elem) && !elem.IsDepScalar() {
			if tn, aerr := AsAtom(elem); aerr == nil {
				if qt, qerr := lookupTypeNameInRegistry(r, tn); qerr == nil {
					params = append(params, FnParam{Type: qt, Quote: true})
					continue
				}
				params = append(params, keywordParam("", tn))
				continue
			}
		}

		_as2, _ := AsWord(elem)
		if IsWord(elem) && (_as2.Name == "|" || _as2.Name == "__SB") {
			// `|` is the canonical barrier marker; `__SB` (Stack
			// Barrier) is its alias for environments where a bare
			// `|` is awkward to type (shell pipelines, etc.).
			barrierPos = len(params)
			continue
		}

		switch {
		case elem.Parent.Equal(TMap) && elem.Data != nil:
			m, err := AsMutableMap(elem)
			if err == nil && m != nil && m.Implicit {
				keys := m.Keys()
				if len(keys) != 1 {
					return nil, 0, fmt.Errorf("function spec: parameter map must have exactly one key")
				}
				name := keys[0]
				optional := false
				if strings.HasSuffix(name, "?") {
					name = strings.TrimSuffix(name, "?")
					optional = true
				}
				rawType, _ := m.Get(keys[0])
				typeVal, terr := EvalSigTypeExpr(r, rawType, name)
				if terr != nil {
					return nil, 0, terr
				}
				if IsDisjunct(typeVal) {
					_as3, _ := AsDisjunct(typeVal)
					alts := _as3.Alternatives
					for _, alt := range alts {
						if IsNoneShape(alt) {
							optional = true
							break
						}
					}
					if optional {
						for _, alt := range alts {
							if !IsNoneShape(alt) {
								typeVal = alt
								break
							}
						}
					}
				}
				// `name:T/q` — the named /q param. The /q suffix turns
				// the type word into a concrete atom, the only surface
				// that puts a plain atom in the pair's type slot (a
				// dependent atom type like `(Atom gt foo/q)` shares
				// Parent=TAtom but is a DepScalar predicate — excluded,
				// it takes ResolveSigType's pattern path). See the
				// unnamed `T/q` branch above for the capture semantics.
				if typeVal.Parent.Equal(TAtom) && IsConcrete(typeVal) && !typeVal.IsDepScalar() {
					tn, aerr := AsAtom(typeVal)
					if aerr == nil {
						if err := ValidateWordName(name); err != nil {
							return nil, 0, fmt.Errorf("function spec: %w", err)
						}
						qt, qerr := lookupTypeNameInRegistry(r, tn)
						if qerr != nil {
							// `name:kw/q` where kw names no type — a KEYWORD
							// slot bound to `name` (see keywordParam).
							kp := keywordParam(name, tn)
							kp.Optional = optional
							params = append(params, kp)
							continue
						}
						params = append(params, FnParam{Name: name, Type: qt, Optional: optional, Quote: true})
						continue
					}
				}
				paramType, pattern, err := ResolveSigType(r, typeVal)
				if err != nil {
					return nil, 0, fmt.Errorf("function spec: invalid type for %q: %w", name, err)
				}
				if err := ValidateWordName(name); err != nil {
					return nil, 0, fmt.Errorf("function spec: %w", err)
				}
				params = append(params, FnParam{Name: name, Type: paramType, Pattern: pattern, Optional: optional})
			} else {
				paramType, pattern, err := ResolveSigType(r, elem)
				if err != nil {
					return nil, 0, fmt.Errorf("function spec: invalid map param: %w", err)
				}
				params = append(params, FnParam{Type: paramType, Pattern: pattern})
			}

		case IsWord(elem):
			_as4, _ := AsWord(elem)
			name := _as4.Name
			// `name:*Type` colon-delimited form. Used by minimal
			// tokenizers (e.g. the borueng spec runner, whose
			// whitespace-only lexer produces a single Word for
			// `n:Integer`). Production parsers using jsonic produce
			// the `{name:*Type}` implicit-map form instead, handled
			// in the TMap case above. Either form is accepted here
			// so a single ParseFnParams serves both consumers.
			if idx := strings.Index(name, ":"); idx > 0 {
				paramName := name[:idx]
				typeName := name[idx+1:]
				optional := false
				if strings.HasSuffix(paramName, "?") {
					paramName = strings.TrimSuffix(paramName, "?")
					optional = true
				}
				// `name:T/q` in the single-Word colon form (minimal
				// tokenizers) — same /q param declaration as the
				// implicit-pair form above.
				quote := false
				if strings.HasSuffix(typeName, "/q") {
					typeName = strings.TrimSuffix(typeName, "/q")
					quote = true
				}
				if err := ValidateWordName(paramName); err != nil {
					return nil, 0, fmt.Errorf("function spec: %w", err)
				}
				paramType, err := lookupTypeNameInRegistry(r, typeName)
				if err != nil {
					// `name:kw/q` where kw names no type — a KEYWORD slot.
					if quote {
						kp := keywordParam(paramName, typeName)
						kp.Optional = optional
						params = append(params, kp)
						continue
					}
					return nil, 0, fmt.Errorf("function spec: invalid type %q: %w", typeName, err)
				}
				params = append(params, FnParam{Name: paramName, Type: paramType, Optional: optional, Quote: quote})
				continue
			}
			// Bare type-name Word: unnamed positional param.
			paramType, err := lookupTypeNameInRegistry(r, name)
			if err != nil {
				return nil, 0, fmt.Errorf("function spec: invalid type %q: %w", name, err)
			}
			params = append(params, FnParam{Type: paramType})

		case IsBareTypeNode(elem):
			elemType := elem
			params = append(params, FnParam{Type: &elemType})

		case elem.Parent.ConformsTo(TInteger):
			pat := elem
			params = append(params, FnParam{Type: TInteger, Pattern: &pat})

		case elem.Parent.ConformsTo(TBoolean):
			pat := elem
			params = append(params, FnParam{Type: TBoolean, Pattern: &pat})

		case elem.Parent.ConformsTo(TString):
			pat := elem
			params = append(params, FnParam{Type: TString, Pattern: &pat})

		case IsReach(elem):
			// Dotted TYPE annotation: `mat:MatrixUtil.Matrix` (the implicit
			// pair folds into the Reach's receiver) or a bare unnamed
			// `MatrixUtil.Matrix`. Resolved at fn-construction time against
			// the live registry — the module export must already be bound.
			if p, ok := dottedParamType(r, elem); ok {
				params = append(params, p)
				continue
			}
			return nil, 0, fmt.Errorf("function spec: invalid parameter: %s (a dotted annotation must reach a type literal through bound module exports)", elem.String())

		default:
			return nil, 0, fmt.Errorf("function spec: invalid parameter: %s", elem.String())
		}
	}

	return params, barrierPos, nil
}

// dottedParamType resolves a Reach-shaped param spec — `mat:Pkg.Type` /
// `Pkg.Type` — to its FnParam. The receiver is either a WORD (unnamed
// positional param) or the implicit single-pair map `{name: word(Pkg)}`
// (named param); the literal dot segments then walk from the word's
// binding via ApplyReach (the same Stage-1 lowering a bare `Pkg.Type`
// expression uses, so module-namespace transparency and getr strictness are
// identical). The reached value must be a bare type literal.
func dottedParamType(r *Registry, elem Value) (FnParam, bool) {
	if r == nil {
		return FnParam{}, false
	}
	info, err := AsReach(elem)
	if err != nil || len(info.Receiver) != 1 || len(info.Segments) == 0 {
		return FnParam{}, false
	}
	for _, seg := range info.Segments {
		if seg.Computed {
			return FnParam{}, false
		}
	}
	paramName := ""
	baseWord := ""
	recv := info.Receiver[0]
	switch {
	case IsWord(recv):
		w, wErr := AsWord(recv)
		if wErr != nil {
			return FnParam{}, false
		}
		baseWord = w.Name
	case recv.Parent != nil && recv.Parent.Equal(TMap) && IsConcrete(recv):
		m, mErr := AsMap(recv)
		if mErr != nil || m.Len() != 1 {
			return FnParam{}, false
		}
		key := m.Keys()[0]
		val, _ := m.Get(key)
		if !IsWord(val) {
			return FnParam{}, false
		}
		w, wErr := AsWord(val)
		if wErr != nil {
			return FnParam{}, false
		}
		paramName, baseWord = key, w.Name
		if err := ValidateWordName(paramName); err != nil {
			return FnParam{}, false
		}
	default:
		return FnParam{}, false
	}
	base, ok := r.Defs.Top(baseWord)
	if !ok {
		return FnParam{}, false
	}
	// unit rides along: dropping it would rebuild the lens cache per call.
	reached, rErr := ApplyReach(r, ReachInfo{Segments: info.Segments, unit: info.unit}, base)
	if rErr != nil || !IsTypeLiteral(reached) {
		return FnParam{}, false
	}
	t := CanonicalType(r, &reached)
	return FnParam{Name: paramName, Type: t}, true
}

// QuoteArgsFromParams derives the Signature.QuoteArgs map from /q-marked
// FnParams (`name:Atom/q`). Single derivation point for FnSig
// constructors that don't pass through InstallFnDef's normalizeSig —
// anonymous fns (afn) and bare fn literals dispatch from their authored
// FnSig directly, so the dispatch-side field must be populated at
// construction. Returns nil when no param is /q-marked.
func QuoteArgsFromParams(params []FnParam) map[int]bool {
	var qa map[int]bool
	for i, p := range params {
		if p.Quote {
			if qa == nil {
				qa = make(map[int]bool)
			}
			qa[i] = true
		}
	}
	return qa
}

// PatternsFromParams collects the per-position value patterns from a
// param list into the Signature.Patterns shape. The afn/lambda path
// builds its FnSig directly (no normalizeSig), so it needs this to
// carry a keyword slot's Atom pattern (or any value pattern) onto the
// dispatch-side sig; the `fn`/`def` path gets it for free from
// normalizeSig.
func PatternsFromParams(params []FnParam) map[int]Value {
	var pats map[int]Value
	for i, p := range params {
		if p.Pattern != nil {
			if pats == nil {
				pats = make(map[int]Value)
			}
			pats[i] = *p.Pattern
		}
	}
	return pats
}

// keywordParam builds a KEYWORD-slot param: a /q slot whose concrete
// Atom pattern (`kw`) admits exactly the literal word `kw` at dispatch
// (patternsOk's keyword branch, eng/go/match.go). It is the boru-source
// spelling of the def constructor forms' keyword slots — an atom `kw/q`
// (or `name:kw/q`) whose name is NOT a type. Binding-agnostic like every
// /q slot: the captured value is the Atom `kw`, bound to `name` when
// named.
func keywordParam(name, kw string) FnParam {
	pat := NewAtom(kw)
	return FnParam{Name: name, Type: TAtom, Quote: true, Pattern: &pat}
}

// ParseFnReturns extracts return types from an output signature.
// The output may be a list of types/values or a single type/value.
// The second result is the positional return PATTERNS — nil when no
// declared return carries one. ResolveSigType already computes them; they
// used to be discarded here, which is what let a declared union return go
// unenforced (its *Type degrades to Any, so the pattern IS the contract).
func ParseFnReturns(r *Registry, outputSig Value) ([]*Type, []*Value, error) {
	if !outputSig.Parent.Equal(TList) || !IsConcrete(outputSig) {
		sig, err := resolveReturnSlot(r, outputSig, 0)
		if err != nil {
			return nil, nil, err
		}
		t, pat, err := ResolveSigType(r, sig)
		if err != nil {
			return nil, nil, err
		}
		if pat == nil {
			return []*Type{t}, nil, nil
		}
		return []*Type{t}, []*Value{pat}, nil
	}
	elems, _ := AsList(outputSig)
	if elems.Len() == 0 {
		return nil, nil, nil
	}
	types := make([]*Type, elems.Len())
	var pats []*Value
	for i, e := range elems.Slice() {
		sig, err := resolveReturnSlot(r, e, i)
		if err != nil {
			return nil, nil, err
		}
		var pat *Value
		types[i], pat, err = ResolveSigType(r, sig)
		if err != nil {
			return nil, nil, err
		}
		if pat != nil {
			if pats == nil {
				pats = make([]*Value, elems.Len())
			}
			pats[i] = pat
		}
	}
	return types, pats, nil
}

// EvalSigTypeExpr reduces the VALUE side of a type annotation to the
// value ResolveSigType can read, and is the single place either
// signature slot does it.
//
// Two shapes need reducing before resolution:
//
//   - A sugar-marker annotation (`t:Map/t` — the type-bound sugar)
//     lowers to its ParenExpr expansion first (ADR-012 rule 3
//     amendment), then evaluates exactly as the pre-marker parser
//     output did.
//   - A parenthesised annotation (`x:(Integer tor String)`) arrives as
//     a raw ParenExpr and must be RUN to become the disjunct value the
//     resolver understands.
//
// A failed or multi-valued paren annotation is a def-time ERROR.
// Keeping the raw ParenExpr instead falls to ResolveSigType's TAny
// tail, which is a silent wildcard: `x:(Map|List)` accepted everything
// because `|` is not a word. That failure mode is why this is shared —
// ParseFnReturns skipped the step entirely, so a declared union return
// (`y:(Integer tor String)`) degraded to Any and enforced nothing,
// while the identical annotation on a param was enforced.
//
// what names the slot in the error, e.g. the param or return name.
func EvalSigTypeExpr(r *Registry, typeVal Value, what string) (Value, error) {
	if sinfo, sok := AsSugar(typeVal); sok && r != nil {
		if exp, serr := SugarExpansion(r, sinfo, typeVal, false); serr == nil && len(exp) == 1 {
			typeVal = exp[0]
		}
	}
	if !IsParenExpr(typeVal) || r == nil {
		return typeVal, nil
	}
	items, _ := AsParenExpr(typeVal)
	sub := New(r)
	input := make([]Value, 0, len(items)+2)
	input = append(input, NewOpenParen())
	input = append(input, items...)
	input = append(input, NewCloseParen())
	result, rerr := sub.Run(input)
	if rerr != nil {
		return Value{}, fmt.Errorf("function spec: invalid type for %q: %w", what, rerr)
	}
	if len(result) != 1 {
		return Value{}, fmt.Errorf("function spec: type annotation for %q must produce one type, got %d values", what, len(result))
	}
	return result[0], nil
}

// unwrapNamedReturn resolves the `name:Type` spelling of a RETURN
// declaration to the type on the pair's value side. Pair syntax lowers a
// bare `i:Integer` to a single-entry map flagged Implicit
// (parser/go/parse.go), so `fn [s:String i:Integer [body]]` hands this
// slot the same shape `ParseFnParams` receives on the input side — and
// the two slots must read it the same way, because
// `fn [x:A y:B [body]]` IS `fn [[x:A] [y:B] [body]]`. The name is
// documentation: FnSig has no named-return concept and nothing
// downstream reads it, but accepting the spelling is what keeps the two
// slots symmetric.
//
// An EXPLICIT map (`{i: Integer}`) is left alone — it declares a
// Map-typed return, the same split ParseFnParams draws on Implicit for
// a Map-typed param.
func unwrapNamedReturn(r *Registry, v Value) (Value, error) {
	// The single-Word colon form. A minimal tokenizer (the borueng and
	// checker spec runners, whose whitespace-only lexers produce one
	// Word for `i:Integer`) never builds the implicit map, so without
	// this arm the promised input/output symmetry held only for
	// consumers of the production parser: ParseFnParams accepts
	// `[y:Integer]` as a param from either tokenizer, and the output
	// slot would have rejected the same token as a type name.
	if IsWord(v) {
		w, _ := AsWord(v)
		idx := strings.Index(w.Name, ":")
		if idx <= 0 {
			return v, nil
		}
		if err := ValidateWordName(w.Name[:idx]); err != nil {
			return Value{}, fmt.Errorf("function spec: %w", err)
		}
		inner := NewWord(w.Name[idx+1:])
		inner.SetPos(v.Pos())
		return inner, nil
	}
	if !v.Parent.Equal(TMap) || v.Data == nil {
		return v, nil
	}
	m, err := AsMutableMap(v)
	if err != nil || m == nil || !m.Implicit {
		return v, nil
	}
	keys := m.Keys()
	if len(keys) != 1 {
		return Value{}, fmt.Errorf("function spec: return map must have exactly one key")
	}
	if err := ValidateWordName(keys[0]); err != nil {
		return Value{}, fmt.Errorf("function spec: %w", err)
	}
	inner, _ := m.Get(keys[0])
	return inner, nil
}

// resolveReturnSlot reduces one declared return to the value
// ResolveSigType reads: the `name:Type` unwrap, then the annotation
// reduction EvalSigTypeExpr performs.
//
// Both steps apply to BOTH spellings, which is the whole point of doing
// them here rather than inside the unwrap. A parenthesised annotation
// has to be run whether or not the return is named — `[(Integer tor
// String)]` and `[y:(Integer tor String)]` are one declaration written
// two ways, and reducing only the named one left the unnamed spelling
// falling to ResolveSigType's TAny tail, enforcing nothing. That split
// is the exact defect this change exists to remove, so it must not be
// reintroduced one slot down.
func resolveReturnSlot(r *Registry, v Value, i int) (Value, error) {
	sig, err := unwrapNamedReturn(r, v)
	if err != nil {
		return Value{}, err
	}
	return EvalSigTypeExpr(r, sig, "return value "+strconv.Itoa(i+1))
}

// looksLikeTypeName reports whether name has the SHAPE of a type name:
// every `/`-separated part starting with an uppercase letter, the rule
// NewType enforces. It is how ResolveSigType tells a misspelled type
// name from a literal string — see the call site.
func looksLikeTypeName(name string) bool {
	if name == "" {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		r, _ := utf8.DecodeRuneInString(part)
		if !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// ResolveSigType converts a Value (from a pair's value side) to a *Type
// plus an optional pattern Value for structural matching.
// noteRuntimeSigType — a typed container's child holding a refinement over
// a bound the analysis pass does not know (`xs:[:(Integer gt (size s))]`):
// the child is no node a forward could stand in for, and a compiled unit
// would carry the pass's placeholder for the bound, so the word building
// the signature declines as the compile-time word it is (NUR231). A
// refinement or union slot compiles (runSigPattern), as a named type does.
func noteRuntimeSigType(r *Registry, v Value) {
	if r != nil && r.analysisActive() && HasUnknownRefinement(v) {
		r.analysisRecorder().NoteRuntimeDependent()
	}
}

func ResolveSigType(r *Registry, v Value) (*Type, *Value, error) {
	if IsBareTypeNode(v) {
		return ValueType(v), nil, nil
	}
	// Word, String, or Atom: extract the name and resolve.
	// LookupDefType already returns the canonical lattice node for
	// bare type literals, and ResolveDefType canonicalizes its own
	// Data==nil result via CanonicalType, so the resolved *Type is
	// identity-stable at every hop.
	//
	// DepScalars are excluded: a String/Atom DepScalar has
	// `Parent.ConformsTo(TString)` true but its payload is
	// `DepScalarInfo`, not `StrPayload` — `AsString(v)` would fail
	// silently, name="" would then fail the kernel-name lookup. The
	// scalar-pattern branch below catches DepScalars correctly
	// (kind = the base type, pattern = the DepScalar Value).
	if (IsWord(v) || v.Parent.ConformsTo(TString) || v.Parent.ConformsTo(TAtom)) && !v.IsDepScalar() {
		var name string
		switch {
		case IsWord(v):
			w, _ := AsWord(v)
			name = w.Name
		case v.Parent.ConformsTo(TAtom):
			// An Atom carries AtomPayload, not StrPayload, so AsString
			// answers ("", err) here and every lookup below then misses
			// on the empty name. That was invisible while an
			// unresolvable name was a hard error — the Atom arm errored
			// either way — but the literal fallback added below turns a
			// missed lookup into a literal, which would have made
			// `Integer/q` an Atom pattern instead of TInteger.
			name, _ = AsAtom(v)
		default:
			name, _ = AsString(v)
		}
		// Authoritative path: `r.LookupTypeName(name)` consults
		// `DefEntry.TypeDef` — the dedicated *Type field that's set
		// non-nil exactly when a name was installed as a type binding
		// (capitalised `def`). When that field is set, the lattice
		// node it points at IS the sig type, regardless of what the
		// body's payload looks like. This works uniformly for every
		// type kind: Object, Table, predicate fn, Disjunct, DepScalar,
		// TypedList, TypedMap, bare nominal subtypes, builtins.
		//
		// Exceptions: Record and Options bodies need a structural
		// Pattern (field map / options map) attached to the sig so
		// the dispatcher checks shape, not just lattice membership.
		// For those, fall through to ResolveDefType, which returns
		// the appropriate TMap + pattern pair.
		//
		// Before this refactor, the dispatcher inferred "is this a
		// type?" from body-payload shape — `Data == nil` (lattice-
		// only body) / `IsRecordType` / `IsOptionsType` / TAny
		// fallthrough. ObjectType bodies (and several others) hit the
		// TAny fallthrough silently, turning `[f:Foo]` into a
		// wildcard. The fix consults the explicit TypeDef flag first
		// so the meaning is unambiguous; body-payload inspection is
		// now confined to the two cases that genuinely need it.
		if r != nil {
			if def := r.LookupTypeName(name); def != nil {
				if body, ok := r.TopTypeBody(name); ok {
					if IsRecordType(body) || IsOptionsType(body) {
						return ResolveDefType(r, body)
					}
				}
				return def, nil, nil
			}
		}
		if defVal := LookupDefType(r, name); defVal != nil {
			return ResolveDefType(r, *defVal)
		}
		t, err := ResolveTypeName(name)
		if err == nil {
			return t, nil, nil
		}
		// A String or Atom that could not NAME a type is a literal VALUE,
		// and a value is a type (ADR-010): it constrains the slot to
		// itself, exactly as the Integer/Float/Boolean literals do in the
		// scalar branch below — which `'ok'` and `red/q` could never reach
		// while this returned the name-resolution error instead.
		//
		// SHAPE decides, not mere unboundness. `'Integr'` is a misspelled
		// `Integer` and has to stay a loud unknown-type error; `'ok'`
		// could never be a type name, so it is the string itself. And a
		// WORD is always a name: a bare lowercase word in a type slot is
		// a typo, not a literal (eng/go/standalone_tail_test.go pins it).
		if IsWord(v) || looksLikeTypeName(name) {
			return nil, nil, err
		}
	}
	// Type VALUES arriving directly rather than by name — the paren
	// annotation path (`x:(Box of [Integer])`) delivers the
	// instantiation body itself. Without these branches they fell to
	// the TAny tail: a silent wildcard (the same degradation the
	// TypeDef refactor closed for NAMED types).
	if IsClassType(v) {
		if oi, aerr := AsClassType(v); aerr == nil && oi.Type != nil {
			return CanonicalType(r, oi.Type), nil, nil
		}
	}
	if IsSurfaceType(v) {
		if si, aerr := AsSurfaceType(v); aerr == nil && si.Type != nil {
			return CanonicalType(r, si.Type), nil, nil
		}
	}
	if IsTypeSchema(v) {
		// A bare schema as a param type means "any instantiation"
		// (family membership by ancestry — D3 in GENERICS.10.md).
		if ti, aerr := AsTypeSchema(v); aerr == nil && ti.Type != nil {
			return CanonicalType(r, ti.Type), nil, nil
		}
	}
	// Bounded Type — `x:Map/t` ≡ `x:(Type of [Map])`: the slot takes
	// TYPE LITERALS conforming to the bound. Slot type TType admits
	// types (typeMembershipBehavior); the body rides as the Pattern so
	// both dispatch paths enforce the bound through Unify.
	if IsBoundedType(v) {
		pattern := v
		return TType, &pattern, nil
	}
	// Inline disjunct — `x:(Integer tor String)`: constrain through the
	// pattern path (Unify's disjunct fold), exactly like the named form
	// constrains through its minted Behavior. Previously this fell to
	// the TAny tail: a silent wildcard that dispatched EVERYTHING.
	if IsDisjunct(v) {
		if p, ok := runSigPattern(r, v); ok {
			return TAny, &p, nil
		}
		pattern := v
		return TAny, &pattern, nil
	}
	if IsRecordType(v) {
		return ResolveDefType(r, v)
	}
	// An inline refinement (`n:(Integer gt 0)`, `b:(Bytes gt …)`): the slot
	// is its base — the refinement's Parent, whichever type declared itself a
	// base (DeclareRefinementBase) — and the refinement rides as the pattern.
	// The literal arm below hand-lists five bases, and a Bytes refinement fell
	// to the TAny tail: a wildcard slot (NUR009).
	if v.IsDepScalar() {
		if p, ok := runSigPattern(r, v); ok {
			return v.Parent, &p, nil
		}
		pattern := v
		return v.Parent, &pattern, nil
	}
	if v.Data != nil && (v.Parent.ConformsTo(TInteger) ||
		v.Parent.ConformsTo(TFloat) ||
		v.Parent.ConformsTo(TBoolean) ||
		v.Parent.ConformsTo(TString) ||
		v.Parent.ConformsTo(TAtom)) {
		pattern := v
		var kind *Type
		switch {
		case v.Parent.ConformsTo(TInteger):
			kind = TInteger
		case v.Parent.ConformsTo(TFloat):
			kind = TFloat
		case v.Parent.ConformsTo(TBoolean):
			kind = TBoolean
		case v.Parent.ConformsTo(TString):
			kind = TString
		default:
			kind = TAtom
		}
		return kind, &pattern, nil
	}
	if v.Parent.Equal(TMap) {
		if IsTypedMap(v) {
			resolved, err := ResolveChildTypeExpr(r, v)
			if err != nil {
				return nil, nil, err
			}
			resolved = ResolveSigChildParam(r, resolved)
			noteRuntimeSigType(r, resolved)
			return TMap, &resolved, nil
		}
		// An INLINE record pattern (`o:{pretty:Boolean}`) resolves its
		// field type words here, where the named spelling's `record`
		// dispatch already resolved them (ResolveSigRecordFields).
		resolved := ResolveSigRecordFields(r, v)
		return TMap, &resolved, nil
	}
	if v.Parent.Equal(TList) {
		if IsTypedList(v) {
			// A ParenExpr child (`xs:[:(Pair of [String Integer])]`)
			// evaluates now; a child Word naming a live gen placeholder
			// (`xs:[:T]`) resolves to the placeholder literal NOW — the
			// binding pops when the def completes, so dispatch-time
			// resolution is impossible (see ResolveSigChildParam).
			resolved, err := ResolveChildTypeExpr(r, v)
			if err != nil {
				return nil, nil, err
			}
			resolved = ResolveSigChildParam(r, resolved)
			noteRuntimeSigType(r, resolved)
			return TList, &resolved, nil
		}
		return TList, &v, nil
	}
	return TAny, nil, nil
}

// LookupDefType resolves a name to its type value via the type stack
// first, then the def stack. Returns nil if neither carries a
// type-body for that name. For a bare type-literal body, returns
// the canonical lattice node (via CanonicalType) so downstream
// identity-sensitive uses (behave installs, sig dispatch) reach the
// same *Type the kernel mints.
func LookupDefType(r *Registry, name string) *Value {
	if r == nil {
		return nil
	}
	if tv, ok := r.TopTypeBody(name); ok {
		if IsTypeBody(tv) {
			if IsBareTypeNode(tv) {
				return CanonicalType(r, &tv)
			}
			return &tv
		}
	}
	val, ok := r.Defs.Top(name)
	if !ok {
		return nil
	}
	if !IsTypeBody(val) {
		return nil
	}
	if IsBareTypeNode(val) {
		return CanonicalType(r, &val)
	}
	return &val
}

// ResolveDefType converts a def'd type value (record, options, plain
// type literal) into a sig type + pattern. For a bare type literal,
// the *Type is canonicalized via CanonicalType so identity-sensitive
// downstream consumers (sig dispatch, behave installs) land on the
// lattice's canonical pointer.
func ResolveDefType(r *Registry, v Value) (*Type, *Value, error) {
	if IsRecordType(v) {
		rt, _ := AsRecordType(v)
		pat := NewMap(rt.Fields)
		return TMap, &pat, nil
	}
	if IsOptionsType(v) {
		_as6, _ := AsOptionsType(v)
		pat := NewOptionsType(_as6.Fields)
		return TMap, &pat, nil
	}
	if IsBareTypeNode(v) {
		// A bare type literal IS its lattice node post type/value
		// merge — the denoted type is the value itself, not its
		// supertype Parent. Route through CanonicalType so user-
		// minted refine subtypes (`:Foo`) keep canonical identity
		// in fn signatures.
		return CanonicalType(r, &v), nil, nil
	}
	// Note: most populated-body cases (Object/Table/Disjunct/
	// DepScalar/TypedList/TypedMap) are now handled at the
	// ResolveSigType caller, which consults DefEntry.TypeDef (the
	// explicit "this is a type" flag) via r.LookupTypeName and uses
	// the lattice node directly. ResolveDefType is reached only via
	// the name-based path when the caller actively wants body-payload
	// inspection — primarily for Record and Options, which need
	// structural patterns attached. Anything else falling through
	// here is either a value masquerading as a type body or a shape
	// that does not contribute a usable dispatch type.
	return TAny, nil, nil
}

// ResolveTypeName maps a type name string to its engine *Type.
// Special-cases the well-known names; falls back to NewType for any
// other slash-separated path.
// lookupTypeNameInRegistry resolves a type-name string against the
// kernel ResolveTypeName table first, falling back to the registry's
// dynamic type stack so user-defined types (`type Person object {…}`)
// are visible to fn-parameter validation. Without this fallback,
// `fn [[Person Person] [Integer] [body]]` would error at parse time
// because the kernel name table only knows builtins.
func lookupTypeNameInRegistry(r *Registry, name string) (*Type, error) {
	if t, err := ResolveTypeName(name); err == nil {
		return t, nil
	}
	if r != nil {
		if def := r.LookupTypeName(name); def != nil {
			return def, nil
		}
	}
	return nil, fmt.Errorf("boru: unknown type %q", name)
}

func ResolveTypeName(name string) (*Type, error) {
	switch name {
	case "Any":
		return TAny, nil
	case "None":
		return TNone, nil
	case "Never":
		return TNever, nil
	case "Number":
		return TNumber, nil
	case "Integer":
		return TInteger, nil
	case "Float":
		return TFloat, nil
	case "String":
		return TString, nil
	case "Boolean":
		return TBoolean, nil
	case "List":
		return TList, nil
	case "Function":
		return TFunction, nil
	case "Map":
		return TMap, nil
	default:
		return NewType(name)
	}
}
