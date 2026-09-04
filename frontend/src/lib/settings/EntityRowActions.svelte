<script>
  import Spinner from '../components/Spinner.svelte';

  let { actions = [] } = $props();
</script>

<div class="flex items-center justify-end gap-1">
  {#each actions as action (action.id)}
    {@const Icon = action.icon}
    <button
      class="p-1.5 rounded hover:opacity-80"
      style="color: {action.danger ? 'var(--ds-text-danger)' : 'var(--ds-text-subtle)'};"
      title={action.title}
      data-testid={action.testId}
      disabled={action.disabled || action.loading}
      onclick={action.onclick}
    >
      {#if action.loading}
        <Spinner size="sm" />
      {:else}
        <Icon size={14} />
      {/if}
    </button>
  {/each}
</div>
