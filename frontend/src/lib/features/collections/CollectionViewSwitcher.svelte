<script>
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { SquareKanban, Inbox, Settings, Globe } from 'lucide-svelte';
  import { backlogStore } from '../../stores/index.js';

  // Props
  let {
    workspaceId,
    collectionId = null,
    activeView = 'board',
    publicSlug = null,
  } = $props();

  // Styles use --ctx-* CSS vars cascaded from parent collection view
  const containerStyle = 'background-color: var(--ctx-surface, var(--ds-background-neutral)); backdrop-filter: var(--ctx-backdrop, none);';
  const activeButtonStyle = 'color: var(--ds-text); background-color: var(--ctx-surface-raised, var(--ds-surface-raised));';
  const inactiveButtonStyle = 'color: var(--ds-text);';
  const hoverBgStyle = 'var(--ctx-surface, var(--ds-background-neutral-hovered))';

  // Navigation functions
  function goToBoard() {
    const url = workspaceId
      ? (collectionId ? `/workspaces/${workspaceId}/collections/${collectionId}/board` : `/workspaces/${workspaceId}/board`)
      : `/collections/${collectionId}/board`;
    navigate(url);
  }

  function goToBacklog() {
    const url = workspaceId
      ? (collectionId ? `/workspaces/${workspaceId}/collections/${collectionId}/backlog` : `/workspaces/${workspaceId}/backlog`)
      : `/collections/${collectionId}/backlog`;
    navigate(url);
  }

  function goToConfigure() {
    const url = workspaceId
      ? (collectionId ? `/workspaces/${workspaceId}/collections/${collectionId}/board/configure` : `/workspaces/${workspaceId}/board/configure`)
      : `/collections/${collectionId}/board/configure`;
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

  {#if publicSlug}
    <a
      href="/board/{publicSlug}"
      target="_blank"
      rel="noopener noreferrer"
      class="px-3 py-1.5 text-sm font-medium rounded transition-colors"
      style={inactiveButtonStyle}
      title={t('collections.publicBoard')}
      onclick={(e) => { e.stopPropagation(); window.open(`/board/${publicSlug}`, '_blank'); e.preventDefault(); }}
      onmouseenter={(e) => e.currentTarget.style.backgroundColor = hoverBgStyle}
      onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
    >
      <div class="flex items-center gap-2">
        <Globe class="w-4 h-4" />
        {t('collections.publicBoard')}
      </div>
    </a>
  {/if}
</div>
