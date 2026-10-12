package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestResolveWordsDeepPreservesTypedContainerPayload pins the
// element/entry preservation contract of resolveWordsDeep's typed-list
// and typed-map arms. The child-only rebuild (`NewTypedList(child)`)
// silently emptied a populated typed container, so a record/class
// schema default like `{xs:[:Integer 1 2]}` — which core_make routes
// through this resolver — instantiated as `[]`. The TS twin
// (eng/ts/src/resolve.ts) always preserved; this is the Go side of that
// parity contract.
//
// The typed-MAP arm has no source syntax that reaches it through the
// spec corpus, so it is pinned directly here.
func TestResolveWordsDeepPreservesTypedContainerPayload(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	// Typed list: the child Word resolves AND the inline elements
	// survive, themselves deep-resolved (the `true` Word becomes a
	// Boolean).
	tl := core.NewTypedListWithElements(core.NewWord("Integer"), []core.Value{
		core.NewInteger(1), core.NewWord("true"),
	})
	got := core.ResolveWordsDeepR(tl, r)
	ct, err := core.AsChildType(got)
	if err != nil {
		t.Fatalf("typed list lost its ChildTypeInfo: %v", err)
	}
	if !core.IsTypeLiteral(&ct.Child) || !core.ValueType(ct.Child).Equal(core.TInteger) {
		t.Errorf("child = %s, want the Integer type literal", ct.Child)
	}
	if len(ct.Elements) != 2 {
		t.Fatalf("elements = %d, want 2 (the child-only rebuild is back)", len(ct.Elements))
	}
	if b, berr := core.AsBoolean(ct.Elements[1]); berr != nil || !b {
		t.Errorf("element 1 = %s, want the resolved Boolean true", ct.Elements[1])
	}

	// Typed map: same contract over Entries, keys preserved verbatim.
	tm := core.NewTypedMapWithEntries(core.NewWord("Integer"), []core.ChildEntry{
		{Key: "a", Value: core.NewInteger(1)},
		{Key: "b", Value: core.NewWord("false")},
	})
	got = core.ResolveWordsDeepR(tm, r)
	ct, err = core.AsChildType(got)
	if err != nil {
		t.Fatalf("typed map lost its ChildTypeInfo: %v", err)
	}
	if len(ct.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(ct.Entries))
	}
	if ct.Entries[0].Key != "a" || ct.Entries[1].Key != "b" {
		t.Errorf("keys = %q/%q, want a/b", ct.Entries[0].Key, ct.Entries[1].Key)
	}
	if b, berr := core.AsBoolean(ct.Entries[1].Value); berr != nil || b {
		t.Errorf("entry b = %s, want the resolved Boolean false", ct.Entries[1].Value)
	}

	// Negative pairing: an EMPTY typed container still takes the
	// child-only rebuild — no payload is invented on either side.
	el := core.ResolveWordsDeepR(core.NewTypedList(core.NewWord("Integer")), r)
	if ct, err = core.AsChildType(el); err != nil || len(ct.Elements) != 0 {
		t.Errorf("empty typed list gained elements: %s", el)
	}
	em := core.ResolveWordsDeepR(core.NewTypedMap(core.NewWord("Integer")), r)
	if ct, err = core.AsChildType(em); err != nil || len(ct.Entries) != 0 {
		t.Errorf("empty typed map gained entries: %s", em)
	}
}
