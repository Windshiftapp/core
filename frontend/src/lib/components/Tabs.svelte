<script>
  import { moveTabFocus } from '../utils/tabKeyboard.js';
  let { tabs = [], activeTab = $bindable(''), onTabChange = null, type = 'default', children } = $props();
  const instanceId = $props.id();
  const navigation = $derived(tabs.some(tab => tab.href));

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
  <div role={navigation ? 'navigation' : 'tablist'} class="tabs-list flex border-b" style="border-color: var(--ds-border);">
    {#each tabs as tab}
      {#if tab.href}
        <a
          href={tab.href}
          aria-current={activeTab === tab.id ? 'page' : undefined}
          data-testid={tab.testid}
          class="ds-focus-ring ds-tab-focus tab-trigger flex flex-none items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative border-b-2 no-underline {tab.className || ''}"
          class:active={activeTab === tab.id}
          onclick={(e) => {
            if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
            switchTab(tab.id);
          }}
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
          role={navigation ? undefined : 'tab'}
          id={`${instanceId}-tab-${tab.id}`}
          data-tab-id={tab.id}
          aria-selected={navigation ? undefined : activeTab === tab.id}
          aria-controls={navigation ? undefined : `${instanceId}-panel`}
          tabindex={navigation || activeTab === tab.id ? 0 : -1}
          data-testid={tab.testid}
          class="ds-focus-ring ds-tab-focus tab-trigger flex flex-none items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative border-b-2 {tab.className || ''}"
          class:active={activeTab === tab.id}
          onclick={() => switchTab(tab.id)}
          onkeydown={event => { if (!navigation) moveTabFocus(event, tabs, tab.id, switchTab); }}
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
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (Only local tabpanels receive tabindex.) -->
  <div id={navigation ? undefined : `${instanceId}-panel`} role={navigation ? undefined : 'tabpanel'} aria-labelledby={navigation ? undefined : `${instanceId}-tab-${activeTab}`} tabindex={navigation ? undefined : 0} class="p-6">
    {@render children?.()}
  </div>
</div>

<style>
  .tabs-list {
    overflow-x: auto;
    overscroll-behavior-inline: contain;
    scrollbar-width: thin;
  }

  .tab-trigger {
    max-width: 100%;
    white-space: normal;
    overflow-wrap: anywhere;
    color: var(--ds-text-subtle);
    border-bottom-color: transparent;
  }

  .tab-trigger:hover:not(.active) {
    color: var(--ds-text);
  }

  .tab-trigger.active {
    color: var(--ds-text-link);
    border-bottom-color: var(--ds-interactive);
    margin-bottom: -1px;
  }
</style>
