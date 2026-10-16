package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestS8CloneObject covers cloneObject (clone.go), reached when CloneValue
// deep-clones a class instance: the field map is duplicated value-by-value
// while the immutable TypeRef descriptor is shared.
func TestS8CloneObject(t *testing.T) {
	fields := core.NewOrderedMap()
	fields.Set("x", core.NewInteger(1))
	tref := &core.ClassTypeInfo{Name: "Class/C", Fields: core.NewOrderedMap()}
	inst := core.NewClassInstance(core.TClass, core.ClassInstanceInfo{TypeRef: tref, Fields: fields})

	cl := core.CloneValue(inst)
	ci, ok := core.DataOf(&cl).(core.ClassInstanceInfo)
	if !ok {
		t.Fatalf("clone Data = %T, want ClassInstanceInfo", core.DataOf(&cl))
	}
	if ci.TypeRef != tref {
		t.Error("cloneObject should share the immutable TypeRef descriptor")
	}
	if ci.Fields == fields {
		t.Error("cloneObject should duplicate the field map, not share it")
	}
	// Mutating the clone's field map must not touch the original.
	ci.Fields.Set("x", core.NewInteger(99))
	if got, _ := fields.Get("x"); func() int64 { n, _ := core.AsInteger(got); return n }() != 1 {
		t.Error("deep clone leaked: mutating the clone changed the original field map")
	}

	// nil-Fields arm: cloneObject leaves Fields nil rather than allocating.
	empty := core.CloneValue(core.NewClassInstance(core.TClass, core.ClassInstanceInfo{TypeRef: tref}))
	if ec, _ := core.DataOf(&empty).(core.ClassInstanceInfo); ec.Fields != nil {
		t.Error("cloneObject with nil Fields should keep Fields nil")
	}
}
