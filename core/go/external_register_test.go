package core

import (
	"fmt"
	"testing"
)

// TestRegisterExternalBuiltin_PluginColor demonstrates the full
// plugin-type registration flow. A "host" module supplies a custom
// type identity (Object/PluginColor) via RegisterExternalBuiltin,
// pairs it with a custom TypeBehavior, and constructs values that
// the kernel renders + matches via the Behavior — no kernel patch
// needed.
//
// This is the Step 8 acceptance test: the registration hook closes
// the kernel-coupling loop that Step 1-5 set up. After this test
// passes, plugin authors can introduce their own types entirely
// outside eng.
func TestRegisterExternalBuiltin_PluginColor(t *testing.T) {
	// Plugin-side: define a payload variant and a Behavior.
	// In a real plugin this lives in the plugin's own package.
	type rgbPayload struct {
		R, G, B byte
	}
	// The plugin can satisfy Payload via ExtensionPayload{Body: rgb}
	// — no need to add the unexported marker (which is impossible
	// from outside eng).
	makeColor := func(r, g, b byte) Value {
		return NewExtension(nil, rgbPayload{R: r, G: g, B: b})
	}

	matchFn := func(v Value, target *Type) bool {
		if ParentOf(&v) != target {
			return false
		}
		ep, ok := DataOf(&v).(ExtensionPayload)
		if !ok {
			return false
		}
		_, ok = ep.Body.(rgbPayload)
		return ok
	}
	formatFn := func(v Value) string {
		ep, _ := DataOf(&v).(ExtensionPayload)
		rgb, _ := ep.Body.(rgbPayload)
		return fmt.Sprintf("#%02x%02x%02x", rgb.R, rgb.G, rgb.B)
	}
	equalFn := func(a, b Value) bool {
		ea, _ := DataOf(&a).(ExtensionPayload)
		eb, _ := DataOf(&b).(ExtensionPayload)
		ra, _ := ea.Body.(rgbPayload)
		rb, _ := eb.Body.(rgbPayload)
		return ra == rb
	}
	cb := &funcBehavior{
		matchFn:  matchFn,
		formatFn: formatFn,
		equalFn:  equalFn,
	}

	// Register against a private dynamic table to avoid mutating
	// the package-level Builtin under test parallelism.
	tt := newBuiltinTypeTable()
	tColor, err := tt.RegisterType("Ideal/TestPluginColor", 99001, "plugin:test", cb)
	if err != nil {
		t.Fatalf("RegisterExternalBuiltin: %v", err)
	}
	if tColor == nil {
		t.Fatal("RegisterExternalBuiltin returned nil Type")
	}
	if tColor.Path() != "Ideal/TestPluginColor" {
		t.Errorf("path = %q, want %q", tColor.Path(), "Ideal/TestPluginColor")
	}
	if tColor.FixedID() != 99001 {
		t.Errorf("FixedID = %d, want 99001", tColor.FixedID())
	}
	if tColor.Behavior() != cb {
		t.Errorf("Behavior = %T, want funcBehavior", tColor.Behavior)
	}
	if OriginOf(tColor) != OriginBuiltin {
		t.Errorf("Origin = %v, want OriginBuiltin", OriginOf(tColor))
	}

	// Construct a value of the new type and verify dispatch goes
	// through the Behavior.
	red := makeColor(255, 0, 0)
	SetParent(&red, tColor)

	got := tColor.Behavior().Format(red)
	if got != "#ff0000" {
		t.Errorf("Format(red) = %q, want %q", got, "#ff0000")
	}

	if !tColor.Behavior().Match(red, tColor) {
		t.Error("Behavior.Match(red, TColor) returned false")
	}

	red2 := makeColor(255, 0, 0)
	SetParent(&red2, tColor)
	if !tColor.Behavior().Equal(red, red2) {
		t.Error("Behavior.Equal of two identical colors returned false")
	}

	blue := makeColor(0, 0, 255)
	SetParent(&blue, tColor)
	if tColor.Behavior().Equal(red, blue) {
		t.Error("Behavior.Equal of red and blue returned true")
	}
}

// TestRegisterExternalBuiltin_DuplicatePath rejects re-registration.
func TestRegisterExternalBuiltin_DuplicatePath(t *testing.T) {
	tt := newBuiltinTypeTable()
	if _, err := tt.RegisterType("Ideal/Dup1", 99100, "plugin:test", nil); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := tt.RegisterType("Ideal/Dup1", 99101, "plugin:test", nil); err == nil {
		t.Fatal("re-registering same path should error")
	}
}

// TestRegisterExternalBuiltin_DuplicateFixedID rejects collision.
func TestRegisterExternalBuiltin_DuplicateFixedID(t *testing.T) {
	tt := newBuiltinTypeTable()
	if _, err := tt.RegisterType("Ideal/DupID1", 99200, "plugin:test", nil); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := tt.RegisterType("Ideal/DupID2", 99200, "plugin:test", nil); err == nil {
		t.Fatal("FixedID collision should error")
	}
}

// TestRegisterExternalBuiltin_MissingParent rejects orphan path.
func TestRegisterExternalBuiltin_MissingParent(t *testing.T) {
	tt := newBuiltinTypeTable()
	if _, err := tt.RegisterType("Undeclared/Foo", 99300, "plugin:test", nil); err == nil {
		t.Fatal("missing parent should error")
	}
}

// TestRegisterExternalBuiltin_LowercasePart rejects bad casing.
func TestRegisterExternalBuiltin_LowercasePart(t *testing.T) {
	tt := newBuiltinTypeTable()
	if _, err := tt.RegisterType("Ideal/lowercase", 99400, "plugin:test", nil); err == nil {
		t.Fatal("lowercase part should error")
	}
}

// TestRegisterExternalBuiltin_DefaultBehavior verifies nil Behavior
// falls back to DefaultBehavior.
func TestRegisterExternalBuiltin_DefaultBehavior(t *testing.T) {
	tt := newBuiltinTypeTable()
	def, err := tt.RegisterType("Ideal/DefaultBeh", 99500, "plugin:test", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if def.Behavior() != DefaultBehavior {
		t.Errorf("nil Behavior should fall back to DefaultBehavior, got %T", def.Behavior)
	}
}

// funcBehavior is a test-only TypeBehavior that delegates to closures.
type funcBehavior struct {
	matchFn  func(Value, *Type) bool
	formatFn func(Value) string
	equalFn  func(Value, Value) bool
}

func (b *funcBehavior) Match(v Value, t *Type) bool { return b.matchFn(v, t) }
func (b *funcBehavior) Format(v Value) string       { return b.formatFn(v) }
func (b *funcBehavior) Equal(a, c Value) bool       { return b.equalFn(a, c) }
