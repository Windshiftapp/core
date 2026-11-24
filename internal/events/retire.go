package events

import "sync"

// Retired consumers are durable subscriptions replaced by a non-durable
// transport. Their historical deliveries are not replayed: the consumer can no
// longer handle them, so replaying one would rewrite abandonment state as
// success without producing the effect.
var (
	retiredConsumersMu sync.RWMutex
	retiredConsumers   = map[string]struct{}{}
)

// SetRetiredConsumers replaces the set of consumer keys that no longer accept
// operator replay. Startup installs the retired side-effect consumers.
func SetRetiredConsumers(keys []string) {
	retiredConsumersMu.Lock()
	defer retiredConsumersMu.Unlock()
	retiredConsumers = make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key != "" {
			retiredConsumers[key] = struct{}{}
		}
	}
}

// IsRetiredConsumer reports whether a consumer key is retired.
func IsRetiredConsumer(key string) bool {
	retiredConsumersMu.RLock()
	defer retiredConsumersMu.RUnlock()
	_, ok := retiredConsumers[key]
	return ok
}
