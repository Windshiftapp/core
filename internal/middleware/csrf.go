package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CSRFMiddleware handles CSRF protection for web requests
type CSRFMiddleware struct {
	tokenStore    map[string]time.Time // In-memory token store
	mutex         sync.RWMutex         // Protects concurrent access to tokenStore
	maxAge        time.Duration
	cleanupTicker *time.Ticker         // Periodic cleanup ticker
}

// NewCSRFMiddleware creates a new CSRF middleware
func NewCSRFMiddleware() *CSRFMiddleware {
	cm := &CSRFMiddleware{
		tokenStore:    make(map[string]time.Time),
		maxAge:        24 * time.Hour, // Tokens valid for 24 hours
		cleanupTicker: time.NewTicker(1 * time.Hour), // Clean up every hour
	}
	
	// Start background cleanup goroutine
	go cm.startCleanupLoop()
	
	return cm
}

// GenerateToken creates a new CSRF token
func (cm *CSRFMiddleware) GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate CSRF token: %w", err)
	}
	
	token := hex.EncodeToString(bytes)
	
	// Thread-safe token storage
	cm.mutex.Lock()
	cm.tokenStore[token] = time.Now()
	cm.mutex.Unlock()
	
	return token, nil
}

// ValidateToken checks if a CSRF token is valid
func (cm *CSRFMiddleware) ValidateToken(token string) bool {
	if token == "" {
		return false
	}
	
	cm.mutex.RLock()
	createdAt, exists := cm.tokenStore[token]
	cm.mutex.RUnlock()
	
	if !exists {
		return false
	}
	
	// Check if token has expired
	if time.Since(createdAt) > cm.maxAge {
		cm.mutex.Lock()
		delete(cm.tokenStore, token)
		cm.mutex.Unlock()
		return false
	}
	
	return true
}

// ConsumeToken validates and removes a CSRF token (one-time use)
func (cm *CSRFMiddleware) ConsumeToken(token string) bool {
	if token == "" {
		return false
	}
	
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	
	createdAt, exists := cm.tokenStore[token]
	if !exists {
		return false
	}
	
	// Check if token has expired
	if time.Since(createdAt) > cm.maxAge {
		delete(cm.tokenStore, token)
		return false
	}
	
	// Remove token after successful validation (one-time use)
	delete(cm.tokenStore, token)
	return true
}

// CSRFProtection middleware that validates CSRF tokens for state-changing operations
func (cm *CSRFMiddleware) CSRFProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip CSRF check if request is marked as exempt (bearer token auth)
		if exempt, ok := r.Context().Value(ContextKeyCSRFExempt).(bool); ok && exempt {
			next.ServeHTTP(w, r)
			return
		}
		
		// Only check CSRF for state-changing operations
		if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}
		
		// Get CSRF token from header or form
		token := r.Header.Get("X-CSRF-Token")
		if token == "" {
			token = r.FormValue("csrf_token")
		}
		
		if token == "" {
			cm.handleCSRFError(w, r, "CSRF token missing")
			return
		}

		if !cm.ConsumeToken(token) {
			cm.handleCSRFError(w, r, "Invalid or expired CSRF token")
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// GetTokenHandler provides a CSRF token to the client
func (cm *CSRFMiddleware) GetTokenHandler(w http.ResponseWriter, r *http.Request) {
	token, err := cm.GenerateToken()
	if err != nil {
		http.Error(w, "Failed to generate CSRF token", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"csrf_token": "%s"}`, token)))
}

// handleCSRFError handles CSRF validation errors
func (cm *CSRFMiddleware) handleCSRFError(w http.ResponseWriter, r *http.Request, message string) {
	// For API requests, return JSON error
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "` + message + `", "code": "CSRF_ERROR"}`))
		return
	}
	
	// For web requests, return 403
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(message))
}

// startCleanupLoop runs periodic cleanup of expired tokens
func (cm *CSRFMiddleware) startCleanupLoop() {
	for {
		select {
		case <-cm.cleanupTicker.C:
			cm.cleanupExpiredTokens()
		}
	}
}

// cleanupExpiredTokens removes expired tokens from memory
func (cm *CSRFMiddleware) cleanupExpiredTokens() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	
	now := time.Now()
	for token, createdAt := range cm.tokenStore {
		if now.Sub(createdAt) > cm.maxAge {
			delete(cm.tokenStore, token)
		}
	}
}

// Stop stops the cleanup ticker and clears all tokens
func (cm *CSRFMiddleware) Stop() {
	if cm.cleanupTicker != nil {
		cm.cleanupTicker.Stop()
	}
	
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.tokenStore = make(map[string]time.Time)
}

// AddCSRFTokenToContext adds a fresh CSRF token to the request context
func (cm *CSRFMiddleware) AddCSRFTokenToContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip for bearer token requests
		if exempt, ok := r.Context().Value(ContextKeyCSRFExempt).(bool); ok && exempt {
			next.ServeHTTP(w, r)
			return
		}
		
		token, err := cm.GenerateToken()
		if err != nil {
			// Don't fail the request, just continue without token
			next.ServeHTTP(w, r)
			return
		}
		
		ctx := context.WithValue(r.Context(), "csrf_token", token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}