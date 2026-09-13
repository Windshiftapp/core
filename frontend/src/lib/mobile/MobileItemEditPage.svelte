<script>
  import { api } from '../api.js';
  import { currentRoute, navigate } from '../router.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { formatItemKey } from '../utils/itemKey.js';
  import MobileEditorPage from './MobileEditorPage.svelte';
  import MobileConfirmSheet from './MobileConfirmSheet.svelte';
  import { Loader } from '@lucide/svelte';

  /**
   * Full-page title/description editor for the phone surface
   * (/m/items/:id/edit) — the mobile answer to "the detail page used to be
   * read-only". Description is Markdown, matching the desktop editor's
   * storage format; a rich-text surface can replace the textarea later.
   */

  const itemId = $derived(Number($currentRoute.params.id));

  let item = $state(null);
  let loading = $state(true);
  let loadErrored = $state(false);
  let title = $state('');
  let description = $state('');
  let saving = $state(false);
  let error = $state('');

  // Unsaved-input guard: leaving with changes asks for confirmation.
  let confirmDiscardOpen = $state(false);
  const isDirty = $derived(
    !!item && (title !== (item.title ?? '') || description !== (item.description ?? ''))
  );

  const pageTitle = $derived(formatItemKey(item) || 'Edit item');
  const canSave = $derived(title.trim() !== '' && !saving && isDirty);

  $effect(() => {
    const id = itemId;
    if (!id) return;
    let cancelled = false;
    loading = true;
    loadErrored = false;
    api.items
      .get(id)
      .then((fresh) => {
        if (cancelled) return;
        item = fresh;
        title = fresh.title ?? '';
        description = fresh.description ?? '';
      })
      .catch((err) => {
        if (cancelled) return;
        console.error('Failed to load item:', err);
        loadErrored = true;
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function save() {
    if (!canSave) return;
    saving = true;
    error = '';
    try {
      await api.items.update(itemId, {
        title: title.trim(),
        description: description.trim(),
      });
      successToast('Item updated.');
      // Replace so back from the detail doesn't return to the editor.
      navigate(`/m/items/${itemId}`, { replace: true });
    } catch (err) {
      console.error('Failed to update item:', err);
      error = err?.message || 'Could not save the item.';
    } finally {
      saving = false;
    }
  }

  function requestCancel() {
    if (isDirty) {
      confirmDiscardOpen = true;
      return;
    }
    leave();
  }

  function leave() {
    // Explicit replace (not history.back) — deterministic even when the editor
    // was reached via deep link, and it drops the stale editor entry.
    navigate(`/m/items/${itemId}`, { replace: true });
  }
</script>

<MobileEditorPage
  title={pageTitle}
  saveLabel="Save"
  {canSave}
  {saving}
  {error}
  onsave={save}
  oncancel={requestCancel}
  dataTestid="mobile-item-edit-page"
>
  {#if loading}
    <div class="center" data-testid="item-edit-loading"><Loader class="spin" size={22} /></div>
  {:else if loadErrored || !item}
    <div class="center" data-testid="item-edit-error">
      <p>Couldn't load this item.</p>
    </div>
  {:else}
    <div class="edit-form" data-testid="item-edit-form">
      <label class="field">
        <span>Title</span>
        <input
          type="text"
          bind:value={title}
          placeholder="What needs doing?"
          autocomplete="off"
          data-testid="item-edit-title"
        />
      </label>

      <label class="field">
        <span>Description <em>(Markdown)</em></span>
        <textarea
          bind:value={description}
          rows={12}
          placeholder="Add detail…"
          data-testid="item-edit-description"
        ></textarea>
      </label>
    </div>
  {/if}
</MobileEditorPage>

<!-- Discard changes? Shown when cancelling with unsaved edits. -->
<MobileConfirmSheet
  bind:isOpen={confirmDiscardOpen}
  title="Discard changes?"
  message="Your edits haven't been saved."
  confirmLabel="Discard"
  cancelLabel="Keep editing"
  destructive
  onconfirm={leave}
  dataTestid="item-edit-discard-sheet"
/>

<style>
  .center {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
    padding: 3rem 1.25rem;
    text-align: center;
    color: var(--ds-text-subtle);
  }
  :global(.spin) {
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .edit-form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
  }
  .field em {
    font-style: normal;
    opacity: 0.7;
  }
  .field input,
  .field textarea {
    padding: 0.6rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-md, 6px);
    background-color: var(--ds-background-input, var(--ds-surface));
    color: var(--ds-text);
    /* >=16px avoids iOS zoom-on-focus (WI-1325). */
    font-size: max(1rem, 16px);
    font-family: inherit;
  }
  .field input {
    font-size: max(1.125rem, 18px);
    font-weight: var(--font-semibold, 600);
  }
  .field textarea {
    resize: vertical;
    min-height: 12rem;
    line-height: 1.5;
  }
  .field input:focus,
  .field textarea:focus {
    outline: none;
    border-color: var(--ds-border-focused, var(--ds-interactive));
  }
</style>
