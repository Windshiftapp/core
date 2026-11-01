package ratelimit

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"windshift/internal/contextkeys"
)

// E2EDisableEnv turns every limiter into a passthrough when set to "1". The
// e2e harness sets it so single-worker runs are not tripped by shared limiter
// budgets; production must never set it.
const E2EDisableEnv = "WINDSHIFT_E2E_DISABLE_RATE_LIMITS"

// E2EDisabled reports whether rate limiting is disabled for this process.
// os.Getenv takes a shared read lock, so concurrent limiter hits do not
// serialize on it.
func E2EDisabled() bool { return os.Getenv(E2EDisableEnv) == "1" }

const (
	defaultShards     = 64
	defaultMaxEntries = 100_000
	defaultIdleTTL    = 10 * time.Minute
	minSweepInterval  = time.Minute
)

// Limiter is a token-bucket rate limiter keyed by a fixed-size Key. It is safe
// for concurrent use.
type Limiter struct {
	rate  rate.Limit
	burst int
	key   KeyFunc

	shards     []shard
	mask       uint64
	perShard   int
	idleTTL    time.Duration
	sweepEvery time.Duration

	stopOnce sync.Once
	stopCh   chan struct{}
}

type shard struct {
	mu      sync.RWMutex
	buckets map[Key]*bucket
}

type bucket struct {
	limiter *rate.Limiter
	// lastSeen is Unix nanoseconds, updated atomically so the allow path never
	// needs a lock to refresh it.
	lastSeen atomic.Int64
}

// Option configures a Limiter.
type Option func(*Limiter)

// WithIdleTTL sets how long an unused bucket survives before the sweeper drops
// it.
func WithIdleTTL(d time.Duration) Option {
	return func(l *Limiter) {
		if d > 0 {
			l.idleTTL = d
		}
	}
}

// WithMaxEntries bounds the total number of buckets across all shards.
func WithMaxEntries(n int) Option {
	return func(l *Limiter) {
		if n > 0 {
			l.perShard = max(1, n/len(l.shards))
		}
	}
}

// WithShards sets the number of shards. Rounded up to a power of two.
func WithShards(n int) Option {
	return func(l *Limiter) {
		if n <= 0 {
			return
		}
		shards := 1
		for shards < n {
			shards <<= 1
		}
		l.shards = make([]shard, shards)
		for i := range l.shards {
			l.shards[i].buckets = make(map[Key]*bucket)
		}
		l.mask = uint64(shards - 1) //nolint:gosec // G115: shards is a small power of two
	}
}

// New builds a limiter with the given steady rate (per second), burst, and key
// function. A nil key function makes every request unlimited.
func New(rps float64, burst int, key KeyFunc, opts ...Option) *Limiter {
	if burst < 1 {
		burst = 1
	}
	l := &Limiter{
		rate:       rate.Limit(rps),
		burst:      burst,
		key:        key,
		shards:     make([]shard, defaultShards),
		mask:       defaultShards - 1,
		perShard:   max(1, defaultMaxEntries/defaultShards),
		idleTTL:    defaultIdleTTL,
		sweepEvery: defaultIdleTTL / 2,
	}
	for i := range l.shards {
		l.shards[i].buckets = make(map[Key]*bucket)
	}
	for _, opt := range opts {
		opt(l)
	}
	if l.sweepEvery < minSweepInterval {
		l.sweepEvery = minSweepInterval
	}
	l.stopCh = make(chan struct{})
	go l.sweepLoop()
	return l
}

// Result describes one limiter decision. The header fields are populated on
// both allowed and denied requests so callers can surface them consistently.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	Reset      time.Time
	RetryAfter time.Duration
}

// HeaderValues returns the X-RateLimit-* headers describing this result.
func (r Result) HeaderValues() http.Header {
	header := http.Header{}
	if r.Limit <= 0 {
		return header
	}
	header.Set("X-RateLimit-Limit", strconv.Itoa(r.Limit))
	header.Set("X-RateLimit-Remaining", strconv.Itoa(r.Remaining))
	if !r.Reset.IsZero() {
		header.Set("X-RateLimit-Reset", strconv.FormatInt(r.Reset.Unix(), 10))
	}
	return header
}

// RetryAfterSeconds returns the Retry-After value for this result, at least 1.
func (r Result) RetryAfterSeconds() int { return retryAfterSeconds(r.RetryAfter) }

// Check consumes one token for the key and reports the resulting state.
func (l *Limiter) Check(key Key) Result {
	if l == nil {
		return Result{Allowed: true}
	}
	now := time.Now()
	b := l.bucket(key, now)

	// Allow + Tokens are both allocation-free; Reserve/Cancel is not, and this
	// runs on every request.
	allowed := b.limiter.Allow()
	tokens := b.limiter.Tokens()

	result := Result{Limit: l.burst, Remaining: int(tokens), Reset: now}
	if allowed {
		result.Allowed = true
		if l.rate > 0 && tokens < float64(l.burst) {
			result.Reset = now.Add(time.Duration((float64(l.burst) - tokens) / float64(l.rate) * float64(time.Second)))
		}
		return result
	}
	if l.rate > 0 {
		missing := 1 - tokens
		if missing < 0 {
			missing = 0
		}
		result.RetryAfter = time.Duration(missing / float64(l.rate) * float64(time.Second))
	} else {
		result.RetryAfter = time.Second
	}
	return result
}

// Allow reports whether the key may proceed.
func (l *Limiter) Allow(key Key) bool {
	if l == nil || E2EDisabled() {
		return true
	}
	return l.Check(key).Allowed
}

// Decide resolves the request key and returns the limiter decision. ok=false
// means the key function declined the request, so it is unlimited.
func (l *Limiter) Decide(r *http.Request) (Result, bool) {
	if l == nil || E2EDisabled() || l.key == nil {
		return Result{Allowed: true}, false
	}
	key, ok := l.key(r)
	if !ok {
		return Result{Allowed: true}, false
	}
	return l.Check(key), true
}

// AllowRequest resolves the request key and reports whether it may proceed.
// Requests the key function declines (for example anonymous traffic on a
// user-keyed limiter) are always allowed.
func (l *Limiter) AllowRequest(r *http.Request) bool {
	result, ok := l.Decide(r)
	return !ok || result.Allowed
}

// Limit wraps a handler with this limiter. Denied requests get a JSON 429 with
// Retry-After; every limited response carries X-RateLimit-* headers.
func (l *Limiter) Limit(next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if E2EDisabled() || l.key == nil {
			next.ServeHTTP(w, r)
			return
		}
		key, ok := l.key(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		result := l.Check(key)
		setRateLimitHeaders(w, result)
		if !result.Allowed {
			writeRateLimited(w, r, result.RetryAfter)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Stop halts the background sweeper. Safe to call multiple times.
func (l *Limiter) Stop() {
	if l == nil {
		return
	}
	l.stopOnce.Do(func() {
		if l.stopCh != nil {
			close(l.stopCh)
		}
	})
}

func (l *Limiter) sweepLoop() {
	ticker := time.NewTicker(l.sweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.sweep(time.Now())
		case <-l.stopCh:
			return
		}
	}
}

func (l *Limiter) sweep(now time.Time) {
	idleBefore := now.Add(-l.idleTTL).UnixNano()
	for i := range l.shards {
		sh := &l.shards[i]
		sh.mu.Lock()
		for key, b := range sh.buckets {
			if b.lastSeen.Load() < idleBefore {
				delete(sh.buckets, key)
			}
		}
		sh.mu.Unlock()
	}
}

func (l *Limiter) bucket(key Key, now time.Time) *bucket {
	sh := &l.shards[key.hash()&l.mask]

	sh.mu.RLock()
	b := sh.buckets[key]
	sh.mu.RUnlock()
	if b != nil {
		b.lastSeen.Store(now.UnixNano())
		return b
	}

	sh.mu.Lock()
	defer sh.mu.Unlock()
	if b = sh.buckets[key]; b != nil {
		b.lastSeen.Store(now.UnixNano())
		return b
	}
	if len(sh.buckets) >= l.perShard {
		l.makeRoom(sh.buckets, now)
	}
	b = &bucket{limiter: rate.NewLimiter(l.rate, l.burst)}
	b.lastSeen.Store(now.UnixNano())
	sh.buckets[key] = b
	return b
}

// makeRoom drops idle buckets and, if the shard is still full, its least
// recently seen bucket. Called with the shard write lock held.
func (l *Limiter) makeRoom(buckets map[Key]*bucket, now time.Time) {
	idleBefore := now.Add(-l.idleTTL).UnixNano()
	var oldestKey Key
	oldest := int64(math.MaxInt64)
	found := false
	for key, b := range buckets {
		seen := b.lastSeen.Load()
		if seen < idleBefore {
			delete(buckets, key)
			continue
		}
		if seen < oldest {
			oldest, oldestKey, found = seen, key, true
		}
	}
	if found && len(buckets) >= l.perShard {
		delete(buckets, oldestKey)
	}
}

func setRateLimitHeaders(w http.ResponseWriter, result Result) {
	for name, values := range result.HeaderValues() {
		for _, value := range values {
			w.Header().Set(name, value)
		}
	}
}

func writeRateLimited(w http.ResponseWriter, r *http.Request, retryAfter time.Duration) {
	requestID, _ := r.Context().Value(contextkeys.RequestID).(string)
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter)))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(struct {
		Error     string `json:"error"`
		Code      string `json:"code"`
		RequestID string `json:"request_id,omitempty"`
	}{
		Error:     "Rate limit exceeded",
		Code:      "RATE_LIMITED",
		RequestID: requestID,
	})
}

func retryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	seconds := int(math.Ceil(d.Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}
