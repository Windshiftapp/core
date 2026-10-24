<script>
  // Divider takes no props; the binding keeps the component type aligned with
  // the empty zod schema used to define it.
  /** @type {import('@openuidev/svelte-lang').ComponentRenderProps<Record<string, never>>} */
  let { props: _props } = $props();
</script>

<div class="my-2 h-px w-full" style="background-color: var(--ds-border);" data-testid="openui-divider"></div>
