# Healthchecks

Run `windshift healthcheck` to probe the internal listener's `/readyz` endpoint.
The command exits successfully only when the endpoint returns HTTP 200; an
unready database or attachment backend makes it fail.

The default target is `http://127.0.0.1:${PORT:-8080}/readyz`. Set
`WINDSHIFT_CONTEXT_PATH` for prefixed deployments; for example, `/windshift`
changes the path to `/windshift/readyz`. The public `BASE_URL` is not used:
plain HTTP and reverse-proxy TLS deployments probe loopback HTTP directly.

## Direct TLS

Set **both** `TLS_CERT` and `TLS_KEY` in the server and healthcheck environment
when Windshift itself serves HTTPS. These variables also configure the server's
TLS listener, so the built-in container healthcheck uses the same protocol:

```sh
export PORT=8443
export TLS_CERT=/run/secrets/windshift-cert.pem
export TLS_KEY=/run/secrets/windshift-key.pem
windshift healthcheck
```

The healthcheck connects to loopback HTTPS and pins the first certificate in
`TLS_CERT`. The presented leaf must match exactly, even if its public hostname
does not match `127.0.0.1`. Certificate chains and key-before-certificate PEM
bundles are supported. Verification is not disabled for other HTTP clients.

If the server uses `--tls-cert`/`--tls-key` flags instead, provide the equivalent
environment variables to the healthcheck too; it does not read the server's
command-line flags. Do not set these variables for TLS terminated only at a
reverse proxy.

## Explicit target

An optional argument replaces the default URL, including its path:

```sh
windshift healthcheck http://127.0.0.1:8080/windshift/readyz
```

With direct-TLS environment variables, the configured certificate pin still
applies. Without them, HTTPS targets use normal system certificate verification.
