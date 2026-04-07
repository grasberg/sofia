package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// maxRequestBody is the maximum allowed size for JSON request bodies (10 MB).
const maxRequestBody = 10 << 20

// authMiddleware checks for a valid bearer token when WebUI.AuthToken is
// configured. If the token is empty, authentication is skipped for backward
// compatibility. It also enforces a CSRF check: mutating methods (POST, PUT,
// DELETE) must include the X-Requested-With header, which browsers will not
// send cross-origin without a CORS preflight.
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Auth check — skip when no token is configured.
		token := s.cfg.WebUI.AuthToken
		if token != "" {
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				s.sendJSONError(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		// CSRF check for mutating methods.
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			if r.Header.Get("X-Requested-With") == "" {
				s.sendJSONError(w, "Missing X-Requested-With header", http.StatusForbidden)
				return
			}
		}

		next(w, r)
	}
}

// rateLimitMiddleware rejects requests that exceed the per-IP rate limit.
// When the limiter is nil (e.g. in tests) all requests are allowed through.
func (s *Server) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.limiter != nil && !s.limiter.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			s.sendJSONError(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// limitBody wraps the request body with http.MaxBytesReader to cap the
// payload size at maxRequestBody bytes.
func limitBody(r *http.Request) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxRequestBody)
}
