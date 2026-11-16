<script>
  import Label from './Label.svelte';
  import { setContext } from 'svelte';
  import { FIELD_CONTEXT, descriptionIds } from '../utils/fieldContext.svelte.js';
  import { cn } from '../utils/cn.js';

  let {
    label = '',
    required = false,
    helper = '',
    error = '',
    id = null,
    labelSnippet = null,
    labelColor = 'default',
    labelSize = 'sm',
    labelClass = 'mb-1.5',
    labelTestid = undefined,
    requiredMark = false,
    ariaDescribedby = undefined,
    /** Keeps the label in the layout (as a spacer) without showing it. */
    labelHidden = false,
    children,
    class: className = ''
  } = $props();
  const instanceId = $props.id();
  const fieldId = $derived(id ?? `${instanceId}-control`);
  let explicitControlId = $state(undefined);
  let registeredLabelId = $state(undefined);
  const controlId = $derived(explicitControlId ?? fieldId);
  const helperId = `${instanceId}-helper`;
  const errorId = `${instanceId}-error`;
  const labelId = `${instanceId}-label`;
  const field = {
    get id() { return fieldId; },
    get controlId() { return controlId; },
    get labelId() { return label || labelSnippet ? labelId : registeredLabelId; },
    get describedby() { return descriptionIds(ariaDescribedby, error ? errorId : helper ? helperId : undefined); },
    get invalid() { return Boolean(error); },
    get required() { return required; },
    get ariaRequired() { return required || requiredMark; },
    registerLabel(value) {
      registeredLabelId = value;
      return () => { registeredLabelId = undefined; };
    },
    registerId(value) {
      explicitControlId = value;
      return () => { explicitControlId = undefined; };
    },
  };
  setContext(FIELD_CONTEXT, field);
</script>

<div class={cn('mb-4', className)}>
  {#if label || labelSnippet}
    <Label id={labelId} for={controlId} dataTestid={labelTestid} required={required || requiredMark} color={labelColor} size={labelSize} class="{labelClass} {labelHidden ? 'invisible' : ''}">
      {#if labelSnippet}{@render labelSnippet()}{:else}{label}{/if}
    </Label>
  {/if}
  {@render children(field)}
  {#if helper && !error}
    <p id={helperId} class="ds-text-helper text-ds-text-subtle mt-1">{helper}</p>
  {/if}
  {#if error}
    <p id={errorId} role="alert" class="ds-text-helper text-ds-text-danger mt-1">{error}</p>
  {/if}
</div>
