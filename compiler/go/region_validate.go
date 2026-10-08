package compiler

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// Validate rejects a malformed region descriptor before anything executes
// it (design/FULL-COMPILATION.0.md §6.2, Stage 4).
//
// The sentinel zero is only half a defence. SlotNone exists so a missed
// initialisation cannot masquerade as "Consts[0]" — the failure
// eng/go/CLAUDE.md's "No Zero-Value Overload (CRITICAL)" names — but a
// sentinel nothing checks just moves the silent wrong answer one step later,
// to whatever reads Idx. This is the check.
//
// It returns an error rather than panicking: a malformed descriptor is a
// compiler defect, and the design's own Stage 9 note says such a case
// becomes a structured internal_error return, never a panic (ADR-005 has no
// exception for "this should be impossible").
func (d *RegionDesc) Validate(nConsts, nFns, nTypes int) error {
	if d == nil {
		return fmt.Errorf("region descriptor is nil")
	}
	if d.Lead == LeadWord && d.Word == "" {
		return fmt.Errorf("region at %v: LeadWord with no word name", d.Pos)
	}
	if d.Lead != LeadWord && d.Word != "" {
		return fmt.Errorf("region at %v: word name %q on a non-word lead", d.Pos, d.Word)
	}
	if d.Mods != nil && d.Mods.Name != d.Word {
		return fmt.Errorf("region at %v: lead modifiers name %q, not the lead %q", d.Pos, d.Mods.Name, d.Word)
	}
	if d.NFwd < 0 || d.NFwd > len(d.Slots) {
		return fmt.Errorf("region at %v: claim bound %d outside the region's %d slots",
			d.Pos, d.NFwd, len(d.Slots))
	}
	for i := range d.Slots {
		// Beyond the recorded claim the invalid zero is not a defect: the slot
		// is inside the region's syntactic span but was not this dispatch's
		// operand, so no source was ever owed (see RegionDesc.NFwd). The
		// runtime defers if a live collection reaches it. Everything else
		// about the slot is still checked — a source that IS set out there is
		// a lowerer writing past its own claim.
		if i >= d.NFwd && d.Slots[i].Source == SlotNone {
			if d.Slots[i].Idx != 0 || d.Slots[i].ResIdx != 0 {
				return fmt.Errorf("region at %v: slot %d is beyond the claim (NFwd %d) "+
					"but carries index %d/%d", d.Pos, i, d.NFwd, d.Slots[i].Idx, d.Slots[i].ResIdx)
			}
			continue
		}
		if err := d.Slots[i].validate(i, d.Pos, nConsts, nFns, nTypes); err != nil {
			return err
		}
	}
	return nil
}

// validate checks one slot. The bounds are checked against the tables the
// index actually addresses, because an in-range-but-wrong index is the
// failure a sentinel cannot catch.
func (s *SlotDesc) validate(i int, pos core.SrcPos, nConsts, nFns, nTypes int) error {
	switch s.Source {
	case SlotNone:
		return fmt.Errorf("region at %v: slot %d was never given a source "+
			"(SlotNone is the invalid zero, not a reference to Consts[0])", pos, i)
	case SlotConst:
		if s.Idx < 0 || s.Idx >= nConsts {
			return fmt.Errorf("region at %v: slot %d const index %d out of range (%d consts)",
				pos, i, s.Idx, nConsts)
		}
	case SlotGroup:
		if s.Idx < 0 || s.Idx >= nFns {
			return fmt.Errorf("region at %v: slot %d fragment index %d out of range (%d fns)",
				pos, i, s.Idx, nFns)
		}
	case SlotType:
		if s.Idx < 0 || s.Idx >= nTypes {
			return fmt.Errorf("region at %v: slot %d type index %d out of range (%d types)",
				pos, i, s.Idx, nTypes)
		}
	case SlotWordRef:
		// A wordRef addresses no table: the name is in Token. A non-zero Idx
		// is a lowerer that thought it was writing an address, and whatever
		// read it would be indexing a table this source never meant.
		if s.Idx != 0 {
			return fmt.Errorf("region at %v: slot %d is a wordRef with index %d "+
				"(a wordRef addresses no table — its name is in Token)", pos, i, s.Idx)
		}
	case SlotLocal, SlotEvent:
		// Frame-local and event indices are validated by the lowerer against
		// the unit being built, which knows its own local count and event
		// sequence; there is no program-wide table to check them against
		// here. Only their sign is meaningful at this level.
		if s.Idx < 0 {
			return fmt.Errorf("region at %v: slot %d has a negative index %d", pos, i, s.Idx)
		}
	default:
		return fmt.Errorf("region at %v: slot %d has an unknown source %d", pos, i, s.Source)
	}
	// ResIdx names a result WITHIN a producing event, so it is meaningless
	// anywhere else. A non-zero value on another source is a lowerer defect
	// that would otherwise be read as "result N" by whatever consumes the
	// slot — the same class of silent wrong answer SlotNone exists to stop,
	// one field over.
	if s.ResIdx < 0 {
		return fmt.Errorf("region at %v: slot %d has a negative result index %d", pos, i, s.ResIdx)
	}
	if s.ResIdx != 0 && s.Source != SlotEvent {
		return fmt.Errorf("region at %v: slot %d carries result index %d on a non-event source %d",
			pos, i, s.ResIdx, s.Source)
	}
	return nil
}
