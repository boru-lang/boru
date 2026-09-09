package eng

import (
	"reflect"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// CheckState's lifecycle is hand-maintained in two places — Begin() resets
// the per-pass fields, Clone() deep-copies the map/slice fields — and a
// field missed in either is a SILENT cross-pass leak (Begin's own comment
// memorialises exactly such a bug on Compiling). This gate forces every
// field to be CLASSIFIED: a new field not listed below fails loudly with
// instructions instead of leaking.
func TestCheckStateLifecycleComplete(t *testing.T) {
	// Fields Begin() resets to their zero value (per-pass state).
	resetByBegin := map[string]bool{
		"Diagnostics": true, "StepCount": true, "BudgetTripped": true,
		"SuppressedRuntimeError": true, "AmbiguousGradualSplit": true,
		"DefsInstalled": true, "DefsUsed": true, "FnNameStack": true,
		"BindLedger": true, "PendingBindPos": true, "PassEndCleanups": true,
		"FnBinders": true, "FnCallGraph": true, "ContextTypes": true, "CtxShapes": true,
		"MethodShapes": true, "PendingMethodApply": true, "FnShapes": true,
		"InflightBails": true, "FnNameInflight": true, "SuppressBodyErrors": true,
		"FnAnalysisCounts": true, "FnBodyDepth": true, "CallShapeDepth": true,
		"FnBodyChecked":   true,
		"PendingFnBodies": true,
		"CaughtBodyDepth": true, "NestedBodyDepth": true, "CondBodyDepth": true,
		"RolledBackBodyDepth":      true,
		"SpecBaselines":            true,
		"LoopBodyDepth":            true,
		"CodeEffectDepth":          true,
		"Compiling":                true,
		"FnCarrierReadSubstituted": true,
		"ParenPlacedFnIDs":         true,
		"ParenReSteppedFnIDs":      true,
		"ArgsFrameUnnamed":         true,
	}
	// Fields Begin() resets to a canonical NON-zero per-pass value.
	resetByBeginToCanonical := map[string]string{
		"Emit": "reset to the inactive no-op recorder (never nil) — a compile pass installs a real *EmitState after Begin",
	}
	// Fields Begin() deliberately does NOT reset.
	persistent := map[string]string{
		"Mode":       "set true by Begin itself; cleared by the returned done()",
		"StepBudget": "configuration with the -1 sentinel, resolved per run",
		"CurCallPos": "transient cursor overwritten per dispatch",
		"CurWordPos": "transient cursor overwritten per dispatch, and the write is " +
			"unconditional and immediately adjacent: execMatch sets it from " +
			"e.currentPos() on the line above the handler call, and a handler is the " +
			"only reader. So no read can ever observe a previous pass's value, which " +
			"is what resetting it would protect against",
		"FnSummaries": "caller-managed memo: CompileCheck nils it per emit pass and SetStrictCheck nils it on a mode change; a plain Check / bare Begin reuses them intentionally",
		"FnInflight":  "caller-managed alongside FnSummaries",
		"Strict":      "strict-mode configuration set by the caller before Begin",
		"ModelEffects": "a property of the registry's ROLE, not of a pass: a module " +
			"sub-registry created while its parent ran a pure check pass models its " +
			"effects for its whole life, and the registry never runs a check pass of " +
			"its own (runModuleBodyCover deliberately withholds Mode). Resetting it " +
			"in Begin would be inert here and wrong if a body ever did check",
	}

	st := reflect.TypeOf(core.CheckState{})
	for i := 0; i < st.NumField(); i++ {
		name := st.Field(i).Name
		_, isReset := resetByBegin[name]
		_, isCanonical := resetByBeginToCanonical[name]
		_, isPersistent := persistent[name]
		switch {
		case (isReset && isPersistent) || (isReset && isCanonical) || (isCanonical && isPersistent):
			t.Errorf("CheckState.%s classified in more than one lifecycle bucket — pick one", name)
		case !isReset && !isCanonical && !isPersistent:
			t.Errorf("CheckState.%s is UNCLASSIFIED: add it to Begin()'s reset list (check.go) "+
				"and resetByBegin here, or document why it persists across passes in the "+
				"persistent list here — an unclassified field is a silent cross-pass leak", name)
		}
	}
	for name := range resetByBegin {
		if _, ok := st.FieldByName(name); !ok {
			t.Errorf("resetByBegin lists %s which is no longer a CheckState field — prune it", name)
		}
	}
	for name := range resetByBeginToCanonical {
		if _, ok := st.FieldByName(name); !ok {
			t.Errorf("resetByBeginToCanonical lists %s which is no longer a CheckState field — prune it", name)
		}
	}
	for name := range persistent {
		if _, ok := st.FieldByName(name); !ok {
			t.Errorf("persistent lists %s which is no longer a CheckState field — prune it", name)
		}
	}

	// Behavioral half: Begin() must actually zero every reset-classified
	// field. Populate a CheckState with non-zero values, Begin, and compare.
	c := &core.CheckState{Emit: compiler.NewEmitState()}
	populateNonZero(reflect.ValueOf(c).Elem())
	done := c.Begin()
	defer done()
	v := reflect.ValueOf(c).Elem()
	for i := 0; i < st.NumField(); i++ {
		name := st.Field(i).Name
		if !resetByBegin[name] {
			continue
		}
		if !v.Field(i).IsZero() {
			t.Errorf("Begin() does not reset CheckState.%s (classified reset-by-Begin) — add it to Begin's reset list", name)
		}
	}
	// The canonical-reset field: Begin must swap any armed recorder back to
	// the inactive no-op (never nil, never a leftover *EmitState).
	if c.Emit != core.TheInactiveEmit {
		t.Errorf("Begin() must reset CheckState.Emit to the inactive no-op recorder, got %T", c.Emit)
	}

	// Clone() must deep-copy every map/slice field: mutating the original
	// after cloning must not change the clone. (Emit is a shared recorder
	// reference by design — per-pass, reset to the inactive no-op by Begin.)
	orig := &core.CheckState{}
	populateNonZero(reflect.ValueOf(orig).Elem())
	clone := orig.Clone()
	mutateContainers(reflect.ValueOf(orig).Elem())
	ov, cv := reflect.ValueOf(orig).Elem(), reflect.ValueOf(clone).Elem()
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		switch f.Type.Kind() {
		case reflect.Map, reflect.Slice:
			if reflect.DeepEqual(ov.Field(i).Interface(), cv.Field(i).Interface()) {
				t.Errorf("Clone() shares CheckState.%s with the original (mutation bled through) — deep-copy it in Clone", f.Name)
			}
		}
	}
}

// populateNonZero fills every settable field with a minimal non-zero value
// (one-element maps/slices, 1, true, non-empty string) so reset/clone
// behavior is observable.
func populateNonZero(v reflect.Value) {
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int, reflect.Int64:
			f.SetInt(1)
		case reflect.String:
			f.SetString("x")
		case reflect.Slice:
			f.Set(reflect.MakeSlice(f.Type(), 1, 1))
		case reflect.Map:
			m := reflect.MakeMap(f.Type())
			k := reflect.New(f.Type().Key()).Elem()
			if k.Kind() == reflect.String {
				k.SetString("k")
			}
			m.SetMapIndex(k, reflect.New(f.Type().Elem()).Elem())
			f.Set(m)
		case reflect.Pointer:
			f.Set(reflect.New(f.Type().Elem()))
		case reflect.Struct:
			populateNonZero(f)
		}
	}
}

// mutateContainers adds a second distinguishable entry to every map/slice so
// a shallow-shared clone diverges from a deep copy.
func mutateContainers(v reflect.Value) {
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		switch f.Kind() {
		case reflect.Slice:
			f.Set(reflect.AppendSlice(f, reflect.MakeSlice(f.Type(), 1, 1)))
		case reflect.Map:
			k := reflect.New(f.Type().Key()).Elem()
			switch k.Kind() {
			case reflect.String:
				k.SetString("k2")
			case reflect.Pointer:
				// Pointer-keyed maps (CtxShapes): a fresh non-nil pointer is
				// a distinguishable second key.
				k.Set(reflect.New(f.Type().Key().Elem()))
			case reflect.Struct:
				// Struct-keyed maps (FnBodyChecked's SrcPos): populateNonZero
				// seeds the map with the ZERO key, so a non-zero one is the
				// distinguishable second. Without this arm the mutation
				// silently did not happen and the clone check PASSED for a
				// map Clone was in fact sharing — the gate's own blind spot,
				// found when the first struct-keyed field arrived.
				populateNonZero(k)
			default:
				continue
			}
			f.SetMapIndex(k, reflect.New(f.Type().Elem()).Elem())
		}
	}
}
