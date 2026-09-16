import { describe, expect, it } from 'vitest';
import { linkDirectionLabel } from './requirementLinkLabels.js';

const link = {
  link_type_id: 12,
  link_type_forward_label: 'specifies',
  link_type_reverse_label: 'specified by',
  source_type: 'page', source_id: 41,
  target_type: 'page', target_id: 42,
};
const linkTypes = [{
  id: 12,
  forward_label: 'specifies',
  reverse_label: 'specified by',
  display_forward_label: 'детализирует',
  display_reverse_label: 'детализируется в',
}];

describe('requirement link direction labels', () => {
  it.each([
    [41, 'детализирует'],
    [42, 'детализируется в'],
  ])('uses the translated catalog label for page %s', (pageId, expected) => {
    expect(linkDirectionLabel(link, pageId, linkTypes)).toBe(expected);
  });

  it.each([
    [41, 'specifies'],
    [42, 'specified by'],
  ])('retains the joined label without a catalog for page %s', (pageId, expected) => {
    expect(linkDirectionLabel(link, pageId)).toBe(expected);
  });
});
