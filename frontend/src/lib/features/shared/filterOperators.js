/** Shared operator definitions for dynamic field filters. */

export const booleanOptions = [
  { value: 'true', label: 'True' },
  { value: 'false', label: 'False' },
];

export const operatorsByType = {
  text: [
    { value: '=', label: 'equals' },
    { value: '!=', label: 'does not equal' },
    { value: '~', label: 'contains' },
  ],
  number: [
    { value: '=', label: 'equals' },
    { value: '!=', label: 'does not equal' },
    { value: '<', label: 'less than' },
    { value: '<=', label: 'less than or equal' },
    { value: '>', label: 'greater than' },
    { value: '>=', label: 'greater than or equal' },
  ],
  date: [
    { value: '=', label: 'on' },
    { value: '!=', label: 'not on' },
    { value: '<', label: 'before' },
    { value: '<=', label: 'on or before' },
    { value: '>', label: 'after' },
    { value: '>=', label: 'on or after' },
  ],
  enum: [
    { value: '=', label: 'is' },
    { value: '!=', label: 'is not' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ],
  select: [
    { value: '=', label: 'is' },
    { value: '!=', label: 'is not' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ],
  boolean: [{ value: '=', label: 'is' }],
  user: [
    { value: '=', label: 'is' },
    { value: '!=', label: 'is not' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ],
  textarea: [
    { value: '=', label: 'equals' },
    { value: '!=', label: 'does not equal' },
    { value: '~', label: 'contains' },
  ],
  reference: [
    { value: '=', label: 'is' },
    { value: '!=', label: 'is not' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ],
  identifier: [
    { value: '=', label: 'equals' },
    { value: '!=', label: 'does not equal' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ],
};

export function isMultiValueOperator(operator) {
  return operator === 'IN' || operator === 'NOT IN';
}
