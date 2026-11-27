package basic

// This file contains the helpers and parsers used by the `fn` word.
// The fn handler itself lives in native_definition.go.

// parseFnDef, parseFnUndefSpec: re-exported from borueng via
// aliases.go (canonical implementations live in core/go/fn_def.go).

// parseFnReturns: re-exported from borueng via aliases.go
// (canonical implementation lives in core/go/fn_params.go).

// parseFnParams, resolveSigType, resolveTypeName: re-exported from
// borueng via aliases.go (canonical implementations live in
// core/go/fn_params.go).

// MatchFnSig is re-exported from core in aliases.go. The implementation moved
// DOWN to core (core/go/fnsig.go) because every operand is a core type and the
// VM needs the same question answered: a dynamic apply must tell "not a
// function" from "a function no overload of which admits these arguments"
// (NUR107), and eng cannot import basic.

// ExpandOptionalSigs: re-exported from borueng via aliases.go
// InstallFnDef: re-exported from borueng via aliases.go
// CallBoru: re-exported from borueng via aliases.go
// InstallDef: re-exported from borueng via aliases.go
// FnDefsOverlap: re-exported from borueng via aliases.go
// UninstallDef: re-exported from borueng via aliases.go
// UninstallFnSigs: re-exported from borueng via aliases.go
