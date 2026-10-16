# TS engine parity for the new Micron leaves — DEFERRED

**Status:** deferred (tracked). **Owner:** unassigned.

The Go kernel gained three Scalar/Micron changes that the TypeScript engine
(`eng/ts/`) does not yet mirror:

| Change | PR | TS state |
|---|---|---|
| `Ipon` — IPv4/IPv6 address leaf (FixedID 114) | #237 | absent |
| `Pathon` Windows **drive** parsing (`C:/x` → `volume`) | #239 | absent |
| `Hoston` — host:port authority leaf (FixedID 115) | #240 | absent |
| `Semveron` — SemVer 2.0.0 leaf (FixedID 116), **custom precedence order** | #244 | absent |
| `Cidron` — CIDR block (FixedID 117), **custom numeric order** (net/netip) | (this PR) | absent |
| `Macon` — MAC-48/EUI-64 leaf (FixedID 118) | (this PR) | absent |
| `Coloron` — sRGB colour (FixedID 119), **custom (r,g,b,a) order** | (this PR) | absent |
| `Mimon` — media type (FixedID 120), canonical params | (this PR) | absent |
| `Qion` — money (FixedID 121), apd Decimal + ISO 4217, **custom order** | (this PR) | absent |
| `Phonon` — E.164 phone (FixedID 122) | (this PR) | absent |

`eng/ts/src/type.ts::typeNameTable()` still declares only `TMicron`, `TPathon`,
`TEmailon`, `TUrlon`.

## Why this is not blocking CI

The cross-engine differential (`test/go/engspec`) fails only on a **divergence**
— both engines succeed but return different values. It permits a **gap** — one
engine errors where the other succeeds.

- `Ipon` / `Hoston` / `Semveron` / `Cidron` / `Macon` / `Coloron` / `Mimon` /
  `Qion` / `Phonon` are **new** types: `make <Leaf> …` succeeds in Go and raises
  `undefined_word` in TS → a permitted gap, not a divergence. The four with a
  **custom Comparer** (Semveron, Cidron, Coloron, Qion) must port that ordering,
  not just the constructor, to close the gap correctly.
- The `Pathon` drive change was deliberately made **POSIX-preserving** (only a
  `C:` drive prefix switches on Windows parsing; `//a//b//` stays `/a/b`), so no
  existing corpus input diverges.

So CI is green, but TS users cannot construct the new scalars and a Windows
drive path is not understood by the TS engine.

## To close the gap

1. Add `Ipon` and `Hoston` to `eng/ts/src/type.ts` (type table + FixedIDs
   114/115, matching `core/go/typetable.go`).
2. Port the constructors/renderers/properties to the TS micron implementation,
   matching Go byte-for-byte:
   - `Ipon`: `net.ParseIP`-equivalent validation + canonical form; `addr`,
     `version` (4/6).
   - `Hoston`: `SplitHostPort` bracketing + optional port; `host`, `port`,
     derived `authority`; reject URL delimiters in the host.
   - `Pathon`: drive-prefix parsing + `volume`; keep driveless paths POSIX.
   - `Semveron`: SemVer 2.0.0 parse (no leading `v`, no leading zeros, numeric
     prerelease unbounded); `major`/`minor`/`patch`/`prerelease`/`build` +
     derived `prereleaseParts`/`buildParts`/`release`/`stable`/`version`; and
     the **custom SemVer precedence Comparer** (mirror `compareSemverons`) —
     lexical render order is semantically wrong here, so the TS port must carry
     the same precedence logic, not just the constructor.
3. Add cross-engine differential rows for each so parity is enforced, not
   assumed.

Until then, keep new kernel scalars **POSIX/behaviour-preserving for existing
inputs** so they can only ever produce permitted gaps, never divergences.
