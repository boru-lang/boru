package modules

import (
	"errors"
	"strings"
	"testing"
	"time"

	core "github.com/boru-lang/boru/core/go"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
	"github.com/boru-lang/boru/lang/go/tuikit"
)

// Direct-handler cover tests for tui.go, driving the arms the engine
// path cannot reach on a terminal-less CI (net_socket_cover_test.go
// pattern): crafted args, error-code assertions, seam swaps.

func tcReg(t *testing.T) *native.Registry {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func tcErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func tcCode(t *testing.T, err error) string {
	t.Helper()
	var ae *core.BoruError
	if !errors.As(err, &ae) {
		t.Fatalf("not a BoruError: %v", err)
	}
	return ae.Code
}

// tcTerm builds an opened Terminal value over a fresh VirtualBackend.
func tcTerm(t *testing.T, cols, rows int) (native.Value, *tuikit.VirtualBackend) {
	t.Helper()
	vb := tuikit.NewVirtualBackend(cols, rows)
	info, err := vb.Open(tuikit.OpenOpts{})
	if err != nil {
		t.Fatal(err)
	}
	return newTerminalValue(vb, info), vb
}

// --- type registration ---

func TestTuiRegisterTypeDuplicateRecorded(t *testing.T) {
	prev := native.SwapTypeInitErrs(nil)
	t.Cleanup(func() { native.SwapTypeInitErrs(prev) })
	registerTuiType("Ideal/Terminal", 5011, terminalBehavior{})
	errs := native.TypeInitError()
	if errs == nil || !strings.Contains(errs.Error(), "tui: register") {
		t.Fatalf("duplicate registration not recorded: %v", errs)
	}
}

// --- behavior ---

func TestTuiTerminalBehaviorEqualFormat(t *testing.T) {
	a, _ := tcTerm(t, 1, 1)
	b, _ := tcTerm(t, 1, 1)
	beh := terminalBehavior{}
	if !beh.Equal(a, a) {
		t.Fatal("same handle not Equal")
	}
	if beh.Equal(a, b) {
		t.Fatal("distinct handles Equal")
	}
	if beh.Equal(a, native.NewInteger(1)) {
		t.Fatal("Terminal Equal Integer")
	}
	if got := beh.Format(a); !strings.HasPrefix(got, "Terminal<TM_") || strings.Contains(got, "closed") {
		t.Fatalf("open Format = %q", got)
	}
	ts, _ := asTerminal(a)
	_ = ts.close()
	if got := beh.Format(a); !strings.Contains(got, " closed>") {
		t.Fatalf("closed Format = %q", got)
	}
	if got := beh.Format(native.NewInteger(1)); got != "Terminal" {
		t.Fatalf("non-Terminal Format = %q", got)
	}
	if _, ok := asTerminal(native.NewInteger(1)); ok {
		t.Fatal("asTerminal accepted an Integer")
	}
}

// --- registration seam ---

func TestTuiRegisterHostValidation(t *testing.T) {
	reg := tcReg(t)
	open := func() (tuikit.Backend, error) { return tuikit.NewVirtualBackend(1, 1), nil }
	if err := RegisterHostTui(reg, TuiSpec{Name: "", Open: open}); err == nil ||
		!strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("empty name = %v", err)
	}
	if err := RegisterHostTui(reg, TuiSpec{Name: "x", Open: nil}); err == nil ||
		!strings.Contains(err.Error(), "open must not be nil") {
		t.Fatalf("nil open = %v", err)
	}
}

// --- policy ---

func TestTuiPolicyArms(t *testing.T) {
	if err := checkTuiPolicy(nil, "open", "open"); err != nil {
		t.Fatalf("nil registry: %v", err)
	}
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		t.Fatal(err)
	}
	_, oErr := tuiOpenHandler(nil, nil, nil, reg)
	tcErrContains(t, oErr, "terminal")
	// Coded by the gate (native/policy_error.go), so the compile failure is
	// dispatchable from boru rather than an opaque foreign error.
	var oAe *native.BoruError
	if !errors.As(oErr, &oAe) || oAe.Code != "capability_not_installed" {
		t.Fatalf("sandbox open = %v", oErr)
	}
	// an installed scope consults Check and proceeds (to no_backend here)
	trusted, err := policy.Load("trusted")
	if err != nil {
		t.Fatal(err)
	}
	treg, err := native.DefaultRegistryWithPolicy(trusted)
	if err != nil {
		t.Fatal(err)
	}
	_, tErr := tuiOpenHandler(nil, nil, nil, treg)
	if got := tcCode(t, tErr); got != "no_backend" {
		t.Fatalf("trusted open code = %q (%v)", got, tErr)
	}
}

// --- open arms ---

func TestTuiOpenHandlerArms(t *testing.T) {
	reg := tcReg(t)
	vb := tuikit.NewVirtualBackend(1, 1)
	if err := RegisterHostTui(reg, TuiSpec{Name: "v", Open: func() (tuikit.Backend, error) { return vb, nil }}); err != nil {
		t.Fatal(err)
	}
	_, err := tuiOpenHandler([]native.Value{native.NewInteger(5)}, nil, nil, reg)
	tcErrContains(t, err, "options must be a Map")

	m := native.NewOrderedMap()
	m.Set("mouse", native.NewString("yes"))
	_, err = tuiOpenHandler([]native.Value{native.NewMap(m)}, nil, nil, reg)
	tcErrContains(t, err, "mouse: must be a Boolean")

	m2 := native.NewOrderedMap()
	m2.Set("title", native.NewInteger(5))
	_, err = tuiOpenHandler([]native.Value{native.NewMap(m2)}, nil, nil, reg)
	tcErrContains(t, err, "title: must be a String")

	vb.OpenErr = errors.New("backend declined")
	_, err = tuiOpenHandler(nil, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("backend.Open failure code = %q (%v)", got, err)
	}

	failReg := tcReg(t)
	if err := RegisterHostTui(failReg, TuiSpec{Name: "f", Open: func() (tuikit.Backend, error) {
		return nil, errors.New("no surface")
	}}); err != nil {
		t.Fatal(err)
	}
	_, err = tuiOpenHandler(nil, nil, nil, failReg)
	tcErrContains(t, err, "no surface")
}

// --- per-word guard + closed + backend-failure arms ---

func TestTuiHandlerGuardArms(t *testing.T) {
	reg := tcReg(t)
	bad := []native.Value{native.NewInteger(1)}
	for name, h := range map[string]native.Handler{
		"close":      tuiCloseHandler,
		"dims":       tuiDimsHandler,
		"read-event": tuiReadEventHandler,
		"clear":      tuiClearHandler,
		"show":       tuiShowHandler,
		"bell":       tuiBellHandler,
	} {
		_, err := h(bad, nil, nil, reg)
		if got := tcCode(t, err); got != "tui_error" {
			t.Fatalf("%s guard code = %q (%v)", name, got, err)
		}
	}
	_, err := tuiPrintAtHandler([]native.Value{native.NewInteger(1), native.NewInteger(0), native.NewInteger(0), native.NewString("x")}, nil, nil, reg)
	if got := tcCode(t, err); got != "tui_error" {
		t.Fatalf("print-at guard code = %q (%v)", got, err)
	}
	_, err = tuiTitleHandler([]native.Value{native.NewInteger(1), native.NewString("s")}, nil, nil, reg)
	if got := tcCode(t, err); got != "tui_error" {
		t.Fatalf("title guard code = %q (%v)", got, err)
	}
}

func TestTuiBackendFailureArms(t *testing.T) {
	reg := tcReg(t)
	term, vb := tcTerm(t, 2, 2)

	vb.SizeErr = errors.New("nope")
	_, err := tuiDimsHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("dims failure code = %q", got)
	}
	vb.SizeErr = nil

	vb.PresentErr = errors.New("nope")
	_, err = tuiShowHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("show failure code = %q", got)
	}
	vb.PresentErr = nil

	vb.TitleErr = errors.New("nope")
	_, err = tuiTitleHandler([]native.Value{term, native.NewString("s")}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("title failure code = %q", got)
	}
	vb.TitleErr = nil

	vb.BellErr = errors.New("nope")
	_, err = tuiBellHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("bell failure code = %q", got)
	}
	vb.BellErr = nil

	vb.CloseErr = errors.New("nope")
	_, err = tuiCloseHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "terminal" {
		t.Fatalf("close failure code = %q", got)
	}
	// the failed close latched the termState; the backend close is owed
	vb.CloseErr = nil
}

// --- read-event arms ---

func TestTuiReadEventArms(t *testing.T) {
	reg := tcReg(t)
	term, vb := tcTerm(t, 2, 2)

	badWithin := native.NewOrderedMap()
	badWithin.Set("within", native.NewInteger(-1))
	_, err := tuiReadEventHandler([]native.Value{term, native.NewMap(badWithin)}, nil, nil, reg)
	tcErrContains(t, err, "within: must be a non-negative Integer")

	// blocking (no deadline) path with a queued event
	vb.Inject(tuikit.Event{Tag: "paste", Text: "clip"})
	out, err := tuiReadEventHandler([]native.Value{term}, nil, nil, reg)
	if err != nil {
		t.Fatal(err)
	}
	mp, _ := native.AsMap(out[0])
	got, _ := mp.Get("text")
	if s, sErr := got.AsConcreteString(); sErr != nil || s != "clip" {
		t.Fatalf("paste event = %v", out[0])
	}

	// end-of-input: the backend closes underneath an open handle
	_ = vb.Close()
	_, err = tuiReadEventHandler([]native.Value{term}, nil, nil, reg)
	if got := tcCode(t, err); got != "closed" {
		t.Fatalf("blocking end-of-input code = %q", got)
	}
	within := native.NewOrderedMap()
	within.Set("within", native.NewInteger(1000))
	_, err = tuiReadEventHandler([]native.Value{term, native.NewMap(within)}, nil, nil, reg)
	if got := tcCode(t, err); got != "closed" {
		t.Fatalf("deadline end-of-input code = %q", got)
	}
}

// --- print-at / title argument arms ---

func TestTuiPrintAtArgumentArms(t *testing.T) {
	reg := tcReg(t)
	term, _ := tcTerm(t, 2, 2)
	i := native.NewInteger
	s := native.NewString

	_, err := tuiPrintAtHandler([]native.Value{term, s("x"), i(0), s("t")}, nil, nil, reg)
	tcErrContains(t, err, "x must be an Integer")
	_, err = tuiPrintAtHandler([]native.Value{term, i(0), s("y"), s("t")}, nil, nil, reg)
	tcErrContains(t, err, "y must be an Integer")
	_, err = tuiPrintAtHandler([]native.Value{term, i(0), i(0), i(7)}, nil, nil, reg)
	tcErrContains(t, err, "text must be a String")
	_, err = tuiPrintAtHandler([]native.Value{term, i(0), i(0), s("t"), i(7)}, nil, nil, reg)
	tcErrContains(t, err, "style must be a Map")

	_, err = tuiTitleHandler([]native.Value{term, i(7)}, nil, nil, reg)
	tcErrContains(t, err, "title: expected a String")
}

// --- style parsing arms ---

func TestTuiStyleFromMapArms(t *testing.T) {
	reg := tcReg(t)
	for key, val := range map[string]native.Value{
		"fg":        native.NewInteger(1),
		"bg":        native.NewInteger(1),
		"bold":      native.NewString("y"),
		"italic":    native.NewString("y"),
		"underline": native.NewString("y"),
		"reverse":   native.NewString("y"),
	} {
		m := native.NewOrderedMap()
		m.Set(key, val)
		_, err := styleFromMap(native.NewMap(m), "print-at", reg)
		tcErrContains(t, err, key+": must be a")
	}
}

// --- event projection ---

func TestTuiEventToMapAllTags(t *testing.T) {
	cases := map[string]tuikit.Event{
		"resize": {Tag: "resize", Cols: 80, Rows: 24},
		"mouse":  {Tag: "mouse", Kind: "press", X: 3, Y: 4},
		"paste":  {Tag: "paste", Text: "p"},
		"focus":  {Tag: "focus", Gained: true},
	}
	for tag, ev := range cases {
		mp, _ := native.AsMap(eventToMap(ev))
		got, ok := mp.Get("tag")
		s, sErr := got.AsConcreteString()
		if !ok || sErr != nil || s != tag {
			t.Fatalf("%s tag = %v", tag, got)
		}
	}
	mp, _ := native.AsMap(eventToMap(tuikit.Event{Tag: "resize", Cols: 80, Rows: 24}))
	cols, _ := mp.Get("cols")
	if n, err := cols.AsConcreteInteger(); err != nil || n != 80 {
		t.Fatalf("resize cols = %v (%v)", cols, err)
	}
}

// --- returns fn ---

func TestTuiNoReturns(t *testing.T) {
	if got := tuiNoReturns(nil, nil); got != nil {
		t.Fatalf("tuiNoReturns = %v", got)
	}
}

// --- no-panic discipline: type literals through every handler ---

func TestTuiTypeLiteralNoPanic(t *testing.T) {
	reg := tcReg(t)
	vb := tuikit.NewVirtualBackend(2, 2)
	if err := RegisterHostTui(reg, TuiSpec{Name: "v", Open: func() (tuikit.Backend, error) { return vb, nil }}); err != nil {
		t.Fatal(err)
	}
	term, tvb := tcTerm(t, 2, 2)
	// a Map TYPE LITERAL as the within: option map reads as "no options"
	// (AsMap returns nil), so read-event blocks — queue an event first.
	tvb.Inject(tuikit.Event{Tag: "key", Key: "x"})
	lit := native.NewTypeLiteral(native.TMap)
	noPanic := func(name string, run func()) {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("%s panicked on a type literal: %v", name, rec)
			}
		}()
		run()
	}
	noPanic("open opts", func() { _, _ = tuiOpenHandler([]native.Value{lit}, nil, nil, reg) })
	noPanic("read-event within", func() { _, _ = tuiReadEventHandler([]native.Value{term, lit}, nil, nil, reg) })
	noPanic("print-at style", func() {
		_, _ = tuiPrintAtHandler([]native.Value{term, native.NewInteger(0), native.NewInteger(0), native.NewString("x"), lit}, nil, nil, reg)
	})
	for name, h := range map[string]native.Handler{
		"close": tuiCloseHandler, "dims": tuiDimsHandler, "clear": tuiClearHandler,
		"show": tuiShowHandler, "bell": tuiBellHandler,
	} {
		noPanic(name+" handle", func() { _, _ = h([]native.Value{lit}, nil, nil, reg) })
	}
}

// The Tier-1 active delivery mode (deliver-events): decoded events
// land in a process mailbox, the stream is exclusive against
// read-event and double delivery, and a dead target releases it.
func TestTuiDeliverEvents(t *testing.T) {
	vb := tuikit.NewVirtualBackend(4, 2)
	reg := trcRegWithBackend(t, vb)
	out, err := runTuiStepsOn(t, reg, []string{`import "boru:tui"`, `Tui.open {}`})
	if err != nil {
		t.Fatal(err)
	}
	var term native.Value
	for _, v := range out {
		if _, isTerm := asTerminal(v); isTerm {
			term = v
		}
	}
	if _, isTerm := asTerminal(term); !isTerm {
		t.Fatalf("no Terminal in %v", out)
	}
	if reg.Procs == nil {
		reg.Procs = core.NewProcessRuntime()
	}
	proc := core.NewProcess(reg.Procs, 8, core.OverflowBlock)
	if err := reg.Procs.Insert(proc); err != nil {
		t.Fatal(err)
	}

	// a non-Pid target is declined before anything starts
	if _, dErr := tuiDeliverEventsHandler([]native.Value{term, native.NewString("x")}, nil, nil, reg); dErr == nil ||
		!strings.Contains(dErr.Error(), "expected a Pid target") {
		t.Fatalf("non-pid target = %v", dErr)
	}

	if _, dErr := tuiDeliverEventsHandler([]native.Value{term, native.NewPid(proc)}, nil, nil, reg); dErr != nil {
		t.Fatal(dErr)
	}
	// events flow into the mailbox as the §2.4 tagged maps
	vb.Inject(tuikit.Event{Tag: "key", Key: "a", Char: "a"})
	msg, ok, pErr := proc.PopFront(2*time.Second, true)
	if pErr != nil || !ok {
		t.Fatalf("no delivered event: %v %v", ok, pErr)
	}
	mp, _ := native.AsMap(msg)
	if tag, _ := mp.Get("tag"); func() string { s, _ := tag.AsConcreteString(); return s }() != "key" {
		t.Fatalf("delivered event = %v", msg)
	}

	// the stream is exclusive: a second delivery and read-event decline
	if _, dErr := tuiDeliverEventsHandler([]native.Value{term, native.NewPid(proc)}, nil, nil, reg); dErr == nil ||
		!strings.Contains(dErr.Error(), "already being delivered") {
		t.Fatalf("double delivery = %v", dErr)
	}
	if _, rErr := tuiReadEventHandler([]native.Value{term}, nil, nil, reg); rErr == nil ||
		!strings.Contains(rErr.Error(), "deliver-events owns the stream") {
		t.Fatalf("read-event during delivery = %v", rErr)
	}

	// a dead target releases the stream for a new delivery
	proc.Close()
	vb.Inject(tuikit.Event{Tag: "key", Key: "b", Char: "b"})
	deadline := time.Now().Add(5 * time.Second)
	for ts, _ := asTerminal(term); ts.delivering.Load(); {
		if time.Now().After(deadline) {
			t.Fatal("delivery never released after the target died")
		}
		time.Sleep(5 * time.Millisecond)
	}
	proc2 := core.NewProcess(reg.Procs, 8, core.OverflowBlock)
	if err := reg.Procs.Insert(proc2); err != nil {
		t.Fatal(err)
	}
	if _, dErr := tuiDeliverEventsHandler([]native.Value{term, native.NewPid(proc2)}, nil, nil, reg); dErr != nil {
		t.Fatalf("re-delivery after release = %v", dErr)
	}
	vb.Inject(tuikit.Event{Tag: "key", Key: "c", Char: "c"})
	if _, ok, _ := proc2.PopFront(2*time.Second, true); !ok {
		t.Fatal("re-delivery received nothing")
	}

	// closing the terminal ends the pump; a fresh delivery is declined
	// on the closed handle
	ts, _ := asTerminal(term)
	if err := ts.close(); err != nil {
		t.Fatal(err)
	}
	if _, dErr := tuiDeliverEventsHandler([]native.Value{term, native.NewPid(proc2)}, nil, nil, reg); dErr == nil ||
		!strings.Contains(dErr.Error(), "closed") {
		t.Fatalf("closed-terminal delivery = %v", dErr)
	}
}

// The §11.7 reservation: alt-screen: is an accepted KEY (true = the
// implemented takeover), false is loudly unsupported until the inline
// tier lands, and non-Booleans are the usual option error.
func TestTuiAltScreenReservation(t *testing.T) {
	// open: the reservation gates before backend acquisition
	reg := tcReg(t)
	for _, c := range []struct{ src, want string }{
		{`Tui.open {alt-screen: false}`, "reserved but not yet implemented"},
		{`Tui.open {alt-screen: 5}`, "alt-screen: must be a Boolean"},
	} {
		_, err := runTuiStepsOn(t, tcReg(t), []string{`import "boru:tui"`, c.src})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s = %v, want %q", c.src, err, c.want)
		}
	}
	_ = reg
	// run: same gate through the app config; alt-screen: true is the
	// accepted spelling of today's behaviour
	base := `update: ([s:Map e:Map] => [s])  view: ([s:Map] => [Tui.spacer])`
	_, err := runTuiStepsOn(t, tcReg(t), []string{`import "boru:tui"`,
		`Tui.run {alt-screen: false  ` + base + `}`})
	if err == nil || !strings.Contains(err.Error(), "reserved but not yet implemented") {
		t.Fatalf("run alt-screen false = %v", err)
	}
	vb := tuikit.NewVirtualBackend(4, 2)
	vb.Inject(tuikit.Event{Tag: "key", Key: "c", Char: "c", Mods: []string{"ctrl"}})
	reg2 := trcRegWithBackend(t, vb)
	out, rErr := runTuiStepsOn(t, reg2, []string{`import "boru:tui"`,
		`Tui.run {alt-screen: true  ` + base + `}`})
	if rErr != nil || len(out) == 0 {
		t.Fatalf("run alt-screen true = %v", rErr)
	}
}
