import { describe, expect, test } from 'vitest';
import { QLBuilder } from './ql.js';

describe('QLBuilder.tryParseToBuilder', () => {
  test('returns null on empty input', () => {
    expect(QLBuilder.tryParseToBuilder('')).toBeNull();
    expect(QLBuilder.tryParseToBuilder('   ')).toBeNull();
  });

  test('recognizes workspace, status, priority, and title clauses', () => {
    const r = QLBuilder.tryParseToBuilder(
      'workspace = "alpha" AND status_id IN (1, 2) AND priority_id = 3 AND title ~ "foo"'
    );
    expect(r).toEqual({
      workspaces: ['alpha'],
      statuses: [1, 2],
      priorities: [3],
      search: 'foo',
      dynamicFields: [],
      dropped: false,
    });
  });

  test('recovers a backticked custom-field equality clause as a dynamicField', () => {
    const r = QLBuilder.tryParseToBuilder('`cf_stage` = "draft"');
    expect(r.dynamicFields).toEqual([
      {
        field: { id: 'cf_stage', type: 'text', name: 'cf_stage' },
        operator: '=',
        value: 'draft',
        values: [],
      },
    ]);
    expect(r.dropped).toBe(false);
  });

  test('uses provided custom-field catalog to resolve type and label', () => {
    const r = QLBuilder.tryParseToBuilder('`cf_priority` = 5', {
      customFields: [{ id: 'cf_priority', type: 'number', name: 'Priority Score' }],
    });
    expect(r.dynamicFields[0].field).toEqual({
      id: 'cf_priority',
      type: 'number',
      name: 'Priority Score',
    });
    expect(r.dynamicFields[0].operator).toBe('=');
    expect(r.dynamicFields[0].value).toBe('5');
  });

  test('recovers IN list with quoted values', () => {
    const r = QLBuilder.tryParseToBuilder('`cf_stage` IN ("draft", "review")');
    expect(r.dynamicFields[0]).toMatchObject({
      operator: 'IN',
      values: ['draft', 'review'],
    });
  });

  test('recovers NOT IN with numeric values and infers number type', () => {
    const r = QLBuilder.tryParseToBuilder('`cf_score` NOT IN (1, 2, 3)');
    expect(r.dynamicFields[0]).toMatchObject({
      operator: 'NOT IN',
      values: ['1', '2', '3'],
      field: { type: 'number' },
    });
  });

  test('recovers ~ contains operator on a custom field', () => {
    const r = QLBuilder.tryParseToBuilder('`cf_notes` ~ "urgent"');
    expect(r.dynamicFields[0]).toMatchObject({
      operator: '~',
      value: 'urgent',
      field: { type: 'text' },
    });
  });

  test('recovers boolean inferred type from bare true/false', () => {
    const r = QLBuilder.tryParseToBuilder('`cf_done` = true');
    expect(r.dynamicFields[0]).toMatchObject({
      operator: '=',
      value: 'true',
      field: { type: 'boolean' },
    });
  });

  test('combines a built-in clause with a custom-field clause', () => {
    const r = QLBuilder.tryParseToBuilder('`cf_stage` = "draft" AND status_id = 5');
    expect(r.statuses).toEqual([5]);
    expect(r.dynamicFields[0]).toMatchObject({
      operator: '=',
      value: 'draft',
    });
    expect(r.dropped).toBe(false);
  });

  test('flags dropped=true when NOT or OR clauses remain unparsed', () => {
    const r = QLBuilder.tryParseToBuilder('NOT (`cf_stage` = "draft") OR `cf_other` = "x"');
    expect(r.dropped).toBe(true);
  });

  test('round-trips a builder query back through buildQuery', () => {
    const original = QLBuilder.buildQuery({
      statuses: [3],
      priorities: [],
      workspaces: [],
      search: '',
      dynamicFields: [
        {
          field: { id: 'cf_stage', type: 'text', name: 'Stage' },
          operator: '=',
          value: 'draft',
          values: [],
        },
      ],
    });
    const parsed = QLBuilder.tryParseToBuilder(original, {
      customFields: [{ id: 'cf_stage', type: 'text', name: 'Stage' }],
    });
    expect(parsed.statuses).toEqual([3]);
    expect(parsed.dynamicFields).toEqual([
      {
        field: { id: 'cf_stage', type: 'text', name: 'Stage' },
        operator: '=',
        value: 'draft',
        values: [],
      },
    ]);
    expect(parsed.dropped).toBe(false);
  });
});
