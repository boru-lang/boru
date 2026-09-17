# SERVICES

Design for a unified **service / server** model in boru — one model that covers
both *user code written in boru* (a value that owns state and answers
pattern-matched requests) and *the CLI's own long-running servers* (`repl`,
`registry`, `lsp`, `exec`, `api`, `tui`, `vault-proxy`). BEAM/OTP is the
inspiration for the request/reply + supervision shape, but the vocabulary stays
**boru-native**.

## Terminology (settled)

Two words, in a deliberate hierarchy:

- **service** — the unit. A value that owns **state** and answers requests
  matched against registered handlers (a gen_server, in OTP terms). This is also
  exactly the existing Go `service.Service` interface (`Name`/`Start`/`Stop`/
  `Status`). One service = one isolated unit of state + messages.
- **server** — a **collection of services**, supervised together. This is exactly
  the existing `serve` supervisor (`boru serve registry + lsp + …`). A server
  starts, stops, restarts, and exposes its services; it owns no request handlers
  of its own.

> **"Behaviour" is deliberately avoided.** In boru a `Behavior` is already a
> core type-system concept — the per-type operation dispatch (Match/Format/
> Equal/Compare) in `eng/go/compare_scalar_behaviors.go` and
> `lang/spec/user-types.tsv`. Reusing the word for services would collide.

### Finding the middle ground (the naming journey)

This design went too far in both directions before settling:

| | dropped (too Seneca) | dropped (too Erlang) | **settled (boru-native)** |
| --- | --- | --- | --- |
| the unit | `service` ✓ | `server` / behaviour | **`service`** |
| collection of units | (plugins in one instance) | supervision tree | **`server`** (a collection of services) |
| register a handler | `add` | `handle` (`handle_call`) | **`add`** (reuse the existing patrun word) |
| sync request → reply | `act` | `call` | **`call`** |
| async, no reply | *(none)* | `cast` | **`send`** (reuse the actor word) |
| handler state | closure `Store` | `[req state]->{reply newstate}` tuple | **`[req state] -> reply`**, `state` a private mutable `Store` |
| compose behaviour (cross-cutting) | `prior` chain ✓ | (no real equivalent) | **`prior`** layering + **`wrap`** middleware (§1) |
| compose units (fault) | (plugins in one instance) | supervision tree | **`server`** supervision (§3) |
| run it | `host` | `start_link` | **`serve`** (reuse the CLI verb) |
| no-match error code | `no_action` | `no_clause` | **`no_match`** |
| framework module | `boru:mesh` | `boru:otp` | **`boru:serve`** + **`boru:net`** |

The two anchors that pull this back to boru: **`add`** (services register handlers
with the same word patrun already uses) and **`send`** (async delivery is the same
word the actor layer uses for a `Pid`). `call` and `serve` are kept because they
are plain and already meaningful (`boru serve`). Erlang's `cast`/`handle`/
`handle_call`/`start_link`/`one_for_one` jargon is dropped.

## Context

The end goal is efficient, safe network servers/clients. `PROCESSES.0.md`
specifies the low-level **actor substrate** (processes, bounded mailboxes, `send`/
`receive` with pattern-matched dispatch via patrun). This document specifies the
layer most
code is actually written against — services and servers — and, crucially, shows
how the CLI's **existing** servers fold into the same model (§5).

### Why this is mostly already in boru

A service is **pattern-matching on requests + owned state**, and the matcher is
already core: patrun (`lang/go/native/native_patrun.go`). `add {pattern} value
patrun` already registers most-specific-wins rules; `find {subject} patrun`
already routes a request (`{op:"inc" n:1}` matches a handler for `{op:"inc"}`,
extra keys are payload) — the *same* matcher `receive` uses for selective receive.
"Match a message" means one thing everywhere.

This began as a **design RFC only**; that is no longer the whole truth:

> **Status (2026-08-14).** The executable slice has landed — the process
> substrate (`spawn`/`self`/`send`/`receive`/registry,
> `lang/go/native/native_process.go`, `core/go/process.go`), the phase-1
> in-process service model (`service`/`add`/`call`/`send`/`state-of`/
> `wrap`/`prior`, `lang/go/native/native_service.go`), and `boru:net`
> tiers 1–2 (raw sockets incl. TLS, codecs, `listen {tcp codec} <svc>`,
> `connect → Endpoint` where an Endpoint IS a `Service`,
> `lang/go/modules/net_socket.go` / `net_codec.go`) — with divergences
> recorded in `NETWORK-IMPLEMENTATION-PLAN.0.md` (notably: services
> serialize with an internal mutex instead of running as served
> processes; the state accessor is `state-of`; state is a flex map).
> `server`/`serve`/`pool`/supervision-as-a-value, `defer`/`reply`,
> streaming replies, and distribution remain design-only. Body text
> below predates that slice; where it says "still missing", consult the
> plan. §7.4 records three requirements adopted 2026-08 from the Elixir
> comparison, one of which (hot code loading) **revises** §7.3's stance.

### Relationship to `NETWORK-SERVERS.0.md`

The `listen`/`connect` transport surface named in §4 (and the "still-missing
TCP/socket server" gap in §10) is **specified** in `NETWORK-SERVERS.0.md`. That
document supplies the acceptor, the per-connection session loop, the **`Codec`**
extension point (bytes ⇄ messages) with built-in `http`/`json-lines`/
`length-prefixed`/`websocket` codecs, and the `Bytes`+bit-syntax binary support
— all dispatching into exactly the `service`/`add`/`call`/`send` model below. Its
high-level tier *is* this document's service model exposed on a wire; its
low-level tier is the raw-socket escape hatch beneath it. The dial side is
`NETWORK-CLIENTS.0.md`, where `connect` returns a bidirectional **`Endpoint`** —
the same `Service` type, so this document's `call`/`send`/`add`/`wrap`/`pool`
words work identically against a remote peer, and the `proxy` of §6 is a `listen`
server whose handlers `call` a `connect`ed client.

### Scope decisions (carried forward)

1. **Uniform surface, tiered cost — assume remote (§8).** Every `call`/`send`
   obeys one failure-aware contract regardless of where the target lives. A
   `Service` is a lightweight in-process value by default — `call` is zero-copy
   direct dispatch (§7.1), state a private `Store` — and is placed in a **server**
   and `serve`d to gain real processes, isolation, enforced timeouts, and
   transport. Crossing that boundary needs **no caller changes**: the contract was
   remote-shaped all along; only the cost, and the firing-rate of failures,
   change.
2. **Hybrid packaging.** The in-process surface — `service`, `add`, `call`,
   `send`, `state` — is **core** (next to the patrun words). Running, supervising,
   proxying, and transport live in modules **`boru:serve`** and **`boru:net`**.
3. **Services live in modules.** A reusable service is just a boru module that
   exports a constructor function returning a `Service`. No special plugin
   mechanism — `import` is the loader.

## 1. The core model — a service

### The `Service` value (core)

A new Ideal type **`Service`** wrapping: a **patrun** of handlers (reuse
`patrunMatcher`); the service's private **state**, a mutable `Store` (default
`{}`); and a **host** reference (`None` in-process, a `Pid` once `serve`d, §4).

`service {state} -> Service` constructs one.

### `add` (core) — register a handler

```
add {pattern} [handler] <service> -> Service
```

`handler` is **`[req:Map state] -> reply`**. The `state` argument is the
service's private mutable `Store`; the handler **mutates it in place** for state
changes and **returns the reply** value. This is the deliberate middle ground:
no functional `{reply, NewState}` tuple ceremony (the over-Erlang trap), and no
hidden closure capture (the Seneca trap) — state is an explicit, named, mutable
value, which is exactly what `Store` is for in boru. It is safe because a service
processes **one request at a time** (in-process: the caller's goroutine; served:
the process's single goroutine — the gen_server guarantee, no locks). Reuses the
existing `patrunAddHandler` registration shape.

> **An `add` pattern routes; it does not bind.** Unlike a `receive` clause
> (`PROCESSES.0.md` §3), where `name:Type` fields are typed binding slots parsed
> by `ParseFnParams`, an `add` pattern is **scalar-tag routing only** — it picks
> *which* handler runs and binds nothing. The whole request arrives as `req`, and
> destructuring its payload (`req.text`, `req.id`) is the handler's job. So
> `add {op:"create" text:String} …` does **not** bind `text`; write
> `add {op:"create"} [ [req state] => [ … req.text … ] ]`. (Binding slots are a
> `receive` feature, not an `add` feature — see `PROCESSES.0.md` §3.)

### `call` (core) — synchronous request → reply

```
call {request} <service> -> reply
```

Route `request` through the patrun (most-specific wins); on no match raise
the **`no_match`** error (`raise no_match …`) unless a catch-all `add {} […]`
exists; invoke the handler with `(request, state)` and return its reply. Reuses
`execFnDefLiteral`/`CallBoru`. `call` also obeys the uniform failure contract —
it may raise `timeout`/`down`/`overload` — which is rare and advisory in-process
and enforced once `serve`d (§8).

### `send` (core) — asynchronous, no reply

```
send {request} <service>
```

Same dispatch, reply discarded. In-process it runs synchronously and returns
`None`; once the service is `serve`d (§4) it is true fire-and-forget delivery to
the process mailbox — the *same* `send` the actor layer uses for a `Pid`, so a
service handle and a `Pid` are interchangeable at the call site.

### `state` (core)

```
state <service> -> Store          # the service's state (rarely needed directly)
```

State is normally touched only inside handlers, via the `state` parameter.

### The `from`/reply question — settled

**Handlers are `[req state] -> reply`. There is no `from` parameter.** This is the
middle ground between OTP (which threads `From` into *every* `handle_call`) and
Seneca (which hides it entirely): the overwhelmingly common handler replies
synchronously by returning a value, so threading a reply-token everywhere is pure
ceremony. The two cases that genuinely need the caller's identity are served
without burdening the common case:

1. **Deferred reply** (the handler cannot answer yet — e.g. a proxy awaiting an
   upstream): the handler returns the sentinel **`defer`**; the reply target is
   available as the request metadata field **`req.@from`** (a `Pid`-like handle),
   and the service later does `reply req.@from value`. (`reply` lives in
   `boru:serve`, used only by deferred/proxy handlers.)
2. **Streaming reply** (the handler answers with a byte/value stream — the proxy
   case, §6): the handler returns a **stream** value; `call` yields a stream the
   caller consumes (ties to `boru:stream` / `STREAM-WORDS.0.md`).

So `[req state] -> reply` is the whole contract for normal services; `@from` and
`defer` exist for the proxy/transport minority. Settled.

### Cross-cutting concerns — handler layering (`prior`) and `wrap` (core)

Patrun's most-specific-match decides *which* handler runs; a separate, orthogonal
mechanism decides how that handler is *decorated* — the cross-cutting axis (auth,
logging, caching, validation, tracing, transactions, rate-limiting). This is
**Seneca's `prior` system**, and it earns its place: supervision trees (§3)
compose *separate* services for fault isolation but do nothing for layering
behaviour on a *single* pattern. The two are orthogonal composition axes, not
substitutes — dropping one for the other (as an earlier draft did) loses real
power.

**Layering — `add` stacks, the handler receives `prior`.** Adding a handler for a
pattern that already has one does **not** overwrite it; it **pushes** onto a stack
for that exact pattern signature (raw patrun overwrites — `patrunAddHandler` in
`lang/go/native/native_patrun.go` replaces `m.side[sig]` on a duplicate pattern —
so the per-pattern handler stack must live in the **`Service` layer**, not in
patrun), newest outermost. A layering handler opts into the third, optional
argument — the continuation **`prior`** — and chooses whether and how to invoke
the handler it shadowed:

```boru
# Wrap the order-submit action with auth + audit, without touching it.
add {role:"order" op:"submit"} [ [req state prior] => [
    require-role req "clerk"                 # cross-cutting: auth (may short-circuit)
    def result ( prior req )                # invoke the shadowed handler
    audit "order-submit" req result          # cross-cutting: after
    result
] ] svc
```

`prior req` runs the next handler down the stack and returns its reply; a base
handler (stack bottom) is the plain `[req state]` form with no `prior`. A
decorator may **short-circuit** (never call `prior` — a cache hit, an auth
reject), **pre-process** (`prior modified-req`), or **post-process** (transform
`prior`'s reply). This is `around`-advice / middleware `next()` realised as an
*explicit captured continuation* — no hidden dynamic-dispatch context, just boru
function values. (It is exactly Seneca's `this.prior(msg, done)`.)

**Service middleware — `wrap` (ambient cross-cutting).** Per-pattern layering
decorates *one* action; concerns that apply to *every* request into a service
(tracing, metrics, panic recovery, a blanket auth gate) use **`wrap`**, which runs
around the whole dispatch regardless of which pattern matches:

```boru
wrap [ [req state prior] => [
    trace-start req
    def out ( prior req )                   # run the rest of the pipeline
    trace-end req
    out
] ] svc
```

`wrap` is *not* a catch-all `add {} […]`: a `{}` handler is the *least-specific*
patrun entry and fires only when nothing else matches, whereas a `wrap`
middleware wraps the chosen handler whatever it is. Multiple `wrap`s nest, newest
outermost. The full dispatch pipeline is therefore:

```
wrap layers (outer → inner)  →  patrun match  →  per-pattern prior stack  →  base handler
```

— all sharing the service's single-threaded `state`, so a caching or metrics
layer keeps its data in `state` with no locks.

**`prior` decorates one pattern signature; `wrap` decorates whatever matched.**
This distinction bites when handlers are registered at *different* patrun
specificities. A `prior` layer added at `{op:"save"}` pushes onto the stack **for
that exact signature only** — it does *not* wrap a more-specific handler such as
`{op:"save" kind:"audit"}`, because patrun routes the audit request to a different
stack entirely. So a cross-cutting concern that must apply **regardless of which
specificity matched** (a blanket timestamp on every save, an audit on every op)
must be a **`wrap`**, not a `prior` at a less-specific pattern — `wrap` runs around
the chosen handler whatever its specificity, whereas a `prior` only layers the
signature it was added to. Use `prior` to decorate *a specific action*; use `wrap`
for *ambient* concerns. (This is the trap a multi-store entity layer hits — see
`ENTITY-STORES.0.md` §5.)

**Scope: local and synchronous.** `prior`/`wrap` compose handlers *within one
service's handling of one message*; they never cross a process boundary, so they
stay zero-cost direct calls even when the service is `serve`d. Cross-cutting that
must span services (distributed tracing, a request id) is the union of a local
`wrap` that reads/writes a trace id in the request's **metadata** (carried like
`@from`; see the message-metadata gap, §10) and the transport propagating that
metadata — local layering + metadata, not a distributed interceptor.

## 2. Services live in modules

A reusable service is a boru module that **exports a constructor** returning a
`Service`. Module-private `def`s are scaffolding; nothing leaks across modules
(encapsulation). No `attach`/`Init`/plugin machinery — `import` is the loader.

```boru
# counter.boru — a module that exports a service constructor
export "New" fn [[opts:Map] [Service] [
  def svc ( service {count: (opts get start ? 0)} )

  add {op:"inc"} [ [req state] => [
      state set count (inc (state get count))   # mutate private state
      state get count                            # reply
  ] ] svc

  add {op:"get"} [ [req state] => [ state get count ] ] svc
]]
```

```boru
import "./counter.boru"
def c ( Counter.New {start: 10} )
call {op:"inc"} c            # → 11
call {op:"get"} c            # → 11
call {op:"nope"} c           # raises no_match
```

## 3. A server is a collection of services (module `boru:serve`)

```
import "boru:serve"
server [services] {restart: "isolated"} -> Server     # a supervised collection
serve <server-or-service>                            # run it
```

- **`server [svc …] {opts}`** builds a server: an ordered, named collection of
  services with a restart policy. Restart policies use plain names —
  `"isolated"` (restart just the failed service; OTP `one_for_one`), `"all"`
  (OTP `one_for_all`), `"cascade"` (OTP `rest_for_one`) — Erlang's jargon noted
  but not surfaced.
- **`serve <server>`** runs it: each service becomes a `PROCESSES.0.md` process,
  supervised per policy; `serve` blocks until shutdown (matching the existing
  supervisor). `serve <service>` is sugar for a one-service server.

This **is** today's `serve` supervisor (`cmd/go/internal/serve`), surfaced as a
value. `boru serve registry + lsp + api` is `serve (server [Registry.New{…}
Lsp.New{…} Api.New{…}])`. This is the **fault/lifecycle** composition axis —
independent services with their own state, supervised together. It is *orthogonal*
to the **behavioural** composition axis (cross-cutting layering with `prior`/`wrap`,
§1): supervision composes separate services for isolation; `prior`/`wrap` layer
behaviour within one service. A system uses both — a supervised collection of
services, each internally decorated by cross-cutting layers — plus routing/
proxying across them (§6).

## 4. Hosting & transport

- **In-process** (default): `call`/`send` dispatch directly; zero process
  machinery. Good for libraries and tests. Honours the uniform failure contract
  (§8), but the failure modes are rare and the deadline is advisory (§8.2).
- **Served**: inside a `server`, a service runs as a supervised process; its loop
  **consumes the front mailbox message and dispatches by patrun** (not selective
  receive — `PROCESSES.0.md` §1); `call` becomes `send {call:req, @from:self}` +
  await the reply **on a dedicated per-call channel** (never a mailbox scan);
  `send` is true async; the deadline, mailbox bound, and `down` detection are now
  *enforced*. Same surface and same contract, different locus — the actor
  `receive` loop and the service's handler patrun are the *same* matcher.
- **Transport** (`boru:net`): **`listen {transport} <service>`** exposes a served
  service over a wire protocol; **`connect {transport} -> Service`** returns a
  local proxy whose `call`/`send` forward to a remote service. A transport is an
  adapter translating wire frames ↔ requests: HTTP/JSON, stdio, raw TCP, and
  LSP-style framed JSON-RPC are all transports over the one service model. JSON
  envelope first (reuse `Format`/`jsonify`/`reify`); binary later (`Bytes` +
  bit-syntax, per `PROCESSES.0.md`). Transport needs the **TCP/socket server**
  boru still lacks (only HTTP-client `fetch` exists) — later phase.

Capability gating: in-process `call`/`add` need no new capability; `serve`
(processes) is gated like `spawn` (the **`process`** scope, `PROCESSES.0.md` §7);
`listen`/`connect` are gated by the **`network`** scope — restrictive profiles get
the service DX but cannot open sockets. Both scopes **already exist** in
`PERMISSIONS.10.md` (`process` and `network` are defined scopes, hard-denied by the
`sandbox`/`read-only` profiles), so gating is enforceable on day one — confirming
they exist is a prerequisite, not new permission work.

## 5. Consolidating the existing CLI servers

The CLI already has the bones of this model — a `service.Service` interface
(`Name`/`Start`/`Stop`/`Status`, optional `Pausable`/`StdioUser`/`WithMetadata`)
and a supervisor in `serve`. The consolidation is to (a) make every CLI server a
**service** over a shared **transport** + **lifecycle** core, (b) make the
supervisor the **server**, (c) lift the lifecycle controls to standard request
patterns, and (d) make services *also* writable in boru. Mapping:

| CLI server (today) | becomes | transport | notes |
| --- | --- | --- | --- |
| `repl` | service | stdio | eval handler; `pause`/`resume` = control requests |
| `registry` | service | HTTP | module-fetch handlers |
| `lsp` | service | stdio **or** TCP, JSON-RPC framing | request handlers; not pausable |
| `exec` | service | HTTP | immutable policy = service config / capability |
| `api` | service | HTTP | introspects its **server** (the supervisor) — a service that queries its own collection |
| `tui` | service | stdio | client of `api` via `connect` |
| `vault-proxy` | **proxy** (§6) | HTTP | the special one |
| `serve` supervisor | **server** | — | the collection + restart policy |

The lifecycle surface unifies onto standard requests/state:

- `Start`/`Stop` → service `serve`/shutdown (the server drives these).
- `Status()` → `call {op:"status"}` (or service `state`).
- `Pausable.Pause/Resume` → control requests `call {op:"pause"}` / `{op:"resume"}`
  that flip a service-state flag and gate handlers — the same flag the proxy
  uses for emergency revocation (§6).
- `WithMetadata.Metadata()` → `call {op:"meta"}` returning the service's map.
- `StdioUser`/transport choice → which `listen` adapter the service is exposed
  through, not an interface bolt-on.

Net effect: one lifecycle, one supervisor, one set of words — and a user can
write a registry-like or exec-like service *in boru* and `serve` it next to the
built-in ones.

## 6. Proxies — careful thought

The vault proxy (`cmd/go/internal/vault/proxy.go`) is the case that justifies a
**first-class proxy abstraction**, because a proxy is *not* an endpoint service:
it **forwards** a request to a *target* (an upstream provider), wrapping that
forward with authorization, transformation, and accounting. Generalize it as:

```
import "boru:serve"
proxy <target> {before: [..] after: [..]} -> Service
```

A `proxy` is a `Service` whose handler, for a matched request:

1. runs **`before`** interceptors — authorize + transform the request;
2. **forwards** to `target` via `call`/`send` (local service, or a remote one via
   `boru:net connect`) — the location-transparent leg;
3. runs **`after`** interceptors — transform + account for the response;
4. returns the (possibly streamed) reply.

`proxy` is really **sugar over the §1 cross-cutting primitives**: `before`/`after`
are `wrap` middleware whose innermost base handler is "`call` the `target`". So a
proxy is "a forwarding base handler + cross-cutting layers" — the same machinery
that decorates any service, pointed at a remote `target`. (Keeping `proxy` as a
named constructor is still worthwhile: forwarding + streaming + capability checks
is a recurring shape worth blessing.)

The vault proxy maps cleanly: `before` = capability-token auth + policy checks +
credential injection; `target` = the upstream provider URL (a `connect`ed
service); `after` = quota/cost accounting + audit. The careful points the model
must honour, each drawn from the real proxy:

- **Streaming replies, never buffered.** The proxy streams the upstream body
  straight back (`proxy.go` copies the response; secrets/bodies are never
  materialised in logs). The service model therefore must let `call` return a
  **stream** (§1 settlement, ties to `boru:stream`) — a reply is not always a
  single immutable value. This is the main reason streaming is a first-class
  reply shape.
- **Secret state is private and never travels in messages.** The proxy holds the
  unsealed session/credentials as its **private `Store` state**; replies crossing
  a process/transport boundary stay immutable and secret-free. The boundary
  immutability check (`PROCESSES.0.md` `not_sendable`) applies to forwarded
  requests/replies, *not* to the proxy's internal secret state.
- **Capability enforcement is the boru permission model, unified.** The proxy's
  capability-token check (hashed token → alias binding → revoked/expired/method/
  budget/host policy, per `proxy_security_test.go`) is the *same* capability/
  scope machinery as `PERMISSIONS.10.md`, not a bespoke mechanism. A `before`
  interceptor consults capabilities; denial → an error reply. This is what makes
  the proxy a **trust boundary** rather than a dumb forwarder.
- **Stateful accounting.** Unlike endpoint services, a proxy tracks per-capability
  call counts and cost budgets across requests — held in service `Store` state,
  recorded in `after` *after* the response is authorized (so accounting never
  blocks the stream), exactly as today.
- **Emergency revocation = the unified pause control.** The proxy's "return 503,
  brake now" (`ProxyService.Pause`) is the §5 `call {op:"pause"}` control request —
  open connections drain, new requests are rejected. Same mechanism as every
  pausable service.
- **Protocol/trust versioning.** The wire envelope carries a protocol version
  (`X-Boru-Vault-Protocol`); the transport adapter (`boru:net`) enforces fail-loud
  on mismatch. Versioning belongs to the transport layer, shared by all
  services, not re-implemented per proxy.

The existing `Proxy` (stateless per-request core) + `ProxyService` (lifecycle/
listener wrapper) split maps directly: the **interceptors + `target`** are the
proxy core, and the **service lifecycle + `listen` transport** are the wrapper —
so consolidation removes the bespoke wrapper, not the credential logic.

## 7. Lessons from BEAM — weaknesses and adopted strengths

BEAM is the inspiration, but its model has well-known weaknesses. Below are the
generally-agreed ones, the ecosystem's responses, and the stance boru takes — two
of them (zero-copy messaging and backpressure) are load-bearing enough to change
the design. §7.4 then records the reverse lesson: three BEAM/Elixir *strengths*
adopted as requirements.

### 7.1 Zero-copy messaging — boru structurally avoids BEAM's biggest tax

BEAM copies *every* message between processes. That cost is **not intrinsic to
the actor model**: it exists because each BEAM process has its **own heap and own
GC**, so a message must be copied to live in the receiver's heap (the same
per-process-heap design that gives BEAM its low pause times). boru has no
per-process heaps — services and processes are **goroutines on one Go heap with
one GC** — and boru values are **immutability-first** (`eng/go/clone.go`: scalars,
plain `List`, plain `Map`, type/function values are never mutated in place).

**So, inside a server, an immutable message is passed by reference — zero copy,
zero race** — both for an in-process `call` and for delivery to a `serve`d
service in another goroutine. The mailbox send supplies the happens-before edge
(Go memory model) so the receiver observes a fully-published value, and
immutability guarantees no concurrent writer. This is exactly the property
`PROCESSES.0.md` already states: boru gets BEAM's **isolation** (a handler can
never observe a caller mutating a message mid-flight) **without** BEAM's **copy
cost** — sidestepping the per-message-copy weakness entirely. (It also gets
Erlang's refc-binary optimization for free: a future immutable `Bytes` is shared
by reference like any other immutable value.)

The one case needing care is the **mutable subset** — `Object`, `Array`, `Store`.
Sharing one across goroutines would reintroduce precisely the data race the actor
model exists to prevent. The rule (already proposed as `PROCESSES.0.md`'s
`not_sendable` check) is **refuse, not copy**: a `call`/`send` crossing a
process/transport boundary must carry only immutable values; a mutable value at
the boundary is an error. So boru *never copies messages* — it shares immutable
ones and forbids mutable ones. A service's own state `Store` is unaffected because
it never crosses a boundary: it is owned and mutated by that one service's single
goroutine. In-process `call` is cheaper still — a direct function call in the
caller's goroutine, nothing sent or copied — so the immutability/`not_sendable`
discipline only becomes load-bearing once a service is `serve`d as a process.

### 7.2 Backpressure — `send` must not be unbounded

BEAM's worst production failure mode is the **unbounded mailbox**: async `send`
with no flow control lets a fast producer grow a slow consumer's mailbox until
OOM, and a large mailbox also makes selective `receive` O(n). The ecosystem bolts
backpressure on afterwards (synchronous `call`, GenStage/Flow/Broadway, sbroker,
jobs). boru should take a stance up front:

- **`call` is the backpressured path** — synchronous request/reply naturally
  rate-limits the caller to the service's throughput. Prefer it.
- **`send` (async) gets a bounded mailbox** — when a `serve`d service's mailbox is
  full, `send` either blocks (applying backpressure) or fails fast with an
  overload error, configurable per service (`server [..] {mailbox: N}`). Never
  silently unbounded.
- A demand-driven streaming path (GenStage-style) is deferred to the `boru:stream`
  integration but should reuse the same bounded-mailbox mechanism.

### 7.3 The rest — mapped to boru stances

| BEAM weakness | ecosystem response | boru stance |
| --- | --- | --- |
| Numeric/CPU throughput | BeamAsm JIT, NIFs/Rustler, Nx | Go-hosted (compiled, real arrays/maps); hot paths are Go natives, not a VM/NIF boundary — the gap mostly doesn't arise |
| Dynamic typing / untyped protocols | Dialyzer, Gleam, Elixir set-theoretic types, Akka Typed | boru has a static-leaning type system + `Behavior`; **a service may carry a schema on its accepted patterns**, validated at the `connect` boundary (Open Q #4) — ahead of Erlang |
| Distribution: full mesh, head-of-line blocking | Partisan, `erpc`, message fragmentation | distribution is "later"; when built, follow Partisan (overlays, parallel connections), not disterl's full mesh |
| Distribution: all-or-nothing trust | (largely unsolved in core) | **the proxy + capability model is the answer** — every cross-boundary `call` carries capability scopes; no ambient "connected = trusted" |
| Location transparency leaks | Orleans virtual actors; "make failure explicit" | **one uniform, failure-aware surface — assume remote** (§8): the same `call` contract local and remote, exposing failure everywhere rather than hiding it (the Waldo-consistent direction), with defaults keeping the common case terse |
| Single `gen_server` bottleneck | pooling, sharding, scalable registries | a server is a collection of services; shard by key into many services; a router/proxy pin (Open Q #2) fronts them |
| NIF safety / scheduler blocking | dirty schedulers, Rustler | natives run on Go's preemptively-scheduled goroutines; a slow native can't stall a cooperative scheduler the way a BEAM NIF can |
| Mnesia limits | khepri, external DBs | no built-in DB ambition — persistence is an external service |
| Hot code loading complexity | rolling/blue-green deploys | **stance revised 2026-08** — adopted as a requirement for the plugin system (§7.4.1); mechanism report + design in `HOT-CODE-LOADING.0.md`. boru dodges most of BEAM's complexity: the interpreter is late-bound and compiled units self-invalidate on rebinding, so reload is a *protocol* (a control message per owning loop), not a global module-table swap — a self-invalidated unit is then re-run on the interpreter, which is an open defect owed a fix (§7.4.1), not a property this stance rests on |
| Testing concurrency | QuickCheck, PropEr, Concuerror | property-based testing is already idiomatic here (`lang/spec`); systematic concurrency testing to follow |

### 7.4 The strengths worth adopting — three requirements (added 2026-08)

The Elixir ecosystem's pitch for this architecture is not only what BEAM
avoids but what falls out of it for free. Three of those payoffs are adopted
as requirements of this design. The observation that motivates adopting
rather than deferring them: *they can all be built from scratch in other
languages, but on BEAM the building blocks are part of the runtime* — and
the audit below shows boru is in the same position: each requirement
composes from machinery the runtime already ships (patrun, forks, services,
capabilities, codecs), not from new substrate.

#### 7.4.1 Hot-code swapping → a plugin system that reloads live without dropping state

**Requirement.** Hot-code swapping must support an extensible **plugin
system** in the style of the Pi coding agent's extensions: a plugin is
loaded into a running host, edited on disk, and **reloaded live without
dropping state** — the host keeps running, the plugin's accumulated state
survives the swap.

**What boru already has** (full report: `HOT-CODE-LOADING.0.md`): word
references resolve through the def table at call time (module-level names
are deliberately not closure-captured); re-`def` shadows; file modules are
**never cached**, so re-`import` already re-reads, re-runs, and rebinds a
module's namespace; compiled units whose dependencies are rebound
self-invalidate (`NotifyNameRebound`) and are re-run on the interpreter
instead — silently at `do`/REPL/built binaries. That routing is
scaffolding containing an open defect, not a fallback this design may lean
on: a unit the compiler cannot carry across a rebind is a compilation
failure owed a fix, and the silence indicts it — a failure that hides
itself is worse, not better. Old fn values pin their old module
sub-registry until GC — BEAM's current/old code generations without the
purge. On this document's own model the split is exactly right: a
service's **state** (a flex map) is untouched by handler registration, and
`add` on an existing pattern *stacks* — so "swap the handlers, keep the
state" is nearly native. The service is the plugin unit: **a plugin is a
module exporting a service constructor** (§2 verbatim — no separate plugin
format), reloaded by re-importing the module and re-registering handlers
on the living service.

**What it adds to this design.** A `reload` word (re-import with
keep-old-generation-on-failure); reload **propagation as a control
message** — a `server` broadcasts `{op:"reload"}` and each service
re-imports inside its own mutex-held dispatch turn, each process inside
its own `receive` loop (never by cross-goroutine mutation; running forks
hold independent def-table snapshots by design); a **state-migration
hook** (`Plugin.migrate old-state -> state`, the `code_change` analog —
precedented by aless's watch-reload re-anchor, which preserves view state
across a document rebuild and keeps the old generation when the new one
fails to parse); handler-stack replacement (today `add` can only push);
and a **persistent `boru:vm` sub-engine** (`Vm.open`) so an *untrusted*
plugin runs behind an attenuated policy and reload = rebuild the
sub-engine, re-injecting host-held state. Gaps carried in §10; phasing in
§11.

#### 7.4.2 Client-server architecture as a byproduct of the actor model

**Requirement.** The OpenCode observation: once the core is an actor
system, splitting a tool into a **server** (the engine and its state) and
thin **clients** (UIs attaching over a wire) is not an architecture
project — it is a byproduct. And the same actor substrate supplies both
**I/O concurrency** (an actor parked per connection) and **CPU
concurrency** (goroutines are M:N scheduled across cores, §7.3) with one
model.

**Audit: this already fell out.** The byproduct effect is not
speculative — it happened in this codebase, twice, before it was a
requirement:

- **`Tui.serve` + `boru attach`** (`lang/go/modules/tui_serve.go`,
  `cmd/go/internal/attach/`): a boru TUI app `serve`d over TCP runs the
  *same driver loop* as local `Tui.run` against the remote half of the
  renderer seam; widget trees stream down as json-lines, input events
  come back up, and the attach client is deliberately dumb — no engine
  client-side. Multiple viewers can attach; with `reattach: true` the app
  **keeps running headless** when the last viewer detaches and a later
  viewer resumes with the current frame — session persistence, the
  OpenCode property, from the actor loop's indifference to where its
  renderer lives.
- **`Endpoint` = `Service`** (`NETWORK-CLIENTS.0.md`; implemented in
  `net_codec.go`): `connect` returns a value of the *same* `Service` type
  this document defines, so `call`/`send`/`wrap` work identically against
  a remote peer — client-server is the in-process model exposed on a
  wire, which is precisely §4's transport tier doing its job. `boru:repl`
  (a socket REPL service) and the `api`+`ctl` control plane (§5) are the
  same shape again.

**What it adds to this design.** Mostly confirmation: §4 (transport) and
§5 (CLI-server consolidation) *are* the client-server story and stay the
priority. Two sharpenings: the §5 consolidation should treat
`Tui.serve`/`attach` as its reference client (protocol conventions —
handshake, auth token, reattach — to be shared, not re-invented per
service), and the uniform contract (§8) is what keeps a client honest
about the server being remote — no local-mode special cases.

#### 7.4.3 Built-in distribution → isolating the brains from the hands

**Requirement.** Distribution must make it natural to isolate the
**brains** (the coordinating stateful session — e.g. an agentic session
holding model context and conversation state) from the **hands** (sandbox
plus tools — executors that run effects). Concretely: run the session on
your machine while it coordinates executors inside Docker containers or
on remote nodes; or one session coordinating **multiple** nodes — the
Livebook topology (one notebook driving several attached runtimes).

**What boru already has.** The pieces are the ones this document already
specifies, plus two that shipped: the **uniform assume-remote contract**
(§8) means the brains' `call {op:"run" …} executor` is written once and
works whether the executor is a local service, a Docker-confined process,
or a remote node; the **proxy + capability model** (§6, §7.3) is the
trust boundary — each hand holds only the scopes its policy grants
(`sandbox`/`compute` profiles hard-deny `process`/`network`/fs), so a
compromised hand cannot become a brain, avoiding disterl's all-or-nothing
trust; **`boru exec`** (`cmd/go/internal/exec/`) is a working hand today —
a stateless HTTP execute service with policy flags, one fresh engine per
request; and **`boru:vm`** is the same attenuation applied in-process.
What Livebook gets from BEAM distribution and boru still lacks is the
node layer itself: membership, health, a cross-node registry, and the
location-transparent `call` over it (§9.1 "Horizontal", §10).

**What it adds to this design.** A named target profile for the
distribution phase — the **brains/hands split** becomes the acceptance
test for "Later: distribution" (§11), replacing the abstract
"location-transparent call across nodes": (1) a **hands node** is `boru
serve` running an exec-shaped service under a restrictive profile,
reachable via `connect`, its capabilities declared to the brains; (2) the
**brains** is an ordinary boru program (or served session) holding
`Endpoint`s to many hands, fanning `call`s out under §8 timeouts/retries
and §9.2 pooling (a pool over hands *is* the multi-node Livebook
topology); (3) the **proxy** (§6) fronts hands that face untrusted input,
so credentials and quota live brains-side. Distribution's gap list (§10)
gains no new mechanism from this — it gains its *shape* and its ordering
rationale: membership + health first, because a brains/hands system is
usable long before full mesh transparency.

## 8. Delivery semantics — one uniform messaging surface

§7.1–7.2 set the *stances*; this section specifies them, under one governing
principle:

> **Assume every send is remote.** The messaging *surface* is uniform — every
> `call` may `timeout`/`down`/`overload`, every `send` may `overload`/`down` —
> whether the target is co-located, in another process, or on another node. You
> never ask "is this local?" at a call site: you write to the failure-aware
> contract once, and a service can move in-process → process → remote with **no
> caller changes**.

This is the correct reading of Waldo et al.'s *A Note on Distributed Computing*
(1994): the sin is making **remote look local** — hiding latency and partial
failure under an infallible local-looking API; the cure is making **local look
remote** — exposing the failure surface everywhere. It is also what BEAM already
does: a `gen_server:call` carries a timeout even locally, and any pid (local or
remote) can hand you a `DOWN`.

Crucially, **a uniform *contract* is not a uniform *cost*.** The implementation
stays tiered: a co-located immutable send is still zero-copy direct dispatch
(§7.1) — no serialization, no scheduler hop — it merely *honours* the same
contract, under which the failure modes are simply rare. And **defaults erase the
ceremony** (§8.2), so the everyday form stays `call {req} svc` and is correct
everywhere. (Syntax illustrative; error forms follow `lang/spec/error.tsv` —
`raise <code> "msg"` and `do […] error […]`.)

### 8.1 Bounded mailboxes & `send`

**Mailbox.** Every `serve`d service has a bounded mailbox. Capacity and overflow
policy are set where the service is placed in a server:

```
server [ svc ] {mailbox: 1024  overflow: "block"}
```

- `mailbox: N` — capacity in messages (default `1024`). `mailbox: "unbounded"` is
  allowed but must be written explicitly — you opt *in* to BEAM's footgun, you
  never get it by accident.
- `overflow:` — what a delivery does when the mailbox is full:
  - **`"block"`** (default) — the sender blocks until space frees. Backpressure
    propagates to the producer, pacing it to the service's drain rate.
  - **`"fail"`** — the delivery raises **`overload`** immediately; the caller sheds
    or reroutes.
  - **`"drop"`** — the new message is discarded and the delivery returns; lossy,
    for best-effort/telemetry traffic (a `"drop_oldest"` variant evicts the head).

**Per-service override.** `mailbox`/`overflow` on the `server` set the **default**
for its services, but a single supervised app routinely mixes policies — a lossy
telemetry sink wants `"drop"` while a worker wants `"block"`. Rather than force each
policy into its own `serve ( server [..] {..} )`, a service carries its own opts
map where it is placed in the server, overriding the server default:

```boru
server [ worker  (metrics {mailbox: 4096  overflow: "drop"}) ]
  {mailbox: 1024  overflow: "block"}        # server default; metrics overrides it
```

Precedence is **per-service > server default > built-in default** (`1024` /
`"block"`). This keeps one server/one supervision tree even when its services need
different backpressure, instead of fragmenting the tree just to vary a mailbox.

**`send` (async)** resolves against the policy: enqueue and return `None` if there
is room; otherwise block / raise `overload` / drop. An optional bound on blocking
— `send {req} svc {within: (TimeUtil.seconds 1)}` — blocks at most that long under
`"block"`, then raises `overload`. `send` to a dead service raises `down`; `send`
never raises `timeout` (no reply is awaited).

**`call` is self-limiting.** A `call` also enqueues (request + reply handle), so it
is bounded by the same mailbox — but because the caller blocks for the reply, at
most `N` calls are in flight before further callers block to enqueue. So `call`
gives backpressure *for free*; the explicit policy mainly governs `send`.

**Observability.** `call {op:"status"} svc` reports `{mailbox: {depth capacity
high_water dropped}}`, so overload is measurable, not silent. (The bound governs
the inbound mailbox; the per-process save-queue used for selective `receive` in
`PROCESSES.0.md` is separate.)

**Why.** Erlang's unbounded mailbox is its classic overload/OOM failure mode and
makes selective `receive` O(n). A bound turns overload into one of three *chosen*
behaviours — pace, shed, or drop — instead of an unbounded-queue death spiral.

```boru
import "boru:serve"
import "boru:time-util"

# A slow worker behind a bounded, backpressured mailbox.
def worker ( service {} )
add {op:"job"} [ [req state] => [ heavy-work req.work  None ] ] worker

# Default "block" paces producers: the 1025th in-flight send parks the
# producer until the worker drains one — no unbounded growth.
serve ( server [ worker ] {mailbox: 1024  overflow: "block"} )

# A telemetry sink that must never block the hot path: drop on overflow.
def metrics ( service {} )
add {op:"metric"} [ [req state] => [ record req  None ] ] metrics
serve ( server [ metrics ] {mailbox: 4096  overflow: "drop"} )

# A front door that sheds load rather than queueing without limit:
serve ( server [ worker ] {mailbox: 256  overflow: "fail"} )
do [ send {op:"job"  work: payload} worker ]
error [ case [
    [get code eq overload/q] [ "503 busy, retry later" ]   # caller shed load
    [ raise ] ] ]                                          # re-raise anything else
```

### 8.2 The uniform failure contract for `call`

Under the principle above, **every `call` carries the same failure contract**,
regardless of where the target lives: it returns the reply, propagates an
*application* error the handler `raise`d, or fails with a *delivery* error. There
is no separate "local call" semantics to learn — co-located calls simply fire
delivery errors rarely (or never). The contract is defined three ways.

**(1) A deadline is always in effect.**

```
call {req} svc {timeout: (TimeUtil.seconds 5)}
```

If omitted, a profile default applies (proposed `5s`); `timeout: "infinity"` is
permitted but must be written explicitly — you can never *accidentally* hang
forever, the way an Erlang `receive` with no `after` can. One honest asymmetry:
when the target is an **un-`serve`d inline value**, the call is a direct dispatch
in the caller's own goroutine, so the deadline is **advisory** — it cannot
preempt what it cannot interrupt (the limitation any direct function call has).
Once the service is **`serve`d**, the deadline is enforced by the caller's timer
against the callee's separate goroutine, exactly as Erlang enforces a
`gen_server:call` timeout. Either way the *contract* — "this may raise `timeout`"
— is identical, so calling code never changes.

**(2) A closed, named set of delivery errors,** distinct from application errors,
each catchable via `do … error …` and dispatchable on `code`:

| code | meaning | did the request run? |
| --- | --- | --- |
| `timeout` | no reply within the deadline | **unknown** — may have run |
| `down` | target crashed / supervisor gave up / never existed — a dead handle (ties to `PROCESSES.0.md` monitors) | **unknown** — possibly partial |
| `overload` | mailbox full under `"fail"` / timed-out `"block"` (§8.1) | **no** — never enqueued |
| `transport` | remote only: connection refused/reset/closed, DNS/TLS — carries the cause | pre-send **no** / post-send **unknown** |

An **application error** the handler raised (e.g. `raise bad_input "…"`) propagates
back **unchanged** — same `code`, `message`, and payload, serialized across
transport — so the caller distinguishes *"the call could not complete"* (the
delivery codes above) from *"the service ran and said no"* (the app error).

**(3) Defaults, not types, carry the uniformity.** Because every `call` is
*uniformly* fallible, the type system does **not** split handles into
local-infallible vs remote-fallible — there is one `Service` type and one
contract, which is what makes a service relocatable with no caller edits. The
clutter is absorbed by **defaults** instead: an unhandled delivery error simply
propagates like any raised error (it is *not* a forced checked-exception), a
`call` with no `{timeout}` takes the profile default, and nothing retries unless
asked. So the everyday call stays `call {req} svc`, with the failure modes
*available* to handle exactly where you choose to. (The checker may *optionally*
warn when a delivery error is never handled anywhere up the stack — Open Q #7 —
but that is lint, not a local/remote type distinction.)

**Retries & idempotency.** Because `timeout`/`down` leave the outcome *unknown*, the
runtime never silently retries them. A caller that knows a request is idempotent
opts in:

```
call {req} svc {timeout: (TimeUtil.seconds 2)  retries: 3  idempotent: true}
```

- `overload` (and pre-send `transport`) → never executed → retried automatically,
  even without `idempotent`.
- `timeout` / `down` / post-send `transport` → unknown → retried **only** when
  `idempotent: true`; otherwise the error surfaces for the caller to reconcile.

This honesty about "unknown outcome" is the whole point: the model refuses to
pretend a timed-out remote mutation definitely did — or definitely did not —
happen.

```boru
import "boru:serve"
import "boru:net"
import "boru:time-util"

# Reach a remote service. Its `call` obeys the same contract as a local one —
# only the failure modes are now common, so we handle them at this call site.
def billing ( connect {http: "https://billing.internal"} )

# Read path — idempotent, so auto-retry is safe; degrade on failure.
def total (
  do [ call {op:"get-total"  user: uid} billing
         {timeout: (TimeUtil.seconds 2)  retries: 3  idempotent: true} ]
  error [ case [
      [get code eq timeout/q]   [ -1 ]                  # unknown → show stale
      [get code eq down/q]      [ -1 ]
      [get code eq transport/q] [ raise unavailable "billing offline" ]
      [ raise ] ] ] )                                   # app errors propagate

# Write path — NOT idempotent: never blind-retry; reconcile a timeout.
do [ call {op:"charge"  user: uid  cents: 500} billing
       {timeout: (TimeUtil.seconds 5)} ]
error [ case [
    [get code eq timeout/q]  [ enqueue-reconciliation uid ]  # unknown → verify
    [get code eq down/q]     [ enqueue-reconciliation uid ]
    [get code eq overload/q] [ retry-later uid ]             # definitely not charged
    [ raise ] ] ]                                           # app errors propagate
```

## 9. Scalability & load balancing

### 9.1 Scalability envelope

A design-stage estimate from the substrate (Go goroutines/channels/GC,
`ForkConcurrent` per process, immutable zero-copy messages (§7.1), patrun
dispatch, the bytecode VM) — projections, not measurements.

**Vertical (single node, ~32–64 cores):**

| Dimension | Estimate | Set by |
| --- | --- | --- |
| Concurrent processes/services | **10⁴–10⁵; ~10⁶ only with lean forks** | per-process `ForkConcurrent` weight, *not* goroutine count |
| Per-process memory | ~5–50 KB | goroutine stack + registry fork + mailbox |
| Per-service throughput | **10⁴–10⁶ msg/s** | single-goroutine serialization × interpreted-handler cost |
| Aggregate node throughput | **10⁶–10⁷ msg/s** (light handlers) | cores × per-service, via Go's M:N scheduler |
| Local `call` latency | **sub-µs–µs** | patrun match + handler; zero-copy message |
| Served `call` latency | **low µs** | + channel send + goroutine wakeup + reply round-trip |
| Connections (once TCP lands) | **C10K trivial, C100K with lean forks** | goroutine + fork + FD per connection |
| GC tail latency | **sub-ms typical, few-ms p99.9 at large heaps** | one shared Go GC |

Limiters, in order: **(1) `ForkConcurrent` weight per process** — the biggest
divergence from BEAM (~2.6 KB/process → 10⁶–10⁷ there); boru forks copy the
*mutable* eval state (sharing read-only infra), so density is **~10× behind BEAM
but tunable** via lean/copy-on-write forks. **(2) The single shared GC** — zero-copy
messaging is a win over BEAM's per-message copy, but BEAM's per-process GC gives
tighter, isolated tail latency at large heaps. **(3) Per-service single-goroutine
serialization** (the gen_server bottleneck, §7.3) → load balancing, §9.2.
**(4) Interpreted-handler floor** (bytecode VM mitigates).

**Horizontal (multi-node):**
- **Today: single-node runtime** (one heap, one GC); scale-out only via network
  services, itself gated on the **missing TCP server** (only HTTP-client `fetch`).
- **Strongest property — relocation:** the uniform "assume remote" surface (§8)
  lets a service move in-process → process → node with **no caller changes**.
- **Secure routing** via proxy + capability (§6) — avoids disterl's all-or-nothing
  trust.
- **Ceiling when built:** follow Partisan (overlays, parallel connections) →
  **hundreds–thousands of nodes** vs disterl's ~100.
- **Missing:** node membership, a cross-node registry, partition/rebalance, and
  the transport itself.

**Benchmark first:** per-process fork weight is the key risk *and* the
highest-leverage lever — measure it before anything else.

### 9.2 Load balancing

Two limiters above — per-service serialization (vertical) and node distribution
(horizontal) — are the same need: **spread load across interchangeable
instances.** The model meets it with one idea at two scopes — a **balanced
service** that fronts a pool of workers and routes each request by a policy.

**A pool is a service (`pool`, `boru:serve`).**

```
import "boru:serve"
pool [worker-spec] {size: N  strategy: "p2c"} -> Service
```

A `pool` is a `Service` whose handler picks a worker from its set and forwards the
`call`/`send`. Its workers are children under the server's supervisor (a crashed
worker is replaced; the pool routes around `down` ones, §8.2). Being just a
service, it composes with everything — `wrap` it with cross-cutting layers (§1),
`serve` it, expose it via `listen`, even pool a set of pools.

**Strategies.**
- `"round-robin"` / `"random"` — stateless, even spread.
- `"least-loaded"` — route to the **shallowest mailbox** (depth is observable,
  §8.1) — backpressure-aware.
- `"p2c"` (power-of-two-choices) — sample two at random, route to the less loaded;
  near-optimal with negligible coordination — a good default.
- `"hash" <key>` — consistent-hash by a request key for **state affinity** (a key
  always lands on the same worker — stateful sharding); rebalances on membership
  change.
- `"weighted"` — for heterogeneous worker capacity.

**Backpressure *is* the load signal.** The bounded-mailbox `overload` error (§8.1)
doubles as the balancer's reroute trigger: a `"fail"`-policy worker returning
`overload` tells the pool to try another or shed; `"least-loaded"`/`"p2c"` use live
mailbox depth to avoid a saturated worker in the first place. Load balancing and
backpressure are one mechanism, not two.

**Same idea, two scopes:**
- **Intra-node** — a `pool` of local worker services across cores defeats the
  per-service single-goroutine ceiling (§9.1 #3): stateless → `"p2c"`, stateful →
  `"hash"`. Works on the phase-2 process layer.
- **Inter-node** — a **proxy whose `target` is a *set* of remote endpoints**
  (§6) *is* a load balancer / API gateway, inheriting the capability model; the
  same strategies apply, and `down`/`transport` errors (§8.2) drive failover.
  Because the surface is uniform, the same `pool`/proxy code balances local
  workers or remote nodes identically.

**Still needed** (with the horizontal gaps): cross-node **membership + health** so
an inter-node pool/proxy knows its target set and liveness, and **rebalancing**
for `"hash"` pools on node join/leave (consistent hashing keeps reshuffling
minimal). Intra-node pools land with phase 2; inter-node balancing with transport
+ distribution.

## 10. Gap analysis — what boru still lacks

- **`Service` type + `add`/`call`/`send`/`state`** and the `[req state] -> reply`
  handler contract (the patrun router is reused; invocation reuses
  `execFnDefLiteral`).
- **Layering (`prior`) + `wrap` middleware** (§1) — the per-pattern handler stack
  (patrun must keep a stack, not overwrite a same-signature rule) and the `prior`
  continuation passed to layering handlers; `wrap` for ambient cross-cutting.
- **Message metadata** — a small per-request envelope (a `@from` reply handle, a
  trace/request id) carried through `call`/`send` and across transport, so
  cross-cutting `wrap` layers can propagate context between services.
- **`server`/`serve`/restart** — depend on `PROCESSES.0.md` (processes, links,
  exit signals).
- **`proxy` + streaming replies** — needs `call` to return a stream (`boru:stream`)
  and the `before`/`after`/`target` plumbing.
- **Transport (`listen`/`connect`)** — needs the **TCP/socket server** boru lacks,
  plus the wire envelope. JSON covered; binary needs `Bytes` + bit-syntax.
- **Capability integration for proxies** — wiring the proxy's auth to
  `PERMISSIONS.10.md` scopes.
- **Refactor of the Go servers** onto the shared service/transport/lifecycle core
  (§5) — mechanical but broad.
- **Bounded mailboxes + backpressure** for `send` (§8.1) — per-service capacity +
  `"block"`/`"fail"`/`"drop"` overflow; depends on the `PROCESSES.0.md` mailbox.
- **Uniform-failure `call`** (§8.2) — mandatory deadline, the `timeout`/`down`/
  `overload`/`transport` delivery-error set, one fallible contract for local and
  remote, and idempotent-only retries; depends on monitors (`PROCESSES.0.md`) and
  transport.
- **`pool` / load balancing** (§9.2) — a worker-pool service with routing
  strategies (`"p2c"`/`"least-loaded"`/`"hash"`/…) using mailbox depth + `overload`
  as the load signal; intra-node first, inter-node via a multi-target `proxy`.
- **Cross-node membership, health & rebalancing** (§9) — for inter-node pools and
  proxies to know their target set, liveness, and to reshuffle `"hash"` routing on
  node join/leave (consistent hashing); lands with distribution.
- **Lean / copy-on-write registry forks** (§9.1) — the #1 scalability lever;
  reduces per-process memory toward BEAM-class process density.
- **Hot code reload + plugin system** (§7.4.1, `HOT-CODE-LOADING.0.md`) — a
  `reload` word (re-import, keep-old-on-failure, generation counter); reload
  propagation as a supervisor control message; the `Plugin.migrate`
  state-migration hook; handler-stack replacement on services (`add` can only
  push today); a persistent attenuated sub-engine (`Vm.open`/`load`/`call`/
  `close`) for untrusted plugins; generation observability via
  `{op:"status"}`.
- **Brains/hands distribution profile** (§7.4.3) — a blessed pairing of a
  coordinating session with capability-attenuated executors: a hands-node
  serve profile (exec-shaped service under `sandbox`/`compute`), Endpoint
  fan-out + §9.2 pooling on the brains side, proxy-fronted credentials.
  Mechanism-wise this is the existing distribution gap (membership, health,
  cross-node registry) — reordered so membership + health land first.

## 11. Phased roadmap

- **Phase 1: in-process service.** `service`, `add`, `call`, `send`, `state`,
  `no_match`, plus `prior` layering + `wrap` middleware (§1) — all core. Pure
  request→handler over reused patrun, state in a `Store`. No processes. **This is
  the recommended first implementation slice** of the whole actors/services effort:
  it depends only on patrun + `execFnDefLiteral` + `Store` and needs **none** of
  the `PROCESSES.0.md` substrate (no `spawn`, mailbox, `Pid`, or `context.Context`),
  so it is the cheapest, highest-signal way to validate the `[req state] -> reply`
  handler contract and the `prior`/`wrap` stack before any concurrency lands.
- **Phase 2: server + supervision.** `boru:serve` `server`/`serve`/restart on the
  `PROCESSES.0.md` process layer; services-in-modules; `pause`/`status`/`meta`
  control requests; bounded mailboxes + backpressure for `send` (§8.1); the
  served `call` deadline + delivery-error set (§8.2); **intra-node `pool`** load
  balancing (§9.2).
- **Phase 2b: hot reload + plugins** (§7.4.1). `reload`, the supervisor
  reload broadcast, `Plugin.migrate`, handler-stack replacement, `Vm.open`.
  Placed with phase 2 because the propagation rule rides the supervisor's
  control requests; the in-process `reload` word itself has no phase-2
  dependency and may land with phase 1.
- **Phase 3: transport + proxy.** `boru:net` `listen`/`connect` (HTTP/stdio/TCP/
  JSON-RPC); remote `call` failure modes + retries under the uniform contract (§8.2);
  `proxy` with streaming replies and capability-checked interceptors;
  **inter-node load balancing** via a multi-target proxy (§9.2); begin refactoring
  the CLI servers (§5) onto the model — vault-proxy as the proving ground, with
  `Tui.serve`/`attach` as the reference client protocol (§7.4.2). → the
  network-server goal.
- **Later: distribution — shaped as brains/hands** (§7.4.3). Cross-node
  membership/health first, then the hands-node serve profile + Endpoint
  fan-out (a pool over hands, §9.2), then location-transparent `call`/`send`
  and `"hash"`-pool rebalancing. Acceptance test: a session on one machine
  coordinating executors in containers and on remote nodes under attenuated
  capabilities.

## 12. Worked example

```boru
import "boru:serve"
import "boru:net"

# A server = a collection of services, supervised; restart each in isolation.
def app ( server [
    ( Registry.New {dir: "./mods"} )      # a boru-written service
    ( Counter.New  {start: 0} )
  ] {restart: "isolated"} )

# A proxy in front of an upstream, with auth + accounting interceptors.
def gw ( proxy ( connect {http: "https://api.example.com"} ) {
    before: [ AuthCheck  InjectCredential ]
    after:  [ RecordUsage ]
  } )

serve ( server [ app gw ] )               # run everything (blocks)
```

## 13. Open questions

1. **`state` mutation vs purity** — the settled model mutates a `Store`; should a
   pure variant (`add` with `[req state] -> {reply state}`) also be offered for
   handlers that prefer functional state? (Leaning: mutable `Store` only, to keep
   one contract.)
2. **`proxy` matching** — does a proxy forward *all* requests to `target`, or only
   those matching a pin pattern (so one server can host several proxies)? (Leaning
   pin pattern, like the vault alias prefix.)
3. **Restart policy granularity** — server-wide policy only, or per-service
   overrides and restart-intensity limits (OTP `max_restarts`)? (Leaning
   server-wide first.)
4. **`connect` reply typing** — should a remote `connect`ed service carry a schema
   so `call` replies are typed/validated at the boundary? (Leaning optional,
   later.)
5. **Go-server refactor order** — refactor the simplest endpoint (registry) first
   to prove the shared core, then the proxy, then stdio (repl/lsp)? (Leaning yes.)
6. **Default deadline & mailbox values** (§8) — are `timeout: 5s`, `mailbox: 1024`,
   `overflow: "block"` the right defaults, and should they be per-profile
   (`PERMISSIONS.10.md`) rather than global constants? (Leaning per-profile
   defaults with per-service overrides.)
7. **Unhandled-delivery-error strictness** (§8.2) — since all calls are uniformly
   fallible, an unhandled delivery error just propagates. Should the checker
   *optionally* warn when one is never handled anywhere up the stack, and should an
   inline hot path be allowed to opt out of the deadline entirely (`timeout:
   "none"`) for zero overhead? (Leaning: opt-in lint + an explicit inline escape.)
8. **Layer ordering** (§1) — `prior`/`wrap` layers nest by add-order (newest
   outermost), which is Seneca's model but makes cross-cutting order depend on load
   order (a known Seneca footgun). Offer an explicit priority/phase (e.g. `add …
   {phase: "auth"}`) so ordering is declared, not incidental? (Leaning: add-order
   default, optional declared priority for `wrap`.)
9. **Default `pool` strategy & elasticity** (§9.2) — is `"p2c"` the right default,
   and should a `pool` auto-scale its worker `size` under sustained `overload`, or
   stay fixed-size with shedding? (Leaning: `"p2c"` default, fixed-size first,
   elasticity later with metrics.)
10. **Reload granularity & failure ownership** (§7.4.1) — is `reload`
    per-module only (per-service rebuilds being supervisor convention), and
    does a failed `Plugin.migrate` keep the old generation running (the aless
    rule) or crash into the restart policy? (Leaning: per-module word;
    keep-old + report — supervision stays about faults, not upgrades. Full
    question set in `HOT-CODE-LOADING.0.md` §6.)
11. **Plugin trust tiers** (§7.4.1) — same-engine module (cheap, shared heap)
    vs per-plugin `Vm.open` sub-engine (isolated, rebuildable): host's choice
    per plugin, or a policy-driven default? (Leaning: host's choice, with the
    restrictive profiles forcing the sub-engine tier.)
12. **The hands-node profile** (§7.4.3) — is a "hand" just `boru serve
    exec`-under-`sandbox` reached via `connect`, or a dedicated agent service
    with a declared tool manifest (capabilities advertised to the brains at
    handshake)? (Leaning: start as the former; the manifest is an `{op:"meta"}`
    convention, not new machinery.)
