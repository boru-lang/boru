# `net/url` → `boru:url`

> **Status: design proposal — not implemented.** A curated, hand-written
> native module wrapping Go's `net/url` with an idiomatic ("boru-ish")
> surface. Read [`README.10.md`](README.10.md) first for the shared
> conventions this note assumes.

## 1. Package & status

Go package: `net/url` — parsing and escaping of URLs and query strings.
This note specifies `boru:url` (namespace `Url`), a value-oriented wrapper
that turns a URL into a plain inspectable Map rather than an opaque
handle. Design proposal; no Go code exists yet.

## 2. Why curated

The raw `go:net/url` bridge would hand back a `*url.URL` pointer — an
opaque, `Any`-typed handle whose fields are reachable only through more
bridged method calls (`(*URL).Hostname`, `(*URL).Query`, …). For a
query/data language that is hostile: a URL is *data*, and the user wants
to read and rewrite its parts. The curated surface flattens the parse
result into a `Map` of plain scalars, so every field is reachable with
ordinary `get` and the whole thing round-trips through `build`. The
escaping helpers collapse the stdlib's `(value, error)` returns into
value-or-error, and `parse-query` returns the natural `Map` of
`key → List[String]` instead of the `url.Values` type.

## 3. Import & namespace

```
import "boru:url"          # binds the Url namespace
```

The bare capitalized package name `Url` is free (not a builtin type, not
an existing module namespace), so **no `-util` suffix** is needed
(contrast `boru:path-util`, whose `Path` would collide with the builtin
`Path` type). Words are dot-accessed: `Url.parse`, `Url.query-escape`, …

## 4. API

Signatures are **top-first, sig order** (position 0 = top of stack), per
the README "Argument order & dispatch" rule. All inner natives use
`BarrierPos: -1` so the infix form `a Url.word b` dispatches.

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `url.Parse` | `parse` | `[String] -> [Map]` | Parse a URL into its components. | **KEY**: no opaque `*url.URL`. Returns a flat `Map {scheme, host, port, path, query, fragment, user}` (`String → Map`); `(*url.URL, error)` collapses to Map-or-error. |
| `(*url.URL).String` | `build` | `[Map] -> [String]` | Render a component Map back to a URL string. | The inverse of `parse`: takes the same flat Map and emits a URL (`Map → String`). Aliased as `format`. Missing keys default empty. |
| `url.QueryEscape` | `query-escape` | `[String] -> [String]` | Percent-encode a string for use in a query value. | Pure `String → String`; spaces become `+`. |
| `url.QueryUnescape` | `query-unescape` | `[String] -> [String]` | Decode a query-escaped string. | `(string, error)` → value-or-error (`error invalid-escape`). |
| `url.PathEscape` | `path-escape` | `[String] -> [String]` | Percent-encode a string for use in a path segment. | Pure `String → String`; spaces become `%20`. |
| `url.PathUnescape` | `path-unescape` | `[String] -> [String]` | Decode a path-escaped string. | `(string, error)` → value-or-error (`error invalid-escape`). |
| `url.ParseQuery` | `parse-query` | `[String] -> [Map]` | Parse a query string into a Map of key → List of values. | `url.Values` (a `map[string][]string`) becomes a `Map` of `key → List[String]`; repeated keys collect into the List. `(Values, error)` → Map-or-error. |
| `url.Values.Encode` | `encode-query` | `[Map] -> [String]` | Encode a key → List Map into a sorted query string. | Inverse of `parse-query`: a `Map` of `key → List[String]` (a bare `String` value is accepted and treated as a one-element List) → sorted, escaped query string. |

### The flattened URL Map

`parse` returns these string keys (all `String`, empty when absent):

| key | source field | notes |
|---|---|---|
| `scheme` | `URL.Scheme` | e.g. `https` |
| `host` | `URL.Hostname()` | host without port |
| `port` | `URL.Port()` | port without host; empty if none |
| `path` | `URL.Path` | decoded path |
| `query` | `URL.RawQuery` | raw query string (run through `parse-query` for a Map) |
| `fragment` | `URL.Fragment` | after `#` |
| `user` | `URL.User.Username()` | empty if no userinfo |

`build` reads the same keys back and is tolerant of a partial Map (a Map
with only `host` and `path` yields a relative-ish URL). Keeping the value
a plain Map means it is inspectable, comparable, and serializable with no
special handling — the central refinement of this module.

## 5. Types

Scalars / List / Map only. **No opaque external handle** — the whole
point of the curated surface is to avoid the `*url.URL` pointer that the
reflection bridge would expose, so there is no `RegisterExternalBuiltin`
/ FixedID allocation for this module.

## 6. Errors

No panics (guard args with `AsConcreteString` / `RequireConcreteMap`
before use). Failure is signalled via `r.BoruError(code, detail, word)`
with kebab codes:

- `parse` — Go `url.Parse` error → `parse`.
- `query-unescape` / `path-unescape` — Go decode error → `invalid-escape`.
- `parse-query` — Go `url.ParseQuery` error → `parse-query`.

A non-String arg to an escape word, or a non-Map arg to `build` /
`encode-query`, errors as `bad-arg` rather than panicking.

## 7. Policy / capabilities

**None — pure.** All operations are in-memory string transforms; nothing
touches the network, disk, env, or clock. Runs under any policy. (Network
access lives in `boru:net`, see Overlap.)

## 8. Overlap

`boru:net` (`Net.fetch` / `Net.prepare` / `Net.direct`) uses `net/url`
**internally** — `native/fetch.go` calls `url.Parse(rawURL)` to validate
and split the target — but it exposes **no url words to boru users**
(`docs_net.go` lists only `direct`, `fetch`, `prepare`). So `boru:url` is a
**genuinely new user-facing surface**, not a re-spec of anything in
`boru:net`; it does not move or change any existing word. The dividing
line: `boru:net` *performs* requests; `boru:url` only *manipulates URL
strings and query data*.

## 9. Examples

All args-before form (`a b Url.word` / `a Url.word b`); never
`Url.word a b`.

```
import "boru:url"

"https://u@host.io:8443/a/b?x=1&x=2#frag" Url.parse
# → {scheme:"https" host:"host.io" port:"8443" path:"/a/b"
#    query:"x=1&x=2" fragment:"frag" user:"u"}

{scheme:"https" host:"host.io" path:"/a"} Url.build
# → "https://host.io/a"

"a b&c" Url.query-escape          # → "a+b%26c"
"a+b%26c" Url.query-unescape      # → "a b&c"

"x=1&x=2&y=3" Url.parse-query     # → {x:["1" "2"] y:["3"]}
{x:["1" "2"] y:["3"]} Url.encode-query   # → "x=1&x=2&y=3"
```

## 10. Open questions / out of scope

- Out of scope: `url.ParseRequestURI`, `url.URL.ResolveReference`
  (relative resolution), `url.UserPassword` (credentials in URLs are a
  security smell). Add later if a concrete need appears.
- Open: whether `build` should re-escape `path` (currently assumes the
  caller passes a decoded path and we escape on emit) — pin both
  directions with a positive/negative TSV pair.
- Open: whether to expose `query` as a pre-parsed Map directly in
  `parse`'s result (today it is the raw string; the user runs
  `parse-query` explicitly). Keeping it raw avoids a lossy double-parse.

## 11. Implementation sketch

Wiring checklist (no code), mirroring `math.go` (pure module):

- `lang/go/modules/url.go` — `BuildUrlModule(parent) (ModuleDesc, error)`:
  fresh `subReg` via `native.DefaultRegistry()`, register the inner
  `[]NativeFunc` (each sig `BarrierPos: -1`), wrap as `FnDef` exports into
  an `*OrderedMap`, return `ModuleDesc{ID: parent.Modules.NextID(),
  Exports: {"Url": …}}`.
- Register `BuildUrlModule` in the `modules` map in
  `lang/go/modules/modules.go`.
- `lang/go/modules/docs_url.go` — `registerDocs("boru:url", {…})` with a
  one-liner per export (else `TestModuleExportDocs` fails).
- `lang/spec/module-url.tsv` — `input⇥expected⇥description` rows leading
  with `import "boru:url"`; every positive row paired with an
  `ERROR:<substring>` negative sibling (Test discipline).
- Boundary conversion via `core.FromNative` / `core.ToNative`
  (String↔`string`, Map↔`map[string]any`, List↔slice).
- No FixedID / external-type entry (no opaque handle).
