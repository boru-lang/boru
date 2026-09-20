package core

import "fmt"

// Sugar markers — the parser-side half of ADR-012 rule 3 (2026-08-04
// amendment): the parser emits STRUCTURE, never names. Each piece of
// surface sugar that historically desugared to word tokens inside the
// parser (`/u /s /f /N`, `X/t`, `+kind<…>` minilang literals, `=>`,
// `Name<Args>`) now parses to ONE kernel marker value (`Word/__SG`,
// SugarInfo payload). When the marker is stepped, the engine lowers it
// to word dispatches, resolving each word through the sugar-role table
// the language layer binds at registration (BindSugarWord). A registry
// with no binding for a stepped role fails loudly — the
// `undefined_word` precedent — so a host that doesn't support a sugar
// declines cleanly, and the bare kernel (calc) runs with zero bindings.
//
// Design: design/legacy/LANG-ENG-CONTENT-AUDIT.0.ignore §7A. Precedents: Reach
// (lowered by the engine), DispatchMod (peeked at dispatch), __SP
// (spliced at the pointer).

// SugarKind identifies a marker's sugar and doubles as the role key
// the language layer binds a word name to. SugarGenHead / SugarGenApply
// are binding-only roles consumed by the Angle and TypeBound
// expansions; they are never emitted as marker kinds themselves.
type SugarKind string

const (
	SugarUsurp       SugarKind = "usurp"        // `/u` — take over the preceding group's dispatch
	SugarStackArgs   SugarKind = "stack-args"   // `/s` — force stack collection
	SugarForwardArgs SugarKind = "forward-args" // `/f` — force forward collection
	SugarForceArity  SugarKind = "force-arity"  // `/N` — filter signatures by arity N
	SugarMini        SugarKind = "mini"         // `+kind<src>` minilang literal
	SugarLambda      SugarKind = "lambda"       // `=>` — the anonymous-fn constructor
	SugarAngle       SugarKind = "angle"        // `Name<Args>` — generic head or use-site
	SugarTypeBound   SugarKind = "type-bound"   // `X/t` — bounded-Type sugar
	SugarGenHead     SugarKind = "gen-head"     // binding-only: the generic-def head word
	SugarGenApply    SugarKind = "gen-apply"    // binding-only: the generic-apply word
	SugarGenDefault  SugarKind = "gen-default"  // `=` in a gen-param entry — the default-value word
)

// SugarInfo is the Word/__SG marker payload. One struct covers every
// kind; unused fields stay zero.
type SugarInfo struct {
	Kind    SugarKind
	N       int64   // SugarForceArity: the arity
	Name    string  // SugarMini: the minilang kind; SugarAngle: the receiver name
	Src     string  // SugarMini: the raw literal source
	Head    Value   // SugarAngle: the converted gen-params list (head form)
	HeadErr string  // SugarAngle: why the head form is unavailable (surfaced only if selected)
	Items   []Value // SugarAngle: the converted use-site type args; SugarTypeBound: the bound tokens
}

// NewSugar builds a Word/__SG sugar marker.
func NewSugar(info SugarInfo) Value {
	return NewValueRaw(TSugar, info)
}

// IsSugar reports whether v is a Word/__SG sugar marker.
func IsSugar(v Value) bool { return v.Parent.Equal(TSugar) }

// AsSugar returns the SugarInfo if v is a Word/__SG marker.
func AsSugar(v Value) (SugarInfo, bool) {
	info, ok := v.Data.(SugarInfo)
	return info, ok
}

// BindSugarWord binds a sugar role to the word name the engine
// expands it to. The language layer calls this once per role at
// registration; rebinding replaces the previous binding.
func (r *Registry) BindSugarWord(kind SugarKind, word string) {
	if r == nil {
		return
	}
	if r.sugarWords == nil {
		r.sugarWords = make(map[SugarKind]string)
	}
	r.sugarWords[kind] = word
}

// SugarWord returns the word name bound to a sugar role.
func (r *Registry) SugarWord(kind SugarKind) (string, bool) {
	if r == nil || r.sugarWords == nil {
		return "", false
	}
	name, ok := r.sugarWords[kind]
	return name, ok
}

// sugarBoundWord resolves a role binding to a positioned Word token,
// or errors loudly when the registry binds no word for the role. src
// donates the source position (the marker value).
func sugarBoundWord(r *Registry, kind SugarKind, src Value) (Value, error) {
	name, ok := r.SugarWord(kind)
	if !ok {
		source := ""
		if r != nil {
			source = r.Source
		}
		return Value{}, makeBoruErrorAt("sugar_unbound",
			fmt.Sprintf("no word bound for sugar role %q — the host registry does not support this surface form", kind),
			string(kind), source, "", src.Pos())
	}
	return WithPos(NewWord(name), src), nil
}

// sugarExpansion lowers a sugar marker to the token run that replaces
// it on the tape. headForm selects the Angle marker's generic-def head
// expansion (`Name gen [params]`) over the use-site paren
// (`(Name of [args])`).
func SugarExpansion(r *Registry, info SugarInfo, src Value, headForm bool) ([]Value, error) {
	switch info.Kind {
	case SugarUsurp, SugarStackArgs, SugarForwardArgs, SugarLambda, SugarGenDefault:
		w, err := sugarBoundWord(r, info.Kind, src)
		if err != nil {
			return nil, err
		}
		return []Value{w}, nil
	case SugarForceArity:
		w, err := sugarBoundWord(r, info.Kind, src)
		if err != nil {
			return nil, err
		}
		return []Value{w, WithPos(NewInteger(info.N), src)}, nil
	case SugarMini:
		w, err := sugarBoundWord(r, info.Kind, src)
		if err != nil {
			return nil, err
		}
		return []Value{w, WithPos(NewWord(info.Name), src), WithPos(NewString(info.Src), src)}, nil
	case SugarTypeBound:
		of, err := sugarBoundWord(r, SugarGenApply, src)
		if err != nil {
			return nil, err
		}
		tt := WithPos(NewTypeLiteral(TType), src)
		return []Value{WithPos(NewParenExpr(append([]Value{tt, of}, info.Items...)), src)}, nil
	case SugarAngle:
		recv := WithPos(NewWord(info.Name), src)
		if headForm {
			if info.HeadErr != "" {
				source := ""
				if r != nil {
					source = r.Source
				}
				return nil, makeBoruErrorAt("syntax_error", info.HeadErr,
					info.Name, source, "", src.Pos())
			}
			gen, err := sugarBoundWord(r, SugarGenHead, src)
			if err != nil {
				return nil, err
			}
			return []Value{recv, gen, info.Head}, nil
		}
		of, err := sugarBoundWord(r, SugarGenApply, src)
		if err != nil {
			return nil, err
		}
		args := WithPos(NewEvalList(info.Items), src)
		return []Value{WithPos(NewParenExpr([]Value{recv, of, args}), src)}, nil
	}
	source := ""
	if r != nil {
		source = r.Source
	}
	return nil, makeBoruErrorAt("sugar_unbound",
		fmt.Sprintf("unknown sugar kind %q", info.Kind), string(info.Kind),
		source, "", src.Pos())
}

// stepSugar fires a sugar marker at the pointer: it splices the
// marker's expansion in place and re-steps, exactly like the __SP
// splice. The Angle marker picks its head form when a parked forward
// is waiting to /q-capture a name (`def Box<T> …` — the binder's name
// slot), the use-site paren otherwise.
func (e *Engine) stepSugar(valIdx int) error {
	info, ok := AsSugar(e.Tape.At(valIdx))
	if !ok { //covergate:allow stepSugar is gated by IsSugar at both call sites, so the payload is always a SugarInfo
		e.Pointer++
		return nil
	}
	src := e.Tape.At(valIdx)
	headForm := info.Kind == SugarAngle && e.hasPendingForwardQuoteArg()
	exp, err := SugarExpansion(e.Registry, info, src, headForm)
	if err != nil {
		return err
	}
	e.Tape.Splice(valIdx, 1, exp...)
	return nil
}
