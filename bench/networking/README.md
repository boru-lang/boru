# Networking performance test — boru vs. Go, TypeScript, Python, Ruby

A like-for-like **TCP echo round-trip** microbenchmark comparing the
`boru:net` module against equivalent hand-written Go, Node/TypeScript, Python,
and Ruby. Every implementation is checked in so the test is repeatable.

## What it measures

One persistent TCP connection carrying a newline-framed request/response
protocol (`"hello world\n"` echoed back). Each runtime performs **20,000
timed round-trips** plus one warm-up exchange, then reports throughput
(`req/s`) and mean per-round-trip latency. Round-trip (ping-pong) latency is
the fairest cross-language networking microbenchmark: it isolates the
per-request cost of the send/receive path rather than raw bulk throughput.

Every implementation is server + client in one process over loopback TCP with
`TCP_NODELAY` on both ends — Go's default (which the `boru:net` sockets inherit,
being `net.Conn` values) and set explicitly in the Node / Python / Ruby ports
to match. Each uses its own port so runs don't collide.

The boru version deliberately uses the same primitive words the networking
examples in `design/examples/apps/` are built on: `Net.serve-raw`,
`Net.connect-raw`, `Net.send-bytes`, `Net.recv-until`.

## Files

| File | Runtime |
|------|---------|
| `echo_boru.boru`  | boru (`boru:net`) — server + client in one process |
| `echo_go.go`    | Plain Go (`net` + `bufio`) |
| `echo_ts.ts`    | Plain TypeScript (Node `net`) |
| `echo_py.py`    | Plain Python (`socket` + `threading`) |
| `echo_rb.rb`    | Plain Ruby (`socket` + `Thread`) |
| `loop_only.boru` | boru no-network control: same per-iteration work, no sockets, to isolate interpreter-loop overhead from socket-word cost |

## Running

```bash
# from the repo root (build the CLI once: cd cmd/go && make build)
cmd/go/bin/boru run -install network bench/networking/echo_boru.boru   # bytecode compiler (default)
cmd/go/bin/boru run -no-compile -install network bench/networking/echo_boru.boru
go run  bench/networking/echo_go.go
npx ts-node bench/networking/echo_ts.ts    # or: bun run bench/networking/echo_ts.ts
python3 bench/networking/echo_py.py
ruby    bench/networking/echo_rb.rb

cmd/go/bin/boru run bench/networking/loop_only.boru   # control (no network capability needed)
```

Take the best of ~3 runs per runtime (loopback microbenchmarks are noisy).

`bench/networking/` is intentionally outside every Go module tree, so it is
never touched by `make fmt/vet/lint/test/cover-gate`.

## Results

Best of 3 runs each (Go 1.24.7, Node 22 / bun 1.3, Python 3.11, Ruby 3.3,
loopback Linux, single box; re-measured 2026-07-10 after the
runtime-stamping work — `design/legacy/RUNTIME-STAMPING.0.ignore`):

| Runtime | req/s | µs / round-trip | vs. Go |
|---|--:|--:|--:|
| Go (`net` + `bufio`)                    | ~112,000 | ~8.9  | 1× (baseline) |
| TypeScript (`bun`, Node `net`)          | ~71,900 | ~13.9  | ~1.6× slower |
| **boru — bytecode compiler (default)**   | **~65,100** | **~15.4** | **~1.7× slower** |
| Python (`socket`)                       | ~15,300 | ~65    | ~7.3× slower |
| Ruby (`socket`)                         | ~15,200 | ~66    | ~7.4× slower |
| boru — interpreter (`-no-compile`)       | ~884    | ~1,131 | ~127× slower |

The **compiled** boru server runs its per-connection handler on the bytecode VM
and lands within ~1.7× of Go — **within ~10% of `bun`, and ~4× ahead of Python
and Ruby**. Only hand-written Go is clearly faster. That is a **~74× jump over
boru's own tree-walking interpreter** (~884 → ~65,100 req/s), the payoff of
compiling the `serve-raw` handler body to a unit (see *Resolution* below). The
handler compiles whether its connection param is typed `Socket` or `Any` (an
earlier revision required `Socket`; two compiler fixes removed that — see
below).

The compiled-vs-interpreted multiplier and the boru-vs-Go ratio are the durable
findings; absolute req/s track the box (an earlier same-day run on a busier
box measured Go ~82k / boru ~47k — the same ~1.75× ratio).

An earlier revision of this file reported ~940 req/s for the "compiler
(default)" row: at that time function-body compilation did not exist, so the
handler ran interpreted regardless of the compile flag. That row is superseded
by the callback-compilation work.

### The other net examples (app-level, boru-only)

Echo is a deliberately minimal microbenchmark — its handler body is exactly the
compilable subset (`for`, `def`, `convert`, `join`, socket words), which is why
it captures the full compiled-vs-interpreted swing. The realistic `boru:net`
examples in `design/examples/apps/` are heavier and boru-only (no other-language
equivalents), so they are measured **compiled vs. interpreted**, not
cross-language. Drivers live in `apps/`; run each from the repo root:

```bash
cmd/go/bin/boru run -install network bench/networking/apps/echo_redis.boru
cmd/go/bin/boru run -no-compile -install network bench/networking/apps/echo_redis.boru
# …and echo_s3.boru / echo_todo.boru
```

(All three drivers pass the default pre-flight check and run without
`-no-check`. An earlier checker false-positive on `echo_redis.boru` was traced to
mini-redis's connection parameter being typed `Any`; it is now typed `Service`
(its real type), which resolves the check — see
`design/legacy/CHECK-FALSE-POSITIVES.0.ignore`.)

| App | tier | interpreted | compiled | speedup |
|---|---|--:|--:|--:|
| `todo-api`   | service + `Net.http` codec + JSON | ~1,660 req/s | ~17,700 req/s | **~10.7×** |
| `mini-redis` | `Net.listen` service + RESP codec | ~840 req/s | ~10,500 req/s | **~12.5×** |
| `mini-s3`    | `serve-raw` + streaming HTTP      | ~205 req/s | ~211 req/s | **~1.0×** |

(Re-measured 2026-07-10 after the runtime-stamping work — see below;
`todo-api` 5,000 GETs, `mini-redis` 10,000 ops, `mini-s3` 6,000 ops;
medians, `-force-compile` ≡ default for all three. Absolute numbers track
the box; the per-app speedup pattern — order-of-magnitude for the
service-tier apps, ~1× for the I/O-bound `serve-raw` app — is the durable
finding.)

**mini-redis and todo-api now run their whole request path on the VM.** An
earlier revision of this table showed mini-redis at ~1.5×: its handlers
passed the compile PROBE, but at runtime the custom boru codec, most
handlers, the nested helpers, and the per-iteration client `MiniRedis.cmd`
apply all still executed on the interpreter (a pprof of the compiled run
put ~97% of callback CPU in `Registry.CallBoru`; `-force-compile` cannot
surface this — stored-handler refusals are probed in a throwaway state and
decline silently). The runtime-stamping work
(`design/legacy/RUNTIME-STAMPING.0.ignore`) closed the gap end to end:

- **detached fn-unit stamping** at the codec-resolution, service-store, and
  module-load sites compiles runtime-constructed callbacks the whole-program
  pass can never see, with invoke-time dep-freshness (a rebound module dep
  falls back to live interpreter resolution);
- **gradual-Any nesting** fixed the class that blocked the `kv-read`-calling
  handlers ("unmatched dispatch recovered at dot" — a nested callee's `Any`
  param was generalised strictly where the handler's own params are gradual);
- **filter-lambda closures** now admit lexical captures and computed
  collections, so the KEYS handler compiles (this also benefits every
  filter/each/fold/scan lambda user);
- the **module-export apply seam** runs stamped module fns on the VM, so the
  client loop compiles too.

After all four: a pprof of the compiled mini-redis run shows **zero
`CallBoru` samples on the steady-state path** (verify with
`go tool pprof -peek 'CallBoru$'` over a profiled run). A follow-on fix then
closed the last interpreted callback — the unknown-command catch-all, whose
whole body is a computed map literal (previously "body result of unknown
provenance"): a callback body's trailing computed map/list now records its
`OpMakeMap`/`OpMakeList` assembly, so **every** mini-redis callback compiles
(and `redis-serve`'s whole body compiles as one unit). todo-api picked up
~34% on top of its previous ~13k from the same work (its handlers are now on
the VM, not just its native `Net.http` codec).

**mini-s3 stays ~1.0×**, as before: it is I/O-bound (streaming HTTP over
`serve-raw`), and its per-connection actor uses `do [ … ] error [ … ]` —
both `CompileFallbackBody`, deliberate interpreter islands. Remaining
blockers are tracked in `design/legacy/NET-COMPILE-FRONTIER.0.ignore`.

### Where boru's time goes (the interpreter path)

This analysis was done on the **interpreted** handler (the `-no-compile` /
pre-compilation ~620 req/s row), and is why compiling the handler mattered.
The `loop_only.boru` control does the same per-iteration `Bytes` build +
`convert`/`join` work with no sockets: **~68 ms for 20,000 iterations
(~3.4 µs/iter)**. The interpreted round-trip cost ~1,065 µs/iter, so **~99.7%
of it was in the networking word layer per call** — socket handle lookup +
signature matching, the per-`recv` read-deadline syscall, the read mutex, and
the forked-registry output syncing on the server goroutine — not the loop body.
Compiling the handler body to a VM unit removes the interpreter's per-word
dispatch overhead and is what lifts boru to ~18 µs/RT (~55,000 req/s).

### Caveats

- Latency-bound (ping-pong). A pipelined/bulk-throughput test would narrow the
  gaps somewhat but not change the ordering.
- Best-of-3 on a single loopback box; treat the figures as order-of-magnitude,
  not precise. Compiled boru (~2× Go, ahead of Node, ~4× ahead of Python/Ruby)
  vs the interpreter (~176× Go) is the headline the numbers support.

## Root-cause analysis

The initial guess (per-call socket syscalls) was wrong. Localizing the cost by
crossing boru against Go on each end (`diag/`) shows the overhead is almost
entirely **server-side**, and is an interpreter-vs-compiler artifact, not a
network artifact:

| Experiment | req/s | µs / RT |
|---|--:|--:|
| Go server + Go client (baseline)            | ~118,000 | ~8.5  |
| **boru client** + Go server                  | ~20,800  | ~48   |
| **boru server** + Go client (full handler)   | ~870     | ~1,150 |
| boru server, *minimal* handler + Go client   | ~11,000  | ~91   |
| Go server with **Nagle ON** + Go client     | ~21,600  | ~46   |

Findings, each isolated:

1. **The boru client path is fine** (~48 µs/RT, ~5–6× Go). The timed client loop
   runs at the top level, which the bytecode compiler *does* compile.
2. **The bottleneck is the `serve-raw` connection handler.** Swapping only which
   end is boru moves the cost by ~24×.
3. **Not Nagle / TCP options.** A Go echo server with `SetNoDelay(false)` stays
   fast — loopback ACKs arrive before Nagle can coalesce single-frame writes.
4. **The handler body runs on the tree-walking interpreter, not the bytecode
   compiler.** The identical `convert`/`join`/`def` loop costs **76 ms compiled
   at top level, 1423 ms under `-no-compile`, and 1457 ms inside a `serve-raw`
   fork** (`loop_only.boru` vs `diag/fork_compute.boru`). The in-fork figure
   matches the interpreter figure, not the compiled one — a ~19× per-word
   penalty paid on every request.

The mechanism is in `eng/go/registry.go`: `Registry.CallBoru` evaluates every
function body via `sub := NewTop(r); sub.Run(tokens)` — the interpreter. The
bytecode compiler applies to the top-level program only, so any boru function
body (and therefore every per-connection `serve-raw` handler) is interpreted.
`-force-compile` at the top level does not change this.

**In one line:** boru's networking isn't slow because of the network — it is slow
because server request handlers are boru *function bodies*, and function bodies
currently execute on the interpreter rather than the compiler, at ~19× the
per-word cost. Compiling fn bodies / handler loops would be the highest-leverage
fix; the socket words themselves are not the constraint.

## Resolution — the handler now compiles (~97× faster)

The highest-leverage fix above **landed**: the callback-compilation seam
(`design/legacy/CALLBACK-COMPILATION.0.ignore`) compiles a `serve-raw` connection handler
body to its own bytecode unit and runs it on the VM (`RunUnit`) per connection,
instead of `Registry.CallBoru` on the interpreter. The echo handler body — `for`,
`def`, `convert`, `join`, and the `Net.recv-until` / `Net.send-bytes` socket
words over the connection — compiles in full, and this benchmark now measures
**~58,000 req/s compiled vs ~600 interpreted** (the table above).

**Param typing (no longer required).** The socket words are module overloads
(`Net.recv-until : [Socket Bytes]` / `[Socket Bytes Map]`). An explicitly-`Any`
handler param now binds a **gradual** carrier, so the socket-word dispatch
resolves optimistically and the whole handler compiles — `[sock:Any]` and
`[sock:Socket]` both reach the VM (the benchmark uses `Socket` only for clarity).
Historically `[sock:Any]` was a hard blocker: the strict `Any` receiver could not
pick an overload, `def line (Net.recv-until sock nl)` was misread as *redefining*
a word `line` with the locked builtin signature `[Socket Bytes Map]`, and that
`locked_signature` error refused whole-program compilation. Two compiler fixes
removed it — see *Compiler fixes* below.

This corrected an earlier hypothesis that the handler was blocked by a deep
"module fn-value dispatch over a dynamic receiver" carrier-inference wall. It is
not: module dispatch over a `Socket` compiles fine (`Net.close sock`,
`Net.recv-until sock (convert Bytes "\n")` both compile); the only blocker was
the strict modelling of the `Any` param.

### Compiler fixes

1. **Gradual `Any` handler params (root).** The stored-handler compile path
   (`fnValueInputs`, `eng/go/emit.go`) built strict `Any` param carriers; it now
   uses `ParamInputCarrier`, so an `Any` param is gradual exactly as on the
   ordinary user-fn compile path. A body word over it poly-matches at runtime
   instead of failing `no_signature` against the strict `Any` top.
2. **Failed dispatch is not a word extension (defense-in-depth).** A value whose
   producing call genuinely fails to dispatch is left as a `FailedDispatch`
   Function carrying the native's locked signatures; `defWordExtension`
   (`lang/go/native/native_definition.go`) now declines to treat it as an
   open-words merge, so a def-bound failed dispatch inside a loop reports the real
   dispatch diagnostic instead of a spurious `locked_signature`.

Both landed gate-green (`verify-bytecode` differential + `-race`, `cover-gate`
100%).

### `diag/` — the localization harness

`go_server.go` / `go_client.go` (split echo), `go_server_nonagle.go`
(Nagle-on control), `boru_server.boru` / `boru_client.boru` (split echo),
`boru_server_min.boru` (minimal handler body), and `fork_compute.boru` (times the
pure compute loop *inside* a handler fork). Each takes a port argument or a
fixed port noted in the file; start a server in the background, then run the
matching client.
