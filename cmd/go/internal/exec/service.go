// service.go provides the Service-shaped wrapper around the exec
// HTTP server: a long-running unit that can be started, stopped,
// and paused under the supervisor in `boru serve`. The pre-existing
// Handler() function and request handlers in exec.go are reused
// unchanged.

package exec

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/service"
	"github.com/boru-lang/boru/lang/go/policy"
)

// Server is the lifecycle-managed exec HTTP server. Construct with
// NewServer, then call Start/Stop. Pause swaps the live handler to
// a 503 responder; Resume restores it.
//
// The policy field is bound at construction and is immutable for
// the lifetime of the server. Requests cannot override or supply a
// policy — that is the security invariant of the exec service.
type Server struct {
	addr     string
	registry string
	policy   policy.Policy
	ln       net.Listener
	srv      *http.Server
	inner    http.Handler // pre-pause handler, restored on Resume
	paused   atomic.Bool
	state    atomic.Int32 // service.State
	mu       sync.Mutex   // guards ln/srv during start/stop
}

// NewServer builds an exec Server bound to addr ("host:port"). The
// registry path (may be empty) is forwarded to every boru instance
// the server creates per request. The policy (may be nil) is fixed
// at construction; clients cannot override it via the request body.
func NewServer(addr, registry string, pol policy.Policy) (*Server, error) {
	if addr == "" {
		addr = "127.0.0.1:8091"
	}
	s := &Server{
		addr:     addr,
		registry: registry,
		policy:   pol,
	}
	inner := Handler(registry, pol)
	s.inner = inner
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.state.Store(int32(service.StateStopped))
	return s, nil
}

// Name returns "exec".
func (s *Server) Name() string { return "exec" }

// Status returns the current lifecycle state.
func (s *Server) Status() service.State {
	return service.State(s.state.Load())
}

// UsesStdio always returns false: the exec service only speaks HTTP.
func (s *Server) UsesStdio() bool { return false }

// Addr returns the configured (or bound, post-Start) listen address.
func (s *Server) Addr() string { return s.addr }

// Registry returns the registry path forwarded to boru instances.
func (s *Server) Registry() string { return s.registry }

// Start binds the listener and serves requests until ctx is canceled
// or the server fails. Returns nil on a clean ctx-driven shutdown.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	s.state.Store(int32(service.StateStarting))
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		s.state.Store(int32(service.StateStopped))
		s.mu.Unlock()
		return fmt.Errorf("exec: listen %s: %w", s.addr, err)
	}
	s.ln = ln
	s.mu.Unlock()

	// Publish the bound port so Metadata() / Addr() report it
	// accurately when the user asked for port 0.
	s.addr = ln.Addr().String()

	s.state.Store(int32(service.StateRunning))

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.srv.Shutdown(shutdownCtx)
		case <-done:
		}
	}()

	err = s.srv.Serve(ln)
	close(done)
	s.state.Store(int32(service.StateStopped))

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Stop requests a graceful HTTP shutdown.
func (s *Server) Stop(ctx context.Context) error {
	s.state.Store(int32(service.StateStopping))
	if err := s.srv.Shutdown(ctx); err != nil {
		return err
	}
	return nil
}

// Pause swaps the live handler for a 503 responder; the listener
// stays bound so existing clients see clear errors instead of
// connection declined.
func (s *Server) Pause(ctx context.Context) error {
	s.paused.Store(true)
	s.state.Store(int32(service.StatePaused))
	return nil
}

// Resume restores the normal handler.
func (s *Server) Resume(ctx context.Context) error {
	s.paused.Store(false)
	s.state.Store(int32(service.StateRunning))
	return nil
}

// serveHTTP is the always-installed handler: it short-circuits to a
// 503 while paused and otherwise delegates to the real handler.
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if s.paused.Load() {
		http.Error(w, "exec paused", http.StatusServiceUnavailable)
		return
	}
	s.inner.ServeHTTP(w, r)
}

// Metadata returns the exec server's observable runtime state for
// the api service to surface.
func (s *Server) Metadata() map[string]string {
	m := map[string]string{"addr": s.addr}
	if s.registry != "" {
		m["registry"] = s.registry
	}
	if s.policy != nil {
		m["policy"] = s.policy.Name()
	}
	return m
}

// compile-time interface checks.
var (
	_ service.Service      = (*Server)(nil)
	_ service.Pausable     = (*Server)(nil)
	_ service.StdioUser    = (*Server)(nil)
	_ service.WithMetadata = (*Server)(nil)
)
