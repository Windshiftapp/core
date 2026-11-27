package jiracsv

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"windshift/internal/csvimport"
	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

var ErrKeyTaken = errors.New("workspace key already in use; choose another key")

func (s *Service) Start(ctx context.Context, actor services.AuditActor, c Config) (string, error) {
	c.Key = strings.ToUpper(strings.TrimSpace(c.Key))
	if err := s.Check(ctx); err != nil {
		return "", err
	}
	path := csvimport.UploadPath(s.root, c.UploadID)
	config, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	claim := func() (bool, error) {
		return s.store.ClaimUpload(kind, actor.UserID, actor.UserID, c.UploadID, path, string(config), time.Now().UTC())
	}
	// A successful job may already have removed the file. Compare the claim
	// before opening it so repeated confirmation still returns the same job.
	if _, e := s.store.GetJob(kind, actor.UserID, c.UploadID); e == nil {
		_, err = claim()
		return c.UploadID, err
	} else if !errors.Is(e, csvimport.ErrUploadNotFound) {
		return "", e
	}
	m, path, err := s.preflight(ctx, actor.UserID, c)
	if err != nil {
		return "", err
	}
	if m.preview.Ready == 0 {
		return "", invalid("no valid new issues to import")
	}
	if c.WorkspaceID == 0 {
		ok, e := s.KeyAvailable(c.Key)
		if e != nil {
			return "", e
		}
		if !ok {
			return "", ErrKeyTaken
		}
	}
	won, err := claim()
	if err != nil || !won {
		return c.UploadID, err
	}
	if c.WorkspaceID == 0 {
		ws, e := s.workspaces.Create(ctx, actor, services.CreateWorkspaceParams{Name: c.Name, Key: c.Key})
		if e != nil {
			if errors.Is(e, repository.ErrDuplicateEntry) {
				e = ErrKeyTaken
			}
			// No destination or rows exist for this claim. Retain the upload so
			// the operator can correct a key collision without uploading again.
			if _, release := s.db.ExecWrite("DELETE FROM import_jobs WHERE id=? AND kind=? AND status='queued'", c.UploadID, kind); release != nil {
				return "", release
			}
			return "", e
		}
		m.cfg.WorkspaceID = ws.ID
	}
	go s.run(context.WithoutCancel(ctx), actor, path, m)
	return c.UploadID, nil
}

func (s *Service) resolvePeople(m *mapper) error {
	for k, id := range m.preview.People {
		if id == -1 {
			ref := sourceRef(m.cfg.Source, m.cfg.Deployment, k)
			email := strings.TrimPrefix(ref, "jira-csv:") + "@imported.invalid"
			user, err := s.jira.FindUserByEmail(email)
			if errors.Is(err, repository.ErrNotFound) {
				name, e := s.jira.UniqueImportedUsername("jira-" + strings.TrimPrefix(ref, "jira-csv:")[:10])
				if e != nil {
					return e
				}
				display := m.preview.Labels[k]
				if display == "" {
					display = strings.SplitN(k, ":", 2)[1]
				}
				_, e = s.jira.CreateImportedUser(email, name, display, "(Jira)", "")
				if e != nil && !errors.Is(e, repository.ErrDuplicateEntry) {
					return e
				}
				user, err = s.jira.FindUserByEmail(email)
			}
			if err != nil {
				return err
			}
			account, err := repository.NewUserRepository(s.db).GetByID(user.UserID)
			if err != nil {
				return err
			}
			if account.IsActive {
				return fmt.Errorf("inactive-user import address is already in use; explicitly map %s to an existing user", k)
			}
			id = user.UserID
			m.users[id] = user.Username
		}
		var dbID any
		if id > 0 {
			dbID = id
		}
		if _, err := s.db.ExecWrite(`INSERT INTO jira_csv_people(source_ref,user_id) VALUES(?,?) ON CONFLICT(source_ref) DO UPDATE SET user_id=excluded.user_id`, bindingPrefix(m.cfg)+k, dbID); err != nil {
			return err
		}
		m.preview.People[k] = id
	}
	return nil
}

func (s *Service) materialize(actor services.AuditActor, m *mapper) (map[string]string, error) {
	keyID, _, _, err := s.jira.EnsureSearchableIssueKey()
	if err != nil {
		return nil, err
	}
	ids := map[string]string{"key": strconv.Itoa(keyID)}
	for target := range m.fields {
		if id, ok := strings.CutPrefix(target, "cf:"); ok {
			ids[target] = id
		}
	}
	for target, f := range m.cfg.NewFields {
		next := 1
		for _, o := range f.Options {
			if o.ID >= next {
				next = o.ID + 1
			}
		}
		options, err := json.Marshal(models.SelectFieldOptions{NextID: next, Items: f.Options})
		if err != nil {
			return nil, err
		}
		d := &models.CustomFieldDefinition{Name: f.Name, FieldType: f.Type}
		if f.Type == "select" || f.Type == "multiselect" {
			d.Options = string(options)
		}
		result, e := services.NewCustomFieldProvisioningService(s.db, services.CustomFieldProvisioningHooks{}).Create(actor, d)
		if e != nil {
			return nil, e
		}
		ids[target] = strconv.Itoa(result.Field.ID)
	}
	return ids, s.resolvePeople(m)
}

type progress struct {
	csvimport.Progress
	WorkspaceID int           `json:"workspace_id"`
	Created     []CreatedItem `json:"created,omitempty"`
}

type CreatedItem struct {
	ID     int    `json:"id"`
	Source string `json:"source"`
	Key    string `json:"key"`
}

func (p progress) json() string { b, _ := json.Marshal(p); return string(b) }

func (s *Service) run(ctx context.Context, actor services.AuditActor, path string, m *mapper) {
	c := m.cfg
	p := progress{WorkspaceID: c.WorkspaceID, Progress: csvimport.Progress{Phase: "importing"}}
	defer func() {
		if recover() != nil {
			_ = s.store.Finish(c.UploadID, "failed", "", p.json(), "Import interrupted")
		}
	}()
	if err := s.store.StartRunning(c.UploadID, p.Phase, p.json()); err != nil {
		return
	}
	fields, err := s.materialize(actor, m)
	if err != nil {
		_ = s.store.Finish(c.UploadID, "failed", "setup", p.json(), err.Error())
		return
	}
	comments := services.NewCommentService(s.db)
	fallback := 0
	seen := map[string]bool{}
	err = s.walk(ctx, path, m, func(row Row, e error) error {
		p.TotalRows++
		if e == nil {
			if seen[row.Key] || m.imported[sourceRef(c.Source, c.Deployment, row.Key)] {
				e = csvimport.ErrSkipRow
			} else {
				var created CreatedItem
				e = s.importRow(actor, m, row, fields, comments, &fallback, &created)
				if e == nil && len(p.Created) < 50 {
					p.Created = append(p.Created, created)
				}
				if e == nil || errors.Is(e, csvimport.ErrSkipRow) {
					seen[row.Key] = true
				}
			}
		}
		switch {
		case errors.Is(e, csvimport.ErrLeaseLost):
			return e
		case errors.Is(e, csvimport.ErrSkipRow):
			p.SkippedCount++
		case e != nil:
			p.FailedCount++
			if len(p.Errors) < csvimport.ErrorCap {
				p.Errors = append(p.Errors, fmt.Sprintf("Record %d (%s): %v", p.TotalRows+1, row.Key, e))
			}
		default:
			p.ImportedCount++
		}
		if p.TotalRows%25 == 0 {
			return s.store.UpdateProgress(c.UploadID, p.Phase, p.json())
		}
		return nil
	})
	status := "completed"
	message := ""
	if err != nil {
		status = "failed"
		message = err.Error()
	} else if p.FailedCount > 0 {
		status = "completed_with_errors"
	}
	p.Phase = "completed"
	if e := s.store.Finish(c.UploadID, status, p.Phase, p.json(), message); e == nil {
		_ = os.RemoveAll(filepath.Dir(path))
	}
}

func (s *Service) importRow(actor services.AuditActor, m *mapper, row Row, fields map[string]string, comments *services.CommentService, fallback *int, created *CreatedItem) error {
	c := m.cfg
	in := row.input
	in.WorkspaceID = c.WorkspaceID
	in.CustomFieldValues = map[string]any{fields["key"]: row.Key}
	for target, v := range row.Fields {
		switch target {
		case "type", "status", "priority":
			id, ok := v.(int)
			if !ok {
				return fmt.Errorf("invalid mapped %s", target)
			}
			switch target {
			case "type":
				in.ItemTypeID = &id
			case "status":
				in.StatusID = &id
			case "priority":
				in.PriorityID = &id
			}
		case "due":
			value, ok := v.(string)
			if !ok {
				return fmt.Errorf("invalid mapped due date")
			}
			d, e := time.Parse("2006-01-02", value)
			if e != nil {
				return e
			}
			in.DueDate = &d
		case "points":
			n, ok := v.(float64)
			if !ok {
				return fmt.Errorf("invalid story points")
			}
			in.StoryPoints = &n
		default:
			in.CustomFieldValues[fields[target]] = v
		}
	}
	if id := m.preview.People[row.assignee]; id > 0 {
		in.AssigneeID = &id
	}
	if id := m.preview.People[row.reporter]; id > 0 {
		in.ReporterID = &id
	}
	for _, comment := range row.Comments {
		if m.preview.People[comment.Author] == 0 && *fallback == 0 {
			id, e := s.jira.EnsureImportedDummyUser("imported-user@jira-import.invalid")
			if e != nil {
				return e
			}
			*fallback = id
		}
	}
	in.AfterRecorded = func(ctx context.Context, tx database.Tx, itemID int) error {
		if err := s.store.RenewLeaseInTx(tx, c.UploadID); err != nil {
			return err
		}
		res, err := tx.ExecWrite(`INSERT INTO item_import_rows(workspace_id,external_ref,item_id,job_id) VALUES(?,?,?,?) ON CONFLICT(workspace_id,external_ref) DO NOTHING`, c.WorkspaceID, sourceRef(c.Source, c.Deployment, row.Key), itemID, c.UploadID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return csvimport.ErrSkipRow
		}
		for _, comment := range row.Comments {
			id := m.preview.People[comment.Author]
			body := comment.Body
			if id == 0 {
				id = *fallback
				if comment.Author != "" {
					body = "Original Jira author: `" + strings.ReplaceAll(comment.Author, "`", "") + "`\n\n" + body
				}
			}
			if _, err = comments.CreateInternalImportedInTx(ctx, tx, itemID, id, body, comment.Date, itemevents.Import(c.UploadID)); err != nil {
				return err
			}
		}
		return nil
	}
	result, err := s.items.CreateImported(actor.UserID, actor.Username, in, c.UploadID)
	if err == nil {
		*created = CreatedItem{ID: int(result.ID), Source: row.Key, Key: fmt.Sprintf("%s-%d", result.Item.WorkspaceKey, result.Item.WorkspaceItemNumber)}
	}
	return err
}

// Job returns a single owned job, even after it leaves the recent history.
func (s *Service) Job(user int, id string) (Job, error) {
	row, err := s.store.GetJob(kind, user, id)
	if err != nil {
		return Job{}, err
	}
	if (row.Status.String == "queued" || row.Status.String == "running") && row.LeaseExpiresAt.Int64 <= time.Now().Unix() {
		if _, err = s.store.ReconcileExpired(kind, time.Now().UTC(), nil); err != nil {
			return Job{}, err
		}
		row, err = s.store.GetJob(kind, user, id)
		if err != nil {
			return Job{}, err
		}
	}
	var p progress
	if row.ProgressJSON.Valid {
		if err := json.Unmarshal([]byte(row.ProgressJSON.String), &p); err != nil {
			return Job{}, err
		}
	}
	var raw string
	if err = s.db.QueryRow("SELECT config_json FROM import_jobs WHERE id=? AND kind=? AND scope_id=?", id, kind, user).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return Job{}, csvimport.ErrUploadNotFound
	} else if err != nil {
		return Job{}, err
	}
	var c Config
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return Job{}, err
	}
	return Job{ID: id, Status: row.Status.String, Error: row.ErrorMessage.String, Progress: p.Progress, WorkspaceID: p.WorkspaceID, Created: p.Created, Filename: filepath.Base(c.Filename)}, nil
}
