package core

// This file owns the canonical multi-sig fn parser. Both the bare
// borueng `fn` (in core_words.go::registerCoreFn) and the production
// boru `def`/`fn` in lang/go/engine/native_definition_fn.go call
// into ParseFnDef. Single source of truth — do NOT duplicate the
// triple-walking logic anywhere else.
//
// Public surface:
//
//   ParseFnDef(r, list)  (FnDefInfo, error)
//      — walks `list` in triples of [input-sig, output-sig, body],
//        building one FnSig per triple. Returns the assembled
//        FnDefInfo. An empty list yields an empty FnDefInfo.
//
// An output sig is ALWAYS the types the returns must match — never
// values to splice. A value IS a type (ADR-010), so a literal there is a
// literal type constraint: `fn x:String 22 [22]` declares one return
// that must match 22, and `fn x:String 22 [33]` fails the return check.
//
// This file used to CLASSIFY the sig first, and an all-concrete sig
// became "return-by-value sugar" whose values were appended to the body
// after an `end`, with the static return types degraded to Any. That
// classification could not be made correct: every value shape it failed
// to recognise as a type — a def'd type name, a Disjunct, a dotted
// `Pkg.Type`, the single-entry map a `name:Type` pair lowers to — became
// a spliced constant instead and surfaced as a phantom "expected N
// return value(s), got N+1" at the first call. Three of those were
// patched one shape at a time before the classification itself was
// retired, along with the helpers that served it
// (OutputSigIsConcreteReturns / IsSigTypeValue / OutputSigValues).

// ParseFnDef parses a function specification list into FnDefInfo.
// The list contains signature triples: [input-sig, output-sig, body],
// repeated as needed for multi-overload definitions:
//
//	def fact fn [
//	  [n:Integer]            [Integer]  [n 1 addq]
//	  [n:Integer m:Integer]  [Integer]  [n m mulq]
//	]
//
// The list above contains 6 elements (= 2 triples), producing 2
// FnSigs. Each element of a triple may be abbreviated: a non-list
// value is treated as a single-element list (so `String` is
// equivalent to `[String]` for an output signature).
//
// The Registry argument is threaded through to ResolveSigType so
// type-named values inside param specs can resolve via the type
// stack and def stack.
func ParseFnDef(r *Registry, list []Value) (FnDefInfo, error) {
	var sigs []FnSig
	for i := 0; i+2 < len(list); i += 3 {
		inputSig := list[i]
		outputSig := list[i+1]
		body := list[i+2]

		if !inputSig.Parent().Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, barrierPos, err := ParseFnParams(r, inputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		returns, returnPatterns, err := ParseFnReturns(r, outputSig)
		if err != nil {
			return FnDefInfo{}, err
		}

		var bodyElems []Value
		if body.Parent().Equal(TList) && body.Data() != nil {
			_lst, _ := AsList(body)
			bodyElems = _lst.Slice()
		} else {
			bodyElems = []Value{body}
		}

		decl := DeclSite{Pos: sigDeclPos(outputSig)}
		if r != nil {
			decl.Source = r.Source
			decl.File = r.BaseFile
		}

		sigs = append(sigs, FnSig{
			Params:         params,
			Returns:        returns,
			ReturnPatterns: returnPatterns,
			Decl:           decl,
			Impl:           Boru(bodyElems),
			BarrierPos:     barrierPos,
			QuoteArgs:      QuoteArgsFromParams(params),
		})
	}
	return FnDefInfo{Signatures: sigs}, nil
}

// ParseFnUndefSpec parses a list of [input output] sig pairs (no body
// — even-length list) into a FnUndefInfo. Each pair becomes one
// FnSigSpec; the resulting FnUndefInfo represents a function-shape
// TYPE: every signature its inhabitants must satisfy.
//
// Used by the `fn` word when the input list has even length: outside
// `def`, `fn [[in] [out]]` is a TYPE, not a function value. Bind it
// via `def f:fn[[in][out]] some-impl` to assert that an existing
// function satisfies the shape.
//
// Mirrors the production boru parseFnUndefSpec in
// lang/go/engine/native_definition_fn.go.
func ParseFnUndefSpec(r *Registry, list []Value) (FnUndefInfo, error) {
	var sigs []FnSigSpec
	for i := 0; i+1 < len(list); i += 2 {
		inputSig := list[i]
		outputSig := list[i+1]

		if !inputSig.Parent().Equal(TList) {
			inputSig = NewList([]Value{inputSig})
		}

		params, _, err := ParseFnParams(r, inputSig)
		if err != nil {
			return FnUndefInfo{}, err
		}

		returns, _, err := ParseFnReturns(r, outputSig)
		if err != nil {
			return FnUndefInfo{}, err
		}

		sigs = append(sigs, FnSigSpec{
			Params:  params,
			Returns: returns,
		})
	}
	return FnUndefInfo{Sigs: sigs}, nil
}

// sigDeclPos locates the declaration span for a signature slot. A slot
// written as a LIST (`[Integer]`) carries its own source position, but a
// bare `name:Type` pair does not: pair syntax lowers to a map, and the
// parser stamps positions only on the nodes its `val` rule wraps, which
// a list-interior pair never is (parser/go/parse.go). An unpositioned
// slot used to drop the "the declaration of `f` expects N return
// value(s)" span entirely (attachDeclSpan bails on Row <= 0), so the two
// spellings of ONE declaration — `fn [x:A y:B [body]]` and
// `fn [[x:A] [y:B] [body]]` — reported different diagnostics for the
// same defect. The pair's own VALUE keeps its position, so descend to
// the first positioned value inside and anchor the span on the declared
// type itself.
func sigDeclPos(sig Value) SrcPos {
	if p := sig.Pos(); p.Row > 0 {
		return p
	}
	if sig.Parent().Equal(TMap) && sig.Data() != nil {
		if m, err := AsMutableMap(sig); err == nil && m != nil {
			for _, k := range m.Keys() {
				v, _ := m.Get(k)
				if p := sigDeclPos(v); p.Row > 0 {
					return p
				}
			}
		}
	}
	if sig.Parent().Equal(TList) && sig.Data() != nil {
		if l, err := AsList(sig); err == nil {
			for _, e := range l.Elems() {
				if p := sigDeclPos(e); p.Row > 0 {
					return p
				}
			}
		}
	}
	return SrcPos{}
}
