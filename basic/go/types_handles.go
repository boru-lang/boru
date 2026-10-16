package basic

import (
	"fmt"

	core "github.com/boru-lang/boru/core/go"
)

// The opaque-handle content types — Ideal/Patrun (5004), Ideal/Pid
// (5007), Ideal/Service (5008) — moved out of lang's word files
// (ADR-013 staged follow-up; audit §4 "weak-coupling tier"). The type
// identity, registration, and Behavior shell live here, in the types'
// owning component; the domain machinery (the patrun matcher/side
// table, the service handler state) stays with the words in lang and
// is reached through the small delegation interfaces below — never by
// an upward import. Pid needs no interface: its handle is the kernel's
// own *core.Process.

// TPatrun is Ideal/Patrun — the pattern-matching dispatch table.
var TPatrun = RegisterPatrunType()

// TPid is Ideal/Pid — the opaque process handle.
var TPid = RegisterPidType()

// TService is Ideal/Service — the in-process service handle.
var TService = RegisterServiceType()

// RegisterPatrunType / RegisterPidType / RegisterServiceType register
// the handle types with their wire-stable FixedIDs. Exported (rather
// than one shared helper) so the language layer's coverage tests can
// drive each duplicate-registration error arm individually, exactly as
// they did when the registrations lived there.
func RegisterPatrunType() *core.Type {
	return registerHandleType("Ideal/Patrun", 5004, PatrunBehavior{})
}

func RegisterPidType() *core.Type {
	return registerHandleType("Ideal/Pid", 5007, PidBehavior{})
}

func RegisterServiceType() *core.Type {
	return registerHandleType("Ideal/Service", 5008, ServiceBehavior{})
}

func registerHandleType(path string, fixedID int, b core.TypeBehavior) *core.Type {
	t, err := core.Builtin.RegisterType(path, fixedID, core.OwnerKernel, b)
	if err != nil {
		// Init-time registration error — recorded, not panicked (ADR-005).
		recordTypeInitErr(fmt.Errorf("types_handles: register %s: %w", path, err))
	}
	return t
}

// PatrunFormatter is the delegation seam PatrunBehavior renders
// through: the language layer's matcher (the ExtensionPayload body of
// every concrete Patrun value) implements it.
type PatrunFormatter interface{ FormatPatrun() string }

// ServiceFormatter is the same seam for Service values; the language
// layer's service state implements it (rendering "Service(…)" or
// "Endpoint(…)" for a connected remote).
type ServiceFormatter interface{ FormatService() string }

// PatrunBehavior renders a Patrun via its matcher's own formatter;
// Match and Equal defer to the kernel default (nominal identity).
type PatrunBehavior struct{}

func (PatrunBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (PatrunBehavior) Equal(a, b Value) bool       { return core.DefaultBehavior.Equal(a, b) }
func (PatrunBehavior) Format(v Value) string {
	if ep, ok := DataOf(&v).(ExtensionPayload); ok {
		if f, ok := ep.Body.(PatrunFormatter); ok {
			return f.FormatPatrun()
		}
	}
	return "Patrun()"
}

// NewPid wraps a process handle as a boru Pid value.
func NewPid(p *core.Process) Value {
	return core.NewExtension(TPid, p)
}

// PidProcess unwraps a Pid value to its process — the hook a Go
// driver uses to deliver messages to a boru-held pid (boru:tui's
// deliver-events). The bool is false for non-Pid values.
func PidProcess(v Value) (*core.Process, bool) {
	return AsPid(v)
}

// AsPid unwraps the process handle behind a Pid value.
func AsPid(v Value) (*core.Process, bool) {
	ep, ok := DataOf(&v).(ExtensionPayload)
	if !ok {
		return nil, false
	}
	p, ok := ep.Body.(*core.Process)
	return p, ok
}

// PidBehavior renders a Pid as "Pid<P_…>"; identity is the process handle.
type PidBehavior struct{}

func (PidBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (PidBehavior) Format(v Value) string {
	if p, ok := AsPid(v); ok {
		return "Pid<" + p.ID + ">"
	}
	return "Pid"
}
func (PidBehavior) Equal(a, b Value) bool {
	pa, oka := AsPid(a)
	pb, okb := AsPid(b)
	if !oka || !okb {
		return core.DefaultBehavior.Equal(a, b)
	}
	return pa == pb
}

// ServiceBehavior renders a Service via its state's own formatter;
// identity is the underlying handle pointer.
type ServiceBehavior struct{}

func (ServiceBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }
func (ServiceBehavior) Equal(a, b Value) bool {
	fa, oka := serviceHandle(a)
	fb, okb := serviceHandle(b)
	if !oka || !okb {
		return core.DefaultBehavior.Equal(a, b)
	}
	return fa == fb
}
func (ServiceBehavior) Format(v Value) string {
	if f, ok := serviceHandle(v); ok {
		return f.FormatService()
	}
	return "Service"
}

func serviceHandle(v Value) (ServiceFormatter, bool) {
	ep, ok := DataOf(&v).(ExtensionPayload)
	if !ok {
		return nil, false
	}
	f, ok := ep.Body.(ServiceFormatter)
	return f, ok
}
