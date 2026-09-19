package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"

	udk "github.com/voxgig/udk/go"
)

// makeTestSDKForDirect creates a UniversalSDK in test mode with entity data.
func makeTestSDKForDirect(t *testing.T) *udk.UniversalSDK {
	t.Helper()

	um := udk.NewUniversalManager(map[string]any{
		"registry": "registry",
	})
	baseSDK := um.Make("voxgig-solardemo")

	testEntity := map[string]any{
		"planet": map[string]any{
			"planet01": map[string]any{
				"id":   "planet01",
				"name": "Mercury",
			},
		},
	}

	testSDK := baseSDK.Test(map[string]any{
		"entity": testEntity,
	}, nil)

	return testSDK
}

func newBoruWithDirectSDK(t *testing.T) *lang.Boru {
	t.Helper()
	a, err := lang.New(lang.Options{Registry: "test/registry"})
	if err != nil {
		t.Fatal(err)
	}
	seedBoru(a)
	a.SetSDK("voxgig-solardemo", makeTestSDKForDirect(t))
	// prepare/direct moved to boru:net; import once (state persists across Run).
	if _, err := runReference(t, a, `import "boru:net"`); err != nil {
		t.Fatal(err)
	}
	return a
}

// --- prepare ---

func TestPrepareAPIBasic(t *testing.T) {
	a := newBoruWithDirectSDK(t)

	result, err := runReference(t, a, `Net.prepare {kind:"api", spec:"voxgig-solardemo", path:"/planets", method:"GET"}`)
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	// The fetchdef should contain the URL with our path.
	if !strings.Contains(s, "/planets") {
		t.Errorf("expected /planets in prepared fetchdef: %s", s)
	}

	// Should contain the method.
	if !strings.Contains(s, "GET") {
		t.Errorf("expected GET in prepared fetchdef: %s", s)
	}
}

func TestPrepareAPIWithHeaders(t *testing.T) {
	a := newBoruWithDirectSDK(t)

	result, err := runReference(t, a, `Net.prepare {kind:"api", spec:"voxgig-solardemo", path:"/planets", method:"POST", headers:{Authorization:"Bearer test123"}}`)
	if err != nil {
		t.Fatalf("prepare with headers failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "POST") {
		t.Errorf("expected POST in prepared fetchdef: %s", s)
	}
}

func TestPrepareAPIDefaultMethod(t *testing.T) {
	a := newBoruWithDirectSDK(t)

	// Without method, should default to GET.
	result, err := runReference(t, a, `Net.prepare {kind:"api", spec:"voxgig-solardemo", path:"/planets"}`)
	if err != nil {
		t.Fatalf("prepare default method failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "GET") {
		t.Errorf("expected GET in prepared fetchdef: %s", s)
	}
}

func TestPrepareAPIWithJsonExtension(t *testing.T) {
	a := newBoruWithDirectSDK(t)

	result, err := runReference(t, a, `Net.prepare {kind:"api", spec:"voxgig-solardemo.json", path:"/test"}`)
	if err != nil {
		t.Fatalf("prepare with .json extension failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// --- direct ---

func TestDirectAPIBasic(t *testing.T) {
	a := newBoruWithDirectSDK(t)

	result, err := runReference(t, a, `Net.direct {kind:"api", spec:"voxgig-solardemo", path:"/planets", method:"GET"}`)
	if err != nil {
		t.Fatalf("direct failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	// Direct returns a result map with ok, status fields.
	if !strings.Contains(s, "ok") {
		t.Errorf("expected ok field in direct result: %s", s)
	}
	if !strings.Contains(s, "status") {
		t.Errorf("expected status field in direct result: %s", s)
	}
}

func TestDirectAPIWithJsonExtension(t *testing.T) {
	a := newBoruWithDirectSDK(t)

	result, err := runReference(t, a, `Net.direct {kind:"api", spec:"voxgig-solardemo.json", path:"/test"}`)
	if err != nil {
		t.Fatalf("direct with .json extension failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestDirectAPIPost(t *testing.T) {
	a := newBoruWithDirectSDK(t)

	result, err := runReference(t, a, `Net.direct {kind:"api", spec:"voxgig-solardemo", path:"/planets", method:"POST", body:"{\"name\":\"Mars\"}"}`)
	if err != nil {
		t.Fatalf("direct POST failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "status") {
		t.Errorf("expected status field in direct result: %s", s)
	}
}
