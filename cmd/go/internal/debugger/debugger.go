// Package debugger implements the interactive session behind
// `boru debug <file.boru>` (design/BORU-DEBUGGER.0.md §5): a host-side
// realisation of the StepController contract that pauses the engine's
// per-step trace between SOURCE LINES, renders the paused state, and
// drives a gdb/pdb-style command prompt.
//
// Phase 1 scope (§10): single-step (source-line coalesced), continue,
// quit (detach — the engine has no mid-run interrupt, ADR-005), inline
// Debug.break / break-when stops, and stack / bt / defs / print / list
// inspection at a pause. One Session object owns all step and pause
// semantics so every front end — the TTY prompt now, a DAP adapter later
// — shares them (§3 principle 2).
package debugger

import (
	"bufio"
	"fmt"
	core "github.com/boru-lang/boru/core/go"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
)

// stepMode is the session's stepping state.
type stepMode int

const (
	// modeStep pauses at the next new source line (the zero value: a
	// session starts paused at the program's first positioned token),
	// descending into body evaluations (§5.1).
	modeStep stepMode = iota
	// modeNext pauses at the next new source line at or above the
	// recorded frame depth — deeper (called) frames and body
	// evaluations auto-continue (§5.2 "over").
	modeNext
	// modeOut runs until the recorded frame is left — the first
	// positioned token strictly above it (§5.2).
	modeOut
	// modeContinue runs to the next breakpoint (or completion).
	modeContinue
	// modeDetached never pauses again; the program drains to completion —
	// `quit` detaches rather than killing (§5.4, ADR-005: no control-flow
	// panics, no mid-run interruption seam).
	modeDetached
)

// maxStackShown caps the one-line stack summary rendered at each pause;
// the `stack` command shows every entry.
const maxStackShown = 8

// historyCap bounds the time-travel ring: how many line-granular
// snapshots `back` can reach. Each entry retains one tape snapshot.
const historyCap = 64

// histEntry is one recorded line visit — the tape snapshot the trace
// fire handed over, its pointer, the source row, whether it came from a
// body evaluation, and the FILE identity of the firing engine (a module
// fn's body belongs to the module file). lines shares the split source
// of that file (a slice reference, not a copy).
type histEntry struct {
	stack   []native.Value
	pointer int
	row     int
	sub     bool
	file    string
	lines   []string
	// mainStops is the session's non-body line-stop ordinal when this
	// entry was recorded — the deterministic address `replay` re-runs to
	// (a body entry addresses its enclosing line stop).
	mainStops int
}

// detachNotice is printed whenever the session detaches (quit or EOF) —
// it must say the program CONTINUES, because that is what detach means.
const detachNotice = "(detached — program continues to completion)"

// Config wires a Session's I/O and source identity.
type Config struct {
	In     io.Reader // debugger command input (a TTY, a pipe, or a --script file)
	Out    io.Writer // debugger UI output
	File   string    // program path, shown in pause locations
	Source string    // program source text, for `list` and location lines
	Echo   bool      // echo each command after the prompt (script/batch mode)
	// BreakOnError pauses at every raise — the engine's "fault:" trace
	// note, fired BEFORE the error unwinds (§6.1) — including raises a
	// `do` handler will catch. The state at the raise is inspectable;
	// resuming lets the error proceed.
	BreakOnError bool
	// PostMortem arms the post-mortem inspection (§6.1): the session keeps
	// the fault's bindings from the "fault:" note — fired BEFORE the
	// error unwinds its frames (Engine.faultReturn, NUR201) — so
	// PostMortem can show the raise's scope after the run has torn it
	// down.
	PostMortem bool
	// NewRegistry builds a FRESH registry wired like the launch's own
	// (output, file identity, program args) — the seam `replay` (§6.4)
	// uses to re-run the program from the start on a clean slate. Nil
	// disables replay with a notice.
	NewRegistry func() (*native.Registry, error)
}

// Session is the interactive debug session. It implements
// capabilities.DebugOps and StepController, so the same object that
// drives the per-step trace also receives the one-shot pauses the
// Debug.break handler raises and the per-step frames of any Debug.step
// the debugged program runs itself.
type Session struct {
	reg   *native.Registry
	in    *bufio.Scanner
	out   io.Writer
	file  string
	lines []string
	echo  bool

	mode     stepMode
	prevRow  int    // row of the last positioned token stepped (line coalescing)
	prevFile string // file of that token — a row in ANOTHER file is a new line
	curRow   int    // row of the current pause (0 = unknown)

	// Pause identity (§12): the registry the firing engine was
	// constructed on (the DebugTraceFrom stamp), and the file + split
	// source it resolves to — the module file inside a module fn's
	// body, the main file otherwise. Banner, list, and the DAP
	// stackTrace all render against these, so a module pause names the
	// right row of the RIGHT file.
	curFrom  *native.Registry
	curFile  string
	curLines []string
	// srcCache memoizes each non-main registry's split Source, so a
	// stepped module body doesn't re-split per trace fire.
	srcCache map[*native.Registry][]string

	// baseDepth is the live-frame depth recorded when `next`/`out` was
	// issued; the pause rule compares against it (§5.2).
	baseDepth int

	// lineBPs / wordBPs are the prompt-settable breakpoints (§7): pause
	// on entering a source line, or when a named word is about to
	// dispatch. They fire in every mode except detached — including
	// inside body evaluations (the sub-engine descent).
	lineBPs map[int]bool
	wordBPs map[string]bool

	// watches are the data watchpoints (§6.2): name → last RENDERED
	// value. Each trace fire compares the binding's current rendering
	// and pauses on change, showing old → new. Rendered-string compare
	// is the only equality every Value supports; the cost (one lookup +
	// render per watched name per step) is paid only while watches are
	// set. watchUnset marks a name watched before it is bound.
	watches map[string]string

	// Pause context: the whole-tape snapshot from the most recent trace
	// fire, for `bt`. Storing the reference is free — the engine already
	// allocated the snapshot for the trace call.
	curStack   []native.Value
	curPointer int

	// inPrompt guards re-entrant pauses: a `print` expression that itself
	// hits Debug.break (or runs Debug.step) must never nest the prompt.
	inPrompt bool

	// baseDefs is the def-table depth snapshot taken at launch, so `defs`
	// can show the PROGRAM's bindings (new or deepened since) rather than
	// the registry's full native/def table; `defs all` shows everything.
	baseDefs map[string]int

	// dead marks a post-mortem session: the program has already
	// terminated, so detach must not claim it "continues".
	dead bool

	// breakOnError arms fault pauses (Config.BreakOnError); lastFault
	// dedups an error's fault notes as it bubbles through nested engines
	// — one pause per raise, not one per unwind level.
	breakOnError bool
	lastFault    string

	// postMortem arms the fault-scope capture (Config.PostMortem):
	// faultDefs is the firing registry's def table as it stood at the
	// LAST raise's fault note — the innermost fire of the note, before
	// the error's unwind (NUR201) pops the fault's frames — and faultReg
	// the registry it was taken from; faultNote dedups the note as it
	// repeats through nested engines, keeping the innermost capture.
	postMortem bool
	faultDefs  core.EntriesSnapshot
	faultReg   *native.Registry
	faultNote  string

	// pauseHook, when set, is an ALTERNATE front end (the DAP adapter):
	// each pause calls it — with mu held, blocking the engine — instead
	// of rendering and prompting; the returned action string goes through
	// applyAction, the same transition the prompt commands use, so the
	// two front ends can never diverge on semantics (§8.2). While the
	// hook blocks, the hook's owner is the only actor and may call the
	// session's unlocked internals (frameEntries, evalText, …).
	pauseHook func(PauseInfo) string

	// source is the program text (Config.Source), kept for replay's
	// fresh re-parse; newRegistry is the replay factory (Config).
	source      string
	newRegistry func() (*native.Registry, error)

	// mainStops counts non-body line stops recorded since launch — a
	// monotonic ordinal that survives the ring trimming, so a browsed
	// entry can be located again in a deterministic re-run (`replay`).
	// replayTarget, when non-zero, marks THIS session as a replay racing
	// to that ordinal: every pause is suppressed until it is reached.
	mainStops    int
	replayTarget int

	// history is the bounded time-travel ring (§6.4's Phase-3 form): one
	// line-granular snapshot per source line VISITED — in every mode, so
	// after a breakpoint you can look BACK at how execution got there.
	// `back`/`forward` move browse through it as a read-only re-render;
	// entry 0 is the oldest, the last entry is the current pause. The
	// engine already allocated each snapshot for the trace fire —
	// recording retains at most historyCap of them.
	history []histEntry
	// browse is the history cursor: 0 = the live pause; k>0 = viewing k
	// entries back. Reset at each new pause.
	browse int

	// mu serializes the pause machinery. The per-step trace runs on the
	// program's goroutine, but the installed DebugOps is shared registry
	// state, so a concurrent fork (a timer body hitting Debug.break) can
	// call OnStep from another goroutine. onTrace holds mu across a
	// pause; OnStep only TryLocks — a caller that cannot acquire the
	// session (another pause is at the prompt, or the pause was raised by
	// the prompt's own `print` evaluation) continues immediately, so
	// exactly one goroutine ever reads the command input.
	mu sync.Mutex

	// clearTrace disarms the engine's per-step trace; set by RunProgram.
	// Called on detach so the draining program stops paying the per-step
	// snapshot the trace forces (detach means "run at full speed").
	clearTrace func()
}

// New builds a Session over the registry the program will run in.
func New(reg *native.Registry, cfg Config) *Session {
	in := bufio.NewScanner(cfg.In)
	// Lift the Scanner's default 64KiB token cap so a long `print`
	// expression is a working command, not a silent detach.
	in.Buffer(make([]byte, 0, 64*1024), 1<<20)
	lines := strings.Split(cfg.Source, "\n")
	return &Session{
		reg:          reg,
		in:           in,
		out:          cfg.Out,
		file:         cfg.File,
		lines:        lines,
		curFile:      cfg.File,
		curLines:     lines,
		source:       cfg.Source,
		newRegistry:  cfg.NewRegistry,
		srcCache:     make(map[*native.Registry][]string),
		echo:         cfg.Echo,
		breakOnError: cfg.BreakOnError,
		postMortem:   cfg.PostMortem,
		lineBPs:      make(map[int]bool),
		wordBPs:      make(map[string]bool),
		watches:      make(map[string]string),
	}
}

// pauseSource resolves the file identity of a trace fire: the firing
// engine's registry names its own file when it has one of its own (a
// file-imported module's captured registry); everything else — the main
// tape, pooled body engines, inline modules — is the main file.
func (s *Session) pauseSource(from *native.Registry) (string, []string) {
	if from == nil || from == s.reg || from.BaseFile == "" || from.BaseFile == s.file {
		return s.file, s.lines
	}
	if cached, ok := s.srcCache[from]; ok {
		return from.BaseFile, cached
	}
	lines := strings.Split(from.Source, "\n")
	s.srcCache[from] = lines
	return from.BaseFile, lines
}

// lineAt returns the trimmed text of a 1-based line of a split source.
func lineAt(lines []string, row int) string {
	if row >= 1 && row <= len(lines) {
		return strings.TrimSpace(lines[row-1])
	}
	return ""
}

// watchUnset is the recorded rendering of a watched name that has no
// binding yet: defining it later is a change (unset → value).
const watchUnset = "(unset)"

// watchValue renders a watched binding's current value, watchUnset when
// the name is not bound.
func (s *Session) watchValue(name string) string {
	if v, ok := s.reg.Defs.Top(name); ok {
		return native.FormatForPrint(v)
	}
	return watchUnset
}

// checkWatches compares every watched binding against its recorded
// rendering, returning a pause label for the first change (and
// re-recording ALL changes, so one pause per transition — not one per
// step thereafter). Names are checked in sorted order so multi-change
// steps pause deterministically.
func (s *Session) checkWatches() string {
	if len(s.watches) == 0 {
		return ""
	}
	names := make([]string, 0, len(s.watches))
	for n := range s.watches {
		names = append(names, n)
	}
	sort.Strings(names)
	hit := ""
	for _, n := range names {
		now := s.watchValue(n)
		if now == s.watches[n] {
			continue
		}
		if hit == "" {
			hit = fmt.Sprintf("watch: %s  %s → %s", n, s.watches[n], now)
		}
		s.watches[n] = now
	}
	return hit
}

// Controller implements capabilities.DebugOps: the session is its own
// step controller.
func (s *Session) Controller() capabilities.StepController { return s }

// PauseInfo describes one pause to an alternate front end (§8.2 DAP,
// §8.3 remote). File/Line/Stack are pre-rendered at the pause so a
// remote adapter can serve them without touching live session state
// from another goroutine.
type PauseInfo struct {
	Kind   string // "step", "breakpoint", "watch", "fault", "marker", "inner-step"
	Row    int    // 1-based source row, 0 unknown
	Detail string // breakpoint label / fault text / inner-step counter
	InBody bool   // the pause is inside a body evaluation
	File   string // the pause's file identity (a module pause names the module file)
	Line   string // the source text of Row in File ("" unknown)
	Stack  string // the one-line data-stack summary at the pause
}

// applyAction performs one control transition — the single semantics
// both front ends share: the prompt's action commands and the DAP
// adapter's requests all land here.
func (s *Session) applyAction(action string) stepMode {
	switch action {
	case "step":
		s.mode = modeStep
	case "next":
		s.mode = modeNext
		s.baseDepth = liveFrameDepth(s.curStack, s.curPointer)
	case "out":
		s.mode = modeOut
		s.baseDepth = liveFrameDepth(s.curStack, s.curPointer)
	case "quit":
		return s.detach()
	default: // an unknown action continues — fail-safe for a foreign front end
		s.mode = modeContinue
	}
	return s.mode
}

// RunProgram executes the parsed program under the session: installs the
// session as the host DebugOps (filling the socket Debug.break pauses
// reach), arms the per-step trace, and runs the tokens on a fresh
// top-level interpreter engine — the debugger forces the interpreter
// path, exactly as Debug.step does (§5.5: the compiled VM does not fire
// the per-token trace).
func (s *Session) RunProgram(tokens []native.Value, source string) ([]native.Value, error) {
	// The session borrows the registry for the run: restore the prior
	// DebugOps (usually none) and Source on the way out, so a host that
	// reuses the registry afterwards doesn't keep prompting a dead session.
	prevOps, hadOps := native.EffectiveDebugOps(s.reg)
	prevSrc := s.reg.Source
	native.SetHostDebugOps(s.reg, s)
	s.reg.Source = source
	s.baseDefs = s.reg.Defs.Snapshot()
	// The registry-level hook is what descends into body evaluations
	// (each/fold/do bodies, paren groups, same-registry fn bodies): every
	// engine the registry spawns starts with it (eng SetDebugTrace). The
	// top-level engine gets the MAIN callback afterwards, so the session
	// can tell its own tape's fires from a body's.
	s.reg.SetDebugTraceFrom(s.onSubTrace)
	engine := native.NewTop(s.reg)
	engine.SetSource(source)
	engine.SetTrace(s.onTrace)
	s.clearTrace = func() {
		engine.SetTrace(nil)
		s.reg.SetDebugTraceFrom(nil)
	}
	defer func() {
		s.reg.SetDebugTraceFrom(nil)
		s.clearTrace = nil
		s.reg.Source = prevSrc
		if hadOps {
			native.SetHostDebugOps(s.reg, prevOps)
		} else {
			native.RemoveHostDebugOps(s.reg)
		}
	}()
	return engine.Run(tokens)
}

// onTrace receives the MAIN engine's per-step fires; onSubTrace — the
// registry-level hook — receives every OTHER engine's (body
// evaluations, paren groups, same-registry fn bodies). The split is
// what gives the modes their semantics: `step` descends into bodies,
// `next`/`out` treat them as part of the line, and breakpoints fire in
// both.
func (s *Session) onTrace(_ int, pointer int, stack []native.Value, note string) {
	s.trace(pointer, stack, note, false, false, s.reg)
}

func (s *Session) onSubTrace(from *native.Registry, step, pointer int, stack []native.Value, note string) {
	// step 0 marks a FRESH body evaluation (each pooled run restarts its
	// counter): line breakpoints re-arm there, so a loop body that stays
	// on one source row still breaks on every iteration. from stamps the
	// firing engine's registry — the file-identity seam (§12).
	s.trace(pointer, stack, note, true, step == 0, from)
}

// trace is the debug event loop's pause decision. It coalesces raw
// engine steps to SOURCE LINES — one stop per contiguous run of a Row,
// skipping position-less synthetic tokens (§5.1) — so `step` means
// "advance one source line", not one tape token.
func (s *Session) trace(pointer int, stack []native.Value, note string, sub, freshRun bool, from *native.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.curStack, s.curPointer = stack, pointer
	if s.mode == modeDetached || s.inPrompt {
		return
	}
	file, lines := s.pauseSource(from)
	if strings.HasPrefix(note, "fault: ") {
		// The engine fires this note at a raise, BEFORE the error unwinds
		// (Engine.faultReturn). Pause once per raise — the same note
		// repeats as the error bubbles through nested engines.
		if s.postMortem && note != s.faultNote {
			// The raise's scope, kept for PostMortem: the unwind that
			// follows this note pops the fault's frames (NUR201), and an
			// uncaught error leaves nothing else to inspect.
			reg := from
			if reg == nil {
				reg = s.reg
			}
			s.faultNote, s.faultReg, s.faultDefs = note, reg, reg.Defs.SnapshotEntries()
		}
		if !s.breakOnError || note == s.lastFault {
			return
		}
		s.lastFault = note
		s.pauseAtFault(pointer, stack, strings.TrimPrefix(note, "fault: "), from, file, lines)
		return
	}
	s.lastFault = ""
	if strings.HasPrefix(note, "for next ") {
		// A `for` iteration boundary is a line boundary: without this, a
		// single-line loop body coalesces ALL its iterations into the
		// first stop (every token shares one Row, contiguously).
		s.prevRow = 0
	}
	row := 0
	if pointer >= 0 && pointer < len(stack) {
		row = stack[pointer].Pos().Row
	}
	prev, prevFile := s.prevRow, s.prevFile
	if row != 0 {
		s.prevRow, s.prevFile = row, file
	}
	// The same row number in ANOTHER file is a different source line:
	// stepping from main line 3 into a module body's line 3 must stop.
	newLine := row != 0 && (row != prev || file != prevFile)
	if newLine {
		if !sub {
			s.mainStops++
		}
		s.recordHistory(histEntry{stack: stack, pointer: pointer, row: row, sub: sub,
			file: file, lines: lines, mainStops: s.mainStops})
	}
	if s.replayTarget != 0 {
		// A replay session races to its target ordinal: nothing pauses
		// en route — not steps, not breakpoints. On arrival the session
		// becomes an ordinary interactive pause at the replayed line.
		if !newLine || sub || s.mainStops < s.replayTarget {
			return
		}
		s.replayTarget = 0
		s.mode = modeStep
	}

	// Breakpoints pause in every mode: a line breakpoint once per line
	// entry, a word breakpoint whenever the word is about to dispatch —
	// inside body evaluations too (the sub-engine descent).
	hit := ""
	switch {
	case row != 0 && s.lineBPs[row] && (newLine || freshRun):
		hit = fmt.Sprintf("breakpoint: line %d", row)
	case len(s.wordBPs) > 0:
		if w, ok := wordNameAt(stack, pointer); ok && s.wordBPs[w] {
			hit = "breakpoint: word " + w
		}
	}
	if hit == "" {
		// Data watchpoints (§6.2) pause in every mode, like breakpoints —
		// the step whose dispatch changed the binding has already run, so
		// the pause lands on the step AFTER the change, old → new in hand.
		hit = s.checkWatches()
	}
	if hit == "" {
		switch s.mode {
		case modeStep:
			if !newLine {
				return
			}
		case modeNext:
			// A body evaluation is part of its line; a deeper inlined
			// frame is a call in progress — both auto-continue.
			if sub || !newLine || liveFrameDepth(stack, pointer) > s.baseDepth {
				return
			}
		case modeOut:
			if sub || row == 0 || liveFrameDepth(stack, pointer) >= s.baseDepth {
				return
			}
		default: // modeContinue: only breakpoints pause
			return
		}
	}
	s.curRow = row
	s.curFrom, s.curFile, s.curLines = from, file, lines
	if s.pauseHook != nil {
		kind := "step"
		switch {
		case strings.HasPrefix(hit, "watch: "):
			kind = "watch"
		case hit != "":
			kind = "breakpoint"
		}
		s.applyAction(s.pauseHook(PauseInfo{Kind: kind, Row: row, Detail: hit, InBody: sub,
			File: file, Line: lineAt(lines, row), Stack: s.stackSummary()}))
		return
	}
	where := ""
	if sub {
		where = " (in body)"
	}
	if hit != "" {
		fmt.Fprintf(s.out, "%s — at %s:%s%s  %s\n", hit, file, rowLabel(row), where, lineAt(lines, row))
	} else {
		fmt.Fprintf(s.out, "at %s:%s%s  %s\n", file, rowLabel(row), where, lineAt(lines, row))
	}
	s.showStack()
	s.prompt()
}

// pauseAtFault renders and prompts a break-on-error stop: the raise has
// happened but not yet unwound, so the stack, scope, and backtrace are
// the fault's own. Resuming (any action command) lets the error proceed
// — to a `do` handler if one encloses it, else out of the program.
func (s *Session) pauseAtFault(pointer int, stack []native.Value, errText string, from *native.Registry, file string, lines []string) {
	row := 0
	if pointer >= 0 && pointer < len(stack) {
		row = stack[pointer].Pos().Row
	}
	s.curRow = row
	s.curFrom, s.curFile, s.curLines = from, file, lines
	if s.pauseHook != nil {
		s.applyAction(s.pauseHook(PauseInfo{Kind: "fault", Row: row, Detail: errText,
			File: file, Line: lineAt(lines, row), Stack: s.stackSummary()}))
		return
	}
	fmt.Fprintf(s.out, "error raised — paused before unwind at %s:%s  %s\n",
		file, rowLabel(row), lineAt(lines, row))
	fmt.Fprintf(s.out, "  %s\n", errText)
	fmt.Fprintln(s.out, "  (resuming lets the error proceed — to a handler if one encloses it)")
	s.showStack()
	s.prompt()
}

// rowLabel renders a 1-based source row, "?" when unknown.
func rowLabel(row int) string {
	if row == 0 {
		return "?"
	}
	return strconv.Itoa(row)
}

// recordHistory appends one line visit to the bounded ring.
func (s *Session) recordHistory(e histEntry) {
	s.history = append(s.history, e)
	if len(s.history) > historyCap {
		s.history = s.history[1:]
	}
}

// viewEntry returns the browsed history entry, ok=false when live.
func (s *Session) viewEntry() (histEntry, bool) {
	if s.browse > 0 && s.browse < len(s.history) {
		return s.history[len(s.history)-1-s.browse], true
	}
	return histEntry{}, false
}

// stepBack moves the history cursor one entry further into the past.
func (s *Session) stepBack() {
	if s.browse+1 >= len(s.history) {
		fmt.Fprintln(s.out, "  (no further history)")
		return
	}
	s.browse++
	s.renderHistoryView()
}

// stepForward moves the cursor back toward the live pause.
func (s *Session) stepForward() {
	if s.browse == 0 {
		fmt.Fprintln(s.out, "  (already at the live pause)")
		return
	}
	s.browse--
	if s.browse == 0 {
		fmt.Fprintln(s.out, "  (back at the live pause)")
		return
	}
	s.renderHistoryView()
}

// renderHistoryView renders the browsed entry — a read-only re-render
// of past state: the snapshot is the truth, the registry is NOT rewound.
func (s *Session) renderHistoryView() {
	e, _ := s.viewEntry()
	where := ""
	if e.sub {
		where = " (in body)"
	}
	fmt.Fprintf(s.out, "  history -%d  at %s:%s%s  %s\n",
		s.browse, e.file, rowLabel(e.row), where, lineAt(e.lines, e.row))
	fmt.Fprintf(s.out, "  stack: %s\n", summarizeStack(resolvedData(e.stack, e.pointer)))
}

// renderHistoryList lists the recorded trail, most recent first. The
// newest entry is the current pause itself, so browsing starts at -1.
func (s *Session) renderHistoryList() {
	if len(s.history) <= 1 {
		fmt.Fprintln(s.out, "  (no earlier steps recorded)")
		return
	}
	for k := 1; k < len(s.history); k++ {
		e := s.history[len(s.history)-1-k]
		where := ""
		if e.sub {
			where = " (in body)"
		}
		fmt.Fprintf(s.out, "  -%d  line %s%s  %s\n", k, rowLabel(e.row), where, lineAt(e.lines, e.row))
	}
}

// wordNameAt returns the name of the Word about to execute, if any.
func wordNameAt(stack []native.Value, pointer int) (string, bool) {
	if pointer < 0 || pointer >= len(stack) || !native.IsWord(stack[pointer]) {
		return "", false
	}
	// IsWord guarantees the payload; AsWord's error arm is unreachable.
	w, _ := native.AsWord(stack[pointer])
	return w.Name, true
}

// liveFrameDepth counts the fn frames open at the pointer — one O(n)
// pass tracking paren nesting, counting only frame-marked opens
// (IsFrameOpen) still unmatched when the pointer is reached.
func liveFrameDepth(stack []native.Value, pointer int) int {
	if pointer > len(stack) {
		pointer = len(stack)
	}
	depth := 0
	var frames []bool
	for i := 0; i < pointer; i++ {
		switch {
		case native.IsOpenParen(stack[i]):
			isFrame := native.IsFrameOpen(stack[i])
			frames = append(frames, isFrame)
			if isFrame {
				depth++
			}
		case native.IsCloseParen(stack[i]):
			if n := len(frames); n > 0 {
				if frames[n-1] {
					depth--
				}
				frames = frames[:n-1]
			}
		}
	}
	return depth
}

// OnStep implements capabilities.StepController. It receives the pauses
// that arrive OUTSIDE the session's own trace: the one-shot pause the
// Debug.break / break-when handler raises (Step == -1, AtBreak), and the
// per-step frames of any Debug.step the debugged program runs itself.
// These can arrive from another goroutine (a fork's body hitting a
// break), so the session is only TryLock-acquired: a pause that loses
// the race — or one raised by the prompt's own `print` evaluation —
// continues immediately rather than interleaving two prompts on one
// command input.
func (s *Session) OnStep(f capabilities.StepFrame) capabilities.StepAction {
	if !s.mu.TryLock() {
		return capabilities.StepContinue
	}
	defer s.mu.Unlock()
	if s.mode == modeDetached {
		// Detached: tell any inner stepper to stand down too.
		return capabilities.StepQuit
	}
	if s.inPrompt {
		// A pause raised while evaluating a `print` expression: never nest
		// the prompt — let the evaluation run through.
		return capabilities.StepContinue
	}
	s.curRow = f.Row
	// A one-shot pause carries a file but no registry: identity falls
	// back to the session's own, and a marker in ANOTHER file has no
	// source text to list (nil lines — `list` answers honestly).
	s.curFrom = s.reg
	if f.File == "" || f.File == s.file {
		s.curFile, s.curLines = s.file, s.lines
	} else {
		s.curFile, s.curLines = f.File, nil
	}
	var mode stepMode
	if s.pauseHook != nil {
		kind := "inner-step"
		if f.AtBreak {
			kind = "marker"
		}
		mode = s.applyAction(s.pauseHook(PauseInfo{
			Kind: kind, Row: f.Row, Detail: fmt.Sprintf("step %d", f.Step),
			File: s.curFile, Line: lineAt(s.curLines, f.Row), Stack: s.stackSummary(),
		}))
	} else {
		if f.AtBreak {
			fmt.Fprintf(s.out, "break hit (Debug.break)%s\n", frameLoc(f))
		} else {
			fmt.Fprintf(s.out, "step %d%s\n", f.Step, frameLoc(f))
		}
		s.showStack()
		mode = s.prompt()
	}
	switch mode {
	case modeContinue:
		return capabilities.StepContinue
	case modeDetached:
		return capabilities.StepQuit
	}
	return capabilities.StepInto
}

// frameLoc renders the optional source location of a StepFrame.
func frameLoc(f capabilities.StepFrame) string {
	if f.Row == 0 {
		return ""
	}
	name := f.File
	if name == "" {
		name = "?"
	}
	return fmt.Sprintf(" at %s:%d", name, f.Row)
}

// prompt reads and dispatches commands until an action command (step /
// continue / quit) or end-of-input decides how execution proceeds,
// returning the session's resulting mode. Inspection commands loop.
func (s *Session) prompt() stepMode {
	s.inPrompt = true
	s.browse = 0 // each pause starts at the live view
	defer func() { s.inPrompt = false }()
	for {
		fmt.Fprint(s.out, "(adbg) ")
		if !s.in.Scan() {
			// End of the command input: detach (§5.4) — the program runs
			// to completion. Say WHY: a read failure (an oversized line,
			// an I/O error) must not masquerade as a clean Ctrl-D or a
			// drained --script file.
			if err := s.in.Err(); err != nil {
				fmt.Fprintf(s.out, "\ncommand input error: %s\n", err)
			}
			fmt.Fprintln(s.out, "\n"+s.detachMsg())
			return s.detach()
		}
		line := strings.TrimSpace(s.in.Text())
		if s.echo {
			fmt.Fprintln(s.out, line)
		}
		cmd, arg := splitCommand(line)
		switch cmd {
		case "":
			continue
		case "step", "s":
			return s.applyAction("step")
		case "next", "n":
			return s.applyAction("next")
		case "out", "o":
			m := s.applyAction("out")
			if s.baseDepth == 0 {
				fmt.Fprintln(s.out, "(out at top level runs to the next breakpoint or completion)")
			}
			return m
		case "continue", "c":
			return s.applyAction("continue")
		case "break", "b":
			if arg == "" {
				s.listBreaks()
			} else {
				fmt.Fprintln(s.out, "  "+s.SetBreak(arg))
			}
		case "delete":
			fmt.Fprintln(s.out, "  "+s.deleteBreak(arg))
		case "watch":
			if arg == "" {
				s.listWatches()
			} else {
				fmt.Fprintln(s.out, "  "+s.setWatch(arg))
			}
		case "unwatch":
			fmt.Fprintln(s.out, "  "+s.deleteWatch(arg))
		case "quit", "q":
			fmt.Fprintln(s.out, s.detachMsg())
			return s.detach()
		case "back":
			s.stepBack()
		case "forward", "fwd":
			s.stepForward()
		case "history":
			s.renderHistoryList()
		case "replay":
			s.runReplay()
		case "stack":
			s.renderFullStack()
		case "bt":
			s.renderBacktrace()
		case "defs", "locals":
			s.liveStateHint()
			s.renderDefs(arg == "all")
		case "print", "p", "eval":
			s.liveStateHint()
			s.evalExpr(arg)
		case "list", "l":
			s.renderList()
		case "help", "h", "?":
			s.renderHelp()
		default:
			fmt.Fprintf(s.out, "unknown command %q — try 'help'\n", cmd)
		}
	}
}

// PostMortem opens an inspection prompt over the FAULT state after
// RunProgram returned an uncaught error (§6.1's post-mortem entry,
// host-only): the per-step trace fired immediately BEFORE the failing
// dispatch, so the retained snapshot (curStack/curPointer) is exactly
// the program state at the raise; the error's unwind popped the fault's
// frames on its way out (Engine.faultReturn, NUR201), so the bindings
// captured at the fault note (faultDefs) are restored here — the program
// has terminated, so the restore is inspection state and nothing else.
// Every inspection command works; the program has already terminated,
// so any action command (step/continue/quit) simply ends the session.
// A detached session gets no post-mortem — the user already left.
func (s *Session) PostMortem(runErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode == modeDetached {
		return
	}
	s.dead = true
	if s.faultReg != nil {
		s.faultReg.Defs.RestoreEntriesSnapshot(s.faultDefs)
	}
	loc := ""
	if s.curPointer >= 0 && s.curPointer < len(s.curStack) {
		if row := s.curStack[s.curPointer].Pos().Row; row != 0 {
			s.curRow = row
			loc = fmt.Sprintf(" at %s:%d  %s", s.curFile, row, lineAt(s.curLines, row))
		}
	}
	fmt.Fprintf(s.out, "uncaught error — post-mortem%s\n", loc)
	fmt.Fprintf(s.out, "  %s\n", runErr)
	fmt.Fprintln(s.out, "  (the program has terminated; inspect its state — step/continue/quit exit)")
	s.showStack()
	s.prompt()
}

// dataStack resolves the data stack an inspection command should show:
// the FIRING engine's (the pause's registry — a module fn's body pause
// shows the module engine's stack), falling back to the session
// registry's innermost engine, or — in a post-mortem, when no engine is
// running — a reconstruction from the retained fault snapshot's
// resolved region. ok is false only when there is none of the three (a
// session that never ran).
func (s *Session) dataStack() ([]native.Value, bool) {
	if s.curFrom != nil {
		if vals, ok := s.curFrom.CurrentStack(); ok {
			return vals, true
		}
	}
	if vals, ok := s.reg.CurrentStack(); ok {
		return vals, true
	}
	if s.curStack == nil {
		return nil, false
	}
	return resolvedData(s.curStack, s.curPointer), true
}

// resolvedData reconstructs the data stack from a tape snapshot's
// resolved region, filtering engine markers — the offline twin of the
// live CurrentStack view, shared by the post-mortem fallback and the
// time-travel history views.
func resolvedData(stack []native.Value, pointer int) []native.Value {
	n := pointer
	if n > len(stack) {
		n = len(stack)
	}
	vals := make([]native.Value, 0, n)
	for _, v := range stack[:n] {
		if isEngineMarker(v) {
			continue
		}
		vals = append(vals, v)
	}
	return vals
}

// isEngineMarker reports whether a resolved-region tape value is engine
// bookkeeping rather than program data — the same classes the live
// CurrentStack view filters.
func isEngineMarker(v native.Value) bool {
	return native.IsWord(v) || native.IsForward(v) || native.IsMark(v) ||
		native.IsMove(v) || native.IsOpenParen(v) || native.IsCloseParen(v) ||
		native.IsEnd(v) || native.IsDefCleanup(v) || native.IsReturnCheck(v)
}

// detach puts the session in its terminal mode and disarms the per-step
// trace, so the draining program stops paying the whole-tape snapshot
// the armed trace forces every step — detach means "run at full speed".
// Clearing the trace from inside the callback is safe: the step loop
// re-checks it each iteration. Nil-guarded for a session that never ran
// a program (a bare prompt unit test).
func (s *Session) detach() stepMode {
	s.mode = modeDetached
	if s.clearTrace != nil {
		s.clearTrace()
	}
	return s.mode
}

// detachMsg is the leave-the-session notice: a running program drains
// to completion; a post-mortem one is already gone.
func (s *Session) detachMsg() string {
	if s.dead {
		return "(post-mortem closed)"
	}
	return detachNotice
}

// SetBreak parses and installs one breakpoint spec — "N" or "file:N"
// (a source line) or a word name — returning a confirmation line. The
// prompt's `break <spec>` and the launch's `--break <spec>` share it.
func (s *Session) SetBreak(spec string) string {
	if n, ok := parseLineSpec(spec); ok {
		s.lineBPs[n] = true
		return fmt.Sprintf("breakpoint set: line %d", n)
	}
	s.wordBPs[spec] = true
	return "breakpoint set: word " + spec
}

// deleteBreak removes one breakpoint (by the same spec grammar), or —
// with an empty spec — every breakpoint.
func (s *Session) deleteBreak(spec string) string {
	if spec == "" {
		s.lineBPs = make(map[int]bool)
		s.wordBPs = make(map[string]bool)
		return "all breakpoints deleted"
	}
	if n, ok := parseLineSpec(spec); ok {
		if !s.lineBPs[n] {
			return fmt.Sprintf("no breakpoint at line %d", n)
		}
		delete(s.lineBPs, n)
		return fmt.Sprintf("deleted: line %d", n)
	}
	if !s.wordBPs[spec] {
		return "no breakpoint on word " + spec
	}
	delete(s.wordBPs, spec)
	return "deleted: word " + spec
}

// parseLineSpec reads "N" or "anything:N" as a positive source line.
func parseLineSpec(spec string) (int, bool) {
	numPart := spec
	if i := strings.LastIndex(spec, ":"); i >= 0 {
		numPart = spec[i+1:]
	}
	n, err := strconv.Atoi(numPart)
	return n, err == nil && n > 0
}

// listBreaks renders the installed breakpoints.
func (s *Session) listBreaks() {
	lines := make([]int, 0, len(s.lineBPs))
	for n := range s.lineBPs {
		lines = append(lines, n)
	}
	sort.Ints(lines)
	words := make([]string, 0, len(s.wordBPs))
	for w := range s.wordBPs {
		words = append(words, w)
	}
	sort.Strings(words)
	if len(lines) == 0 && len(words) == 0 {
		fmt.Fprintln(s.out, "  (no breakpoints)")
		return
	}
	for _, n := range lines {
		fmt.Fprintf(s.out, "  line %d\n", n)
	}
	for _, w := range words {
		fmt.Fprintf(s.out, "  word %s\n", w)
	}
}

// setWatch installs a data watchpoint on a name, snapshotting the
// binding's current rendering as the compare baseline.
func (s *Session) setWatch(name string) string {
	s.watches[name] = s.watchValue(name)
	return fmt.Sprintf("watch set: %s (now %s)", name, s.watches[name])
}

// deleteWatch removes one watchpoint, or — with an empty name — all.
func (s *Session) deleteWatch(name string) string {
	if name == "" {
		s.watches = make(map[string]string)
		return "all watches deleted"
	}
	if _, ok := s.watches[name]; !ok {
		return "no watch on " + name
	}
	delete(s.watches, name)
	return "watch deleted: " + name
}

// listWatches renders the installed watchpoints and their recorded values.
func (s *Session) listWatches() {
	if len(s.watches) == 0 {
		fmt.Fprintln(s.out, "  (no watches)")
		return
	}
	names := make([]string, 0, len(s.watches))
	for n := range s.watches {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(s.out, "  watch %s = %s\n", n, s.watches[n])
	}
}

// splitCommand splits a prompt line into its command word and argument.
func splitCommand(line string) (cmd, arg string) {
	cmd, arg, _ = strings.Cut(line, " ")
	return cmd, strings.TrimSpace(arg)
}

// showStack renders the one-line data-stack summary shown at each pause.
func (s *Session) showStack() {
	if vals, ok := s.dataStack(); ok {
		fmt.Fprintf(s.out, "  stack: %s\n", summarizeStack(vals))
	}
}

// stackSummary is showStack's string form, for PauseInfo ("" when the
// session has no stack to show).
func (s *Session) stackSummary() string {
	if vals, ok := s.dataStack(); ok {
		return summarizeStack(vals)
	}
	return ""
}

// summarizeStack renders a data stack on one line, truncating to the
// top maxStackShown entries.
func summarizeStack(vals []native.Value) string {
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, native.FormatForPrint(v))
	}
	if len(parts) > maxStackShown {
		parts = append(
			[]string{fmt.Sprintf("… %d more", len(parts)-maxStackShown)},
			parts[len(parts)-maxStackShown:]...)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// runReplay re-runs the program FROM THE START on a fresh registry,
// pausing at the browsed entry's line-stop ordinal (§6.4's replay):
// deterministic re-execution reproduces the state exactly, and from
// the replayed pause every command works — including stepping FORWARD
// from a point the live run has already passed. The price is honest:
// the program's side effects re-run. The nested session shares the
// command input, so a script drives both in order; quitting the replay
// detaches it (the re-run drains) and control returns here.
func (s *Session) runReplay() {
	e, browsing := s.viewEntry()
	if !browsing {
		fmt.Fprintln(s.out, "  (replay needs a browsed entry — use back first)")
		return
	}
	if s.newRegistry == nil {
		fmt.Fprintln(s.out, "  (replay unavailable — no registry factory)")
		return
	}
	if e.mainStops == 0 {
		fmt.Fprintln(s.out, "  (nothing to replay to before the first line stop)")
		return
	}
	if e.sub {
		fmt.Fprintln(s.out, "  (a body entry replays to its enclosing line stop)")
	}
	reg, err := s.newRegistry()
	if err != nil {
		fmt.Fprintf(s.out, "  replay: %s\n", err)
		return
	}
	if reg.ParseFunc == nil {
		fmt.Fprintln(s.out, "  replay: the fresh registry has no parser")
		return
	}
	toks, perr := reg.ParseFunc(s.source)
	if perr != nil {
		fmt.Fprintf(s.out, "  replay: parse: %s\n", perr)
		return
	}
	fmt.Fprintf(s.out, "replaying %s to line stop %d — side effects re-run\n", s.file, e.mainStops)
	nested := New(reg, Config{
		In: strings.NewReader(""), Out: s.out,
		File: s.file, Source: s.source, Echo: s.echo,
		NewRegistry: s.newRegistry,
	})
	nested.in = s.in // ONE command input, consumed in order across sessions
	nested.mode = modeContinue
	nested.replayTarget = e.mainStops
	if _, rerr := nested.RunProgram(toks, s.source); rerr != nil {
		fmt.Fprintf(s.out, "replay run error: %s\n", rerr)
	}
	fmt.Fprintln(s.out, "(replay ended — back at the live pause)")
}

// liveStateHint reminds a history-browsing user that bindings and
// evaluation reflect the LIVE pause — the snapshot ring re-renders past
// stacks, it does not rewind the registry.
func (s *Session) liveStateHint() {
	if s.browse > 0 {
		fmt.Fprintln(s.out, "  (live state — the history view does not rewind bindings)")
	}
}

// renderFullStack renders every data-stack entry, top (#0) first — the
// browsed history entry's stack while time-travelling.
func (s *Session) renderFullStack() {
	var vals []native.Value
	if e, browsing := s.viewEntry(); browsing {
		vals = resolvedData(e.stack, e.pointer)
	} else if live, ok := s.dataStack(); ok {
		vals = live
	}
	if len(vals) == 0 {
		fmt.Fprintln(s.out, "  (stack empty)")
		return
	}
	for i := len(vals) - 1; i >= 0; i-- {
		fmt.Fprintf(s.out, "  #%d  %s\n", len(vals)-1-i, native.FormatForPrint(vals[i]))
	}
}

// frameInfo is one live inline fn frame reconstructed from the tape.
type frameInfo struct {
	name     string
	installs []string
}

// frameEntries resolves the frame chain the current view describes:
// the browsed history snapshot while time-travelling; the cross-engine
// chain (outermost first) while engines run — spanning the debugParent
// registry chain down to the pause's own registry, so a module fn's
// frames appear too; the retained fault snapshot at a post-mortem.
// Shared by `bt` and the DAP stackTrace.
func (s *Session) frameEntries() []frameInfo {
	if e, browsing := s.viewEntry(); browsing {
		// Time-travelling: the browsed snapshot is the only truth — the
		// live engine chain describes NOW, not then.
		return liveFrames(e.stack, e.pointer)
	}
	if states := s.reg.RunningEngineChain(s.curFrom); len(states) > 0 {
		var frames []frameInfo
		for _, st := range states {
			if st.Label != "" {
				// A labelled engine IS a fn call (CallBoruNamed): its
				// Defs-based frame leaves no tape marks, so the label
				// stands in as the frame — outer to any inside its tape.
				frames = append(frames, frameInfo{name: st.Label})
			}
			frames = append(frames, liveFrames(st.Stack, st.Pointer)...)
		}
		return frames
	}
	return liveFrames(s.curStack, s.curPointer)
}

// renderBacktrace reconstructs the live fn-frame chain (§5.3). While
// engines are running on the session's registry, the chain spans ALL of
// them — the outer program's frames plus every nested body evaluation's
// (RunningEngineStates, outermost first). At a post-mortem, when no
// engine is running, the retained fault snapshot is scanned instead.
// Module fns' own frames run on their captured registries and are still
// absent — the remaining documented gap.
func (s *Session) renderBacktrace() {
	frames := s.frameEntries()
	if len(frames) == 0 {
		fmt.Fprintln(s.out, "  (top level — no fn frames)")
		return
	}
	for i := len(frames) - 1; i >= 0; i-- { // innermost first, gdb-style
		f := frames[i]
		line := fmt.Sprintf("  #%d  %s", len(frames)-1-i, f.name)
		if len(f.installs) > 0 {
			line += "  (" + strings.Join(f.installs, " ") + ")"
		}
		fmt.Fprintln(s.out, line)
	}
}

// liveFrames scans the tape snapshot for fn frames opened below the
// pointer whose matching close paren is at/after it — calls still in
// flight, mirroring the liveness rule of the engine's own
// unwindLiveFrames (eng/go/fn_frame.go).
func liveFrames(stack []native.Value, pointer int) []frameInfo {
	if pointer > len(stack) {
		pointer = len(stack)
	}
	var out []frameInfo
	for i := 0; i < pointer; i++ {
		if !native.IsFrameOpen(stack[i]) {
			continue
		}
		depth, closed := 0, -1
		for j := i; j < len(stack); j++ {
			switch {
			case native.IsOpenParen(stack[j]):
				depth++
			case native.IsCloseParen(stack[j]):
				depth--
			}
			if depth == 0 {
				closed = j
				break
			}
		}
		if closed >= 0 && closed < pointer {
			continue // the frame already closed — not live
		}
		if info, err := native.AsFrameOpen(stack[i]); err == nil && info.Meta != nil {
			out = append(out, frameInfo{name: info.Meta.Name, installs: info.Meta.InstallNames})
		}
	}
	return out
}

// renderDefs lists current value/type bindings, sorted. By default only
// the PROGRAM's bindings show — names new or deepened since the launch
// snapshot (params, locals, its defs and imports) — because the full def
// table also mirrors every native word; `defs all` lifts the filter.
func (s *Session) renderDefs(all bool) {
	defs := s.programDefs(all)
	fmt.Fprintf(s.out, "  defs (%d):\n", len(defs))
	for _, d := range defs {
		fmt.Fprintf(s.out, "  %s = %s\n", d[0], d[1])
	}
}

// programDefs lists current bindings as (name, rendered value) pairs,
// sorted — the program's own by default, everything under all. Shared
// by `defs` and the DAP variables view.
func (s *Session) programDefs(all bool) [][2]string {
	names := s.reg.Defs.Names()
	sort.Strings(names)
	out := make([][2]string, 0, len(names))
	for _, n := range names {
		if all || s.reg.Defs.Depth(n) > s.baseDefs[n] {
			if v, ok := s.reg.Defs.Top(n); ok {
				out = append(out, [2]string{n, native.FormatForPrint(v)})
			}
		}
	}
	return out
}

// evalExpr parses and runs an expression against the paused program's
// registry in a child interpreter engine — the debugserve eval pattern:
// the expression sees the program's defs and stores, its own defs
// persist (like any eval), and it can never exceed the program's grants
// (§8.1 / §9).
func (s *Session) evalExpr(src string) {
	if src == "" {
		fmt.Fprintln(s.out, "usage: print <expr>")
		return
	}
	fmt.Fprintln(s.out, "  "+s.evalText(src))
}

// evalText is evalExpr's engine: it returns the rendered result (or the
// error / no-result notice) as one line, so the prompt and the DAP
// evaluate request share the evaluation exactly.
func (s *Session) evalText(src string) string {
	if s.reg.ParseFunc == nil {
		return "(no parser configured)"
	}
	toks, err := s.reg.ParseFunc(src)
	if err != nil {
		return fmt.Sprintf("parse error: %s", err)
	}
	// Error positions must render against the EXPRESSION, not the paused
	// program: the registry's Source/BaseFile are the program's (BoruError
	// bakes r.Source in at construction), so swap them to the expression
	// for the duration of the child run — the module loader's established
	// swap/restore shape. FlowCtrl is snapshotted too: a bare `break` /
	// `continue` in the expression would otherwise leak the signal into
	// the suspended program's loop when it resumes.
	savedSrc, savedFile, savedFlow := s.reg.Source, s.reg.BaseFile, s.reg.FlowCtrl
	s.reg.Source, s.reg.BaseFile = src, ""
	// Suppress the registry-level debug hook for the eval: its child
	// engines (paren groups, bodies) must neither pause nor clobber the
	// retained pause context. Re-armed only if a program run armed it.
	if s.clearTrace != nil {
		s.reg.SetDebugTraceFrom(nil)
		defer s.reg.SetDebugTraceFrom(s.onSubTrace)
	}
	engine := native.New(s.reg)
	engine.SetTrace(nil)
	engine.SetSource(src)
	res, err := engine.Run(toks)
	s.reg.Source, s.reg.BaseFile = savedSrc, savedFile
	if s.reg.FlowCtrl != savedFlow {
		leaked := s.reg.FlowCtrl
		s.reg.FlowCtrl = savedFlow
		return fmt.Sprintf("(expression raised a %v signal with no enclosing loop — discarded)", leaked)
	}
	if err != nil {
		return fmt.Sprintf("error: %s", err)
	}
	if len(res) == 0 {
		return "(no result)"
	}
	parts := make([]string, len(res))
	for i, v := range res {
		parts[i] = native.FormatForPrint(v)
	}
	return strings.Join(parts, " ")
}

// renderList shows the source around the current pause line — the
// browsed entry's line (and file) while time-travelling, the pause's
// own file otherwise (a module fn's body lists the module source).
func (s *Session) renderList() {
	row, lines := s.curRow, s.curLines
	if e, browsing := s.viewEntry(); browsing {
		row, lines = e.row, e.lines
	}
	if row <= 0 || row > len(lines) {
		fmt.Fprintln(s.out, "  (source line unknown)")
		return
	}
	lo, hi := row-2, row+2
	if lo < 1 {
		lo = 1
	}
	if hi > len(lines) {
		hi = len(lines)
	}
	for n := lo; n <= hi; n++ {
		marker := "   "
		if n == row {
			marker = "-> "
		}
		fmt.Fprintf(s.out, "  %s%4d  %s\n", marker, n, lines[n-1])
	}
}

// renderHelp lists the Phase-1 command set.
func (s *Session) renderHelp() {
	fmt.Fprint(s.out, `commands:
  step, s        run to the next source line (descends into bodies)
  next, n        like step, but run through deeper frames and bodies
  out, o         run until the current fn frame returns
  continue, c    run to the next breakpoint (or completion)
  break <spec>   set a breakpoint: a line ('break 12'), or a word
                 ('break add'); bare 'break' lists them
  delete [spec]  delete one breakpoint (bare 'delete' clears all)
  watch <name>   pause when the binding changes (bare 'watch' lists)
  unwatch [name] delete one watch (bare 'unwatch' clears all)
  back           view the previous line visited (time-travel, read-only)
  forward, fwd   move back toward the live pause
  history        list the recorded trail of visited lines
  replay         re-run from the start to the browsed entry (side
                 effects re-run; step forward from there)
  quit, q        detach — the program continues to completion
  stack          show every data-stack entry (top first)
  bt             backtrace of live inline fn frames
  defs, locals   the program's bindings in scope ('defs all' for every binding)
  print <expr>   evaluate an expression in the paused program's scope
  list, l        source around the current line
  help, h, ?     this help
`)
}
