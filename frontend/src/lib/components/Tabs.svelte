<script>
  let { tabs = [], activeTab = $bindable(''), onTabChange = null, type = 'default', children } = $props();

  // Initialize activeTab to first tab if not set
  $effect(() => {
    if (!activeTab && tabs.length > 0) {
      activeTab = tabs[0].id;
    }
  });

  function switchTab(tabId) {
    activeTab = tabId;
    if (onTabChange) {
      onTabChange({ tab: tabId });
    }
  }
</script>

<div class="rounded-lg border" style="background: var(--ds-surface-raised); border-color: var(--ds-border);">
  <!-- Tab Navigation -->
  <div class="tabs-list flex border-b" style="border-color: var(--ds-border);">
    {#each tabs as tab}
      {#if tab.href}
        <a
          href={tab.href}
          data-testid={tab.testid}
          class="tab-trigger flex flex-none items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative border-b-2 no-underline whitespace-nowrap {tab.className || ''}"
          style="color: {activeTab === tab.id ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}; border-bottom-color: {activeTab === tab.id ? 'var(--ds-interactive)' : 'transparent'}; {activeTab === tab.id ? 'margin-bottom: -1px;' : ''}"
          onclick={(e) => {
            if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
            switchTab(tab.id);
          }}
          onmouseenter={(e) => { if (activeTab !== tab.id) /** @type {HTMLElement} */ (e.currentTarget).style.color = 'var(--ds-text)'; }}
          onmouseleave={(e) => { if (activeTab !== tab.id) /** @type {HTMLElement} */ (e.currentTarget).style.color = 'var(--ds-text-subtle)'; }}
        >
          {#if tab.icon}
            <tab.icon class="w-4 h-4 flex-shrink-0" />
          {/if}
          {tab.label}
          {#if tab.badge}
            <span style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);" class="text-xs px-2 py-0.5 rounded-full">{tab.badge}</span>
          {/if}
        </a>
      {:else}
        <button
          type="button"
          data-testid={tab.testid}
          class="tab-trigger flex flex-none items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative border-b-2 whitespace-nowrap {tab.className || ''}"
          style="color: {activeTab === tab.id ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}; border-bottom-color: {activeTab === tab.id ? 'var(--ds-interactive)' : 'transparent'}; {activeTab === tab.id ? 'margin-bottom: -1px;' : ''}"
          onclick={() => switchTab(tab.id)}
          onmouseenter={(e) => { if (activeTab !== tab.id) /** @type {HTMLElement} */ (e.currentTarget).style.color = 'var(--ds-text)'; }}
          onmouseleave={(e) => { if (activeTab !== tab.id) /** @type {HTMLElement} */ (e.currentTarget).style.color = 'var(--ds-text-subtle)'; }}
        >
          {#if tab.icon}
            <tab.icon class="w-4 h-4 flex-shrink-0" />
          {/if}
          {tab.label}
          {#if tab.badge}
            <span style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);" class="text-xs px-2 py-0.5 rounded-full">{tab.badge}</span>
          {/if}
        </button>
      {/if}
    {/each}
  </div>

  <!-- Tab Content -->
  <div class="p-6">
    {@render children?.()}
  </div>
</div>

<style>
  .tabs-list {
    overflow-x: auto;
    overscroll-behavior-inline: contain;
    scrollbar-width: thin;
  }
</style>
