-- Logbook schema for PostgreSQL
-- This schema is managed by the logbook binary, not the main Windshift server.

-- Buckets: top-level knowledge containers
CREATE TABLE IF NOT EXISTS logbook_buckets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    workspace_id    INTEGER,
    created_by      INTEGER NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Configuration
    max_age_days       INTEGER,
    approval_required  BOOLEAN NOT NULL DEFAULT false,
    portal_visible     BOOLEAN NOT NULL DEFAULT false,
    email_address      TEXT NOT NULL DEFAULT '',
    default_authority  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_logbook_buckets_workspace ON logbook_buckets(workspace_id) WHERE workspace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_logbook_buckets_created_by ON logbook_buckets(created_by);

-- Bucket permissions: principal-based access control
CREATE TABLE IF NOT EXISTS logbook_bucket_permissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket_id       UUID NOT NULL REFERENCES logbook_buckets(id) ON DELETE CASCADE,
    principal_type  TEXT NOT NULL CHECK (principal_type IN ('user', 'group')),
    principal_id    INTEGER NOT NULL,
    permission      TEXT NOT NULL CHECK (permission IN ('bucket.view', 'bucket.edit', 'bucket.admin')),

    UNIQUE (bucket_id, principal_type, principal_id, permission)
);

CREATE INDEX IF NOT EXISTS idx_logbook_bucket_perms_bucket ON logbook_bucket_permissions(bucket_id);
CREATE INDEX IF NOT EXISTS idx_logbook_bucket_perms_principal ON logbook_bucket_permissions(principal_type, principal_id);

-- Documents: files, notes, emails ingested into buckets
CREATE TABLE IF NOT EXISTS logbook_documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket_id       UUID NOT NULL REFERENCES logbook_buckets(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    source_type     TEXT NOT NULL DEFAULT 'upload' CHECK (source_type IN ('upload', 'note', 'email')),
    source_ref      TEXT NOT NULL DEFAULT '',
    content_hash    TEXT NOT NULL DEFAULT '',
    raw_content     TEXT NOT NULL DEFAULT '',
    article         TEXT NOT NULL DEFAULT '',
    mime_type       TEXT NOT NULL DEFAULT '',
    file_path       TEXT NOT NULL DEFAULT '',
    author          TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'ready', 'error')),
    status_message  TEXT NOT NULL DEFAULT '',
    retrieval_count INTEGER NOT NULL DEFAULT 0,
    created_by      INTEGER NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_logbook_docs_bucket ON logbook_documents(bucket_id);
CREATE INDEX IF NOT EXISTS idx_logbook_docs_status ON logbook_documents(status);
CREATE INDEX IF NOT EXISTS idx_logbook_docs_content_hash ON logbook_documents(content_hash) WHERE content_hash != '';
CREATE INDEX IF NOT EXISTS idx_logbook_docs_created_by ON logbook_documents(created_by);
CREATE INDEX IF NOT EXISTS idx_logbook_docs_archived ON logbook_documents(archived_at) WHERE archived_at IS NULL;

-- Full-text search index on documents
CREATE INDEX IF NOT EXISTS idx_logbook_docs_fts ON logbook_documents
    USING GIN (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(raw_content, '') || ' ' || coalesce(article, '')));

-- Chunks: segments of documents for full-text search
CREATE TABLE IF NOT EXISTS logbook_chunks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id     UUID NOT NULL REFERENCES logbook_documents(id) ON DELETE CASCADE,
    position        INTEGER NOT NULL,
    content         TEXT NOT NULL,
    token_count     INTEGER NOT NULL DEFAULT 0,
    byte_start      INTEGER NOT NULL DEFAULT 0,
    byte_end        INTEGER NOT NULL DEFAULT 0,
    first_page      INTEGER,
    last_page       INTEGER,
    summary         TEXT NOT NULL DEFAULT '',
    tags            TEXT[] NOT NULL DEFAULT '{}',
    retrieval_count INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_logbook_chunks_document ON logbook_chunks(document_id);
CREATE INDEX IF NOT EXISTS idx_logbook_chunks_position ON logbook_chunks(document_id, position);

-- Thumbnail support for documents
ALTER TABLE logbook_documents ADD COLUMN IF NOT EXISTS has_thumbnail BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE logbook_documents ADD COLUMN IF NOT EXISTS thumbnail_path TEXT NOT NULL DEFAULT '';

-- Document classification and content cleaning
ALTER TABLE logbook_documents ADD COLUMN IF NOT EXISTS content_type TEXT NOT NULL DEFAULT '';
ALTER TABLE logbook_documents ADD COLUMN IF NOT EXISTS cleaned_content TEXT NOT NULL DEFAULT '';

-- Review tracking for document health
ALTER TABLE logbook_documents ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
ALTER TABLE logbook_documents ADD COLUMN IF NOT EXISTS reviewed_by INTEGER;

-- Attachments: files attached to logbook documents (images pasted into notes, etc.)
CREATE TABLE IF NOT EXISTS logbook_attachments (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id       UUID NOT NULL REFERENCES logbook_documents(id) ON DELETE CASCADE,
    bucket_id         UUID NOT NULL,
    filename          TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    file_path         TEXT NOT NULL,
    mime_type         TEXT NOT NULL DEFAULT '',
    file_size         BIGINT NOT NULL DEFAULT 0,
    uploaded_by       INTEGER,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_logbook_attachments_document ON logbook_attachments(document_id);
CREATE INDEX IF NOT EXISTS idx_logbook_attachments_bucket ON logbook_attachments(bucket_id);

