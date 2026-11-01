import { navigate } from '../router.js';
import { itemModalStore } from '../stores/itemModalStore.svelte.js';
import { addToast, removeToast } from '../stores/toasts.svelte.js';
import { isTypingInField } from './keyboardShortcuts.js';

const OPEN_SHORTCUT = 'o';
const DEFAULT_DURATION = 8000;

function getItemKey(item) {
  if (!item) return 'Item';
  const key = item.workspace_key || item.workspaceKey || 'WORK';
  const number = item.workspace_item_number || item.workspaceItemNumber || item.id;
  return `${key}-${number}`;
}

function getItemUrl(item) {
  return `/workspaces/${item.workspace_id}/items/${item.id}`;
}

/**
 * Render a clickable toast for a work item and register a short-lived "O"
 * shortcut that runs `openItem`. Clicking the toast or pressing the shortcut
 * dismisses the toast and opens the item.
 */
function showItemActionToast({ title, message, variant, duration, openItem }) {
  let toastId = null;
  let cleanupTimer = null;

  const cleanup = () => {
    document.removeEventListener('keydown', handleKeydown);
    if (cleanupTimer) {
      clearTimeout(cleanupTimer);
      cleanupTimer = null;
    }
  };

  const closeAndOpen = () => {
    // removeToast fires onDismiss which runs cleanup — no need to call it here.
    if (toastId !== null) removeToast(toastId);
    openItem();
  };

  function handleKeydown(event) {
    if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey || event.shiftKey)
      return;
    if (isTypingInField(event)) return;
    if (event.key?.toLowerCase() !== OPEN_SHORTCUT) return;

    event.preventDefault();
    closeAndOpen();
  }

  document.addEventListener('keydown', handleKeydown);
  cleanupTimer = setTimeout(cleanup, duration + 250);

  toastId = addToast({
    title,
    message,
    variant,
    duration,
    clickable: true,
    actionLabel: 'Open item',
    keyboardHint: 'O',
    onClick: closeAndOpen,
    onDismiss: cleanup,
  });

  return toastId;
}

/**
 * Show a success toast for a newly-created work item. The toast is clickable
 * and registers a short-lived "O" shortcut to open the item detail page.
 *
 * @param {object} item
 * @param {{ messagePrefix?: string, duration?: number }} [options]
 */
export function showCreatedItemToast(item, options = {}) {
  if (!item?.id) return null;

  const itemKey = getItemKey(item);
  const duration = options.duration ?? DEFAULT_DURATION;

  return showItemActionToast({
    title: `${itemKey} created`,
    message: `${options.messagePrefix ? `${options.messagePrefix} ` : ''}${item.title || 'Untitled'}`,
    variant: 'success',
    duration,
    openItem: () => navigate(getItemUrl(item)),
  });
}

/**
 * Show an info toast for a work item that was created but is hidden by the
 * current collection filters. The title carries the item key and the toast
 * opens the item modal so the user can adjust it (for example by adding a
 * label) without leaving the collection.
 *
 * @param {object} item
 * @param {{ message?: string, onOpen?: () => void, duration?: number }} [options]
 */
export function showCreatedItemOutsideViewToast(item, options = {}) {
  if (!item?.id) return null;

  const itemKey = getItemKey(item);
  const duration = options.duration ?? DEFAULT_DURATION;
  const onOpen = options.onOpen ?? (() => itemModalStore.open(item.id, item.workspace_id ?? null));

  return showItemActionToast({
    title: `${itemKey} created`,
    message: options.message ?? (item.title || 'Untitled'),
    variant: 'info',
    duration,
    openItem: onOpen,
  });
}
