import { PER_BUCKET_CAP, TOTAL_CAP } from './buckets.js';
import { compareCommands, scoreCommand } from './score.js';
import { deriveLegacyBucket } from './types.js';

/**
 * Score commands against the query, sort by (bucket, score, insertion), and
 * cap per-bucket and overall. Providers set `bucket` explicitly;
 * deriveLegacyBucket is the safety net for commands flowing in through
 * makeExternalProvider that haven't been updated yet. Shared by the desktop
 * palette and the mobile sheet so both surfaces rank identically.
 *
 * @param {string} query
 * @param {any[]} commandsList
 * @returns {any[]}
 */
export function rankCommands(query, commandsList) {
  const annotated = commandsList.map((cmd, i) => {
    const label = cmd.label ?? '';
    const description = cmd.description ?? '';
    const keywords = cmd.keywords ?? [];
    const score = query.trim() ? scoreCommand(query, { label, description, keywords }) : 1;
    return {
      ...cmd,
      bucket: cmd.bucket || deriveLegacyBucket(cmd),
      _score: score,
      _seq: cmd._seq ?? i,
    };
  });

  const filtered = query.trim() ? annotated.filter((c) => c._score > 0) : annotated;
  filtered.sort(compareCommands(query));

  const counts = new Map();
  const out = [];
  for (const c of filtered) {
    if (out.length >= TOTAL_CAP) break;
    const n = counts.get(c.bucket) || 0;
    if (n >= PER_BUCKET_CAP) continue;
    counts.set(c.bucket, n + 1);
    out.push(c);
  }
  return out;
}
