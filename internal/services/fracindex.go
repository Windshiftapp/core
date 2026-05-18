package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lib/pq"

	"windshift/internal/database"
)

// fracIndexMaxRetries caps the number of unique-violation retries on the
// item INSERT / reorder UPDATE paths. The retry path only fires when the
// in-memory cache has drifted from the DB max (or, on Postgres, when the
// column collation differs from the algorithm's byte ordering — see the
// admin diagnostics frac-index panel for live detection).
const fracIndexMaxRetries = 5

// IsFracIndexUniqueViolation reports whether err is specifically a
// UNIQUE-constraint violation on idx_items_frac_index. Other unique
// violations (e.g. workspace_item_number) must not trigger the retry,
// so a generic check would be too broad. Exported for use by handlers
// that wrap reorder writes in their own retry loop.
func IsFracIndexUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" &&
			(pqErr.Constraint == "idx_items_frac_index" ||
				strings.Contains(pqErr.Message, "idx_items_frac_index"))
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed: items.frac_index")
}

// Fractional indexing implementation based on https://github.com/rocicorp/fracdex
// This provides lexicographically sortable keys for ordering items

const base62Digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const smallestInt = "A00000000000000000000000000"
const zero = "a0"

// fracIndexCache provides in-memory caching for the last frac_index to avoid
// expensive global table scans on every item creation.
var (
	fracIndexCache      atomic.Value // stores *string (last known frac_index)
	fracIndexCacheMutex sync.Mutex   // protects cache initialization
	fracIndexCacheHits  int64        // counter for monitoring
	fracIndexCacheMiss  int64        // counter for monitoring
)

// KeyBetween returns a key that sorts lexicographically between a and b.
// Either a or b can be empty strings. If a is empty it indicates smallest key,
// If b is empty it indicates largest key.
// b must be empty string or > a.
func KeyBetween(a, b string) (string, error) {
	if a != "" {
		err := validateOrderKey(a)
		if err != nil {
			return "", err
		}
	}
	if b != "" {
		err := validateOrderKey(b)
		if err != nil {
			return "", err
		}
	}
	if a != "" && b != "" && a >= b {
		return "", fmt.Errorf("%s >= %s", a, b)
	}
	if a == "" {
		if b == "" {
			return zero, nil
		}

		ib, err := getIntPart(b)
		if err != nil {
			return "", err
		}
		fb := b[len(ib):]
		if ib == smallestInt {
			return ib + midpoint("", fb), nil
		}
		if ib < b {
			return ib, nil
		}
		res, err := decrementInt(ib)
		if err != nil {
			return "", err
		}
		if res == "" {
			return "", errors.New("range underflow")
		}
		return res, nil
	}

	if b == "" {
		ia, err := getIntPart(a)
		if err != nil {
			return "", err
		}
		fa := a[len(ia):]
		i, err := incrementInt(ia)
		if err != nil {
			return "", err
		}
		if i == "" {
			return ia + midpoint(fa, ""), nil
		}
		return i, nil
	}

	ia, err := getIntPart(a)
	if err != nil {
		return "", err
	}
	fa := a[len(ia):]
	ib, err := getIntPart(b)
	if err != nil {
		return "", err
	}
	fb := b[len(ib):]
	if ia == ib {
		return ia + midpoint(fa, fb), nil
	}
	i, err := incrementInt(ia)
	if err != nil {
		return "", err
	}
	if i == "" {
		return "", errors.New("range overflow")
	}
	if i < b {
		return i, nil
	}
	return ia + midpoint(fa, ""), nil
}

// `a < b` lexicographically if `b` is non-empty.
// a == "" means first possible string.
// b == "" means last possible string.
func midpoint(a, b string) string {
	if b != "" {
		// remove longest common prefix.  pad `a` with 0s as we
		// go.  note that we don't need to pad `b`, because it can't
		// end before `a` while traversing the common prefix.
		i := 0
		for ; i < len(b); i++ {
			c := byte('0')
			if len(a) > i {
				c = a[i]
			}
			if c != b[i] {
				break
			}
		}
		if i > 0 {
			if i > len(a) {
				return b[0:i] + midpoint("", b[i:])
			}
			return b[0:i] + midpoint(a[i:], b[i:])
		}
	}

	// first digits (or lack of digit) are different
	digitA := 0
	if a != "" {
		digitA = strings.Index(base62Digits, string(a[0]))
	}
	digitB := len(base62Digits)
	if b != "" {
		digitB = strings.Index(base62Digits, string(b[0]))
	}
	if digitB-digitA > 1 {
		midDigit := int(math.Round(0.5 * float64(digitA+digitB)))
		return string(base62Digits[midDigit])
	}

	// first digits are consecutive
	if len(b) > 1 {
		return b[0:1]
	}

	// `b` is empty or has length 1 (a single digit).
	// the first digit of `a` is the previous digit to `b`,
	// or 9 if `b` is null.
	// given, for example, midpoint('49', '5'), return
	// '4' + midpoint('9', null), which will become
	// '4' + '9' + midpoint('', null), which is '495'
	sa := ""
	if a != "" {
		sa = a[1:]
	}
	return string(base62Digits[digitA]) + midpoint(sa, "")
}

func validateInt(i string) error {
	exp, err := getIntLen(i[0])
	if err != nil {
		return err
	}
	if len(i) != exp {
		return fmt.Errorf("invalid integer part of order key: %s", i)
	}
	return nil
}

func getIntLen(head byte) (int, error) {
	switch {
	case head >= 'a' && head <= 'z':
		return int(head - 'a' + 2), nil
	case head >= 'A' && head <= 'Z':
		return int('Z' - head + 2), nil
	default:
		return 0, fmt.Errorf("invalid order key head: %s", string(head))
	}
}

func getIntPart(key string) (string, error) {
	intPartLen, err := getIntLen(key[0])
	if err != nil {
		return "", err
	}
	if intPartLen > len(key) {
		return "", fmt.Errorf("invalid order key: %s", key)
	}
	return key[0:intPartLen], nil
}

func validateOrderKey(key string) error {
	if key == smallestInt {
		return fmt.Errorf("invalid order key: %s", key)
	}
	// getIntPart will return error if the first character is bad,
	// or the key is too short.  we'd call it to check these things
	// even if we didn't need the result
	i, err := getIntPart(key)
	if err != nil {
		return err
	}
	f := key[len(i):]
	if strings.HasSuffix(f, "0") {
		return fmt.Errorf("invalid order key: %s", key)
	}
	return nil
}

// returns error if x is invalid, or if range is exceeded
func incrementInt(x string) (string, error) {
	err := validateInt(x)
	if err != nil {
		return "", err
	}
	digs := strings.Split(x, "")
	head := digs[0]
	digs = digs[1:]
	carry := true
	for i := len(digs) - 1; carry && i >= 0; i-- {
		d := strings.Index(base62Digits, digs[i]) + 1
		if d == len(base62Digits) {
			digs[i] = "0"
		} else {
			digs[i] = string(base62Digits[d])
			carry = false
		}
	}
	if carry {
		if head == "Z" {
			return "a0", nil
		}
		if head == "z" {
			return "", nil
		}
		h := string(head[0] + 1)
		if h > "a" {
			digs = append(digs, "0")
		} else {
			digs = digs[1:]
		}
		return h + strings.Join(digs, ""), nil
	}
	return head + strings.Join(digs, ""), nil
}

func decrementInt(x string) (string, error) {
	err := validateInt(x)
	if err != nil {
		return "", err
	}
	digs := strings.Split(x, "")
	head := digs[0]
	digs = digs[1:]
	borrow := true
	for i := len(digs) - 1; borrow && i >= 0; i-- {
		d := strings.Index(base62Digits, digs[i]) - 1
		if d == -1 {
			digs[i] = string(base62Digits[len(base62Digits)-1])
		} else {
			digs[i] = string(base62Digits[d])
			borrow = false
		}
	}

	if borrow {
		if head == "a" {
			return "Z" + string(base62Digits[len(base62Digits)-1]), nil
		}
		if head == "A" {
			return "", nil
		}
		h := head[0] - 1
		if h < 'Z' {
			digs = append(digs, string(base62Digits[len(base62Digits)-1]))
		} else {
			digs = digs[1:]
		}
		return string(h) + strings.Join(digs, ""), nil
	}

	return head + strings.Join(digs, ""), nil
}

// ===== Integration functions for the windshift application =====

// GenerateFracIndexForNewItem generates a fractional index for a new item at the end of a list.
// It uses an in-memory cache to avoid expensive global table scans on every insert.
// The entire read-compute-store is serialized under a mutex so that two concurrent
// creators cannot derive the same key from the same cached value.
// Note: frac_index is globally unique across all workspaces to allow cross-instance ranking.
func GenerateFracIndexForNewItem(db database.Database, workspaceID int, parentID *int) (string, error) {
	fracIndexCacheMutex.Lock()
	defer fracIndexCacheMutex.Unlock()

	// Try cached value first
	if cached := fracIndexCache.Load(); cached != nil {
		lastIndex, _ := cached.(*string)
		if lastIndex != nil {
			newIndex, err := KeyBetween(*lastIndex, "")
			if err == nil {
				fracIndexCache.Store(&newIndex)
				atomic.AddInt64(&fracIndexCacheHits, 1)
				return newIndex, nil
			}
			slog.Warn("KeyBetween failed for cached value, falling back to DB",
				slog.String("component", "fracindex"),
				slog.String("cached_value", *lastIndex),
				slog.Any("error", err))
		}
	}

	atomic.AddInt64(&fracIndexCacheMiss, 1)

	// Query to get the last item's frac_index globally
	var lastIndex *string
	query := `
		SELECT frac_index
		FROM items
		WHERE frac_index IS NOT NULL
		ORDER BY frac_index DESC
		LIMIT 1
	`

	err := db.QueryRow(query).Scan(&lastIndex)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to get last frac_index: %w", err)
	}

	var newIndex string
	if lastIndex == nil {
		newIndex, err = KeyBetween("", "")
	} else {
		newIndex, err = KeyBetween(*lastIndex, "")
	}
	if err != nil {
		return "", fmt.Errorf("failed to generate frac_index: %w", err)
	}

	fracIndexCache.Store(&newIndex)
	return newIndex, nil
}

// MaybeAdvanceFracIndexCache updates the cache when newIndex is lexicographically
// greater than the currently cached value. This must be called by any code path
// that persists a frac_index outside of GenerateFracIndexForNewItem (notably the
// reorder endpoint), otherwise the cache can lag behind the true maximum and
// subsequent calls to GenerateFracIndexForNewItem would produce duplicate keys.
func MaybeAdvanceFracIndexCache(newIndex string) {
	if newIndex == "" {
		return
	}
	fracIndexCacheMutex.Lock()
	defer fracIndexCacheMutex.Unlock()
	if cached := fracIndexCache.Load(); cached != nil {
		lastIndex, _ := cached.(*string)
		if lastIndex != nil && newIndex <= *lastIndex {
			return
		}
	}
	fracIndexCache.Store(&newIndex)
}

// InvalidateFracIndexCache clears the cache (useful for testing or after bulk deletes)
// deadcode-keep: called by core-tests/internal/services/fracindex_test.go and tests/helpers.go
func InvalidateFracIndexCache() {
	fracIndexCache.Store((*string)(nil))
}

// UpdateItemFracIndex updates the frac_index of an item and advances the
// generator cache so subsequent creates don't reuse the persisted key.
// Callers must NOT skip this function and write frac_index by hand — the
// cache coherence guarantee depends on every external persist going through
// here.
func UpdateItemFracIndex(db database.Database, itemID int, fracIndex string) error {
	query := "UPDATE items SET frac_index = ? WHERE id = ?"
	_, err := db.Exec(query, fracIndex, itemID)
	if err != nil {
		return fmt.Errorf("failed to update frac_index: %w", err)
	}
	MaybeAdvanceFracIndexCache(fracIndex)
	return nil
}

// FracIndexCacheStats describes the in-process generator cache for the admin
// diagnostics panel. It is a read-only snapshot.
type FracIndexCacheStats struct {
	Cached      *string `json:"cached"`                  // current cached "last" key; nil if uninitialized
	NextWouldBe *string `json:"next_would_be,omitempty"` // KeyBetween(cached, "") preview
	NextError   string  `json:"next_error,omitempty"`
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
}

// GetFracIndexCacheStats returns a snapshot of the generator cache state.
// It briefly takes the cache mutex; contention is negligible because
// readers are diagnostic-only.
func GetFracIndexCacheStats() FracIndexCacheStats {
	fracIndexCacheMutex.Lock()
	defer fracIndexCacheMutex.Unlock()

	var cached *string
	if v := fracIndexCache.Load(); v != nil {
		if s, ok := v.(*string); ok && s != nil {
			c := *s
			cached = &c
		}
	}
	out := FracIndexCacheStats{
		Cached: cached,
		Hits:   atomic.LoadInt64(&fracIndexCacheHits),
		Misses: atomic.LoadInt64(&fracIndexCacheMiss),
	}
	if cached != nil {
		next, err := KeyBetween(*cached, "")
		if err != nil {
			out.NextError = err.Error()
		} else {
			out.NextWouldBe = &next
		}
	}
	return out
}
