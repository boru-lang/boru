package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/lang/go"

	udk "github.com/voxgig/udk/go"
)

// makeTestSDKForOps creates a UniversalSDK in test mode with inline entity data
// for use in API operation tests.
func makeTestSDKForOps(t *testing.T) *udk.UniversalSDK {
	t.Helper()

	um := udk.NewUniversalManager(map[string]any{
		"registry": "registry",
	})
	baseSDK := um.Make("voxgig-solardemo")

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
		},
	}

	testSDK := baseSDK.Test(map[string]any{
		"entity": testEntity,
	}, nil)

	return testSDK
}

func newBoruWithSDK(t *testing.T) *lang.Boru {
	t.Helper()
	a, err := lang.New(lang.Options{Registry: "test/registry"})
	if err != nil {
		t.Fatal(err)
	}
	seedBoru(a)
	a.SetSDK("voxgig-solardemo", makeTestSDKForOps(t))
	return a
}

// --- list with query ---

func TestListAPIWithQuery(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `list {kind:"api", spec:"voxgig-solardemo", entity:"planet", query:{id:"planet01"}}`)
	if err != nil {
		t.Fatalf("list with query failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	// Should contain Mercury (planet01).
	if !strings.Contains(s, "Mercury") {
		t.Errorf("expected Mercury in result: %s", s)
	}
}

func TestListAPIWithQueryNoMatch(t *testing.T) {
	a := newBoruWithSDK(t)

	// Query for a non-existent id should return an empty list (not an error).
	result, err := runReference(t, a, `list {kind:"api", spec:"voxgig-solardemo", entity:"planet", query:{id:"planet99"}}`)
	if err != nil {
		t.Fatalf("list with no-match query failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestListAPIWithoutQuery(t *testing.T) {
	a := newBoruWithSDK(t)

	// Without query should return all planets.
	result, err := runReference(t, a, `list {kind:"api", spec:"voxgig-solardemo", entity:"planet"}`)
	if err != nil {
		t.Fatalf("list without query failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	for _, name := range []string{"Mercury", "Venus", "Earth"} {
		if !strings.Contains(s, name) {
			t.Errorf("expected %q in result: %s", name, s)
		}
	}
}

// --- load ---

func TestLoadAPIPlanet(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `load {kind:"api", spec:"voxgig-solardemo", entity:"planet", query:{id:"planet01"}}`)
	if err != nil {
		t.Fatalf("load api planet failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "Mercury") {
		t.Errorf("expected Mercury in result: %s", s)
	}
	if !strings.Contains(s, "planet01") {
		t.Errorf("expected planet01 id in result: %s", s)
	}
}

func TestLoadAPIMoon(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `load {kind:"api", spec:"voxgig-solardemo", entity:"moon", query:{id:"moon01"}}`)
	if err != nil {
		t.Fatalf("load api moon failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "Luna") {
		t.Errorf("expected Luna in result: %s", s)
	}
}

func TestLoadAPINotFound(t *testing.T) {
	a := newBoruWithSDK(t)

	// Loading a non-existent entity should fail.
	_, err := a.Run(`load {kind:"api", spec:"voxgig-solardemo", entity:"planet", query:{id:"planet99"}}`)
	if err == nil {
		t.Fatal("expected error for load not found")
	}
}

func TestLoadAPIWithJsonExtension(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `load {kind:"api", spec:"voxgig-solardemo.json", entity:"planet", query:{id:"planet02"}}`)
	if err != nil {
		t.Fatalf("load api with .json extension failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "Venus") {
		t.Errorf("expected Venus in result: %s", s)
	}
}

// --- create ---

func TestCreateAPIPlanet(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `create {kind:"api", spec:"voxgig-solardemo", entity:"planet", data:{name:"Mars", kind:"terrestrial", diameter:6792}}`)
	if err != nil {
		t.Fatalf("create api planet failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	// Created entity should have the name we provided.
	if !strings.Contains(s, "Mars") {
		t.Errorf("expected Mars in result: %s", s)
	}

	// Created entity should have a generated id.
	if !strings.Contains(s, "id") {
		t.Errorf("expected id field in result: %s", s)
	}
}

func TestCreateAPIMoon(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `create {kind:"api", spec:"voxgig-solardemo", entity:"moon", data:{name:"Deimos", kind:"natural", diameter:12, planet_id:"planet04"}}`)
	if err != nil {
		t.Fatalf("create api moon failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "Deimos") {
		t.Errorf("expected Deimos in result: %s", s)
	}
}

// --- update ---

func TestUpdateAPIPlanet(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `update {kind:"api", spec:"voxgig-solardemo", entity:"planet", data:{id:"planet01", name:"Mercury Updated"}}`)
	if err != nil {
		t.Fatalf("update api planet failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "Mercury Updated") {
		t.Errorf("expected 'Mercury Updated' in result: %s", s)
	}
	if !strings.Contains(s, "planet01") {
		t.Errorf("expected planet01 id in result: %s", s)
	}
}

func TestUpdateAPINotFound(t *testing.T) {
	a := newBoruWithSDK(t)

	_, err := a.Run(`update {kind:"api", spec:"voxgig-solardemo", entity:"planet", data:{id:"planet99", name:"Ghost"}}`)
	if err == nil {
		t.Fatal("expected error for update not found")
	}
}

// --- remove ---

func TestRemoveAPIPlanet(t *testing.T) {
	a := newBoruWithSDK(t)

	// Remove planet01.
	_, err := runReference(t, a, `remove {kind:"api", spec:"voxgig-solardemo", entity:"planet", query:{id:"planet01"}}`)
	if err != nil {
		t.Fatalf("remove api planet failed: %v", err)
	}

	// Verify planet01 is gone by trying to load it.
	_, err = a.Run(`load {kind:"api", spec:"voxgig-solardemo", entity:"planet", query:{id:"planet01"}}`)
	if err == nil {
		t.Fatal("expected error loading removed planet")
	}
}

func TestRemoveAPINotFound(t *testing.T) {
	a := newBoruWithSDK(t)

	_, err := a.Run(`remove {kind:"api", spec:"voxgig-solardemo", entity:"planet", query:{id:"planet99"}}`)
	if err == nil {
		t.Fatal("expected error for remove not found")
	}
}

// --- non-API map falls through ---

func TestLoadAPINonAPIMapFallsThrough(t *testing.T) {
	a := newBoruWithSDK(t)

	// A map without kind:"api" should not trigger the API handler.
	// It should match the [map, map] signature (loadRecordHandler) and return empty map.
	result, err := runReference(t, a, `load {name:"test"} {id:"1"}`)
	if err != nil {
		t.Fatalf("load plain map failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestCreateAPINonAPIMapFallsThrough(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `create {name:"test"} {id:"1", name:"Bob"}`)
	if err != nil {
		t.Fatalf("create plain map failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestUpdateAPINonAPIMapFallsThrough(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `update {name:"test"} {id:"1", name:"Bob"}`)
	if err != nil {
		t.Fatalf("update plain map failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestRemoveAPINonAPIMapFallsThrough(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := runReference(t, a, `remove {name:"test"} {id:"1"}`)
	if err != nil {
		t.Fatalf("remove plain map failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

// --- Entity object type operations ---

func TestListEntityAll(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := a.Run(`def planets (make Entity {kind:"api", spec:"voxgig-solardemo", entity:"planet"})
list planets`)
	if err != nil {
		t.Fatalf("list entity failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	for _, name := range []string{"Mercury", "Venus", "Earth"} {
		if !strings.Contains(s, name) {
			t.Errorf("expected %q in result: %s", name, s)
		}
	}
}

func TestListEntityWithQuery(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := a.Run(`def planets (make Entity {kind:"api", spec:"voxgig-solardemo", entity:"planet"})
planets list {id:"planet01"}`)
	if err != nil {
		t.Fatalf("list entity with query failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "Mercury") {
		t.Errorf("expected Mercury in result: %s", s)
	}
}

func TestLoadEntity(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := a.Run(`def planets (make Entity {kind:"api", spec:"voxgig-solardemo", entity:"planet"})
planets load {id:"planet01"}`)
	if err != nil {
		t.Fatalf("load entity failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "Mercury") {
		t.Errorf("expected Mercury in result: %s", s)
	}
}

func TestCreateEntity(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := a.Run(`def planets (make Entity {kind:"api", spec:"voxgig-solardemo", entity:"planet"})
planets create {name:"Mars", kind:"terrestrial", diameter:6792}`)
	if err != nil {
		t.Fatalf("create entity failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "Mars") {
		t.Errorf("expected Mars in result: %s", s)
	}
}

func TestUpdateEntity(t *testing.T) {
	a := newBoruWithSDK(t)

	result, err := a.Run(`def planets (make Entity {kind:"api", spec:"voxgig-solardemo", entity:"planet"})
planets update {id:"planet01", name:"Mercury Updated"}`)
	if err != nil {
		t.Fatalf("update entity failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	s, ok := result[0].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result[0])
	}

	if !strings.Contains(s, "Mercury Updated") {
		t.Errorf("expected 'Mercury Updated' in result: %s", s)
	}
}

func TestRemoveEntity(t *testing.T) {
	a := newBoruWithSDK(t)

	_, err := a.Run(`def planets (make Entity {kind:"api", spec:"voxgig-solardemo", entity:"planet"})
planets remove {id:"planet01"}`)
	if err != nil {
		t.Fatalf("remove entity failed: %v", err)
	}

	// Verify planet01 is gone.
	_, err = a.Run(`load {kind:"api", spec:"voxgig-solardemo", entity:"planet", query:{id:"planet01"}}`)
	if err == nil {
		t.Fatal("expected error loading removed planet")
	}
}
