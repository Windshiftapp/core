-- Authentication policy tables

-- Add enrollment_required column to user_sessions
-- This tracks whether a user needs to complete passkey enrollment
ALTER TABLE user_sessions ADD COLUMN enrollment_required BOOLEAN DEFAULT 0;

-- Auth policy audit table for tracking policy-related events
CREATE TABLE IF NOT EXISTS auth_policy_audit (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    event_type TEXT NOT NULL,  -- 'enrollment_started', 'enrollment_completed', 'admin_fallback_used'
    policy_at_time TEXT NOT NULL,  -- The policy that was active when this event occurred
    ip_address TEXT,
    user_agent TEXT,
    details TEXT,  -- JSON for additional context
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_auth_policy_audit_user_id ON auth_policy_audit(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_policy_audit_event_type ON auth_policy_audit(event_type);
CREATE INDEX IF NOT EXISTS idx_auth_policy_audit_created_at ON auth_policy_audit(created_at);

-- Admin fallback rate limits table
-- Tracks password login attempts for admin users when stricter policies are active
CREATE TABLE IF NOT EXISTS admin_fallback_rate_limits (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    ip_address TEXT NOT NULL,
    attempts INTEGER DEFAULT 1,
    first_attempt_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    locked_until DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(user_id, ip_address)
);

CREATE INDEX IF NOT EXISTS idx_admin_fallback_user_id ON admin_fallback_rate_limits(user_id);
CREATE INDEX IF NOT EXISTS idx_admin_fallback_ip ON admin_fallback_rate_limits(ip_address);
CREATE INDEX IF NOT EXISTS idx_admin_fallback_locked ON admin_fallback_rate_limits(locked_until);
