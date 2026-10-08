package native

import (
	"crypto/tls"

	"github.com/boru-lang/boru/lang/go/capabilities"
	"github.com/boru-lang/boru/lang/go/policy"
)

// ParseTLSOpts reads a guest `tls: {…}` option map into a resolved
// TLSProfile, per design/legacy/NETWORK-TLS-PLAN.0.ignore §4.3. One parser serves
// every call site (fetch today, connect-raw next) so the two cannot
// drift apart.
//
//	identity: Atom|String|handle  a host-registered client identity, or
//	          the opaque handle `Vault.identity` returns (mutual TLS)
//	verify: Boolean   false disables chain AND hostname checking
//	ca:     Bytes|String   additional CA roots, PEM; REPLACES the system pool
//	sni:    String    server-name override
//	min:    String    "1.2" | "1.3"
//
// Unknown keys are REJECTED rather than ignored: this is a
// security-sensitive map, and silently dropping a misspelled `verifiy:`
// would leave the caller believing they had configured something.
func ParseTLSOpts(r *Registry, v Value, word string) (capabilities.TLSProfile, error) {
	var p capabilities.TLSProfile
	mp, err := RequireConcreteMap(v, word)
	if err != nil {
		return p, err
	}
	for _, key := range mp.Keys() {
		val, _ := mp.Get(key)
		switch key {
		case "verify":
			b, bErr := val.AsConcreteBoolean()
			if bErr != nil {
				return p, r.BoruError("fetch_error",
					word+": tls: verify: must be a Boolean", word)
			}
			p.Insecure = !b
		case "ca":
			pem, ok := tlsPEMArg(val)
			if !ok {
				return p, r.BoruError("fetch_error",
					word+": tls: ca: must be Bytes or a String of PEM", word)
			}
			p.RootsPEM = pem
		case "identity":
			// Atom is the idiomatic spelling (`identity: acme/q`);
			// String is accepted so a computed name works too.
			name, ok := tlsIdentityArg(val)
			if !ok {
				return p, r.BoruError("fetch_error",
					word+": tls: identity: must be an Atom or non-empty String", word)
			}
			p.Identity = name
		case "sni":
			s, sErr := val.AsConcreteString()
			if sErr != nil || s == "" {
				return p, r.BoruError("fetch_error",
					word+": tls: sni: must be a non-empty String", word)
			}
			p.ServerName = s
		case "min":
			v, mErr := tlsMinVersion(r, val, word)
			if mErr != nil {
				return p, mErr
			}
			p.MinVersion = v
		default:
			return p, unknownTLSOpt(r, key, word, serverOnlyTLSOpts)
		}
	}
	return p, nil
}

// serverOnlyTLSOpts / clientOnlyTLSOpts name the keys each side does
// NOT take, so a misuse says which side the option belongs to instead
// of the bare "unknown option" a reader would suspect of being a typo.
var serverOnlyTLSOpts = map[string]string{
	"require-client": "a listen option (the client-CA pool a server demands)",
}

var clientOnlyTLSOpts = map[string]string{
	"verify": "a dial option (a server does not verify a server chain)",
	"ca":     "a dial option (the roots a client checks the server against); a server names its client roots with require-client:",
	"sni":    "a dial option (the server name a client sends)",
}

func unknownTLSOpt(r *Registry, key, word string, otherSide map[string]string) error {
	if why, ok := otherSide[key]; ok {
		return r.BoruErrorHint("fetch_error",
			word+": tls: "+key+": is not available here", word,
			key+": is "+why)
	}
	return r.BoruError("fetch_error", word+": tls: unknown option \""+key+"\"", word)
}

// tlsMinVersion reads the `min:` option. Shared by both sides so the
// accepted spellings cannot drift apart.
func tlsMinVersion(r *Registry, val Value, word string) (uint16, error) {
	s, sErr := val.AsConcreteString()
	if sErr != nil {
		return 0, r.BoruError("fetch_error",
			word+`: tls: min: must be "1.2" or "1.3"`, word)
	}
	switch s {
	case "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	}
	return 0, r.BoruError("fetch_error",
		word+`: tls: min: must be "1.2" or "1.3", got "`+s+`"`, word)
}

// ParseServerTLSOpts reads the `tls: {…}` map on `listen` into a
// resolved ServerTLSProfile.
//
//	identity:       Atom|String|handle  the credential to PRESENT (required)
//	require-client: Bytes|String        client-CA roots, PEM; demands and
//	                                    verifies a client certificate
//	min:            String              "1.2" | "1.3"
//
// It shares `identity:` and `min:` with the dial-side parser through
// the same helpers, so an option cannot come to mean two things. The
// client-only keys are rejected by name rather than falling into the
// generic unknown-option arm — a `verify: false` silently ignored on a
// listener would read as configured when it is not.
func ParseServerTLSOpts(r *Registry, v Value, word string) (capabilities.ServerTLSProfile, error) {
	var p capabilities.ServerTLSProfile
	mp, err := RequireConcreteMap(v, word)
	if err != nil {
		return p, err
	}
	for _, key := range mp.Keys() {
		val, _ := mp.Get(key)
		switch key {
		case "identity":
			name, ok := tlsIdentityArg(val)
			if !ok {
				return p, r.BoruError("fetch_error",
					word+": tls: identity: must be an Atom or non-empty String", word)
			}
			p.Identity = name
		case "require-client":
			pem, ok := tlsPEMArg(val)
			if !ok {
				return p, r.BoruError("fetch_error",
					word+": tls: require-client: must be Bytes or a String of PEM", word)
			}
			p.ClientCAsPEM = pem
		case "min":
			v, mErr := tlsMinVersion(r, val, word)
			if mErr != nil {
				return p, mErr
			}
			p.MinVersion = v
		default:
			return p, unknownTLSOpt(r, key, word, clientOnlyTLSOpts)
		}
	}
	if p.Identity == "" {
		return p, r.BoruErrorHint("net_error",
			word+": tls: needs identity: — a TLS server must present a certificate", word,
			"the host registers credentials with (*Boru).RegisterClientIdentity")
	}
	return p, nil
}

// TLSIdentityHandles lets a module teach ParseTLSOpts an opaque handle
// shape that names a registered identity — boru:vault registers one so
// `tls: {identity: (Vault.identity "acme")}` works without a private key
// ever becoming a boru value.
//
// It is a hook rather than a direct call because the handle types live
// in lang/go/modules, which imports this package; the dependency cannot
// run the other way.
var TLSIdentityHandles []func(Value) (string, bool)

// tlsIdentityArg accepts an Atom (the idiomatic `acme/q`), a String, or
// a registered opaque handle.
func tlsIdentityArg(v Value) (string, bool) {
	for _, h := range TLSIdentityHandles {
		if name, ok := h(v); ok {
			return name, true
		}
	}
	if a, err := v.AsConcreteAtom(); err == nil && a != "" {
		return a, true
	}
	if s, err := v.AsConcreteString(); err == nil && s != "" {
		return s, true
	}
	return "", false
}

// tlsPEMArg accepts either a Bytes value (the natural output of reading
// a .pem through boru:io) or a String holding PEM text.
func tlsPEMArg(v Value) (string, bool) {
	if b, ok := AsBytesValue(v); ok {
		return string(b), true
	}
	if s, err := v.AsConcreteString(); err == nil && s != "" {
		return s, true
	}
	return "", false
}

// CheckTLSPolicy gates the parts of a TLSProfile that carry authority:
// `verify: false` (network/tls-insecure) removes verification, and
// `identity:` (network/client-cert) presents a credential. Supplying CA
// roots, an SNI override or a minimum version narrows or re-points
// trust rather than widening it, so those are ungated. Both ops sit in
// the existing `network` scope — a declared scope default-denies ops it
// does not list, so a profile that gates network at all gates these
// without further wiring.
func CheckTLSPolicy(r *Registry, p capabilities.TLSProfile, host string, port int) error {
	if r == nil {
		return nil
	}
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	args := policy.Args{"host": host, "port": port}
	if p.Identity != "" {
		// Presenting a credential is its own authority: the policy —
		// not the program — decides WHICH identity may be used and
		// against which host. The identity name rides in the args so a
		// profile can allow-list per host.
		args["identity"] = p.Identity
		if err := pol.Check("network", "client-cert", args); err != nil {
			return err
		}
	}
	if p.Insecure {
		return pol.Check("network", "tls-insecure", args)
	}
	return nil
}

// CheckServerTLSPolicy gates presenting a SERVER certificate, under its
// own op rather than `client-cert`. The two are different authorities:
// a client certificate proves who a program is when it calls out, while
// a server certificate lets it answer AS a service on the network — a
// deployment may well grant one and decline the other. `require-client:`
// is ungated for the same reason `ca:` is on the dial side: demanding a
// client certificate narrows who may connect, it never widens it.
func CheckServerTLSPolicy(r *Registry, p capabilities.ServerTLSProfile, host string, port int) error {
	if r == nil {
		return nil
	}
	pol := HostPolicy(r)
	if pol == nil {
		return nil
	}
	return pol.Check("network", "server-cert", policy.Args{
		"host": host, "port": port, "identity": p.Identity,
	})
}
