package jiracsv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"windshift/internal/csvimport"
	"windshift/internal/database"
	"windshift/internal/jiraimport"
	"windshift/internal/models"
	"windshift/internal/operatorcontrols"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type Service struct {
	db         database.Database
	root       string
	store      *csvimport.Store
	jira       *jiraimport.Service
	workspaces *services.WorkspaceApplicationService
	items      *services.ItemCreationService
}

func New(db database.Database, root string, workspaces *services.WorkspaceApplicationService, items *services.ItemCreationService) *Service {
	return &Service{db: db, root: root, store: csvimport.NewStore(db), jira: jiraimport.New(db, nil), workspaces: workspaces, items: items}
}

func (s *Service) Check(ctx context.Context) error {
	return operatorcontrols.From(s.db).CheckImport(ctx, kind)
}

func (s *Service) Upload(ctx context.Context, user int, name, delimiter string, r io.Reader) (*csvimport.StagedUpload, error) {
	if err := s.Check(ctx); err != nil {
		return nil, err
	}
	if s.root == "" {
		return nil, fmt.Errorf("import storage is not configured")
	}
	if err := s.expireUploads(ctx); err != nil {
		return nil, err
	}
	data, _, err := csvimport.StageUploadContext(ctx, s.store, s.root, kind, user, user, name, true, delimiter, r, 50<<20)
	return data, err
}

// Reap abandoned drafts and terminal-job files alongside the next upload.
// Removing the ownership row first fences a concurrent claim of an expired draft.
func (s *Service) expireUploads(ctx context.Context) error {
	rows, err := s.db.QueryWriteContext(ctx, `DELETE FROM import_uploads WHERE id IN (
		SELECT u.id FROM import_uploads u WHERE u.kind=? AND u.created_at < ?
		AND NOT EXISTS (SELECT 1 FROM import_jobs j WHERE j.id=u.id AND j.status IN ('queued','running'))
		ORDER BY u.created_at LIMIT 100) RETURNING id`, kind, time.Now().Add(-24*time.Hour).Unix())
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := os.RemoveAll(filepath.Dir(csvimport.UploadPath(s.root, id))); err != nil {
			return err
		}
	}
	return nil
}

type Catalog struct {
	Fields     map[string]Field             `json:"fields"`
	Workspaces []jiraimport.WorkspaceTarget `json:"workspaces"`
	Users      []Person                     `json:"users"`
}

func (s *Service) Catalog(ctx context.Context, workspace int) (Catalog, error) {
	c := Catalog{Fields: map[string]Field{}}
	workspaces, err := repository.NewWorkspaceRepository(s.db).ListActiveBasics()
	if err != nil {
		return c, err
	}
	for _, w := range workspaces {
		c.Workspaces = append(c.Workspaces, jiraimport.WorkspaceTarget{ID: w.ID, Name: w.Name, Key: w.Key})
	}
	configs := repository.NewConfigurationSetRepository(s.db)
	var effective *repository.EffectiveConfiguration
	if workspace != 0 {
		valid, e := s.jira.WorkspaceImportTarget(workspace)
		if e != nil {
			return c, e
		}
		if valid == nil {
			return c, fmt.Errorf("invalid destination workspace")
		}
		effective, err = configs.ResolveForWorkspace(ctx, workspace, nil)
	} else {
		effective, err = configs.ResolveDefault(ctx, nil)
	}
	if err != nil {
		return c, err
	}
	if effective == nil {
		effective = &repository.EffectiveConfiguration{}
	}
	types, err := repository.NewItemTypeRepository(s.db).List(effective.ConfigurationSetID)
	if err != nil {
		return c, err
	}
	f := Field{Name: "Issue type", Type: "select"}
	for _, v := range types {
		f.Options = append(f.Options, models.SelectOption{ID: v.ID, Label: v.Name})
	}
	c.Fields["type"] = f
	priorities, err := repository.NewPriorityRepository(s.db).List(effective.ConfigurationSetID)
	if err != nil {
		return c, err
	}
	f = Field{Name: "Priority", Type: "select"}
	for _, v := range priorities {
		f.Options = append(f.Options, models.SelectOption{ID: v.ID, Label: v.Name})
	}
	c.Fields["priority"] = f
	f = Field{Name: "Status", Type: "select"}
	var workspaceIDs []int
	if workspace > 0 {
		workspaceIDs = []int{workspace}
	}
	statuses, err := repository.NewStatusRepository(s.db).ListForWorkspaces(workspaceIDs)
	if err != nil {
		return c, err
	}
	for _, v := range statuses {
		f.Options = append(f.Options, models.SelectOption{ID: v.ID, Label: v.Name})
	}
	c.Fields["status"] = f
	c.Fields["points"] = Field{Name: "Story points", Type: "number"}
	c.Fields["due"] = Field{Name: "Due date", Type: "date"}
	defs, err := repository.NewCustomFieldRepository(s.db).List()
	if err != nil {
		return c, err
	}
	for _, d := range defs {
		if !supportedTypes[d.FieldType] || jiraimport.ReservedIssueKeyName(d.Name) {
			continue
		}
		f = Field{Name: d.Name, Type: d.FieldType}
		if f.Type == "select" || f.Type == "multiselect" {
			opts, e := models.ParseSelectOptions(d.Options)
			if e != nil {
				return c, e
			}
			f.Options = opts.Items
		}
		c.Fields["cf:"+strconv.Itoa(d.ID)] = f
	}
	users, err := s.db.QueryContext(ctx, `SELECT id,username FROM users WHERE is_active=true AND COALESCE(is_agent,false)=false ORDER BY username,id`)
	if err != nil {
		return c, err
	}
	defer func() { _ = users.Close() }()
	for users.Next() {
		var u Person
		if err := users.Scan(&u.ID, &u.Username); err != nil {
			return c, err
		}
		c.Users = append(c.Users, u)
	}
	return c, users.Err()
}

func (s *Service) KeyAvailable(key string) (bool, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if !services.ValidWorkspaceKey(key) {
		return false, nil
	}
	used, err := repository.NewWorkspaceRepository(s.db).KeyExists(key)
	return !used, err
}

func (s *Service) owned(user int, id string) (string, error) {
	ok, err := s.store.UploadOwnedBy(kind, user, user, id)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", csvimport.ErrUploadNotFound
	}
	return csvimport.UploadPath(s.root, id), nil
}

func (s *Service) prepare(ctx context.Context, c Config) (*mapper, error) {
	switch c.Deployment {
	case "cloud":
		if c.AuthorKind != "cloud" {
			return nil, fmt.Errorf("use account IDs for Cloud comment authors")
		}
	case "dc":
		if c.AuthorKind != "name" && c.AuthorKind != "key" && c.AuthorKind != "unknown" {
			return nil, fmt.Errorf("choose a Data Center author identifier kind")
		}
	default:
		return nil, fmt.Errorf("choose a source deployment")
	}
	if strings.TrimSpace(c.Source) == "" || len(c.Source) > 200 {
		return nil, fmt.Errorf("a source instance label is required")
	}
	if c.WorkspaceID == 0 {
		if !services.ValidWorkspaceKey(c.Key) || strings.TrimSpace(c.Name) == "" || len(c.Name) > 100 {
			return nil, fmt.Errorf("valid workspace name and key are required")
		}
	} else if c.Key != "" || c.Name != "" {
		return nil, fmt.Errorf("choose one destination")
	}
	if dateLayouts[c.DateFormat] == "" || c.Timezone == "" {
		return nil, fmt.Errorf("choose a date format and timezone")
	}
	zone, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("unknown timezone")
	}
	cat, err := s.Catalog(ctx, c.WorkspaceID)
	if err != nil {
		return nil, err
	}
	m := &mapper{cfg: c, fields: cat.Fields, users: map[int]string{}, saved: map[string]int{}, logins: map[string]int{}, zone: zone, preview: Preview{Choices: map[string]map[string]int{}, People: map[string]int{}}}
	for _, u := range cat.Users {
		k := strings.ToLower(u.Username)
		if _, ok := m.logins[k]; ok {
			m.logins[k] = 0
		} else {
			m.logins[k] = u.ID
		}
		m.users[u.ID] = u.Username
	}
	// Only decisions from this exact instance and deployment are reusable.
	prefix := bindingPrefix(c)
	rows, err := s.db.QueryContext(ctx, `SELECT p.source_ref, COALESCE(u.id,0), COALESCE(u.username,'') FROM jira_csv_people p LEFT JOIN users u ON p.user_id=u.id WHERE p.source_ref LIKE ?`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ref, name string
		var id int
		if err := rows.Scan(&ref, &id, &name); err != nil {
			return nil, err
		}
		m.saved[strings.TrimPrefix(ref, prefix)] = id
		if id > 0 {
			m.users[id] = name
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for k, id := range c.People {
		if id < -1 || id > 0 && m.users[id] == "" || id == -1 && strings.HasPrefix(k, "unknown:") {
			return nil, fmt.Errorf("invalid identity mapping")
		}
	}
	for target, f := range c.NewFields {
		if !strings.HasPrefix(target, "new:") || strings.TrimSpace(f.Name) == "" || len(f.Name) > 100 || !supportedTypes[f.Type] || jiraimport.ReservedIssueKeyName(f.Name) {
			return nil, fmt.Errorf("invalid new field definition")
		}
		if f.Type == "select" || f.Type == "multiselect" {
			ids := map[int]bool{}
			labels := map[string]bool{}
			for _, o := range f.Options {
				label := strings.ToLower(strings.TrimSpace(o.Label))
				if o.ID < 1 || label == "" || ids[o.ID] || labels[label] {
					return nil, fmt.Errorf("provide distinct option labels and IDs for %s", f.Name)
				}
				ids[o.ID] = true
				labels[label] = true
			}
		}
		for _, existing := range m.fields {
			if strings.EqualFold(existing.Name, f.Name) {
				return nil, fmt.Errorf("field %q already exists; map to it instead", f.Name)
			}
		}
		m.fields[target] = f
	}
	counts := map[string]int{}
	for _, target := range c.Columns {
		if target == "" {
			continue
		}
		switch target {
		case "key", "title", "description", "assignee", "reporter", "assignee-name", "reporter-name", "assignee-key", "reporter-key", "comments":
		default:
			if _, ok := m.fields[target]; !ok {
				return nil, fmt.Errorf("invalid target field %q", target)
			}
		}
		base := target
		if strings.HasPrefix(base, "reporter-") {
			base = "reporter"
		}
		if strings.HasPrefix(base, "assignee-") {
			base = "assignee"
		}
		counts[base]++
		if counts[base] > 1 && target != "comments" && m.fields[target].Type != "multiselect" {
			return nil, fmt.Errorf("multiple columns mapped to %s", base)
		}
	}
	if counts["title"] != 1 || counts["key"] != 1 {
		return nil, fmt.Errorf("map Issue key and Summary")
	}
	for k := range c.NewFields {
		if counts[k] == 0 {
			return nil, fmt.Errorf("unused field definition")
		}
	}
	return m, nil
}

func bindingPrefix(c Config) string { return sourceRef(c.Deployment, c.Source) + ":" }

func (s *Service) walk(ctx context.Context, path string, m *mapper, fn func(Row, error) error) error {
	file, err := os.Open(path) //nolint:gosec // ownership was checked before resolving the staged path
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	reader := csvimport.NewReader(file, csvimport.ParseDelimiter(m.cfg.Delimiter))
	headers, err := reader.Read()
	if err != nil {
		return err
	}
	if len(headers) != len(m.cfg.Columns) {
		return fmt.Errorf("mapping does not match the file header")
	}
	// Cloud exports often carry a name next to its opaque ID. The name
	// describes the person in the picker; it is never a lookup key.
	var pairs [][2]int
	if m.cfg.Deployment == "cloud" {
		indices := map[string]int{}
		for i, h := range headers {
			indices[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF")))] = i
		}
		for _, field := range []string{"reporter", "assignee", "creator", "project lead"} {
			name, ok := indices[field]
			id, okID := indices[field+" id"]
			if ok && okID {
				pairs = append(pairs, [2]int{name, id})
			}
		}
	}
	m.preview.Labels = map[string]string{}
	project := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rec, e := reader.Read()
		if errors.Is(e, io.EOF) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, v := range rec {
			if !utf8.ValidString(v) {
				return fmt.Errorf("file must be UTF-8")
			}
		}
		row, e := m.row(rec)
		for _, pair := range pairs {
			if pair[0] < len(rec) && pair[1] < len(rec) && rec[pair[0]] != "" && rec[pair[1]] != "" {
				m.preview.Labels["cloud:"+rec[pair[1]]] = rec[pair[0]]
			}
		}
		if parts := issueKey.FindStringSubmatch(row.Key); parts != nil {
			if project != "" && project != parts[1] {
				return fmt.Errorf("export a single Jira project per file")
			}
			project = parts[1]
		}
		if len(rec) != len(headers) {
			e = fmt.Errorf("record width differs from the header")
		}
		if err := fn(row, e); err != nil {
			return err
		}
	}
}

func (s *Service) preflight(ctx context.Context, user int, c Config) (*mapper, string, error) {
	path, err := s.owned(user, c.UploadID)
	if err != nil {
		return nil, "", err
	}
	m, err := s.prepare(ctx, c)
	if err != nil {
		return nil, "", err
	}
	if c.WorkspaceID != 0 {
		m.imported, err = s.importedRefs(ctx, c.WorkspaceID)
		if err != nil {
			return nil, "", err
		}
	}
	seen := map[string]bool{}
	row := 1
	err = s.walk(ctx, path, m, func(r Row, e error) error {
		row++
		if e != nil {
			m.preview.Invalid++
			if len(m.preview.Errors) < 50 {
				m.preview.Errors = append(m.preview.Errors, fmt.Sprintf("Record %d (%s): %v", row, r.Key, e))
			}
			return nil
		}
		if seen[r.Key] {
			m.preview.Skipped++
			return nil
		}
		seen[r.Key] = true
		if m.imported[sourceRef(c.Source, c.Deployment, r.Key)] {
			m.preview.Skipped++
			return nil
		}
		m.preview.Ready++
		m.preview.Comments += len(r.Comments)
		if len(m.preview.Samples) < 5 {
			m.preview.Samples = append(m.preview.Samples, r)
		}
		return nil
	})
	return m, path, err
}

func (s *Service) importedRefs(ctx context.Context, workspaceID int) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT external_ref FROM item_import_rows WHERE workspace_id=? AND external_ref LIKE 'jira-csv:%'", workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	refs := map[string]bool{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		refs[ref] = true
	}
	return refs, rows.Err()
}

func (s *Service) Preview(ctx context.Context, user int, c Config) (Preview, error) {
	m, _, err := s.preflight(ctx, user, c)
	if err != nil {
		return Preview{}, err
	}
	return m.preview, nil
}

type Job struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	csvimport.Progress
	WorkspaceID int           `json:"workspace_id"`
	Created     []CreatedItem `json:"created,omitempty"`
}

func (s *Service) History(ctx context.Context, user int) ([]Job, error) {
	if _, err := s.store.ReconcileExpired(kind, time.Now().UTC(), nil); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, config_json,status,COALESCE(error_message,''),COALESCE(progress_json,'{}') FROM import_jobs WHERE kind=? AND scope_id=? ORDER BY created_at DESC LIMIT 50`, kind, user)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	jobs := []Job{}
	for rows.Next() {
		var j Job
		var cfg, progress string
		if err := rows.Scan(&j.ID, &cfg, &j.Status, &j.Error, &progress); err != nil {
			return nil, err
		}
		var c Config
		if err := json.Unmarshal([]byte(cfg), &c); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(progress), &j.Progress); err != nil {
			return nil, err
		}
		var dest struct {
			WorkspaceID int `json:"workspace_id"`
		}
		_ = json.Unmarshal([]byte(progress), &dest)
		j.WorkspaceID = dest.WorkspaceID
		j.Filename = filepath.Base(c.Filename)
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
