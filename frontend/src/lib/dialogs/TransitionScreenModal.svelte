<script>
  // Transition screen modal: shown before a status move whose governing
  // transition carries a screen. It collects the screen's fields plus an
  // optional transition comment and submits them with the move (WI-1835).
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import { untrack } from 'svelte';
  import ScreenFieldsForm from '../features/items/ScreenFieldsForm.svelte';
  import { screens } from '../api/configuration.js';
  import { t } from '../stores/i18n.svelte.js';
  import { getShortcut, getShortcutDisplay, matchesShortcut } from '../utils/keyboardShortcuts.js';

  let {
    show = $bindable(false),
    screen = null,
    targetStatusName = '',
    workspaceId = null,
    milestones = [],
    iterations = [],
    itemLabels = [],
    // Item the move applies to. Its current values pre-fill the displayed
    // fields so the user edits instead of retyping title/description.
    item = null,
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

  // Seed the form from the item's stored values, restricted to the fields the
  // screen actually shows. Only seeded keys are submitted, so a hidden field
  // is never overwritten by a stale copy.
  function initialValues(sourceItem, screenFields) {
    if (!sourceItem) return {};
    const identifiers = new Set((screenFields || []).map((field) => field.field_identifier));
    const seeded = {};
    if (identifiers.has('title')) seeded.title = sourceItem.title ?? '';
    if (identifiers.has('description')) seeded.description = sourceItem.description ?? '';
    if (identifiers.has('priority')) seeded.priority_id = sourceItem.priority_id ?? null;
    if (identifiers.has('assignee')) seeded.assignee_id = sourceItem.assignee_id ?? null;
    if (identifiers.has('team')) seeded.team_id = sourceItem.team_id ?? null;
    if (identifiers.has('iteration')) seeded.iteration_id = sourceItem.iteration_id ?? null;
    if (identifiers.has('project')) seeded.project_id = sourceItem.project_id ?? null;
    for (const key of ['due_date', 'start_date', 'end_date']) {
      if (identifiers.has(key)) seeded[key] = sourceItem[key] ?? '';
    }
    if (identifiers.has('story_points')) seeded.story_points = sourceItem.story_points ?? null;
    if (identifiers.has('estimate') || identifiers.has('estimate_minutes')) {
      seeded.estimate_minutes = sourceItem.estimate_minutes ?? null;
    }
    const customIds = (screenFields || [])
      .filter((field) => field.field_type === 'custom')
      .map((field) => String(field.field_identifier));
    const storedCustom = sourceItem.custom_field_values || {};
    const customValues = {};
    for (const id of customIds) {
      if (storedCustom[id] !== undefined) customValues[id] = storedCustom[id];
    }
    if (Object.keys(customValues).length > 0) seeded.custom_field_values = customValues;
    return seeded;
  }

  $effect(() => {
    const screenId = screen?.id;
    if (!show || !screenId) return;
    const token = ++loadToken;
    // Read the item without subscribing: live item updates must not reset a
    // half-filled screen while the modal is open.
    const sourceItem = untrack(() => item);
    values = {};
    comment = '';
    errorMessage = '';
    missingFields = [];
    loading = true;
    screens
      .getFields(screenId)
      .then((result) => {
        if (token !== loadToken) return;
        fields = result || [];
        values = initialValues(sourceItem, fields);
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

  // Mirror Modal.svelte: once the screen has focus it owns the keyboard, so
  // background shortcuts do not also fire.
  function handleKeydown(event) {
    if (!show) return;
    event.stopPropagation();
    if (matchesShortcut(event, getShortcut('modal', 'submit'))) {
      event.preventDefault();
      submit();
      return;
    }
    if (event.key === 'Enter' && !event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey) {
      const target = /** @type {HTMLElement} */ (event.target);
      if (target?.tagName === 'TEXTAREA' || target?.isContentEditable) return;
      event.preventDefault();
      submit();
    }
  }

  const submitHint = $derived(getShortcutDisplay('modal', 'submit'));

  const missingLabel = $derived(
    missingFields
      .map((field) => field.identifier)
      .filter(Boolean)
      .join(', ')
  );
</script>

<ModalBackdrop
  bind:show
  onclose={cancel}
  onKeydown={handleKeydown}
  ariaLabelledBy="transition-screen-title"
  zIndex={60}
  portal={true}
>
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
        <ScreenFieldsForm bind:values {workspaceId} {milestones} {iterations} {itemLabels} screenFields={fields} />

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
      showKeyboardHint={true}
      confirmKeyboardHint={submitHint}
    />
  </div>
</ModalBackdrop>
