// ProseMirror plugin that sanitizes link hrefs and image srcs to prevent XSS
// from javascript:, vbscript:, and data: URL schemes rendered via Markdown.

import { Plugin, PluginKey } from '@milkdown/kit/prose/state';
import { $prose } from '@milkdown/kit/utils';

const SAFE_URL_SCHEMES = /^(https?:|mailto:|tel:|#|\/)/i;

/**
 * Check whether a URL is safe to navigate to.
 * Allows http(s), mailto, tel, fragment (#), and relative URLs.
 * Blocks javascript:, vbscript:, data:, and any other dangerous scheme.
 * @param {string} url
 * @returns {boolean}
 */
export function isSafeUrl(url) {
  if (!url) return true;
  const trimmed = url.trim();
  if (trimmed === '') return true;
  // Relative URLs (no scheme) are safe
  if (!trimmed.includes(':')) return true;
  return SAFE_URL_SCHEMES.test(trimmed);
}

export const linkSanitizerPluginKey = new PluginKey('link-sanitizer');

/**
 * Milkdown plugin that sanitizes all link hrefs and image srcs on every
 * document change (including initial load). This is a belt-and-suspenders
 * defense against malicious Markdown stored in the database.
 */
export const linkSanitizerPlugin = $prose(() => {
  /**
   * Walk the document and build a transaction that replaces any unsafe
   * link hrefs or image srcs with '#unsafe-link-removed'.
   */
  function sanitizeDoc(tr) {
    let changed = false;
    tr.doc.descendants((node, pos) => {
      // Sanitize link marks on text nodes
      if (node.marks) {
        node.marks.forEach((mark) => {
          if (mark.type.name === 'link' && mark.attrs.href && !isSafeUrl(mark.attrs.href)) {
            tr.removeMark(pos, pos + node.nodeSize, mark.type);
            tr.addMark(
              pos,
              pos + node.nodeSize,
              mark.type.create({ ...mark.attrs, href: '#unsafe-link-removed' })
            );
            changed = true;
          }
        });
      }
      // Sanitize image nodes
      if (node.type.name === 'image' && node.attrs.src && !isSafeUrl(node.attrs.src)) {
        tr.setNodeMarkup(pos, undefined, { ...node.attrs, src: '#unsafe-link-removed' });
        changed = true;
      }
    });
    return changed;
  }

  return new Plugin({
    key: linkSanitizerPluginKey,

    // Intercept every transaction to sanitize any new or changed links
    appendTransaction(_transactions, _oldState, newState) {
      const tr = newState.tr;
      const changed = sanitizeDoc(tr);
      return changed ? tr : null;
    },

    props: {
      // Belt-and-suspenders: block click navigation to unsafe URLs
      handleClick(_view, _pos, event) {
        const link = event.target.closest?.('a[href]');
        if (link && !isSafeUrl(link.getAttribute('href'))) {
          event.preventDefault();
          event.stopPropagation();
          return true;
        }
        return false;
      },
    },
  });
});
