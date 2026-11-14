package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/allegro/bigcache/v3"
	"golang.org/x/sync/singleflight"

	"windshift/internal/cacheutil"
	"windshift/internal/config"
	"windshift/internal/models"
)

const (
	sessionValidationCacheMaxEntry    = 8 * 1024
	sessionValidationCacheCleanWindow = time.Second
	sessionValidationMutationLimit    = 16 * 1024
)

type cachedSessionValidation struct {
	LoadedAt        time.Time `json:"loaded_at"`
	Epoch           uint64    `json:"epoch"`
	TokenVersion    uint64    `json:"token_version"`
	SessionVersion  uint64    `json:"session_version"`
	UserVersion     uint64    `json:"user_version"`
	Session         Session   `json:"session"`
	AuthPendingType string    `json:"auth_pending_type"`
}

type sessionValidationLoad struct {
	epoch    uint64
	sequence uint64
	cacheKey string
	session  *Session
}

type sessionValidator struct {
	cache *bigcache.BigCache
	ttl   time.Duration

	loads    singleflight.Group
	epoch    atomic.Uint64
	sequence atomic.Uint64

	mutationMu      sync.RWMutex
	tokenVersions   map[string]uint64
	sessionVersions map[int]uint64
	userVersions    map[int]uint64

	hits                atomic.Uint64
	misses              atomic.Uint64
	databaseLoads       atomic.Uint64
	coalescedWaiters    atomic.Uint64
	invalidatedEntries  atomic.Uint64
	evictedEntries      atomic.Uint64
	staleRejections     atomic.Uint64
	cacheDecodeFailures atomic.Uint64
}

// SessionValidationCacheStats is a point-in-time view of the local session
// validation cache. It intentionally contains no token or user identifiers.
type SessionValidationCacheStats struct {
	Enabled             bool
	TTL                 time.Duration
	Entries             int
	Hits                uint64
	Misses              uint64
	DatabaseLoads       uint64
	CoalescedWaiters    uint64
	InvalidatedEntries  uint64
	EvictedEntries      uint64
	StaleRejections     uint64
	CacheDecodeFailures uint64
}

func newSessionValidator(ttl time.Duration, cacheName string, cacheSizeMB ...int) *sessionValidator {
	validator := &sessionValidator{
		ttl:             ttl,
		tokenVersions:   make(map[string]uint64),
		sessionVersions: make(map[int]uint64),
		userVersions:    make(map[int]uint64),
	}
	if ttl <= 0 {
		return validator
	}

	maxCacheMB := 8
	if len(cacheSizeMB) > 0 && cacheSizeMB[0] > 0 {
		maxCacheMB = cacheSizeMB[0]
	}
	cache, err := cacheutil.New(cacheName, cacheutil.BigCacheOptions{
		TTL:               ttl,
		MaxCacheMB:        maxCacheMB,
		Shards:            16,
		MaxEntrySize:      sessionValidationCacheMaxEntry,
		InitialCapacityMB: 1,
		CleanWindow:       sessionValidationCacheCleanWindow,
		OnRemoveWithReason: func(_ string, _ []byte, reason bigcache.RemoveReason) {
			if reason == bigcache.Expired || reason == bigcache.NoSpace {
				validator.evictedEntries.Add(1)
			}
		},
	})
	if err != nil {
		slog.Error("failed to initialize session validation cache; continuing without retained entries",
			slog.Any("error", err))
		return validator
	}
	validator.cache = cache
	return validator
}

// SessionValidationCacheStats returns aggregate cache counters for diagnostics.
func (sm *SessionManager) SessionValidationCacheStats() SessionValidationCacheStats {
	validator := sm.sessionValidation
	if validator == nil {
		return SessionValidationCacheStats{}
	}
	entries := 0
	if validator.cache != nil {
		entries = validator.cache.Len()
	}
	return SessionValidationCacheStats{
		Enabled:             validator.cache != nil,
		TTL:                 validator.ttl,
		Entries:             entries,
		Hits:                validator.hits.Load(),
		Misses:              validator.misses.Load(),
		DatabaseLoads:       validator.databaseLoads.Load(),
		CoalescedWaiters:    validator.coalescedWaiters.Load(),
		InvalidatedEntries:  validator.invalidatedEntries.Load(),
		EvictedEntries:      validator.evictedEntries.Load(),
		StaleRejections:     validator.staleRejections.Load(),
		CacheDecodeFailures: validator.cacheDecodeFailures.Load(),
	}
}

// ValidateSession validates a session outside an HTTP request. Request-owned
// work should call ValidateSessionContext so cancellation reaches the database.
func (sm *SessionManager) ValidateSession(token, ipAddress string) (*Session, error) {
	return sm.ValidateSessionContext(context.Background(), token, ipAddress)
}

// ValidateSessionContext validates a session token and returns an independent
// session snapshot. Cache keys contain only the SHA-256 token digest.
func (sm *SessionManager) ValidateSessionContext(ctx context.Context, token, ipAddress string) (*Session, error) {
	if token == "" {
		return nil, ErrInvalidSession
	}
	if ctx == nil {
		ctx = context.Background()
	}

	validator := sm.sessionValidation
	if validator == nil {
		validator = newSessionValidator(0, "session_validation")
		sm.sessionValidation = validator
	}
	cacheKey := hashSessionToken(token)

	for {
		lookupEpoch := validator.epoch.Load()
		if session, ok := validator.get(cacheKey); ok {
			if lookupEpoch != validator.epoch.Load() {
				continue
			}
			validator.hits.Add(1)
			return sm.validateSessionSnapshot(token, session, ipAddress)
		}
		validator.misses.Add(1)

		loadEpoch := validator.epoch.Load()
		loadSequence := validator.sequence.Load()
		resultChannel := validator.loads.DoChan(cacheKey, func() (any, error) {
			// A concurrent leader may have populated the cache between this
			// request's first lookup and joining the single-flight group.
			if session, ok := validator.get(cacheKey); ok {
				return &sessionValidationLoad{
					epoch:    loadEpoch,
					sequence: loadSequence,
					cacheKey: cacheKey,
					session:  session,
				}, nil
			}

			validator.databaseLoads.Add(1)
			session, err := sm.loadSessionContext(ctx, token)
			if err != nil {
				return nil, err
			}
			if err := validateSessionState(session); err != nil {
				if errors.Is(err, ErrSessionExpired) {
					_ = sm.DeleteSession(token)
				}
				return nil, err
			}

			// Do not insert a snapshot loaded across a mutation of this token,
			// session, or user. Unrelated session mutations remain cache-local.
			validator.setIfCurrent(cacheKey, session, loadEpoch, loadSequence)
			return &sessionValidationLoad{
				epoch:    loadEpoch,
				sequence: loadSequence,
				cacheKey: cacheKey,
				session:  session,
			}, nil
		})

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-resultChannel:
			if result.Shared {
				validator.coalescedWaiters.Add(1)
			}
			if result.Err != nil {
				return nil, result.Err
			}
			load, ok := result.Val.(*sessionValidationLoad)
			if !ok || load == nil || load.session == nil {
				return nil, ErrInvalidSession
			}
			// A waiter can join a load that started before logout or another
			// security mutation. Retry rather than returning that old snapshot.
			if !validator.loadIsCurrent(load.cacheKey, load.session, load.epoch, load.sequence) {
				validator.staleRejections.Add(1)
				continue
			}
			return sm.validateSessionSnapshot(token, cloneSession(load.session), ipAddress)
		}
	}
}

func (sm *SessionManager) loadSessionContext(ctx context.Context, token string) (*Session, error) {
	query := `
		SELECT
			s.id, s.user_id, s.session_token, s.expires_at, s.ip_address, s.user_agent, s.is_active,
			COALESCE(s.enrollment_required, false),
			COALESCE(s.auth_pending_type, CASE WHEN COALESCE(s.enrollment_required, false) THEN 'passkey_enrollment' ELSE '' END),
			s.created_at,
			u.email, u.username, u.first_name, u.last_name, u.is_active, u.avatar_url, u.requires_password_reset, u.timezone, u.language, u.email_verified, u.created_at, u.updated_at
		FROM user_sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.session_token IN (?, ?) AND s.is_active = true
	`

	session := &Session{User: &models.User{}}
	var avatarURL, timezone, language sql.NullString
	err := sm.db.QueryRowContext(ctx, query, hashSessionToken(token), token).Scan(
		&session.ID, &session.UserID, &session.Token, &session.ExpiresAt, &session.IPAddress, &session.UserAgent, &session.IsActive,
		&session.EnrollmentRequired, &session.AuthPendingType, &session.CreatedAt,
		&session.User.Email, &session.User.Username, &session.User.FirstName, &session.User.LastName, &session.User.IsActive, &avatarURL, &session.User.RequiresPasswordReset, &timezone, &language, &session.User.EmailVerified, &session.User.CreatedAt, &session.User.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to validate session: %w", err)
	}

	session.User.ID = session.UserID
	if avatarURL.Valid {
		session.User.AvatarURL = avatarURL.String
	}
	if timezone.Valid {
		session.User.Timezone = timezone.String
	}
	if language.Valid {
		session.User.Language = language.String
	} else {
		session.User.Language = "en"
	}
	session.User.FullName = fmt.Sprintf("%s %s", session.User.FirstName, session.User.LastName)
	return session, nil
}

func validateSessionState(session *Session) error {
	if session == nil || !session.IsActive || session.User == nil || !session.User.IsActive {
		return ErrInvalidSession
	}
	if !time.Now().Before(session.ExpiresAt) {
		return ErrSessionExpired
	}
	return nil
}

// ipBindingDecision is what a validator should do about the request's client
// IP. The decision is shared by the user-session and portal validators; each
// keeps its own log wording and error value.
type ipBindingDecision int

const (
	// ipBindingMatch: the request IP equals the bound IP — nothing to do.
	ipBindingMatch ipBindingDecision = iota
	// ipBindingSkip: binding disabled; no comparison and no log line.
	ipBindingSkip
	// ipBindingLegacyUnbound: the session predates IP binding (no stored IP).
	ipBindingLegacyUnbound
	// ipBindingRejectNoRequestIP: bound session, no client IP, fail closed.
	ipBindingRejectNoRequestIP
	// ipBindingAcceptNoRequestIP: same situation, reported but served.
	ipBindingAcceptNoRequestIP
	// ipBindingAcceptUnparsedIP: the request carries an IP that is not a valid
	// address, so there is nothing safe to rebind to; reported but served.
	ipBindingAcceptUnparsedIP
	// ipBindingRejectMismatch: the session moved to another IP, fail closed.
	ipBindingRejectMismatch
	// ipBindingRebindMismatch: the session moved; rebind it and serve.
	ipBindingRebindMismatch
)

// decideIPBinding maps a SESSION_IP_BINDING mode plus the stored/request IP
// pair onto one decision.
//
// Any mode other than log or off is treated as strict, so a manager built
// outside config.Load (which validates the env var) keeps the historical
// fail-closed behavior rather than silently loosening it.
func decideIPBinding(mode, sessionIP, requestIP string) ipBindingDecision {
	if mode == config.SessionIPBindingOff {
		return ipBindingSkip
	}
	lenient := mode == config.SessionIPBindingLog

	switch {
	case sessionIP == "":
		return ipBindingLegacyUnbound
	case requestIP == "":
		if lenient {
			return ipBindingAcceptNoRequestIP
		}
		return ipBindingRejectNoRequestIP
	case sessionIP != requestIP:
		if lenient {
			// Only a real address may be written back to the session row. The
			// client-IP extractor returns RemoteAddr verbatim when it does not
			// parse (utils.IPExtractor.GetClientIP), and these validators are
			// exported, so an unparseable value must be reported and served
			// rather than persisted as the session's new binding.
			if net.ParseIP(requestIP) == nil {
				return ipBindingAcceptUnparsedIP
			}
			return ipBindingRebindMismatch
		}
		return ipBindingRejectMismatch
	}
	return ipBindingMatch
}

func (sm *SessionManager) validateSessionSnapshot(token string, session *Session, ipAddress string) (*Session, error) {
	if err := validateSessionState(session); err != nil {
		if errors.Is(err, ErrSessionExpired) {
			_ = sm.DeleteSession(token)
		}
		return nil, err
	}

	if err := sm.applySessionIPBinding(token, session, ipAddress); err != nil {
		return nil, err
	}
	return session, nil
}

// applySessionIPBinding enforces the configured SESSION_IP_BINDING mode. The
// check is deliberately performed for every request, including cache hits.
// Empty stored IPs remain accepted for legacy sessions; under strict an empty
// request IP fails closed for sessions that are bound to an IP, while log
// reports the same situations and serves them. No mode deletes or deactivates
// the session: a rejection fails this request only.
func (sm *SessionManager) applySessionIPBinding(token string, session *Session, ipAddress string) error {
	switch decideIPBinding(sm.ipBinding, session.IPAddress, ipAddress) {
	case ipBindingLegacyUnbound:
		slog.Warn("session has no recorded IP, skipping bind check",
			slog.Int("user_id", session.UserID),
			slog.Int("session_id", session.ID))
	case ipBindingRejectNoRequestIP:
		slog.Warn("request has no client IP, rejecting IP-bound session",
			slog.Int("user_id", session.UserID),
			slog.String("session_ip", session.IPAddress))
		return ErrInvalidSession
	case ipBindingAcceptNoRequestIP:
		slog.Warn("request has no client IP, accepting IP-bound session",
			slog.Int("user_id", session.UserID),
			slog.String("session_ip", session.IPAddress),
			slog.String("session_ip_binding", sm.ipBinding))
	case ipBindingAcceptUnparsedIP:
		slog.Warn("request client IP is not a valid address, accepting IP-bound session without rebinding",
			slog.Int("user_id", session.UserID),
			slog.String("session_ip", session.IPAddress),
			slog.String("request_ip", ipAddress),
			slog.String("session_ip_binding", sm.ipBinding))
	case ipBindingRejectMismatch:
		slog.Warn("session IP mismatch",
			slog.Int("user_id", session.UserID),
			slog.String("session_ip", session.IPAddress),
			slog.String("request_ip", ipAddress))
		return ErrInvalidSession
	case ipBindingRebindMismatch:
		slog.Warn("session IP mismatch",
			slog.Int("user_id", session.UserID),
			slog.String("session_ip", session.IPAddress),
			slog.String("request_ip", ipAddress))
		sm.rebindSessionIP(token, session, ipAddress)
	case ipBindingMatch, ipBindingSkip:
	}
	return nil
}

// rebindSessionIP moves a session to the client IP it is now presented from.
// A failed write costs bookkeeping, not availability: the request is still
// served and the next one retries the rebind. The in-memory snapshot is
// advanced only on success, because it is what the triggering request reports
// back through /api/auth/me.
func (sm *SessionManager) rebindSessionIP(token string, session *Session, ipAddress string) {
	if err := sm.UpdateSessionIP(token, ipAddress); err != nil {
		slog.Error("failed to rebind session to the new client IP",
			slog.Int("user_id", session.UserID),
			slog.Int("session_id", session.ID),
			slog.String("request_ip", ipAddress),
			slog.Any("error", err))
		return
	}
	session.IPAddress = ipAddress
}

func (validator *sessionValidator) get(key string) (*Session, bool) {
	if validator.cache == nil {
		return nil, false
	}
	value, err := validator.cache.Get(key)
	if err != nil {
		return nil, false
	}

	var entry cachedSessionValidation
	if err := json.Unmarshal(value, &entry); err != nil {
		validator.cacheDecodeFailures.Add(1)
		_ = validator.cache.Delete(key)
		return nil, false
	}
	if entry.LoadedAt.IsZero() || time.Since(entry.LoadedAt) >= validator.ttl {
		validator.staleRejections.Add(1)
		_ = validator.cache.Delete(key)
		return nil, false
	}
	if entry.Epoch != validator.epoch.Load() {
		validator.staleRejections.Add(1)
		_ = validator.cache.Delete(key)
		return nil, false
	}
	tokenVersion, sessionVersion, userVersion := validator.versions(key, entry.Session.ID, entry.Session.UserID)
	if entry.TokenVersion != tokenVersion || entry.SessionVersion != sessionVersion || entry.UserVersion != userVersion {
		validator.staleRejections.Add(1)
		_ = validator.cache.Delete(key)
		return nil, false
	}
	entry.Session.AuthPendingType = entry.AuthPendingType
	return cloneSession(&entry.Session), true
}

func (validator *sessionValidator) setIfCurrent(key string, session *Session, epoch, sequence uint64) bool {
	if validator.cache == nil || session == nil || epoch != validator.epoch.Load() {
		return false
	}
	validator.mutationMu.RLock()
	defer validator.mutationMu.RUnlock()
	tokenVersion := validator.tokenVersions[key]
	sessionVersion := validator.sessionVersions[session.ID]
	userVersion := validator.userVersions[session.UserID]
	if tokenVersion > sequence || sessionVersion > sequence || userVersion > sequence || epoch != validator.epoch.Load() {
		return false
	}

	value, err := json.Marshal(cachedSessionValidation{
		LoadedAt:        time.Now(),
		Epoch:           epoch,
		TokenVersion:    tokenVersion,
		SessionVersion:  sessionVersion,
		UserVersion:     userVersion,
		Session:         *cloneSession(session),
		AuthPendingType: session.AuthPendingType,
	})
	if err != nil {
		validator.cacheDecodeFailures.Add(1)
		return false
	}
	if err := validator.cache.Set(key, value); err != nil {
		slog.Warn("failed to store session validation cache entry", slog.Any("error", err))
		return false
	}
	return true
}

func (validator *sessionValidator) versions(cacheKey string, sessionID, userID int) (tokenVersion, sessionVersion, userVersion uint64) {
	validator.mutationMu.RLock()
	defer validator.mutationMu.RUnlock()
	return validator.tokenVersions[cacheKey], validator.sessionVersions[sessionID], validator.userVersions[userID]
}

func (validator *sessionValidator) loadIsCurrent(cacheKey string, session *Session, epoch, sequence uint64) bool {
	if session == nil || epoch != validator.epoch.Load() {
		return false
	}
	tokenVersion, sessionVersion, userVersion := validator.versions(cacheKey, session.ID, session.UserID)
	return tokenVersion <= sequence && sessionVersion <= sequence && userVersion <= sequence
}

func (validator *sessionValidator) recordTokenMutation(cacheKey string) {
	validator.recordMutation(func(version uint64) {
		validator.tokenVersions[cacheKey] = version
	})
}

func (validator *sessionValidator) recordSessionMutation(sessionID int) {
	validator.recordMutation(func(version uint64) {
		validator.sessionVersions[sessionID] = version
	})
}

func (validator *sessionValidator) recordUserMutation(userID int) {
	validator.recordMutation(func(version uint64) {
		validator.userVersions[userID] = version
	})
}

// recordMutation updates one targeted generation. The maps are bounded; if
// unusually high churn fills the journal, a full epoch reset safely rejects
// every in-flight load and starts a fresh journal.
func (validator *sessionValidator) recordMutation(store func(version uint64)) {
	validator.mutationMu.Lock()
	defer validator.mutationMu.Unlock()
	version := validator.sequence.Add(1)
	if len(validator.tokenVersions)+len(validator.sessionVersions)+len(validator.userVersions) >= sessionValidationMutationLimit {
		validator.epoch.Add(1)
		clear(validator.tokenVersions)
		clear(validator.sessionVersions)
		clear(validator.userVersions)
	}
	store(version)
}

func cloneSession(session *Session) *Session {
	if session == nil {
		return nil
	}
	clone := *session
	if session.User != nil {
		user := *session.User
		clone.User = &user
	}
	return &clone
}

func (sm *SessionManager) invalidateSessionValidationToken(token string) {
	validator := sm.sessionValidation
	if validator == nil || token == "" {
		return
	}
	cacheKey := hashSessionToken(token)
	validator.recordTokenMutation(cacheKey)
	validator.loads.Forget(cacheKey)
	if validator.cache == nil {
		return
	}
	if err := validator.cache.Delete(cacheKey); err == nil {
		validator.invalidatedEntries.Add(1)
	}
}

func (sm *SessionManager) invalidateSessionValidationID(sessionID int) {
	validator := sm.sessionValidation
	if validator == nil {
		return
	}
	validator.recordSessionMutation(sessionID)
	sm.deleteSessionValidationMatching(func(session *Session) bool {
		return session.ID == sessionID
	})
}

// InvalidateUserSessionValidation removes every locally cached session for a
// user. Security-sensitive user mutation paths can call this after committing.
func (sm *SessionManager) InvalidateUserSessionValidation(userID int) {
	validator := sm.sessionValidation
	if validator == nil {
		return
	}
	validator.recordUserMutation(userID)
	sm.deleteSessionValidationMatching(func(session *Session) bool {
		return session.UserID == userID
	})
}

func (sm *SessionManager) deleteSessionValidationMatching(match func(*Session) bool) {
	validator := sm.sessionValidation
	if validator == nil {
		return
	}
	if validator.cache == nil {
		return
	}

	keys := make([]string, 0)
	iterator := validator.cache.Iterator()
	for iterator.SetNext() {
		entryInfo, err := iterator.Value()
		if err != nil {
			continue
		}
		var entry cachedSessionValidation
		if err := json.Unmarshal(entryInfo.Value(), &entry); err != nil {
			validator.cacheDecodeFailures.Add(1)
			keys = append(keys, entryInfo.Key())
			continue
		}
		if match(&entry.Session) {
			keys = append(keys, entryInfo.Key())
		}
	}
	for _, key := range keys {
		validator.loads.Forget(key)
		if err := validator.cache.Delete(key); err == nil {
			validator.invalidatedEntries.Add(1)
		}
	}
}
