<script>
  import BackButton from '../components/BackButton.svelte';
  import SetDetailActions from './SetDetailActions.svelte';

  let {
    loading = false,
    title = '',
    description = '',
    backTooltip = '',
    onBack,
    hasUnsavedChanges = false,
    saving = false,
    onSave,
    saveTestid = undefined,
    loadingLabel = '',
    contentClass = 'max-w-4xl mx-auto',
    children,
  } = $props();
</script>

<div class="flex flex-col h-full" style="background-color: var(--ds-surface);">
  <!-- Header -->
  <div class="border-b px-6 py-4" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-4">
        <BackButton onclick={onBack} tooltip={backTooltip} class="p-1" />
        <div>
          <h1 class="text-xl font-semibold" style="color: var(--ds-text);">{title}</h1>
          <p class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">{description}</p>
        </div>
      </div>
      <div class="flex items-center gap-3">
        <SetDetailActions {hasUnsavedChanges} {saving} onCancel={onBack} {onSave} {saveTestid} />
      </div>
    </div>
  </div>

  <!-- Content -->
  <div class="flex-1 overflow-y-auto p-6" style="background-color: var(--ds-surface);">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <div style="color: var(--ds-text-subtle);">{loadingLabel}</div>
      </div>
    {:else}
      <div class={contentClass}>
        {@render children?.()}
      </div>
    {/if}
  </div>
</div>
