<script>
  let {
    value = $bindable(''),
    placeholder = '',
    disabled = false,
    required = false,
    rows = 3,
    size = 'medium',
    id = undefined,
    class: className = '',
    // Svelte 5 event handlers
    oninput = undefined,
    onchange = undefined,
    onfocus = undefined,
    onblur = undefined,
    onkeydown = undefined
  } = $props();
  export { className as class };

  // Size variants
  const sizeClasses = $derived({
    small: 'px-3 py-2.5 text-sm',
    medium: 'px-4 py-3'
  }[size] || 'px-4 py-3');

  // Combine all classes
  const allClasses = $derived([
    'w-full rounded border transition-all duration-200 resize-none',
    'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50',
    sizeClasses,
    className
  ].filter(Boolean).join(' '));
</script>

<textarea
  {id}
  bind:value
  {placeholder}
  {disabled}
  {required}
  {rows}
  class={allClasses}
  style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
  {oninput}
  {onchange}
  {onfocus}
  {onblur}
  {onkeydown}
></textarea>
