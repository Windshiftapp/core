<script>
  import Label from './Label.svelte';
  import HelperText from './HelperText.svelte';

  let {
    label = '',
    required = false,
    helper = '',
    error = '',
    id = null,
    children,
    class: className = ''
  } = $props();
</script>

<div class="mb-4 {className}">
  {#if label}
    <Label for={id} {required} color="default" class="mb-1.5">
      {label}
    </Label>
  {/if}
  {@render children()}
  {#if helper && !error}
    <HelperText>{helper}</HelperText>
  {/if}
  {#if error}
    <HelperText variant="error">{error}</HelperText>
  {/if}
</div>
