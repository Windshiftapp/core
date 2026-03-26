#!/usr/bin/env node

/**
 * i18n Key Comparison Script
 *
 * Compares all locale files against English (reference locale)
 * and highlights missing or extra keys.
 *
 * Usage: node frontend/scripts/check-i18n.js [--verbose]
 */

import { readdir } from 'node:fs/promises';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const LOCALES_DIR = join(__dirname, '..', 'src', 'lib', 'locales');
const REFERENCE_LOCALE = 'en';

function flattenKeys(obj, prefix = '') {
  const keys = [];
  for (const [key, value] of Object.entries(obj)) {
    const fullKey = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
      keys.push(...flattenKeys(value, fullKey));
    } else {
      keys.push(fullKey);
    }
  }
  return keys;
}

function findSourceFile(key, fileMap) {
  for (const [filename, keys] of Object.entries(fileMap)) {
    if (keys.has(key)) return filename;
  }
  return '?';
}

async function loadLocaleFiles(localeCode) {
  const localeDir = join(LOCALES_DIR, localeCode);
  const files = (await readdir(localeDir)).filter(
    (f) => f.endsWith('.js') && f !== 'index.js'
  );

  const merged = {};
  const fileKeyMap = {};

  for (const file of files) {
    const mod = await import(join(localeDir, file));
    const data = mod.default || mod;
    const keys = new Set(flattenKeys(data));
    fileKeyMap[file] = keys;
    Object.assign(merged, data);
  }

  return { merged, fileKeyMap };
}

async function main() {
  const verbose = process.argv.includes('--verbose');

  const localeDirs = (await readdir(LOCALES_DIR, { withFileTypes: true }))
    .filter((d) => d.isDirectory())
    .map((d) => d.name);

  if (!localeDirs.includes(REFERENCE_LOCALE)) {
    console.error(`Reference locale "${REFERENCE_LOCALE}" not found.`);
    process.exit(1);
  }

  // Load reference locale
  const ref = await loadLocaleFiles(REFERENCE_LOCALE);
  const refKeys = new Set(flattenKeys(ref.merged));

  console.log(`\n  Reference: ${REFERENCE_LOCALE} (${refKeys.size} keys)\n`);

  const otherLocales = localeDirs.filter((l) => l !== REFERENCE_LOCALE).sort();
  let totalMissing = 0;
  let totalExtra = 0;

  for (const locale of otherLocales) {
    const loc = await loadLocaleFiles(locale);
    const locKeys = new Set(flattenKeys(loc.merged));

    const missing = [...refKeys].filter((k) => !locKeys.has(k)).sort();
    const extra = [...locKeys].filter((k) => !refKeys.has(k)).sort();

    totalMissing += missing.length;
    totalExtra += extra.length;

    const coverage = (((refKeys.size - missing.length) / refKeys.size) * 100).toFixed(1);

    if (missing.length === 0 && extra.length === 0) {
      console.log(`  ✓ ${locale}  ${coverage}% coverage  (${locKeys.size} keys)`);
    } else {
      console.log(
        `  ✗ ${locale}  ${coverage}% coverage  (${locKeys.size} keys, ${missing.length} missing, ${extra.length} extra)`
      );
    }

    if (missing.length > 0) {
      // Group missing keys by source file
      const byFile = {};
      for (const key of missing) {
        const file = findSourceFile(key, ref.fileKeyMap);
        (byFile[file] ??= []).push(key);
      }
      for (const [file, keys] of Object.entries(byFile).sort()) {
        console.log(`      missing in ${file}:`);
        for (const key of keys) {
          console.log(`        - ${key}`);
        }
      }
    }

    if (extra.length > 0 && verbose) {
      const byFile = {};
      for (const key of extra) {
        const file = findSourceFile(key, loc.fileKeyMap);
        (byFile[file] ??= []).push(key);
      }
      for (const [file, keys] of Object.entries(byFile).sort()) {
        console.log(`      extra in ${file}:`);
        for (const key of keys) {
          console.log(`        + ${key}`);
        }
      }
    }
  }

  console.log('');
  if (totalMissing === 0 && totalExtra === 0) {
    console.log('  All locales are in sync!\n');
  } else {
    if (totalMissing > 0) console.log(`  ${totalMissing} missing key(s) total`);
    if (totalExtra > 0) console.log(`  ${totalExtra} extra key(s) total (use --verbose to list)`);
    console.log('');
    process.exit(1);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
