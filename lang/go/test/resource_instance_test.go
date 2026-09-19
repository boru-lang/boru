package test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// Resource/Entity instances carry their own ResourceInstanceInfo struct
// (Stage 3 gave the SDK object-type hierarchy its own representation, so
// they no longer satisfy IsClassInstance). Every class-instance
// special-case therefore had to be extended to the shared flat-instance
// shape. These Go tests pin the two behaviors that are awkward to
// express as .tsv spec rows (the rest live in lang/spec/resource.tsv §9).
// Regression guard for PR #210 review.

// TestResourceInstanceJsonify: StructUtil.jsonify must serialize a
// Resource/Entity instance as its resolved field map — not a Go pointer
// dump (Entity({0xc…})) and not the bare render string.
func TestResourceInstanceJsonify(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := runReference(t, a, `import "boru:struct-util"  StructUtil.jsonify (make Entity {kind:'api' spec:'s' entity:'x'})`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	got := fmt.Sprintf("%v", res[0])
	for _, want := range []string{`"kind"`, `"api"`, `"spec"`, `"s"`, `"entity"`, `"x"`} {
		if !strings.Contains(got, want) {
			t.Errorf("jsonify missing %s; got %q", want, got)
		}
	}
	if strings.Contains(got, "0xc") || strings.Contains(got, "Entity{") {
		t.Errorf("jsonify leaked a pointer / render string: %q", got)
	}
	// Resource/Entity are not part of the class reify contract, so no
	// $class metadata key is emitted (unlike class jsonify).
	if strings.Contains(got, "$class") {
		t.Errorf("jsonify emitted $class for a Resource instance: %q", got)
	}
}

// TestResourceInstanceFieldReturnType: a Resource field read carries its
// declared schema type, so a function that returns the wrong type is
// rejected — the getResourceReturns ReturnsFn parity with class fields.
// `spec` is a String field; returning it where Integer is declared must
// fail (statically or at the runtime return check), while returning it
// where String is declared succeeds.
func TestResourceInstanceFieldReturnType(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	okSrc := `def e:Entity {kind:'api' spec:'s' entity:'x'} def f fn [[x:Entity] [String] [x .spec]] f e`
	res, err := runReference(t, a, okSrc)
	if err != nil {
		t.Fatalf("String-return should type-check: %v", err)
	}
	if len(res) != 1 || fmt.Sprintf("%v", res[0]) != "s" {
		t.Fatalf("expected s, got %v", res)
	}

	b, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	badSrc := `def e:Entity {kind:'api' spec:'s' entity:'x'} def f fn [[x:Entity] [Integer] [x .spec]] f e`
	if _, err := b.Run(badSrc); err == nil {
		t.Fatalf("Integer-return of a String field must be rejected, but it was accepted")
	}
}
