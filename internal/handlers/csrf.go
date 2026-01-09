package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const (
	csrfCookieName = "csrf_token"
	csrfFormField  = "csrf_token"
	csrfTokenLen   = 32
	csrfMaxAge     = 12 * time.Hour
)

// csrfManager handles CSRF token generation and validation
type csrfManager struct {
	mu     sync.RWMutex
	tokens map[string]time.Time // token -> expiry
}

var csrf = &csrfManager{
	tokens: make(map[string]time.Time),
}

// generateToken creates a new cryptographically secure CSRF token
func (m *csrfManager) generateToken() (string, error) {
	bytes := make([]byte, csrfTokenLen)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(bytes)

	m.mu.Lock()
	m.tokens[token] = time.Now().Add(csrfMaxAge)
	m.mu.Unlock()

	return token, nil
}

// validateToken checks if a token is valid and not expired
func (m *csrfManager) validateToken(token string) bool {
	if token == "" {
		return false
	}

	m.mu.RLock()
	expiry, exists := m.tokens[token]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	return time.Now().Before(expiry)
}

// cleanup removes expired tokens (called periodically)
func (m *csrfManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for token, expiry := range m.tokens {
		if now.After(expiry) {
			delete(m.tokens, token)
		}
	}
}

// getOrCreateToken gets existing token from cookie or creates new one
func (h *Handler) getOrCreateCSRFToken(w http.ResponseWriter, r *http.Request) string {
	// Check for existing valid token in cookie
	if cookie, err := r.Cookie(csrfCookieName); err == nil {
		if csrf.validateToken(cookie.Value) {
			return cookie.Value
		}
	}

	// Generate new token
	token, err := csrf.generateToken()
	if err != nil {
		return ""
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(csrfMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	return token
}

// requireCSRF validates CSRF and writes error response if invalid.
// Returns true if valid, false if invalid (response already written).
func (h *Handler) requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	if h.validateCSRF(r) {
		return true
	}
	http.Error(w, "Invalid CSRF token", http.StatusForbidden)
	return false
}

// validateCSRF checks CSRF token on POST requests
func (h *Handler) validateCSRF(r *http.Request) bool {
	// Skip CSRF validation if disabled (desktop mode)
	if h.disableCSRF {
		return true
	}

	// Only validate POST, PUT, DELETE
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}

	// Get token from cookie
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return false
	}

	// Get token from form
	if err := r.ParseForm(); err != nil {
		return false
	}
	formToken := r.FormValue(csrfFormField)

	// Tokens must match and be valid
	return cookie.Value == formToken && csrf.validateToken(formToken)
}

// CSRFField returns an HTML hidden input field with the CSRF token
func (h *Handler) CSRFField(token string) string {
	return `<input type="hidden" name="` + csrfFormField + `" value="` + token + `">`
}

// StartCSRFCleanup starts periodic cleanup of expired tokens
func StartCSRFCleanup() {
	go func() {
		ticker := time.NewTicker(time.Hour)
		for range ticker.C {
			csrf.cleanup()
		}
	}()
}
