<script>
  import { cn } from '../utils/cn.js';
  import { controlSizeClasses } from '../utils/controlSizes.js';
  import DatePicker from '../pickers/DatePicker.svelte';

  /**
   * @type {{
   *   type?: string,
   *   value?: any,
   *   placeholder?: string,
   *   disabled?: boolean,
   *   required?: boolean,
   *   autofocus?: boolean,
   *   variant?: 'default' | 'ghost',
   *   size?: string,
   *   min?: any,
   *   max?: any,
   *   step?: any,
   *   id?: string,
   *   name?: string,
   *   pattern?: string,
   *   title?: string,
   *   autocomplete?: string,
   *   enterkeyhint?: 'enter' | 'done' | 'go' | 'next' | 'previous' | 'search' | 'send',
   *   readonly?: boolean,
   *   minlength?: number,
   *   maxlength?: number,
   *   list?: string,
   *   class?: string,
   *   style?: string,
   *   dataTestid?: string,
   *   dataAutofocus?: boolean,
   *   ariaLabel?: string,
   *   ariaDescribedby?: string,
   *   inputRef?: any,
   *   oninput?: (e?: any) => void,
   *   onchange?: (e?: any) => void,
   *   onfocus?: (e?: any) => void,
   *   onblur?: (e?: any) => void,
   *   onclick?: (e?: any) => void,
   *   onkeydown?: (e?: any) => void,
   *   onkeyup?: (e?: any) => void,
   * }}
   */
  let {
    type = 'text',
    value = $bindable(''),
    placeholder = '',
    disabled = false,
    required = false,
    autofocus = false,
    variant = 'default',
    size = 'medium',
    min = undefined,
    max = undefined,
    step = undefined,
    id = undefined,
    name = undefined,
    pattern = undefined,
    title = undefined,
    autocomplete = undefined,
    enterkeyhint = undefined,
    readonly = false,
    minlength = undefined,
    maxlength = undefined,
    list = undefined,
    class: className = '',
    style = '',
    dataTestid = undefined,
    dataAutofocus = false,
    ariaLabel = undefined,
    ariaDescribedby = undefined,
    // Optional ref binding for parent components that need the raw input element
    inputRef = $bindable(null),
    // Event handlers
    oninput = undefined,
    onchange = undefined,
    onfocus = undefined,
    onblur = undefined,
    onclick = undefined,
    onkeydown = undefined,
    onkeyup = undefined
  } = $props();
  export { className as class };

  // Size variants. Ghost inputs are chrome-less, so they skip control padding.
  const sizeClasses = $derived(variant === 'ghost' ? '' : controlSizeClasses(size));

  // Combine all classes
  const allClasses = $derived(cn(
    variant === 'ghost' ? 'transition-all duration-200' : 'w-full transition-all duration-200',
    variant === 'ghost'
      ? 'border-0 bg-transparent focus:outline-none focus:ring-0'
      : 'rounded border focus:outline-none focus:ring-2 focus:ring-[var(--ds-border-focused)] focus:ring-opacity-50',
    sizeClasses,
    className
  ));
  const variantStyle = $derived(
    variant === 'ghost'
      ? 'background-color: transparent; border-color: transparent; color: var(--ds-text);'
      : 'background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);'
  );
  /** @type {any} */
  const autocompleteValue = $derived(autocomplete);
</script>

<!-- svelte-ignore a11y_autofocus -->
{#if type === 'date'}
  <DatePicker
    bind:value
    bind:inputRef
    {id}
    {name}
    {min}
    {max}
    {disabled}
    {required}
    readonly={readonly}
    {placeholder}
    {size}
    class={className}
    {dataTestid}
    {ariaLabel}
    {onkeydown}
    {onblur}
    {onfocus}
    onchange={(next) => {
      const synthetic = { target: { value: next } };
      oninput?.(synthetic);
      onchange?.(synthetic);
    }}
  />
{:else}
<input
  {type}
  {id}
  {name}
  {pattern}
  {title}
  autocomplete={autocompleteValue}
  {enterkeyhint}
  {readonly}
  {minlength}
  {maxlength}
  {list}
  bind:value
  bind:this={inputRef}
  data-testid={dataTestid}
  data-autofocus={dataAutofocus || undefined}
  aria-label={ariaLabel}
  aria-describedby={ariaDescribedby}
  {placeholder}
  {disabled}
  {required}
  {autofocus}
  {min}
  {max}
  {step}
  class={allClasses}
  style="{variantStyle} {style}"
  {oninput}
  {onchange}
  {onfocus}
  {onblur}
  {onclick}
  {onkeydown}
  {onkeyup}
/>
{/if}
