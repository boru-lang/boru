package core

import (
	"fmt"
	"regexp"
	"sort"
)

// Error-code enumeration.
//
// A boru error code is a DISPATCH CONTRACT, not a log string. Programs
// branch on it:
//
//	do [risky] error [dot code case [not_found/q "…" read_error/q "…"]]
//
// so a code is part of the language's observable surface in the same way a
// word name is. Until this file existed there was no list of them anywhere:
// codes were string literals at ~700 construction sites across eng and lang,
// and the only way to learn whether one existed was to provoke the condition
// and print the result. That is how REFERENCE.md came to document seven codes
// no site could mint (design/verse-report-defects-investigation.0.md §E) — a
// `case` arm against any of them could never fire, and nothing could have
// told the author.
//
// This is the enumeration those findings asked for. It is deliberately a
// list of NAMES and OWNERS and nothing else:
//
//   - The names are the contract, and completeness is what makes the list
//     worth having. The bidirectional gate
//     (test/go/docexamples/errorcodes_test.go) fails both when a site mints
//     a code that is not registered here and when a code registered here is
//     minted nowhere — so a new code cannot reach users without a deliberate
//     entry, which is the review point a naming rule needs in order to mean
//     anything.
//   - Descriptions live in REFERENCE.md, where they are already written and
//     already read. Restating 232 of them here — authored in one pass, in a
//     file that reads as authoritative — would create a large body of
//     unreviewed prose competing with the reviewed copy. A wrong description
//     is worse than a pointer to the right one.
//
// STABILITY. Because programs dispatch on codes, RENAMING one is a breaking
// change to every handler that names it, and it is a SILENT one: the old arm
// simply stops firing. Removing one is the same. Treat an entry here as
// published API — add freely, rename and remove only deliberately. Making
// both visible in a diff is the guarantee this file can actually offer; it
// cannot prevent a rename, only make it impossible to do by accident.
//
// OWNERSHIP is layered the same way type registration is (see "Where a Type
// Lives" in eng/go/CLAUDE.md): the kernel owns the mechanism and its own
// codes, and each layer above registers its own. A code minted in BOTH eng
// and lang is owned by eng — the kernel's definition is the one that
// travels, and lang's site is raising the kernel's error rather than defining
// its own.
//
// The enumeration is therefore per-BUILD: a binary linking only eng reports
// only the kernel's codes, which is the honest answer to "what can this
// program raise?" rather than a superset copied from a larger build.

// ErrorCode is one registered code and the layer that owns it.
type ErrorCode struct {
	// Code is the bare name, without the "boru/" render prefix — the exact
	// string `dot code` yields and a `case` arm matches.
	Code string
	// Owner is the registering layer's id (OwnerKernel for the kernel).
	Owner string
}

// errorCodeNamePattern is the naming rule, enforced at registration. What it
// enforces is DISPATCHABILITY, not house style: a code has to be spellable as
// a `case` arm, so lower-case, starting with a letter, and no whitespace.
//
// A hyphen is allowed because boru word names use them freely (`for-each`,
// `with-decimal`) and four codes follow suit — `expected-byte`,
// `bad-encoding`, `cancel-timeout_error`, `cancel-interval_error`. Snake_case
// is the overwhelming convention (233 of 237) and the right choice for a new
// code, but the four hyphenated ones are matchable and renaming a code is a
// silent break for every handler that names it, so the rule admits them.
//
// An earlier draft of this comment claimed every existing code was
// snake_case. That was an artefact of how the doc gate FOUND codes: its
// extraction pattern only matched snake_case, so the hyphenated four — and a
// `"unknown key_error"` typo with a SPACE in it, which no `case` arm could
// ever match — were invisible to it. The gate now scans every string literal
// handed to an error constructor, which is what surfaced them.
var errorCodeNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

var (
	errorCodeRegistry = map[string]ErrorCode{}
	errorCodeInitErrs []error
)

// RegisterErrorCodes declares that owner can attach each of codes to an error
// or a check diagnostic. Call it once per layer from a package-level
// initialiser, so merely linking the layer registers its codes.
//
// A duplicate registration under a DIFFERENT owner is an error: two layers
// each believing they define a code is exactly the drift this enumeration
// exists to prevent. A repeat under the SAME owner is a no-op, so a layer
// whose initialiser runs more than once is harmless.
//
// Errors are accumulated rather than returned, because the call site is a
// package initialiser where there is nobody to return them to; NewRegistry
// surfaces them, per ADR-005's no-init-time-panic rule and following
// BuiltinInitError's precedent.
func RegisterErrorCodes(owner string, codes ...string) {
	for _, code := range codes {
		if !errorCodeNamePattern.MatchString(code) {
			errorCodeInitErrs = append(errorCodeInitErrs, fmt.Errorf(
				"error code %q (owner %q) is not lower snake_case; codes are a "+
					"dispatch contract and must be spellable in a case arm", code, owner))
			continue
		}
		if prev, dup := errorCodeRegistry[code]; dup {
			if prev.Owner != owner {
				errorCodeInitErrs = append(errorCodeInitErrs, fmt.Errorf(
					"error code %q registered by both %q and %q; one layer must own it",
					code, prev.Owner, owner))
			}
			continue
		}
		errorCodeRegistry[code] = ErrorCode{Code: code, Owner: owner}
	}
}

// ErrorCodeInitError returns the first error accumulated while registering
// error codes, or nil. NewRegistry consults it so a malformed or double-owned
// code is reported at construction time rather than silently accepted.
func ErrorCodeInitError() error {
	if len(errorCodeInitErrs) == 0 {
		return nil
	}
	return errorCodeInitErrs[0]
}

// ErrorCodes returns every registered code, sorted by name. This is the data
// source for tooling that has to enumerate codes rather than guess at them —
// a `boru explain <code>`, a REFERENCE.md generator, an editor completion
// list.
func ErrorCodes() []ErrorCode {
	out := make([]ErrorCode, 0, len(errorCodeRegistry))
	for _, ec := range errorCodeRegistry {
		out = append(out, ec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// LookupErrorCode returns the registration for code, or ok=false when no
// layer in this build declares it. A false answer is meaningful: it says a
// `case` arm on that name can never fire.
func LookupErrorCode(code string) (ErrorCode, bool) {
	ec, ok := errorCodeRegistry[code]
	return ec, ok
}

// kernelErrorCodes are the codes the eng kernel itself attaches — parse and
// dispatch failures, the check-mode diagnostic families, the resource
// ceilings, and the compiled/interpreted runtime contracts.
// `runtime_error` is the interpreter's own runtime-fault code (`while` /
// `if`: "condition produced no value"). It has been reaching users since
// those raises existed, but only through `Engine.runtimeError` — a helper
// codeMintPatterns does not match — so nothing ever required it to be
// enumerated. The forty-second increment's check mirror and terminal trap
// attach it from matched sites, which is what surfaced the gap.
//
// MEASURED WHILE FIXING IT, and NOT fixed here: the same blind spot still
// hides `flow_error` and `halt`, the other two codes `runtimeError` mints
// that no matched site attaches (`move_error`, `evaluation_limit` and
// `tape_exhausted` are registered by other routes). Closing it properly
// means adding a `runtimeError\(` pattern to codeMintPatterns and
// registering those two — a change to the code-stability contract that
// wants its own increment rather than a ride on this one.
var kernelErrorCodes = []string{
	"analysis_truncated", "arity_mismatch", "branch_error", "concurrency_error",
	"constraint_violation", "def_error", "dynamic_dispatch", "evaluation_limit",
	"exit", "extend_conflict", "extend_owner", "float_overflow",
	"fn_body_error",
	"for_error", "forward_strands_operand", "gen_without_constructor", "illegal_key", "illegal_ref",
	"incomparable", "index_out_of_range", "integer_overflow", "internal_error",
	"invalid_word_name", "locked_signature", "macro_error", "macroexpand_error",
	"micron_name", "missing_returns", "mixed_form_call", "no_signature",
	"no_value_error", "partial_dispatch", "record_shape_mismatch", "redundant_guard",
	"reserved_word", "runtime_error", "signature_error", "speculative_forward_commit", "step_budget_exceeded",
	"stranded_type_call",
	"syntax_error", "tape_exhausted", "type_error", "unbound_param",
	"sugar_unbound",
	"uncalled_function", "undefined_word", "unsupported", "unused_def",
	"weak_value_error", "while_error",
}

var _ = registerKernelErrorCodes()

func registerKernelErrorCodes() struct{} {
	RegisterErrorCodes(OwnerKernel, kernelErrorCodes...)
	return struct{}{}
}
