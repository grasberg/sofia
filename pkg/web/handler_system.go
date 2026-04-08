package web

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/grasberg/sofia/pkg/audit"
	"github.com/grasberg/sofia/pkg/cron"
	"github.com/grasberg/sofia/pkg/logger"
)

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})

	go func() {
		time.Sleep(500 * time.Millisecond)
		argv0, err := os.Executable()
		if err != nil {
			logger.ErrorCF("web", "Failed to get executable for restart", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
		logger.InfoCF("web", "Restarting Sofia via Web UI...", nil)
		err = syscall.Exec(argv0, os.Args, os.Environ())
		if err != nil {
			logger.ErrorCF("web", "Exec failed", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
	}()
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logger.InfoCF("web", "Starting update process via Web UI...", nil)

	cmd := exec.Command("git", "pull")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.ErrorCF("web", "Git pull failed", map[string]any{"error": err.Error()})
		http.Error(w, "Failed to pull updates: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cmd = exec.Command("make", "build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.ErrorCF("web", "Make build failed", map[string]any{"error": err.Error()})
		http.Error(w, "Failed to build: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})

	go func() {
		time.Sleep(500 * time.Millisecond)
		argv0, err := os.Executable()
		if err != nil {
			logger.ErrorCF("web", "Failed to get executable for restart", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
		logger.InfoCF("web", "Restarting Sofia after update...", nil)
		err = syscall.Exec(argv0, os.Args, os.Environ())
		if err != nil {
			logger.ErrorCF("web", "Exec failed after update", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
	}()
}

// handleReset handles POST /api/reset — cancels in-flight work, clears sessions, and resets goals.
func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result := s.agentLoop.Reset()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handlePresence returns the current agent presence state as JSON.
func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	presence := s.agentLoop.DashboardHub().GetPresence()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(presence)
}

// SetAuditLogger assigns the audit logger used by the /api/audit endpoint.
func (s *Server) SetAuditLogger(al *audit.AuditLogger) {
	s.auditLogger = al
}

// SetCronService assigns the cron service used by the /api/cron endpoint.
func (s *Server) SetCronService(cs *cron.CronService) {
	s.cronService = cs
}

// handleTorStatus returns the current Tor service state as JSON.
func (s *Server) handleTorStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"enabled": s.agentLoop.TorService().IsEnabled()})
}

// handleTorToggle starts or stops the Tor service based on its current state.
func (s *Server) handleTorToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	svc := s.agentLoop.TorService()
	w.Header().Set("Content-Type", "application/json")

	if svc.IsEnabled() {
		if err := svc.Stop(); err != nil {
			json.NewEncoder(w).Encode(map[string]any{"enabled": false, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"enabled": false})
	} else {
		if err := svc.Start(); err != nil {
			json.NewEncoder(w).Encode(map[string]any{"enabled": false, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"enabled": true})
	}
}
