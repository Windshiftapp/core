#!/usr/bin/env node
// Fails when editing a non-Svelte module would reload the page in dev.
//
// Vite propagates a hot update up the importer graph until it reaches a module
// that accepts it. Svelte components accept their own updates; `main.js` does
// not. Any JS module with an importer chain to `main.js` that avoids a Svelte
// component therefore reloads the whole page.

import { readFile } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const defaultSrcDir = path.resolve(scriptDir, '../src');
const defaultEntry = 'main.js';

const MODULE_EXTENSIONS = ['.js', '.ts', '.mjs', '.cjs', '.svelte'];
const RESOLVE_SUFFIXES = ['', '.js', '.ts', '.mjs', '.cjs', '.svelte'];
const RESOLVE_INDEXES = ['index.js', 'index.ts', 'index.mjs', 'index.svelte'];

// Entry-level modules that run before any Svelte component mounts. Each needs a
// reason: without one the list rots into an allowlist for real regressions.
export const ENTRY_ALLOWLIST = Object.freeze([
  {
    file: 'lib/runtime/contextPath.js',
    reason: 'installed before the app chunk loads so base URLs are correct for every later import',
  },
  {
    file: 'lib/mobile/serviceWorkerClient.js',
    reason: 'registered by main.js before the app chunk so PWA updates survive startup failures',
  },
]);

const IMPORT_PATTERNS = [
  // import './side-effect.js'
  /\bimport\s*['"]([^'"]+)['"]/g,
  // import x from './x.js', import { a, b } from "./x.js", import type …
  /\bimport\s+(?:type\s+)?[\w$*{}\s,]+?\bfrom\s*['"]([^'"]+)['"]/g,
  // export { x } from './x.js', export * from './x.js'
  /\bexport\s+(?:type\s+)?(?:[\w$*{}\s,]+?\bfrom\s*)['"]([^'"]+)['"]/g,
  // import('./x.js')
  /\bimport\s*\(\s*['"]([^'"]+)['"]\s*[,)]/g,
];

/** True when a file is a Svelte component, i.e. an HMR boundary. */
export function isSvelteComponent(file) {
  return file.endsWith('.svelte');
}

function isModuleFile(file) {
  return MODULE_EXTENSIONS.some((extension) => file.endsWith(extension));
}

/**
 * Blank out comments while keeping the rest of the source (and its string
 * literals) intact, so a commented-out import is not mistaken for a real one.
 * @param {string} source
 */
function maskComments(source) {
  let result = '';
  let index = 0;
  let quote = null;

  while (index < source.length) {
    const character = source[index];

    if (quote) {
      result += character;
      if (character === '\\') {
        result += source[index + 1] ?? '';
        index += 2;
        continue;
      }
      if (character === quote) quote = null;
      index += 1;
      continue;
    }

    if (character === '"' || character === "'" || character === '`') {
      quote = character;
      result += character;
      index += 1;
      continue;
    }

    if (character === '/' && source[index + 1] === '/') {
      while (index < source.length && source[index] !== '\n') {
        result += ' ';
        index += 1;
      }
      continue;
    }

    if (character === '/' && source[index + 1] === '*') {
      result += '  ';
      index += 2;
      while (index < source.length && !(source[index] === '*' && source[index + 1] === '/')) {
        result += ' ';
        index += 1;
      }
      if (index < source.length) {
        result += '  ';
        index += 2;
      }
      continue;
    }

    result += character;
    index += 1;
  }

  return result;
}

/**
 * Every module specifier a file imports, statically or dynamically.
 * Non-literal dynamic imports (`import(someName)`) are invisible to Vite's
 * module graph as well, so they cannot create an importer edge.
 * @param {string} source
 * @returns {string[]}
 */
export function extractImportSpecifiers(source) {
  const code = maskComments(source);
  const specifiers = new Set();
  for (const pattern of IMPORT_PATTERNS) {
    for (const match of code.matchAll(pattern)) {
      specifiers.add(match[1]);
    }
  }
  return [...specifiers];
}

/**
 * Resolve a relative import specifier to a file on disk.
 * @param {string} fromFile absolute path of the importing file
 * @param {string} specifier raw specifier
 * @param {(candidate: string) => Promise<boolean>} [exists]
 * @returns {Promise<string|null>} absolute path, or null for bare/bundled imports
 */
export async function resolveSpecifier(fromFile, specifier, exists = fileExists) {
  if (!specifier.startsWith('.')) return null; // package import; Vite pre-bundles it
  const withoutQuery = specifier.split('?')[0];
  const base = path.resolve(path.dirname(fromFile), withoutQuery);

  for (const suffix of RESOLVE_SUFFIXES) {
    const candidate = `${base}${suffix}`;
    if (await exists(candidate)) return candidate;
  }
  for (const indexFile of RESOLVE_INDEXES) {
    const candidate = path.join(base, indexFile);
    if (await exists(candidate)) return candidate;
  }
  return null;
}

async function fileExists(file) {
  try {
    const { stat } = await import('node:fs/promises');
    return (await stat(file)).isFile();
  } catch {
    return false;
  }
}

/**
 * Build the importer graph for every module reachable from the entry.
 * @param {{ srcDir: string, entry?: string, exists?: (file: string) => Promise<boolean> }} options
 * @returns {Promise<Map<string, Set<string>>>} module -> absolute importer paths
 */
export async function buildImporterGraph({ srcDir, entry = defaultEntry, exists = fileExists }) {
  const entryFile = path.join(srcDir, entry);
  const importers = new Map([[entryFile, new Set()]]);
  const queue = [entryFile];
  const unresolved = [];

  while (queue.length > 0) {
    const file = queue.shift();
    const source = await readFile(file, 'utf8');
    for (const specifier of extractImportSpecifiers(source)) {
      const resolved = await resolveSpecifier(file, specifier, exists);
      if (resolved === null) {
        if (specifier.startsWith('.')) unresolved.push({ file, specifier });
        continue;
      }
      // CSS, JSON and asset imports are leaves with no HMR graph of their own.
      if (!isModuleFile(resolved)) continue;
      if (!importers.has(resolved)) {
        importers.set(resolved, new Set());
        queue.push(resolved);
      }
      importers.get(resolved).add(file);
    }
  }

  if (unresolved.length > 0) {
    const list = unresolved
      .map(({ file, specifier }) => `  ${path.relative(srcDir, file)}: ${specifier}`)
      .join('\n');
    throw new Error(`Could not resolve relative imports:\n${list}`);
  }

  return importers;
}

/**
 * Find non-Svelte modules whose importer chain to the entry avoids every
 * Svelte component.
 * @param {Map<string, Set<string>>} importers
 * @param {{ srcDir: string, entry?: string, allowlist?: readonly { file: string, reason: string }[] }} options
 * @returns {{ file: string, chain: string[] }[]} sorted violations, chain runs from the module to the entry
 */
export function findBoundaryViolations(
  importers,
  { srcDir, entry = defaultEntry, allowlist = [] }
) {
  const entryFile = path.join(srcDir, entry);
  const allowed = new Set(allowlist.map((entryItem) => path.join(srcDir, entryItem.file)));
  const violations = [];

  for (const file of importers.keys()) {
    if (file === entryFile || isSvelteComponent(file) || allowed.has(file)) continue;

    const previous = new Map([[file, null]]);
    const queue = [file];
    let reachedEntry = false;

    while (queue.length > 0 && !reachedEntry) {
      const current = queue.shift();
      for (const importer of importers.get(current) ?? []) {
        if (previous.has(importer)) continue;
        previous.set(importer, current);
        if (importer === entryFile) {
          reachedEntry = true;
          break;
        }
        if (isSvelteComponent(importer)) continue; // HMR boundary stops propagation
        queue.push(importer);
      }
    }

    if (!reachedEntry) continue;
    const chain = [];
    let cursor = entryFile;
    while (cursor) {
      chain.push(path.relative(srcDir, cursor));
      cursor = previous.get(cursor);
    }
    violations.push({ file: path.relative(srcDir, file), chain: chain.reverse() });
  }

  return violations.sort((a, b) => a.file.localeCompare(b.file));
}

/**
 * Allowlist entries that no longer suppress anything, so the list cannot rot.
 * @param {Map<string, Set<string>>} importers
 * @param {{ srcDir: string, entry?: string, allowlist?: readonly { file: string, reason: string }[] }} options
 */
export function findStaleAllowlistEntries(
  importers,
  { srcDir, entry = defaultEntry, allowlist = [] }
) {
  const withoutAllowlist = findBoundaryViolations(importers, { srcDir, entry });
  const stillNeeded = new Set(withoutAllowlist.map((violation) => violation.file));
  return allowlist.filter((entryItem) => !stillNeeded.has(entryItem.file));
}

function formatChain(chain) {
  return chain.join(' <- ');
}

async function main() {
  const srcDir = process.argv[2] ? path.resolve(process.argv[2]) : defaultSrcDir;
  const importers = await buildImporterGraph({ srcDir });
  const violations = findBoundaryViolations(importers, { srcDir, allowlist: ENTRY_ALLOWLIST });
  const stale = findStaleAllowlistEntries(importers, { srcDir, allowlist: ENTRY_ALLOWLIST });

  if (violations.length > 0) {
    console.error(
      'These modules can reach the app entry without passing a Svelte component, so editing them\n' +
        'reloads the page in the Vite dev server:'
    );
    for (const violation of violations) {
      console.error(`\n- ${violation.file}\n  ${formatChain(violation.chain)}`);
    }
    console.error(
      '\nImport the module from a .svelte component, or add it to ENTRY_ALLOWLIST with a reason\n' +
        'when it genuinely has to run before the app chunk loads.'
    );
    process.exitCode = 1;
    return;
  }

  if (stale.length > 0) {
    console.error('ENTRY_ALLOWLIST entries that no longer need an exemption:');
    for (const entryItem of stale) {
      console.error(`- ${entryItem.file}: ${entryItem.reason}`);
    }
    process.exitCode = 1;
    return;
  }

  console.log(
    `HMR boundary guard passed (${importers.size} modules from src/${defaultEntry}, ` +
      `${ENTRY_ALLOWLIST.length} allowlisted entry modules).`
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await main();
}
