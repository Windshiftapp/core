# Windshift Frontend

Svelte 5 + Vite + Tailwind. Built to `dist/` and embedded into the Go binary.

## Local development

```sh
npm ci         # use ci, not install — respects the lockfile
npm run dev    # vite dev server
npm run build  # production build (what release.sh runs)
npm run check  # biome lint + format check
npm run test   # vitest
```

## Supply-chain hardening

This directory has an `.npmrc` that adds two install-time guards:

- **`min-release-age=14`** — `npm install` refuses to resolve any package version published less than 14 days ago. Protects against fast-burning supply-chain attacks (e.g. chalk/debug-style) where a malicious release is yanked within hours. Requires **npm >= 11** (enforced by `engines` + `engine-strict=true` — older npm will fail rather than silently skip the check).
- **`ignore-scripts=true`** — disables `postinstall`/`preinstall` hooks during install, which is the primary malware execution vector. If a specific package legitimately needs its scripts (rare in this dep set), run `npm rebuild <pkg>` explicitly after install.

### Overriding the cooldown

To pull a fresh release urgently (e.g. a same-day security patch):

```sh
npm install <pkg> --min-release-age=0
```

The override is per-invocation; don't add it to `.npmrc`. Commit the resulting lockfile change with a note explaining why.

### Why this is here

`Dependabot` enforces a longer cooldown for the PRs it opens (see `.github/dependabot.yml`). The `.npmrc` here covers the other install paths — local `npm install` on a dev laptop, and `npm ci` from `release.sh` — so a manually pulled fresh-and-malicious version can't land in the lockfile either.
