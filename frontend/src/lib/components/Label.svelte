<script>
  import { getContext } from 'svelte';
  import { FIELD_CONTEXT } from '../utils/fieldContext.svelte.js';
  const field = getContext(FIELD_CONTEXT);
  const instanceId = $props.id();
  let {
    id = undefined,
    for: forId = null,
    required = false,
    size = 'sm',           // 'xs' | 'sm' | 'md'
    color = 'subtle',      // 'default' | 'subtle'
    dataTestid = undefined,
    class: className = '',
    children
  } = $props();
  const labelId = $derived(id ?? `${instanceId}-label`);
  const controlId = $derived(forId ?? field?.controlId);
  $effect(() => field?.registerLabel(labelId));

  function focusCustomControl(event) {
    if (event.target.closest('a, button, input, textarea, select')) return;
    const control = controlId ? document.getElementById(controlId) : null;
    if (!control || (control.matches('button, input, select, textarea') && control.getAttribute('role') !== 'combobox') || control.getAttribute('aria-disabled') === 'true') return;
    event.preventDefault();
    control.focus();
  }

  const sizeClass = $derived({
    xs: 'text-xs',
    sm: 'text-sm',
    md: 'text-base'
  }[size] || 'text-sm');

  const colorToken = $derived(color === 'subtle' ? '--ds-text-subtle' : '--ds-text');
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions (Labels delegate focus; controls own keyboard interaction.) -->
<label
  id={labelId}
  for={controlId}
  onclick={focusCustomControl}
  data-testid={dataTestid}
  class="block font-medium {sizeClass} {className}"
  style="color: var({colorToken});"
>
  {@render children?.()}
  {#if required}
    <span aria-hidden="true" class="text-ds-text-danger ml-0.5">*</span>
  {/if}
</label>
