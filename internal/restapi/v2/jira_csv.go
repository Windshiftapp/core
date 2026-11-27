package v2

import (
	"errors"
	"net/http"
	"strconv"

	"windshift/internal/csvimport"
	"windshift/internal/jiracsv"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/operatorcontrols"
)

const jiraCSVBase = "/admin/jira-csv"

type jiraCSVForm struct {
	File      string `form:"file"`
	Delimiter string `form:"delimiter"`
}

func csvError(err error) error {
	var denied *operatorcontrols.DeniedError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &denied):
		return err
	case errors.Is(err, csvimport.ErrUploadNotFound):
		return newError(404, "not_found", "Upload or job not found")
	case errors.Is(err, jiracsv.ErrKeyTaken), errors.Is(err, csvimport.ErrConfigConflict):
		return newError(409, "conflict", err.Error())
	default:
		return newError(400, "invalid_request", err.Error())
	}
}
func registerJiraCSVRoutes(b *routeBuilder, d Deps) {
	auth := func(r *http.Request) (*models.User, error) {
		u, e := requireSystemAdmin(r, d)
		if e != nil {
			return nil, e
		}
		if d.JiraCSV == nil {
			return nil, newError(503, "unavailable", "CSV import is not configured")
		}
		return u, d.JiraCSV.Check(r.Context())
	}
	b.sessionOnly(func() {
		b.RawDocument[jiraCSVForm, csvimport.StagedUpload]("POST", jiraCSVBase+"/upload", 201, "multipart/form-data", AuthAuthenticated, nil, func(w http.ResponseWriter, r *http.Request) error {
			u, e := auth(r)
			if e != nil {
				return e
			}
			r.Body = http.MaxBytesReader(w, r.Body, 51<<20)
			if e = r.ParseMultipartForm(1 << 20); e != nil {
				return newError(400, "invalid_request", "Invalid CSV upload (maximum 50 MiB)")
			}
			defer func() { _ = r.MultipartForm.RemoveAll() }()
			f, h, e := r.FormFile("file")
			if e != nil {
				return newError(400, "invalid_request", "Choose a CSV file")
			}
			defer func() { _ = f.Close() }()
			data, e := d.JiraCSV.Upload(r.Context(), u.ID, h.Filename, r.FormValue("delimiter"), f)
			if e != nil {
				return csvError(e)
			}
			return writeDocument(w, 201, data)
		})
	})
	b.SessionRead(jiraCSVBase+"/catalog", func(r *http.Request) (jiracsv.Catalog, error) {
		if _, e := auth(r); e != nil {
			return jiracsv.Catalog{}, e
		}
		id := 0
		if raw := r.URL.Query().Get("workspace_id"); raw != "" {
			var e error
			id, e = strconv.Atoi(raw)
			if e != nil || id < 1 {
				return jiracsv.Catalog{}, newError(400, "invalid_request", "Invalid workspace")
			}
		}
		return d.JiraCSV.Catalog(r.Context(), id)
	})
	b.SessionRead(jiraCSVBase+"/key", func(r *http.Request) (bool, error) {
		if _, e := auth(r); e != nil {
			return false, e
		}
		return d.JiraCSV.KeyAvailable(r.URL.Query().Get("key"))
	})
	b.SessionJSON("POST", jiraCSVBase+"/preview", 200, false, func(r *http.Request, c jiracsv.Config) (jiracsv.Preview, error) {
		u, e := auth(r)
		if e != nil {
			return jiracsv.Preview{}, e
		}
		result, e := d.JiraCSV.Preview(r.Context(), u.ID, c)
		return result, csvError(e)
	})
	b.SessionJSON("POST", jiraCSVBase+"/start", 202, false, func(r *http.Request, c jiracsv.Config) (string, error) {
		u, e := auth(r)
		if e != nil {
			return "", e
		}
		id, e := d.JiraCSV.Start(r.Context(), auditActor(r, u), c)
		if e == nil {
			logger.Record(r, logger.ActionJiraImport, logger.ResourceWorkspace, nil, "Jira CSV", map[string]any{"job_id": id})
		}
		return id, csvError(e)
	})
	b.SessionRead(jiraCSVBase+"/jobs", func(r *http.Request) ([]jiracsv.Job, error) {
		u, e := auth(r)
		if e != nil {
			return nil, e
		}
		return d.JiraCSV.History(r.Context(), u.ID)
	})
	b.SessionRead(jiraCSVBase+"/jobs/{job_id}", func(r *http.Request) (jiracsv.Job, error) {
		u, e := auth(r)
		if e != nil {
			return jiracsv.Job{}, e
		}
		job, e := d.JiraCSV.Job(u.ID, r.PathValue("job_id"))
		return job, csvError(e)
	})
}
