<script>
  import Label from './Label.svelte';
  import Input from './Input.svelte';

  /**
   * @type {{
   *   label?: string,
   *   labelSnippet?: import('svelte').Snippet | null,
   *   value?: any,
   *   type?: string,
   *   placeholder?: string,
   *   required?: boolean,
   *   requiredMark?: boolean,
   *   disabled?: boolean,
   *   autofocus?: boolean,
   *   id?: string,
   *   size?: string,
   *   min?: any,
   *   max?: any,
   *   step?: any,
   *   name?: string,
   *   autocomplete?: string,
   *   pattern?: string,
   *   readonly?: boolean,
   *   minlength?: number,
   *   maxlength?: number,
   *   title?: string,
   *   labelColor?: 'default' | 'subtle',
   *   labelClass?: string,
   *   labelSize?: 'xs' | 'sm' | 'md',
   *   inputClass?: string,
   *   dataTestid?: string,
   *   inputRef?: any,
   *   oninput?: (e?: any) => void,
   *   onchange?: (e?: any) => void,
   *   onfocus?: (e?: any) => void,
   *   onblur?: (e?: any) => void,
   *   onkeydown?: (e?: any) => void,
   *   onkeyup?: (e?: any) => void,
   *   class?: string,
   * }}
   */
  let {
    label = '',
    // Renders rich label content (icons, helper spans) in place of `label`.
    labelSnippet = null,
    value = $bindable(''),
    type = 'text',
    placeholder = '',
    required = false,
    // Shows the required asterisk without setting the input's `required`
    // attribute, for fields the form validates itself.
    requiredMark = false,
    disabled = false,
    autofocus = false,
    id = undefined,
    size = 'medium',
    min = undefined,
    max = undefined,
    step = undefined,
    name = undefined,
    autocomplete = undefined,
    pattern = undefined,
    readonly = false,
    minlength = undefined,
    maxlength = undefined,
    title = undefined,
    labelColor = 'subtle', // 'default' | 'subtle'
    labelClass = 'mb-2',
    labelSize = 'sm',
    inputClass = '',
    dataTestid = undefined,
    inputRef = $bindable(null),
    oninput = undefined,
    onchange = undefined,
    onfocus = undefined,
    onblur = undefined,
    onkeydown = undefined,
    onkeyup = undefined,
    class: className = ''
  } = $props();
</script>

<div class={className}>
  {#if labelSnippet}
    <Label for={id} required={required || requiredMark} color={labelColor} size={labelSize} class={labelClass}>
      {@render labelSnippet()}
    </Label>
  {:else if label}
    <Label for={id} required={required || requiredMark} color={labelColor} size={labelSize} class={labelClass}>{label}</Label>
  {/if}
  <Input
    {type}
    {id}
    bind:value
    bind:inputRef
    {placeholder}
    {required}
    {disabled}
    {autofocus}
    {size}
    class={inputClass}
    {min}
    {max}
    {step}
    {name}
    {autocomplete}
    {pattern}
    {readonly}
    {minlength}
    {maxlength}
    {title}
    {dataTestid}
    {oninput}
    {onchange}
    {onfocus}
    {onblur}
    {onkeydown}
    {onkeyup}
  />
</div>
