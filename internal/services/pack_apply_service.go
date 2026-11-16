package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/operatorcontrols"
	"windshift/internal/repository"
)

// Pack apply installs a framework pack into a workspace: plugin references
// are verified first (a missing or outdated plugin fails the apply before
// schema or content are touched), then the schema layer is adopted or
// imported and attached, then the content layer, and finally the manifest's
// required statuses are checked. Apply is idempotent: every stage converges
// on the manifest's stable names instead of duplicating entities.
//
// Configuration entities fall into two classes. Owned entities (the
// configuration set, its workflow, screens, item types) are created from the
// template when absent. Shared-registry entities (statuses, priorities,
// categories) are global and unique by name, so the pack adopts whatever the
// instance already has under those names — including an admin-tailored
// configuration set — and never gates on their attributes. The only
// schema-layer failure is a required status that is still missing after the
// schema stage.

const (
	PackApplyStatusApplied   = "applied"
	PackApplyStatusFailed    = "failed"
	PackVerifyStatusVerified = "verified"

	PackStagePlugins     = "plugins"
	PackStageWorkspace   = "workspace"
	PackStageSchema      = "schema"
	PackStageContent     = "content"
	PackStageConformance = "conformance"

	PackStageStatusOK      = "ok"
	PackStageStatusSkipped = "skipped"
	PackStageStatusFailed  = "failed"
)

type PackPluginCheck struct {
	Name         string `json:"name"`
	MinVersion   string `json:"min_version"`
	FoundVersion string `json:"found_version,omitempty"`
	Installed    bool   `json:"installed"`
	Enabled      bool   `json:"enabled"`
	OK           bool   `json:"ok"`
}

type PackApplyStage struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | skipped | failed
	Detail string `json:"detail,omitempty"`
}

type PackApplyReport struct {
	Pack             string                      `json:"pack"`
	PackVersion      string                      `json:"pack_version"`
	WorkspaceID      int                         `json:"workspace_id"`
	WorkspaceCreated bool                        `json:"workspace_created"`
	ConfigSetID      int                         `json:"configuration_set_id,omitempty"`
	Status           string                      `json:"status"`
	Stages           []PackApplyStage            `json:"stages"`
	Plugins          []PackPluginCheck           `json:"plugins,omitempty"`
	Conformance      *ConfigSetConformanceReport `json:"conformance,omitempty"`
}

// PackApplyTarget names the new workspace a pack provisions. Framework packs
// only apply to new workspaces, so there is no existing-workspace target.
type PackApplyTarget struct {
	WorkspaceName string
}

type PackApplyRequest struct {
	Archive *PackArchive
	Target  PackApplyTarget
	Actor   AuditActor
}

// PackApplyService orchestrates the pack apply flow.
type PackApplyService struct {
	db            database.Database
	workspaces    *WorkspaceApplicationService
	configSetRepo *repository.ConfigurationSetRepository
	conformance   *ConfigSetConformanceService
	bundleImport  *WorkspaceBundleImportService
}

func NewPackApplyService(
	db database.Database,
	workspaces *WorkspaceApplicationService,
	configSetRepo *repository.ConfigurationSetRepository,
	conformance *ConfigSetConformanceService,
	bundleImport *WorkspaceBundleImportService,
) *PackApplyService {
	return &PackApplyService{
		db: db, workspaces: workspaces, configSetRepo: configSetRepo,
		conformance: conformance, bundleImport: bundleImport,
	}
}

// Verify validates a pack without applying it: manifest, referenced files,
// and plugin availability. No workspace is created and no content written.
func (s *PackApplyService) Verify(ctx context.Context, req PackApplyRequest) (*PackApplyReport, error) {
	report, err := s.newReport(req)
	if err != nil {
		return nil, err
	}
	report.Status = PackVerifyStatusVerified
	plugins, satisfied := s.checkPlugins(req.Archive.Manifest.Plugins)
	report.Plugins = plugins
	if !satisfied {
		report.Status = PackApplyStatusFailed
		s.appendStage(report, PackStagePlugins, PackStageStatusFailed, "plugin requirements are not satisfied")
		return report, nil
	}
	s.appendStage(report, PackStagePlugins, PackStageStatusOK, "plugin requirements satisfied")
	return report, nil
}

// Apply provisions a new workspace from the pack. Framework packs only apply
// to new workspaces: the target name must not already exist.
func (s *PackApplyService) Apply(ctx context.Context, req PackApplyRequest) (*PackApplyReport, error) {
	if err := operatorcontrols.From(s.db).CheckImport(ctx, "pack"); err != nil {
		return nil, err
	}
	report, err := s.newReport(req)
	if err != nil {
		return nil, err
	}

	// Stage 1: plugins, before anything is written (AC: a missing plugin
	// leaves schema and content unapplied).
	plugins, satisfied := s.checkPlugins(req.Archive.Manifest.Plugins)
	report.Plugins = plugins
	if !satisfied {
		return s.failReport(report, PackStagePlugins, "plugin requirements are not satisfied")
	}
	s.appendStage(report, PackStagePlugins, PackStageStatusOK, "plugin requirements satisfied")

	// Stage 2: create the workspace. A name that already exists is refused, so
	// an apply can never modify a workspace that already exists.
	workspaceID, err := s.resolveWorkspace(ctx, req)
	if err != nil {
		return s.failReport(report, PackStageWorkspace, err.Error())
	}
	report.WorkspaceID = workspaceID
	report.WorkspaceCreated = true
	s.appendStage(report, PackStageWorkspace, PackStageStatusOK, fmt.Sprintf("created workspace %q", req.Target.WorkspaceName))

	return s.applyWorkspaceStages(ctx, req, report)
}

// ApplyToWorkspace runs the schema, content, and conformance stages against an
// already-existing workspace. The caller owns workspace creation and plugin
// verification; this is the shared tail of Apply and create-from-template-pack.
func (s *PackApplyService) ApplyToWorkspace(ctx context.Context, actor AuditActor, workspaceID int, archive *PackArchive) (*PackApplyReport, error) {
	if err := operatorcontrols.From(s.db).CheckImport(ctx, "pack"); err != nil {
		return nil, err
	}
	req := PackApplyRequest{Archive: archive, Actor: actor}
	report, err := s.newReport(req)
	if err != nil {
		return nil, err
	}
	report.WorkspaceID = workspaceID
	s.appendStage(report, PackStageWorkspace, PackStageStatusSkipped, "workspace already exists")
	return s.applyWorkspaceStages(ctx, req, report)
}

// applyWorkspaceStages runs stages 3-5 and leaves report.Status set to applied
// or failed. Stage failures are reported in the result, not as an error.
func (s *PackApplyService) applyWorkspaceStages(ctx context.Context, req PackApplyRequest, report *PackApplyReport) (*PackApplyReport, error) {
	workspaceID := report.WorkspaceID

	// Stage 3: schema — adopt an existing configuration set with the
	// template's name (an admin may have already imported and tailored it),
	// otherwise import the template fresh. Either way the set is attached to
	// the workspace. Idempotent re-runs converge on the same set.
	schemaTemplate, err := req.Archive.ConfigurationSetTemplate()
	if err != nil {
		return s.failReport(report, PackStageSchema, err.Error())
	}
	configSetID, reused, stageDetail, err := s.adoptOrImportConfigurationSet(ctx, workspaceID, schemaTemplate)
	if err != nil {
		return s.failReport(report, PackStageSchema, err.Error())
	}
	report.ConfigSetID = configSetID
	if reused {
		s.appendStage(report, PackStageSchema, PackStageStatusSkipped, stageDetail)
	} else {
		s.appendStage(report, PackStageSchema, PackStageStatusOK, stageDetail)
	}

	// Stage 4: content.
	bundle, hasContent, err := req.Archive.WorkspaceBundleFile()
	if err != nil {
		return s.failReport(report, PackStageContent, err.Error())
	}
	if hasContent {
		contentBundle := &WorkspaceBundle{}
		if err := decodeBundle(bundle, contentBundle); err != nil {
			return s.failReport(report, PackStageContent, err.Error())
		}
		importer := s.bundleImport
		result, err := importer.ImportWithOptions(ctx, req.Actor, workspaceID, contentBundle, &WorkspaceBundleImportOptions{Idempotent: true})
		if err != nil {
			return s.failReport(report, PackStageContent, err.Error())
		}
		if failed := failedContentOutcomeDetails(result.Outcomes); len(failed) > 0 {
			return s.failReport(report, PackStageContent,
				fmt.Sprintf("content bundle reported %d failed entity(ies): %s", len(failed), strings.Join(failed, "; ")))
		}
		s.appendStage(report, PackStageContent, PackStageStatusOK,
			fmt.Sprintf("pages %d, items %d, links %d, calendars %d, metrics %d, views %t",
				result.PagesImported, result.ItemsImported, result.ItemLinksImported,
				result.CalendarsImported, result.MetricsImported, result.ViewsApplied))
	} else {
		s.appendStage(report, PackStageContent, PackStageStatusSkipped, "pack declares no content bundle")
	}

	// Stage 5: availability — the manifest's required statuses must exist
	// after the schema stage. Shared-registry attributes (descriptions,
	// categories) are the instance's to design, so attribute differences in
	// the informational check below never gate the apply.
	conformance, err := s.conformance.Check(ctx, configSetID, schemaTemplate)
	if err != nil {
		return s.failReport(report, PackStageConformance, err.Error())
	}
	report.Conformance = conformance
	if missing := missingRequiredStatuses(ctx, s.db, requiredStatuses(req.Archive)); len(missing) > 0 {
		return s.failReport(report, PackStageConformance,
			fmt.Sprintf("required statuses are not available and could not be created: %s", strings.Join(missing, ", ")))
	}
	s.appendStage(report, PackStageConformance, PackStageStatusOK,
		"required statuses available; shared configuration entities adopted as-is")

	report.Status = PackApplyStatusApplied
	return report, nil
}

// failReport records a failed stage and returns the report without an error,
// matching Apply's "failures are reported in the result" contract.
func (s *PackApplyService) failReport(report *PackApplyReport, stage, detail string) (*PackApplyReport, error) {
	s.appendStage(report, stage, PackStageStatusFailed, detail)
	report.Status = PackApplyStatusFailed
	return report, nil
}

// failedContentOutcomeDetails summarizes the content entities a bundle import
// could not write, so the apply report can fail instead of claiming success for
// missing seed content.
func failedContentOutcomeDetails(outcomes []WorkspaceBundleImportOutcome) []string {
	var failed []string
	for _, o := range outcomes {
		if o.Status != "failed" {
			continue
		}
		label := o.Entity
		if o.Name != "" {
			label += " " + o.Name
		} else if o.Ref != "" {
			label += " " + o.Ref
		}
		if o.Detail != "" {
			label += ": " + o.Detail
		}
		failed = append(failed, label)
	}
	return failed
}

// VerifyBuiltinPack resolves an embedded pack and validates its manifest and
// plugin requirements without writing anything. Implements
// WorkspacePackProvisioner.
func (s *PackApplyService) VerifyBuiltinPack(ctx context.Context, name string) (*PackApplyReport, error) {
	archive, err := BuiltinPackArchive(name)
	if err != nil {
		return nil, err
	}
	return s.Verify(ctx, PackApplyRequest{Archive: archive})
}

// ApplyBuiltinPackToWorkspace resolves an embedded pack and applies it to an
// already-created workspace. Implements WorkspacePackProvisioner.
func (s *PackApplyService) ApplyBuiltinPackToWorkspace(ctx context.Context, actor AuditActor, name string, workspaceID int) (*PackApplyReport, error) {
	archive, err := BuiltinPackArchive(name)
	if err != nil {
		return nil, err
	}
	return s.ApplyToWorkspace(ctx, actor, workspaceID, archive)
}

// ---- stages -----------------------------------------------------------------

// checkPlugins verifies every plugin reference against the registry.
func (s *PackApplyService) checkPlugins(refs []PackPluginRef) (checks []PackPluginCheck, satisfied bool) {
	satisfied = true
	checks = make([]PackPluginCheck, 0, len(refs))
	for _, ref := range refs {
		check := PackPluginCheck{Name: ref.Name, MinVersion: ref.MinVersion}
		var version string
		var enabled bool
		err := s.db.QueryRow(
			`SELECT COALESCE(version, ''), COALESCE(enabled, false) FROM plugin_registry WHERE LOWER(name) = LOWER(?)`,
			ref.Name,
		).Scan(&version, &enabled)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			check.FoundVersion = ""
			check.Installed = false
		case err != nil:
			check.Installed = false
			satisfied = false
			checks = append(checks, check)
			continue
		default:
			check.Installed = true
			check.Enabled = enabled
			check.FoundVersion = version
		}
		check.OK = check.Installed && check.Enabled
		if check.OK {
			cmp, err := compareSemver(check.FoundVersion, ref.MinVersion)
			if err != nil || cmp < 0 {
				check.OK = false
			}
		}
		if !check.OK {
			satisfied = false
		}
		checks = append(checks, check)
	}
	return checks, satisfied
}

// resolveWorkspace creates the workspace a pack provisions. Framework packs
// only apply to new workspaces, so a name that already exists is refused
// instead of reused.
func (s *PackApplyService) resolveWorkspace(ctx context.Context, req PackApplyRequest) (int, error) {
	name := strings.TrimSpace(req.Target.WorkspaceName)
	if name == "" {
		return 0, errors.New("workspace_name is required")
	}
	var existing int
	scanErr := s.db.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE LOWER(name) = LOWER(?)`, name).Scan(&existing)
	switch {
	case scanErr == nil:
		return 0, fmt.Errorf("workspace %q already exists; framework packs only apply to new workspaces", name)
	case !errors.Is(scanErr, sql.ErrNoRows):
		return 0, scanErr
	}
	workspace, createErr := s.workspaces.Create(ctx, req.Actor, CreateWorkspaceParams{
		Name:        name,
		Key:         packWorkspaceKey(name),
		Description: fmt.Sprintf("Workspace provisioned by pack %s %s", req.Archive.Manifest.Name, req.Archive.Manifest.Version),
	})
	if createErr != nil {
		return 0, fmt.Errorf("create workspace %q: %w", name, createErr)
	}
	return workspace.ID, nil
}

// adoptOrImportConfigurationSet converges on one configuration set for the
// template's name: the set already attached to the workspace wins, then the
// oldest same-named set anywhere in the instance (the admin's established
// design), otherwise the template imports fresh. The chosen set is attached
// to the workspace, replacing any prior assignment.
func (s *PackApplyService) adoptOrImportConfigurationSet(ctx context.Context, workspaceID int, template *ConfigSetTemplate) (configSetID int, reused bool, stageDetail string, err error) {
	templateName := template.Payload.ConfigurationSet.Name

	// Already-attached set with the template's name: idempotent re-run.
	var attachedID int
	var attachedName string
	scanErr := s.db.QueryRowContext(ctx, `
		SELECT cs.id, cs.name
		FROM workspace_configuration_sets wcs
		JOIN configuration_sets cs ON cs.id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ?
		ORDER BY cs.id DESC
	`, workspaceID).Scan(&attachedID, &attachedName)
	switch {
	case scanErr == nil && strings.EqualFold(attachedName, templateName):
		return attachedID, true, "configuration set already attached", nil
	case scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows):
		return 0, false, "", scanErr
	}

	// Same-named set elsewhere in the instance: adopt it as-is. Its
	// attributes are the admin's design, not drift to repair.
	var adoptedID int
	em := s.db.QueryRowContext(ctx, `
		SELECT id FROM configuration_sets
		WHERE LOWER(name) = LOWER(?)
		ORDER BY id ASC
		LIMIT 1
	`, templateName).Scan(&adoptedID)
	switch {
	case em == nil:
		if err := attachConfigurationSet(ctx, s.db, workspaceID, adoptedID); err != nil {
			return 0, false, "", err
		}
		return adoptedID, true, fmt.Sprintf("adopted existing configuration set %q", templateName), nil
	case !errors.Is(em, sql.ErrNoRows):
		return 0, false, "", em
	}

	// Fresh import; the importer creates any missing shared entities.
	importer := NewConfigSetImportService(s.db, s.configSetRepo)
	importedID, _, importErr := importer.Import(ctx, template)
	if importErr != nil {
		return 0, false, "", fmt.Errorf("import configuration set: %w", importErr)
	}
	if err := attachConfigurationSet(ctx, s.db, workspaceID, importedID); err != nil {
		return 0, false, "", err
	}
	return importedID, false, fmt.Sprintf("imported configuration set %q and attached it to the workspace", templateName), nil
}

// attachConfigurationSet replaces the workspace's configuration-set
// assignment. Scoping the delete to configuration_set_id would detach the set
// from every other workspace sharing it.
func attachConfigurationSet(ctx context.Context, db database.Database, workspaceID, configSetID int) error {
	// One transaction: a crash or error between delete and insert must not
	// leave the workspace without any assignment (WI-1534).
	return database.WithTx(db, func(tx database.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_configuration_sets WHERE workspace_id = ?`, workspaceID); err != nil {
			return fmt.Errorf("attach configuration set: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
			VALUES (?, ?, ?)
		`, workspaceID, configSetID, time.Now()); err != nil {
			return fmt.Errorf("attach configuration set: %w", err)
		}
		return nil
	})
}

// requiredStatuses returns the manifest-declared statuses that must exist
// after the schema stage.
func requiredStatuses(archive *PackArchive) []string {
	if archive == nil || archive.Manifest == nil || archive.Manifest.Conformance == nil {
		return nil
	}
	return archive.Manifest.Conformance.RequiredStatuses
}

// missingRequiredStatuses reports which of the required statuses are absent
// from the instance's global status registry (case-insensitive).
func missingRequiredStatuses(ctx context.Context, db database.Database, required []string) []string {
	var missing []string
	for _, name := range required {
		name := strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var exists bool
		if err := db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM statuses WHERE LOWER(name) = LOWER(?))`, name,
		).Scan(&exists); err != nil || !exists {
			missing = append(missing, name)
		}
	}
	return missing
}

func (s *PackApplyService) appendStage(report *PackApplyReport, name, status, detail string) {
	report.Stages = append(report.Stages, PackApplyStage{Name: name, Status: status, Detail: detail})
}

// firstFailedPackStage returns the first stage reported as failed, so callers
// can surface the concrete reason instead of the bare "failed" status.
func firstFailedPackStage(report *PackApplyReport) *PackApplyStage {
	if report == nil {
		return nil
	}
	for i := range report.Stages {
		if report.Stages[i].Status == PackStageStatusFailed {
			return &report.Stages[i]
		}
	}
	return nil
}

func (s *PackApplyService) newReport(req PackApplyRequest) (*PackApplyReport, error) {
	if req.Archive == nil || req.Archive.Manifest == nil {
		return nil, errors.New("pack apply: no pack archive")
	}
	status := PackApplyStatusApplied
	return &PackApplyReport{
		Pack:        req.Archive.Manifest.Name,
		PackVersion: req.Archive.Manifest.Version,
		Status:      status,
	}, nil
}

// packWorkspaceKey derives a workspace key from the pack workspace name.
// Workspace keys are 2-10 alphanumeric characters, so everything else is
// stripped; the pack resolves the workspace by name first, so the key only
// needs to be stable enough for the initial creation.
func packWorkspaceKey(name string) string {
	var out []rune
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		}
	}
	key := string(out)
	if len(key) < 2 {
		key = "pack" + key
	}
	if len(key) > 10 {
		key = key[:10]
	}
	return key
}

// decodeBundle converts a parsed workspace-bundle JSON document into the
// typed bundle.
func decodeBundle(raw map[string]any, out *WorkspaceBundle) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, out)
}

// AuditApply records a pack apply with its outcome.
func (s *PackApplyService) AuditApply(actor AuditActor, workspaceID int, pack, packVersion, status string) {
	emitServiceAudit(s.db, actor, logger.ActionPackApply, logger.ResourceWorkspace, &workspaceID, pack, map[string]any{
		"pack": pack, "pack_version": packVersion, "status": status,
	})
}
