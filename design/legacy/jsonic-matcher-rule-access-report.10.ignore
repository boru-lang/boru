# Jsonic Matcher Rule Access: TypeScript vs Go

**Date:** 2026-04-04
**Context:** Exploring jsonic-native string interpolation for boru backtick strings

---

## Summary

The TypeScript version of jsonic passes the current parsing **rule** to lexer matchers, giving them direct access to rule state (K/U/N maps). The Go port (v0.1.5) does **not** — matchers only receive the `*Lex` instance. This gap prevents a clean jsonic-native implementation of `${...}` string interpolation in Go without resorting to shared mutable closure state.

---

## TypeScript Jsonic (`jsonicjs/jsonic`)

**Source:** `src/lexer.ts`, around line 1016

### Matcher Signature

```typescript
function matcherName(lex: Lex, rule: Rule, tI: number = 0): Token | undefined
```

### Matcher Invocation (in `next()` method)

```typescript
for (let mat of this.cfg.lex.match) {
  if ((tkn = mat(this, rule, tI))) {
    match = mat
    break
  }
}
```

### Key Observations

- Matchers receive **three parameters**: the lexer instance (`this`), the current parsing rule, and an optional token index (`tI`).
- The `rule` parameter allows matchers to access contextual state such as `rule.state` and `rule.spec.def.tcol`.
- Some matchers use the rule directly — e.g., `matchMatcher` checks `'o' === rule.state` to determine matching behavior.
- This design enables matchers to make context-sensitive decisions based on which grammar rule is active.

---

## Go Jsonic (v0.1.5)

**Source:** `github.com/jsonicjs/jsonic/go@v0.1.5`

### Matcher Signature

```go
type LexMatcher func(lex *Lex) *Token
```

### Matcher Invocation (in `nextRaw()`)

```go
for _, matcher := range lex.Config.Lex.Match {
    if tkn := matcher(lex); tkn != nil {
        return tkn
    }
}
```

### Key Observations

- Matchers receive **only** the `*Lex` instance — no rule parameter.
- No access to `rule.K`, `rule.U`, or `rule.N` maps from within a matcher.
- The `LexSub` callback (`func(tkn *Token, rule *Rule, ctx *Context)`) fires **after** a token is produced and does receive the rule, but it cannot influence token production.
- Workaround: use a shared mutable variable (closure state) bridging `LexSub` and a custom `LexMatcher`, but this is fragile and non-idiomatic.

---

## Comparison Table

| Aspect                  | TypeScript                          | Go (v0.1.5)                     |
|-------------------------|-------------------------------------|---------------------------------|
| Matcher signature       | `(lex, rule, tI) => Token?`        | `(lex *Lex) *Token`            |
| Rule access in matcher  | Yes — direct                        | No                              |
| K/U/N map access        | Yes — via `rule.K`, etc.            | Not available                   |
| Context-sensitive lexing | Native                              | Requires closure workaround     |
| LexSub (post-token)     | N/A (not needed)                    | Has rule, but fires too late    |

---

## Impact on boru String Interpolation

### Current Approach (Phase 1 — Implemented)

Post-processing of backtick strings after jsonic tokenizes them. The parser's `splitInterpolation` function scans the string content for `${...}` patterns and recursively parses expressions. This works but bypasses jsonic's lexer entirely for interpolation detection.

### Desired Approach (Phase 2 — Blocked)

A jsonic-native approach where:

1. Backtick starts a template string (tracked via `rule.K["in_template"]`)
2. `${` is a registered multi-char token that triggers interpolation mode
3. A custom matcher checks `rule.K["in_template"]` to decide whether `${` should emit an interpolation token or be treated as literal text
4. Jsonic rules handle nesting and expression parsing natively

**This approach requires matchers to access the rule**, which the Go version does not support.

### Closure Workaround

```go
var inTemplate bool // shared mutable state

j.AddMatcher("template_interp", 1000000, func(lex *Lex) *Token {
    if !inTemplate {
        return nil
    }
    // check for ${ and emit token...
})

// LexSub sets inTemplate based on rule.K
```

This works but has drawbacks:
- Non-local mutable state shared between lexer and parser phases
- Fragile — ordering and concurrency concerns
- Does not align with jsonic's intended architecture

---

## Resolution

**Resolved in jsonic/go v0.1.6.** The Go library was updated to pass the
rule to matchers, aligning with the TypeScript version:

```go
// v0.1.6 signature (matches TS)
type LexMatcher func(lex *Lex, rule *Rule) *Token
```

boru's string interpolation now uses this API. A custom `LexMatcher`
(priority 1M) checks `rule.K["boru_tpl"]` to produce template literal
tokens (#TL) only inside backtick strings. The interp/ielem/iexpr/ieval
grammar rules use K map propagation for state tracking, and nesting
works to any depth since each `iexpr` pushes to `valof` which can open
a fresh `interp` rule.

The closure workaround described above is no longer needed.

---

## Files Referenced

| File | Description |
|------|-------------|
| `jsonicjs/jsonic` `src/lexer.ts:~1016` | TS matcher invocation with rule |
| `jsonic/go@v0.1.6` `lexer.go` | Go `LexMatcher` type and `nextRaw()` |
| `jsonic/go@v0.1.6` `plugin.go` | `AddMatcher`, `LexSub` APIs |
| `jsonic/go@v0.1.6` `rule.go` | `ParseAlts` where `lex.Next()` called |
| `lang/go/internal/parser/parse.go` | Jsonic-native interpolation rules |
| `lang/go/internal/engine/engine.go` | InterpString evaluation |
