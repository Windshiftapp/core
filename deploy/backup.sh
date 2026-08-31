#!/usr/bin/env bash
set -euo pipefail

PROGRAM=${0##*/}
readonly FORMAT_VERSION=3 SERVICE=windshift
readonly DEFAULT_HELPER_IMAGE='alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d'
COMPOSE=(docker compose)
DATA_PATH=/data POSTGRES_SERVICE= HELPER_IMAGE=${WINDSHIFT_BACKUP_HELPER_IMAGE:-$DEFAULT_HELPER_IMAGE}
ROLLBACK_ROOT=${WINDSHIFT_BACKUP_ROLLBACK_ROOT:-/var/tmp}
INCLUDE_SSO_SECRET=false ACTION= FORCE=false BACKUP_PATH=
BACKUP_DEST_CREATED=false BACKUP_READY=false
WINDSHIFT_CONTAINER= DATA_MOUNT_SOURCE= WAS_RUNNING=false LOCK_DIR= LOCK_PHYSICAL= STAGE_DIR= ROLLBACK_DIR= ROLLBACK_CHECKSUM=
DB_ROLLBACK= DB_ROLLBACK_CHECKSUM= PHASE=preflight CLEANUP_RUNNING=false SQLITE_DB_REL= LIVE_TYPE= PG_SERVICE= result=
HEALTH_TIMEOUT=${WINDSHIFT_BACKUP_HEALTH_TIMEOUT:-90} PG_REMOTE= PG_REMOTE_SERVICE=

die() { printf '%s: %s\n' "$PROGRAM" "$*" >&2; exit 1; }
warn() { printf '%s: warning: %s\n' "$PROGRAM" "$*" >&2; }
usage() { cat <<EOF
Usage:
  $PROGRAM backup [options] BACKUP_DIRECTORY
  $PROGRAM restore [options] --force BACKUP_DIRECTORY

Options:
  --compose-file PATH       Compose file to use
  --data-path PATH          Persistent container path, default /data
  --postgres-service NAME   Stock-Compose PostgreSQL service
  --helper-image IMAGE      OCI image with tar and a POSIX shell
  --rollback-root PATH      Private, disk-backed host directory, default /var/tmp
  --include-sso-secret      Store the effective secret as a highly sensitive backup file
  --force                   Required for restore
EOF
}
need() { command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"; }
sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}
sha256_stdin() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum | awk '{print $1}'
  else shasum -a 256 | awk '{print $1}'; fi
}
valid_container_path() {
  case "$1" in /*) ;; *) return 1;; esac
  case "$1/" in *'/../'*|*'/./'*|*'//'*) return 1;; esac
  case "$1" in *$'\n'*|*$'\r'*|*$'\t'*) return 1;; esac
}
normalise_container_path() {
  local p=$1
  while [ "$p" != / ] && [ "${p%/}" != "$p" ]; do p=${p%/}; done
  valid_container_path "$p" || return 1
  [ "$p" != / ] || return 1
  printf '%s\n' "$p"
}
normalise_host_path() {
  local p=$1
  case "$p" in /*) ;; *) return 1;; esac
  while [ "$p" != / ] && [ "${p%/}" != "$p" ]; do p=${p%/}; done
  case "$p" in *:*) return 1;; esac
  case "/${p#/}/" in *'/./'*|*'/../'*|*'//'*) return 1;; esac
  case "$p" in *$'\n'*|*$'\r'*|*$'\t'*) return 1;; esac
  printf '%s\n' "$p"
}
physical_existing_dir() { (cd -P -- "$1" 2>/dev/null && pwd -P); }
physical_new_child_path() {
  local path=$1 parent name physical_parent
  parent=${path%/*}; name=${path##*/}
  [ -n "$parent" ] || parent=/
  [ -n "$name" ] || return 1
  physical_parent=$(physical_existing_dir "$parent") || return 1
  printf '%s/%s\n' "${physical_parent%/}" "$name"
}
env_value() {
  local entry key=$2 value= found=false sentinel=false
  while IFS= read -r -d '' entry; do
    if [ "$entry" = __WINDSHIFT_ENV_END__ ]; then sentinel=true; continue; fi
    case "$entry" in "$key"=*) [ "$found" = false ] || return 1; value=${entry#*=}; found=true;; esac
  done < <(docker inspect -f '{{range .Config.Env}}{{printf "%s\x00" .}}{{end}}{{printf "__WINDSHIFT_ENV_END__\x00"}}' "$1") || return 1
  [ "$sentinel" = true ] || return 1
  case "$value" in *$'\r'*|*$'\n'*) return 1;; esac
  printf '%s' "$value"
}
read_env() {
  local value
  value=$(env_value "$1" "$2") || return 1
  printf '%s' "$value"
}
container() {
  local ids count
  ids=$("${COMPOSE[@]}" ps --all -q "$1") || return 1
  count=$(printf '%s\n' "$ids" | sed '/^$/d' | wc -l | tr -d ' ') || return 1
  [ "$count" = 1 ] || return 1
  printf '%s\n' "$ids"
}
require_container() {
  local id
  id=$(container "$1") || die "expected exactly one Compose container for '$1'; replicas and missing services are unsupported"
  printf '%s\n' "$id"
}
container_running_state() {
  local state
  state=$(docker inspect -f '{{.State.Running}}' "$1") || return 1
  case "$state" in true|false) printf '%s\n' "$state";; *) return 1;; esac
}
running() { [ "$(container_running_state "$1")" = true ]; }
stopped() { [ "$(container_running_state "$1")" = false ]; }
health_status() { docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$1"; }
data_mount_present() {
  local field destination= expect=destination count=0 nested=false sentinel=false
  DATA_MOUNT_SOURCE=
  while IFS= read -r -d '' field; do
    if [ "$field" = __WINDSHIFT_MOUNT_END__ ]; then sentinel=true; continue; fi
    if [ "$expect" = destination ]; then
      destination=$field
      expect=source
    else
      if [ "$destination" = "$DATA_PATH" ]; then count=$((count + 1)); DATA_MOUNT_SOURCE=$field
      else case "$destination" in "$DATA_PATH"/*) nested=true;; esac
      fi
      expect=destination
    fi
  done < <(docker inspect -f '{{range .Mounts}}{{printf "%s\x00%s\x00" .Destination .Source}}{{end}}{{printf "__WINDSHIFT_MOUNT_END__\x00"}}' "$WINDSHIFT_CONTAINER") || return 1
  [ "$sentinel" = true ] && [ "$expect" = destination ] && [ "$count" = 1 ] && [ "$nested" = false ] || return 1
  DATA_MOUNT_SOURCE=$(normalise_host_path "$DATA_MOUNT_SOURCE") || return 1
  if [ -d "$DATA_MOUNT_SOURCE" ]; then DATA_MOUNT_SOURCE=$(physical_existing_dir "$DATA_MOUNT_SOURCE") || return 1; fi
  [ "$DATA_MOUNT_SOURCE" != / ]
}
require_outside_data_mount() {
  case "$1" in "$DATA_MOUNT_SOURCE"|"$DATA_MOUNT_SOURCE"/*) die "$2 must be outside the host source of --data-path ($DATA_MOUNT_SOURCE)";; esac
}
inside_data() {
  local value=$1 name=$2 normal
  [ -z "$value" ] && return 0
  normal=$(normalise_container_path "$value") || die "$name is not a safe absolute container path"
  case "$normal" in "$DATA_PATH"|"$DATA_PATH"/*) ;; *) die "$name ($value) is outside --data-path ($DATA_PATH)";; esac
}
trim() { sed 's/^[[:space:]]*//;s/[[:space:]]*$//' <<<"$1"; }
data_relative_path() {
  local path=$1 rel
  path=$(normalise_container_path "$path") || return 1
  inside_data "$path" DB_PATH
  [ "$path" != "$DATA_PATH" ] || return 1
  rel=${path#"$DATA_PATH"/}
  case "$rel" in ''|/*|*'//'|*'..'*|*[!A-Za-z0-9._/-]*) return 1;; esac
  printf '%s\n' "$rel"
}
effective_secret() {
  local sso session
  sso=$(read_env "$WINDSHIFT_CONTAINER" SSO_SECRET) || return 1
  session=$(read_env "$WINDSHIFT_CONTAINER" SESSION_SECRET) || return 1
  if [ -n "$sso" ]; then printf '%s' "$sso"
  elif [ -n "$session" ]; then printf '%s' "$session"
  else return 1; fi
}
secret_fingerprint() {
  local secret
  secret=$(effective_secret) || return 1
  printf 'windshift-backup-sso-secret-v1\0%s' "$secret" | sha256_stdin
}
database_type() {
  local db_type connection entrypoint args ssh_enabled token
  db_type=$(read_env "$WINDSHIFT_CONTAINER" DB_TYPE) || return 1
  connection=$(read_env "$WINDSHIFT_CONTAINER" POSTGRES_CONNECTION_STRING) || return 1
  ssh_enabled=$(read_env "$WINDSHIFT_CONTAINER" SSH_ENABLED) || return 1
  [ -z "$connection" ] || return 2
  case "$ssh_enabled" in true|TRUE|True|1|yes|YES|Yes) return 3;; esac
  entrypoint=$(docker inspect -f '{{range .Config.Entrypoint}}{{println .}}{{end}}' "$WINDSHIFT_CONTAINER") || return 1
  [ "$entrypoint" = /windshift ] || return 3
  args=$(docker inspect -f '{{range .Config.Cmd}}{{println .}}{{end}}' "$WINDSHIFT_CONTAINER") || return 1
  while IFS= read -r token; do
    case "$token" in
      -postgres-connection-string|--postgres-connection-string|-postgres-connection-string=*|--postgres-connection-string=*|-pg-conn|--pg-conn|-pg-conn=*|--pg-conn=*|-ssh|--ssh|-ssh=*|--ssh=*|-db|--db|-db=*|--db=*|-attachment-path|--attachment-path|-attachment-path=*|--attachment-path=*|-llm-providers|--llm-providers|-llm-providers=*|--llm-providers=*|-ai-prompts-dir|--ai-prompts-dir|-ai-prompts-dir=*|--ai-prompts-dir=*) return 3;;
    esac
  done <<<"$args"
  case "$db_type" in ''|sqlite) printf sqlite;; postgres) printf postgres;; *) return 3;; esac
}
check_paths() {
  local type=$1 value path sqlite_path
  if [ "$type" = sqlite ]; then
    value=$(read_env "$WINDSHIFT_CONTAINER" DB_PATH) || return 1
    [ -n "$value" ] || return 1
    sqlite_path=$value
    SQLITE_DB_REL=$(data_relative_path "$sqlite_path") || return 1
  fi
  for path in ATTACHMENT_PATH PLUGIN_DIR AI_PROMPTS_DIR LLM_PROVIDERS_FILE; do
    value=$(read_env "$WINDSHIFT_CONTAINER" "$path") || return 1
    case "$path" in
      PLUGIN_DIR) [ -n "$value" ] || return 1; inside_data "$value" "$path";;
      *) [ -z "$value" ] || inside_data "$value" "$path";;
    esac
  done
  value=$(read_env "$WINDSHIFT_CONTAINER" PLUGIN_DIRS) || return 1
  while [ -n "$value" ]; do
    case "$value" in *,*) path=${value%%,*}; value=${value#*,};; *) path=$value; value=;; esac
    path=$(trim "$path")
    [ -z "$path" ] || inside_data "$path" PLUGIN_DIRS
  done
}
postgres_service() {
  local candidate
  if [ -n "$POSTGRES_SERVICE" ]; then valid_service_name "$POSTGRES_SERVICE" || return 1; require_container "$POSTGRES_SERVICE" >/dev/null; printf '%s\n' "$POSTGRES_SERVICE"; return; fi
  for candidate in postgres db; do
    if container "$candidate" >/dev/null 2>&1; then printf '%s\n' "$candidate"; return; fi
  done
  return 1
}
valid_service_name() {
  case "$1" in ''|-*|*[!A-Za-z0-9_.-]*) return 1;; esac
}
validate_postgres_service() {
  local service=$1 host port app_user app_db db_container db_user db_db
  host=$(read_env "$WINDSHIFT_CONTAINER" POSTGRES_HOST) || return 1
  port=$(read_env "$WINDSHIFT_CONTAINER" POSTGRES_PORT) || return 1
  app_user=$(read_env "$WINDSHIFT_CONTAINER" POSTGRES_USER) || return 1
  app_db=$(read_env "$WINDSHIFT_CONTAINER" POSTGRES_DB) || return 1
  [ "$host" = "$service" ] && { [ -z "$port" ] || [ "$port" = 5432 ]; } && [ -n "$app_user" ] && [ -n "$app_db" ] || return 1
  db_container=$(container "$service") || return 1
  db_user=$(read_env "$db_container" POSTGRES_USER) || return 1
  db_db=$(read_env "$db_container" POSTGRES_DB) || return 1
  [ "$app_user" = "$db_user" ] && [ "$app_db" = "$db_db" ]
}
acquire_lock() {
  local labels project service key candidate
  labels=$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}} {{index .Config.Labels "com.docker.compose.service"}}' "$WINDSHIFT_CONTAINER") || die "could not read Compose labels for lock"
  project=${labels%% *}; service=${labels#* }
  [ -n "$project" ] && [ "$service" = windshift ] || die "container is not an identifiable Compose windshift service"
  key=$(printf 'windshift-backup-lock-v1\0%s\0%s' "$project" "$service" | sha256_stdin) || die "could not hash lock identity"
  candidate="/var/tmp/windshift-backup-$key.lock"
  if ! mkdir -m 700 "$candidate" 2>/dev/null; then
    [ -e "$candidate" ] || die "could not create system lock directory: $candidate"
    die "backup/restore lock exists: $candidate. Confirm no operation is running, then remove only that stale lock directory"
  fi
  LOCK_DIR=$candidate
  LOCK_PHYSICAL=$(physical_existing_dir "$LOCK_DIR") || die "could not resolve newly created recovery lock"
  require_outside_data_mount "$LOCK_PHYSICAL" "backup/restore lock"
  printf '%s\n' "$$" >"$LOCK_DIR/pid"
}
release_lock() {
  if [ -n "$LOCK_DIR" ]; then
    if ! rm -f "$LOCK_DIR/pid"; then warn "could not remove lock owner file $LOCK_DIR/pid"
    elif ! rmdir "$LOCK_DIR"; then warn "could not remove nonempty lock $LOCK_DIR"
    fi
  fi
  LOCK_DIR= LOCK_PHYSICAL=
}
stop_app() {
  WAS_RUNNING=false
  case "$(container_running_state "$WINDSHIFT_CONTAINER")" in
    false) return 0;;
    true) WAS_RUNNING=true; "${COMPOSE[@]}" stop --timeout 60 "$SERVICE" || return 1; stopped "$WINDSHIFT_CONTAINER";;
    *) return 1;;
  esac
}
stop_for_rollback() {
  case "$(container_running_state "$WINDSHIFT_CONTAINER")" in
    false) return 0;;
    true) "${COMPOSE[@]}" stop --timeout 60 "$SERVICE" || return 1; stopped "$WINDSHIFT_CONTAINER";;
    *) return 1;;
  esac
}
start_and_verify() {
  [ "$WAS_RUNNING" = true ] || return 0
  "${COMPOSE[@]}" start "$SERVICE" || return 1
  local status deadline=$((SECONDS + HEALTH_TIMEOUT))
  status=$(health_status "$WINDSHIFT_CONTAINER") || return 1
  if [ -z "$status" ]; then
    while ((SECONDS < deadline)); do
      running "$WINDSHIFT_CONTAINER" || return 1
      "${COMPOSE[@]}" exec -T "$SERVICE" /windshift healthcheck && return 0
      ((SECONDS < deadline)) || break
      sleep 1
    done
    return 1
  fi
  while ((SECONDS < deadline)); do
    running "$WINDSHIFT_CONTAINER" || return 1
    status=$(health_status "$WINDSHIFT_CONTAINER") || return 1
    [ "$status" = healthy ] && return 0
    [ "$status" = unhealthy ] && return 1
    ((SECONDS < deadline)) || break
    sleep 1
  done
  return 1
}
archive_data() {
  local dest=$1 output="$1/data.tar.gz"
  (umask 077; : >"$output") || return 1
  chmod 600 "$output" || return 1
  docker run --rm --network none --read-only --security-opt no-new-privileges --volumes-from "$WINDSHIFT_CONTAINER:ro" "$HELPER_IMAGE" sh -ec 'tar -C "$1" -czf - .' sh "$DATA_PATH" >"$output"
}
verify_data_archive() {
  local source=$1 sqlite_rel=${2:-} helper
  read -r -d '' helper <<'EOF' || true
test -s /backup/data.tar.gz
tar -tzf /backup/data.tar.gz >/dev/null
tar -tvzf /backup/data.tar.gz >/dev/null
tar -tzf /backup/data.tar.gz | awk '
{
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
}
END { if (!count) exit 1 }
'
if tar -tzf /backup/data.tar.gz | grep -Eq '(^/|(^|/)\.\.($|/))'; then exit 1; fi
if tar -tvzf /backup/data.tar.gz | grep -Eqv '^[-d]'; then exit 1; fi
if tar -tvzf /backup/data.tar.gz | grep -E '^-.* -> '; then exit 1; fi
tar -tzf /backup/data.tar.gz | grep -q .
EOF
  if [ -n "$sqlite_rel" ]; then
    helper+=$'\nsqlite_path=$1\ntar -tzf /backup/data.tar.gz | grep -Fx "./$sqlite_path" >/dev/null\ntar -tvzf /backup/data.tar.gz | awk -v path="./$sqlite_path" \'$NF == path && substr($1, 1, 1) == "-" { found = 1 } END { exit !found }\'\ntar -xOf /backup/data.tar.gz "./$sqlite_path" | wc -c | awk \'{ bytes += $1 } END { exit !(bytes > 0) }\''
  fi
  docker run --rm --network none --read-only --security-opt no-new-privileges -v "$source:/backup:ro" "$HELPER_IMAGE" sh -ec "$helper" sh "$sqlite_rel"
}
extract_data() {
  local source=$1
  docker run --rm --network none --read-only --security-opt no-new-privileges --volumes-from "$WINDSHIFT_CONTAINER" -v "$source:/backup:ro" "$HELPER_IMAGE" sh -ec 'mkdir -p "$1"; find "$1" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +; tar -C "$1" -xzf /backup/data.tar.gz' sh "$DATA_PATH"
}
dump_postgres() {
  (umask 077; "${COMPOSE[@]}" exec -T "$2" sh -ec 'pg_dump --format=custom --no-owner --no-privileges -U "$POSTGRES_USER" "$POSTGRES_DB"' >"$1") && chmod 600 "$1"
}
restore_postgres() {
  local dump=$1 service=$2 db_container token remote
  [ -f "$dump" ] && [ ! -L "$dump" ] && [ -s "$dump" ] || return 2
  db_container=$(container "$service") || return 1
  token="$(date +%s)-$$-${RANDOM}"
  remote="/tmp/windshift-restore-$token.dump"
  PG_REMOTE=$remote
  PG_REMOTE_SERVICE=$service
  if ! docker cp "$dump" "$db_container:$remote"; then
    if "${COMPOSE[@]}" exec -T "$service" sh -ec "rm -f '$remote'"; then PG_REMOTE=; PG_REMOTE_SERVICE=; else warn "could not remove a possibly partial PostgreSQL dump $remote"; fi
    return 2
  fi
  if ! "${COMPOSE[@]}" exec -T "$service" sh -ec "pg_restore --list '$remote' >/dev/null"; then
    if "${COMPOSE[@]}" exec -T "$service" sh -ec "rm -f '$remote'"; then PG_REMOTE=; PG_REMOTE_SERVICE=; else warn "could not remove invalid PostgreSQL dump $remote"; fi
    return 2
  fi
  PHASE=pg-mutating
  "${COMPOSE[@]}" exec -T "$service" sh -ec "pg_restore --clean --if-exists --no-owner --no-privileges --single-transaction --exit-on-error -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\" '$remote'" || return 1
  if "${COMPOSE[@]}" exec -T "$service" sh -ec "rm -f '$remote'"; then PG_REMOTE=; PG_REMOTE_SERVICE=; else warn "could not remove temporary PostgreSQL dump $remote"; fi
  return 0
}
validate_postgres_dump() {
  local dump=$1 service=$2 db_container token remote valid=true
  [ -f "$dump" ] && [ ! -L "$dump" ] && [ -s "$dump" ] || return 1
  db_container=$(container "$service") || return 1
  token="$(date +%s)-$$-${RANDOM}"
  remote="/tmp/windshift-validate-$token.dump"
  PG_REMOTE=$remote
  PG_REMOTE_SERVICE=$service
  if ! docker cp "$dump" "$db_container:$remote"; then
    if "${COMPOSE[@]}" exec -T "$service" sh -ec "rm -f '$remote'"; then PG_REMOTE=; PG_REMOTE_SERVICE=; else warn "could not remove a possibly partial PostgreSQL validation dump $remote"; fi
    return 1
  fi
  "${COMPOSE[@]}" exec -T "$service" sh -ec "pg_restore --list '$remote' >/dev/null" || valid=false
  if "${COMPOSE[@]}" exec -T "$service" sh -ec "rm -f '$remote'"; then PG_REMOTE=; PG_REMOTE_SERVICE=; else warn "could not remove temporary PostgreSQL validation dump $remote"; return 1; fi
  [ "$valid" = true ]
}
remove_unmutated_pg_remote() {
  [ -n "$PG_REMOTE" ] || return 0
  [ -n "$PG_REMOTE_SERVICE" ] || return 1
  if "${COMPOSE[@]}" exec -T "$PG_REMOTE_SERVICE" sh -ec "rm -f '$PG_REMOTE'"; then PG_REMOTE=; PG_REMOTE_SERVICE=; return 0; fi
  return 1
}
manifest() {
  local dest=$1 type=$2 service=$3 fingerprint=$4 source=$5 included=$6
  {
    printf 'format_version=%s\ncreated_at=%s\ndatabase_type=%s\ndata_path=%s\npostgres_service=%s\nsecret_source=%s\nsecret_fingerprint=%s\nsso_secret_included=%s\nsqlite_db_path=%s\n' "$FORMAT_VERSION" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$type" "$DATA_PATH" "$service" "$source" "$fingerprint" "$included" "${SQLITE_DB_REL:-}"
  } >"$dest/manifest.env" && chmod 600 "$dest/manifest.env"
}
write_sso_secret() { local secret; secret=$(effective_secret) || return 1; printf '%s' "$secret" >"$1/sso-secret" && chmod 600 "$1/sso-secret"; }
checksums() {
  local dest=$1 type=$2 included=$3 f sum
  : >"$dest/checksums.sha256" || return 1
  for f in manifest.env data.tar.gz; do sum=$(sha256 "$dest/$f") || return 1; printf '%s  %s\n' "$sum" "$f" >>"$dest/checksums.sha256" || return 1; done
  if [ "$type" = postgres ]; then sum=$(sha256 "$dest/database.dump") || return 1; printf '%s  database.dump\n' "$sum" >>"$dest/checksums.sha256" || return 1; fi
  if [ "$included" = true ]; then sum=$(sha256 "$dest/sso-secret") || return 1; printf '%s  sso-secret\n' "$sum" >>"$dest/checksums.sha256" || return 1; fi
  chmod 600 "$dest/checksums.sha256"
}
manifest_value() { awk -F= -v key="$2" '$1 == key {print substr($0,length(key)+2); exit}' "$1"; }
require_regular_artifacts() {
  local source=$1 type=$2 included=$3 f
  for f in manifest.env checksums.sha256 data.tar.gz; do [ -f "$source/$f" ] && [ ! -L "$source/$f" ] || return 1; done
  [ "$type" != postgres ] || { [ -f "$source/database.dump" ] && [ ! -L "$source/database.dump" ]; } || return 1
  [ "$included" != true ] || { [ -f "$source/sso-secret" ] && [ ! -L "$source/sso-secret" ]; } || return 1
}
verify() {
  local source=$1 type included required expected file actual sqlite_rel
  local seen_manifest=0 seen_data=0 seen_database=0 seen_secret=0
  [ -f "$source/manifest.env" ] && [ ! -L "$source/manifest.env" ] || return 1
  [ "$(manifest_value "$source/manifest.env" format_version)" = "$FORMAT_VERSION" ] || return 1
  type=$(manifest_value "$source/manifest.env" database_type); included=$(manifest_value "$source/manifest.env" sso_secret_included)
  case "$type:$included" in sqlite:true|sqlite:false|postgres:true|postgres:false) ;; *) return 1;; esac
  grep -Eq '^[[:xdigit:]]{64}$' < <(manifest_value "$source/manifest.env" secret_fingerprint) || return 1
  sqlite_rel=$(manifest_value "$source/manifest.env" sqlite_db_path)
  [ "$type" != sqlite ] || { case "$sqlite_rel" in ''|*'..'*|*[!A-Za-z0-9._/-]*) return 1;; esac; }
  require_regular_artifacts "$source" "$type" "$included" || return 1
  for required in manifest.env data.tar.gz; do grep -Eq "^[[:xdigit:]]{64}  $required$" "$source/checksums.sha256" || return 1; done
  [ "$type" != postgres ] || grep -Eq '^[[:xdigit:]]{64}  database\.dump$' "$source/checksums.sha256" || return 1
  [ "$included" != true ] || grep -Eq '^[[:xdigit:]]{64}  sso-secret$' "$source/checksums.sha256" || return 1
  while read -r expected file || [ -n "$expected$file" ]; do
    case "$file" in
      manifest.env) seen_manifest=$((seen_manifest + 1)); [ "$seen_manifest" = 1 ] || return 1;;
      data.tar.gz) seen_data=$((seen_data + 1)); [ "$seen_data" = 1 ] || return 1;;
      database.dump) seen_database=$((seen_database + 1)); [ "$seen_database" = 1 ] || return 1;;
      sso-secret) seen_secret=$((seen_secret + 1)); [ "$seen_secret" = 1 ] || return 1;;
      *) return 1;;
    esac
    actual=$(sha256 "$source/$file") || return 1
    [ "$actual" = "$expected" ] || return 1
  done <"$source/checksums.sha256"
  [ "$seen_manifest" = 1 ] && [ "$seen_data" = 1 ] || return 1
  if [ "$type" = postgres ]; then [ "$seen_database" = 1 ] || return 1; else [ "$seen_database" = 0 ] || return 1; fi
  if [ "$included" = true ]; then [ "$seen_secret" = 1 ] || return 1; else [ "$seen_secret" = 0 ] || return 1; fi
  verify_data_archive "$source" "$sqlite_rel" || return 1
  printf '%s\n' "$type"
}
stage_backup() {
  local source=$1 type included f
  STAGE_DIR=$(mktemp -d "$ROLLBACK_ROOT/windshift-restore-stage.XXXXXX") || return 1
  chmod 700 "$STAGE_DIR" || return 1
  for f in manifest.env checksums.sha256; do [ -f "$source/$f" ] && [ ! -L "$source/$f" ] && cp -p "$source/$f" "$STAGE_DIR/$f" && [ -f "$STAGE_DIR/$f" ] && [ ! -L "$STAGE_DIR/$f" ] || return 1; done
  type=$(manifest_value "$STAGE_DIR/manifest.env" database_type)
  included=$(manifest_value "$STAGE_DIR/manifest.env" sso_secret_included)
  case "$type:$included" in sqlite:true|sqlite:false|postgres:true|postgres:false) ;; *) return 1;; esac
  require_regular_artifacts "$source" "$type" "$included" || return 1
  for f in data.tar.gz; do cp -p "$source/$f" "$STAGE_DIR/$f" && [ -f "$STAGE_DIR/$f" ] && [ ! -L "$STAGE_DIR/$f" ] || return 1; done
  [ "$type" != postgres ] || { cp -p "$source/database.dump" "$STAGE_DIR/database.dump" && [ -f "$STAGE_DIR/database.dump" ] && [ ! -L "$STAGE_DIR/database.dump" ]; } || return 1
  [ "$included" != true ] || { cp -p "$source/sso-secret" "$STAGE_DIR/sso-secret" && [ -f "$STAGE_DIR/sso-secret" ] && [ ! -L "$STAGE_DIR/sso-secret" ]; } || return 1
  verify "$STAGE_DIR" >/dev/null
}
create_rollback() {
  ROLLBACK_DIR=$(mktemp -d "$ROLLBACK_ROOT/windshift-restore-rollback.XXXXXX") || return 1
  chmod 700 "$ROLLBACK_DIR" || return 1
  archive_data "$ROLLBACK_DIR" || return 1
  verify_data_archive "$ROLLBACK_DIR" "${SQLITE_DB_REL:-}" || return 1
  ROLLBACK_CHECKSUM=$(sha256 "$ROLLBACK_DIR/data.tar.gz") || return 1
  : >"$ROLLBACK_DIR/checksums.sha256" || return 1
  printf '%s  data.tar.gz\n' "$ROLLBACK_CHECKSUM" >>"$ROLLBACK_DIR/checksums.sha256" || return 1
  if [ "$LIVE_TYPE" = postgres ]; then
    DB_ROLLBACK="$ROLLBACK_DIR/database-before.dump"
    dump_postgres "$DB_ROLLBACK" "$PG_SERVICE" || return 1
    validate_postgres_dump "$DB_ROLLBACK" "$PG_SERVICE" || return 1
    DB_ROLLBACK_CHECKSUM=$(sha256 "$DB_ROLLBACK") || return 1
    printf '%s  database-before.dump\n' "$DB_ROLLBACK_CHECKSUM" >>"$ROLLBACK_DIR/checksums.sha256" || return 1
  fi
  chmod 600 "$ROLLBACK_DIR/checksums.sha256"
}
validate_rollback_data() {
  [ -n "$ROLLBACK_DIR" ] && [ -f "$ROLLBACK_DIR/checksums.sha256" ] && [ ! -L "$ROLLBACK_DIR/checksums.sha256" ] && [ -f "$ROLLBACK_DIR/data.tar.gz" ] && [ ! -L "$ROLLBACK_DIR/data.tar.gz" ] && [ "$(sha256 "$ROLLBACK_DIR/data.tar.gz")" = "$ROLLBACK_CHECKSUM" ] && verify_data_archive "$ROLLBACK_DIR" "${SQLITE_DB_REL:-}"
}
restore_rollback_data() {
  validate_rollback_data && extract_data "$ROLLBACK_DIR"
}
validate_rollback_postgres() {
  [ -f "$DB_ROLLBACK" ] && [ ! -L "$DB_ROLLBACK" ] && [ -s "$DB_ROLLBACK" ] && [ "$(sha256 "$DB_ROLLBACK")" = "$DB_ROLLBACK_CHECKSUM" ] && validate_postgres_dump "$DB_ROLLBACK" "$PG_SERVICE"
}
restore_full_rollback() {
  validate_rollback_data || return 1
  [ "$LIVE_TYPE" != postgres ] || validate_rollback_postgres || return 1
  extract_data "$ROLLBACK_DIR" || return 1
  [ "$LIVE_TYPE" != postgres ] || restore_postgres "$DB_ROLLBACK" "$PG_SERVICE"
}
cleanup_restore() {
  local uncertainty
  [ "$CLEANUP_RUNNING" = false ] || return
  CLEANUP_RUNNING=true
  trap - EXIT
  trap '' INT TERM HUP
  if [ "$ACTION" = backup ]; then
    remove_unmutated_pg_remote || warn "could not remove unmutated PostgreSQL dump $PG_REMOTE"
    if [ "$BACKUP_READY" = true ]; then
      warn "valid backup retained after restart/health failure: $BACKUP_PATH"
    elif [ "$BACKUP_DEST_CREATED" = true ]; then
      rm -rf "$BACKUP_PATH" || warn "could not remove task-created partial backup $BACKUP_PATH"
    fi
    [ "$WAS_RUNNING" != true ] || start_and_verify || warn "backup failed and Windshift could not be restarted"
  elif [ "$PHASE" = stopped-clean ] || [ "$PHASE" = preflight ]; then
    remove_unmutated_pg_remote || warn "could not remove unmutated PostgreSQL dump $PG_REMOTE"
    [ -z "$STAGE_DIR" ] || rm -rf "$STAGE_DIR" || warn "could not remove preflight stage $STAGE_DIR"
    if [ -n "$ROLLBACK_DIR" ] && ! rm -rf "$ROLLBACK_DIR"; then warn "could not remove partial pre-mutation rollback $ROLLBACK_DIR"; fi
    [ "$WAS_RUNNING" != true ] || start_and_verify || warn "operation stopped Windshift but could not restart it"
  elif [ "$PHASE" = data-mutating ] && [ -n "$ROLLBACK_DIR" ]; then
    remove_unmutated_pg_remote || warn "could not remove unmutated PostgreSQL dump $PG_REMOTE"
    if ! restore_rollback_data; then
      stop_for_rollback || warn "rollback failed and Windshift could not be proven stopped"
      warn "restore aborted and /data rollback failed; Windshift remains stopped and the recovery lock is retained. rollback=$ROLLBACK_DIR staged=$STAGE_DIR lock=$LOCK_DIR"
      return
    fi
    if start_and_verify; then
      if [ "$WAS_RUNNING" = true ]; then
        if ! rm -rf "$ROLLBACK_DIR" "$STAGE_DIR"; then warn "data rollback succeeded but retained artifacts at rollback=$ROLLBACK_DIR staged=$STAGE_DIR"; fi
        warn "restore aborted during /data replacement; original /data was restored"
      else
        if ! rm -rf "$STAGE_DIR"; then warn "could not remove staged backup $STAGE_DIR"; fi
        warn "restore aborted during /data replacement while Windshift was already stopped; no health check was possible. Retained rollback: $ROLLBACK_DIR"
      fi
    else
      stop_for_rollback || warn "rolled-back Windshift could not be proven stopped after its health check failed"
      warn "original /data was restored but failed its health check; Windshift remains stopped and the recovery lock is retained. rollback=$ROLLBACK_DIR staged=$STAGE_DIR lock=$LOCK_DIR"
      return
    fi
  elif [ "$PHASE" = pg-mutating ] || [ "$PHASE" = post-restore ]; then
    if [ "$LIVE_TYPE" = postgres ]; then uncertainty="PostgreSQL or /data state is uncertain"; else uncertainty="/data state is uncertain"; fi
    if stop_for_rollback; then
      warn "$uncertainty; Windshift is stopped and the recovery lock is retained. rollback=$ROLLBACK_DIR staged=$STAGE_DIR remote=$PG_REMOTE lock=$LOCK_DIR"
    else
      warn "$uncertainty and Windshift could not be proven stopped. Do not mutate rollback artifacts or remove the recovery lock. rollback=$ROLLBACK_DIR staged=$STAGE_DIR remote=$PG_REMOTE lock=$LOCK_DIR"
    fi
    return
  fi
  release_lock
}
on_exit() { local status=$?; cleanup_restore; exit "$status"; }
on_signal() { die "interrupted during $PHASE"; }
backup() {
  local dest=$1 type service= fingerprint secret_source included=false sso physical_dest physical_rollback
  WINDSHIFT_CONTAINER=$(require_container "$SERVICE")
  data_mount_present || die "--data-path must be exactly one persistent mounted container destination"
  physical_rollback=$(physical_existing_dir "$ROLLBACK_ROOT") || die "could not resolve rollback root"
  physical_dest=$(physical_new_child_path "$dest") || die "backup directory parent must already exist and be resolvable"
  require_outside_data_mount "$physical_rollback" "rollback root"
  require_outside_data_mount "$physical_dest" "backup directory"
  type=$(database_type) || die "deployment uses an unsupported database, SSH, entrypoint, or CLI override configuration"
  check_paths "$type" || die "configured persistent paths are unsupported"
  fingerprint=$(secret_fingerprint) || die "could not read and fingerprint SSO_SECRET/SESSION_SECRET"
  sso=$(read_env "$WINDSHIFT_CONTAINER" SSO_SECRET) || die "could not read SSO_SECRET"
  secret_source=SESSION_SECRET; [ -z "$sso" ] || secret_source=SSO_SECRET
  if [ "$type" = postgres ]; then service=$(postgres_service) || die "no unambiguous PostgreSQL service"; validate_postgres_service "$service" || die "PostgreSQL settings do not prove a stock Compose service mapping"; fi
  acquire_lock
  case "$physical_dest" in "$LOCK_PHYSICAL"|"$LOCK_PHYSICAL"/*) die "backup directory must not resolve to the lock directory or a descendant: $dest";; esac
  [ ! -e "$dest" ] || die "backup directory already exists: $dest"
  mkdir -m 700 "$dest" || die "could not create backup directory"
  BACKUP_DEST_CREATED=true
  stop_app || die "could not stop Windshift within 60 seconds"
  PHASE=backup-stopped
  archive_data "$dest" || die "could not archive /data"
  verify_data_archive "$dest" "${SQLITE_DB_REL:-}" || die "created data archive failed validation"
  if [ "$type" = postgres ]; then
    dump_postgres "$dest/database.dump" "$service" || die "could not dump PostgreSQL"
    validate_postgres_dump "$dest/database.dump" "$service" || die "created PostgreSQL dump failed structural validation"
  fi
  [ "$INCLUDE_SSO_SECRET" != true ] || included=true
  manifest "$dest" "$type" "$service" "$fingerprint" "$secret_source" "$included" || die "could not write manifest"
  [ "$included" != true ] || write_sso_secret "$dest" || die "could not write sensitive secret file"
  checksums "$dest" "$type" "$included" || die "could not create checksums"
  verify "$dest" >/dev/null || die "created backup failed final verification"
  PHASE=backup-ready
  BACKUP_READY=true
  start_and_verify || die "backup completed but Windshift did not become healthy"
  release_lock
  trap - EXIT INT TERM HUP
  printf 'Backup created: %s\n' "$dest"
}
restore() {
  local source=$1 type included expected actual manifest_service physical_source physical_rollback
  [ -d "$source" ] || die "backup directory does not exist"
  [ -f "$source/manifest.env" ] && [ ! -L "$source/manifest.env" ] || die "backup manifest is not a regular file"
  WINDSHIFT_CONTAINER=$(require_container "$SERVICE")
  data_mount_present || die "--data-path must be exactly one persistent mounted container destination"
  physical_rollback=$(physical_existing_dir "$ROLLBACK_ROOT") || die "could not resolve rollback root"
  physical_source=$(physical_existing_dir "$source") || die "could not resolve backup directory"
  require_outside_data_mount "$physical_rollback" "rollback root"
  require_outside_data_mount "$physical_source" "backup directory"
  LIVE_TYPE=$(database_type) || die "deployment uses an unsupported database, SSH, entrypoint, or CLI override configuration"
  check_paths "$LIVE_TYPE" || die "configured persistent paths are unsupported"
  acquire_lock
  stage_backup "$source" || die "could not create a verified private staged backup snapshot"
  type=$(verify "$STAGE_DIR") || die "staged backup validation failed"
  included=$(manifest_value "$STAGE_DIR/manifest.env" sso_secret_included)
  [ "$LIVE_TYPE" = "$type" ] || die "staged backup database type does not match deployment"
  [ "$type" != sqlite ] || [ "$(manifest_value "$STAGE_DIR/manifest.env" sqlite_db_path)" = "$SQLITE_DB_REL" ] || die "staged SQLite DB path does not match the live DB_PATH"
  expected=$(manifest_value "$STAGE_DIR/manifest.env" secret_fingerprint); actual=$(secret_fingerprint) || die "could not fingerprint current secret"
  [ "$actual" = "$expected" ] || die "backup uses a different SSO_SECRET/SESSION_SECRET; configure the source value first"
  [ "$(manifest_value "$STAGE_DIR/manifest.env" data_path)" = "$DATA_PATH" ] || die "backup data path does not match"
  if [ "$type" = postgres ]; then
    manifest_service=$(manifest_value "$STAGE_DIR/manifest.env" postgres_service)
    [ -n "$POSTGRES_SERVICE" ] || POSTGRES_SERVICE=$manifest_service
    PG_SERVICE=$(postgres_service) || die "no unambiguous PostgreSQL service"
    validate_postgres_service "$PG_SERVICE" || die "PostgreSQL settings do not prove a stock Compose service mapping"
    validate_postgres_dump "$STAGE_DIR/database.dump" "$PG_SERVICE" || die "staged PostgreSQL dump failed structural preflight"
  fi
  stop_app || { PHASE=stopped-clean; die "could not stop Windshift within 60 seconds"; }
  PHASE=stopped-clean
  create_rollback || die "could not create complete verified rollback artifacts"
  PHASE=data-mutating
  extract_data "$STAGE_DIR" || die "target data extraction failed"
  if [ "$type" = postgres ]; then
    if restore_postgres "$STAGE_DIR/database.dump" "$PG_SERVICE"; then :; else
      result=$?
      [ "$result" = 2 ] && die "PostgreSQL dump copy or structural validation failed before database mutation"
      PHASE=pg-mutating
      die "PostgreSQL restore failed; state is uncertain"
    fi
  fi
  PHASE=post-restore
  if ! start_and_verify; then
    stop_for_rollback || die "restored state was unhealthy and could not be stopped; rollback was not attempted. rollback=$ROLLBACK_DIR staged=$STAGE_DIR"
    if restore_full_rollback && start_and_verify; then
      PHASE=rolled-back
      if ! rm -rf "$ROLLBACK_DIR" "$STAGE_DIR"; then warn "rollback succeeded but retained artifacts at rollback=$ROLLBACK_DIR staged=$STAGE_DIR"; fi
      release_lock
      trap - EXIT INT TERM HUP
      die "restored state was unhealthy; previous state was restored"
    fi
    die "restored state was unhealthy and rollback could not be proven; Windshift remains stopped. rollback=$ROLLBACK_DIR staged=$STAGE_DIR"
  fi
  PHASE=complete
  if [ "$WAS_RUNNING" != true ]; then
    if ! rm -rf "$STAGE_DIR"; then warn "restore completed while stopped but retained staged backup at $STAGE_DIR"; fi
    release_lock
    trap - EXIT INT TERM HUP
    printf 'Restore completed while Windshift was already stopped; no health check was possible. Retained rollback: %s\n' "$ROLLBACK_DIR"
    return 0
  fi
  if ! rm -rf "$ROLLBACK_DIR" "$STAGE_DIR"; then warn "restore is healthy but rollback/staging cleanup failed; retain private artifacts at rollback=$ROLLBACK_DIR staged=$STAGE_DIR"; fi
  release_lock
  trap - EXIT INT TERM HUP
  printf 'Restore completed from: %s\n' "$source"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    backup|restore) [ -z "$ACTION" ] || die "choose one action"; ACTION=$1;;
    --compose-file|--data-path|--postgres-service|--helper-image|--rollback-root) [ "$#" -ge 2 ] || die "$1 needs a value"; case "$1" in --compose-file) COMPOSE+=(-f "$2");; --data-path) DATA_PATH=$2;; --postgres-service) POSTGRES_SERVICE=$2;; --helper-image) HELPER_IMAGE=$2;; --rollback-root) ROLLBACK_ROOT=$2;; esac; shift;;
    --include-sso-secret) INCLUDE_SSO_SECRET=true;;
    --force) FORCE=true;;
    -h|--help) usage; exit 0;;
    -*) die "unknown option: $1";;
    *) [ -z "$BACKUP_PATH" ] || die "only one backup directory may be supplied"; BACKUP_PATH=$1;;
  esac
  shift
done
[ -n "$ACTION" ] && [ -n "$BACKUP_PATH" ] || { usage >&2; exit 1; }
DATA_PATH=$(normalise_container_path "$DATA_PATH") || die "unsafe --data-path"
BACKUP_PATH=$(normalise_host_path "$BACKUP_PATH") || die "backup path must be absolute, normalized, and contain no ':'"
ROLLBACK_ROOT=$(normalise_host_path "$ROLLBACK_ROOT") || die "rollback root must be absolute, normalized, and contain no ':'"
[ -d "$ROLLBACK_ROOT" ] && [ -w "$ROLLBACK_ROOT" ] || die "rollback root must exist and be writable: $ROLLBACK_ROOT"
case "$HELPER_IMAGE" in ''|-*) die "helper image must be a nonempty image reference and must not start with '-'";; esac
[ "$ACTION" != restore ] || [ "$FORCE" = true ] || die "restore requires --force"
[ "$ACTION" != restore ] || [ "$INCLUDE_SSO_SECRET" != true ] || die "--include-sso-secret is backup-only"
case "$HEALTH_TIMEOUT" in ''|*[!0-9]*) die "WINDSHIFT_BACKUP_HEALTH_TIMEOUT must be an integer from 3 to 3600";; esac
[ "$HEALTH_TIMEOUT" -ge 3 ] && [ "$HEALTH_TIMEOUT" -le 3600 ] || die "WINDSHIFT_BACKUP_HEALTH_TIMEOUT must be from 3 to 3600 seconds"
need docker; need awk; need cp; need mktemp; command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 || die "required command not found: sha256sum or shasum"
trap on_exit EXIT
trap on_signal INT TERM HUP
"$ACTION" "$BACKUP_PATH"
