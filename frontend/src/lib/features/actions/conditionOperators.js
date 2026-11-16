// Condition operator options shared by the asset and workspace action flow
// editors. Keeping them in one place stops the two editors from drifting.
const operatorKeys = {
  eq: 'equals',
  ne: 'notEquals',
  contains: 'contains',
  not_contains: 'notContains',
  starts_with: 'startsWith',
  ends_with: 'endsWith',
  gt: 'greaterThan',
  lt: 'lessThan',
  is_empty: 'isEmpty',
  is_not_empty: 'isNotEmpty',
  matches: 'matches',
};
const assetLabels = [
  'Equals',
  'Not Equals',
  'Contains',
  'Not Contains',
  'Starts With',
  'Ends With',
];
export const conditionOperators = [
  'eq',
  'ne',
  'contains',
  'not_contains',
  'starts_with',
  'ends_with',
].map((value, index) => ({ value, label: assetLabels[index] }));

export function conditionOperatorLabel(operator, t) {
  return Object.hasOwn(operatorKeys, operator)
    ? t(`actions.operators.${operatorKeys[operator]}`)
    : operator;
}

export function workspaceConditionOperators(t) {
  return ['eq', 'ne', 'contains', 'gt', 'lt', 'is_empty', 'is_not_empty'].map((value) => ({
    value,
    label: conditionOperatorLabel(value, t),
  }));
}

// Text-bearing triggers (documents) additionally support regex matching.
export const conditionOperatorsWithRegex = [
  ...conditionOperators,
  { value: 'matches', label: 'Matches (Regex)' },
];
