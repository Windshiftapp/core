package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseDuration parses friendly time duration strings into a time.Duration.
//
// Supported formats:
//   - "1h", "0.5h"     hours (fractional ok)
//   - "30m", "15m"     minutes (fractional ok)
//   - "1h30m"          combined hours + minutes
//   - "1d", "2d"       days (1d = 8 hours)
//
// Returns an error if the input is empty or doesn't match.
func ParseDuration(input string) (time.Duration, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if strings.HasSuffix(input, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(input, "d"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid day format: %s", input)
		}
		return time.Duration(days * 8 * float64(time.Hour)), nil
	}

	re := regexp.MustCompile(`(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m)?`)
	matches := re.FindStringSubmatch(input)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid duration format: %s", input)
	}

	var total time.Duration
	if matches[1] != "" {
		hours, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hour format: %s", matches[1])
		}
		total += time.Duration(hours * float64(time.Hour))
	}
	if matches[2] != "" {
		minutes, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minute format: %s", matches[2])
		}
		total += time.Duration(minutes * float64(time.Minute))
	}
	if total == 0 {
		return 0, fmt.Errorf("no time duration found in: %s", input)
	}
	return total, nil
}
