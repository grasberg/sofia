package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const checkCacheTTL = 30 * time.Second

type Server struct {
	server    *http.Server
	mu        sync.RWMutex
	ready     bool
	checkFns  map[string]func() (bool, string)
	cache     map[string]cachedCheck
	startTime time.Time
}

type cachedCheck struct {
	result Check
	expiry time.Time
}

type Check struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type StatusResponse struct {
	Status string           `json:"status"`
	Uptime string           `json:"uptime"`
	Checks map[string]Check `json:"checks,omitempty"`
}

func NewServer(host string, port int) *Server {
	mux := http.NewServeMux()
	s := &Server{
		ready:     false,
		checkFns:  make(map[string]func() (bool, string)),
		cache:     make(map[string]cachedCheck),
		startTime: time.Now(),
	}

	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/ready", s.readyHandler)

	addr := fmt.Sprintf("%s:%d", host, port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return s
}

// RegisterMetrics attaches a /metrics endpoint served by the given MetricsProvider.
func (s *Server) RegisterMetrics(mp *MetricsProvider) {
	// The mux is only accessible via the server handler.
	// Since we own the mux, we can type-assert it back.
	if mux, ok := s.server.Handler.(*http.ServeMux); ok {
		mux.HandleFunc("/metrics", mp.Handler())
	}
}

func (s *Server) Start() error {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	return s.server.ListenAndServe()
}

func (s *Server) StartContext(ctx context.Context) error {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.server.Shutdown(context.Background())
	}
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.ready = false
	s.mu.Unlock()
	return s.server.Shutdown(ctx)
}

func (s *Server) SetReady(ready bool) {
	s.mu.Lock()
	s.ready = ready
	s.mu.Unlock()
}

func (s *Server) RegisterCheck(name string, checkFn func() (bool, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkFns[name] = checkFn
}

// evaluateChecks runs all registered check functions, using cached results
// when available and not expired.
func (s *Server) evaluateChecks() map[string]Check {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	checks := make(map[string]Check, len(s.checkFns))
	for name, fn := range s.checkFns {
		if cached, ok := s.cache[name]; ok && now.Before(cached.expiry) {
			checks[name] = cached.result
			continue
		}
		status, msg := fn()
		c := Check{
			Name:      name,
			Status:    statusString(status),
			Message:   msg,
			Timestamp: now,
		}
		s.cache[name] = cachedCheck{result: c, expiry: now.Add(checkCacheTTL)}
		checks[name] = c
	}
	return checks
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	uptime := time.Since(s.startTime)
	resp := StatusResponse{
		Status: "ok",
		Uptime: uptime.String(),
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Re-evaluate checks (with TTL caching) on each request.
	checks := s.evaluateChecks()

	s.mu.RLock()
	ready := s.ready
	s.mu.RUnlock()

	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(StatusResponse{
			Status: "not ready",
			Checks: checks,
		})
		return
	}

	for _, check := range checks {
		if check.Status == "fail" {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(StatusResponse{
				Status: "not ready",
				Checks: checks,
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	uptime := time.Since(s.startTime)
	json.NewEncoder(w).Encode(StatusResponse{
		Status: "ready",
		Uptime: uptime.String(),
		Checks: checks,
	})
}

func statusString(ok bool) string {
	if ok {
		return "ok"
	}
	return "fail"
}
