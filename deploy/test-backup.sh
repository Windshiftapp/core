#!/usr/bin/env bash
# Focused Docker-free contract test for deploy/backup.sh.
set -euo pipefail
export WINDSHIFT_BACKUP_HEALTH_TIMEOUT=3

repo_root=$(cd "$(dirname "$0")/.." && pwd)
test_tmp=${TMPDIR:-/tmp}
test_root=$(mktemp -d "${test_tmp%/}/windshift-backup-test.XXXXXX")
lock_path=
cleanup() {
  if [ -n "$lock_path" ]; then rm -f "$lock_path/pid" 2>/dev/null || true; rmdir "$lock_path" 2>/dev/null || true; fi
  rm -rf "$test_root"
}
trap cleanup EXIT
state="$test_root/state"
bin="$test_root/bin"
mkdir -p "$state/windshift/data/attachments" "$bin"
printf 'original database\n' >"$state/windshift/data/windshift.db"
printf 'original upload\n' >"$state/windshift/data/attachments/file.txt"
printf 'backup-secret\n' >"$state/secret"
printf 'false\n' >"$state/stopped"

cat >"$bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
state=${FAKE_DOCKER_STATE:?}
printf '%s\n' "$*" >>"$state/docker-args"

if [ "$1" = compose ]; then
  shift
  case "$1" in
    ps)
      case "${4:-}" in
        windshift) printf 'windshift-id\n';;
        customdb) [ -f "$state/db-type" ] && printf 'customdb-id\n';;
        otherdb) [ -f "$state/db-type" ] && printf 'otherdb-id\n';;
      esac
      ;;
    stop) printf 'true\n' >"$state/stopped"; rm -f "$state/active-health-failure";;
    start)
      printf 'false\n' >"$state/stopped"
      [ ! -f "$state/arm-health-failure" ] || mv "$state/arm-health-failure" "$state/active-health-failure"
      ;;
    exec)
      if [[ "$*" == *'/windshift healthcheck'* ]] && [ -f "$state/active-health-failure" ]; then exit 1; fi
      if [[ "$*" == *'rm -f '* ]] && [[ "$*" == *'windshift-restore-'* ]] && [ -f "$state/fail-pg-cleanup" ]; then exit 1; fi
      if [[ "$*" == *'pg_dump '* ]]; then
        printf 'fake-postgresql-dump\n'
      elif [[ "$*" == *'pg_restore --list '* ]]; then
        grep -qx fake-postgresql-dump "$state/remote-dump"
      elif [[ "$*" == *'pg_restore --clean '* ]]; then
        [ ! -f "$state/fail-pg-restore" ] || exit 1
        grep -qx fake-postgresql-dump "$state/remote-dump"
      elif [[ "$*" == *'rm -f '* ]]; then
        rm -f "$state/remote-dump"
      else
        printf 'fake PostgreSQL command\n'
      fi
      ;;
    *) exit 2;;
  esac
  exit
fi

if [ "$1" = inspect ]; then
  if [ "$3" = '{{.State.Running}}' ]; then
    if [ "$(cat "$state/stopped")" = true ]; then printf 'false\n'; else printf 'true\n'; fi
  elif [ "$3" = '{{if .State.Health}}{{.State.Health.Status}}{{end}}' ]; then
    printf '\n'
  elif [[ "$3" == *'.Config.Cmd'* ]]; then
    [ ! -f "$state/cmd-args" ] || cat "$state/cmd-args"
  elif [[ "$3" == *'.Config.Entrypoint'* ]]; then
    if [ -f "$state/custom-entrypoint" ]; then printf '/custom-entrypoint\n'; else printf '/windshift\n'; fi
  elif [[ "$3" == *'__WINDSHIFT_MOUNT_END__'* ]]; then
    mount_source="$state/windshift/data"
    [ ! -f "$state/mount-source" ] || mount_source=$(cat "$state/mount-source")
    printf '/data\0%s\0' "$mount_source"
    [ ! -f "$state/nested-mount" ] || printf '/data/attachments\0%s\0' "$state/nested-attachments"
    printf '__WINDSHIFT_MOUNT_END__\0'
  elif [[ "$3" == *'com.docker.compose.project'* ]]; then
    printf '%s windshift\n' "${FAKE_COMPOSE_PROJECT:?}"
  elif [ "$4" = customdb-id ] || [ "$4" = otherdb-id ]; then
    if [[ "$3" == *'__WINDSHIFT_ENV_END__'* ]]; then
      printf '%s\0' POSTGRES_USER=windshift POSTGRES_DB=windshift
      printf '__WINDSHIFT_ENV_END__\0'
    else
      printf '%s\n' POSTGRES_USER=windshift POSTGRES_DB=windshift
    fi
  else
    db_path=/data/windshift.db
    plugin_dir=/data/plugins
    [ ! -f "$state/empty-db-path" ] || db_path=
    [ ! -f "$state/empty-plugin-dir" ] || plugin_dir=
    if [[ "$3" == *'__WINDSHIFT_ENV_END__'* ]]; then
      printf 'SSO_SECRET=%s\0' "$(cat "$state/secret")"
      printf 'DB_PATH=%s\0' "$db_path"
      printf '%s\0' ATTACHMENT_PATH=/data/attachments AI_PROMPTS_DIR=/data/prompts
      printf 'PLUGIN_DIR=%s\0' "$plugin_dir"
      [ ! -f "$state/db-type" ] || printf '%s\0' DB_TYPE=postgres "POSTGRES_HOST=$(cat "$state/pg-host")" POSTGRES_PORT=5432 POSTGRES_USER=windshift POSTGRES_DB=windshift
      [ ! -f "$state/connection" ] || printf '%s\0' POSTGRES_CONNECTION_STRING=postgres://unsupported
      printf '__WINDSHIFT_ENV_END__\0'
      exit
    fi
    printf 'SSO_SECRET=%s\n' "$(cat "$state/secret")"
    printf 'DB_PATH=%s\n' "$db_path"
    printf '%s\n' ATTACHMENT_PATH=/data/attachments AI_PROMPTS_DIR=/data/prompts
    printf 'PLUGIN_DIR=%s\n' "$plugin_dir"
    [ ! -f "$state/db-type" ] || printf '%s\n' DB_TYPE=postgres POSTGRES_HOST="$(cat "$state/pg-host")" POSTGRES_PORT=5432 POSTGRES_USER=windshift POSTGRES_DB=windshift
    [ ! -f "$state/connection" ] || printf '%s\n' POSTGRES_CONNECTION_STRING=postgres://unsupported
  fi
  exit
fi

if [ "$1" = cp ]; then
  if [ -f "$state/fail-pg-cp" ] && [[ "$3" == *'windshift-restore-'* ]]; then exit 1; fi
  cp "$2" "$state/remote-dump"
  exit 0
fi

if [ "$1" = run ]; then
  backup=
  script=
  for arg in "$@"; do
    case "$arg" in
      *:/backup|*:/backup:ro) backup=${arg%%:/backup*};;
      *'tar -C '*|*'tar -tzf '*) script=$arg;;
    esac
  done
  [ -n "$backup" ] || [[ "$script" == *'-czf -'* ]] || exit 2
  if [[ "$script" == *'-czf -'* ]]; then
    tar -C "$state/windshift/data" -czf - .
  elif [[ "$script" == *'-czf /backup/data.tar.gz'* ]]; then
    tar -C "$state/windshift/data" -czf "$backup/data.tar.gz" .
  elif [[ "$script" == *'-xzf /backup/data.tar.gz'* ]]; then
    if [ -f "$state/fail-extract-always" ]; then
      find "$state/windshift/data" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
      printf 'partial restore\n' >"$state/windshift/data/windshift.db"
      exit 1
    fi
    if [ -f "$state/fail-extract-once" ]; then
      rm -f "$state/fail-extract-once"
      find "$state/windshift/data" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
      printf 'partial restore\n' >"$state/windshift/data/windshift.db"
      exit 1
    fi
    find "$state/windshift/data" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
    tar -C "$state/windshift/data" -xzf "$backup/data.tar.gz"
  else
    translated=${script//\/backup/$backup}
    sh -ec "$translated" sh "${!#}"
  fi
  exit
fi
exit 2
EOF
chmod +x "$bin/docker"

export FAKE_DOCKER_STATE="$state"
export FAKE_COMPOSE_PROJECT="windshift-test-${test_root##*.}"
export WINDSHIFT_BACKUP_ROLLBACK_ROOT="$test_root"
export PATH="$bin:$PATH"
hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}
hash_stdin() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum | awk '{print $1}'; else shasum -a 256 | awk '{print $1}'; fi
}
lock_key=$(printf 'windshift-backup-lock-v1\0%s\0windshift' "$FAKE_COMPOSE_PROJECT" | hash_stdin)
grep -Fq -- '- PLUGIN_DIR=/data/plugins' "$repo_root/deploy/docker-compose-main.yml"
existing="$test_root/existing"
mkdir "$existing"
printf 'keep\n' >"$existing/marker"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$existing"; then
  printf 'backup over existing destination unexpectedly succeeded\n' >&2
  exit 1
fi
grep -qx keep "$existing/marker"
backup="$test_root/backup"
bash "$repo_root/deploy/backup.sh" backup --include-sso-secret --helper-image fake-helper "$backup"
grep -qx database_type=sqlite "$backup/manifest.env"
grep -qx sso_secret_included=true "$backup/manifest.env"
grep -qx backup-secret "$backup/sso-secret"
grep -Fq -- '--volumes-from windshift-id:ro' "$state/docker-args"
[ "$(cat "$state/stopped")" = false ]

printf 'line one\nline two\n' >"$state/secret"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/multiline-secret"; then
  printf 'backup with a multiline secret unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/multiline-secret" ]
[ "$(cat "$state/stopped")" = false ]
printf 'backup-secret\n' >"$state/secret"

printf '%s\n' '--postgres-replica-count=2' >"$state/cmd-args"
bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/allowed-cli"
printf '%s\n' '--db=/outside/windshift.db' >"$state/cmd-args"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/blocked-cli"; then
  printf 'backup with a CLI path override unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/blocked-cli" ]
rm "$state/cmd-args"

touch "$state/custom-entrypoint"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/custom-entrypoint"; then
  printf 'backup with a custom entrypoint unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/custom-entrypoint" ]
rm "$state/custom-entrypoint"

if bash "$repo_root/deploy/backup.sh" backup --data-path /other --helper-image fake-helper "$test_root/unmounted-data"; then
  printf 'backup with an unmounted data path unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/unmounted-data" ]

touch "$state/nested-mount"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/nested-mount"; then
  printf 'backup with a nested data mount unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/nested-mount" ]
rm "$state/nested-mount"

printf '/var/tmp\n' >"$state/mount-source"
lock_path="/var/tmp/windshift-backup-$lock_key.lock"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/lock-in-data-mount"; then
  printf 'backup with its lock inside the live data mount unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/lock-in-data-mount" ]
[ ! -e "$lock_path" ]
lock_path=
rm "$state/mount-source"

if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$state/windshift/data/nested-backup"; then
  printf 'backup inside the live data mount unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$state/windshift/data/nested-backup" ]

if bash "$repo_root/deploy/backup.sh" backup --rollback-root "$state/windshift/data" --helper-image fake-helper "$test_root/nested-rollback-root"; then
  printf 'backup with a rollback root inside the live data mount unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/nested-rollback-root" ]

touch "$state/empty-db-path"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/empty-db-path"; then
  printf 'backup with an empty SQLite DB_PATH unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/empty-db-path" ]
rm "$state/empty-db-path"

touch "$state/empty-plugin-dir"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/empty-plugin-dir"; then
  printf 'backup with an empty PLUGIN_DIR unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/empty-plugin-dir" ]
rm "$state/empty-plugin-dir"

if bash "$repo_root/deploy/backup.sh" backup --helper-image --privileged "$test_root/helper-option"; then
  printf 'backup with a helper image that looks like a Docker option unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/helper-option" ]

printf 'changed database\n' >"$state/windshift/data/windshift.db"

lock_path="/var/tmp/windshift-backup-$lock_key.lock"
mkdir "$lock_path"
mkdir "$test_root/alternate-rollback-root"
if bash "$repo_root/deploy/backup.sh" backup --rollback-root "$test_root/alternate-rollback-root" --helper-image fake-helper "$test_root/locked"; then
  printf 'backup with lock contention unexpectedly succeeded\n' >&2
  exit 1
fi
rmdir "$lock_path"
lock_path=

nested_lock="/var/tmp/windshift-backup-$lock_key.lock"
ln -s "$nested_lock" "$test_root/lock-alias"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/lock-alias/result"; then
  printf 'backup through a symlink to its lock directory unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$nested_lock" ]
rm "$test_root/lock-alias"

if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$nested_lock/result"; then
  printf 'backup below its own lock directory unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$nested_lock" ]

if bash "$repo_root/deploy/backup.sh" restore --helper-image fake-helper "$backup"; then
  printf 'restore without --force unexpectedly succeeded\n' >&2
  exit 1
fi
grep -qx 'changed database' "$state/windshift/data/windshift.db"

printf 'different-secret\n' >"$state/secret"
if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$backup"; then
  printf 'restore with a different secret unexpectedly succeeded\n' >&2
  exit 1
fi
grep -qx 'changed database' "$state/windshift/data/windshift.db"
[ "$(cat "$state/stopped")" = false ]
printf 'backup-secret\n' >"$state/secret"

unterminated_checksums="$test_root/unterminated-checksums"
cp -R "$backup" "$unterminated_checksums"
awk '$2 != "sso-secret" {print}' "$unterminated_checksums/checksums.sha256" >"$unterminated_checksums/checksums.next"
printf '%064d  sso-secret' 0 >>"$unterminated_checksums/checksums.next"
mv "$unterminated_checksums/checksums.next" "$unterminated_checksums/checksums.sha256"
if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$unterminated_checksums"; then
  printf 'restore with an unchecked unterminated checksum unexpectedly succeeded\n' >&2
  exit 1
fi
[ "$(cat "$state/stopped")" = false ]

duplicate_checksums="$test_root/duplicate-checksums"
cp -R "$backup" "$duplicate_checksums"
manifest_checksum=$(awk '$2 == "manifest.env" {print; exit}' "$duplicate_checksums/checksums.sha256")
printf '%s\n' "$manifest_checksum" >>"$duplicate_checksums/checksums.sha256"
if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$duplicate_checksums"; then
  printf 'restore with a duplicate checksum entry unexpectedly succeeded\n' >&2
  exit 1
fi
[ "$(cat "$state/stopped")" = false ]

touch "$state/fail-extract-once"
if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$backup"; then
  printf 'restore with extraction failure unexpectedly succeeded\n' >&2
  exit 1
fi
grep -qx 'changed database' "$state/windshift/data/windshift.db"
[ "$(cat "$state/stopped")" = false ]

printf 'true\n' >"$state/stopped"
touch "$state/fail-extract-once"
stopped_failure_error="$test_root/stopped-data-failure.err"
if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$backup" 2>"$stopped_failure_error"; then
  printf 'stopped restore with an extraction failure unexpectedly succeeded\n' >&2
  exit 1
fi
grep -Fq 'no health check was possible' "$stopped_failure_error"
stopped_failure_rollback=$(sed -n 's/.*Retained rollback: //p' "$stopped_failure_error" | tail -1)
[ -d "$stopped_failure_rollback" ]
grep -qx 'changed database' "$state/windshift/data/windshift.db"
[ "$(cat "$state/stopped")" = true ]
[ ! -e "$nested_lock" ]
if find "$test_root" -maxdepth 1 -type d -name 'windshift-restore-stage.*' -print -quit | grep -q .; then
  printf 'staging directory was retained after a stopped rollback\n' >&2
  exit 1
fi
rm -rf "$stopped_failure_rollback"
printf 'false\n' >"$state/stopped"

touch "$state/fail-extract-always"
data_failure_error="$test_root/data-failure.err"
lock_path="$nested_lock"
if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$backup" 2>"$data_failure_error"; then
  printf 'restore with a failed data rollback unexpectedly succeeded\n' >&2
  exit 1
fi
grep -Fq '/data rollback failed' "$data_failure_error"
grep -Fq 'recovery lock is retained' "$data_failure_error"
[ -d "$lock_path" ]
[ -f "$lock_path/pid" ]
[ "$(cat "$state/stopped")" = true ]
retained_rollback=$(sed -n 's/.*rollback=\([^ ]*\).*/\1/p' "$data_failure_error" | tail -1)
retained_stage=$(sed -n 's/.*staged=\([^ ]*\).*/\1/p' "$data_failure_error" | tail -1)
[ -d "$retained_rollback" ]
[ -d "$retained_stage" ]
rm -rf "$retained_rollback" "$retained_stage"
rm -f "$state/fail-extract-always" "$lock_path/pid"
rmdir "$lock_path"
lock_path=
printf 'changed database\n' >"$state/windshift/data/windshift.db"
printf 'false\n' >"$state/stopped"

bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$backup"
grep -qx 'original database' "$state/windshift/data/windshift.db"
grep -qx 'original upload' "$state/windshift/data/attachments/file.txt"
[ "$(cat "$state/stopped")" = false ]

printf 'state before failed readiness\n' >"$state/windshift/data/windshift.db"
touch "$state/arm-health-failure"
if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$backup"; then
  printf 'restore with a failing readiness probe unexpectedly succeeded\n' >&2
  exit 1
fi
grep -qx 'state before failed readiness' "$state/windshift/data/windshift.db"
[ "$(cat "$state/stopped")" = false ]

printf 'changed while stopped\n' >"$state/windshift/data/windshift.db"
printf 'true\n' >"$state/stopped"
stopped_output=$(bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$backup")
rollback_path=${stopped_output##*Retained rollback: }
[ -d "$rollback_path" ]
grep -qx 'original database' "$state/windshift/data/windshift.db"
[ "$(cat "$state/stopped")" = true ]
if find "$test_root" -maxdepth 1 -type d -name 'windshift-restore-stage.*' -print -quit | grep -q .; then
  printf 'staging directory was retained after a stopped-service restore\n' >&2
  exit 1
fi
rm -rf "$rollback_path"
printf 'false\n' >"$state/stopped"

rewrite_data_checksum() {
  local dir=$1 sum
  sum=$(hash_file "$dir/data.tar.gz")
  awk -v sum="$sum" '$2 == "data.tar.gz" {print sum "  " $2; next} {print}' "$dir/checksums.sha256" >"$dir/checksums.next"
  mv "$dir/checksums.next" "$dir/checksums.sha256"
}
mismatch="$test_root/sqlite-path-mismatch"
cp -R "$backup" "$mismatch"
mkdir "$mismatch/content"
tar -C "$mismatch/content" -xzf "$mismatch/data.tar.gz"
cp "$mismatch/content/windshift.db" "$mismatch/content/other.db"
tar -C "$mismatch/content" -czf "$mismatch/data.tar.gz" .
rm -rf "$mismatch/content"
rewrite_data_checksum "$mismatch"
awk -F= '$1 == "sqlite_db_path" {$0 = "sqlite_db_path=other.db"} {print}' "$mismatch/manifest.env" >"$mismatch/manifest.next"
mv "$mismatch/manifest.next" "$mismatch/manifest.env"
manifest_sum=$(hash_file "$mismatch/manifest.env")
awk -v sum="$manifest_sum" '$2 == "manifest.env" {print sum "  " $2; next} {print}' "$mismatch/checksums.sha256" >"$mismatch/checksums.next"
mv "$mismatch/checksums.next" "$mismatch/checksums.sha256"
if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$mismatch"; then
  printf 'restore with mismatched SQLite path unexpectedly succeeded\n' >&2
  exit 1
fi
[ "$(cat "$state/stopped")" = false ]

for kind in empty-db symlink hardlink duplicate; do
  bad="$test_root/$kind"
  cp -R "$backup" "$bad"
  mkdir "$bad/content"
  case "$kind" in
    empty-db) : >"$bad/content/windshift.db";;
    symlink) printf 'db\n' >"$bad/content/windshift.db"; ln -s windshift.db "$bad/content/link";;
    hardlink) printf 'db\n' >"$bad/content/windshift.db"; ln "$bad/content/windshift.db" "$bad/content/alias";;
    duplicate) printf 'db\n' >"$bad/content/windshift.db";;
  esac
  if [ "$kind" = duplicate ]; then tar -C "$bad/content" -czf "$bad/data.tar.gz" ./windshift.db ./windshift.db
  else tar -C "$bad/content" -czf "$bad/data.tar.gz" .
  fi
  rewrite_data_checksum "$bad"
  if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$bad"; then
    printf 'restore of %s archive unexpectedly succeeded\n' "$kind" >&2
    exit 1
  fi
  [ "$(cat "$state/stopped")" = false ]
done

touch "$state/connection"
if bash "$repo_root/deploy/backup.sh" backup --helper-image fake-helper "$test_root/unsupported-connection"; then
  printf 'backup with POSTGRES_CONNECTION_STRING unexpectedly succeeded\n' >&2
  exit 1
fi
[ ! -e "$test_root/unsupported-connection" ]
rm "$state/connection"

printf 'postgres\n' >"$state/db-type"
printf 'customdb\n' >"$state/pg-host"
postgres_backup="$test_root/postgres-backup"
bash "$repo_root/deploy/backup.sh" backup --postgres-service customdb --helper-image fake-helper "$postgres_backup"
grep -qx postgres_service=customdb "$postgres_backup/manifest.env"
grep -Fq 'pg_restore --list ' "$state/docker-args"
invalid_postgres="$test_root/invalid-postgres"
cp -R "$postgres_backup" "$invalid_postgres"
printf 'invalid dump\n' >"$invalid_postgres/database.dump"
database_sum=$(hash_file "$invalid_postgres/database.dump")
awk -v sum="$database_sum" '$2 == "database.dump" {print sum "  " $2; next} {print}' "$invalid_postgres/checksums.sha256" >"$invalid_postgres/checksums.next"
mv "$invalid_postgres/checksums.next" "$invalid_postgres/checksums.sha256"
invalid_extracts_before=$(grep -c -- '-xzf /backup/data.tar.gz' "$state/docker-args" || true)
if bash "$repo_root/deploy/backup.sh" restore --force --postgres-service customdb --helper-image fake-helper "$invalid_postgres"; then
  printf 'restore with a structurally invalid PostgreSQL dump unexpectedly succeeded\n' >&2
  exit 1
fi
[ "$(cat "$state/stopped")" = false ]
invalid_extracts_after=$(grep -c -- '-xzf /backup/data.tar.gz' "$state/docker-args" || true)
[ "$invalid_extracts_after" -eq "$invalid_extracts_before" ]
printf 'otherdb\n' >"$state/pg-host"
bash "$repo_root/deploy/backup.sh" restore --force --postgres-service otherdb --helper-image fake-helper "$postgres_backup"
grep -Fq 'exec -T otherdb sh -ec pg_restore' "$state/docker-args"
touch "$state/fail-pg-cleanup"
bash "$repo_root/deploy/backup.sh" restore --force --postgres-service otherdb --helper-image fake-helper "$postgres_backup"
[ "$(cat "$state/stopped")" = false ]
rm "$state/fail-pg-cleanup"

printf 'state before failed PostgreSQL readiness\n' >"$state/windshift/data/windshift.db"
pg_restores_before=$(grep -c 'pg_restore --clean ' "$state/docker-args" || true)
touch "$state/arm-health-failure"
if bash "$repo_root/deploy/backup.sh" restore --force --postgres-service otherdb --helper-image fake-helper "$postgres_backup"; then
  printf 'PostgreSQL restore with a failing readiness probe unexpectedly succeeded\n' >&2
  exit 1
fi
grep -qx 'state before failed PostgreSQL readiness' "$state/windshift/data/windshift.db"
[ "$(cat "$state/stopped")" = false ]
pg_restores_after=$(grep -c 'pg_restore --clean ' "$state/docker-args" || true)
[ "$pg_restores_after" -eq "$((pg_restores_before + 2))" ]

printf 'true\n' >"$state/stopped"
postgres_stopped_output=$(bash "$repo_root/deploy/backup.sh" restore --force --postgres-service otherdb --helper-image fake-helper "$postgres_backup")
postgres_rollback=${postgres_stopped_output##*Retained rollback: }
[ -d "$postgres_rollback" ]
grep -Eq '^[[:xdigit:]]{64}  data\.tar\.gz$' "$postgres_rollback/checksums.sha256"
grep -Eq '^[[:xdigit:]]{64}  database-before\.dump$' "$postgres_rollback/checksums.sha256"
[ "$(hash_file "$postgres_rollback/data.tar.gz")" = "$(awk '$2 == "data.tar.gz" {print $1}' "$postgres_rollback/checksums.sha256")" ]
[ "$(hash_file "$postgres_rollback/database-before.dump")" = "$(awk '$2 == "database-before.dump" {print $1}' "$postgres_rollback/checksums.sha256")" ]
rm -rf "$postgres_rollback"
printf 'false\n' >"$state/stopped"
rm "$state/db-type" "$state/pg-host"

printf tampered >>"$backup/data.tar.gz"
if bash "$repo_root/deploy/backup.sh" restore --force --helper-image fake-helper "$backup"; then
  printf 'restore of a tampered backup unexpectedly succeeded\n' >&2
  exit 1
fi
grep -qx 'original database' "$state/windshift/data/windshift.db"
[ "$(cat "$state/stopped")" = false ]

printf 'postgres\n' >"$state/db-type"
printf 'customdb\n' >"$state/pg-host"
touch "$state/fail-pg-cp"
printf 'state before PostgreSQL copy failure\n' >"$state/windshift/data/windshift.db"
if bash "$repo_root/deploy/backup.sh" restore --force --postgres-service customdb --helper-image fake-helper "$postgres_backup"; then
  printf 'restore with PostgreSQL copy failure unexpectedly succeeded\n' >&2
  exit 1
fi
grep -qx 'state before PostgreSQL copy failure' "$state/windshift/data/windshift.db"
[ "$(cat "$state/stopped")" = false ]
rm "$state/fail-pg-cp"

touch "$state/fail-pg-restore"
pg_failure_error="$test_root/pg-failure.err"
lock_path="$nested_lock"
if bash "$repo_root/deploy/backup.sh" restore --force --postgres-service customdb --helper-image fake-helper "$postgres_backup" 2>"$pg_failure_error"; then
  printf 'restore with a mutating PostgreSQL failure unexpectedly succeeded\n' >&2
  exit 1
fi
grep -Fq 'recovery lock is retained' "$pg_failure_error"
[ -d "$lock_path" ]
[ -f "$lock_path/pid" ]
[ "$(cat "$state/stopped")" = true ]
retained_rollback=$(sed -n 's/.*rollback=\([^ ]*\).*/\1/p' "$pg_failure_error" | tail -1)
retained_stage=$(sed -n 's/.*staged=\([^ ]*\).*/\1/p' "$pg_failure_error" | tail -1)
[ -d "$retained_rollback" ]
[ -d "$retained_stage" ]
rm -rf "$retained_rollback" "$retained_stage"
rm -f "$state/remote-dump" "$state/fail-pg-restore" "$lock_path/pid"
rmdir "$lock_path"
lock_path=
printf 'false\n' >"$state/stopped"
rm "$state/db-type" "$state/pg-host"
printf 'backup script focused tests passed\n'
