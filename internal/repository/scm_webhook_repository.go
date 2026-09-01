package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
)

// SCMWebhookRepository persists GitLab webhook configuration and deliveries.
type SCMWebhookRepository struct {
	db database.Database
}

// SCMWebhookRepositoryAccess identifies the workspace and provider for a linked repository.
type SCMWebhookRepositoryAccess struct {
	WorkspaceID  int
	ProviderType string
}

// SCMWebhookConfig is the persisted configuration visible to workspace admins.
type SCMWebhookConfig struct {
	WebhookKey     string
	Active         bool
	LastDeliveryAt *time.Time
}

// SCMWebhookTarget contains the trusted data needed to validate an inbound webhook.
type SCMWebhookTarget struct {
	ID                    int
	WorkspaceRepositoryID int
	EncryptedSecret       string
	RepositoryExternalID  string
	ProviderType          string
}

// NewSCMWebhookRepository constructs an SCM webhook repository.
func NewSCMWebhookRepository(db database.Database) *SCMWebhookRepository {
	return &SCMWebhookRepository{db: db}
}

// GetRepositoryAccess returns workspace and provider metadata for a linked repository.
func (r *SCMWebhookRepository) GetRepositoryAccess(ctx context.Context, repoID int) (SCMWebhookRepositoryAccess, error) {
	var access SCMWebhookRepositoryAccess
	err := r.db.QueryRowContext(ctx, `
		SELECT wsc.workspace_id, sp.provider_type
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wr.id = ?
	`, repoID).Scan(&access.WorkspaceID, &access.ProviderType)
	if errors.Is(err, sql.ErrNoRows) {
		return SCMWebhookRepositoryAccess{}, ErrNotFound
	}
	if err != nil {
		return SCMWebhookRepositoryAccess{}, fmt.Errorf("load SCM webhook repository access: %w", err)
	}
	return access, nil
}

// GetConfig returns the webhook configuration for a linked repository.
func (r *SCMWebhookRepository) GetConfig(ctx context.Context, repoID int) (SCMWebhookConfig, error) {
	var config SCMWebhookConfig
	var lastDelivery sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT webhook_key, is_active, last_delivery_at
		FROM scm_webhooks
		WHERE workspace_repository_id = ?
	`, repoID).Scan(&config.WebhookKey, &config.Active, &lastDelivery)
	if errors.Is(err, sql.ErrNoRows) {
		return SCMWebhookConfig{}, ErrNotFound
	}
	if err != nil {
		return SCMWebhookConfig{}, fmt.Errorf("load SCM webhook config: %w", err)
	}
	if lastDelivery.Valid {
		config.LastDeliveryAt = &lastDelivery.Time
	}
	return config, nil
}

// RotateConfig creates or rotates a webhook secret while preserving an existing callback key.
func (r *SCMWebhookRepository) RotateConfig(ctx context.Context, repoID int, newKey, encryptedSecret, events string) (string, error) {
	var key string
	err := r.db.QueryRowContext(ctx, `SELECT webhook_key FROM scm_webhooks WHERE workspace_repository_id = ?`, repoID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		key = newKey
		_, err = r.db.ExecWriteContext(ctx, `
			INSERT INTO scm_webhooks(workspace_repository_id, webhook_key, webhook_secret_encrypted, events, is_active)
			VALUES (?, ?, ?, ?, true)
		`, repoID, key, encryptedSecret, events)
	} else if err == nil {
		_, err = r.db.ExecWriteContext(ctx, `
			UPDATE scm_webhooks
			SET webhook_secret_encrypted = ?, is_active = true, updated_at = CURRENT_TIMESTAMP
			WHERE workspace_repository_id = ?
		`, encryptedSecret, repoID)
	}
	if err != nil {
		return "", fmt.Errorf("rotate SCM webhook config: %w", err)
	}
	return key, nil
}

// DeleteConfig removes a linked repository's webhook configuration.
func (r *SCMWebhookRepository) DeleteConfig(ctx context.Context, repoID int) error {
	if _, err := r.db.ExecWriteContext(ctx, `DELETE FROM scm_webhooks WHERE workspace_repository_id = ?`, repoID); err != nil {
		return fmt.Errorf("delete SCM webhook config: %w", err)
	}
	return nil
}

// GetTargetByKey returns an active webhook target and its validation metadata.
func (r *SCMWebhookRepository) GetTargetByKey(ctx context.Context, key string) (SCMWebhookTarget, error) {
	var target SCMWebhookTarget
	err := r.db.QueryRowContext(ctx, `
		SELECT sw.id, sw.workspace_repository_id, sw.webhook_secret_encrypted,
		       wr.repository_external_id, sp.provider_type
		FROM scm_webhooks sw
		JOIN workspace_repositories wr ON wr.id = sw.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE sw.webhook_key = ? AND sw.is_active = true AND wr.is_active = true
	`, key).Scan(
		&target.ID,
		&target.WorkspaceRepositoryID,
		&target.EncryptedSecret,
		&target.RepositoryExternalID,
		&target.ProviderType,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SCMWebhookTarget{}, ErrNotFound
	}
	if err != nil {
		return SCMWebhookTarget{}, fmt.Errorf("load SCM webhook target: %w", err)
	}
	return target, nil
}

// RecordPendingDelivery records a deduplicated delivery and updates webhook activity.
func (r *SCMWebhookRepository) RecordPendingDelivery(ctx context.Context, webhookID int, deliveryID, eventType, summary string) (bool, error) {
	result, err := r.db.ExecWriteContext(ctx, `
		INSERT INTO scm_webhook_deliveries(scm_webhook_id, delivery_id, event_type, payload_summary, status)
		VALUES (?, ?, ?, ?, 'pending') ON CONFLICT DO NOTHING
	`, webhookID, deliveryID, eventType, summary)
	if err != nil {
		return false, fmt.Errorf("record pending SCM webhook delivery: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read pending SCM webhook delivery result: %w", err)
	}
	if _, err := r.db.ExecWriteContext(ctx, `
		UPDATE scm_webhooks
		SET last_delivery_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, webhookID); err != nil {
		return false, fmt.Errorf("update SCM webhook activity: %w", err)
	}
	return inserted > 0, nil
}

// CompleteDelivery records the final delivery outcome.
func (r *SCMWebhookRepository) CompleteDelivery(
	ctx context.Context,
	webhookID int,
	deliveryID, status, errorMessage string,
	processingTime time.Duration,
) error {
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE scm_webhook_deliveries
		SET status = ?, error_message = ?, processing_time_ms = ?, updated_at = CURRENT_TIMESTAMP
		WHERE scm_webhook_id = ? AND delivery_id = ?
	`, status, errorMessage, processingTime.Milliseconds(), webhookID, deliveryID)
	if err != nil {
		return fmt.Errorf("complete SCM webhook delivery: %w", err)
	}
	return nil
}
