/** Shared operator definitions for dynamic field filters. */

export const booleanOptions = [
  { value: 'true', label: 'True' },
  { value: 'false', label: 'False' },
];

const nullOperators = [
  { value: 'IS NULL', label: 'is empty' },
  { value: 'IS NOT NULL', label: 'is not empty' },
];

const withNullOperators = (operators) => [...operators, ...nullOperators];

export const operatorsByType = {
  text: withNullOperators([
    { value: '=', label: 'equals' },
    { value: '!=', label: 'does not equal' },
    { value: '~', label: 'contains' },
  ]),
  number: withNullOperators([
    { value: '=', label: 'equals' },
    { value: '!=', label: 'does not equal' },
    { value: '<', label: 'less than' },
    { value: '<=', label: 'less than or equal' },
    { value: '>', label: 'greater than' },
    { value: '>=', label: 'greater than or equal' },
  ]),
  date: withNullOperators([
    { value: '=', label: 'on' },
    { value: '!=', label: 'not on' },
    { value: '<', label: 'before' },
    { value: '<=', label: 'on or before' },
    { value: '>', label: 'after' },
    { value: '>=', label: 'on or after' },
  ]),
  enum: withNullOperators([
    { value: '=', label: 'is' },
    { value: '!=', label: 'is not' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ]),
  select: withNullOperators([
    { value: '=', label: 'is' },
    { value: '!=', label: 'is not' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ]),
  multiselect: withNullOperators([
    { value: '=', label: 'is' },
    { value: '!=', label: 'is not' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ]),
  boolean: withNullOperators([{ value: '=', label: 'is' }]),
  user: withNullOperators([
    { value: '=', label: 'is' },
    { value: '!=', label: 'is not' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ]),
  textarea: withNullOperators([
    { value: '=', label: 'equals' },
    { value: '!=', label: 'does not equal' },
    { value: '~', label: 'contains' },
  ]),
  reference: withNullOperators([
    { value: '=', label: 'is' },
    { value: '!=', label: 'is not' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ]),
  identifier: withNullOperators([
    { value: '=', label: 'equals' },
    { value: '!=', label: 'does not equal' },
    { value: 'IN', label: 'is one of' },
    { value: 'NOT IN', label: 'is not one of' },
  ]),
};

export function isMultiValueOperator(operator) {
  return operator === 'IN' || operator === 'NOT IN';
}

export function isNullOperator(operator) {
  return operator === 'IS NULL' || operator === 'IS NOT NULL';
}
