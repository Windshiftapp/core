import { REQUIREMENT_TYPES } from './requirementTypes.js';

/** @param {string} type */
function isKnownRequirementType(type) {
  return REQUIREMENT_TYPES.includes(type);
}

/**
 * @param {string} requirementType
 * @param {(key: string) => string} t
 */
export function getRequirementStarterContent(requirementType, t) {
  if (!isKnownRequirementType(requirementType)) return '';
  const key = `requirements.templates.${requirementType}.body`;
  const body = t(key);
  if (!body || body === key) return '';
  return body;
}
