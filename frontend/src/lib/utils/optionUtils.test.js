import { describe, expect, test } from 'vitest';
import {
  parseFieldOptions,
  resolveOptionLabel,
  resolveOptionLabels,
  serializeOptions,
} from './optionUtils.js';

// The custom-field option format on the wire is:
//   { "next_id": 5, "items": [{ "id": 1, "label": "Critical" }, ...] }
// The id is the stable identifier stored on items; the label is for display.
// When an option is deleted server-side (handled by cleanupRemovedOptions in
// custom_fields.go), the item's stored id is removed from its cfv. The
// frontend's resolveOptionLabel is the safety net for the brief window
// before that cleanup completes (or for stale UI state): an unknown id
// falls back to String(value) so the user sees the raw id rather than
// nothing.

describe('parseFieldOptions', () => {
  test('returns empty shape for null / empty string', () => {
    expect(parseFieldOptions(null)).toEqual({ nextId: 1, items: [] });
    expect(parseFieldOptions('')).toEqual({ nextId: 1, items: [] });
    expect(parseFieldOptions(undefined)).toEqual({ nextId: 1, items: [] });
  });

  test('parses the canonical format', () => {
    const json = JSON.stringify({
      next_id: 4,
      items: [
        { id: 1, label: 'Low' },
        { id: 2, label: 'Medium' },
        { id: 3, label: 'High' },
      ],
    });
    expect(parseFieldOptions(json)).toEqual({
      nextId: 4,
      items: [
        { id: 1, label: 'Low' },
        { id: 2, label: 'Medium' },
        { id: 3, label: 'High' },
      ],
    });
  });

  test('drops unknown fields from each item (only id+label are kept)', () => {
    const json = JSON.stringify({
      next_id: 2,
      items: [{ id: 1, label: 'X', color: '#ff0', deprecated: true }],
    });
    expect(parseFieldOptions(json).items).toEqual([{ id: 1, label: 'X' }]);
  });

  test('returns empty shape on malformed JSON', () => {
    expect(parseFieldOptions('{not json')).toEqual({ nextId: 1, items: [] });
  });

  test('returns empty shape on legacy/foreign shape (no next_id)', () => {
    // Older format without next_id is treated as "no options" — the upgrade
    // path runs server-side; the frontend never produces this shape.
    expect(parseFieldOptions(JSON.stringify(['Low', 'High']))).toEqual({
      nextId: 1,
      items: [],
    });
    expect(parseFieldOptions(JSON.stringify({ items: [{ id: 1, label: 'X' }] }))).toEqual({
      nextId: 1,
      items: [],
    });
  });
});

describe('resolveOptionLabel', () => {
  const OPTIONS = JSON.stringify({
    next_id: 4,
    items: [
      { id: 1, label: 'Low' },
      { id: 2, label: 'Medium' },
      { id: 3, label: 'High' },
    ],
  });

  test('empty input → empty string', () => {
    expect(resolveOptionLabel(OPTIONS, null)).toBe('');
    expect(resolveOptionLabel(OPTIONS, '')).toBe('');
    expect(resolveOptionLabel(OPTIONS, undefined)).toBe('');
  });

  test('numeric id resolves to label', () => {
    expect(resolveOptionLabel(OPTIONS, 1)).toBe('Low');
    expect(resolveOptionLabel(OPTIONS, 3)).toBe('High');
  });

  test('numeric string id resolves to label', () => {
    expect(resolveOptionLabel(OPTIONS, '2')).toBe('Medium');
  });

  test('orphaned id (option was deleted) falls back to raw id as string', () => {
    // This is the safety-net behavior: if the backend's cleanupRemovedOptions
    // hasn't yet scrubbed an item's stored id — or if the UI is showing
    // stale data — the renderer shows the raw id rather than crashing.
    // The user sees "99" instead of "Critical", which is visually obvious
    // as a data issue.
    expect(resolveOptionLabel(OPTIONS, 99)).toBe('99');
    expect(resolveOptionLabel(OPTIONS, '99')).toBe('99');
  });

  test('non-numeric value falls back to raw string', () => {
    expect(resolveOptionLabel(OPTIONS, 'unexpected')).toBe('unexpected');
  });

  test('null options + a value returns the value as string (full degradation)', () => {
    expect(resolveOptionLabel(null, 5)).toBe('5');
    expect(resolveOptionLabel('', 5)).toBe('5');
  });

  test('null options + null value returns empty string', () => {
    expect(resolveOptionLabel(null, null)).toBe('');
  });
});

describe('resolveOptionLabels', () => {
  const OPTIONS = JSON.stringify({
    next_id: 4,
    items: [
      { id: 1, label: 'Low' },
      { id: 2, label: 'Medium' },
      { id: 3, label: 'High' },
    ],
  });

  test('empty / non-array input → empty array', () => {
    expect(resolveOptionLabels(OPTIONS, [])).toEqual([]);
    expect(resolveOptionLabels(OPTIONS, null)).toEqual([]);
    expect(resolveOptionLabels(OPTIONS, undefined)).toEqual([]);
    // The function only accepts arrays — passing a single value yields [].
    expect(resolveOptionLabels(OPTIONS, 2)).toEqual([]);
  });

  test('resolves a list of ids preserving order', () => {
    expect(resolveOptionLabels(OPTIONS, [3, 1])).toEqual(['High', 'Low']);
  });

  test('mixed valid + orphaned ids preserves the orphans as raw strings', () => {
    // Multiselect with one option deleted: the deleted slot still shows
    // its raw id until backend cleanup or a refresh removes it.
    expect(resolveOptionLabels(OPTIONS, [1, 99, 2])).toEqual(['Low', '99', 'Medium']);
  });
});

describe('serializeOptions', () => {
  test('produces the canonical {next_id, items} shape', () => {
    const out = serializeOptions(5, [
      { id: 1, label: 'A' },
      { id: 4, label: 'D' },
    ]);
    expect(JSON.parse(out)).toEqual({
      next_id: 5,
      items: [
        { id: 1, label: 'A' },
        { id: 4, label: 'D' },
      ],
    });
  });

  test('round-trips through parseFieldOptions', () => {
    const original = {
      nextId: 7,
      items: [
        { id: 1, label: 'Open' },
        { id: 6, label: 'Closed' },
      ],
    };
    const serialized = serializeOptions(original.nextId, original.items);
    expect(parseFieldOptions(serialized)).toEqual(original);
  });
});
