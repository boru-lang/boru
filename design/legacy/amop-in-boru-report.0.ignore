# Ambient-Oriented Programming (AmOP) in boru: Implementation Review

## Scope
This report reviews the Ambient-Oriented Programming material at https://soft.vub.ac.be/amop/ and evaluates how its core ideas could be implemented in boru, with emphasis on:

1. **Syntax design**
2. **Implementation architecture**
3. **Feasibility and risk**
4. **Developer experience (DX)**

## Source Snapshot (AmOP / AmbientTalk)
Based on the AmOP pages, AmbientTalk emphasizes:

- Actor-style, event-driven concurrency
- Asynchronous message passing as the default communication model
- Peer discovery and dynamic participation in ad hoc networks
- Built-in handling for disconnection/reconnection behavior
- Time-aware primitives (timeouts, leasing, subscriptions)
- Prototype-oriented object model with reflective extensibility
- JVM interoperability in the reference implementation

Representative references:

- Home/overview: https://soft.vub.ac.be/amop/
- Intro: https://soft.vub.ac.be/amop/at/introduction
- Key expressions: https://soft.vub.ac.be/amop/at/byexample
- Language reference entry point: https://soft.vub.ac.be/amop/at/reference/reference

---

## boru Baseline (Current Repository)
boru in this repository is currently strongest as a **concatenative data/query language** with:

- Stack + forward arg collection evaluation
- Module/import/export mechanics
- Typed records and function signatures
- Collections, transforms, query/table operations
- Timers and cancellation (`timeout`, `interval`, `cancel`)
- Parallel branch evaluation (`await` modes such as `all`, `full`, `first`, `any`)
- CLI/REPL and module packaging/registry workflows

It is **not yet an ambient/distributed runtime model** by default.

---

## 1) Syntax: How AmOP Concepts Map to boru

## 1.1 What maps naturally
Several AmOP concepts can be represented with existing boru syntax without language changes:

- **Service descriptors / tags** → typed records and maps
- **Discovery subscriptions** → data structures + callback quotations
- **Timeout and lease metadata** → existing temporal words + records
- **Protocol messages** → explicit maps (`{kind:..., payload:...}`)

Example boru style for message/event envelopes:

```boru
def Msg (refine Record [kind:String from:String to:String payload:Any ts:Number])

def mk-msg fn [[kind:String from:String to:String payload:Any] [Map] [
  make Msg {kind:kind from:from to:to payload:payload ts:(now)}
]]
```

## 1.2 Where syntax extension is advisable
AmbientTalk has very compact notation for distributed interaction (`obj<-msg()`, `when: ... discovered:` forms). boru can emulate this, but readability will degrade unless we add DSL words.

### Proposed minimal DSL layer (library-first)
- `spawn-actor [behavior] -> ActorRef`
- `send [actor-ref msg]`
- `request [actor-ref msg timeout-ms] -> Future`
- `discover [service-tag handler] -> Subscription`
- `on-disconnect [actor-ref handler] -> Subscription`
- `on-reconnect [actor-ref handler] -> Subscription`
- `lease [actor-ref ttl-ms] -> LeaseRef`

This should begin as a standard module (`boru:ambient`) rather than parser-level syntax. If adoption is strong, optional sugar can be considered later.

## 1.3 Syntax recommendation
**Recommendation:** Do not change the core parser first. Build AmOP semantics as **composable words + typed records + module conventions**. Only introduce syntax sugar after validating usage pain in real examples.

---

## 2) Implementation Architecture in boru

## 2.1 Runtime model options

### Option A — Pure library/runtime on current engine (recommended first)
Implement actors as managed state machines in a dedicated runtime module:

- Actor mailbox = queue in runtime map
- Event loop = scheduled ticks (`interval`) that drain mailboxes
- Futures/promises = IDs mapped to resolver state
- Discovery bus = registry of `{tag -> actor refs}` with subscription callbacks
- Failure state = explicit heartbeat/TTL checks

**Pros:** Fastest path, no parser changes, backward-compatible.
**Cons:** Semantics are cooperative/emulated, not deeply integrated in evaluator.

### Option B — Engine-level ambient primitives
Add native engine primitives for actor isolation, remote references, and discovery events.

**Pros:** Strong semantics/perf, clearer guarantees.
**Cons:** Larger maintenance footprint and deeper VM coupling.

### Option C — Hybrid
Begin with Option A, then migrate hot paths/critical semantics to engine natives.

**Best long-term balance.**

## 2.2 Suggested internal modules

- `boru:ambient.actor` — actor refs, mailbox operations, spawn/send
- `boru:ambient.future` — request/reply correlation, timeout behavior
- `boru:ambient.discovery` — publish/discover/subscribe lifecycle
- `boru:ambient.failure` — heartbeat, disconnect/reconnect events
- `boru:ambient.lease` — lease lifecycle and expiration callbacks
- `boru:ambient.net` — transport adapters (in-memory, HTTP/WebSocket, later P2P)

## 2.3 Transport strategy
AmOP assumes volatility and intermittent links. In boru, this should be abstracted behind pluggable transports:

1. **In-memory transport** for deterministic tests
2. **WebSocket transport** for practical peer messaging
3. **Optional local discovery bridge** (mDNS or registry relay)

---

## 3) Feasibility Assessment

## 3.1 Feasible now (high confidence)
- Actor-like event handlers
- Async request workflows (via futures + `await` + timeout words)
- Subscription/callback APIs
- Lease and timeout behavior
- Modular packaging of ambient libraries

## 3.2 Feasible with moderate effort
- Discovery semantics mirroring AmbientTalk examples
- Reconnection/disconnection lifecycle hooks
- Best-effort message buffering and replay
- Developer-facing API surface similar to AmbientTalk “key expressions”

## 3.3 Hard / higher-risk areas
- Hard actor isolation guarantees equivalent to VM-level actors
- Transparent remote references with full location transparency
- Robust mobile ad hoc networking semantics in hostile real networks
- Security model and capability discipline for distributed objects

## 3.4 Overall feasibility verdict
**Feasible as a boru subsystem** if we position it as “Ambient-inspired distributed coordination runtime” rather than a full AmbientTalk-equivalent language/runtime in phase 1.

---

## 4) Developer Experience (DX)

## 4.1 Expected DX wins
- boru’s concise, composable word style is good for pipeline-oriented event processing.
- Record types and map literals make protocol payloads explicit.
- Existing module/import tooling supports packaging ambient utilities.

## 4.2 Expected DX friction
- Concatenative style can become opaque in deeply asynchronous flows.
- Without dedicated ambient words, code may become verbose/noisy.
- Debugging distributed event flows needs stronger tracing tools than local stack traces.

## 4.3 DX improvements to prioritize

1. **Structured tracing**
   - Correlation IDs for `send/request/reply`
   - Timeline logs (queued/sent/retried/expired)

2. **Ambient REPL helpers**
   - `actors`, `mailboxes`, `subs`, `leases` inspection commands

3. **Golden scenario templates**
   - Chat/presence example
   - Discovery + heartbeat example
   - Partition/reconnect simulation harness

4. **Error model conventions**
   - Standard error records: `offline`, `timeout`, `expired-lease`, `unknown-service`

---

## 5) Concrete Gap Analysis: AmbientTalk Feature vs boru State

| AmbientTalk / AmOP Feature | boru Today | Gap | Suggested Path |
|---|---|---|---|
| Async messaging as first-class | Partial (`await`, timers, function composition) | No dedicated actor/message abstraction | Add `boru:ambient.actor` words |
| Service discovery primitives | No native equivalent | Major | `boru:ambient.discovery` module + subscription API |
| Disconnect/reconnect semantics | No native equivalent | Major | Heartbeat + link-state monitor module |
| Leasing semantics | Partial (timeouts/cancel primitives exist) | Medium | Lease object + auto-renew conventions |
| Resilient remote refs | No | Major | Introduce explicit `ActorRef`/`RemoteRef` data model |
| Actor isolation guarantee | Weak/emulated | Major | Start emulated, then add engine-native isolation if needed |
| Reflection/metaobject extensibility | Different model | Medium | Provide meta hooks at module/runtime layer |

---

## 6) Phased Delivery Plan

## Phase 0 — Design/Spec (1-2 weeks)
- Define ambient runtime vocabulary and typed records.
- Specify failure semantics and retry/timeout contracts.

## Phase 1 — In-process ambient runtime (2-4 weeks)
- Implement actor mailbox runtime + futures + timeout helpers.
- Implement discovery registry + subscriptions in memory.
- Add deterministic test harness for partition/reconnect simulation.

## Phase 2 — Networked transport (3-6 weeks)
- WebSocket adapter + reconnection policy.
- Message buffering policy and idempotency hints.

## Phase 3 — DX hardening (2-4 weeks)
- Tracing, inspection commands, scenario templates.
- Docs: “Ambient in boru in 10 steps”.

## Phase 4 — Optional engine natives (as needed)
- Promote performance-critical or semantics-critical pieces into native engine words.

---

## 7) Recommendation

If the goal is practical ambient/distributed programming in this codebase, the best strategy is:

1. **Implement a `boru:ambient` module suite first** (library-driven semantics).
2. **Adopt AmbientTalk-inspired API names** where they map cleanly, to reduce conceptual translation cost.
3. **Measure developer ergonomics with 2-3 canonical examples** before introducing parser syntax changes.
4. **Only then consider engine-native primitives** for stronger guarantees/performance.

This yields strong feasibility with controlled risk while preserving boru’s current strengths.

---

## Appendix A — Example Ambient-Style boru Sketch

```boru
# pseudocode-ish boru module sketch

import "boru:ambient.actor"
import "boru:ambient.discovery"
import "boru:ambient.future"

def im spawn-actor [
  def state {name:"alice" buddies:[]}

  on "text" [
    # handle inbound text message
  ]
]

publish "InstantMessenger" im

discover "InstantMessenger" [
  |peer|
  request peer {kind:"hello" from:"alice"} 10000
  await
]
```

The key point is that this can be prototyped with boru modules and words first, then hardened as needed.
