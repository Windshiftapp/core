// Package gherkin parses the BDD test-case format's supported Gherkin subset.
//
// The supported subset is English-keyword Gherkin: Feature, Background,
// Scenario, Scenario Outline, Examples, Given/When/Then/And/But/* steps,
// step data tables, doc strings, tags, and comments. Unsupported constructs
// (Rule, Scenario Template, a # language directive for another language,
// non-English keywords) are rejected with line/column errors rather than
// being silently dropped.
package gherkin

import (
	"fmt"
	"strings"
)

// Step is one Given/When/Then step with its optional argument.
type Step struct {
	Keyword   string     `json:"keyword"`
	Text      string     `json:"text"`
	DataTable *DataTable `json:"data_table,omitempty"`
	DocString *DocString `json:"doc_string,omitempty"`
	// Line is the 1-based source line of the step keyword.
	Line int `json:"line,omitempty"`
}

// DataTable is a step argument table. Cells are unescaped and edge-trimmed.
// Step data tables carry data rows only; ExamplesBlock owns the header
// semantics of Examples tables.
type DataTable struct {
	Rows [][]string `json:"rows"`
}

// DocString is a fenced step argument.
type DocString struct {
	ContentType string `json:"content_type,omitempty"`
	Content     string `json:"content"`
}

// ExamplesBlock is one Examples section of a Scenario Outline.
type ExamplesBlock struct {
	Name   string     `json:"name,omitempty"`
	Tags   []string   `json:"tags,omitempty"`
	Header []string   `json:"header"`
	Rows   [][]string `json:"rows"`
	// Line is the 1-based source line of the Examples keyword.
	Line int `json:"line,omitempty"`
}

// Scenario is one Scenario or Scenario Outline with its authored content.
type Scenario struct {
	Keyword     string          `json:"keyword"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Steps       []Step          `json:"steps"`
	Examples    []ExamplesBlock `json:"examples,omitempty"`
	Line        int             `json:"line,omitempty"`
}

// Document is a parsed .feature file.
type Document struct {
	FeatureName        string     `json:"feature_name"`
	FeatureDescription string     `json:"feature_description,omitempty"`
	FeatureTags        []string   `json:"feature_tags,omitempty"`
	Background         []Step     `json:"background,omitempty"`
	Scenarios          []Scenario `json:"scenarios"`
}

// ScenarioSpec is the derived, storable representation of one authored
// scenario together with its Feature context. One Scenario or Scenario
// Outline becomes one test case; the spec is persisted as JSON in
// test_case_bdd.spec and in run snapshots.
type ScenarioSpec struct {
	FeatureName         string          `json:"feature_name,omitempty"`
	FeatureDescription  string          `json:"feature_description,omitempty"`
	FeatureTags         []string        `json:"feature_tags,omitempty"`
	Background          []Step          `json:"background,omitempty"`
	ScenarioKeyword     string          `json:"scenario_keyword"`
	ScenarioName        string          `json:"scenario_name"`
	ScenarioDescription string          `json:"scenario_description,omitempty"`
	ScenarioTags        []string        `json:"scenario_tags,omitempty"`
	Steps               []Step          `json:"steps"`
	Examples            []ExamplesBlock `json:"examples,omitempty"`
}

// ScenarioSpecFrom returns the storable spec for one scenario of a document.
func (d *Document) ScenarioSpecFrom(scenario *Scenario) ScenarioSpec {
	return ScenarioSpec{
		FeatureName:         d.FeatureName,
		FeatureDescription:  d.FeatureDescription,
		FeatureTags:         d.FeatureTags,
		Background:          d.Background,
		ScenarioKeyword:     scenario.Keyword,
		ScenarioName:        scenario.Name,
		ScenarioDescription: scenario.Description,
		ScenarioTags:        scenario.Tags,
		Steps:               scenario.Steps,
		Examples:            scenario.Examples,
	}
}

// ParseError is a validation error with a 1-based line and column.
type ParseError struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

func (e ParseError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Message)
}

// ErrorList is the aggregate parse failure returned by Parse. It renders as
// one message per line so callers can surface every problem at once.
type ErrorList struct {
	Errors []ParseError `json:"errors"`
}

func (l *ErrorList) Error() string {
	parts := make([]string, len(l.Errors))
	for i, err := range l.Errors {
		parts[i] = err.Error()
	}
	return strings.Join(parts, "\n")
}

// Empty reports whether the list carries no errors.
func (l *ErrorList) Empty() bool {
	return l == nil || len(l.Errors) == 0
}
