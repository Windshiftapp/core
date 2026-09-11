package board

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"windshift/internal/tui/core"
	"windshift/internal/tui/data"
)

// listPane renders the grouped item rows and owns cursor + scroll state.
// Scrolling is a plain offset over the flattened rows — no viewport
// component, so there is nothing to nil-deref before sizing.
type listPane struct {
	ctx *core.Ctx

	rows   []Row
	cursor int // index into rows; always sits on a RowItem when possible
	offset int // first visible row

	width  int
	height int
}

func newListPane(ctx *core.Ctx) *listPane {
	return &listPane{ctx: ctx}
}

func (l *listPane) setSize(w, h int) {
	l.width = w
	l.height = h
	l.ensureVisible()
}

// setRows swaps the flattened rows, trying to keep the cursor on the same
// item id.
func (l *listPane) setRows(rows []Row) {
	prevID := 0
	if it := l.selectedItem(); it != nil {
		prevID = it.ID
	}
	l.rows = rows
	if prevID != 0 {
		for i, r := range rows {
			if r.Kind == RowItem && r.Item.ID == prevID {
				l.cursor = i
				l.ensureVisible()
				return
			}
		}
	}
	l.cursor = 0
	l.snapToItem(1)
	l.ensureVisible()
}

// selectedItem returns the item under the cursor, nil when on a header or
// the list is empty.
func (l *listPane) selectedItem() *data.WorkItem {
	if l.cursor < 0 || l.cursor >= len(l.rows) {
		return nil
	}
	r := l.rows[l.cursor]
	if r.Kind != RowItem {
		return nil
	}
	return r.Item
}

// selectedGroupKey returns the group the cursor is in (header or item).
func (l *listPane) selectedGroupKey() string {
	if l.cursor < 0 || l.cursor >= len(l.rows) {
		return ""
	}
	return l.rows[l.cursor].GroupKey
}

// move advances the cursor by delta, skipping group headers (they are
// selectable only when their group is collapsed, so collapsing never
// strands the cursor).
func (l *listPane) move(delta int) {
	if len(l.rows) == 0 {
		return
	}
	i := l.cursor
	for {
		i += delta
		if i < 0 || i >= len(l.rows) {
			return // stop at the ends, no wrap in a grouped list
		}
		if l.selectable(i) {
			l.cursor = i
			l.ensureVisible()
			return
		}
	}
}

func (l *listPane) selectable(i int) bool {
	r := l.rows[i]
	return r.Kind == RowItem || (r.Kind == RowHeader && r.Collapsed)
}

// snapToItem puts the cursor on the nearest selectable row in direction dir
// (used after rebuilds when the cursor may sit on a header).
func (l *listPane) snapToItem(dir int) {
	if len(l.rows) == 0 {
		l.cursor = 0
		return
	}
	if l.cursor >= len(l.rows) {
		l.cursor = len(l.rows) - 1
	}
	if l.selectable(l.cursor) {
		return
	}
	i := l.cursor
	for i >= 0 && i < len(l.rows) {
		if l.selectable(i) {
			l.cursor = i
			return
		}
		i += dir
	}
	// Fell off one end — try the other direction.
	i = l.cursor
	for i >= 0 && i < len(l.rows) {
		if l.selectable(i) {
			l.cursor = i
			return
		}
		i -= dir
	}
}

// jumpGroup moves the cursor to the first selectable row of the next (+1) or
// previous (-1) group.
func (l *listPane) jumpGroup(dir int) {
	if len(l.rows) == 0 {
		return
	}
	cur := l.selectedGroupKey()
	i := l.cursor
	for {
		i += dir
		if i < 0 || i >= len(l.rows) {
			return
		}
		if l.rows[i].GroupKey != cur {
			// Land on the first selectable row inside that group.
			target := l.rows[i].GroupKey
			for j := i; j >= 0 && j < len(l.rows) && l.rows[j].GroupKey == target; j += dir {
				i = j
			}
			if dir < 0 {
				// Walked to the group's header end; re-walk forward to its
				// first selectable row.
				for j := i; j < len(l.rows) && l.rows[j].GroupKey == target; j++ {
					if l.selectable(j) {
						l.cursor = j
						l.ensureVisible()
						return
					}
				}
				return
			}
			for j := i; j < len(l.rows) && l.rows[j].GroupKey == target; j++ {
				if l.selectable(j) {
					l.cursor = j
					l.ensureVisible()
					return
				}
			}
			return
		}
	}
}

// halfPage moves the cursor half a pane in dir.
func (l *listPane) halfPage(dir int) {
	steps := l.height / 2
	if steps < 1 {
		steps = 1
	}
	for range steps {
		before := l.cursor
		l.move(dir)
		if l.cursor == before {
			break
		}
	}
}

func (l *listPane) ensureVisible() {
	if l.height <= 0 {
		return
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+l.height {
		l.offset = l.cursor - l.height + 1
	}
	if l.offset < 0 {
		l.offset = 0
	}
}

// view renders the visible slice of rows into a width×height block.
func (l *listPane) view() string {
	s := l.ctx.Styles
	if len(l.rows) == 0 {
		return s.List.Empty.Render("No work items yet. Press 'n' to create one.")
	}

	end := l.offset + l.height
	if end > len(l.rows) {
		end = len(l.rows)
	}

	lines := make([]string, 0, l.height)
	for i := l.offset; i < end; i++ {
		lines = append(lines, l.renderRow(i))
	}
	return strings.Join(lines, "\n")
}

func (l *listPane) renderRow(i int) string {
	s := l.ctx.Styles
	r := l.rows[i]
	selected := i == l.cursor

	if r.Kind == RowHeader {
		arrow := "▾"
		if r.Collapsed {
			arrow = "▸"
		}
		count := fmt.Sprintf("(%d)", r.Count)
		if r.Shown != r.Count {
			count = fmt.Sprintf("(%d/%d)", r.Shown, r.Count)
		}
		// Category-colored header: the dot + name carry the workspace's own
		// category color; arrow and count stay muted chrome.
		nameStyle := s.List.Header
		dot := ""
		if r.GroupColor != "" {
			c := lipgloss.Color(r.GroupColor)
			nameStyle = lipgloss.NewStyle().Foreground(c).Bold(true)
			dot = lipgloss.NewStyle().Foreground(c).Render("●") + " "
		}
		text := s.List.Muted.Render(arrow+" ") + dot +
			nameStyle.Render(r.GroupName) + " " +
			s.List.Counter.Render(count)
		var line string
		if selected {
			line = s.List.SelBar.Render("▎") + " " + text
		} else {
			line = "  " + text
		}
		return lipgloss.NewStyle().MaxWidth(l.width).Render(line)
	}

	it := r.Item
	key := ""
	if key == "" && l.ctx.Workspace != nil {
		key = l.ctx.Workspace.Key
	}
	itemKey := it.DisplayKey(key)

	mine := ""
	if l.ctx.User != nil && it.AssigneeID != nil && *it.AssigneeID == l.ctx.User.UserID {
		mine = " " + s.List.SelBar.Render("●")
	} else if it.AssigneeName != "" {
		mine = " " + s.List.Muted.Render(initials(it.AssigneeName))
	}

	// Budget: gutter (2) + style padding (2) + key + space + mine-marker.
	room := l.width - 4 - len(itemKey) - 1 - lipgloss.Width(mine)
	if room < 4 {
		room = 4
	}
	title := it.Title
	if runes := []rune(title); len(runes) > room {
		title = string(runes[:room-1]) + "…"
	}

	text := s.List.Muted.Render(itemKey) + " " + title + mine
	if selected {
		text = itemKey + " " + title + mine
		return s.List.SelBar.Render("▎") + " " + s.List.ItemSelected.Render(text)
	}
	return "  " + s.List.Item.Render(text)
}

// initials collapses "Ada Lovelace" to "AL" for the assignee marker.
func initials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	out := ""
	for i, f := range fields {
		if i > 2 {
			break
		}
		out += strings.ToUpper(f[:1])
	}
	return out
}
