import type { RenderToken, RowPosition, VirtualRow } from './types';

export const VIRTUAL_ROW_THRESHOLD = 100;
export const ROW_OVERSCAN = 8;
export const DRAG_OVERSCAN = 16;

export function indexRows(rows: readonly VirtualRow[]) {
  const index = new Map<string, number>();
  rows.forEach((row, position) => {
    if (!row.key || index.has(row.key))
      throw new Error(`Duplicate or empty virtual row key: ${row.key}`);
    if (!Number.isFinite(row.estimatedSize) || row.estimatedSize <= 0) {
      throw new Error(`Invalid virtual row size: ${row.key}`);
    }
    index.set(row.key, position);
  });
  return index;
}

export function renderTokens(rows: readonly RowPosition[], totalSize: number): RenderToken[] {
  const tokens: RenderToken[] = [];
  let end = 0;
  for (const row of rows) {
    if (row.start > end) tokens.push({ kind: 'gap', key: `gap:${row.key}`, size: row.start - end });
    tokens.push({ kind: 'row', key: `row:${row.key}`, row });
    end = row.end;
  }
  if (totalSize > end) tokens.push({ kind: 'gap', key: 'gap:end', size: totalSize - end });
  return tokens;
}

export function sparseIndexes(
  near: readonly number[],
  pinned: Iterable<string>,
  indexes: Map<string, number>
) {
  const result = new Set(near);
  for (const key of pinned) {
    const index = indexes.get(key);
    if (index !== undefined) result.add(index);
  }
  return [...result].sort((a, b) => a - b);
}
