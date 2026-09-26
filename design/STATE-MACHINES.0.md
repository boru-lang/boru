# STATE-MACHINES

Design for general-purpose **state machines** in boru — a builtin
**`boru:state`** module (mixed Go+boru), plus the verdict on language
primitives: state machines arrive as **words and data literals, not syntax**.

> **Revised 2026-08-14** against J.V. Noble, *Finite State Machines in Forth*
> (<https://www.forth.org/literature/noble.html>), the tabular lineage's
> clearest statement. The survey behind §2 was entirely statechart-shaped;
> Noble's paper is the other tradition, and reading it against this design
> found one genuine hole and several places where the argument was weaker
> than it needed to be. The substantive change is **§3.6**, which moves input
> classification — how raw bytes or code points become alphabet members —
> inside the definition, with freeze item 13 and two new diagnostics. The
> rest is reinforcement or correction: prior-art rows 17–22 and §2.1, the
> rationale that §3.3.12 was missing (and the outright decline of computed
> transition targets), error coordinates in §3.3.4, the unblessed
> boolean-flag encoding in §6.4, a sharper argument in §8 item 1, a revised
> leaning on open question #2, and two new worked examples (§§11.2–11.3)
> taken straight from the paper.

## Context

boru has no state-machine facility today — `grep -rn "boru:state"` over the
tree finds nothing, and no prior design doc covers the topic. Yet the repo
already writes state machines by hand: `design/examples/todo/sessions.boru`
opens by calling itself "the SERVICE layer (request/reply state machine)",
every multi-state service scatters its transition logic across patrun rules
and `case` clauses, and the network roadmap (`NETWORK-SERVERS.0.md` codecs,
connection lifecycles) is heading into the most FSM-dense territory in
software. Most languages hand this problem to a library; this RFC specifies
what boru's library should be, and why the language itself needs no new
primitives to support it well.

What boru already owns that this design reuses (each claim verified against
the tree at design time — by `boru describe`, by running the program shown, or
by the cited file):

- **The actor substrate is implemented**: `spawn` / `self` / `send` /
  `receive` (with `after`) / `register` / `whereis`, bounded mailboxes,
  pattern-matched consume-front dispatch (`design/PROCESSES.0.md`; verified by
  `describe spawn` and by run). A hosted machine's event loop is these words.
  Note: `design/legacy/IMPLEMENTATION-STATUS.10.ignore` still records PROCESSES/SERVICES
  as "RFC; no code" — that is stale; `design/legacy/NETWORK-IMPLEMENTATION-PLAN.0.ignore`
  §1 is the ground truth for the shipped subset.
- **The service layer is implemented**: `service` / `add` / `call` / `send` /
  `state-of` — "a value that owns state and answers pattern-matched requests"
  (`design/SERVICES.0.md` §1; verified by run: a counter service with an
  `add {op: "inc"} (…) svc` handler answers `call {op: "inc"} svc`). A machine
  is precisely the *disciplined* version of such a service.
- **Gating exhaustiveness machinery** — the asset no mainstream FSM host
  language had: `case_not_exhaustive` is an Error-severity finding that names
  the uncovered member and gates `boru check`, `run` preflight, and
  compilation (`design/case-exhaustiveness.0.md`, landed July 2026); and
  `partial_dispatch` over a declared `tor` union is Error-severity and fires
  **even for uncalled fns** at construction (`check/go/carrier.go`).
- **Closed state sets as types**: `enum [a/q b/q c/q]` mints a closed atom
  union; `class` mints sealed nominal records; `const` mints singletons;
  `refine` mints newtypes with a nominal coverage boundary.
- **patrun** most-specific-wins routing (`lang/go/native/native_patrun.go`) —
  the dispatch engine under services and `receive`.
- **Code as data**: quoted lists, `do`, `canon` round-tripping data values,
  and the spec-map-driven-library precedent (`boru:cli`: one spec map drives
  parse, dispatch, usage, and main). A machine definition can be a plain data
  value with *named code* at the leaves — boru does not have to choose between
  a data DSL and a code API.
- **Replay for free**: `fold [step] events s0` folds a pure step over an event
  log (verified by run: `fold [bump] [{e: 1} {e: 2} {e: 3}] {n: 0}` → `{n:3}`
  with the element bound first, accumulator second); `scan` yields the audit
  trajectory.
- **TCO is a language guarantee** (`design/legacy/TCO.10.ignore`) — a process host's
  tail-recursive receive loop cannot blow the stack.
- **Timer machinery at the host layer**: `receive … after <ms>`, plus
  `boru:time-util`'s clock-capability-gated words.
- **Immutability-first values** (`eng/go/clone.go`) and immutable-only
  messages (`PROCESSES.0.md` §6) — snapshots and events are plain immutable
  maps, so they are always legal messages and always race-free. One
  cost-model correction against that RFC's zero-copy stance: the shipped
  `send` **deep-copies** even immutable payloads at the process boundary
  ("Deep-copy at the boundary so the receiver can never observe the sender
  mutating a shared container", `lang/go/native/native_process.go`), so a
  snapshot crossing a process is copied today, not shared by reference.

This is a **design RFC only — no implementation code yet**, matching how other
subsystems were designed first (`PROCESSES.0.md`, `SERVICES.0.md`,
`STREAM-WORDS.0.md`).

### Relationship to `PROCESSES.0.md` and `SERVICES.0.md`

Those RFCs (now partially shipped) supply the two **hosts** this design runs
on; neither changes. `PROCESSES.0.md` §3 already names this document's core
move: the idiomatic alternative to selective receive "is **explicit
state-based deferral** (`gen_statem`-style: stash the deferred message in
process state and handle it after the state transition)". `boru:state` is that
idea made a facility — the machine owns the deferral discipline, visibly and
boundedly, instead of every process reinventing it. A served machine is a
`Service` whose handler is the machine's step; a spawned machine is a process
whose receive loop is the machine's step. The machine itself is neither: it is
a **value**, host-independent and testable with no concurrency at all.

### Relationship to `case-exhaustiveness.0.md` and `legacy/VALUE-PATTERN-DISPATCH.0.ignore`

The exhaustiveness pass is the strongest static asset this design leans on,
and it is already load-bearing: hand-written machines that encode states as an
`enum` and dispatch with `case` get gating state×event coverage today (§6.4).
`legacy/VALUE-PATTERN-DISPATCH.0.ignore` records the precision gap that blocks the
*overload* encoding of the same idea (enum-state value-pattern overloads fail
through variable references); this RFC **endorses that fix as an independent
effort** (§8 item 3) — it is the one language-level investment adjacent to
state machines that pays for itself regardless.

### Relationship to `BORU-VIZ.0.md`, `BORU-SCRY.0.md`, and `MODULE-VIEWS.0.md`

The machine's tooling story rides the viz/scry split rather than growing its
own: `boru:state` is a *producer* of the shared view-data contract
(`BORU-VIZ.0.md` §3), never an emitter. `State.graph` returns the machine's
structure as a §3.1 graph — states as nodes, transitions as labelled edges,
the machine's semantics carried by the contract's open `kind:` vocabulary
(`'initial'`/`'final'`, `'guarded'`/`'timer'` arcs, and `'current'` when
given a snapshot) — so drawing is `Viz.graph (State.graph m) {}`, honesty
about scale (budgets/elision), escaping, and determinism are inherited, and
the graph a test asserts about (`Viz.cycles`, reachability) is
byte-for-byte the graph the docs render. `State.lint` follows scry's
introspection-as-data principle (findings are values, not prose). How a
machine becomes an interactive *widget* — the `Tui.run {service: view:}`
inspector, the `view`-word display bundle — is specified once for all
modules in `MODULE-VIEWS.0.md`, with the machine as its worked example.

### Scope decisions (agreed)

Maintainer-decided 2026-08-12:

1. **Library, not syntax.** State machines ship as the `boru:state` module —
   words and data literals only, per the corpus default ("new behaviour is a
   word or a literal, nothing else"). The primitives considered and declined
   are recorded in §8.
2. **Mixed Go+boru module from day one.** A pure-boru module measurably cannot
   deliver check-time validation (the probes in §6.2), and static checking is
   this design's headline advantage — so `State.define` is Go-native and mints
   `state_*` diagnostics from the first release.
3. **Default unhandled-event policy is `error`.** A machine that receives an
   event its current state does not handle raises, unless the state defers it
   or the machine declares `policy: ignore/q`. Protocol-safe by default; UIs
   opt out per machine.
4. **Flat v1; hierarchy is phase 2; orthogonal regions never.** v1 machines
   are flat; the spec shape reserves nested `states:` (rejected with a clear
   `state_bad_spec` "not yet" error). Phase 2 adopts the SCXML algorithm
   (inner-first handling, LCA entry/exit ordering) wholesale rather than
   minting a 21st statechart semantics. In-machine parallel regions are
   permanently declined — concurrent machines are processes/services, boru's
   native composition story.
5. **Machine ≠ host; three artifacts, never fused.** The **definition** is
   plain data (canonical, diffable, hashable); **bindings** attach code to
   names late; the **snapshot** is a small plain value the *host* owns. The
   pure step is the product; the two hosts are thin adapters over the shipped
   service/process layers.
6. **Semantics frozen here, pinned by a conformance corpus.** The §3.3 freeze
   list is the specification; `lang/spec/module-state.tsv` rows pin every item
   from the first implementation (ADR-003), because the statechart tradition's
   defining failure is semantic fragmentation (§2 row 12).
7. **Out of scope for v1**: supervision integration and cancel-on-exit child
   processes (blocked on `boru:serve` restart machinery), domain replies from
   served machines (blocked on the `@from` reply handle, `SERVICES.0.md` §1),
   persistence storage (the snapshot is *persistable*; storing it is the
   host program's business), distribution, and UML do-activities.

## 1. Motivation & end-goal

The target programming model: **define a machine as a data value, drive it
with a pure step, host it wherever it needs to live.**

```boru
def door (State.define door-spec {announce: announce/r})   # value
def r (State.step door snap {event: open/q})               # pure step
def svc (State.serve door)                                 # or: a Service
def pid (State.start door)                                 # or: a process
```

Because the definition is data, the same value answers every downstream need:
`boru check` lints it (unreachable states, unknown transition targets,
state×event holes), a diagram is generated from it, a model-based test is
derived from it, a snapshot is pinned to its hash. Because the step is pure,
a machine is tested with plain values and replayed with `fold` — no process,
no clock, no mock. Because the hosts are the existing service/process layers,
a machine gains request/reply, mailboxes, backpressure, and (in phase 2 of the
services roadmap) supervision without this module owning any concurrency.

For a *query language* this is the natural shape: the machine definition is
one more value the language can query, transform, and generate — the property
every code-first FSM library gives up, and every data-first workflow language
(Amazon States Language, BPMN) pays for with a crippled expression language.
boru's homoiconic spec maps refuse that dichotomy: the shape is data, the
leaves are named boru words.

## 2. Prior art — the hard lessons this design obeys

The design was preceded by a survey of the statechart tradition (Harel 1987,
UML, SCXML, Samek's QP), languages with first-class machine support (Erlang
`gen_fsm`→`gen_statem`, P, UnrealScript, Ragel, Esterel, Plaid/typestate),
the library ecosystem (XState, Stateless, Boost.*, Python `transitions`,
Rails AASM, Redux), durable/distributed machines (Step Functions, Temporal,
replicated state machines, event sourcing), the practical-verification
literature (model checking adoption, eqc_statem-style model-based testing),
and — added in this revision — the **tabular lineage**: transition tables as
the artifact, from the dragon book's lexer tables through Noble's Forth FSM
compiler (rows 17–22) to Ragel and `re2c`. The rows below are the lessons
that survived contact with production, mapped onto boru; each shaped a
concrete decision in §3.

| # | Hard lesson | Source | boru realization |
|---|---|---|---|
| 1 | RTC (run-to-completion) with queued events, never reentrant `fire()` — every library that shipped reentrant dispatch retrofitted a queue | Stateless, Python `transitions`, AASM | RTC is free per host: a service serializes handlers; a process loop takes one message per `receive` (§3.3.1) |
| 2 | Postpone/defer must exist — reinvented independently four times (selective receive, `gen_statem` `postpone`, P `defer`, Akka `stash`); its absence helped kill `gen_fsm` | Erlang/OTP, P, Akka | per-state `defer:` lists; deferred events live **in the snapshot**, bounded, replayed in arrival order on state change (§3.3.3) — the visible form `PROCESSES.0.md` §3 already blesses |
| 3 | Internal events drain before the next external event, or users bypass the machinery | SCXML microsteps, `gen_statem` `next_event`, P `raise` | the step drains `raise`d internal events before returning (§3.3.2) |
| 4 | State-scoped **named** timers, auto-cancelled by transition, kill the stale-timer bug class; one timer per state is below the floor (a TCP-ish machine needs retransmit + keepalive + 2MSL concurrently) | `gen_statem` timeouts; RFC 9293 | `after:` is a **map of named timers** per state; any state change cancels them; hosts implement, the pure step only ever sees timer *events* (§3.3.6) |
| 5 | Pure step in, effects out; I/O never inside the machine | sans-io (h11, quinn-proto), Elm's `update : Msg -> Model -> (Model, Cmd)`, Redux, Schneider's replicated-state-machine kernel | `State.step` is pure: `(machine, snap, event) → {snap effects status}`; hosts execute effects (§3.2) |
| 6 | Definition-as-data wins tooling; code-only definitions can't be diagrammed, diffed, or versioned — but pure-data DSLs grow ad-hoc expression languages | ASL's JSONata capitulation vs Temporal's code-first pain | spec map with **atoms naming words** at guard/action leaves; boru is the expression language (§3.1) |
| 7 | Bind guard/action *implementations* late, by name — embedding closures in the definition kills serialization and reuse | XState v5 `setup`/`provide` | `State.define {spec} {bindings}`: the spec carries names; the bindings map attaches fns (§3.1) |
| 8 | Pin persisted instances to an immutable definition version; nobody auto-migrates in-flight machines successfully | Step Functions versions, event-sourcing upcasters | snapshots carry `v:` (content hash of the canonical definition); `step` refuses a mismatch unless given `{migrate: …}` (§3.3.9) |
| 9 | Declared, typed identifiers — never stringly-typed states/events | a decade of ecosystem retrofits | states/events are atoms; an optional `events:` block declares the alphabet and payload schemas, making typos check-time errors (§3.3.11) |
| 10 | Unhandled-event policy is domain-dependent: protocols want loud, UIs want silent — make it declared, then check totality against it | P (mandatory handle/defer/ignore) vs XState (removed strict mode) | per-machine `policy:`, default `error/q` (scope decision 4); `state_unhandled` advisory computed against it (§6.3) |
| 11 | Hierarchy is the one statechart feature worth its cost (the principled home for "handle X from any state"); regions, deep history, and the pseudostate zoo are traps; entry/exit actions are the safety feature | Harel→UML fragmentation, QP, XState v5 | flat v1 + phase-2 XOR hierarchy via the SCXML algorithm; entry/exit in v1 (§3.3.4); regions never |
| 12 | Underspecification is not flexibility: 20+ mutually incompatible statechart semantics; the cure is one executable semantics + a conformance suite | von der Beeck's survey; SCXML's IRP tests | the §3.3 freeze list + `lang/spec/module-state.tsv` conformance rows from day one |
| 13 | Callbacks fused to persistence is a tarpit; the machine must never own storage | Rails AASM/state_machines | snapshots are plain values; hosts and programs decide storage; no callback ever fires from a persistence layer |
| 14 | The visualizer drives adoption; diagrams are a *generated view*, never a source | XState/Stately vs BPMN round-tripping | `State.graph` emits the shared viz data contract from the definition; `Viz.graph` draws it (`BORU-VIZ.0.md` §3.1; phase 2) |
| 15 | Model-based testing is the verification sweet spot that actually ships; full model checking demands a second artifact and drifts | Erlang QuickCheck `eqc_statem`, AWS S3 ShardStore vs SPIN/TLA+ | the definition *is* the model: `State.explore` derives generate-run-shrink testing from it (phase 3) |
| 16 | The FSM-densest domain — RFC protocol machines (TCP, TLS 1.3 App. A, QUIC, HTTP/2) — never adopted FSM libraries; its correctness story is conformance suites and monitors | RFC 9293 §3.3.2, RFC 8446, RFC 9000 | `State.conform` trace monitors (phase 3); and honesty: `boru:net` codecs get a *candidate* tool, not a mandate |
| 17 | The definition must be **isomorphic to the state table** — not merely machine-readable, but readable *as* the table, one cell per state×input — so a reader audits the design by inspection instead of by tracing control flow | Noble, *FSM in Forth* §3.3 (the stated goal: "a one-to-one relation between the definition and the state table") | the spec map's `states:`×`on:` nesting *is* that table; §3.6's `classes:` supplies the column headings and §6.3's totality matrix is literally the table's shape |
| 18 | Factor input classification into **one** pure classifier that runs once per input — ideally a lookup table — never as predicates re-tested inside each state | Noble, *FSM in Forth* §3.2 (`cat->col#`), §4 (`TAB:`/`install` decoders) | `classify:` / `classes:` in the definition (§3.6), pure, exactly one call per external event (freeze item 13) |
| 19 | Totality is the *shape* of a table, not an afterthought: an explicit "other" column plus a fully written grid leaves no hole to forget | Noble, *FSM in Forth* Fig. 1 and every table in §§3–4 (an `other?` column throughout) | `classes:`' `any/q` catch-all keeps the alphabet small and closed, so the §6.3 totality matrix is dense and `total: true` becomes cheap (open question #2) |
| 20 | Mediate a transition through a **named** thing, never a bare number — names read better, and decisively, a name can later *compute* its target, which is how a deterministic table becomes nondeterministic with no change to the machinery | Noble, *FSM in Forth* §3.3, §4 (the `>1` CONSTANT becoming the `>1?` word) | boru takes the **names** and declines the **computed target**: guarded variants (§3.3.12) buy the same compression while every `to:` stays a literal atom, so `State.graph` and `state_unknown_target` keep working (§3.4) |
| 21 | Extended state collapses a repetitive state chain — Noble's eight-state identifier table becomes two states plus a counter — but the rule then lives *outside* the table, where no table-level tool can see it | Noble, *FSM in Forth* §4 (the `(id)` machine, both ways) | `ctx` plus a **named guard** keeps the bound visible *in* the table as `when: under-limit/q`: the middle term §3.6.3 recommends and §11.3 works through |
| 22 | The table outlives the dispatch technique — the same table needed different implementations on indirect- versus direct-threaded Forths | Noble, *FSM in Forth* Appendix | the §3.3 freeze is over **semantics**, never representation: a future dense or compiled step is a representation change that needs no new spec row (§3.6.2) |

### 2.1 Two lineages, one module

Rows 1–16 come from the **statechart lineage**, which exists to structure
large reactive systems: its problems are hierarchy, deferral, entry/exit
discipline, and run-to-completion, and its inputs are already symbolic
(`open`, `lock`, a button press). Rows 17–22 come from the **tabular
lineage**, which exists to dispatch dense input cheaply and legibly: its
problems are classifying a byte or code point, filling every cell of a
state×class grid, and keeping the source text isomorphic to that grid.

The two are not rivals — they answer different questions, and this design
had only the first set of answers. That mattered because boru's own
FSM-densest code sits in the *second* lineage: the parser drives a
declarative table artifact both engines load
(`design/DECLARATIVE-GRAMMAR.0.md`), and the `boru:net` codecs ahead of us
are byte-stream machines. Row 16 recorded that protocol machines never
adopted FSM libraries and left it there; the tabular lineage supplies the
missing half of the explanation — those machines were already using a
better-fitting formalism, and a statechart library does not serve it.

The concrete consequence is §3.6: the machine's **edge** — how raw input
becomes an alphabet member — moves inside the definition, where it is
hashed, drawn, and checked with everything else. Everything else in the
tabular lineage lands as reinforcement of choices already made (§3.3.12,
§6.4, §8 item 1) or as a correction of emphasis (§3.3.4's error
coordinates, §3.6.3's tradeoff), not as new machinery.

## 3. The machine model

### 3.1 Three artifacts

**Definition** — a plain, canonical map. States and events are atoms; guards
and actions are **atoms naming words**. It contains no function values, so
`canon` round-trips it, `deq` diffs it, and a content hash of it is
well-defined. The spec shape (verified: this literal parses and dot-paths
resolve, e.g. `spec.states.closed.on.open.to` → `open`):

```boru
def door-spec {
  initial: closed/q
  # policy: error/q                 # the default; ignore/q opts out per machine
  ctx: {key: ""  locks: 0}          # extended-state defaults; init merges over these
  events: {                         # optional declared alphabet (§3.3.11)
    open: {}  close: {}  lock: {key: String}  unlock: {key: String}
    lock-timeout: {}                # timer events are ordinary alphabet members
  }
  states: {
    closed: { on: { open:  { to: open/q  act: announce/q } } }
    open:   { on: { close: { to: closed/q }
                    lock:  { to: locked/q  when: key-fits/q }
                    lock-timeout: { to: locked/q } }   # auto-relock on timer
              after: { relock: {ms: 30000  event: lock-timeout/q} } }
    locked: { entry: log-locked/q
              defer: [open/q]       # postponed while locked; replayed on exit
              on: { unlock: { to: closed/q  when: key-fits/q } } }
  }
}
```

The full set of top-level spec keys is `initial:`, `policy:`, `ctx:`,
`events:`, `states:`, `catch:` (§3.3.7), `defer-cap:` (§3.3.3), `version:`
(§3.3.9), `total:` (§6.3), and — for machines fed raw input rather than
symbolic events — exactly one of `classes:` or `classify:` (§3.6). Anything
else is `state_bad_spec`, so this list is normative and grows only by
revision of this document.

**Bindings** — a map from those names to function values, supplied to
`State.define` (the `/r` reference form, or any fn value). Late binding is
deliberate (§2 row 7): the same definition runs with production
implementations or test stubs. Bindings are **complete at define time** —
every name the spec references must resolve, and each bound value must fit
its role's shape (§6.3 `state_unknown_name`, `state_bad_binding`) — so a
missing or malformed implementation is a define-time error, never a
step-time surprise; a test that wants inert behaviour binds stubs.

**Snapshot** — a plain map owned by the caller/host, never by the machine:
`{state: closed/q  ctx: {…}  deferred: []  timers: {…}  v: "…"}`. `ctx` is the
extended state (a plain map the actions/guards read and return). Deliberately
**not** a minted type: the snapshot is the persistence artifact, and a plain
map is exactly what `canon`/`jsonify` and the immutable-message rule
(`PROCESSES.0.md` §6) already handle.

Externalizing the snapshot completes a move the tabular lineage started and
stopped halfway through. Noble's FSM begins with a global `mystate` variable,
and §3.4 of that paper relocates it *into each machine's data structure* — for
exactly the reason this design cares about, that a global "precludes such
finesse" as nesting a machine inside another or recursing. But state-per-
definition only buys one instance per definition: embed the same machine value
twice in a parent and the two children share a state cell again, and the paper
needs a `state<` accessor to reach in and initialize it. Carrying the state
entirely outside the definition is the fixed point of that argument — the
definition becomes reentrant by construction, phase 2's machine-in-machine
composition (§3.4) needs no per-instance cloning, and there is nothing to
accessor into.

`State.define` fuses definition + bindings into a **`Machine`** value (§5) and
is where all §6.3 validation fires.

### 3.2 The pure step

```
State.step <machine> <snap> {event: <atom> …} (opts) -> {snap: Map  effects: List  status: Atom}
```

The attached code splits into two kinds, so the step stays pure and replay
stays honest (the Elm/sans-io split, §2 rows 5–6):

- **Reducers.** The `act:`/`entry:`/`exit:` names bind to **pure context
  reducers**, `(event ctx) -> <new-ctx>`, which the step runs **inside
  itself**, in the §3.3.4 order, folding their results into `snap.ctx`
  before it returns. A reducer that returns a plain map returns the new
  `ctx`; the record form `{ctx: <map>  raise: [<event> …]  fx: [<desc> …]}`
  additionally queues internal events (drained in-step, §3.3.2) and
  requests host effects.
- **Effects.** The `fx:` descriptors accumulated from the step's reducers
  (plus timer bookkeeping the hosts consume) come back in `effects`, in
  request order, for the **host** to execute after it adopts the new
  snapshot (§3.3.7). Effects are the only place I/O lives; their results
  re-enter the machine as ordinary events.

So `State.step` is pure end to end — no clock, no I/O, no mailbox; time
reaches it only as timer events — and `fold`-replay is correct by
construction: context evolution happens in-step, while `fx` descriptors are
deliberately **not** re-executed on replay (a replay reconstructs state; it
must not re-fire the outside world).

`status` is one of `handled/q`, `deferred/q`, `ignored/q`, `done/q`
(§3.3.8). Guards (`when:`) are pure predicates over `(event ctx)`. Purity —
of guards and reducers alike — is a **documented contract, not an enforced
property**: boru has no effect analysis for user fns today; §10 lists it as
a later possibility. A guard or reducer that raises aborts the step. `opts`
carries `{migrate: <fn>}` for version skew (§3.3.9).

Replay falls out of purity (idiom verified by run — element first,
accumulator second, per `fold`'s binding order):

```boru
def apply-event fn [[ev:Map snap:Map] [Map] [
  def r (State.step door snap ev)
  r.snap
]]
fold [apply-event] event-log (State.init door) dot snap
```

### 3.3 The v1 semantic freeze

These thirteen items are the specification. Every one gets conformance rows in
`lang/spec/module-state.tsv`; an implementation that diverges from this list
is wrong, and a change to this list is a new revision of this document.

1. **Run-to-completion.** One external event per step; a step never observes a
   half-applied predecessor. Hosts guarantee serialization (the service
   handler mutex; the process loop's one-message-per-receive).
2. **Internal before external.** A reducer may queue internal events via its
   record form's `raise:` list (§3.2); raised events are processed within the
   same step, depth-first in raise order, before the step returns. The
   returned snapshot reflects the full microstep chain. A raise depth cap
   (default 64) turns runaway loops into an error rather than a hang.
3. **Postpone.** A state's `defer:` list names events to postpone. A deferred
   event is appended to `snap.deferred` with `status: deferred/q`. On any
   state *change*, the deferred list is replayed in original arrival order
   before any new external event, and events still deferred by the new state
   go back on the list. The list is **bounded**: default cap 256, overridable
   per machine (`defer-cap: N` in the spec), and exceeding it raises
   `state_defer_overflow` — the *behaviour* is frozen; only the default's
   size is still tunable (open question #1). An unbounded defer list would
   silently recreate the mailbox-growth footgun that `PROCESSES.0.md` §1
   rejects.
4. **Entry/exit ordering, self-transitions, and failure coordinates.** A transition runs: exit
   reducer of the source state → transition `act:` reducer → entry reducer of
   the target, each folding `ctx` in that order (§3.2). A transition **with
   `to:`** — including `to:` the current state — is *external*: exit and
   entry run. A transition **without `to:`** is *internal*: the machine stays
   put, only `act:` runs, no exit/entry, timers untouched. This single
   explicit distinction removes the classic self-transition bug class; there
   is no silent default. It is also the one item that does not survive a
   naive port *from* a tabular machine: a Forth-style table has no entry/exit
   actions, so its "stay in state 1" cell (`EMIT >1` from state 1) means
   boru's **internal** transition — `{act: …}` with no `to:` — and
   transliterating the `>1` into `to:` silently adds an exit/entry pair and
   re-arms the state's timers (§11.2 shows the correct reading).

   **Where a failure happened is part of the failure.** A guard or reducer
   that raises aborts the step atomically: the caller's snapshot is untouched,
   so there is never a half-applied state to reason about. The raised error
   nevertheless carries the transition's coordinates —
   `{from: <state>  event: <atom>  to: <state>|none  phase: guard/q|exit/q|act/q|entry/q}`.
   Noble took the other half of this trade, deliberately updating the state
   variable *before* running the cell's action so that an `ABORT` inside it
   could report where it occurred without a per-cell error handler; putting the
   coordinates in the error buys the same locatability without giving up
   atomicity, which matters more here because the snapshot is the persistence
   artifact and a half-applied one would be persisted.
5. **Initial entry.** `State.init` runs the initial state's entry reducer —
   its `ctx` result is folded into the returned snapshot, its `fx` requests
   come back in `effects` (SCXML behaviour): initial entry happens
   observably, once, at init — not on the first step.
6. **Timers.** `after:` is a per-state **map of named timers**
   (`{relock: {ms: N  event: E}}`). Semantics: entering a state arms its
   timers (absolute deadlines from arrival time); *any state change* cancels
   all timers of the exited state; a timer is **one-shot** — expiry removes
   it from `snap.timers` before its `event:` is stepped, so an expiry the
   machine ignores or handles internally cannot re-fire, and re-arming
   happens only through a subsequent entry to the state. The pure step never
   reads a clock — it receives timer events and carries arm/cancel
   bookkeeping in the snapshot's `timers:` map; **hosts** own the clock.
   Hosts must check expired absolute deadlines **before** taking the next
   mailbox message: the shipped consume-front primitive pops a queued
   message before it checks its own deadline (`core/go/process.go`
   `PopFront`), so a naive `receive … after` loop under continuous traffic
   would starve timers forever — the process host therefore delivers any
   already-expired timer event first, and only then receives. v1: the
   process host implements timers; the service host **has no timers**
   (§3.5.2, honestly: nothing runs between calls).
7. **Effects.** The host executes `effects` in list order, *after* adopting
   the new snapshot. An effect that raises: the host reifies the error
   (`do … error …`); if the spec declares a top-level `catch: <event>` —
   an ordinary alphabet member (§3.3.11) — the error is fed back as that
   event with the error map as payload; otherwise the host re-raises.
   Effects must not `call` the machine's own hosting service (self-`call`
   deadlocks — documented for services generally); `send` to it is legal.
8. **Final states.** A state with `final: true` may have `entry:` but no
   `on:`/`after:`/`defer:`. Stepping a machine whose snapshot is final
   returns `status: done/q` and the snapshot unchanged (no error — done is a
   result, not a fault). The process host's loop exits after entering a final
   state; the service host answers subsequent calls with `done/q`. Phase 2's
   composition surfaces `done` to a parent machine as an event.
9. **Version pinning.** `State.init` stamps `snap.v` with the content hash of
   the canonical definition. `State.step` with a mismatched pair raises
   `state_version_skew` unless called with `{migrate: <fn>}` in its opts,
   whose result snapshot must carry the new hash. No auto-migration, ever
   (§2 row 8). The hash covers the **definition only** — including its
   optional author-bumped `version:` field — never the bindings: function
   identity does not hash, so changing a bound implementation without any
   spec change keeps `v` stable, and authors signal behavioural breaks by
   bumping `version:`. That is the honest limit of value-hashing code
   (open question #3).
10. **Invalid input is loud.** A snapshot naming an unknown state, a
    malformed event (no `event:` atom), or an event outside a declared
    alphabet raises typed errors (`state_bad_snapshot`, `state_bad_event`).
    Under `policy: error/q` an unhandled-but-well-formed event raises
    `state_unhandled_event` naming the state and event; under `ignore/q` the
    step returns `status: ignored/q` with the snapshot unchanged.
11. **Declared alphabet (optional, recommended).** With an `events:` block,
    every event name used anywhere (`on:`, `defer:`, `after:` targets,
    `raise`, the machine's `catch:`) must be declared — checked at define
    time (§6.3) — and event payloads are validated against the per-event
    schema at step time. Without the block, the alphabet is implicit and
    payloads unvalidated; the totality advisory (§6.3) then covers only
    mentioned events.
12. **Determinism and guarded variants.** A state×event key maps to either a
    single transition map or a **list of guarded variants** tried in
    declaration order, first satisfied wins; an all-guards-false outcome
    falls through to the unhandled policy. The list is the *only* spelling
    for multiple arcs on one event: a duplicate map key collapses silently
    at the parser, last one wins (verified by run: `{a: 1 a: 2}` → `{a:2}`),
    so `State.define` never sees the duplicate and cannot diagnose it.
    `state_conflict` is therefore defined over the list form: an unguarded
    variant anywhere but last — it shadows every variant after it — is the
    Error.

    The guarded-variant list is also the *reason* this design has no
    **computed** transition target, the tabular lineage's alternative spelling.
    Noble mediates every transition through a word, and observes that the same
    machinery then serves nondeterministic machines for free: swap the `>1`
    constant for a `>1?` word that returns state 1 or state 2 depending on a
    counter, and nothing else changes. His framing of what that buys is exactly
    right and boru keeps it — there is nothing random about such a machine,
    "what permits multiple possibilities is additional information, external
    to the current state and current input," which is precisely `ctx`. What
    boru declines is the *spelling*. A computed target is opaque to everything
    downstream of the definition: `state_unknown_target` cannot check it,
    `State.graph` cannot draw the arc, `State.can` cannot report it, and the
    totality matrix cannot count it — the definition stops being isomorphic to
    the table (row 17) at the moment it matters most. A guarded variant list
    recovers the whole of that expressiveness with literal `to:` atoms: the
    branch is still data-dependent, the alternatives are merely enumerated
    instead of computed. Computed targets are therefore declined outright
    (§3.4), not deferred.
13. **Classification is pure, and happens exactly once.** When a machine
    declares `classify:` or `classes:` (§3.6), an external event supplied as
    `{raw: <value>}` is classified once, before transition selection, and the
    resulting event map is what the state table, `defer:`, and the snapshot
    all see; the raw value reaches guards and reducers only as that map's
    payload (§3.6.2). Internal `raise`d events and timer events
    are **never** classified: they are already alphabet members. A step given
    an explicit `{event: …}` skips classification entirely, so a classified
    machine stays directly testable at the symbolic level and a replay over a
    log of classified events never re-runs the classifier.

### 3.4 What v1 deliberately does not have

Hierarchy (phase 2, by the SCXML algorithm — the spec's nested-`states:`
shape is *reserved* and rejected with a clear error so flat specs stay
forward-compatible), machine-in-machine composition (phase 2: a parent
embedding a child machine value, stepping it purely, and receiving its
`done`), orthogonal regions (never), do-activities (never — long-running work
is a child process, cancel-on-exit, once supervision lands), computed
transition targets (never — §3.3.12), history states (open question #6), and
domain replies computed by actions on the service host (phase 3, blocked on
`@from`).

### 3.5 Hosts

#### 3.5.1 Standalone (no host)

`State.init` + `State.step` + your own loop or `fold`. This is the testing
and replay surface, and the sans-io shape (§2 rows 5, 16): a codec or
protocol core can be a machine stepped by whatever owns the bytes.

#### 3.5.2 Service host — `State.serve <machine> -> Service`

A `Service` whose single handler runs the step: `call {event: …} svc`
answers with the step outcome `{state: <atom>  status: <atom>}` — the
*outcome record*, not a domain reply. Two honest limitations, stated rather
than papered over:

- **Deferral over `call` returns a receipt.** A deferred event's reply is
  `{status: deferred/q …}` immediately; when the event later replays, its
  effects run during whichever call triggered the state change. No caller
  waits across the deferral — that requires the decoupled reply handle
  (`@from`, `SERVICES.0.md` §1), which is design-only today; phase 3 adopts
  it when it ships.
- **No timers.** A service runs only when called; nothing fires between
  calls. Machines that need `after:` use the process host (or arrange an
  external ticker that `send`s timer events — which is exactly what the
  process host automates).

RTC comes from the service layer's handler serialization; mailbox bounds and
backpressure come from `SERVICES.0.md` §8.1 when the service is `serve`d.

#### 3.5.3 Process host — `State.start <machine> (opts) -> Pid`

A `spawn`ed tail-recursive receive loop (TCO-guaranteed) holding the snapshot
as its loop binding: each iteration first delivers any **already-expired**
timer deadline as its event — the expiry check precedes the mailbox take,
per §3.3.6, because the consume-front primitive would otherwise starve timers
under continuous traffic — otherwise `receive`s with the nearest deadline as
its `after`, steps the machine, executes effects, and recurses on the new
snapshot; it returns when a final state is entered. Events are tagged maps (`send {event: open/q} pid`),
the established message convention. Capability-gated like any `spawn`
(`process` scope), with `clock` scope for the timer arm (§7).

**Implementation note (probed):** a pure-boru module body runs in an isolated
sub-registry with its **own lazily-created `ProcessRuntime`** — a pid spawned
inside module code is invisible to the importer's `whereis`, and `self`/
shutdown-context diverge. The `boru:state` loader must share the parent
registry's `Procs` pointer (one line in the loader, plus a pinning test), and
this constraint holds for any future module that spawns on the user's behalf.

### 3.6 Input classification — the machine's edge

Everything above assumes events *arrive* symbolic: `{event: open/q}`,
`{event: lock/q key: "k1"}`. That holds for a door, a service request, an
actor message. It is false for the domain the repo has the most of. A codec
or a scanner is fed bytes or code points, and the machine's alphabet is a
handful of **categories** — digit, letter, delimiter, everything-else — that
some layer must derive from each input.

Before this revision that layer was unowned: the honest reading of §3.5 was
"the host does it, somehow." That puts the alphabet's definition outside the
definition, with four consequences worth naming, since each is a promise this
document makes elsewhere and quietly broke here. The mapping is not covered
by `snap.v`, so two machines that classify differently can share a version
hash. It is invisible to `State.graph`, so the drawn edge labels are the
categories with nothing saying what produces them. It cannot be checked
against `events:`, so a classifier emitting an undeclared atom is a
step-time surprise in a design that promised define-time. And each host
reimplements it, so the service and process hosts of one machine can
disagree about what its input means.

The tabular lineage factors this seam *into* the machine and has done since
the 1990s: Noble's `cat->col#` maps a character to a column index, and his
`TAB:` decoder does it as a 128-entry lookup table filled by range
(`1 ' [id] ASCII Z ASCII A install`). That is the missing piece, and it
arrives here in two forms.

#### 3.6.1 `classify:` — the function form

A machine-level name in the spec, bound like any other (§3.1), whose role is
`(raw:Any) -> Atom`: **pure**, returning the CLASS the input belongs to — one
of the atoms its declaration lists in `yields:` (below; decided 2026-09-26,
NUR065). `State.step` then
accepts `{raw: <value>}` where it otherwise takes `{event: <atom>}`. Purity is
the same documented-not-enforced contract as guards and reducers (§3.2), for
the same reason — the step must stay replayable — and it is the reason
classification is not simply "host code the machine calls."

The role takes **`raw` alone, not `(raw ctx)`** — frozen, and worth stating
because the opposite is the tempting default. Three things fall out of it, all
of which the context-taking version loses. Classification stays reproducible:
the same input classifies the same way regardless of when in a run it arrives,
so a classified event log means one thing forever. `State.classify <machine>
<raw>` is well defined, needing no snapshot to answer with (a context-taking
classifier could not be exercised standalone at all — the thing §4 says is
most worth testing alone). And the two forms stay uniform: `classes:` is a
partition over input values and could not consult a context if it wanted to,
so a context-taking fn form would put the two spellings of one role on
different footings. Context-sensitivity has a home already, one step later and
visible in the table: a **guard**, which sees `(event ctx)` and can refuse a
transition the classification alone would have allowed.

The function form is the escape hatch: it handles inputs no partition
describes — classifying a parsed record by three of its fields, say. It is
one role with two spellings, so it is held to the table form's standard
(NUR065, resolved 2026-09-26 by deciding open question #7). A fn's output
domain is not statically knowable, so the declaration SAYS it:

```boru
classify: {fn: by-fields  yields: [header row trailer any/q]}
```

and every guarantee follows from that, the same way for both forms:

- **Alphabet closure at define time.** Every `yields:` atom must be a
  declared `events:` member (`state_unknown_name`), exactly as every
  `classes:` key must — so the closure guarantee (§3.3.11) holds through the
  machine's edge for both spellings. At step time a fn that returns an atom
  outside its `yields:` has classified the input to no declared class, which
  is the table form's unmatched input: `state_bad_event`, the same code.
- **One payload shape.** The fn returns the class ATOM, never an event map,
  and the machine builds the frozen event of §3.6.2 — `{event: <class>  raw:
  v}` — so a reducer reaches the input as `ev.raw` whichever form classified
  it.
- **The same diagnostics.** `state_bad_class` covers a malformed `classify:`
  declaration (no `yields:`, an empty or duplicated one, both forms declared)
  as it covers a malformed table, and `state_class_gap` (Info) flags a
  classifier with no `any/q` class, in either form: a fn with `any/q` in
  `yields:` is total by construction, like a table with the catch-all.
  Disjointness needs no check for a function — each input has one class by
  construction.

What stays different is only what MUST: the mapping inside the fn is opaque,
so a `classes:` table is still the form `State.graph` can draw per input and
the preferred one where a partition describes the inputs.

#### 3.6.2 `classes:` — the table form (preferred)

A map from **event atom** to a list of selectors, each either a literal value
or a two-element inclusive range `[lo hi]` over boru's total value order
(`TYPE-ORDERING.10.md`). Exactly one class may be the atom `any/q` — the
catch-all, Noble's `other?` column. Verified against today's parser (this
literal parses; `spec.classes.digit` → `[["0" "9"]]`,
`spec.classes get other/q` → `any`):

```boru
classes: {
  digit: [["0" "9"]]
  letter: [["A" "Z"] ["a" "z"]]
  point: ["."]
  other: any/q
}
```

This is `TAB:` and `install` — except that it is *data*, so it is hashed with
the definition, drawn with the graph, and checked with the alphabet. Four
frozen properties, each earning something:

- **Disjoint.** Non-catch-all classes must not overlap; overlap is
  `state_bad_class` (Error). Disjointness makes the classifier a genuine
  partition, so its result cannot depend on map key order — the same reason
  §3.3.12 refuses to read meaning into duplicate keys.
- **Total, or flagged.** With `any/q` the classifier is total by
  construction. Without it, an unmatched input raises `state_bad_event` at
  step time and `define` emits `state_class_gap` (Info) at check time. Every
  table in Noble's paper carries the `other?` column; this makes its absence
  visible rather than fatal, matching the "gate on wrongness, advise on
  smell" precedent.
- **Closed against the alphabet.** Every class key must be a declared
  `events:` member (`state_unknown_name`), so the column headings and the
  alphabet are one artifact rather than two that can drift. The classes are
  a *subset* of the alphabet — `raise`d internal events and `after:` timer
  events are declared members that no input classifies to — which makes the
  §6.3 totality matrix `|states| × |events|` overall and
  `|states| × |classes|` over the externally-driven part. That width is
  Noble's `WIDE`, which his tables must declare and which his row shapes
  check structurally (`4 WIDE FSM: …`); boru derives it from `events:`
  instead of restating it, so the two cannot disagree.
- **Representation-free.** `classes:` specifies a partition, not a lookup
  table. An implementation may compile it to a 256-entry byte table, a
  code-point trie, or a linear scan; the semantics are identical, so its
  conformance rows pin the *partition* and never a dispatch cost. This is
  row 22: the same table needed two implementations across threading models,
  and survived both because the table was never the technique.

The event a `classes:` partition produces is frozen too, because a table
form that inferred payloads would be exactly the ad-hoc magic row 6 warns
about: classifying `v` yields `{event: <class-atom>  raw: v}` — the class
becomes the event and the input survives verbatim under `raw:`, the same key
`State.step` accepts it under. So a state's reducers reach the original value
as `ev.raw`, and a classified event's `events:` entry declares `{raw: <type>}`
like any other payload (§11.2). The fn form produces the same event: its fn
returns the class atom and the machine builds `{event: <class> raw: v}`
(§3.6.1), so the payload shape is one rule for both spellings.

#### 3.6.3 Classification and the state-explosion tradeoff

Noble's identifier machine appears twice in the paper. First as eight states
— one per accepted character position, since a FORTRAN identifier is at most
seven characters — with a table he calls "rather repetitious." Then as two
states plus an `id.len` counter and a computed transition, which "has fewer
states and consumes less memory despite the extra definitions."

The second version is smaller, and the cost he does not price is the one this
design cares about most: **the seven-character bound has left the table.** No
diagram shows it, no lint reasons about it, `State.graph` would draw two nodes
where the rule has eight positions, and the totality matrix is total over a
model that no longer expresses the constraint. Extended state does not merely
compress a machine; it moves part of the specification somewhere tools cannot
follow.

boru's recommendation is the middle term that Noble's own machinery makes
available and his example skips: keep the counter in `ctx`, and express the
decision as a **named guard in the table** — `when: under-limit/q` — rather
than as a computed target. Two states, and the bound stays a labelled arc
that `State.graph` draws, `State.can` reports, and `state_conflict` checks
for a last-position unguarded fallback. §11.3 works both encodings through.

The general rule, and the honest limit of it: prefer states for structure the
reader should see, `ctx` for quantities, and a named guard wherever a
quantity decides structure. No check enforces this — it is a modelling
judgement, and a `ctx` field that secretly encodes a state is beyond anything
§6.3 can detect.

#### 3.6.4 What classification is not

It is not a lexer. `classes:` partitions **one** input value per step: no
lookahead, no maximal munch, no capture. A tokeniser over `boru:state`
classifies one code point per step and accumulates the lexeme in `ctx` —
which is exactly the shape of Noble's `<fp#>` loop, and of §11.2 below. boru's
own parser stays on tabnas and its declarative grammar artifact
(`design/DECLARATIVE-GRAMMAR.0.md`); nothing here proposes to replace it, and
the parity obligations on that artifact are a good reason not to try.

## 4. Language surface (the `boru:state` words)

All proposed names verified collision-free: `State` resolves to nothing today
(`describe State` → no description; `def State …` works), and every word below
is reached as `State.<word>` per the module convention, so no core word is
shadowed (ADR-001; no core-word overloads are proposed). Signatures use the
top-first convention with `describe`-ready summaries.

- **`State.define <spec:Map> <bindings:Map> -> Machine`** — validate the spec
  (all §6.3 rules), resolve guard/reducer names against `bindings` and check
  each bound value against its role's shape (§6.3 `state_bad_binding`), and
  mint the `Machine`. Raises `state_bad_spec` (and friends) on any violation;
  the same Error rules fire at **check time** when the literals are concrete
  (§6.3).
- **`State.init <machine:Machine> (opts:Map) -> Map`** — `{snap effects}`:
  the initial snapshot (version-stamped, §3.3.9; `ctx` = the spec's `ctx:`
  defaults merged under `opts.ctx`) and the initial entry's `fx` (§3.3.5).
- **`State.step <machine:Machine> <snap:Map> <event:Map> (opts:Map) -> Map`**
  — the pure step (§3.2): `{snap effects status}`. `opts` carries
  `{migrate: <fn>}`, the only path across a version skew (§3.3.9).
- **`State.classify <machine:Machine> <raw:Any> -> Map`** — run the machine's
  classifier alone (§3.6) and return the event map it yields; raises
  `state_bad_event` when the input matches no class and the machine declares
  no `any/q` catch-all, and `state_bad_spec` when the machine declares no
  classifier at all. Pure, so it needs no host. It exists because the
  classifier is the piece most worth testing in isolation — Noble checks
  `cat->col#` at the REPL character by character before wiring it into
  anything — and because that test should not require constructing a snapshot.
- **`State.can <machine:Machine> <state:Atom> -> List`** — the event atoms
  with *table-level* transitions from that state. Documented loudly as
  table-level: guards are **not** evaluated (advertising guard-aware
  availability is the lie XState removed `nextEvents` over).
- **`State.spec <machine:Machine> -> Map`** — the canonical definition back
  (bindings excluded): the diagram/diff/hash artifact.
- **`State.serve <machine:Machine> (opts:Map) -> Service`** — the service
  host (§3.5.2); `opts.ctx` seeds the instance's extended state as in `init`.
- **`State.start <machine:Machine> (opts:Map) -> Pid`** — the process host
  (§3.5.3); `opts.ctx` as in `init`, remaining opts pass through to `spawn`
  (mailbox bound etc.).

Phase 2 adds `State.graph` (the machine's structure as the shared viz data
contract, `BORU-VIZ.0.md` §3.1 — drawing is `Viz.graph (State.graph m) {}`,
never a bespoke Mermaid emitter; the machine's semantics ride the contract
as `kind:` tags, including `kind: 'current'` when given `{snap: s}` — see
`MODULE-VIEWS.0.md` §2) and `State.lint` (the §6.3 advisories as plain
data, the observability channel for dynamically built specs); phase 3 adds
`State.conform` (trace monitor; a machine's event history is emitted as viz
§3.3 trace rows, so `Viz.seq` draws it) and `State.explore` (derived
model-based testing) — named here so the v1 data model reserves nothing
that blocks them.

Every export lands with `lang/spec/module-state.tsv` rows (ADR-003), and the
conformance corpus for §3.3 lives in the same file.

## 5. New types in the lattice

One minted type: **`Machine`**, an Ideal following the `Timeout`/`Interval`
precedent (`design/IDEAL.10.md`) — **opaque** (internals reached only via
`State.spec`/`State.can`/`State.classify`), **immutable** (a legal message payload; send a
machine to a process), **comparable** by its definition content hash
(consistent with `TYPE-ORDERING.10.md`), **printable** as e.g.
`Machine<door 3 states>`. Minted per the module-type pattern so `describe`
and dispatch integrate.

The snapshot is deliberately **not** a type (§3.1): plainness *is* its
contract. If implementation experience shows snapshot-shaped Maps demand
their own carrier, that is a revision of this document, not a quiet change.

## 6. Static checking

### 6.1 What the checker already gives state machines

Verified against the shipped checker: `case` over an `enum` scrutinee
hard-gates on exhaustiveness (`case_not_exhaustive` names the uncovered
member); `partial_dispatch` fires as a gating Error when a declared `tor`
union reaches an overload set missing an alternative — **even for uncalled
fns** at construction; and calling a word with a state type it has no
signature for is a check-time `no_signature`. That is: exhaustive event
handling per state, exhaustive state handling per event, and invalid
transition calls rejected statically — for machines *hand-written* in the
right encodings (§6.4). No mainstream FSM host language ships this.

### 6.2 Why the module must be Go-native to check anything (measured)

Three probes, all against the current binary, decided scope decision 2:

- A pure-boru module fn that `raise`s on a concretely-bad spec literal
  passes `boru check` with **zero errors** (the branch-guarded raise is not
  decided even on a statically-decidable condition); the failure surfaces
  only at run time.
- An exported macro invoked across the module boundary returns its **raw
  unexpanded template** — so a pure-boru `State.compile` cannot lower a spec
  into checked `case`/class forms in the importer's scope.
- The workaround — returning quoted tokens for the user to `do` — installs
  definitions, but the checker does **not** gate generated code: a
  non-exhaustive `case` over a 3-member enum inside a `do`-run quote passes
  `boru check` clean, while the directly-written spelling is a gating Error.

So library-authored *lowering* cannot inherit the gating diagnostics, and
library-authored *validation* cannot reach check time from boru source. The
check-time story requires a Go-native `State.define` with a check-mode
mirror — the established `parse_bad_spec` pattern, severities registered in
the single table (`core/go/check_state.go`).

### 6.3 The `state_*` diagnostic family

Minted by the Go-native `define` when its spec (and bindings-key set) are
concrete literals at the call site — the common case for machine definitions.
The **Error** rows are enforced identically at runtime for dynamic specs
(same codes, same messages, per the checker's mirror discipline); the
**Info** advisories are check-time findings only — a runtime `define` can
neither gate nor warn, so dynamically built specs get them as plain data
from `State.lint` (phase 2):

| code | severity | meaning |
|---|---|---|
| `state_bad_spec` | Error | malformed shape: no `initial:`, unknown keys, nested `states:` (reserved for phase 2), `final` state with `on:` |
| `state_unknown_target` | Error | a `to:` names an undeclared state |
| `state_unknown_name` | Error | an unbound `act:`/`when:`/`entry:`/`exit:`/`classify:` name; or, with a declared alphabet, an event in `on:`/`defer:`/`after:`/`raise`/`catch:`/`classes:`/`yields:` outside `events:` |
| `state_bad_binding` | Error | a bound value is not a function or does not fit its role — guard: `(Map Map) -> Boolean`; reducer: `(Map Map) ->` a map or the `{ctx raise fx}` record (§3.2); classifier: `(Any) -> Map` (§3.6.1) |
| `state_conflict` | Error | an unguarded variant that is not last in its state×event variant list, shadowing every variant after it (§3.3.12) |
| `state_bad_class` | Error | a malformed classifier: a `classes:` range that is not `[lo hi]` with `lo` ordered before `hi`, a class overlapping another non-catch-all class, more than one `any/q`; a `classify:` with no `yields:` or an empty or duplicated one; or both `classes:` and `classify:` declared (§3.6.1, §3.6.2) |
| `state_class_gap` | Info | a classifier with no `any/q` class — a `classes:` table with no catch-all, or a `classify:` whose `yields:` lacks it — so the input domain has holes that surface only at step time as `state_bad_event` (Noble's `other?` column, §3.6.2) |
| `state_unreachable` | Info | a state with no path from `initial:` (advisory per the "gate on wrongness, advise on smell" precedent, `case_unreachable_clause`) |
| `state_unhandled` | Info | the state×alphabet totality matrix's holes, computed against the machine's declared policy; **Error** iff the spec opts in with `total: true` (open question #2) |
| `state_no_final_path` | Info | machine declares a final state some state cannot reach — the honest pseudo-liveness check; real liveness is out of scope |

This is the ranked shortlist the verification literature says actually ships
(checks over the artifact the author already wrote), and nothing more: no
session types, no model-checking second artifact.

### 6.4 The hand-written encodings stay blessed

The declarative machine is not the only citizen. Two encodings of
state-machine logic in plain boru get §6.1's checking for free, today:

- **enum + `case`**: states as `enum [a/q b/q]`, a step fn dispatching
  `case s [ … ]` per state then per event — every state×event hole is a
  gating `case_not_exhaustive`.
- **class-per-state + `tor` union**: data-carrying states as one class each,
  the state type as their union, transitions as per-state overloads —
  `partial_dispatch` reports the uncovered state, and an illegal
  transition call is a check-time `no_signature`. This is typestate-lite,
  with the caveat stated honestly: class instances sit in boru's
  shared-mutable column ("class instances are shared mutable state: writes
  are visible through every alias", REFERENCE.md §Classes), so an alias to a
  superseded state value can still invoke old-state operations — the
  encoding delivers *narrowing*, never an alias-safe typestate proof, and
  narrowing is the right ambition here (full typestate died of aliasing
  everywhere it was tried).

The module's documentation presents both as the drop-down path when the
declarative table doesn't fit (HOWTO material), and `legacy/VALUE-PATTERN-DISPATCH.0.ignore`'s
precision fixes (§8 item 3) make the second encoding robust through variables.

And a third encoding stays deliberately **unblessed**: *boolean history
flags* — a `previous-minus?`/`previous-dp?` pair of mutable fields consulted
by a nest of `if`s. This is the shape the module displaces, and the tabular
lineage supplies the sharpest argument against it on record. Noble opens by
writing exactly this version of a five-rule number validator, complete with
"history semaphores," and then observes of the result: "it is difficult to
tell by inspection that the word `LEGAL?`'s logic is actually incorrect." The
bug is in the published example, after factoring and simplification, and the
encoding is why it survives.

The lesson is not that flags are verbose. It is that **correctness stops
being checkable by reading**: a table has one variable and one cell per
state×input, so a wrong cell is a wrong cell you can point at, whereas N
booleans admit 2^N configurations with no artifact enumerating them. That is
also why the checker cannot help here and never will — `case_not_exhaustive`
needs a closed type to be exhaustive over, and a flag pile declares none.
This is the concrete content of §Context's observation that today's services
"scatter their transition logic across patrun rules and `case` clauses": the
cost is not the scattering, it is that no reviewer can confirm the result.

## 7. Safety & capability integration

Per `design/PERMISSIONS.10.md` scopes, and composing with `boru:vm`
attenuation:

- **`State.define` / `init` / `step` / `can` / `spec` / `classify`**: pure — no
  capability, usable in `sandbox`/`compute` profiles, spec-rowable under the
  frozen spec clock (no `hermeticExempt` growth). Classification is inside
  this set by design (§3.6.1): a classifier that reached for I/O would put
  I/O in the step, which §3.2 forbids, so the pure-role contract is what keeps
  the machine's new edge on the correct side of this line.
- **`State.start`**: gated exactly like `spawn` (`process` scope), plus
  `clock` scope when the machine declares `after:` timers (the host arms real
  deadlines). Restrictive profiles get the whole pure surface and `State.serve`,
  but cannot spawn machine processes — the same posture as the actor layer.
- **`State.serve`**: no new capability in-process (it is `service` + `add`);
  placing the service in a served `server` inherits that layer's gating when
  it lands.

Effects run with the host's ordinary permissions — the machine adds no
ambient authority; an action can do exactly what the program hosting the
machine could do directly.

## 8. Language primitives — considered and declined

The corpus default stands: "new behaviour is a word or a literal, nothing
else" (`legacy/effect-oriented-programming-in-boru-report.0.ignore`,
`legacy/fsharp-units-in-boru-report.0.ignore`), and `legacy/amop-in-boru-report.0.ignore` §2.1
already recommended library-first for exactly this shape ("Do not change the
core parser first"). Candidates, with verdicts:

1. **Dedicated machine syntax** (`machine Door … on open -> open …`) —
   **declined.** The strongest external evidence in the whole survey: language-
   level FSM syntax is a graveyard (UnrealScript `state` blocks dropped in
   UE4; Akka's FSM DSL deprecated for plain `become`; Plaid dead; SDL niche)
   while library embeddings survive decades. Internally, the parser is the
   costliest surface in the repo (100%-gated leaf module, Go+TS parity, eng
   lowering, checker, compiler, formatter), and the spec map already parses
   as idiomatic forward-form boru (verified, §3.1) — the sugar would buy
   punctuation. Revisit only on demonstrated post-v1 usage pain, per the amop
   report's condition. *(Maintainer-decided 2026-08-12: words only.)*
   The tabular lineage adds a datum that sharpens this considerably. Forth is
   the language in which minting syntax is *cheapest* — the compiler is
   user-extensible by premise, and Noble's `FSM:` **is** a compiler extension,
   a defining word. He had every facility to mint machine syntax, and the
   paper is four successive refinements (§§3.1→3.4) of *not* doing so: each
   step factors more out of the code and into a **data table**, and the stated
   goal throughout is that the definition read one-to-one as that table. If
   the language whose whole premise is "extend the compiler" converges on
   library-plus-data, the case for boru — whose parser is its most expensive
   surface — to mint syntax instead is very weak. Note also what the paper
   *does* consider worth a language-level construct: not the machine, but the
   `CASE:`/`;CASE` and `TAB:` defining words underneath it, i.e. dispatch and
   lookup primitives. boru has those already (`case`, maps, `patrun`).
2. **Mailbox-level postpone / selective receive** — **declined.**
   `PROCESSES.0.md` deliberately demoted selective receive; snapshot-held
   deferral is the visible, bounded version of the same power, and Erlang's
   operational history (O(n) mailbox scans, silent accumulation) argues the
   RFC's side.
3. **Value-pattern-dispatch precision fixes** — **endorsed, as an independent
   effort.** The two partition bugs and the variable-reference gap recorded
   in `legacy/VALUE-PATTERN-DISPATCH.0.ignore` are not state-machine work, but fixing
   them completes §6.4's second encoding (overload-level exhaustiveness
   through variables). This document adds a consumer to that design's
   motivation; it does not depend on it.
4. **Checker flow-narrowing** ("this binding is `Idle` in this branch, so
   `stop` is invalid here") — **deferred.** The guard-narrowing chassis makes
   it plausible and value semantics makes it sound-ish without linearity, but
   it should be driven by evidence from `state_*` diagnostics in use, not
   designed speculatively.
5. **Generalizing `receive`-style binding slots** to `add`/machine clauses
   (healing the route-only vs route+bind asymmetry) — **out of scope here**;
   it belongs to the processes/services design line. The asymmetry itself is
   now recorded in the register as **NUR064** (Pending), so it cannot be
   silently baselined while that line decides.

## 9. Gap analysis

**This RFC adds:** the machine model (three artifacts, pure step, the §3.3
freeze), the `boru:state` surface (§4), the `Machine` type (§5), the
`state_*` check family (§6.3), two hosts over shipped layers (§3.5), the
in-definition classification edge (§3.6), and the conformance-corpus
obligation (scope decision 6).

**Preconditions (phase 0):**

- **The VM double-run bug.** Reproduced at design time: a two-`call` counter
  service whose handler body is `state set count (state.count add 1)` yields
  `{count:4}` compiled vs `{count:2}` under `--no-compile` — the poly-dispatch
  VM fallback ("result count 1 differs from the recorded claim 0; deferring to
  the interpreter") re-runs the mutating body. It is shape-dependent (the
  forward-form spelling `(add (state get count/q) 1)` does not trigger the
  fallback), so spec rows cannot rely on avoidance: `State.serve` rows driving
  mutating handlers would fail the repo's own compiled-vs-interpreted gates.
  Fix (or prove `State.serve`'s generated handler avoids the fallback path)
  before phase 1 rows land.
- **`ProcessRuntime` sharing in the module loader** (§3.5.3 note).

**Still missing after v1 (and where it lands):** hierarchy and definition-
level composition (phase 2); `State.graph` over the viz contract (phase 2);
conform/explore (phase 3);
deferred domain replies over `call` — blocked on `@from` (phase 3, with
`boru:serve`); cancel-on-exit child work — blocked on links/monitors/
supervision (phase 3+); persistence storage and distribution — not this
module's business (the snapshot is the portable artifact; `SERVICES.0.md`'s
transport story is the distribution story).

## 10. Phased roadmap

- **Phase 0 — preconditions.** The VM fallback double-run fix; loader `Procs`
  sharing; (independent, already recorded elsewhere) the
  `legacy/VALUE-PATTERN-DISPATCH.0.ignore` partition fixes.
- **Phase 1 — the core module.** `define`/`init`/`step`/`can`/`spec`/
  `classify`/`serve`/`start`; the §3.3 semantics complete (RTC, internal
  drain, bounded postpone, entry/exit + explicit self-transition kinds, named
  state timers on the process host, effects contract with `catch:`, final
  states, version pinning, alphabet + payload validation, determinism rules,
  classify-once); the classification edge of §3.6 — the `classes:` partition
  with its disjointness and closure checks first, since it is the form that
  earns the diagnostics, with `classify:` alongside it as the escape hatch
  (open question #7); the `state_*` Errors and advisories of §6.3; the
  conformance TSV corpus pinning every freeze item, positive and negative
  rows paired.
- **Phase 2 — hierarchy, composition, views.** Nested `states:` per the
  SCXML algorithm (inner-first, LCA entry/exit, declaration-order
  tie-breaks) with its own conformance rows; parent machines embedding child
  machine values (pure child-step, child `done` surfacing as a parent
  event); `State.graph` over the viz contract (with hierarchy mapping onto
  the contract's `group:` nesting, so `Viz.collapse` yields the
  parent-state view for free); shallow history only if it falls out of the
  algorithm (open question #6).
- **Phase 3 — tooling and services integration.** `State.conform` trace
  monitors; `State.explore` derived model-based testing (quota-capped);
  `@from`-based deferred domain replies on the service host; supervised
  machines and cancel-on-exit invoke when `boru:serve` restart machinery
  lands.
- **Later, evidence-driven.** Checker flow-narrowing over machine states;
  effect/purity analysis for guards; persistence conventions
  (snapshot-store recipes, not machinery).

## 11. Worked examples

Three: the door, for the statechart features (hosts, deferral, timers); then
Noble's two machines, for the tabular ones (classification, total tables, and
the state-versus-`ctx` tradeoff). In all three the syntax is illustrative and
settles during implementation, but every spec literal, dot-path, reducer,
guard, and `fold`/`split` idiom below was **run against today's parser and
binary**; only the `State.*` and `Viz.*` words are prospective.

### 11.1 The door — hosts, deferral, timers

```boru
import "boru:state"
import "boru:viz"

# ---- bindings: ordinary words; the spec refers to them by name ----------
def announce  fn [[ev:Map ctx:Map] [Map] [ ctx ]]            # act: no ctx change
def log-locked fn [[ev:Map ctx:Map] [Map] [ set locks/q (add 1 ctx.locks) ctx ]]
def key-fits  fn [[ev:Map ctx:Map] [Boolean] [ eq ev.key ctx.key ]]   # pure guard

# ---- the machine: definition (data) + bindings (code), fused ------------
def door (State.define {
  initial: closed/q                       # policy: error/q is the default
  ctx: {key: "k1"  locks: 0}              # extended-state defaults; see init below
  events: { open: {}  close: {}  lock: {key: String}
            unlock: {key: String}  lock-timeout: {} }
  states: {
    closed: { on: { open: { to: open/q  act: announce/q } } }
    open:   { on: { close: { to: closed/q }
                    lock:  { to: locked/q  when: key-fits/q }
                    lock-timeout: { to: locked/q } }   # auto-relock on timer
              after: { relock: {ms: 30000  event: lock-timeout/q} } }
    locked: { entry: log-locked/q
              defer: [open/q]             # an open while locked waits, bounded
              on: { unlock: { to: closed/q  when: key-fits/q }
                    lock:   { } } }       # internal: no to:, no exit/entry
  }
} {announce: announce/r  log-locked: log-locked/r  key-fits: key-fits/r})

# ---- standalone: pure stepping and replay -------------------------------
def r0 (State.init door {ctx: {key: "front-door"}})   # opts.ctx merges over
                                          # the spec defaults; {snap effects}
def r1 (State.step door r0.snap {event: open/q})
r1.snap.state                             # => open
r1.status                                 # => handled

def apply-event fn [[ev:Map snap:Map] [Map] [
  def r (State.step door snap ev)
  r.snap
]]
fold [apply-event] event-log r0.snap      # replay an audit log to its end state

# ---- service host: request/reply outcomes -------------------------------
def svc (State.serve door)
call {event: open/q} svc                  # => {state: open  status: handled}
call {event: lock/q  key: "k1"} svc       # guard consulted; outcome record back

# ---- process host: timers live here -------------------------------------
def pid (State.start door)                # process- and clock-gated
send {event: open/q} pid                  # 30s later, relock delivers
                                          # lock-timeout to the machine

# ---- views: the same value, drawn (phase 2; BORU-VIZ.0.md) ---------------
Viz.graph (State.graph door {snap: r1.snap}) {title: "door"}
#   ~> Mermaid source: states as nodes, transitions as labelled edges, the
#   current state tagged kind:'current' — viz styles it without ever
#   learning what a snapshot is (MODULE-VIEWS.0.md §2).
```

Note the conventions in play: the definition is a plain map whose guard and
reducer leaves are **atoms naming words**, bound late by `State.define`, so
the definition alone is canonical, diffable, and hashable; the reducers run
*inside* the pure step (§3.2), which is why the `fold` replay reconstructs
`ctx` faithfully without re-firing any effect; events are **tagged maps**
(`{event: open/q …}`), the same convention the actor layer uses; the
snapshot is caller-owned plain data — nothing above holds hidden
state except the hosts, which hold exactly the snapshot; the guard is a pure
predicate over `(event ctx)`; the `relock` timer armed in `open` is cancelled
by any exit from `open` and otherwise delivers `lock-timeout` as an ordinary
event (auto-relock); `lock` in `locked` shows an *internal* transition (no
`to:`, no exit/entry — locking a locked door consumes the event and moves
nothing); and the deferred `open` while locked replays, in order, the moment
`unlock` succeeds.

### 11.2 Noble's fixed-point number — classification and a total table

The paper's running example, transcribed. His Fig. 1 is reproduced in the
comment so the isomorphism (row 17) can be checked by eye:

```boru
import "boru:state"
import "boru:string-util"

#   input:  |  other?   |   num?    |  minus?   |    dp?    |
#   ( 0 )     DROP >0     EMIT >1     EMIT >1     EMIT >2
#   ( 1 )     DROP >1     EMIT >1     DROP >1     EMIT >2
#   ( 2 )     DROP >2     EMIT >2     DROP >2     DROP >2

def keep fn [[ev:Map ctx:Map] [Map] [ set text/q (add ev.raw ctx.text) ctx ]]

def fixed-point (State.define {
  initial: start/q
  ctx: {text: ""}
  classes: {                          # his cat->col# / TAB: decoder, as data
    digit: [["0" "9"]]
    minus: ["-"]
    point: ["."]
    other: any/q                      # his `other?` column
  }
  events: { digit: {raw: String}  minus: {raw: String}
            point: {raw: String}  other: {raw: String} }
  states: {
    #           other?      num?                minus?              dp?
    start: {on: {other: {}  digit: {to: int/q   act: keep/q}
                            minus: {to: int/q   act: keep/q}
                            point: {to: frac/q  act: keep/q}}}
    int:   {on: {other: {}  digit: {act: keep/q}
                            minus: {}
                            point: {to: frac/q  act: keep/q}}}
    frac:  {on: {other: {}  digit: {act: keep/q}
                            minus: {}
                            point: {}}}
  }
} {keep: keep/r})

State.classify fixed-point "3"        # => {event: digit  raw: "3"}
State.classify fixed-point "x"        # => {event: other  raw: "x"}

def scan fn [[c:String snap:Map] [Map] [
  def r (State.step fixed-point snap {raw: c})
  r.snap
]]
def r0 (State.init fixed-point)
fold [scan] (StringUtil.split '' "-3.1x4159.7") r0.snap
#   => state frac, ctx.text "-3.141597" — the 'x' and the second '.' are
#      consumed and dropped while every legal character after them is still
#      accepted, exactly as Noble's Getafix declines to echo an illegal
#      character but keeps taking input.
```

Four things this pins down, three of them corrections the port forces:

- **The table is total, so `policy:` is inert.** Twelve cells, all written,
  because the `other` class exists. Under `policy: error/q` (the default,
  scope decision 3) nothing here can raise `state_unhandled_event` — not
  because the policy is lenient but because the alphabet is closed and the
  grid is full. This is row 19 in one machine, and the reason open question
  #2 now leans shape-dependent.
- **`{}` is an explicit ignore, and it is *internal*.** Those cells are
  Noble's `DROP`. Note carefully that his state-1 `DROP >1` and `EMIT >1`
  become `{}` and `{act: keep/q}` with **no `to:`** — because his Forth has no
  entry/exit actions, `>1` from state 1 means "stay", whereas boru's `to:` the
  current state is an *external* self-transition that runs exit and entry and
  re-arms the state's timers (§3.3.4). A mechanical transliteration of the
  arrow is a bug; the explicit distinction is what makes it a visible one.
- **`EMIT` cannot survive the port, and shouldn't.** His action echoes to the
  terminal — I/O inside the machine, which §3.2 forbids. Two honest
  translations: accumulate into `ctx`, as above, which is right when the
  validated text is the product; or have the reducer return an `fx:`
  descriptor for the host to execute, which is right when the echo genuinely
  is the point. That choice is precisely the sans-io split of row 5 — a
  constraint the Forth design predates rather than violates.
- **Termination is the host's, until you make it the machine's.** His loop
  ends on a carriage return tested outside the FSM. Above, `fold` simply runs
  out of characters. Giving the alphabet an end-of-input event and the machine
  `final: true` accept/reject states moves the decision inside, which is what
  §3.3.8 and the `state_no_final_path` advisory (§6.3) are able to reason
  about — §11.3 does it that way.

### 11.3 Noble's identifier — states versus `ctx`, and where the bound lives

A FORTRAN identifier: a letter, then up to six more letters or digits. The
paper gives it twice — eight states, one per character position, in a table
he calls "rather repetitious"; then two states plus an `id.len` counter and a
`>1?` word that computes its own target. boru takes the second's size and the
first's visibility (§3.6.3), and puts the constant back in the definition:

```boru
def bump        fn [[ev:Map ctx:Map] [Map] [ set n/q (add 1 ctx.n) ctx ]]
def under-limit fn [[ev:Map ctx:Map] [Boolean] [ lt ctx.max ctx.n ]]

def ident (State.define {
  initial: start/q
  ctx: {n: 0  max: 7}                 # the bound is spec data, not a literal
  classes: { letter: [["A" "Z"] ["a" "z"]]  digit: [["0" "9"]]  other: any/q }
  events: { letter: {raw: String}  digit: {raw: String}  other: {raw: String}
            input-end: {} }         # declared, but no class produces it
  states: {
    start: { on: { letter:    {to: body/q  act: bump/q}
                   digit:     {to: bad/q}
                   other:     {to: bad/q}
                   input-end: {to: bad/q} } }
    body:  { on: { letter:    [ {to: body/q  act: bump/q  when: under-limit/q}
                                {to: too-long/q} ]
                   digit:     [ {to: body/q  act: bump/q  when: under-limit/q}
                                {to: too-long/q} ]
                   other:     {to: done/q}
                   input-end: {to: done/q} } }
    done:     {final: true}
    bad:      {final: true}
    too-long: {final: true}
  }
} {bump: bump/r  under-limit: under-limit/r})

# the driver classifies each character, then injects end-of-input itself:
#   fold [scan] (StringUtil.split '' "x1y") (State.init ident).snap
#   State.step ident that-snap {event: input-end/q}   # => status done
```

Five states where Noble has eight or two, and the accounting is worth being
precise about, because it is the whole content of row 21:

- **Two live states, three outcomes.** Both of his versions collapse "not an
  identifier" and "identifier ended" into one terminal state and recover the
  distinction afterwards from a flag test on the state variable
  (`state< (id) @ 1 =`). Distinct `final: true` states put the outcome *in*
  the machine, where §3.3.8 returns `done/q` and a reader can see all three
  results without reading the driving loop.
- **`input-end:` is a declared event that no class produces.** It is the
  concrete case of §3.6.2's classes-are-a-subset-of-the-alphabet rule: the
  driver classifies characters and then injects `{event: input-end/q}`
  itself. (Not spelled `end`: that is the `end`/`;` keyword, and while it is a
  legal map key and atom, `spec.states.body.on.end.to` does not resolve —
  NUR066.) Without it
  the machine only reaches `done` when a *non*-identifier character happens to
  follow, so an input ending exactly at the identifier's last character would
  sit in `body` forever and the "termination is now the machine's" claim of
  §11.2 would be false.
- **The branch is enumerated, not computed.** `[{… when: under-limit/q} {to:
  too-long/q}]` is his `>1?` — same data dependence, same two possible
  successors, same `ctx` supplying the "additional information, external to
  the current state and current input." The difference is that both targets
  are literal atoms, so `State.graph` draws `body --letter [under-limit]-->
  body` alongside `body --letter--> too-long` and `state_conflict` confirms
  the unguarded variant is last (§3.3.12). (`State.can` is *not* part of that
  claim: per §4 it answers with event atoms only, so it reports `letter` and
  says nothing about which of the two targets a given `ctx` would reach —
  deliberately, for the same reason it does not evaluate guards.)
- **The bound is declared data, not a literal buried in the guard.** This is
  the refinement §3.6.3 asks for and Noble's example does not reach: `max: 7`
  sits in the spec, so it is diffable, returned by `State.spec`, covered by
  the definition hash (§3.3.9), and visible to anything reading the machine —
  while `under-limit` stays a general "have we room" predicate reusable across
  machines. Written the other way, with `7` inside the fn, the table shows
  *that* a bound decides the branch but never *what* it is, and the
  eight-state version — whose only virtue was making the number countable —
  would still be telling the reader something this one hides.

  **What that does *not* buy, stated plainly:** `ctx:` holds *defaults*, and
  §4's `State.init` merges `opts.ctx` over them. So `State.init ident
  {ctx: {max: 100}}` yields an instance that accepts 100-character
  identifiers while carrying the same `snap.v` — the hash covers the
  definition, and the definition's `max` is still 7. Declaring the bound in
  `ctx:` therefore buys **visibility and diffability, not inviolability**: it
  is the right home for a tunable default, and the wrong home for an
  invariant that must hold across every instance. v1 has nowhere better to
  put one — open question #9 asks whether it should.

## Open questions

1. **Defer-list cap size** — the overflow *behaviour* is frozen (§3.3.3:
   bounded, `state_defer_overflow` error, per-machine `defer-cap: N`
   override); is 256 the right default number? (Leaning yes; mirrors the
   mailbox-bound posture.)
2. **`state_unhandled` gating opt-in** — is `total: true` in the spec the
   right switch to promote the totality advisory to a gating Error, or should
   the strictest `policy: error/q` machines get it automatically? (Leaning
   the explicit `total: true`: policy governs *runtime* behaviour, totality
   is a *model* claim, and conflating them makes the terse case noisy.)
   **Revised by §3.6:** the leaning is now that the right default is
   *shape-dependent*, because the cost of totality is not constant. A machine
   with a `classes:` partition has a small closed alphabet with a catch-all
   in it, so its matrix is `|states| × |classes|` and filling it is what
   writing the table already means — Noble never writes a partial one, and
   §11.2's twelve cells cost nothing. A sparse `events:`-only machine over a
   wide alphabet pays per hole. Candidate rule: `classes:` implies
   `total: true` unless the spec says `total: false`; everything else keeps
   the opt-in. Maintainer call; the advisory itself is unchanged either way.

   That rule needs the matrix named, because §3.6.2 defines two and they are
   not interchangeable. **External totality** is `|states| × |classes|`: every
   state handles every input category. **Full-alphabet totality** is
   `|states| × |events|`, which additionally demands a cell for each declared
   event no class produces — `raise`d internal events, `after:` timer events,
   the `end:` of §11.3. The implication above is meant as the *external* one
   only: filling the class grid is what writing a table already means, whereas
   requiring every state to handle every timer event would force a wall of
   `{}` cells for timers only one state can ever arm, which is noise, not
   coverage. So the two should be separate checks with separate switches —
   external totality implied by `classes:`, full-alphabet totality remaining
   the explicit `total: true` — and `state_unhandled` should say which grid a
   given hole is in. Decide this before either becomes gating; shipping one
   ambiguous `total:` is precisely the semantic-fragmentation failure §2 row
   12 exists to prevent.
3. **Content-hash algorithm for `snap.v`, and the bindings gap** — canonical
   `canon` text hashed how? (Leaning fnv64 over the canonical form, the kg
   pipeline's digest precedent; the field is opaque either way.) And is the
   author-bumped `version:` field (§3.3.9) enough to signal
   binding-behaviour changes, or should `define` accept an explicit
   implementation-version that folds into `v`? (Leaning `version:` alone:
   an impl-version parameter is one more thing to forget, and it cannot be
   verified against the code either.)
4. **Outcome record shape from `State.serve`** — is `{state status}` enough,
   or should it carry the effect names executed for observability? (Leaning
   minimal now; `call {op: "status"}`-style introspection can come with the
   services consolidation.)
5. **Timer events colliding with the alphabet** — must `after:` events be
   declared in `events:` like any other (current spec: yes, they are ordinary
   events), or auto-declared? (Leaning declared explicitly: one alphabet, no
   hidden members.)
6. **Shallow history in phase 2** — adopt iff it falls out of the SCXML
   algorithm without new semantics; deep history is declined outright.
   (Leaning yes-if-free.)
7. **Should `classify:` (the fn form, §3.6.1) ship at all in v1?**
   **Decided 2026-09-26 (NUR065): ship both, on ONE set of guarantees.** The
   fn form declares its output alphabet (`yields:`) and returns a class atom
   the machine wraps in the frozen `{event raw}` payload, so alphabet
   closure, payload shape and the `state_*` diagnostics are the table
   form's for both spellings; only the mapping inside the fn stays opaque.
   (The question was framed as whether the fn form earns any of §6.3's
   checks; declaring `yields:` is what makes it earn them.)
8. **How far does `classes:` range over?** §3.6.2 defines selectors over
   boru's total value order, which makes `[1 9]` over integers and
   `[a-atom z-atom]` over atoms as legal as `["0" "9"]` over single-character
   strings. That generality is free at the specification level and may not be
   free at the implementation level — a dense byte table has an obvious
   compilation, an arbitrary ordered range does not. (Leaning: keep the
   general specification, and let the implementation compile the dense cases
   and scan the rest, per §3.6.2's representation-freedom clause. Revisit iff
   a scanning classifier shows up in the codec benchmarks.)
9. **Is there a home for a non-overridable definition constant?** §11.3 puts
   the identifier's seven-character bound in `ctx:` so the table shows it, and
   §11.3's closing note records what that misses: `opts.ctx` merges over
   `ctx:` at `State.init` (§4), so an instance can be started with a different
   bound while carrying the same `snap.v`. `ctx:` is the right home for a
   tunable default and the wrong one for a model invariant, and v1 offers
   nothing else. Candidate: a `consts:` block — plain data, hashed with the
   definition, readable by guards and reducers as a third argument or through
   a distinguished `ctx` key, and simply not mergeable at `init`. (Leaning
   defer: it is one more spec key and one more binding-shape change, and the
   §11.3 bound is a genuine tunable rather than an invariant, so the
   motivating example does not actually demand it. Revisit if a v1 machine
   turns up whose correctness depends on a constant no caller may move — a
   protocol machine's window or retry cap is the likely first one.)
