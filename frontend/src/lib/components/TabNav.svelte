<script>
  import { currentRoute, navigate } from '../router.js';
  import Link from './Link.svelte';
  import { toExternal } from '../runtime/contextPath.js';

  let {
    tabs = [],        // Array of { id, label }
    basePath = '',    // e.g., '/tests/reports' or '/admin/statuses'
    paramName = 'subtab',  // Query param name
    defaultTab = '',   // Fallback tab ID
    testidPrefix = 'tab-nav',
    ariaLabel = 'Tabs'
  } = $props();

  const activeTab = $derived($currentRoute.query?.[paramName] || defaultTab || tabs[0]?.id);

  function tabPath(tabId) {
    const query = new URLSearchParams($currentRoute.query);
    query.set(paramName, tabId);
    return `${basePath}?${query}`;
  }

  function isActive(tab) {
    return activeTab === tab.id || tab.matches?.includes(activeTab);
  }
</script>

<div class="border-b" style="border-color: var(--ds-border);">
  <nav class="-mb-px flex gap-x-8 overflow-x-auto p-1" aria-label={ariaLabel}>
    {#each tabs as tab}
      <Link
        href={toExternal(tabPath(tab.id))}
        onClick={event => { event.preventDefault(); navigate(tabPath(tab.id)); }}
        data-testid={tab.testid || `${testidPrefix}-${tab.id}`}
        active={isActive(tab)}
        class="ds-focus-ring ds-tab-focus max-w-full flex-none whitespace-normal break-words py-4 px-1 border-b-2 font-medium text-sm transition-colors"
        style={isActive(tab)
          ? 'border-color: var(--ds-interactive); color: var(--ds-text-link);'
          : 'border-color: transparent; color: var(--ds-text-subtle);'}
      >
        {tab.label}
      </Link>
    {/each}
  </nav>
</div>
