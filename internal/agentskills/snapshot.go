package agentskills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"windshift/internal/models"
)

const MaxSnapshotSkills = 32

var ErrSnapshotTooLarge = errors.New("binding skill snapshot exceeds 32 skills, 256 KiB, or 65536 estimated tokens")

type SnapshotReader interface {
	ListEnabledForBindingSnapshot(context.Context, int) ([]*models.WorkspaceAgentSkill, error)
	PageRefsForSkill(context.Context, int) ([]models.SkillPageReference, error)
}

// SnapshotBinding fails closed for both runtimes rather than silently omitting a required skill.
func SnapshotBinding(ctx context.Context, reader SnapshotReader, bindingID, workspaceID int) ([]models.SkillGrant, error) {
	skills, err := reader.ListEnabledForBindingSnapshot(ctx, bindingID)
	if err != nil {
		return nil, fmt.Errorf("list binding skills: %w", err)
	}
	if len(skills) > MaxSnapshotSkills {
		return nil, ErrSnapshotTooLarge
	}
	grants := make([]models.SkillGrant, 0, len(skills))
	for _, skill := range skills {
		if skill.WorkspaceID != workspaceID {
			return nil, errors.New("binding skill is outside the run workspace")
		}
		if err := ValidateMetadata(skill.Name, skill.Description); err != nil {
			return nil, fmt.Errorf("skill %d metadata: %w", skill.ID, err)
		}
		refs, err := reader.PageRefsForSkill(ctx, skill.ID)
		if err != nil {
			return nil, fmt.Errorf("skill %d page snapshots: %w", skill.ID, err)
		}
		body, _, err := RenderActivation(skill.Body, refs)
		if err != nil {
			return nil, fmt.Errorf("skill %d activation: %w", skill.ID, err)
		}
		grants = append(grants, models.SkillGrant{ID: skill.ID, Name: skill.Name, Description: skill.Description, Body: body})
		if err := ValidateSnapshot(grants); err != nil {
			return nil, err
		}
	}
	return grants, nil
}

// ValidateAttachmentCount counts distinct attachments, including disabled skills.
func ValidateAttachmentCount(ids []int) error {
	seen := make(map[int]struct{})
	for _, id := range ids {
		seen[id] = struct{}{}
		if len(seen) > MaxSnapshotSkills {
			return ErrSnapshotTooLarge
		}
	}
	return nil
}

// ValidateSnapshot bounds the complete serialized grant set, including metadata and JSON escaping.
func ValidateSnapshot(grants []models.SkillGrant) error {
	if len(grants) > MaxSnapshotSkills {
		return ErrSnapshotTooLarge
	}
	encoded, err := json.Marshal(grants)
	if err != nil {
		return err
	}
	if len(encoded) > MaxActivationBytes || max((len(encoded)+3)/4, utf8.RuneCount(encoded)) > MaxActivationTokens {
		return ErrSnapshotTooLarge
	}
	return nil
}
