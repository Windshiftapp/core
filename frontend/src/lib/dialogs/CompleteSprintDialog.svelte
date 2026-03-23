<script>
  import { CheckCircle, X } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import ItemPicker from '../pickers/ItemPicker.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { getShortcut, matchesShortcut } from '../utils/keyboardShortcuts.js';

  let {
    show = $bindable(false),
    iteration = null,
    incompleteItems = [],
    targetIterations = [],
    onconfirm = null,
    oncancel = null,
  } = $props();

  const submitShortcut = getShortcut('modal', 'submit');

  let moveTarget = $state('backlog'); // 'backlog' | 'sprint'
  let selectedSprintId = $state(null);

  let doneCount = $derived(
    iteration ? (iteration._totalItems ?? 0) - incompleteItems.length : 0
  );
  let incompleteCount = $derived(incompleteItems.length);
  let hasIncomplete = $derived(incompleteCount > 0);

  const sprintPickerConfig = {
    primary: { text: (item) => item.name },
    secondary: { text: (item) => item.status },
    searchFields: ['name'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
  };

  // Reset state when dialog opens
  $effect(() => {
    if (show) {
      moveTarget = 'backlog';
      selectedSprintId = null;
    }
  });

  function handleKeydown(event) {
    if (!show) return;

    if (matchesShortcut(event, submitShortcut)) {
      event.preventDefault();
      doConfirm();
      return;
    }

    if (event.key === 'Enter' && !event.ctrlKey && !event.metaKey) {
      const activeElement = document.activeElement;
      const isOnCancelButton = activeElement?.textContent?.trim() === t('common.cancel');
      if (!isOnCancelButton) {
        event.preventDefault();
        doConfirm();
      }
    }
  }

  function doConfirm() {
    if (hasIncomplete && moveTarget === 'sprint' && !selectedSprintId) return;

    const result = moveTarget === 'backlog'
      ? { type: 'backlog' }
      : { type: 'sprint', iterationId: selectedSprintId };

    onconfirm?.(result);
    show = false;
  }

  function cancel() {
    oncancel?.();
    show = false;
  }

  let confirmDisabled = $derived(
    hasIncomplete && moveTarget === 'sprint' && !selectedSprintId
  );
</script>

<svelte:window onkeydown={handleKeydown} />

<ModalBackdrop bind:show onclose={cancel} ariaLabelledBy="complete-sprint-title" zIndex={70}>
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div
      role="presentation"
      class="bg-white rounded shadow-xl max-w-md w-full transform transition-all"
      style="background-color: var(--ds-surface-raised);"
      onclick={(e) => e.stopPropagation()}
    >
      <!-- Header -->
      <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
        <div class="flex items-center gap-3">
          <div class="flex-shrink-0">
            <CheckCircle
              class="w-6 h-6"
              style="color: var(--ds-icon-success, var(--ds-interactive));"
            />
          </div>
          <h3
            id="complete-sprint-title"
            class="text-lg font-medium flex-1"
            style="color: var(--ds-text);"
          >
            {t('iterations.completeSprint')}
          </h3>
          <Button
            variant="ghost"
            icon={X}
            onclick={cancel}
            title={t('common.close')}
          />
        </div>
      </div>

      <!-- Body -->
      <div class="px-6 py-4 space-y-4">
        <p class="text-sm" style="color: var(--ds-text);">
          {t('iterations.completeSprintConfirm', { name: iteration?.name ?? '' })}
        </p>

        {#if hasIncomplete}
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {t('iterations.itemsDone', { count: doneCount })}, {t('iterations.itemsIncomplete', { count: incompleteCount })}
          </p>

          <div class="space-y-3">
            <p class="text-sm font-medium" style="color: var(--ds-text);">
              {t('iterations.incompleteItemsAction')}
            </p>

            <!-- Radio: Move to backlog -->
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="radio"
                name="move-target"
                value="backlog"
                bind:group={moveTarget}
                class="accent-[var(--ds-interactive)]"
              />
              <span class="text-sm" style="color: var(--ds-text);">
                {t('iterations.moveToBacklog')}
              </span>
            </label>

            <!-- Radio: Move to another sprint -->
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="radio"
                name="move-target"
                value="sprint"
                bind:group={moveTarget}
                class="accent-[var(--ds-interactive)]"
              />
              <span class="text-sm" style="color: var(--ds-text);">
                {t('iterations.moveToSprint')}
              </span>
            </label>

            {#if moveTarget === 'sprint'}
              <div class="ml-6">
                {#if targetIterations.length > 0}
                  <ItemPicker
                    bind:value={selectedSprintId}
                    items={targetIterations}
                    config={sprintPickerConfig}
                    placeholder={t('iterations.filterBySprint')}
                    allowClear={false}
                  />
                {:else}
                  <p class="text-sm" style="color: var(--ds-text-subtle);">
                    {t('iterations.noIterations')}
                  </p>
                {/if}
              </div>
            {/if}
          </div>
        {:else}
          <p class="text-sm" style="color: var(--ds-text-success, var(--ds-text-subtle));">
            {t('iterations.allItemsDone')}
          </p>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t flex justify-end gap-3" style="border-color: var(--ds-border);">
        <Button
          variant="default"
          onclick={cancel}
          size="small"
          keyboardHint="Esc"
        >
          {t('common.cancel')}
        </Button>

        <Button
          variant="primary"
          onclick={doConfirm}
          size="small"
          disabled={confirmDisabled}
          keyboardHint="↵"
        >
          {t('iterations.complete')}
        </Button>
      </div>
    </div>
</ModalBackdrop>

<style>
  /* ItemPicker popover portals to body at z-[60], but modal backdrop is z-70.
     Override the popover z-index so it renders above the modal. */
  :global([data-melt-popover-content]) {
    z-index: 80 !important;
  }
</style>
