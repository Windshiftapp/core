<script>
  let {
    type = 'text',
    value = $bindable(''),
    placeholder = '',
    disabled = false,
    required = false,
    autofocus = false,
    size = 'medium',
    min = undefined,
    max = undefined,
    step = undefined,
    id = undefined,
    class: className = '',
    // Optional ref binding for parent components that need the raw input element
    inputRef = $bindable(null)
  } = $props();
  export { className as class };

  // Size variants
  const sizeClasses = $derived({
    small: 'px-3 py-2.5 text-sm',
    medium: 'px-4 py-3'
  }[size] || 'px-4 py-3');

  // Combine all classes
  const allClasses = $derived([
    'w-full rounded border transition-all duration-200',
    'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50',
    sizeClasses,
    className
  ].filter(Boolean).join(' '));
</script>

<input
  {type}
  {id}
  bind:value
  bind:this={inputRef}
  {placeholder}
  {disabled}
  {required}
  {autofocus}
  {min}
  {max}
  {step}
  class={allClasses}
  style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
  on:input
  on:change
  on:focus
  on:blur
  on:keydown
  on:keyup
/>
