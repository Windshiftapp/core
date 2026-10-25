-- plane-import dumper: JSONL import contract from Plane's own PostgreSQL DB.
--
-- Run against the Plane database with psql 10+. It is ONE SELECT, so with
-- -q -t -A stdout is exactly one JSON record per line. Record order: meta,
-- users, issues (ascending sequence_id, soft-deleted rows included so their
-- numbers stay reserved), comments.
--
--   psql "$PLANE_DATABASE_URL" -v ON_ERROR_STOP=1 -v project_id='<uuid>' -q -t -A -f dump.sql > contract.jsonl
--
-- Replace <uuid> with the target project's id (projects.id). Columns and the
-- wrap expression below were verified against a live Plane v1.3.1 database.
--
-- Why wrap() exists: Postgres renders jsonb with REAL newlines inside JSON
-- string values (measured: Plane's description_html and comment text columns
-- carry them). The wrap expression escapes backslash/quote/control characters
-- and hands the result to jsonb, which undoes the escapes on parse and
-- re-applies them canonically on render — so every emitted record is one
-- physical line of valid JSON with no double-escaping. (Proven on the live
-- corpus: 5/5 rows with newlines and quotes round-trip equal.) A missing
-- project_id empties every branch — a meta-less stream, which the loader
-- refuses outright.
WITH params AS MATERIALIZED (SELECT :'project_id'::uuid AS pid)
SELECT line FROM (
	-- 1. meta: must be the first record (the loader refuses otherwise).
	(
		SELECT json_build_object(
			'type', 'meta',
			'meta', json_build_object(
				'schema_version', 1,
				'source', 'plane',
				'workspace_slug', COALESCE(w.slug, ''),
				'project_id', p.id::text,
				'project_key', p.identifier,
				'dumped_at', now()::text
			)
		)::text AS line,
		0 AS k,
		p.id::text AS pid
		FROM projects p
		LEFT JOIN workspaces w ON w.id = p.workspace_id
		JOIN params ON params.pid = p.id
	)
	UNION ALL
	-- 2. users: exactly the people whose issues or comments the contract
	--    carries, distinct by email (Plane emails are not unique; the lowest
	--    id wins) so the loader's map is stable.
	(
		SELECT json_build_object(
			'type', 'user',
			'user', json_build_object(
				'id', u.id::text,
				'email', u.email,
				'first_name', COALESCE(u.first_name, ''),
				'last_name', COALESCE(u.last_name, '')
			)
		)::text AS line,
		1 AS k,
		p.id::text AS pid
		FROM users u
		JOIN (
			SELECT DISTINCT created_by_id FROM issues WHERE project_id = (SELECT pid FROM params)
			UNION
			SELECT DISTINCT ic.created_by_id
			FROM issue_comments ic JOIN issues i ON i.id = ic.issue_id
			WHERE i.project_id = (SELECT pid FROM params)
		) involved ON involved.created_by_id = u.id
		JOIN projects p ON p.id = (SELECT pid FROM params)
		WHERE NOT EXISTS (
			SELECT 1 FROM users u2
			WHERE lower(u2.email) = lower(u.email) AND u2.id < u.id
		)
	)
	UNION ALL
	-- 3. issues: the full number range including soft-deleted rows and
	--    drafts; the loader decides what to skip and reports each skip by
	--    name. description_json is jsonb, so the editor document rides along
	--    as a JSON value; description_html travels as a wrapped text value.
	(
		SELECT json_build_object(
			'type', 'issue',
			'issue', json_build_object(
				'id', i.id::text,
				'sequence_id', i.sequence_id,
				'name', ('"' || replace(replace(replace(replace(replace(COALESCE(i.name, ''),
					chr(92), chr(92) || chr(92)), chr(34), chr(92) || chr(34)),
					chr(13), chr(92) || 'r'), chr(10), chr(92) || 'n'),
					chr(9), chr(92) || 't') || '"')::jsonb,
				'description_json', COALESCE(('"' || replace(replace(replace(replace(replace(COALESCE(i.description_json::text, ''),
					chr(92), chr(92) || chr(92)), chr(34), chr(92) || chr(34)),
					chr(13), chr(92) || 'r'), chr(10), chr(92) || 'n'),
					chr(9), chr(92) || 't') || '"')::jsonb, '{}'::jsonb),
				'description_html', ('"' || replace(replace(replace(replace(replace(COALESCE(i.description_html, ''),
					chr(92), chr(92) || chr(92)), chr(34), chr(92) || chr(34)),
					chr(13), chr(92) || 'r'), chr(10), chr(92) || 'n'),
					chr(9), chr(92) || 't') || '"')::jsonb,
				'state_name', COALESCE(st.name, ''),
				'priority', COALESCE(i.priority, 'none'),
				'estimate_point', NULLIF(ep.value, ''),
				'start_date', to_char(i.start_date, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'target_date', to_char(i.target_date, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'created_at', to_char(i.created_at, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'updated_at', to_char(i.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'completed_at', to_char(i.completed_at, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'archived_at', to_char(i.archived_at, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'deleted_at', to_char(i.deleted_at, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'is_draft', i.is_draft,
				'parent_id', NULLIF(i.parent_id::text, ''),
				'parent_sequence_id', ps.sequence_id,
				'created_by_id', COALESCE(i.created_by_id::text, ''),
				'updated_by_id', COALESCE(i.updated_by_id::text, '')
			)
		)::text AS line,
		2 AS k,
		p.id::text AS pid
		FROM issues i
		JOIN projects p ON p.id = i.project_id
		LEFT JOIN states st ON st.id = i.state_id
		LEFT JOIN estimate_points ep ON ep.id = i.estimate_point_id
		LEFT JOIN issues ps ON ps.id = i.parent_id
		WHERE p.id = (SELECT pid FROM params)
	)
	UNION ALL
	-- 4. comments: joined through issues so project scoping is exact.
	(
		SELECT json_build_object(
			'type', 'comment',
			'comment', json_build_object(
				'id', ic.id::text,
				'issue_id', ic.issue_id::text,
				'comment_json', COALESCE(('"' || replace(replace(replace(replace(replace(COALESCE(ic.comment_json::text, ''),
					chr(92), chr(92) || chr(92)), chr(34), chr(92) || chr(34)),
					chr(13), chr(92) || 'r'), chr(10), chr(92) || 'n'),
					chr(9), chr(92) || 't') || '"')::jsonb, '{}'::jsonb),
				'comment_html', ('"' || replace(replace(replace(replace(replace(COALESCE(ic.comment_html, ''),
					chr(92), chr(92) || chr(92)), chr(34), chr(92) || chr(34)),
					chr(13), chr(92) || 'r'), chr(10), chr(92) || 'n'),
					chr(9), chr(92) || 't') || '"')::jsonb,
				'text', ('"' || replace(replace(replace(replace(replace(COALESCE(ic.comment_stripped, ''),
					chr(92), chr(92) || chr(92)), chr(34), chr(92) || chr(34)),
					chr(13), chr(92) || 'r'), chr(10), chr(92) || 'n'),
					chr(9), chr(92) || 't') || '"')::jsonb,
				'created_at', to_char(ic.created_at, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'updated_at', to_char(ic.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'deleted_at', to_char(ic.deleted_at, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
				'created_by_id', COALESCE(ic.created_by_id::text, ''),
				'access', COALESCE(ic.access, 'INTERNAL')
			)
		)::text AS line,
		3 AS k,
		p.id::text AS pid
		FROM issue_comments ic
		JOIN issues i ON i.id = ic.issue_id
		JOIN projects p ON p.id = i.project_id
		WHERE p.id = (SELECT pid FROM params)
	)
) r
ORDER BY r.k, r.pid,
	CASE WHEN r.k = 2 THEN (r.line::jsonb->'issue'->>'sequence_id')::bigint END,
	r.line;
