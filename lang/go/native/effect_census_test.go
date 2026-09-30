package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// effect_census_test.go pins which native words declare an observable
// effect (core.CompileSideEffect, side_effect.go, NUR356): the compiler runs
// every value-returning word that does not where the interpreter runs it
// later or never, so a missing declaration is a silent early effect.

func sigEffect(s *Signature) bool { return s.CompileEffect.Has(core.CompileSideEffect) }

// wordEffects reports, for word in r, whether every and whether any of its
// signatures declares the flag.
func wordEffects(t *testing.T, r *Registry, word string) (all, some bool) {
	t.Helper()
	fn := r.Lookup(word)
	if fn == nil {
		t.Fatalf("%s is not registered", word)
	}
	all = true
	for i := range fn.Signatures {
		if fn.Signatures[i].Fallback {
			continue
		}
		e := sigEffect(&fn.Signatures[i])
		all, some = all && e, some || e
	}
	return all, some
}

func TestSideEffectingDeclares(t *testing.T) {
	in := []NativeFunc{{Name: "a"}, {Name: "b", CompileEffect: CompileQuoteKey}}
	out := SideEffecting(in, "a")
	if out[0].CompileEffect.Has(core.CompileSideEffect) {
		t.Error("a quiet name stays quiet")
	}
	if !out[1].CompileEffect.Has(core.CompileSideEffect) || !out[1].CompileEffect.Has(core.CompileQuoteKey) {
		t.Errorf("b keeps its flags and gains the effect: %v", out[1].CompileEffect)
	}
	if in[1].CompileEffect.Has(core.CompileSideEffect) {
		t.Error("the input is not modified")
	}
}

// TestEffectCensusDefaultRegistry: the core words with an effect of their
// own declare it on every overload, the flex containers' in-place writes on
// theirs alone, and the pure words on none.
func TestEffectCensusDefaultRegistry(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"print", "gensym", "behave", "help", "describe", "spawn", "send", "receive", "register", "unregister", "call", "append"} {
		if all, _ := wordEffects(t, r, w); !all {
			t.Errorf("%s: every overload has an effect", w)
		}
	}
	for _, w := range []string{"push", "pop", "shift", "unshift", "set", "del"} {
		all, some := wordEffects(t, r, w)
		if !some || all {
			t.Errorf("%s: its flex overloads write in place, its plain ones do not (all=%v some=%v)", w, all, some)
		}
		fn := r.Lookup(w)
		for i := range fn.Signatures {
			s := &fn.Signatures[i]
			flex := false
			for _, a := range s.ArgTypes() {
				flex = flex || a != nil && (a.ConformsTo(TFlexList) || a.ConformsTo(TFlexMap) || a.ConformsTo(TFlexXml) ||
					a.ConformsTo(TWeakFlexList) || a.ConformsTo(TWeakFlexMap) || a.ConformsTo(TWeakFlexXml) || a.ConformsTo(TStore))
			}
			if sigEffect(s) && !flex {
				t.Errorf("%s: an overload over no flex container declares an effect: %v", w, s.ArgTypes())
			}
		}
	}
	for _, w := range []string{"sub", "mul", "get", "size", "keys", "flatten", "make", "self", "whereis", "state-of", "service", "sort"} {
		if _, some := wordEffects(t, r, w); some {
			t.Errorf("%s is quiet", w)
		}
	}
}

// TestEffectCensusModuleNatives: the IO, Net, Log and TimeUtil async words
// declare their effect; the stream-handle constructors do not.
func TestEffectCensusModuleNatives(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	io := IOModuleNativeFuncs(IOModuleTypes{})
	quiet := map[string]bool{"stdin": true, "stdout": true, "stderr": true}
	for _, n := range io {
		if got := n.CompileEffect.Has(core.CompileSideEffect); got == quiet[n.Name] {
			t.Errorf("IO %s: effect %v", n.Name, got)
		}
	}
	for _, ext := range IOWordExtensions(nil) {
		for i := range ext.Signatures {
			if !sigEffect(&ext.Signatures[i]) {
				t.Errorf("IO word extension %s: a filesystem read or write", ext.Name)
			}
		}
	}
	groups := map[string][]NativeFunc{
		"Net":      NetModuleNatives(MintFetchTypes(r)),
		"TimeUtil": TimeAsyncModuleNatives(MintTemporalModuleTypes(r)),
		"Log":      LogModuleNativeFuncs(NewLogSinkRegistry()),
		"logger":   loggerNatives(&loggerState{lsr: NewLogSinkRegistry()}),
		"span":     spanNatives(&spanState{}, NewLogSinkRegistry()),
	}
	for g, fns := range groups {
		if len(fns) == 0 {
			t.Errorf("%s: no natives", g)
		}
		for _, n := range fns {
			if !n.CompileEffect.Has(core.CompileSideEffect) {
				t.Errorf("%s %s: declares no effect", g, n.Name)
			}
		}
	}
	inst, err := buildInstrumentInstance(&instrumentState{name: "c", kind: "counter", lsr: NewLogSinkRegistry()})
	if err != nil || inst == nil {
		t.Fatalf("instrument: %v", err)
	}
}
