package gherkin

import (
	"fmt"
	"strings"
)

const (
	keywordFeature    = "Feature"
	keywordBackground = "Background"
	keywordScenario   = "Scenario"
	keywordOutline    = "Scenario Outline"
	keywordExamples   = "Examples"
)

var stepKeywords = map[string]bool{
	"Given": true, "When": true, "Then": true, "And": true, "But": true, "*": true,
}

// maxReportedErrors bounds the diagnostic list so a pathological document
// cannot flood the API response.
const maxReportedErrors = 20

type pendingTag struct {
	name string
	line int
	col  int
}

type parser struct {
	lines []string
	errs  []ParseError

	tags []pendingTag

	doc        *Document
	scenario   *Scenario // current scenario being filled
	examples   *ExamplesBlock
	steps      *[]Step // step slice new steps append to
	lastStep   *Step
	skipTo     int // 1-based line after a consumed doc string body
	featureSet bool
	inBackgrnd bool // description lines land on the feature while false
}

// Parse parses a .feature document. When the returned list is non-empty the
// document is invalid: the document is still returned best-effort for
// preview purposes, but callers must not persist it.
func Parse(source string) (*Document, *ErrorList) {
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	p := &parser{lines: strings.Split(normalized, "\n")}
	p.run()
	if len(p.errs) > 0 {
		return p.doc, &ErrorList{Errors: p.errs}
	}
	return p.doc, nil
}

func (p *parser) errorAt(line, col int, format string, args ...any) {
	if len(p.errs) >= maxReportedErrors {
		return
	}
	p.errs = append(p.errs, ParseError{Line: line, Column: col, Message: fmt.Sprintf(format, args...)})
}

func (p *parser) errorf(line int, format string, args ...any) {
	p.errorAt(line, 1, format, args...)
}

// contentColumn returns the 1-based column of the first non-blank rune.
func contentColumn(line string) int {
	trimmed := strings.TrimLeft(line, " \t")
	return len(line) - len(trimmed) + 1
}

func (p *parser) run() {
	for i := 0; i < len(p.lines); i++ {
		raw := strings.TrimRight(p.lines[i], "\r")
		line := i + 1
		trimmed := strings.TrimSpace(raw)
		col := contentColumn(raw)

		switch {
		case trimmed == "":
		case strings.HasPrefix(trimmed, "#"):
			p.handleComment(line, col, trimmed)
		case strings.HasPrefix(trimmed, "@"):
			p.handleTagLine(line, col, trimmed)
		case strings.HasPrefix(trimmed, "|"):
			p.handleTableRow(line, col, raw)
		case strings.HasPrefix(trimmed, "```"), strings.HasPrefix(trimmed, `"""`):
			p.handleDocString(line, col, raw, trimmed)
			i = p.skipTo - 1
		default:
			p.handleKeywordOrText(line, col, trimmed)
		}
		if len(p.errs) >= maxReportedErrors {
			return
		}
	}
	p.finish()
}

// handleComment processes comment lines; only the # language directive is
// meaningful, and only English is supported.
func (p *parser) handleComment(line, col int, trimmed string) {
	body := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
	if !strings.HasPrefix(body, "language") {
		return
	}
	lang := strings.TrimSpace(strings.TrimPrefix(body, "language"))
	lang = strings.TrimSpace(strings.TrimPrefix(lang, ":"))
	if lang != "" && lang != "en" {
		p.errorAt(line, col, "unsupported language %q: only English Gherkin keywords are supported", lang)
	}
}

// handleTagLine collects tags for the next Feature, Scenario, or Examples.
func (p *parser) handleTagLine(line, col int, trimmed string) {
	if p.examples != nil {
		// A tag after example rows starts a new block.
		p.examples = nil
	}
	for _, token := range strings.Fields(trimmed) {
		name, ok := normalizeTag(token)
		if !ok {
			p.errorAt(line, col, "invalid tag %q: tags start with @ and may contain letters, digits, underscore, hyphen, and period", token)
			continue
		}
		p.tags = append(p.tags, pendingTag{name: name, line: line, col: col})
	}
}

func normalizeTag(token string) (string, bool) {
	if len(token) < 2 || token[0] != '@' {
		return "", false
	}
	for _, r := range token[1:] {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return "", false
		}
	}
	return token, true
}

func (p *parser) takeTags() []pendingTag {
	tags := p.tags
	p.tags = nil
	return tags
}

func tagNameList(tags []pendingTag) []string {
	if len(tags) == 0 {
		return nil
	}
	names := make([]string, len(tags))
	for i, tag := range tags {
		names[i] = tag.name
	}
	return names
}

// handleTableRow parses one | cell | cell | row and attaches it to the
// current Examples table or the current step's data table.
func (p *parser) handleTableRow(line, col int, raw string) {
	trimmed := strings.TrimSpace(raw)
	cells, ok := parseTableRowCells(trimmed, p, line, col)
	if !ok {
		return
	}
	switch {
	case p.examples != nil:
		p.appendExampleRow(line, cells)
	case p.lastStep != nil && p.lastStep.DataTable != nil && p.lastStep.DocString == nil:
		p.lastStep.DataTable.Rows = append(p.lastStep.DataTable.Rows, cells)
	case p.lastStep != nil && p.lastStep.DocString == nil:
		p.lastStep.DataTable = &DataTable{Rows: [][]string{cells}}
	default:
		p.errorAt(line, col, "data table row is not attached to a step or Examples table")
	}
}

// parseTableRowCells splits a row body on unescaped pipes and unescapes each
// cell. The structural empties around the opening and closing pipes are
// dropped.
func parseTableRowCells(trimmed string, p *parser, line, col int) ([]string, bool) {
	if !strings.HasSuffix(trimmed, "|") {
		p.errorAt(line, col+len(trimmed), "table row must end with |")
		return nil, false
	}
	var cells []string
	var current strings.Builder
	escaped := false
	for _, r := range trimmed {
		switch {
		case escaped:
			switch r {
			case 'n':
				current.WriteRune('\n')
			default:
				current.WriteRune(r)
			}
			escaped = false
		case r == '\\':
			escaped = true
		case r == '|':
			cells = append(cells, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if escaped {
		p.errorAt(line, col, "table row ends with an incomplete escape sequence")
		return nil, false
	}
	// current holds text after the final pipe; a well-formed row ends with
	// | so it is empty. A non-empty remainder means a missing closing pipe,
	// reported above.
	if len(cells) == 0 || (len(cells) == 1 && cells[0] == "") {
		p.errorAt(line, col, "table row has no cells")
		return nil, false
	}
	if cells[0] == "" {
		cells = cells[1:]
	}
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells, true
}

func (p *parser) appendExampleRow(line int, cells []string) {
	table := p.examples
	if len(table.Header) == 0 {
		table.Header = cells
		return
	}
	if len(cells) != len(table.Header) {
		p.errorf(line, "example row has %d cells but the header defines %d columns", len(cells), len(table.Header))
		return
	}
	table.Rows = append(table.Rows, cells)
}

// handleDocString consumes a doc string block attached to the current step
// and records how far the main loop should skip.
func (p *parser) handleDocString(line, col int, raw, trimmed string) {
	p.skipTo = line + 1
	delimiter := docStringDelimiter(trimmed)
	if delimiter == "" {
		p.errorAt(line, col, "doc string delimiter must be ``` or \"\"\"")
		return
	}
	if p.lastStep == nil {
		p.errorAt(line, col, "doc string is not attached to a step")
		p.skipDocStringBody(line, delimiter)
		return
	}
	if p.lastStep.DocString != nil || p.lastStep.DataTable != nil {
		p.errorAt(line, col, "step already has a doc string or data table")
		p.skipDocStringBody(line, delimiter)
		return
	}
	contentType := strings.TrimSpace(strings.TrimPrefix(trimmed, delimiter))
	indent := raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))]
	var body []string
	closed := false
	for j := line; j < len(p.lines); j++ {
		if strings.TrimSpace(strings.TrimRight(p.lines[j], "\r")) == delimiter {
			p.skipTo = j + 1
			closed = true
			break
		}
		body = append(body, strings.TrimPrefix(strings.TrimRight(p.lines[j], "\r"), indent))
	}
	if !closed {
		p.errorf(line, "doc string opened here is never closed")
		return
	}
	p.lastStep.DocString = &DocString{
		ContentType: contentType,
		Content:     unescapeDocString(strings.Join(body, "\n"), delimiter),
	}
}

// skipDocStringBody consumes an unusable doc string so its body is not
// re-parsed as content, emitting one error for a missing terminator.
func (p *parser) skipDocStringBody(line int, delimiter string) {
	for j := line; j < len(p.lines); j++ {
		if strings.TrimSpace(strings.TrimRight(p.lines[j], "\r")) == delimiter {
			p.skipTo = j + 1
			return
		}
	}
	p.errorf(line, "doc string opened here is never closed")
}

func docStringDelimiter(trimmed string) string {
	switch {
	case strings.HasPrefix(trimmed, "```"):
		return "```"
	case strings.HasPrefix(trimmed, `"""`):
		return `"""`
	default:
		return ""
	}
}

// unescapeDocString applies the Gherkin doc string escapes: only the
// delimiter sequence and the backslash itself are escapable; any other
// backslash stays literal.
func unescapeDocString(content, delimiter string) string {
	if !strings.Contains(content, "\\") {
		return content
	}
	var out strings.Builder
	for i := 0; i < len(content); i++ {
		if content[i] != '\\' {
			out.WriteByte(content[i])
			continue
		}
		rest := content[i+1:]
		switch {
		case strings.HasPrefix(rest, "\\"):
			out.WriteByte('\\')
			i++
		case strings.HasPrefix(rest, delimiter):
			out.WriteString(delimiter)
			i += len(delimiter)
		default:
			out.WriteByte('\\')
		}
	}
	return out.String()
}

// handleKeywordOrText routes a structural keyword line or records
// description text.
func (p *parser) handleKeywordOrText(line, col int, trimmed string) {
	if strings.HasPrefix(trimmed, "Rule:") {
		p.errorAt(line, col, "Rule is not supported: represent the grouping as separate Features instead")
		return
	}
	if strings.HasPrefix(trimmed, "Scenario Template:") {
		p.errorAt(line, col, "Scenario Template is not supported: use Scenario Outline")
		return
	}

	switch {
	case strings.HasPrefix(trimmed, "Feature:"):
		p.startFeature(line, col, trimmed)
	case strings.HasPrefix(trimmed, "Background:"):
		p.startBackground(line, col)
	case strings.HasPrefix(trimmed, "Scenario Outline:"):
		p.startScenario(line, col, trimmed, keywordOutline)
	case strings.HasPrefix(trimmed, "Scenario:"):
		p.startScenario(line, col, trimmed, keywordScenario)
	case strings.HasPrefix(trimmed, "Examples:"):
		p.startExamples(line, col, trimmed)
	default:
		p.handleDescriptionOrStep(line, col, trimmed)
	}
}

func (p *parser) startFeature(line, col int, trimmed string) {
	if p.featureSet {
		p.errorAt(line, col, "a document may contain only one Feature; start a new file for the second Feature")
		return
	}
	tags := p.takeTags()
	title := strings.TrimSpace(strings.TrimPrefix(trimmed, "Feature:"))
	p.doc = &Document{FeatureName: title, FeatureTags: tagNameList(tags)}
	p.featureSet = true
	p.scenario = nil
	p.examples = nil
	p.steps = nil
	p.lastStep = nil
}

func (p *parser) startBackground(line, col int) {
	if !p.featureSet {
		p.errorAt(line, col, "Background must appear after the Feature line")
		p.takeTags()
		return
	}
	if p.doc.Background != nil || len(p.doc.Scenarios) > 0 {
		p.errorAt(line, col, "Background must appear once, directly after the Feature description")
		p.takeTags()
		return
	}
	p.takeTags()
	p.doc.Background = []Step{}
	p.steps = &p.doc.Background
	p.scenario = nil
	p.examples = nil
	p.lastStep = nil
	p.inBackgrnd = true
}

func (p *parser) startScenario(line, col int, trimmed, keyword string) {
	if !p.featureSet {
		p.errorAt(line, col, "%s must appear after the Feature line", keyword)
		p.takeTags()
		return
	}
	tags := p.takeTags()
	p.doc.Scenarios = append(p.doc.Scenarios, Scenario{
		Keyword: keyword,
		Name:    strings.TrimSpace(strings.TrimPrefix(trimmed, keyword+":")),
		Tags:    tagNameList(tags),
		Steps:   []Step{},
		Line:    line,
	})
	p.scenario = &p.doc.Scenarios[len(p.doc.Scenarios)-1]
	p.examples = nil
	p.steps = &p.scenario.Steps
	p.lastStep = nil
	p.inBackgrnd = false
}

func (p *parser) startExamples(line, col int, trimmed string) {
	if p.scenario == nil || p.scenario.Keyword != keywordOutline {
		p.errorAt(line, col, "Examples are only valid inside a Scenario Outline")
		p.takeTags()
		return
	}
	tags := p.takeTags()
	p.scenario.Examples = append(p.scenario.Examples, ExamplesBlock{
		Name: strings.TrimSpace(strings.TrimPrefix(trimmed, "Examples:")),
		Tags: tagNameList(tags),
		Line: line,
	})
	p.examples = &p.scenario.Examples[len(p.scenario.Examples)-1]
	p.lastStep = nil
}

// handleDescriptionOrStep treats the line as a step when it starts with a
// step keyword, and as description text otherwise. A keyword-shaped line
// before the Feature line is most likely a non-English or misspelled
// keyword, so it is reported instead of being absorbed silently.
func (p *parser) handleDescriptionOrStep(line, col int, trimmed string) {
	keyword, text, ok := splitStepKeyword(trimmed)
	if ok {
		p.addStep(line, col, keyword, text)
		return
	}
	if !p.featureSet && looksLikeKeywordLine(trimmed) {
		p.errorAt(line, col, "unsupported keyword %q: only English Gherkin keywords are supported (Feature, Background, Scenario, Scenario Outline, Examples, Given, When, Then, And, But)", trimmed)
		return
	}
	p.appendDescription(line, trimmed)
}

func splitStepKeyword(trimmed string) (keyword, text string, ok bool) {
	first, rest, found := strings.Cut(trimmed, " ")
	if !found {
		if stepKeywords[trimmed] {
			return trimmed, "", true
		}
		return "", "", false
	}
	if !stepKeywords[first] {
		return "", "", false
	}
	return first, strings.TrimSpace(rest), true
}

// looksLikeKeywordLine reports whether a line has the "Word: text" shape of
// a Gherkin block keyword in another language or a misspelled keyword.
func looksLikeKeywordLine(trimmed string) bool {
	first, _, found := strings.Cut(trimmed, ":")
	if !found {
		return false
	}
	first = strings.TrimSpace(first)
	if first == "" || strings.ContainsAny(first, " \t") {
		return false
	}
	for _, r := range first {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= 'À' && r <= 'ÿ':
		case r >= 'Ā' && r <= 'ſ':
		default:
			return false
		}
	}
	return true
}

func (p *parser) addStep(line, col int, keyword, text string) {
	if p.steps == nil {
		p.errorAt(line, col, "step %q is outside of a Scenario or Background", stepLabel(keyword, text))
		return
	}
	if p.examples != nil {
		p.errorAt(line, col, "step %q appears after Examples; steps must precede the Examples table", stepLabel(keyword, text))
		return
	}
	if (keyword == "And" || keyword == "But") && len(*p.steps) == 0 {
		p.errorAt(line, col, "%q cannot be the first step of a block; start with Given, When, or Then", keyword)
		return
	}
	*p.steps = append(*p.steps, Step{Keyword: keyword, Text: text, Line: line})
	p.lastStep = &(*p.steps)[len(*p.steps)-1]
}

func stepLabel(keyword, text string) string {
	if text == "" {
		return keyword
	}
	return keyword + " " + text
}

// appendDescription records free text under the current block's description.
func (p *parser) appendDescription(line int, trimmed string) {
	switch {
	case p.examples != nil, p.inBackgrnd:
		// Free text after Examples: or Background: is not carried into the
		// case format.
	case p.scenario != nil:
		p.scenario.Description = appendDescriptionLine(p.scenario.Description, trimmed)
	case p.doc != nil:
		p.doc.FeatureDescription = appendDescriptionLine(p.doc.FeatureDescription, trimmed)
	default:
		p.errorAt(line, 1, "unexpected text before the Feature line: %q", trimmed)
	}
}

func appendDescriptionLine(existing, line string) string {
	if existing == "" {
		return line
	}
	return existing + "\n" + line
}

// finish runs the structural validations that require the whole document.
func (p *parser) finish() {
	if len(p.tags) > 0 {
		tag := p.tags[0]
		p.errorAt(tag.line, tag.col, "tag %s is not attached to a Feature, Scenario, or Examples block", tag.name)
	}
	if !p.featureSet {
		p.errorf(1, "missing Feature: a BDD case must contain a Feature with at least one Scenario")
		return
	}
	if len(p.doc.Scenarios) == 0 {
		p.errorf(1, "Feature has no scenarios: add a Scenario or Scenario Outline")
		return
	}
	for i := range p.doc.Scenarios {
		p.validateScenario(&p.doc.Scenarios[i])
	}
}

func (p *parser) validateScenario(scenario *Scenario) {
	if len(scenario.Steps) == 0 {
		p.errorf(scenario.Line, "%s %q has no steps", scenario.Keyword, scenario.Name)
	}
	if scenario.Keyword != keywordOutline {
		return
	}
	if len(scenario.Examples) == 0 {
		p.errorf(scenario.Line, "Scenario Outline %q has no Examples: add an Examples table", scenario.Name)
		return
	}
	headers := map[string]bool{}
	for i := range scenario.Examples {
		block := &scenario.Examples[i]
		if len(block.Header) == 0 {
			p.errorf(block.Line, "Examples %q has no table: add a header row and at least one example row", block.Name)
			continue
		}
		if len(block.Rows) == 0 {
			p.errorf(block.Line, "Examples %q has a header but no example rows: every outline execution needs at least one row", block.Name)
		}
		seen := map[string]bool{}
		for _, column := range block.Header {
			if column == "" {
				p.errorf(block.Line, "Examples %q has an empty column name", block.Name)
				continue
			}
			if seen[column] {
				p.errorf(block.Line, "Examples %q has duplicate column %q", block.Name, column)
			}
			seen[column] = true
			headers[column] = true
		}
	}
	p.validatePlaceholders(scenario, headers)
}

// validatePlaceholders verifies every <param> used by the outline's steps is
// provided by the Examples tables.
func (p *parser) validatePlaceholders(scenario *Scenario, headers map[string]bool) {
	missing := map[string]bool{}
	var firstLine int
	collect := func(text string, line int) {
		for _, name := range findPlaceholders(text) {
			if !headers[name] && !missing[name] {
				missing[name] = true
				if firstLine == 0 {
					firstLine = line
				}
			}
		}
	}
	for i := range scenario.Steps {
		step := &scenario.Steps[i]
		collect(step.Text, step.Line)
		if step.DocString != nil {
			collect(step.DocString.Content, step.Line)
		}
		if step.DataTable != nil {
			for _, row := range step.DataTable.Rows {
				for _, cell := range row {
					collect(cell, step.Line)
				}
			}
		}
	}
	if len(missing) == 0 {
		return
	}
	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, "<"+name+">")
	}
	line := firstLine
	if line == 0 {
		line = scenario.Line
	}
	p.errorf(line, "Examples tables do not provide %s: add the column or remove the placeholder", strings.Join(names, ", "))
}

// findPlaceholders returns the <param> names in text, left to right,
// deduplicated.
func findPlaceholders(text string) []string {
	var names []string
	seen := map[string]bool{}
	for {
		start := strings.Index(text, "<")
		if start < 0 {
			return names
		}
		end := strings.Index(text[start:], ">")
		if end < 0 {
			return names
		}
		name := text[start+1 : start+end]
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		text = text[start+end+1:]
	}
}
