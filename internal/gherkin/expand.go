package gherkin

import "strings"

// ExpandText replaces every <name> placeholder whose name exists in params
// with the row value. Unknown placeholders are left untouched so a typo
// stays visible in the execution view. Replacement values are not rescanned.
func ExpandText(text string, params map[string]string) string {
	if len(params) == 0 || !strings.Contains(text, "<") {
		return text
	}
	var out strings.Builder
	for {
		start := strings.Index(text, "<")
		if start < 0 {
			out.WriteString(text)
			return out.String()
		}
		end := strings.Index(text[start:], ">")
		if end < 0 {
			out.WriteString(text)
			return out.String()
		}
		name := text[start+1 : start+end]
		out.WriteString(text[:start])
		if value, ok := params[name]; ok {
			out.WriteString(value)
		} else {
			out.WriteString(text[start : start+end+1])
		}
		text = text[start+end+1:]
	}
}

// ExpandStep returns a copy of step with all placeholders in its text,
// doc string, and data table substituted from params.
func ExpandStep(step Step, params map[string]string) Step {
	result := Step{Keyword: step.Keyword, Text: ExpandText(step.Text, params), Line: step.Line}
	if step.DataTable != nil {
		table := &DataTable{}
		table.Rows = make([][]string, len(step.DataTable.Rows))
		for i, row := range step.DataTable.Rows {
			table.Rows[i] = expandCells(row, params)
		}
		result.DataTable = table
	}
	if step.DocString != nil {
		result.DocString = &DocString{
			ContentType: ExpandText(step.DocString.ContentType, params),
			Content:     ExpandText(step.DocString.Content, params),
		}
	}
	return result
}

func expandCells(cells []string, params map[string]string) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = ExpandText(cell, params)
	}
	return out
}

// ExecutionSteps returns the concrete steps for one execution of spec:
// Background steps followed by the scenario steps, with placeholder values
// from row (keyed by Examples column name) substituted. row may be nil for
// plain Scenarios.
func ExecutionSteps(spec *ScenarioSpec, row map[string]string) []Step {
	steps := make([]Step, 0, len(spec.Background)+len(spec.Steps))
	for _, step := range spec.Background {
		steps = append(steps, ExpandStep(step, row))
	}
	for _, step := range spec.Steps {
		steps = append(steps, ExpandStep(step, row))
	}
	return steps
}

// ExampleRowMap converts an Examples row into a column-name keyed map using
// the block's header.
func ExampleRowMap(block *ExamplesBlock, rowIndex int) map[string]string {
	if rowIndex < 0 || rowIndex >= len(block.Rows) {
		return nil
	}
	row := block.Rows[rowIndex]
	params := make(map[string]string, len(block.Header))
	for i, column := range block.Header {
		if i < len(row) {
			params[column] = row[i]
		}
	}
	return params
}
