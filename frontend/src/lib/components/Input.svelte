<script>
  import { cn } from '../utils/cn.js';

  /**
   * @type {{
   *   type?: string,
   *   value?: any,
   *   placeholder?: string,
   *   disabled?: boolean,
   *   required?: boolean,
   *   autofocus?: boolean,
   *   size?: string,
   *   min?: any,
   *   max?: any,
   *   step?: any,
   *   id?: string,
   *   name?: string,
   *   pattern?: string,
   *   title?: string,
   *   autocomplete?: string,
   *   readonly?: boolean,
   *   maxlength?: number,
   *   class?: string,
   *   inputRef?: any,
   *   oninput?: (e?: any) => void,
   *   onchange?: (e?: any) => void,
   *   onfocus?: (e?: any) => void,
   *   onblur?: (e?: any) => void,
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
    size = 'medium',
    min = undefined,
    max = undefined,
    step = undefined,
    id = undefined,
    name = undefined,
    pattern = undefined,
    title = undefined,
    autocomplete = undefined,
    readonly = false,
    maxlength = undefined,
    class: className = '',
    // Optional ref binding for parent components that need the raw input element
    inputRef = $bindable(null),
    // Event handlers
    oninput = undefined,
    onchange = undefined,
    onfocus = undefined,
    onblur = undefined,
    onkeydown = undefined,
    onkeyup = undefined
  } = $props();
  export { className as class };

  // Size variants
  const sizeClasses = $derived({
    small: 'px-3 py-1.5 text-sm',
    medium: 'px-4 py-3'
  }[size] || 'px-4 py-3');

  // Combine all classes
  const allClasses = $derived(cn(
    'w-full rounded border transition-all duration-200',
    'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50',
    sizeClasses,
    className
  ));
  /** @type {any} */
  const autocompleteValue = autocomplete;
</script>

<!-- svelte-ignore a11y_autofocus -->
<input
  {type}
  {id}
  {name}
  {pattern}
  {title}
  autocomplete={autocompleteValue}
  {readonly}
  {maxlength}
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
  {oninput}
  {onchange}
  {onfocus}
  {onblur}
  {onkeydown}
  {onkeyup}
/>
