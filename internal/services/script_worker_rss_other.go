//go:build !linux

package services

// processRSSBytes is unavailable off Linux. The worker's own heap monitor still
// stops a runaway allocation and reports an explainable failure; the parent
// simply cannot add the RSS backstop.
func processRSSBytes(int) (int64, error) { return 0, nil }
