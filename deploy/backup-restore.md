# Backing up and restoring Windshift

`backup.sh` is a conservative host-side workflow for the stock Docker Compose deployments in this repository.
It is not a general-purpose Docker or database recovery tool.
It backs up the complete configured `/data` tree and, for the supported PostgreSQL shape, a custom-format logical dump.

The helper image is pinned to the Alpine digest used by `deploy/coding-agent/Dockerfile`.
It has no network, a read-only root filesystem, and no-new-privileges.
The backup helper receives Windshift volumes read-only.
The restore helper receives a writable data volume only after all preflight checks and a local rollback snapshot have completed.

## Supported scope

The workflow requires exactly one Compose `windshift` container using the stock `/windshift` entrypoint and exactly one persistent mount at the configured data path.
Additional mounts below that path are rejected because deleting a mounted child can leave both restore and rollback incomplete.
The backup directory and rollback root must be outside that mount's host source so an archive cannot include or delete itself.
Replicas are rejected.
It derives a stable, domain-separated hash from the Compose project and service labels and uses a system-wide mode-700 lock directory under `/var/tmp`.

SQLite is supported only when `DB_PATH` is below `/data`.
An empty `DB_PATH` is rejected because Windshift would otherwise fall back to a path outside the mounted data tree.
The database path is recorded in the manifest and must exist as a nonempty regular archive entry.
The script also checks `ATTACHMENT_PATH`, `PLUGIN_DIR`, `AI_PROMPTS_DIR`, `LLM_PROVIDERS_FILE`, and trimmed comma-separated `PLUGIN_DIRS` when configured.
An empty `PLUGIN_DIR` is rejected because Windshift would otherwise fall back to a relative path outside the data mount.
SSH host keys and SSH-enabled deployments are outside this workflow because the image-relative default path is not guaranteed to be under `/data`.
Any extra host mounts or persisted paths outside `/data` are out of scope and cause a safe failure when they are configured through these variables.

PostgreSQL is supported only when all of the following are true:

- `DB_TYPE=postgres` and `POSTGRES_CONNECTION_STRING` is unset.
- `POSTGRES_HOST` exactly equals the selected Compose service.
- `POSTGRES_PORT` is unset or 5432.
- `POSTGRES_USER` and `POSTGRES_DB` exactly match that service's container environment.
- Exactly one PostgreSQL service container exists.

The automatic service candidates are `postgres` and `db`.
Use `--postgres-service NAME` for another stock-style service.
On restore, that explicit option overrides the manifest value.
External databases, connection strings, non-default ports, custom database selection, and replicas are deliberately unsupported.
Any command-line use of `-postgres-connection-string`, `--postgres-connection-string`, `-pg-conn`, or `--pg-conn` is also out of scope and fails closed.
The path override flags `-db`, `--db`, `-attachment-path`, `--attachment-path`, `-llm-providers`, `--llm-providers`, `-ai-prompts-dir`, and `--ai-prompts-dir` are out of scope too.
Environment values containing CR or LF are rejected because this workflow cannot safely fingerprint or archive them.
Use a database-specific procedure for those deployments.

Use the same PostgreSQL major version for backup and restore.
Restore only into the same Windshift image and schema generation that produced the backup unless a separately verified migration plan says otherwise.

## Create a backup

Run from the Compose directory:

    cd deploy

    ./backup.sh backup /srv/windshift-backups/windshift-$(date -u +%Y%m%dT%H%M%SZ)

The backup path must be an absolute, normalized host path without a colon because it is passed to Docker's `-v` syntax.
The created directory is mode 700.
It contains `data.tar.gz`, `manifest.env`, and `checksums.sha256`.
PostgreSQL backups also contain `database.dump`.

The script takes an atomic lock before stopping the app or creating backup files.
If a lock exists, do not run another operation.
First confirm that no backup or restore is running, then remove only the reported stale lock directory.

SHA-256 checksums detect accidental corruption and incomplete copies.
They do not authenticate a backup.
Use access-controlled, authenticated, encrypted storage and transport.

The manifest stores a domain-separated SHA-256 fingerprint of the effective `SSO_SECRET`, or `SESSION_SECRET` when no `SSO_SECRET` is present.
This is a compatibility check, not a password verifier.
Weak secrets therefore still produce easily guessable fingerprints.

Pass `--include-sso-secret` only for an explicitly encrypted secret-recovery copy.
It writes the raw effective secret as mode-600 `sso-secret` inside the mode-700 backup and includes it in the checksum file.
This artifact is highly sensitive and must never be logged or copied to ordinary backup media.

## Restore

Run from the same Compose directory:

    cd deploy

    ./backup.sh restore --force /srv/windshift-backups/windshift-20260829T120000Z

Before stopping Windshift, restore rejects symlinked or non-regular input artifacts, copies approved artifacts into a private staging directory, and verifies only that staged snapshot thereafter.
The helper checks the staged archive in the same image that performs extraction.
It accepts only regular files and directories, rejects absolute and parent paths, and rejects symbolic links, hard links, devices, and FIFOs.

Restore compares the live secret fingerprint before any state change.
On mismatch it fails closed and never writes a secret into Compose.
Configure the source secret in Compose or the secret manager first.

After the app stops, the script creates a mode-700 rollback directory under `--rollback-root` or `WINDSHIFT_BACKUP_ROLLBACK_ROOT`.
The default is disk-backed `/var/tmp`.
That directory must already exist and be writable.
Choose a local, private filesystem with space for a full compressed `/data` copy, staging copy, and, for PostgreSQL, a second logical dump.
Do not put it on volatile `/tmp` unless its loss during reboot is acceptable.

For PostgreSQL the rollback contains both `/data` and a pre-restore database dump.
The script writes SHA-256 checksums for retained rollback artifacts and structurally validates custom-format PostgreSQL dumps with `pg_restore --list` before relying on them.
The target database restore uses `pg_restore --single-transaction --exit-on-error`.
It cleans and restores archive objects, not necessarily unrelated database objects.

The rollback is retained until the started application is healthy.
When Docker exposes a health check, the script waits for it for up to `WINDSHIFT_BACKUP_HEALTH_TIMEOUT` seconds, default 90.
Without a Docker health status, it executes `/windshift healthcheck` through Compose until the configured deadline.
Increase `WINDSHIFT_BACKUP_HEALTH_TIMEOUT` before restore when startup or migrations can legitimately take longer than 90 seconds.
If the target is unhealthy, it restores both rollback data and PostgreSQL before trying the old state.
If PostgreSQL state is uncertain because of an interruption or failed database command, Windshift remains stopped and the error retains both rollback and staged paths.
The recovery lock also remains in place because a detached server-side `pg_restore` may still be running.
Remove that lock only after confirming that no backup, restore, or PostgreSQL restore process is active and after resolving the database state.
If Windshift was already stopped before restore, the script cannot perform a health check or automatic start.
It reports success with the retained rollback path so the operator can validate a manual start before removing it.

The command uses a 60-second Compose stop timeout.
An interrupted command, host reboot, or Docker failure can still leave the service stopped.
Read the printed recovery paths before restarting it.
On a confirmed healthy restore, cleanup failures only warn and retain the private rollback or stage instead of changing the restored state.
The ephemeral backup helper streams its tar archive to a host-created mode-600 file, so it does not need to chown host files.
The Docker-free contract test does not prove behavior against a real Docker daemon, image, filesystem driver, or PostgreSQL server.

## Manual emergency recovery

This is a destructive emergency procedure.
Leave Windshift stopped and first copy every reported stage and rollback directory to immutable, access-controlled storage.
Use only the same Windshift image/schema generation and the same PostgreSQL major version.

Before stopping or deleting anything, validate the retained checksums and data archive in a separate preflight.
Use the SHA-256 tool available on the host and the exact pinned helper image shown below:

    cd deploy
    (cd "$ROLLBACK_DIR" && sha256sum -c checksums.sha256)
    docker run --rm --network none --read-only --security-opt no-new-privileges -v "$ROLLBACK_DIR:/backup:ro" alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d sh -ec '
      test -s /backup/data.tar.gz
      tar -tzf /backup/data.tar.gz >/dev/null
      tar -tvzf /backup/data.tar.gz >/dev/null
      tar -tzf /backup/data.tar.gz | awk '\''{
        path = $0
        if (path == "./") key = "."
        else {
          if (substr(path, 1, 2) != "./") exit 1
          path = substr(path, 3)
          if (path == "" || path ~ /(^|\/)\.($|\/)/ || path ~ /(^|\/)\.\.($|\/)/ || path ~ /\/\//) exit 1
          sub(/\/$/, "", path)
          if (path == "") exit 1
          key = path
        }
        if (seen[key]++) exit 1
        count++
      } END { if (!count) exit 1 }'\''
      if tar -tvzf /backup/data.tar.gz | grep -Eqv "^[-d]"; then exit 1; fi
      if tar -tvzf /backup/data.tar.gz | grep -E "^-.* -> "; then exit 1; fi
    '

If the host provides `shasum` instead, replace the checksum command with `shasum -a 256 -c checksums.sha256`.
Do not continue unless every command succeeds.

For PostgreSQL, validate the retained database dump before changing `/data`.
Set `POSTGRES_SERVICE` to the confirmed service name and keep `remote_dump` for the restore step below:

    POSTGRES_SERVICE=postgres
    db_id=$(docker compose ps --all -q "$POSTGRES_SERVICE")
    [ "$(printf '%s\n' "$db_id" | sed '/^$/d' | wc -l | tr -d ' ')" = 1 ] || exit 1
    remote_dump=/tmp/windshift-manual-rollback-$$.dump
    docker cp "$ROLLBACK_DIR/database-before.dump" "$db_id:$remote_dump"
    docker compose exec -T -e REMOTE_DUMP="$remote_dump" "$POSTGRES_SERVICE" sh -ec 'pg_restore --list "$REMOTE_DUMP" >/dev/null'

For `/data`, resolve exactly one Windshift container before stopping it:

    cd deploy
    windshift_id=$(docker compose ps --all -q windshift)
    [ "$(printf '%s\n' "$windshift_id" | sed '/^$/d' | wc -l | tr -d ' ')" = 1 ] || exit 1
    mount_destinations=$(docker inspect -f '{{range .Mounts}}{{println .Destination}}{{end}}' "$windshift_id")
    [ "$(printf '%s\n' "$mount_destinations" | grep -Fxc /data)" = 1 ] || exit 1
    if printf '%s\n' "$mount_destinations" | grep -Eq '^/data/'; then exit 1; fi
    docker compose stop --timeout 60 windshift || exit 1
    [ "$(docker compose ps --all -q windshift)" = "$windshift_id" ] || exit 1
    [ "$(docker inspect -f '{{.State.Running}}' "$windshift_id")" = false ] || exit 1
    docker run --rm --network none --read-only --security-opt no-new-privileges --volumes-from "$windshift_id" -v "$ROLLBACK_DIR:/backup:ro" alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d sh -ec 'find /data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +; tar -C /data -xzf /backup/data.tar.gz'

For PostgreSQL, restore the already validated remote dump only after confirming the target database again:

    docker compose exec -T -e REMOTE_DUMP="$remote_dump" "$POSTGRES_SERVICE" sh -ec 'pg_restore --clean --if-exists --no-owner --no-privileges --single-transaction --exit-on-error -U "$POSTGRES_USER" -d "$POSTGRES_DB" "$REMOTE_DUMP"'
    docker compose exec -T "$POSTGRES_SERVICE" rm -f "$remote_dump"

The PostgreSQL rollback cleans objects listed in its dump but may leave unrelated objects created by a newer schema.
Remove any container path printed as `remote=` after the database state has been recovered and the retained dump has been preserved elsewhere.
Only then start Windshift and run its health check.
