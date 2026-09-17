# STREAM-WORDS

Design for `boru:stream` — the streaming & concurrency module.

## Context

boru today has eager `list` and one-shot async primitives (`await`,
`await-any`, `await-first`, `await-full` in
`lang/go/native/native_temporal_await.go`), but no first-class story for:

- **lazy / unbounded data** (line-by-line file reading, paginated APIs,
  long-running event sources),
- **back-pressured pipelines** between concurrent producers and consumers,
- **structured fan-out / fan-in** with bounded parallelism.

Without these, idiomatic shell-style pipelines ("read this huge file,
transform each row, hit an API in parallel, write the survivors") are
either impossible or force everything into memory.

This design adds a typed-pipe model — PowerShell / Nushell heritage, with
bash kept only as a *reference* for ergonomics — landing as a single
importable native module `boru:stream`. All new words, types, and
runtime support are scoped under that module so the global namespace
stays clean and users opt in with `import "boru:stream"`.

The intended outcome: a boru programmer can write

```boru
import "boru:stream"

"./events.log" stream.from-lines
    [ stream.parse-json ] stream.map
    [ . status "error" eq ] stream.filter
    8 [ enrich-from-api ] stream.pmap
    "./errors.jsonl" stream.to-lines
```

and get a bounded-memory, fail-fast, back-pressured pipeline with 8-way
parallel enrichment.

## Scope decisions

1. **v1 surface:** `select`/`race`, `pmap N`, `with-timeout`, an explicit
   `Channel` type *in addition to* `Stream`, plus the lazy-list / Job
   core.
2. **Pipefail semantics:** fail-fast. Any stage error aborts the whole
   pipeline and propagates the first error to the caller; downstream
   stages see a cancellation signal and must release resources.
3. **External processes:** out-of-scope for implementation; a short
   "future extension" section appears below.
4. **Framing:** typed pipes carrying boru `Value`s end-to-end (no string
   serialisation between stages). Bash pipelines are the familiar
   mental model; PowerShell / Nushell are the actual reference for
   typed elements and pluggable framing.
5. **Module:** every new word, type, and helper lives under
   `boru:stream`. Nothing is added to the global / `core` namespace.

## Core model

### Types

Three new value types, registered alongside the existing ones in
`lang/go/native/native_type.go`:

- `Stream<T>` — lazy, single-consumer, possibly unbounded sequence of
  `T`. Cold by default; iteration drives production. Closed exactly
  once; carries an optional terminal error.
- `Channel<T>` — hot, multi-producer / multi-consumer rendezvous with a
  fixed buffer. Distinct from `Stream` because semantics differ
  (`send` can block; closing is explicit; multiple readers fan out by
  steal, not by broadcast).
- `Job<T>` — handle to a running task that will eventually yield a `T`
  or an error. Already implicitly present in the await machinery; we
  promote it to a named type so it can flow through `select`/`race`.

All three are reference values with deterministic finalisers (see
"Lifecycle & cancellation" below).

### Words

All exported under `boru:stream`. Names below are the dotted form a
user sees after `import "boru:stream"`.

**Sources**

> The `Bytes` element type used by `from-bytes`/`to-bytes` below is the
> first-class binary leaf specified in `design/go-modules/BYTES.10.md` (core,
> immutable, zero-copy). No surface change here — these rows now carry the real
> `Bytes` type rather than a placeholder.

| Word | Stack effect |
|---|---|
| `stream.from-list`    | `List<T>          -- Stream<T>`      |
| `stream.from-lines`   | `String           -- Stream<String>` *(file path)* |
| `stream.from-bytes`   | `String           -- Stream<Bytes>`  *(chunked file)* |
| `stream.from-channel` | `Channel<T>       -- Stream<T>`      |
| `stream.repeat`       | `T Integer        -- Stream<T>`      |
| `stream.iota`         | `Integer Integer  -- Stream<Integer>` |
| `stream.empty`        | `-- Stream<T>`                       |
| `stream.once`         | `T                -- Stream<T>`      |

**Transforms** (lazy; all return a new `Stream`)

| Word | Stack effect |
|---|---|
| `stream.map`          | `Stream<T> Block<T->U>      -- Stream<U>`       |
| `stream.filter`       | `Stream<T> Block<T->Bool>   -- Stream<T>`       |
| `stream.flat-map`     | `Stream<T> Block<T->List<U>> -- Stream<U>`      |
| `stream.take`         | `Stream<T> Integer          -- Stream<T>`       |
| `stream.drop`         | `Stream<T> Integer          -- Stream<T>`       |
| `stream.take-while`   | `Stream<T> Block<T->Bool>   -- Stream<T>`       |
| `stream.drop-while`   | `Stream<T> Block<T->Bool>   -- Stream<T>`       |
| `stream.chunks-of`    | `Stream<T> Integer          -- Stream<List<T>>` |
| `stream.with-timeout` | `Stream<T> Duration         -- Stream<T>`       |

`stream.with-timeout` imposes a per-element deadline; on miss the
stream terminates with a `TimeoutError`.

**Concurrent transforms**

| Word | Stack effect |
|---|---|
| `stream.pmap`           | `Stream<T> Integer Block<T->U> -- Stream<U>` |
| `stream.pmap-unordered` | `Stream<T> Integer Block<T->U> -- Stream<U>` |
| `stream.merge`          | `List<Stream<T>>               -- Stream<T>` |
| `stream.partition`      | `Stream<T> Integer             -- List<Stream<T>>` |

`pmap` preserves input order; `pmap-unordered` yields results as they
finish.

**Sinks / terminators** (drive the pipeline)

| Word | Stack effect |
|---|---|
| `stream.to-list`  | `Stream<T>                      -- List<T>` |
| `stream.to-lines` | `Stream<String> String          -- Void`   *(path)* |
| `stream.to-bytes` | `Stream<Bytes> String           -- Void`   *(path)* |
| `stream.for-each` | `Stream<T> Block<T->Void>       -- Void`   |
| `stream.fold`     | `Stream<T> U Block<U T -> U>    -- U`      |
| `stream.count`    | `Stream<T>                      -- Integer` |

**Channels**

| Word | Stack effect |
|---|---|
| `stream.channel`  | `Integer           -- Channel<T>` *(buffer; 0 = rendezvous)* |
| `stream.send`     | `Channel<T> T      -- Void`       |
| `stream.recv`     | `Channel<T>        -- T`          |
| `stream.close`    | `Channel<T>        -- Void`       |
| `stream.try-send` | `Channel<T> T      -- Boolean`    |
| `stream.try-recv` | `Channel<T>        -- T?`         |

**Jobs / racing**

| Word | Stack effect |
|---|---|
| `stream.spawn`  | `Block<-- T>      -- Job<T>` |
| `stream.join`   | `Job<T>           -- T`      |
| `stream.select` | `List<Job<T>>     -- T`      |
| `stream.race`   | alias for `stream.select`    |

`select` returns the first job to complete and cancels the rest.

The existing global `await`, `await-any`, `await-first`, `await-full`
words stay where they are, and `boru:stream` re-exports them so a user
who has imported the module finds a complete async toolkit in one
place.

## Common patterns

Side-by-side with the bash equivalent. Bash is shown as the reference
mental model; the boru versions are typed end-to-end and back-pressured.

Every boru snippet assumes `import "boru:stream"` at the top. Words
that appear inside filter / map blocks come from elsewhere in boru and
are shown here for context: `contains` (string search,
`lang/go/native/native_string.go`), `eq` (equality,
`lang/go/native/native_compare.go`), and `.` (record / list field
access, alias for `get`; see `lang/doc/design/legacy/SAMPLES.10.ignore`).
Duration literals like `30s` do **not** exist — durations are built
via `boru:time` constructors (`30 seconds`).

### 1. Count error lines in a log

```bash
grep ERROR ./app.log | wc -l
```

```boru
"./app.log" stream.from-lines
    [ "ERROR" contains ] stream.filter
    stream.count
```

### 2. Bounded parallel HTTP fetches

```bash
cat urls.txt | xargs -P 8 -I{} curl -sf {} -o /dev/null
```

```boru
"./urls.txt" stream.from-lines
    8 [ http.get ] stream.pmap
    stream.to-list
```

The `8` is the concurrency cap; order is preserved. Use
`stream.pmap-unordered` to yield results as they finish.

### 3. First responder wins (race across regions)

```bash
# bash has no clean idiom — you background each, `wait -n` for the
# first to exit, then have to identify the winner yourself and kill
# the rest.
( fetch us-east > /tmp/a ) &
( fetch eu-west > /tmp/b ) &
( fetch ap-south > /tmp/c ) &
wait -n
kill %1 %2 %3 2>/dev/null
```

```boru
[ [ "us-east"  fetch-region ] stream.spawn
  [ "eu-west"  fetch-region ] stream.spawn
  [ "ap-south" fetch-region ] stream.spawn
] stream.race
```

`stream.race` returns the winner's value and cancels the losers'
tokens — the in-flight HTTP requests abort instead of leaking.

### 4. Tail a log with a deadline

```bash
timeout 30s tail -f /var/log/events | grep DEPLOY
```

```boru
import "boru:time"   # for `seconds`

"/var/log/events" stream.from-lines
    30 seconds stream.with-timeout
    [ "DEPLOY" contains ] stream.filter
    [ println ] stream.for-each
```

`with-timeout` is per-element: if no new line arrives within 30s, the
stream terminates with a `TimeoutError` and the file handle is
released. Duration values are constructed by `boru:time` — there is no
`30s` shorthand.

### 5. Batch a JSONL stream into bulk inserts

```bash
# Awkward in bash: split into files, then loop. No streaming bulk-insert.
split -l 100 events.jsonl batch_
for f in batch_*; do bulk-insert < "$f"; done
rm batch_*
```

```boru
"./events.jsonl" stream.from-lines
    [ parse-json ] stream.map
    100 stream.chunks-of
    [ bulk-insert ] stream.for-each
```

No temp files, constant memory, and `bulk-insert` sees a real
`List<Record>` rather than re-parsing text.

### 6. Merge multiple log sources, filter, write out

```bash
# tail -f on multiple files prefixes filenames and interleaves by
# kernel scheduling; ordering is best-effort, error handling
# nonexistent.
tail -f a.log b.log | grep ERROR > errors.log
```

```boru
[ "./a.log" stream.from-lines
  "./b.log" stream.from-lines
] stream.merge
    [ "ERROR" contains ] stream.filter
    "./errors.log" stream.to-lines
```

If either source errors, fail-fast cancels both and the partial
output file is closed cleanly.

### 7. Enrich + filter + persist (the full pipeline)

The motivating example from the top of the doc, shown against its
bash equivalent for completeness:

```bash
# Approximation — true line-by-line typed enrichment with 8-way
# parallelism requires GNU parallel and careful quoting:
cat events.log \
  | jq -c '.' \
  | parallel -j 8 --keep-order 'enrich-from-api {}' \
  | jq -c 'select(.status == "error")' \
  > errors.jsonl
```

```boru
"./events.log" stream.from-lines
    [ parse-json ] stream.map
    [ . status "error" eq ] stream.filter
    8 [ enrich-from-api ] stream.pmap
    "./errors.jsonl" stream.to-lines
```

## Lifecycle & cancellation

- Every `Stream`, `Channel`, and `Job` is born attached to a
  *cancellation token*. Entering a pipeline creates a token rooted at
  the current invocation.
- A terminal error in any stage cancels the token; upstream producers
  see the cancellation on their next yield and return; downstream
  consumers see a terminal-error `Stream`.
- `with-timeout` and `select` install scoped child tokens.
- Native words honour cancellation by checking the token at I/O
  boundaries (line read, channel send/recv, sleep). Pure transforms
  (`map`, `filter`) check between elements.
- Resources (file handles, OS pipes) close deterministically when the
  `Stream` is fully consumed, when the token cancels, or when the
  value is finalised by the GC — whichever comes first.

## Pipefail (fail-fast) behaviour

- Errors propagate **downstream as a terminal `Stream` error** and
  **upstream as a cancellation**.
- The first error wins; later errors are recorded in a debug trace
  but do not overwrite the primary cause.
- The terminator (`to-list`, `for-each`, `fold`, …) re-raises the
  primary error to the boru caller.
- A future `stream.try` combinator (not in v1) will let users opt into
  per-element error capture.

## Module layout (Go side)

New files mirror the `boru:time` pattern in `lang/go/modules/`:

- `lang/go/modules/stream.go` — `BuildStreamModule` plus the exports
  table. Mirrors `BuildTimeModule` in `lang/go/modules/time.go`.
- `lang/go/modules/stream_runtime.go` — concrete `Stream`, `Channel`,
  `Job` implementations: goroutines, bounded channels, cancellation
  tokens. Internal to the module.
- `lang/go/modules/stream_words.go` — native-word adapters that move
  boru `Value`s in and out of the runtime.
- `lang/go/modules/stream_test.go` — unit tests (see Verification).

Registration in `lang/go/modules/modules.go`:

```go
var modules = map[string]func(parent *native.Registry) (native.ModuleDesc, error){
    "math":      BuildMathModule,
    "time":      BuildTimeModule,
    "matrix":    BuildMatrixModule,
    "decision":  BuildDecisionModule,
    "solardemo": BuildSolarDemoModule,
    "stream":    BuildStreamModule,   // NEW
}
```

New type registrations in `lang/go/native/native_type.go` (`TStream`,
`TChannel`, `TJob`). The existing await machinery in
`lang/go/native/native_temporal_await.go` is reused for `Job`; the
stream module wraps but does not duplicate it.

## External processes (sketch only — NOT in v1)

A future `stream.exec` would return a record:

```boru
{ stdin:  Channel<Bytes>
, stdout: Stream<Bytes>
, stderr: Stream<Bytes>
, wait:   Job<ExitStatus>
}
```

letting boru pipelines splice in shell commands the same way Nushell
does. Cancellation kills the child; `wait` joins it. The v1 types
above are deliberately shaped to accommodate this, but no code lands
in v1.

## Verification

End-to-end checks before declaring v1 done:

1. **Unit tests** in `lang/go/modules/stream_test.go`:
   - lazy semantics (a `repeat` source piped through `take 5` reads
     exactly 5 elements);
   - back-pressure (a slow consumer of a channel pauses the producer);
   - fail-fast (an erroring `map` cancels the upstream source);
   - `pmap N` respects the concurrency bound and preserves order;
   - `select` cancels the losing jobs;
   - `with-timeout` fires under a stalled source.

2. **boru-level integration test** under `lang/go/test/` (mirroring the
   existing `pipe_barrier_test.go` style) that runs the worked
   example from the Context section against a fixture file and
   checks the output.

3. **Smoke run** via `lang/go/Makefile`'s standard test target
   (`go test ./...`) plus a one-liner REPL session driving each new
   word once.

4. **Doc check:** confirm the new words show up in `boru:stream` help
   output and that `lang/doc/design/NATIVE-MODULES.10.md` is updated
   to list the new module.

5. **No global-namespace leakage:** a `boru` REPL without
   `import "boru:stream"` must not resolve `stream.map` — verified by
   a negative test.
