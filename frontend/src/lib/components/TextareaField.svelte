<script>
  import FormField from './FormField.svelte';
  import Textarea from './Textarea.svelte';

  /**
   * @type {{
    *   label?: string,
    *   helper?: string,
    *   error?: string,
    *   ariaDescribedby?: string,
    *   ariaInvalid?: boolean,
   *   value?: any,
   *   placeholder?: string,
   *   required?: boolean,
   *   disabled?: boolean,
   *   autofocus?: boolean,
   *   spellcheck?: boolean,
   *   rows?: number,
   *   id?: string,
   *   size?: string,
   *   labelColor?: 'default' | 'subtle',
   *   labelClass?: string,
   *   textareaRef?: any,
   *   dataTestid?: string,
   *   oninput?: (e?: any) => void,
   *   class?: string,
   * }}}
   */
  let {
    label = '',
    helper = '',
    error = '',
    ariaDescribedby = undefined,
    ariaInvalid = undefined,
    value = $bindable(''),
    placeholder = '',
    required = false,
    disabled = false,
    autofocus = false,
    spellcheck = undefined,
    rows = 3,
    id = undefined,
    size = 'medium',
    labelColor = 'subtle',
    labelClass = 'mb-2',
    textareaRef = $bindable(null),
    dataTestid = undefined,
    oninput = undefined,
    class: className = ''
  } = $props();
</script>

<FormField {label} {id} {required} {helper} {error} {labelColor} {labelClass} labelTestid={dataTestid ? `${dataTestid}-label` : undefined} class="mb-0 {className}">
  <Textarea
    {id}
    {ariaDescribedby}
    {ariaInvalid}
    bind:value
    bind:textareaRef
    {placeholder}
    {required}
    {disabled}
    {autofocus}
    {spellcheck}
    {rows}
    {size}
    data-testid={dataTestid}
    {oninput}
  />
</FormField>
