<script>
  import { cn } from '../utils/cn.js';
  import { controlSizeClasses } from '../utils/controlSizes.js';

  let {
    value = $bindable(''),
    options = [],
    placeholder = '',
    disabled = false,
    required = false,
    multiple = false,
    size = 'medium',
    id = undefined,
    class: className = '',
    dataTestid = undefined,
    ariaLabel = undefined,
    onchange = undefined,
  } = $props();

  const sizeClasses = $derived(controlSizeClasses(size));

  const classes = $derived(
    cn(
      'w-full rounded border transition-all duration-200',
      'focus:outline-none focus:ring-2 focus:ring-[var(--ds-border-focused)] focus:ring-opacity-50',
      'disabled:cursor-not-allowed disabled:opacity-50',
      sizeClasses,
      className,
    ),
  );

  function handleChange(event) {
    onchange?.(value, event);
  }
</script>

{#if multiple}
  <select
    {id}
    bind:value
    {disabled}
    {required}
    multiple
    dir="auto"
    data-testid={dataTestid}
    aria-label={ariaLabel}
    class={classes}
    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
    onchange={handleChange}
  >
    {#each options as option}
      <option value={option.value} disabled={option.disabled}>{option.label}</option>
    {/each}
  </select>
{:else}
  <select
    {id}
    bind:value
    {disabled}
    {required}
    dir="auto"
    data-testid={dataTestid}
    aria-label={ariaLabel}
    class={classes}
    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
    onchange={handleChange}
  >
    {#if placeholder}
      <option value="" disabled={required}>{placeholder}</option>
    {/if}
    {#each options as option}
      <option value={option.value} disabled={option.disabled}>{option.label}</option>
    {/each}
  </select>
{/if}
