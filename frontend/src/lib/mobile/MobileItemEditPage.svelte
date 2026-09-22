<script>
  import { api } from '../api.js';
  import { currentRoute, navigate, setNavigationInterceptor } from '../router.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { formatItemKey } from '../utils/itemKey.js';
  import MobileEditorPage from './MobileEditorPage.svelte';
  import MobileConfirmSheet from './MobileConfirmSheet.svelte';
  import { autoGrow, enterMovesFocus } from './autoGrowTextarea.js';
  import { Loader } from '@lucide/svelte';
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

  // Unsaved-input guard: leaving with changes asks for confirmation — via
  // the header Cancel and via the back gesture (router interceptor).
  let confirmDiscardOpen = $state(false);
  const isDirty = $derived(
    !!item && (title !== (item.title ?? '') || description !== (item.description ?? ''))
  );

  $effect(() => {
    setNavigationInterceptor(() => {
      if (!isDirty) return false;
      confirmDiscardOpen = true;
      return true;
    });
    return () => setNavigationInterceptor(null);
  });

  // Draft persistence (sessionStorage): edits survive a reload or app kill;
  // cleared on save and on explicit discard.
  const draftKey = $derived(`ws-draft:m-edit:${itemId}`);
  let draftRetired = false;
  $effect(() => {
    if (!item) return;
    try {
      const raw = sessionStorage.getItem(draftKey);
      if (!raw) return;
      const draft = JSON.parse(raw);
      if (typeof draft?.title === 'string') title = draft.title;
      if (typeof draft?.description === 'string') description = draft.description;
    } catch {
      /* corrupt draft — use server values */
    }
  });
  $effect(() => {
    if (!item || saving || draftRetired) return;
    try {
      if (isDirty) {
        sessionStorage.setItem(draftKey, JSON.stringify({ title, description }));
      } else {
        sessionStorage.removeItem(draftKey);
      }
    } catch {
      /* storage unavailable — drafts are best-effort */
    }
  });

  function clearDraft() {
    // Retire the write-back effect first: when `saving` flips to false in
    // save's finally, the effect must not resurrect the draft before the
    // page unmounts.
    draftRetired = true;
    try {
      sessionStorage.removeItem(draftKey);
    } catch {
      /* ignore */
    }
  }

  const pageTitle = $derived(formatItemKey(item) || t('mobile.item.edit'));
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
      clearDraft();
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
    clearDraft();
    setNavigationInterceptor(null);
    confirmDiscardOpen = false;
    navigate(`/m/items/${itemId}`, { replace: true });
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
        data-testid="item-edit-description"
      ></textarea>
    </div>
  {/if}
</MobileEditorPage>

<!-- Discard changes? Shown when cancelling (or pressing back) with
     unsaved edits. The sheet pushes no history sentinel: the navigation
     interceptor owns the back gesture on this page. -->
<MobileConfirmSheet
  bind:isOpen={confirmDiscardOpen}
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

  /* Linear-style borderless hero fields. The title textarea wraps and grows
     with its content (autoGrow action) so long titles stay fully visible. */
  .hero-title {
    width: 100%;
    margin: 0.75rem 0 0;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text);
    font-family: inherit;
    font-size: 1.35rem;
    font-weight: var(--font-semibold, 600);
    line-height: 1.25;
    overflow: hidden;
    resize: none;
  }
  .hero-title::placeholder { color: var(--ds-text-subtlest, var(--ds-text-subtle)); font-weight: var(--font-semibold, 600); }
  .hero-desc {
    width: 100%;
    min-height: 14rem;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text);
    font-family: inherit;
    font-size: max(1rem, 16px);
    line-height: 1.5;
    resize: vertical;
  }
  .hero-desc::placeholder { color: var(--ds-text-subtlest, var(--ds-text-subtle)); }
  .hero-title:focus,
  .hero-desc:focus { outline: none; }
</style>
