#!/usr/bin/env node
// Blocks the release on high/critical advisories in production dependencies.
// Advisories that cannot be fixed yet are dismissed in
// production-audit-allowlist.json with a reason and a review date, so the
// exception stays visible and expires.
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const allowlistPath = path.join(scriptDir, 'production-audit-allowlist.json');
const blockedSeverities = new Set(['high', 'critical']);

const allowlist = JSON.parse(readFileSync(allowlistPath, 'utf8'));
const today = new Date().toISOString().slice(0, 10);

// npm audit exits non-zero when it finds vulnerabilities; the JSON report on
// stdout is still what we want to evaluate.
const audit = spawnSync('npm', ['audit', '--json', '--omit=dev'], {
  encoding: 'utf8',
  shell: process.platform === 'win32',
});

if (!audit.stdout) {
  console.error('npm audit produced no report.');
  if (audit.stderr) console.error(audit.stderr.trim());
  process.exit(1);
}

let report;
try {
  report = JSON.parse(audit.stdout);
} catch (error) {
  console.error(`npm audit returned invalid JSON: ${error.message}`);
  process.exit(1);
}

// A vulnerability's `via` is either an advisory object or the name of another
// vulnerable package. Collect the advisory objects once, keyed by URL.
const advisories = new Map();
for (const vulnerability of Object.values(report.vulnerabilities ?? {})) {
  for (const via of vulnerability.via ?? []) {
    if (typeof via !== 'object' || !via.url) continue;
    advisories.set(via.url, via);
  }
}

const dismissed = [];
const expired = [];
const unresolved = [];

for (const advisory of advisories.values()) {
  if (!blockedSeverities.has(advisory.severity)) continue;
  const id = advisory.url.split('/').pop();
  const entry = allowlist[id];
  if (!entry) {
    unresolved.push({ ...advisory, id });
  } else if (entry.review_by && entry.review_by < today) {
    expired.push({ ...advisory, id, reviewBy: entry.review_by });
  } else {
    dismissed.push({ ...advisory, id, reason: entry.reason });
  }
}

for (const advisory of dismissed) {
  console.log(
    `Dismissed ${advisory.id} (${advisory.name}, ${advisory.severity}): ${advisory.reason}`
  );
}

for (const advisory of expired) {
  console.error(
    `Dismissal for ${advisory.id} expired on ${advisory.reviewBy}; re-review production-audit-allowlist.json.`
  );
}

for (const advisory of unresolved) {
  console.error(`Unresolved ${advisory.severity} advisory ${advisory.id}: ${advisory.title}`);
  console.error(`  ${advisory.url}`);
}

if (unresolved.length > 0 || expired.length > 0) {
  process.exit(1);
}

console.log(
  `Production audit passed: ${dismissed.length} dismissed, no unresolved high/critical advisories.`
);
