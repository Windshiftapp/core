package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"windshift/internal/database"
)

// windshiftExport is the post-import snapshot written alongside
// jira_responses.json when JIRA_CAPTURE_PAYLOADS is set. The diff harness
// (scripts/jira_import_diff.py) consumes both files to detect fidelity
// regressions per the Phase 0 plan in docs/jira-import-remediation-plan.md.
//
// Field order is intentional: encoding/json emits struct fields in declaration
// order, so this struct also fixes the JSON key order for diff-friendliness.
type windshiftExport struct {
	JobID         string                `json:"job_id"`
	GeneratedAt   string                `json:"generated_at"`
	SchemaVersion int                   `json:"schema_version"`
	Items         []windshiftExportItem `json:"items"`
	Warnings      []string              `json:"warnings"`
}

type windshiftExportItem struct {
	JiraKey          string                      `json:"jira_key"`
	WindshiftID      int                         `json:"windshift_id"`
	Title            string                      `json:"title"`
	Description      string                      `json:"description"`
	StatusName       string                      `json:"status_name"`
	ItemTypeName     string                      `json:"item_type_name"`
	PriorityName     string                      `json:"priority_name,omitempty"`
	AssigneeUsername string                      `json:"assignee_username,omitempty"`
	ReporterUsername string                      `json:"reporter_username,omitempty"`
	CreatorUsername  string                      `json:"creator_username,omitempty"`
	ParentJiraKey    string                      `json:"parent_jira_key,omitempty"`
	StoryPoints      *float64                    `json:"story_points,omitempty"`
	DueDate          string                      `json:"due_date,omitempty"`
	CreatedAt        string                      `json:"created_at"`
	UpdatedAt        string                      `json:"updated_at"`
	Labels           []string                    `json:"labels"`
	Milestones       []string                    `json:"milestones"`
	CustomFields     map[string]json.RawMessage  `json:"custom_fields"`
	Comments         []windshiftExportComment    `json:"comments"`
	Attachments      []windshiftExportAttachment `json:"attachments"`
	Links            []windshiftExportLink       `json:"links"`
	Worklogs         []windshiftExportWorklog    `json:"worklogs"`
}

type windshiftExportComment struct {
	JiraID         string `json:"jira_id"`
	AuthorUsername string `json:"author_username,omitempty"`
	Content        string `json:"content"`
	CreatedAt      string `json:"created_at"`
}

type windshiftExportAttachment struct {
	JiraID           string `json:"jira_id"`
	OriginalFilename string `json:"original_filename"`
	MimeType         string `json:"mime_type"`
	FileSize         int64  `json:"file_size"`
	UploaderUsername string `json:"uploader_username,omitempty"`
}

type windshiftExportLink struct {
	LinkType      string `json:"link_type"`
	TargetJiraKey string `json:"target_jira_key"`
}

type windshiftExportWorklog struct {
	JiraID           string `json:"jira_id"`
	AuthorUsername   string `json:"author_username,omitempty"`
	TimeSpentSeconds int    `json:"time_spent_seconds"`
	Started          string `json:"started"`
}

// writeWindshiftExport assembles a deterministic snapshot of everything imported
// under jobID and writes it to <dir>/windshift_export.json. Always filters from
// jira_import_id_mappings WHERE job_id = ? so a partial re-import on the same
// workspace cannot leak rows from a prior completed job.
func writeWindshiftExport(db database.Database, jobID, dir string) error {
	exp := windshiftExport{
		JobID:         jobID,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		SchemaVersion: 1,
		Items:         []windshiftExportItem{},
		Warnings:      []string{},
	}

	// First pass: gather every item mapping for this job and a windshift_id ->
	// jira_key index that the link resolver needs later.
	itemMappings, idToKey, warnings, err := loadItemMappings(db, jobID)
	if err != nil {
		return fmt.Errorf("load item mappings: %w", err)
	}
	exp.Warnings = append(exp.Warnings, warnings...)

	for _, im := range itemMappings {
		item, ok, warn := loadItemRow(db, im.windshiftID, im.jiraKey)
		if warn != "" {
			exp.Warnings = append(exp.Warnings, warn)
		}
		if !ok {
			continue
		}
		item.JiraKey = im.jiraKey
		item.WindshiftID = im.windshiftID
		item.ParentJiraKey = im.parentKey

		item.Labels = loadItemLabels(db, im.windshiftID)
		item.Milestones = loadItemMilestones(db, im.windshiftID)
		item.Comments = loadItemComments(db, jobID, im.jiraKey, im.windshiftID)
		item.Attachments = loadItemAttachments(db, jobID, im.jiraKey, im.windshiftID)
		item.Links = loadItemLinks(db, im.windshiftID, idToKey)
		item.Worklogs = []windshiftExportWorklog{} // Phase 1.4 will populate

		exp.Items = append(exp.Items, item)
	}

	sort.Slice(exp.Items, func(i, j int) bool { return exp.Items[i].JiraKey < exp.Items[j].JiraKey })

	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal export: %w", err)
	}

	path := filepath.Join(dir, "windshift_export.json")
	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // path built from filepath.Join with operator-supplied dir
		return fmt.Errorf("write %s: %w", path, err)
	}

	slog.Info("Saved windshift export snapshot", slog.String("component", "jira"),
		slog.String("path", path), slog.Int("items", len(exp.Items)))
	return nil
}

type itemMapping struct {
	jiraKey     string
	windshiftID int
	parentKey   string
}

//nolint:gocritic // result names would shadow loop locals; positional return is clearer
func loadItemMappings(db database.Database, jobID string) ([]itemMapping, map[int]string, []string, error) {
	rows, err := db.Query(`
		SELECT jira_key, windshift_id, COALESCE(metadata_json, '{}')
		FROM jira_import_id_mappings
		WHERE job_id = ? AND entity_type = 'item'
	`, jobID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	var mappings []itemMapping
	idToKey := map[int]string{}
	var warnings []string
	for rows.Next() {
		var (
			jiraKey  string
			wsID     int
			metaJSON string
		)
		if err := rows.Scan(&jiraKey, &wsID, &metaJSON); err != nil {
			warnings = append(warnings, fmt.Sprintf("scan item mapping: %v", err))
			continue
		}
		parent := ""
		if metaJSON != "" {
			var meta map[string]interface{}
			if err := json.Unmarshal([]byte(metaJSON), &meta); err == nil {
				if pk, ok := meta["parent_key"].(string); ok {
					parent = pk
				}
			}
		}
		mappings = append(mappings, itemMapping{jiraKey: jiraKey, windshiftID: wsID, parentKey: parent})
		idToKey[wsID] = jiraKey
	}
	return mappings, idToKey, warnings, rows.Err()
}

//nolint:gocritic // positional return: (row, ok, warning); naming wouldn't add clarity
func loadItemRow(db database.Database, itemID int, jiraKey string) (windshiftExportItem, bool, string) {
	var (
		title       string
		description *string
		statusName  *string
		typeName    *string
		priority    *string
		assignee    *string
		reporter    *string
		creator     *string
		storyPts    *float64
		dueDate     *string
		created     *string
		updated     *string
		cfValues    *string
	)
	err := db.QueryRow(`
		SELECT i.title,
		       i.description,
		       s.name,
		       t.name,
		       p.name,
		       ua.username,
		       ur.username,
		       uc.username,
		       i.story_points,
		       CAST(i.due_date AS TEXT),
		       CAST(i.created_at AS TEXT),
		       CAST(i.updated_at AS TEXT),
		       CAST(i.custom_field_values AS TEXT)
		FROM items i
		LEFT JOIN statuses s   ON s.id = i.status_id
		LEFT JOIN item_types t ON t.id = i.item_type_id
		LEFT JOIN priorities p ON p.id = i.priority_id
		LEFT JOIN users ua     ON ua.id = i.assignee_id
		LEFT JOIN users ur     ON ur.id = i.reporter_id
		LEFT JOIN users uc     ON uc.id = i.creator_id
		WHERE i.id = ?
	`, itemID).Scan(&title, &description, &statusName, &typeName, &priority,
		&assignee, &reporter, &creator, &storyPts, &dueDate, &created, &updated, &cfValues)
	if err != nil {
		return windshiftExportItem{}, false, fmt.Sprintf("item %s (id=%d) missing: %v", jiraKey, itemID, err)
	}

	out := windshiftExportItem{
		Title:            title,
		Description:      deref(description),
		StatusName:       deref(statusName),
		ItemTypeName:     deref(typeName),
		PriorityName:     deref(priority),
		AssigneeUsername: deref(assignee),
		ReporterUsername: deref(reporter),
		CreatorUsername:  deref(creator),
		StoryPoints:      storyPts,
		DueDate:          deref(dueDate),
		CreatedAt:        deref(created),
		UpdatedAt:        deref(updated),
		CustomFields:     map[string]json.RawMessage{},
	}
	if cfValues != nil && *cfValues != "" {
		var bag map[string]json.RawMessage
		if err := json.Unmarshal([]byte(*cfValues), &bag); err == nil {
			out.CustomFields = bag
		}
	}
	return out, true, ""
}

func loadItemLabels(db database.Database, itemID int) []string {
	rows, err := db.Query(`
		SELECT l.name
		FROM item_labels il
		JOIN labels l ON l.id = il.label_id
		WHERE il.item_id = ?
	`, itemID)
	if err != nil {
		return []string{}
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			out = append(out, name)
		}
	}
	_ = rows.Err()
	sort.Strings(out)
	if out == nil {
		return []string{}
	}
	return out
}

func loadItemMilestones(db database.Database, itemID int) []string {
	rows, err := db.Query(`
		SELECT m.name
		FROM item_milestones im
		JOIN milestones m ON m.id = im.milestone_id
		WHERE im.item_id = ?
	`, itemID)
	if err != nil {
		return []string{}
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			out = append(out, name)
		}
	}
	_ = rows.Err()
	sort.Strings(out)
	if out == nil {
		return []string{}
	}
	return out
}

func loadItemComments(db database.Database, jobID, jiraKey string, itemID int) []windshiftExportComment {
	rows, err := db.Query(`
		SELECT m.jira_id, COALESCE(u.username, ''), COALESCE(c.content, ''), CAST(c.created_at AS TEXT)
		FROM jira_import_id_mappings m
		JOIN comments c ON c.id = m.windshift_id AND c.item_id = ?
		LEFT JOIN users u ON u.id = c.author_id
		WHERE m.job_id = ? AND m.entity_type = 'comment' AND m.jira_key = ?
	`, itemID, jobID, jiraKey)
	if err != nil {
		return []windshiftExportComment{}
	}
	defer func() { _ = rows.Close() }()
	var out []windshiftExportComment
	for rows.Next() {
		var c windshiftExportComment
		if err := rows.Scan(&c.JiraID, &c.AuthorUsername, &c.Content, &c.CreatedAt); err == nil {
			out = append(out, c)
		}
	}
	_ = rows.Err()
	sort.Slice(out, func(i, j int) bool { return out[i].JiraID < out[j].JiraID })
	if out == nil {
		return []windshiftExportComment{}
	}
	return out
}

func loadItemAttachments(db database.Database, jobID, jiraKey string, itemID int) []windshiftExportAttachment {
	rows, err := db.Query(`
		SELECT m.jira_id,
		       COALESCE(a.original_filename, ''),
		       COALESCE(a.mime_type, ''),
		       COALESCE(a.file_size, 0),
		       COALESCE(u.username, '')
		FROM jira_import_id_mappings m
		JOIN attachments a ON a.id = m.windshift_id AND a.item_id = ?
		LEFT JOIN users u ON u.id = a.uploaded_by
		WHERE m.job_id = ? AND m.entity_type = 'attachment' AND m.jira_key = ?
	`, itemID, jobID, jiraKey)
	if err != nil {
		return []windshiftExportAttachment{}
	}
	defer func() { _ = rows.Close() }()
	var out []windshiftExportAttachment
	for rows.Next() {
		var a windshiftExportAttachment
		if err := rows.Scan(&a.JiraID, &a.OriginalFilename, &a.MimeType, &a.FileSize, &a.UploaderUsername); err == nil {
			out = append(out, a)
		}
	}
	_ = rows.Err()
	sort.Slice(out, func(i, j int) bool { return out[i].JiraID < out[j].JiraID })
	if out == nil {
		return []windshiftExportAttachment{}
	}
	return out
}

func loadItemLinks(db database.Database, itemID int, idToKey map[int]string) []windshiftExportLink {
	rows, err := db.Query(`
		SELECT COALESCE(lt.name, ''), il.target_id
		FROM item_links il
		LEFT JOIN link_types lt ON lt.id = il.link_type_id
		WHERE il.source_type = 'item' AND il.source_id = ? AND il.target_type = 'item'
	`, itemID)
	if err != nil {
		return []windshiftExportLink{}
	}
	defer func() { _ = rows.Close() }()
	var out []windshiftExportLink
	for rows.Next() {
		var (
			linkType string
			targetID int
		)
		if err := rows.Scan(&linkType, &targetID); err != nil {
			continue
		}
		targetKey, ok := idToKey[targetID]
		if !ok {
			// Link points outside this job's imported items; skip — the diff
			// harness treats these as expected gaps and the importer logs them.
			continue
		}
		out = append(out, windshiftExportLink{LinkType: linkType, TargetJiraKey: targetKey})
	}
	_ = rows.Err()
	sort.Slice(out, func(i, j int) bool {
		if out[i].LinkType != out[j].LinkType {
			return out[i].LinkType < out[j].LinkType
		}
		return out[i].TargetJiraKey < out[j].TargetJiraKey
	})
	if out == nil {
		return []windshiftExportLink{}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
