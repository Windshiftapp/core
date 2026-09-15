/**
 * @param {Record<string, unknown>} link
 * @param {number} pageId
 * @returns {'outgoing' | 'incoming' | null}
 */
export function pageToPagePeerDirection(link, pageId) {
  const pageIsSource = link.source_type === 'page' && link.source_id === pageId;
  const pageIsTarget = link.target_type === 'page' && link.target_id === pageId;
  if (pageIsSource && link.target_type === 'page' && link.target_id !== pageId) {
    return 'outgoing';
  }
  if (pageIsTarget && link.source_type === 'page' && link.source_id !== pageId) {
    return 'incoming';
  }
  return null;
}

/**
 * @param {Record<string, unknown>} link
 * @param {number} pageId
 */
export function linkDirectionLabel(link, pageId) {
  const pageIsSource = link.source_type === 'page' && link.source_id === pageId;
  const label = pageIsSource ? link.link_type_forward_label : link.link_type_reverse_label;
  return typeof label === 'string' ? label : '';
}
