#!/usr/bin/env node

/**
 * i18n Validation Script
 *
 * Validates locale files against English (reference locale) and detects:
 * - Missing-key coverage and invalid extra keys in non-English locales
 *   (missing keys use the application's English runtime fallback)
 * - Source keys referenced in code but missing from English catalog
 * - Placeholder mismatches between English and other locales
 * - Untranslated English carryovers in non-English locales
 *
 * Usage: node frontend/scripts/check-i18n.js [--verbose]
 */

import { glob, readdir, readFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ENGLISH_FALLBACK_KEYS } from '../src/lib/locales/adminOperationsFallback.js';
import { mergeInto } from '../src/lib/locales/createLocale.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const LOCALES_DIR = join(__dirname, '..', 'src', 'lib', 'locales');
const SRC_DIR = join(__dirname, '..', 'src');
const REFERENCE_LOCALE = 'en';
// Russian is shipped as a fully reviewed locale and must never silently fall
// back to English. Older locales may remain partially translated while still
// using the application's documented English runtime fallback.
const REQUIRED_FULL_COVERAGE_LOCALES = new Set(['ru']);
const PLURAL_SUFFIX_PATTERN = /_(zero|one|two|few|many|other)$/;

// These values intentionally retain product names, code syntax, URLs, or sample identifiers.
const INTENTIONAL_CARRYOVERS = new Set([
  'about.libCharmbracelet',
  'channel.microsoftOrGoogle',
  'collections.queryPlaceholder',
  'iterations.iterationNamePlaceholder',
  'jiraImport.form.urlCloud',
  'jiraImport.form.urlDatacenter',
  'jiraImport.title.cloud',
  'jiraImport.title.datacenter',
  'portal.qlQueryFormPlaceholder',
  'portal.qlQueryPlaceholder',
  'settings.sso.title',
  'workspaces.customers.placeholders.phone',
]);
const ENGLISH_FUNCTION_WORDS = new Set([
  'a',
  'an',
  'and',
  'are',
  'for',
  'from',
  'is',
  'of',
  'that',
  'the',
  'this',
  'to',
  'with',
  'you',
  'your',
]);

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

function flattenAll(obj, prefix = '') {
  const entries = [];
  for (const [key, value] of Object.entries(obj)) {
    const fullKey = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
      entries.push(...flattenAll(value, fullKey));
    } else {
      entries.push([fullKey, value]);
    }
  }
  return entries;
}

function extractPlaceholders(str) {
  if (typeof str !== 'string') return [];
  const matches = str.match(/\{(\w+)\}/g);
  return matches ? matches.map((m) => m.slice(1, -1)).sort() : [];
}

function splitPluralKey(key) {
  const match = key.match(PLURAL_SUFFIX_PATTERN);
  if (!match) return null;

  return {
    baseKey: key.slice(0, -match[0].length),
    category: match[1],
  };
}

function getPluralCategories(localeCode) {
  return new Set(new Intl.PluralRules(localeCode).resolvedOptions().pluralCategories);
}

function collectPluralBases(keys) {
  const categoriesByBase = new Map();

  for (const key of keys) {
    const plural = splitPluralKey(key);
    if (!plural) continue;

    const categories = categoriesByBase.get(plural.baseKey) ?? new Set();
    categories.add(plural.category);
    categoriesByBase.set(plural.baseKey, categories);
  }

  return new Set(
    [...categoriesByBase]
      .filter(([, categories]) => categories.has('other') && categories.size > 1)
      .map(([baseKey]) => baseKey)
  );
}

function isLocaleSpecificPluralKey(key, pluralBases, localePluralCategories) {
  const plural = splitPluralKey(key);
  return (
    plural !== null &&
    pluralBases.has(plural.baseKey) &&
    localePluralCategories.has(plural.category)
  );
}

function findReferenceEntry(key, refEntries, pluralBases, localePluralCategories) {
  if (Object.hasOwn(refEntries, key)) {
    return { key, value: refEntries[key] };
  }

  if (!isLocaleSpecificPluralKey(key, pluralBases, localePluralCategories)) {
    return null;
  }

  const { baseKey } = splitPluralKey(key);
  const fallbackKey = [`${baseKey}_other`, `${baseKey}_one`].find((candidate) =>
    Object.hasOwn(refEntries, candidate)
  );

  return fallbackKey ? { key: fallbackKey, value: refEntries[fallbackKey] } : null;
}

function findSourceFile(key, fileMap) {
  for (const [filename, keys] of Object.entries(fileMap)) {
    if (keys.has(key)) return filename;
  }
  return '?';
}

async function loadLocaleFiles(localeCode) {
  const localeDir = join(LOCALES_DIR, localeCode);
  const files = (await readdir(localeDir))
    .filter((f) => f.endsWith('.js') && f !== 'index.js')
    .sort((a, b) => {
      const rank = (file) =>
        file === 'review.js'
          ? 3
          : file === 'quality.js'
            ? 2
            : file === 'supplemental.js'
              ? 1
              : file === 'dashboard.js'
                ? 0.5
                : 0;
      return rank(a) - rank(b) || a.localeCompare(b);
    });

  const merged = {};
  const fileKeyMap = {};
  const fallbackKeys = new Set();
  const explicitKeys = new Set();

  for (const file of files) {
    const mod = await import(join(localeDir, file));
    const data = mod.default || mod;
    const keys = new Set(flattenKeys(data));
    const fileFallbackKeys = data[ENGLISH_FALLBACK_KEYS] ?? new Set();
    fileKeyMap[file] = keys;
    for (const key of keys) {
      if (fileFallbackKeys.has(key)) fallbackKeys.add(key);
      else explicitKeys.add(key);
    }
    mergeInto(merged, data);
  }

  for (const key of explicitKeys) fallbackKeys.delete(key);

  return { merged, fileKeyMap, fallbackKeys };
}

async function extractSourceKeys() {
  const pattern = join(SRC_DIR, '**/*.{svelte,js,ts}');
  const files = await Array.fromAsync(
    glob(pattern, { exclude: (f) => f.includes('node_modules') || f.includes('locales') })
  );
  const keys = new Set();

  for (const file of files) {
    const content = await readFile(file, 'utf-8');
    const matches = content.matchAll(/\bt\(['"]([a-zA-Z][a-zA-Z0-9_.]+)['"]/g);
    for (const match of matches) {
      keys.add(match[1]);
    }
    const metadataMatches = content.matchAll(
      /\b(?:name|description|label|title|subtitle)Key:\s*['"]([a-zA-Z][a-zA-Z0-9_.]+)['"]/g
    );
    for (const match of metadataMatches) {
      keys.add(match[1]);
    }
  }

  return keys;
}

function detectCarryovers(english, other, localeCode, fallbackKeys = new Set()) {
  const carryovers = [];
  const enEntries = Object.fromEntries(
    flattenAll(english).filter(([, v]) => typeof v === 'string')
  );
  const pluralBases = collectPluralBases(Object.keys(enEntries));
  const localePluralCategories = getPluralCategories(localeCode);

  for (const [key, value] of flattenAll(other)) {
    if (INTENTIONAL_CARRYOVERS.has(key)) continue;
    if (fallbackKeys.has(key)) continue;
    if (typeof value !== 'string') continue;
    const reference = findReferenceEntry(key, enEntries, pluralBases, localePluralCategories);
    if (!reference) continue;

    const enValue = reference.value;

    const words = value.split(/\s+/);
    const wordCount = words.length;

    if (enValue === value && wordCount >= 3) {
      carryovers.push({ key, value, enValue, matchRatio: 1 });
      continue;
    }

    if (enValue === value) continue;

    const englishWordsInValue = value.match(/[A-Za-z][A-Za-z'-]*/g) ?? [];
    if (englishWordsInValue.length >= 3) {
      const enWords = enValue.match(/[A-Za-z][A-Za-z'-]*/g) ?? [];
      const matching = englishWordsInValue.filter((w) => enWords.includes(w));
      const matchingWords = matching.length;
      const matchRatio = matchingWords / englishWordsInValue.length;
      const functionWordMatches = matching.filter((word) =>
        ENGLISH_FUNCTION_WORDS.has(word.toLowerCase())
      ).length;
      if (matchingWords >= 4 && functionWordMatches >= 2 && matchRatio > 0.7) {
        carryovers.push({ key, value, enValue, matchRatio });
      }
    }
  }

  return carryovers;
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

  let exitCode = 0;

  const ref = await loadLocaleFiles(REFERENCE_LOCALE);
  const refLeafKeys = new Set(flattenKeys(ref.merged));
  const refEntries = Object.fromEntries(
    flattenAll(ref.merged).filter(([, v]) => typeof v === 'string')
  );
  const refPluralBases = collectPluralBases(refLeafKeys);

  console.log(`\n  Reference: ${REFERENCE_LOCALE} (${refLeafKeys.size} leaf keys)\n`);

  // --- Check 1: Source keys missing from English ---
  console.log('  --- Source key presence ---');
  const sourceKeys = await extractSourceKeys();
  const missingFromEn = [...sourceKeys]
    .filter((k) => !refLeafKeys.has(k))
    .filter((k) => k.includes('.'))
    .sort();

  if (missingFromEn.length > 0) {
    console.log(`  ✗ ${missingFromEn.length} source key(s) missing from English catalog:`);
    for (const key of missingFromEn) {
      console.log(`      - ${key}`);
    }
    exitCode = 1;
  } else {
    console.log('  ✓ All source keys present in English catalog');
  }
  console.log('');

  // --- Check 2: Non-English locale parity ---
  console.log('  --- Locale key parity ---');
  const otherLocales = localeDirs.filter((l) => l !== REFERENCE_LOCALE).sort();
  let totalMissing = 0;
  let requiredMissing = 0;
  let totalExtra = 0;

  for (const locale of otherLocales) {
    const loc = await loadLocaleFiles(locale);
    const locKeys = new Set(flattenKeys(loc.merged));
    const localePluralCategories = getPluralCategories(locale);

    const missing = [...refLeafKeys].filter((k) => !locKeys.has(k)).sort();
    const localePluralVariants = [...locKeys]
      .filter(
        (k) =>
          !refLeafKeys.has(k) &&
          isLocaleSpecificPluralKey(k, refPluralBases, localePluralCategories)
      )
      .sort();
    const extra = [...locKeys]
      .filter(
        (k) =>
          !refLeafKeys.has(k) &&
          !isLocaleSpecificPluralKey(k, refPluralBases, localePluralCategories)
      )
      .sort();

    totalMissing += missing.length;
    if (REQUIRED_FULL_COVERAGE_LOCALES.has(locale)) requiredMissing += missing.length;
    totalExtra += extra.length;

    const coverage = (((refLeafKeys.size - missing.length) / refLeafKeys.size) * 100).toFixed(1);

    if (missing.length === 0 && extra.length === 0) {
      const pluralSuffix =
        localePluralVariants.length > 0
          ? `, ${localePluralVariants.length} locale plural variant(s)`
          : '';
      console.log(`  ✓ ${locale}  ${coverage}% coverage  (${locKeys.size} keys${pluralSuffix})`);
    } else if (extra.length > 0 || REQUIRED_FULL_COVERAGE_LOCALES.has(locale)) {
      console.log(
        `  ✗ ${locale}  ${coverage}% coverage  (${locKeys.size} keys, ${missing.length} missing, ${extra.length} extra)`
      );
    } else {
      console.log(
        `  ⚠ ${locale}  ${coverage}% coverage  (${locKeys.size} keys, ${missing.length} using English fallback)`
      );
    }

    if (missing.length > 0 && verbose) {
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
  }
  console.log('');

  // --- Check 3: Placeholder mismatches ---
  console.log('  --- Placeholder parity ---');
  let placeholderErrors = 0;

  for (const locale of otherLocales) {
    const loc = await loadLocaleFiles(locale);
    const locEntries = Object.fromEntries(
      flattenAll(loc.merged).filter(([, v]) => typeof v === 'string')
    );
    const localePluralCategories = getPluralCategories(locale);

    const mismatches = [];
    for (const [key, locValue] of Object.entries(locEntries)) {
      const reference = findReferenceEntry(key, refEntries, refPluralBases, localePluralCategories);
      if (!reference || !locValue) continue;

      const enValue = reference.value;

      const enPlaceholders = extractPlaceholders(enValue);
      const locPlaceholders = extractPlaceholders(locValue);

      if (enPlaceholders.length === 0 && locPlaceholders.length === 0) continue;

      const locSet = new Set(locPlaceholders);

      const missing = enPlaceholders.filter((p) => !locSet.has(p) && p !== 'plural');
      const enSet = new Set(enPlaceholders.filter((p) => p !== 'plural'));
      const extra = locPlaceholders.filter((p) => p !== 'plural' && !enSet.has(p));

      if (missing.length > 0 || extra.length > 0) {
        mismatches.push({
          key,
          referenceKey: reference.key,
          enValue,
          locValue,
          missing,
          extra,
        });
      }
    }

    if (mismatches.length > 0) {
      console.log(`  ✗ ${locale}: ${mismatches.length} placeholder mismatch(es):`);
      for (const m of mismatches) {
        const details = [];
        if (m.missing.length > 0) details.push(`missing: {${m.missing.join('}, {')}}`);
        if (m.extra.length > 0) details.push(`extra: {${m.extra.join('}, {')}}`);
        console.log(`      ${m.key} — ${details.join(', ')}`);
        if (verbose) {
          const referenceLabel = m.referenceKey === m.key ? 'EN' : `EN (${m.referenceKey})`;
          console.log(`        ${referenceLabel}: ${m.enValue}`);
          console.log(`        ${locale}: ${m.locValue}`);
        }
      }
      placeholderErrors += mismatches.length;
      exitCode = 1;
    } else {
      console.log(`  ✓ ${locale}: all placeholders match`);
    }
  }

  if (placeholderErrors === 0) {
    console.log('  ✓ All placeholders match across locales');
  }
  console.log('');

  // --- Check 4: Untranslated carryovers ---
  console.log('  --- Untranslated carryover detection ---');
  let carryoverTotal = 0;

  for (const locale of otherLocales) {
    const loc = await loadLocaleFiles(locale);
    const carryovers = detectCarryovers(ref.merged, loc.merged, locale, loc.fallbackKeys);

    if (carryovers.length > 0) {
      console.log(`  ✗ ${locale}: ${carryovers.length} suspected carryover(s):`);
      if (verbose) {
        for (const c of carryovers.slice(0, 20)) {
          console.log(`      ${c.key}`);
        }
        if (carryovers.length > 20) {
          console.log(`      ... and ${carryovers.length - 20} more`);
        }
      }
      carryoverTotal += carryovers.length;
      exitCode = 1;
    } else {
      console.log(`  ✓ ${locale}: no obvious carryovers detected`);
    }
  }

  if (carryoverTotal === 0) {
    console.log('  ✓ No carryovers detected');
  }
  console.log('');

  // --- Summary ---
  if (requiredMissing > 0 || totalExtra > 0) {
    exitCode = 1;
  }

  if (exitCode === 0) {
    console.log('  All checks passed!\n');
  } else {
    const issues = [];
    if (missingFromEn.length > 0)
      issues.push(`${missingFromEn.length} source key(s) missing from English`);
    if (totalMissing > 0) issues.push(`${totalMissing} locale key(s) using English fallback`);
    if (requiredMissing > 0) issues.push(`${requiredMissing} required locale key(s) missing`);
    if (totalExtra > 0) issues.push(`${totalExtra} extra locale key(s)`);
    if (placeholderErrors > 0) issues.push(`${placeholderErrors} placeholder mismatch(es)`);
    if (carryoverTotal > 0) issues.push(`${carryoverTotal} suspected carryover(s)`);
    console.log(`  ${issues.join(', ')}`);
    console.log('');
    process.exit(1);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
