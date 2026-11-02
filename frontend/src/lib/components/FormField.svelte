<script>
  import Label from './Label.svelte';
  import Text from './Text.svelte';

  let {
    label = '',
    required = false,
    helper = '',
    error = '',
    id = null,
    /** Keeps the label in the layout (as a spacer) without showing it. */
    labelHidden = false,
    children,
    class: className = ''
  } = $props();
</script>

<div class="mb-4 {className}">
  {#if label}
    <Label for={id} {required} color="default" class="mb-1.5 {labelHidden ? 'invisible' : ''}">
      {label}
    </Label>
  {/if}
  {@render children()}
  {#if helper && !error}
    <Text as="p" variant="subtle" size="xs" class="mt-1">{helper}</Text>
  {/if}
  {#if error}
    <Text as="p" variant="danger" size="xs" class="mt-1">{error}</Text>
  {/if}
</div>
