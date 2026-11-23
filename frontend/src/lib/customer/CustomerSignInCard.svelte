<script>
  import Button from '../components/Button.svelte';
  // Shared sign-in entry for portals and boards. The caller supplies the copy
  // and owns the modal; this is the single visible affordance that opens it.
  let {
    title,
    description,
    actionLabel,
    hint = null,
    id = 'portal-sign-in',
    testid = 'customer-sign-in-card',
    actionTestid = 'customer-sign-in',
    onaction = null,
  } = $props();
</script>

<div
  class="rounded-lg border border-ds-border bg-ds-surface-card p-6 sm:p-8"
  data-testid={testid}
>
  <h2 class="mb-2 text-lg font-semibold text-ds-text">{title}</h2>
  {#if description}
    <p class="mb-6 text-sm leading-relaxed text-ds-text-subtle">{description}</p>
  {/if}
  <Button {id} variant="primary" fullWidth={true} onclick={onaction} dataTestid={actionTestid}>
    {actionLabel}
  </Button>
  {#if hint}
    <p class="mt-4 text-xs text-ds-text-subtle">{hint}</p>
  {/if}
</div>
