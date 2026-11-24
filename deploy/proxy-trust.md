# Proxy trust and client IP resolution

Windshift sits behind a reverse proxy (nginx, Traefik, Caddy, HAProxy, an
Ingress controller) in most deployments. This page explains how it decides
which client IP to use, how to configure that trust correctly, and how to tell
whether a deployment is affected by the private-range finding in WI-2359.

## What Windshift sees

Two inputs are available for every request:

- **The socket peer** — the TCP source address in `r.RemoteAddr`. It cannot be
  forged by the client; only the network path determines it.
- **Forwarding headers** — `X-Forwarded-For` (a comma-separated chain),
  `X-Real-IP` (a single address), and `X-Forwarded-Proto`. Every one of these is
  client-controlled unless a trusted proxy overwrites or appends it.

Windshift resolves the client IP only when the socket peer is a trusted proxy.
Otherwise the peer **is** the client and all forwarding headers are ignored.

For a trusted peer the `X-Forwarded-For` chain is walked **right to left**:
each proxy appends the address it saw, so entries nearest the right are the
most trustworthy and the leftmost entries are the most likely to be
client-supplied. Trusted hops are skipped; the first untrusted hop is the
client. `X-Real-IP` is a single hop and is trusted only when it is not itself a
trusted proxy. If every hop is a trusted proxy there is no attributable client,
so the socket peer is used.

The same resolved IP is used by the login limiter, `LoginLockout`, the
FIDO/SCIM/OAuth/portal limiters, session IP binding, audit attribution, and the
cookie `Secure`/HSTS decision. The admin **Diagnostics → Proxy trust** tab
reports the resolved policy and the peers this replica has seen.

## Trust modes

`PROXY_TRUST_MODE` selects the policy. It applies to both explicit
`USE_PROXY=true` and auto-detected proxy mode.

| Mode | Trusted peers | Status |
| --- | --- | --- |
| `legacy` (default through 0.9.7) | loopback, link-local, **and every private (RFC1918/ULA) address** | Existing behavior. A private-network client can choose its own resolved IP, and LAN users behind a proxy share one lockout bucket. |
| `strict` (opt-in in 0.9.6, default from 0.9.8) | loopback and `ADDITIONAL_PROXIES` only | A private address that is not listed is the client; its forwarding headers are ignored. |

`USE_PROXY` must still be `true` for any forwarded header to be trusted. With
proxy mode disabled, the socket peer is always the client.

The loopback exception is deliberate: a request from `127.0.0.1`/`::1` comes
from a local process on the same host, which can already read the process
memory and configuration, so it is treated as trusted in both modes.

**0.9.8 will make `strict` the default.** Staying on `legacy` keeps the
spoofing and shared-bucket behavior. Opt in early by setting
`PROXY_TRUST_MODE=strict` and listing the proxies as described below.

## `ADDITIONAL_PROXIES`

`ADDITIONAL_PROXIES` is a comma-separated list of trusted proxy addresses. It
accepts **literal IPv4 and IPv6 addresses only** — no CIDR ranges, hostnames, or
wildcards. An entry that is not a literal IP is ignored with a startup warning.

Because of that constraint, a proxy whose address changes (a Kubernetes Ingress
pod, a recreated Docker container) cannot be trusted by listing its dynamic
address. Use one of the stable-source setups below.

In strict mode, an unlisted proxy is treated as the client. Its clients then
collapse to the proxy address, and `X-Forwarded-Proto` is ignored, so session
cookies may lose their `Secure` flag and HSTS may not be sent. The Diagnostics →
Proxy trust tab reports these peers under **Unlisted proxies** with the
`action_required` status. A peer that sends forwarding headers is reported
whether its address is private or public; a public-address upstream is exactly
the case a private-only check would miss.

## Header rules for the proxy

Configure the proxy so Windshift receives an unambiguous chain:

- **`X-Forwarded-For`.** Append the real peer (`$proxy_add_x_forwarded_for` in
  nginx, the default in Traefik/Caddy/HAProxy). Do **not** pass a
  client-supplied `X-Forwarded-For` through unchanged. If the proxy overwrites
  instead of appending, that is also safe — but it must strip the inbound
  header first.
- **`X-Real-IP`.** Set it to the immediate peer (`$remote_addr`) or remove the
  header. If you forward a client-supplied `X-Real-IP`, a trusted proxy becomes
  a spoofing vector.
- **`X-Forwarded-Proto`.** Set it to the scheme the client used. Windshift only
  believes it from a trusted peer, and uses it for the cookie `Secure` flag,
  HSTS, and CORS origins.
- **Repeated headers.** Send one `X-Forwarded-For` header. Windshift joins
  every `X-Forwarded-For` header line into one comma-separated chain before
  walking it, so a duplicated header from an untrusted source is just more
  leftmost entries to skip. `X-Real-IP` and `X-Forwarded-Proto` use the first
  line.

## Upload limits

Windshift owns the attachment size limit so an administrator can change it
without redeploying the proxy. The effective per-file limit defaults to 50 MiB
and can be lowered by the operator ceiling. The v2 item upload permits the
effective limit plus 1 MiB for multipart framing and enforces the exact per-file
limit in the application.

A proxy must not impose a smaller request-body cap, or uploads are rejected at
the edge before Windshift can apply the configured limit:

- **nginx / nginx-unprivileged.** `client_max_body_size 0;` removes nginx's
  default 1 MiB request-body cap; `proxy_request_buffering off;` streams the
  request to Windshift instead of spooling it to the sidecar's temporary volume.
- **ingress-nginx.** `nginx.ingress.kubernetes.io/proxy-body-size: "0"` and
  `nginx.ingress.kubernetes.io/proxy-request-buffering: "off"`.

Set a smaller cap at the edge only if you deliberately want a stricter limit
than Windshift's. Windshift still bounds every request and enforces the
effective per-file limit in the application.

## Stable-source setups for dynamic ingress

### Kubernetes: a transparent loopback sidecar

Run a small reverse proxy as a second container in the Windshift pod and point
the Ingress at that container's port. The sidecar forwards to Windshift over
`127.0.0.1`, which is always trusted, so no address ever needs listing and the
setup survives Ingress rollouts.

The sidecar must be **transparent**: it passes the Ingress's forwarding headers
through unchanged. It must not append its own peer with
`$proxy_add_x_forwarded_for` (that would make the Ingress pod the rightmost —
and therefore the resolved — hop) and must not set `X-Forwarded-Proto $scheme`
(the Ingress→sidecar hop is plain HTTP, which would drop the `Secure` flag and
HSTS).

**Run it as a non-root, read-only container.** The Windshift pod runs as
`65534:65534` with `runAsNonRoot` and a read-only root filesystem. Stock
`nginx:*-alpine` fails that policy: it writes `/run/nginx.pid` and its temp
paths. Use `nginx/nginx-unprivileged`, the official non-root NGINX image (PID
and temp files under `/tmp`); `stable-alpine` tracks the current stable minor and
is rebuilt weekly and for CVEs, so pin a digest if you need reproducibility.
Mount a writable `/tmp` and the server block from a ConfigMap.

```yaml
containers:
  - name: windshift
    ports:
      - name: http          # the probes keep using this port
        containerPort: 8080
  - name: proxy
    image: nginx/nginx-unprivileged:stable-alpine
    ports:
      - name: web           # the Service targets this port
        containerPort: 8081
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
    volumeMounts:
      - name: proxy-config
        mountPath: /etc/nginx/conf.d
      - name: proxy-tmp
        mountPath: /tmp
volumes:
  - name: proxy-config
    configMap:
      name: windshift-proxy-config
  - name: proxy-tmp
    emptyDir:
      sizeLimit: 16Mi
```

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: windshift-proxy-config
  namespace: windshift
data:
  default.conf: |
    server {
        listen 8081;
        server_name _;

        # Item-detail SSE responses must not be buffered.
        proxy_buffering off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;

        location / {
            proxy_pass http://127.0.0.1:8080;
            proxy_http_version 1.1;
            # Let Windshift enforce the configured attachment limit. nginx's
            # default 1 MiB request-body cap would reject uploads first, and
            # request buffering would spool them to the 16 MiB tmp volume.
            client_max_body_size 0;
            proxy_request_buffering off;
            proxy_set_header Upgrade           $http_upgrade;
            proxy_set_header Connection        "";
            # Transparent: pass the Ingress's headers through unchanged.
            proxy_set_header X-Forwarded-For   $http_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $http_x_forwarded_proto;
            proxy_set_header X-Real-IP         $http_x_real_ip;
            proxy_set_header Host              $http_host;
        }
    }
```

Point the Service's `targetPort` at the sidecar port; the Ingress keeps
referencing the Service port by name:

```yaml
# 30-service.yaml
spec:
  ports:
    - name: http
      port: 80
      targetPort: web
```

**Restrict who can reach the sidecar.** The sidecar passes headers through
unchanged and Windshift trusts its loopback peer, so any workload that can open
a connection to the sidecar port can forge the client IP and the scheme. Publish
only the sidecar's port and enforce that only the Ingress controller may reach
it, with a NetworkPolicy (below) or by terminating an authenticated/mTLS
upstream in the sidecar. Apply the policy in the same namespace as the pod and
keep the kubelet's probe port open.

The NetworkPolicy below selects the Windshift pod and allows only the Ingress
controller to reach the sidecar on `8081`. Adjust the namespace and pod labels
to your controller (the example is ingress-nginx). Because a policy that selects
the pod also gates the probe port, it allows `8080` from the node CIDR so the
liveness/readiness probes keep working — replace that CIDR with your cluster's
node network, or use your CNI's probe allowance.

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: windshift-proxy-ingress-only
  namespace: windshift
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: windshift
  policyTypes:
    - Ingress
  ingress:
    # Only the Ingress controller may supply forwarding headers.
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
          podSelector:
            matchLabels:
              app.kubernetes.io/component: controller
      ports:
        - protocol: TCP
          port: 8081
    # Kubelet probes; replace with your node CIDR.
    - from:
        - ipBlock:
            cidr: 10.0.0.0/8
      ports:
        - protocol: TCP
          port: 8080
```

Because the hop into Windshift is loopback, `PROXY_TRUST_MODE=strict` needs no
`ADDITIONAL_PROXIES` — but only if the Ingress actually appends
`X-Forwarded-For` and sets `X-Forwarded-Proto`. nginx Ingress does this by
default; confirm it for your controller, and verify per the checklist below. If
the Ingress sends no address header, Windshift sees the sidecar (loopback) as
the client and every user collapses to `127.0.0.1`.

### Kubernetes: a host-network ingress controller

If your Ingress controller runs with `hostNetwork: true`, the socket peer is the
**node IP**, which is stable per node. List each node address:

```
ADDITIONAL_PROXIES=10.0.0.11,10.0.0.12,10.0.0.13
```

### Docker Compose / Dokploy: a fixed-address proxy

Assign the proxy a static address on a user-defined network and list it.

```yaml
services:
  proxy:
    # Traefik, Caddy, or nginx
    networks:
      windshift:
        ipv4_address: 172.28.0.10
  windshift:
    environment:
      - USE_PROXY=true
      - PROXY_TRUST_MODE=strict
      - ADDITIONAL_PROXIES=172.28.0.10
    networks:
      - windshift

networks:
  windshift:
    ipam:
      config:
        - subnet: 172.28.0.0/16
```

The published-port path also works: when the proxy runs on the host and
forwards to the published container port, the peer is the Docker bridge
gateway (for example `172.17.0.1`), which is stable. Confirm the actual peer
address on the Diagnostics → Proxy trust tab before listing it.

## Verify the setup

1. Open **Admin → Diagnostics → Proxy trust**.
2. `Trust mode` is `Strict`, `Affected` is `No`, and `Status` is `protected`.
3. The peers listed under **Unlisted proxies** are empty. If not, confirm each
   is really your reverse proxy, add it to `ADDITIONAL_PROXIES`, and restart.
   Do not add an address you cannot attribute to the proxy: a client can send
   forwarding headers too.
4. With a sidecar, confirm only the Ingress controller can reach the sidecar
   port: from another pod, a connection to the Windshift pod on `8081` must be
   refused or time out, while the Ingress still serves the site. Confirm the
   liveness/readiness probes still pass after the NetworkPolicy is applied.
5. Have two different LAN clients sign in over HTTPS. Their resolved IPs, the
   IPs stored on their sessions, and the IPs in the audit log must differ and
   match the clients, not the proxy.
6. Confirm the session cookie still carries `Secure` and the response still
   sends `Strict-Transport-Security` (`curl -sI https://<host>/`). A sidecar or
   proxy that forwards `X-Forwarded-Proto: http` drops both, which the
   diagnostics alone cannot see.
7. Set the attachment limit to a test value and upload a file exactly at that
   size, then one byte over. The first must be stored; the second must reach
   Windshift and be rejected by the application's size check, not by nginx with
   a `413`.
8. Re-run steps 2–7 after a proxy restart or rollout. A recreated proxy that
   changes address shows up under **Unlisted proxies** immediately; a sidecar
   setup keeps working because the hop into Windshift stays loopback, provided
   the ingress-controller-only policy still holds.

The same values are returned by `GET /api/v2/admin/diagnostics/proxy-trust`,
and by the unversioned `GET /api/admin/diagnostics/proxy-trust`.
