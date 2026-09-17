# NETWORK-SERVERS

Design for the **network-server handling API** in boru — the layer where a
developer actually writes the code that handles a network connection. It
completes the "efficient, safe network servers" end-goal that
`PROCESSES.0.md` (the actor substrate) and `SERVICES.0.md` (the service/server
model) both scope networking *out* of, as "phase 3".

The design delivers **two tiers** of handling API over the **same BEAM
actor-per-connection model**:

- a **low-level handling API** — raw connections, byte buffers, and
  hand-written framing, for protocol authors and the tightest control; and
- a **high-level message-handling API** — a codec turns the byte stream into
  **messages** dispatched to a `service`'s patrun handlers, for the code most
  servers are actually written in.

Both ride the actors of `PROCESSES.0.md`. The high-level tier is *literally the
low-level tier plus a codec and a dispatch loop* — §6 shows the reduction.
Binary protocols are made first-class by a new **`Bytes` type + bit-syntax**
(§3); streaming data reuses `boru:stream` (`STREAM-WORDS.0.md`) for both
request bodies and replies (§7).

This is a **design RFC only — no implementation code yet**, matching how the
adjacent subsystems were designed first (`PROCESSES.0.md`, `SERVICES.0.md`,
`STREAM-WORDS.0.md`).

The **dial side** is specified symmetrically in `NETWORK-CLIENTS.0.md`: it reuses
this document's `Socket`, `Codec`, `Bytes`/bit-syntax, and streaming machinery
unchanged, adds the `connect-raw`/`connect` dial primitives, and models a client
as a **bidirectional `Endpoint`** — a `Service` you both `call`/`send` to the
peer and `add` handlers to for what the peer pushes back. A served connection's
session and a `connect`ed endpoint are the *same* type, so a proxy/gateway is
just this document composed with its sibling.

> **Decisions taken at design-review time** (the four forks this RFC opened):
> (1) **one** new document covering both tiers (this file); (2) a **full**
> `Bytes` value type **and an Erlang-style bit-syntax** for framing (§3), made
> an explicit prerequisite phase; (3) the low-level socket offers **both**
> Erlang-style **passive** (default, `recv`-driven) **and active** (bytes as
> mailbox messages) modes (§4.3); (4) the worked-example suite spans
> **line/text + JSON-over-TCP, HTTP/JSON REST, length-prefixed binary RPC, and
> streaming/long-lived** protocols (§5, §6, §7, §8).

## 0. Review of the existing design — what is settled, what this adds

This RFC is an *extension*, not a rewrite. The two anchor documents already
settle most of the model; the table records what each contributes and the
single gap this file fills.

| Concern | Settled by | This RFC |
| --- | --- | --- |
| Lightweight processes, `spawn`/`self`/`send`/`receive`, `Pid`, bounded mailbox, patrun dispatch | `PROCESSES.0.md` §2–4 | reused verbatim; a connection handler *is* such a process |
| Immutable-only, zero-copy messages (`not_sendable`) | `PROCESSES.0.md` §6, `SERVICES.0.md` §7.1 | extended to cover `Bytes` and `Socket` handles (§3.1, §4.1) |
| `service`/`add`/`call`/`send`/`state`, `prior`/`wrap`, `server`/`serve`, supervision | `SERVICES.0.md` §1–3 | a transport's per-connection sessions dispatch into exactly these (§6) |
| Uniform failure contract, bounded mailboxes, backpressure, `pool` | `SERVICES.0.md` §8–9 | inherited at the wire edge; backpressure now also paces the *socket* (§7.3) |
| `listen`/`connect` *named* as the transport surface | `SERVICES.0.md` §4 | **specified here** — the actual acceptor, codec, session, and handler API |
| The "still-missing TCP/socket server" and "no `Bytes`/bit-syntax" gaps | `PROCESSES.0.md` §8, `SERVICES.0.md` §10 | **closed here** (§3, §4) |
| Back-pressured byte/value pipelines, `Stream`/`Channel`/`Job` | `STREAM-WORDS.0.md` | the streaming-reply and streaming-body shapes bind to these (§7) |

In one line: **the substrate and the service model exist on paper; this
document specifies the two handling APIs that sit between a socket and a
handler, and the `Bytes`/bit-syntax that binary framing needs.**

## 1. The two tiers at a glance

The same job — *accept a connection, turn its bytes into something, act, reply*
— written at both tiers, so the trade-off is visible before any detail.

**Low level (you own the bytes and the framing):**

```boru
import "boru:net"

# Echo server: one actor per connection, reading and writing raw bytes.
def echo fn [[conn:Socket] [Never] [
  do [
    def chunk ( recv conn 0 )      # block for whatever bytes are available
    send-bytes chunk conn          # write them straight back
    echo conn                      # tail-loop
  ]
  error [ case [
    [get code eq closed/q] [ None ]   # peer hung up → this actor exits cleanly
    [ raise ] ] ]
]]

# A listener actor accepts forever, spawning one `echo` actor per connection.
serve-raw {tcp: 7} [ [conn] => [ echo conn ] ]
```

**High level (a codec gives you messages; you write handlers):**

```boru
import "boru:net"
import "boru:serve"

# The same echo, as a message service over a newline-delimited text codec.
def echo-svc ( service {} )
add {} [ [req state] => [ req.line ] ] echo-svc      # reply == the line read

listen {tcp: 7  codec: lines} echo-svc
```

Both spawn one BEAM-style process per connection and both are zero-copy. The
low-level version hands you a `Socket` and a byte buffer; the high-level version
hands you a decoded `{line: …}` message and turns your return value back into
bytes. Everything else in this document is filling in those two surfaces and the
binary/streaming machinery they share.

## 2. The connection model (BEAM → sockets)

The canonical BEAM server shape — **one cheap, isolated process per
connection** — maps onto `PROCESSES.0.md` actors with one addition (a socket
handle) and one new runtime owner (a `net` listener). Nothing here needs a new
scheduler.

| BEAM / `gen_tcp` concept | boru realization |
| --- | --- |
| `gen_tcp:listen/2` → listen socket | **`listen {tcp: …} -> Listener`**, a new opaque Ideal handle (like `Pid`) |
| `gen_tcp:accept/1` (acceptor process) | **`accept <Listener> -> Socket`**, blocking; idiomatically called in a loop that `spawn`s a handler |
| connection socket | **`Socket`** Ideal handle (§4.1) |
| `{active, false}` passive recv | **`recv`/`recv-bytes`/`recv-until`** on a `Socket` (§4.3) — *default* |
| `{active, once}` / `{active, true}` | socket forwards `{tag:"data" …}` mailbox messages to its owner (§4.3) — opt-in |
| controlling process | the actor that holds the `Socket`; transfer via `set-owner` (§4.1) |
| `gen_tcp:send/2` | **`send-bytes <Bytes> <Socket>`** (§4.2) |
| `inet:setopts` (packet framing) | socket options + the `Bytes` bit-syntax (§3) and codecs (§6) |
| acceptor pool | a `pool` of acceptor actors (`SERVICES.0.md` §9.2) — same word, no new concept |
| "let it crash" per connection | each handler actor `recover()`s independently (`PROCESSES.0.md`); a crashed connection never touches its neighbours |

The listener is owned by a new **`NetRuntime`** pointer on `Registry` (sibling
of `PROCESSES.0.md`'s `ProcessRuntime`, auto-shared by `ForkConcurrent`'s
shallow copy), lazily created on first `listen`, holding the open listen sockets
and a `context.Context` for shutdown so no FD or goroutine leaks on host exit.

## 3. `Bytes` and bit-syntax (the binary prerequisite)

Binary wire protocols need two things boru lacks: a **byte-string value** and a
terse, safe way to **build and pattern-match frames**. Both `PROCESSES.0.md` §8
and `SERVICES.0.md` §10 name this gap. The **full design is `BYTES.10.md`**
(`design/go-modules/`); this section is the working summary the rest of this doc
relies on. It is its own phase (§10) because the handling API depends on it but
the type is independently useful (file I/O, hashing, `boru:stream`'s
already-referenced `from-bytes`).

### 3.1 The `Bytes` value type (summary)

A new core **`Bytes`** type: an **immutable** finite sequence of octets.

- **Immutable ⇒ sendable & zero-copy.** `Bytes` joins the "sendable" class of
  `PROCESSES.0.md` §6 — a received frame is shared into a handler by reference,
  never copied, exactly like a `List` or `Map`. A defensive copy happens only at
  ingest (`recv`, the Go bridge); after that every op is copy-free
  (`BYTES.10.md` §4). This is boru getting Erlang's refc-binary optimisation "for
  free", as `SERVICES.0.md` §7.1 anticipated.
- **Distinct from `String`.** `String` is text (UTF-8, character-indexed);
  `Bytes` is octets (byte-indexed). `utf8 <String> -> Bytes` encodes (total);
  `to-text <Bytes> -> String` decodes (raises `bad-encoding`).
- **Comparable / printable.** Ordered lexicographically by octet; printed as
  `Bytes<de ad be ef>`.

Core surface (forward form, **no import**): the `convert` text/ints⇄Bytes
overloads (+ compact), `slice`, `add`; `size`/`eq`/ordering via type behaviors.
Hex/binary constants are the `+hb/deadbeef/` / `+bb/01001100/` kinds in
`boru:minilang` (BYTES.10.md §6). Crypto, hashing, and hex/base encodings live in
`boru:bin-util` (taking/returning `Bytes`). Full table: `BYTES.10.md` §5. (There
is **no `0x"…"` core literal and no `b"…"` literal** — write
`convert Bytes "GET "` for text-as-bytes.)

### 3.2 Bit-syntax — `pack` / `unpack` (summary)

Erlang's bit syntax (`<<Len:16, Body:Len/binary>>`) is what makes binary code
short *and* safe. boru's equivalent reuses the **`name:Type` binding slot** parsed
by `eng.ParseFnParams` (the same one `PROCESSES.0.md` §3 uses for `receive`
clauses), extended with a size and modifier suffix. A frame spec is a list of
*segments* `<value-or-name> : <seg-type> ( (size) )? ( / <modifier> )*` —
seg-types `u8…u64`/`i8…i64`/`f32 f64`/`bits(n)`/`bytes`/`utf8`, modifiers
`/be`(default)`/le`, `/signed`/`/unsigned`, size an integer or a
previously-bound name (`body:bytes(len)`, the killer feature). Full grammar and
semantics: `BYTES.10.md` §7.

```boru
# build + match a [ver=1][u16 len][len bytes body] frame
def body  ( utf8 "hello" )
def frame ( pack [ 1:u8  (length body):u16  body:bytes ] )
# frame == Bytes<01 00 05 68 65 6c 6c 6f>
def parts ( unpack frame [ ver:u8  len:u16  body:bytes(len) ] )   # 1:u8 would guard
# parts == {ver: 1  len: 5  body: Bytes<68 65 6c 6c 6f>}
to-text parts.body          # → "hello"
```

For framing over a socket without knowing the length up front,
**`unpack-prefix <Bytes> [ … ] -> {ok: Map  rest: Bytes} | {need: Integer}`**
matches as much as the spec needs and returns the bound fields plus the leftover
bytes, or `{need: n}` more bytes required — the one primitive a length-prefixed
framer needs (§5.3), instead of a hand-rolled state machine.

## 4. Tier 1 — the low-level handling API (`boru:net`)

For protocol authors, performance-critical paths, and anything a codec doesn't
yet cover. You get a socket and the bytes; you own framing, parsing, and the
reply.

### 4.1 The `Socket` and `Listener` handles

Two new opaque Ideal types (like `Pid`):

- **`Listener`** — a bound, listening socket. Created by `listen` (§4.4),
  consumed by `accept`.
- **`Socket`** — one connection. Holds the OS connection, a read buffer, its
  mode (passive/active), and its **owning process** (for active delivery).

Both are **sendable handles** (like `Pid`): the *identity* travels in a message
zero-copy, so a listener actor can `spawn [ conn handler ]` or `send conn
worker`. The *OS resource* is owned by whichever actor currently holds it for
I/O; ownership for active-mode delivery is explicit:

- **`set-owner <Socket> <Pid>`** — make `<Pid>` the controlling process (active
  messages go there). Defaults to the accepting actor. (Erlang
  `controlling_process/2`.)

A `Socket` is **not** in the immutable-sendable class for *content* — it wraps a
mutable OS resource — but it is sent by *handle identity* exactly like a `Pid`,
so the `not_sendable` rule (§3.1, `PROCESSES.0.md` §6) admits it as a handle and
still rejects `Object`/`Array`/`Store`.

### 4.2 Writing

- **`send-bytes <Bytes> <Socket> -> <Bytes>`** — write all the bytes (handling
  short writes internally); returns the bytes for chaining. Raises `closed` if
  the peer is gone, `transport` on a socket error.
- **`send-stream <Stream<Bytes>> <Socket>`** — write every chunk of a byte
  stream, with socket back-pressure pacing the stream (§7.3). The streaming
  reply primitive.
- **`flush <Socket>`** / **`close <Socket>`** / **`shutdown <Socket> <half>`**
  (`half` = `"read"`/`"write"`/`"both"`).

### 4.3 Reading — passive (default) and active (opt-in)

**Passive** is the default and the framing-friendly path: the actor *pulls*
bytes when it wants them, so a framing loop reads exactly what it needs.

| Word | Effect |
| --- | --- |
| `recv <Socket> <n> -> Bytes` | block until ≥1 byte, return up to `n` (n=0 → whatever is available). EOF raises `closed`. |
| `recv-bytes <Socket> <n> -> Bytes` | block until **exactly** `n` bytes (or `closed`). The fixed-frame reader. |
| `recv-until <Socket> <delim:Bytes> -> Bytes` | read through the next `delim` (e.g. `utf8 "\r\n"`); returns the chunk **without** the delimiter. The line/text reader. |
| `recv-frame <Socket> [ <bit-syntax> ] -> Map` | read one frame described by §3.2 bit-syntax, pulling exactly the bytes the spec (incl. a length field) requires. The length-prefixed reader. |

`recv*` honour an optional `{within: <Duration>}` deadline
(`recv conn 4096 {within: (TimeUtil.seconds 30)}` → raises `timeout`), so a
slow-loris peer cannot pin an actor forever — the socket equivalent of
`receive … after`.

**Active** mode unifies socket data with the actor's other mailbox traffic — the
purest BEAM style, needed when a connection actor must react to *both* network
bytes *and* messages from other processes (a chat actor: socket input **and**
broadcast pushes). Turn it on per-read or persistently:

- **`set-active <Socket> <mode>`** — `false` (passive), `true` (every chunk
  delivered), or `"once"` (deliver one chunk, then auto-revert to passive —
  the back-pressure-safe default for active use, Erlang `{active, once}`).

In active mode the socket sends **tagged-map messages** to its owner's mailbox,
matched by ordinary `receive` clauses (§3 of `PROCESSES.0.md`):

```
{tag: "data"    bytes: Bytes}     # a chunk arrived
{tag: "closed"}                   # peer closed
{tag: "error"   reason: Atom}     # socket error
```

The same `{within}`/`after` timeout story applies via the `receive`'s `after`
clause. Mixing: a connection actor can be in active `"once"` for socket data and
still `receive` application messages in the same loop (§5.4 chat example).

### 4.4 Accepting — `listen` / `accept` and the `serve-raw` sugar

```
listen {tcp: <port>  …opts} -> Listener        # network-capability gated (§9)
accept <Listener> -> Socket                     # blocks for the next connection
```

`listen` options: `host:` (bind address, default all), `backlog:`, `reuse:`,
and **`tls: {identity: <name>}`** (TLS termination; the `Socket` is then a
decrypted stream — TLS is a socket option, not a separate API). `unix:` selects
a Unix-domain socket instead of `tcp:`.

> **AMENDED (as built).** This originally read `tls: {cert: … key: …}` with
> file PATHS. The client half of that form was replaced by a host-registered
> `identity:` — a guest-chosen path dereferenced under host authority is a
> confused deputy, and a credential modelled as bytes cannot express an
> HSM- or agent-backed key. See [NETWORK-CLIENTS.0.md](NETWORK-CLIENTS.0.md)
> §4.4 for the full rationale. The server side (phase 6 of
> [legacy/NETWORK-TLS-PLAN.0.ignore](legacy/NETWORK-TLS-PLAN.0.ignore)) has since landed, using
> the same named identities plus `require-client:` for the client CA pool:
>
> ```
> listen {tcp: 8443  tls: {identity: gw/q  require-client: ca-bundle}}
> ```
>
> `identity:` is required — a TLS server with no certificate has nothing
> to present, and quietly serving cleartext on a socket the program asked
> to be TLS is the worst reading of that mistake. `require-client:` takes
> PEM roots and makes a client certificate MANDATORY and verified; there
> is no request-but-do-not-verify middle setting, because a certificate
> nobody checks is decoration. `min:` is shared with the dial side; the
> dial-only keys (`verify:`, `ca:`, `sni:`) are rejected by name here.
>
> `accept` completes the handshake before the `Socket` escapes, so a
> `recv` never returns plaintext from an unauthenticated peer and a
> `require-client:` rejection surfaces at `accept` rather than later as
> an opaque read error; `{within: ms}` bounds the handshake too.
> `serve-raw` inherits all of it (it binds through `listen`), handshaking
> on the per-connection goroutine so a stalled peer costs one goroutine
> rather than the acceptor.
>
> **`peer-cert <Socket>`** returns the VERIFIED peer certificate as a map
> — `{subject common-name issuer serial not-before not-after dns-names
> emails}` — or `none` on a plain socket or where no client certificate
> was demanded. Only the verified chain is surfaced, never
> `PeerCertificates`: an unauthenticated name that looks exactly like an
> authenticated one is the whole failure mode this word exists to avoid.
> It is what makes `require-client:` useful rather than merely
> restrictive — TLS proves the peer holds a trusted key, and `peer-cert`
> hands that identity to boru so the *authorization* rule can be written
> in the language.
>
> Presenting a server certificate is gated by `network`/`server-cert`,
> its own op: answering AS a service is a different authority from
> calling out as a client (`network`/`client-cert`).

The idiomatic acceptor is a three-line actor; `serve-raw` is the blessed sugar
for it so nobody hand-writes the accept loop wrong (forgetting to spawn, leaking
the listener on crash):

```
serve-raw {tcp: <port> …} [ [conn] => [ … ] ]   # accept-loop + per-conn spawn + recover
```

`serve-raw` spawns a listener actor that `accept`s forever and, per connection,
`spawn`s the handler block with the new `Socket` bound — each on its own forked
registry, each `recover()`ed, so one connection crashing logs and dies alone.

## 5. Tier-1 worked examples (DX assessment)

A spread across the four target families, *all at the low level*, so the cost of
"owning the bytes" is honest. (§6 re-does several at the high level for
comparison.)

### 5.1 Line/text — a line-reversing server

```boru
import "boru:net"

def rev-conn fn [[conn:Socket] [Never] [
  do [
    def line ( recv-until conn (utf8 "\n") )    # one line, delimiter stripped
    send-bytes (concat [ (bytes (reverse (byte-ints line)))  (utf8 "\n") ]) conn
    rev-conn conn
  ]
  error [ case [ [get code eq closed/q] [ None ] [ raise ] ] ]
]]

serve-raw {tcp: 8001} [ [conn] => [ rev-conn conn ] ]
```

### 5.2 JSON-over-TCP — newline-delimited JSON requests

Reuses the existing `Format`/`reify`/`jsonify` JSON path (`PROCESSES.0.md` §8
notes JSON is already covered), with only framing done by hand.

```boru
import "boru:net"

def json-conn fn [[conn:Socket] [Never] [
  do [
    def req ( reify (to-text (recv-until conn (utf8 "\n"))) )   # bytes → text → value
    def reply ( handle-request req )                       # ordinary boru
    send-bytes (concat [ (utf8 (jsonify reply))  (utf8 "\n") ]) conn
    json-conn conn
  ]
  error [ case [
    [get code eq closed/q]      [ None ]
    [get code eq bad-encoding/q] [ send-bytes (utf8 "{\"err\":\"bad utf8\"}\n") conn  json-conn conn ]
    [ raise ] ] ]
]]

serve-raw {tcp: 8002} [ [conn] => [ json-conn conn ] ]
```

### 5.3 Length-prefixed binary RPC — `recv-frame` + bit-syntax

A `[u8 op][u32 len][len bytes payload]` wire protocol. `recv-frame` pulls
exactly one frame using the §3.2 spec (it reads the 5-byte header, learns
`len`, then reads `len` more) — no manual buffering.

```boru
import "boru:net"
import "boru:bin-util"

def rpc-conn fn [[conn:Socket] [Never] [
  do [
    # read one framed request; `payload:bytes(len)` is sized by the just-read `len`
    def f ( recv-frame conn [ op:u8  len:u32  payload:bytes(len) ] )
    def out ( dispatch-op f.op (reify (to-text f.payload)) )   # op-code → handler
    def body ( utf8 (jsonify out) )
    send-bytes (pack [ f.op:u8  (length body):u32  body:bytes ]) conn
    rpc-conn conn
  ]
  error [ case [ [get code eq closed/q] [ None ] [ raise ] ] ]
]]

serve-raw {tcp: 8003} [ [conn] => [ rpc-conn conn ] ]
```

### 5.4 Streaming / long-lived — a chat connection mixing socket + broadcasts

This is the case that *needs* **active mode**: the connection actor must react to
both inbound lines (from its socket) and broadcast messages (from other
connections), so both arrive in one mailbox and one `receive` handles them.
A named `"chatroom"` process (an ordinary `PROCESSES.0.md` actor, elided) keeps
the member set and re-broadcasts.

```boru
import "boru:net"

def chat-conn fn [[conn:Socket] [Never] [
  set-active conn "once"                          # socket data now arrives as messages
  send {cmd: "join" who: self} "chatroom"         # register this actor with the room
  chat-loop conn
]]

def chat-loop fn [[conn:Socket] [Never] [
  receive [
    # a line from THIS connection's socket → tell the room to fan it out
    {tag: "data" bytes: Bytes} [
      send {cmd: "say" text: (to-text bytes)} "chatroom"
      set-active conn "once"                       # re-arm one-shot active read
      chat-loop conn ]
    # a broadcast from the room (another connection's line) → write to our socket
    {tag: "broadcast" line: String} [
      send-bytes (utf8 line) conn
      chat-loop conn ]
    {tag: "closed"} [ send {cmd: "leave" who: self} "chatroom"  None ]
  ]
]]

serve-raw {tcp: 8004} [ [conn] => [ chat-conn conn ] ]
```

The DX read: passive mode (5.1–5.3) is the cleaner story when the actor only
ever reacts to its own socket; active mode (5.4) earns its complexity precisely
when the actor is *also* an addressable participant in a wider message graph.
That is the §4.3 "both, passive default" decision validated against real shapes.

## 6. Tier 2 — the high-level message-handling API

Most servers don't want to touch bytes. The high-level tier supplies a
**codec** (bytes ⇄ messages) and a **per-connection dispatch loop** so the
developer writes only `service` handlers — the exact `add`/`call`/`send` model
of `SERVICES.0.md`. The wire surface is the two words that document already
named:

```
listen  {tcp: <port>  codec: <codec>  …opts} <service>     # expose a service on the wire
connect {tcp: <addr>  codec: <codec>}        -> Service     # a local proxy to a remote one
```

### 6.1 What a codec is, and the reduction to Tier 1

A **`Codec`** is a value with two functions over `Bytes`:

```
{ decode: [ <buffer:Bytes> => {msg: <Value>  rest: <Bytes>} | {need: <Integer>} ]
  encode: [ <reply:Value>  => <Bytes> ] }
```

`decode` is fed the accumulated read buffer and returns one parsed message plus
the leftover bytes, or `{need:n}` to ask for more (the §3.2 `unpack-prefix`
shape, generalised to whole protocols). `encode` turns a handler's reply into
bytes. **That is the whole extension point** — a custom binary or text protocol
is one `Codec` value, written with §3–§4 primitives.

`listen {codec} svc` is then *exactly* this generic Tier-1 handler (this is the
"high level = low level + codec + dispatch" reduction promised in §1):

```boru
# Conceptual desugaring of `listen {tcp:P codec:C} SVC`:
serve-raw {tcp: P} [ [conn] => [ conn-session conn C SVC (convert Bytes []) ] ]

def conn-session fn [[conn:Socket  codec:Codec  svc:Service  buf:Bytes] [Never] [
  def step ( codec.decode buf )
  do [ case [
    [ get need ?? false ] [                                # need more bytes
      conn-session conn codec svc (concat [ buf (recv conn 0) ]) ]
    [ ] [                                                  # got a message
      def reply ( call step.msg.with-meta svc )           # dispatch into the service
      send-bytes (codec.encode reply) conn                # encode + write
      conn-session conn codec svc step.rest ] ] ]         # continue with leftover
  error [ case [ [get code eq closed/q] [ None ] [ raise ] ] ]
]]
```

So Tier 2 introduces **no new concurrency** — same actor-per-connection, same
mailbox, same `service`. It only factors out "read → decode → dispatch → encode
→ write" so the developer supplies a `service`, not a loop.

### 6.2 Built-in codecs

Shipped in `boru:net`, each a `Codec` value usable as `codec:` above:

| Codec | Decodes to message | Encodes reply |
| --- | --- | --- |
| `lines` | `{line: String}` (newline-delimited text) | text + `\n` |
| `json-lines` | `{…parsed JSON…}` (NDJSON, via `reify`) | `jsonify` + `\n` |
| `http` | `{method: String  path: String  headers: Map  body: Bytes}` | status + headers + body |
| `jsonrpc` | `{id len-framed JSON-RPC}` (LSP-style `Content-Length` framing) | framed JSON-RPC |
| `length-prefixed [ <bit-syntax> ]` | `unpack` of the spec (§3.2) | `pack` of the reply spec |
| `websocket` | `{op: "text"/"binary"  data: …}` after the HTTP upgrade handshake | a WS frame |

`length-prefixed` is *parameterised by a bit-syntax spec*, so the §5.3 binary
RPC becomes declarative (§6.5). Everything else is a fixed codec.

### 6.3 Message metadata, replies, per-connection identity

Each decoded message is dispatched with the `SERVICES.0.md` §1 metadata
envelope, here populated by the transport:

- **`req.@conn`** — a handle to the connection actor (a `Pid`), for **server
  push** (`send {…} req.@conn`) and to address one connection from elsewhere.
- **`req.@peer`** — `{host port}` of the remote.
- **`req.@from`** — the reply target, as in `SERVICES.0.md` (deferred replies).

A handler answers three ways, all per `SERVICES.0.md`:

1. **Return a value** → the codec encodes it and the transport writes it (the
   common case).
2. **Return a `Stream`** → streaming reply; the transport encodes & writes each
   element as it arrives (§7).
3. **Return `defer`** and later `reply req.@from value` → async/proxied reply.

**Per-connection state.** Two models, picked by an option:

- **Shared service (default).** One `service` instance serves all connections;
  per-connection state, if any, is keyed by `@conn` in the service `Store`. Best
  for stateless request/response (HTTP REST).
- **Session-per-connection** (`listen {… session: true} ConstructorFn`). The
  transport calls the constructor **once per connection** to get a fresh
  `Service` with private session state, and routes that connection's messages to
  it. Best for stateful protocols (a logged-in connection, a game session). The
  constructor receives `{@conn @peer}` so the session can register itself.

Lifecycle messages are delivered as ordinary requests so handlers can `add` for
them: `{op:"@connect" @peer}` on accept and `{op:"@disconnect" reason}` on
close — making setup/teardown just more patterns, not a side API.

### 6.4 High-level examples — line, JSON, HTTP REST

**Echo and JSON, the §5.1–5.2 jobs without the framing:**

```boru
import "boru:net"
import "boru:serve"

def upper ( service {} )
add {} [ [req state] => [ str.upper req.line ] ] upper
listen {tcp: 8001  codec: lines} upper                 # text in, text out
```

**HTTP/JSON REST — the stated primary use case.** Patrun routes on
`method`+`path` (with path patterns), handlers return values the `http` codec
renders as JSON responses. A shared service holds a `todos` store.

```boru
import "boru:net"
import "boru:serve"

def api ( service {todos: {}} )

# GET /todos  → list
add {method: "GET" path: "/todos"} [ [req state] => [
    values (state get todos) ] ] api

# POST /todos → create (body already decoded to a value by an upstream wrap, below)
add {method: "POST" path: "/todos"} [ [req state] => [
    def id ( new-id )
    state set-in [ todos id ] {id: id  text: req.body-json.text  done: false}
    {status: 201  body: (state get-in [ todos id ])} ] ] api

# GET /todos/:id → one, or a 404 reply shape
add {method: "GET" path: "/todos/:id"} [ [req state] => [
    def t ( state get-in [ todos req.params.id ] )
    t ?? {status: 404  body: {error: "not found"}} ] ] api

# Cross-cutting: decode JSON bodies once, for every route (SERVICES §1 `wrap`).
wrap [ [req state prior] => [
    def r ( req.body length gt 0 ? (req set body-json (reify (to-text req.body))) req )
    prior r ] ] api

# Cross-cutting: structured access logging around every request.
wrap [ [req state prior] => [
    def started ( TimeUtil.now )
    def res ( prior req )
    log.info "req" {method: req.method  path: req.path  ms: (TimeUtil.since started)}
    res ] ] api

listen {tcp: 8080  codec: http} api
```

The DX claim under test: an HTTP API reads as *routing patterns + plain
returns*, with auth/logging/body-parsing as `wrap` layers — no socket, no
framing, no manual JSON plumbing in the handlers. `:id` path segments surface as
`req.params`.

### 6.5 High-level binary RPC — the §5.3 protocol as a declarative codec

The hand-framed §5.3 server, rewritten with a `length-prefixed` codec carrying a
bit-syntax spec. The byte handling vanishes; only the op→handler dispatch
remains.

```boru
import "boru:net"
import "boru:serve"

def rpc ( service {} )
add {op: 1} [ [req state] => [ {op: 1  result: (do-ping req.payload)} ] ] rpc
add {op: 2} [ [req state] => [ {op: 2  result: (do-echo req.payload)} ] ] rpc

listen {
    tcp: 8003
    codec: ( length-prefixed [ op:u8  len:u32  payload:bytes(len) ] )
  } rpc
```

The codec's `decode` runs `unpack-prefix` of the spec over the read buffer
(asking for more bytes via `{need:n}` until a frame completes); its `encode`
`pack`s the reply against the same segment shape. Routing is on `op` (a patrun
scalar tag, §3 of `PROCESSES.0.md`). This is the §5.3 protocol with all the
buffering deleted — the payoff of making bit-syntax a codec parameter.

### 6.6 A custom codec, written by the user

When no built-in fits, a codec is just a value. A minimal STOMP-ish
frame (`COMMAND\n…headers…\n\nbody\0`) shows the surface:

```boru
import "boru:net"
import "boru:bin-util"

def stomp-codec {
  decode: [ [buf] => [
    def end ( index-of (byte-ints buf) 0 )         # position of NUL frame terminator
    end lt 0
      ? {need: 1}                                   # incomplete: ask for more
      : { msg:  (parse-stomp (slice buf 0 end))
          rest: (slice buf (inc end) ((length buf) sub (inc end))) } ] ]
  encode: [ [reply] => [ render-stomp reply ] ]
}

def broker ( service {subs: {}} )
add {command: "SEND"}      [ [req state] => [ fan-out req  None ] ] broker
add {command: "SUBSCRIBE"} [ [req state] => [ add-sub req.@conn req  None ] ] broker

listen {tcp: 61613  codec: stomp-codec} broker
```

This is the extension story end to end: bytes → `{msg,rest}` via §3 primitives,
dispatch via patrun, and `@conn` for the broker's push to subscribers — no
change to the actor or service machinery.

## 7. Streaming data

Streaming is a first-class reply shape (`SERVICES.0.md` §1, §6) and binds to
`boru:stream` (`STREAM-WORDS.0.md`). Three directions:

### 7.1 Streaming replies (server → client)

A handler returns a `Stream<Bytes>` (or `Stream<Value>` the codec encodes
per element). The transport writes each element as it is produced — chunked
HTTP, Server-Sent Events, or repeated WS frames depending on the codec — and
never buffers the whole body (the vault-proxy requirement, `SERVICES.0.md` §6).

```boru
# Server-Sent Events: stream price ticks to a subscriber.
add {method: "GET" path: "/prices/stream"} [ [req state] => [
    price-channel req.params.symbol
      stream.from-channel
      [ [tick] => [ {event: "price"  data: tick} ] ] stream.map ] ] api
# codec `http` recognises a Stream reply → emits `Content-Type: text/event-stream`
# and writes one `data: …\n\n` per element until the stream ends or the client drops.
```

At Tier 1 the same is `send-stream <Stream<Bytes>> <Socket>` (§4.2).

### 7.2 Streaming request bodies (client → server)

A large or unbounded upload is presented to the handler as `req.body` being a
`Stream<Bytes>` rather than a materialised `Bytes`, so a handler folds it in
bounded memory:

```boru
add {method: "POST" path: "/upload"} [ [req state] => [
    req.body
      "./incoming.dat" stream.to-bytes        # bounded-memory spool to disk
    {status: 201} ] ] api
```

This is exactly `STREAM-WORDS.0.md`'s `from-bytes`/`to-bytes`, now sourced from a
socket instead of a file — closing the loop on that doc's already-declared
`Bytes` stream sources.

### 7.3 Back-pressure all the way to the socket

The load-bearing property: back-pressure composes from the TCP window through
the stream to the producer, and from the mailbox to the sender — one mechanism,
not two (`SERVICES.0.md` §7.2, §8.1).

- **Outbound:** `send-stream` / streamed replies write at the socket's drain
  rate; a slow client stalls `send-stream`, which (being a `Stream` consumer)
  stalls the producing stage — no unbounded buffer grows. A bounded
  `boru:stream` channel between producer and socket gives an explicit cap.
- **Inbound:** active-mode `"once"` (§4.3) is the socket analog of a bounded
  mailbox — the socket delivers one chunk, then waits to be re-armed, so a fast
  peer cannot flood the actor's mailbox past the §8.1 bound. Passive `recv`
  is back-pressured by construction (you read when ready).
- **Served services:** the §8.1 `mailbox`/`overflow` policy applies unchanged to
  the connection actor; `overflow:"block"` propagates TCP back-pressure to the
  peer, `"drop"` sheds (telemetry sockets), `"fail"` → `overload` for a load
  shedder to act on.

## 8. A larger worked example — a streaming binary gateway

Pulling the threads together: a server that terminates a length-prefixed binary
protocol, authorises via the capability model, **streams** a large result back,
and is supervised — exercising Tier 2, bit-syntax, streaming, and
`SERVICES.0.md` composition at once.

```boru
import "boru:net"
import "boru:serve"
import "boru:stream"

# A session per connection: holds the authenticated principal in private state.
def make-session fn [[meta:Map] [Service] [
  def s ( service {principal: None  peer: meta.@peer} )

  add {op: 1 (* HELLO *)} [ [req state] => [
      def who ( verify-token req.payload )                 # → principal or raise
      state set principal who
      {op: 1  ok: true} ] ] s

  # A streaming read: reply is a Stream<Bytes>, written frame-by-frame.
  add {op: 2 (* FETCH *)} [ [req state] => [
      state.principal eq None ? (raise unauthorized "say HELLO first") None
      require-cap state.principal "fetch"                  # PERMISSIONS.10.md scope
      open-blob req.payload                                 # → Stream<Bytes>, lazy
        4096 stream.chunks-of
        [ [chunk] => [ pack [ 2:u8 (length chunk):u32 chunk:bytes ] ] ] stream.map ] ] s

  s
]]

# Supervised, with a bounded mailbox and explicit backpressure.
serve ( server [
    ( listen {
        tcp: 9000
        tls: {identity: srv/q}                             # TLS at the socket (§4.4)
        codec: ( length-prefixed [ op:u8 len:u32 payload:bytes(len) ] )
        session: true                                       # one session per connection
      } make-session )
  ] {restart: "isolated"  mailbox: 1024  overflow: "block"} )
```

What each layer buys, in one place: `session:true` + private `state` = a secure
per-connection principal that never crosses a message boundary
(`SERVICES.0.md` §7.1); `length-prefixed [..]` = declarative framing (§3, §6.5);
a `Stream<Bytes>` reply = bounded-memory streaming with socket back-pressure
(§7); `require-cap` = the unified capability model (§9); `server …{restart}` =
supervision and bounded mailboxes (§8.1 of `SERVICES.0.md`). No byte loop, no
lock, no manual buffering — the DX target.

## 9. Capability & safety

Networking is the sharpest capability edge, and both scopes it needs **already
exist** in `PERMISSIONS.10.md` (confirmed in `SERVICES.0.md` §4) — so gating is
enforceable on day one, not new permission work:

- **`network.listen`** gates `listen` / `serve-raw` / `accept` (opening a server
  socket). The `sandbox`/`read-only`/`compute` profiles hard-deny it; only
  `trusted`/`full` may bind a port.
- **`network.connect`** gates `connect` and any outbound socket (it already
  gates `fetch`). A proxy/gateway needs both.
- **`process`** still gates the per-connection `spawn` (§2) via the substrate.

Denials carry the usual blame chain. A sandboxed sub-engine (`boru:vm`) can
attenuate networking away entirely, so untrusted boru can parse and transform
wire data (codecs are pure!) without ever being able to open a socket.

Two safety properties fall out of the model rather than needing enforcement:
**(1)** a crashed connection is isolated (`recover()` per actor, §2), so one
malformed frame kills one connection; **(2)** a codec is a **pure
`Bytes`→`Value` function** with no socket access, so the parsing attack surface
runs with no I/O capability at all — protocol parsing and protocol *serving* are
separable trust tiers.

## 10. Gap analysis

What this RFC adds, and what remains:

- **Added (binary):** the `Bytes` value type, `pack`/`unpack`/`unpack-prefix`
  bit-syntax, and the `boru:bin-util` byte words (§3) — also unblocks
  `STREAM-WORDS.0.md`'s already-referenced `from-bytes`/`to-bytes`.
- **Added (Tier 1):** `Listener`/`Socket` handles, `listen`/`accept`/
  `serve-raw`, passive `recv*` and active-mode messages, `send-bytes`/
  `send-stream`, TLS-as-socket-option, the `NetRuntime` runtime owner (§4) —
  closing the "no TCP/socket server" gap of `PROCESSES.0.md` §8.
- **Added (Tier 2):** the `Codec` type + extension point, built-in codecs
  (`lines`/`json-lines`/`http`/`jsonrpc`/`length-prefixed`/`websocket`),
  `listen`/`connect` over services, message metadata (`@conn`/`@peer`),
  shared vs session-per-connection, lifecycle-as-requests (§6) — filling the
  `listen`/`connect` placeholder of `SERVICES.0.md` §4.
- **Added (streaming):** streaming replies and request bodies over sockets, and
  socket↔stream↔mailbox back-pressure composition (§7).
- **Depends on:** `PROCESSES.0.md` phase 1 (actors), `SERVICES.0.md` phases 1–2
  (services, `serve`, supervision, bounded mailboxes), `boru:stream`.
- **Still out of scope:** HTTP/2 & QUIC (start with HTTP/1.1 + WS); a full
  TLS-client story for `connect` (mutual TLS); protocol *fuzzing* harnesses;
  cross-node distribution (`SERVICES.0.md` §9 "later"). UDP/datagram sockets are
  a near-term follow-on (the `Socket` model is TCP/stream-shaped; datagrams want
  a `recv-from`/`send-to` pair) — flagged, not designed here.

## 11. Phased roadmap

Layered so each phase is independently testable and the cheapest useful slice
lands first.

- **Phase A — `Bytes` + bit-syntax (§3).** The value type, `pack`/`unpack`/
  `unpack-prefix`, `boru:bin-util` words. **No networking.** Highest-leverage and
  independently useful (binary files, hashing, `boru:stream` bytes). The
  recommended first slice — it only touches the type table and a new module.
- **Phase B — Tier 1 low-level (§4–5).** `Listener`/`Socket`, `listen`/
  `accept`/`serve-raw`, passive + active reads, `send-bytes`/`send-stream`, TLS.
  Depends on Phase A and `PROCESSES.0.md` phase 1. Delivers raw actor-per-
  connection servers (§5 examples) end to end.
- **Phase C — Tier 2 high-level (§6).** The `Codec` type + `listen`/`connect`
  over `service`s, the built-in codecs, metadata, sessions. Depends on Phase B
  and `SERVICES.0.md` phases 1–2. Delivers the HTTP/JSON/binary message servers
  (§6 examples) — the network-server end-goal.
- **Phase D — streaming polish + CLI refactor (§7, §8).** Streaming
  bodies/replies wired through codecs; back-pressure tuning; begin folding the
  CLI's own servers (`registry`/`lsp`/`api`, `SERVICES.0.md` §5) onto the codec
  model, with the vault-proxy as the streaming proving ground.
- **Later:** HTTP/2, UDP, mutual TLS, cross-node distribution.

## 12. Open questions

1. **`Socket` ownership transfer semantics.** When a listener `spawn`s a handler
   and hands over the `Socket`, do we *require* an explicit `set-owner`, or make
   the spawned actor the owner implicitly by capture? (Leaning: implicit on
   `serve-raw`'s spawn — the common path — with explicit `set-owner` for the
   rare hand-off between live actors.)
2. **Bit-syntax sub-byte alignment.** `bits(n)` allows non-byte-aligned fields;
   should `pack`/`unpack` *require* the segment list to sum to a whole number of
   bytes (reject ragged frames at build time), or allow trailing bit padding
   like Erlang? (Leaning: require byte-alignment by default, with an explicit
   `pad(n)` segment to opt into padding — safer, still expressive.)
3. **Codec error surface.** When `decode` hits a malformed frame, does it
   `raise` (killing the connection, "let it crash") or return a sentinel the
   transport turns into a protocol error reply? (Leaning: `raise` by default —
   per-connection isolation makes this cheap — with a codec opting into a
   `{error:…}` reply for protocols that have an error frame, like HTTP 400.)
4. **Default codec for `tcp:` with no `codec:`.** Should `listen {tcp:N} svc`
   with no codec be an error (force an explicit choice), or default to
   `json-lines` (the stated primary use case)? (Leaning: error — a wire protocol
   should never be implicit.)
5. **`http` routing power.** How much routing does the `http` codec do itself
   (path params, method, query) vs. leaving it to patrun on a flat message?
   §6.4 assumes `:id` path patterns and `req.params`; is that the codec's job or
   a thin `boru:net` router layer above the service? (Leaning: a small router in
   `boru:net` that produces patrun-friendly messages, kept separate from the raw
   `http` codec so non-REST HTTP is still possible.)
6. **WebSocket as codec vs. upgrade.** WS starts as HTTP then switches framing
   mid-connection. Model it as one `websocket` codec that does the handshake, or
   as an explicit `upgrade` step a handler performs on an `http` connection?
   (Leaning: a `websocket` codec for the common "WS endpoint" case, with a
   lower-level `upgrade <Socket>` for hybrid HTTP+WS servers.)
7. **Streaming reply detection.** The transport distinguishes a `Stream` reply
   from a value reply by type (§7.1). Is type-dispatch enough, or do we want an
   explicit `stream-reply` wrapper so a service can stream a value that *happens*
   to be a `Stream`? (Leaning: type-dispatch is enough; `Stream` is already a
   distinct type.)
8. **Per-connection backpressure default.** Active mode defaults to `"once"`
   (safe) but costs a re-arm per chunk. Should high-throughput servers get a
   bounded-active mode (`{active: N}` — deliver up to N then wait, Erlang's
   `{active, N}`) as the middle ground? (Leaning: yes, ship `{active: N}` in
   Phase B; it is the same counter as the bounded mailbox.)
