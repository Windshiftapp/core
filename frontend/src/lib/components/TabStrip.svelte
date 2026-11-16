<script>
  import Badge from './Badge.svelte';
  import { moveTabFocus } from '../utils/tabKeyboard.js';

  /**
   * TabStrip - Underline tab bar for local (non-URL) tab state.
   *
   * Use TabNav when the active tab lives in the URL query, Tabs for a full
   * bordered panel, and TabStrip for a bare inline tab bar.
   *
   * @example
   * <TabStrip
   *   tabs={[
   *     { id: 'managers', label: t('time.permissions.managers'), count: managers.length },
   *     { id: 'members', label: t('time.permissions.members'), count: members.length },
   *   ]}
   *   bind:activeTab
   *   onTabChange={resetAddForm}
   * />
   */
  /**
   * @type {{
   *   tabs?: Array<{
   *     id: string,
   *     label: string,
   *     count?: number,
   *     countVariant?: 'neutral' | 'info' | 'success' | 'warning' | 'danger',
   *     icon?: import('svelte').Component<any, any, any> | null,
    *     testid?: string,
    *     disabled?: boolean,
   *   }>,
   *   activeTab?: string,
   *   onTabChange?: ((tab: string) => void) | null,
   *   ariaLabel?: string,
   *   class?: string,
   *   fill?: boolean,
    *   trailing?: import('svelte').Snippet | undefined,
    *   children?: import('svelte').Snippet | undefined,
    *   panelId?: string,
    *   panelElement?: HTMLElement | null,
   * }}
   */
  let {
    tabs = [],
    activeTab = $bindable(''),
    onTabChange = null,
    ariaLabel = '',
    class: className = '',
    fill = false,
    trailing = undefined,
    children = undefined,
    panelId = undefined,
    panelElement = null
  } = $props();
  const instanceId = $props.id();
  const contentId = $derived(panelId ?? (children || panelElement ? `${instanceId}-panel` : undefined));
  const selectedId = $derived(tabs.some(tab => tab.id === activeTab && !tab.disabled) ? activeTab : tabs.find(tab => !tab.disabled)?.id);
  $effect(() => { if (selectedId && activeTab !== selectedId) activeTab = selectedId; });
  // External panels keep their layout/retained state; this bar owns only their
  // accessible relationship. Prefer the children snippet for new widgets.
  $effect(() => {
    if (!panelElement) return;
    const attributes = { id: contentId, role: 'tabpanel', 'aria-labelledby': `${instanceId}-tab-${selectedId}`, tabindex: '0' };
    const original = Object.fromEntries(Object.keys(attributes).map(name => [name, panelElement.getAttribute(name)]));
    const panel = panelElement;
    for (const [name, value] of Object.entries(attributes)) panel.setAttribute(name, value);
    return () => {
      for (const [name, value] of Object.entries(original)) {
        if (value === null) panel.removeAttribute(name);
        else panel.setAttribute(name, value);
      }
    };
  });

  function select(id) {
    if (id === activeTab) return;
    activeTab = id;
    onTabChange?.(id);
  }
</script>

<div class="flex border-b {className}" style="border-color: var(--ds-border);">
  <div class="-mb-px flex flex-1 min-w-0 overflow-x-auto" role="tablist" aria-label={ariaLabel}>
    {#each tabs as tab (tab.id)}
      <button
        type="button"
        role="tab"
        id={`${instanceId}-tab-${tab.id}`}
        data-tab-id={tab.id}
        aria-controls={contentId}
        tabindex={selectedId === tab.id ? 0 : -1}
        disabled={tab.disabled}
        data-testid={tab.testid}
        aria-selected={activeTab === tab.id}
        class="ds-focus-ring ds-tab-focus max-w-full shrink-0 px-4 py-2 text-sm font-medium flex items-center gap-2 border-b-2 transition-colors whitespace-normal break-words {fill
          ? 'flex-1 justify-center'
          : ''}"
        class:border-transparent={activeTab !== tab.id}
        style="{activeTab === tab.id
          ? 'border-color: var(--ds-interactive); color: var(--ds-text-link);'
          : 'color: var(--ds-text-subtle);'}"
        onclick={() => select(tab.id)}
        onkeydown={event => moveTabFocus(event, tabs, tab.id, select)}
      >
        {#if tab.icon}
          {@const Icon = tab.icon}
          <Icon size={16} />
        {/if}
        {tab.label}
        {#if tab.count}
          <Badge size="xs" variant={tab.countVariant || 'neutral'}>{tab.count}</Badge>
        {/if}
      </button>
    {/each}
  </div>
  {#if trailing}
    <div class="flex flex-shrink-0 items-center gap-3 pl-4">
      {@render trailing()}
    </div>
  {/if}
</div>
{#if children}
  <div id={contentId} role="tabpanel" aria-labelledby={`${instanceId}-tab-${selectedId}`} tabindex="0">
    {@render children()}
  </div>
{/if}
