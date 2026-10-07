package basic

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// The `var` word (design/IMMUTABLE-DEF.1.md §2.3): `var NAME value` and
// `var NAME:Type value`. A var is the one binding whose value changes in
// place. If a var NAME is visible within the current frame — declared in the
// current scope, in an enclosing block of the same frame, or at the module
// scope when no frame is open — the statement ASSIGNS it (a typed var checks
// the value against its type: type_error); otherwise it DECLARES a var in the
// current scope. A var of another frame — a module var from inside a fn body,
// an enclosing fn's var from a closure — is readable (a read is a lookup, as
// every binding's) and never assignable: var_error. A var holds a VALUE: the
// constructor keyword forms (`var f fn […]`, `var C class {…}`, …) and a
// type literal are refused with var_error; a function value computed at run
// time is stored and dispatches when read, as any fn value does. A closure
// made while a fn-local var is live captures the var's value at creation, as
// ComputeCaptures snapshots every enclosing-fn local.
//
// Both lanes see a declaration as the binding it is (installAndRecord's
// hooks, the bind ledger's BindDef) and an assignment as a REPLACE — the
// ledger's BindDefReplace, whose twin replaces rather than pushes, with the
// same recorder hooks a def rebind records (a loop-carried store, a new
// value-def epoch), so a read after the assignment compiles to the new value.
//
// REGISTRATION mirrors def's: the typed-name form first (a Map sorts
// before the name forms, as def's does), then the string and quoted names.
// (While the `var [[…]]` construct shared the name, only the quoted-name
// form was registered: the construct's one-list signature beside a form a
// STACK value could fill made `var [[a b] …]`'s forward/stack split depend on
// gradual operands. The construct is gone — design/IMMUTABLE-DEF.1.md §5
// phase 1 — and the three forms are one word again.)
var varWordSignatures = []Signature{
	{
		// Typed-name binding: var name:Type value.
		Args:          []*Type{TMap, TAny},
		NoEvalMapArgs: map[int]bool{0: true},
		Impl:          Go(VarTypedHandler, RunInCheck()),
		Returns:       []*Type{},
		BarrierPos:    -1,
	},
	{
		Args:       []*Type{TString, TAny},
		Impl:       Go(VarWordHandler, RunInCheck()),
		Returns:    []*Type{},
		BarrierPos: -1,
	},
	{
		Args:       []*Type{TAtom, TAny},
		QuoteArgs:  map[int]bool{0: true},
		Impl:       Go(VarWordHandler, RunInCheck()),
		Returns:    []*Type{},
		BarrierPos: -1,
		// The quoted operand is the NAME of a registry write, as def's.
		CompileEffect: CompileQuoteKey,
	},
}

// VarWordHandler is `var NAME value`.
func VarWordHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return bindVar(r, DefName(args[0]), nil, "", args[1], args[0].Pos())
}

// VarTypedHandler is `var NAME:Type value`: the annotation must be a type
// value, as def's typed form requires; the value is checked against it here
// and at every assignment.
func VarTypedHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	nameMap, _ := AsMap(args[0])
	if nameMap == nil || nameMap.Len() != 1 {
		return nil, r.BoruError("var_error", "var: typed-name map must have exactly one key", "var")
	}
	name := nameMap.Keys()[0]
	constraint, _ := nameMap.Get(name)
	pos := args[0].Pos()
	if pos.Row == 0 {
		pos = r.Check.CurWordPos // the typed-name map is synthesised, unpositioned
	}
	constraint, perr := evalParenAnnotation(r, name, constraint)
	if perr != nil {
		return nil, perr
	}
	var typeName string
	constraint, typeName, _ = r.ResolveTypedNameValue(constraint)
	if !IsTypeBody(constraint) {
		return nil, r.BoruError("var_error",
			fmt.Sprintf("var %s: type annotation must be a type value, got %s", name, constraint.String()), "var")
	}
	if !IsBareTypeNode(constraint) {
		// Phase 1 types a var by a NAMED or builtin type node; the structural
		// annotations def's typed form accepts (a predicate, a schema, a
		// typed container) are def's.
		return nil, r.BoruError("var_error",
			fmt.Sprintf("var %s: a var's type is a type name, got %s", name, constraint.String()), "var")
	}
	varType := core.CanonicalType(r, &constraint)
	if typeName == "" {
		typeName = constraint.String()
	}
	return bindVar(r, name, varType, typeName, args[1], pos)
}

// bindVar declares or assigns name (§2.3), after the name and value checks
// every var statement makes.
func bindVar(r *Registry, name string, varType *Type, typeName string, v Value, pos SrcPos) ([]Value, error) {
	if IsCapitalisedName(name) {
		return nil, r.BoruError("var_error",
			fmt.Sprintf("var %s: a var holds a value — a capitalised name is a type; use def", name), "var")
	}
	if err := ValidateWordName(name); err != nil {
		return nil, fmt.Errorf("var %s: %w", name, err)
	}
	if r.IsBuiltinWord(name) {
		return nil, reservedWordError(r, "var", name)
	}
	if r.Defs.IsType(name) {
		return nil, r.BoruError("var_error", fmt.Sprintf("var %s: name clash — already a type", name), "var")
	}
	if IsBareTypeNode(v) {
		return nil, r.BoruError("var_error",
			fmt.Sprintf("var %s: a var holds a value: use def for a type", name), "var")
	}
	if top, has := r.Defs.TopEntry(name); has && top.Var {
		if top.Frame {
			// A CAPTURED var (core.InstallCapturedBinding): the closure
			// reads the value the enclosing fn's var held at its
			// construction and assigns it no more than a fn body assigns
			// a module var — one frame rule for both. A plain capture let
			// this statement declare or assign a copy of the cell silently
			// (var.tsv §3, 2026-10-07).
			detail := fmt.Sprintf("var %s: cannot assign a var of an enclosing frame (a closure reads the var it captured but does not assign it)", name)
			varRuntimeRaise(r, "var_error", detail, pos)
			return nil, r.BoruError("var_error", detail, "var")
		}
		if !r.Defs.InCurrentFrame(top.Scope) {
			// The frame rule is the check pass's: a compiled unit keeps no
			// frame scope on the registry, so the compiled lane raises this
			// error through a trap at the site (or declines the program).
			detail := fmt.Sprintf("var %s: cannot assign a var of an enclosing frame (a fn body reads a module var but does not assign it)", name)
			varRuntimeRaise(r, "var_error", detail, pos)
			return nil, r.BoruError("var_error", detail, "var")
		}
		if varType != nil {
			detail := fmt.Sprintf("var %s: already declared — assign it as `var %s value`", name, name)
			varRuntimeRaise(r, "var_error", detail, pos)
			return nil, r.BoruError("var_error", detail, "var")
		}
		if top.VarType != nil {
			if !IsConcrete(v) {
				// A computed value's membership is the run's to check; the
				// compiled assign op checks nothing, so the statement is the
				// interpreter's (as a typed def's DepScalar bind is).
				r.Check.SuppressedRuntimeError = true
			}
			unified, ok := varValueOfType(r, v, top.VarType)
			if !ok {
				return nil, r.BoruError("type_error",
					fmt.Sprintf("var %s: value %s does not unify with declared type %s", name, v.String(), top.VarType.Name()), name)
			}
			v = unified
		}
		// The compiler lowers the bind that follows as a replace of the cell.
		r.Check.Recorder().NoteVarAssign(name)
		return installAndRecord(r, name, v, pos, func(nv Value) { core.AssignVar(r, name, nv, pos) })
	}
	if varType != nil {
		if !IsConcrete(v) {
			r.Check.SuppressedRuntimeError = true
		}
		unified, ok := varValueOfType(r, v, varType)
		if !ok {
			return nil, r.BoruError("type_error",
				fmt.Sprintf("var %s: value %s does not unify with declared type %s", name, v.String(), typeName), name)
		}
		v = unified
	}
	return installAndRecord(r, name, v, pos, func(nv Value) { core.InstallVar(r, name, nv, varType) })
}

// varRuntimeRaise makes the compiled lane raise a var refusal the check pass
// met where the interpreter raises it at run time: a trap at the site when
// the recorder can own one — the program's root (RecordTrap) or a unit's
// root (RecordUnitTrapErr, a lambda body's `var acc …` over a module var) —
// else the check-suppressed-runtime-error blanket, which declines the
// program to the interpreter. A no-op outside a check pass.
func varRuntimeRaise(r *Registry, code, detail string, pos SrcPos) {
	es := r.Check.Recorder()
	if !es.Active() {
		// A plain check, or an analysis the compile pass runs with recording
		// paused (a fn body's shape analysis, the pending-body pass): the
		// diagnostic is the verdict, and a unit compile of the same body
		// meets the refusal again, recording.
		return
	}
	if es.RecordTrap(code, detail, "var", "", pos) {
		return
	}
	if es.RecordUnitTrapErr(&core.BoruError{Code: code, Detail: detail, Src: "var"}, pos) {
		return
	}
	r.Check.SuppressedRuntimeError = true
}

// varValueOfType checks v against a typed var's declared type: the
// registry-armed Unify def's typed form ends in, so a user type's own
// membership runs. A carrier (the check pass's stand-in) that conforms by
// its static type is admitted and keeps its identity.
func varValueOfType(r *Registry, v Value, t *Type) (Value, bool) {
	if !IsConcrete(v) {
		return v, v.Parent != nil && v.Parent.ConformsTo(t)
	}
	unified, ok := UnifyR(v, NewTypeLiteral(t), r)
	if !ok {
		return v, false
	}
	if IsBareTypeNode(unified) {
		// Unify swaps when the type side orders the value's own node; the
		// VALUE is what the var holds.
		return v, true
	}
	return unified, true
}
