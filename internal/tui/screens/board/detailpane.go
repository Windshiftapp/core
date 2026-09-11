package board

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"windshift/internal/tui/components/chip"
	"windshift/internal/tui/components/markdown"
	"windshift/internal/tui/core"
	"windshift/internal/tui/data"
)

// detailPane renders the selected item's full view: title, chips, metadata,
// description and (lazily loaded) comments. Scrolling is a plain line
// offset over pre-wrapped content.
type detailPane struct {
	ctx *core.Ctx

	item         *data.WorkItem
	comments     []data.Comment
	commentState loadState

	runs          []data.AgentRun
	runState      loadState
	agentAssignee bool // the item's assignee is a coding agent

	lines  []string // wrapped content cache
	offset int

	md *markdown.Renderer // width-bound; recreated on resize/theme change

	width  int
	height int
}

func newDetailPane(ctx *core.Ctx) *detailPane {
	return &detailPane{ctx: ctx}
}

func (d *detailPane) setSize(w, h int) {
	if w != d.width {
		d.md = nil // renderer is width-bound
	}
	d.width = w
	d.height = h
	d.rebuild()
}

// resetRenderer drops the markdown renderer so the next rebuild re-derives
// it (theme changes).
func (d *detailPane) resetRenderer() {
	d.md = nil
}

// setItem swaps the displayed item, resetting scroll and async-section state.
func (d *detailPane) setItem(item *data.WorkItem, comments []data.Comment, commentState loadState, runs []data.AgentRun, runState loadState, agentAssignee bool) {
	d.item = item
	d.comments = comments
	d.commentState = commentState
	d.runs = runs
	d.runState = runState
	d.agentAssignee = agentAssignee
	d.offset = 0
	d.rebuild()
}

// setComments updates the comment thread without resetting scroll (they
// arrive async under the same item).
func (d *detailPane) setComments(comments []data.Comment) {
	d.comments = comments
	d.commentState = loadLoaded
	d.rebuild()
}

// setAgentRuns updates the agent-activity section without resetting scroll.
func (d *detailPane) setAgentRuns(runs []data.AgentRun) {
	d.runs = runs
	d.runState = loadLoaded
	d.rebuild()
}

func (d *detailPane) scroll(delta int) {
	d.offset += delta
	limit := len(d.lines) - d.height
	if limit < 0 {
		limit = 0
	}
	if d.offset > limit {
		d.offset = limit
	}
	if d.offset < 0 {
		d.offset = 0
	}
}

func (d *detailPane) halfPage(dir int) {
	steps := d.height / 2
	if steps < 1 {
		steps = 1
	}
	d.scroll(dir * steps)
}

// rebuild re-renders the wrapped content lines for the current item/width.
func (d *detailPane) rebuild() {
	d.lines = nil
	if d.item == nil || d.width <= 0 {
		return
	}
	s := d.ctx.Styles
	it := d.item
	w := d.width

	wrap := lipgloss.NewStyle().Width(w)
	var out []string
	add := func(block string) {
		if block == "" {
			out = append(out, "")
			return
		}
		out = append(out, strings.Split(wrap.Render(block), "\n")...)
	}

	add(s.Base.Heading.Render(it.Title))
	add("")

	var chips []string
	if it.StatusName != "" {
		chips = append(chips, chip.Status(s, it.StatusName, it.StatusCategoryColor))
	}
	if it.PriorityName != "" {
		chips = append(chips, chip.Priority(s, it.PriorityName, it.PriorityColor))
	}
	if len(chips) > 0 {
		add(strings.Join(chips, " "))
		add("")
	}

	meta := func(label, value string) {
		if value == "" {
			return
		}
		add(s.Form.Label.Render(label+" ") + value)
	}
	meta("Assignee:", it.AssigneeName)
	meta("Creator:", it.CreatorName)
	meta("Updated:", shortDate(it.UpdatedAt))
	add("")
	add(s.List.Rule.Render(strings.Repeat("─", min(w, 60))))
	add("")

	if d.md == nil {
		d.md = markdown.New(s, w)
	}

	// Markdown blocks arrive pre-wrapped by the renderer — append raw.
	addMD := func(text string) {
		out = append(out, strings.Split(d.md.Render(text), "\n")...)
	}

	if strings.TrimSpace(it.Description) != "" {
		addMD(it.Description)
	} else {
		add(s.Base.Hint.Render("(no description)"))
	}

	// Agent activity — shown when the item has runs or is assigned to a
	// coding agent (so an empty panel still signals "a run will appear").
	if d.agentAssignee || len(d.runs) > 0 {
		add("")
		agentLabel := "Agent activity"
		if d.runState == loadLoaded {
			agentLabel = fmt.Sprintf("Agent activity (%d)", len(d.runs))
		}
		add(s.Form.Label.Render(agentLabel) + " " + s.List.Rule.Render(strings.Repeat("─", max(0, min(w, 60)-len(agentLabel)-1))))
		add("")
		switch {
		case d.runState == loadFailed:
			add(s.Status.Error.Render("Agent activity could not be loaded."))
		case d.runState != loadLoaded:
			add(s.Base.Hint.Render("Loading agent runs…"))
		case len(d.runs) == 0:
			add(s.Base.Hint.Render("No agent runs yet — one starts when the agent picks this up."))
		default:
			for _, run := range d.runs {
				add(d.renderAgentRun(run))
				if run.Error != "" {
					add("  " + s.Status.Error.Render(run.Error))
				}
			}
		}
	}

	add("")
	label := "Comments"
	if d.commentState == loadLoaded {
		label = fmt.Sprintf("Comments (%d)", len(d.comments))
	}
	add(s.Form.Label.Render(label) + " " + s.List.Rule.Render(strings.Repeat("─", max(0, min(w, 60)-len(label)-1))))
	add("")

	switch {
	case d.commentState == loadFailed:
		add(s.Status.Error.Render("Comments could not be loaded."))
	case d.commentState != loadLoaded:
		add(s.Base.Hint.Render("Loading comments…"))
	case len(d.comments) == 0:
		add(s.Base.Hint.Render("No comments yet. Press 'c' to add one."))
	default:
		for _, c := range d.comments {
			author := "Unknown"
			if c.AuthorName != nil {
				author = *c.AuthorName
			}
			add(s.Base.Heading.Render(author) + " " + s.Base.Hint.Render("· "+shortDate(c.CreatedAt)))
			addMD(c.Content)
			add("")
		}
	}

	d.lines = out
	// Re-clamp scroll for the new content length.
	d.scroll(0)
}

func (d *detailPane) view() string {
	s := d.ctx.Styles
	if d.item == nil {
		return s.Base.Hint.Render("Select a work item")
	}
	end := d.offset + d.height
	if end > len(d.lines) {
		end = len(d.lines)
	}
	if d.offset >= end {
		return ""
	}
	return strings.Join(d.lines[d.offset:end], "\n")
}

// renderAgentRun paints one run line: status glyph + status + kind + when.
func (d *detailPane) renderAgentRun(run data.AgentRun) string {
	s := d.ctx.Styles

	var glyph string
	switch run.Status {
	case "succeeded":
		glyph = s.Status.Success.Render("✓")
	case "failed":
		glyph = s.Status.Error.Render("✗")
	case "running":
		glyph = s.Status.Info.Render("●")
	case "canceled", "killed":
		glyph = s.Base.Hint.Render("⊘")
	default: // queued
		glyph = s.Base.Hint.Render("○")
	}

	when := run.EndedAt
	if when == "" {
		when = run.StartedAt
	}
	if when == "" {
		when = run.QueuedAt
	}

	line := glyph + " " + run.Status
	if run.JobKind != "" && run.JobKind != "coding_agent" {
		line += " " + s.List.Muted.Render("("+run.JobKind+")")
	}
	line += " " + s.Base.Hint.Render("· "+shortDateTime(when))
	return line
}

// shortDate trims an RFC3339 timestamp to its date part for display.
func shortDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// shortDateTime trims an RFC3339 timestamp to date + hh:mm.
func shortDateTime(ts string) string {
	if len(ts) >= 16 {
		return ts[:10] + " " + ts[11:16]
	}
	return ts
}
