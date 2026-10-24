<script>
  import X from '@lucide/svelte/icons/x';
  import { gradients } from '../utils/gradients.js';
  import { t } from '../stores/i18n.svelte.js';
  import Label from './Label.svelte';

  let {
    selectedGradient = 0,
    hasBackgroundImage = false,
    onSelect = () => {},
    label = null,
    testIdPrefix = 'gradient-swatch',
  } = $props();

  const isSelected = (index) => selectedGradient === index && !hasBackgroundImage;
</script>

<div>
  <Label class="mb-3">{label || t('lookAndFeel.gradients')}</Label>
  <div class="grid grid-cols-10 gap-3">
    {#each gradients as gradient, index}
      <button
        type="button"
        onclick={() => onSelect(index)}
        class="group relative w-[30px] h-[30px] rounded overflow-hidden transition-all hover:scale-110"
        style={isSelected(index) ? 'box-shadow: 0 0 0 2px var(--ds-border-focused); outline-offset: 2px;' : ''}
        title={gradient.name}
        data-testid={`${testIdPrefix}-${index}`}
        aria-pressed={isSelected(index)}
      >
        {#if index === 0}
          <div class="w-full h-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
            <X class="w-3 h-3" style="color: var(--ds-text-subtle);" />
          </div>
        {:else}
          <div class="w-full h-full" style="background: {gradient.value};"></div>
        {/if}

        {#if isSelected(index)}
          <div class="absolute inset-0 flex items-center justify-center bg-black/20">
            <div class="w-3 h-3 bg-white rounded-full flex items-center justify-center">
              <svg class="w-2 h-2" style="color: var(--ds-icon-accent-blue);" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
              </svg>
            </div>
          </div>
        {/if}
      </button>
    {/each}
  </div>
</div>
