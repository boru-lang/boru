# Go stdlib coverage map & non-goals

> **Status: living reference.** Maps the Go standard library against boru's
> module surface — what is covered (shipped or designed), what boru
> **deliberately will not cover** (non-goals), and what remains a
> **coverable gap** (fits boru's model, not yet designed). Read
> [NATIVE-MODULES.10.md](NATIVE-MODULES.10.md),
> [GO-MODULES.10.md](GO-MODULES.10.md),
> [go-modules/README.10.md](go-modules/README.10.md), and
> [EXTENSION-MODULES.10.md](EXTENSION-MODULES.10.md) first.
>
> For a proposed *home* for everything still in buckets C and D — which
> module absorbs it, which new module owns it, which seam it belongs to —
> see [STDLIB-ALLOCATION.0.md](STDLIB-ALLOCATION.0.md).

## Framing

boru is a **sealed, deterministic, single-threaded data/query language**
(no runtime `reflect`/`plugin`, one engine per registry, host effects
behind capability seams). That model — not feature parity with Go — is
what decides whether a stdlib package belongs. This note records the
boundary so it stays auditable as modules land.

Four buckets:

- **A. Covered** — shipped or specified by a design note.
- **B. Non-goals** — against boru's nature; intentionally never wrapped.
- **C. Coverable gaps** — fit the model, simply not designed yet.
- **D. Not yet considered** — absent from A, B **and** C: no design note,
  no non-goal ruling, no gap entry. §D is the audit residue, not a
  roadmap.

Buckets A–C are audited against `go list std` on **Go 1.24** — 172
importable packages once `internal/…` and `vendor/…` are dropped. 123 of
those land in A, B or C; the remaining **49 have never been ruled on** and
are listed in §D.

## A. Covered (shipped or designed)

| Go package(s) | boru home | Status |
|---|---|---|
| `math`, `math/big`, `math/bits` | `boru:math-util`, `boru:bin-util` | shipped |
| `math/cmplx`, `math/big` (rationals) | number systems (`Arith` seam) | designed — [EXTENSION-MODULES](EXTENSION-MODULES.10.md) §6 |
| `math/rand` | `boru:rand` | shipped |
| `strings`, `regexp`, `unicode` | `boru:string-util` (+ Unicode classification) | shipped + designed — [STRING-UTIL](go-modules/STRING-UTIL.10.md) |
| `strconv` | `boru:strconv` | designed — [STRCONV](go-modules/STRCONV.10.md) |
| `fmt` | `boru:fmt` (+ core `format`, template strings) | designed — [FMT](go-modules/FMT.10.md) |
| `time` | `boru:time-util` | shipped |
| `sort` | core `sort` + `boru:array-util` | shipped |
| `bytes` | the `Bytes` type | designed — [BYTES](go-modules/BYTES.10.md) |
| `crypto/{sha*,md5,hmac,rand}`, `hash/{fnv,crc32,crc64}` | `boru:bin-util` | designed — [BIN-UTIL](go-modules/BIN-UTIL.10.md) |
| `encoding/{hex,base32,base64,ascii85}` (binary) | `boru:bin-util` | designed |
| `encoding/{json,xml,csv}`, yaml/toml/ini/… | `boru:parselang` / `boru:emitlang` / `boru:csv` | shipped + designed |
| `html` (escape), `net/url` (escape), text escapes, `mime/quotedprintable` | `boru:coding` | designed — [EXTENSION-MODULES](EXTENSION-MODULES.10.md) §4 |
| `net/mail`, message/header micro-formats | `boru:micro-format` | designed — [EXTENSION-MODULES](EXTENSION-MODULES.10.md) §5 |
| `net/http`, `net/url` (structure) | `boru:net`, `boru:url` | shipped + designed — [NET-URL](go-modules/NET-URL.10.md) |
| `net` (TCP/UDP sockets) | `boru:net` (`listen`/`accept`/`connect-raw`/`serve-raw`/`recv*`/`send-bytes`/`shutdown`/`peer`) | shipped — `lang/go/modules/net_socket.go`, per [NETWORK-SERVERS](NETWORK-SERVERS.0.md) §4 |
| `archive/zip` (read) | `boru:io` `mount` (a zip mounts as a read-only `FileOps` backend) | shipped — `lang/go/capabilities/zipfs.go` |
| `log`, `log/slog` | `boru:log` | shipped — [LOG-MODULE](LOG-MODULE.10.md) (phases 1–5) |
| `crypto/{aes,cipher,subtle}`, KDFs, `crypto/rand` | `boru:crypto` | designed — [BORU-CRYPTO](BORU-CRYPTO.0.md), [BORU-CRYPTO-EXTRA](BORU-CRYPTO-EXTRA.0.md) |
| `crypto/tls`, `crypto/x509` (verification only) | `boru:net` — `tls: {…}` options on `fetch` / `connect-raw`, incl. mutual TLS via host-registered identities | shipped — [NETWORK-TLS-PLAN](legacy/NETWORK-TLS-PLAN.0.ignore) phases 1-4 |
| `io`, `io/fs`, all `os` filesystem | `boru:io` (all filesystem) | shipped + designed — [IO](go-modules/IO.10.md) |
| `os` (env/args/identity/exit) | `boru:os` | designed — [OS](go-modules/OS.10.md) |
| `os/exec` (run a command, capture output) | `boru:exec` | designed — [EXEC](go-modules/EXEC.10.md) |
| `runtime` (GOOS/GOARCH/NumCPU/…) | `boru:runtime` | designed — [RUNTIME](go-modules/RUNTIME.10.md) |
| `path`, `path/filepath` | `boru:path-util`, `boru:filepath` | designed |
| `text/template` | `boru:template` | designed — [TEXT-TEMPLATE](go-modules/TEXT-TEMPLATE.10.md) |
| `slices`, `maps` | `boru:array-util`, `boru:struct-util` | shipped |
| `testing` | `boru:test` | shipped |
| `context` (cancellation/deadlines) | `boru:time-util` async (timeout/await/cancel) | shipped (capability, not the package) |

## B. Non-goals — boru deliberately will **not** cover

These serve Go-the-systems-language; they have no meaning in a sealed
query language. The reflection bridge ([GO-MODULES.10.md](GO-MODULES.10.md))
remains the escape hatch if a host ever truly needs one ad hoc.

| Area | Packages | Why excluded |
|---|---|---|
| Go toolchain / introspection | `go/*`, `debug/*`, `reflect`, `unsafe`, `embed` | Compiler/AST/unsafe-memory/build features. `reflect` is used *inside* the bridge, never exposed. |
| Dynamic loading | `plugin` | Breaks the sealed, deterministic model (rejected in GO-MODULES). |
| Concurrency primitives | `sync`, `sync/atomic`, `syscall`, `golang.org/x/sys` | boru concurrency is the `time-util` async model; raw goroutines/mutexes/syscalls are not a query-language concern (one engine per registry). |
| Go-specific serialization / errors | `encoding/gob`, `errors` | `gob` is a Go-only wire format; boru has its own `Error` type and value model. |
| Native collections (already built in) | `container/{heap,list,ring}` | boru has `List`/`Map`/`Table`/`Record` + `array-util`/`struct-util`. |
| Host / runtime internals | `runtime/{pprof,trace,debug}`, `expvar`, `os/signal`, `flag` | Profiling, signals, metrics export, CLI-flag parsing belong to the *host* (`cmd/go`), not the language. |
| Graphics | `image/*` | boru is a data/query language, not a rendering toolkit. |
| Low-level interface plumbing | exposed `io.Reader`/`Writer`, `bufio` | Streaming stays behind the `io`/capability seam; not a user-facing surface. |

## C. Coverable gaps — fit the model, not yet designed

Plausible future modules, roughly in priority order. None conflicts with
the model; each would follow the standard module conventions (and, where
effectful, a capability seam + policy scope).

| Candidate | Go packages | Notes / priority |
|---|---|---|
| **signing / PKI** | `crypto/{rsa,ecdsa,ed25519,x509}`, `encoding/{pem,asn1}` | **Biggest gap.** [BORU-CRYPTO](BORU-CRYPTO.0.md) designs symmetric AEAD, KDFs, hashes and `box` only — **signatures and certificate handling remain uncovered**. (TLS itself graduated to bucket A: `boru:net` verifies chains and presents client certificates, but exposes no certificate *documents* — that is what `boru:pki` would add.) |
| **`boru:archive`** (tar; zip write) | `archive/{tar,zip}` | Zip *read* ships as a `boru:io` mount; `tar` and archive *creation* are gaps. Pairs with `Bytes`/`io`. |
| **`boru:compress`** | `compress/{gzip,flate,zlib,bzip2}` | Byte→byte; sits beside `bin-util`. |
| **DNS / mail transport** | `net.Lookup*` (DNS), `net/smtp`, `net/textproto` | Sockets ship (bucket A); name resolution and SMTP do not — no `lookup`-family word exists (security-sensitive, capability-gated). |
| **`boru:mime`** | `mime`, `mime/multipart` | Content-type detection + multipart form uploads (HTTP-adjacent). |
| html auto-escaping | `html/template` | `text/template` is covered; the *contextual auto-escaping* variant would build on `boru:coding`. |
| DB drivers | `database/sql` + non-sqlite drivers | boru has native sqlite (`lang/go/native/sqlite.go`); other backends uncovered. |
| misc text/encoding | `encoding/binary` (struct pack/unpack), `unicode/utf16`, `text/tabwriter`, `os/user`, `hash/adler32` | Small/niche; `encoding/binary` extends `Bytes`; `tabwriter` overlaps `boru:report`. |

## D. Not yet considered — never ruled on

The 49 packages below appear in **no** bucket above and in no other design
note: they have neither been claimed, excluded, nor logged as a gap. Each
row carries a **suggested** destination bucket so the residue can be
retired by decision rather than by re-derivation; the suggestion is
advisory until someone moves the row.
[STDLIB-ALLOCATION.0.md](STDLIB-ALLOCATION.0.md) works every §C and §D
row through to a destination module or seam.

### D.1 Post-map Go additions (1.21 → 1.24)

The map's families predate these; nothing points at them.

| Package | Since | Note | Suggest |
|---|---|---|---|
| `cmp` | 1.21 | Go's `Ordered`/`Compare` helpers. boru has its own `cmp` word and type ordering ([TYPE-ORDERING](TYPE-ORDERING.10.md)). | B |
| `iter` | 1.23 | Range-over-func iterators — a Go *language* mechanism; boru iterates with `each`/`fold` and [STREAM-WORDS](STREAM-WORDS.0.md). | B |
| `unique` | 1.23 | Value interning. Runtime mechanic, no guest surface (already used internally by `lang/go/modules/docs.go`). | B |
| `weak` | 1.24 | Weak pointers. Runtime mechanic (already used internally by `eng/go/weak_flex.go`). | B |
| `structs` | 1.24 | `HostLayout` marker — pure Go memory layout. | B |
| `hash/maphash` | 1.19 | Fast non-cryptographic hashing of strings/bytes; the `hash/{fnv,crc32,crc64}` row stops short of it. | C — `boru:bin-util` |
| `math/rand/v2` | 1.22 | The modern PRNG. `boru:rand` wraps v1; only [BATTERIES-INCLUDED](legacy/BATTERIES-INCLUDED-REPORT.5.ignore) (historical) names v2. | C — `boru:rand` |
| `crypto/sha3` | 1.24 | Nominally swept by this map's `crypto/{sha*}` glob but **never enumerated** — [BIN-UTIL](go-modules/BIN-UTIL.10.md) lists only sha1/256/512. | C — `boru:bin-util` |
| `crypto/hkdf` | 1.24 | [BORU-CRYPTO](BORU-CRYPTO.0.md) §12 cites `golang.org/x/crypto/hkdf`; the stdlib promotion supersedes it. | C — `boru:crypto` |
| `crypto/pbkdf2` | 1.24 | [BORU-CRYPTO-EXTRA](BORU-CRYPTO-EXTRA.0.md) names PBKDF2 as a candidate KDF but no package. | C — `boru:crypto` |
| `crypto/ecdh` | 1.20 | BORU-CRYPTO-EXTRA discusses key agreement without naming a package. | C — `boru:crypto` |
| `crypto/mlkem` | 1.24 | Post-quantum KEM. Future-facing; no current driver. | C (low) |
| `crypto/fips140` | 1.24 | A build/runtime mode toggle, not a callable surface. | B |
| `testing/slogtest` | 1.21 | Validates `slog` handlers — a Go-side harness for `boru:log` providers, not a guest word. | B |

### D.2 Whole packages with no mention anywhere

| Package | Note | Suggest |
|---|---|---|
| `time/tzdata` | Embeds the IANA tz database in the binary. Directly relevant: a **sealed single-binary `boru`** otherwise depends on host tz files for `boru:time-util` zone handling. Currently only reached from one test. | C — `boru:time-util` |
| `index/suffixarray` | Substring index over a corpus — a natural fit for a *query* language, and unlike anything `string-util` offers. | C |
| `text/scanner` | Tokenizer for Go-like syntax; overlaps `boru:parse` / [MINILANG](MINILANG.5.md). | C (low) |
| `compress/lzw` | The one codec missing from the bucket-C `compress/{gzip,flate,zlib,bzip2}` group. | C — `boru:compress` |
| `log/syslog` | Unix syslog client — a plausible `boru:log` provider hook ([LOG-MODULE](LOG-MODULE.10.md) §5 sink registry). | C (low) |
| `net/rpc`, `net/rpc/jsonrpc` | Go-specific RPC wire format; boru has its own codec/service model ([SERVICES](SERVICES.0.md)). | B |
| `io/ioutil` | Deprecated alias shim for `io`/`os`. | B |
| `crypto`, `encoding`, `hash` (roots) | Interface/registry packages with no callable surface of their own. | B |

### D.3 Sub-packages of a considered parent, never named

The parent is bucketed; these were never individually ruled on.

| Packages | Parent | Note | Suggest |
|---|---|---|---|
| `net/http/cookiejar` | `net/http` (A) | Session-cookie persistence for the `boru:net` client — a real gap in the HTTP surface. | C — `boru:net` |
| `net/http/httputil` | `net/http` (A) | Reverse proxy + request/response dump; pairs with [SERVICES](SERVICES.0.md). | C — `boru:net` |
| `net/netip` | `net` (A) | Typed IP/prefix values. boru already **has** a `Cidron` type ordered by `net/netip`, but no address-manipulation words. | C — `boru:net` |
| `net/http/{cgi,fcgi,pprof}` | `net/http` (A) | Host deployment / profiling endpoints — same rationale as `runtime/pprof`. | B |
| `net/http/{httptest,httptrace}` | `net/http` (A) | Go-side test harness and connection tracing. | B |
| `testing/quick` | `testing` (A) | Property-based testing — boru already has its own PBT ([PBT-PLAN](legacy/PBT-PLAN.10.ignore)); worth an explicit "superseded" ruling. | B (superseded) |
| `testing/fstest` | `testing` (A) | In-memory FS. boru's equivalent is the `FileOps` capability seam (`overlay.go`, `zipfs.go`) — likewise worth an explicit ruling. | B (superseded) |
| `testing/iotest` | `testing` (A) | Go `io` error-injection readers. | B |
| `runtime/{cgo,coverage,metrics,race}` | `runtime` (A) / `runtime/{pprof,trace,debug}` (B) | Host/runtime internals; same rationale as the bucket-B runtime row. | B |
| `crypto/{des,rc4}` | crypto (A/C) | Broken/legacy ciphers — exclude **deliberately**, not by omission. | B |
| `crypto/{dsa,elliptic}` | crypto (A/C) | Deprecated by Go in favour of `ecdsa`/`ecdh`. | B |
| `crypto/x509/pkix` | `crypto/x509` (C) | ASN.1 structures for the x509 row. | C (with x509) |
| `database/sql/driver` | `database/sql` (C) | Driver-author interface, not a guest surface. | B |
| `regexp/syntax` | `regexp` (A) | Regex AST — implementation detail. | B |
| `text/template/parse` | `text/template` (C) | Template AST — implementation detail. | B |
| `go/{build/constraint,doc/comment}`, `image/color/palette` | `go/*`, `image/*` (B) | Already swept by their parent's wildcard; listed for completeness. | B |
| `unicode/utf8` | `unicode` (A) | Rune counting / validity. `unicode/utf16` is named in bucket C but utf8 never was. | C — `boru:string-util` |

## Bottom line

The **"won't cover"** boundary is bucket B — Go's systems/runtime/
toolchain machinery, intentionally invisible to a sealed query language.
Everything in bucket C fits boru's grain; the only sizeable, broadly-useful
omission from current designs is **signing / PKI / TLS**, since
[BORU-CRYPTO](BORU-CRYPTO.0.md) stopped at symmetric AEAD and KDFs. After
that, **archive / compress** and **DNS / SMTP** are the next most
defensible additions (`os/exec` graduated to bucket A —
[EXEC](go-modules/EXEC.10.md); sockets, zip-read and `boru:log` likewise).

From §D, the entries that most deserve promotion to C on their merits are
**`time/tzdata`** (a sealed binary should not need host tz files),
**`net/http/cookiejar`** + **`net/netip`** (concrete holes in a shipped
module), **`crypto/{sha3,hkdf,pbkdf2,ecdh}`** (Go 1.24 promotions the
crypto notes predate), and **`math/rand/v2`**. The rest of §D is expected
to retire into B.
