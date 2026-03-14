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

<div class="rounded-xl border shadow-sm" style="background: var(--ds-surface-raised); border-color: var(--ds-border);">
  <!-- Tab Navigation -->
  <div class="flex border-b" style="border-color: var(--ds-border);">
    {#each tabs as tab}
      <button
        class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative border-b-2 {tab.className || ''}"
        style="color: {activeTab === tab.id ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}; border-bottom-color: {activeTab === tab.id ? 'var(--ds-interactive)' : 'transparent'}; {activeTab === tab.id ? 'margin-bottom: -1px;' : ''}"
        onclick={() => switchTab(tab.id)}
        onmouseenter={(e) => { if (activeTab !== tab.id) e.target.style.color = 'var(--ds-text)'; }}
        onmouseleave={(e) => { if (activeTab !== tab.id) e.target.style.color = 'var(--ds-text-subtle)'; }}
      >
        {#if tab.icon}
          <tab.icon class="w-4 h-4" />
        {/if}
        {tab.label}
        {#if tab.badge}
          <span style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);" class="text-xs px-2 py-0.5 rounded-full">{tab.badge}</span>
        {/if}
      </button>
    {/each}
  </div>

  <!-- Tab Content -->
  <div class="p-6">
    {@render children?.()}
  </div>
</div>
