package compiler

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// kept_defs_test.go covers the kept-defs latch (kept_defs.go) and its proof
// side (kept_defs_scan.go), NUR210: where the latch arms, what it poisons, how
// a unit carries it to where the unit runs, and which computed bodies the
// token proof clears.

func keptDefsNative(t *testing.T, r *core.Registry, name string, inCheck bool) {
	t.Helper()
	var opts []core.GoOpt
	if inCheck {
		opts = append(opts, core.RunInCheck())
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: name,
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny},
			Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return a, nil
			}, opts...),
			BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

func keptDefsUserFn(r *core.Registry, name string, body ...core.Value) {
	r.Register(name, core.Signature{Args: []*core.Type{core.TInteger}, Impl: core.Boru(body), BarrierPos: -1})
}

func words(names ...string) []core.Value {
	out := make([]core.Value, len(names))
	for i, n := range names {
		out[i] = core.NewWord(n)
	}
	return out
}

func TestKeptDefsLatchArmsAtTheShallowestDepth(t *testing.T) {
	es := NewEmitState()
	es.units = append(es.units, &emitUnit{})
	es.latchKeptDefs("do")
	if es.keptDefsLevel != 2 || es.keptDefsWord != "do" {
		t.Fatalf("armed at the unit's depth: level=%d word=%q", es.keptDefsLevel, es.keptDefsWord)
	}
	es.units = es.units[:1]
	es.latchKeptDefs("each")
	if es.keptDefsLevel != 1 || es.keptDefsWord != "each" {
		t.Errorf("a shallower arm replaces a deeper one: level=%d word=%q", es.keptDefsLevel, es.keptDefsWord)
	}
	es.units = append(es.units, &emitUnit{})
	es.latchKeptDefs("do")
	if es.keptDefsLevel != 1 || es.keptDefsWord != "each" {
		t.Errorf("a deeper arm never narrows a shallower one: level=%d word=%q", es.keptDefsLevel, es.keptDefsWord)
	}
}

func TestKeptDefsObserverPoisons(t *testing.T) {
	es := NewEmitState()
	es.noteKeptDefsObserver("the read of `x`")
	if es.armReadCompileFailure != "" {
		t.Fatal("no latch armed: an observer poisons nothing")
	}
	es.latchKeptDefs("do")
	es.trapAt = 3
	es.noteKeptDefsObserver("the read of `x`")
	if es.armReadCompileFailure != "" {
		t.Fatal("after a terminal trap the observer is unreachable")
	}
	es.trapAt = 0
	es.noteKeptDefsObserver("the read of `x`")
	if !strings.Contains(es.armReadCompileFailure, "do: a computed body keeps its defs") || !strings.Contains(es.armReadCompileFailure, "the read of `x`") {
		t.Fatalf("the first observer poisons: %q", es.armReadCompileFailure)
	}
	first := es.armReadCompileFailure
	es.noteKeptDefsObserver("a user fn call")
	if es.armReadCompileFailure != first {
		t.Error("the first poison wins")
	}
}

func TestKeptDefsObserverNotesOpenRecursiveCalls(t *testing.T) {
	es := NewEmitState()
	es.fnRecs = []*fnUnitRec{{calledOpen: true}, {}}
	es.openUnitRecs = []int{0, 1}
	es.noteKeptDefsObserver("the read of `n`")
	if es.fnRecs[0].openCallObserver != "the read of `n`" || es.fnRecs[1].openCallObserver != "" {
		t.Fatalf("only a unit a recursive call reached notes the observer: %+v %+v", es.fnRecs[0], es.fnRecs[1])
	}
	es.noteKeptDefsObserver("a user fn call")
	if es.fnRecs[0].openCallObserver != "the read of `n`" {
		t.Error("the first observer is the one kept")
	}
	if es.armReadCompileFailure != "" {
		t.Error("no latch armed: noting is not poisoning")
	}
}

func TestKeptDefsObserverEvent(t *testing.T) {
	for want, ev := range map[string]EmitEvent{
		"a user fn call":                   {kind: evCallUser},
		"the fn-value apply at `apply`":    {kind: evCall, call: emitCall{word: "apply", dynApply: 1}},
		"the fn-value apply at `mixed`":    {kind: evCall, call: emitCall{word: "mixed", dynMixed: true}},
		"the fn-value apply at `m.method`": {kind: evCall, call: emitCall{word: "m.method", dynMethod: &DynMethodSpec{}}},
	} {
		if got := keptDefsObserverEvent(&ev); got != want {
			t.Errorf("observer %q, got %q", want, got)
		}
	}
	for _, ev := range []EmitEvent{{kind: evCall, call: emitCall{word: "add"}}, {kind: evStore}, {kind: evFallback}} {
		if got := keptDefsObserverEvent(&ev); got != "" {
			t.Errorf("%+v observes nothing stale, got %q", ev.kind, got)
		}
	}
}

func TestKeptDefsEventArmsAfterACallOfAKeptDefsUnit(t *testing.T) {
	es := NewEmitState()
	es.fnRecs = []*fnUnitRec{{runsKeptDefs: "do", finished: true}, {}}
	// A call of the unit is itself checked first (nothing armed: no poison),
	// then arms the latch for what follows.
	es.keptDefsEvent(&EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 0}})
	if es.armReadCompileFailure != "" || es.keptDefsLevel != 1 || es.keptDefsWord != "do" {
		t.Fatalf("the call arms, never poisons itself: poison=%q level=%d", es.armReadCompileFailure, es.keptDefsLevel)
	}
	es.keptDefsEvent(&EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 1}})
	if !strings.Contains(es.armReadCompileFailure, "a user fn call") {
		t.Errorf("a later call observes the armed latch: %q", es.armReadCompileFailure)
	}
	// A call reaching a unit still being recorded marks it.
	es.keptDefsEvent(&EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 1}})
	if !es.fnRecs[1].calledOpen {
		t.Error("a call of an open unit marks it calledOpen")
	}
	// An out-of-range unit index is left alone.
	es.keptDefsEvent(&EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 9}})
}

func TestKeptDefsInvoker(t *testing.T) {
	es := NewEmitState()
	es.fnRecs = []*fnUnitRec{{runsKeptDefs: "each"}, {}}
	if w := es.keptDefsInvoker(&EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 0}}); w != "each" {
		t.Errorf("a direct call of a kept-defs unit runs it: %q", w)
	}
	if w := es.keptDefsInvoker(&EmitEvent{kind: evCallUser, uc: emitUserCall{unit: 1}}); w != "" {
		t.Errorf("no unit runs a kept-defs body yet: %q", w)
	}
	if w := es.keptDefsInvoker(&EmitEvent{kind: evFallback}); w != "" {
		t.Errorf("with no kept-defs unit an island invokes nothing: %q", w)
	}
	es.keptDefsUnitWord = "do"
	es.consts = []core.Value{core.NewInteger(1), {Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}}
	for name, ev := range map[string]EmitEvent{
		"an island":                      {kind: evFallback},
		"a fn-value apply":               {kind: evCall, call: emitCall{dynApply: 1}},
		"a mixed window":                 {kind: evCall, call: emitCall{dynMixed: true}},
		"a method apply":                 {kind: evCall, call: emitCall{dynMethod: &DynMethodSpec{}}},
		"a native over a fn const":       {kind: evCall, call: emitCall{ops: []EmitOperand{{kind: opConst, idx: 1}}}},
		"a native over a run-time value": {kind: evCall, call: emitCall{ops: []EmitOperand{EventOperand(3, 0)}}},
	} {
		if w := es.keptDefsInvoker(&ev); w != "do" {
			t.Errorf("%s may invoke a kept-defs unit: %q", name, w)
		}
	}
	for name, ev := range map[string]EmitEvent{
		"a native over data":     {kind: evCall, call: emitCall{ops: []EmitOperand{{kind: opConst, idx: 0}, typeOperand(0)}}},
		"a native over no const": {kind: evCall, call: emitCall{ops: []EmitOperand{{kind: opConst, idx: 7}}}},
		"a store":                {kind: evStore},
	} {
		if w := es.keptDefsInvoker(&ev); w != "" {
			t.Errorf("%s invokes nothing: %q", name, w)
		}
	}
}

func TestRunKeptDefsMarksOpenUnits(t *testing.T) {
	es := NewEmitState()
	es.runKeptDefs("do")
	if es.keptDefsUnitWord != "" || es.keptDefsLevel != 1 {
		t.Fatalf("at the top level no unit carries it; the latch arms: unitWord=%q level=%d", es.keptDefsUnitWord, es.keptDefsLevel)
	}
	es = NewEmitState()
	es.fnRecs = []*fnUnitRec{{}, {runsKeptDefs: "each"}}
	es.openUnitRecs = []int{0, 1}
	es.units = append(es.units, &emitUnit{}, &emitUnit{})
	es.runKeptDefs("do")
	if es.fnRecs[0].runsKeptDefs != "do" || es.fnRecs[1].runsKeptDefs != "each" || es.keptDefsUnitWord != "do" || es.keptDefsLevel != 3 {
		t.Errorf("every open unit runs it (the first word kept): %+v %+v unitWord=%q level=%d", es.fnRecs[0], es.fnRecs[1], es.keptDefsUnitWord, es.keptDefsLevel)
	}
}

func TestFinishKeptDefs(t *testing.T) {
	es := NewEmitState()
	es.units = append(es.units, &emitUnit{})
	plain := &fnUnitRec{}
	es.latchKeptDefs("do")
	es.finishKeptDefs(plain)
	if es.keptDefsLevel != 2 {
		t.Fatal("a unit that runs no kept-defs body leaves the latch alone")
	}
	named := &fnUnitRec{runsKeptDefs: "do"}
	es.finishKeptDefs(named)
	if es.keptDefsLevel != 0 || es.keptDefsWord != "" {
		t.Fatalf("a latch armed inside the unit is disarmed at its finish: level=%d", es.keptDefsLevel)
	}
	// A closure is invoked by the native it is handed to: it re-arms at the
	// enclosing depth at once.
	es.latchKeptDefs("do")
	closure := &fnUnitRec{runsKeptDefs: "do", closure: true}
	es.finishKeptDefs(closure)
	if es.keptDefsLevel != 1 || es.keptDefsWord != "do" {
		t.Fatalf("a closure re-arms at the enclosing depth: level=%d", es.keptDefsLevel)
	}
	// A shallower latch survives a deeper unit's finish.
	es.finishKeptDefs(named)
	if es.keptDefsLevel != 1 {
		t.Error("a latch armed outside the unit survives its finish")
	}
	// A recursive call observed before the finish learned the answer: poison.
	rec := &fnUnitRec{runsKeptDefs: "do", calledOpen: true, openCallObserver: "the read of `n`"}
	es.finishKeptDefs(rec)
	if !strings.Contains(es.armReadCompileFailure, "the read of `n`") {
		t.Errorf("a recursive call's observer poisons at the finish: %q", es.armReadCompileFailure)
	}
}

func TestKeptDefsHandedOn(t *testing.T) {
	es := NewEmitState()
	es.keptDefsHandedOn(&fnUnitRec{runsKeptDefs: "do"}, 1)
	es.keptDefsHandedOn(&fnUnitRec{closure: true}, 1)
	if es.keptDefsLevel != 0 {
		t.Fatal("only a closure that runs a kept-defs body hands the latch on")
	}
	es.keptDefsHandedOn(&fnUnitRec{closure: true, runsKeptDefs: "each"}, 2)
	es.keptDefsHandedOn(&fnUnitRec{closure: true, runsKeptDefs: "do"}, 3)
	if es.keptDefsLevel != 2 || es.keptDefsWord != "each" {
		t.Errorf("a memo hit arms where the closure runs, never deeper than an armed latch: level=%d word=%q", es.keptDefsLevel, es.keptDefsWord)
	}
}

func TestKeepsComputedDefs(t *testing.T) {
	once := &core.CallableSpec{BodyOnceKeepsDefs: true}
	multi := &core.CallableSpec{BodyMultiRunKeepsDefs: true}
	for name, c := range map[string]struct {
		spec *core.CallableSpec
		body core.Value
		want bool
	}{
		"a computed List body of do":         {once, core.NewCarrier(core.TList), true},
		"a gradual body of each":             {multi, core.NewDynamicCarrier(core.TAny), true},
		"an Any-typed body":                  {once, core.NewCarrier(core.TAny), true},
		"a Function callback":                {multi, core.NewCarrier(core.TFunction), false},
		"a concrete body":                    {once, core.NewList(words("add")), false},
		"a word that keeps no defs":          {&core.CallableSpec{}, core.NewCarrier(core.TList), false},
		"a typeless carrier may be anything": {once, core.Value{Carrier: true}, true},
	} {
		if got := keepsComputedDefs(c.spec, c.body); got != c.want {
			t.Errorf("%s: keepsComputedDefs=%v, want %v", name, got, c.want)
		}
	}
}

func TestDynRegionMayBeFn(t *testing.T) {
	if (&lowerer{}).dynRegionMayBeFn(1) {
		t.Error("a lowerer with no recorder answers no")
	}
	es := NewEmitState()
	es.eventInfo[1] = eventFlags{dynBodyResult: true, regionMayBeFn: true}
	es.eventInfo[2] = eventFlags{regionMayBeFn: true}
	lw := &lowerer{es: es}
	if !lw.dynRegionMayBeFn(1) || lw.dynRegionMayBeFn(2) {
		t.Error("only a dyn-body region that may leave a callable")
	}
}

func TestKeptDefsScan(t *testing.T) {
	r := newTestRegistry(t)
	keptDefsNative(t, r, "add", false)
	keptDefsNative(t, r, "def", true)
	keptDefsNative(t, r, "gen", true)
	keptDefsUserFn(r, "inc", core.NewWord("n"), core.NewWord("add"), core.NewInteger(1))
	keptDefsUserFn(r, "local", core.NewWord("def"), core.NewWord("t"), core.NewInteger(0), core.NewWord("t"))
	keptDefsUserFn(r, "bad", core.NewWord("undef"), core.NewWord("x"))
	keptDefsUserFn(r, "rec", core.NewWord("rec"))
	core.InstallDef(r, "k", core.NewInteger(1))
	core.InstallDef(r, "code", core.NewList(words("undef", "x")))
	scan := func(vs ...core.Value) bool {
		sc := &keptDefsScan{r: r, seen: map[string]bool{}}
		return sc.all(vs)
	}
	m := core.NewOrderedMap()
	m.Set("a", core.NewList(words("undef")))
	nested := core.NewInteger(1)
	for i := 0; i < keptDefsScanDepth+1; i++ {
		nested = core.NewList([]core.Value{nested})
	}
	for name, c := range map[string]struct {
		toks []core.Value
		want bool
	}{
		"a native over literals":              {[]core.Value{core.NewWord("add"), core.NewInteger(1)}, true},
		"the literal words":                   {words("true", "false", "none"), true},
		"a bound value read":                  {words("k"), true},
		"a user fn over its param":            {words("inc", "inc"), true},
		"a fn's own def is its frame's":       {words("local"), true},
		"a recursive fn is visited once":      {words("rec"), true},
		"a paren group":                       {[]core.Value{core.NewParenExpr(words("add"))}, true},
		"a Integer carrier":                   {[]core.Value{core.NewCarrier(core.TInteger)}, true},
		"a fn value":                          {[]core.Value{{Parent: core.TFunction, Data: core.FnDefInfo{Signatures: []core.Signature{{Impl: core.Boru(words("add"))}}}}}, true},
		"a def at the body's top level":       {words("def"), false},
		"an undef":                            {words("undef"), false},
		"an unresolved name":                  {words("zz"), false},
		"a check-mode native":                 {words("gen"), false},
		"a user fn that undefs":               {words("bad"), false},
		"a bound list run as code":            {words("code"), false},
		"a map holding an undef":              {[]core.Value{core.NewMap(m)}, false},
		"a reach":                             {[]core.Value{core.NewReachFromKeys(core.NewWord("m"), []core.Value{core.NewAtom("f")})}, false},
		"a splice":                            {[]core.Value{core.NewSplice(core.NewInteger(1))}, false},
		"a List carrier":                      {[]core.Value{core.NewCarrier(core.TList)}, false},
		"an interpolation":                    {[]core.Value{core.NewInterpString([]core.InterpPart{{Lit: "a"}})}, false},
		"a closure":                           {[]core.Value{{Parent: core.TFunction, Data: core.ClosurePayload{}}}, false},
		"a body nested past the walk's bound": {[]core.Value{nested}, false},
	} {
		if got := scan(c.toks...); got != c.want {
			t.Errorf("%s: scan=%v, want %v", name, got, c.want)
		}
	}
	// A nil-map payload walks as empty.
	if !scan(core.Value{Parent: core.TMap, Data: core.MapPayload{}}) {
		t.Error("an empty map payload binds nothing")
	}
}

func TestProvenBodyTokens(t *testing.T) {
	r := newTestRegistry(t)
	keptDefsNative(t, r, "add", false)
	keptDefsNative(t, r, "def", true)
	es := NewEmitState()
	// A factory's declared List result resolves through its producing
	// event; the dynamic stand-in keeps a typed-list carrier's type-body
	// identity out of this fixture's way.
	body := core.NewDynamicCarrier(core.TList)
	body.ID = "mk-out"
	if _, ok := es.provenBodyTokens(body); ok {
		t.Fatal("an unresolvable operand proves nothing")
	}
	es.consts = []core.Value{core.NewList(words("add")), core.NewInteger(3), core.NewList(words("def"))}
	es.fnRecs = []*fnUnitRec{{outOps: []EmitOperand{{kind: opConst, idx: 0}}}, {outOps: []EmitOperand{{kind: opConst, idx: 1}}}, {outOps: []EmitOperand{{kind: opConst, idx: 2}}}}
	es.frames[0] = []EmitEvent{
		{seq: 1, kind: evCallUser, uc: emitUserCall{unit: 0, nout: 1}},
		{seq: 2, kind: evCallUser, uc: emitUserCall{unit: 1, nout: 1}},
		{seq: 3, kind: evCallUser, uc: emitUserCall{unit: 2, nout: 1}},
	}
	es.producedBy[body.ID] = producer{seq: 1}
	toks, ok := es.provenBodyTokens(body)
	if !ok || len(toks) != 1 {
		t.Fatalf("a factory call returning a const list proves its tokens: %v %v", toks, ok)
	}
	if !es.bodyBindsNothing(r, body) {
		t.Error("`[add]` binds nothing")
	}
	if es.bodyPlainData(body) {
		t.Error("`[add]` is not plain data: a word")
	}
	es.producedBy[body.ID] = producer{seq: 3}
	if es.bodyBindsNothing(r, body) {
		t.Error("`[def]` binds")
	}
	es.producedBy[body.ID] = producer{seq: 2}
	if _, ok := es.provenBodyTokens(body); ok || es.bodyPlainData(body) || es.bodyBindsNothing(r, body) {
		t.Error("a const that is not a list proves nothing")
	}
	es.consts = append(es.consts, core.NewList([]core.Value{core.NewInteger(1), core.NewString("a")}))
	es.fnRecs = append(es.fnRecs, &fnUnitRec{outOps: []EmitOperand{{kind: opConst, idx: 3}}})
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 4, kind: evCallUser, uc: emitUserCall{unit: 3, nout: 1}})
	es.producedBy[body.ID] = producer{seq: 4}
	if !es.bodyPlainData(body) {
		t.Error("`[1 'a']` is plain data")
	}
	es.producedBy[body.ID] = producer{seq: 1, idx: 1}
	if _, ok := es.provenBodyTokens(body); ok {
		t.Error("a second result proves nothing")
	}
	es.frames[0] = append(es.frames[0], EmitEvent{seq: 5, kind: evCall, call: emitCall{word: "get", nout: 1}})
	es.producedBy[body.ID] = producer{seq: 5}
	if _, ok := es.provenBodyTokens(body); ok {
		t.Error("a native's result proves nothing")
	}
	delete(es.producedBy, body.ID)
	es.units[0].localByID[body.ID] = 0
	if _, ok := es.provenBodyTokens(body); ok {
		t.Error("a local with no producer proves nothing")
	}
	// A capture slot of the open unit whose value a factory call produced
	// (the promoted value-def read inside a body) proves through its
	// producer.
	es.units = append(es.units, &emitUnit{capID: map[string]bool{body.ID: true}, localByID: map[string]int{body.ID: 0}})
	es.producedBy[body.ID] = producer{seq: 1}
	if toks, ok := es.provenBodyTokens(body); !ok || len(toks) != 1 {
		t.Errorf("a captured factory result proves its tokens: %v %v", toks, ok)
	}
	es.producedBy[body.ID] = producer{seq: 1, idx: 1}
	if _, ok := es.provenBodyTokens(body); ok {
		t.Error("a captured second result proves nothing")
	}
}

func TestBodyProvenFn(t *testing.T) {
	es := NewEmitState()
	fnVal := core.Value{Parent: core.TFunction, Data: core.FnDefInfo{Name: "g"}}
	if !es.bodyProvenFn(fnVal) {
		t.Error("a const fn value is a proven callback")
	}
	unresolved := core.NewCarrier(core.TList)
	unresolved.ID = "nowhere"
	if es.bodyProvenFn(unresolved) {
		t.Error("an unresolvable body is not")
	}
}

func TestEventRunLast(t *testing.T) {
	run := []EmitOperand{EventOperand(4, 0), EventOperand(4, 1)}
	if !eventRunLast(run, 4) {
		t.Error("the event's run, in result order, is last")
	}
	for name, ops := range map[string][]EmitOperand{
		"a value after the run":  append(append([]EmitOperand{}, run...), typeOperand(0)),
		"another event's result": {EventOperand(5, 0)},
		"results out of order":   {EventOperand(4, 1), EventOperand(4, 0)},
	} {
		if eventRunLast(ops, 4) {
			t.Errorf("%s: not the run alone", name)
		}
	}
}
