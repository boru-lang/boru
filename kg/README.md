# kg/ — the project knowledge graph

This directory holds two things:

1. **A reusable, evidence-backed knowledge-graph pipeline written in
   boru** — ingest candidate facts, normalize entities, resolve identity,
   validate every claim, preserve provenance, and query the result with
   bounded traversals.
2. **The knowledge graph of this repository itself** — built by that
   pipeline and committed at [`out/graph.json`](out/graph.json), with a
   human/agent-readable projection at [`out/graph.md`](out/graph.md).

The project graph is built from **two bundles, and code wins on code
facts**:

| Bundle | Source | Supplies |
|--------|--------|----------|
| **Code** — [`gomod.boru`](gomod.boru) | `go.work` + every `go.mod` + a one-level package walk | the module inventory, the package inventory, and the **directed `depends_on` edges**, quoting the actual `use` and `require` lines as evidence |
| **Prose** — [`project/boru-project.jsonic`](project/boru-project.jsonic) | README.md, AGENTS.md, CLI.md, design notes | what only documentation knows: which guide covers which module, the language concepts, the shipped artifacts |

Prose is a poor source for structure, in two specific ways this design
answers. It is **undirected** — the README's "core/go … eng builds on
it" became a `related_to` edge, which cannot distinguish "eng depends on
core" from the reverse, so a plain question like "what does `lang/go`
build on?" had to be answered by reading `go.mod` anyway. And it
**states intent as fact** — a design note reading "the parser re-points
to core" looks exactly like a note saying it already has, and ingesting
the first as the second asserts something the code contradicts
(the parser, then a package inside the engine module, still imported
it). `go.mod` has neither problem.

**Keep it fresh.** The code half refreshes itself on every `make graph`.
When a PR changes the *documentation* set, update
`project/boru-project.jsonic` and rebuild (`make graph` here, or
`cd kg && ../cmd/go/bin/boru main.boru`). The build is deterministic, so
an unchanged input produces a byte-identical bundle and a clean diff.

**Check it without rebuilding: `make verify`.** The bundle records a
content digest of every input file, so the verifier can tell you whether
the committed graph still matches the working tree — and name the files
that moved. It exits non-zero when stale, so it can gate a commit.

## Quick start

Everything runs **from this directory** (boru imports resolve against the
working directory):

```bash
make -C ../cmd/go build     # once: build the boru binary
cd kg
make check                  # boru check every module and test
make test                   # run the whole test suite
make graph                  # rebuild out/graph.{json,sql,md}
make verify                 # is the COMMITTED graph still true of the tree?
```

**Just want to read it? Read [`out/graph.md`](out/graph.md)** — the same
graph as a few hundred lines of outline, module dependency view first.
It exists because `out/graph.json` is ~115 KB of fingerprints and nested
evidence, which is a poor trade against simply reading AGENTS.md's
layout table; `graph.md` is about a tenth of that and answers more.

Query the committed graph from boru:

```boru
import "boru:io"
import "./queries.boru"
def g (IO.read (make Pathon "out/graph.json"))

KgQuery.modules g                              # every Go module
KgQuery.packages g                             # every discovered package
KgQuery.code-unit-by-path g "eng/go/parser"    # none => not a code unit
KgQuery.dependencies-of g "<module id>"        # what it builds on
KgQuery.dependents-of g "<module id>"          # what breaks if it changes

KgQuery.entities-by-type g "Document"
KgQuery.neighbors g "<entity id>"
KgQuery.two-hop-paths g "<id a>" "<id b>"
```

## Architecture

```
go.work + go.mod + package walk        prose candidate bundles
        │  gomod.boru                          │  (JSON / JSONic / CSV /
        │  modules, packages, depends_on       │   TSV rows / text-derived)
        └───────────────┬──────────────────────┘
                        ▼   candidate bundles — CODE FIRST, so go.mod wins
        │  ingest.boru      candidates -> typed records, deterministic ids
        ▼
entities + assertions + sources        (schema.boru: typed Records; every
        │                               object built via checked mk-*)
        │  resolve.boru     evidence-based identity decisions + safe merges
        │  assertions.boru  conflict detection -> disputed, never dropped
        ▼
draft bundle                           digest.boru — input_digest over every
        │  validate.boru    rules -> KgIssue records      file the build read
        │  report.boru      competency report, review queue, summary
        ▼
boru-kg/1 bundle  ──►  storage.boru  ──►  out/graph.json   the machine contract
                                          out/graph.sql    normalized relational
                                          out/graph.md     the READ path
                                          queries.boru — bounded graph queries
                                          verify.boru  — digest vs the tree
```

| File | Responsibility |
|------|----------------|
| `schema.boru` | Closed vocabularies, typed `Record` shapes, checked constructors |
| `identifiers.boru` | Deterministic FNV-1a ids, canonicalization, collision detection |
| `normalize.boru` | Label normalization (NFC, trim, whitespace collapse, casefold) |
| `ingest.boru` | Candidate bundles → graph objects; CSV/TSV and bundle re-ingest adapters |
| `entities.boru` | Entity indexing, reversible merges, merge-chain resolution |
| `assertions.boru` | Assertion construction (evidence required), conflict marking |
| `resolve.boru` | Identity-resolution policy and automatic-merge rules |
| `validate.boru` | Every bundle-level validation rule, reported as issues |
| `queries.boru` | The query API — lookups, evidence, bounded 1/2-hop traversal, code-unit/dependency queries, review views |
| `storage.boru` | JSON bundle write/read, round-trip check, normalized SQL emission, the Markdown projection |
| `report.boru` | Pipeline orchestration (`build-graph`), competency report, summary |
| `gomod.boru` | The CODE bundle: reads `go.work` + every `go.mod`, walks packages, emits `depends_on` |
| `digest.boru` | Content digests over the input files — the freshness signal `generated_at` cannot be |
| `main.boru` | Builds the project graph from the code bundle + `project/boru-project.jsonic` |
| `verify.boru` | Checks the committed bundle's digest against the working tree; exits non-zero when stale |
| `util.boru` | Shared helpers (`get-or`, `list-at`, `starts-with`, `as-map`) |

## The bundle (output contract)

`out/graph.json` follows `schema_version: "boru-kg/1"`:

```
{ schema_version, generated_at, input_digest{},
  sources[], entities[], assertions[], identity_decisions[],
  validation_issues[], schema_proposals[], competency_results[],
  human_review_queue[], summary{} }
```

- **Source** `{id kind locator title retrieved_at content_hash authority metadata}`
- **Entity** `{id type label normalized_label aliases[] external_ids{} attributes{} status}`
  — status ∈ accepted | candidate | merged | rejected
- **Assertion** `{id subject_id predicate object evidence[] confidence
  status valid_from valid_to recorded_at rule}` — the object is exactly
  one of `{kind:"entity" entity_id}` or `{kind:"literal" value datatype
  unit language}`
- **Evidence** `{source_id locator quote extraction_method extractor}`
- **Identity decision** `{id left_entity_id right_entity_id decision
  supporting_evidence[] conflicting_evidence[] confidence review_required}`
- **Validation issue** `{id severity rule target_kind target_id message
  suggested_correction automatic_correction_safe}`
- **Schema proposal** — new vocabulary terms are **never added
  silently**; they ship as proposals with `status: "requires_approval"`.
- **Input digest** `{algorithm file_count combined files[{path digest
  chars}]}` — see *Freshness* below.

Approved entity types: Person, Organization, Place, Event, Document,
Product, Concept, Role, Identifier, **SoftwareModule**, Other. Approved
predicates: type, same_as, part_of, located_in, member_of, works_for,
owns, created_by, participated_in, has_role, occurred_at, mentions,
supports, contradicts, supersedes, related_to, **depends_on**,
has_attribute.

`SoftwareModule` and `depends_on` were approved on 2026-08-07.
`SoftwareModule` discharges the standing proposal that repository
modules "are currently typed Product, which blurs shipped artifacts and
source-tree modules"; it is reserved for units the code bundle can
evidence from `go.work`/`go.mod`, and each carries
`attributes.unit` ∈ `go-module` | `go-package`. Shipped artifacts (the
`boru` binary), plain directories (`lang/spec`) and boru-level modules
stay `Product`. `depends_on` is **directed** — subject depends on
object — and only DIRECT `require` entries produce one: an
`// indirect` line records what the module graph drags in, not what a
module was written against.

## Identifiers

All local ids are **deterministic** (64-bit FNV-1a over a canonical
string; no random ids anywhere):

```
src:<fp(locator|content_hash)>
ent:<Type>:<fp(Type|normalized_label|key)>
ast:<fp(subject|predicate|canonical object|valid_from|valid_to|sorted source ids)>
identity:<fp(left|right)>          issue:<fp(rule|target|message)>
```

The candidate **key** joins the entity fingerprint so two same-named
records stay *distinct* entities at construction — identity is decided
by resolution evidence, never by an id coincidence. A hash match alone
is never trusted: `KgId.collisions` reports any id claimed by two
different canonical inputs as a `kg_id_collision` error. Labels
normalize as NFC → trim → collapse whitespace → lowercase (comparison
form); the display label is preserved separately. Authoritative external
identifiers live in `external_ids` and drive identity resolution.

## Identity resolution

Names alone **never** merge entities:

| Evidence | Decision | Auto-merge? |
|----------|----------|-------------|
| Matching external id (same scheme+value, no conflicting scheme) | same_entity 0.98 | yes |
| Same type+label and ≥2 corroborating attributes, no conflicts | same_entity 0.95 | yes |
| Explicit `same_as` assertion, no corroboration | same_entity (assertion confidence) | **no — review** |
| Same type+label only | possible_match 0.60 | **no — review** |
| Shared scheme, conflicting values | different_entity (or insufficient_evidence if others match) | no |

Merges are **non-destructive and reversible**: the loser keeps its
record with `status: "merged"` and `attributes.merged_into` pointing at
the canonical entity; its label and aliases fold into the winner's
aliases and its external ids union in. Queries chase merge chains
through a **bounded** resolver (8 hops max).

## Claims, provenance, conflicts

- Every assertion **must** carry ≥1 evidence record; the constructor
  refuses evidence-free claims and ingest reports them as
  `kg_no_provenance` errors.
- Model/agent-proposed candidates pass the same checks; **model output
  is never evidence** — evidence must point at the original source, and
  model-extracted evidence without a quote is flagged for review.
- Conflicting claims on a single-valued predicate (type, located_in,
  works_for, occurred_at) are **preserved** and marked `disputed` —
  nothing is overwritten or discarded; the pair lands in the review
  queue. Claims whose objects canonicalize identically share one id
  (deduplication, not conflict).
- `inferred` assertions must record their rule id.

## Validation

`validate.boru` reports (never hides, never auto-repairs) issues for:
duplicate/missing ids everywhere; unknown source kinds/authorities;
unknown entity types/statuses; empty labels; merged-entity pointer
integrity (existing target, no self-merge); unknown predicates;
malformed objects (must be exactly entity-ref XOR literal); dangling
subjects/objects/sources; unknown datatypes; missing provenance;
confidence outside [0,1]; inverted validity intervals; rule-less
inferred assertions; lone disputed assertions; illegal or unsupported
merges (threshold 0.95 + evidence, possible_match never merged);
non-`requires_approval` proposals; fingerprint collisions; missing
quotes on text/model evidence (warnings); and
`kg_module_without_code_evidence`.

That last rule guards the one coupling between the two bundles. A module
or package must be declared in *both* — in `gomod.boru` so the facts are
code-derived, and in `project/boru-project.jsonic` so prose assertions
can reference it by key — and the two declarations unify into one entity
only when key, type and normalized label fingerprint alike. Get the
label wrong by a word and nothing errors: you simply get two entities,
one holding the `go.mod` truth and one holding the prose, each looking
complete on its own. The rule fails the build when a `SoftwareModule`
carries no assertion evidenced by a code source, which catches both that
drift and a module deleted from `go.work` whose prose declaration
lingers.

## Queries

All pure functions over the bundle in `queries.boru`: `entity-by-id`,
`entities-by-type`, `entities-by-label` (alias-aware, normalized),
`assertions-for-subject`, `assertions-for-object`,
`assertions-by-predicate`, `assertions-by-source`,
`assertions-in-range`, `evidence-for-assertion`, `edge-list`,
`neighbors`, `one-hop-paths`, `two-hop-paths`, `code-units`, `modules`,
`packages`, `code-unit-by-path`, `unit-of`, `dependencies-of`,
`dependents-of`, `unresolved-identities`, `conflicting-assertions`,
`validation-errors`, `human-review-items`.

`dependencies-of` and `dependents-of` are genuinely different answers —
that is the point of `depends_on` replacing `related_to`, which could
only say the two were somehow connected. `code-unit-by-path` answering
`none` is itself the answer to "is this path a module?".

Traversal depth is **bounded by construction** —
one and two hops only; there is deliberately no recursive walker.
(`boru:query`'s SQL pipeline resolves FROM-tables from the context
store — a Go-registration fit — so the query layer uses plain
`filter`/`fold`, which also reads clearer here.)

## Testing

```bash
cd kg && make test
```

`tests/` covers: the `go.work`/`go.mod` line scanners against inline
fixture text (block and single-line `require`, `// indirect`, a
`replace (` block that must not be read as requires) plus the real
workspace end to end; input digests and every staleness verdict
(unchanged / edited / added / removed); the code-unit and dependency
queries, including that direction is not symmetric; the Markdown
projection; the `kg_module_without_code_evidence` rule on a synthetic
orphan; constructor accept/reject pairs, vocabulary membership,
deterministic ids (including evidence-order independence), collision
handling, normalization (with property tests: idempotence,
id-as-function-of-input), every resolution decision (safe merge,
rejected name-only merge, corroborated merge, conflicting ids, same_as
without corroboration, bounded chain chasing), every planted defect in
`fixtures/invalid.jsonic`, the full query API over the fixture graph,
JSON round-trips, file write/read-back, build determinism, re-ingest id
stability, and SQL emission (determinism + escaping). Fixtures:
`valid.jsonic`, `conflicts.jsonic`, `ambiguous-identities.jsonic`,
`invalid.jsonic`.

The parser tests deliberately use inline fixture text rather than the
repository's own `go.mod` files, so a legitimate dependency change
cannot turn a *parser* test red; the end-to-end assertions check only
invariants that hold whatever the workspace contains (core depends on no
sibling; the compiler lane's modules are no longer members).

## SQLite

REFERENCE.md documents `sqlite-open`/`sqlite-exec`/`sqlite-query` behind
a `sqlite` capability, but the current engine build does not register
those words (`boru describe sqlite-open` → no description). Until it
does, `storage.boru` emits the graph as **normalized SQL**
(`out/graph.sql` — bundle_meta, input_files, sources, entities,
entity_aliases, entity_external_ids, entity_attributes, assertions,
assertion_evidence, identity_decisions, validation_issues,
schema_proposals; one transaction, never an opaque JSON blob). The
`bundle_meta`/`input_files` tables carry the input digest, so a SQL
reader can check a loaded graph against the tree exactly as
`verify.boru` does against the JSON:

```bash
sqlite3 graph.db ".read out/graph.sql"
```

## Freshness

`generated_at` and `recorded_at` are pinned to a constant in `main.boru`
(`run-stamp`) so a rebuild over unchanged input is byte-identical. A
pinned stamp is therefore **not** a freshness signal, and it is actively
misleading as one — the stamp read `2026-07-22` at a point when the
graph already contained `check/go` and `compiler/go`, modules that only
came into existence on 2026-08-06.

So the bundle carries `input_digest` instead: a real 64-bit FNV-1a
digest of every file the build read — `go.work`, every member `go.mod`,
the project candidate file, and each document it cites — plus one
combined digest over the sorted `path|digest` lines, so it moves when
any input's content changes *and* when an input is added or removed.

```bash
make -C kg verify     # exits 0 if current, 1 (naming the files) if stale
```

`verify.boru` recomputes the input **set** the same way `main.boru` does
rather than reading it back from the bundle, so a newly cited document
registers as `added` instead of passing unnoticed.

Why the digest does not live on `KgSource.content_hash`, where it would
seem to belong: source ids fingerprint `locator|content_hash`, and
assertion ids fingerprint their evidence source ids, so a real digest
there would re-key every assertion in the bundle on any documentation
edit — unreadable diffs and no stable identifier to link against.
`content_hash` stays a stable version label; freshness is a separate,
non-identifying block.

## Permissions

The pipeline needs only local file reads/writes inside `kg/` (the CLI's
default `fileio` capability). It performs no network access, no process
execution, and stores no credentials anywhere. Model-assisted extraction,
if you use it, happens *outside* this pipeline and enters as candidate
bundles subject to the full evidence checks.

## Known limitations

- ~~**`boru fmt` cannot be run on these sources.**~~ Fixed 2026-07-29.
  These sources were hand-formatted for two reasons, both now resolved:
  the formatter corrupted template interpolations (`` `hi ${name}` ``
  re-parsed the `${expr}` hole as a map literal), and formatting the tree
  took ~10 minutes. The formatter now scans backtick literals verbatim,
  and `emitNode` is memoised — `validate.boru` went from 53s to 40ms, the
  whole kg tree formats in under a second. `make fmt` is live and part of
  `make all`. Every file still passes `boru check` (`make check`).
- Native SQLite words are unavailable in the current build (see above) —
  `out/graph.sql` is the relational path; it loads cleanly into stock
  `sqlite3` with zero foreign-key violations.
- Pair generation in resolution is a bounded O(n²) sweep — fine at this
  repo's scale; add blocking (e.g. by normalized-label prefix) before
  pointing it at very large entity sets.
- `text`-kind sources are ingested as *candidate assertion bundles*
  (extraction happens outside boru — by an agent, a rule, or a model);
  boru validates evidence and quotes but does not itself do NLP.
- Package discovery is **one level deep** by design — it names the units
  people ask about (`lang/go/native`, `cmd/go/internal`), not the whole
  tree. A nested package is not in the graph.
- Only `go.work` members are discovered automatically. A real module
  outside the workspace must be listed in `main.boru`'s `extra-modules`
  (currently `editors/tree-sitter/bindings/go`); it is ingested with
  `workspace_member: false` so the distinction stays visible.
