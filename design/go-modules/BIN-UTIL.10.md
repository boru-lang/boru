# `boru:bin-util` (expanded) — the binary-adjacent hub

> **Status: design proposal, not implemented.** This note specifies a
> large expansion of the existing `boru:bin-util` module
> (`lang/go/modules/binary.go`, namespace `BinUtil`) to **concentrate all
> binary-adjacent functionality in one place**: cryptographic hashes,
> HMAC, secure random, CRC checksums, hex / base32 / base64 / base128 /
> ascii85 encoding, and GUIDs. It supersedes the standalone CRYPTO-*,
> HASH-CRC*, and ENCODING-BASE64/HEX notes (now removed). It builds on the
> new [`Bytes`](BYTES.10.md) type. Read [README.10.md](README.10.md) first.

## 1. Why concentrate, not scatter

`bin-util` already owns the bit/byte/hash domain — bitwise ops,
`popcount`/`clz`/`ctz`, `fnv32`/`fnv64`, `ord`/`chr` — delegating to
`math/bits` and `hash/fnv`. Crypto, CRC, encodings, and UUIDs are the
same family of "work with raw bytes" operations. Rather than a dozen
single-purpose modules (`boru:base64`, `boru:sha256`, …), the user's
decision is **one coherent module** whose words all speak the `Bytes`
type and read uniformly. This keeps imports simple (`import
"boru:bin-util"`), the API discoverable (`describe BinUtil`), and the byte
model consistent.

## 2. Import & namespace

```
import "boru:bin-util"        # binds the BinUtil namespace (unchanged)
```

The `-util` id + `BinUtil` namespace is unchanged (the existing module).
Words dot-access flat: `BinUtil.sha256`, `BinUtil.base64-encode`, etc. —
**no nested sub-namespaces** (the chosen naming scheme matches today's
flat `BinUtil.*`).

## 3. Conventions specific to this module

- **Data inputs accept `String` or `Bytes`.** Any word that takes "data
  to hash/encode/sign" has two overloads; a `String` is UTF-8 encoded
  first. This keeps the text case terse (`"abc" BinUtil.sha256`) while
  supporting true binary (`someBytes BinUtil.sha256`).
- **Digests and raw binary results are `Bytes`.** Pipe through
  `hex-encode` / `base64-encode` for display. This is forced by the type
  system: a 32-byte SHA-256 digest and a CRC64 value both **exceed boru's
  `int64` `Integer`**, so they cannot be Integers.
- **Decoders return `Bytes`**; core `to-text` converts back to String when wanted.
- Top-first sig order; all inner native sigs `BarrierPos: -1`; invoked
  args-before-dot (`a b BinUtil.word` / `a BinUtil.word b`). See
  README.10.md "Argument order & dispatch".

## 4. Word families

Existing words (bitwise, bit-twiddling, `fnv32`/`fnv64`, `ord`/`chr`) are
**unchanged**. New words below.

### 4.1 Bytes interop (now **core**, not `bin-util`)

The `Bytes` type, frame spec types (`BinarySpec` kind: `def P (refine BinarySpec
[layout])` builds a sealed class; `make` → a `Binary` instance; `convert Bytes`
serialises; `unpack`/`unpack-prefix` decode), and the value overloads (`convert`
text/ints⇄Bytes + compact, `slice`, `add`; plus `size`/`eq`/ordering via type
behaviors) are **core — no import** (see [BYTES.10.md](BYTES.10.md) §5, §7). Hex/binary byte constants are the `+hb/…/` /
`+bb/…/` kinds in `boru:minilang` (BYTES.10.md §6), not a `bin-util` word.
`bin-util` does **not** re-export any of these. This keeps the network-framing hot
path importable-free; `bin-util` builds *on top of* the core type, taking and
returning `Bytes`.

### 4.2 Encoding — hex / base32 / base64 / base128 / ascii85

`encode`: data (`String|Bytes`) → `String`. `decode`: `String` → `Bytes`.

| Go source | encode word | decode word | notes / refinement |
|---|---|---|---|
| `encoding/hex` | `hex-encode` | `hex-decode` | lowercase hex; decode errors `hex-decode`. |
| `encoding/base32` | `base32-encode` | `base32-decode` | std RFC-4648 alphabet. |
| `encoding/base64` (Std) | `base64-encode` | `base64-decode` | padded std alphabet. |
| `encoding/base64` (URL) | `base64url-encode` | `base64url-decode` | URL/filename-safe alphabet. |
| `encoding/base64` (Raw) | `base64-raw-encode` | `base64-raw-decode` | no `=` padding. |
| *(custom, in-tree)* | `base128-encode` | `base128-decode` | 7-bit packing over a fixed 128-symbol printable alphabet; documented in the module (no stdlib equivalent). decode errors `base128-decode`. |
| `encoding/ascii85` | `ascii85-encode` | `ascii85-decode` | base85, denser than base64; decode errors `ascii85-decode`. |

### 4.3 Cryptographic hashes

> **Superseded — see [BORU-CRYPTO.0](../BORU-CRYPTO.0.md).** The
> cryptographic surface below (§4.3 hashes, §4.5 HMAC, §4.6 secure random)
> was later reassigned to a dedicated, policy-scoped `boru:crypto` module,
> which also carries the AEAD / KDF / sealed-box primitives the vault
> migration needs and which `boru:bin-util` does not. Treat these three
> subsections as withdrawn from this proposal: `boru:bin-util` keeps its
> **non-cryptographic** binary surface (bitwise ops, rotates, `popcount`,
> CRC, the `base*`/`hex`/`ascii85` encodings, UUIDs); `sha*`/`hmac`/
> `random-*` live in `Crypto.*`. (Both docs are unimplemented proposals;
> the final call is the maintainer's.)

data (`String|Bytes`) → `Bytes` digest. Pipe through `hex-encode` /
`base64-encode` to render.

| Go symbol | word | digest size | note |
|---|---|---|---|
| `sha256.Sum256` | `sha256` | 32 B | |
| `sha256.Sum224` | `sha224` | 28 B | |
| `sha512.Sum512` | `sha512` | 64 B | |
| `sha512.Sum384` | `sha384` | 48 B | |
| `sha512.Sum512_256` | `sha512-256` | 32 B | |
| `sha1.Sum` | `sha1` | 20 B | **legacy** — broken for security; fine for checksums/interop. |
| `md5.Sum` | `md5` | 16 B | **legacy** — broken for security; fine for checksums/interop. |

### 4.4 Non-cryptographic checksums (CRC)

| Go source | word | result | refinement |
|---|---|---|---|
| `crc32.ChecksumIEEE` | `crc32` | `Integer` | uint32 fits int64. |
| `crc32` Castagnoli table | `crc32c` | `Integer` | `MakeTable(Castagnoli)`. |
| `crc64` ISO table | `crc64-iso` | `Bytes` (8 B) | **uint64 overflows int64** → returned as 8-byte `Bytes`; `hex-encode` for text. |
| `crc64` ECMA table | `crc64-ecma` | `Bytes` (8 B) | same overflow handling. |

(`fnv32`/`fnv64` already exist and stay `Integer`.)

### 4.5 HMAC (keyed MAC)

| word | signature (top-first) | meaning |
|---|---|---|
| `hmac` | `[Atom algo, String\|Bytes msg, String\|Bytes key] -> Bytes` | keyed MAC; `algo` ∈ `sha256`/`sha512`/`sha1`/`md5`. |
| `hmac-verify` | `[Atom algo, Bytes mac, String\|Bytes msg, String\|Bytes key] -> Boolean` | constant-time check via `hmac.Equal`. |

`hmac-verify` uses **`crypto/hmac.Equal`** (constant-time) rather than
`Bytes.equal`, so verification time does not leak how many leading bytes
matched — the standard defense against timing attacks. Unknown `algo`
errors `unknown-algo`.

### 4.6 Secure random (the non-pure surface)

| word | signature | meaning |
|---|---|---|
| `random-bytes` | `[Integer n] -> Bytes` | `n` cryptographically-random bytes. |
| `random-int` | `[Integer max] -> Integer` | uniform in `[0, max)`. |
| `random-hex` | `[Integer n] -> String` | convenience = `random-bytes` then `hex-encode`. |

### 4.7 GUID / UUID

Backed by **`github.com/google/uuid`** (new dependency).

| word | signature | meaning |
|---|---|---|
| `uuid` | `[] -> String` | a random v4 UUID (the default; `guid` is an alias). |
| `uuid-v4` | `[] -> String` | explicit random v4. |
| `uuid-v7` | `[] -> String` | time-ordered, lexicographically-sortable v7. |
| `uuid-parse` | `[String] -> Bytes` | 16 raw bytes; error `bad-uuid`. |
| `uuid-format` | `[Bytes] -> String` | canonical text from 16 bytes; error `bad-uuid`. |
| `uuid-valid` | `[String] -> Boolean` | well-formedness check. |
| `uuid-nil` | `[] -> String` | the all-zero UUID. |

## 5. Types

Uses scalars + `List` + the new [`Bytes`](BYTES.10.md) type. No further
external types are introduced here; the digest/encoding/random/CRC64
results are all `Bytes`. (`Bytes` itself is registered once globally — see
BYTES.10.md §2 — so values produced here carry the real `Bytes` type and
`x is Bytes` works.)

## 6. Errors

Kebab codes via `r.BoruError(code, detail, word)`, Go `error` unwrapped,
never panic (guard with `AsConcreteString` / `AsConcreteInteger` / the
`Bytes` unwrap helper):

| code | raised when |
|---|---|
| `hex-decode` / `base32-decode` / `base64-decode` / `base64url-decode` / `base64-raw-decode` / `base128-decode` / `ascii85-decode` | malformed encoded input. |
| `unknown-algo` | `hmac` / `hmac-verify` given an unsupported `algo` atom. |
| `bad-uuid` | `uuid-parse` / `uuid-format` given malformed input. |
| `random-length` | `random-bytes` / `random-hex` given a negative `n`. |
| `random-range` | `random-int` given `max <= 0`. |
| (plus the core `Bytes` codes: `expected-byte`, `bad-encoding`, `index-range`) | see BYTES.10.md. |

## 7. Policy / capabilities

The split is the whole point of keeping `bin-util` mostly pure:

- **Pure, ungated, deterministic** — all bitwise, byte-interop, encoding
  (hex/base*/ascii85), hashing (sha*/md5/fnv), CRC, and HMAC words. They
  run under any policy and are referentially transparent.
- **Non-pure — the only gated surface** — `random-bytes`, `random-int`,
  `random-hex`, and the entropy-using UUID words (`uuid`, `uuid-v4`,
  `uuid-v7`). These consume OS entropy (and, for v7, the clock). They must
  route through a host **`EntropySource` capability seam** (modeled on
  `FileOps` / `EffectiveClock` in `lang/go/capabilities`), so a sandbox
  can deny them or supply a deterministic source for tests. Add a
  `random` entry to `policy.KnownScopes` and a `random` global cap to
  `policy.GlobalOps` (`lang/go/policy/policy.go`) — none exists today.

Contrast with **`boru:rand`** (seedable PRNG, deterministic, good for
reproducible tests): `bin-util` random is the **unseedable CSPRNG** for
security-grade randomness. Both can coexist.

## 8. Overlap

- **`boru:rand`** — different randomness class (CSPRNG vs seedable PRNG);
  no word collision (`BinUtil.random-*` vs `Rand.*`). Noted in §7.
- **`boru:string-util`** — core `utf8`/`to-text` are the explicit String↔Bytes
  bridge; they do not duplicate string search/transform words.
- The folded standalone notes (base64/hex/sha*/hmac/crc*/crypto-rand) are
  fully represented here; their separate `boru:*` modules are **not**
  created.

## 9. Examples (args-before form)

```
import "boru:bin-util"

# hashing → hex
"hello" BinUtil.sha256 BinUtil.hex-encode
  # "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

# base64 round-trip
"hi there" BinUtil.base64-encode                 # "aGkgdGhlcmU="
"aGkgdGhlcmU=" BinUtil.base64-decode to-text      # "hi there"  (to-text is core)

# HMAC sign + constant-time verify
"secret" "msg" sha256/q BinUtil.hmac                       # Bytes(32) …
"secret" "msg" sha256/q BinUtil.hmac
  "secret" "msg" sha256/q BinUtil.hmac BinUtil.hmac-verify # true

# CRC
"123456789" BinUtil.crc32                          # 3421780262
"123456789" BinUtil.crc64-ecma BinUtil.hex-encode  # "995dc9bbdf1939fa"

# UUID
BinUtil.uuid-v7                                     # "0190a1b2-..." (sortable)
"not-a-uuid" BinUtil.uuid-valid                    # false

# errors
"zz" BinUtil.hex-decode                            # ERROR:hex-decode
"x" "m" bogus/q BinUtil.hmac                        # ERROR:unknown-algo
```

(`sha256/q` passes the algo name as an Atom — see `lang/go/CLAUDE.md`
"Undefined Words".)

## 10. Open questions / out of scope

- **`*-hex` / `*-base64` digest conveniences** — words return raw `Bytes`
  and the user pipes through `hex-encode`/`base64-encode`. Whether to also
  add `sha256-hex` shortcuts is left open (keeps the surface smaller for
  now).
- **base128 alphabet** — the exact 128-symbol alphabet and bit-packing
  must be fixed and documented in `binary.go` so encode/decode are stable
  across versions; pin it with spec rows.
- **A dedicated `random` policy scope vs reusing a global cap** — proposed
  as a new scope; confirm during implementation against the policy schema.
- **Streaming hashes** (incremental `hash.Hash` over large inputs) — out
  of scope; the one-shot `Sum*` form covers the data sizes boru handles.
- **`google/uuid` dependency** — adds an external module; note the
  `GOPROXY=direct` fallback (`lang/go/CLAUDE.md` "Dependencies").

## 11. Implementation wiring

Reference builders: `lang/go/modules/binary.go` (`BuildBinaryModule`, the
sub-registry + `makeBin*FnDef` wrappers) and `io.go` (capability-backed
variant, for the `EntropySource` seam).

- **`core/go/gobridge.go`** — `[]byte` ↔ `Bytes` (see BYTES.10.md §4).
- **`lang/go/native/native_bytes.go`** — register the `Bytes` type
  (BYTES.10.md §11); **`lang/go/test/fixedid_stability_test.go`** — pin
  its FixedID.
- **`lang/go/modules/binary.go`** — add the new natives, grouped
  (`cryptoNatives`, `encodingNatives`, `crcNatives`, `uuidNatives`,
  `randomNatives`, `bytesInteropNatives`) and registered into the existing
  sub-registry; export `FnDef` wrappers via the existing `makeBin*FnDef`
  helpers (all inner sigs `BarrierPos: -1`; zero-arg `uuid*` constants use
  `BarrierPos: 0`). Imports: `crypto/{sha256,sha512,sha1,md5,hmac,rand}`,
  `hash/{crc32,crc64}`, `encoding/{hex,base32,base64,ascii85}`, the custom
  base128 codec, and `github.com/google/uuid`.
- **`lang/go/capabilities`** — an `EntropySource` interface +
  OS-backed/in-memory impls; thread it like `FileOps`. **`lang/go/policy/
  policy.go`** — add the `random` scope + global cap and the `GlobalsFor`
  binding.
- **`lang/go/modules/docs_bin.go`** — a one-line `registerDocs` entry per
  new export (`TestModuleExportDocs` enforces completeness).
- **`lang/spec/module-bin.tsv`** — positive rows leading with `import
  "boru:bin-util"` plus an `ERROR:<substring>` negative sibling per word
  (Test discipline, `lang/go/CLAUDE.md`); include known-answer vectors
  (e.g. the SHA-256 of `"hello"`, CRC of `"123456789"`).
- **`lang/go/go.mod`** — add `github.com/google/uuid`.
