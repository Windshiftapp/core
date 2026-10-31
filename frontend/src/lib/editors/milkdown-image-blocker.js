// ProseMirror plugin that keeps images out of content with no attachment
// support (e.g. milestone descriptions). Upload paths are gated separately in
// MilkdownEditor; this catches everything else that can create an image node:
// typed `![](url)` input rules, pasted Markdown/HTML, and the image-block
// component's own upload/link placeholder.

import { Plugin, PluginKey } from '@milkdown/kit/prose/state';
import { $prose } from '@milkdown/kit/utils';

const IMAGE_NODE_TYPES = new Set(['image', 'image-block']);

function countImages(doc) {
  let count = 0;
  doc.descendants((node) => {
    if (IMAGE_NODE_TYPES.has(node.type.name)) count++;
  });
  return count;
}

const imageBlockerPluginKey = new PluginKey('image-blocker');

/**
 * Rejects any transaction that adds an image. Images already present in the
 * stored content are left alone so they can still be edited around or removed.
 */
export const imageBlockerPlugin = $prose(
  () =>
    new Plugin({
      key: imageBlockerPluginKey,
      filterTransaction(tr, state) {
        if (!tr.docChanged) return true;
        return countImages(tr.doc) <= countImages(state.doc);
      },
    })
);
