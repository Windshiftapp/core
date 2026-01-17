<script>
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { SquareKanban, Inbox, Settings } from 'lucide-svelte';
  import { backlogStore } from '../../stores/index.js';

  // Props
  let {
    workspaceId,
    collectionId = null,
    activeView = 'board',
    hasGradient = false
  } = $props();

  // Computed styles
  let containerStyle = $derived(hasGradient
    ? 'background-color: var(--ds-glass-bg); backdrop-filter: blur(12px);'
    : 'background-color: var(--ds-background-neutral);');

  let activeButtonStyle = $derived(hasGradient
    ? 'color: var(--ds-text); background-color: var(--ds-glass-bg);'
    : 'color: var(--ds-text); background-color: var(--ds-surface-raised);');

  let inactiveButtonStyle = $derived('color: var(--ds-text);');

  let hoverBgStyle = $derived(hasGradient
    ? 'var(--ds-glass-bg)'
    : 'var(--ds-background-neutral-hovered)');

  // Navigation functions
  function goToBoard() {
    const url = collectionId
      ? `/workspaces/${workspaceId}/collections/${collectionId}/board`
      : `/workspaces/${workspaceId}/board`;
    navigate(url);
  }

  function goToBacklog() {
    const url = collectionId
      ? `/workspaces/${workspaceId}/collections/${collectionId}/backlog`
      : `/workspaces/${workspaceId}/backlog`;
    navigate(url);
  }

  function goToConfigure() {
    const url = collectionId
      ? `/workspaces/${workspaceId}/collections/${collectionId}/board/configure`
      : `/workspaces/${workspaceId}/board/configure`;
    navigate(url);
  }
</script>

<div class="flex rounded p-1" style={containerStyle}>
  <!-- Board Button -->
  <button
    class="px-3 py-1.5 text-sm font-medium rounded transition-colors"
    class:shadow-sm={activeView === 'board'}
    style={activeView === 'board' ? activeButtonStyle : inactiveButtonStyle}
    onmouseenter={(e) => activeView !== 'board' && (e.currentTarget.style.backgroundColor = hoverBgStyle)}
    onmouseleave={(e) => activeView !== 'board' && (e.currentTarget.style.backgroundColor = '')}
    onclick={activeView !== 'board' ? goToBoard : undefined}
  >
    <div class="flex items-center gap-2">
      <SquareKanban class="w-4 h-4" />
      {t('collections.board')}
    </div>
  </button>

  <!-- Backlog Button -->
  <button
    class="px-3 py-1.5 text-sm font-medium rounded transition-colors"
    class:shadow-sm={activeView === 'backlog'}
    style={activeView === 'backlog' ? activeButtonStyle : inactiveButtonStyle}
    onmouseenter={(e) => activeView !== 'backlog' && (e.currentTarget.style.backgroundColor = hoverBgStyle)}
    onmouseleave={(e) => activeView !== 'backlog' && (e.currentTarget.style.backgroundColor = '')}
    onclick={activeView !== 'backlog' ? goToBacklog : undefined}
  >
    <div class="flex items-center gap-2">
      <Inbox class="w-4 h-4" />
      {t('collections.backlog')}
      {#if backlogStore.count > 0}
        <span class="px-1.5 py-0.5 rounded-full text-xs" style="background-color: var(--ds-accent-blue-subtle); color: var(--ds-text-info);">
          {backlogStore.count}
        </span>
      {/if}
    </div>
  </button>

  <!-- Configure Button -->
  <button
    class="px-3 py-1.5 text-sm font-medium rounded transition-colors"
    class:shadow-sm={activeView === 'configure'}
    style={activeView === 'configure' ? activeButtonStyle : inactiveButtonStyle}
    onmouseenter={(e) => activeView !== 'configure' && (e.currentTarget.style.backgroundColor = hoverBgStyle)}
    onmouseleave={(e) => activeView !== 'configure' && (e.currentTarget.style.backgroundColor = '')}
    onclick={activeView !== 'configure' ? goToConfigure : undefined}
  >
    <div class="flex items-center gap-2">
      <Settings class="w-4 h-4" />
      {t('collections.configure')}
    </div>
  </button>
</div>
