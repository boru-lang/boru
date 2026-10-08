package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/native"
	"github.com/boru-lang/boru/lang/go/policy"
)

// io_env_test.go — IO.env's HOST-DEPENDENT surface. The spec rows
// (lang/spec/module-io.tsv §19) pin the uninstalled default, which is the
// only environment-independent thing there is; everything that depends on
// what the host installed is pinned here, through the map-backed fake.

// envFake is the deterministic environment these tests read. "" is
// deliberately present: an empty value is a REAL value and must be
// distinguishable from unset.
func envFake() capabilities.EnvOps {
	return capabilities.MapEnvOps{Vars: map[string]string{
		"HOME":   "/home/ada",
		"LANG":   "en_GB.UTF-8",
		"EMPTY":  "",
		"SECRET": "hunter2",
	}}
}

func runEnv(t *testing.T, a *lang.Boru, src string) any {
	t.Helper()
	res, err := runReference(t, a, `import "boru:io"  `+src)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	if len(res) != 1 {
		t.Fatalf("%s: returned %d values, want 1: %v", src, len(res), res)
	}
	return res[0]
}

func newEnvBoru(t *testing.T, opts lang.Options) *lang.Boru {
	t.Helper()
	a, err := lang.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// The lookup form returns the installed value.
func TestIOEnvLookup(t *testing.T) {
	a := newEnvBoru(t, lang.Options{Env: envFake()})
	if got := runEnv(t, a, `IO.env "HOME"`); got != "/home/ada" {
		t.Errorf(`IO.env "HOME" = %#v, want /home/ada`, got)
	}
}

// "" is a value, not absence: it must come back as the empty String and
// NOT as none, or a caller can never tell `X=` from an unset X.
func TestIOEnvEmptyValueIsNotUnset(t *testing.T) {
	a := newEnvBoru(t, lang.Options{Env: envFake()})
	if got := runEnv(t, a, `IO.env "EMPTY"`); got != "" {
		t.Errorf(`IO.env "EMPTY" = %#v, want ""`, got)
	}
	if got := runEnv(t, a, `typeof (IO.env "EMPTY")`); got != "EmptyString" {
		t.Errorf(`typeof (IO.env "EMPTY") = %#v, want EmptyString`, got)
	}
	if got := runEnv(t, a, `typeof (IO.env "NOPE")`); got != "None" {
		t.Errorf(`typeof (IO.env "NOPE") = %#v, want None`, got)
	}
}

// The bare form snapshots everything visible, sorted by name.
func TestIOEnvAll(t *testing.T) {
	a := newEnvBoru(t, lang.Options{Env: envFake()})
	if got := runEnv(t, a, `size (IO.env-all)`); got != int64(4) {
		t.Errorf("size (IO.env-all) = %#v, want 4", got)
	}
	if got := runEnv(t, a, `(IO.env-all) get "LANG"`); got != "en_GB.UTF-8" {
		t.Errorf(`(IO.env-all) get "LANG" = %#v, want en_GB.UTF-8`, got)
	}
	if got := runEnv(t, a, `(keys (IO.env-all)) get 0`); got != "EMPTY" {
		t.Errorf("first key = %#v, want EMPTY (names are sorted)", got)
	}
}

// Every call form reaches the lookup word. Worth pinning because a module
// export's argument is NOT visible from where the fn value resolves:
// `Mod.word arg` lowers to `( Mod dot word ) arg`, and the close paren is
// a forward-scan boundary. A single-signature word escapes the group and
// re-dispatches outside — the reason `env` shipped as one word per form
// rather than one word with a 0-arg overload. The engine no longer forces
// that choice (a paren expression resolving to a function word does not
// induce a call), but every call form still has to work, so this stays.
func TestIOEnvCallForms(t *testing.T) {
	a := newEnvBoru(t, lang.Options{Env: envFake()})
	for _, src := range []string{
		`IO.env "HOME"`,          // forward form (canonical)
		`"HOME" IO.env`,          // stack form
		`def h "HOME"  IO.env h`, // via a binding
	} {
		if got := runEnv(t, a, src); got != "/home/ada" {
			t.Errorf("%s = %#v, want /home/ada", src, got)
		}
	}
	// And the snapshot word answers in every form it has.
	if got := runEnv(t, a, `size (IO.env-all)`); got != int64(4) {
		t.Errorf("size (IO.env-all) = %#v, want 4", got)
	}
}

// No installation is not "read the real process environment": the runtime
// never reaches for ambient state the host did not hand it.
func TestIOEnvUninstalledSeesNothing(t *testing.T) {
	a := newEnvBoru(t, lang.Options{})
	if got := runEnv(t, a, `typeof (IO.env "HOME")`); got != "None" {
		t.Errorf(`uninstalled IO.env "HOME" = %#v, want None`, got)
	}
	if got := runEnv(t, a, `size (IO.env-all)`); got != int64(0) {
		t.Errorf("uninstalled size (IO.env-all) = %#v, want 0", got)
	}
}

// SetHostEnvOps(nil) clears a previously-installed view.
func TestSetHostEnvOpsNilClears(t *testing.T) {
	a := newEnvBoru(t, lang.Options{Env: envFake()})
	reg := a.NativeRegistry()
	if native.HostEnvOps(reg) == nil {
		t.Fatal("expected an installed EnvOps")
	}
	native.SetHostEnvOps(reg, nil)
	if native.HostEnvOps(reg) != nil {
		t.Fatal("after nil clear, HostEnvOps should be nil")
	}
	if got := runEnv(t, a, `size (IO.env-all)`); got != int64(0) {
		t.Errorf("after clear, size (IO.env-all) = %#v, want 0", got)
	}
}

// SetHostEnvOps with no policy installed leaves the ops unwrapped — the
// nil-policy arm of the policy-aware installer.
func TestSetHostEnvOpsNoPolicyUnwrapped(t *testing.T) {
	a := newEnvBoru(t, lang.Options{})
	reg := a.NativeRegistry()
	native.SetHostEnvOps(reg, envFake())
	// MapEnvOps is uncomparable (it holds a map), so identity is asserted
	// by TYPE: an unwrapped fake, not a permissionedEnvOps wrapper.
	if got := native.HostEnvOps(reg); !isMapEnvOps(got) {
		t.Errorf("with no policy the installed ops should be the fake itself, got %T", got)
	}
}

func isMapEnvOps(ops capabilities.EnvOps) bool {
	_, ok := ops.(capabilities.MapEnvOps)
	return ok
}

func envPolicy(t *testing.T, name string) policy.Policy {
	t.Helper()
	pol, err := policy.Load(name)
	if err != nil {
		t.Fatalf("load profile %s: %v", name, err)
	}
	return pol
}

// read-only allows env.read for an ALLOWLIST (LANG, TZ, BORU_*). A denied
// name reads as UNSET rather than raising: a program probing for an
// optional variable must take its default path, and an error would leak
// which names exist.
func TestIOEnvPolicyAllowlist(t *testing.T) {
	a := newEnvBoru(t, lang.Options{Env: envFake(), Policy: envPolicy(t, "read-only")})
	if got := runEnv(t, a, `IO.env "LANG"`); got != "en_GB.UTF-8" {
		t.Errorf(`allowed IO.env "LANG" = %#v, want en_GB.UTF-8`, got)
	}
	if got := runEnv(t, a, `typeof (IO.env "SECRET")`); got != "None" {
		t.Errorf(`denied IO.env "SECRET" = %#v, want None`, got)
	}
	// The snapshot is filtered by the same rule, so a denied name cannot
	// be recovered by enumerating instead of asking.
	if got := runEnv(t, a, `size (IO.env-all)`); got != int64(1) {
		t.Errorf("allowlisted size (IO.env-all) = %#v, want 1 (LANG only)", got)
	}
	if got := runEnv(t, a, `(keys (IO.env-all)) get 0`); got != "LANG" {
		t.Errorf("filtered snapshot key = %#v, want LANG", got)
	}
}

// sandbox denies the whole env scope: every name reads as unset and the
// snapshot is empty.
func TestIOEnvPolicyDeniesAll(t *testing.T) {
	a := newEnvBoru(t, lang.Options{Env: envFake(), Policy: envPolicy(t, "sandbox")})
	if got := runEnv(t, a, `typeof (IO.env "HOME")`); got != "None" {
		t.Errorf(`sandboxed IO.env "HOME" = %#v, want None`, got)
	}
	if got := runEnv(t, a, `size (IO.env-all)`); got != int64(0) {
		t.Errorf("sandboxed size (IO.env-all) = %#v, want 0", got)
	}
}

// `env: {install: false}` UNINSTALLS the scope. That is stronger than
// denying it: the capability slot is cleared, so there is nothing
// installed to decline. (The shipped compute/gen profiles do this, but
// they also forbid importing boru:io, so the arm is exercised through an
// inline profile that uninstalls env and nothing else.)
func TestIOEnvPolicyUninstalls(t *testing.T) {
	pol, err := policy.LoadInline(`{version:1 name:"env-off" scopes:{
		global:{words:{default:"allow"}}
		modules:{words:{default:"allow"}}
		env:{install:false}
	}}`)
	if err != nil {
		t.Fatal(err)
	}
	a := newEnvBoru(t, lang.Options{Env: envFake(), Policy: pol})
	if ops := native.HostEnvOps(a.NativeRegistry()); ops != nil {
		t.Errorf("compute uninstalls env: HostEnvOps = %T, want nil", ops)
	}
	if got := runEnv(t, a, `size (IO.env-all)`); got != int64(0) {
		t.Errorf("uninstalled-scope size (IO.env-all) = %#v, want 0", got)
	}
}

// OSEnvOps is the production view. The values are the real process
// environment, so the assertions are shape-only: whatever Go's os package
// reports for a name, IO.env reports the same.
func TestOSEnvOpsMatchesProcess(t *testing.T) {
	t.Setenv("BORU_ENV_PROBE", "probe-value")
	ops := capabilities.OSEnvOps{}
	if v, ok := ops.Get("BORU_ENV_PROBE"); !ok || v != "probe-value" {
		t.Errorf("OSEnvOps.Get = (%q, %v), want (probe-value, true)", v, ok)
	}
	if _, ok := ops.Get("BORU_DEFINITELY_UNSET_XYZZY"); ok {
		t.Error("OSEnvOps.Get reported an unset name as set")
	}
	var found bool
	for _, n := range ops.All() {
		if n == "BORU_ENV_PROBE" {
			found = true
		}
	}
	if !found {
		t.Error("OSEnvOps.All omitted a name that Get reports as set")
	}
	a := newEnvBoru(t, lang.Options{Env: ops})
	if got := runEnv(t, a, `IO.env "BORU_ENV_PROBE"`); got != "probe-value" {
		t.Errorf(`IO.env through OSEnvOps = %#v, want probe-value`, got)
	}
}

// A MODULE BODY sees the same environment and the same argument vector the
// top level sees. Both ride in through ModuleInheritedCaps (modules/io.go),
// alongside the shared stdin reader.
//
// Without the inheritance the failure was silent and asymmetric: a library
// that renders help wrapped to `IO.env "COLUMNS"`, or that logs the argv it
// was handed, read an EMPTY environment and an empty vector while its
// importer read the real ones. A capability the host DID install must not
// disappear one import deep — that is a wrong answer, not a compile failure, and it
// is exactly the shape boru:cli depends on.
func TestEnvAndArgsReachModuleBodies(t *testing.T) {
	a := newEnvBoru(t, lang.Options{Env: envFake(), ScriptArgs: []string{"alpha", "beta"}})
	res, err := a.Run(`import "boru:io"
import module [
  import "boru:io"
  def m-home fn [[] [Any] [ IO.env "HOME" ]]
  def m-argv fn [[] [List] [ IO.args ]]
  export "M" {home: m-home/v, argv: m-argv/v}
] end
print (M.home)
print (M.argv)
IO.env "HOME"`)
	if err != nil {
		t.Fatal(err)
	}
	if got := res[len(res)-1]; got != "/home/ada" {
		t.Errorf("top-level IO.env = %#v, want /home/ada", got)
	}
	var out strings.Builder
	a2 := newEnvBoru(t, lang.Options{Env: envFake(), ScriptArgs: []string{"alpha", "beta"}})
	a2.SetOutput(&out)
	if _, err := a2.Run(`import module [
  import "boru:io"
  def m-home fn [[] [Any] [ IO.env "HOME" ]]
  def m-argv fn [[] [List] [ IO.args ]]
  export "M" {home: m-home/v, argv: m-argv/v}
] end
print (M.home)
print (M.argv)`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/home/ada") {
		t.Errorf("module body IO.env printed %q, want the host's HOME", out.String())
	}
	if !strings.Contains(out.String(), `["alpha", "beta"]`) {
		t.Errorf("module body IO.args printed %q, want the host's argument vector", out.String())
	}
}

// The inheritance carries the POLICY-WRAPPED view, not a bypass: a profile
// that denies an environment name denies it inside a module body too.
func TestModuleBodyEnvStaysPolicyWrapped(t *testing.T) {
	pol, err := policy.Load("sandbox")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	a := newEnvBoru(t, lang.Options{Env: envFake(), Policy: pol})
	a.SetOutput(&out)
	if _, err := a.Run(`import module [
  import "boru:io"
  def m-home fn [[] [Any] [ IO.env "HOME" ]]
  export "M" {home: m-home/v}
] end
print (M.home)`); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "None" {
		t.Errorf("module body under sandbox printed %q, want None (the profile denies env.read)", out.String())
	}
}
