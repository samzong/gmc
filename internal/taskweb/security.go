package taskweb

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const tokenHeader = "X-GMC-Token"

const tokenPlaceholder = "__GMC_TOKEN_VALUE__"

func newSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")

		if !s.hostAllowed(r.Host) {
			writeError(w, http.StatusForbidden, "request host does not belong to this server")
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && !s.originAllowed(origin) {
			writeError(w, http.StatusForbidden, "cross-origin requests are not allowed")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get(tokenHeader) != s.token {
			writeError(w, http.StatusUnauthorized, "missing or invalid "+tokenHeader)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostAllowed(host string) bool {
	name, port, err := net.SplitHostPort(strings.TrimSpace(host))
	if err != nil {
		return false
	}
	if s.port != "" && port != s.port {
		return false
	}

	switch strings.ToLower(name) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func (s *Server) originAllowed(origin string) bool {
	if strings.EqualFold(origin, "null") {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return s.hostAllowed(parsed.Host)
}
