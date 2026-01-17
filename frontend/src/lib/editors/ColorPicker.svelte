<script>
  import Modal from '../dialogs/Modal.svelte';
  import Label from '../components/Label.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Color palette matching IconSelector - 25 colors for 5x5 grid
  export const DEFAULT_COLORS = [
    '#7c3aed', '#2563eb', '#059669', '#dc2626', '#ea580c',
    '#6b7280', '#8b5cf6', '#3b82f6', '#10b981', '#ef4444',
    '#f59e0b', '#84cc16', '#06b6d4', '#ec4899', '#f97316',
    '#64748b', '#7c2d12', '#1e40af', '#065f46', '#991b1b',
    '#92400e', '#365314', '#0e7490', '#be185d', '#9a3412'
  ];

  let {
    value = $bindable('#6b7280'),
    label = '',
    presets = DEFAULT_COLORS,
    compact = false  // When true, shows a swatch that opens a modal
  } = $props();

  let showModal = $state(false);
</script>

{#if compact}
  <!-- Compact mode: color swatch that opens modal -->
  <button
    type="button"
    class="w-6 h-6 rounded cursor-pointer transition-transform hover:scale-105"
    style="background-color: {value}; border: 1px solid var(--ds-border);"
    onclick={() => showModal = true}
    title={t('editors.clickToChangeColor')}
  />

  <Modal isOpen={showModal} onclose={() => showModal = false} maxWidth="max-w-[200px]">
    <div class="p-3">
      {#if label}
        <h3 class="text-sm font-medium mb-2" style="color: var(--ds-text);">{label}</h3>
      {/if}

      <!-- Preset color grid - 5x5 -->
      <div class="grid grid-cols-5 gap-1.5 mb-3">
        {#each presets as color}
          <button
            type="button"
            class="w-6 h-6 rounded cursor-pointer transition-transform hover:scale-110"
            style="background-color: {color}; border: 2px solid {value === color ? 'var(--ds-border-focused, #3b82f6)' : 'transparent'}; {value === color ? 'box-shadow: 0 0 0 1px var(--ds-focus-ring, rgba(59, 130, 246, 0.3));' : ''}"
            onclick={() => { value = color; showModal = false; }}
            title={color}
          />
        {/each}
      </div>

      <!-- Native color input for custom colors -->
      <div class="flex items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
        <input
          type="color"
          bind:value
          class="w-6 h-6 rounded cursor-pointer p-0"
          style="border: 1px solid var(--ds-border);"
        />
        <span class="text-xs" style="color: var(--ds-text-subtle);">{t('common.custom')}</span>
      </div>
    </div>
  </Modal>
{:else}
  <!-- Full mode: inline preset grid + native picker -->
  {#if label}
    <Label color="default" class="mb-2">{label}</Label>
  {/if}

  <!-- Preset color grid - 5x5 -->
  <div class="grid grid-cols-5 gap-1.5 mb-3">
    {#each presets as color}
      <button
        type="button"
        class="w-6 h-6 rounded cursor-pointer transition-transform hover:scale-110"
        style="background-color: {color}; border: 2px solid {value === color ? 'var(--ds-border-focused, #3b82f6)' : 'transparent'}; {value === color ? 'box-shadow: 0 0 0 1px var(--ds-focus-ring, rgba(59, 130, 246, 0.3));' : ''}"
        onclick={() => value = color}
        title={color}
      />
    {/each}
  </div>

  <!-- Native color input for custom colors -->
  <div class="flex items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
    <input
      type="color"
      bind:value
      class="w-6 h-6 rounded cursor-pointer p-0"
      style="border: 1px solid var(--ds-border);"
    />
    <span class="text-xs" style="color: var(--ds-text-subtle);">{t('common.custom')}</span>
  </div>
{/if}
