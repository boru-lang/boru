package basic

// Scalar/Micron — the structured-scalar family.
//
// A Micron is a value that behaves as a scalar — content equality (two
// values with the same fields ARE equal), immutability, a place in the
// total order — yet is object-like, exposing named read-only
// properties via dot/get (the property signatures live in the language
// layer; the kernel owns identity, construction, and ordering).
//
// The family:
//
//	Scalar/Micron            — abstract root; `refine Micron {fields}`
//	                           constructs user kinds.
//	Scalar/Micron/Pathon     — filesystem-style path (formerly
//	                           Scalar/Path; PathonPayload).
//	Scalar/Micron/Emailon    — email address (MicronPayload:
//	                           user, host; derived address).
//	Scalar/Micron/Urlon      — absolute URL (MicronPayload: scheme,
//	                           host, port?, path?, query?, fragment?;
//	                           derived href).
//
// Every type under Micron must have a name ending in the suffix "on"
// (requireMicronName — enforced at every bind site in core_type.go /
// define_type.go). The rule is one-directional: names ending in "on"
// elsewhere in the lattice (Duration, Function, Negation, …) are not
// Microns.
//
// Construction is `make` / `refine` via the Micron Ideal
// (registerKernelIdeals): Node (map of named fields) AND parsed string
// forms are both validated, and the map form re-renders through the
// string parser so there is exactly ONE validator per kind.

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"mime"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/apd/v3"

	core "github.com/boru-lang/boru/core/go"
)

// requireMicronName enforces the Micron naming rule: the last
// slash-part of a name bound under Scalar/Micron must end in the
// suffix "on" (case-sensitive — `BARON` is rejected) and be at least
// three characters (rejects the bare `On`). Callers gate on the
// about-to-mint parent conforming to TMicron; the rule is
// one-directional (Micron ⇒ -on, never -on ⇒ Micron).
func requireMicronName(name string) error {
	part := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		part = name[i+1:]
	}
	if len(part) >= 3 && strings.HasSuffix(part, "on") {
		return nil
	}
	return &BoruError{
		Code:   "micron_name",
		Detail: "type " + name + ": names under Scalar/Micron must end in the suffix 'on'",
		Hint:   "rename it — e.g. " + part + "on",
	}
}

// ---- behavior ----

// micronBehavior is the one TypeBehavior for the whole family,
// installed on the root, every builtin leaf, and every user mint (leaf
// installation is load-bearing: ValuesEqual routes to Behavior only
// when both sides share a Parent with a non-default Behavior, and
// scalarSemanticEqual needs the LCA Comparer for newtype-vs-base).
//
//   - Match: nominal (embedded default — ConformsTo).
//   - Equal: content — Pathon by segments+abs, MicronPayload by field
//     map.
//   - Compare: family order. A leaf instance resolves against its own
//     kind (Pathon keeps the verbatim comparePathons ordering; other
//     kinds order by canonical render); the root opts out for
//     cross-kind pairs (ErrNoComparer → Rank fallback → cmp/lt raise
//     incomparable, tcmp orders by Rank).
//   - Format: canonical render (Emailon → address, Urlon → href, user
//     kinds → field-map render); Pathon delegates to the kernel
//     default (segments join).
type micronBehavior struct {
	kind *Type
	// info is the field schema for user-minted kinds (set by
	// InstallType's Micron branch); nil for the root and the builtin
	// leaves. micronInstantiate walks the parent chain for it when
	// constructing a newtype of a user kind.
	info *MicronTypeInfo
}

func (micronBehavior) Match(v Value, t *Type) bool { return core.DefaultBehavior.Match(v, t) }

func (micronBehavior) Format(v Value) string {
	if IsMicronValue(v) {
		return micronRender(v)
	}
	if info, ok := DataOf(&v).(MicronTypeInfo); ok {
		// A type body renders as its bound name; an anonymous body
		// (not yet def-bound) as its schema.
		if info.Name != "" {
			return info.Name
		}
		return "Micron " + NewMap(info.Fields).String()
	}
	return core.DefaultBehavior.Format(v)
}

func (micronBehavior) Equal(a, b Value) bool {
	if ap, ok := DataOf(&a).(PathonPayload); ok {
		if bp, ok := DataOf(&b).(PathonPayload); ok {
			return PathonContentEqual(ap.Info, bp.Info)
		}
		return false
	}
	if am, ok := DataOf(&a).(MicronPayload); ok {
		if bm, ok := DataOf(&b).(MicronPayload); ok {
			return core.MicronFieldsEqual(am.Fields, bm.Fields)
		}
		return false
	}
	return core.DefaultBehavior.Equal(a, b)
}

func (mb micronBehavior) Compare(a, b Value) (int, error) {
	// Type-literal-first rule, inside the family only (the cross-family
	// scalar catch-all on TScalar stays Rank-only).
	if c, ok := core.LitVsConcreteOrder(a, b); ok {
		return c, nil
	}
	if IsBareTypeNode(&a) && IsBareTypeNode(&b) {
		return core.LitVsLitOrder(a, b), nil
	}
	// Two concrete Microns. This Comparer owns the pair only when it
	// sits on a leaf kind — the LCA walk lands there exactly when both
	// operands share that kind (including newtype-vs-base). On the
	// root the operands are of DIFFERENT kinds: opt out, so the
	// restricted words (cmp/lt/…) raise incomparable and tcmp falls to
	// the Rank order.
	if mb.kind == nil || mb.kind.Equal(TMicron) {
		return 0, ErrNoComparer
	}
	if mb.kind.ConformsTo(TPathon) {
		return core.ComparePathons(a, b), nil
	}
	if mb.kind.ConformsTo(TSemveron) {
		return compareSemverons(a, b), nil
	}
	if mb.kind.ConformsTo(TCidron) {
		return compareCidrons(a, b), nil
	}
	if mb.kind.ConformsTo(TColoron) {
		return compareColorons(a, b), nil
	}
	if mb.kind.ConformsTo(TQion) {
		return compareQions(a, b), nil
	}
	return strings.Compare(micronCompareKey(a), micronCompareKey(b)), nil
}

// micronCompareKey is the ordering key for non-Pathon Micron leaves:
// the canonical render, computed directly (not via the receiver's
// Behavior walk, which a newtype's wrapper Behavior would reroute) so
// newtype-vs-base pairs with equal content compare equal.
func micronCompareKey(v Value) string {
	if IsMicronValue(v) {
		return micronRender(v)
	}
	return CanonValue(v)
}

// micronRender is the canonical render for MicronPayload instances:
// Emailon → the address (user@host), Urlon → the href, user kinds →
// the field-map render. Also the Compare key for non-Pathon leaves.
func micronRender(v Value) string {
	fields, err := AsMicronFields(v)
	if err != nil {
		return core.DefaultBehavior.Format(v)
	}
	switch {
	case ParentOf(&v).ConformsTo(TEmailon):
		return micronEmailAddress(fields)
	case ParentOf(&v).ConformsTo(TUrlon):
		return micronURLHref(fields)
	case ParentOf(&v).ConformsTo(TIpon):
		return micronFieldString(fields, "addr")
	case ParentOf(&v).ConformsTo(THoston):
		return micronHostonAuthority(fields)
	case ParentOf(&v).ConformsTo(TSemveron):
		return micronSemveronRender(fields)
	case ParentOf(&v).ConformsTo(TCidron):
		return micronFieldString(fields, "cidr")
	case ParentOf(&v).ConformsTo(TMacon):
		return micronFieldString(fields, "addr")
	case ParentOf(&v).ConformsTo(TColoron):
		return micronColoronHex(fields)
	case ParentOf(&v).ConformsTo(TMimon):
		return micronMimonRender(fields)
	case ParentOf(&v).ConformsTo(TQion):
		return micronQionRender(fields)
	case ParentOf(&v).ConformsTo(TPhonon):
		return "+" + micronFieldString(fields, "digits")
	default:
		return NewMap(fields).String()
	}
}

func micronFieldString(fields *OrderedMap, key string) string {
	v, ok := fields.Get(key)
	if !ok {
		return ""
	}
	s, _ := AsString(v)
	return s
}

// micronEmailAddress synthesizes Emailon's derived address property:
// user@host.
func micronEmailAddress(fields *OrderedMap) string {
	return micronFieldString(fields, "user") + "@" + micronFieldString(fields, "host")
}

// micronURLHref synthesizes Urlon's derived href property:
// scheme://host[:port][path][?query][#fragment].
func micronURLHref(fields *OrderedMap) string {
	var sb strings.Builder
	sb.WriteString(micronFieldString(fields, "scheme"))
	sb.WriteString("://")
	sb.WriteString(micronFieldString(fields, "host"))
	if p, ok := fields.Get("port"); ok {
		n, _ := AsInteger(p)
		sb.WriteString(":")
		sb.WriteString(strconv.FormatInt(n, 10))
	}
	sb.WriteString(micronFieldString(fields, "path"))
	if q := micronFieldString(fields, "query"); q != "" {
		sb.WriteString("?")
		sb.WriteString(q)
	}
	if f := micronFieldString(fields, "fragment"); f != "" {
		sb.WriteString("#")
		sb.WriteString(f)
	}
	return sb.String()
}

// micronHostonAuthority synthesizes Hoston's derived authority property:
// host[:port], bracketing an IPv6 host when a port is present
// ([::1]:8080), matching net.JoinHostPort's convention.
func micronHostonAuthority(fields *OrderedMap) string {
	host := micronFieldString(fields, "host")
	if p, ok := fields.Get("port"); ok {
		n, _ := AsInteger(p)
		return micronHostonRender(host, n, true)
	}
	return micronHostonRender(host, 0, false)
}

func init() {
	// Attach the family Behavior to the root and every builtin leaf.
	// The identities are kernel-declared (eng builtinDecls — the
	// Resource/Entity precedent); the content layer attaches their
	// Behaviors at import, exactly as the RegisterType path would.
	// Package init is single-threaded, so the global-table writes are
	// safe; T* pointers are live because basic's aliases initialise
	// before init() runs.
	for _, t := range []*Type{
		TMicron, TPathon, TEmailon, TUrlon, TIpon, THoston, TSemveron,
		TCidron, TMacon, TColoron, TMimon, TQion, TPhonon,
	} {
		t.SetBehavior(micronBehavior{kind: t})
	}
	// The display-string backstop for wrapper-Behavior micron values
	// renders through this bridge (eng value.go).
	core.RegisterMicronRenderBridge(micronRender)
}

// ValidateSubtypeName implements core.SubtypeNamer (ADR-012 rule 5): the
// family's -on naming rule, applied by the kernel to every subtype
// minted beneath a Micron kind — typed defs, bare-nominal refines,
// dependent scalars, host member types.
func (micronBehavior) ValidateSubtypeName(name string) error {
	return requireMicronName(name)
}

// MintSubtypeBehavior implements core.MicronSubtypeMinter: a
// `refine Micron {fields}` body bound to a capitalised name mints its
// subtype with the family Behavior carrying the schema, so `make`
// finds it through the parent walk.
func (micronBehavior) MintSubtypeBehavior(def *Type, info *MicronTypeInfo) TypeBehavior {
	return micronBehavior{kind: def, info: info}
}

// InstallMicronIdeals registers the family's Ideal descriptor on r —
// `refine Micron {fields}` construction and `make ‹kind› data`
// instantiation. The kernel no longer registers the family
// (registerKernelIdeals dropped it with the content move); every
// registry that should construct microns installs this, the way
// lang's installIdeals wires Object/Record/Table.
func InstallMicronIdeals(r *Registry) {
	r.Ideals.Register(&core.Ideal{
		Name:      "Micron",
		Enabled:   true,
		Accepts:   micronAccepts,
		Construct: micronConstruct,
		// micronInstantiate is a pure literal parse — string/map in,
		// MicronPayload value out, no host effects — so the check-mode
		// `make` model may execute it over concrete data.
		Instantiate:     micronInstantiate,
		PureInstantiate: true,
	})
}

// MicronProperty reads a named property of a Micron instance: the
// primary fields, plus the synthesized derived properties — Pathon's
// parts/abs, Emailon's address, Urlon's href. Derived values are
// computed at read time and never stored, so equality stays a
// primary-field comparison. Returns ok=false on a miss (the language
// layer maps a miss to `none` for get/dot and to a strict error for
// getr/dotr).
func MicronProperty(v Value, key string) (Value, bool) {
	if p, ok := DataOf(&v).(PathonPayload); ok {
		switch key {
		case "parts":
			elems := make([]Value, len(p.Info.Parts))
			for i, s := range p.Info.Parts {
				elems[i] = NewString(s)
			}
			return NewList(elems), true
		case "abs":
			return NewBoolean(p.Info.Abs), true
		case "volume":
			// The Windows drive ("C:") or UNC root ("\\server\share");
			// empty for a POSIX / driveless path.
			return NewString(p.Info.Volume), true
		}
		return Value{}, false
	}
	fields, err := AsMicronFields(v)
	if err != nil {
		return Value{}, false
	}
	if val, ok := fields.Get(key); ok {
		return val, true
	}
	switch {
	case ParentOf(&v).ConformsTo(TEmailon) && key == "address":
		return NewString(micronEmailAddress(fields)), true
	case ParentOf(&v).ConformsTo(TUrlon) && key == "href":
		return NewString(micronURLHref(fields)), true
	case ParentOf(&v).ConformsTo(THoston) && key == "authority":
		return NewString(micronHostonAuthority(fields)), true
	case ParentOf(&v).ConformsTo(TSemveron):
		switch key {
		case "prereleaseParts":
			return semveronParts(micronFieldString(fields, "prerelease")), true
		case "buildParts":
			return semveronParts(micronFieldString(fields, "build")), true
		case "release":
			return NewString(fmt.Sprintf("%d.%d.%d",
				micronFieldInt(fields, "major"),
				micronFieldInt(fields, "minor"),
				micronFieldInt(fields, "patch"))), true
		case "stable":
			return NewBoolean(micronFieldString(fields, "prerelease") == ""), true
		case "version":
			return NewString(micronSemveronRender(fields)), true
		}
	case ParentOf(&v).ConformsTo(TCidron):
		switch key {
		case "hostbits":
			return NewInteger(cidronHostbits(fields)), true
		case "count":
			n := new(big.Int).Lsh(big.NewInt(1), uint(cidronHostbits(fields)))
			if n.IsInt64() {
				return NewInteger(n.Int64()), true
			}
			return NewBigInteger(n), true
		}
	case ParentOf(&v).ConformsTo(TMacon):
		hw, ok := maconBytes(fields)
		if !ok {
			return Value{}, false
		}
		switch key {
		case "bits":
			return NewInteger(int64(len(hw) * 8)), true
		case "eui64":
			return NewBoolean(len(hw) == 8), true
		case "oui":
			return NewString(hw[:3].String()), true
		case "multicast":
			return NewBoolean(hw[0]&1 != 0), true
		case "local":
			return NewBoolean(hw[0]&2 != 0), true
		case "broadcast":
			return NewBoolean(maconAllOnes(hw)), true
		}
	case ParentOf(&v).ConformsTo(TColoron):
		switch key {
		case "hex":
			return NewString(micronColoronHex(fields)), true
		case "opaque":
			return NewBoolean(micronFieldInt(fields, "a") == 255), true
		case "alpha":
			return NewFloat(float64(micronFieldInt(fields, "a")) / 255), true
		case "css":
			return NewString(coloronCSS(fields)), true
		}
	case ParentOf(&v).ConformsTo(TMimon):
		switch key {
		case "essence":
			return NewString(micronFieldString(fields, "type") + "/" + micronFieldString(fields, "subtype")), true
		case "mime":
			return NewString(micronMimonRender(fields)), true
		case "suffix":
			sub := micronFieldString(fields, "subtype")
			if i := strings.LastIndexByte(sub, '+'); i >= 0 && i < len(sub)-1 {
				return NewString(sub[i+1:]), true
			}
			return Value{}, false
		case "facet":
			sub := micronFieldString(fields, "subtype")
			if i := strings.IndexByte(sub, '.'); i > 0 {
				return NewString(sub[:i]), true
			}
			return Value{}, false
		}
	case ParentOf(&v).ConformsTo(TQion):
		switch key {
		case "units":
			return qionUnits(fields)
		case "negative":
			amt, ok := fields.Get("amount")
			if !ok {
				return Value{}, false
			}
			d, derr := AsBigDecimal(amt)
			return NewBoolean(derr == nil && d.Negative && !d.IsZero()), true
		case "display":
			return NewString(micronQionRender(fields)), true
		}
	case ParentOf(&v).ConformsTo(TPhonon):
		switch key {
		case "e164":
			return NewString("+" + micronFieldString(fields, "digits")), true
		case "length":
			return NewInteger(int64(len(micronFieldString(fields, "digits")))), true
		}
	}
	return Value{}, false
}

// semveronParts splits a dot-separated prerelease/build string into a
// List of String identifiers; an empty string yields an empty list.
func semveronParts(s string) Value {
	if s == "" {
		return NewList(nil)
	}
	segs := strings.Split(s, ".")
	elems := make([]Value, len(segs))
	for i, seg := range segs {
		elems[i] = NewString(seg)
	}
	return NewList(elems)
}

// MicronSchemaFor returns the field schema of the nearest user-defined
// Micron kind on t's parent chain (the schema a `refine Micron
// {fields}` construction attached at bind time). ok=false for the
// builtin leaves and the root.
func MicronSchemaFor(t *Type) (*OrderedMap, bool) {
	for ; t != nil && t.ConformsTo(TMicron); t = ParentOf(t) {
		if mb, ok := t.Behavior().(micronBehavior); ok && mb.info != nil {
			return mb.info.Fields, true
		}
	}
	return nil, false
}

// ---- construction ----

// makeEmailon builds an Emailon from a plain-address string
// ("alice@example.com") or a {user host} map. The map form re-renders
// and re-validates through the string path — one validator.
func makeEmailon(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return emailonFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Emailon")
		if err != nil {
			return nil, err
		}
		var user, host string
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			sv, serr := v.AsConcreteString()
			if serr != nil {
				return nil, &BoruError{Code: "type_error",
					Detail: fmt.Sprintf("make: Emailon field %s must be a String, got %s", k, v.String())}
			}
			switch k {
			case "user":
				user = sv
			case "host":
				host = sv
			default:
				return nil, &BoruError{Code: "type_error",
					Detail: "make: Emailon has no field " + k + " (fields: user, host)"}
			}
		}
		if user == "" || host == "" {
			return nil, &BoruError{Code: "type_error",
				Detail: "make: Emailon requires both user and host fields"}
		}
		return emailonFromString(user + "@" + host)
	}
	return nil, &BoruError{Code: "type_error",
		Detail: fmt.Sprintf("make: Emailon source must be a string or map, got %s", src.String())}
}

func emailonFromString(s string) ([]Value, error) {
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return nil, &BoruError{Code: "type_error",
			Detail: fmt.Sprintf("make: invalid email address %q: %v", s, err)}
	}
	// Reject display-name / angle-bracket forms ("Alice <a@x.com>") —
	// an Emailon is the plain address only.
	if addr.Name != "" || addr.Address != s {
		return nil, &BoruError{Code: "type_error",
			Detail: fmt.Sprintf("make: Emailon takes a plain user@host address, got %q", s)}
	}
	at := strings.LastIndex(s, "@")
	fields := NewOrderedMap()
	fields.Set("user", NewString(s[:at]))
	fields.Set("host", NewString(s[at+1:]))
	return []Value{NewValueRaw(TEmailon, MicronPayload{Fields: fields})}, nil
}

// makeIpon builds an Ipon from an IPv4/IPv6 address string (validated
// and canonicalized via net.ParseIP) or an {addr} map. Content-equal by
// construction: alternate spellings of the same address —
// `2001:0db8::1` / `2001:db8::1`, or an IPv4-mapped `::ffff:10.0.0.7` /
// `10.0.0.7` — all canonicalize to one stored `addr`.
func makeIpon(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return iponFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Ipon")
		if err != nil {
			return nil, err
		}
		var addr string
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			sv, serr := v.AsConcreteString()
			if serr != nil {
				return nil, &BoruError{Code: "type_error",
					Detail: fmt.Sprintf("make: Ipon field %s must be a String, got %s", k, v.String())}
			}
			if k != "addr" {
				return nil, &BoruError{Code: "type_error",
					Detail: "make: Ipon has no field " + k + " (field: addr)"}
			}
			addr = sv
		}
		if addr == "" {
			return nil, &BoruError{Code: "type_error", Detail: "make: Ipon requires an addr field"}
		}
		return iponFromString(addr)
	}
	return nil, &BoruError{Code: "type_error",
		Detail: fmt.Sprintf("make: Ipon source must be a string or map, got %s", src.String())}
}

func iponFromString(s string) ([]Value, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, &BoruError{Code: "type_error",
			Detail: fmt.Sprintf("make: invalid IP address %q (want IPv4 or IPv6, e.g. 203.0.113.7 or 2001:db8::1)", s)}
	}
	version := int64(6)
	if ip.To4() != nil {
		version = 4
	}
	fields := NewOrderedMap()
	fields.Set("addr", NewString(ip.String())) // canonical form
	fields.Set("version", NewInteger(version))
	return []Value{NewValueRaw(TIpon, MicronPayload{Fields: fields})}, nil
}

// makeHoston builds a Hoston — a "host:port" network authority with an
// OPTIONAL port — from a string ("example.com:8080", "example.com",
// "[::1]:8080", "2001:db8::1") or a {host port?} map. The map form
// re-renders and re-validates through the string parser — one validator.
func makeHoston(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return hostonFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Hoston")
		if err != nil {
			return nil, err
		}
		host, hasHost := "", false
		port, hasPort := int64(0), false
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			switch k {
			case "host":
				sv, serr := v.AsConcreteString()
				if serr != nil {
					return nil, hostonErr(fmt.Sprintf("Hoston field host must be a String, got %s", v.String()))
				}
				host, hasHost = sv, true
			case "port":
				pv, perr := v.AsConcreteInteger()
				if perr != nil {
					return nil, hostonErr(fmt.Sprintf("Hoston field port must be an Integer, got %s", v.String()))
				}
				port, hasPort = pv, true
			default:
				return nil, hostonErr("Hoston has no field " + k + " (fields: host, port)")
			}
		}
		if !hasHost {
			return nil, hostonErr("Hoston requires a host field")
		}
		// Build directly from the two named fields — do NOT re-parse the host
		// as a full authority, or `{host:'a.com:80'}` would silently set a
		// port the caller never supplied (violating the {host port?} shape).
		if err := validHostonHostField(host); err != nil {
			return nil, err
		}
		if hasPort && (port < 0 || port > 65535) {
			return nil, hostonErr(fmt.Sprintf("Hoston port %d is not in 0..65535", port))
		}
		fields := NewOrderedMap()
		fields.Set("host", NewString(host))
		if hasPort {
			fields.Set("port", NewInteger(port))
		}
		return []Value{NewValueRaw(THoston, MicronPayload{Fields: fields})}, nil
	}
	return nil, hostonErr(fmt.Sprintf("Hoston source must be a string or map, got %s", src.String()))
}

// hostonFromString parses "host[:port]" (optional port; IPv6 hosts
// bracketed when a port is present) into a Hoston payload.
func hostonFromString(s string) ([]Value, error) {
	host, port, hasPort, err := splitHoston(s)
	if err != nil {
		return nil, err
	}
	fields := NewOrderedMap()
	fields.Set("host", NewString(host))
	if hasPort {
		fields.Set("port", NewInteger(port))
	}
	return []Value{NewValueRaw(THoston, MicronPayload{Fields: fields})}, nil
}

// splitHoston splits a host authority into its host and optional port,
// following net.SplitHostPort's bracketing rule for IPv6.
func splitHoston(s string) (host string, port int64, hasPort bool, err error) {
	if s == "" {
		return "", 0, false, hostonErr("Hoston requires a non-empty host")
	}
	if s[0] == '[' { // bracketed IPv6: [addr] or [addr]:port
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return "", 0, false, hostonErr(fmt.Sprintf("Hoston has an unclosed '[' in %q", s))
		}
		host = s[1:end]
		if net.ParseIP(host) == nil {
			return "", 0, false, hostonErr(fmt.Sprintf("Hoston bracketed host %q is not a valid IP", host))
		}
		rest := s[end+1:]
		if rest == "" {
			return host, 0, false, nil
		}
		if rest[0] != ':' {
			return "", 0, false, hostonErr(fmt.Sprintf("Hoston expected ':port' after ']' in %q", s))
		}
		if port, err = parseHostonPort(rest[1:]); err != nil {
			return "", 0, false, err
		}
		return host, port, true, nil
	}
	switch strings.Count(s, ":") {
	case 0: // bare host / IPv4 / hostname
		if err = validHostonHost(s); err != nil {
			return "", 0, false, err
		}
		return s, 0, false, nil
	case 1: // host:port
		i := strings.IndexByte(s, ':')
		host = s[:i]
		if err = validHostonHost(host); err != nil {
			return "", 0, false, err
		}
		if port, err = parseHostonPort(s[i+1:]); err != nil {
			return "", 0, false, err
		}
		return host, port, true, nil
	default: // multiple colons, unbracketed → a bare IPv6 literal, no port
		if net.ParseIP(s) == nil {
			return "", 0, false, hostonErr(fmt.Sprintf(
				"Hoston %q has multiple ':' but is not a valid IPv6 (bracket it as [addr]:port to add a port)", s))
		}
		return s, 0, false, nil
	}
}

// validHostonHost rejects an empty or whitespace-bearing host.
func validHostonHost(h string) error {
	if h == "" {
		return hostonErr("Hoston requires a non-empty host")
	}
	if strings.ContainsAny(h, " \t\r\n") {
		return hostonErr(fmt.Sprintf("Hoston host %q must not contain whitespace", h))
	}
	// A Hoston is a network authority (host[:port]), not a URL: the
	// authority delimiters end it, so a host must not contain them.
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		return hostonErr(fmt.Sprintf("Hoston host %q must not contain %q (it is a host, not a URL)", h, h[i:i+1]))
	}
	return nil
}

// validHostonHostField validates a Hoston map's `host` field as a HOST, not a
// full authority: a ':' is allowed only inside an IPv6 literal, so a
// "host:port" string in the host field is a loud error rather than quietly
// populating the optional port.
func validHostonHostField(h string) error {
	if err := validHostonHost(h); err != nil {
		return err
	}
	if strings.Contains(h, ":") && net.ParseIP(h) == nil {
		return hostonErr(fmt.Sprintf(
			"Hoston host %q is not valid (a ':' is only allowed in an IPv6 literal; put the port in the port field)", h))
	}
	return nil
}

// parseHostonPort parses a decimal port in the range 0..65535.
func parseHostonPort(s string) (int64, error) {
	p, err := strconv.ParseInt(s, 10, 64)
	if err != nil || p < 0 || p > 65535 {
		return 0, hostonErr(fmt.Sprintf("Hoston port %q is not an integer in 0..65535", s))
	}
	return p, nil
}

// micronHostonRender joins a host and port back into the string form
// (bracketing an IPv6 host), used to re-validate the map construction
// form through the single string parser.
func micronHostonRender(host string, port int64, hasPort bool) string {
	if !hasPort {
		return host
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + strconv.FormatInt(port, 10)
	}
	return host + ":" + strconv.FormatInt(port, 10)
}

func hostonErr(detail string) error {
	return &BoruError{Code: "type_error", Detail: "make: " + detail}
}

// ---- Semveron (Semantic Versioning 2.0.0) ----

// semveronFieldOrder is the canonical field layout of a Semveron map
// source (major/minor/patch are required; prerelease/build optional).
var semveronFieldOrder = []string{"major", "minor", "patch", "prerelease", "build"}

// makeSemveron builds a Semveron — a Semantic Versioning 2.0.0 version —
// from a string ("1.2.3", "1.0.0-rc.1+build.5") or a
// {major minor patch prerelease? build?} map. The map form re-renders
// to the canonical string and re-parses through the single validator.
func makeSemveron(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return semveronFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Semveron")
		if err != nil {
			return nil, err
		}
		var major, minor, patch int64
		var haveMajor, haveMinor, havePatch bool
		var prerelease, build string
		var havePrerelease, haveBuild bool
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			switch k {
			case "major", "minor", "patch":
				n, nerr := v.AsConcreteInteger()
				if nerr != nil {
					return nil, semveronErr(fmt.Sprintf("Semveron field %s must be an Integer, got %s", k, v.String()))
				}
				if n < 0 {
					return nil, semveronErr(fmt.Sprintf("Semveron field %s must be non-negative, got %d", k, n))
				}
				switch k {
				case "major":
					major, haveMajor = n, true
				case "minor":
					minor, haveMinor = n, true
				case "patch":
					patch, havePatch = n, true
				}
			case "prerelease":
				sv, serr := semveronIdentField(v, "prerelease")
				if serr != nil {
					return nil, serr
				}
				prerelease, havePrerelease = sv, true
			case "build":
				sv, serr := semveronIdentField(v, "build")
				if serr != nil {
					return nil, serr
				}
				build, haveBuild = sv, true
			default:
				return nil, semveronErr("Semveron has no field " + k +
					" (fields: " + strings.Join(semveronFieldOrder, ", ") + ")")
			}
		}
		if !haveMajor || !haveMinor || !havePatch {
			return nil, semveronErr("Semveron requires major, minor and patch fields")
		}
		// Re-render to the canonical string and re-validate through the
		// single string parser — one validator for both forms. Gate the
		// marker on field PRESENCE, not on a non-empty value: an explicit
		// prerelease/build of '' (or an empty/all-empty identifier list)
		// must reach the parser as a trailing '-'/'+' so validSemver*
		// rejects it, exactly as the string form '1.2.3-' is rejected —
		// never silently drop the field and construct the plain release.
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d.%d.%d", major, minor, patch)
		if havePrerelease {
			sb.WriteString("-")
			sb.WriteString(prerelease)
		}
		if haveBuild {
			sb.WriteString("+")
			sb.WriteString(build)
		}
		return semveronFromString(sb.String())
	}
	return nil, semveronErr(fmt.Sprintf("Semveron source must be a string or map, got %s", src.String()))
}

// semveronFromString parses a Semantic Versioning 2.0.0 string
// (semver.org grammar, no leading 'v') into a Semveron payload. The
// stored primary fields are major/minor/patch (Integer) and the
// optional prerelease/build (String); prereleaseParts, buildParts,
// release, stable and version are derived at read time.
func semveronFromString(s string) ([]Value, error) {
	if s == "" {
		return nil, semveronErr("Semveron requires a version, e.g. 1.2.3 or 1.0.0-rc.1+build.5")
	}
	if s[0] == 'v' || s[0] == 'V' {
		return nil, semveronErr(fmt.Sprintf("Semveron %q must not carry a leading 'v' (use %q)", s, s[1:]))
	}
	rest := s
	// Split off build at the first '+'. Build identifiers are
	// [0-9A-Za-z-] with no '+', so a second '+' fails identifier
	// validation below. A '+' with nothing after it (hasBuild but empty)
	// is a loud error, not an absent build.
	build, hasBuild := "", false
	if i := strings.IndexByte(rest, '+'); i >= 0 {
		build, hasBuild = rest[i+1:], true
		rest = rest[:i]
	}
	// Split off prerelease at the first '-'. The version core is digits
	// and dots only, so the first '-' necessarily begins the prerelease;
	// a trailing '-' with nothing after it is an error, not an absence.
	prerelease, hasPre := "", false
	if i := strings.IndexByte(rest, '-'); i >= 0 {
		prerelease, hasPre = rest[i+1:], true
		rest = rest[:i]
	}
	// Version core: exactly major.minor.patch, each a numeric identifier.
	core := strings.Split(rest, ".")
	if len(core) != 3 {
		return nil, semveronErr(fmt.Sprintf("Semveron %q core must be major.minor.patch", s))
	}
	nums := make([]int64, 3)
	for i, part := range core {
		n, err := parseSemverNumeric(part)
		if err != nil {
			return nil, semveronErr(fmt.Sprintf("Semveron %q: %v", s, err))
		}
		nums[i] = n
	}
	if hasPre {
		if err := validSemverPrerelease(prerelease); err != nil {
			return nil, semveronErr(fmt.Sprintf("Semveron %q prerelease: %v", s, err))
		}
	}
	if hasBuild {
		if err := validSemverBuild(build); err != nil {
			return nil, semveronErr(fmt.Sprintf("Semveron %q build: %v", s, err))
		}
	}
	fields := NewOrderedMap()
	fields.Set("major", NewInteger(nums[0]))
	fields.Set("minor", NewInteger(nums[1]))
	fields.Set("patch", NewInteger(nums[2]))
	if hasPre {
		fields.Set("prerelease", NewString(prerelease))
	}
	if hasBuild {
		fields.Set("build", NewString(build))
	}
	return []Value{NewValueRaw(TSemveron, MicronPayload{Fields: fields})}, nil
}

// parseSemverNumeric parses a SemVer numeric identifier — digits only,
// no leading zeros (except "0" itself) — fitting an int64.
func parseSemverNumeric(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty numeric identifier")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%q is not a number", s)
		}
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("%q has a leading zero", s)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is out of range", s)
	}
	return n, nil
}

// isSemverIdentChar reports whether c is a legal SemVer identifier
// character ([0-9A-Za-z-]).
func isSemverIdentChar(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '-'
}

// isSemverNumericIdent reports whether every character of s is a digit.
func isSemverNumericIdent(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

// validSemverPrerelease validates a dot-separated prerelease: each
// identifier is non-empty, drawn from [0-9A-Za-z-], and a purely
// numeric identifier carries no leading zero.
func validSemverPrerelease(s string) error {
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return fmt.Errorf("empty identifier")
		}
		for i := 0; i < len(id); i++ {
			if !isSemverIdentChar(id[i]) {
				return fmt.Errorf("identifier %q has an invalid character", id)
			}
		}
		if isSemverNumericIdent(id) && len(id) > 1 && id[0] == '0' {
			return fmt.Errorf("numeric identifier %q has a leading zero", id)
		}
	}
	return nil
}

// validSemverBuild validates a dot-separated build: each identifier is
// non-empty and drawn from [0-9A-Za-z-]. Leading zeros are allowed
// (build metadata is opaque and never ordered).
func validSemverBuild(s string) error {
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return fmt.Errorf("empty identifier")
		}
		for i := 0; i < len(id); i++ {
			if !isSemverIdentChar(id[i]) {
				return fmt.Errorf("identifier %q has an invalid character", id)
			}
		}
	}
	return nil
}

// micronSemveronRender renders a Semveron payload as its canonical
// string: major.minor.patch[-prerelease][+build].
func micronSemveronRender(fields *OrderedMap) string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatInt(micronFieldInt(fields, "major"), 10))
	sb.WriteString(".")
	sb.WriteString(strconv.FormatInt(micronFieldInt(fields, "minor"), 10))
	sb.WriteString(".")
	sb.WriteString(strconv.FormatInt(micronFieldInt(fields, "patch"), 10))
	if pre := micronFieldString(fields, "prerelease"); pre != "" {
		sb.WriteString("-")
		sb.WriteString(pre)
	}
	if build := micronFieldString(fields, "build"); build != "" {
		sb.WriteString("+")
		sb.WriteString(build)
	}
	return sb.String()
}

// micronFieldInt reads an Integer field, defaulting to 0 when absent.
func micronFieldInt(fields *OrderedMap, key string) int64 {
	v, ok := fields.Get(key)
	if !ok {
		return 0
	}
	n, _ := AsInteger(v)
	return n
}

// compareSemverons orders two Semveron values by SemVer 2.0.0
// precedence: major, minor, patch numerically; a version WITH a
// prerelease ranks below the same core without one; prerelease
// identifiers compare field-by-field (numeric < alphanumeric, numeric
// by value, alphanumeric by ASCII, a longer identifier set ranking
// above a shorter prefix). Build metadata does not affect precedence
// per the spec, but a final ASCII tiebreak on build keeps the order a
// strict total order consistent with content equality (two Semverons
// are equal iff their canonical renders match).
func compareSemverons(a, b Value) int {
	af, aerr := AsMicronFields(a)
	bf, berr := AsMicronFields(b)
	if aerr != nil || berr != nil {
		return strings.Compare(micronCompareKey(a), micronCompareKey(b))
	}
	for _, k := range []string{"major", "minor", "patch"} {
		if c := cmpInt64(micronFieldInt(af, k), micronFieldInt(bf, k)); c != 0 {
			return c
		}
	}
	apre := micronFieldString(af, "prerelease")
	bpre := micronFieldString(bf, "prerelease")
	if c := comparePrerelease(apre, bpre); c != 0 {
		return c
	}
	// Precedence-equal; break the remaining tie on build metadata so the
	// order is total and agrees with content equality (absent < present).
	return compareSemverBuild(micronFieldString(af, "build"), micronFieldString(bf, "build"))
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// comparePrerelease implements SemVer §11 prerelease precedence. An
// empty prerelease (a release) ranks ABOVE a non-empty one.
func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1 // release > prerelease
	}
	if b == "" {
		return -1
	}
	ai := strings.Split(a, ".")
	bi := strings.Split(b, ".")
	for i := 0; i < len(ai) && i < len(bi); i++ {
		if c := comparePrereleaseIdent(ai[i], bi[i]); c != 0 {
			return c
		}
	}
	// All shared identifiers equal — the larger set has higher precedence.
	return cmpInt64(int64(len(ai)), int64(len(bi)))
}

// comparePrereleaseIdent orders one pair of prerelease identifiers:
// both numeric compare by value; both non-numeric by ASCII; a numeric
// identifier always ranks below an alphanumeric one.
func comparePrereleaseIdent(a, b string) int {
	aNum := isSemverNumericIdent(a)
	bNum := isSemverNumericIdent(b)
	switch {
	case aNum && bNum:
		// Both fit is not guaranteed (numeric prerelease is unbounded),
		// so compare by length then lexically rather than by int64.
		if len(a) != len(b) {
			return cmpInt64(int64(len(a)), int64(len(b)))
		}
		return strings.Compare(a, b)
	case aNum && !bNum:
		return -1 // numeric < alphanumeric
	case !aNum && bNum:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// compareSemverBuild is the final total-order tiebreak on build
// metadata: absent sorts below present, otherwise ASCII lexical. Build
// does NOT participate in SemVer precedence — this only disambiguates
// two otherwise precedence-equal versions so ordering agrees with
// content equality.
func compareSemverBuild(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return -1
	case b == "":
		return 1
	}
	return strings.Compare(a, b)
}

// semveronIdentField reads a Semveron prerelease/build map field, which
// may be given as a String ("rc.1") or a List of String identifiers
// (["rc" "1"]); the List is joined with '.' so both forms re-validate
// through the single string parser identifier-by-identifier.
func semveronIdentField(v Value, field string) (string, error) {
	if ParentOf(&v).ConformsTo(TString) && IsConcrete(&v) {
		s, _ := AsString(v)
		return s, nil
	}
	if ParentOf(&v).ConformsTo(TList) && IsConcrete(&v) {
		lst, lerr := RequireConcreteList(v, "make Semveron")
		if lerr != nil {
			return "", semveronErr(fmt.Sprintf("Semveron field %s: %v", field, lerr))
		}
		parts := make([]string, lst.Len())
		for i := 0; i < lst.Len(); i++ {
			el := lst.Get(i)
			s, serr := el.AsConcreteString()
			if serr != nil {
				return "", semveronErr(fmt.Sprintf("Semveron field %s list element %d must be a String, got %s", field, i, el.String()))
			}
			parts[i] = s
		}
		return strings.Join(parts, "."), nil
	}
	return "", semveronErr(fmt.Sprintf("Semveron field %s must be a String or a List of String, got %s", field, v.String()))
}

func semveronErr(detail string) error {
	return &BoruError{Code: "type_error", Detail: "make: " + detail}
}

// ---- Cidron (CIDR block) ----

// makeCidron builds a Cidron — an IPv4/IPv6 CIDR block — from a string
// ("10.0.0.0/8", "2001:db8::/32") or an {addr prefix version?} map. Host
// bits are masked away (10.0.0.5/8 → 10.0.0.0/8). Backed by net/netip,
// which (unlike net.ParseCIDR) preserves an IPv4-mapped IPv6 prefix.
func makeCidron(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return cidronFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Cidron")
		if err != nil {
			return nil, err
		}
		var addr string
		var prefix int64
		var haveAddr, havePrefix bool
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			switch k {
			case "addr":
				sv, serr := v.AsConcreteString()
				if serr != nil {
					return nil, cidronErr(fmt.Sprintf("Cidron field addr must be a String, got %s", v.String()))
				}
				addr, haveAddr = sv, true
			case "prefix":
				pv, perr := v.AsConcreteInteger()
				if perr != nil {
					return nil, cidronErr(fmt.Sprintf("Cidron field prefix must be an Integer, got %s", v.String()))
				}
				prefix, havePrefix = pv, true
			case "version":
				// version is derived from addr; accept and ignore.
				if _, verr := v.AsConcreteInteger(); verr != nil {
					return nil, cidronErr(fmt.Sprintf("Cidron field version must be an Integer, got %s", v.String()))
				}
			default:
				return nil, cidronErr("Cidron has no field " + k + " (fields: addr, prefix, version)")
			}
		}
		if !haveAddr || !havePrefix {
			return nil, cidronErr("Cidron requires addr and prefix fields")
		}
		return cidronFromString(fmt.Sprintf("%s/%d", addr, prefix))
	}
	return nil, cidronErr(fmt.Sprintf("Cidron source must be a string or map, got %s", src.String()))
}

func cidronFromString(s string) ([]Value, error) {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return nil, cidronErr(fmt.Sprintf("invalid CIDR block %q: %v", s, err))
	}
	p = p.Masked()
	version := int64(6)
	if p.Addr().Is4() {
		version = 4
	}
	fields := NewOrderedMap()
	fields.Set("cidr", NewString(p.String()))
	fields.Set("addr", NewString(p.Addr().String()))
	fields.Set("prefix", NewInteger(int64(p.Bits())))
	fields.Set("version", NewInteger(version))
	return []Value{NewValueRaw(TCidron, MicronPayload{Fields: fields})}, nil
}

// cidronHostbits is the number of host bits in the block.
func cidronHostbits(fields *OrderedMap) int64 {
	total := int64(128)
	if micronFieldInt(fields, "version") == 4 {
		total = 32
	}
	return total - micronFieldInt(fields, "prefix")
}

// cidronPrefixOf re-parses the stored canonical cidr for numeric compare
// (infallible in practice — the string was validated at construction).
func cidronPrefixOf(v Value) (netip.Prefix, bool) {
	fields, err := AsMicronFields(v)
	if err != nil {
		return netip.Prefix{}, false
	}
	p, perr := netip.ParsePrefix(micronFieldString(fields, "cidr"))
	if perr != nil {
		return netip.Prefix{}, false
	}
	return p, true
}

// compareCidrons orders CIDR blocks numerically: family (v4 before v6),
// then network address ascending, then prefix length ascending (the
// larger, less-specific block first). netip.Addr.Compare already orders
// the v4 family below the v6 family.
func compareCidrons(a, b Value) int {
	pa, oka := cidronPrefixOf(a)
	pb, okb := cidronPrefixOf(b)
	if !oka || !okb {
		return strings.Compare(micronCompareKey(a), micronCompareKey(b))
	}
	if c := pa.Addr().Compare(pb.Addr()); c != 0 {
		return c
	}
	return cmpInt64(int64(pa.Bits()), int64(pb.Bits()))
}

func cidronErr(detail string) error {
	return &BoruError{Code: "type_error", Detail: "make: " + detail}
}

// ---- Macon (IEEE 802 hardware address) ----

// makeMacon builds a Macon — a MAC-48/EUI-48 or EUI-64 address — from a
// string (any form net.ParseMAC accepts) or an {addr} map. The 20-octet
// InfiniBand form net.ParseMAC also accepts is rejected: a Macon is 48-
// or 64-bit only. Canonical form is lowercase colon-separated hex.
func makeMacon(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return maconFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Macon")
		if err != nil {
			return nil, err
		}
		var addr string
		var have bool
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			if k != "addr" {
				return nil, maconErr("Macon has no field " + k + " (field: addr)")
			}
			sv, serr := v.AsConcreteString()
			if serr != nil {
				return nil, maconErr(fmt.Sprintf("Macon field addr must be a String, got %s", v.String()))
			}
			addr, have = sv, true
		}
		if !have {
			return nil, maconErr("Macon requires an addr field")
		}
		return maconFromString(addr)
	}
	return nil, maconErr(fmt.Sprintf("Macon source must be a string or map, got %s", src.String()))
}

func maconFromString(s string) ([]Value, error) {
	hw, err := net.ParseMAC(maconNormalizeBareHex(s))
	if err != nil {
		return nil, maconErr(fmt.Sprintf("invalid MAC address %q: %v", s, err))
	}
	if len(hw) != 6 && len(hw) != 8 {
		return nil, maconErr(fmt.Sprintf("Macon must be a 48-bit or 64-bit address, got %d octets", len(hw)))
	}
	fields := NewOrderedMap()
	fields.Set("addr", NewString(hw.String()))
	return []Value{NewValueRaw(TMacon, MicronPayload{Fields: fields})}, nil
}

// maconNormalizeBareHex rewrites a separator-less hex MAC ("001a2b3c4d5e")
// into the colon-separated form net.ParseMAC accepts. Only a bare run of
// exactly 12 or 16 hex digits (a 48- or 64-bit address) is rewritten; any
// string carrying a separator, or of another length, is returned verbatim
// so net.ParseMAC renders the authoritative error.
func maconNormalizeBareHex(s string) string {
	if len(s) != 12 && len(s) != 16 {
		return s
	}
	for i := 0; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return s
		}
	}
	var sb strings.Builder
	sb.Grow(len(s) + len(s)/2 - 1)
	for i := 0; i < len(s); i += 2 {
		if i > 0 {
			sb.WriteByte(':')
		}
		sb.WriteString(s[i : i+2])
	}
	return sb.String()
}

// maconBytes re-parses the stored canonical addr into its octets for the
// derived properties (never stored, so equality stays a pure-addr
// comparison).
func maconBytes(fields *OrderedMap) (net.HardwareAddr, bool) {
	hw, err := net.ParseMAC(micronFieldString(fields, "addr"))
	if err != nil {
		return nil, false
	}
	return hw, true
}

func maconAllOnes(hw net.HardwareAddr) bool {
	for _, b := range hw {
		if b != 0xff {
			return false
		}
	}
	return len(hw) > 0
}

func maconErr(detail string) error {
	return &BoruError{Code: "type_error", Detail: "make: " + detail}
}

// ---- Coloron (sRGB colour with alpha) ----

// makeColoron builds a Coloron from a #hex spelling (#rgb / #rgba /
// #rrggbb / #rrggbbaa), an rgb()/rgba() function (legacy comma or modern
// slash-alpha), or an {r g b a?} channel map. Canonical form is
// lowercase hex, 6 digits when opaque else 8.
func makeColoron(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return coloronFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Coloron")
		if err != nil {
			return nil, err
		}
		r, g, b, a := int64(0), int64(0), int64(0), int64(255)
		var haveR, haveG, haveB bool
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			n, nerr := v.AsConcreteInteger()
			if nerr != nil {
				return nil, coloronErr(fmt.Sprintf("Coloron field %s must be an Integer, got %s", k, v.String()))
			}
			switch k {
			case "r":
				r, haveR = n, true
			case "g":
				g, haveG = n, true
			case "b":
				b, haveB = n, true
			case "a":
				a = n
			default:
				return nil, coloronErr("Coloron has no field " + k + " (fields: r, g, b, a)")
			}
		}
		if !haveR || !haveG || !haveB {
			return nil, coloronErr("Coloron requires r, g and b fields")
		}
		for _, ch := range []struct {
			name string
			val  int64
		}{{"r", r}, {"g", g}, {"b", b}, {"a", a}} {
			if ch.val < 0 || ch.val > 255 {
				return nil, coloronErr(fmt.Sprintf("Coloron field %s must be in 0..255, got %d", ch.name, ch.val))
			}
		}
		return coloronValue(int(r), int(g), int(b), int(a)), nil
	}
	return nil, coloronErr(fmt.Sprintf("Coloron source must be a string or map, got %s", src.String()))
}

func coloronValue(r, g, b, a int) []Value {
	fields := NewOrderedMap()
	fields.Set("r", NewInteger(int64(r)))
	fields.Set("g", NewInteger(int64(g)))
	fields.Set("b", NewInteger(int64(b)))
	fields.Set("a", NewInteger(int64(a)))
	return []Value{NewValueRaw(TColoron, MicronPayload{Fields: fields})}
}

func coloronFromString(s string) ([]Value, error) {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "#") {
		return coloronFromHex(t)
	}
	lower := strings.ToLower(t)
	if strings.HasPrefix(lower, "rgb(") || strings.HasPrefix(lower, "rgba(") {
		return coloronFromFunc(t)
	}
	return nil, coloronErr(fmt.Sprintf("invalid colour %q (want #hex or rgb()/rgba())", s))
}

func coloronFromHex(t string) ([]Value, error) {
	h := t[1:]
	for i := 0; i < len(h); i++ {
		if !isHexDigit(h[i]) {
			return nil, coloronErr(fmt.Sprintf("invalid hex colour %q", t))
		}
	}
	r, g, b, a := 0, 0, 0, 255
	switch len(h) {
	case 3:
		r, g, b = hexNibble(h[0]), hexNibble(h[1]), hexNibble(h[2])
	case 4:
		r, g, b, a = hexNibble(h[0]), hexNibble(h[1]), hexNibble(h[2]), hexNibble(h[3])
	case 6:
		r, g, b = hexByte(h[0:2]), hexByte(h[2:4]), hexByte(h[4:6])
	case 8:
		r, g, b, a = hexByte(h[0:2]), hexByte(h[2:4]), hexByte(h[4:6]), hexByte(h[6:8])
	default:
		return nil, coloronErr(fmt.Sprintf("hex colour %q must have 3, 4, 6, or 8 digits", t))
	}
	return coloronValue(r, g, b, a), nil
}

func coloronFromFunc(t string) ([]Value, error) {
	open := strings.IndexByte(t, '(')
	if open < 0 || !strings.HasSuffix(t, ")") {
		return nil, coloronErr(fmt.Sprintf("invalid colour function %q", t))
	}
	inner := t[open+1 : len(t)-1]
	hasComma := strings.ContainsRune(inner, ',')
	hasSlash := strings.ContainsRune(inner, '/')
	if hasComma && hasSlash {
		return nil, coloronErr(fmt.Sprintf("colour %q mixes ',' and '/' separators", t))
	}
	var chanToks []string
	alphaTok := ""
	haveAlpha := false
	if hasComma {
		parts := splitTrim(inner, ",")
		if len(parts) == 4 {
			alphaTok, haveAlpha = parts[3], true
			parts = parts[:3]
		}
		chanToks = parts
	} else {
		body := inner
		if hasSlash {
			i := strings.IndexByte(inner, '/')
			body = inner[:i]
			alphaTok, haveAlpha = strings.TrimSpace(inner[i+1:]), true
		}
		chanToks = strings.Fields(body)
	}
	if len(chanToks) != 3 {
		return nil, coloronErr(fmt.Sprintf("colour %q needs exactly 3 channels", t))
	}
	r, err := coloronChannel(chanToks[0])
	if err != nil {
		return nil, err
	}
	g, err := coloronChannel(chanToks[1])
	if err != nil {
		return nil, err
	}
	b, err := coloronChannel(chanToks[2])
	if err != nil {
		return nil, err
	}
	a := 255
	if haveAlpha {
		a, err = coloronAlpha(alphaTok)
		if err != nil {
			return nil, err
		}
	}
	return coloronValue(r, g, b, a), nil
}

// coloronChannel parses one r/g/b channel: a number (0..255) or a
// percentage (0..100%), or `none` (→0), rounded to the nearest byte.
func coloronChannel(tok string) (int, error) {
	tok = strings.TrimSpace(tok)
	if strings.EqualFold(tok, "none") {
		return 0, nil
	}
	if pct, ok := strings.CutSuffix(tok, "%"); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(pct), 64)
		if err != nil {
			return 0, coloronErr(fmt.Sprintf("invalid colour channel %q", tok))
		}
		return int(math.Round(clampFloat(f, 0, 100) / 100 * 255)), nil
	}
	f, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return 0, coloronErr(fmt.Sprintf("invalid colour channel %q", tok))
	}
	return int(math.Round(clampFloat(f, 0, 255))), nil
}

// coloronAlpha parses the optional alpha: a number in 0..1, a percentage
// in 0..100%, or `none` (→0).
func coloronAlpha(tok string) (int, error) {
	tok = strings.TrimSpace(tok)
	if strings.EqualFold(tok, "none") {
		return 0, nil
	}
	if pct, ok := strings.CutSuffix(tok, "%"); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(pct), 64)
		if err != nil {
			return 0, coloronErr(fmt.Sprintf("invalid alpha %q", tok))
		}
		return int(math.Round(clampFloat(f, 0, 100) / 100 * 255)), nil
	}
	f, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return 0, coloronErr(fmt.Sprintf("invalid alpha %q", tok))
	}
	return int(math.Round(clampFloat(f, 0, 1) * 255)), nil
}

// micronColoronHex is the canonical Coloron render.
func micronColoronHex(fields *OrderedMap) string {
	r, g, b, a := micronFieldInt(fields, "r"), micronFieldInt(fields, "g"), micronFieldInt(fields, "b"), micronFieldInt(fields, "a")
	if a == 255 {
		return fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", r, g, b, a)
}

// coloronCSS is the derived modern rgb() render.
func coloronCSS(fields *OrderedMap) string {
	r, g, b, a := micronFieldInt(fields, "r"), micronFieldInt(fields, "g"), micronFieldInt(fields, "b"), micronFieldInt(fields, "a")
	if a == 255 {
		return fmt.Sprintf("rgb(%d %d %d)", r, g, b)
	}
	return fmt.Sprintf("rgb(%d %d %d / %s)", r, g, b, strconv.FormatFloat(float64(a)/255, 'f', -1, 64))
}

// compareColorons orders colours by the (r,g,b,a) channel tuple.
func compareColorons(a, b Value) int {
	af, aerr := AsMicronFields(a)
	bf, berr := AsMicronFields(b)
	if aerr != nil || berr != nil {
		return strings.Compare(micronCompareKey(a), micronCompareKey(b))
	}
	for _, k := range []string{"r", "g", "b", "a"} {
		if c := cmpInt64(micronFieldInt(af, k), micronFieldInt(bf, k)); c != 0 {
			return c
		}
	}
	return 0
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

func hexNibble(c byte) int { n := hexVal(c); return n*16 + n }
func hexByte(s string) int { return hexVal(s[0])*16 + hexVal(s[1]) }

func clampFloat(f, lo, hi float64) float64 {
	if f < lo {
		return lo
	}
	if f > hi {
		return hi
	}
	return f
}

func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func coloronErr(detail string) error {
	return &BoruError{Code: "type_error", Detail: "make: " + detail}
}

// ---- Mimon (MIME / media type) ----

// makeMimon builds a Mimon — a canonical media type (type/subtype with
// optional parameters) — from a string or a {type subtype params?} map.
// Backed by the stdlib mime package plus an RFC 6838 restricted-name
// strict layer (rejects a bare type, wildcards / ranges, and empty
// parameter values).
func makeMimon(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return mimonFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Mimon")
		if err != nil {
			return nil, err
		}
		var typ, sub string
		var haveType, haveSub bool
		params := map[string]string{}
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			switch k {
			case "type":
				sv, serr := v.AsConcreteString()
				if serr != nil {
					return nil, mimonErr(fmt.Sprintf("Mimon field type must be a String, got %s", v.String()))
				}
				typ, haveType = sv, true
			case "subtype":
				sv, serr := v.AsConcreteString()
				if serr != nil {
					return nil, mimonErr(fmt.Sprintf("Mimon field subtype must be a String, got %s", v.String()))
				}
				sub, haveSub = sv, true
			case "params":
				pm, perr := RequireConcreteMap(v, "make Mimon params")
				if perr != nil {
					return nil, mimonErr(fmt.Sprintf("Mimon field params must be a Map, got %s", v.String()))
				}
				for _, pk := range pm.Keys() {
					pv, _ := pm.Get(pk)
					psv, pserr := pv.AsConcreteString()
					if pserr != nil {
						return nil, mimonErr(fmt.Sprintf("Mimon param %s must be a String, got %s", pk, pv.String()))
					}
					params[pk] = psv
				}
			default:
				return nil, mimonErr("Mimon has no field " + k + " (fields: type, subtype, params)")
			}
		}
		if !haveType || !haveSub {
			return nil, mimonErr("Mimon requires type and subtype fields")
		}
		rendered := mime.FormatMediaType(typ+"/"+sub, params)
		if rendered == "" {
			return nil, mimonErr(fmt.Sprintf("Mimon invalid media type %s/%s", typ, sub))
		}
		return mimonFromString(rendered)
	}
	return nil, mimonErr(fmt.Sprintf("Mimon source must be a string or map, got %s", src.String()))
}

func mimonFromString(s string) ([]Value, error) {
	mt, params, err := mime.ParseMediaType(s)
	if err != nil {
		return nil, mimonErr(fmt.Sprintf("invalid media type %q: %v", s, err))
	}
	i := strings.IndexByte(mt, '/')
	if i <= 0 || i >= len(mt)-1 {
		return nil, mimonErr(fmt.Sprintf("media type %q must be type/subtype", s))
	}
	typ, sub := mt[:i], mt[i+1:]
	if !validRestrictedName(typ) {
		return nil, mimonErr(fmt.Sprintf("media type %q has an invalid type %q", s, typ))
	}
	if !validRestrictedName(sub) {
		return nil, mimonErr(fmt.Sprintf("media type %q has an invalid subtype %q", s, sub))
	}
	for name, val := range params {
		if val == "" {
			return nil, mimonErr(fmt.Sprintf("media type %q parameter %q has an empty value", s, name))
		}
	}
	fields := NewOrderedMap()
	fields.Set("type", NewString(typ))
	fields.Set("subtype", NewString(sub))
	if len(params) > 0 {
		names := make([]string, 0, len(params))
		for n := range params {
			names = append(names, n)
		}
		sort.Strings(names)
		pm := NewOrderedMap()
		for _, n := range names {
			pm.Set(n, NewString(params[n]))
		}
		fields.Set("params", NewMap(pm))
	}
	return []Value{NewValueRaw(TMimon, MicronPayload{Fields: fields})}, nil
}

// validRestrictedName enforces the RFC 6838 restricted-name grammar on a
// media type or subtype: first char ALPHA/DIGIT, rest from the restricted
// set, length ≤ 127. This rejects wildcard ranges ('*').
func validRestrictedName(name string) bool {
	if name == "" || len(name) > 127 || !isAlphaNum(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if isAlphaNum(c) || strings.IndexByte("!#$&-^_.+", c) >= 0 {
			continue
		}
		return false
	}
	return true
}

func isAlphaNum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// mimonParams rebuilds the plain string map from the stored params Map.
func mimonParams(fields *OrderedMap) map[string]string {
	pv, ok := fields.Get("params")
	if !ok {
		return nil
	}
	pm, err := AsMap(pv)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, k := range pm.Keys() {
		v, _ := pm.Get(k)
		s, _ := AsString(v)
		out[k] = s
	}
	return out
}

// micronMimonRender is the canonical Mimon render: essence plus canonical
// (name-sorted, minimally-quoted) parameters via mime.FormatMediaType.
func micronMimonRender(fields *OrderedMap) string {
	essence := micronFieldString(fields, "type") + "/" + micronFieldString(fields, "subtype")
	params := mimonParams(fields)
	if len(params) == 0 {
		return essence
	}
	if rendered := mime.FormatMediaType(essence, params); rendered != "" {
		return rendered
	}
	return essence
}

func mimonErr(detail string) error {
	return &BoruError{Code: "type_error", Detail: "make: " + detail}
}

// ---- Qion (monetary amount) ----

// makeQion builds a Qion — an exact decimal amount in a currency — from a
// "CODE AMOUNT" string ("USD 12.50") or a {code amount} map. The amount
// is scaled to the currency's ISO 4217 minor-unit precision (an input
// with MORE fractional digits is rejected, not rounded).
func makeQion(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return qionFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Qion")
		if err != nil {
			return nil, err
		}
		var code, amount string
		var haveCode, haveAmount bool
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			switch k {
			case "code":
				sv, serr := v.AsConcreteString()
				if serr != nil {
					return nil, qionErr(fmt.Sprintf("Qion field code must be a String, got %s", v.String()))
				}
				code, haveCode = sv, true
			case "amount":
				av, aerr := qionAmountText(v)
				if aerr != nil {
					return nil, aerr
				}
				amount, haveAmount = av, true
			default:
				return nil, qionErr("Qion has no field " + k + " (fields: code, amount)")
			}
		}
		if !haveCode || !haveAmount {
			return nil, qionErr("Qion requires code and amount fields")
		}
		return qionFromString(code + " " + amount)
	}
	return nil, qionErr(fmt.Sprintf("Qion source must be a string or map, got %s", src.String()))
}

// qionAmountText renders a map-supplied amount (Decimal, Integer, or a
// String) to plain-decimal text for the single string validator.
func qionAmountText(v Value) (string, error) {
	switch {
	case ParentOf(&v).ConformsTo(TBigDecimal):
		d, err := AsBigDecimal(v)
		if err != nil {
			return "", qionErr("Qion amount is not a valid decimal")
		}
		return d.Text('f'), nil
	case ParentOf(&v).ConformsTo(TInteger) && IsConcrete(&v):
		n, _ := AsInteger(v)
		return strconv.FormatInt(n, 10), nil
	case ParentOf(&v).ConformsTo(TString) && IsConcrete(&v):
		s, _ := AsString(v)
		return s, nil
	}
	return "", qionErr(fmt.Sprintf("Qion amount must be a Decimal, Integer or String, got %s", v.String()))
}

var qionAmountRe = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

func qionFromString(s string) ([]Value, error) {
	toks := strings.Fields(strings.TrimSpace(s))
	if len(toks) != 2 {
		return nil, qionErr(fmt.Sprintf("Qion %q must be 'CODE AMOUNT'", s))
	}
	code := strings.ToUpper(toks[0])
	if !isThreeLetters(code) {
		return nil, qionErr(fmt.Sprintf("Qion currency code %q must be 3 letters", toks[0]))
	}
	minor, ok := iso4217[code]
	if !ok {
		return nil, qionErr(fmt.Sprintf("Qion %q is not a known ISO 4217 currency code", code))
	}
	amountStr := toks[1]
	if !qionAmountRe.MatchString(amountStr) {
		return nil, qionErr(fmt.Sprintf("Qion amount %q must be a plain decimal (optional '-', no grouping or exponent)", amountStr))
	}
	neg := strings.HasPrefix(amountStr, "-")
	body := strings.TrimPrefix(amountStr, "-")
	intPart, fracPart := body, ""
	if dot := strings.IndexByte(body, '.'); dot >= 0 {
		intPart, fracPart = body[:dot], body[dot+1:]
	}
	if len(fracPart) > minor {
		return nil, qionErr(fmt.Sprintf("Qion amount %q has more than %d fractional digits for %s", amountStr, minor, code))
	}
	// Zero-pad the fractional part up to the minor-unit scale, building the
	// canonical amount text directly (exact — no rounding needed).
	fracPart += strings.Repeat("0", minor-len(fracPart))
	canon := intPart
	if minor > 0 {
		canon = intPart + "." + fracPart
	}
	if neg {
		canon = "-" + canon
	}
	d, _, derr := apd.NewFromString(canon)
	if derr != nil || d.Form != apd.Finite { //covergate:allow Qion: apd.NewFromString over a regex-validated plain-decimal `canon` is always finite, so the parse/Form guard cannot fire (§kernel)
		return nil, qionErr(fmt.Sprintf("Qion amount %q is not a finite decimal", amountStr))
	}
	if d.IsZero() {
		d.Negative = false // normalise negative zero
	}
	fields := NewOrderedMap()
	fields.Set("code", NewString(code))
	fields.Set("amount", NewBigDecimal(d))
	fields.Set("minor", NewInteger(int64(minor)))
	return []Value{NewValueRaw(TQion, MicronPayload{Fields: fields})}, nil
}

// micronQionRender is the canonical Qion render: "CODE AMOUNT" with the
// amount at exactly the minor-unit scale.
func micronQionRender(fields *OrderedMap) string {
	code := micronFieldString(fields, "code")
	amt, ok := fields.Get("amount")
	if !ok {
		return code
	}
	d, err := AsBigDecimal(amt)
	if err != nil {
		return code
	}
	return code + " " + d.Text('f')
}

// qionUnits returns the integer count of minor units (amount × 10^minor),
// derived from the canonical render. Absent when it overflows int64.
func qionUnits(fields *OrderedMap) (Value, bool) {
	amt, ok := fields.Get("amount")
	if !ok {
		return Value{}, false
	}
	d, err := AsBigDecimal(amt)
	if err != nil {
		return Value{}, false
	}
	s := d.Text('f')
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	s = strings.Replace(s, ".", "", 1)
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return Value{}, false
	}
	if neg {
		n.Neg(n)
	}
	if !n.IsInt64() {
		return Value{}, false
	}
	return NewInteger(n.Int64()), true
}

// compareQions orders money by currency code (lexical) then, within the
// same code, by exact decimal magnitude.
func compareQions(a, b Value) int {
	af, aerr := AsMicronFields(a)
	bf, berr := AsMicronFields(b)
	if aerr != nil || berr != nil {
		return strings.Compare(micronCompareKey(a), micronCompareKey(b))
	}
	if c := strings.Compare(micronFieldString(af, "code"), micronFieldString(bf, "code")); c != 0 {
		return c
	}
	amtA, okA := af.Get("amount")
	amtB, okB := bf.Get("amount")
	if !okA || !okB {
		return strings.Compare(micronCompareKey(a), micronCompareKey(b))
	}
	c, err := CompareValues(amtA, amtB)
	if err != nil { //covergate:allow Qion: CompareValues over two stored BigDecimal amounts never errors (cross-family pairs fall back to Rank, not error) (§kernel)
		return strings.Compare(micronCompareKey(a), micronCompareKey(b))
	}
	return c
}

func isThreeLetters(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	return true
}

func qionErr(detail string) error {
	return &BoruError{Code: "type_error", Detail: "make: " + detail}
}

// ---- Phonon (E.164 telephone number) ----

var phononRe = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)

// phononSep strips the visual separators permitted in presentation forms;
// the '+' must be present literally (an "00"/"011" access prefix is NOT
// mapped to '+').
var phononSep = strings.NewReplacer(" ", "", "\t", "", "-", "", ".", "", "(", "", ")", "", "\u00a0", "")

// makePhonon builds a Phonon — a strict ITU-T E.164 number — from a
// string (presentation separators stripped) or a {cc subscriber} map.
func makePhonon(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return phononFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Phonon")
		if err != nil {
			return nil, err
		}
		var cc, sub string
		var haveCC, haveSub bool
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			switch k {
			case "cc":
				sv, serr := v.AsConcreteString()
				if serr != nil {
					return nil, phononErr(fmt.Sprintf("Phonon field cc must be a String, got %s", v.String()))
				}
				cc, haveCC = sv, true
			case "subscriber":
				sv, serr := v.AsConcreteString()
				if serr != nil {
					return nil, phononErr(fmt.Sprintf("Phonon field subscriber must be a String, got %s", v.String()))
				}
				sub, haveSub = sv, true
			default:
				return nil, phononErr("Phonon has no field " + k + " (fields: cc, subscriber)")
			}
		}
		if !haveCC || !haveSub {
			return nil, phononErr("Phonon requires cc and subscriber fields")
		}
		return phononFromString("+" + cc + sub)
	}
	return nil, phononErr(fmt.Sprintf("Phonon source must be a string or map, got %s", src.String()))
}

func phononFromString(s string) ([]Value, error) {
	stripped := phononSep.Replace(strings.TrimSpace(s))
	if !phononRe.MatchString(stripped) {
		return nil, phononErr(fmt.Sprintf("Phonon %q is not valid E.164 (want '+' then 2..15 digits, first non-zero)", s))
	}
	fields := NewOrderedMap()
	fields.Set("digits", NewString(stripped[1:])) // stored without the leading '+'
	return []Value{NewValueRaw(TPhonon, MicronPayload{Fields: fields})}, nil
}

func phononErr(detail string) error {
	return &BoruError{Code: "type_error", Detail: "make: " + detail}
}

// urlonFieldOrder is the canonical field layout of an Urlon map source
// (and of the stored payload — optional fields are simply absent).
var urlonFieldOrder = []string{"scheme", "host", "port", "path", "query", "fragment"}

// makeUrlon builds an Urlon from an absolute-URL string or a
// {scheme host port? path? query? fragment?} map. The map form
// re-renders to an href and re-parses through the string path — one
// validator.
func makeUrlon(src Value) ([]Value, error) {
	switch {
	case ParentOf(&src).ConformsTo(TString) && IsConcrete(&src):
		s, _ := AsString(src)
		return urlonFromString(s)
	case ParentOf(&src).ConformsTo(TMap) && IsConcrete(&src):
		m, err := RequireConcreteMap(src, "make Urlon")
		if err != nil {
			return nil, err
		}
		parts := map[string]string{}
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			valid := false
			for _, f := range urlonFieldOrder {
				if k == f {
					valid = true
					break
				}
			}
			if !valid {
				return nil, &BoruError{Code: "type_error",
					Detail: "make: Urlon has no field " + k + " (fields: " + strings.Join(urlonFieldOrder, ", ") + ")"}
			}
			if k == "port" {
				n, nerr := v.AsConcreteInteger()
				if nerr != nil {
					return nil, &BoruError{Code: "type_error",
						Detail: fmt.Sprintf("make: Urlon port must be an Integer, got %s", v.String())}
				}
				parts[k] = strconv.FormatInt(n, 10)
				continue
			}
			sv, serr := v.AsConcreteString()
			if serr != nil {
				return nil, &BoruError{Code: "type_error",
					Detail: fmt.Sprintf("make: Urlon field %s must be a String, got %s", k, v.String())}
			}
			parts[k] = sv
		}
		if parts["scheme"] == "" || parts["host"] == "" {
			return nil, &BoruError{Code: "type_error",
				Detail: "make: Urlon requires both scheme and host fields"}
		}
		href := parts["scheme"] + "://" + parts["host"]
		if parts["port"] != "" {
			href += ":" + parts["port"]
		}
		href += parts["path"]
		if parts["query"] != "" {
			href += "?" + parts["query"]
		}
		if parts["fragment"] != "" {
			href += "#" + parts["fragment"]
		}
		return urlonFromString(href)
	}
	return nil, &BoruError{Code: "type_error",
		Detail: fmt.Sprintf("make: Urlon source must be a string or map, got %s", src.String())}
}

func urlonFromString(s string) ([]Value, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, &BoruError{Code: "type_error",
			Detail: fmt.Sprintf("make: invalid URL %q: %v", s, err)}
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, &BoruError{Code: "type_error",
			Detail: fmt.Sprintf("make: Urlon requires an absolute URL (scheme://host…), got %q", s)}
	}
	fields := NewOrderedMap()
	fields.Set("scheme", NewString(u.Scheme))
	fields.Set("host", NewString(u.Hostname()))
	if p := u.Port(); p != "" {
		n, perr := strconv.ParseInt(p, 10, 64)
		if perr != nil {
			return nil, &BoruError{Code: "type_error",
				Detail: fmt.Sprintf("make: invalid URL port %q", p)}
		}
		fields.Set("port", NewInteger(n))
	}
	if u.Path != "" {
		fields.Set("path", NewString(u.Path))
	}
	if u.RawQuery != "" {
		fields.Set("query", NewString(u.RawQuery))
	}
	if u.Fragment != "" {
		fields.Set("fragment", NewString(u.Fragment))
	}
	return []Value{NewValueRaw(TUrlon, MicronPayload{Fields: fields})}, nil
}

// makeMicronUser instantiates a user-defined Micron kind from a
// concrete map of fields. Unknown and missing fields error; each field
// validates against its schema constraint via MakeClassFieldValue (the
// same per-field rules class construction applies — concrete defaults,
// nominal class fields, predicate types).
func makeMicronUser(info MicronTypeInfo, data Value, r *Registry) ([]Value, error) {
	kindName := info.Name
	if kindName == "" {
		kindName = "Micron"
	}
	if !ParentOf(&data).ConformsTo(TMap) || !IsConcrete(&data) {
		return nil, &BoruError{Code: "type_error",
			Detail: fmt.Sprintf("make: %s takes a map of fields, got %s", kindName, data.String())}
	}
	provided, err := RequireConcreteMap(data, "make "+kindName)
	if err != nil {
		return nil, err
	}
	for _, key := range provided.Keys() {
		if _, ok := info.Fields.Get(key); !ok {
			return nil, &BoruError{Code: "type_error",
				Detail: fmt.Sprintf("make: %s has no field %q", kindName, key)}
		}
	}
	result := NewOrderedMap()
	for _, key := range info.Fields.Keys() {
		constraint, _ := info.Fields.Get(key)
		val, ok := provided.Get(key)
		if !ok {
			// A concrete (non-type-body) schema entry is a default.
			if DataOf(&constraint) != nil && !IsTypeBody(&constraint) {
				result.Set(key, constraint)
				continue
			}
			return nil, &BoruError{Code: "type_error",
				Detail: fmt.Sprintf("make: %s is missing field %q", kindName, key)}
		}
		checked, cerr := MakeClassFieldValue(val, constraint, r)
		if cerr != nil {
			return nil, &BoruError{Code: "type_error",
				Detail: fmt.Sprintf("make: %s field %q: %v", kindName, key, cerr)}
		}
		result.Set(key, checked)
	}
	parent := info.Type
	if parent == nil {
		parent = TMicron
	}
	return []Value{NewValueRaw(parent, MicronPayload{Fields: result})}, nil
}

// CheckMicronConstruction is the CHECK-MODE mirror of the Micron
// constructors (the analogue of CheckMakeConstruction for class /
// Resource targets): when the make target is a Micron kind and the
// source is fully concrete, the whole validation — string parsing,
// unknown/missing fields, per-field types, the abstract root — is
// decidable right here, so the real constructor runs and its error
// surfaces as a type_error diagnostic with the byte-identical runtime
// message. A carrier source, or a map source carrying any carrier
// field, is value-dependent and stays with the runtime constructor.
// Deduped by detail+position. No-op outside check mode.
func CheckMicronConstruction(r *Registry, target, src Value, pos SrcPos) {
	if r == nil || !r.Check.IsActive() {
		return
	}
	target = ResolveTypeLiteralDef(target, r)
	if !micronAccepts(target) {
		return
	}
	if !IsConcrete(&src) {
		return
	}
	if ParentOf(&src).ConformsTo(TMap) {
		m, _ := AsMap(src)
		if m == nil {
			return
		}
		for _, k := range m.Keys() {
			if fv, ok := m.Get(k); ok && !IsConcrete(&fv) {
				return
			}
		}
	}
	// The Micron constructors are pure (no registry mutation, no I/O),
	// so running the real one at analysis time is safe.
	if _, err := micronInstantiate(target, src, r); err != nil {
		code, detail := "type_error", err.Error()
		var ae *BoruError
		if errors.As(err, &ae) {
			code, detail = ae.Code, ae.Detail
		}
		CheckAddUniqueDiagnostic(r, code, detail, "make", pos)
	}
}

// ---- the Micron Ideal (refine / make dispatch) ----

// micronAccepts is the Ideal dispatch predicate: a bare lattice node
// under Scalar/Micron, or a MicronTypeInfo type body. Deliberately
// narrow — kernel Ideals win Accepts ties, so this must not overclaim.
func micronAccepts(v Value) bool {
	if IsMicronType(v) {
		return true
	}
	return IsBareTypeNode(&v) && (&v).ConformsTo(TMicron)
}

// micronConstruct is `refine ‹base› {fields}` for the family.
// Schema-carrying refinement is allowed of the ROOT only — a leaf's
// field set is its validation contract (D5). Bare nominal refinement
// of any Micron kind rides the ordinary refine-prefab path instead.
func micronConstruct(base, arg Value, r *Registry) ([]Value, error) {
	if !(IsBareTypeNode(&base) && base.Equal(TMicron)) {
		name := ParentOf(&base).Name()
		if IsBareTypeNode(&base) {
			name = base.Name()
		} else if info, err := AsMicronType(base); err == nil && info.Name != "" {
			name = info.Name
		}
		return nil, &BoruError{Code: "type_error",
			Detail: "refine: " + name + " is a Micron kind — its field set is its validation contract",
			Hint:   "newtype it (def Newon refine " + name + ") or define a fresh kind: def Nameon refine Micron {field:Type}"}
	}
	fieldsIn, err := RequireConcreteMap(arg, "refine Micron")
	if err != nil {
		return nil, &BoruError{Code: "type_error",
			Detail: "refine Micron: takes a map of fields, e.g. refine Micron {foo:String}"}
	}
	fields := NewOrderedMap()
	for _, k := range fieldsIn.Keys() {
		v, _ := fieldsIn.Get(k)
		v = ResolveWordValue(v)
		if !IsTypeBody(&v) && !IsConcrete(&v) {
			return nil, &BoruError{Code: "type_error",
				Detail: fmt.Sprintf("refine Micron: field %s must be a type or a concrete default, got %s", k, v.String())}
		}
		fields.Set(k, v)
	}
	return []Value{NewValueRaw(TMicron, MicronTypeInfo{Fields: fields})}, nil
}

// micronInstantiate is `make ‹kind› data` for the family.
//
// The instance carries the CONSTRUCTOR ARGUMENT's source position. `make
// Pathon "/etc/hosts"` builds a fresh Value, and a fresh Value has no
// position of its own — so without this every later failure that blames
// the micron could only report "source position unknown", however
// carefully the reporting site was threaded. That is not hypothetical:
// it is why `IO.read (make Pathon "/nope")` had no location to point at.
// WithPos is the documented seam for a handler constructing a value from
// an input (lang/go/CLAUDE.md, "Pos threading"); doing it here covers
// every Micron kind at one choke point rather than per-constructor.
func micronInstantiate(typ, data Value, r *Registry) ([]Value, error) {
	out, err := micronInstantiateAt(typ, data, r)
	for i := range out {
		ThreadPos(&out[i], &data)
	}
	return out, err
}

func micronInstantiateAt(typ, data Value, r *Registry) ([]Value, error) {
	if IsMicronType(typ) {
		info, _ := AsMicronType(typ)
		return makeMicronUser(info, data, r)
	}
	kind := NodeOf(&typ)
	switch {
	case kind.Equal(TMicron):
		return nil, &BoruError{Code: "type_error",
			Detail: "make: Micron is abstract — construct a leaf (Pathon / Emailon / Urlon / Ipon / Hoston / Semveron / Cidron / Macon / Coloron / Mimon / Qion / Phonon) or a user-defined Micron kind",
			Hint:   "define one with: def Nameon refine Micron {field:Type}"}
	case kind.Equal(TPathon):
		return MakePathon(data, false)
	case kind.Equal(TEmailon):
		return makeEmailon(data)
	case kind.Equal(TUrlon):
		return makeUrlon(data)
	case kind.Equal(TIpon):
		return makeIpon(data)
	case kind.Equal(THoston):
		return makeHoston(data)
	case kind.Equal(TSemveron):
		return makeSemveron(data)
	case kind.Equal(TCidron):
		return makeCidron(data)
	case kind.Equal(TMacon):
		return makeMacon(data)
	case kind.Equal(TColoron):
		return makeColoron(data)
	case kind.Equal(TMimon):
		return makeMimon(data)
	case kind.Equal(TQion):
		return makeQion(data)
	case kind.Equal(TPhonon):
		return makePhonon(data)
	}
	// A newtype (bare nominal refine) of a builtin leaf or of a user
	// kind: construct the base, then tag the result with the newtype —
	// the same reparent the typed-def path (`def x:Foo v`) performs.
	for t := kind; t != nil && t.ConformsTo(TMicron); t = ParentOf(t) {
		var out []Value
		var err error
		switch {
		case t.Equal(TPathon):
			out, err = MakePathon(data, false)
		case t.Equal(TEmailon):
			out, err = makeEmailon(data)
		case t.Equal(TUrlon):
			out, err = makeUrlon(data)
		case t.Equal(TIpon):
			out, err = makeIpon(data)
		case t.Equal(THoston):
			out, err = makeHoston(data)
		case t.Equal(TSemveron):
			out, err = makeSemveron(data)
		case t.Equal(TCidron):
			out, err = makeCidron(data)
		case t.Equal(TMacon):
			out, err = makeMacon(data)
		case t.Equal(TColoron):
			out, err = makeColoron(data)
		case t.Equal(TMimon):
			out, err = makeMimon(data)
		case t.Equal(TQion):
			out, err = makeQion(data)
		case t.Equal(TPhonon):
			out, err = makePhonon(data)
		default:
			if mb, ok := t.Behavior().(micronBehavior); ok && mb.info != nil {
				out, err = makeMicronUser(*mb.info, data, r)
			} else {
				continue
			}
		}
		if err != nil {
			return nil, err
		}
		return []Value{MintRetagged(&out[0], kind)}, nil
	}
	return nil, &BoruError{Code: "type_error",
		Detail: "make: " + kind.Name() + " has no field schema — define one with refine Micron {fields}"}
}
