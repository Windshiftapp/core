# Deploying Windshift on Kubernetes

This page is an example deployment: a small set of plain manifests under
[`deploy/kubernetes/`](kubernetes/) plus the notes you need to run them. It is
meant to be copied and adapted, not applied blindly to production.

## Windshift runs as a single replica

**Run exactly one Windshift pod.** This is not a tuning suggestion; the
application is not safe to scale horizontally today:

- **Schema migrations run at startup with no distributed lock.** Every replica
  applies pending migrations as it boots. Several replicas starting at once
  against an out-of-date database can race DDL.
- **Live updates are per-process.** The server-sent-events hub is in-memory, so
  a client only receives item updates from the pod it is connected to. A second
  replica would serve stale live views.
- **Background schedulers run per process without leader election.**
  Notification digests, outbound email, recurrence, custom-field cleanup,
  runner lease reaping, SCM sync, magic-link cleanup, and issue sync all tick in
  every process. Two replicas would send duplicate mail and run duplicate
  syncs.

The manifests therefore set `replicas: 1` and `strategy: Recreate`. If you want
horizontal scaling, the work is in the application (a migration lock, an SSE
backplane, and leases for the schedulers), not in the deployment.

## Files

| File | Purpose |
| --- | --- |
| `kubernetes/00-namespace.yaml` | Namespace. Remove if you deploy into an existing one. |
| `kubernetes/10-pvc.yaml` | `ReadWriteOnce` PVC for `/data`. |
| `kubernetes/20-deployment.yaml` | The Windshift pod, probes, env, and security context. |
| `kubernetes/30-service.yaml` | ClusterIP Service on port 80 → 8080. |
| `kubernetes/40-ingress.yaml` | Example Ingress with SSE-safe timeouts. Delete if unused. |
| `kubernetes/secret.yaml.example` | Shape of the Secret to create; not applied by `kubectl`. |

## Prerequisites

- A Kubernetes cluster (1.27+) with a default StorageClass
- `kubectl`
- For production: a PostgreSQL 14+ server

## Quick start (SQLite)

The default manifest uses SQLite on the `/data` PVC, which is enough to try
Windshift out.

```bash
# 1. Create the namespace and the session secret.
kubectl apply -f deploy/kubernetes/00-namespace.yaml
kubectl -n windshift create secret generic windshift-secrets \
  --from-literal=SSO_SECRET="$(openssl rand -hex 32)"

# 2. Edit BASE_URL and (optionally) the Ingress host, then apply the rest.
#    The Secret file is intentionally not matched by `kubectl apply -f`.
kubectl apply -f deploy/kubernetes/10-pvc.yaml
kubectl apply -f deploy/kubernetes/20-deployment.yaml
kubectl apply -f deploy/kubernetes/30-service.yaml
# kubectl apply -f deploy/kubernetes/40-ingress.yaml

# 3. Watch it come up.
kubectl -n windshift rollout status deploy/windshift

# 4. Reach it without an Ingress.
kubectl -n windshift port-forward svc/windshift 8080:80
# open http://127.0.0.1:8080 and complete the setup wizard
```

`SSO_SECRET` signs sessions and encrypts stored credentials. Keep it stable
across upgrades: changing it invalidates sessions and stored secrets. If you
delete and recreate the Secret, restore the previous value.

## Configuration

Settings are plain environment variables on the Deployment. The full set,
with explanations, is documented in [`deploy/.env.example`](.env.example); the
ones that matter most on Kubernetes:

| Variable | Notes |
| --- | --- |
| `BASE_URL` | Public scheme + host. Must match how users reach Windshift, or cookies, links, and WebAuthn break. |
| `USE_PROXY` | `true` behind an Ingress/reverse proxy. |
| `SSO_SECRET` | Required. From the `windshift-secrets` Secret. |
| `WINDSHIFT_CONTEXT_PATH` | Set when serving under a subpath, e.g. `/windshift`. Probes follow it. |
| `ATTACHMENT_PATH`, `PLUGIN_DIR`, `AI_PROMPTS_DIR` | Default under `/data`. |
| `DB_TYPE`, `DB_PATH` | `sqlite` and `/data/windshift.db` by default. |
| `WINDSHIFT_METRICS_TOKEN` | Optional bearer token protecting `/metrics`. |
| `WINDSHIFT_MEMORY_LIMIT_MB` | Process budget. Keep at or below the container memory limit. |

### Probes

The Deployment uses the server's built-in endpoints:

- liveness: `GET /healthz`
- readiness and startup: `GET /readyz`

Both are public and honor `WINDSHIFT_CONTEXT_PATH`.

### PostgreSQL

Comment out `DB_TYPE=sqlite` and `DB_PATH`, uncomment the `POSTGRES_*` block in
`20-deployment.yaml`, and add the password to the Secret:

```bash
kubectl -n windshift create secret generic windshift-secrets \
  --from-literal=SSO_SECRET="$(openssl rand -hex 32)" \
  --from-literal=POSTGRES_PASSWORD="..."
```

Keep the declared connection budget consistent with the database's
`max_connections`. With a single Windshift replica, `POSTGRES_REPLICA_COUNT`
stays at `1`; Windshift validates the budget at startup and reports it in
Admin → Diagnostics.

## Ingress and TLS

`40-ingress.yaml` is an nginx example. Two things to get right:

1. **Streaming timeouts.** Item detail views hold a server-sent-events
   connection open, and nginx's default 60s read timeout drops it. The example
   sets `proxy-read-timeout`/`proxy-send-timeout` to 3600s. Translate to your
   controller's annotations.
2. **`BASE_URL`.** Set it to the public URL, and keep `USE_PROXY=true`.

## Backups

Back up the `/data` volume and, when using PostgreSQL, the database. The
in-tree guide at [`deploy/backup-restore.md`](backup-restore.md) covers the
Docker Compose case; the same files and database are involved here. A simple
PVC snapshot plus `pg_dump` is enough for most installs.

## Upgrades

1. Change `image:` in `20-deployment.yaml` to the new release tag.
2. `kubectl apply -f deploy/kubernetes/20-deployment.yaml`

The `Recreate` strategy means a short outage while the old pod releases the
`ReadWriteOnce` volume. Migrations run automatically on the new pod's startup.
Back up first if the release notes mention a schema change.

## Hardening already in the manifests

The Deployment follows the same posture as the official image:

- runs as non-root (`65534:65534`) with `fsGroup` on the volume
- `readOnlyRootFilesystem: true`, with `/tmp` as a sized `emptyDir`
- `allowPrivilegeEscalation: false`, all capabilities dropped,
  `seccompProfile: RuntimeDefault`
- no service-account token mounted

These were validated against the published image: it starts, writes SQLite to
`/data`, and serves `/readyz` under a read-only root filesystem.

## Validating before you apply

```bash
kubectl apply --dry-run=client -f deploy/kubernetes/10-pvc.yaml
kubectl apply --dry-run=client -f deploy/kubernetes/20-deployment.yaml
kubectl apply --dry-run=client -f deploy/kubernetes/30-service.yaml
kubectl apply --dry-run=client -f deploy/kubernetes/40-ingress.yaml
```
