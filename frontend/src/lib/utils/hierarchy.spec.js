import { describe, expect, it } from 'vitest';
import { canHaveChildren, childItemTypesForParent } from './hierarchy.js';

const types = [
  { id: 1, hierarchy_level: 1 },
  { id: 2, hierarchy_level: 2 },
  { id: 3, hierarchy_level: 3 },
  { id: 4, hierarchy_level: 3 },
  { id: 9, hierarchy_level: -1 },
];

describe('child item types for a parent', () => {
  it('level-2 parent yields level-3 plus generic types', () => {
    expect(childItemTypesForParent(types, types[1]).map((x) => x.id)).toEqual([3, 4, 9]);
  });
  it('deepest-level parent without generic subtask types cannot have children', () => {
    expect(
      canHaveChildren(
        types.filter((x) => x.id !== 9),
        types[2]
      )
    ).toBe(false);
  });
  it('generic-subtask parent cannot have children', () => {
    expect(canHaveChildren(types, types[4])).toBe(false);
  });
});
