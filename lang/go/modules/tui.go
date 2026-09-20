package modules

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// Tier-1 raw-terminal words for boru:tui — the executable slice of
// design/TUI.0.md §4 (Stage P1 of design/legacy/TUI-IMPLEMENTATION-PLAN.0.ignore):
//
//	open {mouse: Bool title: "…"}        -> Terminal      (terminal.open gated)
//	close <Terminal>                                       (restore; idempotent)
//	dims <Terminal>                       -> {cols rows}
//	read-event <Terminal> ({within: ms})  -> Map | None    (None on deadline)
//	print-at <Terminal> <x> <y> <text> ({style})           (offscreen grid)
//	clear <Terminal>                                       (offscreen grid)
//	show <Terminal>                                        (present the grid)
//	title <Terminal> <s>   /  bell <Terminal>
//
// The words draw into a double buffer: print-at/clear mutate an
// offscreen grid, show hands the full frame to the backend (which owns
// any diffing). The terminal itself is HOST-INJECTED via RegisterHostTui
// (the host-state capability pattern) — with no backend registered every
// entry word raises `no_backend`, which is the natural state of the spec
// harness and of hosts without a terminal. Error vocabulary: `closed`
// (operations on a closed Terminal or end of input), `no_backend`,
// `terminal` (backend operation failures), `tui_error` (usage).
// The app runtime (Tui.run/serve — the Tier-2 driver loop) is Stage P3+;
// nothing here starts a goroutine.

// TTerminal is the opaque terminal handle. FixedID 5011 — next free in
// the 5000-band after Socket 5009 / Listener 5010. Pinned in
// fixedid_stability_test.go.
var TTerminal = registerTuiType("Ideal/Terminal", 5011, terminalBehavior{})

func registerTuiType(path string, id int, b core.TypeBehavior) *core.Type {
	t, err := core.Builtin.RegisterType(path, id, "boru:tui", b)
	if err != nil {
		native.RecordTypeInitError(fmt.Errorf("tui: register %s: %w", path, err))
	}
	return t
}

// termState wraps one opened backend surface: the offscreen grid the
// drawing words mutate (guarded by mu — handles are sendable, so two
// processes may hold one), and a closed latch so double-close is a no-op.
type termState struct {
	id         string
	backend    tuikit.Backend
	mu         sync.Mutex
	grid       *tuikit.Frame
	closed     atomic.Bool
	delivering atomic.Bool
}

func newTerminalValue(b tuikit.Backend, info tuikit.Info) native.Value {
	ts := &termState{
		id:      native.GenerateID("TM_"),
		backend: b,
		grid:    tuikit.NewFrame(info.Cols, info.Rows),
	}
	return core.NewExtension(TTerminal, ts)
}

func asTerminal(v native.Value) (*termState, bool) {
	ep, ok := v.Data.(core.ExtensionPayload)
	if !ok {
		return nil, false
	}
	ts, ok := ep.Body.(*termState)
	return ts, ok
}

func (ts *termState) close() error {
	if ts.closed.Swap(true) {
		return nil
	}
	return ts.backend.Close()
}

type terminalBehavior struct{}

func (terminalBehavior) Match(v native.Value, t *native.Type) bool {
	return native.DefaultBehavior.Match(v, t)
}

func (terminalBehavior) Equal(a, b native.Value) bool {
	ta, oka := asTerminal(a)
	tb, okb := asTerminal(b)
	if !oka || !okb {
		return native.DefaultBehavior.Equal(a, b)
	}
	return ta == tb
}

func (terminalBehavior) Format(v native.Value) string {
	if ts, ok := asTerminal(v); ok {
		state := ""
		if ts.closed.Load() {
			state = " closed"
		}
		return "Terminal<" + ts.id + state + ">"
	}
	return "Terminal"
}

// ---- host backend registration (design/TUI.0.md §4.3) ----

// TuiSpec describes a host-provided terminal backend. Open is called
// once per Tui.open and must return a fresh (or freshly reusable)
// Backend; failures surface to the boru caller as `terminal` errors.
type TuiSpec struct {
	// Name identifies the backend in diagnostics ("tty", "virtual", …).
	Name string
	// Open constructs the backend surface. Required.
	Open func() (tuikit.Backend, error)
}

// capTuiHost is the registry capability slot holding the host-registered
// terminal backend. Per-registry, like capParseLangHost — no process
// global. One backend per registry (TUI.0.md §4.3); a second
// registration errors.
const capTuiHost = "engine.tui.host"

type tuiHostState struct {
	mu   sync.Mutex
	spec *TuiSpec
}

func tuiHostStateFor(r *native.Registry, create bool) *tuiHostState {
	if s, ok, _ := core.Cap[*tuiHostState](r, capTuiHost); ok && s != nil {
		return s
	}
	if !create {
		return nil
	}
	s := &tuiHostState{}
	_ = r.Capabilities.Set(capTuiHost, s)
	return s
}

func init() {
	// module bodies (file/inline imports) resolve the host backend too —
	// the sub-registry inherits this slot by pointer at import time
	native.ModuleInheritedCaps = append(native.ModuleInheritedCaps, capTuiHost)
}

// RegisterHostTui installs a terminal backend on reg — the embedder
// seam of design/TUI.0.md §4.3. Registration may happen before or after
// the program imports "boru:tui": the words resolve the backend at
// dispatch time, not at module build, so there is no replay step
// (register before the program runs if it loads TUI apps as file
// modules — the module sub-registry snapshots the slot at import).
func RegisterHostTui(reg *native.Registry, spec TuiSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("register tui backend: name must not be empty")
	}
	if spec.Open == nil {
		return fmt.Errorf("register tui backend %q: open must not be nil", spec.Name)
	}
	state := tuiHostStateFor(reg, true)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.spec != nil {
		return fmt.Errorf("register tui backend %q: backend %q already registered", spec.Name, state.spec.Name)
	}
	state.spec = &spec
	return nil
}

func hostTuiSpec(r *native.Registry) *TuiSpec {
	state := tuiHostStateFor(r, false)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.spec
}

// ---- policy ----

func checkTuiPolicy(r *native.Registry, word, op string) error {
	if r == nil {
		return nil
	}
	pol := native.HostPolicy(r)
	if pol == nil {
		return nil
	}
	args := policy.Args{}
	if !pol.Installed("terminal") {
		return native.PolicyRefusal(r, word, &policy.Denied{
			Code:    policy.CodeCapabilityNotInstalled,
			Scope:   "terminal",
			Op:      op,
			Profile: pol.Name(),
			Blame:   "terminal.install=false",
			Args:    args,
		})
	}
	return native.PolicyRefusal(r, word, pol.Check("terminal", op, args))
}

// tuiAltScreenReserved enforces the §11.7 reservation: `alt-screen:`
// is an accepted option KEY today so the inline tier can land without
// a breaking opts change, but only the current behaviour (true, the
// alt-screen takeover) is implemented — false is declined loudly.
func tuiAltScreenReserved(mp native.ReadMap, word string, r *native.Registry) error {
	v, ok := mp.Get("alt-screen")
	if !ok {
		return nil
	}
	b, err := v.AsConcreteBoolean()
	if err != nil {
		return r.BoruError("tui_error", word+": alt-screen: must be a Boolean", word)
	}
	if !b {
		return r.BoruError("unsupported", word+": alt-screen: false (the inline tier) is reserved but not yet implemented — see TUI.0.md §9", word)
	}
	return nil
}

// mapTuiErr converts a backend failure into the `terminal` error code —
// the transport analog of mapNetErr's vocabulary.
func mapTuiErr(r *native.Registry, word string, err error) error {
	return r.BoruError("terminal", word+": "+err.Error(), word)
}

// ---- guards and option parsing ----

func tuiTerm(args []native.Value, word string, r *native.Registry) (*termState, error) {
	ts, ok := asTerminal(args[0])
	if !ok {
		return nil, r.BoruError("tui_error", word+": expected a Terminal, got "+args[0].Parent.String(), word)
	}
	return ts, nil
}

func tuiOpenTerm(args []native.Value, word string, r *native.Registry) (*termState, error) {
	ts, err := tuiTerm(args, word, r)
	if err != nil {
		return nil, err
	}
	if ts.closed.Load() {
		return nil, r.BoruError("closed", word+": terminal is closed", word)
	}
	return ts, nil
}

// tuiOptString / tuiOptBoolean read an optional key from an options map,
// erroring on a present-but-wrong-typed value (the recvDeadline
// convention: absent is fine, malformed is a hard error).
func tuiOptString(mp native.ReadMap, key string) (string, error) {
	v, ok := mp.Get(key)
	if !ok {
		return "", nil
	}
	s, err := v.AsConcreteString()
	if err != nil {
		return "", fmt.Errorf("%s: must be a String", key)
	}
	return s, nil
}

func tuiOptBoolean(mp native.ReadMap, key string) (bool, error) {
	v, ok := mp.Get(key)
	if !ok {
		return false, nil
	}
	b, err := v.AsConcreteBoolean()
	if err != nil {
		return false, fmt.Errorf("%s: must be a Boolean", key)
	}
	return b, nil
}

// styleFromMap projects a style map ({fg bg bold italic underline
// reverse}, TUI.0.md §3.3) onto tuikit.Style. Unknown keys are ignored;
// wrong-typed known keys are errors.
func styleFromMap(v native.Value, word string, r *native.Registry) (tuikit.Style, error) {
	st := tuikit.Style{}
	mp, _ := native.AsMap(v)
	if mp == nil {
		return st, r.BoruError("tui_error", word+": style must be a Map", word)
	}
	var err error
	if st.FG, err = tuiOptString(mp, "fg"); err == nil {
		st.BG, err = tuiOptString(mp, "bg")
	}
	if err == nil {
		st.Bold, err = tuiOptBoolean(mp, "bold")
	}
	if err == nil {
		st.Italic, err = tuiOptBoolean(mp, "italic")
	}
	if err == nil {
		st.Underline, err = tuiOptBoolean(mp, "underline")
	}
	if err == nil {
		st.Reverse, err = tuiOptBoolean(mp, "reverse")
	}
	if err != nil {
		return tuikit.Style{}, r.BoruError("tui_error", word+": "+err.Error(), word)
	}
	return st, nil
}

// eventToMap projects a decoded backend event onto the plain tagged map
// shapes of TUI.0.md §2.4 (minus `init`, which is the Tier-2 driver's).
func eventToMap(ev tuikit.Event) native.Value {
	m := native.NewOrderedMap()
	m.Set("tag", native.NewString(ev.Tag))
	switch ev.Tag {
	case "key":
		m.Set("key", native.NewString(ev.Key))
		m.Set("char", native.NewString(ev.Char))
		mods := make([]native.Value, 0, len(ev.Mods))
		for _, mod := range ev.Mods {
			mods = append(mods, native.NewString(mod))
		}
		m.Set("mods", native.NewList(mods))
	case "resize":
		m.Set("cols", native.NewInteger(int64(ev.Cols)))
		m.Set("rows", native.NewInteger(int64(ev.Rows)))
	case "mouse":
		m.Set("kind", native.NewString(ev.Kind))
		m.Set("x", native.NewInteger(int64(ev.X)))
		m.Set("y", native.NewInteger(int64(ev.Y)))
	case "paste":
		m.Set("text", native.NewString(ev.Text))
	case "focus":
		m.Set("gained", native.NewBoolean(ev.Gained))
	}
	return native.NewMap(m)
}

// ---- word handlers ----

func tuiOpenHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if err := checkTuiPolicy(r, "open", "open"); err != nil {
		return nil, err
	}
	opts := tuikit.OpenOpts{}
	if len(args) > 0 {
		mp, _ := native.AsMap(args[0])
		if mp == nil {
			return nil, r.BoruError("tui_error", "open: options must be a Map", "open")
		}
		var err error
		if opts.Mouse, err = tuiOptBoolean(mp, "mouse"); err == nil {
			opts.Title, err = tuiOptString(mp, "title")
		}
		if err != nil {
			return nil, r.BoruError("tui_error", "open: "+err.Error(), "open")
		}
		if err := tuiAltScreenReserved(mp, "open", r); err != nil {
			return nil, err
		}
	}
	backend, info, err := acquireTuiBackend(r, "open", opts)
	if err != nil {
		return nil, err
	}
	return []native.Value{newTerminalValue(backend, info)}, nil
}

func tuiCloseHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ts, err := tuiTerm(args, "close", r)
	if err != nil {
		return nil, err
	}
	if cErr := ts.close(); cErr != nil {
		return nil, mapTuiErr(r, "close", cErr)
	}
	return nil, nil
}

func tuiDimsHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ts, err := tuiOpenTerm(args, "dims", r)
	if err != nil {
		return nil, err
	}
	cols, rows, sErr := ts.backend.Size()
	if sErr != nil {
		return nil, mapTuiErr(r, "dims", sErr)
	}
	m := native.NewOrderedMap()
	m.Set("cols", native.NewInteger(int64(cols)))
	m.Set("rows", native.NewInteger(int64(rows)))
	return []native.Value{native.NewMap(m)}, nil
}

func tuiReadEventHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ts, err := tuiOpenTerm(args, "read-event", r)
	if err != nil {
		return nil, err
	}
	if ts.delivering.Load() {
		return nil, r.BoruError("tui_error", "read-event: events are being delivered to a process (deliver-events owns the stream)", "read-event")
	}
	dur, has, dErr := recvDeadline(args, 1)
	if dErr != nil {
		return nil, r.BoruError("tui_error", "read-event: "+dErr.Error(), "read-event")
	}
	events := ts.backend.Events()
	if !has {
		ev, ok := <-events
		if !ok {
			return nil, r.BoruError("closed", "read-event: terminal input has ended", "read-event")
		}
		return []native.Value{eventToMap(ev)}, nil
	}
	timer := time.NewTimer(dur)
	defer timer.Stop()
	select {
	case ev, ok := <-events:
		if !ok {
			return nil, r.BoruError("closed", "read-event: terminal input has ended", "read-event")
		}
		return []native.Value{eventToMap(ev)}, nil
	case <-timer.C:
		return []native.Value{native.NewTypeLiteral(native.TNone)}, nil
	}
}

// tuiDeliverEventsHandler is the Tier-1 ACTIVE delivery mode
// (TUI.0.md §11.3 revisited, design plan follow-on): decoded events
// flow to a process mailbox instead of being pulled via read-event, so
// a Tier-1 program can fold terminal input together with its other
// messages the actor way. One delivery per terminal at a time;
// read-event declines while a delivery owns the stream. Delivery stops
// when the terminal closes (the event channel ends) or the target
// process dies (Send fails), the latter releasing the stream for a new
// deliver-events or read-event.
func tuiDeliverEventsHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ts, err := tuiOpenTerm(args, "deliver-events", r)
	if err != nil {
		return nil, err
	}
	proc, ok := native.PidProcess(args[1])
	if !ok {
		return nil, r.BoruError("tui_error", "deliver-events: expected a Pid target, got "+args[1].Parent.String(), "deliver-events")
	}
	if !ts.delivering.CompareAndSwap(false, true) {
		return nil, r.BoruError("tui_error", "deliver-events: events are already being delivered", "deliver-events")
	}
	go func() {
		defer ts.delivering.Store(false)
		defer func() {
			if rec := recover(); rec != nil { //covergate:allow delivery-pump recover body: the loop only selects on channels and calls Process.Send, both panic-free by construction (§modules)
				_ = rec
			}
		}()
		events := ts.backend.Events()
		done := proc.Done()
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					return // the terminal closed
				}
				// a send to a dead process silently drops (BEAM), so
				// liveness is watched via Done below, not the error
				_ = proc.Send(eventToMap(ev))
			case <-done:
				return // the target died: release the stream
			}
		}
	}()
	return nil, nil
}

func tuiPrintAtHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ts, err := tuiOpenTerm(args, "print-at", r)
	if err != nil {
		return nil, err
	}
	x64, xErr := args[1].AsConcreteInteger()
	if xErr != nil {
		return nil, r.BoruError("tui_error", "print-at: x must be an Integer", "print-at")
	}
	y64, yErr := args[2].AsConcreteInteger()
	if yErr != nil {
		return nil, r.BoruError("tui_error", "print-at: y must be an Integer", "print-at")
	}
	text, tErr := args[3].AsConcreteString()
	if tErr != nil {
		return nil, r.BoruError("tui_error", "print-at: text must be a String", "print-at")
	}
	st := tuikit.Style{}
	if len(args) > 4 {
		if st, err = styleFromMap(args[4], "print-at", r); err != nil {
			return nil, err
		}
	}
	// Cells carry real terminal widths (same idiom as render.go's
	// drawText): wide runes take two columns with a Width -1
	// continuation cell, combining marks join the previous cell.
	ts.mu.Lock()
	x, y := int(x64), int(y64)
	start := x
	for _, rn := range text {
		w := tuikit.RuneWidth(rn)
		if w == 0 {
			if x-1 >= start {
				if c, ok := ts.grid.At(x-1, y); ok && c.Content != "" {
					c.Content += string(rn)
					ts.grid.Set(x-1, y, c)
				}
			}
			continue
		}
		if x+w > ts.grid.Cols {
			break
		}
		ts.grid.Set(x, y, tuikit.Cell{Content: string(rn), Width: int8(w), Style: st})
		if w == 2 {
			ts.grid.Set(x+1, y, tuikit.Cell{Content: "", Width: -1, Style: st})
		}
		x += w
	}
	ts.mu.Unlock()
	return nil, nil
}

func tuiClearHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ts, err := tuiOpenTerm(args, "clear", r)
	if err != nil {
		return nil, err
	}
	ts.mu.Lock()
	ts.grid = tuikit.NewFrame(ts.grid.Cols, ts.grid.Rows)
	ts.mu.Unlock()
	return nil, nil
}

func tuiShowHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ts, err := tuiOpenTerm(args, "show", r)
	if err != nil {
		return nil, err
	}
	ts.mu.Lock()
	frame := ts.grid.Clone()
	ts.mu.Unlock()
	if pErr := ts.backend.Present(frame); pErr != nil {
		return nil, mapTuiErr(r, "show", pErr)
	}
	return nil, nil
}

func tuiTitleHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ts, err := tuiOpenTerm(args, "title", r)
	if err != nil {
		return nil, err
	}
	s, sErr := args[1].AsConcreteString()
	if sErr != nil {
		return nil, r.BoruError("tui_error", "title: expected a String", "title")
	}
	if tErr := ts.backend.SetTitle(s); tErr != nil {
		return nil, mapTuiErr(r, "title", tErr)
	}
	return nil, nil
}

func tuiBellHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	ts, err := tuiOpenTerm(args, "bell", r)
	if err != nil {
		return nil, err
	}
	if bErr := ts.backend.Bell(); bErr != nil {
		return nil, mapTuiErr(r, "bell", bErr)
	}
	return nil, nil
}

func tuiNoReturns(_ []native.Value, _ *native.Registry) []native.Value { return nil }

// tuiNatives lists everything BuildTuiModule registers: the Tier-1
// terminal words below plus the Tier-2 pure words (tui_widgets.go) and
// the app runtime (tui_run.go).
func tuiNatives() []native.NativeFunc {
	all := tuiTier1Natives()
	all = append(all, tuiWidgetNatives()...)
	all = append(all, tuiRunNatives()...)
	all = append(all, tuiServeNatives()...)
	all = append(all, tuiUtilNatives()...)
	return all
}

// tuiTier1Natives lists the Tier-1 raw-terminal words.
func tuiTier1Natives() []native.NativeFunc {
	T := func(ts ...*native.Type) []*native.Type { return ts }
	return []native.NativeFunc{
		{Name: "open", Signatures: []native.Signature{
			{Args: T(native.TMap), Impl: native.Go(tuiOpenHandler), Returns: T(TTerminal),
				ReturnsFn: tuiOpenMirror(), BarrierPos: -1},
		}},
		{Name: "close", Signatures: []native.Signature{
			{Args: T(TTerminal), Impl: native.Go(tuiCloseHandler), Returns: T(),
				ReturnsFn: tuiNoReturns, BarrierPos: -1},
		}},
		{Name: "dims", Signatures: []native.Signature{
			{Args: T(TTerminal), Impl: native.Go(tuiDimsHandler), Returns: T(native.TMap), BarrierPos: -1},
		}},
		{Name: "read-event", Signatures: []native.Signature{
			{Args: T(TTerminal, native.TMap), Impl: native.Go(tuiReadEventHandler), Returns: T(native.TAny), BarrierPos: -1},
			{Args: T(TTerminal), Impl: native.Go(tuiReadEventHandler), Returns: T(native.TAny), BarrierPos: -1},
		}},
		{Name: "deliver-events", Signatures: []native.Signature{
			{Args: T(TTerminal, native.TPid), Impl: native.Go(tuiDeliverEventsHandler), Returns: T(),
				ReturnsFn: tuiNoReturns, BarrierPos: -1},
		}},
		{Name: "print-at", Signatures: []native.Signature{
			{Args: T(TTerminal, native.TInteger, native.TInteger, native.TString, native.TMap),
				Impl: native.Go(tuiPrintAtHandler), Returns: T(), ReturnsFn: tuiNoReturns, BarrierPos: -1},
			{Args: T(TTerminal, native.TInteger, native.TInteger, native.TString),
				Impl: native.Go(tuiPrintAtHandler), Returns: T(), ReturnsFn: tuiNoReturns, BarrierPos: -1},
		}},
		{Name: "clear", Signatures: []native.Signature{
			{Args: T(TTerminal), Impl: native.Go(tuiClearHandler), Returns: T(),
				ReturnsFn: tuiNoReturns, BarrierPos: -1},
		}},
		{Name: "show", Signatures: []native.Signature{
			{Args: T(TTerminal), Impl: native.Go(tuiShowHandler), Returns: T(),
				ReturnsFn: tuiNoReturns, BarrierPos: -1},
		}},
		{Name: "title", Signatures: []native.Signature{
			{Args: T(TTerminal, native.TString), Impl: native.Go(tuiTitleHandler), Returns: T(),
				ReturnsFn: tuiNoReturns, BarrierPos: -1},
		}},
		{Name: "bell", Signatures: []native.Signature{
			{Args: T(TTerminal), Impl: native.Go(tuiBellHandler), Returns: T(),
				ReturnsFn: tuiNoReturns, BarrierPos: -1},
		}},
	}
}

// BuildTuiModule creates the "boru:tui" native module: the Tier-1 words
// in an isolated sub-registry behind trivial-delegation wrappers, plus
// the Terminal type literal (so `x is Tui.Terminal` works after import).
func BuildTuiModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	natives := tuiNatives()
	for _, n := range natives {
		subReg.RegisterNativeFunc(n)
	}
	exports := delegatingExports(natives, subReg)
	exports.Set("Terminal", native.NewTypeLiteral(TTerminal))
	return moduleDesc(parent, "Tui", subReg, exports), nil
}
