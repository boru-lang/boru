package lang

import "testing"

// Stage-2 test-test pins (voxgig zero-refusals plan): the boru:test imperative
// harness — `[ body… ] "name" Test.test end` — already declares its closure
// shape (CallableSpec on test-test, lang/go/modules/test.go) and re-enters the
// VM through InvokeBody, but every corpus body still refused as "code-body
// word test-test (Stage 2)". The masked inner leaf: an assertion arg of
// DYNAMIC modality (a user-poly / declared-Any fn result) reaching a module
// native referenced by `/v` in an export map (`equal: assert-equal/v`).
// Such a reference carries body-less sigs and a real Go handler, so
// execFnDefLiteral's wrapper branch deliberately skips it — and the final
// pure-stack dispatch built its MatchResult WITHOUT Reg. tryRecordPoly then
// resolved matchReg to the MAIN registry, where `assert-equal` is not a
// builtin, declined, and RecordCall refused "dynamic input at assert-equal".
//
// The fix (engine.go execFnDefLiteral, pure-stack tail): a foreign-registry
// FnDef ref sets match.Reg = fnDef.Registry, exactly like the trivial-
// delegation branch above it. The recorder's poly re-match then validates the
// matched sig against the sub-registry's own binding (pointer identity) and
// the VM re-matches over PolyRef.Reg — the same MatchSignature first-match
// the interpreter's dispatch takes.
//
// The differential corpus is blind to these off-corpus shapes, so pin them
// (the memory-noted RunCompiledStrict discipline): compile+parity for the
// newly-compiling shapes, sound-fallback for a body that must still refuse.

// The mechanism in isolation, no test body: a dynamic (declared-Any user-fn)
// result feeding the module-native ref `Assert.equal` at module scope must
// lower (OpCallNativePoly over the boru:test sub-registry) with no island.
func TestModuleNativeRefDynamicArgPolyCompiles(t *testing.T) {
	loopCarriedCompilesClean(t, `import "boru:test" end
def blur fn [[v:Any] [Any] [v]]
(blur (2 add 2)) 4 Assert.equal end
Test.fail-count end`)
}

// The RAISING path of the same dispatch: the runtime re-match runs the same
// assert-equal handler, so the assertion_failure is identical on both
// surfaces (error parity, not just value parity).
func TestModuleNativeRefDynamicArgPolyRaisesAlike(t *testing.T) {
	stage1aSound(t, `import "boru:test" end
def blur fn [[v:Any] [Any] [v]]
(blur 5) 4 Assert.equal end`)
}

// A Test.test body with a PASSING dynamic-arg assertion: the body compiles
// to a closure unit, the harness runs it through InvokeBody, and
// Test.fail-count reads 0 on both surfaces.
func TestTestBodyPassingAssertCompiles(t *testing.T) {
	loopCarriedCompilesClean(t, `import "boru:test" end
def blur fn [[v:Any] [Any] [v]]
[ (blur (1 add 2)) 3 Assert.equal end ] "adds" Test.test end
Test.fail-count end`)
}

// A FAILING assertion inside the body: the assertion raises inside the
// compiled closure, the harness bookkeeping (runCase) traps it, and the
// whole program's observable state — fail-count AND the report text with
// the formatted failure row — is identical on both surfaces.
func TestTestBodyFailingAssertParity(t *testing.T) {
	loopCarriedCompilesClean(t, `import "boru:test" end
def blur fn [[v:Any] [Any] [v]]
[ (blur 2) 3 Assert.equal end ] "bad" Test.test end
[ (blur 3) 3 Assert.equal end ] "good" Test.test end
[ (Test.fail-count) (Test.report) ]`)
}

// A body calling a USER fn (the corpus shape: library calls inside the case
// body): the call compiles inside the closure unit; fail-count parity after.
func TestTestBodyUserFnCallCompiles(t *testing.T) {
	loopCarriedCompilesClean(t, `import "boru:test" end
def triple fn [[n:Integer] [Integer] [n 3 mul]]
[ (triple 4) 12 Assert.equal end ] "triples" Test.test end
Test.fail-count end`)
}

// NEGATIVE: a body hitting the dot-METHOD leaf (`rig.int 1 6` — method
// dispatch on a Map whose members are fn values, the prop-spec gen-body
// blocker) must still refuse and refuse and be interpreted — identical results,
// fail-count included.
func TestTestBodyDotMethodStaysSound(t *testing.T) {
	// Legacy refusal+fallback-parity contract: pins the one-release
	stage1aSound(t, `import "boru:test" end
def mkrig fn [[] [Map] [ {int: ([a:Integer b:Integer] => [a b add])} ]]
def rig (mkrig)
[ (rig.int 1 6) 7 Assert.equal end ] "dotm" Test.test end
Test.fail-count end`)
}
