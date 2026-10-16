package native

import (
	"errors"
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// Seam-7 coverage for log_logger.go (sub-registry init-error arms),
// notinstalled_fileops.go and permissioned_fileops.go
// (design/TEST-SEAMS.10.md).

func TestSeam7BuildLoggerInstanceInitError(t *testing.T) {
	r := seam5Reg(t)
	lsr := NewLogSinkRegistry()

	// Positive: with the real sub-registry, the constructors build.
	if _, err := buildLoggerInstance(&loggerState{name: "app", attrs: NewOrderedMap(), lsr: lsr}); err != nil {
		t.Fatalf("buildLoggerInstance positive: %v", err)
	}
	logH := logLoggerNative(lsr).Signatures[0].Impl.(*GoImpl).Handler
	if _, err := logH([]Value{NewString("app")}, nil, nil, r); err != nil {
		t.Fatalf("log-logger positive: %v", err)
	}

	// Swap the sub-registry constructor to fail; every buildLoggerInstance
	// consumer surfaces the init error.
	saved := newSubRegistry
	t.Cleanup(func() { newSubRegistry = saved })
	newSubRegistry = func(...func(*Registry)) (*Registry, error) { return nil, errors.New("subreg boom") }

	// buildLoggerInstance itself.
	if _, err := buildLoggerInstance(&loggerState{name: "x", attrs: NewOrderedMap(), lsr: lsr}); err == nil ||
		!strings.Contains(err.Error(), "subreg boom") {
		t.Fatalf("expected buildLoggerInstance error, got %v", err)
	}

	// Log.logger handler.
	if _, err := logH([]Value{NewString("app")}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "subreg boom") {
		t.Fatalf("expected log-logger error, got %v", err)
	}

	// Log.with handler.
	withH := logWithNative(lsr).Signatures[0].Impl.(*GoImpl).Handler
	if _, err := withH([]Value{NewString("app"), NewMap(NewOrderedMap())}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "subreg boom") {
		t.Fatalf("expected log-with error, got %v", err)
	}

	// Logger.child handler.
	childH := loggerChildNative(&loggerState{name: "p", attrs: NewOrderedMap(), lsr: lsr}).
		Signatures[0].Impl.(*GoImpl).Handler
	if _, err := childH([]Value{NewMap(NewOrderedMap())}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "child logger") {
		t.Fatalf("expected logger-child error, got %v", err)
	}

	// loggerShapeReturns falls back to a bare Map carrier on init error.
	got := loggerShapeReturns(lsr)(nil, r)
	if len(got) != 1 || !got[0].Parent().ConformsTo(TMap) {
		t.Fatalf("loggerShapeReturns fallback: %v", got)
	}
}

func TestSeam7NotInstalledFileOps(t *testing.T) {
	ni := notInstalledFileOps{}

	if _, err := ni.ReadFile("/x"); err == nil ||
		!strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected read denial, got %v", err)
	}
	if err := ni.WriteFile("/x", []byte("y"), 0); err == nil ||
		!strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected write denial, got %v", err)
	}
	if err := ni.MkdirAll("/x", 0); err == nil ||
		!strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected mkdir denial, got %v", err)
	}
	// ResolvePath is pure and returns the input unchanged.
	if p, err := ni.ResolvePath("/x"); err != nil || p != "/x" {
		t.Fatalf("ResolvePath: %q / %v", p, err)
	}
}

func TestSeam7PermissionedFileOps(t *testing.T) {
	mem := capabilities.NewMem()

	// nil policy returns the input unchanged.
	if got := NewPermissionedFileOps(mem, nil); got != mem {
		t.Fatal("nil policy must return the inner ops unchanged")
	}

	allow, err := policy.LoadInline(`{name:"allow"}`)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := NewPermissionedFileOps(mem, allow)

	// Allow paths delegate to the inner mem ops.
	if err := wrapped.WriteFile("/f", []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile allow: %v", err)
	}
	if b, err := wrapped.ReadFile("/f"); err != nil || string(b) != "data" {
		t.Fatalf("ReadFile allow: %q / %v", b, err)
	}
	if err := wrapped.MkdirAll("/d", 0o755); err != nil {
		t.Fatalf("MkdirAll allow: %v", err)
	}
	if p, err := wrapped.ResolvePath("/f"); err != nil || p == "" {
		t.Fatalf("ResolvePath: %q / %v", p, err)
	}

	// A structurally-uninstalled fileops scope denies MkdirAll.
	deny, err := policy.LoadInline(`{name:"deny" scopes:{fileops:{install:false}}}`)
	if err != nil {
		t.Fatal(err)
	}
	denied := NewPermissionedFileOps(mem, deny)
	if err := denied.MkdirAll("/d2", 0o755); err == nil {
		t.Fatal("expected mkdir denial under uninstalled fileops")
	}
}
