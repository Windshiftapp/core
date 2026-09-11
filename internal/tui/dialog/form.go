package dialog

import (
	"reflect"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"windshift/internal/tui/components/inputs"
	"windshift/internal/tui/styles"
)

// FormField is one labeled input in a Form. Exactly one of Input, Area, or
// Choice is used.
type FormField struct {
	Key       string // key in the submitted values map
	Label     string
	Multiline bool
	Input     textinput.Model
	Area      textarea.Model
	Choice    *FormChoice
}

// FormChoice describes a picker-backed form field.
type FormChoice struct {
	PickerID    string
	PickerTitle string
	Options     []Option
	Value       any
}

// FormResult is delivered through ResultMsg.Value on submit.
type FormResult struct {
	Values  map[string]string
	Choices map[string]any
}

// Form is a modal form dialog with separate field selection and editing
// modes. Enter edits the selected field, esc stops editing, and s submits
// from selection mode.
type Form struct {
	id      string
	title   string
	fields  []FormField
	focus   int
	styles  *styles.Styles
	width   int
	editing bool
}

// NewForm builds a form dialog. width is the inner content width the
// fields are sized to.
func NewForm(id, title string, fields []FormField, s *styles.Styles, width int) *Form {
	if width < 1 {
		width = 1
	}
	f := &Form{
		id:     id,
		title:  title,
		fields: fields,
		styles: s,
		width:  width,
	}
	for i := range f.fields {
		switch {
		case f.fields[i].Choice != nil:
			continue
		case f.fields[i].Multiline:
			f.fields[i].Area.SetWidth(width)
		default:
			f.fields[i].Input.SetWidth(width)
		}
	}
	f.selectField(0)
	f.OnThemeChanged(s)
	return f
}

func (f *Form) ID() string    { return f.id }
func (f *Form) Title() string { return f.title }

// PreferredWidth lets the app's overlay compositor size the frame to the
// form instead of the default picker width.
func (f *Form) PreferredWidth() int { return f.width + 4 }

// Footer suppresses the default picker footer — the form renders its own
// hint line.
func (f *Form) Footer() string { return "" }

func (f *Form) OnThemeChanged(s *styles.Styles) {
	f.styles = s
	for i := range f.fields {
		switch {
		case f.fields[i].Choice != nil:
			continue
		case f.fields[i].Multiline:
			f.fields[i].Area.SetStyles(inputs.TextareaStyles(s))
		default:
			f.fields[i].Input.SetStyles(inputs.Styles(s))
		}
	}
}

func (f *Form) selectField(i int) {
	if len(f.fields) == 0 {
		return
	}
	if i < 0 {
		i = len(f.fields) - 1
	}
	i %= len(f.fields)
	for j := range f.fields {
		switch {
		case f.fields[j].Choice != nil:
			continue
		case f.fields[j].Multiline:
			f.fields[j].Area.Blur()
		default:
			f.fields[j].Input.Blur()
		}
	}
	f.focus = i
}

func (f *Form) beginEditing() tea.Cmd {
	field := &f.fields[f.focus]
	if field.Choice != nil {
		selected := 0
		for i, option := range field.Choice.Options {
			if reflect.DeepEqual(option.Value, field.Choice.Value) {
				selected = i
				break
			}
		}
		return Open(NewPicker(field.Choice.PickerID, field.Choice.PickerTitle, field.Choice.Options, selected, f.styles))
	}
	f.editing = true
	if field.Multiline {
		return field.Area.Focus()
	}
	cmd := field.Input.Focus()
	field.Input.CursorEnd()
	return cmd
}

func (f *Form) stopEditing() {
	f.editing = false
	field := &f.fields[f.focus]
	if field.Multiline {
		field.Area.Blur()
	} else {
		field.Input.Blur()
	}
}

func (f *Form) values() FormResult {
	vals := make(map[string]string, len(f.fields))
	choices := make(map[string]any)
	for i := range f.fields {
		switch {
		case f.fields[i].Choice != nil:
			choices[f.fields[i].Key] = f.fields[i].Choice.Value
		case f.fields[i].Multiline:
			vals[f.fields[i].Key] = f.fields[i].Area.Value()
		default:
			vals[f.fields[i].Key] = f.fields[i].Input.Value()
		}
	}
	return FormResult{Values: vals, Choices: choices}
}

func (f *Form) HandleKey(msg tea.KeyPressMsg) Action {
	if f.editing {
		return f.handleEditingKey(msg)
	}

	if msg.String() == "s" {
		return Action{Close: true, Selected: f.values()}
	}

	if len(f.fields) == 0 {
		return Action{}
	}
	switch msg.String() {
	case "esc":
		return Action{Close: true}
	case "tab", "down", "j":
		f.selectField(f.focus + 1)
		return Action{}
	case "shift+tab", "up", "k":
		f.selectField(f.focus - 1)
		return Action{}
	case "enter":
		return Action{Cmd: f.beginEditing()}
	}
	return Action{}
}

func (f *Form) handleEditingKey(msg tea.KeyPressMsg) Action {
	cur := &f.fields[f.focus]
	switch msg.String() {
	case "esc":
		f.stopEditing()
		return Action{}
	case "tab":
		f.stopEditing()
		f.selectField(f.focus + 1)
		return Action{}
	case "shift+tab":
		f.stopEditing()
		f.selectField(f.focus - 1)
		return Action{}
	case "enter":
		if !cur.Multiline {
			f.stopEditing()
			return Action{}
		}
	}
	var cmd tea.Cmd
	if cur.Multiline {
		cur.Area, cmd = cur.Area.Update(msg)
	} else {
		cur.Input, cmd = cur.Input.Update(msg)
	}
	return Action{Cmd: cmd}
}

func (f *Form) HandleResult(msg ResultMsg) tea.Cmd {
	for i := range f.fields {
		choice := f.fields[i].Choice
		if choice != nil && choice.PickerID == msg.ID {
			choice.Value = msg.Value
			return nil
		}
	}
	return nil
}

func (f *Form) HandleMessage(msg tea.Msg) tea.Cmd {
	if !f.editing || len(f.fields) == 0 {
		return nil
	}
	field := &f.fields[f.focus]
	var cmd tea.Cmd
	if field.Multiline {
		field.Area, cmd = field.Area.Update(msg)
	} else {
		field.Input, cmd = field.Input.Update(msg)
	}
	return cmd
}

func (f *Form) View(width, height int) string {
	s := f.styles
	var rows []string
	focusLine := 0
	fieldWidth := min(f.width, max(1, width))
	for i := range f.fields {
		fld := &f.fields[i]
		if i == f.focus {
			focusLine = lineCount(rows)
		}
		if fld.Choice == nil {
			if fld.Multiline {
				fld.Area.SetWidth(fieldWidth)
			} else {
				fld.Input.SetWidth(fieldWidth)
			}
		}
		label := s.Form.Label.Render(fld.Label)
		if i == f.focus {
			label = s.List.SelBar.Render("▎") + " " + label
		} else {
			label = "  " + label
		}
		rows = append(rows, label)
		switch {
		case fld.Choice != nil:
			value := s.Base.Hint.Render("(none)")
			for _, option := range fld.Choice.Options {
				if reflect.DeepEqual(option.Value, fld.Choice.Value) {
					value = option.Label
					break
				}
			}
			rows = append(rows, inputs.RenderPickerCell(s, value, i == f.focus))
		case fld.Multiline:
			rows = append(rows, fld.Area.View())
		default:
			rows = append(rows, inputs.Render(s, fld.Input, i == f.focus, f.editing && i == f.focus, fieldWidth))
		}
		if i < len(f.fields)-1 {
			rows = append(rows, "")
		}
	}
	hint := "↑↓ select · enter edit · s save · esc cancel"
	if f.editing {
		hint = "esc stop editing · tab next field"
		if f.fields[f.focus].Multiline {
			hint += " · enter newline"
		}
	}
	rows = append(rows, "", s.Form.Hint.Render(hint))
	lines := strings.Split(strings.Join(rows, "\n"), "\n")
	if height < 1 || len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	start := focusLine - height/3
	if start < 0 {
		start = 0
	}
	if start+height > len(lines) {
		start = len(lines) - height
	}
	return strings.Join(lines[start:start+height], "\n")
}

func lineCount(rows []string) int {
	count := 0
	for _, row := range rows {
		count += strings.Count(row, "\n") + 1
	}
	return count
}
