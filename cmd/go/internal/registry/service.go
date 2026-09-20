// service.go provides the Service-shaped wrapper around the registry
// HTTP server: a long-running unit that can be started, stopped, and
// paused under the supervisor in `boru serve`. The pre-existing
// Handler() function and request handlers in registry.go are reused
// unchanged.

package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boru-lang/boru/cmd/go/internal/service"
)

// Server is the lifecycle-managed registry HTTP server. Construct
// with NewServer, then call Start/Stop. Pause swaps the live handler
// to a 503 responder; Resume restores it.
type Server struct {
	dir    string
	addr   string
	ln     net.Listener
	srv    *http.Server
	inner  http.Handler // pre-pause handler, restored on Resume
	paused atomic.Bool
	state  atomic.Int32 // service.State
	mu     sync.Mutex   // guards ln/srv during start/stop
}

// NewServer validates dir, builds the HTTP server, and returns a
// Server ready to Start. It does not bind the listener until Start
// runs, so port conflicts surface from Start rather than NewServer.
func NewServer(dir string, port int) (*Server, error) {
	if dir == "" {
		return nil, errors.New("registry: -r <folder> is required")
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("registry: folder %q not found", dir)
	}

	s := &Server{
		dir:  dir,
		addr: fmt.Sprintf(":%d", port),
	}
	inner := Handler(dir)
	s.inner = inner
	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: http.HandlerFunc(s.serveHTTP),
		// Bound the slow-client surface (slowloris): the registry serves
		// uploads and is the most network-facing service, so it must set
		// the same timeouts as exec/api/proxy rather than rely on the
		// unbounded net/http defaults.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	s.state.Store(int32(service.StateStopped))
	return s, nil
}

// Name returns "registry".
func (s *Server) Name() string { return "registry" }

// Status returns the current lifecycle state.
func (s *Server) Status() service.State {
	return service.State(s.state.Load())
}

// UsesStdio always returns false: the registry only speaks HTTP.
func (s *Server) UsesStdio() bool { return false }

// Addr returns the configured listen address (":<port>"). Exposed so
// the supervisor can include it in status output.
func (s *Server) Addr() string { return s.addr }

// Dir returns the registry root directory.
func (s *Server) Dir() string { return s.dir }

// Start binds the listener and serves requests until ctx is canceled
// or the server fails. Returns nil on a clean ctx-driven shutdown.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	s.state.Store(int32(service.StateStarting))
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		s.state.Store(int32(service.StateStopped))
		s.mu.Unlock()
		return fmt.Errorf("registry: listen %s: %w", s.addr, err)
	}
	s.ln = ln
	s.mu.Unlock()

	// Once we know the bound port (relevant when port=0), publish
	// it on s.addr so Status output is accurate.
	s.addr = ln.Addr().String()

	s.state.Store(int32(service.StateRunning))

	// Cancel-on-ctx: graceful Shutdown when the supervisor cancels.
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
		http.Error(w, "registry paused", http.StatusServiceUnavailable)
		return
	}
	s.inner.ServeHTTP(w, r)
}

// Metadata returns the registry's observable runtime state for the
// api service to surface.
func (s *Server) Metadata() map[string]string {
	return map[string]string{
		"addr": s.addr,
		"dir":  s.dir,
	}
}

// compile-time interface checks.
var (
	_ service.Service      = (*Server)(nil)
	_ service.Pausable     = (*Server)(nil)
	_ service.StdioUser    = (*Server)(nil)
	_ service.WithMetadata = (*Server)(nil)
)
