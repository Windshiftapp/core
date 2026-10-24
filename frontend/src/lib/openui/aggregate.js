// Pure aggregation helpers for widget components.
//
// Everything here works on the normalized row shapes produced by the data
// endpoint, so it is entity-agnostic: the same functions group items and
// assets. No Svelte, no fetching, so it is directly unit-testable.

import { fieldValue } from './querySpec.js';

/** Resolve a group label, falling back to a stable placeholder. */
function groupLabel(row, groupBy, unknownLabel) {
  const value = fieldValue(row, groupBy);
  if (value === undefined || value === null || value === '') return unknownLabel;
  return String(value);
}

function numericValue(row, field) {
  if (!field) return 1;
  const value = fieldValue(row, field);
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

/**
 * Count rows per distinct value of `groupBy`.
 * @returns {Array<{label: string, value: number}>} sorted by value desc
 */
export function groupCount(rows, groupBy, { unknownLabel = 'Unknown', limit = 0 } = {}) {
  const buckets = new Map();
  for (const row of rows ?? []) {
    const label = groupLabel(row, groupBy, unknownLabel);
    buckets.set(label, (buckets.get(label) ?? 0) + 1);
  }
  return finalize(buckets, limit);
}

/**
 * Sum `field` per distinct value of `groupBy`.
 * @returns {Array<{label: string, value: number}>} sorted by value desc
 */
export function groupSum(rows, groupBy, field, { unknownLabel = 'Unknown', limit = 0 } = {}) {
  const buckets = new Map();
  for (const row of rows ?? []) {
    const label = groupLabel(row, groupBy, unknownLabel);
    buckets.set(label, (buckets.get(label) ?? 0) + numericValue(row, field));
  }
  return finalize(buckets, limit);
}

/**
 * Aggregate rows into time buckets keyed by a date field.
 * @param {'day'|'week'|'month'} unit
 * @returns {Array<{label: string, value: number}>} ascending by bucket
 */
export function bucketByDate(rows, field, unit = 'week', { metric = 'count', sumField = '' } = {}) {
  const buckets = new Map();
  for (const row of rows ?? []) {
    const raw = fieldValue(row, field);
    if (!raw) continue;
    const date = raw instanceof Date ? raw : new Date(raw);
    if (Number.isNaN(date.getTime())) continue;
    const key = bucketStart(date, unit).toISOString();
    const increment = metric === 'sum' ? numericValue(row, sumField) : 1;
    buckets.set(key, (buckets.get(key) ?? 0) + increment);
  }
  return [...buckets.entries()]
    .sort(([a], [b]) => (a < b ? -1 : 1))
    .map(([key, value]) => ({ label: key, value }));
}

/** Snap a date to the start of its bucket, in local time. */
export function bucketStart(date, unit) {
  const start = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  if (unit === 'month') {
    return new Date(date.getFullYear(), date.getMonth(), 1);
  }
  if (unit === 'week') {
    // ISO weeks start on Monday.
    const weekday = (start.getDay() + 6) % 7;
    start.setDate(start.getDate() - weekday);
    return start;
  }
  return start;
}

/**
 * Expand rows into one entry per distinct value of a multi-valued field, so a
 * row linked to three values contributes to three buckets.
 */
export function expandMultiValue(rows, field) {
  const out = [];
  for (const row of rows ?? []) {
    const raw = fieldValue(row, field);
    const values = Array.isArray(raw) ? raw : [raw];
    for (const value of values) {
      out.push({ ...row, __group: value });
    }
  }
  return out;
}

function finalize(buckets, limit) {
  const entries = [...buckets.entries()].map(([label, value]) => ({ label, value }));
  entries.sort((a, b) => b.value - a.value || a.label.localeCompare(b.label));
  return limit > 0 ? entries.slice(0, limit) : entries;
}

/**
 * Group rows by a plain field or by a field reached through the linked-item
 * projection.
 *
 * `groupBy` is either a field path ("status") or a linked path
 * ("linked_items.creator"). Linked grouping is multi-valued, so `mode` decides
 * whether a row counts once per link ("fanout") or once in total ("primary").
 */
export function groupRows(rows, options = {}) {
  const {
    groupBy,
    metric = 'count',
    field = '',
    mode = 'fanout',
    limit = 0,
    unknownLabel = 'Unknown',
  } = options;
  const linkedPrefix = 'linked_items.';
  if (typeof groupBy === 'string' && groupBy.startsWith(linkedPrefix)) {
    return groupLinkedRows(rows, groupBy.slice(linkedPrefix.length), {
      metric,
      field,
      mode,
      limit,
      unknownLabel,
    });
  }
  return metric === 'sum'
    ? groupSum(rows, groupBy, field, { limit, unknownLabel })
    : groupCount(rows, groupBy, { limit, unknownLabel });
}

function groupLinkedRows(rows, valueField, { metric, field, mode, limit, unknownLabel }) {
  const buckets = new Map();
  const add = (label, row) => {
    const increment = metric === 'sum' ? numericValue(row, field) : 1;
    buckets.set(label, (buckets.get(label) ?? 0) + increment);
  };

  for (const row of rows ?? []) {
    const links = Array.isArray(row?.linked_items) ? row.linked_items : [];
    if (links.length === 0) {
      add(unknownLabel, row);
      continue;
    }
    if (mode === 'primary') {
      add(groupLabel(links[0], valueField, unknownLabel), row);
      continue;
    }
    for (const link of links) {
      add(groupLabel(link, valueField, unknownLabel), row);
    }
  }
  return finalize(buckets, limit);
}
