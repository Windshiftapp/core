import { mergeInto } from './createLocale.js';
import english from './en/adminOperations.js';

export const ENGLISH_FALLBACK_KEYS = Symbol.for('windshift.i18n.englishFallbackKeys');

function flattenKeys(object, prefix = '') {
  const keys = [];
  for (const [key, value] of Object.entries(object)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
      keys.push(...flattenKeys(value, path));
    } else {
      keys.push(path);
    }
  }
  return keys;
}

export function withEnglishAdminOperations(overrides) {
  const result = mergeInto(structuredClone(english), overrides);
  const overrideKeys = new Set(flattenKeys(overrides));
  const fallbackKeys = new Set(flattenKeys(english).filter((key) => !overrideKeys.has(key)));

  Object.defineProperty(result, ENGLISH_FALLBACK_KEYS, {
    value: fallbackKeys,
    enumerable: false,
  });

  return result;
}
