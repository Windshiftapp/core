<script>
  import Label from './Label.svelte';
  import Input from './Input.svelte';

  let {
    label = '',
    value = $bindable(''),
    type = 'text',
    placeholder = '',
    required = false,
    disabled = false,
    autofocus = false,
    id = undefined,
    size = 'medium',
    min = undefined,
    max = undefined,
    step = undefined,
    class: className = '',
    inputRef = $bindable(null)
  } = $props();
</script>

<div class={className}>
  {#if label}
    <Label for={id} {required} class="mb-2">{label}</Label>
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
    {min}
    {max}
    {step}
  />
</div>
