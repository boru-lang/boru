# PERMISSIONS.10 — Capability-Scoped Permissions for boru

## Status: Implemented (Phases 1–8 landed; PR-by-PR on `claude/sleepy-hamilton-7OYSz`)

This document records the design of boru's permissions model: the JSON
profile shape, the uniform scope structure with hard caps and
subscopes, the wrapped-capability enforcement layer, the CLI surface,
and the planned `boru:vm` native module that uses the same mechanism
to provide sandboxed sub-engine execution. It is the canonical
reference for `lang/go/policy` (forthcoming), the permissioned
capability wrappers, and the `--perms*` / `--allow` / `--deny` /
`--install` / `--no-install` flag family.

## TL;DR

- **Profiles are JSON/jsonic documents** with one uniform shape:
  `scope = { install?, words: { default, rules[] }, scopes?: { name: scope } }`.
- **Every scope has a `words` block.** Engine kernel words live in
  the renamed `engine` scope; module exports live in `modules.scopes.<id>.words`;
  capability operations (`read`, `write`, `connect`, …) live in their
  own scopes (`fileops`, `network`, `sqlite`, `formats`, `env`,
  `process`, `clock`).
- **`global` is a top-level scope with a hardcoded enum of coarse
  cap names** (`disk.read`, `disk.write`, `network`, `process`, `env`,
  `clock`, `system-info`, `mutate`). Globals are *hard caps*:
  capability rules cannot grant what a global denies.
- **`install: false` uninstalls a capability** from the registry
  entirely (no wrapper, no slot, distinct error code from
  policy-denied). Default is `install: true`.
- **Defaults are allow-everything.** Absence of a policy, or a policy
  field, means no restriction. Permissions are opt-in.
- **Profiles inherit via `extends`.** Rules append, defaults override.
  Last-match-wins inside a scope.
- **Enforcement is by wrapping capabilities at registry-construction
  time** — never by stack inspection. The wrapper is the only path
  to the operation; there is no unwrapped reference reachable from
  handler code.
- **The HTTP exec service is policy-immutable.** Policy is set at
  service startup; requests cannot supply or modify it. Run multiple
  exec instances on different ports for different policies.
- **The `boru:vm` native module exposes sub-engine execution** under
  a further-restricted policy. Capability attenuation: a sub-engine's
  policy must be a subset of its parent's effective policy.

---

## Motivation

boru is increasingly used in contexts where untrusted or
semi-trusted code runs inside a trusted host:

1. The new `boru exec` HTTP service evaluates code submitted by
   clients. Without a permissions model, anything a `boru` binary
   can do — read `/etc`, open sockets, write files, fork processes —
   is reachable from a curl request.
2. Plugin and extension scenarios (REPL extensions, embedded scripts
   in larger Go programs, the wasm playground) want defence in depth
   beyond "the language has no I/O" (it does, via the `read`/`write`/
   `import`/`fetch`/sqlite/etc. words).
3. Educational and CTF deployments want preset sandboxes
   ("read-only", "compute-only", "network-client") that are reliable,
   debuggable, and easy to extend.
4. Multi-tenant exec deployments want to bind different policies to
   different ports/clients/tokens.

The existing FileOps abstraction
([FILE-ACCESS.10](FILE-ACCESS.10.md)) already proves the pattern: a
single capability interface, swappable for testing or sandboxing.
This proposal generalises that pattern to every host capability and
adds a structured policy layer on top.

## Lessons that shape the design

We reviewed three prior systems' failure modes:

- **Java SecurityManager** (1996–2025, removed in JEP 486). Killed
  by stack-inspection unsoundness, leaky coverage (every new JDK
  feature reopened gaps), 50+ permission classes with bespoke
  shapes, performance overhead leading to widespread disabling,
  and policy files no production team could write correctly.
- **.NET Code Access Security** (2002–2016, dropped from .NET Core).
  Same shape, same failure modes. Evidence-based code identity
  fragile under reflection, P/Invoke, Reflection.Emit.
- **Deno permissions** (2018–). The first working modern attempt
  — coarser than Java SM, but capability-style at the syscall layer,
  with declarative scoping (`--allow-read=/tmp`). Still process-wide.

The shape we adopted explicitly responds to these:

| Failure mode | This design |
|---|---|
| Stack inspection's confused-deputy & `doPrivileged` footguns | Object-capability via wrapping; no stack walk |
| 50+ permission classes with bespoke shapes | One scope shape, recursively nested |
| Coverage leakage as new features appear | The capability surface is small (≤10 caps, ~200 native words); every new I/O path goes through an existing wrapper |
| Performance overhead from always-on stack walks | Single hashtable lookup per call; structural denial when uninstalled |
| Unauthorable policy files | Declarative JSON with `extends`; built-in profiles ship in the binary |
| Opaque denials ("permission denied" with no context) | Errors carry the blame chain (`policy 'X' rule #3 denied <scope>.<op> with args <args>`) |
| In-process bypass via native code | Capability slot is the only access path; no `Unsafe`-equivalent exposed |

In-process permissions are not a substitute for OS-level isolation
for adversarial workloads. This design is defence in depth for
moderately untrusted code; deployments handling truly hostile input
should additionally run the host process in a container with
seccomp, a read-only rootfs, and net namespace isolation.

---

## Profile JSON shape

A profile is a jsonic document with the following top-level fields:

```jsonc
{
  "version": 1,
  "name":    "untrusted-eval",
  "extends": "sandbox",                    // optional parent profile

  "limits": {                              // optional resource bounds
    "timeoutMs":      1000,
    "maxStepBudget":  100000,
    "maxStackDepth":  256,
    "maxMemoryBytes": 67108864,
    "maxOutputBytes": 65536
  },

  "scopes": {
    "global":  { ... },                    // hard caps; fixed enum
    "engine":  { ... },                    // kernel native words
    "modules": { ... },                    // module import + per-module
    "fileops": { ... },
    "network": { ... },
    "sqlite":  { ... },
    "formats": { ... },
    "env":     { ... },
    "process": { ... },
    "clock":   { ... }
  }
}
```

### The recursive scope shape

```
scope = {
  install?: bool,                          // default true; false → uninstall
  words:    { default, rules[] },          // operation gate for this scope
  scopes?:  { <name>: scope, ... }         // optional subscopes
}

words = {
  default: "allow" | "deny",               // applied if no rule matches
  rules:   rule[]                          // ordered; last match wins
}

rule = {
  allow?: string[],                        // op-name globs; always an array
  deny?:  string[],                        // op-name globs; always an array
  where?: { <arg-name>: <value>[], ... }   // predicates; all arrays
}
```

A rule has exactly one of `allow` / `deny`. To grant some and deny
others, write two rules. Rule order is meaningful (later overrides
earlier — IAM/AppArmor-style).

### Defaults

The defaults are **allow-everything**. A `*lang.Boru` with no policy
runs without any check. A profile with no `scopes` block is allow
everything. A scope with no fields acts as
`{install: true, words: {default: "allow", rules: []}, scopes: {}}`.

This makes the model **opt-in**: existing code that doesn't set a
policy is unaffected. Backwards compatibility is preserved.

### The `global` scope

`global` is special: its `words` block addresses a **hardcoded enum**
of coarse cap names. The schema rejects unknown names.

| Global op | Bound by capabilities |
|---|---|
| `disk.read` | fileops.read, sqlite.open, format.decode-from-file |
| `disk.write` | fileops.write, fileops.mkdir, sqlite.exec(write), sqlite.open(rw), format.encode-to-file |
| `network` | network.*, fetch.*, http.* |
| `process` | process.spawn, shell.exec |
| `env` | env.read, env.write |
| `clock` | clock.now, clock.monotonic |
| `system-info` | hostname, os, arch, cpu, user |
| `mutate` | def, undef, set (on shared state) |

Each capability wrapper declares which globals its operations touch.
On every call, the policy checks the relevant globals first; if any
denies, the call denies. **Capability scope rules cannot grant what a
global denies.** This is the AWS IAM SCP / OpenBSD `pledge` /
Linux capability bounding-set pattern.

`install: false` is invalid on `global` (the cap-name layer is
intrinsic to the policy mechanism — can't uninstall the policy
itself).

### The `engine` scope

`engine.words` controls the kernel native words registered via
`lang/go/native/register.go::Register` — the contents of
`mathNatives`, `stackNatives`, `controlNatives`, `definitionNatives`,
etc., plus the consolidated `Natives` slice.

Module exports are **not** controlled here; they live under
`modules.scopes.<id>.words`. There is no `math.*` glob in the
engine scope.

`install: false` is invalid on `engine` (can't uninstall the language).

### The `modules` scope

Two-layer:

- **Outer**: `modules.words` controls which module IDs (`boru:math`,
  `boru:time`, etc.) can be imported.
- **Inner**: `modules.scopes.<module-id>.words` controls which
  exports of that module are callable once imported. Same scope
  shape applied recursively.

Setting `modules.install = false` disables the module system
entirely — the resolver is not attached. `import` returns
`[boru/modules_disabled]` before any policy lookup.

Setting `modules.scopes.<id>.install = false` is equivalent to
denying the module ID in `modules.words` — the resolver refuses to
load it. Useful when many modules are denied and a per-module
declaration is more readable.

### Capability scopes

`fileops`, `network`, `sqlite`, `formats`, `env`, `process`, `clock`.

- `<cap>.install = false` removes the capability from the registry.
  The slot is empty; `HostX(r)` returns `nil`; handlers that try to
  reach it raise `[boru/capability_not_installed]: <cap>`.
- `<cap>.words.{default, rules}` controls calls to installed
  capabilities. Operations and where-predicates are
  capability-specific (paths for fileops, host/port for network,
  format names for formats, etc.).

### Example profile

```jsonc
{
  "version": 1,
  "name":    "untrusted-eval",
  "extends": "sandbox",

  "limits": {
    "timeoutMs":     1000,
    "maxStepBudget": 100000
  },

  "scopes": {
    "global": {
      "words": {
        "default": "deny",
        "rules": [
          { "allow": ["disk.read", "clock", "mutate"] },
          { "deny":  ["disk.write", "network", "process",
                      "env", "system-info"] }
        ]
      }
    },

    "engine": {
      "words": {
        "default": "deny",
        "rules": [
          { "allow": ["add", "sub", "mul", "div", "abs", "sqrt"] },
          { "allow": ["dup", "drop", "swap", "over", "rot", "depth"] },
          { "allow": ["if", "for", "each", "fold"] },
          { "allow": ["def", "undef"] },
          { "allow": ["concat", "upper", "lower", "split", "trim"] },
          { "allow": ["eq", "lt", "gt", "lte", "gte", "cmp"] },
          { "deny":  ["shell", "spawn"] }
        ]
      }
    },

    "modules": {
      "words": {
        "default": "deny",
        "rules": [{ "allow": ["boru:math-util", "boru:time-util"] }]
      },
      "scopes": {
        "boru:math-util": {
          "words": {
            "default": "allow",
            "rules": [{ "deny": ["pow"] }]
          }
        },
        "boru:time-util": {
          "words": {
            "default": "allow",
            "rules": [{ "deny": ["sleep"] }]
          }
        }
      }
    },

    "fileops": {
      "words": {
        "default": "deny",
        "rules": [
          { "allow": ["read"],
            "where": { "path": ["/tmp/boru/**", "/srv/data/**"] } },
          { "deny":  ["read"],
            "where": { "path": ["**/.git/**", "**/secrets/**"] } },
          { "allow": ["write"],
            "where": { "path": ["/tmp/boru/**"],
                       "maxBytes": [1048576] } }
        ]
      }
    },

    "network": { "install": false },
    "sqlite":  { "install": false },
    "process": { "install": false },

    "formats": {
      "words": {
        "default": "allow",
        "rules": [
          { "deny": ["encode", "decode"],
            "where": { "format": ["sqlite", "binary"] } }
        ]
      }
    },

    "env": {
      "words": {
        "default": "deny",
        "rules": [
          { "allow": ["read"],
            "where": { "name": ["LANG", "TZ", "BORU_*"] } }
        ]
      }
    }
  }
}
```

---

## Evaluation algorithm

A capability operation passes both checkpoints, in order:

```
check(scope_name, op, args):
  # 1. Hard caps. Each global this op touches must allow.
  for g in globals_touched_by(scope_name, op):
    if eval(policy.scopes.global.words, g, {}) != allow:
      return DENY(blame="global." + g)

  # 2. Scope rule.
  s = policy.scopes[scope_name]
  if s == nil:
    return ALLOW                # absent scope → allow-all default
  if s.install == false:
    return DENY(blame=scope_name + ".install=false",
                code="capability_not_installed")
  if eval(s.words, op, args) != allow:
    return DENY(blame=scope_name + ".words")

  # 3. Subscope, if applicable (modules.export call).
  if scope_name == "modules" and op == "call":
    sub = s.scopes[args.module]
    if sub == nil:
      return DENY(blame="modules.scopes." + args.module + " missing")
    if sub.install == false:
      return DENY(blame="modules.scopes." + args.module + ".install=false",
                  code="capability_not_installed")
    if eval(sub.words, args.export, args) != allow:
      return DENY(blame="modules.scopes." + args.module + ".words")

  return ALLOW

eval(words, op, args):
  decision = words.default
  for rule in words.rules:
    if !match_any(rule.allow ?? rule.deny, op): continue
    if !match_where(rule.where, args): continue
    decision = (rule.allow != nil) ? allow : deny
  return decision
```

This is the AppArmor/AWS-IAM/Sentinel last-match-wins rule. It's
more predictable under profile inheritance than "deny-always-wins"
because rule order corresponds directly to the source order in the
extended chain.

### Profile resolution (`extends`)

```
resolve(name):
  if base = name.extends:
    parent = resolve(base)
    out = parent.clone()
  else:
    out = empty_policy()
  for scope_name, scope in name.scopes:
    if scope_name in out.scopes:
      out.scopes[scope_name].words.rules += scope.words.rules
      out.scopes[scope_name].words.default = scope.words.default ?? parent.default
      out.scopes[scope_name].install = scope.install ?? parent.install
      out.scopes[scope_name].scopes.merge(scope.scopes)
    else:
      out.scopes[scope_name] = scope
  return out
```

Child rules are *appended* to parent rules — so a child can override
specific cases without rewriting the parent. Profile inheritance is
single (one `extends`), to avoid diamond merge questions; multiple
inheritance can be added later with explicit precedence if needed.

---

## Built-in profiles

These ship in the binary. Resolution order: built-ins first, then
`$XDG_CONFIG_HOME/boru/policies/<name>.jsonic`, then user-defined
aliases in `~/.borurc`.

| Name | Description |
|---|---|
| `full` | Default. Allow everything. Equivalent to no policy. |
| `trusted` | All capabilities installed; all defaults allow. |
| `sandbox` | Hard-deny `disk.write`, `network`, `process`. Read disk allowed; engine words allowed; modules allow `boru:math` only. The baseline sandbox. |
| `compute` | Pure computation. All caps `install: false` except formats decode (in-memory only). No I/O at all. |
| `read-only` | Read disk + read env vars + parse formats. No writes, no network, no process. |
| `client` | Read disk + outbound HTTPS to a configurable allowlist. No writes, no inbound network. |

Users extend these via `extends: sandbox` or override fields directly.

---

## Go-side implementation

### Package layout

```
lang/go/policy/
├── policy.go           # public Policy interface + Profile struct
├── profile.go          # JSON shape + JSON/jsonic loader
├── resolve.go          # extends chain → flat profile
├── evaluate.go         # the check() algorithm
├── glob.go             # path/host/word glob matcher
├── builtin.go          # //go:embed of the built-in profile JSONs
├── error.go            # Denied error type with blame chain
└── policy_test.go      # parsing / extends / glob / eval

lang/go/native/
├── permissioned_fileops.go
├── permissioned_formats.go
├── permissioned_sqlite.go
├── permissioned_network.go   (when network capability exists)
├── permissioned_env.go       (when env capability exists)
└── policy_install.go         # auto-wrap on SetHostX

lang/go/policy/profiles/
├── full.jsonic
├── trusted.jsonic
├── sandbox.jsonic
├── compute.jsonic
├── read-only.jsonic
└── client.jsonic
```

### Public types

```go
package policy

type Policy interface {
    // Scope returns the named scope (or a zero-value Scope if absent;
    // a zero Scope means "allow all" per the defaults rule).
    Scope(name string) Scope

    // Check verifies (scope, op, args) is allowed. Returns *Denied on
    // failure; nil on allow.
    Check(scope, op string, args Args) error

    // CheckGlobal verifies a global hard-cap name is allowed. Returns
    // *Denied on failure; nil on allow.
    CheckGlobal(name string) error

    // Limits returns the policy's resource limits, with package
    // defaults filled in for unset fields.
    Limits() Limits
}

type Scope struct {
    Install *bool                  `json:"install,omitempty"`
    Words   WordsBlock             `json:"words,omitempty"`
    Scopes  map[string]Scope       `json:"scopes,omitempty"`
}

func (s Scope) Installed() bool {
    return s.Install == nil || *s.Install
}

type WordsBlock struct {
    Default Effect   `json:"default,omitempty"`   // "allow" | "deny"
    Rules   []Rule   `json:"rules,omitempty"`
}

type Rule struct {
    Allow []string             `json:"allow,omitempty"`
    Deny  []string             `json:"deny,omitempty"`
    Where map[string][]any     `json:"where,omitempty"`
}

type Args map[string]any

type Effect string
const ( EffectAllow Effect = "allow"; EffectDeny Effect = "deny" )

type Denied struct {
    Scope, Op, Profile, Blame string
    Args                      Args
    Code                      string   // permission_denied | capability_not_installed | …
}
func (d *Denied) Error() string { ... }

type Limits struct {
    TimeoutMs      int64 `json:"timeoutMs,omitempty"`
    MaxStepBudget  int64 `json:"maxStepBudget,omitempty"`
    MaxStackDepth  int   `json:"maxStackDepth,omitempty"`
    MaxMemoryBytes int64 `json:"maxMemoryBytes,omitempty"`
    MaxOutputBytes int64 `json:"maxOutputBytes,omitempty"`
}
```

### Capability slot

The policy is stored on the registry as a capability:

```go
// lang/go/native/capabilities.go
const CapPolicy = "engine.policy"

func HostPolicy(r *Registry) policy.Policy { ... }
func SetHostPolicy(r *Registry, p policy.Policy) { ... }
```

### Wrapped capabilities

One wrapper per capability, all the same three-line method shape:

```go
type permissionedFileOps struct {
    inner  capabilities.FileOps
    policy policy.Policy
}

func (p *permissionedFileOps) WriteFile(path string, data []byte, m os.FileMode) error {
    if err := p.policy.CheckGlobal("disk.write"); err != nil { return err }
    if err := p.policy.Check("fileops", "write", policy.Args{
        "path":  path,
        "bytes": int64(len(data)),
    }); err != nil { return err }
    return p.inner.WriteFile(path, data, m)
}
// ReadFile, MkdirAll, ResolvePath — same shape.
```

### Install hooks

`SetHostFileOps` etc. consult the policy at install time:

```go
func SetHostFileOps(r *Registry, ops capabilities.FileOps) {
    pol := HostPolicy(r)
    if pol != nil && !pol.Scope("fileops").Installed() {
        return                              // slot stays empty
    }
    if pol != nil {
        ops = NewPermissionedFileOps(ops, pol)
    }
    _ = r.Capabilities.Set(CapFileOps, ops)
}
```

Policy must be set **before** capabilities (or `RewrapCapabilities(r)`
called after the policy changes). `DefaultRegistry` enforces the
order: policy from `Options.Policy` is installed first; capability
defaults follow.

### Engine integration

`stepWord` consults the policy for engine kernel words:

```go
// eng/go/engine.go (sketch)
func (e *Engine) stepWord(name string) error {
    if pol := HostPolicy(e.r); pol != nil {
        if err := pol.Check("engine", name, nil); err != nil {
            return err
        }
    }
    // …existing dispatch…
}
```

Module resolver consults the policy at `import`:

```go
// lang/go/modules/modules.go (sketch)
func Resolve(name string, parent *native.Registry) (native.ModuleDesc, error) {
    if pol := native.HostPolicy(parent); pol != nil {
        if err := pol.Check("modules", "import", policy.Args{"module": "boru:" + name}); err != nil {
            return native.ModuleDesc{}, err
        }
    }
    // …existing resolution…
}
```

And per-export dispatch — **implemented 2026-08-02 (NUR045)**, though
not by a check at each dispatch site: the export's policy identity is
stamped onto its dispatchable signatures once at module-resolution time
(`eng.StampModuleCallGates`), and every chokepoint on both engines gates
on that stamp, so no dispatch route can be missed and an unruled profile
pays one precomputed boolean. The shape of the check is as sketched:


```go
// When dispatching a module-imported word "math.sin":
if pol := HostPolicy(r); pol != nil {
    if err := pol.Check("modules", "call", policy.Args{
        "module": "boru:math-util",
        "export": "sin",
    }); err != nil {
        return err
    }
}
```

> **Caveat.** These hooks read the policy off the registry the engine is
> *currently executing on*. That is the top-level registry for a top-level
> call, but an imported module runs its body on a **child registry that
> does not receive `CapPolicy`**, so the checks above silently become
> allow-all inside module code. See **[Known gap: child module registries
> do not inherit the policy](#known-gap-child-module-registries-do-not-inherit-the-policy)**
> below.

---

## CLI surface

### Policy specification

```
--perms=<name|path|jsonic>     # auto-detect by content
--perms-file=<path>            # explicit file path
--perms-inline=<jsonic|@-|@file>  # explicit inline / stdin / file
```

Detection rule: starts with `{` → inline jsonic; contains `/` or
ends `.jsonic`/`.json` → file path; otherwise → profile name.

### Incremental modifiers

```
--allow=<scope>.<op>           # add an allow rule for the op
--deny=<scope>.<op>            # add a deny rule for the op
--allow-global=<op>            # raise a global hard cap
--deny-global=<op>             # lower a global hard cap
--no-install=<scope>           # set scope.install=false
--install=<scope>              # set scope.install=true (override base)
```

Repeated flags accumulate; each invocation adds one rule. The path
is `scope.op` or `scope.subscope.op` (e.g. `engine.add`,
`modules.boru:math`, `modules.boru:math.sin`).

Where-bearing rules (paths, hosts, byte limits) cannot be expressed
as atomic flags. Use `--perms-inline` or `--perms-file` for those.

### Process baseline in `boru serve`

The top-level `--perms*` flag (before any segment) is the **process
baseline**: a hard cap every service in the process inherits. Each
segment can carry its own `--perms*` flags that further restrict
within the baseline.

```bash
boru serve --perms=baseline \
    exec -p 8091 --perms=sandbox-public \
  + exec -p 8092 --perms=sandbox-internal \
  + lsp  -p 9000 --perms=trusted
```

Effective policy per engine = baseline ∩ service-policy. Hard caps
in baseline cannot be relaxed by any segment.

For multiple instances of the same service type with different
policies, segment aliasing (planned: `<type>@<alias>` syntax) is
required since `boru serve` rejects duplicate segment names.

### Per-request: deliberately omitted

The HTTP exec service does **not** accept policy in the request
body. The policy is bound at service startup; clients cannot supply
or modify it. Deployments needing per-client policies run multiple
exec instances (one per policy, on different ports/tokens).

### Authoring subcommand: `boru policy`

```bash
boru policy list                                # all available profiles
boru policy show <name>                         # pretty-print resolved profile
boru policy show <name> --json                  # raw JSON
boru policy diff <a> <b>                        # difference between profiles
boru policy resolve <name> --allow=engine.add   # show post-flag policy

boru policy new <name> --extends <base>         # creates user policy file
boru policy edit <name>                         # $EDITOR on the file
boru policy validate <path>                     # schema + sanity check

boru policy test <name> --check=<scope>.<op>    # exit 0 allowed, 1 denied
boru policy explain <name> \
    --check=fileops.write \
    --args=path=/tmp/foo,bytes=1024            # which rule decided, why
```

`explain` is the killer DX feature — direct response to the Java-SM
debuggability lesson. Output identifies the matching rule index,
the resolved profile chain, the active globals, and a suggested
rule that would change the answer.

### Dry-run mode

> **Status: not implemented.** The observe-only decorator this section
> designs was never built; the `--policy-dry-run` flag shipped inert
> (parsed but never wired to anything) and was removed under NUR042 —
> a security-adjacent flag that does nothing is worse than no flag.
> This section remains the spec if the decorator is ever genuinely
> needed.

```bash
boru exec --perms=sandbox --policy-dry-run script.boru
# stderr:
# [policy] WOULD-DENY fileops.write path=/etc/hosts          (line 12)
# [policy] WOULD-ALLOW engine.add                            (line 22)
```

The program still runs (capabilities allow everything) but every
check is logged with the decision the live policy would have made.
Workflow: run with dry-run, audit the log, remove dry-run.

### Environment fallback

```bash
BORU_POLICY=sandbox boru do '1 add 2'
BORU_POLICY_FILE=./prod-policy.jsonic boru script.boru
```

Precedence: explicit `--perms*` flags > `BORU_POLICY*` env >
in-script frontmatter (`#boru:policy=sandbox`) > default `full`.

---

## The `boru:vm` native module — sandboxed sub-engine execution

The same policy mechanism that protects the host registry can be
used by boru code itself to spawn restricted sub-engines. The
`boru:vm` module exposes this surface.

### Words

```boru
import "boru:vm"

# Run code in the default-sandboxed sub-engine.
"1 add 2" vm.run                          # → 3

# Run with an explicit policy map.
"...code..." vm.run-with {
  global: { "disk.write": "deny", "network": "deny" }
  scopes: { fileops: { words: { default: "deny", rules: [] } } }
}

# Convenience constructors that compose into a policy map.
"...code..." vm.run-sandbox               # uses built-in 'sandbox' profile
"...code..." vm.run-compute               # built-in 'compute' profile

# Resource limits.
"...code..." vm.run-with { limits: { timeoutMs: 500 } }

# Catch denials as values rather than errors.
"...code..." vm.try-run                   # → { ok: true, result: 3 }
                                          # or { ok: false, error: "..." }
```

### Implementation sketch

```go
// lang/go/modules/vm.go
package modules

import (
    lang "github.com/boru-lang/boru/lang/go"
    "github.com/boru-lang/boru/lang/go/native"
    "github.com/boru-lang/boru/lang/go/policy"
)

func BuildVMModule(parent *native.Registry) (native.ModuleDesc, error) {
    // … register vm.run, vm.run-with, vm.run-sandbox, vm.try-run …
}

// run-with handler:
func vmRunWith(args []native.Value, _ map[string]native.Value, _ []native.Value, parent *native.Registry) ([]native.Value, error) {
    code := args[0].AsConcreteString()
    polMap := args[1]                                 // jsonic-shape map

    // 1. Parse the policy from the map.
    inner, err := policy.FromMap(polMap)
    if err != nil { return nil, err }

    // 2. Attenuate: the child's effective policy must be a subset
    //    of the parent's. The child cannot grant anything the
    //    parent doesn't already have.
    parentPol := native.HostPolicy(parent)
    if parentPol != nil {
        if err := policy.RequireSubset(inner, parentPol); err != nil {
            return nil, err
        }
    }

    // 3. Construct the sub-engine.
    a, err := lang.New(lang.Options{Policy: inner})
    if err != nil { return nil, err }

    // 4. Run.
    result, err := a.Run(code)
    if err != nil { return nil, err }
    return marshalToParentStack(result), nil
}
```

### Attenuation rule (CRITICAL)

A sub-engine's policy must be a **subset** of its parent's effective
policy. The child cannot grant itself anything the parent doesn't
have. Concretely, `policy.RequireSubset(child, parent)`:

- For every scope, `child.install == false || parent.install == true`
  (child can disable a cap; can't enable one the parent disabled).
- For every `(scope, op, args)` the parent denies, the child must
  also deny.
- For the global scope, `child.global.deny ⊇ parent.global.deny`.

The simplest implementation: compute the parent's effective policy
and intersect with the child's. If the child requests anything
outside the intersection, raise an attenuation error before
constructing the sub-engine. This is the standard
capability-attenuation pattern (Deno workers, browser iframes with
`sandbox` attribute, OCAP languages).

### Composition

Sub-engines compose recursively. An outer engine spawns a sub-engine;
the sub-engine can itself `import` `boru:vm` and spawn its own
further-restricted sub-engine. Each layer is bounded by its parent's
effective policy. The chain is finite (depth limit configurable as a
limit field; default 8) to prevent runaway recursion.

### Use cases enabled by `boru:vm`

1. **Property testing harness**. A trusted test runner spawns
   sub-engines to evaluate candidate solutions under restricted
   policies; bugs in candidate code can't damage the harness.
2. **REPL extensions and macros**. User-written REPL meta-commands
   run in a sandboxed sub-engine — a misbehaving macro can't
   exfiltrate environment variables or write files.
3. **Untrusted formula evaluation**. boru embedded in spreadsheet or
   reporting tools: formulae from untrusted sources run in
   sub-engines.
4. **Module loading with policy**. `import` could grow a
   `with-policy` form: `import "boru:third-party"-with {...}` —
   the imported module runs under the specified policy.
5. **The wasm playground (`wpg`)**. The browser playground can
   default to `sandbox` for shared sessions; users can opt up to
   `trusted` for their own sessions.
6. **The HTTP exec service**. Each request could optionally spawn a
   sub-engine via vm.run-with to further restrict beyond the
   service's bound policy — useful for tiered access (token →
   policy → optional further restriction).

### Why this works (capability hygiene)

Three invariants make `boru:vm` sound:

- **Sub-engine has its own registry.** Capability slots are not
  shared with the parent. Wrapped capabilities use the child policy.
- **Attenuation is enforced at construction.** No way to bypass by
  running first and asking forgiveness — the policy intersection
  check happens before `lang.New`.
- **No shared mutable state.** The parent's def table, args stack,
  context store are not visible from the sub-engine. (Marshalled
  values are by-copy.) This is the same isolation `boru serve` uses
  to keep multiple service engines independent.

The structural guarantee that made the host-side enforcement sound
(wrapped capabilities, no ambient authority) carries directly into
the boru-level surface. `vm.run` is not "a privileged escape hatch";
it's "the same enforcement, exposed as a word."

---

## Comparison to prior art

| System | Decision primitive | Granularity | Modern verdict |
|---|---|---|---|
| Java SecurityManager | Stack inspection + policy file | 50+ permission classes | Removed (JEP 486) |
| .NET CAS | Stack inspection + evidence | Bespoke per-permission | Dropped in .NET Core |
| Deno permissions | CLI flags + runtime API | OS-syscall families | Working baseline |
| Node `--experimental-permission` | Process-wide flags | Coarse (fs / net / proc) | Improving |
| WASI capabilities | Imports = ambient authority | Per host function | Foundational |
| Pony / E / OCAP | Type-level capability refs | Object-level | Strongest model |
| Tcl safe-tcl | Allowlisted command set on slave interp | Per-command | Closest match |
| AppArmor | Profile file, path/cap globs | Per-process | Production baseline |
| AWS IAM | JSON policy, Effect × Action × Resource | Per-API call | Cloud lingua franca |
| OpenBSD pledge/unveil | Voluntary syscall + path narrowing | Per-process | Underrated |
| Capsicum | Capability mode, fd-based | Process-level | Minimal & sound |

boru's design borrows: the **per-command allowlist** (safe-tcl), the
**uniform JSON shape** (IAM), the **profile-files-with-glob-rules**
shape (AppArmor), the **wrap-the-capability** structural guarantee
(WASI / OCAP), the **last-match-wins** ordering (IAM/AppArmor), and
the **declarative-only** surface (Deno).

It explicitly **avoids**: stack inspection, runtime grant/revoke
APIs (Deno's are deprecated for this reason), evidence-based code
identity, and process-wide-only scope.

---

## Known gap: child module registries do not inherit the policy

**Status update (2026-09-26): CLOSED by NUR079.** A module body now runs
under the importer's policy (`runModuleBodyCover` installs
`HostPolicy(parent)` on the child — half (i), 2026-08-18), and a FILE
module import passes the same three checks a native one does, keyed on
the import's ref with `kind: "file"` (half (ii)): the restrictive built-ins
admit file modules as a class, a profile can refuse one or all of them,
refusals carry their policy code, and `boru check`, the run's pre-flight,
`boru describe` and the language server run the analysis — which executes
module bodies — under the same profile. The text below is the disclosure
as written before those fixes, kept for its analysis.

**Status: known defect (as of 2026-07).** The enforcement hooks in
[Engine integration](#engine-integration) are correct for top-level code
but do **not** reach code that runs *inside an imported module*. The
supply-chain framing, the per-import-edge fix, and the `Compose`-not-inherit
rule are worked out in depth in
[MODULE-SECURITY.0](MODULE-SECURITY.0.md) §3.4 and §5–§6; this section is
the disclosure on the *implemented* side, so a reader of this doc does not
assume module code is gated when it is not.

### Why it happens

Every policy check reads `HostPolicy(r)` off the registry the **live
dispatching engine** is bound to — `stepWord` reads `HostPolicy(e.r)`, and
a capability handler is invoked with `e.registry` (`eng/go/engine.go`
`execMatch`), *not* the sub-registry a module's exports were registered in.
At the top level that registry carries the policy, so a top-level
`Vault.reveal` or `IO.write` is gated exactly as designed.

But an imported module runs its body on a **fresh child registry that never
receives `CapPolicy`**:

- `native.ModuleInheritedCaps` — the allow-list of capability slots copied
  parent→child at import (`lang/go/native/native_module_module.go`
  `RunModuleBody`) — contains only the two host seams (`engine.tui.host`,
  `engine.vault.host`). The policy capability (`CapPolicy = "engine.policy"`)
  is never on it, and no loader calls `SetHostPolicy` on a child.
- The child is built by `DefaultRegistryWithPolicy(nil, …)`, which installs
  a policy only when one is passed (`if p != nil { SetHostPolicy(r, p) }`).
  So `HostPolicy(childReg)` is `nil`.
- A `nil` policy means **allow-all** everywhere: `checkVaultPolicy`,
  `checkNetPolicy`, `checkFetchPolicy`, `checkProcessPolicy`,
  `checkTuiPolicy`, and the log checks all `return nil` when
  `pol == nil`, and `policyGateWord` no-ops when
  `LookupWordChecker(e.registry)` is nil.

### What leaks

Inside a module body (`e.registry` is the policy-free child):

- **Dispatch-time capability checks are skipped.** A profile that extends
  `trusted` with `vault.words.default: deny` still lets an imported module
  call `Vault.rotate` / `Vault.reveal`, because the vault *host seam* is
  copied into the child (it is on `ModuleInheritedCaps`) while the policy
  that would gate it is not. The same holds for `fetch`, sockets, process
  spawn, tui, and log emit/install.
- **The kernel-word (`engine`-scope) gate is disabled** — `policyGateWord`
  finds no checker on the child, so per-word `engine` denials do not fire
  for words run inside the body.
- **The import gate is self-bypassing for nested imports.** The module
  resolver checks `HostPolicy(parent)`; for an `import` issued *from within*
  a module body the parent **is** the policy-free child, so
  `modules.install`, per-module `install:false`, and the `modules.import`
  check are all skipped. A top-level program under a restrictive policy can
  therefore circumvent it wholesale by moving the denied imports and
  capability calls into an imported file or inline `module [ … ]` body —
  reachable through a single `import "./m.boru"` that the policy did not
  gate.

### What bounds it (and what makes it worse)

- **FileOps stays gated on the file/inline path.** `RunModuleBody` copies
  the parent's *already-wrapped* `permissionedFileOps` **value** into the
  child (`SetHostFileOps(modReg, HostFileOps(parent))`). Enforcement is
  baked into that object, not re-read off the registry at call time, so
  `IO.read` / `IO.write` inside a file or inline module body stay policed
  even though `HostPolicy(child)` is `nil`. Only checks that re-read the
  policy off the registry leak.
- **`boru:vm` sub-engines are exempt.** `Vm.run` deliberately
  `policy.Compose`s the parent policy and installs it on the sub-engine
  (`newSubEngineRegistry` → `newDefaultRegistryWithPolicy(pol)`), so the
  [Why this works](#why-this-works-capability-hygiene) reasoning holds
  there — the gap is specific to the `import` path, not sub-engines.
- **The hybrid boru-app loaders are strictly worse.** `boru:sift`,
  `boru:repl`, and `boru:vault-tui` build their child with a bare
  `newDefaultRegistry()` and run an embedded boru program on it. With no
  policy present, that child's `fileops` **and** `sqlite` slots are
  installed *fresh and unwrapped* (the `SetHostX` hooks wrap only when a
  policy is present) — so on these loaders even file and database effects
  run unchecked, on top of the dispatch-time checks above.

None of this bites the default posture: with no policy installed there is
nothing to escape. It is load-bearing only when a host installs a
restrictive policy and expects it to cover imported module code — exactly
the threat model of [MODULE-SECURITY.0](MODULE-SECURITY.0.md).

### Fix direction

Propagate the policy into every child registry at import — minimally by
adding `CapPolicy` to `native.ModuleInheritedCaps`, or (properly, per the
attenuation design) by having `RunModuleBody` and the native-module install
path install `effective`-parameterised capability *wrappers* plus a
composed child `CapPolicy`, the way `boru:vm` already does. The per-edge
grant algebra and the `Compose`-not-inherit rule live in
[MODULE-SECURITY.0](MODULE-SECURITY.0.md) §5.2 and §6. A regression test
should pin a policy that denies `vault` (and one that denies a nested
`import`) which today passes *only* because the child sees a `nil` policy.

---

## What this design does **not** address

- **Resource accounting beyond limits.** CPU/memory caps are
  best-effort via the `Limits` block; for hard guarantees, run the
  host process under cgroups.
- **Side-channel attacks.** Timing oracles, cache-based attacks etc.
  are out of scope — handle at the OS/container layer.
- **Network capability is unimplemented.** boru doesn't currently
  have a first-class network word set. When one is added (planned
  for `fetch`-family generalisation), it slots into `scopes.network`
  with the same shape.
- **Process capability is unimplemented.** Likewise — boru has no
  shell word today. If one is ever added, it requires the `process`
  scope plus the `process` global cap.
- **Cryptographic operation policy.** A future `boru:crypto` module
  could gate `sign`/`verify`/`encrypt`/`decrypt` via its own subscope
  under `modules.scopes.boru:crypto`.
- **User identity and multi-tenancy at the policy layer.** The
  policy doesn't know about users. Bind a user to a policy at the
  application layer (e.g. token → policy lookup in the exec
  service's auth middleware) before the boru instance is constructed.

---

## Open questions

- **Glob syntax**: posix-shell (`*` no `/`, `**` greedy) vs.
  Go's `filepath.Match` (no `**`). Lean towards posix-shell with
  `doublestar` library since paths cross directory boundaries.
- **Where-predicate language**: keep flat key→list, or add
  comparison ops (`{ "bytes": { ">": 1048576 } }`)? Start flat;
  add ops if needed.
- **Multi-extends**: single `extends` keeps merge semantics
  trivial. If users want layered profiles
  (`base + audit + tenant`), do they want explicit precedence or
  list ordering? Defer until requested.
- **Policy versioning at runtime**: `boru ctl` (the supervisor
  control plane) could grow `policy reload` for the exec service.
  Mid-flight requests keep their existing engine; new requests
  pick up the new policy. Possible but not in scope here.

---

## See also

- [FILE-ACCESS.10](FILE-ACCESS.10.md) — the FileOps capability
  abstraction that this generalises.
- [NATIVE-MODULES.10](NATIVE-MODULES.10.md) — module system the
  `boru:vm` module slots into.
- [IMPORTS.10](IMPORTS.10.md) — module resolution path the policy
  hooks into.
- [MODULE-SECURITY.0](MODULE-SECURITY.0.md) — the transitive-dependency
  attenuation RFC; §3.4 and §5–§6 carry the analysis and fix plan for the
  [child-module policy gap](#known-gap-child-module-registries-do-not-inherit-the-policy).
- `lang/go/CLAUDE.md` § "Helper API discipline" — the
  capability-slot convention.
