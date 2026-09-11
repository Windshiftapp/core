package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

var (
	ErrAgentSessionNotFound = errors.New("agent session not found")
	ErrAgentSessionArchived = errors.New("agent session is archived")
	ErrAgentSessionBusy     = errors.New("agent session already has an active turn")
)

type AgentConversationRepository struct {
	db database.Database
}

func NewAgentConversationRepository(db database.Database) *AgentConversationRepository {
	return &AgentConversationRepository{db: db}
}

func (r *AgentConversationRepository) EnsureGeneralSession(ctx context.Context, userID int) (*models.AgentSession, error) {
	session, err := r.findGeneral(ctx, userID)
	if err != nil || session != nil {
		return session, err
	}
	err = database.WithTx(r.db, func(tx database.Tx) error {
		var id int
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO agent_sessions(session_type, owner_user_id, title)
			VALUES (?, ?, ?)
			RETURNING id
		`, models.AgentSessionGeneral, userID, "General").Scan(&id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO agent_session_participants(session_id, user_id, participant_role)
			VALUES (?, ?, 'human')
		`, id, userID)
		return err
	})
	if err != nil {
		// A concurrent first request may have won the unique General-session
		// insert. Re-read before surfacing the constraint error.
		if session, readErr := r.findGeneral(ctx, userID); readErr == nil && session != nil {
			return session, nil
		}
		return nil, fmt.Errorf("create General agent session: %w", err)
	}
	return r.findGeneral(ctx, userID)
}

func (r *AgentConversationRepository) findGeneral(ctx context.Context, userID int) (*models.AgentSession, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM agent_sessions
		WHERE session_type = ? AND owner_user_id = ?
	`, models.AgentSessionGeneral, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.GetForParticipant(ctx, id, userID)
}

func (r *AgentConversationRepository) CreateStandardSession(ctx context.Context, ownerUserID, workspaceID int, profile *models.WorkspaceAgentBinding, title string) (*models.AgentSession, error) {
	if profile == nil || profile.ProfileType != models.AgentProfileStandard ||
		profile.Lifecycle != models.AgentLifecycleReady || profile.WorkspaceID != workspaceID {
		return nil, errors.New("create Standard session: profile is not Ready")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = profile.DisplayName
	}
	if title == "" {
		title = "Agent conversation"
	}
	if len(title) > 200 {
		title = title[:200]
	}
	var id int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO agent_sessions(
				session_type, owner_user_id, workspace_id, agent_profile_id, title
			) VALUES (?, ?, ?, ?, ?)
			RETURNING id
		`, models.AgentSessionStandard, ownerUserID, workspaceID, profile.ID, title).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO agent_session_participants(session_id, user_id, participant_role)
			VALUES (?, ?, 'human')
		`, id, ownerUserID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO agent_session_participants(session_id, user_id, participant_role)
			VALUES (?, ?, 'agent')
		`, id, profile.ActingUserID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("create Standard agent session: %w", err)
	}
	return r.GetForParticipant(ctx, id, ownerUserID)
}

func (r *AgentConversationRepository) GetForParticipant(ctx context.Context, sessionID, userID int) (*models.AgentSession, error) {
	var allowed int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_session_participants
		WHERE session_id = ? AND user_id = ?
	`, sessionID, userID).Scan(&allowed); err != nil {
		return nil, err
	}
	if allowed == 0 {
		return nil, ErrAgentSessionNotFound
	}
	return r.Get(ctx, sessionID)
}

// Get is the unscoped administrative read used only after system-admin
// middleware has authorized audit transcript resolution.
func (r *AgentConversationRepository) Get(ctx context.Context, sessionID int) (*models.AgentSession, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, session_type, owner_user_id, workspace_id, agent_profile_id,
		       title, archived_at, created_at, updated_at
		FROM agent_sessions WHERE id = ?
	`, sessionID)
	var session models.AgentSession
	var workspaceID, profileID sql.NullInt64
	var archivedAt sql.NullTime
	if err := row.Scan(&session.ID, &session.SessionType, &session.OwnerUserID,
		&workspaceID, &profileID, &session.Title, &archivedAt,
		&session.CreatedAt, &session.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAgentSessionNotFound
		}
		return nil, err
	}
	if workspaceID.Valid {
		v := int(workspaceID.Int64)
		session.WorkspaceID = &v
	}
	if profileID.Valid {
		v := int(profileID.Int64)
		session.AgentProfileID = &v
	}
	if archivedAt.Valid {
		session.ArchivedAt = &archivedAt.Time
	}
	participants, err := r.listParticipants(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	session.Participants = participants
	return &session, nil
}

func (r *AgentConversationRepository) listParticipants(ctx context.Context, sessionID int) ([]models.AgentSessionParticipant, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.user_id, p.participant_role, p.joined_at,
		       COALESCE(NULLIF(TRIM(u.first_name || ' ' || u.last_name), ''), u.username),
		       u.username, COALESCE(u.is_agent, false)
		FROM agent_session_participants p
		JOIN users u ON u.id = p.user_id
		WHERE p.session_id = ?
		ORDER BY CASE p.participant_role WHEN 'human' THEN 0 ELSE 1 END, p.user_id
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []models.AgentSessionParticipant
	for rows.Next() {
		var participant models.AgentSessionParticipant
		if err := rows.Scan(&participant.UserID, &participant.ParticipantRole,
			&participant.JoinedAt, &participant.DisplayName, &participant.Username,
			&participant.IsAgent); err != nil {
			return nil, err
		}
		out = append(out, participant)
	}
	return out, rows.Err()
}

func (r *AgentConversationRepository) ListForParticipant(ctx context.Context, userID int, includeArchived bool) ([]*models.AgentSession, error) {
	query := `
		SELECT s.id
		FROM agent_sessions s
		JOIN agent_session_participants p ON p.session_id = s.id
		WHERE p.user_id = ?
	`
	if !includeArchived {
		query += " AND s.archived_at IS NULL"
	}
	query += " ORDER BY s.updated_at DESC, s.id DESC"
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]*models.AgentSession, 0, len(ids))
	for _, id := range ids {
		session, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, nil
}

func (r *AgentConversationRepository) ArchiveOwnedStandard(ctx context.Context, sessionID, ownerUserID int) (bool, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_sessions
		SET archived_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND owner_user_id = ? AND session_type = ? AND archived_at IS NULL
	`, sessionID, ownerUserID, models.AgentSessionStandard)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *AgentConversationRepository) ListMessagesForParticipant(ctx context.Context, sessionID, userID, beforeID, limit int) ([]models.AgentMessage, error) {
	if _, err := r.GetForParticipant(ctx, sessionID, userID); err != nil {
		return nil, err
	}
	return r.listMessages(ctx, sessionID, 0, beforeID, limit)
}

func (r *AgentConversationRepository) ListMessagesForParticipantAfter(ctx context.Context, sessionID, userID, afterID, beforeID, limit int) ([]models.AgentMessage, error) {
	if _, err := r.GetForParticipant(ctx, sessionID, userID); err != nil {
		return nil, err
	}
	return r.listMessages(ctx, sessionID, afterID, beforeID, limit)
}

func (r *AgentConversationRepository) ListMessages(ctx context.Context, sessionID, beforeID, limit int) ([]models.AgentMessage, error) {
	if _, err := r.Get(ctx, sessionID); err != nil {
		return nil, err
	}
	return r.listMessages(ctx, sessionID, 0, beforeID, limit)
}

func (r *AgentConversationRepository) listMessages(ctx context.Context, sessionID, afterID, beforeID, limit int) ([]models.AgentMessage, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	query := `
		SELECT id, session_id, role, author_user_id, agent_run_id, content,
		       context_json, metadata_json, created_at
		FROM agent_messages
		WHERE session_id = ?
	`
	args := []any{sessionID}
	if afterID > 0 {
		query += " AND id > ?"
		args = append(args, afterID)
	}
	if beforeID > 0 {
		query += " AND id < ?"
		args = append(args, beforeID)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var reversed []models.AgentMessage
	for rows.Next() {
		var message models.AgentMessage
		var authorID, runID sql.NullInt64
		var contextJSON, metadataJSON sql.NullString
		if err := rows.Scan(&message.ID, &message.SessionID, &message.Role,
			&authorID, &runID, &message.Content, &contextJSON, &metadataJSON,
			&message.CreatedAt); err != nil {
			return nil, err
		}
		if authorID.Valid {
			v := int(authorID.Int64)
			message.AuthorUserID = &v
		}
		if runID.Valid {
			v := int(runID.Int64)
			message.AgentRunID = &v
		}
		message.ContextJSON = contextJSON.String
		message.MetadataJSON = metadataJSON.String
		reversed = append(reversed, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]models.AgentMessage, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out, nil
}

type BeginAgentTurnInput struct {
	SessionID           int
	SenderUserID        int
	SenderUsername      string
	ActingUserID        int
	WorkspaceID         int
	BindingID           *int
	JobKind             string
	ProfileVersion      int
	ProfileSnapshotJSON string
	GrantsJSON          string
	Content             string
	ContextJSON         string
}

type BegunAgentTurn struct {
	MessageID int
	RunID     int
}

// BeginTurn atomically persists the exact user turn, queued run, and
// correlation-only audit row. Execution must not begin unless it commits.
func (r *AgentConversationRepository) BeginTurn(ctx context.Context, in BeginAgentTurnInput) (*BegunAgentTurn, error) {
	var out BegunAgentTurn
	err := database.WithTx(r.db, func(tx database.Tx) error {
		var archivedAt sql.NullTime
		var participant int
		sessionQuery := `
			SELECT s.archived_at,
			       (SELECT COUNT(*) FROM agent_session_participants p
			        WHERE p.session_id = s.id AND p.user_id = ?)
			FROM agent_sessions s WHERE s.id = ?
		`
		if r.db.GetDriverName() == "postgres" {
			// Serialize turn admission on the session row. The partial unique
			// index remains the final invariant for every backend.
			sessionQuery += " FOR UPDATE"
		}
		if err := tx.QueryRowContext(ctx, sessionQuery, in.SenderUserID, in.SessionID).
			Scan(&archivedAt, &participant); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAgentSessionNotFound
			}
			return err
		}
		if participant == 0 {
			return ErrAgentSessionNotFound
		}
		if archivedAt.Valid {
			return ErrAgentSessionArchived
		}
		var active int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM agent_runs
			WHERE session_id = ? AND status IN (?, ?)
		`, strconv.Itoa(in.SessionID), models.AgentRunStatusQueued, models.AgentRunStatusRunning).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return ErrAgentSessionBusy
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO agent_messages(session_id, role, author_user_id, content, context_json)
			VALUES (?, 'user', ?, ?, ?)
			RETURNING id
		`, in.SessionID, in.SenderUserID, in.Content, nullStringArg(in.ContextJSON)).Scan(&out.MessageID); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO agent_runs(
				workspace_id, binding_id, status, job_kind, triggered_by_user_id,
				acting_user_id, root_initiator_user_id, immediate_trigger_user_id,
				chain_depth, session_id, profile_version, grants_json, profile_snapshot_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)
			RETURNING id
		`, in.WorkspaceID, nullIntArg(in.BindingID), models.AgentRunStatusQueued,
			in.JobKind, in.SenderUserID, in.ActingUserID, in.SenderUserID,
			in.SenderUserID, strconv.Itoa(in.SessionID), nullPositiveIntArg(in.ProfileVersion),
			nullStringArg(in.GrantsJSON), nullStringArg(in.ProfileSnapshotJSON)).Scan(&out.RunID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE agent_messages SET agent_run_id = ? WHERE id = ?
		`, out.RunID, out.MessageID); err != nil {
			return err
		}
		details, err := json.Marshal(map[string]any{
			"source":                    "agent_chat",
			"agent_session_id":          in.SessionID,
			"agent_message_id":          out.MessageID,
			"agent_run_id":              out.RunID,
			"root_initiator_user_id":    in.SenderUserID,
			"immediate_trigger_user_id": in.SenderUserID,
			"acting_user_id":            in.ActingUserID,
			"workspace_id":              in.WorkspaceID,
		})
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO audit_logs(
				timestamp, user_id, username, action_type, resource_type,
				resource_id, resource_name, details, success
			) VALUES (?, ?, ?, 'agent.chat.turn', 'agent_session', ?, 'Agent chat turn', ?, true)
		`, time.Now().UTC(), in.SenderUserID, in.SenderUsername, in.SessionID, string(details)); err != nil {
			return fmt.Errorf("persist agent turn audit: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE agent_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?
		`, in.SessionID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AgentConversationRepository) CompleteTurn(ctx context.Context, sessionID, runID, authorUserID int, content, metadataJSON string) (int, error) {
	var messageID int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO agent_messages(
				session_id, role, author_user_id, agent_run_id, content, metadata_json
			) VALUES (?, 'assistant', ?, ?, ?, ?)
			RETURNING id
		`, sessionID, authorUserID, runID, content, nullStringArg(metadataJSON)).Scan(&messageID); err != nil {
			return err
		}
		now := time.Now().UTC()
		res, err := tx.ExecContext(ctx, `
			UPDATE agent_runs
			SET status = ?, ended_at = ?, error = NULL, updated_at = ?
			WHERE id = ? AND status = ?
		`, models.AgentRunStatusSucceeded, now, now, runID, models.AgentRunStatusRunning)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.New("complete agent turn: run is not running")
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE agent_sessions SET updated_at = ? WHERE id = ?
		`, now, sessionID)
		return err
	})
	return messageID, err
}

func (r *AgentConversationRepository) FailInterruptedRuns(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE agent_runs
		SET status = ?, ended_at = ?, error = ?, updated_at = ?
		WHERE session_id IS NOT NULL AND status IN (?, ?)
	`, models.AgentRunStatusFailed, now, "Agent chat interrupted by server restart", now,
		models.AgentRunStatusQueued, models.AgentRunStatusRunning)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
