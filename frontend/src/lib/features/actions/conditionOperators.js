// Condition operator options shared by the asset and logbook action flow
// editors. Keeping them in one place stops the two editors from drifting.
export const conditionOperators = [
  { value: 'eq', label: 'Equals' },
  { value: 'ne', label: 'Not Equals' },
  { value: 'contains', label: 'Contains' },
  { value: 'not_contains', label: 'Not Contains' },
  { value: 'starts_with', label: 'Starts With' },
  { value: 'ends_with', label: 'Ends With' },
];

// Text-bearing triggers (documents) additionally support regex matching.
export const conditionOperatorsWithRegex = [
  ...conditionOperators,
  { value: 'matches', label: 'Matches (Regex)' },
];
