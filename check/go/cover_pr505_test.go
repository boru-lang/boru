package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestModuleExportsOfANonNamespace: a value that is no module namespace
// shares no export map, so two such values never read as the same module.
func TestModuleExportsOfANonNamespace(t *testing.T) {
	if moduleExports(core.NewInteger(1)) != nil {
		t.Error("an Integer has no export map")
	}
	m := core.NewOrderedMap()
	if moduleExports(core.NewMap(m)) != m {
		t.Error("a map value shares its own map")
	}
}
