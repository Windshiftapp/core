// Custom Milkdown plugin for rendering @mentions as styled chips

import { Plugin, PluginKey } from '@milkdown/kit/prose/state';
import { Decoration, DecorationSet } from '@milkdown/kit/prose/view';
import { $prose } from '@milkdown/kit/utils';

// Regex to find mentions in text: @username or @"Display Name"
const MENTION_REGEX = /(?<![a-zA-Z0-9.])@([a-zA-Z0-9_.-]+)|(?<![a-zA-Z0-9.])@"([^"]+)"/g;

/**
 * Creates decorations for all mentions found in the document
 * @param {Node} doc - ProseMirror document node
 * @returns {DecorationSet} - Set of decorations to apply
 */
function createMentionDecorations(doc) {
  const decorations = [];

  doc.descendants((node, pos) => {
    if (node.isText && node.text) {
      const text = node.text;
      let match;
      // Reset regex lastIndex for each text node
      MENTION_REGEX.lastIndex = 0;

      while ((match = MENTION_REGEX.exec(text)) !== null) {
        const fullMatch = match[0];
        const from = pos + match.index;
        const to = from + fullMatch.length;
        // Extract the identifier (username or display name without quotes)
        const identifier = match[1] || match[2];
        const isQuoted = match[2] !== undefined;

        if (isQuoted) {
          // For quoted mentions like @"Display Name", create three decorations:
          // 1. Hide the @" prefix
          // 2. Show the name with chip styling
          // 3. Hide the closing "

          // @" prefix (hide it)
          decorations.push(
            Decoration.inline(from, from + 2, {
              class: 'mention-chip-hidden',
            })
          );

          // The display name part (show as chip)
          decorations.push(
            Decoration.inline(from + 2, to - 1, {
              class: 'mention-chip mention-chip-name',
              'data-mention': identifier,
            })
          );

          // Closing " (hide it)
          decorations.push(
            Decoration.inline(to - 1, to, {
              class: 'mention-chip-hidden',
            })
          );
        } else {
          // Simple @username mention - just style the whole thing
          decorations.push(
            Decoration.inline(from, to, {
              class: 'mention-chip',
              'data-mention': identifier,
            })
          );
        }
      }
    }
  });

  return DecorationSet.create(doc, decorations);
}

// Plugin key for the mention decoration plugin
export const mentionDecorationPluginKey = new PluginKey('mention-decoration');

/**
 * Milkdown plugin that applies mention decorations to the document.
 * This plugin scans the document for @mention patterns and wraps them
 * with a span that has the 'mention-chip' class for styling.
 */
export const mentionDecorationPlugin = $prose(() => {
  return new Plugin({
    key: mentionDecorationPluginKey,
    state: {
      init(_, { doc }) {
        return createMentionDecorations(doc);
      },
      apply(tr, old) {
        // Only recalculate if document changed
        return tr.docChanged ? createMentionDecorations(tr.doc) : old;
      },
    },
    props: {
      decorations(state) {
        return this.getState(state);
      },
    },
  });
});
