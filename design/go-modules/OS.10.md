# `os` → `boru:os` (`Os`)

> **Status: design proposal — not implemented.** This note specifies the
> curated boru surface for Go's `os` package. No Go code exists yet; the
> note exists so the proposed surface — and especially its **policy
> gating** — is auditable before any handler is written. Read
> [`README.10.md`](README.10.md) first for the shared conventions this
> note assumes.

## 1. Package & status

Go [`os`](https://pkg.go.dev/os) is the process's view of its host:
environment variables, command-line arguments, identity (hostname, pid),
the working directory, well-known directories (home, temp), and process
termination. `boru:os` curates the **process/environment** slice of that
surface. It is the first genuinely *side-effecting, host-fingerprinting*
module in the curated family, so it is gated with care.

**All filesystem functionality is OUT of scope** — including the
filesystem-location getters. `os.Open`, `os.ReadFile`, `os.Create`,
`os.Mkdir`, `os.Remove`, `os.Stat`, the `*os.File` handle, **and the
location getters `os.Getwd` (cwd), `os.UserHomeDir` (home dir), and
`os.TempDir` (temp dir)** all belong to `boru:io` (the `IO` namespace,
backed by the `FileOps` capability — see [`IO.10.md`](IO.10.md),
`lang/go/native/io_module.go`, and
[`../FILE-ACCESS.10.md`](../FILE-ACCESS.10.md)). `boru:os` exposes only the
**environment / process / identity** words below; anything that names or
resolves a filesystem path is `boru:io`.

## 2. Why curated

The raw `go:` reflection bridge would expose `os.LookupEnv` as
`(string, bool)`, `os.Getwd` / `os.Hostname` / `os.UserHomeDir` as
`(string, error)`, and `os.Exit(int)` as a bare void call — all with
`Any` boundaries and Go names. The curated module:

- collapses `LookupEnv`'s `(value, ok)` to **value-or-None** so a missing
  variable is `None` (testable with `is None`) rather than an
  out-of-band boolean;
- collapses the `(value, error)` return (`hostname`) to
  **value-or-error** (`r.BoruError`);
- renames to kebab idiom (`Getenv`→`getenv`, `Getpid`→`pid`);
- shapes `Environ` (a `[]string` of `"K=V"`) into a **Map**, and `Args`
  into a **List[String]**;
- and, most importantly, **routes every effect through a host capability
  seam** (§7) so a sandbox can deny, fake, or scope it — instead of the
  bridge's direct, ungated `os.*` calls.

## 3. Import & namespace

```
import "boru:os"          # binds the Os namespace
```

Namespace is the plain capitalized package name, **`Os`** — no `-util`
suffix (the `-util` convention is only for pure helper libraries and for
avoiding builtin-type clashes; `Os` clashes with nothing). Words are
reached args-before-dot: `"PATH" Os.getenv`.

## 4. API

Signatures are **top-first, sig order** (position 0 = top of stack).
`→` separates args from result. Every word's inner native uses
`BarrierPos: -1` so the infix form dispatches (zero-arg words note their
`BarrierPos: 0` in §4 notes); see `README.10.md` "Argument order &
dispatch".

| Go symbol | boru word | signature (top-first) | one-line doc | boru-ish refinement |
|---|---|---|---|---|
| `os.Getenv` / `os.LookupEnv` | `getenv` | `String → String\|None` | Value of an environment variable, or None if unset. | **Merges** `Getenv` and `LookupEnv`: refines `LookupEnv`'s `(value, ok)` to value-or-**None** so "unset" is a first-class None — distinct from the empty string `Getenv` returns for both. |
| `os.Setenv` | `setenv` | `name:String value:String →` (nothing) | Set an environment variable for this process. | `(error)` → value-or-error; returns nothing on success. **Mutating + off by default** (§7). |
| `os.Unsetenv` | `unsetenv` | `String →` (nothing) | Remove an environment variable. | `(error)` → value-or-error. **Mutating + off by default** (§7). |
| `os.Environ` | `environ` | `→ Map` | All environment variables as a name→value Map. | `[]string` of `"K=V"` → **Map[String,String]** (split on the first `=`), idiomatic and key-addressable. |
| `os.Args` | `args` | `→ List[String]` | The process command-line arguments. | `[]string` → **List[String]**. Element 0 is the program name, as in Go. |
| `os.Hostname` | `hostname` | `→ String` | The host's network name. | `(value, error)` → value-or-error. |
| `os.Getpid` | `pid` | `→ Integer` | This process's id. | `int` → Integer. |
| `os.Exit` | `exit` | `Integer → ` (never returns) | Terminate the host process with the given status code. | void → never-returns. **Dangerous** — terminates the whole host; off by default (§7). |

The filesystem-location getters `cwd` (`os.Getwd`), `home-dir`
(`os.UserHomeDir`), and `temp-dir` (`os.TempDir`) live in `boru:io`, not
here — see [`IO.10.md`](IO.10.md).

Zero-arg words (`environ`, `args`, `hostname`, `pid`) read nothing from
the stack; their inner native sigs declare `BarrierPos: 0`
(constant-style, like `math.pi` in `math.go`). The
arg-taking words (`getenv`, `setenv`, `unsetenv`, `exit`) declare
`BarrierPos: -1` so both the stack form `"K" Os.getenv` and the
forward `Os.getenv "K"`-style dispatch resolve.

### Refinement notes

- **`getenv` value-or-None.** Go forces a choice between `Getenv`
  (empty string for both "unset" and "set-to-empty") and `LookupEnv`
  (the `ok` disambiguates). boru exposes **one** word that returns the
  string when set and `None` when unset, so `"X" Os.getenv is None`
  is the unset test and a present-but-empty var returns `""`.
- **`environ` as a Map.** Go's `[]string{"K=V", …}` is split on the
  first `=` into an `*OrderedMap`. A line with no `=` (rare, malformed)
  maps the whole string to `""`.
- **`exit` is a tail effect.** It returns no boru value because the
  process is gone; document it as "never returns". In a sandbox it MUST
  be denied or stubbed (§7) — a guest must never be able to kill the
  host.

## 5. Types

Scalars (`String`, `Integer`, `None`), `List[String]` (`args`), and
`Map` (`environ`) only — all map through `core.FromNative` /
`core.ToNative` (`core/go/gobridge.go`). **No opaque external handle** is
introduced: `boru:os` exposes no `*os.File`, `*os.Process`, or `os.FileInfo`
(those would belong to `boru:io`). No `FixedID` from the `10000+`
host/third-party range is needed, and no entry in
`lang/go/test/fixedid_stability_test.go`.

## 6. Errors

`r.BoruError(code, detail, word)` with kebab-case codes; a Go `error`
return is unwrapped into the `BoruError`. Codes:

| code | raised by | when |
|---|---|---|
| `os-setenv-failed` | `setenv` | `os.Setenv` returns an error (e.g. invalid name). |
| `os-unsetenv-failed` | `unsetenv` | `os.Unsetenv` returns an error. |
| `os-hostname-failed` | `hostname` | `os.Hostname` returns an error. |
| `os-denied` | any gated word | the host policy denies the operation (the wrapper returns the `*policy.Denied` verbatim; surface its `Code`). |
| `os-not-installed` | any gated word | the env/process capability scope is `install:false` — the seam stub returns a `capability_not_installed`-style error, never a nil deref. |

Bad-type args are guarded with `AsConcreteString` / `AsConcreteInteger`
**before** any host call (no panics — `core/go/CLAUDE.md` "Panic
Prevention"). Every positive `.tsv` row pairs with an `ERROR:<substring>`
sibling (`lang/go/CLAUDE.md` "Test discipline").

## 7. Policy / capabilities (CRITICAL)

`boru:os` is the policy-heavy half of this family. **None of these words
may call `os.*` directly from the handler.** Every effect routes through
a **host-provided capability seam** mirroring `FileOps`
(`lang/go/capabilities/capabilities.go`) and its wiring
(`lang/go/native/capabilities.go`, `permissioned_fileops.go`,
`notinstalled_fileops.go`). Concretely the analogous seam:

- a `capabilities.HostEnv` / `capabilities.HostProc` interface in
  `lang/go/capabilities/` (`Getenv`, `LookupEnv`, `Setenv`, `Unsetenv`,
  `Environ`; `Args`, `Hostname`, `Pid`, `Exit` — the location getters
  `Getwd`/`UserHomeDir`/`TempDir` live on the `FileOps` seam used by
  `boru:io`, not here) with an **OS-backed default** (delegates to `os.*`,
  the `OSFileOps` analogue) and an **in-memory / fake** implementation
  for specs and sandboxes (the `MemFileOps` analogue — fixed hostname,
  scripted env map, no real `Exit`);
- installed via `SetHostEnv` / `SetHostProc` and retrieved via
  `HostEnv(r)` / `HostProc(r)` (the `SetHostFileOps` / `HostFileOps`
  pattern). When the relevant scope is `install:false`, the slot is
  removed and the accessor returns a `notInstalled*` stub whose methods
  return `capability_not_installed`, so handlers need no nil guard;
- **auto-wrapped** by a `permissionedEnv` / `permissionedProc` gate
  (the `permissionedFileOps` analogue) when `HostPolicy(r)` is non-nil,
  so each call first consults the policy and only then delegates.

The gate consults `lang/go/policy` exactly as `permissionedFileOps`
does (`policy.Check(scope, op, args)`, which checks the bound globals
from `policy.GlobalsFor` first). Real scope/cap names from
`policy.KnownScopes` / `policy.GlobalOps`:

| word(s) | scope (`KnownScopes`) | global cap (`GlobalOps`, via `GlobalsFor`) | default posture |
|---|---|---|---|
| `getenv`, `environ` | `env` | `env` | on if `env` installed (read) |
| `setenv`, `unsetenv` | `env` | `env` | **off by default** (mutating) |
| `args` | `process` | `process` | on if `process` installed |
| `pid` | `process` | `process` + `system-info` | on if installed |
| `hostname` | `process` | `system-info` | on if installed (host fingerprint) |
| `exit` | `process` | `process` | **off by default** (dangerous) |

Notes for the gate:

- **`env` scope.** `getenv`/`environ` gate as **reads**; `setenv`/
  `unsetenv` gate as **mutations** and SHOULD be `deny` by default in
  any sandbox profile (a guest mutating the host's process environment
  is rarely intended). `policy.GlobalsFor` should bind `env` for both;
  the read/write distinction is carried in the `op` (`get`/`environ`
  vs `set`/`unset`) and the predicate args (`{"name": …}`) so a profile
  can allow reads while denying writes, just as fileops allows `read`
  while denying `write`.
- **`process` scope + `system-info`.** `args`/`pid` identify the
  process; `hostname` additionally leaks host fingerprint, so it binds
  the `system-info` global cap (shared with `boru:runtime`). A
  determinism-seeking sandbox denies/stubs them via the fake
  `HostEnv`/`HostProc`.
- **`exit` is the dangerous one.** It terminates the **host** process,
  not the boru engine, so it MUST gate on `process` and be **off by
  default** in every sandbox profile. The recommended default is
  `deny`; a host that genuinely wants guest-driven exit installs an
  explicit allow rule. The fake implementation never calls
  `os.Exit` — it records the code (or returns `os-denied`) so specs
  stay in-process.
- **No policy installed ⇒ allow-everything.** `HostPolicy(r) == nil`
  means "no permissions configured", the default opt-in posture
  (`capabilities.go` `HostPolicy` doc) — the OS-backed defaults run
  ungated, matching today's fileops behaviour. Sandboxes opt *in* to
  restriction by installing a policy.

## 8. Overlap

- **`boru:io` (`IO`).** Owns **all** filesystem functionality — content
  I/O, tree mutation, listing, `stat`, the existence/type predicates,
  and the filesystem-location getters `cwd`/`home-dir`/`temp-dir` (see
  [`IO.10.md`](IO.10.md)) — via the `FileOps` capability. `boru:os` owns
  only environment/process/identity and never names or resolves a
  filesystem path. Dividing line: anything filesystem (incl. a path
  getter) is `boru:io`; a process/environment attribute is `boru:os`.
- **`boru:runtime` (`Runtime`).** Shares the `system-info` cap and the
  "host fingerprint / determinism" concern, but reports the **Go
  runtime** (GOOS, NumCPU, …) rather than the OS process. No word
  overlap.
- **`boru:filepath` / `boru:path-util`.** Pure path-string manipulation;
  `boru:io` produces the location strings (`cwd`/`home-dir`/`temp-dir`)
  those modules then manipulate. `boru:os` produces none. No word overlap.

## 9. Examples (args-before form)

```
import "boru:os"

# read a var, defaulting when unset (value-or-None)
"EDITOR" Os.getenv (default "vi")              # → "vi" when EDITOR unset

# explicit unset test
"PATH" Os.getenv is None                       # → false on a normal host

# all vars as a Map, then pull one key
Os.environ get "HOME"                          # → "/home/user"

# process identity
Os.pid                                          # → 4242 (Integer)
Os.hostname                                      # → "workstation" (or os-hostname-failed)

# args list — element 0 is the program name
Os.args len                                     # → arg count incl. program

# mutating + dangerous words (off by default in a sandbox)
"FEATURE_X" "on" Os.setenv                       # name value → ; or os-denied
1 Os.exit                                        # never returns; or os-denied
```

## 10. Open questions / out of scope

- **Out of scope:** all filesystem functionality — file ops
  (`Open`/`ReadFile`/`Create`/`Stat`/…) **and the location getters**
  (`Getwd`/`UserHomeDir`/`TempDir`) — → `boru:io` ([`IO.10.md`](IO.10.md));
  `os/exec` process spawning (a separate, even more
  dangerous capability — `boru:exec` with its own `process`
  gate, now designed: [EXEC](EXEC.10.md)); `os.Getenv`-style `os.Expand`;
  signals (`os/signal`); user/group lookup (`os/user`).
- **Open:** should `setenv`/`unsetenv` exist at all, given how rarely a
  query-language guest should mutate the host environment? Proposal:
  ship them but gate them off by default; a host opts in. Revisit if no
  real use-case appears.
- **Open:** should `exit` be omitted entirely rather than shipped-and-
  denied? Shipping it keeps the surface complete and the denial
  explicit/auditable; omitting it is safer-by-construction. Leaning
  toward ship-and-deny so the policy story is uniform.
- **Open:** `Args`/`Environ` snapshot-vs-live — these are read once per
  call from the capability; a fake can return a fixed snapshot for
  reproducible specs.

## 11. Implementation sketch (wiring checklist — no code)

Follow `io.go` (the **capability-backed** reference), not `math.go`
(pure):

1. **Seam.** Add `HostEnv`/`HostProc` interfaces + OS-backed and
   in-memory impls to `lang/go/capabilities/capabilities.go` (mirror
   `FileOps`/`OSFileOps`/`MemFileOps`).
2. **Wiring.** Add `CapEnv`/`CapProc` keys, `SetHostEnv`/`HostEnv`
   (and proc) accessors, `permissionedEnv`/`permissionedProc` gates,
   and `notInstalledEnv`/`notInstalledProc` stubs in
   `lang/go/native/` (mirror `capabilities.go`,
   `permissioned_fileops.go`, `notinstalled_fileops.go`).
3. **Policy bindings.** Extend `policy.GlobalsFor` so `env`→`env`,
   `process`→`process`/`system-info` per §7. `env`/`process` are
   already in `KnownScopes`; `system-info` is already in `GlobalOps`.
4. **Module.** `BuildOsModule(parent) (native.ModuleDesc, error)` in
   `lang/go/modules/os.go`: fresh `subReg := native.DefaultRegistry()`,
   register the inner `OsNatives` (each handler calls `HostEnv(r)` /
   `HostProc(r)`, **never** `os.*`), wrap each as an `FnDef` into an
   `*OrderedMap` keyed by word, export under `"Os"`,
   `ID: parent.Modules.NextID()`. Inner sigs: `BarrierPos: -1` for
   arg-taking words, `BarrierPos: 0` for the zero-arg readers.
5. Register `BuildOsModule` in the `modules` map in
   `lang/go/modules/modules.go`.
6. **Docs.** `lang/go/modules/docs_os.go` →
   `registerDocs("boru:os", {…})` with a one-line summary per export
   (`TestModuleExportDocs` enforces).
7. **Spec.** `lang/spec/module-os.tsv` — rows lead with
   `import "boru:os"`; every positive row gets an `ERROR:<substring>`
   sibling (esp. `os-denied` rows exercised under a deny policy, and a
   `Mem`-backed deterministic clock/env so `getenv`/`hostname` specs
   are reproducible). `describe` surfaces the module live via
   `stampExportProvenance` — no static help wiring.

## 12. Vault-migration additions (VAULT-TUI-PORT §7.4)

> The "vault logic in boru" phase ([VAULT-TUI-PORT.0](../VAULT-TUI-PORT.0.md)
> §7.4) names two `Os.*` needs — a single-variable env read (**N1**) and a
> clipboard copy (**G2**). This section is the spec for those two, added
> here rather than in a fourth module so the OS surface stays in one place.

### 12.1 `Os.env` is the existing `getenv`

VAULT-TUI-PORT §7.4 writes `Os.env <name> → String|None`. That is
**exactly** `Os.getenv` (§4) — value-or-None env read, `env` scope, `env`
global. There is no second word: **`getenv` is the canonical spelling**
(it merges `Getenv`+`LookupEnv` and matches the `setenv`/`unsetenv`
family). The vault's env read (N1) calls `Os.getenv`; treat "`Os.env`" in
the port RFC as an informal alias for it. If a shorter reader alias is
ever wanted, it should be a documented alias of `getenv`, not a divergent
word.

### 12.2 `clipboard-copy` — a seam-backed effect

| Go mechanism | boru word | signature (top-first) | one-line doc |
|---|---|---|---|
| platform clipboard CLI (see below) | `clipboard-copy` | `String → ` (nothing) | Place text on the host clipboard. |

Clipboard is **not** an environment/process-identity attribute, and its
implementation is a **process spawn** (the Go vault shells out to
`pbcopy` / `wl-copy` / `xclip` / `xsel` / PowerShell `Set-Clipboard`,
piping the text on **stdin, never argv** —
`cmd/go/internal/vault/clipboard.go`). OS.10 deliberately pushes
`os/exec` spawning out to [`boru:exec`](EXEC.10.md), so `clipboard-copy`
does **not** call `os/exec` from its handler either. Instead it routes
through a **host clipboard seam**, exactly like every other effect here:

- **`lang/go` defines ONLY the interface** — a `capabilities.HostClipboard`
  (`Copy(text string) error`, plus a `Paste`/`Clear` pair reserved for a
  future read word) and an **in-memory fake** for specs (records the last
  copied string; no spawn). `lang/go` **must not** import
  `cmd/go/internal/vault/clipboard.go` — Go's internal-visibility rule
  forbids it, and an `os/exec`-backed default in `lang/go` would
  reintroduce the very process-spawning dependency this seam exists to
  keep out of the language layer (the same reason `FileOps` keeps its OS
  backing in the host). So the exec default lives **host-side**;
- the **exec-backed implementation is provided by `cmd/go`** at
  registration time — `cmd/go` adapts the vault's platform detection
  (`clipboard.go`) to `HostClipboard` and calls `SetHostClipboard` (the
  `FileOps` / `OSFileOps` split: interface in `lang/go`, OS impl injected
  by the command). With nothing registered the slot is empty and the word
  raises `capability_not_installed`;
- retrieved via `HostClipboard(r)`, removed to a `notInstalled` stub under
  `install:false`, and **auto-wrapped** by the `permissioned*` gate when a
  policy is present.

**Policy.** Because the effect spawns a helper process, `clipboard-copy`
binds the **`process`** global — the same choice the Go vault makes for
its clipboard op (`policy.GlobalsFor` maps the vault `clipboard` op →
`process`; VAULT-TUI-PORT §5). Add a `GlobalsFor` row `(os,
clipboard-copy) → process`; scope stays `process` in `KnownScopes` (no
new scope needed). A determinism/no-spawn sandbox denies it via the fake
backend.

| word | scope | global cap | default posture |
|---|---|---|---|
| `clipboard-copy` | `process` | `process` | off by default in a no-spawn sandbox |

**Errors.** `os-clipboard-failed` when the host backend errors (no
clipboard tool on `PATH`, or the tool exits non-zero) — a single error,
matching the vault's `copy not supported` / non-zero-exit handling. `Copy`
of empty text is a valid clear, not an error.

**Robustness note (child-registry leak).** A dispatch-time `os-denied`
gate would become allow-all inside an imported module body (see
[PERMISSIONS.10 → Known gap](../PERMISSIONS.10.md#known-gap-child-module-registries-do-not-inherit-the-policy)),
which is where the vault-in-boru clipboard call runs. The
`permissioned*`-wrapper form recommended in §7 (policy baked into the
capability object, inherited by value) is leak-resistant and is the
required mechanism for `clipboard-copy`.

**Overlap.** `boru:vault`'s own `clipboard` op (the recipe/secret copy)
delegates to `Os.clipboard-copy` after the migration rather than
re-implementing the platform exec; [`boru:exec`](EXEC.10.md) owns
general process spawning, of which this is a single, curated, host-sealed
instance.
