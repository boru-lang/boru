package modules

import (
	"errors"
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// Direct-driver cover arms for tui_run.go that the engine path routes
// around: option rejections, policy denial, runtime-down paths, and the
// per-branch update/view contract failures.

func trcApp(t *testing.T, reg *native.Registry, src string) native.Value {
	t.Helper()
	out, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`, src})
	if err != nil {
		t.Fatal(err)
	}
	return out[len(out)-1]
}

func trcRegWithBackend(t *testing.T, vb *tuikit.VirtualBackend) *native.Registry {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterHostTui(reg, TuiSpec{Name: "v", Open: func() (tuikit.Backend, error) { return vb, nil }}); err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestTuiRunConfigOptionArms(t *testing.T) {
	vb := tuikit.NewVirtualBackend(4, 2)
	reg := trcRegWithBackend(t, vb)
	base := `{update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])`
	for _, c := range []struct{ cfg, want string }{
		{base + `  mouse: "yes"}`, "mouse: must be a Boolean"},
		{base + `  title: 5}`, "title: must be a String"},
	} {
		_, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`, `Tui.run ` + c.cfg})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("cfg %s = %v", c.cfg, err)
		}
	}
	// the explicit {ctrl-c: "quit"} spelling is accepted
	vb2 := tuikit.NewVirtualBackend(4, 2)
	vb2.Inject(tuikit.Event{Tag: "key", Key: "c", Char: "c", Mods: []string{"ctrl"}})
	reg2 := trcRegWithBackend(t, vb2)
	if v := trcApp(t, reg2,
		`Tui.run {ctrl-c: "quit"  update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`); v.Data == nil {
		t.Fatal("explicit ctrl-c quit returned no state")
	}
	// a non-concrete config map is rejected before anything opens
	_, err := tuiRunHandler([]native.Value{native.NewTypeLiteral(native.TMap)}, nil, nil, reg)
	if err == nil || !strings.Contains(err.Error(), "expected an app config Map") {
		t.Fatalf("type-literal config = %v", err)
	}
}

func TestTuiRunPolicyLazyAndBackendFailure(t *testing.T) {
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	preg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	_, rErr := tuiRunHandler([]native.Value{native.NewMap(native.NewOrderedMap())}, nil, nil, preg)
	// The gate now codes the compile failure, so the scope is no longer a struct
	// field to read — it rides in the detail, which is the form the user sees.
	var rAe *native.BoruError
	if !errors.As(rErr, &rAe) || rAe.Code != "capability_not_installed" ||
		!strings.Contains(rErr.Error(), "terminal") {
		t.Fatalf("sandbox run = %v", rErr)
	}

	// lazy runtime creation: a registry stripped of its process runtime
	vb := tuikit.NewVirtualBackend(4, 2)
	vb.Inject(tuikit.Event{Tag: "key", Key: "c", Char: "c", Mods: []string{"ctrl"}})
	reg := trcRegWithBackend(t, vb)
	reg.Procs = nil
	if v := trcApp(t, reg,
		`Tui.run {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`); v.Data == nil {
		t.Fatal("lazy-runtime run returned no state")
	}
	if reg.Procs == nil {
		t.Fatal("run did not create the process runtime")
	}

	// backend acquisition failure after the name lock releases the lock
	vb2 := tuikit.NewVirtualBackend(4, 2)
	vb2.OpenErr = errors.New("no surface")
	reg2 := trcRegWithBackend(t, vb2)
	_, err2 := runTuiStepsOn(t, reg2, []string{`import "boru:tui"`,
		`Tui.run {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`})
	if err2 == nil || !strings.Contains(err2.Error(), "no surface") {
		t.Fatalf("backend failure = %v", err2)
	}
	if _, taken := reg2.Procs.Whereis(tuiRegisteredName); taken {
		t.Fatal("failed open left the tui name registered")
	}

	// insert on a shut-down runtime fails cleanly
	reg3 := trcRegWithBackend(t, tuikit.NewVirtualBackend(4, 2))
	reg3.Procs.Shutdown()
	_, err3 := runTuiStepsOn(t, reg3, []string{`import "boru:tui"`,
		`Tui.run {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`})
	if err3 == nil || !strings.Contains(err3.Error(), "shut down") {
		t.Fatalf("down-runtime run = %v", err3)
	}
}

func TestTuiRunUpdateViewContractArms(t *testing.T) {
	run := func(vb *tuikit.VirtualBackend, src string) error {
		reg := trcRegWithBackend(t, vb)
		_, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`, src})
		return err
	}
	// update raising ON the init event
	err := run(tuikit.NewVirtualBackend(4, 2),
		`Tui.run {update: ([s:Map e:Map] => [raise "init-boom"])  view: ([s:Map] => [Tui.spacer])}`)
	if err == nil || !strings.Contains(err.Error(), "init-boom") {
		t.Fatalf("init raise = %v", err)
	}
	// update quitting ON the init event: run returns before any render
	vb := tuikit.NewVirtualBackend(4, 2)
	reg := trcRegWithBackend(t, vb)
	v := trcApp(t, reg,
		`Tui.run {init: 7  update: ([s:Any e:Map] => [Tui.quit s])  view: ([s:Any] => [Tui.spacer])}`)
	if n, nErr := v.AsConcreteInteger(); nErr != nil || n != 7 {
		t.Fatalf("init quit state = %v (%v)", v, nErr)
	}
	if vb.Presents() != 0 {
		t.Fatalf("init quit still presented %d frames", vb.Presents())
	}
	// a view that fails only on a later state: init renders, the event
	// render errors
	vb2 := tuikit.NewVirtualBackend(6, 2)
	vb2.Inject(tuikit.Event{Tag: "key", Key: "x", Char: "x"})
	err = run(vb2, `Tui.run {
	   init: {bad: false}
	   update: ([s:Map e:Map] => [ case e.tag [ "key" [ drop s set bad true ] s ] ])
	   view: ([s:Map] => [ if s.bad [ 5 ] [ Tui.spacer ] ])}`)
	if err == nil || !strings.Contains(err.Error(), "bad widget") {
		t.Fatalf("late bad view = %v", err)
	}
	// update raising on a DRAINED event (the second queued message)
	vb3 := tuikit.NewVirtualBackend(4, 2)
	vb3.Inject(tuikit.Event{Tag: "key", Key: "a", Char: "a"})
	vb3.Inject(tuikit.Event{Tag: "key", Key: "b", Char: "b"})
	err = run(vb3, `Tui.run {
	   update: ([s:Map e:Map] => [ case e.key [ "b" [ drop raise "drain-boom" ] s ] ])
	   view: ([s:Map] => [Tui.spacer])}`)
	if err == nil || !strings.Contains(err.Error(), "drain-boom") {
		t.Fatalf("drain raise = %v", err)
	}
	// quit arriving via the drain (second message quits)
	vb4 := tuikit.NewVirtualBackend(4, 2)
	vb4.Inject(tuikit.Event{Tag: "key", Key: "a", Char: "a"})
	vb4.Inject(tuikit.Event{Tag: "key", Key: "q", Char: "q"})
	reg4 := trcRegWithBackend(t, vb4)
	v = trcApp(t, reg4, `Tui.run {init: 1
	   update: ([s:Any e:Map] => [ case e.key [ "q" [ drop Tui.quit s ] s ] ])
	   view: ([s:Any] => [Tui.spacer])}`)
	if n, _ := v.AsConcreteInteger(); n != 1 {
		t.Fatalf("drain quit state = %v", v)
	}
	// update whose sig cannot take (state, event)
	err = run(tuikit.NewVirtualBackend(4, 2),
		`Tui.run {update: ([s:Integer e:Integer] => [s])  view: ([s:Map] => [Tui.spacer])}`)
	if err == nil || !strings.Contains(err.Error(), "does not accept (state, event)") {
		t.Fatalf("sig-miss update = %v", err)
	}
	// view whose sig cannot take the state
	err = run(tuikit.NewVirtualBackend(4, 2),
		`Tui.run {init: 5  update: ([s:Any e:Map] => [s])  view: ([s:String] => [Tui.spacer])}`)
	if err == nil || !strings.Contains(err.Error(), "view does not accept") {
		t.Fatalf("sig-miss view = %v", err)
	}
	// update/view producing NO value (an empty body leaves no residual)
	err = run(tuikit.NewVirtualBackend(4, 2),
		`Tui.run {update: ([s:Map e:Map] => [])  view: ([s:Map] => [Tui.spacer])}`)
	if err == nil || !strings.Contains(err.Error(), "update returned no state") {
		t.Fatalf("empty update = %v", err)
	}
	err = run(tuikit.NewVirtualBackend(4, 2),
		`Tui.run {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [])}`)
	if err == nil || !strings.Contains(err.Error(), "view returned no widget tree") {
		t.Fatalf("empty view = %v", err)
	}
	// a non-string ctrl-c option
	err = run(tuikit.NewVirtualBackend(4, 2),
		`Tui.run {ctrl-c: 5  update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`)
	if err == nil || !strings.Contains(err.Error(), "ctrl-c: must be a String") {
		t.Fatalf("non-string ctrl-c = %v", err)
	}
	// a view that raises
	err = run(tuikit.NewVirtualBackend(4, 2),
		`Tui.run {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [raise "view-boom"])}`)
	if err == nil || !strings.Contains(err.Error(), "view-boom") {
		t.Fatalf("raising view = %v", err)
	}
	// Present/SetCursor failures surface as terminal errors
	vb5 := tuikit.NewVirtualBackend(4, 2)
	vb5.PresentErr = errors.New("wedged")
	err = run(vb5, `Tui.run {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])}`)
	if err == nil || !strings.Contains(err.Error(), "wedged") {
		t.Fatalf("present failure = %v", err)
	}
	vb6 := tuikit.NewVirtualBackend(6, 2)
	vb6.CursorErr = errors.New("no cursor")
	err = run(vb6, `Tui.run {update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.input "x" {focus: true}])}`)
	if err == nil || !strings.Contains(err.Error(), "no cursor") {
		t.Fatalf("cursor failure = %v", err)
	}
}

// Runtime-down while the driver is blocked: PopFront reports down, the
// driver returns the current state as final.
func TestTuiRunRuntimeShutdownMidRun(t *testing.T) {
	vb := tuikit.NewVirtualBackend(4, 2)
	reg := trcRegWithBackend(t, vb)
	app := &tuiApp{init: native.NewInteger(9)}
	src := `import "boru:tui"  def u ([s:Any e:Map] => [s])  def v ([s:Any] => [Tui.spacer])`
	out, err := runTuiStepsOn(t, reg, []string{src, `[u/v v/v]`})
	if err != nil {
		t.Fatal(err)
	}
	fns, _ := native.AsList(out[len(out)-1])
	app.update = fns.Get(0)
	app.view = fns.Get(1)
	app.updateInfo, _ = native.FnDefFromValue(app.update)
	app.viewInfo, _ = native.FnDefFromValue(app.view)
	info, oErr := vb.Open(tuikit.OpenOpts{})
	if oErr != nil {
		t.Fatal(oErr)
	}
	rt := reg.Procs
	proc := core.NewProcess(rt, 8, core.OverflowBlock)
	if err := rt.Insert(proc); err != nil {
		t.Fatal(err)
	}
	d := &tuiDriver{
		reg: reg, app: app, proc: proc,
		events: vb.Events(),
		paint:  localPaint(reg, "run", vb),
		finish: func() { _ = vb.Close() },
		cols:   info.Cols, rows: info.Rows,
	}
	go func() { rt.Shutdown() }()
	final, dErr := d.run()
	if dErr != nil {
		t.Fatalf("shutdown run: %v", dErr)
	}
	if n, _ := final.AsConcreteInteger(); n != 9 {
		t.Fatalf("shutdown final = %v", final)
	}
}

// step's resize field guards, the disconnect marker, and the chord
// classifier's arms.
func TestTuiStepAndChordArms(t *testing.T) {
	reg := tcReg(t)
	d := &tuiDriver{reg: reg, cols: 4, rows: 2}
	appSrc := `def u ([s:Any e:Any] => [s])`
	out, err := runTuiStepsOn(t, reg, []string{appSrc, `[u/v]`})
	if err != nil {
		t.Fatal(err)
	}
	// the read-side disconnect marker quits with the current state,
	// before the update ever runs
	disc := native.NewOrderedMap()
	disc.Set("tag", native.NewString("__disconnect"))
	dState, dQuit, dErr := d.step(native.NewInteger(3), native.NewMap(disc))
	if dErr != nil || !dQuit {
		t.Fatalf("disconnect step = %v %v", dQuit, dErr)
	}
	if n, _ := dState.AsConcreteInteger(); n != 3 {
		t.Fatalf("disconnect state = %v", dState)
	}
	fns, _ := native.AsList(out[len(out)-1])
	d.app = &tuiApp{update: fns.Get(0)}
	d.app.updateInfo, _ = native.FnDefFromValue(d.app.update)

	badResize := native.NewOrderedMap()
	badResize.Set("tag", native.NewString("resize"))
	badResize.Set("cols", native.NewString("wide"))
	badResize.Set("rows", native.NewString("tall"))
	if _, _, sErr := d.step(native.NewInteger(0), native.NewMap(badResize)); sErr != nil {
		t.Fatalf("bad resize fields: %v", sErr)
	}
	if d.cols != 4 || d.rows != 2 {
		t.Fatalf("bad resize mutated dims: %d,%d", d.cols, d.rows)
	}
	// a non-map message passes straight to update
	if _, _, sErr := d.step(native.NewInteger(0), native.NewInteger(5)); sErr != nil {
		t.Fatalf("non-map message: %v", sErr)
	}

	mk := func(kv ...native.Value) native.ReadMap {
		m := native.NewOrderedMap()
		for i := 0; i+1 < len(kv); i += 2 {
			s, _ := kv[i].AsConcreteString()
			m.Set(s, kv[i+1])
		}
		v := native.NewMap(m)
		rm, _ := native.AsMap(v)
		return rm
	}
	str := native.NewString
	if chordCtrl(mk(str("key"), native.NewInteger(5)), "c") {
		t.Error("non-string key chorded")
	}
	if chordCtrl(mk(str("key"), str("c")), "c") {
		t.Error("modless key chorded")
	}
	if chordCtrl(mk(str("key"), str("c"), str("mods"), native.NewInteger(5)), "c") {
		t.Error("non-list mods chorded")
	}
	if chordCtrl(mk(str("key"), str("c"), str("mods"), native.NewList([]native.Value{str("alt")})), "c") {
		t.Error("alt-only chorded")
	}
	if !chordCtrl(mk(str("key"), str("c"), str("mods"), native.NewList([]native.Value{str("ctrl")})), "c") {
		t.Error("real chord missed")
	}
	empty := native.NewOrderedMap()
	rm, _ := native.AsMap(native.NewMap(empty))
	if chordCtrl(rm, "c") {
		t.Error("keyless map chorded")
	}
}

// Widget-word residual arms: non-printable chars are ignored by edit;
// focusable descends children only through concrete lists.
func TestTuiWidgetResidualArms(t *testing.T) {
	reg := tcReg(t)
	mkInput := func(value string) native.Value {
		m := native.NewOrderedMap()
		m.Set("value", native.NewString(value))
		return native.NewMap(m)
	}
	ev := native.NewOrderedMap()
	ev.Set("key", native.NewString("a"))
	ev.Set("char", native.NewString("\x01"))
	ev.Set("mods", native.NewList(nil))
	out, err := tuiEditHandler([]native.Value{mkInput("ab"), native.NewMap(ev)}, nil, nil, reg)
	if err != nil {
		t.Fatal(err)
	}
	mp, _ := native.AsMap(out[0])
	v, _ := mp.Get("value")
	if s, _ := v.AsConcreteString(); s != "ab" {
		t.Fatalf("non-printable inserted: %q", s)
	}
	// focusable: a non-list children value is skipped, ids elsewhere kept
	tree := native.NewOrderedMap()
	tree.Set("id", native.NewString("root"))
	tree.Set("children", native.NewInteger(5))
	got, fErr := tuiFocusableHandler([]native.Value{native.NewMap(tree)}, nil, nil, reg)
	if fErr != nil {
		t.Fatal(fErr)
	}
	l, _ := native.AsList(got[0])
	if l.Len() != 1 {
		t.Fatalf("focusable = %v", got[0])
	}
	// …and a non-map element inside a children list is skipped
	tree2 := native.NewOrderedMap()
	tree2.Set("children", native.NewList([]native.Value{native.NewInteger(5)}))
	got, fErr = tuiFocusableHandler([]native.Value{native.NewMap(tree2)}, nil, nil, reg)
	if fErr != nil {
		t.Fatal(fErr)
	}
	l, _ = native.AsList(got[0])
	if l.Len() != 0 {
		t.Fatalf("non-map child yielded ids: %v", got[0])
	}
	// edit: a non-list mods field is tolerated (no chord detected)
	in := native.NewOrderedMap()
	in.Set("value", native.NewString("ab"))
	ev2 := native.NewOrderedMap()
	ev2.Set("key", native.NewString("a"))
	ev2.Set("char", native.NewString("a"))
	ev2.Set("mods", native.NewInteger(5))
	out2, eErr := tuiEditHandler([]native.Value{native.NewMap(in), native.NewMap(ev2)}, nil, nil, reg)
	if eErr != nil {
		t.Fatal(eErr)
	}
	mp2, _ := native.AsMap(out2[0])
	v2, _ := mp2.Get("value")
	if s, _ := v2.AsConcreteString(); s != "aba" {
		t.Fatalf("non-list mods blocked the insert: %q", s)
	}
}

// a viewer vanishing mid-frame (the remote paint's errTuiViewerGone)
// is the graceful §6.3 disconnect-quit carrying the current state —
// both the init-frame arm and the loop-frame arm.
func TestTuiRunViewerGoneArms(t *testing.T) {
	reg := tcReg(t)
	src := `def u ([s:Any e:Map] => [s])  def v ([s:Any] => [Tui.spacer])`
	out, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`, src, `[u/v v/v]`})
	if err != nil {
		t.Fatal(err)
	}
	fns, _ := native.AsList(out[len(out)-1])
	app := &tuiApp{init: native.NewInteger(7), update: fns.Get(0), view: fns.Get(1)}
	app.updateInfo, _ = native.FnDefFromValue(app.update)
	app.viewInfo, _ = native.FnDefFromValue(app.view)
	rt := reg.Procs
	newDriver := func(paint func(native.Value, int, int) error, evs chan tuikit.Event) *tuiDriver {
		t.Helper()
		proc := core.NewProcess(rt, 8, core.OverflowBlock)
		if err := rt.Insert(proc); err != nil {
			t.Fatal(err)
		}
		return &tuiDriver{reg: reg, app: app, proc: proc,
			events: evs, paint: paint, finish: func() {}, cols: 4, rows: 2}
	}
	noEvents := make(chan tuikit.Event)
	close(noEvents)

	// the init frame's write fails: final = the post-init state
	d1 := newDriver(func(native.Value, int, int) error { return errTuiViewerGone }, noEvents)
	final1, err1 := d1.run()
	if err1 != nil {
		t.Fatalf("init-frame viewer-gone: %v", err1)
	}
	if n, _ := final1.AsConcreteInteger(); n != 7 {
		t.Fatalf("init-frame final = %v", final1)
	}

	// the init frame succeeds, an event-triggered frame fails
	evs := make(chan tuikit.Event, 1)
	evs <- tuikit.Event{Tag: "key", Key: "x", Char: "x"}
	close(evs)
	calls := 0
	d2 := newDriver(func(native.Value, int, int) error {
		calls++
		if calls > 1 {
			return errTuiViewerGone
		}
		return nil
	}, evs)
	final2, err2 := d2.run()
	if err2 != nil {
		t.Fatalf("loop-frame viewer-gone: %v", err2)
	}
	if n, _ := final2.AsConcreteInteger(); n != 7 {
		t.Fatalf("loop-frame final = %v", final2)
	}
	if calls != 2 {
		t.Fatalf("paint calls = %d, want 2", calls)
	}
}

// The §11.1 resolution: a SERVICE-shaped app — patrun handlers fold
// events, wrap middleware observes every matched dispatch (the probe's
// F4 ask), unmatched events (init, resize noise) are ignored, and the
// quit marker rides the handler reply.
func TestTuiRunServiceShapedApp(t *testing.T) {
	vb := tuikit.NewVirtualBackend(8, 2)
	for _, ev := range []tuikit.Event{
		{Tag: "key", Key: "up"},
		{Tag: "resize", Cols: 9, Rows: 3}, // no handler → ignored
		{Tag: "key", Key: "up"},
		{Tag: "key", Key: "q", Char: "q"},
	} {
		vb.Inject(ev)
	}
	reg := trcRegWithBackend(t, vb)
	out, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`,
		`def svc (service {n: 0  events: 0})`,
		`add {tag: "key"  key: "up"} ([ev:Map state:Any] => [
		   (state set n (add 1 state.n)) drop
		   state ]) svc`,
		`add {tag: "key"  key: "q"} ([ev:Map state:Any] => [ Tui.quit state ]) svc`,
		`wrap ([ev:Map state:Any prior:Any] => [
		   (state set events (add 1 state.events)) drop
		   prior ev ]) svc`,
		`def final (Tui.run {service: svc  view: ([s:Any] => [ Tui.text (convert String s.n) ])})`,
		`join "|" [(convert String final.n) (convert String final.events)]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := out[len(out)-1].AsConcreteString()
	if s != "2|3" {
		t.Fatalf("service app final = %q, want 2|3 (n from handlers, events from wrap)", s)
	}
	// the init frame rendered from the SERVICE's state
	if vb.FrameCount() == 0 || !strings.Contains(strings.Join(vb.ScreenAt(0), "\n"), "0") {
		t.Fatal("no init frame from the service state")
	}
}

// The service-shaped config's own contract arms.
func TestTuiRunServiceConfigArms(t *testing.T) {
	view := `view: ([s:Any] => [Tui.spacer])`
	for _, c := range []struct{ cfg, want string }{
		{`{service: 5  ` + view + `}`, "service: must be a Service"},
		{`{service: (service {})  update: ([s:Map e:Map] => [s])  ` + view + `}`, "service: and update: are exclusive"},
		{`{service: (service {})  init: 5  ` + view + `}`, "service: and init: are exclusive"},
		{`{service: (service {})}`, "missing view"},
	} {
		_, err := runTuiStepsOn(t, tcReg(t), []string{`import "boru:tui"`, `Tui.run ` + c.cfg})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("cfg %s = %v, want %q", c.cfg, err, c.want)
		}
	}
	// a handler ERROR mid-dispatch propagates out of run
	vb := tuikit.NewVirtualBackend(4, 2)
	vb.Inject(tuikit.Event{Tag: "key", Key: "x", Char: "x"})
	reg := trcRegWithBackend(t, vb)
	_, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`,
		`def svc (service {})`,
		`add {tag: "key"} ([ev:Map state:Any] => [ raise "handler-boom" ]) svc`,
		`Tui.run {service: svc  view: ([s:Any] => [Tui.spacer])}`,
	})
	if err == nil || !strings.Contains(err.Error(), "handler-boom") {
		t.Fatalf("handler error = %v", err)
	}
}
