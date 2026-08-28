import { mergeInto } from './createLocale.js';
import english from './en/adminOperations.js';

export function withEnglishAdminOperations(overrides) {
  return mergeInto(structuredClone(english), overrides);
}
