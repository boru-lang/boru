package native

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
)

// io_env_exit_cov_test.go — the hard edges of the env / exit handlers,
// reached directly because the language surface cannot produce them: a
// signature slot admits a DepScalar CONSTRAINT (`Integer gt 3`) where the
// handler expects a concrete value, and the AsConcreteX accessors are the
// guard that turns that into a clean error instead of a zero value.

// A DepScalar arriving in the name slot is declined, not silently read as
// the empty string.
func TestEnvLookupRejectsDepScalar(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	dep := NewDepScalar(DepLT, NewString("z"))
	_, herr := envLookupHandler([]Value{dep}, nil, nil, r)
	if herr == nil {
		t.Fatal("a String CONSTRAINT was accepted as an environment name")
	}
	if !strings.Contains(herr.Error(), "must be a String") {
		t.Errorf("error = %v, want the name-type compile failure", herr)
	}
}

// The same for the exit code: a numeric constraint is not a status.
func TestExitRejectsDepScalar(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	dep := NewDepScalar(DepGTE, NewInteger(1))
	_, herr := exitHandler([]Value{dep}, nil, nil, r)
	if herr == nil {
		t.Fatal("an Integer CONSTRAINT was accepted as an exit code")
	}
	if !strings.Contains(herr.Error(), "must be an Integer") {
		t.Errorf("error = %v, want the code-type compile failure", herr)
	}
}

// SetHostEnvOps tolerates a nil registry, like every other host installer:
// an embedding host that has not built one yet must get a no-op, not a
// panic (ADR-005).
func TestSetHostEnvOpsNilRegistry(t *testing.T) {
	SetHostEnvOps(nil, capabilities.MapEnvOps{Vars: map[string]string{"A": "1"}})
	if ops := HostEnvOps(nil); ops != nil {
		t.Errorf("HostEnvOps(nil) = %T, want nil", ops)
	}
}
