package capabilities

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
)

// HTTPOps is the host capability that supplies the outbound HTTP
// transport used by boru:net's `fetch`. It exists so the transport —
// and therefore TLS configuration, the connection pool, and protocol
// negotiation — is chosen by the host rather than hard-wired into the
// word handler.
//
// It returns a RoundTripper rather than an *http.Client deliberately.
// The pool and the TLS configuration live on the transport, while the
// per-request timeout lives on the client, so `fetch` builds a cheap
// client around a shared transport for each call. Handing back a client
// instead would force the implementation to key its cache on the
// guest-supplied timeout, which is an unbounded cache keyed by
// untrusted input.
//
// TLSProfile is the resolved TLS configuration for the request; id is
// the resolved client identity (nil when the profile names none).
// Implementations build and return a transport — CACHING IS THE
// CALLER'S JOB (native.ResolveTransport), because a profile names an
// identity by a registry-scoped name, so a process-wide cache keyed on
// the profile alone would serve registry A's transport to registry B's
// identically-named identity.
type HTTPOps interface {
	Transport(p TLSProfile, id ClientIdentity) (http.RoundTripper, error)
}

// TLSProfile is the resolved TLS configuration for one outbound
// request, derived from the guest's `tls:` option map.
//
// The zero value means "Go's defaults": verify against the system root
// pool, no server-name override, Go's minimum protocol version. Note
// which way the Insecure field points — per the no-zero-value-overload
// rule (core/go/CLAUDE.md), the SAFE state is the zero value, so a
// profile nobody configured cannot silently skip verification.
//
// Every field is comparable so the whole struct can be a map key.
type TLSProfile struct {
	// Insecure disables verification of the peer's certificate chain
	// AND host name. It is policy-gated (network/tls-insecure).
	Insecure bool
	// RootsPEM is additional CA roots in PEM form. Empty means the
	// system pool. When set, the roots REPLACE the system pool — an
	// explicit private-CA endpoint should not also trust the public
	// web.
	RootsPEM string
	// ServerName overrides SNI and the name checked against the
	// certificate. Empty means derive from the URL host.
	ServerName string
	// MinVersion is a crypto/tls VersionTLSxx constant; 0 means Go's
	// default minimum.
	MinVersion uint16
	// Identity names a host-registered ClientIdentity to present for
	// mutual TLS. Empty means present none. It is a NAME, not a
	// credential: the guest can select an identity but can never read
	// or construct one.
	Identity string
}

// ErrBadRootsPEM is returned when RootsPEM contains no usable
// certificate — a typo or a truncated file, which would otherwise
// silently produce an empty trust pool that rejects everything.
var ErrBadRootsPEM = errors.New("tls: ca contains no usable certificate")

// DefaultHTTPOps is the HTTPOps used when a host installs none. For the
// zero profile it serves http.DefaultTransport, which is exactly what
// an *http.Client with a nil Transport would have used — so installing
// nothing preserves the historical behaviour, including proxy support
// from the environment, the shared idle-connection pool, and automatic
// HTTP/2. For a configured profile it builds a transport cloned from
// http.DefaultTransport so those same defaults are kept.
type DefaultHTTPOps struct{}

// Transport returns http.DefaultTransport for the zero profile with no
// identity — byte-for-byte the pre-TLS behaviour — and otherwise builds
// a transport carrying p and id. It does not cache: see the HTTPOps
// doc for why that is the caller's job.
func (DefaultHTTPOps) Transport(p TLSProfile, id ClientIdentity) (http.RoundTripper, error) {
	if p == (TLSProfile{}) && id == nil {
		return http.DefaultTransport, nil
	}
	return buildTransport(p, id)
}

// TLSConfigFor builds the crypto/tls configuration a profile
// describes. It is exported because BOTH transports need it — the HTTP
// client here and the raw sockets in boru:net — and a second
// hand-written copy is exactly how the two would drift into disagreeing
// about what `verify: false` or `ca:` mean.
func TLSConfigFor(p TLSProfile, id ClientIdentity) (*tls.Config, error) {
	cfg := &tls.Config{
		InsecureSkipVerify: p.Insecure, //nolint:gosec // policy-gated; see network/tls-insecure
		ServerName:         p.ServerName,
		MinVersion:         p.MinVersion,
	}
	if id != nil {
		cfg.GetClientCertificate = getClientCertificate(id, p.ServerName)
	}
	if p.RootsPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(p.RootsPEM)) {
			return nil, ErrBadRootsPEM
		}
		cfg.RootCAs = pool
	}
	return cfg, nil
}

// buildTransport clones http.DefaultTransport and applies p.
func buildTransport(p TLSProfile, id ClientIdentity) (http.RoundTripper, error) {
	cfg, err := TLSConfigFor(p, id)
	if err != nil {
		return nil, err
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok { //covergate:allow http.DefaultTransport is a *http.Transport in every supported Go release
		return nil, errors.New("tls: http.DefaultTransport is not an *http.Transport")
	}
	tr := base.Clone()
	tr.TLSClientConfig = cfg
	// Setting TLSClientConfig on a Transport disables automatic HTTP/2
	// unless this is on; without it, configuring TLS would silently
	// downgrade every request to HTTP/1.1. Clone() carries
	// DefaultTransport's true, but set it explicitly so a future change
	// to the default cannot reintroduce the downgrade unnoticed.
	tr.ForceAttemptHTTP2 = true
	return tr, nil
}
