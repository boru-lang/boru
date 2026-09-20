package core

import (
	"strings"
	"testing"
)

// Ownership-anchored signature rules (design/OPEN-WORDS.1.md) — unit
// pins for the provenance stamps and the nominal-anchor classifier.

// TestOwnerStamps pins the Owner provenance at every registration path.
func TestOwnerStamps(t *testing.T) {
	if got := TInteger.OwnerID(); got != OwnerKernel {
		t.Errorf("kernel builtin owner = %q", got)
	}
	tt := NewDynamicTypeTable()
	if minted := tt.MintType("OwnTest", TMap); minted.OwnerID() != OwnerProgram {
		t.Errorf("program mint owner = %q", minted.OwnerID())
	}
	tt.MintOwner = "module#test"
	if minted := tt.MintType("OwnTest2", TMap); minted.OwnerID() != "module#test" {
		t.Errorf("module mint owner = %q", minted.OwnerID())
	}
	// Clone and CloneDynamic both carry the mint owner forward.
	if c := tt.Clone(); c.MintOwner != "module#test" {
		t.Errorf("Clone lost MintOwner: %q", c.MintOwner)
	}
	if c := tt.CloneDynamic(); c.MintOwner != "module#test" {
		t.Errorf("CloneDynamic lost MintOwner: %q", c.MintOwner)
	}
	// An ordinary runtime value has no owner.
	if got := NewInteger(1).OwnerID(); got != "" {
		t.Errorf("scalar OwnerID = %q, want empty", got)
	}
	// SetOwner stamps through the shared tmeta.
	v := NewTypeLiteral(TMap)
	v.SetOwner("plugin:x")
	if v.OwnerID() != "plugin:x" {
		t.Errorf("SetOwner round-trip = %q", v.OwnerID())
	}
}

// TestRegisterTypeRequiresOwner pins the empty-owner compile failure on the
// unified registration path.
func TestRegisterTypeRequiresOwner(t *testing.T) {
	tt := NewDynamicTypeTable()
	if _, err := tt.RegisterType("NoOwnerRoot", 98800, "", nil); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("empty owner accepted: %v", err)
	}
	def, err := tt.RegisterType("OwnedRoot", 98801, "plugin:test", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if def.OwnerID() != "plugin:test" {
		t.Errorf("registered owner = %q", def.OwnerID())
	}
}

// TestIsNominalAnchor pins the classifier: nil and unowned decline;
// content-based behaviors decline; tag-carried types anchor.
func TestIsNominalAnchor(t *testing.T) {
	if IsNominalAnchor(nil) {
		t.Error("nil anchored")
	}
	adhoc := &Type{}
	if IsNominalAnchor(adhoc) {
		t.Error("unowned ad-hoc type anchored")
	}
	if !IsNominalAnchor(TInteger) {
		t.Error("kernel Integer must anchor (for kernel-authored sigs)")
	}
	tt := NewDynamicTypeTable()
	minted := tt.MintType("NomA", TMap)
	if !IsNominalAnchor(minted) {
		t.Error("bare mint (DefaultBehavior) must anchor")
	}
	member := tt.MintMemberType("MemA", TMap, func(Value) bool { return true })
	if IsNominalAnchor(member) {
		t.Error("member (content) type must not anchor")
	}
}

// TestContentMembershipInventory pins the set of content-deciding
// behaviors carrying the marker — a new content behavior missing it
// would silently become anchor-eligible and break the reachability
// theorem, so the inventory is explicit.
func TestContentMembershipInventory(t *testing.T) {
	content := []TypeBehavior{
		MemberUnifier{},
		&DepScalarUnifier{},
		&DisjunctUnifier{},
		&NegationUnifier{},
		&PredicateUnifier{},
		&genParamUnifier{},
		&schemaUnifier{},
		typeMembershipBehavior{},
		&surfaceUnifier{},
		&BindingBodyUnifier{},
	}
	for i, b := range content {
		if _, ok := b.(ContentMembership); !ok {
			t.Errorf("content behavior %d (%T) missing the ContentMembership marker", i, b)
		}
	}
	for _, b := range []TypeBehavior{DefaultBehavior, &bareRefineUnifier{}} {
		if _, ok := b.(ContentMembership); ok {
			t.Errorf("nominal behavior %T wrongly marked content-based", b)
		}
	}
}

// delegatingBehavior is a test stand-in for behave's userBehavior: a
// wrapper that DELEGATES Match to an inner behavior (MatchDelegating).
type delegatingBehavior struct {
	defaultBehavior
	inner TypeBehavior
}

func (d delegatingBehavior) DelegatesMatchTo() TypeBehavior { return d.inner }

// TestBehaviorIsContentWalksDelegation pins behaviorIsContent: it sees
// through a Match-delegating wrapper to a content behavior beneath (the
// behave-hole fix), while a Match-OVERRIDING wrapper (bareRefine, the
// unifiers) keeps its own nominal verdict.
func TestBehaviorIsContentWalksDelegation(t *testing.T) {
	// Direct content marker → content.
	if !behaviorIsContent(&PredicateUnifier{}) {
		t.Error("a marked content behavior must read as content")
	}
	// Plain nominal → not content.
	if behaviorIsContent(DefaultBehavior) {
		t.Error("DefaultBehavior is nominal, not content")
	}
	// Delegating wrapper over a content behavior → content (the hole fix).
	if !behaviorIsContent(delegatingBehavior{inner: &DepScalarUnifier{}}) {
		t.Error("a delegating wrapper over a predicate must read as content")
	}
	// Delegating wrapper over a nominal behavior → not content (no
	// over-exclusion: a behave'd refine still anchors).
	if behaviorIsContent(delegatingBehavior{inner: &bareRefineUnifier{}}) {
		t.Error("a delegating wrapper over a nominal behavior stays nominal")
	}
	// Delegating wrapper whose inner is nil → walk terminates, not content.
	if behaviorIsContent(delegatingBehavior{inner: nil}) {
		t.Error("a delegating wrapper over nil terminates as nominal")
	}
	// A Match-OVERRIDING wrapper (bareRefine) does NOT implement
	// MatchDelegating, so its own nominal verdict stands even if it embeds
	// a content parent — the walk must not reach past it.
	if _, ok := interface{}(&bareRefineUnifier{}).(MatchDelegating); ok {
		t.Error("bareRefineUnifier must not be MatchDelegating")
	}
}

// TestSigAnchorHelpers pins sigHasOwnedAnchor / sigHasModuleAnchor.
func TestSigAnchorHelpers(t *testing.T) {
	tt := NewDynamicTypeTable()
	tt.MintOwner = "module#a"
	modT := tt.MintType("AnchorA", TMap)
	progTT := NewDynamicTypeTable()
	progT := progTT.MintType("AnchorP", TMap)

	sig := &Signature{Args: []*Type{TInteger, modT}}
	if !sigHasOwnedAnchor(sig, "module#a") {
		t.Error("owned anchor not found")
	}
	if sigHasOwnedAnchor(sig, "module#b") {
		t.Error("foreign owner accepted")
	}
	if sigHasOwnedAnchor(sig, "") {
		t.Error("empty owner accepted")
	}
	if !sigHasModuleAnchor(sig) {
		t.Error("module anchor not found")
	}
	kernelOnly := &Signature{Args: []*Type{TInteger, TBoolean}}
	if sigHasModuleAnchor(kernelOnly) {
		t.Error("kernel tuple claimed a module anchor")
	}
	progOnly := &Signature{Args: []*Type{progT}}
	if sigHasModuleAnchor(progOnly) {
		t.Error("program mint accepted as a MODULE anchor")
	}
	// The non-anchor skip arm: a nil position and a content-based type
	// are both passed over before the module anchor is found.
	member := tt.MintMemberType("SkipMem", TMap, func(Value) bool { return true })
	skippy := &Signature{Args: []*Type{nil, member, modT}}
	if !sigHasModuleAnchor(skippy) {
		t.Error("module anchor not found past nil/content positions")
	}
}

// TestRequireOwnedAnchorModuleCarveout pins the transplant re-export
// arm: a sig anchored on a FOREIGN module's type passes only when
// allowModuleAnchors is set (source-clone transplants), and stays
// declined on the strict def-time path.
func TestRequireOwnedAnchorModuleCarveout(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tt := NewDynamicTypeTable()
	tt.MintOwner = "module#inner"
	innerT := tt.MintType("InnerAnchor", TMap)
	sigs := []Signature{{Args: []*Type{innerT}}}
	if err := requireOwnedAnchor(r, "w", "import", sigs, "module#outer", true); err != nil {
		t.Errorf("re-export carveout declined a module-minted anchor: %v", err)
	}
	if err := requireOwnedAnchor(r, "w", "def", sigs, "module#outer", false); err == nil {
		t.Error("strict path accepted a foreign module anchor")
	}
}

// TestFlexLiteralUnifiesCarrier pins the flex-retag gap fix: a
// check-mode carrier tagged at (or under) the flex type satisfies the
// flex literal; foreign carriers still decline.
func TestFlexLiteralUnifiesCarrier(t *testing.T) {
	if _, ok := Unify(NewCarrier(TFlexMap), NewTypeLiteral(TFlexMap)); !ok {
		t.Error("FlexMap carrier declined by its own literal")
	}
	if _, ok := Unify(NewCarrier(TWeakFlexMap), NewTypeLiteral(TFlexMap)); !ok {
		t.Error("subtype carrier declined by the parent flex literal")
	}
	if _, ok := Unify(NewCarrier(TInteger), NewTypeLiteral(TFlexMap)); ok {
		t.Error("foreign carrier accepted by the flex literal")
	}
}
