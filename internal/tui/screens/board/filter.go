package board

import (
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"

	"windshift/internal/tui/data"
)

// Filter is the board's instant filter: case-insensitive substring over
// key + title + assignee, with a fuzzy fallback when the substring misses.
// It prunes items without changing grouping.
type Filter struct {
	Query string
}

func (f Filter) Active() bool { return strings.TrimSpace(f.Query) != "" }

// Match reports whether the item survives the filter. wsKey supplies the
// workspace prefix for "WI-123"-style queries when the item doesn't carry
// its own key.
func (f Filter) Match(it *data.WorkItem, wsKey string) bool {
	if !f.Active() {
		return true
	}
	q := strings.ToLower(strings.TrimSpace(f.Query))

	hay := strings.ToLower(fmt.Sprintf("%s %s %s", it.DisplayKey(wsKey), it.Title, it.AssigneeName))

	if strings.Contains(hay, q) {
		return true
	}
	return len(fuzzy.Find(q, []string{hay})) > 0
}
