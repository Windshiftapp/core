/** System requirement types — mirrors backend models.RequirementTypes. */
export const REQUIREMENT_TYPES = [
  'business_requirement',
  'functional_requirement',
  'non_functional_requirement',
  'business_rule',
  'use_case',
  'business_process',
  'system_specification',
  'api_specification',
  'data_model',
  'architecture_decision',
  'glossary_entry',
];

export function requirementTypeOptions(t) {
  return REQUIREMENT_TYPES.map((value) => ({
    value,
    label: t(`requirements.type.${value}`),
  }));
}
