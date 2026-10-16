package native

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// TestMicronLeafTableSync guards the property table in members.go against
// drift from the real property resolver: every property the table lists for a
// leaf must be one getMicronReturns recognises (not a None miss), and an
// invented key must miss. If a leaf gains/loses a property, this fails until
// the table is updated.
func TestMicronLeafTableSync(t *testing.T) {
	r := seam5Reg(t)
	for _, leaf := range MicronLeafNames() {
		lt := core.Builtin.Lookup("Scalar/Micron/" + leaf)
		if lt == nil {
			t.Fatalf("leaf type Scalar/Micron/%s not registered", leaf)
			continue
		}
		carrier := NewCarrier(lt)
		for _, m := range micronLeafMembers[leaf] {
			out := getMicronReturns([]Value{NewString(m.Name), carrier}, r)
			if len(out) != 1 {
				t.Fatalf("%s.%s: getMicronReturns arity %d", leaf, m.Name, len(out))
			}
			// A real property narrows to a typed OR dynamic carrier; only an
			// unknown key returns a None carrier.
			if out[0].Parent() != nil && out[0].Parent().Equal(TNone) {
				t.Errorf("%s.%s is in the table but getMicronReturns reports it unknown (None)", leaf, m.Name)
			}
		}
		// An invented member must miss (→ None), proving the leaf case exists.
		out := getMicronReturns([]Value{NewString("zzNotAProp"), carrier}, r)
		if !(out[0].Parent() != nil && out[0].Parent().Equal(TNone)) {
			t.Errorf("%s: unknown key did not return a None carrier (got %v)", leaf, out[0])
		}
	}
}

func names(ms []Member) map[string]bool {
	m := map[string]bool{}
	for _, x := range ms {
		m[x.Name] = true
	}
	return m
}

func TestMembersOfTypeMicronLeaf(t *testing.T) {
	r := seam5Reg(t)
	got := names(MembersOfType("Semveron", r))
	for _, want := range []string{"major", "minor", "patch", "prerelease", "prereleaseParts", "release", "stable", "version"} {
		if !got[want] {
			t.Errorf("Semveron members missing %q; got %v", want, got)
		}
	}
	if MembersOfType("Emailon", r)[0].Kind != "property" {
		t.Error("Micron members should be kind=property")
	}
	if MembersOfType("NoSuchType", r) != nil {
		t.Error("unknown type should yield nil members")
	}
	if MembersOfType("", r) != nil {
		t.Error("empty type name should yield nil members")
	}
}

func TestMembersOfTypeUserKinds(t *testing.T) {
	r := seam5Reg(t)
	cases := []struct {
		src, typ string
		want     []string
		kind     string
	}{
		{"def P class {x:1 y:2}\nP", "P", []string{"x", "y"}, "field"},
		{"def R refine Record [a:Integer b:String]\nR", "R", []string{"a", "b"}, "field"},
		{"def Baron refine Micron {foo:String bar:Integer}\nBaron", "Baron", []string{"foo", "bar"}, "property"},
	}
	for _, c := range cases {
		if _, err := seam5Run(r, c.src); err != nil {
			t.Fatalf("define %s: %v", c.typ, err)
		}
		ms := MembersOfType(c.typ, r)
		got := names(ms)
		for _, w := range c.want {
			if !got[w] {
				t.Errorf("%s members = %v, missing %q", c.typ, got, w)
			}
		}
		if len(ms) > 0 && ms[0].Kind != c.kind {
			t.Errorf("%s member kind = %q, want %q", c.typ, ms[0].Kind, c.kind)
		}
		// Field detail should carry the declared type where known.
		if c.typ == "R" {
			for _, m := range ms {
				if m.Name == "a" && m.Detail != "Integer" {
					t.Errorf("R.a detail = %q, want Integer", m.Detail)
				}
			}
		}
	}
}

func TestMembersOfTypeTableAndNewtype(t *testing.T) {
	r := seam5Reg(t)
	// Table type — members are its row-record fields.
	if _, err := seam5Run(r, "def R refine Record [a:Integer b:String]\ndef T refine Table R\nT"); err != nil {
		t.Fatalf("define table: %v", err)
	}
	if got := names(MembersOfType("T", r)); !got["a"] || !got["b"] {
		t.Errorf("Table T members = %v, want a,b", got)
	}
	// A newtype of a user Micron kind still carries the inherited schema on its
	// bound body, so its properties resolve.
	if _, err := seam5Run(r, "def Baron refine Micron {foo:String}\ndef Subon refine Baron\nSubon"); err != nil {
		t.Fatalf("define micron newtype: %v", err)
	}
	if got := names(MembersOfType("Subon", r)); !got["foo"] {
		t.Errorf("Micron newtype Subon members = %v, want foo", got)
	}
	// A plain alias binds a non-schema body → no members (membersOfTypeBody
	// falls through).
	if _, err := seam5Run(r, "def MyAlias Integer\nMyAlias"); err != nil {
		t.Fatalf("define alias: %v", err)
	}
	if ms := MembersOfType("MyAlias", r); ms != nil {
		t.Errorf("alias type members = %v, want nil", ms)
	}
}

func TestMembersOfTypeDefensive(t *testing.T) {
	// A nil registry: only the builtin Micron table answers; a user name yields
	// nil rather than dereferencing r.
	if MembersOfType("NotAType", nil) != nil {
		t.Error("unknown type with nil registry should be nil")
	}
	if len(MembersOfType("Emailon", nil)) == 0 {
		t.Error("a builtin leaf resolves without a registry")
	}
	// fieldsToMembers tolerates a nil schema map.
	if fieldsToMembers(nil, "field") != nil {
		t.Error("nil field map should yield nil members")
	}
}
