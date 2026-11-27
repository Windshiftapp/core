// Package jiracsv imports exported issues without connecting to Jira.
package jiracsv

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"windshift/internal/jira"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/validation"
)

const kind = "jira_csv"

type Config struct {
	UploadID    string                    `json:"upload_id"`
	Filename    string                    `json:"filename"`
	Delimiter   string                    `json:"delimiter"`
	Deployment  string                    `json:"deployment"`
	Source      string                    `json:"source"`
	AuthorKind  string                    `json:"author_kind"`
	WorkspaceID int                       `json:"workspace_id"`
	Name        string                    `json:"name"`
	Key         string                    `json:"key"`
	Columns     []string                  `json:"columns"`
	Values      map[string]map[string]int `json:"values"`
	People      map[string]int            `json:"people"` // 0: unresolved; -1: create inactive.
	NewFields   map[string]Field          `json:"new_fields"`
	DateFormat  string                    `json:"date_format"`
	Timezone    string                    `json:"timezone"`
}

type Field struct {
	Name    string                `json:"name"`
	Type    string                `json:"type"`
	Options []models.SelectOption `json:"options,omitempty"`
}

type Person struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type Comment struct {
	Author string    `json:"author"`
	Body   string    `json:"body"`
	Date   time.Time `json:"date"`
}

type Row struct {
	Key                string         `json:"key"`
	Title              string         `json:"title"`
	Description        string         `json:"description,omitempty"`
	Comments           []Comment      `json:"comments,omitempty"`
	Fields             map[string]any `json:"fields,omitempty"`
	assignee, reporter string
	input              services.ItemCreateInput
}

type Preview struct {
	Ready       int                       `json:"ready"`
	Skipped     int                       `json:"skipped"`
	Invalid     int                       `json:"invalid"`
	Comments    int                       `json:"comments"`
	Samples     []Row                     `json:"samples"`
	Errors      []string                  `json:"errors"`
	Choices     map[string]map[string]int `json:"choices"`
	People      map[string]int            `json:"people"`
	Labels      map[string]string         `json:"people_labels,omitempty"`
	Suggestions map[string]map[string]int `json:"suggestions,omitempty"`
}

var issueKey = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)-[1-9]\d*$`)
var supportedTypes = map[string]bool{"text": true, "textarea": true, "richtext": true, "number": true, "date": true, "boolean": true, "checkbox": true, "select": true, "multiselect": true}
var dateLayouts = map[string]string{"jira": "2/Jan/06 3:04 PM", "iso": "2006-01-02", "dmy": "02/01/2006 15:04", "mdy": "01/02/2006 15:04"}

func sourceRef(parts ...string) string {
	b, _ := json.Marshal(parts)
	return fmt.Sprintf("jira-csv:%x", sha256.Sum256(b))
}

type mapper struct {
	cfg      Config
	fields   map[string]Field
	users    map[int]string
	saved    map[string]int
	logins   map[string]int
	imported map[string]bool
	zone     *time.Location
	preview  Preview
}

func (m *mapper) person(k string) int {
	if v, ok := m.preview.People[k]; ok {
		return v
	}
	id, ok := m.cfg.People[k]
	if !ok {
		id, ok = m.saved[k]
	}
	if !ok && strings.HasPrefix(k, "name:") {
		id = m.logins[strings.ToLower(strings.TrimPrefix(k, "name:"))]
	}
	m.preview.People[k] = id
	return id
}

func (m *mapper) rich(v string) string {
	resolve := func(k string) string {
		id := m.person(k)
		if name := m.users[id]; id > 0 && name != "" {
			return name
		}
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(v), "{") {
		var doc map[string]any
		if json.Unmarshal([]byte(v), &doc) == nil && jira.IsADFDocument(doc) {
			return jira.ConvertADFToMarkdown(doc, func(id string) string {
				if m.cfg.Deployment == "cloud" {
					return resolve("cloud:" + id)
				}
				return ""
			}, nil)
		}
	}
	return jira.ConvertWikiMarkupWithMentions(v, nil, func(token string) string {
		var k string
		if id, ok := strings.CutPrefix(token, "accountid:"); ok && m.cfg.Deployment == "cloud" {
			k = "cloud:" + id
		} else if !ok && m.cfg.Deployment == "dc" {
			k = "name:" + token
		}
		if k != "" {
			if name := resolve(k); name != "" {
				return "@" + name
			}
		}
		return "`" + strings.ReplaceAll(token, "`", "") + "`"
	})
}

func (m *mapper) date(v string) (time.Time, error) {
	for _, format := range []string{time.RFC3339, dateLayouts[m.cfg.DateFormat], "2006-01-02"} {
		if t, err := time.ParseInLocation(format, strings.TrimSpace(v), m.zone); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q", v)
}

func (m *mapper) choice(target, v string, f Field) (int, error) {
	if m.preview.Choices[target] == nil {
		m.preview.Choices[target] = map[string]int{}
	}
	m.preview.Choices[target][v]++
	selected, explicit := m.cfg.Values[target][v]
	if explicit && selected == 0 {
		return 0, nil
	}
	for _, opt := range f.Options {
		if explicit && opt.ID == selected || !explicit && strings.EqualFold(opt.Label, v) {
			return opt.ID, nil
		}
	}
	if !explicit && target == "priority" {
		if label, known := jira.PriorityMappingTarget(v); known {
			for _, o := range f.Options {
				if strings.EqualFold(o.Label, label) {
					if m.preview.Suggestions == nil {
						m.preview.Suggestions = map[string]map[string]int{}
					}
					if m.preview.Suggestions[target] == nil {
						m.preview.Suggestions[target] = map[string]int{}
					}
					m.preview.Suggestions[target][v] = o.ID
					return o.ID, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("map %s value %q or choose the default", f.Name, v)
}

func (m *mapper) convert(target, v string, f Field) (any, error) {
	switch f.Type {
	case "richtext":
		return m.rich(v), nil
	case "text", "textarea":
		return v, nil
	case "number":
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, fmt.Errorf("invalid number %q", v)
		}
		return n, nil
	case "boolean", "checkbox":
		switch strings.ToLower(v) {
		case "yes", "checked", "true", "1":
			return true, nil
		case "no", "unchecked", "false", "0":
			return false, nil
		}
		return nil, fmt.Errorf("invalid checkbox value %q", v)
	case "date":
		t, err := m.date(v)
		if err != nil {
			return nil, err
		}
		return t.Format("2006-01-02"), nil
	case "select", "multiselect":
		id, err := m.choice(target, v, f)
		if id == 0 {
			return nil, err
		}
		return id, err
	default:
		return nil, fmt.Errorf("unsupported field type %q", f.Type)
	}
}

func (m *mapper) row(record []string) (Row, error) {
	r := Row{Fields: map[string]any{}}
	var problem error
	for i, target := range m.cfg.Columns {
		if target == "" || i >= len(record) {
			continue
		}
		v := strings.TrimSpace(record[i])
		if v == "" {
			continue
		}
		switch target {
		case "key":
			r.Key = strings.ToUpper(v)
		case "title":
			r.Title = v
		case "description":
			r.Description = m.rich(v)
		case "reporter", "assignee", "reporter-name", "assignee-name", "reporter-key", "assignee-key":
			identifier := "cloud:"
			if m.cfg.Deployment == "dc" {
				identifier = "unknown:"
				if strings.HasSuffix(target, "-name") {
					identifier = "name:"
				}
				if strings.HasSuffix(target, "-key") {
					identifier = "key:"
				}
			}
			k := identifier + v
			m.person(k)
			if strings.HasPrefix(target, "reporter") {
				r.reporter = k
			} else {
				r.assignee = k
			}
		case "comments":
			ts, rest, ok := strings.Cut(v, ";")
			author, body, ok2 := strings.Cut(rest, ";")
			t, err := m.date(ts)
			if !ok || !ok2 || err != nil || strings.TrimSpace(body) == "" {
				problem = fmt.Errorf("invalid comment in column %d: expected timestamp;author;body", i+1)
				continue
			}
			k := m.cfg.AuthorKind + ":" + strings.TrimSpace(author)
			if author != "" {
				m.person(k)
			} else {
				k = ""
			}
			r.Comments = append(r.Comments, Comment{Author: k, Body: m.rich(body), Date: t})
		default:
			f := m.fields[target]
			val, err := m.convert(target, v, f)
			if err != nil {
				problem = fmt.Errorf("column %d: %w", i+1, err)
				continue
			}
			if val == nil {
				continue
			}
			if f.Type == "multiselect" {
				list, _ := r.Fields[target].([]any)
				r.Fields[target] = append(list, val)
			} else {
				r.Fields[target] = val
			}
		}
	}
	if !issueKey.MatchString(r.Key) {
		return r, fmt.Errorf("missing or invalid Jira issue key")
	}
	var err error
	if r.Title, err = validation.NormalizeTitle(r.Title); err != nil {
		return r, err
	}
	r.input.Title = r.Title
	r.input.Description = r.Description
	if err := validation.ValidateMarkdownSource("description", r.Description, validation.MarkdownMaxBytes, false); err != nil {
		return r, err
	}
	if points, ok := r.Fields["points"].(float64); ok && points < 0 {
		return r, fmt.Errorf("story points cannot be negative")
	}
	return r, problem
}
