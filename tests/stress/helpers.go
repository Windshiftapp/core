// Package stress provides helper utilities for stress testing.
package stress

import (
	"time"
)

// averageDuration calculates the average of a slice of durations
//
//nolint:unused // used by test files which are not scanned by the linter
func averageDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}
