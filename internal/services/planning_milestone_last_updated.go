package services

import (
	"strings"
	"time"
)

// Last-updated time of a listed milestone: the latest change to the
// milestone or anything in it. The milestone list query reads one MAX column
// per source next to the milestone's own updated_at, and Go picks the latest.
// Comparing in Go avoids GREATEST (Postgres) versus scalar MAX (SQLite) and
// the mixed text layouts SQLite stores timestamps in.
//
// Sources, besides the milestone's own updated_at:
//   - milestone_history rows (description, status, and target-date changes)
//   - milestone comments, including edits
//   - page links (item_links rows with source_type 'milestone')
//   - member items' updated_at (item_milestones)
//   - comments on member items, including edits: creating a comment only
//     moves the item's last_active_at, not its updated_at
//   - items added to the milestone (item_milestones.created_at)
//
// Every source is an indexed lookup by milestone or item. Removing an item
// does not count: finding removals means matching the milestone ID inside
// every item_history 'milestones' value, a scan that grows with the whole
// history on each list request.
//
// Item sources count only items in the viewer's accessible workspaces, the
// rule the Activity tab and the progress view apply. With no accessible
// workspace those columns are NULL.
func milestoneLastUpdatedColumns(workspaceIDs []int) (columns string, args []any) {
	columns = `,
	       (SELECT MAX(mh.changed_at) FROM milestone_history mh WHERE mh.milestone_id = m.id),
	       (SELECT MAX(mcm.updated_at) FROM milestone_comments mcm WHERE mcm.milestone_id = m.id),
	       (SELECT MAX(il.created_at) FROM item_links il
	         WHERE il.source_type = 'milestone' AND il.target_type = 'page' AND il.source_id = m.id)`
	if len(workspaceIDs) == 0 {
		return columns + `, NULL, NULL, NULL`, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(workspaceIDs)), ",")
	columns += `,
	       (SELECT MAX(lui.updated_at) FROM item_milestones luim
	         JOIN items lui ON lui.id = luim.item_id
	         WHERE luim.milestone_id = m.id AND lui.workspace_id IN (` + placeholders + `)),
	       (SELECT MAX(luc.updated_at) FROM comments luc
	         JOIN item_milestones luim ON luim.item_id = luc.item_id
	         JOIN items lui ON lui.id = luc.item_id
	         WHERE luim.milestone_id = m.id AND lui.workspace_id IN (` + placeholders + `)),
	       (SELECT MAX(luim.created_at) FROM item_milestones luim
	         JOIN items lui ON lui.id = luim.item_id
	         WHERE luim.milestone_id = m.id AND lui.workspace_id IN (` + placeholders + `))`
	for range 3 {
		for _, id := range workspaceIDs {
			args = append(args, id)
		}
	}
	return columns, args
}

// milestoneLastUpdatedSourceCount is the number of columns
// milestoneLastUpdatedColumns adds.
const milestoneLastUpdatedSourceCount = 6

// latestMilestoneUpdate returns the latest of updatedAt and the scanned source
// values. Postgres scans MAX(timestamp) as time.Time; SQLite returns the
// stored text. NULL and unparsable values are skipped.
func latestMilestoneUpdate(updatedAt time.Time, sources []any) time.Time {
	latest := updatedAt
	for _, source := range sources {
		if text, ok := source.(string); ok {
			source = stripMonotonicClock(text)
		}
		if at, ok := analyticsDBTime(source); ok && at.After(latest) {
			latest = at
		}
	}
	return latest.UTC()
}

// stripMonotonicClock drops the " m=+…" suffix of time.Time.String() text,
// which no time layout parses.
func stripMonotonicClock(text string) string {
	if idx := strings.Index(text, " m="); idx >= 0 {
		return text[:idx]
	}
	return text
}
