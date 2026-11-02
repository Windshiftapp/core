<script>
  import { items } from '../api/items.js';
  import { currentRoute, navigate } from '../router.js';
  import { useMobileDraft } from './useMobileDraft.svelte.js';
  import './mobileHeroFields.css';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { formatItemKey } from '../utils/itemKey.js';
  import MobileEditorPage from './MobileEditorPage.svelte';
  import MobileConfirmSheet from './MobileConfirmSheet.svelte';
  import { autoGrow, enterMovesFocus } from '../utils/autoGrowTextarea.js';
  import Loader from '@lucide/svelte/icons/loader';
  import { t, translateError } from '../stores/i18n.svelte.js';

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
  let descriptionField = $state(null);
  let saving = $state(false);
  let error = $state('');

  // Unsaved-input guard and draft persistence: edits ask for confirmation
  // before leaving and survive a reload or app kill until saved or discarded.
  const isDirty = $derived(
    !!item && (title !== (item.title ?? '') || description !== (item.description ?? ''))
  );
  const draft = useMobileDraft({
    draftKey: () => `ws-draft:m-edit:${itemId}`,
    isDirty: () => isDirty,
    isSaving: () => saving,
    values: () => ({ title, description }),
    applyDraft: (saved) => {
      if (typeof saved?.title === 'string') title = saved.title;
      if (typeof saved?.description === 'string') description = saved.description;
    },
    shouldWrite: () => isDirty,
    canHydrate: () => !!item,
  });

  const pageTitle = $derived(formatItemKey(item) || t('mobile.item.edit'));
  const canSave = $derived(title.trim() !== '' && !saving && isDirty);

  $effect(() => {
    const id = itemId;
    if (!id) return;
    let cancelled = false;
    loading = true;
    loadErrored = false;
    items
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
      await items.update(itemId, {
        title: title.trim(),
        description: description.trim(),
      });
      draft.clearDraft();
      successToast(t('mobile.item.updated'));
      // Replace so back from the detail doesn't return to the editor.
      navigate(`/m/items/${itemId}`, { replace: true });
    } catch (err) {
      console.error('Failed to update item:', err);
      error = (err?.code || err?.errorCode || err?.message) ? translateError(err) : t('mobile.item.saveFailed');
    } finally {
      saving = false;
    }
  }

  // Discard: leave the editor. The confirm sheet pushes no history sentinel
  // here (the navigation interceptor already owns the back gesture), so a
  // plain replace is deterministic — including when the editor was reached
  // via deep link and has no sensible entry to pop back to.
  function discardAndLeave() {
    draft.clearDraft();
    draft.confirmDiscardOpen = false;
    navigate(`/m/items/${itemId}`, { replace: true });
  }

  function requestCancel() {
    if (isDirty) {
      draft.confirmDiscardOpen = true;
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
  saveLabel={t('common.save')}
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
      <p>{t('mobile.item.loadFailed')}</p>
    </div>
  {:else}
    <div class="edit-form" data-testid="item-edit-form">
      <!-- Linear-style borderless hero fields. The title is an auto-growing
           textarea: long titles wrap instead of scrolling out of view. -->
      <textarea
        class="hero-title"
        bind:value={title}
        placeholder={t('createModal.issueTitle')}
        autocomplete="off"
        rows={1}
        enterkeyhint="next"
        use:autoGrow={title}
        use:enterMovesFocus={{ next: descriptionField }}
        data-testid="item-edit-title"
      ></textarea>
      <textarea
        class="hero-desc"
        bind:value={description}
        bind:this={descriptionField}
        rows={12}
        placeholder={t('mobile.item.descriptionPlaceholder')}
        use:autoGrow={description}
        data-testid="item-edit-description"
      ></textarea>
    </div>
  {/if}
</MobileEditorPage>

<!-- Discard changes? Shown when cancelling (or pressing back) with
     unsaved edits. The sheet pushes no history sentinel: the navigation
     interceptor owns the back gesture on this page. -->
<MobileConfirmSheet
  bind:isOpen={draft.confirmDiscardOpen}
  title={t('mobile.item.discardChanges')}
  message={t('mobile.item.unsavedChanges')}
  confirmLabel={t('common.discard')}
  cancelLabel={t('mobile.item.keepEditing')}
  destructive
  pushHistory={false}
  onconfirm={discardAndLeave}
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
    gap: 0.5rem;
  }

  /* The borderless hero field styles are shared via mobileHeroFields.css;
     only the edit page's description height differs. */
  .hero-desc {
    min-height: 14rem;
    resize: vertical;
  }
</style>
