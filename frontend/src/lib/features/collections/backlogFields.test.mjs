import { describe, expect, it } from 'vitest';
import { DEFAULT_BACKLOG_FIELDS, normalizeBacklogFields } from './backlogFields.js';

describe('backlog field configuration', () => {
  it('uses the current backlog metadata as the default', () => {
    expect(normalizeBacklogFields(undefined)).toEqual(DEFAULT_BACKLOG_FIELDS);
  });

  it('allows every field to be hidden', () => {
    expect(normalizeBacklogFields([])).toEqual([]);
  });

  it('drops unknown and duplicate stored fields', () => {
    expect(normalizeBacklogFields(['parent', 'unknown', 'parent', 'assignee'])).toEqual([
      'parent',
      'assignee',
    ]);
  });
});
