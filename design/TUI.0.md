# TUI

Design for the **`boru:tui` module** — the layer where a developer writes a
terminal user interface in boru: full-screen apps (dashboards, browsers,
editors-of-things) that run on a local terminal **or** are served from a
process and viewed from a thin remote client.

The design delivers **two tiers** over the **same actor substrate** that the
networking stack rides (`PROCESSES.0.md`):

- a **low-level raw-terminal API** — an opaque `Terminal` handle, styled
  cell-addressed drawing, and decoded input events, for the tightest control
  and for building new widget kinds; and
- a **high-level app API** — *the app is an actor*: input events arrive as
  mailbox messages, an `update` function folds them into state, a **pure
  `view` function renders state to a declarative widget tree (plain data)**,
  and the runtime lays out, diffs, and paints.

Because a view's output is ordinary sendable data (maps/lists/scalars — the
`PROCESSES.0.md` §6 sendable class), the **remote story is a codec, not a
subsystem**: `Tui.serve` ships widget trees down and events up over the
`boru:net` json-lines machinery, and `boru attach` renders them with the same
layout engine. Local and remote are the same app, differing only in where
frames are painted.

This is a **design RFC only — no implementation code yet**, matching how the
adjacent subsystems were designed first (`PROCESSES.0.md`, `SERVICES.0.md`,
`NETWORK-SERVERS.0.md`, `STREAM-WORDS.0.md`).

> **Decisions proposed at design time** (the forks this RFC closes; a
> reviewer ratifies or reopens them):
> (1) the app loop is a **native driver bound to a real `eng.Process`
> mailbox** — not a loop written in boru, and not a synthetic queue (§2.2);
> (2) **full-screen apps first** — inline prompt/progress widgets are a later
> tier on the same runtime (§9);
> (3) the remote wire carries **widget trees, not cell diffs and not ANSI**
> (§6.1);
> (4) layout is **Ratatui-style size constraints as data**, not
> lipgloss-style content joins (§3.2);
> (5) the thin client is a **new `boru attach` command** — the existing
> `boru tui` supervisor dashboard keeps its name untouched (§6.4);
> (6) v1 remote sessions **quit on disconnect** — reattach/persistence is
> deferred (§6.3);
> (7) the TTY backend is built on **`x/term` + `x/ansi` + `x/cellbuf`
> directly**, not on bubbletea's `Program` (§4.4);
> (8) update handlers are **ordinary boru code** (may do I/O); only `view`
> must be pure — the pragmatic-BEAM shape, not strict-Elm commands-as-data
> (§2.3).

## 0. Review of the existing design — what is settled, what this adds

This RFC is an *extension*, not a rewrite. The actor/service/network family
settles most of the model; this file adds the terminal-specific pieces.

| Concern | Settled by | This RFC |
| --- | --- | --- |
| Lightweight processes, `spawn`/`self`/`send`/`receive`, `Pid`, bounded mailbox, patrun dispatch | `PROCESSES.0.md` §2–4 (shipped as core words) | reused verbatim; **the UI app *is* such a process** — its mailbox is the event queue (§2) |
| Immutable-only, zero-copy messages (`not_sendable`) | `PROCESSES.0.md` §6 | the reason widget trees and events are **plain data** — sendable to the UI and serializable to the wire for free (§3, §6) |
| `service`/`add` patrun handlers, `call`/`send` | `SERVICES.0.md` §1 (shipped) | **deferred** — v1 has a single `update` function; a patrun-clause variant is an open question (§11.1) |
| Listener/codec plumbing, `json-lines`, deadline & error vocabulary (`closed`/`timeout`/`transport`), token auth | `NETWORK-SERVERS.0.md` §4/§6, `NETWORK-CLIENTS.0.md`; shipped in `lang/go/modules/net_socket.go`, `net_codec.go` | reused for `Tui.serve`/`boru attach` (§6); the `Service`/`RemoteDispatch` layer is deliberately **not** used (§6.2) |
| Host-registration seam for pluggable capability backends (Spec + `RegisterHostX` + registry state) | `EXTENSION-MODULES.10.md` §2; shipped as `RegisterHostParser` (`lang/go/modules/parselang.go`) | instantiated as `RegisterHostTui` — the terminal backend is host-injected; `lang/go` stays terminal-dependency-free (§4.3) |
| Module build/registration/docs/spec/coverage house rules | `NATIVE-MODULES.10.md`; ADR-001/003/005/008 | followed throughout; word surface pre-audited against core words (§7.2) |
| Timers (`timeout`/`interval`/`await`) | shipped, `boru:time-util` | reused: a tick is an `interval` whose body `send`s to the UI's pid — **no native tick machinery** (§2.4, §11.2) |
| Streams / channels | `STREAM-WORDS.0.md` (design only) | out of scope; noted for future media/log-follow widgets (§9) |

In one line: **the actor substrate, the wire machinery, and the host-seam
pattern all exist; this document specifies the terminal backend, the widget
tree, the app loop that connects a mailbox to a screen, and the remote
protocol that falls out.**

## 1. The two tiers at a glance

The same job — *draw a counter, react to keys, leave the terminal clean* —
at both tiers, so the trade-off is visible before any detail.

**Low level (you own the cells and the loop):**

```boru
import "boru:tui"

def t ( Tui.open {} )                      # raw mode + alt-screen; Terminal handle
def loop fn [[n:Integer] [Integer] [
  do [
    Tui.clear t
    Tui.print-at t 2 1 (join "" ["count: " (convert String n)]) {bold: true}
    Tui.show t                             # flush the diff to the screen
    def ev ( Tui.read-event t {within: 1000} )   # decoded event map, or None
    case [
      [ev eq None]        [ loop n ]                 # idle tick — redraw
      [ev.key eq "up"]    [ loop (add n 1) ]
      [ev.key eq "down"]  [ loop (sub n 1) ]
      [ev.key eq "q"]     [ n ]                      # fall out of the loop
      [ loop n ] ]
  ]
  error [ Tui.close t  raise ]             # never leave the terminal raw
]]
def final ( loop 0 )
Tui.close t
```

**High level (the runtime owns the loop; you write update + view):**

```boru
import "boru:tui"

Tui.run {
  init: {n: 0}
  update: [ [state ev] => [ case [
      [ev.key eq "up"]    [ state set n (add state.n 1) ]
      [ev.key eq "down"]  [ state set n (sub state.n 1) ]
      [ev.key eq "q"]     [ Tui.quit state ]
      [ state ] ] ] ]                      # any other event: unchanged state
  view: [ [state] => [
      Tui.box (Tui.text (join "" ["count: " (convert String state.n)])
                        {style: {bold: true}})
              {title: "counter"  border: "round"} ] ]
}
```

Both leave the terminal restored on every exit path. The low-level version
hands you a `Terminal` and cell-addressed drawing; the high-level version
hands you events as values and turns your pure view into paint. Tier 2 is
*literally Tier 1 plus a process mailbox, a layout engine, and a diff* — §2.5
shows the reduction. Everything else in this document fills in those two
surfaces and the remote protocol they share.

## 2. The app model (TEA → actors)

### 2.1 Precedents, and what is taken from each

The high-level tier is The Elm Architecture (TEA) mapped onto boru's existing
actor substrate — the combination LiveView proved on the server and Bubble
Tea proved in the terminal.

| Precedent | What `boru:tui` takes | What it rejects |
| --- | --- | --- |
| **Elm / Bubble Tea** (TEA) | the update/view split; runtime owns terminal + loop; declarative view (the direction bubbletea v2 moved) | strict-Elm commands-as-data — boru already has actors for effects (§2.3) |
| **Textual** | a named widget vocabulary; `list-view` naming; watch/computed reactivity noted as future sugar (§9) | class-based widgets — boru widgets are data, not subclasses |
| **Ratatui** | constraint layout as data; frame = pure function of state; cell-buffer diffing | immediate mode's app-owned render loop — the engine must not own a blocking loop it cannot interrupt |
| **Rebol/Red VID** | UI as a block of plain data in the language's own syntax; a future dialect layer via `mini`/`parse` (§9) | inventing the dialect *now* — the data form comes first, sugar later |
| **Phoenix LiveView** | server holds state, events up / renders down, thin client; disconnect semantics | HTML/DOM diffing — trees are small enough to ship whole in v1 (§6.1) |
| **Charm Wish** | serving TUIs to remote viewers as a first-class mode | shipping ANSI bytes — an opaque wire that forecloses client-side layout, resize locality, and non-TTY clients (§6.1) |

### 2.2 The app is a process (decided)

`Tui.run` binds the app to a **real `eng.Process`** from the shipped process
runtime — not a private queue that merely resembles one:

- The UI has a genuine **`Pid`**, registered under the name **`"tui"`**
  (first app wins; a second concurrent `Tui.run` in the same runtime raises
  `already_running`). Any process reaches the UI with plain
  `send msg (whereis "tui")` — **that is the whole subscription story.** No
  new pub/sub machinery: timers, workers, socket handlers, and other actors
  all address the UI the way they address anything else.
- The **driver is native Go** and runs on the *calling* engine's goroutine
  (the call blocks until quit, exactly as a `serve-raw` acceptor owns its
  actor). An **input pump** goroutine reads decoded events from the backend
  and does nothing but `Send` them into the mailbox — the
  `serveRawHandler` acceptor shape from `lang/go/modules/net_socket.go`,
  recovered, with a `//covergate:allow` pragma.
- Loop body: pop the front message → invoke `update` (via
  `eng.InvokeCallback`, the established host-owns-loop pattern) → **drain**:
  keep popping with a zero timeout, folding `update`, until the mailbox is
  empty or 32 messages are consumed — then invoke `view` once, lay out,
  diff, present. Coalescing is why an event storm costs one render, not one
  per keystroke; queued `resize` events collapse to the last.

**Why not write the loop in boru** (the way `boru:repl` is written over
`boru:net`)? Three concrete losses: (a) coalescing needs
inspect-and-drain with zero timeout — `receive` is one-message-one-wakeup,
so a boru loop repaints per keystroke; (b) the raw-mode restore guarantee
must sit in a single Go `defer`/`recover` wrapping *everything* including
layout — a boru loop puts engine frames between a panic and the restore;
(c) the engine has **no mid-`Run` interruption seam**
(`lang/go/modules/debug_step.go`), so the host side must own the loop to
own teardown. The *model* is fully preserved — real pid, real bounded
mailbox, `send`/`whereis` unchanged — only the pump is native.

### 2.3 Effects: pragmatic BEAM (decided)

`update` is **ordinary boru code**. It may read files, call `Net.fetch`, or
`send` to other processes directly — no command vocabulary, no effect
sandbox. The contract is instead about *time*, and it is documented rather
than enforced:

> **The UI cannot paint while `update` runs.** The engine cannot interrupt a
> running evaluation, so a slow `update` freezes the screen. Do slow work in
> a `spawn`ed process and `send` the result back to the UI's pid; handle it
> as just another event.

`view` **must be pure** (state in, widget tree out) — it is re-invoked at
the runtime's discretion (after updates, on resize, on remote re-attach in
later versions), so side effects in `view` run an unpredictable number of
times. Purity of `view` is also what makes the TSV spec strategy work (§8.2)
and the remote tier trivial (§6).

The strict-Elm alternative (pure `update` returning commands-as-data) buys
deterministic replay at the cost of a command vocabulary duplicating what
actors already do; in a language whose processes are the effect system, the
LiveView/GenServer shape is the honest one.

### 2.4 The event vocabulary

Events are **tagged maps** — the same convention as active-mode socket
messages (`NETWORK-SERVERS.0.md` §4.3) — with every discriminating field a
top-level scalar, so they route cleanly through patrun `receive` clauses if
an app ever forwards them:

| Event | Shape |
| --- | --- |
| init | `{tag: "init"  ui: Pid  cols: Integer  rows: Integer}` — always the first event; delivers the UI's own pid (the caller is blocked in `Tui.run`, so `self` cannot be used to learn it) |
| key | `{tag: "key"  key: String  char: String  mods: List}` — `key` is a stable name (`"a"`, `"enter"`, `"esc"`, `"up"`, `"tab"`, `"f5"`, …); `char` is the printable rune or `""`; `mods` ⊆ `["ctrl" "alt" "shift"]` |
| resize | `{tag: "resize"  cols: Integer  rows: Integer}` — coalesced under storm |
| mouse | `{tag: "mouse"  kind: String  x: Integer  y: Integer  mods: List}` — `kind` ∈ `press/release/move/wheel-up/wheel-down`; **opt-in** via `{mouse: true}` (§7.1), so native text selection keeps working by default |
| paste | `{tag: "paste"  text: String}` — bracketed paste arrives whole, not as key events |
| focus | `{tag: "focus"  gained: Boolean}` |
| anything else | a message another process `send`s to the UI's pid, delivered to `update` **verbatim** |

Ticks are deliberately absent: an animation is
`TimeUtil.interval 100 [ send {tag: "tick"} (whereis "tui") ]` — the shipped
timer words already compose with the mailbox (§11.2 records the leaning).

**Mailbox policy.** The input pump enqueues key/mouse events with
drop-oldest semantics under storm (stale input is the correct thing to shed;
a pending resize is *replaced*, never dropped). Messages from other
processes use the normal bounded-`block` default — a slow UI back-pressures
its workers, which is the BEAM-correct outcome
(`PROCESSES.0.md` "bounded mailbox").

### 2.5 Quit, teardown, and the two escape hatches

- **Quit is a value.** `update` returns `Tui.quit state'`, which wraps the
  final state in a marker map keyed with the `@` metadata convention of
  `SERVICES.0.md` (`{@quit: state'}`). The driver recognizes it, tears down,
  and **`Tui.run` returns the final state** to the caller — an app is an
  expression with a result, which is what makes it composable and testable.
- **Teardown order** (all paths): stop the input pump → `Backend.Close()`
  (idempotent, `atomic.Bool` latch — the `net_socket.go` close-latch
  pattern) → process unregister/close. An error raised by `update`/`view`
  tears down **first** (screen restored), then propagates as the `BoruError`.
  A driver-level panic is recovered once, restores the terminal, and
  re-raises as `tui_error internal` — panics never escape (ADR-005).
- **Ctrl-C** is consumed by raw mode, so the driver gives it back: by
  default it is treated as quit-with-current-state (no `update` call). Apps
  that need the key opt out with `{ctrl-c: "deliver"}`, receiving it as
  `{tag:"key" key:"c" mods:["ctrl"]}`. **Ctrl-\ always quits and is never
  deliverable** — the guaranteed exit, mirroring the REPL's
  prompt-only-interrupt philosophy. The host `signal.NotifyContext` path
  (SIGINT/SIGTERM during a TUI) closes the backend before exit, so a killed
  process still restores the terminal.

The Tier-2/Tier-1 reduction, for the record (conceptual — the shipped driver
is native for the §2.2 reasons):

```boru
# Conceptual desugaring of `Tui.run app`:
def t ( Tui.open app.opts )
def ui-loop fn [[state:Map] [Map] [
  def ev ( next-event )                    # mailbox front: input OR worker msg
  def state2 ( app.update state ev )
  case [
    [has state2 @quit] [ Tui.close t  state2.@quit ]
    [ paint t (app.view state2)  ui-loop state2 ] ]   # layout+diff+show
]]
ui-loop app.init
```

## 3. Widget trees as data

### 3.1 The vocabulary (nine widgets)

`view` returns a tree of **plain maps** discriminated by a `w:` field,
produced by pure constructor words. Constructors do nothing but assemble and
default-fill the map — data in, data out — which is what makes every one of
them a one-line TSV spec row (§8.2) and the whole tree wire-ready (§6).

| Constructor (forward form) | Produces |
| --- | --- |
| `Tui.text <s> {opts?}` | `{w:"text"  text:s  style:{}  wrap:"truncate"}` — leaf; `wrap:` ∈ `truncate/wrap` |
| `Tui.rows [c1 c2 …] {opts?}` | `{w:"rows"  children:[…]  gap:0}` — vertical stack |
| `Tui.cols [c1 c2 …] {opts?}` | `{w:"cols"  children:[…]  gap:0}` — horizontal stack |
| `Tui.box <child> {opts?}` | `{w:"box"  child:…  title:""  border:"line"  pad:[0 0]}` — chrome; `border:` ∈ `line/round/double/none` |
| `Tui.list-view <items> {opts?}` | `{w:"list-view"  items:[…]  cursor:0}` — items are strings or `{label:…}` maps; cursor row rendered highlighted, kept in view |
| `Tui.table <columns> <rows> {opts?}` | `{w:"table"  columns:[{title width}…]  rows:[[…]…]  cursor:0}` |
| `Tui.input <value> {opts?}` | `{w:"input"  value:""  cursor:0  placeholder:""  focus:false}` — a **stateless render of state-owned text** (§3.4) |
| `Tui.viewport <child> {opts?}` | `{w:"viewport"  child:…  offset:[0 0]}` — scroll position is app state |
| `Tui.spacer` | `{w:"spacer"}` — a flex-1 filler |

Nine widgets, nothing stateful, everything composable. Deliberately absent
from v1 (each is a composition or a later tier, §9): buttons and forms
(huh-style), tabs, progress/spinner, trees, text editors.

### 3.2 Layout: constraints as data (decided)

Every widget map accepts a **`size:`** attribute, interpreted by its parent
stack, in one of four forms:

| `size:` | Meaning |
| --- | --- |
| `<Integer>` | fixed cells (rows in a `rows`, columns in a `cols`) |
| `{pct: n}` | percent of the parent's extent |
| `"fit"` | intrinsic content size (a text's height, a list's length), capped by what remains |
| `{flex: n}` | weighted share of the remainder — **the default**, weight 1 |

The solver resolves fixed → pct → fit → flex (largest-remainder rounding so
columns always sum exactly), clamps at zero, and assigns each widget a
rectangle. It runs **natively** in the shared `tuikit` package (§4.2),
top-down from the full terminal rectangle — chosen over lipgloss-style
bottom-up content joins because a full-screen app must *fill the screen*
(top-down is the natural default), and because a constraint is **data in the
tree** (`size: {pct: 30}` travels over the wire; a join is host code).

### 3.3 Style as data

`style:` is a plain map, merged down the tree (child unset keys inherit):

```boru
{fg: "red"  bg: "#202030"  bold: true  italic: false  underline: false  reverse: false}
```

Colors are names (`"red"`, the 16), 256-palette integers, or `"#rrggbb"`.
`Tui.style base overrides` is sugar for the merge. The backend degrades
truecolor to the terminal's actual capability (§4.4); style *data* is always
written against the full model. No lipgloss values cross the seam — lipgloss
remains a `cmd/go` dashboard concern.

### 3.4 Focus is app state

The runtime has **no focus manager in v1**. Widgets accept `id: <String>`
and `input` accepts `focus: Boolean`; the app keeps `focus: "some-id"` in
its own state, routes key events in `update` accordingly, and sets
`focus: true` on the matching `input` when building the view. Two pure
helpers keep that ergonomic:

- `Tui.focusable <tree> -> [ids]` — the `id:`-carrying widgets in document
  order (so Tab cycling is `next-after ids state.focus`).
- `Tui.edit <input-map> <key-event> -> <input-map>` — the standard text-edit
  fold (insert, backspace, delete, arrows, home/end), so a text field is one
  line of `update`, while `view` stays pure and the value stays in app
  state.

The runtime's only focus behaviour: if exactly one `input` in the frame has
`focus: true`, the hardware cursor is placed at its cursor position;
otherwise the cursor is hidden. A runtime focus manager is deferred to the
inline-widget tier (§9).

## 4. Tier 1 — the raw terminal and the backend seam

### 4.1 The `Terminal` handle

One new opaque handle type, following `Socket`/`Listener`
(`NETWORK-SERVERS.0.md` §4.1, `design/IDEAL.10.md`): **`Terminal`**, global
**FixedID 5011** (next free in the 5000 band after net's 5009/5010; the
`fixedid_stability_test.go` snapshot gains a row). Wire-stable because
Tier-1 handles can leak into error values and debug output exactly as
sockets did. `eng.NewExtension(TTerminal, *termState)`; explicit
`Tui.close` with an `atomic.Bool` double-close latch; every Tier-1 word
guards on the latch and raises `tui_error closed` after. There is **no
`App` and no `Frame` boru type** — `run`/`serve` block and return plain
state; frames are internal Go values that never cross into boru.

Tier-1 drawing is double-buffered: `print-at`/`clear` mutate an offscreen
grid; **`show`** diffs it against the screen and flushes — so Tier-1 users
get the same damage-tracking the Tier-2 renderer uses, and a naive redraw
loop does not flicker.

While a `Tui.run`/`Tui.serve` app owns the terminal, `Tui.open` raises
`already_running` (single-owner rule, same as the second-`run` case, §2.2).

### 4.2 `tuikit` — the shared, dependency-free core

A new leaf package **`lang/go/tuikit`** holds everything both `lang/go` and
`cmd/go` need, with **zero terminal dependencies** (pure types + pure
functions), preserving the layering rule that `lang/go`/`eng/go` never link
terminal code:

```go
type Backend interface {
    Open(opts OpenOpts) (Info, error) // raw mode, alt-screen, cursor, mouse per opts
    Close() error                     // restore everything; idempotent
    Events() <-chan Event             // decoded key/resize/mouse/paste/focus; closed by Close
    Present(f *Frame) error           // full desired frame; the backend owns diffing
    SetTitle(string) error
    Bell() error
}
type Frame struct{ Cols, Rows int; Cells []Cell }   // row-major
type Cell  struct{ Content string /* one grapheme */; Width int8; Style Style }
```

Plus: the layout solver (§3.2), widget measurement/painting (tree →
`Frame`), style merge/degradation tables, a single **grapheme-width
function** used by every path (the one source of truth for CJK/emoji
widths), `DiffFrames(prev, next) []Span` so backends share damage tracking,
and the **`VirtualBackend`** (§8.1). `Present` takes the *full* frame — the
seam stays minimal and a dumb backend is trivially correct; smart backends
diff internally with the shared helper.

### 4.3 Host registration — `RegisterHostTui`

Exactly the `EXTENSION-MODULES.10.md` §2 shape, template the
(since-removed) `RegisterHostParser` — the frozen kind namespaces retired
the parse-side original, but RegisterHostTui keeps the recipe:

```go
type TuiSpec struct {
    Name string                        // diagnostics: "tty", "virtual", …
    Open func() (tuikit.Backend, error)
}
func RegisterHostTui(reg *native.Registry, spec TuiSpec) error
```

State lives on the registry under capability key `engine.tui.host`
(mutex-guarded; registration works before **or** after `import "boru:tui"`;
a duplicate registration errors). One backend per registry. With **no
backend registered**, every Tier-1/Tier-2 entry word raises
`tui_error "no terminal backend registered"` — a first-class negative that
the TSV spec pins (§8.2), and the natural state of the wasm playground until
an xterm.js backend exists (§9). Unlike parselang there is **no boru-level
`register` word**: a terminal backend cannot be written in boru.

### 4.4 Backend implementations

- **`tty`** — `cmd/go/internal/termback`. Built directly on **`x/term`**
  (raw mode, size, IsTerminal), **`x/ansi`** (sequence emission and input
  decoding via `ansi.DecodeSequence`), and **`x/cellbuf`** (screen buffer +
  cursor-motion-optimized flush; promoted from indirect to direct
  dependency). These are the primitives bubbletea itself is built from —
  already vendored, stable across the bubbletea v1→v2 break. **Not**
  bubbletea's `Program`: wrapping it means two event loops fighting over
  input and render timing, and pins the seam to an API that just made its
  first breaking major-version jump. The existing `boru tui` dashboard and
  `boru vault -i` keep using bubbletea, untouched. `Open` on a non-terminal
  stdout fails with `tui_error not_a_tty`.
- **`virtual`** — `tuikit.VirtualBackend`, exported for tests everywhere
  (§8.1): fixed-size in-memory grid, `Inject(ev)` to script input,
  `Screen() []string` / `CellAt(x,y)` for assertions over the current
  grid, **plus a bounded history of every `Present`ed frame**
  (`FrameCount()` / `ScreenAt(i)`) — the terminal is restored on quit, so
  every meaningful golden is a *mid-run* frame and the current grid alone
  cannot see them (surfaced by the `design/examples/tui/` probe). No OS
  anything.
- **Registration wiring**: `cmd/go`'s run/REPL/exec setup registers the
  `tty` spec unconditionally (registration is cheap; failure is deferred to
  `Open`), in the same place host policy/FileOps wiring already happens. A
  host embedding `lang` without registering anything gets the clean
  `no terminal backend` error, never a crash.

### 4.5 Tier-1 word semantics

All words raise `tui_error closed` on a closed handle and honour the
`{within: <ms>}` deadline convention of `recv*`
(`NETWORK-SERVERS.0.md` §4.3):

| Word | Effect |
| --- | --- |
| `Tui.open {opts?} -> Terminal` | enter raw mode + alt-screen (hide cursor, enable bracketed paste); `{mouse: true}` opts into mouse reporting; capability-gated (§7.3) |
| `Tui.close <t> -> None` | restore cooked mode, main screen, cursor; idempotent |
| `Tui.dims <t> -> {cols rows}` | current dimensions (renamed from `size` — a core word, §7.2) |
| `Tui.read-event <t> {within?: ms} -> Map \| None` | next decoded event (§2.4 shapes, minus `init`); `None` on deadline |
| `Tui.print-at <t> <x> <y> <text> {style?} -> None` | write styled text into the offscreen grid at cell (x, y) |
| `Tui.clear <t> -> None` | clear the offscreen grid |
| `Tui.show <t> -> None` | diff offscreen vs screen, flush the damage |
| `Tui.title <t> <s> -> None`, `Tui.bell <t> -> None` | window title / audible bell |

## 5. Tier-2 worked examples (DX assessment)

### 5.1 The counter

§1's high-level example, complete at ~15 lines. The DX claim: a minimal app
is one map with three keys, no loop, no terminal handling, no cleanup code.

### 5.2 A list browser with a filter and a background loader

The shape most real tools take: a filterable list where the data arrives
*after* startup from a spawned worker — exercising focus-as-state, `edit`,
the spawn-and-send idiom, and layout constraints.

```boru
import "boru:tui"

def browse {
  init: {q: (Tui.input "" {focus: true})  items: []  sel: 0  status: "loading…"}

  update: [ [state ev] => [ case [
      # first event: we now know our own pid — start the slow load off-loop
      [ev.tag eq "init"] [
        spawn [ send {tag: "loaded"  items: (scan-inventory)} ev.ui ]
        state ]
      # worker result arrives as an ordinary message, verbatim
      [ev.tag eq "loaded"] [
        (state set items ev.items) set status "" ]
      # navigation
      [ev.key eq "up"]   [ state set sel (max 0 (sub state.sel 1)) ]
      [ev.key eq "down"] [ state set sel (add state.sel 1) ]
      [ev.key eq "esc"]  [ Tui.quit None ]
      [ev.key eq "enter"] [ Tui.quit (pick-row state) ]
      # every other key edits the filter query (the standard fold)
      [ev.tag eq "key"]  [ state set q (Tui.edit state.q ev) ]
      [ state ] ] ] ]

  view: [ [state] => [
      Tui.rows [
        (Tui.box state.q {title: "filter"  size: 3})
        (Tui.list-view (match-rows state.items state.q.value)
                       {cursor: state.sel  size: {flex: 1}})
        (Tui.text (join "  " [state.status "↑/↓ move" "enter pick" "esc quit"])
                  {style: {fg: "grey"}  size: 1})
      ] ] ]
}

def choice ( Tui.run browse )      # blocks; returns the picked row (or None)
```

(`scan-inventory`, `match-rows`, `pick-row` are ordinary boru helpers,
elided.) The DX reads to note: the worker never touches the screen — it
sends a value; the app never blocks — slow work happens elsewhere; quitting
returns a *value* to the surrounding program, so a TUI can sit mid-pipeline
as a picker.

### 5.3 The same app, served

```boru
# on the server (headless — no TTY, no backend needed for serve):
import "boru:tui"
Tui.serve {tcp: 9700  token: "s3cret"} browse
```

```
# on any machine with the boru binary:
$ boru attach 10.0.0.5:9700 --token s3cret
```

The viewer sees pixel-for-cell the §5.2 app; keys travel up, trees travel
down (§6). `Tui.serve` returns the final state exactly as `Tui.run` does —
the picked row is available *on the server* when the viewer picks or
disconnects.

The reuse idiom this rests on: an app exports its `{init update view}`
**map** (not only a `run`-shaped word) — `Tui.run` and `Tui.serve` take
the same value, so serving is a one-word change. The
`design/examples/tui/` probe pins the shape (`TodoTui.app` beside
`TodoTui.run`).

## 6. The remote tier

### 6.1 What goes over the wire (decided): widget trees

Three candidate encodings were weighed:

1. **ANSI bytes** (the Wish model — ship the rendered escape stream): zero
   protocol design, but the client is a dumb pipe: resize needs a
   round-trip, the server must render per-client, non-TTY clients (a future
   xterm.js panel, a test harness) get nothing structured, and the wire is
   opaque to tooling.
2. **Cell diffs** (render server-side, ship damage spans): compact, but
   needs per-client frame state, sequence recovery on loss, and *still*
   couples layout to the server's idea of the client's size.
3. **Widget trees** — ship `view`'s output verbatim. The tree is *already*
   plain sendable data (a §0 invariant, held deliberately), so
   `native.ValueToAny` → JSON needs **zero new serialization**; the client
   owns layout for its own dimensions (a resize re-lays-out locally and
   costs no round trip unless the app's view actually reads the size);
   and the frame is inspectable data end to end.

Chosen: **(3), full tree per render, over the `json-lines` codec.** A
200×60 dashboard's tree is a few KB and renders are event-driven, not
per-frame — bandwidth-naive is fine at TUI scale. Tree-diffing and a
cell-mode for dumb clients are explicitly v2 extensions (§9), possible
without breaking the message envelope below.

### 6.2 Protocol

One json-line per message. Client→server:

| Message | Meaning |
| --- | --- |
| `{tag:"attach"  token:String  cols:Integer  rows:Integer  proto:1}` | handshake; first line on the connection |
| `{tag:"key" …}` / `{tag:"paste" …}` / `{tag:"mouse" …}` / `{tag:"focus" …}` | §2.4 events, verbatim |
| `{tag:"resize"  cols  rows}` | re-lays-out locally **and** is forwarded (the app's view may read the size from state) |

Server→client:

| Message | Meaning |
| --- | --- |
| `{tag:"accept"  proto:1}` / `{tag:"deny"  why:String}` | handshake result (`why` ∈ `bad-token/busy/proto`) |
| `{tag:"frame"  seq:Integer  tree:Map}` | a render — the widget tree |
| `{tag:"title"  text:String}` / `{tag:"bell"}` | pass-through chrome |
| `{tag:"quit"}` | app ended; client restores its terminal and exits |

The transport is the shipped Tier-1 net plumbing (listener + json-lines
framing shared with `net_codec.go`) and the shipped **error vocabulary**
(`closed`/`timeout`/`transport`). The `Service`/`RemoteDispatch` layer is
deliberately **not** used: a TUI session is a long-lived, server-*push*
stream, and `RemoteDispatch` models client-initiated `call`/`send`
request-response — forcing frames through it inverts the push direction for
no gain.

Internally the driver is written once against a two-implementation seam, so
local and remote are the same loop:

```go
type Renderer interface {
    Start() (cols, rows int, events <-chan eng.Value, err error)
    Render(tree eng.Value) error   // local: layout+diff+Present · remote: emit frame msg
    Title(string) error; Bell() error; Stop() error
}
```

### 6.3 Session rules (v1)

- **Auth**: constant-time bearer-token compare, the `api`/`debug` service
  pattern. An **empty `token:` refuses to listen** — no accidentally-open
  TUIs; `token: "none"` is the explicit localhost-dev opt-out.
- **One viewer at a time BY DEFAULT** — `viewers: N` (landed v2, cap
  64) admits up to N concurrent viewers: frames broadcast to all
  (each attach client lays out at its OWN geometry — the tree wire
  makes multi-size free), input merges from all, and over-cap
  authenticated attaches get `deny busy` (the busy verdict comes
  AFTER auth, so an unauthenticated probe learns nothing). A frame
  identical to the previous one is not re-sent, and late joiners
  replay the title plus the current frame.
- **Disconnect quits BY DEFAULT** (decided): the LAST viewer vanishing
  ends the app — `Tui.serve` returns the current state, symmetric with
  Ctrl-C locally. `reattach: true` (landed v2) keeps the app running
  headless instead; the next authenticated viewer resumes with the
  title and current frame.
- **Capability**: `Tui.serve` requires **both** `terminal` (§7.3) and the
  existing `network.listen` scope — it is a TUI *and* it binds a port.

### 6.4 The thin client: `boru attach` (decided)

A new top-level command, `cmd/go/internal/attach`. Deliberately dumb: dial,
handshake, then run the **local `tty` backend fed by remote frames** — the
same `tuikit` layout engine, with the process mailbox replaced by the
socket. No boru engine runs client-side. Named `attach` rather than a
`boru tui …` subcommand because the **`boru tui` supervisor dashboard already
exists** and keeps its name and flags untouched; `attach` is a verb, clash-
free (verified against the command table), and generic enough to later
attach to other session kinds. The module/CLI homonym (`boru:tui` the module,
`boru tui` the dashboard) is acknowledged and tolerated — the dashboard may
eventually be rebuilt *as* a `boru:tui` app, at which point the name
converges rather than collides.

## 7. Word surface, types, capability

### 7.1 Exports (28)

Module id **`boru:tui`**, namespace **`Tui`** — a capability/framework
module, so a plain name, not `-util` (`lang/go/CLAUDE.md` naming rules).

| Group | Words |
| --- | --- |
| Tier 1 | `open` `close` `dims` `read-event` `deliver-events` `print-at` `clear` `show` `title` `bell` |
| Tier 2 runtime | `run` `serve` `quit` |
| Pure helpers | `style` `edit` `focusable` |
| Widget constructors | `text` `rows` `cols` `box` `list-view` `table` `input` `viewport` `spacer` |
| Standalone utilities | `colorize` `strip-ansi` `text-width` ([legacy/TUI-UTILITIES.0.ignore](legacy/TUI-UTILITIES.0.ignore)) |
| Type exports | `Terminal` (type literal, so `x is Tui.Terminal` works — the `IO.StreamKind`/`Net.Socket` precedent) |

`Tui.run` app config: `{init update view}` (fn-shaped) **or**
`{service view}` (service-shaped — §11.1, resolved), plus the options
(all optional): `{mouse: Boolean  ctrl-c: "quit"|"deliver"  title: String}`.
`Tui.serve` transport options: `{tcp: Integer  token: String
viewers: Integer  reattach: Boolean}` (viewers default 1, cap 64).
`deliver-events <Terminal> <Pid>` is the Tier-1 ACTIVE input mode:
decoded events flow to the pid's mailbox (one delivery per terminal;
`read-event` refuses while a delivery owns the stream; the stream
releases when the target dies).

### 7.2 ADR-001 audit (run against the live binary)

Every proposed export was checked with `boru describe <name>` against the
core word set:

- **Collisions found and avoided**: `size` is core (list category) → the
  Tier-1 word is **`dims`**; `update` is core (query CRUD verb) → never
  exported, it is only a key in the `run` config map; `list` resolves in
  core (list category) → the widget is **`list-view`** (Textual precedent).
- **Verified free**: all 24 exported names above, including `close`/`serve`
  (which follow `boru:net`'s exact precedent) and `table` (the mutable
  container is a *type*, not a core word).
- **Considered and not exported**: `view` (free, but only a config key —
  exporting it invites shadow-adjacent confusion with zero benefit).

### 7.3 Capability

A **new `terminal` policy scope** (this is an *addition* to the permission
vocabulary — unlike `network.*`, it does not pre-exist), checked via
`native.HostPolicy(r)` exactly as `checkNetPolicy` does:

- `terminal.open` gates `Tui.open` and `Tui.run` (taking the host terminal
  over is an effect on the user's session — sandbox/read-only/compute
  profiles deny it).
- `terminal.serve` gates `Tui.serve`, **in addition to** `network.listen`.
- Module import itself is already gated by the `modules` scope
  (`lang/go/modules/modules.go::Resolve`), unchanged.

Pure words (constructors, `style`, `edit`, `focusable`, `quit`) are
**ungated** — building a description of a UI is not an effect, the same
stance as codecs-are-pure in `NETWORK-SERVERS.0.md` §9.

## 8. Testing under the house gates

### 8.1 Go tests — the loop, against `VirtualBackend`

The driver is tested end-to-end with scripted event queues and screen
assertions, no TTY anywhere (which CI does not have): inject
`init → keys → ctrl-c`, assert `run`'s return value and golden snapshots
taken from the backend's **recorded frame history** (§4.4 — after quit
the live grid is restored-empty, so goldens read `ScreenAt(i)`, not
`Screen()`; CJK/emoji rows pin the width tables); error paths one-for-one with positives (ADR-house rule): update
raises mid-loop (terminal restored, error propagates), view returns a
non-widget (`tui_error bad_widget`), second `run` (`already_running`),
backend `Open` failure, no-backend-registered. Remote: a loopback
`Tui.serve` + in-process attach against a second `VirtualBackend`, plus
`deny` negatives (bad token, busy, bad proto). Layout and painting are pure
Go — table-driven tree→Frame tests, no engine involved.

**Coverage (ADR-008)**: exactly three `//covergate:allow`-annotated
`recover`s are anticipated — the input-pump goroutine, the serve acceptor
goroutine, and the driver's terminal-restore recover — each carrying the
`net_socket.go`-style proof comment. Everything else is reachable through
the virtual backend by construction.

### 8.2 TSV spec (ADR-003) — `lang/spec/module-tui.tsv`

Every export gets at least one row; the pure-data design makes most of them
exact-equality one-liners:

- namespace binding: `import "boru:tui" convert List Tui.$module` → `['Tui']`.
- constructors by equality: `Tui.text "hi"` → its literal map; likewise
  `rows/cols/box/list-view/table/input/viewport/spacer`, `style` merges,
  `edit` folds (each key kind), `focusable` on a nested tree, `quit`'s
  marker shape. Positive + negative per word (`ERROR:` rows for bad arg
  types).
- loop/terminal words pin their **guard arms**, which are deterministic
  with no backend registered in the spec harness: `Tui.open {}` →
  `ERROR:no terminal backend registered`; `Tui.run {}` →
  `ERROR:missing update`; policy-denied rows via the spec policy fixture.
  Live-loop happy paths stay in Go tests — exactly how `module-net.tsv`
  pins descriptor validation and leaves live sockets to Go.

### 8.3 Registration checklist (from `NATIVE-MODULES.10.md`)

`lang/go/modules/tui.go` (`BuildTuiModule`, sub-registry +
trivial-delegation wrappers, **inner sigs `BarrierPos:-1`**) · row in the
`modules.go` map · catalog row in `native/help/help_render.go`
(`tui  Terminal UIs: full-screen apps, declarative widget trees, remote attach.`)
· `docs_tui.go` via `registerDocs` (every export documented —
`docs_test.go` enforces) · FixedID 5011 row in
`fixedid_stability_test.go`.

## 9. Gap analysis

- **Added (Tier 1)**: `Terminal` handle (FixedID 5011), raw-mode/alt-screen
  lifecycle, double-buffered cell drawing, decoded input events, the
  `terminal` capability scope.
- **Added (seam)**: `lang/go/tuikit` (Backend/Frame/Cell/Style, layout
  solver, width tables, `DiffFrames`, `VirtualBackend`), `RegisterHostTui`,
  the `cmd/go/internal/termback` tty backend (x/term + x/ansi + x/cellbuf).
- **Added (Tier 2)**: the app runtime on a real `Pid`/mailbox, the §2.4
  event vocabulary, nine widget constructors, constraint layout, style
  inheritance, `edit`/`focusable`, quit-as-value.
- **Added (remote)**: the §6.2 tree protocol over json-lines, `Tui.serve`,
  the `Renderer` seam, `boru attach`.
- **Depends on**: shipped processes (`spawn`/`send`/`receive`), shipped
  `boru:net` Tier-1 + json-lines, `EXTENSION-MODULES` host-seam pattern,
  `boru:time-util` timers.
- **Still out of scope (candidate later tiers, in rough order)**: inline
  non-alt-screen widgets (prompt/confirm/select/progress — the huh tier)
  and a runtime focus manager to power them; watch/computed reactivity
  sugar (Textual-style) over the same render loop; a VID-style layout
  dialect via `boru:minilang`/`boru:parselang`; theming/stylesheets;
  tree-diff and cell-mode wire encodings (CLOSED by measurement —
  §11.5); the playground browser `Backend` (LANDED — `wpg/wasm/tuiweb.go`
  renders frames as styled HTML rows in the page and feeds keydown
  events back, verified end to end by `wpg/e2e/tui-e2e.js` driving a
  live app under headless chromium; a DOM renderer rather than
  xterm.js, deliberately — the tuikit `Frame` is already a laid-out
  cell grid, so `<pre>`+spans needs no vendored terminal emulator and
  no escape-sequence round trip);
  `Stream`-fed widgets (log followers) pending `boru:stream`; Windows
  mouse/VT-input parity (raw mode works via `x/term`; declared best-effort
  in v1); image/graphics protocols (sixel/kitty).

## 10. Phased roadmap

Layered so each phase is independently testable and the cheapest useful
slice lands first (the `legacy/NETWORK-IMPLEMENTATION-PLAN.0.ignore` discipline; a
`legacy/TUI-IMPLEMENTATION-PLAN.0.ignore` will track the rollout when implementation
starts):

- **Phase A — `tuikit` + seam + Tier 1.** The leaf package (types, widths,
  diff, `VirtualBackend`), `RegisterHostTui`, `modules/tui.go` with the nine
  Tier-1 words + `Terminal` + the `terminal` scope, docs/TSV/catalog
  skeleton. Deliverable: the §1 low-level counter against the virtual
  backend in tests, and against a real TTY via a provisional `termback`.
- **Phase B — layout/render core.** Constraint solver, widget
  measure/paint, style inheritance/degradation, tree→Frame goldens (CJK/
  emoji included). Pure Go; no engine involvement.
- **Phase C — the Tier-2 loop.** Driver on a real `Process`, event
  vocabulary, coalescing, quit marker, Ctrl-C/Ctrl-\ policy, restore
  guarantees, the constructor words, `edit`/`focusable`, finished
  `termback` + registration in the run/REPL wiring. Deliverable: §5.1 and
  §5.2 running locally.
- **Phase D — remote.** `Renderer` seam refactor, `Tui.serve`, the §6.2
  protocol + token auth, `boru attach`, loopback tests. Deliverable: §5.3
  end to end.
- **Phase E — polish.** Full TSV coverage, docs catalog, cover-gate to
  100%, examples, keystroke-storm benchmark (see risk below), retro notes
  into `legacy/TUI-IMPLEMENTATION-PLAN.0.ignore`.

**Known risks, with mitigations**: grapheme/CJK width drift → one width
source in `tuikit`, goldens pin it; view-in-boru cost per event → the
bytecode VM runs `view`, coalescing bounds invocations, and an
identical-state check (cheap on immutable values) skips re-render — Phase E
benchmarks a keystroke storm before declaring victory; event storms →
drop-stale input policy (§2.4); long `update` freezes paint → documented
contract + the spawn idiom in both worked examples (§2.3); bubbletea v2
divergence → neutralized by building on the x/ primitives (§4.4); Windows
console → raw mode via `x/term` works, VT input decoding declared
best-effort, mouse gated off (§9).

## 11. Open questions

1. **Patrun-clause update — RESOLVED (landed).** The service surface
   stabilized, so `Tui.run`/`Tui.serve` now accept a SERVICE-shaped app:
   `{service: svc  view: v}` instead of `{init update view}`. Events
   dispatch through the service's patrun handlers (the probe's F1
   focus-routing ask), `wrap`/`prior` middleware observes every matched
   dispatch for free (the F4 ask), the service owns the state (`init:`
   and `update:` are rejected alongside `service:`), an event NO
   handler matches is ignored (resize/mouse noise must not error a
   pattern-routed app), and a handler reply carrying the quit marker
   ends the app. The single-`update` fn shape remains the simple
   default; both shapes share the driver.
2. **Native ticks.** Ship a `{tick: ms}` run option, or leave animation to
   `TimeUtil.interval` + `send`? (Leaning: no native ticks — the timer words
   exist, compose with the mailbox, and a tick option would be the first
   piece of event machinery not expressible as a message.)
3. **`read-event` and Tier-2 coexistence.** Should `read-event` work
   *inside* an app (peeking the mailbox) or stay Tier-1-only? (Leaning
   held: Tier-1-only; mixing pull and push event delivery in one app
   invites ordering bugs. The landed `deliver-events` gives Tier-1 the
   ACTIVE mode instead — events to a pid's mailbox — and the same
   hazard is fenced there too: `read-event` refuses while a delivery
   owns the stream.)
4. **Input widget depth.** Rune-level editing with grapheme-aware display
   is v1; is that enough before an editor-grade text widget exists?
   (Leaning: yes — `edit` covers line editing; multi-line editing is a
   later widget, not a v1 blocker. IME composition is explicitly out of
   scope for v1.)
5. **Frame delta encoding trigger — RESOLVED (measured, closed).**
   `BenchmarkTuiWireBytes` (modules) marshals the graduated todo app's
   page shape at scaled list sizes: **2.6 KB/frame at 50 rows, 9.6 KB
   at 200, 47 KB at 1000** (~16/40/148 µs to marshal). With
   unchanged-frame suppression zeroing every idle frame (landed), the
   worst CHANGING case — a pathological 1000-row tree under a
   sustained ~20-frame/s storm — is ~0.9 MB/s: fine on a LAN, only
   borderline on a slow WAN link, and unreachable by realistic apps
   (≤200-row pages at human input rates are tens of KB/s). Verdict:
   full-tree-per-changed-frame STANDS; a diff/cell-mode encoding buys
   nothing until an app demonstrably sustains huge trees at storm
   rates, and the §6.2 envelope admits the extension without a
   version bump if one ever does.
6. **Color capability negotiation over the wire.** The local backend
   degrades truecolor to the terminal's profile (§3.3); should `attach`
   report its profile in the handshake so the server could pre-degrade?
   (Leaning: no — degradation is a render-side concern; keep style data
   full-fidelity on the wire, degrade at the painting client.)
7. **`alt-screen: false` inline mode — RESOLVED (reserved, landed).**
   The `alt-screen:` key is accepted in `Tui.open`/`Tui.run`/`Tui.serve`
   configs: `true` is the implemented takeover (the default), `false`
   raises `unsupported` until the inline tier lands, and non-Booleans
   get the standard option error. Implementing the reservation exposed
   a LATENT dispatch defect: `open`'s zero-arg overload dispatched
   eagerly and a following options map never bound (open's options —
   mouse, title — had NEVER worked through the engine; only the
   direct-handler tests saw them). A zero-arg sig beside a map-taking
   sig is an unshippable pair under current dispatch, so `open` is now
   single-signature: **`Tui.open {}` is the canonical spelling** (the
   bare word is not a call), matching how every other options-taking
   word already reads. Handle-anchored optional-map pairs
   (`read-event t {within}`-style) are unaffected — verified.

8. **Image/graphics protocols (sixel/kitty) — design reserved, tier
   deferred.** The negotiation story, recorded so nothing ships that
   fights it later: (a) capability advertisement rides the EXISTING
   handshake additively — the attach hello may carry
   `caps: ["sixel", "kitty", …]` (absent = none) and the accept reply
   echoes the server-permitted subset; proto stays 1 (unknown fields
   are already ignored by both ends). Local backends advertise the
   same way via a `Caps []string` field on `tuikit.Info` when the tier
   lands. (b) The widget shape is reserved as `{w: "image", src:
   Bytes|String, fit: …}` — the renderer's unknown-widget rejection
   already refuses it loudly today, which IS the reservation. (c)
   Degradation is render-side, like color (§3.3/§11.6): a backend
   without the cap renders the image widget's `alt:` text. Deferred
   because the demand is niche and every piece above is additive;
   nothing in the landed surface forecloses it.
