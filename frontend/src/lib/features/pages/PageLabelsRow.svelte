<script>
  import IconSelector from '../../pickers/IconSelector.svelte';
  import PageLabelPicker from './PageLabelPicker.svelte';
  import IconX from '@tabler/icons-svelte-runes/icons/x';
  import { t } from '../../stores/i18n.svelte.js';

  /** Page-icon appearance controls and the attached label chips. */
  let {
    page,
    workspaceId,
    editable = false,
    appearanceSaving = false,
    pickerIcon = $bindable('Plus'),
    pickerColor = $bindable('#3b82f6'),
    selectedLabelIds = new Set(),
    onToggleLabel,
    onChangeAppearance,
    onClearAppearance,
  } = $props();
</script>

<div
  class="-mt-2 flex flex-wrap items-center gap-1.5 px-(--page-gutter)"
  data-testid="page-label-row"
>
  {#if editable}
    <div class="appearance-actions group inline-flex items-center gap-0" aria-label="Page icon">
      <IconSelector
        dataTestid="page-icon-picker"
        bind:selectedIcon={pickerIcon}
        bind:selectedColor={pickerColor}
        compact
        hideLabel
        label=""
        triggerVariant="badge"
        triggerTitle={page.metadata?.icon ? 'Change page icon' : 'Add page icon'}
        onchange={(event) => onChangeAppearance(event.detail)}
      />
      {#if page.metadata?.icon}
        <button
          type="button"
          class="pointer-events-none ml-0.5 inline-flex h-6 w-4 items-center justify-center rounded-sm border-0 bg-transparent p-0 text-ds-text-subtle opacity-0 transition-[opacity,background-color,color] duration-120 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100 hover:bg-ds-background-neutral-hovered hover:text-ds-text disabled:cursor-not-allowed disabled:opacity-50"
          onclick={() => onClearAppearance()}
          disabled={appearanceSaving}
          aria-label="Remove page icon"
          title="Remove page icon"
        >
          <IconX size={12} />
        </button>
      {/if}
    </div>
  {/if}
  {#each page.labels || [] as label (label.id)}
    <span
      class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs"
      style="background-color: {label.color || '#3B82F6'}1A; color: var(--ds-text); border: 1px solid {label.color || '#3B82F6'};"
      data-testid="page-label-chip"
      data-label-id={label.id}
    >
      <span
        class="inline-block w-2 h-2 rounded-full"
        style="background-color: {label.color || '#3B82F6'};"
        aria-hidden="true"
      ></span>
      {label.name}
      {#if editable}
        <button
          type="button"
          class="inline-flex cursor-pointer items-center justify-center border-0 bg-transparent p-0 text-inherit opacity-70 transition-opacity duration-120 hover:opacity-100"
          onclick={() => onToggleLabel(label)}
          aria-label={t('pages.labelsRemoveAria', { name: label.name })}
          data-testid="page-label-chip-remove"
        >
          <IconX size={12} />
        </button>
      {/if}
    </span>
  {/each}
  {#if editable}
    <PageLabelPicker
      {workspaceId}
      selectedIds={selectedLabelIds}
      onToggle={onToggleLabel}
      triggerLabel={t('pages.labelsAdd')}
    />
  {/if}
</div>

<style>
  /* IconSelector's compact variant is a full-width form control; as the
     badge-style page-icon trigger it must hug its content instead. */
  .appearance-actions :global(.icon-selector-compact) {
    width: auto;
  }
</style>
