/**
 * @param {Record<string, unknown>} link
 * @param {number} pageId
 */
export function linkDirectionLabel(link, pageId) {
  const pageIsSource = link.source_type === 'page' && link.source_id === pageId;
  return pageIsSource ? link.link_type_forward_label : link.link_type_reverse_label;
}
