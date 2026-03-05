const DEFAULT_DECAY_DAYS = 180;

/**
 * Compute document health score and color.
 * Returns { score, color } or null if health doesn't apply.
 */
export function computeDocumentHealth(doc) {
  if (!doc) return null;

  // Health only applies to knowledge and correspondence
  if (
    !doc.content_type ||
    (doc.content_type !== 'knowledge' && doc.content_type !== 'correspondence')
  ) {
    return null;
  }

  // Not applicable for non-ready documents
  if (doc.status !== 'ready') {
    return null;
  }

  // Reference date: reviewed_at > updated_at > created_at
  const refDate = doc.reviewed_at || doc.updated_at || doc.created_at;
  if (!refDate) return null;

  const now = Date.now();
  const refMs = new Date(refDate).getTime();
  const daysSince = (now - refMs) / (1000 * 60 * 60 * 24);

  // Decay period from bucket's max_age_days, or default
  const decayPeriod = doc.max_age_days || DEFAULT_DECAY_DAYS;

  // Base score: reviewed → 1.0, human note → 1.0, has LLM article → 0.7, else → 0.5
  let baseScore;
  if (doc.reviewed_at) {
    baseScore = 1.0;
  } else if (doc.source_type === 'note') {
    baseScore = 1.0;
  } else if (doc.has_article || doc.article) {
    baseScore = 0.7;
  } else {
    baseScore = 0.5;
  }

  const decay = Math.max(0, 1 - daysSince / decayPeriod);
  const score = Math.round(baseScore * decay * 100);

  let color;
  if (score >= 60) color = 'success';
  else if (score >= 30) color = 'warning';
  else color = 'danger';

  return { score, color };
}
