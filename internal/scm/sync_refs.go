package scm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"windshift/internal/models"
)

// refShort normalizes canonical v<digit> tags and release/ branches for
// shared upsert keys; unusual refs pass through unchanged.
func refShort(refType, refName string) string {
	switch refType {
	case "tag":
		if len(refName) > 1 && (refName[0] == 'v' || refName[0] == 'V') && refName[1] >= '0' && refName[1] <= '9' {
			return refName[1:]
		}
		return refName
	case "branch":
		if strings.HasPrefix(refName, "release/") {
			return strings.TrimPrefix(refName, "release/")
		}
		return refName
	default:
		return refName
	}
}

// matchGlob treats empty or malformed patterns as no match.
func matchGlob(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	ok, err := filepath.Match(pattern, name)
	if err != nil {
		// Malformed patterns must not fire on a ref.
		return false
	}
	return ok
}

// tagPatternMatches reports whether a tag name is in scope for a pattern. An
// empty pattern means "every tag", which matchGlob alone treats as no match.
func tagPatternMatches(pattern, name string) bool {
	if pattern == "" {
		return true
	}
	return matchGlob(pattern, name)
}

// globLiteralPrefix returns the leading literal run of a glob so a provider
// can narrow a server-side ref query. It returns "" when the pattern contains
// an escape or starts with a metacharacter, keeping the result a true prefix
// of every match.
func globLiteralPrefix(pattern string) string {
	if strings.Contains(pattern, `\`) {
		return ""
	}
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*', '?', '[':
			return pattern[:i]
		}
	}
	return pattern
}

// loadMilestonePatterns reads the per-repo tag + branch glob from
// workspace_repositories. Missing columns (older installs that haven't
// hit the migration yet) yield the schema defaults.
func (s *SyncService) loadMilestonePatterns(ctx context.Context, repoID int) (tagPattern, branchPattern string, err error) {
	var tagNS, branchNS sql.NullString
	if err = s.db.QueryRowContext(ctx, `
		SELECT milestone_tag_pattern, milestone_branch_pattern
		FROM workspace_repositories
		WHERE id = ?
	`, repoID).Scan(&tagNS, &branchNS); err != nil {
		return "v*", "release/*", fmt.Errorf("load milestone patterns: %w", err)
	}
	tagPattern = "v*"
	if tagNS.Valid && tagNS.String != "" {
		tagPattern = tagNS.String
	}
	branchPattern = "release/*"
	if branchNS.Valid && branchNS.String != "" {
		branchPattern = branchNS.String
	}
	return tagPattern, branchPattern, nil
}

// isRefProcessed reports whether (repoID, refType, refName) is already
// in scm_processed_refs. A row means the ref was baselined or its event was
// durably admitted, so later syncs must not observe it as new.
func (s *SyncService) isRefProcessed(ctx context.Context, repoID int, refType, refName string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM scm_processed_refs
		WHERE workspace_repository_id = ? AND ref_type = ? AND ref_name = ?
	`, repoID, refType, refName).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// markRefProcessed inserts the idempotency row for a baseline or a legacy
// event emission. Durable admission uses recordRefObservation's transaction.
func (s *SyncService) markRefProcessed(ctx context.Context, repoID int, refType, refName, sha string) error {
	_, err := s.db.ExecWriteContext(ctx, `
		INSERT INTO scm_processed_refs(workspace_repository_id, ref_type, ref_name, sha, processed_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, repoID, refType, refName, sha)
	return err
}

func (s *SyncService) recordRefObservation(
	ctx context.Context,
	repoID int,
	refType, refName, sha string,
	event *models.ActionEvent,
	emit bool,
) error {
	if s.durableActionEvents == nil {
		if emit && s.actionEvents != nil {
			s.actionEvents.EmitActionEvent(event)
		}
		return s.markRefProcessed(ctx, repoID, refType, refName, sha)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SCM ref observation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO scm_processed_refs(workspace_repository_id, ref_type, ref_name, sha, processed_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (workspace_repository_id, ref_type, ref_name) DO NOTHING
	`, repoID, refType, refName, sha)
	if err != nil {
		return fmt.Errorf("admit SCM ref identity: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read SCM ref admission result: %w", err)
	}
	if inserted == 0 {
		return nil
	}
	if emit {
		if err := s.durableActionEvents.EmitActionEventInTx(ctx, tx, event); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SCM ref observation: %w", err)
	}
	return nil
}

// syncTagsAndReleases baselines first-sync tags and emits later matching tags
// with chronological prev_tag. Providers without tag support no-op.
func (s *SyncService) syncTagsAndReleases(
	ctx context.Context,
	provider Provider,
	owner, repo string,
	repoID, workspaceID int,
	baseline bool,
) error {
	rp, ok := provider.(RefProvider)
	if !ok {
		return nil
	}
	tagPattern, _, err := s.loadMilestonePatterns(ctx, repoID)
	if err != nil {
		return err
	}

	// The provider applies the pattern before resolving tag timestamps, so a
	// non-matching tag never costs a lookup.
	tags, err := rp.ListTags(ctx, owner, repo, tagPattern)
	if err != nil {
		return fmt.Errorf("list tags: %w", err)
	}

	// Providers may return tags in any order; sort by CreatedAt ascending so
	// the "previous tag" lookup is prev = i-1.
	sort.SliceStable(tags, func(i, j int) bool {
		return tags[i].CreatedAt.Before(tags[j].CreatedAt)
	})

	var syncErrs []error
	for i, t := range tags {
		if err := ctx.Err(); err != nil {
			return err
		}
		seen, err := s.isRefProcessed(ctx, repoID, "tag", t.Name)
		if err != nil {
			syncErrs = append(syncErrs, fmt.Errorf("check processed tag %q: %w", t.Name, err))
			continue
		}
		if seen {
			continue
		}
		var prev string
		if i > 0 {
			prev = tags[i-1].Name
		}
		if err := s.recordRefObservation(
			ctx,
			repoID,
			"tag",
			t.Name,
			t.SHA,
			s.tagEvent(workspaceID, repoID, owner, repo, t, prev),
			!baseline,
		); err != nil {
			syncErrs = append(syncErrs, fmt.Errorf("persist tag %q observation: %w", t.Name, err))
		}
	}
	return errors.Join(syncErrs...)
}

// syncReleaseBranches lists branches matching the repo's branch pattern. It
// baselines first-sync branches and emits an ActionEvent for later refs.
// Branches don't carry
// a usable creation timestamp from most providers, so there's no
// "prev_branch" concept — the create_milestone action's upsert_key is
// what pairs the branch with its later tag.
func (s *SyncService) syncReleaseBranches(
	ctx context.Context,
	provider Provider,
	owner, repo string,
	repoID, workspaceID int,
	baseline bool,
) error {
	_, branchPattern, err := s.loadMilestonePatterns(ctx, repoID)
	if err != nil {
		return err
	}
	branches, err := provider.ListBranches(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("list branches: %w", err)
	}
	var syncErrs []error
	for _, b := range branches {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !matchGlob(branchPattern, b.Name) {
			continue
		}
		seen, err := s.isRefProcessed(ctx, repoID, "branch", b.Name)
		if err != nil {
			syncErrs = append(syncErrs, fmt.Errorf("check processed branch %q: %w", b.Name, err))
			continue
		}
		if seen {
			continue
		}
		if err := s.recordRefObservation(
			ctx,
			repoID,
			"branch",
			b.Name,
			b.SHA,
			s.branchEvent(workspaceID, repoID, owner, repo, b),
			!baseline,
		); err != nil {
			syncErrs = append(syncErrs, fmt.Errorf("persist branch %q observation: %w", b.Name, err))
		}
	}
	return errors.Join(syncErrs...)
}

// emitTagEvent constructs and dispatches an ActionEvent for a newly
// observed tag. The payload lives entirely in NewValues so the variable
// substitution layer (`{{ref.short}}` etc.) can read it without a new
// struct field on ActionEvent.
func (s *SyncService) tagEvent(workspaceID, repoID int, owner, repo string, t Tag, prevTagName string) *models.ActionEvent {
	values := map[string]any{
		"ref.name":                     t.Name,
		"ref.short":                    refShort("tag", t.Name),
		"ref.sha":                      t.SHA,
		"ref.type":                     "tag",
		"ref.prev_name":                prevTagName,
		"repo.owner":                   owner,
		"repo.name":                    repo,
		"repo.full_name":               fmt.Sprintf("%s/%s", owner, repo),
		"repo.workspace_repository_id": repoID,
	}
	return &models.ActionEvent{
		EventType:   models.ActionTriggerSCMTagCreated,
		WorkspaceID: workspaceID,
		NewValues:   values,
	}
}

// emitBranchEvent: same as emitTagEvent but for the release-branch case.
// No prev_branch; the action's upsert_key is the pairing mechanism.
func (s *SyncService) branchEvent(workspaceID, repoID int, owner, repo string, b Branch) *models.ActionEvent {
	values := map[string]any{
		"ref.name":                     b.Name,
		"ref.short":                    refShort("branch", b.Name),
		"ref.sha":                      b.SHA,
		"ref.type":                     "branch",
		"repo.owner":                   owner,
		"repo.name":                    repo,
		"repo.full_name":               fmt.Sprintf("%s/%s", owner, repo),
		"repo.workspace_repository_id": repoID,
	}
	return &models.ActionEvent{
		EventType:   models.ActionTriggerSCMReleaseBranchCreated,
		WorkspaceID: workspaceID,
		NewValues:   values,
	}
}
