package core

import "testing"

// TestTokenBodyStampCache: the run-time unit cache for token bodies is per
// registry — a miss reads as unseen, a stored value reads back under its
// key, and a concurrent fork starts empty (its stamps compile against its
// own bindings).
func TestTokenBodyStampCache(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.TokenBodyStamp("id:x/1,Integer"); ok {
		t.Fatal("an unseen key must read as absent")
	}
	r.SetTokenBodyStamp("id:x/1,Integer", "ref")
	r.SetTokenBodyStamp("txt:add@1:2 /0", struct{}{})
	if v, ok := r.TokenBodyStamp("id:x/1,Integer"); !ok || v != "ref" {
		t.Fatalf("stored stamp reads back: %v %v", v, ok)
	}
	if _, ok := r.TokenBodyStamp("txt:add@1:2 /0"); !ok {
		t.Fatal("the declined marker reads back under its key")
	}
	fork := r.ForkConcurrent()
	if _, ok := fork.TokenBodyStamp("id:x/1,Integer"); ok {
		t.Fatal("a fork starts with an empty cache")
	}
	if _, ok := r.TokenBodyStamp("id:x/1,Integer"); !ok {
		t.Fatal("the parent keeps its cache")
	}
}
