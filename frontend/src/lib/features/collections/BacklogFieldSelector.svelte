<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
  import X from '@lucide/svelte/icons/x';
  import Checkbox from '../../components/Checkbox.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { BACKLOG_FIELD_OPTIONS, normalizeBacklogFields } from './backlogFields.js';

  let {
    fields = [],
    onchange = null,
  } = $props();

  const {
    elements: { trigger, content },
    states: { open },
  } = createPopover({
    forceVisible: true,
    positioning: {
      strategy: 'fixed',
      fitViewport: true,
      placement: 'bottom-start',
    },
    portal: 'body',
  });

  let draftFields = $state([]);

  $effect(() => {
    draftFields = normalizeBacklogFields(fields);
  });

  function setField(fieldId, checked) {
    if (checked) {
      draftFields = normalizeBacklogFields([...draftFields, fieldId]);
    } else {
      draftFields = draftFields.filter((id) => id !== fieldId);
    }
  }

  function cancel() {
    draftFields = normalizeBacklogFields(fields);
    open.set(false);
  }

  function apply() {
    onchange?.(normalizeBacklogFields(draftFields));
    open.set(false);
  }
</script>

<button
  use:melt={$trigger}
  type="button"
  class="flex items-center gap-2 px-3 py-2 text-sm rounded-lg text-ds-text-subtle transition-colors"
  title={t('fields.configureFields')}
  aria-label={t('fields.configureFields')}
  data-testid="backlog-field-selector-trigger"
>
  <SlidersHorizontal class="w-4 h-4" />
  <span>{t('fields.configureFields')}</span>
  <ChevronDown class="w-3 h-3" />
</button>

{#if $open}
  <div
    use:melt={$content}
    class="w-72 rounded-lg shadow-xl border border-ds-border bg-ds-surface-raised z-[60] overflow-hidden"
    data-testid="backlog-field-selector-menu"
  >
    <div
      class="flex items-center justify-between border-b border-ds-border px-4 py-3"
    >
      <span class="text-sm font-medium text-ds-text">
        {t('fields.configureFields')}
      </span>
      <button
        type="button"
        class="rounded p-1 text-ds-text-subtle transition-colors hover:bg-[var(--ds-background-neutral-hovered)]"
        aria-label={t('common.close')}
        onclick={cancel}
      >
        <X class="w-4 h-4" />
      </button>
    </div>

    <div class="flex flex-col gap-1 p-2">
      {#each BACKLOG_FIELD_OPTIONS as field (field.id)}
        <div class="rounded px-2 py-2 hover:bg-[var(--ds-background-neutral-hovered)]">
          <Checkbox
            checked={draftFields.includes(field.id)}
            label={t(field.labelKey)}
            onchange={(checked) => setField(field.id, checked)}
            dataTestid={`backlog-field-${field.id}`}
          />
        </div>
      {/each}
    </div>

    <div
      class="flex items-center justify-end gap-2 border-t border-ds-border px-4 py-3"
    >
      <button
        type="button"
        class="rounded px-3 py-1.5 text-sm text-ds-text-subtle transition-colors hover:bg-[var(--ds-background-neutral-hovered)]"
        onclick={cancel}
      >
        {t('common.cancel')}
      </button>
      <button
        type="button"
        class="rounded bg-ds-interactive px-3 py-1.5 text-sm text-ds-text-inverse transition-colors hover:bg-ds-interactive-hovered"
        onclick={apply}
        data-testid="backlog-field-selector-apply"
      >
        {t('common.apply')}
      </button>
    </div>
  </div>
{/if}
