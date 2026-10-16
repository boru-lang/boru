# The `Bytes` type — a first-class binary leaf + bit-syntax

> **Status: design proposal, not implemented.** This note specifies a new
> boru value type, `Bytes`, the foundation for all binary-adjacent
> functionality, together with its **storage & memory model** (§4) and the
> **`make`/`unpack` bit-syntax** (§7) that binary wire protocols are written
> against. The heavier binary words (cryptographic hashes, HMAC,
> CRC, base/hex encodings, secure random, UUIDs) live in the expanded
> `boru:bin-util` module — see [BIN-UTIL.10.md](BIN-UTIL.10.md). The type, its
> literal, its interop overloads, and the bit-syntax are **core** (no import).
> Read [README.10.md](README.10.md) for the shared conventions.
>
> **Decisions taken at design-review time:** (1) this doc is the **authoritative
> full `Bytes` design** (the earlier type-only draft is folded in); (2) `Bytes`
> is a **plain binary Scalar** — its value operations are signature overloads of
> existing words (`convert` text/ints ⇄ Bytes + compact, `slice`, `add`, and
> `size`/`cmp`/`eq` via behaviors); a **binary frame layout is a TYPE**, modelled
> on the class/object machinery: **`BinarySpec : Binary :: Class : Object`** —
> `BinarySpec` is the spec **kind**, a layout refines it into a sealed class
> (`def Header (refine BinarySpec [layout])`), `make Header {fields}` builds a
> field-accessible **`Binary` INSTANCE** (like an object instance: `p.ver`,
> `typeof p → Header`), `convert Bytes <inst>` serialises the instance to wire
> `Bytes`, and `unpack` / `unpack-prefix` decode wire `Bytes` back into an
> instance (§7) — there is no `make Bytes [spec]` data-vs-spec overload and no
> `bytes` word; crypto/encoding/hash/uuid stay in `boru:bin-util`; (3) hex/binary
> byte constants are the **`+hb/…/` and `+bb/…/` minilang kinds**
> (`import "boru:minilang"`), **not** a core lexer literal — there is no `0x"…"`
> or `b"…"` form (see §6); (4) the memory model is **zero-copy share +
> copy-on-ingest** (this supersedes the earlier `DeepCloner` decision — see §4).
> The layout-as-type and no-secondary-parsing decisions are recorded in
> **ADR-007**.
>
> Used by `NETWORK-SERVERS.0.md` §3, `NETWORK-CLIENTS.0.md` §3, and
> `STREAM-WORDS.0.md` (`from-bytes`/`to-bytes`), which now reference this doc
> rather than sketching the type inline.

## 1. Why a type (not List[Integer] or String)

boru has no binary representation today: file reads degrade to `String`
(UTF-8), and `core.FromNative`/`ToNative` (`core/go/gobridge.go`) have **no
`[]byte` case** — a Go byte slice falls through to a best-effort
`fmt.Sprintf` string. Every binary operation (hashing, base64/hex, HMAC, CRC,
UUID bytes, secure random) is defined over `[]byte`, and — newly — **network
framing** (`NETWORK-SERVERS.0.md`) reads and writes raw octets off a socket.
A coherent surface needs one shared, lossless, type-safe carrier.

The two cheaper models were rejected:

- **`List[Integer]`** — workable but verbose, allocation-heavy (one
  `IntPayload` per byte — 100s of bytes of overhead per KB of data), and
  ambiguous (any list of small ints *looks* like bytes). It cannot carry a
  Comparer/Format that reads as binary, and it defeats the zero-copy messaging
  (§4) that the actor/service layers depend on.
- **`String`** — composes with file reads but is lossy for non-UTF-8 data and
  conflates text with bytes.

A real `Bytes` leaf makes binary **first-class and boru-native**: `x is Bytes`
works, values render as hex, comparison is byte-lexicographic, `[]byte`
round-trips through the Go bridge unchanged, and — because the backing slice is
never mutated in place (§4) — a `Bytes` is **shared zero-copy** across forks,
sends, and slices, exactly like a `String`. That last property is what lets a
parsed frame flow from a socket through a handler and into a reply with no
intermediate copy (`SERVICES.0.md` §7.1).

## 2. Registration

`Bytes` is a **global external builtin**, registered in a new
`lang/go/native/native_bytes.go`, mirroring the established external-type
pattern used for `Time`, the fetch result, `Timeout`, and `Interval`
(`RegisterExternalBuiltin` with a stable `FixedID`):

- **FixedID** — allocate the next free value in the `lang/go/native` scalar
  band **1000–1999** (`core/go/CLAUDE.md` "FixedID Allocation"; document the
  exact number chosen) and pin it in `lang/go/test/fixedid_stability_test.go`.
  (Not the 10000+ host range — this is a language builtin, not a host plugin
  type.)
- **No-panic init** — registration records any error via the `TypeInitError`
  path checked at `DefaultRegistryWithPolicy`, per ADR-005 (`core/go/CLAUDE.md`
  "Panic Prevention" — no init-time panic).
- Path/name: `Scalar/Bytes` — a concrete **Scalar** data leaf (sibling of
  `String` and the `Scalar/Time` family, so `x is Scalar` holds, and the
  `Scalar/…` prefix agrees with the 1000–1999 band above), exported so core and
  `BinUtil`-built values type as `Bytes` and `x is Bytes` resolves.

## 3. Payload & behavior

- **Payload** — `ExtensionPayload{Body: bytesBody{data []byte}}` via
  `core.NewExtension` (`core/go/payload.go`), so **no kernel struct change** is
  required; the slice is the whole state.
- **`TypeBehavior`** (the seam external types implement):
  - `Format` → hex, length-capped, e.g. `Bytes<68 65 6c 6c 6f>` (short) and
    `Bytes<00 01 02 03 04 … (1024)>` (truncated with the length shown). The full
    value is obtained with `BinUtil.hex-encode` / `base64-encode`.
  - `Comparer` → byte-lexicographic (`bytes.Compare`), opening with the
    `litVsConcreteOrder` rule so the bare `Bytes` literal sorts below every
    concrete value (`design/TYPE-ORDERING.10.md`,
    `core/go/compare_scalar_behaviors.go`).
  - `Equal` → bytewise (`bytes.Equal`). (HMAC verification uses a separate
    *constant-time* compare — see BIN-UTIL.10.md — not this `Equal`.)
  - `Sizer` → byte count, so the generic `length` / `size` word works on
    `Bytes` with no dedicated word (`core/go/typebehavior.go` optional
    capabilities).
  - **No `DeepCloner`.** `Bytes` is immutable; clone shares the slice — see §4.

## 4. Storage & memory model

This is the load-bearing section: `Bytes` is designed so that the common
network/data paths copy **once, at ingest, and never again.**

### 4.1 Immutable, shared, zero-copy

The backing store is a single Go `[]byte` inside `bytesBody`. **No word mutates
a `Bytes` in place** — every operation (`slice`, `add`, `convert Bytes <inst>`, …) returns a
fresh value. Because the storage is therefore never written after construction,
a `Bytes` is **shared by reference on clone** and pays nothing:

- `CloneValue` (`core/go/clone.go`) shares an `ExtensionPayload` whose `Body`
  does **not** implement `DeepCloner`. `bytesBody` deliberately omits it, so a
  cloned `Bytes` shares the original's slice header and backing array — exactly
  how `StrPayload` (an immutable Go `string`) is shared.
- This is what makes `PROCESSES.0.md` §6 / `SERVICES.0.md` §7.1 **zero-copy
  messaging** real for binary: `send`ing a `Bytes` to another process, or
  `ForkConcurrent`-copying a registry that holds one, copies a slice header
  (24 bytes), not the payload. A 1 MB frame received on a socket reaches a
  handler in another goroutine with zero byte-copies.
- The Go memory model's happens-before edge on the mailbox send, plus the
  no-writer guarantee from immutability, means the receiver observes a fully
  published, race-free value (`SERVICES.0.md` §7.1).

> This **supersedes** the earlier draft's `DeepCloner` choice. Deep-cloning an
> immutable value on every fork/send is pure cost with no safety benefit; the
> only place a copy is genuinely needed is at the *ingest* boundary (§4.2).

### 4.2 Copy-on-ingest at trust boundaries

The one place a defensive copy is mandatory is where a `[]byte` enters boru from
code that may still mutate it:

- **`core.FromNative([]byte)`** copies the incoming slice before wrapping it. A
  Go caller (or a reused buffer) must not be able to mutate a live `Bytes`.
- **Socket `recv`** (`NETWORK-SERVERS.0.md` §4) reads into a reusable buffer and
  hands the handler a `Bytes` over a **copy** of just the bytes read — the read
  buffer is then free to be reused for the next read without corrupting the
  frame already in flight.

After ingest, the `Bytes` owns its storage and is immutable, so every
downstream op is copy-free. The cost model is thus "**copy once at the edge,
share forever after**".

### 4.3 Slice views share the backing array

`slice` and the bit-syntax `bytes(n)` segments (§7) return a **sub-slice view**
that shares the parent's backing array — zero-copy, O(1). This is ideal for
parsing (a length-prefixed body is a view into the read buffer, not a copy), but
carries Go's standard slice-retention caveat:

- **Retention caveat.** A small view keeps its *entire* backing array alive: a
  4-byte `slice` of a 1 MB frame pins all 1 MB until the view is unreachable.
- **`convert Bytes <Bytes>`** forces a minimal fresh copy (`len`-sized backing
  array) for the rare case where a small slice must outlive a large buffer
  (e.g. caching one field extracted from each of many big frames). Converting
  Bytes to Bytes is the "compact" operation; you only reach for it when
  retention actually bites.

### 4.4 Cost summary

| Operation | Copies | Notes |
| --- | --- | --- |
| construct from external `[]byte` / `recv` | 1 | the ingest copy (§4.2) |
| `+hb/…/` / `+bb/…/` literal kind | 1 (at the call) | the decoded constant, built once |
| clone / `ForkConcurrent` / `send` | 0 | slice header only (§4.1) |
| `slice`, `unpack` `bytes(n)` view | 0 | sub-slice view (§4.3) |
| `convert Bytes <bytes>` (compact) | 1 | force-copy to drop retention |
| `add` (concat) | 1 | one build of the joined slice |
| `convert Bytes <inst>` (serialise a frame) | 1 | one build of the frame |
| `convert String`/`convert Bytes` (text⇄bytes) | 1 | crosses the bytes⇄text boundary |

## 5. Word surface — Scalar value ops as overloads; frames as a type

`Bytes` value operations are **signature overloads of existing words** (no new
core words). A `Bytes` overload is a more-specific signature that wins dispatch
over the generic `[Scalar …]`/`[Any …]` form, so `convert`/`slice`/`add` "just
work" on Bytes the way they already do on String/List/Number. Heavier binary
operations (crypto, encodings, hashing, random, UUID) live in `boru:bin-util`
(BIN-UTIL.10.md), each accepting `String | Bytes`.

| operation | word | form |
| --- | --- | --- |
| encode text → Bytes (UTF-8) | `convert` | `convert Bytes "hi"` |
| decode Bytes → text (raises `bad-encoding`) | `convert` | `convert String b` |
| ints (0–255) → Bytes | `convert` | `convert Bytes [104 105]` |
| Bytes → ints | `convert` | `convert List b` |
| compact (force a fresh minimal copy, §4.3) | `convert` | `convert Bytes b` |
| sub-range (end-exclusive, zero-copy view) | `slice` | `slice <start> <end> b` |
| concatenate two byte strings | `add` | `a add b` |
| define a frame spec TYPE (§7) | `refine` | `def Header (refine BinarySpec [layout])` |
| build a `Binary` instance (§7) | `make` | `make Header {fields}` |
| serialise an instance to wire Bytes (§7) | `convert` | `convert Bytes <inst>` |
| decode Bytes to a `Binary` instance (§7) | `unpack` | `unpack Header b` |
| streaming-decode a leading frame (§7) | `unpack-prefix` | `unpack-prefix Header b` |

`size`/`length`, `eq`, value ordering (`cmp`/`lt`/`sort`), and `is Bytes` need
**no dedicated word** — they resolve through the `Sizer`/`Equal`/`Comparer`
behaviors (§3). The `+hb/…/` / `+bb/…/` minilang kinds (§6) cover hex/binary
*input*; `BinUtil.hex-encode` covers hex *output*.

**Why this split.** `Bytes` is a value type like `String`, so its value ops slot
into the generic words by type-dispatch rather than a parallel `byte-*`
vocabulary. A binary *frame layout*, by contrast, is a **schema** — so it is a
**type** under the `BinarySpec` kind, exactly as a class schema is a type:
`def Header (refine BinarySpec [layout])` mints a sealed class, `make Header
{fields}` is uniform with `make <Class>`/`make <ObjectType>` and yields a
field-accessible `Binary` **instance**, and `unpack`/`unpack-prefix` dispatch on
the spec type to return one. Serialising an instance to the wire is just
`convert Bytes <inst>` — the instance→data direction, mirroring `convert Bytes`
on a String/List. There is deliberately **no `make Bytes [spec]`** (that
overloaded `make Bytes` to mean two different things — construct-from-data vs
interpret-a-spec) and **no `bytes` word**. Only `unpack-prefix` is a genuinely new
word (streaming, no generic equivalent). (Implementation: the value overloads are
appended from `native_bytes.go` via `RegisterNativeFunc`; a frame type reuses the
object/class machinery wholesale — `BinarySpec`'s `Ideal.Construct` mints a
sealed class carrying the layout (`ObjectTypeInfo.BinaryLayout`), and `make`
reuses the class instance path — see §7 / §14.)

## 6. Hex / binary constants — the `+hb/…/` and `+bb/…/` minilang kinds

Byte constants for protocol magic numbers and fixtures are written with two
**`boru:minilang` kinds** rather than a bespoke core lexer literal: `hb` (hex
bytes) and `bb` (binary bytes), invoked through the standard `+name<delim>src<delim>`
literal sugar:

```boru
import "boru:minilang"

+hb/deadbeef/       # Bytes<de ad be ef>
+hb/de ad be ef/    # Bytes<de ad be ef>   (whitespace/`_` group, ignored)
+bb/01001100/       # Bytes<4c>            (8 bits, MSB-first)
+bb/01001100 11110000/   # Bytes<4c f0>   (grouping ignored)
```

- **`hb`** decodes an **even** number of hex digits (`[0-9a-fA-F]`); spaces,
  tabs, newlines and `_` are grouping separators and dropped. An odd count or a
  non-hex digit raises `mini_parse_error` when the kind runs.
- **`bb`** decodes a run of `0`/`1` whose length (after dropping the same
  grouping separators) is a **multiple of 8**, MSB-first per byte. A non-binary
  digit or a non-multiple-of-8 length raises `mini_parse_error`.
- Both are **generator kinds** (`[src:String opts:Map] → [Bytes]`), so the
  explicit `mini hb 'deadbeef'` / desugared `MiniLang.lang_hb 'deadbeef' {} end`
  forms are equivalent — see `lang/spec/module-minilang.tsv` §6.
- **Why a minilang kind, not a `0x"…"` core literal.** Byte constants are an
  *occasional* need, dominated by the network/binary code that already imports
  `boru:minilang` for its other DSLs; folding them into the existing
  `+name/src/` lexer sugar avoids a new core token and the `0x`-prefix
  disambiguation it would require (`0xff` stays the `Integer` 255 —
  `isBasePrefixedInteger`, `parser/go/parse.go` — untouched). There is **no**
  `b"…"` ASCII literal either — use `convert Bytes "GET "` for text-as-bytes,
  which keeps the encoding explicit.
- **The `+hb/…/` result feeds the words directly**, but note the auto-`end` the
  literal sugar emits stops forward collection, so a kind result that is itself
  a forward arg must be parenthesised: `unpack (+hb/0100026869/) [ {name:'op' type:'u8'} … ]`.
  Implementation: `lang/go/modules/minilang.go` (`hb`/`bb` kinds + handlers),
  `lang/go/modules/docs_minilang.go` (docs).

## 7. Frame types — `BinarySpec : Binary :: Class : Object`

Erlang's bit syntax (`<<Len:16, Body:Len/binary>>`) is what makes binary code
short *and* safe. boru's equivalent reuses the **class/object** machinery
wholesale, by the analogy **`BinarySpec : Binary :: Class : Object`**:

- **`BinarySpec`** is the spec **kind** — the type-of-a-frame-type, exactly as
  `Class` is the type-of-a-class. A concrete frame layout **refines** it:
  `def Header (refine BinarySpec [layout])` mints a sealed **class** that carries
  the wire layout (on `ObjectTypeInfo.BinaryLayout`). `Header` is thereafter a
  first-class type — usable in signatures, `typeof`, `is Header`, etc.
- **`Binary`** is the **instance** kind — what `make Header {fields}` produces: a
  field-accessible **object instance** of the `Header` class (`p.ver`,
  `typeof p → Header`, `p is Header`), indistinguishable from any other object
  instance for field access / `convert Map` / dispatch.
- **`Bytes`** stays purely the **wire data**. `convert Bytes <inst>` serialises
  an instance (the instance→`Bytes` direction); `unpack`/`unpack-prefix` decode
  `Bytes` back into a `Binary` instance.

```
def Header (refine BinarySpec [ {name:'ver'  type:'u8'}
                                {name:'len'  type:'u16' endian:'le'}
                                {name:'body' type:'bytes' size:'len'} ])

def p (make Header {ver:1 len:2 body:(convert Bytes "hi")})  # a Binary instance
p.ver                                                        # 1  (field access)
convert Bytes p                                              # → a plain Bytes (the wire)
unpack Header wire                                           # → a Binary instance
```

> **No secondary parsing (ADR-007).** The layout is ordinary Node data — a
> `List` of segment `Map`s — that the implementation only **reads**. There is no
> token-level sub-language and no in-handler parser (an earlier draft captured
> `name:u16/be` raw and `strings.Split`/`strconv`-interpreted it; that second
> grammar is removed). The seg-`type` is a plain String the codec enum-dispatches
> on (data interpretation, not parsing); the whole layout is macro-constructable
> and JSON-serialisable. It is also **not** a `boru:minilang` (`MINILANG.5.md`)
> kind — those take an opaque String `src`; this is structured data.

> **Caveat — `is Class` leaks; `is BinarySpec`/`is Binary` not yet wired.**
> Because a frame type reuses the class machinery, a `Header` value is *also*
> `is Class` and an instance is *also* `is Object` (they ARE sealed classes /
> object instances under the hood) — a deliberate simplification that keeps
> field access, `make`, `convert Map`, and `typeof` free of new wiring.
> Separately, `BinarySpec` and `Binary` are installed as **membership types**
> (a Go predicate over the class layer), which the `Value.Is` *method* answers,
> but the **`is` word** and signature dispatch consult lattice `ConformsTo`, not
> membership, for object/type operands — so `Header is BinarySpec` and
> `p is Binary` currently return **false**. Use `typeof`/`is Header` (which do
> work) to test frames; rooting instances under a dedicated lattice node so the
> two predicates resolve through `is` is the clean follow-up.

### 7.1 Segment schema

The layout `List` holds one `Map` per segment — a fully-explicit descriptor,
every value a String / Integer / Boolean:

| key | type | meaning |
| --- | --- | --- |
| `name` | String | the field name: **bound** on unpack, **read from the fields map** on pack. |
| `value` | Integer | a **pack constant** / an **unpack match-guard**. Mutually exclusive with `name`; exactly one is required. |
| `type` | String | `u8 u16 u32 u64`, `i8 i16 i32 i64`, `f32 f64` (IEEE-754, ref `IEEE-754-COMPLIANCE.8.md`), `bits` (sub-byte uint), `bytes` (raw `Bytes`), `utf8` (length-known text → `String`), `pad` (zero bits, no field). |
| `endian` | String | `"be"` (default, network order) or `"le"`. |
| `signed` | Boolean | overrides the `u`/`i`-prefix default (ints only). |
| `size` | Integer \| String | a literal count, or a **String naming an earlier field** (`size:'len'`, Erlang's `Body:Len/binary` — the killer feature). Required for `bits`/`pad`; for `bytes`/`utf8` omitting it means "the rest". |

### 7.2 `def Header (refine BinarySpec [layout])` — define a spec type

`refine BinarySpec [layout]` mints a **sealed class** carrying the layout: the
layout is validated (unknown `type`, missing `size`, etc. raise `bytes_error`
here, at definition time) and stored on `ObjectTypeInfo.BinaryLayout`, and the
class's field schema (name → type) is derived from the layout's named segments.
`def Header` names it; `Header` is thereafter a first-class type — usable in
signatures, `typeof`, `is Header`, etc.

### 7.3 `make Header {fields}` — build a `Binary` instance

```
make <BinarySpec> <Map> -> Binary       # a field-accessible object instance
```

Reuses the **class instance path** (no Bytes-specific overload): `make` validates
`{fields}` against the class's derived field schema and returns a `Binary`
instance. Field access (`p.ver`), `typeof p` (→ `Header`), and `convert Map p`
all work as on any object instance. A `value` (constant) segment is *not* a
field — it is supplied at serialise time, not by `make`. A missing required
field raises the class machinery's `make` error.

### 7.4 `convert Bytes <inst>` — serialise to the wire

```
convert Bytes <Binary> -> Bytes         # the instance → wire-data direction
```

Reads the layout off the instance's spec type and packs each segment
left-to-right into a single freshly-built `[]byte` (one allocation, §4.4): a
`name` segment takes the field from the instance, a `value` segment packs the
constant; numerics honour width/`endian`/`signed`. Given a non-`Binary` instance
(an ordinary class) it raises `type_error`.

### 7.5 `unpack BinarySpec b` — decode to a `Binary` instance

```
unpack <BinarySpec> <Bytes> -> Binary
```

Reads the layout from the spec type and decodes `b`, **returning a `Binary`
instance** (composable; not a scope side effect). A `value` segment is a
**guard**; `bytes`/`utf8` segments are **zero-copy sub-slice views** (§4.3). A
guard mismatch or a too-short buffer `raise`s `no_match`; trailing bytes are left
unread (use `unpack-prefix` to capture them).

### 7.6 `unpack-prefix BinarySpec b` — the streaming framer

```
unpack-prefix <BinarySpec> <Bytes> -> {ok: Binary  rest: Bytes} | {need: Integer}
```

Reads **only what the layout requires** and returns the decoded instance plus the
**leftover bytes** (`rest`), or — when the buffer is too short — `{need: n}`
(more bytes required, when computable, else a positive lower bound). This is the
single primitive a length-prefixed framing loop needs: read header → learn
length → read body, declaratively, with no hand-rolled buffer state machine
(`NETWORK-SERVERS.0.md` §5.3, §6.1).

### 7.7 Alignment rule

`convert Bytes`/`unpack` are **byte-aligned by default**: a contiguous run of
`bits` (and `pad`) segments must sum to a whole number of bytes, or it `raise`s
**`unaligned`**. There is no implicit trailing padding — add an explicit `pad`
segment to round out a sub-byte run. (This resolves `NETWORK-SERVERS.0.md` §12
Q2 in favour of safety: ragged frames are an error, not a silent surprise.)

### 7.8 How the layout rides on the type

`def Header (refine BinarySpec [layout])` runs `BinarySpec`'s `Ideal.Construct`
(`installIdeals`): it validates the layout via `readBitSegments`, derives the
field schema (`binaryFieldSchema`), and mints an `ObjectTypeInfo` with
`Class:true` and `BinaryLayout` set to the raw layout. So the layout lives on the
*type* (no kernel type-kind, no value-level spec, no side table), the instance is
a plain object instance, and the codec recovers the layout from
`ObjectTypeInfo.BinaryLayout` (`binarySpecLayout` on a spec type;
`binaryInstanceLayout` via an instance's `TypeRef`). `size:'len'` resolves
against the frame's already-decoded fields, so the non-binding `unpack-prefix`
resolves `len` with no scope dependency.

## 8. Usage examples

Forward form. `import "boru:bin-util"` only where a crypto/encoding word appears,
`import "boru:minilang"` where an `+hb/…/` / `+bb/…/` constant appears; the type
and bit-syntax need no import.

```boru
# hex/binary constants via the minilang kinds; 0xff (no quote) stays an Integer
import "boru:minilang"
+hb/deadbeef/                              # Bytes<de ad be ef>
+bb/01001100/                              # Bytes<4c>
0xff                                       # 255

# text round-trip (é is two UTF-8 bytes)
convert String (convert Bytes "café")     # 'café'
size (convert Bytes "café")               # 5

# define a frame type, then build + serialise + decode a [ver=1][u16 len][body] frame
def Msg (refine BinarySpec [ {name:'ver' type:'u8'}  {name:'len' type:'u16'}  {name:'body' type:'bytes' size:'len'} ])
def body ( convert Bytes "hello" )        # Bytes<68 65 6c 6c 6f>
def msg  ( make Msg {ver:1 len:(size body) body:body} )   # a Binary instance
msg.ver                                    # 1   (field access; typeof msg → Msg)
def wire ( convert Bytes msg )             # Bytes<01 00 05 68 65 6c 6c 6f>
def f ( unpack Msg wire )                  # → a Binary instance
convert String f.body                      # 'hello'

# floats (little-endian) + signed
def F (refine BinarySpec [ {name:'x' type:'f64' endian:'le'} ])  convert Bytes (make F {x:1.5})  # 8-byte LE float
def S (refine BinarySpec [ {name:'k' type:'i16'} ])              convert Bytes (make S {k:-2})   # Bytes<ff fe>

# sub-byte flags: a version nibble + flags (sums to one byte)
def Flags (refine BinarySpec [ {name:'ver' type:'bits' size:4}  {name:'a' type:'bits' size:1}  {name:'b' type:'bits' size:1}  {value:0 type:'bits' size:2} ])
convert Bytes (make Flags {ver:9 a:0 b:0})  # Bytes<90>

# streaming framer loop: pull complete [u32 len][len body] frames from a buffer
def Frame (refine BinarySpec [ {name:'len' type:'u32'}  {name:'body' type:'bytes' size:'len'} ])
def drain fn [[buf:Bytes] [Never] [
  def step ( unpack-prefix Frame buf )
  case [
    [ step.need ?? false ] [ buf ]              # incomplete → return buffer to refill
    [ ]                    [ handle step.ok.body   drain step.rest ] ]  # got one → continue
]]

# core ↔ bin-util handoff: hash a Bytes, render hex
import "boru:bin-util"
convert Bytes "hello"  BinUtil.sha256  BinUtil.hex-encode
  # "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

# zero-copy slice + retention: keep one small field out of a huge frame
def tag ( convert Bytes (slice 4 8 big-frame) )   # convert-to-Bytes drops the big backing array

# negative cases
def Hdr (refine BinarySpec [ {name:'ver' type:'u8'}  {name:'len' type:'u16'} ])
unpack Hdr (+hb/01/)                          # ERROR:no_match   (only 1 byte, needs 3)
def Odd (refine BinarySpec [ {value:1 type:'bits' size:3} ])  convert Bytes (make Odd {})  # ERROR:unaligned (3 bits)
refine BinarySpec [ {name:'v' type:'nope'} ]  # ERROR:bytes_error (unknown segment type, at definition)
def C class {a:1}  convert Bytes (make C {})  # ERROR:type_error  (an ordinary class is not a Binary instance)
convert Bytes [256]                           # ERROR:expected-byte
convert String (+hb/ff/)                      # ERROR:bad-encoding (0xff is not valid UTF-8)
+hb/abc/                                      # ERROR:mini_parse_error (odd hex digit count)
```

## 9. Errors

| code | raised when |
| --- | --- |
| `mini_parse_error` | a `+hb/…/` source has an odd hex-digit count or a non-hex digit, or a `+bb/…/` source has a non-binary digit or a non-multiple-of-8 bit count (raised when the kind runs). |
| `expected-byte` | a `List` element passed to `convert Bytes` is not an Integer in 0–255. |
| `bad-encoding` | `convert String` (or a `utf8` segment) is given bytes that are not valid UTF-8. |
| `no_match` | an `unpack` guard fails (a `value` segment mismatches, or a `pad` segment carries nonzero bits), or the buffer is too short for the layout. |
| `unaligned` | a `bits`/`pad` run in a layout does not sum to a whole byte. |
| `bytes_error` | a layout segment is malformed (unknown `type`, missing/negative `size`, bad `name`/`value`, a `value` literal on a float/bytes/utf8/pad segment) — at `refine` time; a field that does not fit its declared segment width (`300`/`-1` for `u8`, `31` for a 4-bit field) or a sized `bytes`/`utf8` field whose length disagrees with its declared `size` — at `convert Bytes` time; a `size:'name'` referencing a field not decoded earlier, or a `u64` whose value exceeds the `Integer` range — at `unpack` time. |
| `type_error` | `convert Bytes`/`unpack`/`unpack-prefix` is given a target that is not a binary frame (e.g. an ordinary class instance, with no layout). |

Guard with `RequireConcreteList` / a `Bytes` unwrap helper and range-check
before converting; never panic (`core/go/CLAUDE.md`). `slice` clamps an
out-of-range window rather than erroring. A short buffer during streaming is
**not** an error — it is `unpack-prefix`'s `{need: n}` result.

## 10. Policy / capabilities

None — the type, its literal, the core interop words, and the whole bit-syntax
are **pure** and run under any policy. (The *entropy* words that produce `Bytes`
— `random-bytes`, `uuid` — are gated; that gating lives with those words in
BIN-UTIL.10.md, not on the type.)

## 11. Overlap & relationships

- **Net-new type** — there is no binary type today. It *complements* `String`:
  `convert Bytes` / `convert String` are the explicit, lossless bridge, where
  previously binary silently became a (possibly lossy) String at the I/O
  boundary.
- **`boru:bin-util`** (BIN-UTIL.10.md) — owns crypto/HMAC/CRC/encodings/random/
  UUID; all consume/produce `Bytes`. §4.1 of that doc ("Bytes interop") now
  points here: those interop words are **core**, not module words.
- **`NETWORK-SERVERS.0.md` / `NETWORK-CLIENTS.0.md`** — `recv-frame`,
  `length-prefixed` codecs, and `pack`/`unpack` on the wire are this section's
  bit-syntax; the zero-copy memory model (§4) is what their §7 streaming/
  backpressure claims rest on.
- **`STREAM-WORDS.0.md`** — `stream.from-bytes`/`to-bytes` now carry the real
  `Bytes` type (no surface change).

## 12. Open questions / out of scope

- **Binary file I/O** — an `IO.read-bytes` (and `IO.write` accepting `Bytes`)
  yielding/consuming `Bytes` instead of String is the natural follow-on, but
  lives in `boru:io`; out of scope here. Cross-reference once Bytes lands.
- **Sub-byte endianness** — `bits(n)` for `n > 8` spanning bytes uses the
  segment's `/be`/`/le`; the exact bit-order for non-byte-multiple spans is
  pinned to big-endian-bit-order in the spec rows (Erlang's default). Confirm
  against real protocols during implementation.
- **`unpack-prefix` `need` precision** — when the missing length is itself in an
  unread field, `need` returns the bytes required to read *that* field (a lower
  bound), then the caller re-tries. Document the two-step convergence.
- **A pure functional `with-byte` mutator** — deliberately omitted; `Bytes` is
  immutable and edits go through `add`/`slice`/`convert Bytes <inst>`. Revisit only if a
  builder pattern proves necessary.

## 13. Phased roadmap

Aligned with `NETWORK-SERVERS.0.md` §11 Phase A (the binary prerequisite):

- **Phase A1 — type + memory + interop overloads.** Register `Bytes` (§2–3),
  the zero-copy/copy-on-ingest model (§4), `gobridge` `[]byte`↔`Bytes`, and the
  `convert`/`slice`/`add` Bytes overloads (§5). No literal, no bit-syntax.
  Independently useful (hashing, file I/O). **(landed)**
- **Phase A2 — the `+hb/…/` / `+bb/…/` minilang kinds** (§6). Two generator
  kinds registered in `boru:minilang`, reusing the existing `+name/src/` literal
  sugar — no core lexer change. **(landed)**
- **Phase A3 — frame types** (§7): the `BinarySpec` spec kind +
  `def Header (refine BinarySpec [layout])` building a sealed class, `make`
  producing a `Binary` instance, the `convert Bytes <inst>` serialiser, the
  `unpack` overload, and the new `unpack-prefix` word — all dispatching on the
  frame type / instance. This unblocks the declarative `length-prefixed` codec
  and `recv-frame`. **(landed)**

## 14. Implementation sketch

- **`core/go/gobridge.go`** — add the `[]byte` ↔ `Bytes` cases, with the
  copy-on-ingest copy in `FromNative` (§4.2).
- **`core/go/gobridge.go`** — add the `[]byte` ↔ `Bytes` cases (via the
  `RegisterBytesBridge` hook), with the copy-on-ingest copy in `FromNative`
  (§4.2). (No kernel type-kind for frames — they reuse the existing
  `ObjectTypeInfo`/`ObjectInstanceInfo` machinery, per the kernel/domain
  boundary in `core/go/CLAUDE.md`.)
- **`core/go/value.go`** — `ObjectTypeInfo` gains a `BinaryLayout Value` field:
  the raw layout `List<Map>` a binary-frame class carries (zero Value on an
  ordinary class). This is the single hook that turns the class machinery into a
  frame carrier.
- **`lang/go/native/native_bytes.go`** — `registerBytesType()` via
  `RegisterExternalBuiltin("Scalar/Bytes", 1009, bytesBehavior{})`; behavior
  implements `Format`/`Comparer`/`Equal`/`Sizer` (§3) and **omits**
  `DeepCloner` (§4.1); the `convert`/`slice`/`add` value overloads (§5) appended
  via `RegisterNativeFunc`; funnel init errors through `TypeInitError`.
- **`lang/go/native/native_bytes.go`** — the frame codec: `binarySpecLayout`
  (layout off a spec class) / `binaryInstanceLayout` (off an instance's
  `TypeRef`); `binaryFieldSchema` derives the class's field schema from the
  layout; `convert Bytes <Binary>` (`[TBytes,TClass]`, serialise), `unpack`
  (`[TClass,TBytes]` → instance), `unpack-prefix` (`[TClass,TBytes]` → `{ok rest}`
  | `{need}`) — all dispatching on `TClass` (a frame type is a sealed class) with
  a handler guard that declines non-frame classes. `readBitSegments`/`readOneSeg`
  **read** (never parse) the `List<Map>` layout — `name`/`value`, `type`,
  `endian`, `signed`, `size` keys (ADR-007); an MSB-first bit writer/reader;
  `frameInstance` builds the instance via `core.MakeObject`. No `make` overload —
  `make Header {fields}` reuses the object/class instance path unchanged.
- **`lang/go/native/native_type.go` (`installIdeals`)** — `Binary` and
  `BinarySpec` installed as **membership types** (`DefineMemberType` over
  `TClass`): `Binary` matches an instance whose type carries a `BinaryLayout`,
  `BinarySpec` matches such a class type (and is the `refine` base). The
  `BinarySpec` `Ideal.Construct` validates the layout, derives the field schema,
  and mints an `ObjectTypeInfo{Class:true, BinaryLayout:arg}` — so
  `refine BinarySpec [layout]` builds the sealed-class spec type. **(Caveat: the
  `is` word / dispatch don't consult membership for object/type operands, so
  `is Binary`/`is BinarySpec` return false — see §7. The members have no FixedID;
  they are per-registry.)**
- **`lang/go/modules/minilang.go`** — the `hb`/`bb` generator kinds
  (`[src:String opts:Map] → [Bytes]`) + handlers, building `Bytes` via
  `core.FromNative`; `docs_minilang.go` — the `lang_hb`/`lang_bb` doc entries.
- **`lang/go/native/help/`** — `help_bytes.go` describes `unpack-prefix`;
  `help_control.go`'s `unpack` entry notes the `unpack <BinarySpec> b` form;
  there is no `bytes` word. (`make`/`unpack` keep their existing entries.)
- **`lang/go/test/fixedid_stability_test.go`** — pin `Scalar/Bytes` = 1009 (no
  FixedID for frame types / the membership kinds — frame types are ordinary
  class-minted subtypes; `Binary`/`BinarySpec` are per-registry members).
- **`lang/go/test/bytes.tsv`** — value-op rows (convert round-trips, slice,
  `add`, compact, size/eq) plus frame rows (`def H (refine BinarySpec […])`,
  field access, `typeof`, `is H`, `convert Bytes`, `unpack`/`unpack-prefix`,
  float/signed) each paired with an `ERROR:<substring>` negative sibling
  (`no_match`, `unaligned`, `bytes_error`, `type_error`, `expected-byte`,
  `bad-encoding`); the `+hb/…/` / `+bb/…/` literal rows live in
  `lang/spec/module-minilang.tsv` §6; plus a Go-bridge test.
