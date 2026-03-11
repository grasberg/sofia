package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	s := NewServer("localhost", 8080)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.server.Addr != "localhost:8080" {
		t.Errorf("expected address localhost:8080, got %s", s.server.Addr)
	}
	if s.ready != false {
		t.Errorf("expected ready to be false initially, got %v", s.ready)
	}
}

func TestHealthHandler(t *testing.T) {
	s := NewServer("localhost", 0)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	s.healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp StatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
}

func TestReadyHandlerNotReady(t *testing.T) {
	s := NewServer("localhost", 0)
	s.SetReady(false)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	s.readyHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}

	var resp StatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "not ready" {
		t.Errorf("expected status 'not ready', got %q", resp.Status)
	}
}

func TestReadyHandlerReady(t *testing.T) {
	s := NewServer("localhost", 0)
	s.SetReady(true)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	s.readyHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp StatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ready" {
		t.Errorf("expected status 'ready', got %q", resp.Status)
	}
}

func TestSetReady(t *testing.T) {
	s := NewServer("localhost", 0)

	s.SetReady(true)
	if !s.ready {
		t.Errorf("expected ready to be true after SetReady(true)")
	}

	s.SetReady(false)
	if s.ready {
		t.Errorf("expected ready to be false after SetReady(false)")
	}
}

func TestRegisterCheck(t *testing.T) {
	s := NewServer("localhost", 0)

	s.RegisterCheck("test_check", func() (bool, string) {
		return true, "all good"
	})

	check, exists := s.checks["test_check"]
	if !exists {
		t.Fatal("expected check to be registered")
	}

	if check.Name != "test_check" {
		t.Errorf("expected check name 'test_check', got %q", check.Name)
	}

	if check.Status != "ok" {
		t.Errorf("expected check status 'ok', got %q", check.Status)
	}

	if check.Message != "all good" {
		t.Errorf("expected check message 'all good', got %q", check.Message)
	}
}

func TestStop(t *testing.T) {
	s := NewServer("localhost", 0)
	s.SetReady(true)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := s.Stop(ctx)
	if err != nil {
		t.Logf("Stop returned error (expected in test context): %v", err)
	}

	if s.ready {
		t.Errorf("expected ready to be false after Stop")
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		ok       bool
		expected string
	}{
		{true, "ok"},
		{false, "fail"},
	}

	for _, tt := range tests {
		result := statusString(tt.ok)
		if result != tt.expected {
			t.Errorf("statusString(%v) = %q, expected %q", tt.ok, result, tt.expected)
		}
	}
}
