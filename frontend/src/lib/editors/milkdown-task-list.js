// GFM task-list rendering and interaction for the Milkdown editor.
//
// Milkdown's GFM preset serializes a task item as
// `<li data-item-type="task" data-checked="true|false">` and never emits an
// `<input>`. The editor styles draw the checkbox from `data-checked`; this
// plugin adds the missing behavior:
//   - toggles `checked` when the marker gutter is clicked,
//   - exposes task items as checkboxes to assistive technology,
//   - provides a toolbar command to turn a list item into a task item.
//
// The stored Markdown stays standard GFM (`- [ ]` / `- [x]`), so copy,
// duplicate and template flows keep working without a storage change.

import { bulletListSchema } from '@milkdown/kit/preset/commonmark';
import { wrapInList } from '@milkdown/kit/prose/schema-list';
import { Plugin, PluginKey } from '@milkdown/kit/prose/state';
import { Decoration, DecorationSet } from '@milkdown/kit/prose/view';
import { $command, $prose } from '@milkdown/kit/utils';

export const taskListPluginKey = new PluginKey('task-list');

/**
 * Find the nearest list item at or above `pos`.
 * @returns {{ node: import('prosemirror-model').Node, pos: number } | null}
 */
export function findListItem(doc, pos) {
  if (typeof pos !== 'number' || pos < 0 || pos > doc.content.size) return null;
  const $pos = doc.resolve(pos);
  for (let depth = $pos.depth; depth > 0; depth -= 1) {
    const node = $pos.node(depth);
    if (node.type.name === 'list_item') {
      return { node, pos: $pos.before(depth) };
    }
  }
  return null;
}

/** A list item is a task item when its `checked` attribute is set. */
export function isTaskListItem(node) {
  return node?.type?.name === 'list_item' && node.attrs?.checked != null;
}

/** Find the nearest *task* list item at or above `pos`. */
export function findTaskListItem(doc, pos) {
  const found = findListItem(doc, pos);
  return found && isTaskListItem(found.node) ? found : null;
}

/** Toolbar: turn a plain list item into a task item, or a task back to plain. */
export function toggleTaskState(state, found) {
  return state.tr.setNodeMarkup(found.pos, undefined, {
    ...found.node.attrs,
    checked: found.node.attrs.checked == null ? false : null,
  });
}

/** Click/keyboard: flip the checkbox of an existing task item. */
export function flipTaskChecked(state, found) {
  return state.tr.setNodeMarkup(found.pos, undefined, {
    ...found.node.attrs,
    checked: !found.node.attrs.checked,
  });
}

function buildDecorations(doc) {
  const decorations = [];
  doc.descendants((node, pos) => {
    if (!isTaskListItem(node)) return;
    decorations.push(
      Decoration.node(pos, pos + node.nodeSize, {
        class: 'task-list-item',
        role: 'checkbox',
        'aria-checked': node.attrs.checked ? 'true' : 'false',
        tabindex: '0',
      })
    );
  });
  return DecorationSet.create(doc, decorations);
}

/** Mirror the editor's editability onto task markers for assistive tech. */
function syncAriaDisabled(view) {
  for (const li of view.dom.querySelectorAll('li[data-item-type="task"]')) {
    if (view.editable) li.removeAttribute('aria-disabled');
    else li.setAttribute('aria-disabled', 'true');
  }
}

/**
 * Resolve the task list item for a DOM event inside the editor, or null when
 * the event did not land on a task item.
 */
function taskListItemFromEvent(view, target) {
  if (!(target instanceof Element)) return null;
  const li = target.closest('li[data-item-type="task"]');
  if (!li) return null;
  return findTaskListItem(view.state.doc, view.posAtDOM(li, 0));
}

/**
 * True when the pointer sits in the marker gutter rather than on the item
 * text. The gutter is the space before the first content block; comparing to
 * that block's left edge avoids hard-coding the CSS gutter width.
 */
function isMarkerClick(li, event) {
  const content = li.firstElementChild;
  const boundary = content ? content.getBoundingClientRect().left : li.getBoundingClientRect().left;
  return event.clientX < boundary;
}

/**
 * Apply a checkbox toggle and keep keyboard focus on the marker. Changing the
 * list item's markup makes ProseMirror replace the focused <li>, so focus is
 * restored to the replacement to preserve the Tab sequence.
 */
function applyToggle(view, found, marker) {
  const hadFocus = marker === document.activeElement;
  view.dispatch(flipTaskChecked(view.state, found));
  if (!hadFocus) return;
  const replacement = view.nodeDOM(found.pos);
  if (replacement instanceof HTMLElement) replacement.focus();
}

export const taskListPlugin = $prose(() => {
  return new Plugin({
    key: taskListPluginKey,
    state: {
      init(_, { doc }) {
        return buildDecorations(doc);
      },
      apply(tr, value) {
        return tr.docChanged ? buildDecorations(tr.doc) : value;
      },
    },
    props: {
      decorations(state) {
        return this.getState(state);
      },
      handleDOMEvents: {
        mousedown(view, event) {
          if (!view.editable || event.button !== 0) return false;
          const target = event.target;
          if (!(target instanceof Element)) return false;
          const li = target.closest('li[data-item-type="task"]');
          if (!li || !isMarkerClick(li, event)) return false;
          const found = taskListItemFromEvent(view, target);
          if (!found) return false;
          // Keep the caret where it was instead of selecting on click.
          event.preventDefault();
          applyToggle(view, found, li);
          return true;
        },
      },
      handleKeyDown(view, event) {
        if (!view.editable) return false;
        if (event.key !== ' ' && event.key !== 'Enter') return false;
        const target = event.target;
        // Only react when the marker itself is focused, never while typing.
        if (!(target instanceof Element) || !target.matches('li[data-item-type="task"]')) {
          return false;
        }
        const found = taskListItemFromEvent(view, target);
        if (!found) return false;
        event.preventDefault();
        applyToggle(view, found, target);
        return true;
      },
    },
    view(view) {
      const sync = () => syncAriaDisabled(view);
      sync();
      // Catch the first paint if plugin views are created before the doc DOM.
      const raf = requestAnimationFrame(sync);
      return {
        update: sync,
        destroy: () => cancelAnimationFrame(raf),
      };
    },
  });
});

/**
 * Toolbar command: turn the current list item into a task item (or back).
 * Outside a list, wrap the selection in a bullet list first.
 */
export const toggleTaskListCommand = $command('ToggleTaskList', (ctx) => () => {
  return (state, dispatch) => {
    const item = findListItem(state.doc, state.selection.from);
    if (item) {
      if (dispatch) dispatch(toggleTaskState(state, item));
      return true;
    }

    const listType = bulletListSchema.type(ctx);
    return wrapInList(listType)(state, (tr) => {
      if (!dispatch) return;
      const created = findListItem(tr.doc, tr.selection.from);
      if (created) {
        tr.setNodeMarkup(created.pos, undefined, {
          ...created.node.attrs,
          checked: false,
        });
      }
      dispatch(tr);
    });
  };
});
