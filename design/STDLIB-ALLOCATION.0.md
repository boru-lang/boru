# Allocating the remaining Go stdlib to boru modules

> **Status: recommendation.** One row has since SHIPPED — `crypto/tls`
> as `tls: {…}` options on `boru:net`, with mutual TLS through a
> host-registered identity seam ([NETWORK-TLS-PLAN](legacy/NETWORK-TLS-PLAN.0.ignore)
> phases 1-4). Everything else here is unimplemented and unapproved. It
> takes the residue of [STDLIB-COVERAGE.10.md](STDLIB-COVERAGE.10.md) —
> bucket **C** (coverable gaps) plus bucket **D** (never ruled on) — and
> proposes a *home* for every part of it that is not Go-language-specific:
> which existing module absorbs it, which new module owns it, or which
> non-module seam (build flag, host provider, type) it belongs to.
> Read [go-modules/README.10.md](go-modules/README.10.md) first — its
> conventions are assumed throughout and not re-derived here.

## 1. Scope

"Remaining" means: **not in bucket A**. That is **24** packages from bucket
C and **49** from bucket D — 73 in all. Of those, this note allocates
**38** to a module or seam, recommends **34 be closed into bucket B** —
Go-language or Go-runtime mechanics with no meaning in a sealed query
language (§7) — and defers **1** (`crypto/mlkem`). It also proposes one
move in the other direction: `image/color` out of bucket B and into
`boru:tui` (§5.8).

Excluded by the brief ("unless Go-language specific") and therefore not
allocated: `cmp`, `iter`, `unique`, `weak`, `structs`, `io/ioutil`,
`regexp/syntax`, `text/template/parse`, `runtime/*`, `testing/*`, the
`crypto` / `encoding` / `hash` root interface packages, and
`database/sql/driver` **as a guest surface** (it returns as a host seam in
§6.3).

## 2. Allocation principles

Derived from what the repo already does, not invented here.

| # | Principle | Precedent |
|---|---|---|
| **P1** | **Consolidate into hubs.** Prefer folding into an existing module over minting one module per Go package. | `boru:bin-util` absorbed the whole crypto-hash/CRC/base-N family; `boru:string-util` absorbed `unicode` ([go-modules/README](go-modules/README.10.md) "Consolidation") |
| **P2** | **A module needs a *domain*, not a package.** A package with a two-or-three-function surface folds; it does not earn a namespace. | `net/smtp`, `hash/adler32` below |
| **P3** | **Types before words.** If a builtin type already models the value, the package's parsing/validation belongs to that type, not to a new module. | the twelve Micron leaves — `Ipon`, `Cidron`, `Mimon`, `Hoston`, `Coloron` already wrap `net`, `net/netip`, `mime` |
| **P4** | **Effects reuse the existing seams.** New *ops* inside a known `policy.KnownScopes` scope; a new scope only for a genuinely new kind of authority. | `fileops`/`network`/`process` in `lang/go/policy/policy.go` |
| **P5** | **Provider/driver packages become host seams, not modules.** Where the Go package *is* an interface for plugging things in, the boru answer is host registration. | `LogSinkSpec` ([LOG-MODULE](LOG-MODULE.10.md) §5), `RegisterHostKeyring` ([OS-KEYRING](OS-KEYRING.0.md)), `RegisterGoPackage` ([GO-MODULES](GO-MODULES.10.md)) |
| **P6** | **Naming.** Bare capitalised package name as the namespace; the `-util` id + `*Util` namespace **only** on a clash with a builtin type or an existing namespace. | `go-modules/README.10.md` "The roster" |
| **P7** | **Not everything becomes a word.** Some packages are build wiring or internal plumbing and correctly have no user surface at all. | `time/tzdata`, `net/textproto` below |

## 3. Summary — the whole allocation on one page

| Destination | Kind | Packages allocated |
|---|---|---|
| `boru:bin-util` | existing | `crypto/sha3`, `hash/adler32`, `hash/maphash`, `encoding/binary` |
| `boru:string-util` | existing | `unicode/utf8`, `unicode/utf16`, `index/suffixarray` |
| `boru:rand` | existing | `math/rand/v2` |
| `boru:net` | existing | `net.Lookup*` (DNS), `net/netip`, `net/http/cookiejar`, `net/http/httputil` (proxy), `mime/multipart` (HTTP forms), `crypto/tls` (as options) |
| `boru:io` | existing | `mime` (`TypeByExtension` → `Mimon`) |
| `boru:os` | designed | `os/user` |
| `boru:report` | existing | `text/tabwriter` |
| `boru:template` | designed | `html/template` (autoescape mode) |
| `boru:mail` | designed | `net/smtp`, `mime/multipart` (message parts) |
| `boru:tui` | existing | `image/color` (carve-out from bucket B) |
| `boru:log` | existing | `log/syslog` (host sink, not a word) |
| `boru:debug` | existing | `net/http/httputil` (dump helpers) |
| **`boru:crypto`** | designed — charter extended | `crypto/hkdf`, `crypto/pbkdf2`, `crypto/ecdh`, `crypto/{ed25519,ecdsa,rsa}` (sign/verify/keygen) |
| **`boru:pki`** | **new** | `crypto/x509`, `crypto/x509/pkix`, `encoding/pem`, `encoding/asn1` |
| **`boru:compress`** | **new** | `compress/{gzip,flate,zlib,bzip2,lzw}` |
| **`boru:archive`** | **new** | `archive/{tar,zip}` (value-level; read-mount stays in `boru:io`) |
| build wiring | non-module | `time/tzdata` |
| host seam | non-module | `database/sql` (Store backends), `crypto/tls` client identities (**shipped**: `RegisterClientIdentity`), `log/syslog` |
| internal only | non-module | `net/textproto` |
| **bucket B** | closed | 34 packages — §7 |

**Three new modules, one extended charter.** Everything else folds into a
module that already exists or is already designed. That ratio is the point
of P1: the remaining stdlib is mostly *holes in modules we already have*,
not missing modules.

## 4. New modules

### 4.1 `boru:pki` (`Pki`) — certificates and key documents

| | |
|---|---|
| **Packages** | `crypto/x509`, `crypto/x509/pkix`, `encoding/pem`, `encoding/asn1` |
| **Import / namespace** | `import "boru:pki"` → `Pki` (no clash; bare name per P6) |
| **Gate** | none for parsing/encoding (pure `Bytes`↔`Record`). Verification against the **host trust store** goes through a new `RootsProvider` capability seam (P4/P5); verification against an explicitly supplied pool stays pure. |
| **Owns** | PEM block encode/decode, DER/ASN.1 decode, certificate parse → `Record`, chain verification, CSR build/parse, public/private key document encoding. |
| **Does NOT own** | the cryptography itself — signing and key generation are `boru:crypto` words (§5.1). `Pki` reads and writes the *documents*; `Crypto` does the *math*. |

**Why a new module rather than folding into `boru:crypto`.** They are
different kinds of thing and they fail differently. `Crypto` is bytes and
keys — deterministic transforms with a small, sharp error vocabulary
([BORU-CRYPTO](BORU-CRYPTO.0.md) §9). `Pki` is *documents and trust* —
parsing attacker-controlled ASN.1, deciding whether a chain is acceptable,
handling expiry and name constraints. Merging them would put a
policy-shaped trust decision inside a module whose stated posture is "no
implicit sealing, no ambient authority". Splitting also lets a deployment
take certificate *parsing* (log analysis, inventory) without enabling any
signing surface.

**Why not fold PEM/ASN.1 into `boru:coding`.** `boru:coding` is
**text→text** escaping and encoding ([EXTENSION-MODULES](EXTENSION-MODULES.10.md)
§4). PEM is a `Bytes`-carrying container and ASN.1 is a binary structure
grammar; neither is a string escape. `Coding.encode base64` on the PEM
body is the shared primitive, and `Pki` delegates to it.

### 4.2 `boru:compress` (`Compress`) — byte codecs

| | |
|---|---|
| **Packages** | `compress/gzip`, `compress/flate`, `compress/zlib`, `compress/bzip2`, `compress/lzw` |
| **Import / namespace** | `import "boru:compress"` → `Compress` |
| **Gate** | none — pure `Bytes` → `Bytes`. A decompression **size cap** is an option, not a policy scope (same reasoning BORU-CRYPTO applies to scrypt cost: a resource concern, not a scope concern). |
| **Owns** | `deflate`/`inflate` dispatching on a codec Atom (`gzip/q`, `flate/q`, `zlib/q`, `bzip2/q`, `lzw/q`), mirroring the `parse <kind>` registry shape; streaming variants through the `boru:io` stream seam for data larger than memory. |
| **Note** | `bzip2` and `lzw`(read) are decompress-only in Go — the module must report that through `unsupported-op` rather than silently omitting words. |

**Why a module and not `boru:bin-util`.** `bin-util` is the *identity and
digest* hub — hashes, MACs, base-N, GUIDs. Compression is a different
contract: it is lossless and reversible, it needs a size-cap option and a
streaming form, and it is the one place a `Bytes` word can blow up memory
from untrusted input. Keeping it separate keeps that hazard visible.

### 4.3 `boru:archive` (`Archive`) — container files

| | |
|---|---|
| **Packages** | `archive/tar`, `archive/zip` |
| **Import / namespace** | `import "boru:archive"` → `Archive` |
| **Gate** | none on `Bytes`-in/`Bytes`-out. Reading from or writing to disk is done by handing the result to `boru:io`, which keeps the `fileops` gate in exactly one place. |
| **Owns** | entry listing → `Table`, per-entry extraction → `Bytes`, and **archive creation** (the half `boru:io` cannot do), with an optional `{compress: gzip/q}` that delegates to `boru:compress` so `.tar.gz` is one call. |
| **Overlap** | `boru:io` already mounts a zip **read-only as a filesystem** (`ZipFileOps`, shipped). That stays: `mount` is for *traversing* an archive as if it were a tree; `Archive` is for *values* — build one, list one, pull one entry out. The dividing line is the same as `boru:io` vs `boru:os` for files. |

## 5. Existing and already-designed modules

### 5.1 `boru:crypto` — charter extension (no new module)

[BORU-CRYPTO.0.md](BORU-CRYPTO.0.md) scopes itself to the vault's primitives:
AEAD, scrypt, HKDF, hashes, constant-time equality, CSPRNG. Three additions
follow from the Go 1.24 stdlib and from the largest remaining gap:

| Add | Package | Note |
|---|---|---|
| stdlib KDF paths | `crypto/hkdf`, `crypto/pbkdf2` | BORU-CRYPTO §12 wires `golang.org/x/crypto/hkdf`; Go 1.24 promoted both to stdlib. Straight substitution, no surface change. |
| key agreement | `crypto/ecdh` | [BORU-CRYPTO-EXTRA](BORU-CRYPTO-EXTRA.0.md) already argues the *shape* (a derived secret must pass through an explicit KDF step) but names no package. |
| **sign / verify / keygen** | `crypto/ed25519`, `crypto/ecdsa`, `crypto/rsa` | The biggest single hole in the whole stdlib map. Ed25519 first — one key size, no parameter choices, no padding modes. |

`crypto/sha3` deliberately goes to `bin-util` instead (§5.2): it is a hash,
and hashes are `bin-util`'s domain. `crypto/mlkem` is real but has no
current driver — leave it in bucket C, low priority.

### 5.2 `boru:bin-util` — the digest and byte-layout hub

| Add | Package | Word sketch |
|---|---|---|
| SHA-3 / SHAKE | `crypto/sha3` | Fills the gap the coverage map's `crypto/{sha*}` glob only *appeared* to cover — [BIN-UTIL](go-modules/BIN-UTIL.10.md) enumerates sha1/256/512 only. |
| Adler-32 | `hash/adler32` | Completes the checksum row beside CRC-32/64. |
| fast non-crypto hash | `hash/maphash` | For bucketing/sharding — must be documented as **not stable across runs** unless an explicit seed is supplied, or it breaks the determinism contract. |
| struct pack/unpack | `encoding/binary` | The natural extension of the [`Bytes`](go-modules/BYTES.10.md) type, whose note already models "a binary frame layout is a TYPE". `pack`/`unpack` take that layout type plus endianness. |

### 5.3 `boru:string-util` — text and text indexing

| Add | Package | Word sketch |
|---|---|---|
| UTF-8 | `unicode/utf8` | rune count, validity, rune/byte offset conversion — the missing companion to the Unicode classification already designed in [STRING-UTIL](go-modules/STRING-UTIL.10.md). |
| UTF-16 | `unicode/utf16` | encode/decode for interop (already bucket C, unallocated until now). |
| substring index | `index/suffixarray` | Build an index once, query many times. Needs a module-minted opaque type `StringUtil.Index` (the `IO.StreamKind` pattern). The one entry here that is a genuinely *new capability* rather than a gap-fill: a query language that can answer "every occurrence, across a large corpus" without rescanning. |

### 5.4 `boru:net` — protocol completeness

| Add | Package | Word / shape |
|---|---|---|
| DNS | `net.Lookup*` | `Net.lookup` — host→addresses, plus MX/TXT/SRV. New op `dns` inside the existing `network` scope (P4). The single most conspicuous hole: sockets ship, but nothing resolves a name. |
| address arithmetic | `net/netip` | Operations over the **existing** `Ipon`/`Cidron` Micron leaves — `contains`, `overlaps`, `mask`, `range`, `next`. P3: the types exist and `Cidron` is *already* ordered by `net/netip`; only the verbs are missing. |
| cookies | `net/http/cookiejar` | A client option (`{jar: …}`), not a value type — sessions are the common case and today every request is stateless. |
| reverse proxy | `net/http/httputil` | `Net.proxy` for [SERVICES](SERVICES.0.md); the dump helpers go to `boru:debug` (§5.8). |
| HTTP forms | `mime/multipart` | Encode/decode multipart form bodies — file upload. Shares its Go core with `boru:mail` (§5.7); two word surfaces, one implementation. |
| TLS | `crypto/tls` | **SHIPPED** ([NETWORK-TLS-PLAN](legacy/NETWORK-TLS-PLAN.0.ignore) phases 1-4). Options on an existing verb, never a `TlsConfig` value: `fetch`/`connect-raw` take `tls: {ca: … verify: … sni: … min: … identity: …}`. CA roots arrive as `Bytes`; the client **private key** does NOT — it stays behind a host-registered `ClientIdentity` interface, since a credential modelled as bytes cannot express an HSM- or agent-backed key. Exposing `tls.Config` as a guest value would put a security-critical struct behind gradual typing — exactly what the sealed model exists to avoid. |

`boru:net` absorbs a lot here. It stays coherent because every entry is
"the client or server does X" — the module's existing charter. The two
things that are *not* about a connection (message formats, certificate
documents) go elsewhere, to `boru:mail` and `boru:pki`.

### 5.5 `boru:io` — one addition

`mime.TypeByExtension` / `ExtensionsByType` → `IO.content-type`
(`Pathon → Mimon`) and its inverse. P3 again: `Mimon` already exists and
already parses media types via `mime.ParseMediaType` in `eng/go/micron.go`;
what is missing is the extension lookup, and the question "what type is
this file" sits next to `stat`. This is a pure word in an otherwise
effectful module — call that out in its doc rather than minting a
`boru:mime` module for one lookup table (P2).

### 5.6 `boru:os`, `boru:report`, `boru:template` — one each

| Module | Add | Note |
|---|---|---|
| `boru:os` ([OS](go-modules/OS.10.md)) | `os/user` | Current user / lookup by name. Gates on the `env` scope the module already uses for identity words. |
| `boru:report` | `text/tabwriter` | Column alignment. The coverage map already flagged the overlap; the resolution is that `tabwriter` becomes an *implementation* of a `Report` layout, not its own word set. |
| `boru:template` ([TEXT-TEMPLATE](go-modules/TEXT-TEMPLATE.10.md)) | `html/template` | A `{html: true}` mode, not a second module — same template language, contextual auto-escaping switched on, delegating to `boru:coding` for the escape kinds. Defaulting HTML rendering to the *escaping* engine is the safe default and should be stated as such. |

### 5.7 `boru:mail` — charter extension

The designed `boru:mail` ([NET-MAIL](go-modules/NET-MAIL.10.md), flagged
"niche") wraps `net/mail` — parsing addresses and headers. Two additions
make it non-niche:

- `net/smtp` → `Mail.send`. Gated `network` (new op `smtp`). Three Go
  functions; a standalone `boru:smtp` module would fail P2.
- `mime/multipart` (message side) → MIME part walk/build, so a parsed mail
  yields its attachments. Same Go core as the HTTP-form words in
  `boru:net`; the note must state the dividing line (`Net` does form
  bodies, `Mail` does message parts).

### 5.8 `boru:tui` and `boru:debug`

- **`boru:tui` ← `image/color`.** A deliberate carve-out from the bucket-B
  "Graphics" row, which excludes `image/*` on the grounds that boru is not
  a rendering toolkit. `image/color` is not rendering — it is colour-space
  arithmetic, and boru already has a `Coloron` Micron leaf plus a
  `colorize`/`truecolor` path in `tui_utils.go` that needs sRGB→256-colour
  mapping. Recommend narrowing the bucket-B row to `image/*` **except**
  `image/color`. (If `Coloron` maths later wants a home outside a terminal
  module, a small `boru:color` is the fallback — noted, not recommended.)
- **`boru:debug` ← `net/http/httputil`** `DumpRequest`/`DumpResponse`, which
  is a debugging surface, not a client feature.

### 5.9 `boru:log` ← `log/syslog` (as a sink, not a word)

P5. [LOG-MODULE](LOG-MODULE.10.md) §5 already defines a host sink registry
(`LogSinkSpec`); syslog is a *provider* for it, wired host-side. No guest
word, no new policy scope — it inherits the `log` scope.

## 6. Non-module allocations

### 6.1 `time/tzdata` → a build decision

Not a word at all (P7). It is a blank import that embeds the IANA database
in the binary. Today `boru:time-util` zone handling depends on host tz files,
which contradicts the single-binary, deterministic posture: the *same*
program gives different answers on a host with a stale or absent zoneinfo.
Recommend a `cmd/go` build tag (`timetzdata`), on by default for released
binaries, with the tzdata version reported by `boru describe`/`boru version`
so the embedded data is auditable.

### 6.2 `net/textproto` → internal, never exposed

Header/dot-encoding plumbing consumed by `boru:mail` and `boru:net`. It is a
Go implementation detail of protocols we already expose. → bucket B.

### 6.3 `database/sql` → additional **Store backends**, not a SQL module

The coverage map's "DB drivers" row assumes boru wants a SQL surface. It
does not, and the code says so: `lang/go/native/sqlite.go` is a
`SQLiteStore` — a *backend* behind boru's own `Store`/`Table` query words —
not a `Db.query` module. So the correct allocation for other backends
(Postgres, MySQL, …) is **another implementation of that same seam**,
host-registered per P5, reusing the `sqlite` policy scope generalised to
`store`. Guests keep writing boru queries; only the host chooses where they
execute. This is strictly better than exposing `database/sql`: it keeps
the sealed model, avoids putting driver-specific SQL dialects into guest
source, and needs no new module.

## 7. Recommended closures into bucket B (34)

Allocating the remainder is only half the job; the other half is *closing*
the residue so it stops being re-derived. Recommend bucket B for:

| Group | Packages | Why |
|---|---|---|
| Go language mechanics | `cmp`, `iter`, `unique`, `weak`, `structs` | Go-side ordering, iteration, interning and memory layout. boru has its own `cmp`, its own iteration model, and its own value identity. |
| Deprecated / superseded Go APIs | `io/ioutil`, `crypto/elliptic`, `crypto/dsa` | Superseded within Go itself. |
| Broken primitives | `crypto/des`, `crypto/rc4` | Exclude **deliberately** — a security-sensitive module should refuse these by design, not omit them by accident. |
| Build modes / interface roots | `crypto/fips140`, `crypto`, `encoding`, `hash` | A build toggle and three interface-only registries — nothing callable. |
| Implementation-detail sub-packages | `regexp/syntax`, `text/template/parse`, `database/sql/driver` | ASTs and driver interfaces behind a surface that is already bucketed. (`go/build/constraint`, `go/doc/comment` and `image/color/palette` belong here too but are already swept by the `go/*` and `image/*` rows, so they are not counted again.) |
| Host / runtime internals | `runtime/{cgo,coverage,metrics,race}`, `net/http/{cgi,fcgi,pprof}` | Same rationale as the existing bucket-B `runtime/{pprof,trace,debug}` row: host concerns, not language ones. |
| Go-side test harnesses | `testing/{fstest,iotest,quick,slogtest}`, `net/http/{httptest,httptrace}` | boru has `boru:test`, its own PBT ([PBT-PLAN](legacy/PBT-PLAN.10.ignore)) and its own FS seam (`overlay.go`, `zipfs.go`). Worth an explicit *superseded* ruling rather than silence. |
| Superseded by boru's own stack | `text/scanner`, `net/rpc`, `net/rpc/jsonrpc`, `net/textproto` | Tokenizing is `boru:parse`/`boru:minilang`; RPC is [SERVICES](SERVICES.0.md) with its own codecs. |

Deferred rather than closed: `crypto/mlkem` (real, no driver yet) stays in
bucket C at low priority.

## 8. Sequencing

Ordered by ratio of value to risk, not by size.

| Phase | Work | Rationale |
|---|---|---|
| **1 — gap-fill** | `bin-util` (sha3, adler32, maphash, `encoding/binary`), `string-util` (utf8, utf16), `rand` (v2), `os` (`os/user`), `report` (tabwriter), `io` (`content-type`), tzdata build tag | Words inside modules that already exist, with docs/spec/coverage plumbing in place. Cheapest possible closure of the long tail. |
| **2 — net completeness** | DNS `lookup`, `netip` verbs, cookie jar, multipart forms | Holes in a *shipped* module. DNS first: sockets without name resolution is the most visible incoherence in the current surface. |
| **3 — pure new modules** | `boru:compress`, then `boru:archive` | Self-contained, no policy design, no capability seam. Good first exercises of the new-module checklist in `go-modules/README.10.md`. |
| **4 — security** | `boru:crypto` extension (hkdf/pbkdf2/ecdh, then ed25519 sign/verify) → `boru:pki` → TLS options on `boru:net` | The largest gap and the highest risk; strictly ordered because each layer consumes the previous one. Ed25519 before ECDSA/RSA — no parameter choices to get wrong. |
| **5 — long tail** | `boru:mail` + smtp, `template` html mode, `suffixarray` index, `httputil` proxy/dump, syslog sink, extra Store backends | Independent of each other; schedule against demand. |

## 9. Open questions

1. **`boru:compress` + `boru:archive` as one module?** Kept separate here
   (codec vs container, matching Go). A single `boru:pack` with both would
   be more consolidated per P1; the `.tar.gz` path is the argument for it.
2. **Where does `Coloron` arithmetic live** if a non-terminal consumer
   appears — stay in `boru:tui`, or promote to `boru:color`?
3. **Does `Pki` verification need a policy scope**, or is the
   `RootsProvider` capability seam sufficient? Reading the OS trust store
   is a host-authority act, and no current scope covers it.
4. **Decompression caps**: option-per-call, or a module-level ceiling? A
   zip bomb through `Archive` + `Compress` is the one denial-of-service
   path these modules introduce.
5. **`hash/maphash` determinism.** Per-process seeding conflicts with the
   determinism contract; is a mandatory explicit seed acceptable, or should
   the package be closed into bucket B instead?
6. **Generalising the `sqlite` policy scope** to `store` (§6.3) is a
   rename of a `policy.KnownScopes` entry — needs a compatibility call.
