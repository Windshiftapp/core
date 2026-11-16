<script>
  import FormField from './FormField.svelte';
  import Select from './Select.svelte';

  /**
   * @type {{
    *   label?: string,
    *   helper?: string,
    *   error?: string,
    *   ariaDescribedby?: string,
    *   ariaInvalid?: boolean,
    *   dataTestid?: string,
   *   value?: any,
   *   options?: Array<any>,
   *   placeholder?: string,
   *   required?: boolean,
   *   disabled?: boolean,
   *   id?: string,
   *   size?: string,
   *   labelColor?: 'default' | 'subtle',
   *   labelClass?: string,
   *   ariaLabel?: string,
   *   onchange?: (e?: any) => void,
   *   class?: string,
   * }}
   */
  let {
    label = '',
    helper = '',
    error = '',
    ariaDescribedby = undefined,
    ariaInvalid = undefined,
    dataTestid = undefined,
    value = $bindable(''),
    options = [],
    placeholder = '',
    required = false,
    disabled = false,
    id = undefined,
    size = 'medium',
    labelColor = 'subtle',
    labelClass = 'mb-2',
    ariaLabel = undefined,
    onchange = undefined,
    class: className = ''
  } = $props();
</script>

<FormField {label} {id} {required} {helper} {error} {labelColor} {labelClass} labelTestid={dataTestid ? `${dataTestid}-label` : undefined} class="mb-0 {className}">
  <Select
    {id}
    {ariaDescribedby}
    {ariaInvalid}
    {dataTestid}
    bind:value
    {options}
    {placeholder}
    {required}
    {disabled}
    {size}
    {ariaLabel}
    {onchange}
  />
</FormField>
