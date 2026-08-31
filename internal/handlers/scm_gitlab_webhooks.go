package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"
)

var gitLabWebhookEvents = []string{"push", "tag_push", "merge_request", "note", "release"}

type gitLabWebhookConfigResponse struct {
	Configured     bool       `json:"configured"`
	CallbackURL    string     `json:"callback_url,omitempty"`
	Secret         string     `json:"secret,omitempty"`
	Events         []string   `json:"events"`
	Active         bool       `json:"active"`
	LastDeliveryAt *time.Time `json:"last_delivery_at,omitempty"`
}

func (h *SCMItemLinksHandler) webhookRepoAdmin(w http.ResponseWriter, r *http.Request, repoID int) (string, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return "", false
	}
	var workspaceID int
	var providerType string
	err := h.db.QueryRowContext(r.Context(), `
		SELECT wsc.workspace_id, sp.provider_type
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wr.id = ?
	`, repoID).Scan(&workspaceID, &providerType)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "workspace_repository")
		return "", false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return "", false
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return "", false
	}
	return providerType, true
}

func (h *SCMItemLinksHandler) webhookCallbackURL(r *http.Request, key string) string {
	base := h.baseURL
	if base == "" {
		scheme := "https"
		if r.TLS == nil {
			if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
				scheme = forwarded
			} else {
				scheme = "http"
			}
		}
		host := r.Host
		if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
			host = forwarded
		}
		base = scheme + "://" + host
	}
	return strings.TrimRight(base, "/") + "/api/scm/webhooks/gitlab/" + key
}

// GetGitLabWebhookConfig returns manual setup information without exposing the secret.
func (h *SCMItemLinksHandler) GetGitLabWebhookConfig(w http.ResponseWriter, r *http.Request) {
	repoID, ok := requireIDParam(w, r, "repoId")
	if !ok {
		return
	}
	providerType, ok := h.webhookRepoAdmin(w, r, repoID)
	if !ok {
		return
	}
	if providerType != string(models.SCMProviderTypeGitLab) {
		respondBadRequest(w, r, "Webhooks on this endpoint are only available for GitLab repositories")
		return
	}
	var key string
	var active bool
	var lastDelivery sql.NullTime
	err := h.db.QueryRowContext(r.Context(), `SELECT webhook_key, is_active, last_delivery_at FROM scm_webhooks WHERE workspace_repository_id = ?`, repoID).Scan(&key, &active, &lastDelivery)
	if errors.Is(err, sql.ErrNoRows) {
		respondJSONOK(w, gitLabWebhookConfigResponse{Configured: false, Events: gitLabWebhookEvents})
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	response := gitLabWebhookConfigResponse{Configured: true, CallbackURL: h.webhookCallbackURL(r, key), Events: gitLabWebhookEvents, Active: active}
	if lastDelivery.Valid {
		response.LastDeliveryAt = &lastDelivery.Time
	}
	respondJSONOK(w, response)
}

func randomWebhookValue(bytesCount int) (string, error) {
	raw := make([]byte, bytesCount)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// RotateGitLabWebhookSecret creates or rotates the one-time manual setup secret.
func (h *SCMItemLinksHandler) RotateGitLabWebhookSecret(w http.ResponseWriter, r *http.Request) {
	repoID, ok := requireIDParam(w, r, "repoId")
	if !ok {
		return
	}
	providerType, ok := h.webhookRepoAdmin(w, r, repoID)
	if !ok {
		return
	}
	if providerType != string(models.SCMProviderTypeGitLab) {
		respondBadRequest(w, r, "Webhooks on this endpoint are only available for GitLab repositories")
		return
	}
	secret, err := randomWebhookValue(32)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	encrypted, err := h.encryption.Encrypt(secret)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	var key string
	err = h.db.QueryRowContext(r.Context(), `SELECT webhook_key FROM scm_webhooks WHERE workspace_repository_id = ?`, repoID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		key, err = randomWebhookValue(18)
		if err == nil {
			_, err = h.db.ExecWriteContext(r.Context(), `
				INSERT INTO scm_webhooks(workspace_repository_id, webhook_key, webhook_secret_encrypted, events, is_active)
				VALUES (?, ?, ?, ?, true)
			`, repoID, key, encrypted, `["push","tag_push","merge_request","note","release"]`)
		}
	} else if err == nil {
		_, err = h.db.ExecWriteContext(r.Context(), `UPDATE scm_webhooks SET webhook_secret_encrypted=?, is_active=true, updated_at=CURRENT_TIMESTAMP WHERE workspace_repository_id=?`, encrypted, repoID)
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, gitLabWebhookConfigResponse{Configured: true, CallbackURL: h.webhookCallbackURL(r, key), Secret: secret, Events: gitLabWebhookEvents, Active: true})
}

func (h *SCMItemLinksHandler) DeleteGitLabWebhookConfig(w http.ResponseWriter, r *http.Request) {
	repoID, ok := requireIDParam(w, r, "repoId")
	if !ok {
		return
	}
	if _, ok := h.webhookRepoAdmin(w, r, repoID); !ok {
		return
	}
	if _, err := h.db.ExecWriteContext(r.Context(), `DELETE FROM scm_webhooks WHERE workspace_repository_id = ?`, repoID); err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, map[string]bool{"deleted": true})
}

type gitLabWebhookPayload struct {
	ObjectKind string `json:"object_kind"`
	EventName  string `json:"event_name"`
	ProjectID  int64  `json:"project_id"`
	Ref        string `json:"ref"`
	Project    struct {
		ID                int64  `json:"id"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	ObjectAttributes struct {
		IID    int    `json:"iid"`
		Action string `json:"action"`
		Tag    string `json:"tag"`
	} `json:"object_attributes"`
}

func normalizeGitLabWebhookKind(payload gitLabWebhookPayload) string {
	kind := strings.ToLower(strings.TrimSpace(payload.ObjectKind))
	switch kind {
	case "push", "tag_push", "merge_request", "note", "release":
		return kind
	default:
		return ""
	}
}

// ReceiveGitLabWebhook validates and deduplicates a GitLab delivery, then
// schedules a targeted repository sync. Polling remains the recovery path.
func (h *SCMItemLinksHandler) ReceiveGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("webhookKey")
	var hookID, repoID int
	var encryptedSecret, externalProjectID, providerType string
	err := h.db.QueryRowContext(r.Context(), `
		SELECT sw.id, sw.workspace_repository_id, sw.webhook_secret_encrypted,
		       wr.repository_external_id, sp.provider_type
		FROM scm_webhooks sw
		JOIN workspace_repositories wr ON wr.id = sw.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE sw.webhook_key = ? AND sw.is_active = true AND wr.is_active = true
	`, key).Scan(&hookID, &repoID, &encryptedSecret, &externalProjectID, &providerType)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "scm_webhook")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if providerType != string(models.SCMProviderTypeGitLab) {
		respondBadRequest(w, r, "Invalid webhook provider")
		return
	}
	secret, err := h.encryption.Decrypt(encryptedSecret)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	provided := r.Header.Get("X-Gitlab-Token")
	if len(secret) != len(provided) || subtle.ConstantTimeCompare([]byte(secret), []byte(provided)) != 1 {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid webhook token"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondBadRequest(w, r, "Webhook payload is too large or unreadable")
		return
	}
	var payload gitLabWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		respondBadRequest(w, r, "Invalid GitLab webhook JSON")
		return
	}
	eventType := normalizeGitLabWebhookKind(payload)
	if eventType == "" {
		respondJSON(w, http.StatusAccepted, map[string]bool{"ignored": true})
		return
	}
	projectID := payload.Project.ID
	if projectID == 0 {
		projectID = payload.ProjectID
	}
	if strconv.FormatInt(projectID, 10) != externalProjectID {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "project mismatch"})
		return
	}
	deliveryID := strings.TrimSpace(r.Header.Get("X-Gitlab-Event-UUID"))
	if deliveryID == "" {
		digest := sha256.Sum256(body)
		deliveryID = hex.EncodeToString(digest[:])
	}
	summary, _ := json.Marshal(map[string]any{"object_kind": eventType, "project_id": projectID, "path": payload.Project.PathWithNamespace, "iid": payload.ObjectAttributes.IID, "action": payload.ObjectAttributes.Action, "ref": payload.Ref, "tag": payload.ObjectAttributes.Tag})
	result, err := h.db.ExecWriteContext(r.Context(), `
		INSERT INTO scm_webhook_deliveries(scm_webhook_id, delivery_id, event_type, payload_summary, status)
		VALUES (?, ?, ?, ?, 'pending') ON CONFLICT DO NOTHING
	`, hookID, deliveryID, eventType, string(summary))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	inserted, _ := result.RowsAffected()
	_, _ = h.db.ExecWriteContext(r.Context(), `UPDATE scm_webhooks SET last_delivery_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?`, hookID)
	respondJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
	if inserted == 0 {
		return
	}

	go func() {
		started := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		syncErr := h.syncService.SyncRepository(ctx, repoID)
		status, errorMessage := "processed", ""
		if syncErr != nil {
			status, errorMessage = "failed", syncErr.Error()
			slog.Warn("GitLab webhook sync failed", slog.Int("repository_id", repoID), slog.Any("error", syncErr))
		}
		_, updateErr := h.db.ExecWriteContext(ctx, `
			UPDATE scm_webhook_deliveries SET status=?, error_message=?, processing_time_ms=?, updated_at=CURRENT_TIMESTAMP
			WHERE scm_webhook_id=? AND delivery_id=?
		`, status, errorMessage, time.Since(started).Milliseconds(), hookID, deliveryID)
		if updateErr != nil {
			slog.Warn("GitLab webhook delivery update failed", slog.Any("error", updateErr), slog.String("delivery_id", deliveryID))
		}
	}()
}
