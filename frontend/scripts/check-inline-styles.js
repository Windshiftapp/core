#!/usr/bin/env node

/**
 * Guard against new inline color styles.
 *
 * Every solid --ds-* color is exposed as a Tailwind utility (text-ds-*,
 * bg-ds-*, border-ds-*), so `style="color: var(--ds-text)"` is not necessary.
 * Existing occurrences are baselined per file and may only shrink; a file that
 * is not in the baseline must have none.
 *
 * Transparent values are exempt. Tailwind's opacity handling mangles
 * already-transparent colors, so rgba()/hsla(), 8-digit hex, `transparent`,
 * color-mix(), and the semi-transparent glass tokens stay as inline styles or
 * component CSS.
 *
 * This guard is intentionally not part of `npm run check`: it is CPU-heavy and
 * is only run when Svelte files change (scripts/pre-commit and the
 * ci-frontend target), so unrelated frontend commits do not pay for it.
 *
 * Usage:
 *   node scripts/check-inline-styles.js            # verify against baseline
 *   node scripts/check-inline-styles.js --update   # rewrite the baseline
 */

import { readdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(scriptDir, '..');
const srcDir = path.join(rootDir, 'src');
const baselinePath = path.join(scriptDir, 'inline-style-baseline.json');
const exemptionMarker = 'inline-style-guard-exempt';

const STYLE_ATTRIBUTE = /style\s*=\s*"([^"]*)"/g;

// Properties that carry a color. Shorthands (background/border/outline) are
// included because they can embed one.
const COLOR_PROPERTY =
  /(?:^|;|\s)(background|background-color|border|border-color|border-top|border-right|border-bottom|border-left|border-top-color|border-right-color|border-bottom-color|border-left-color|outline|outline-color|color|fill|stroke|caret-color|accent-color|text-decoration-color)\s*:\s*([^;]+)/g;

const COLOR_VALUE = /var\(--ds-|#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/;

function lineFor(source, index) {
  return source.slice(0, index).split('\n').length;
}

/**
 * A value that Tailwind cannot round-trip: already-transparent colors.
 * @param {string} value
 */
function isTransparent(value) {
  const v = value.trim().toLowerCase();
  if (v.includes('transparent')) return true;
  if (v.includes('var(--ds-glass-')) return true;
  if (v.includes('color-mix(')) return true;

  const alphaFn = v.match(/(?:rgba|hsla)\([^)]*?,\s*([0-9.]+%?)\s*\)/);
  if (alphaFn) {
    const raw = alphaFn[1];
    const numeric = raw.endsWith('%') ? Number.parseFloat(raw) / 100 : Number.parseFloat(raw);
    if (Number.isFinite(numeric) && numeric < 1) return true;
  }

  const hex8 = v.match(/#([0-9a-f]{8})\b/);
  if (hex8 && hex8[1].slice(6) !== 'ff') return true;

  const hex4 = v.match(/#([0-9a-f]{4})\b/);
  if (hex4 && hex4[1][3] !== 'f') return true;

  return false;
}

async function listSvelteFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    if (entry.name === 'node_modules' || entry.name === 'dist') continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) files.push(...(await listSvelteFiles(full)));
    else if (entry.isFile() && entry.name.endsWith('.svelte')) files.push(full);
  }
  return files;
}

function findViolations(source) {
  const violations = [];
  for (const styleMatch of source.matchAll(STYLE_ATTRIBUTE)) {
    const styleIndex = styleMatch.index ?? 0;
    const recent = source.slice(Math.max(0, styleIndex - 260), styleIndex);
    if (recent.includes(exemptionMarker)) continue;

    for (const propMatch of styleMatch[1].matchAll(COLOR_PROPERTY)) {
      const value = propMatch[2];
      if (!COLOR_VALUE.test(value)) continue;
      if (isTransparent(value)) continue;
      violations.push({ line: lineFor(source, styleIndex), property: propMatch[1].trim() });
    }
  }
  return violations;
}

async function collect() {
  const counts = new Map();
  const examples = new Map();
  for (const file of await listSvelteFiles(srcDir)) {
    const relative = path.relative(rootDir, file).split(path.sep).join('/');
    const violations = findViolations(await readFile(file, 'utf8'));
    if (violations.length === 0) continue;
    counts.set(relative, violations.length);
    examples.set(relative, violations[0]);
  }
  return { counts, examples };
}

const update = process.argv.slice(2).includes('--update');

if (update) {
  const { counts } = await collect();
  const files = [...counts.entries()]
    .map(([file, count]) => ({ file, count }))
    .sort((a, b) => a.file.localeCompare(b.file));
  const total = files.reduce((sum, entry) => sum + entry.count, 0);
  await writeFile(baselinePath, `${JSON.stringify({ version: 1, total, files }, null, 2)}\n`);
  console.log(`Updated inline-style baseline: ${total} occurrence(s) in ${files.length} file(s).`);
  process.exit(0);
}

const { counts, examples } = await collect();

let baseline;
try {
  baseline = JSON.parse(await readFile(baselinePath, 'utf8'));
} catch {
  console.error('Missing scripts/inline-style-baseline.json; run with --update.');
  process.exit(1);
}
const allowed = new Map((baseline.files ?? []).map((entry) => [entry.file, entry.count]));

const regressions = [];
for (const [file, count] of counts) {
  const limit = allowed.get(file) ?? 0;
  if (count > limit) regressions.push({ file, count, limit, example: examples.get(file) });
}
regressions.sort((a, b) => a.file.localeCompare(b.file));

if (regressions.length > 0) {
  console.error('Inline color-style guard failed. Use the design-token Tailwind utilities');
  console.error('(text-ds-*, bg-ds-*, border-ds-*) instead of inline color styles:');
  for (const regression of regressions) {
    console.error(
      `  ${regression.file}:${regression.example.line} (${regression.count} now, ${regression.limit} allowed) ${regression.example.property}`
    );
  }
  console.error('');
  console.error('Transparent values (rgba/hsla, 8-digit hex, glass tokens, color-mix) are exempt.');
  console.error('If a style genuinely cannot move, add an inline-style-guard-exempt comment.');
  process.exit(1);
}

const total = [...counts.values()].reduce((sum, count) => sum + count, 0);
console.log(`Inline-style guard passed (${total} baselined occurrence(s), no new ones).`);
