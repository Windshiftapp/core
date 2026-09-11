<script>
  let {
    locked = false,
    lockedClass = 'w-8 h-8 flex items-center justify-center flex-shrink-0',
    lockedColor = 'var(--ds-text-subtlest)',
    removeClass = 'p-1 rounded flex-shrink-0',
    removeStyle = '',
    removeTitle,
    onremove = () => {}
  } = $props();
</script>

{#if locked}
  <div class={lockedClass}>
    <svg class="w-4 h-4" style:color={lockedColor} fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
    </svg>
  </div>
{:else}
  <button onclick={(event) => onremove(event)} class={removeClass} style={removeStyle} title={removeTitle}>
    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
    </svg>
  </button>
{/if}
