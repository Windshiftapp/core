package gherkin

import (
	"strings"
)

// RenderSpec composes valid Gherkin source for one scenario with its Feature
// context: the inverse of parsing. Export and previews use it so stored
// structure and exported text cannot drift apart.
func RenderSpec(spec ScenarioSpec) string {
	var out strings.Builder

	if len(spec.FeatureTags) > 0 {
		out.WriteString(strings.Join(spec.FeatureTags, " "))
		out.WriteString("\n")
	}
	out.WriteString("Feature: " + spec.FeatureName + "\n")
	writeDescription(&out, spec.FeatureDescription)

	if len(spec.Background) > 0 {
		out.WriteString("\n  Background:\n")
		for _, step := range spec.Background {
			writeStep(&out, step)
		}
	}

	out.WriteString("\n")
	if len(spec.ScenarioTags) > 0 {
		out.WriteString("  " + strings.Join(spec.ScenarioTags, " ") + "\n")
	}
	out.WriteString("  " + spec.ScenarioKeyword + ": " + spec.ScenarioName + "\n")
	writeDescription(&out, spec.ScenarioDescription)
	for _, step := range spec.Steps {
		writeStep(&out, step)
	}

	for _, block := range spec.Examples {
		out.WriteString("\n")
		if len(block.Tags) > 0 {
			out.WriteString("    " + strings.Join(block.Tags, " ") + "\n")
		}
		out.WriteString("    Examples:")
		if block.Name != "" {
			out.WriteString(" " + block.Name)
		}
		out.WriteString("\n")
		widths := exampleColumnWidths(block)
		writeExampleRow(&out, block.Header, widths)
		for _, row := range block.Rows {
			writeExampleRow(&out, row, widths)
		}
	}

	return out.String()
}

func writeDescription(out *strings.Builder, description string) {
	if description == "" {
		return
	}
	for _, line := range strings.Split(description, "\n") {
		out.WriteString("  " + line + "\n")
	}
}

func writeStep(out *strings.Builder, step Step) {
	out.WriteString("    " + step.Keyword + " " + step.Text + "\n")
	if step.DocString != nil {
		out.WriteString("      ```" + step.DocString.ContentType + "\n")
		for _, line := range strings.Split(step.DocString.Content, "\n") {
			out.WriteString("      " + line + "\n")
		}
		out.WriteString("      ```\n")
	}
	if step.DataTable != nil {
		widths := tableColumnWidths(step.DataTable.Rows)
		for _, row := range step.DataTable.Rows {
			writeTableRow(out, row, widths)
		}
	}
}

func tableColumnWidths(rows [][]string) []int {
	widths := make([]int, 0)
	for _, row := range rows {
		for i, cell := range row {
			for i >= len(widths) {
				widths = append(widths, 0)
			}
			// Widths are measured on the escaped form so padding matches
			// what is written.
			if n := len(escapeTableCell(cell)); n > widths[i] {
				widths[i] = n
			}
		}
	}
	return widths
}

func writeTableRow(out *strings.Builder, row []string, widths []int) {
	out.WriteString("      |")
	for i, cell := range row {
		out.WriteString(" " + escapeTableCell(cell))
		if i < len(widths) {
			out.WriteString(strings.Repeat(" ", widths[i]-len(escapeTableCell(cell))))
		}
		out.WriteString(" |")
	}
	out.WriteString("\n")
}

func exampleColumnWidths(block ExamplesBlock) []int {
	widths := make([]int, len(block.Header))
	for i, cell := range block.Header {
		widths[i] = len(escapeTableCell(cell))
	}
	for _, row := range block.Rows {
		for i, cell := range row {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			if n := len(escapeTableCell(cell)); n > widths[i] {
				widths[i] = n
			}
		}
	}
	return widths
}

func writeExampleRow(out *strings.Builder, cells []string, widths []int) {
	out.WriteString("      |")
	for i, cell := range cells {
		escaped := escapeTableCell(cell)
		out.WriteString(" " + escaped)
		if i < len(widths) {
			out.WriteString(strings.Repeat(" ", widths[i]-len(escaped)))
		}
		out.WriteString(" |")
	}
	out.WriteString("\n")
}

// escapeTableCell applies the Gherkin data-table cell escapes.
func escapeTableCell(cell string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "|", "\\|", "\n", "\\n")
	return replacer.Replace(cell)
}
