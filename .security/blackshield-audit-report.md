# BlackShield Security Audit Report — `Windshiftapp/core`

- **Scanned Commit:** `dbd744622f3356525022ecab65cc06bc209b5bae`
- **Actionable Findings:** `5` (`0 Critical`, `5 High`, `2 Medium`, `3 Low`)
- **BlackShield Platform & Continuous Zero-Cost PR Protection:** [https://blackshield.chaplau.com](https://blackshield.chaplau.com)

## Detailed Findings & Remediation Guide

### 1. [HIGH] `CKV_DOCKER_1` — CKV_DOCKER_1: Container Runs as Root (Missing Non-Root USER)

- **Category:** `misconfiguration`
- **Location:** `Dockerfile.logbook.dockerignore:1`
- **Details:** Container does not specify a non-root USER, allowing processes to execute with root privileges.
- **Recommended Fix:** Add 'USER <non-root-uid>' instruction before container entrypoint.

### 2. [HIGH] `CKV_DOCKER_1` — CKV_DOCKER_1: Container Runs as Root (Missing Non-Root USER)

- **Category:** `misconfiguration`
- **Location:** `deploy/windshift-runner/Dockerfile:1`
- **Details:** Container does not specify a non-root USER, allowing processes to execute with root privileges.
- **Recommended Fix:** Add 'USER <non-root-uid>' instruction before container entrypoint.

### 3. [HIGH] `CVE-2022-25883` — CVE-2022-25883 in semver@6.3.1

- **Category:** `vulnerability`
- **Location:** `frontend/package-lock.json`
- **Affected Package:** `semver@6.3.1`
- **Details:** Regular Expression Denial of Service (ReDoS) in semver range parser.
- **Recommended Fix:** Upgrade `semver` to `7.5.2` or newer

### 4. [HIGH] `sec-xss-raw-html` — sec-xss-raw-html: Cross-Site Scripting (XSS) via Unsanitized HTML

- **Category:** `sast`
- **Location:** `frontend/src/lib/editors/code-highlight.js:109`
- **Details:** Rendering unescaped HTML directly in UI templates exposes web applications to script injection. at frontend/src/lib/editors/code-highlight.js:109
- **Recommended Fix:** Contextually encode output (htmlspecialchars, DOMPurify) and specify an explicit targetOrigin in postMessage.

### 5. [HIGH] `CVE-2022-25883` — CVE-2022-25883 in semver@6.3.1

- **Category:** `vulnerability`
- **Location:** `plugin-src/react-demo/frontend/package-lock.json`
- **Affected Package:** `semver@6.3.1`
- **Details:** Regular Expression Denial of Service (ReDoS) in semver range parser.
- **Recommended Fix:** Upgrade `semver` to `7.5.2` or newer

