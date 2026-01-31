<script>
  let {
    checked = $bindable(false),
    disabled = false,
    size = 'medium',           // 'small' | 'medium'
    label = null,
    hint = null,               // Optional hint text (e.g., "Not visible on portal")
    labelPosition = 'right',   // 'left' | 'right'
    onchange = null,
    class: className = ''
  } = $props();

  // Size variants
  const sizes = {
    small: { box: 'w-4 h-4', checkmark: 'w-2.5 h-2.5', text: 'text-xs', hint: 'text-[11px]' },
    medium: { box: 'w-5 h-5', checkmark: 'w-3 h-3', text: 'text-sm', hint: 'text-xs' }
  };

  const currentSize = $derived(sizes[size] || sizes.medium);

  function handleChange(event) {
    checked = event.target.checked;
    onchange?.(checked);
  }
</script>

<label
  class="checkbox-wrapper inline-flex items-center gap-2 {labelPosition === 'left' ? 'flex-row-reverse' : ''} {disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'} {className}"
>
  <span class="checkbox-box {currentSize.box}" class:checked class:disabled>
    <input
      type="checkbox"
      {checked}
      {disabled}
      onchange={handleChange}
      class="sr-only"
    />
    {#if checked}
      <svg class="checkmark {currentSize.checkmark}" viewBox="0 0 12 12" fill="none">
        <path d="M2.5 6L5 8.5L9.5 3.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    {/if}
  </span>
  {#if label}
    <span class="checkbox-label {currentSize.text}">{label}</span>
  {/if}
  {#if hint}
    <span class="checkbox-hint {currentSize.hint}">({hint})</span>
  {/if}
</label>

<style>
  .checkbox-wrapper {
    user-select: none;
  }

  .checkbox-box {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 4px;
    border: 1.5px solid var(--ds-border);
    background-color: var(--ds-background-input);
    transition: all 150ms ease;
    flex-shrink: 0;
  }

  .checkbox-box:not(.disabled):hover {
    border-color: var(--ds-border-focused);
  }

  .checkbox-box.checked {
    background-color: var(--ds-interactive);
    border-color: var(--ds-interactive);
  }

  .checkbox-box.checked:not(.disabled):hover {
    background-color: var(--ds-interactive-hovered);
    border-color: var(--ds-interactive-hovered);
  }

  .checkbox-box.disabled {
    cursor: not-allowed;
  }

  .checkmark {
    color: white;
  }

  .checkbox-label {
    font-weight: 500;
    color: var(--ds-text);
  }

  .checkbox-hint {
    color: var(--ds-text-subtlest);
  }
</style>
