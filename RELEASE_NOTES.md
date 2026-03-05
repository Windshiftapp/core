# Windshift v0.2.2 - "Dry Dock"

---

> **Not recommended for production use.**
>
> Windshift is an early release that is still undergoing internal testing. APIs, data formats, and configuration may change between releases without migration paths. We publish this release to invite early exploration, testing, and feedback - not to support production workloads.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Bug Fixes

- **`ALLOWED_HOSTS` environment variable now works correctly.** The `ALLOWED_HOSTS` env var was documented but silently ignored because the environment read happened after `BASE_URL` had already populated the value. It is now read at the correct point so that the intended priority order is respected:
  1. `--allowed-hosts` CLI flag (highest priority)
  2. `ALLOWED_HOSTS` environment variable
  3. Hostname auto-derived from `BASE_URL` / `PUBLIC_URL`
