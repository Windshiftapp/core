//go:build linux

package services

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// processRSSBytes reads the resident set size of a process from procfs. It is
// the parent-side backstop that bounds a runaway worker's real memory, not just
// its Go heap.
func processRSSBytes(pid int) (int64, error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected statm format for pid %d", pid)
	}
	residentPages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return residentPages * int64(os.Getpagesize()), nil
}
