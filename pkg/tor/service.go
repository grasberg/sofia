package tor

import (
	"fmt"
	"net"
	"os/exec"
	"sync"
	"time"
)

const (
	torSocksAddr = "127.0.0.1:9050"
	torProxyURL  = "socks5://127.0.0.1:9050"
	startTimeout = 30 * time.Second
)

// Service manages a tor process lifecycle for anonymous web requests.
type Service struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	enabled   bool
	baseProxy string // fallback when Tor is disabled (from config)
}

// New creates a new Service. baseProxy is used for web tools when Tor is disabled.
func New(baseProxy string) *Service {
	return &Service{baseProxy: baseProxy}
}

// Start launches the tor binary and waits until its SOCKS5 port is ready.
// Returns an error if tor is not installed or fails to start within 30s.
func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.enabled {
		return nil
	}

	torPath, err := exec.LookPath("tor")
	if err != nil {
		return fmt.Errorf("tor not found — install with: brew install tor  (macOS) or  apt install tor  (Linux)")
	}

	cmd := exec.Command(torPath) //nolint:gosec
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start tor: %w", err)
	}

	if err := waitForPort(torSocksAddr, startTimeout); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("tor did not become ready: %w", err)
	}

	s.cmd = cmd
	s.enabled = true
	return nil
}

// Stop kills the running tor process.
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.enabled || s.cmd == nil {
		s.enabled = false
		return nil
	}

	if err := s.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to stop tor: %w", err)
	}
	_ = s.cmd.Wait()
	s.cmd = nil
	s.enabled = false
	return nil
}

// IsEnabled returns true when the tor process is running and active.
func (s *Service) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

// ProxyURL returns the SOCKS5 proxy URL when Tor is enabled, or the base
// proxy (from config) when disabled. Tools call this on each request.
func (s *Service) ProxyURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled {
		return torProxyURL
	}
	return s.baseProxy
}

// waitForPort polls addr until it accepts a TCP connection or the timeout elapses.
func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s waiting for %s", timeout, addr)
}
