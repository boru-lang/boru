# `net/mail` → `boru:mail`  *(NICHE)*

> **Status: design proposal — not implemented. NICHE.** A small, curated
> native module wrapping the address-parsing slice of Go's `net/mail`.
> Read [`README.10.md`](README.10.md) first for the shared conventions
> this note assumes.

## 1. Package & status

Go package: `net/mail` — parsing of mail messages and RFC 5322 address
headers. This note specifies `boru:mail` (namespace `Mail`), covering
**only the address-parsing helpers** — the most reusable, value-shaped
part of the package. Flagged **niche**: useful when handling email
addresses, but not a core data-language need. Design proposal; no Go code
exists yet.

## 2. Why curated

The raw `go:net/mail` bridge would return `*mail.Address` pointers and a
`[]*mail.Address` slice of handles. The curated surface flattens each
address into a plain `Map {name, address}` (and a list of them), so the
parsed parts are ordinary inspectable data, and `format-address` is the
exact inverse. `(value, error)` returns collapse to value-or-error.

## 3. Import & namespace

```
import "boru:mail"            # binds the Mail namespace
```

`Mail` is not a builtin type and not an existing module namespace, so the
**bare namespace is used (no `-util` suffix)**. Words: `Mail.parse-address`,
`Mail.format-address`, …

## 4. API

Signatures are **top-first, sig order** (position 0 = top of stack). All
inner natives use `BarrierPos: -1` so the infix form dispatches.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `mail.ParseAddress` | `parse-address` | `[String] -> [Map]` | Parse a single RFC 5322 address. | `(*mail.Address, error)` → a flat `Map {name, address}` or error (`parse-address`). `name` is the display name (empty if none); `address` is the `local@domain` part. |
| `mail.ParseAddressList` | `parse-address-list` | `[String] -> [List]` | Parse a comma-separated address list. | `([]*mail.Address, error)` → a `List[Map {name, address}]` or error (`parse-address`). |
| `(*mail.Address).String` | `format-address` | `[Map] -> [String]` | Render an address Map to RFC 5322 form. | Inverse of `parse-address`: a `Map {name, address}` → String (quotes the display name when needed, as Go does). Missing `name` → bare address. |

## 5. Types

Scalars / List / Map only. No opaque external handle, no FixedID
allocation — the `*mail.Address` handle is deliberately flattened to a
Map.

## 6. Errors

No panics — guard with `AsConcreteString` / `RequireConcreteMap`. Failure
via `r.BoruError(code, detail, word)`:

- `parse-address` / `parse-address-list` — Go parse error (malformed
  address) → `parse-address`.
- A non-String arg to a parse word, or a non-Map arg to `format-address`,
  or a Map missing `address` → `bad-arg`.

## 7. Policy / capabilities

**None — pure.** In-memory string parsing/formatting only; no network,
disk, env, or clock. Runs under any policy.

## 8. Overlap

None. No existing module parses email addresses. (`boru:net` performs HTTP;
it does not touch mail headers.)

## 9. Examples

All args-before form; never `Mail.word a b`.

```
import "boru:mail"

"Ada <ada@x.io>" Mail.parse-address
# → {name:"Ada" address:"ada@x.io"}

"a@x.io, Bob <b@x.io>" Mail.parse-address-list
# → [{name:"" address:"a@x.io"} {name:"Bob" address:"b@x.io"}]

{name:"Ada" address:"ada@x.io"} Mail.format-address
# → "\"Ada\" <ada@x.io>"
```

## 10. Open questions / out of scope

- Out of scope: `mail.ReadMessage` / `mail.Message` (full message
  parsing with bodies → streaming, belongs with `boru:io` if ever),
  `mail.Header` accessors, `mail.ParseDate` (date strings →
  `boru:time-util` territory). Keep this module to the three address words.

## 11. Implementation sketch

Wiring checklist (no code), mirroring `math.go` (pure module):

- `lang/go/modules/mail.go` —
  `BuildMailModule(parent) (ModuleDesc, error)`: fresh `subReg`, register
  inner `[]NativeFunc` (each sig `BarrierPos: -1`), wrap as `FnDef`
  exports, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Mail": …}}`.
- Register `BuildMailModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_mail.go` — `registerDocs("boru:mail", {…})` with a
  one-liner per export.
- `lang/spec/module-mail.tsv` — rows leading with `import "boru:mail"`;
  every positive row paired with an `ERROR:<substring>` negative sibling
  (a malformed address → `ERROR:parse-address`).
- Boundary conversion via `core.FromNative` / `core.ToNative`
  (String↔`string`, Map↔`map[string]any`, List↔slice).
- No FixedID / external-type entry.
