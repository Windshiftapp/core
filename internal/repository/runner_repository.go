package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// RunnerRepository persists the remote-runner control-plane tables
// (runner_registration_tokens, runner_instances) for Initiative WI-141.
// Only hashes are stored; the service layer owns plaintext generation.
type RunnerRepository struct {
	db database.Database
}

// NewRunnerRepository constructs a new repository.
func NewRunnerRepository(db database.Database) *RunnerRepository {
	return &RunnerRepository{db: db}
}

// nullTimeArg returns a driver-friendly nil for a nil *time.Time.
func nullTimeArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// InsertRegistrationToken stores a new pool-scoped registration token (by
// hash) and returns its id.
func (r *RunnerRepository) InsertRegistrationToken(ctx context.Context, poolID int, tokenHash, tokenPrefix, description string, createdBy *int, expiresAt *time.Time) (int, error) {
	// RETURNING id (not LastInsertId) so this works on Postgres as well as
	// SQLite — the pq/pgx driver does not support LastInsertId.
	var id int64
	err := r.db.QueryRowWriteContext(ctx, `
		INSERT INTO runner_registration_tokens(pool_capability_id, token_hash, token_prefix, description, created_by_user_id, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		poolID, tokenHash, tokenPrefix, description, nullIntArg(createdBy), nullTimeArg(expiresAt),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert runner registration token: %w", err)
	}
	return int(id), nil
}

// IsEnabledRunnerPool reports whether poolID still names an enabled
// runner_pool capability. Runner control-plane rows intentionally use soft
// references so they remain available for audit after pool deletion; runtime
// registration and claim paths must therefore enforce this relationship.
func (r *RunnerRepository) IsEnabledRunnerPool(ctx context.Context, poolID int) (bool, error) {
	var enabled bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM action_capabilities
			WHERE id = ? AND capability_type = ? AND is_enabled = true
		)
	`, poolID, models.CapabilityRunnerPool).Scan(&enabled)
	if err != nil {
		return false, fmt.Errorf("check enabled runner pool: %w", err)
	}
	return enabled, nil
}

// GetActiveRegistrationTokenByHash returns the registration token matching
// the hash that is neither revoked nor expired as of now. Returns
// sql.ErrNoRows when no such active token exists.
func (r *RunnerRepository) GetActiveRegistrationTokenByHash(ctx context.Context, tokenHash string, now time.Time) (*models.RunnerRegistrationToken, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, pool_capability_id, token_prefix, description, created_by_user_id, created_at, expires_at, revoked_at
		FROM runner_registration_tokens
		WHERE token_hash = ?
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > ?)
	`, tokenHash, now)
	return scanRegistrationToken(row)
}

// RevokeRegistrationToken marks a registration token revoked. Idempotent:
// re-revoking leaves the original revoked_at in place.
func (r *RunnerRepository) RevokeRegistrationToken(ctx context.Context, id int, now time.Time) error {
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE runner_registration_tokens SET revoked_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, now, id)
	if err != nil {
		return fmt.Errorf("revoke runner registration token: %w", err)
	}
	return nil
}

// ConsumeRegistrationToken atomically marks an active registration token used
// (revoked) and reports whether THIS call performed the transition. A false
// return means the token was already used/revoked — a single-use registration
// token bootstraps exactly one runner, so the caller must then reject the
// registration (WI-238 security Phase 6). Race-safe: the UPDATE's
// `revoked_at IS NULL` guard means only one concurrent registration wins.
func (r *RunnerRepository) ConsumeRegistrationToken(ctx context.Context, id int, now time.Time) (bool, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE runner_registration_tokens SET revoked_at = ?
		WHERE id = ? AND revoked_at IS NULL
		  AND EXISTS (
			SELECT 1 FROM action_capabilities ac
			WHERE ac.id = runner_registration_tokens.pool_capability_id
			  AND ac.capability_type = ?
			  AND ac.is_enabled = true
		  )
	`, now, id, models.CapabilityRunnerPool)
	if err != nil {
		return false, fmt.Errorf("consume runner registration token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("consume runner registration token: rows affected: %w", err)
	}
	return n == 1, nil
}

// ListRegistrationTokensForPool returns every registration token for a pool
// (including revoked/expired ones) newest-first, for the admin lifecycle
// surface. Plaintext is never stored, so only the prefix is exposed.
func (r *RunnerRepository) ListRegistrationTokensForPool(ctx context.Context, poolID int) ([]*models.RunnerRegistrationToken, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, pool_capability_id, token_prefix, description, created_by_user_id, created_at, expires_at, revoked_at
		FROM runner_registration_tokens
		WHERE pool_capability_id = ?
		ORDER BY created_at DESC, id DESC
	`, poolID)
	if err != nil {
		return nil, fmt.Errorf("list runner registration tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*models.RunnerRegistrationToken
	for rows.Next() {
		tok, err := scanRegistrationToken(rows)
		if err != nil {
			return nil, fmt.Errorf("scan runner registration token: %w", err)
		}
		out = append(out, tok)
	}
	return out, rows.Err()
}

// ListInstancesForPool returns every runner instance for a pool (including
// revoked ones) newest-first, for the admin lifecycle surface.
func (r *RunnerRepository) ListInstancesForPool(ctx context.Context, poolID int) ([]*models.RunnerInstance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, pool_capability_id, name, status, registered_at, last_heartbeat_at, revoked_at
		FROM runner_instances
		WHERE pool_capability_id = ?
		ORDER BY registered_at DESC, id DESC
	`, poolID)
	if err != nil {
		return nil, fmt.Errorf("list runner instances: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*models.RunnerInstance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan runner instance: %w", err)
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

// InsertInstance stores a newly-registered runner (by credential hash) in
// the active state and returns its id.
func (r *RunnerRepository) InsertInstance(ctx context.Context, poolID int, name, credentialHash string, now time.Time) (int, error) {
	// RETURNING id (not LastInsertId) for Postgres compatibility.
	var id int64
	err := r.db.QueryRowWriteContext(ctx, `
		INSERT INTO runner_instances(pool_capability_id, name, credential_hash, status, registered_at)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id
	`,
		poolID, name, credentialHash, models.RunnerInstanceStatusActive, now,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert runner instance: %w", err)
	}
	return int(id), nil
}

// GetActiveInstanceByCredentialHash returns the active runner instance whose
// credential hashes to the given value, or sql.ErrNoRows if none is active.
func (r *RunnerRepository) GetActiveInstanceByCredentialHash(ctx context.Context, credentialHash string) (*models.RunnerInstance, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, pool_capability_id, name, status, registered_at, last_heartbeat_at, revoked_at
		FROM runner_instances
		WHERE credential_hash = ? AND status = ? AND revoked_at IS NULL
	`, credentialHash, models.RunnerInstanceStatusActive)
	return scanInstance(row)
}

// TouchHeartbeat records a runner's liveness ping. Only active instances are
// updated, so a heartbeat from a revoked runner is a no-op.
func (r *RunnerRepository) TouchHeartbeat(ctx context.Context, id int, now time.Time) error {
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE runner_instances SET last_heartbeat_at = ?
		WHERE id = ? AND status = ?
	`, now, id, models.RunnerInstanceStatusActive)
	if err != nil {
		return fmt.Errorf("touch runner heartbeat: %w", err)
	}
	return nil
}

// RevokeInstance evicts a single runner. Idempotent.
func (r *RunnerRepository) RevokeInstance(ctx context.Context, id int, now time.Time) error {
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE runner_instances SET status = ?, revoked_at = ?
		WHERE id = ? AND status = ?
	`, models.RunnerInstanceStatusRevoked, now, id, models.RunnerInstanceStatusActive)
	if err != nil {
		return fmt.Errorf("revoke runner instance: %w", err)
	}
	return nil
}

// RevokeStaleInstances marks active runners whose heartbeat has gone stale
// (older than staleBefore, or never seen and registered before staleBefore)
// as revoked. Returns the number evicted. Kept as a repository primitive for
// explicit maintenance/admin use; the normal lease reaper no longer calls it
// automatically, so idle runners can restart with their persisted credential
// after the liveness window (WI-545).
func (r *RunnerRepository) RevokeStaleInstances(ctx context.Context, staleBefore, now time.Time) (int, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE runner_instances
		SET status = ?, revoked_at = ?
		WHERE status = ?
		  AND ((last_heartbeat_at IS NOT NULL AND last_heartbeat_at < ?)
		    OR (last_heartbeat_at IS NULL AND registered_at < ?))
	`,
		models.RunnerInstanceStatusRevoked, now,
		models.RunnerInstanceStatusActive, staleBefore, staleBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("revoke stale instances: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("revoke stale instances: rows affected: %w", err)
	}
	return int(n), nil
}

// ListLiveInstancesForPool returns the pool's active runners with a fresh
// heartbeat (at or after freshSince; a never-heartbeated instance counts as
// live only within its registration grace window), oldest-registered first
// so round-robin (WI-514) starts with the longest-waiting runner. Returned
// ordered by registered_at, id (both ASC) for a stable turn. Callers that
// only need the count should prefer CountLiveInstancesForPool.
func (r *RunnerRepository) ListLiveInstancesForPool(ctx context.Context, poolID int, freshSince time.Time) ([]*models.RunnerInstance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, pool_capability_id, name, status, registered_at, last_heartbeat_at, revoked_at
		FROM runner_instances
		WHERE pool_capability_id = ? AND status = ?
		  AND ((last_heartbeat_at IS NOT NULL AND last_heartbeat_at >= ?)
		    OR (last_heartbeat_at IS NULL AND registered_at >= ?))
		ORDER BY registered_at ASC, id ASC
	`, poolID, models.RunnerInstanceStatusActive, freshSince, freshSince)
	if err != nil {
		return nil, fmt.Errorf("list live instances for pool: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*models.RunnerInstance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan live runner instance: %w", err)
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

// CountLiveInstancesForPool counts the pool's active runners with a fresh
// heartbeat (at or after freshSince; a never-heartbeated instance counts as
// live only within its registration grace window). Observability companion
// to RevokeStaleInstances: "how many runners could actually claim work right
// now" — used for stall diagnostics and the agent-presence UI.
func (r *RunnerRepository) CountLiveInstancesForPool(ctx context.Context, poolID int, freshSince time.Time) (int, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM runner_instances
		WHERE pool_capability_id = ? AND status = ?
		  AND ((last_heartbeat_at IS NOT NULL AND last_heartbeat_at >= ?)
		    OR (last_heartbeat_at IS NULL AND registered_at >= ?))
	`, poolID, models.RunnerInstanceStatusActive, freshSince, freshSince)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count live instances for pool: %w", err)
	}
	return n, nil
}

func scanRegistrationToken(row interface{ Scan(...any) error }) (*models.RunnerRegistrationToken, error) {
	tok := &models.RunnerRegistrationToken{}
	var description sql.NullString
	var createdBy sql.NullInt64
	var expiresAt, revokedAt sql.NullTime
	if err := row.Scan(
		&tok.ID, &tok.PoolCapabilityID, &tok.TokenPrefix, &description,
		&createdBy, &tok.CreatedAt, &expiresAt, &revokedAt,
	); err != nil {
		return nil, err
	}
	if description.Valid {
		tok.Description = description.String
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		tok.CreatedByUserID = &v
	}
	if expiresAt.Valid {
		tok.ExpiresAt = &expiresAt.Time
	}
	if revokedAt.Valid {
		tok.RevokedAt = &revokedAt.Time
	}
	return tok, nil
}

func scanInstance(row interface{ Scan(...any) error }) (*models.RunnerInstance, error) {
	inst := &models.RunnerInstance{}
	var name sql.NullString
	var lastHeartbeat, revokedAt sql.NullTime
	if err := row.Scan(
		&inst.ID, &inst.PoolCapabilityID, &name, &inst.Status,
		&inst.RegisteredAt, &lastHeartbeat, &revokedAt,
	); err != nil {
		return nil, err
	}
	if name.Valid {
		inst.Name = name.String
	}
	if lastHeartbeat.Valid {
		inst.LastHeartbeatAt = &lastHeartbeat.Time
	}
	if revokedAt.Valid {
		inst.RevokedAt = &revokedAt.Time
	}
	return inst, nil
}
