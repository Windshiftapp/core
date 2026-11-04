<script>
  // Transition screen modal: shown before a status move whose governing
  // transition carries a screen. It collects the screen's fields plus an
  // optional transition comment and submits them with the move (WI-1835).
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import ScreenFieldsForm from '../features/items/ScreenFieldsForm.svelte';
  import { screens } from '../api/configuration.js';
  import { t } from '../stores/i18n.svelte.js';
  import { getShortcut, matchesShortcut } from '../utils/keyboardShortcuts.js';

  let {
    show = $bindable(false),
    screen = null,
    targetStatusName = '',
    workspaceId = null,
    milestones = [],
    iterations = [],
    onconfirm = null,
    oncancel = null,
  } = $props();

  let fields = $state([]);
  let values = $state({});
  let comment = $state('');
  let loading = $state(false);
  let submitting = $state(false);
  let errorMessage = $state('');
  let missingFields = $state([]);

  let loadToken = 0;

  $effect(() => {
    const screenId = screen?.id;
    if (!show || !screenId) return;
    const token = ++loadToken;
    values = {};
    comment = '';
    errorMessage = '';
    missingFields = [];
    loading = true;
    screens
      .getFields(screenId)
      .then((result) => {
        if (token === loadToken) fields = result || [];
      })
      .catch(() => {
        if (token === loadToken) fields = [];
      })
      .finally(() => {
        if (token === loadToken) loading = false;
      });
  });

  async function submit() {
    if (submitting || loading) return;
    submitting = true;
    errorMessage = '';
    missingFields = [];
    const payload = /** @type {Record<string, any>} */ ({ ...values });
    if (comment.trim()) {
      payload.comment = comment.trim();
    }
    try {
      await onconfirm?.(payload);
      show = false;
    } catch (err) {
      if (err?.code === 'screen_required' && Array.isArray(err?.details?.missing_fields)) {
        missingFields = err.details.missing_fields;
        errorMessage = t('transitionScreen.missingFields');
      } else {
        errorMessage = err?.message || t('transitionScreen.failed');
      }
    } finally {
      submitting = false;
    }
  }

  function cancel() {
    oncancel?.();
    show = false;
  }

  function handleKeydown(event) {
    if (!show) return;
    if (matchesShortcut(event, getShortcut('modal', 'submit'))) {
      event.preventDefault();
      submit();
    }
  }

  const missingLabel = $derived(
    missingFields
      .map((field) => field.identifier)
      .filter(Boolean)
      .join(', ')
  );
</script>

<svelte:window onkeydown={handleKeydown} />

<ModalBackdrop bind:show onclose={cancel} ariaLabelledBy="transition-screen-title">
  <div
    class="w-full max-w-lg rounded-lg shadow-xl"
    style="background-color: var(--ds-surface-overlay, var(--ds-surface)); color: var(--ds-text);"
    data-testid="transition-screen-modal"
  >
    <div class="px-5 py-4 border-b" style="border-color: var(--ds-border);">
      <h2 id="transition-screen-title" class="text-lg font-semibold">
        {screen?.name || t('transitionScreen.title')}
      </h2>
      {#if targetStatusName}
        <p class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">
          {t('transitionScreen.moveTo', { status: targetStatusName })}
        </p>
      {/if}
    </div>

    <div class="px-5 py-4 max-h-[60vh] overflow-y-auto space-y-4">
      {#if loading}
        <div class="py-6 text-center text-sm" style="color: var(--ds-text-subtle);">
          {t('common.loading')}
        </div>
      {:else}
        <ScreenFieldsForm bind:values {workspaceId} {milestones} {iterations} screenFields={fields} />

        <div class="space-y-1">
          <Label color="default">{t('transitionScreen.comment')}</Label>
          <Textarea bind:value={comment} placeholder={t('transitionScreen.commentPlaceholder')} data-testid="transition-screen-comment" />
        </div>

        {#if errorMessage}
          <div
            class="rounded border px-3 py-2 text-sm"
            style="border-color: var(--ds-border-danger, #ef4444); color: var(--ds-text-danger, #ef4444);"
            data-testid="transition-screen-error"
          >
            {errorMessage}
            {#if missingLabel}
              <span class="block mt-0.5">{missingLabel}</span>
            {/if}
          </div>
        {/if}
      {/if}
    </div>

    <DialogFooter
      onCancel={cancel}
      onConfirm={submit}
      loading={submitting}
      disabled={loading}
      confirmLabel={t('transitionScreen.confirm')}
    />
  </div>
</ModalBackdrop>
