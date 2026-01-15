<script>
  import { currentRoute, navigate } from '../router.js';

  let {
    tabs = [],        // Array of { id, label }
    basePath = '',    // e.g., '/tests/reports' or '/admin/statuses'
    paramName = 'subtab',  // Query param name
    defaultTab = ''   // Fallback tab ID
  } = $props();

  const activeTab = $derived($currentRoute.query?.[paramName] || defaultTab || tabs[0]?.id);

  function switchTab(tabId) {
    navigate(`${basePath}?${paramName}=${tabId}`);
  }
</script>

<div class="border-b" style="border-color: var(--ds-border);">
  <nav class="-mb-px flex space-x-8" aria-label="Tabs">
    {#each tabs as tab}
      <button
        onclick={() => switchTab(tab.id)}
        class="whitespace-nowrap py-4 px-1 border-b-2 font-medium text-sm transition-colors"
        style="{activeTab === tab.id
          ? 'border-color: var(--ds-interactive); color: var(--ds-interactive);'
          : 'border-color: transparent; color: var(--ds-text-subtle);'}"
        aria-current={activeTab === tab.id ? 'page' : undefined}
      >
        {tab.label}
      </button>
    {/each}
  </nav>
</div>
