# NETWORK-CLIENTS

Design for the **network-client handling API** in boru — the dial side of
`NETWORK-SERVERS.0.md`, made deliberately **symmetric** with it so that a
developer who has learned to write a server already knows how to write a client.

A network connection is bidirectional; the *only* real asymmetry between the two
ends is which one binds a port (`listen`) and which one dials (`connect`). Once
the connection exists, both ends are peers — either may send, either may handle.
This document makes that symmetry explicit and turns it into DX:

- the **`Socket`, `Codec`, `Bytes`/bit-syntax, and streaming** machinery of
  `NETWORK-SERVERS.0.md` is **shared unchanged** — a client is not a second
  stack; and
- the high-level client handle is a **unified bidirectional endpoint**: a
  `Service`-typed value you can both `call`/`send` *to the peer* **and** `add`
  handlers to *for messages the peer pushes back*. The same `Endpoint` type
  describes a `connect`ed client and a served connection's session.

It mirrors `NETWORK-SERVERS.0.md` section-for-section. Where a concept is shared,
this doc **references** the server doc rather than restating it. Like its
sibling, this is a **design RFC only — no implementation code yet**.

> **Decisions taken at design-review time:** (1) the client lives in **this
> companion doc**, mirroring the server doc's structure and referencing its
> shared machinery; (2) the symmetry goes all the way to a **unified
> bidirectional endpoint** — both `connect`-side and `listen`-side yield a
> `Service`-typed endpoint that can `call`/`send` to the peer and `add` handlers
> for inbound, so full-duplex protocols (JSON-RPC, chat, SSE, gRPC-bidi) read
> naturally on either end.

## 0. Review — what is shared, what the client adds

The client reuses almost everything; it adds the dial primitive, reply
correlation, and the peer-handler direction.

| Concern | Defined in | Client side |
| --- | --- | --- |
| `Bytes` type + `pack`/`unpack`/`unpack-prefix` bit-syntax | `NETWORK-SERVERS.0.md` §3 | **identical** — used to build requests and match replies (§3) |
| `Socket` handle + `recv*`/`send-bytes`/`send-stream`/active mode | server §4.1–4.3 | **identical** — a dialed socket *is* a `Socket` (§4) |
| `Codec` (bytes ⇄ messages) + built-ins | server §6.1–6.2 | **identical** — the same codec frames a client and a server (§6) |
| Streaming replies / request bodies / backpressure | server §7 | **identical**, directions swapped (client consumes server streams, uploads request streams) (§7) |
| `service`/`add`/`call`/`send`/`state`/`wrap`, supervision, `pool` | `SERVICES.0.md` | the endpoint *is* a `Service`; `pool` becomes a connection pool (§6.5) |
| Uniform failure contract (`timeout`/`down`/`overload`/`transport`, retries) | `SERVICES.0.md` §8.2 | **this is where it earns its keep** — a client `call` is the canonical fallible call (§6.4) |
| **Dial** (`connect-raw`, `connect`) | — | **new here** (§4.4, §6) |
| **Reply correlation** over a wire | — | **new here** (§6.3) |
| **Peer-push handlers** (inbound dispatch on a client) | — | **new here** — the unified endpoint (§6) |

In one line: **the client is the server's machinery dialed instead of bound,
plus reply correlation and the realisation that a client can handle messages
too.**

## 1. The two tiers at a glance

The same dial job at both tiers, mirroring `NETWORK-SERVERS.0.md` §1.

**Low level (you own the bytes):**

```boru
import "boru:net"

def sock ( connect-raw {tcp: "localhost:7"} )   # dial → a Socket (same type as accept's)
send-bytes (utf8 "ping\n") sock
to-text (recv-until sock (utf8 "\n"))                  # → "ping"  (echo server)
close sock
```

**High level (a codec gives you a remote service):**

```boru
import "boru:net"
import "boru:serve"

def echo ( connect {tcp: "localhost:7"  codec: lines} )   # → an Endpoint (a Service)
call {line: "ping"} echo                                    # → "ping"
```

`connect-raw` is the exact dual of `accept` (both yield a `Socket`); `connect` is
the exact dual of `listen {codec} svc` (one binds a service to a port, the other
binds a port to a local `Service` handle). Everything below fills in those two
surfaces and the bidirectional-endpoint model they share.

## 2. The connection model (BEAM → dialing) and the symmetry table

Dialing maps onto the same actor model: a `connect`ed client is (at the high
tier) a small **connection actor** that owns the socket, correlates replies, and
dispatches peer pushes to handlers — the mirror image of a served connection's
session actor. The substrate (`PROCESSES.0.md`) and the runtime owner
(`NetRuntime`, `NETWORK-SERVERS.0.md` §2) are unchanged; `connect-raw`/`connect`
register their sockets with the same `NetRuntime` for shutdown.

The DX payoff is the **symmetry table** — read either column and the other tells
you the dual:

| Concern | Server (`listen`-side) | Client (`connect`-side) |
| --- | --- | --- |
| Establish | `listen {tcp:…} -> Listener` (bind) | `connect-raw {tcp:…} -> Socket` (dial) |
| Raw socket | `accept <Listener> -> Socket` | `connect-raw {tcp:…} -> Socket` |
| Tier-1 sugar | `serve-raw {tcp} [handler]` (accept-loop) | `connect-raw` (one shot); `pool` for many (§6.5) |
| High-level handle | `listen {tcp codec} <service>` | `connect {tcp codec} -> Endpoint` |
| Handle type | a `Service` (the session) | a `Service` (the `Endpoint`) |
| Outbound to peer | `send {…} req.@conn` (server push) | `call`/`send` to the endpoint |
| Inbound from peer | `add {pattern} [handler]` (requests) | `add {pattern} [handler]` (pushes) |
| Reply | return a value / `Stream` / `defer` | a `call` returns the value / `Stream` |
| Lifecycle hooks | `{op:"@connect"}` / `{op:"@disconnect"}` | same two messages |
| Codec | shared | shared |
| Streaming | reply / request body (§7 server) | consume / upload (§7 here) |
| Fault tolerance | `server [..] {restart}` supervises sessions | supervised endpoint / pool reconnects (§6.6) |
| Capability | `network.listen` | `network.connect` |

The only rows that differ in *kind* rather than *direction* are "Establish" and
"Capability" — everything else is the same word pointing the other way.

## 3. `Bytes` and bit-syntax — shared, used in reverse

The `Bytes` type and `pack`/`unpack`/`unpack-prefix` of `NETWORK-SERVERS.0.md`
§3 are used **unchanged**; the client just runs them in the opposite order from a
server. Where a server `unpack`s a request and `pack`s a reply, a client
`pack`s a request and `unpack`s a reply:

```boru
import "boru:bin-util"

# build a request frame  [u8 op][u32 len][len bytes payload]
def body  ( utf8 (jsonify {user: 42}) )
def frame ( pack [ 2:u8  (length body):u32  body:bytes ] )

# … send it, read the reply frame, then match it (sized by the reply's own len)
def r ( unpack reply-bytes [ op:u8  len:u32  payload:bytes(len) ] )
reify (to-text r.payload)
```

Because `Bytes` is immutable and sendable (server §3.1), a built request frame
is a zero-copy value that can flow through `pool`s, `wrap` layers, and the
mailbox exactly like any other value. No new binary machinery is needed on the
client.

## 4. Tier 1 — the low-level client API (`boru:net`)

For protocol authors and the tightest control. You dial a `Socket` and then use
the **same socket words as the server** (`NETWORK-SERVERS.0.md` §4.1–4.3):
`recv`/`recv-bytes`/`recv-until`/`recv-frame`, `send-bytes`/`send-stream`,
passive (default) and active (`set-active`) modes, `{within:}` deadlines,
`close`/`shutdown`. A `Socket` from `connect-raw` and a `Socket` from `accept`
are the same type with the same operations — that is the load-bearing symmetry.

### 4.4 Dialing — `connect-raw`

```
connect-raw {tcp: <"host:port">  …opts} -> Socket      # network.connect gated (§9)
```

Options mirror `listen`'s where they make sense: `tls: {…}` (TLS client; with
`verify:`/`ca:`/`sni:`/`min:` and `identity:` for mutual TLS — see the box
below), `within: <Duration>` (connect deadline → `transport` on failure),
`bind: <addr>` (source address), `unix:` (Unix-domain). It raises `transport`
on connection refused / DNS / TLS failure — the same `transport` code a high-
level `call` surfaces (`SERVICES.0.md` §8.2), so the failure vocabulary is one
set from the bottom up.

> **AMENDED (as built) — a client certificate is NAMED, not pathed.** This
> section originally specified `cert:`/`key:` as **file paths**, and the §8
> gateway example below still shows that form for `listen`. That was not
> implemented and should not be: the path would be chosen by the guest but
> dereferenced by the network capability under host authority, so a program
> holding `network` but not `fileops` would obtain a file read it is not
> entitled to — and, even blind to the bytes, an oracle for path existence
> and key well-formedness. It also puts key locations in source, logs and
> error text, and forecloses rotation and hardware-backed keys.
>
> What shipped instead is `identity:`, naming a credential the HOST
> registered:
>
> ```
> connect-raw {tcp: "billing.internal:443"  tls: {identity: acme/q}}
> fetch {url: "https://api.internal/v1"     tls: {identity: acme/q}}
> ```
>
> The host registers with `(*Boru).RegisterClientIdentity(name, id)`, where
> `id` is a `ClientIdentity` — an interface returning a `*tls.Certificate`
> **per handshake**, so the private key may live in a file, a vault slot, an
> HSM or a SPIFFE agent that rotates hourly, and never becomes a boru value.
> Guest source can SELECT an identity; it can never read or construct one.
> Which identity a program may present, and against which host, is a policy
> decision (`network`/`client-cert`), not the program's.
>
> A program that must choose its own credential without the host naming it
> in advance gets one from the vault:
>
> ```
> import "boru:vault"
> fetch {url: "https://api.internal/v1"  tls: {identity: (Vault.identity "acme-mtls")}}
> ```
>
> `Vault.identity` returns an **opaque handle**, not a String. `Vault.reveal`
> returns a String, which is right for an API token a program pastes into a
> header and wrong for a private key: the moment a key is a guest value it
> can be printed, logged, stored, or sent somewhere. The handle formats as
> `<vault identity>` — it does not even print its alias — and the vault is
> read lazily, inside the handshake, so a rotated secret is picked up
> without re-running the program.
>
> Explicit `cert:`/`key:` **`Bytes`** (obtained through `boru:io` or
> `boru:pki` under their own gates) remain available as a deliberate escape
> hatch — but never as a path the network layer dereferences.
>
> The holding principle: **a certificate is data, a private key is a
> capability.** Rationale and phasing in
> [legacy/NETWORK-TLS-PLAN.0.ignore](legacy/NETWORK-TLS-PLAN.0.ignore) §3.

There is deliberately **no `dial-loop` sugar**: a server accepts *many*
connections (hence `serve-raw`'s loop), a client dials *one*. Dialing many — a
connection pool — is the high-level `pool` (§6.5), not a Tier-1 loop.

## 5. Tier-1 worked examples (DX assessment)

A spread mirroring `NETWORK-SERVERS.0.md` §5, all at the low level.

### 5.1 Line/text — a line client

```boru
import "boru:net"

def line-rpc fn [[sock:Socket  msg:String] [String] [
  send-bytes (utf8 (str.concat [ msg "\n" ])) sock
  to-text (recv-until sock (utf8 "\n"))
]]

def sock ( connect-raw {tcp: "localhost:8001"} )
line-rpc sock "hello"        # → server's line reply
line-rpc sock "world"        # reuse the same connection
close sock
```

### 5.2 JSON-over-TCP — newline-delimited JSON client

```boru
import "boru:net"

def json-rpc fn [[sock:Socket  req:Map] [Map] [
  send-bytes (concat [ (utf8 (jsonify req))  (utf8 "\n") ]) sock
  reify (to-text (recv-until sock (utf8 "\n")))
]]

def sock ( connect-raw {tcp: "localhost:8002"} )
json-rpc sock {op: "get" id: 7}      # → the decoded JSON reply
```

### 5.3 Length-prefixed binary RPC — `pack` request, `recv-frame` reply

The exact dual of `NETWORK-SERVERS.0.md` §5.3: the client `pack`s the request
and reads the reply with `recv-frame` (which pulls exactly one frame, using the
reply's own length field).

```boru
import "boru:net"
import "boru:bin-util"

def bin-rpc fn [[sock:Socket  op:Integer  payload:Map] [Map] [
  def body ( utf8 (jsonify payload) )
  send-bytes (pack [ op:u8  (length body):u32  body:bytes ]) sock
  def r ( recv-frame sock [ op:u8  len:u32  payload:bytes(len) ] )
  reify (to-text r.payload)
]]

def sock ( connect-raw {tcp: "localhost:8003"} )
bin-rpc sock 2 {echo: "hi"}          # → {op:2 result:…}
```

### 5.4 Full-duplex raw — a chat client mixing socket + console

The dual of the server's §5.4 chat *connection*: a client that must react to
**both** inbound broadcasts (from the socket) **and** lines the user types
(delivered to the same actor). This is the Tier-1 case that needs **active
mode**, exactly as the server's did.

```boru
import "boru:net"

def chat-client fn [[sock:Socket] [Never] [
  set-active sock "once"                         # socket data arrives as messages
  spawn [ sock console-pump ]                    # an actor that reads stdin → us
  chat-loop sock
]]

def chat-loop fn [[sock:Socket] [Never] [
  receive [
    {tag: "data" bytes: Bytes}   [ print (to-text bytes)   set-active sock "once"  chat-loop sock ]
    {tag: "line" text: String}   [ send-bytes (utf8 text) sock  chat-loop sock ]   # from console-pump
    {tag: "closed"}              [ None ]
  ]
]]
```

The DX read is symmetric to the server's: passive `recv` (5.1–5.3) is cleaner
when the actor only reacts to its own socket; active mode (5.4) earns its keep
when the actor is *also* an addressable participant in a wider message graph.

## 6. Tier 2 — the high-level client: the bidirectional `Endpoint`

`connect` returns an **`Endpoint`** — a `Service` value (the `SERVICES.0.md`
type) bound to a remote peer over a codec. Because it is a `Service`, *everything
you know about services applies*: `call`/`send` dispatch outbound to the peer;
`add` registers handlers for messages the peer pushes back; `wrap` layers
cross-cutting concerns; `state` holds per-connection client state; it can be
`pool`ed and supervised. The same `Endpoint` type also describes a served
connection's session, so a function can take an endpoint and not care which end
of the wire it is on.

```
connect {tcp: <addr>  codec: <codec>  …opts} -> Endpoint      # network.connect gated
```

### 6.1 The reduction to Tier 1 (mirror of server §6.1)

`connect {tcp codec}` desugars to a connection actor over a `connect-raw`
socket — the dual of the server's `conn-session`:

```boru
# Conceptual desugaring of `connect {tcp:A codec:C}`:
def ep ( endpoint C )                          # a Service that also tracks pending calls
def sock ( connect-raw {tcp: A} )
spawn [ ep sock C client-pump (convert Bytes []) ]   # actor: decode peer bytes → replies + pushes
ep                                             # return the Endpoint handle to the caller
```

The `client-pump` actor reads bytes, runs `codec.decode`, and routes each decoded
message either to the **pending call** it answers (correlation, §6.3) or, if it
matches no pending call, into the endpoint's **`add` handlers** as a peer push.
An outbound `call`/`send` runs `codec.encode` and `send-bytes`. No new
concurrency — same actor-per-connection as the server.

### 6.2 Codecs — shared, with a correlation capability

The built-in codecs of `NETWORK-SERVERS.0.md` §6.2 are reused unchanged. One new
*property* matters for clients: a codec is either **correlated** (carries a
request/response id, so `call` can match a reply to its request) or **streaming/
uncorrelated** (no id; use `send` + push handlers). This is honest and symmetric
— a server can only `reply` to a `call` when the codec frames replies, and a
client can only `call` when the codec correlates them.

| Codec | Correlated? | Client uses |
| --- | --- | --- |
| `http` | yes (request↔response pairing) | `call` → response; streaming bodies via `Stream` |
| `jsonrpc` | yes (`id`) | `call` (request), `send` (notification), `add` (server→client requests) |
| `length-prefixed [ … id:… ]` | yes *iff* the spec reserves an id field | `call`/`send` |
| `lines` / `json-lines` | no by default | `send` + `add` push handlers (or app-level id) |
| `websocket` | no (frames) | `send` + `add`; `call` only with an app-level id field |

For an uncorrelated protocol that nonetheless wants request/reply, the app
supplies its own id field and matches the reply with an `add` handler — the
same correlation-by-tag convention `PROCESSES.0.md` §10 uses for actor replies.

### 6.3 Reply correlation, metadata, peer pushes

When `call {req} ep` runs over a correlated codec, the transport assigns a
correlation id, records a **pending-call** entry keyed by it (with the caller's
reply channel and deadline), encodes the request, and writes it. When
`client-pump` decodes a message carrying that id, it completes the pending call
(zero-copy, on the caller's reply channel — never a mailbox scan, mirroring the
served `call` of `SERVICES.0.md` §4). A decoded message with **no** matching
pending id is a **peer push**: it is dispatched through the endpoint's `add`
handlers exactly like a server dispatches a request.

Message metadata mirrors the server (`NETWORK-SERVERS.0.md` §6.3):

- **`@peer`** — `{host port}` of the remote (here, the server).
- **`@conn`** — the endpoint's own connection actor (so other local actors can
  push outbound through this connection: `send {…} ep`).
- **`@from`** — on a peer push that is itself a *request* (full-duplex, e.g. an
  LSP server→client request), the reply target; the client's handler returns a
  value and the transport routes it back. On a one-way push (SSE, chat
  broadcast) there is no `@from` and the handler's return is discarded — same as
  a server-side async `send`.

Lifecycle is delivered as requests so handlers can `add` for them, identically to
the server: `{op:"@connect" @peer}` once connected, `{op:"@disconnect" reason}`
on close (the latter is also what a supervised endpoint reconnects on, §6.6).

### 6.4 Examples — line, JSON, HTTP REST, and the uniform failure contract

**JSON-lines and the duals of §5.1–5.2 without the framing:**

```boru
import "boru:net"
import "boru:serve"

def api ( connect {tcp: "localhost:8002"  codec: json-lines} )
call {op: "get" id: 7} api           # request/reply if the server echoes the id field
send {op: "log" line: "hi"} api      # fire-and-forget
```

**HTTP/JSON REST — and the relationship to `fetch`.** A REST client is `connect`
+ `call`; the existing one-shot `fetch` (`lang/go/native/fetch.go`) becomes
**sugar over a one-shot HTTP `connect`+`call`+close**, so there is one HTTP
client model with two ergonomic front-ends (persistent endpoint vs. one-liner):

```boru
import "boru:net"

def todos ( connect {tcp: "api.example.com:443"  tls: {} codec: http} )

call {method: "GET"  path: "/todos"} todos                      # → {status body …}
call {method: "POST" path: "/todos"  body: (jsonify {text:"x"})} todos

# `fetch` is the sugar — a single request without a kept-alive endpoint:
fetch {method: "GET"  url: "https://api.example.com/todos"}      # == connect+call+close
```

**The uniform failure contract is the client's home turf.** Every client `call`
carries the `SERVICES.0.md` §8.2 contract — a mandatory deadline and the closed
`timeout`/`down`/`overload`/`transport` delivery-error set, with idempotent-only
retries — because a client is *always* remote. The §8.2 billing example *is* a
client; restated here as the canonical client read:

```boru
import "boru:net"
import "boru:time-util"

def billing ( connect {tcp: "billing.internal:443"  tls: {} codec: http} )

# Read path — idempotent → safe to auto-retry; degrade on failure.
def total (
  do [ call {method:"GET" path:"/total" user: uid} billing
         {timeout: (TimeUtil.seconds 2)  retries: 3  idempotent: true} ]
  error [ case [
      [get code eq timeout/q]   [ -1 ]                          # unknown → show stale
      [get code eq down/q]      [ -1 ]
      [get code eq transport/q] [ raise unavailable "billing offline" ]
      [ raise ] ] ] )                                           # app errors propagate

# Write path — NOT idempotent → never blind-retry; reconcile a timeout.
do [ call {method:"POST" path:"/charge" user: uid cents: 500} billing
       {timeout: (TimeUtil.seconds 5)} ]
error [ case [
    [get code eq timeout/q]  [ enqueue-reconciliation uid ]     # unknown → verify
    [get code eq overload/q] [ retry-later uid ]                # definitely not charged
    [ raise ] ] ]
```

**Binary RPC client — the §5.3 protocol declaratively (dual of server §6.5):**

```boru
import "boru:net"

def rpc ( connect {
    tcp: "localhost:8003"
    codec: ( length-prefixed [ op:u8 id:u32 len:u32 payload:bytes(len) ] )  # `id` makes it correlated
  } )

call {op: 2  payload: (utf8 (jsonify {echo:"hi"}))} rpc      # transport fills `id`, matches reply
```

### 6.5 Full-duplex and connection pooling — the unified endpoint paying off

**Full-duplex (the model's reason to exist).** A chat client `send`s lines *and*
`add`s a handler for broadcasts — on one `Endpoint`, no separate push API. This
is the §5.4 raw client with the framing and the active-mode plumbing gone:

```boru
import "boru:net"

def room ( connect {tcp: "chat.example.com:6667"  codec: lines} )

# handle messages the SERVER pushes to us (broadcasts) — same `add` as a server
add {} [ [msg state] => [ print msg.line  None ] ] room

# send our own lines whenever we like — same `call`/`send` as any service
send {line: "hello everyone"} room
```

**Bidirectional JSON-RPC (LSP client).** The endpoint both calls server methods
*and* answers server→client requests — the case that flatly needs the unified
endpoint:

```boru
import "boru:net"

def lsp ( connect {tcp: "localhost:9999"  codec: jsonrpc} )

# we call the server
call {method: "initialize"  params: {…}} lsp

# the server calls us back (e.g. workspace/configuration) — we handle and reply
add {method: "workspace/configuration"} [ [req state] => [ load-config req.params ] ] lsp
# the server notifies us (no reply) — handle, return discarded
add {method: "textDocument/publishDiagnostics"} [ [req state] => [ show-diags req.params  None ] ] lsp
```

**Connection pooling — the dual of the server's worker pool.** Many outbound
connections to one peer (or a set) are a `pool` of endpoints (`SERVICES.0.md`
§9.2) — the same word, pointed outward. Being a `Service`, the pool is a drop-in
for a single endpoint:

```boru
import "boru:net"
import "boru:serve"

# 16 pooled HTTP connections; `call` picks one by power-of-two-choices.
def api ( pool [ [ connect {tcp:"api.example.com:443" tls:{} codec:http} ] ]
            {size: 16  strategy: "p2c"} )

call {method:"GET" path:"/health"} api        # transparently uses one of the 16
```

A `"hash" <key>` pool gives **connection affinity** (sticky sessions); a
multi-target pool whose endpoints point at different hosts *is* a client-side
load balancer / failover group, with `down`/`transport` driving failover —
identical strategies to the server pool (`SERVICES.0.md` §9.2), because the
surface is uniform.

### 6.6 Reconnection & supervision — the dual of the server's restart

A long-lived client must survive the peer restarting. Symmetric with the
server's `server [..] {restart}`, an endpoint opts into **automatic
reconnection**:

```boru
connect {tcp: addr  codec: c  reconnect: {retries: "forever"  backoff: "exponential"}} -> Endpoint
```

On `down`/`transport`, the endpoint actor re-dials per the backoff policy;
in-flight `call`s get `down` (their outcome is unknown — the §8.2 retry rules
apply), and a `{op:"@connect"}` fires again on success so the handler can
re-subscribe/re-authenticate. A pooled endpoint reconnects per member. This is
the client analog of supervision: the server *restarts a crashed session*; the
client *re-dials a dropped connection*. Placing the endpoint in a `server` makes
its connection actor a supervised child, so reconnection composes with the rest
of the supervision tree.

## 7. Streaming data — directions swapped

The streaming machinery of `NETWORK-SERVERS.0.md` §7 is reused with client and
server roles swapped; ties to `boru:stream` (`STREAM-WORDS.0.md`) unchanged.

### 7.1 Consuming a server stream (server → client)

A `call` over a streaming endpoint returns a **`Stream`** the client consumes
lazily, in bounded memory, with TCP back-pressure pacing it — the dual of the
server's streaming reply (server §7.1). SSE, chunked downloads, and WS message
streams all surface this way:

```boru
import "boru:net"
import "boru:stream"

def feed ( connect {tcp: "market.example.com:443"  tls: {} codec: http} )

# the reply is a Stream<Map> of SSE events; consume it like any stream
call {method: "GET"  path: "/prices/stream?sym=boru"} feed
  [ [ev] => [ ev.data ] ] stream.map
  [ [px] => [ println px ] ] stream.for-each      # back-pressured; ends when server closes
```

A subscription is the same shape with a push handler: `add` a handler for the
server's pushed events and `send` a subscribe request — for codecs without a
streaming `call`.

### 7.2 Uploading a request stream (client → server)

A large upload is a streaming request body: pass a `Stream<Bytes>` as the body
and the endpoint writes it with socket back-pressure — the dual of the server's
streaming request body (server §7.2), and Tier-1 `send-stream` underneath:

```boru
import "boru:net"
import "boru:stream"

def store ( connect {tcp: "uploads.example.com:443"  tls: {} codec: http} )

call {method: "PUT"  path: "/blob/42"
      body: ( "./big.dat" stream.from-bytes )} store      # streamed, bounded memory
```

### 7.3 Back-pressure — same single mechanism

Back-pressure composes exactly as in server §7.3, directions swapped: a slow
*server* stalls a client's consuming `Stream` (no unbounded client buffer); a
slow *server* stalls a client's `send-stream` upload; active `"once"` bounds
inbound pushes into the endpoint's mailbox; and a pooled/served endpoint's
mailbox `overflow` policy (`SERVICES.0.md` §8.1) sheds or paces outbound load.
One mechanism, both ends.

## 8. A larger worked example — a gateway is a client *and* a server

The capstone that proves the symmetry: an **API gateway** is simply a `listen`
(server) whose handlers `call` a `connect`ed upstream (client) — the two halves
of this design composed, which is exactly the `proxy` of `SERVICES.0.md` §6. One
program, both directions, one vocabulary.

```boru
import "boru:net"
import "boru:serve"

# Client half: a pooled, auto-reconnecting endpoint to the upstream service.
def upstream ( pool [ [ connect {
      tcp: "upstream.internal:443"  tls: {}  codec: http
      reconnect: {retries: "forever"  backoff: "exponential"}
    } ] ] {size: 32  strategy: "p2c"} )

# Server half: a public service whose handlers forward to the client half.
def gw ( service {} )

wrap [ [req state prior] => [ require-cap req "gateway"  prior req ] ] gw   # auth (server §9)

add {method: "GET"  path: "/v1/*rest"} [ [req state] => [
    # forward downstream→upstream with the uniform failure contract on the call
    do [ call {method: "GET"  path: (str.concat ["/internal/" req.params.rest])} upstream
           {timeout: (TimeUtil.seconds 5)  retries: 2  idempotent: true} ]
    error [ case [
        [get code eq timeout/q]   [ {status: 504  body: {error: "upstream timeout"}} ]
        [get code eq down/q]      [ {status: 502  body: {error: "upstream down"}} ]
        [get code eq transport/q] [ {status: 502  body: {error: "upstream unreachable"}} ]
        [ raise ] ] ] ] ] gw

serve ( server [ ( listen {tcp: 8443  tls: {identity: gw/q}  codec: http} gw ) ]
          {restart: "isolated"  mailbox: 1024  overflow: "block"} )
```

What the symmetry buys here, in one place: the **same `http` codec** frames both
the downstream server and the upstream client; the **same `call`** the public
clients use is the call the gateway makes upstream; the **same uniform failure
contract** turns upstream failures into clean downstream status codes; the
**same `pool`/`reconnect`** that load-balances a client also fronts the upstream;
and the **same `Endpoint` type** lets a streaming response flow straight through
(server §7 + §7.1 here) without buffering. A proxy is not a third thing — it is
this document plus its sibling, composed.

## 9. Capability & safety

Symmetric with `NETWORK-SERVERS.0.md` §9 and already present in
`PERMISSIONS.10.md`:

- **`network.connect`** gates `connect` / `connect-raw` / `fetch` (any outbound
  socket). The `sandbox`/`read-only`/`compute` profiles hard-deny it; a client
  that may only reach an *allow-listed* host set is expressed as the scope's host
  policy (the same machinery the vault proxy uses, `SERVICES.0.md` §6).
- **`process`** gates the per-connection `spawn` (the endpoint's connection
  actor) via the substrate.
- TLS verification is **on by default** for `connect-raw`/`connect`
  (`verify:true`); disabling it must be explicit (`tls:{verify:false}`), so a
  client never silently drops authentication — the dual of the server never
  silently binding without a codec (server open-Q #4).

The two model-level safety properties are symmetric too: a dropped connection
isolates to one endpoint actor (`recover()`, never the host); and a codec stays a
**pure `Bytes`→`Value` function** with no socket capability, so reply-parsing
runs with no I/O rights — a malicious server's payload cannot reach the network
through the parser. Parsing and connecting are separable trust tiers on the
client just as on the server.

## 10. Gap analysis

- **Added (Tier 1):** `connect-raw` (dial → `Socket`), TLS-client options
  including mutual TLS, connect deadlines (§4) — reusing all server socket words.
- **Added (Tier 2):** the **`Endpoint`** (a bidirectional `Service` over a
  codec), `connect` over codecs, **reply correlation** + pending-call tracking,
  **peer-push dispatch** via `add`, `@peer`/`@conn`/`@from` on the client side,
  `reconnect` policy, and `fetch`-as-`connect`-sugar (§6).
- **Added (streaming):** client stream consumption and streaming uploads (§7),
  reusing the server's streaming machinery in reverse.
- **Reused unchanged:** `Bytes`/bit-syntax (server §3), all `Socket` words
  (server §4), all built-in codecs (server §6.2), `pool`/supervision/uniform
  failure contract (`SERVICES.0.md`).
- **Depends on:** `NETWORK-SERVERS.0.md` (Bytes, Socket, Codec), `PROCESSES.0.md`
  phase 1, `SERVICES.0.md` phases 1–2, `boru:stream`.
- **Still out of scope:** HTTP/2 & QUIC clients, UDP datagram clients
  (`recv-from`/`send-to`), a connection-multiplexing single socket (HTTP/2-style
  stream IDs over one connection — distinct from a `pool` of sockets), and
  cross-node distribution (`SERVICES.0.md` §9 "later").

## 11. Phased roadmap (aligned with the server phases)

The client lands *alongside* the matching server phase — each server phase and
its client dual are testable against each other (a client example is the
integration test for its server example).

- **Phase A — `Bytes` + bit-syntax.** Shared with the server (server §11 Phase
  A); nothing client-specific.
- **Phase B — Tier 1.** `connect-raw` + TLS-client, landing with the server's
  raw sockets so §5's client/server pairs run end to end.
- **Phase C — Tier 2.** The `Endpoint`, `connect`, reply correlation, peer-push
  dispatch, `fetch`-as-sugar — landing with the server's codec tier so §6's
  pairs run end to end.
- **Phase D — streaming + pooling/reconnect.** Client stream consume/upload,
  `pool` of endpoints, `reconnect` policy — landing with the server's streaming
  polish; the §8 gateway is the cross-cutting acceptance test.
- **Later:** HTTP/2, UDP, single-connection multiplexing, distribution.

## 12. Open questions

1. **`connect-raw` vs. `dial` naming.** `connect-raw` is parallel to `connect`
   (codec vs. no codec), but `dial` is the more conventional client verb. Keep
   `connect-raw` for symmetry, or use `dial` and rename the server's `accept`
   relationship in docs? (Leaning: `connect-raw`, for the visible `connect`/
   `connect-raw` pairing that mirrors the server.)
2. **Correlation for uncorrelated codecs.** When a codec has no native id,
   should the transport *synthesise* one (wrap every frame in an envelope with an
   id) so `call` works universally, or refuse `call` and force `send`+handler?
   (Leaning: refuse by default — a synthesised envelope changes the wire format
   the peer sees — with an opt-in `{envelope: true}` for boru-to-boru links where
   both ends cooperate.)
3. **`fetch` migration.** Make `fetch` literally call the new HTTP `connect`
   path (one implementation), or keep the existing `fetch` and merely *document*
   the equivalence? (Leaning: re-implement `fetch` over `connect` once Phase C
   lands, so there is one HTTP client, but keep its terse signature.)
4. **Pending-call cap.** A correlated endpoint tracks pending calls; an unbounded
   pending map is a client-side memory footgun symmetric to the unbounded
   mailbox. Bound it (max in-flight calls per endpoint, `overload` past the cap),
   reusing the §8.1 mailbox bound? (Leaning: yes — same bound, same `overload`.)
5. **Reconnect & in-flight calls.** On reconnect, in-flight calls get `down`. Is
   that always right, or should idempotent calls (with `retries`) be re-sent on
   the *new* connection automatically? (Leaning: surface `down` and let the
   §8.2 retry rules re-issue — never silently replay a non-idempotent call.)
6. **Endpoint as a first-class value across the wire.** Since an `Endpoint` is a
   `Service` and a `Pid`-like handle, can it be *sent* to another local actor to
   share a connection (multiple producers `send` through one endpoint)? (Leaning:
   yes — it is a sendable handle like `Socket`/`Pid`; concurrent `call`s
   serialise through the connection actor, so sharing is safe by construction.)
7. **Symmetry of `@from` on the client.** A full-duplex peer push that is a
   *request* carries `@from` so the client can reply. Should the client be able
   to **`defer`** such a reply (await something, then `reply @from`) exactly like
   a server handler? (Leaning: yes — the endpoint is a `Service`, so `defer`/
   `reply` should work identically, completing the symmetry.)
```
