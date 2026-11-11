<script>
  import { controlSizeClasses } from '../utils/controlSizes.js';

  let {
    value = $bindable(''),
    placeholder = '',
    disabled = false,
    required = false,
    autofocus = false,
    spellcheck = undefined,
    rows = 3,
    dir = 'auto',
    size = 'medium',
    id = undefined,
    class: className = '',
    style = '',
    'data-testid': dataTestid = undefined,
    textareaRef = $bindable(null),
    // Svelte 5 event handlers
    oninput = undefined,
    onchange = undefined,
    onfocus = undefined,
    onblur = undefined,
    onkeydown = undefined,
    onclick = undefined,
    ...restProps
  } = $props();
  export { className as class };

  // Size variants
  const sizeClasses = $derived(controlSizeClasses(size));

  /** @type {'ltr' | 'rtl' | 'auto'} */
  const dirValue = $derived(/** @type {any} */ (dir));

  // Combine all classes
  const allClasses = $derived([
    'w-full rounded border transition-all duration-200 resize-none',
    'focus:outline-none focus:ring-2 focus:ring-[var(--ds-border-focused)] focus:ring-opacity-50',
    sizeClasses,
    className
  ].filter(Boolean).join(' '));
</script>

<!-- svelte-ignore a11y_autofocus -->
<textarea
  {...restProps}
  {id}
  bind:value
  bind:this={textareaRef}
  {placeholder}
  {disabled}
  {required}
  {autofocus}
  {spellcheck}
  {rows}
  dir={dirValue}
  class={allClasses}
  style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text); {style}"
  data-testid={dataTestid}
  {oninput}
  {onchange}
  {onfocus}
  {onblur}
  {onkeydown}
  {onclick}
></textarea>
