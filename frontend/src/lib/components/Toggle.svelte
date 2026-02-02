<script>
  let {
    checked = $bindable(false),
    disabled = false,
    size = 'medium',           // 'small' | 'medium' | 'large'
    label = null,
    labelPosition = 'right',   // 'left' | 'right'
    onchange = null,
    class: className = ''
  } = $props();

  // Size variants
  const sizes = {
    small: { button: 'h-5 w-9', knob: 'h-3 w-3', translateOn: 'translate-x-5', translateOff: 'translate-x-1' },
    medium: { button: 'h-6 w-11', knob: 'h-4 w-4', translateOn: 'translate-x-6', translateOff: 'translate-x-1' },
    large: { button: 'h-7 w-14', knob: 'h-5 w-5', translateOn: 'translate-x-8', translateOff: 'translate-x-1' }
  };

  const currentSize = $derived(sizes[size] || sizes.medium);

  function handleClick() {
    if (disabled) return;
    checked = !checked;
    onchange?.(checked);
  }
</script>

{#if label}
  <div
    class="inline-flex items-center gap-3 {labelPosition === 'left' ? 'flex-row-reverse' : ''} {disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}"
    role="group"
  >
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      {disabled}
      class="relative inline-flex items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--ds-border-focused)]
             disabled:cursor-not-allowed {currentSize.button} {className}"
      style="background-color: {checked ? 'var(--ds-interactive)' : 'var(--ds-background-neutral)'};"
      onclick={handleClick}
    >
      <span
        class="inline-block transform rounded-full bg-white transition-transform shadow-sm {currentSize.knob} {checked ? currentSize.translateOn : currentSize.translateOff}"
      ></span>
    </button>
    <span class="text-sm text-[var(--ds-text)]">{label}</span>
  </div>
{:else}
  <button
    type="button"
    role="switch"
    aria-checked={checked}
    {disabled}
    class="relative inline-flex items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--ds-border-focused)]
           disabled:opacity-50 disabled:cursor-not-allowed {currentSize.button} {className}"
    style="background-color: {checked ? 'var(--ds-interactive)' : 'var(--ds-background-neutral)'};"
    onclick={handleClick}
  >
    <span
      class="inline-block transform rounded-full bg-white transition-transform shadow-sm {currentSize.knob} {checked ? currentSize.translateOn : currentSize.translateOff}"
    ></span>
  </button>
{/if}
