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
	"windshift/internal/repository"
)

// Pack apply installs a framework pack into a workspace: plugin references
// are verified first (a missing or outdated plugin fails the apply before
// schema or content are touched), then the schema layer is imported and
// attached, then the content layer, and finally the declared conformance
// check runs. Apply is idempotent: every stage converges on the manifest's
// stable names instead of duplicating entities.

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

// PackApplyTarget selects the workspace an pack applies to: an existing
// workspace by ID, or a workspace by name (created when missing).
type PackApplyTarget struct {
	WorkspaceID   int
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

// Apply installs the pack into the target workspace.
func (s *PackApplyService) Apply(ctx context.Context, req PackApplyRequest) (*PackApplyReport, error) {
	report, err := s.newReport(req)
	if err != nil {
		return nil, err
	}
	fail := func(stage, detail string) (*PackApplyReport, error) {
		s.appendStage(report, stage, PackStageStatusFailed, detail)
		report.Status = PackApplyStatusFailed
		return report, nil
	}

	// Stage 1: plugins, before anything is written (AC: a missing plugin
	// leaves schema and content unapplied).
	plugins, satisfied := s.checkPlugins(req.Archive.Manifest.Plugins)
	report.Plugins = plugins
	if !satisfied {
		return fail(PackStagePlugins, "plugin requirements are not satisfied")
	}
	s.appendStage(report, PackStagePlugins, PackStageStatusOK, "plugin requirements satisfied")

	// Stage 2: create-or-target the workspace.
	workspaceID, created, err := s.resolveWorkspace(ctx, req)
	if err != nil {
		return fail(PackStageWorkspace, err.Error())
	}
	report.WorkspaceID = workspaceID
	report.WorkspaceCreated = created
	if created {
		s.appendStage(report, PackStageWorkspace, PackStageStatusOK, fmt.Sprintf("created workspace %q", req.Target.WorkspaceName))
	} else {
		s.appendStage(report, PackStageWorkspace, PackStageStatusOK, "targeting existing workspace")
	}

	// Stage 3: schema — import the configuration-set template unless this
	// pack's configuration set is already attached (idempotent re-run).
	schemaTemplate, err := req.Archive.ConfigurationSetTemplate()
	if err != nil {
		return fail(PackStageSchema, err.Error())
	}
	configSetID, reused, err := s.importOrReuseConfigurationSet(ctx, workspaceID, schemaTemplate)
	if err != nil {
		return fail(PackStageSchema, err.Error())
	}
	report.ConfigSetID = configSetID
	if reused {
		s.appendStage(report, PackStageSchema, PackStageStatusSkipped, "configuration set already attached")
	} else {
		s.appendStage(report, PackStageSchema, PackStageStatusOK, "imported configuration set and attached it to the workspace")
	}

	// Stage 4: content.
	bundle, hasContent, err := req.Archive.WorkspaceBundleFile()
	if err != nil {
		return fail(PackStageContent, err.Error())
	}
	if hasContent {
		contentBundle := &WorkspaceBundle{}
		if err := decodeBundle(bundle, contentBundle); err != nil {
			return fail(PackStageContent, err.Error())
		}
		importer := s.bundleImport
		result, err := importer.ImportWithOptions(ctx, req.Actor, workspaceID, contentBundle, &WorkspaceBundleImportOptions{Idempotent: true})
		if err != nil {
			return fail(PackStageContent, err.Error())
		}
		s.appendStage(report, PackStageContent, PackStageStatusOK,
			fmt.Sprintf("pages %d, items %d, links %d", result.PagesImported, result.ItemsImported, result.ItemLinksImported))
	} else {
		s.appendStage(report, PackStageContent, PackStageStatusSkipped, "pack declares no content bundle")
	}

	// Stage 5: conformance — the schema template must match the live set.
	conformance, err := s.conformance.Check(ctx, configSetID, schemaTemplate)
	if err != nil {
		return fail(PackStageConformance, err.Error())
	}
	report.Conformance = conformance
	if !conformance.Conformant {
		return fail(PackStageConformance, fmt.Sprintf("conformance check reports %d drift rows", conformance.DriftCount))
	}
	s.appendStage(report, PackStageConformance, PackStageStatusOK, "conformance check passed")

	report.Status = PackApplyStatusApplied
	return report, nil
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

// resolveWorkspace finds the target by ID or by name, creating it when the
// caller addressed the pack by workspace name.
func (s *PackApplyService) resolveWorkspace(ctx context.Context, req PackApplyRequest) (workspaceID int, created bool, err error) {
	if req.Target.WorkspaceID > 0 {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)`, req.Target.WorkspaceID).Scan(&exists); err != nil {
			return 0, false, err
		}
		if !exists {
			return 0, false, fmt.Errorf("target workspace %d does not exist", req.Target.WorkspaceID)
		}
		return req.Target.WorkspaceID, false, nil
	}
	name := strings.TrimSpace(req.Target.WorkspaceName)
	if name == "" {
		return 0, false, errors.New("either workspace_id or workspace_name is required")
	}
	var id int
	scanErr := s.db.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE LOWER(name) = LOWER(?)`, name).Scan(&id)
	if scanErr == nil {
		return id, false, nil
	}
	if !errors.Is(scanErr, sql.ErrNoRows) {
		return 0, false, scanErr
	}
	workspace, createErr := s.workspaces.Create(ctx, req.Actor, CreateWorkspaceParams{
		Name:        name,
		Key:         packWorkspaceKey(name),
		Description: fmt.Sprintf("Workspace provisioned by pack %s %s", req.Archive.Manifest.Name, req.Archive.Manifest.Version),
	})
	if createErr != nil {
		// A concurrent apply may have created it in between; converge.
		var again int
		if lookupErr := s.db.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE LOWER(name) = LOWER(?)`, name).Scan(&again); lookupErr == nil {
			return again, false, nil
		}
		return 0, false, fmt.Errorf("create workspace %q: %w", name, createErr)
	}
	return workspace.ID, true, nil
}

// importOrReuseConfigurationSet imports the schema template and attaches the
// resulting set, unless a configuration set with the same stable name is
// already attached to the workspace (idempotent re-run).
func (s *PackApplyService) importOrReuseConfigurationSet(ctx context.Context, workspaceID int, template *ConfigSetTemplate) (configSetID int, reused bool, err error) {
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
	case scanErr == nil && strings.EqualFold(attachedName, template.Payload.ConfigurationSet.Name):
		return attachedID, true, nil
	case scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows):
		return 0, false, scanErr
	}

	importer := NewConfigSetImportService(s.db, s.configSetRepo)
	importedID, _, importErr := importer.Import(ctx, template)
	if importErr != nil {
		return 0, false, fmt.Errorf("import configuration set: %w", importErr)
	}
	// Attach: replace the workspace's assignment (matching the assignment
	// endpoint's semantics).
	if _, err := s.db.ExecContext(ctx, `DELETE FROM workspace_configuration_sets WHERE configuration_set_id = ?`, importedID); err != nil {
		return 0, false, fmt.Errorf("attach configuration set: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
		VALUES (?, ?, ?)
	`, workspaceID, importedID, time.Now()); err != nil {
		return 0, false, fmt.Errorf("attach configuration set: %w", err)
	}
	return importedID, false, nil
}

func (s *PackApplyService) appendStage(report *PackApplyReport, name, status, detail string) {
	report.Stages = append(report.Stages, PackApplyStage{Name: name, Status: status, Detail: detail})
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
