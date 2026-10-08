package modules

import (
	"errors"
	"fmt"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// The Tier-2 app runtime of design/TUI.0.md §2 — "native loop, real
// mailbox": Tui.run binds the app to a real core.Process (registered as
// "tui", so any process can `send` to it), pumps decoded input into the
// mailbox, folds events through the app's update word, renders the pure
// view word's widget tree via the P2 layout core, and presents frames
// through the host backend. The driver runs ON the calling engine's
// goroutine (the call blocks until quit and returns the final state);
// only the input pump is concurrent, and it does nothing but Send.

// tuiRegisteredName is the UI process's registered name (§2.2): first
// app wins, a second concurrent run raises already_running.
const tuiRegisteredName = "tui"

// tuiDrainLimit bounds the per-render mailbox drain (§2.2 coalescing).
const tuiDrainLimit = 32

// errTuiViewerGone is the remote paint's "viewer vanished mid-frame"
// signal: losing the viewer is the §6.3 disconnect-quit (the current
// state becomes the final state), not an app error.
var errTuiViewerGone = errors.New("tui: viewer disconnected")

// acquireTuiBackend resolves the host backend and opens it — shared by
// Tui.open (which wraps the surface in a Terminal handle) and Tui.run
// (which drives it directly).
func acquireTuiBackend(r *native.Registry, word string, opts tuikit.OpenOpts) (tuikit.Backend, tuikit.Info, error) {
	spec := hostTuiSpec(r)
	if spec == nil {
		return nil, tuikit.Info{}, r.BoruError("no_backend", word+": no terminal backend registered", word)
	}
	backend, err := spec.Open()
	if err != nil {
		return nil, tuikit.Info{}, mapTuiErr(r, word, err)
	}
	info, err := backend.Open(opts)
	if err != nil {
		return nil, tuikit.Info{}, mapTuiErr(r, word, err)
	}
	return backend, info, nil
}

// tuiRunOpts reads the run-config option keys (§7.1).
func tuiRunOpts(cfg native.ReadMap, word string, r *native.Registry) (tuikit.OpenOpts, bool, error) {
	opts := tuikit.OpenOpts{}
	deliverCtrlC := false
	var err error
	if opts.Mouse, err = tuiOptBoolean(cfg, "mouse"); err == nil {
		opts.Title, err = tuiOptString(cfg, "title")
	}
	if err != nil {
		return opts, false, r.BoruError("tui_error", word+": "+err.Error(), word)
	}
	if err := tuiAltScreenReserved(cfg, word, r); err != nil {
		return opts, false, err
	}
	mode, err := tuiOptString(cfg, "ctrl-c")
	if err != nil {
		return opts, false, r.BoruError("tui_error", word+": "+err.Error(), word)
	}
	switch mode {
	case "":
	case "quit":
	case "deliver":
		deliverCtrlC = true
	default:
		return opts, false, r.BoruError("tui_error", word+`: ctrl-c: must be "quit" or "deliver"`, word)
	}
	return opts, deliverCtrlC, nil
}

// tuiApp is the parsed run config. An app is either fn-shaped
// ({init update view} — update folds state) or SERVICE-shaped
// ({service view} — events dispatch through the service's patrun
// handlers and wrap middleware, and the service owns the state;
// TUI.0.md §11.1, resolved).
type tuiApp struct {
	update, view native.Value
	updateInfo   *native.FnDefInfo
	viewInfo     *native.FnDefInfo
	init         native.Value
	service      native.Value
	isService    bool
	opts         tuikit.OpenOpts
	deliverCtrlC bool
}

func parseTuiApp(cfgV native.Value, word string, r *native.Registry) (*tuiApp, error) {
	cfg, err := native.RequireConcreteMap(cfgV, word)
	if err != nil {
		return nil, r.BoruError("tui_error", word+": expected an app config Map", word)
	}
	app := &tuiApp{}
	if sv, hasService := cfg.Get("service"); hasService {
		if _, hasUpdate := cfg.Get("update"); hasUpdate {
			return nil, r.BoruError("tui_error", word+": service: and update: are exclusive — the service owns the fold", word)
		}
		if _, hasInit := cfg.Get("init"); hasInit {
			return nil, r.BoruError("tui_error", word+": service: and init: are exclusive — the service owns the state", word)
		}
		state, isSvc := native.ServiceStateOf(sv)
		if !isSvc {
			return nil, r.BoruError("tui_error", word+": service: must be a Service", word)
		}
		app.service = sv
		app.isService = true
		app.init = state
	} else {
		up, ok := cfg.Get("update")
		if !ok {
			return nil, r.BoruError("tui_error", word+": missing update", word)
		}
		if app.updateInfo, ok = native.FnDefFromValue(up); !ok {
			return nil, r.BoruError("tui_error", word+": update must be a function taking (state, event)", word)
		}
		app.update = up
		if iv, ok := cfg.Get("init"); ok {
			app.init = iv
		} else {
			app.init = native.NewMap(native.NewOrderedMap())
		}
	}
	vw, ok := cfg.Get("view")
	if !ok {
		return nil, r.BoruError("tui_error", word+": missing view", word)
	}
	if app.viewInfo, ok = native.FnDefFromValue(vw); !ok {
		return nil, r.BoruError("tui_error", word+": view must be a function taking the state", word)
	}
	app.view = vw
	if app.opts, app.deliverCtrlC, err = tuiRunOpts(cfg, word, r); err != nil {
		return nil, err
	}
	return app, nil
}

func tuiRunHandler(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
	if err := checkTuiPolicy(r, "run", "open"); err != nil {
		return nil, err
	}
	app, err := parseTuiApp(args[0], "run", r)
	if err != nil {
		return nil, err
	}
	rt := r.Procs
	if rt == nil {
		rt = core.NewProcessRuntime()
		r.Procs = rt
	}
	// RegisterName REPLACES an existing binding (BEAM re-register
	// semantics), so the single-owner rule is a Whereis pre-check.
	if _, taken := rt.Whereis(tuiRegisteredName); taken {
		return nil, r.BoruError("already_running", "run: another TUI app already owns the terminal", "run")
	}
	proc := core.NewProcess(rt, core.DefaultMailboxBound, core.OverflowBlock)
	if err := rt.Insert(proc); err != nil {
		return nil, mapTuiErr(r, "run", err)
	}
	if err := rt.RegisterName(tuiRegisteredName, proc); err != nil { //covergate:allow RegisterName fails only when the runtime is down, which the Insert call above already rejected; the guard covers the down-race between the two calls (§modules)
		proc.Close()
		return nil, mapTuiErr(r, "run", err)
	}
	backend, info, err := acquireTuiBackend(r, "run", app.opts)
	if err != nil {
		rt.UnregisterName(tuiRegisteredName)
		proc.Close()
		return nil, err
	}
	d := &tuiDriver{
		reg: r, app: app, proc: proc,
		events: backend.Events(),
		paint:  localPaint(r, "run", backend),
		finish: func() { _ = backend.Close() },
		cols:   info.Cols, rows: info.Rows,
	}
	final, err := d.run()
	rt.UnregisterName(tuiRegisteredName)
	proc.Close()
	if err != nil {
		return nil, err
	}
	return []native.Value{final}, nil
}

// localPaint is the local half of the renderer seam (P4): view trees
// lay out through the P2 core and present on the backend. The remote
// half (tui_serve.go) marshals the tree onto the wire instead.
func localPaint(r *native.Registry, word string, backend tuikit.Backend) func(native.Value, int, int) error {
	return func(tree native.Value, cols, rows int) error {
		res, rErr := tuikit.Render(native.ValueToAny(tree), cols, rows)
		if rErr != nil {
			return r.BoruError("bad_widget", word+": "+rErr.Error(), word)
		}
		if pErr := backend.Present(res.Frame); pErr != nil {
			return mapTuiErr(r, word, pErr)
		}
		var cErr error
		if res.Cursor != nil {
			cErr = backend.SetCursor(res.Cursor.X, res.Cursor.Y, true)
		} else {
			cErr = backend.SetCursor(0, 0, false)
		}
		if cErr != nil {
			return mapTuiErr(r, word, cErr)
		}
		return nil
	}
}

type tuiDriver struct {
	reg    *native.Registry
	app    *tuiApp
	events <-chan tuikit.Event
	paint  func(tree native.Value, cols, rows int) error
	finish func()
	proc   *core.Process
	cols   int
	rows   int
}

// run owns the loop. The terminal (or connection) is released via
// finish on EVERY exit path — quit, update/view error, driver panic —
// before the error propagates (§2.5 teardown order).
func (d *tuiDriver) run() (final native.Value, err error) {
	defer func() {
		d.finish()
		if rec := recover(); rec != nil { //covergate:allow driver recover body: update/view run under InvokeCallback, whose CallBoru sub-engine converts body panics into internal_error BoruErrors flowing the covered error path; the remaining driver calls are the panic-free tuikit renderer and backend methods (§modules)
			final = native.Value{}
			err = d.reg.BoruError("internal", fmt.Sprintf("run: driver crashed: %v", rec), "run")
		}
	}()

	// the input pump: decoded events → the real mailbox, nothing else
	go func() {
		defer func() {
			if rec := recover(); rec != nil { //covergate:allow pump recover body: the loop only ranges a channel and calls Process.Send, both panic-free by construction (§modules)
				_ = rec
			}
		}()
		for ev := range d.events {
			_ = d.proc.Send(eventToMap(ev))
		}
	}()

	// the init event is applied directly — deterministically first,
	// before any pumped input (§2.4)
	initEv := native.NewOrderedMap()
	initEv.Set("tag", native.NewString("init"))
	initEv.Set("ui", native.NewPid(d.proc))
	initEv.Set("cols", native.NewInteger(int64(d.cols)))
	initEv.Set("rows", native.NewInteger(int64(d.rows)))
	state, quit, err := d.applyUpdate(d.app.init, native.NewMap(initEv))
	if err != nil {
		return native.Value{}, err
	}
	if quit {
		return state, nil
	}
	if err := d.render(state); err != nil {
		if errors.Is(err, errTuiViewerGone) {
			return state, nil
		}
		return native.Value{}, err
	}

	for {
		msg, ok, perr := d.proc.PopFront(0, false)
		if perr != nil || !ok {
			// runtime shut down underneath the app (host exit): the
			// current state is the final state
			return state, nil
		}
		state, quit, err = d.step(state, msg)
		if err != nil {
			return native.Value{}, err
		}
		if quit {
			return state, nil
		}
		// drain-and-coalesce: fold whatever queued up, render once
		for i := 0; i < tuiDrainLimit; i++ {
			more, ok, _ := d.proc.PopFront(0, true)
			if !ok {
				break
			}
			state, quit, err = d.step(state, more)
			if err != nil {
				return native.Value{}, err
			}
			if quit {
				return state, nil
			}
		}
		if err := d.render(state); err != nil {
			if errors.Is(err, errTuiViewerGone) {
				return state, nil
			}
			return native.Value{}, err
		}
	}
}

// step classifies one mailbox message (§2.4/§2.5) and folds it.
func (d *tuiDriver) step(state, msg native.Value) (native.Value, bool, error) {
	if mp, _ := native.AsMap(msg); mp != nil {
		if tag, ok := mp.Get("tag"); ok {
			if s, _ := tag.AsConcreteString(); s == "__disconnect" {
				// the remote viewer vanished: v1 disconnect-quits (§6.3)
				return state, true, nil
			} else if s == "key" {
				if chordCtrl(mp, `\`) {
					return state, true, nil // Ctrl-\ always quits (§2.5)
				}
				if !d.app.deliverCtrlC && chordCtrl(mp, "c") {
					return state, true, nil
				}
			} else if s == "resize" {
				if v, ok := mp.Get("cols"); ok {
					if n, err := v.AsConcreteInteger(); err == nil {
						d.cols = int(n)
					}
				}
				if v, ok := mp.Get("rows"); ok {
					if n, err := v.AsConcreteInteger(); err == nil {
						d.rows = int(n)
					}
				}
			}
		}
	}
	return d.applyUpdate(state, msg)
}

func chordCtrl(ev native.ReadMap, key string) bool {
	k, ok := ev.Get("key")
	if !ok {
		return false
	}
	if s, err := k.AsConcreteString(); err != nil || s != key {
		return false
	}
	mods, ok := ev.Get("mods")
	if !ok {
		return false
	}
	l, err := native.RequireConcreteList(mods, "run")
	if err != nil {
		return false
	}
	for i := 0; i < l.Len(); i++ {
		if s, sErr := l.Get(i).AsConcreteString(); sErr == nil && s == "ctrl" {
			return true
		}
	}
	return false
}

func (d *tuiDriver) applyUpdate(state, msg native.Value) (native.Value, bool, error) {
	if d.app.isService {
		return d.applyServiceUpdate(msg)
	}
	sig := native.MatchFnSig(d.app.update, []native.Value{state, msg})
	if sig == nil {
		return native.Value{}, false, d.reg.BoruError("tui_error", "run: update does not accept (state, event)", "run")
	}
	out, err := core.InvokeCallbackFn(d.reg, d.app.updateInfo, sig, []native.Value{state, msg})
	if err != nil {
		return native.Value{}, false, err
	}
	if len(out) == 0 {
		return native.Value{}, false, d.reg.BoruError("tui_error", "run: update returned no state", "run")
	}
	next := out[len(out)-1]
	if payload, ok := isQuitMarker(next); ok {
		return payload, true, nil
	}
	return next, false, nil
}

// applyServiceUpdate dispatches one event through the service-shaped
// app: wraps run outermost, the patrun-matched handler mutates the
// service state, and the reply carries the quit marker when the app is
// done. An event NO handler matches is ignored — resize/mouse noise
// must not error a pattern-routed app.
func (d *tuiDriver) applyServiceUpdate(msg native.Value) (native.Value, bool, error) {
	out, err := native.DispatchServiceValue(d.reg, d.app.service, msg)
	if err != nil {
		var ae *core.BoruError
		if errors.As(err, &ae) && ae.Code == "no_match" {
			state, _ := native.ServiceStateOf(d.app.service)
			return state, false, nil
		}
		return native.Value{}, false, err
	}
	if len(out) > 0 {
		if payload, ok := isQuitMarker(out[len(out)-1]); ok {
			return payload, true, nil
		}
	}
	state, _ := native.ServiceStateOf(d.app.service)
	return state, false, nil
}

func (d *tuiDriver) render(state native.Value) error {
	sig := native.MatchFnSig(d.app.view, []native.Value{state})
	if sig == nil {
		return d.reg.BoruError("tui_error", "run: view does not accept the state", "run")
	}
	out, err := core.InvokeCallbackFn(d.reg, d.app.viewInfo, sig, []native.Value{state})
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return d.reg.BoruError("bad_widget", "run: view returned no widget tree", "run")
	}
	return d.paint(out[len(out)-1], d.cols, d.rows)
}

// tuiRunNatives lists the Tier-2 runtime words.
func tuiRunNatives() []native.NativeFunc {
	T := func(ts ...*native.Type) []*native.Type { return ts }
	return native.SideEffecting([]native.NativeFunc{
		{Name: "run", Signatures: []native.Signature{
			{Args: T(native.TMap), Impl: native.Go(tuiRunHandler), Returns: T(native.TAny),
				ReturnsFn: tuiRunMirror(), BarrierPos: -1, CompileEffect: native.CompileStoresFn},
		}},
	})
}
