package test

import (
	"testing"

	"github.com/boru-lang/boru/lang/go"

	udk "github.com/voxgig/udk/go"
)

// makeTestSDK creates a UniversalSDK in test mode with inline entity data.
func makeTestSDK(t *testing.T) *udk.UniversalSDK {
	t.Helper()

	um := udk.NewUniversalManager(map[string]any{
		"registry": "registry",
	})
	baseSDK := um.Make("voxgig-solardemo")

	// Inline test data for planets and moons.
	testEntity := map[string]any{
		"planet": map[string]any{
			"planet01": map[string]any{
				"id":       "planet01",
				"name":     "Mercury",
				"kind":     "terrestrial",
				"diameter": 4879,
			},
			"planet02": map[string]any{
				"id":       "planet02",
				"name":     "Venus",
				"kind":     "terrestrial",
				"diameter": 12104,
			},
			"planet03": map[string]any{
				"id":       "planet03",
				"name":     "Earth",
				"kind":     "terrestrial",
				"diameter": 12756,
			},
		},
		"moon": map[string]any{
			"moon01": map[string]any{
				"id":        "moon01",
				"name":      "Luna",
				"kind":      "natural",
				"diameter":  3474,
				"planet_id": "planet03",
			},
			"moon02": map[string]any{
				"id":        "moon02",
				"name":      "Phobos",
				"kind":      "natural",
				"diameter":  22,
				"planet_id": "planet04",
			},
		},
	}

	testSDK := baseSDK.Test(map[string]any{
		"entity": testEntity,
	}, nil)

	return testSDK
}

func TestListAPIPlanet(t *testing.T) {
	a, err := lang.New(lang.Options{Registry: "test/registry"})
	if err != nil {
		t.Fatal(err)
	}
	seedBoru(a)

	a.SetSDK("voxgig-solardemo", makeTestSDK(t))

	result, err := runReference(t, a, `list {kind:"api", spec:"voxgig-solardemo", entity:"planet"}`)
	if err != nil {
		t.Fatalf("list api planet failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// Result should be a string representation of a list of maps.
	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	// Verify all three planets are present.
	for _, name := range []string{"Mercury", "Venus", "Earth"} {
		if !contains(s, name) {
			t.Errorf("expected planet %q in result: %s", name, s)
		}
	}
}

func TestListAPIMoon(t *testing.T) {
	a, err := lang.New(lang.Options{Registry: "test/registry"})
	if err != nil {
		t.Fatal(err)
	}
	seedBoru(a)

	a.SetSDK("voxgig-solardemo", makeTestSDK(t))

	result, err := runReference(t, a, `list {kind:"api", spec:"voxgig-solardemo", entity:"moon"}`)
	if err != nil {
		t.Fatalf("list api moon failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	// Verify moons are present.
	for _, name := range []string{"Luna", "Phobos"} {
		if !contains(s, name) {
			t.Errorf("expected moon %q in result: %s", name, s)
		}
	}
}

func TestListAPIWithJsonExtension(t *testing.T) {
	a, err := lang.New(lang.Options{Registry: "test/registry"})
	if err != nil {
		t.Fatal(err)
	}
	seedBoru(a)

	a.SetSDK("voxgig-solardemo", makeTestSDK(t))

	// spec with .json extension should also work.
	result, err := runReference(t, a, `list {kind:"api", spec:"voxgig-solardemo.json", entity:"planet"}`)
	if err != nil {
		t.Fatalf("list api with .json extension failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestListAPINonAPIMapFallsThrough(t *testing.T) {
	a, err := lang.New(lang.Options{Registry: "test/registry"})
	if err != nil {
		t.Fatal(err)
	}
	seedBoru(a)

	// A map without kind:"api" should not trigger the API handler.
	// It should be treated as a record type and return an empty list.
	result, err := runReference(t, a, `list {name:"test"}`)
	if err != nil {
		t.Fatalf("list plain map failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
