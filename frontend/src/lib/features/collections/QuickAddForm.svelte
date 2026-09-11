<script>
  import { Plus, X, Package, ChevronDown, LoaderCircle } from '@lucide/svelte';
  import { useEventListener } from 'runed';
  import Button from '../../components/Button.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import { portal } from '../../actions/portal.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { itemTypeIconMap, workspaceIconMap } from '../../utils/icons.js';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import {
    getDisplayString,
    getShortcut,
    matchesShortcut,
  } from '../../utils/keyboardShortcuts.js';
  const iconMap = { ...workspaceIconMap, ...itemTypeIconMap };
  const createShortcut = getShortcut('quickAdd', 'create');
  const cancelShortcut = getShortcut('quickAdd', 'cancel');

  let {
    parentId,
    formState,
    workspaces = [],
    compact = false,
    cardBgStyle = '',
    onUpdateField = () => {},
    onCreate = () => {},
    onCancel = () => {}
  } = $props();

  let selectedWorkspace = $derived(workspaces.find(w => w.id === formState.workspaceId));
  let selectedItemType = $derived(formState.availableTypes?.find(it => it.id === formState.itemTypeId));

  // Dropdown management. Both menus are portalled to <body> and positioned
  // against their trigger: the board clips swimlanes with overflow-hidden, so
  // an absolutely-positioned menu is cut off at the lane boundary.
  let showWorkspaceDropdown = $state(false);
  let showItemTypeDropdown = $state(false);
  let workspaceAnchor = $state(null);
  let itemTypeAnchor = $state(null);
  let workspaceMenuStyle = $state('');
  let itemTypeMenuStyle = $state('');

  const MENU_WIDTH = 192; // w-48
  const MENU_MAX_HEIGHT = 240;
  const VIEWPORT_MARGIN = 8;

  function menuPositionStyle(anchor) {
    if (!anchor) return '';
    const rect = anchor.getBoundingClientRect();
    const gap = 4;
    const spaceBelow = window.innerHeight - rect.bottom;
    const openUp = spaceBelow < MENU_MAX_HEIGHT && rect.top > spaceBelow;
    const left = Math.max(
      VIEWPORT_MARGIN,
      Math.min(rect.left, window.innerWidth - MENU_WIDTH - VIEWPORT_MARGIN)
    );
    const vertical = openUp
      ? `bottom: ${window.innerHeight - rect.top + gap}px; max-height: ${Math.min(MENU_MAX_HEIGHT, rect.top - gap - VIEWPORT_MARGIN)}px;`
      : `top: ${rect.bottom + gap}px; max-height: ${Math.min(MENU_MAX_HEIGHT, spaceBelow - gap - VIEWPORT_MARGIN)}px;`;
    return `left: ${left}px; width: ${MENU_WIDTH}px; ${vertical}`;
  }

  function repositionMenus() {
    if (showWorkspaceDropdown) workspaceMenuStyle = menuPositionStyle(workspaceAnchor);
    if (showItemTypeDropdown) itemTypeMenuStyle = menuPositionStyle(itemTypeAnchor);
  }

  function toggleWorkspaceDropdown() {
    showItemTypeDropdown = false;
    showWorkspaceDropdown = !showWorkspaceDropdown;
    if (showWorkspaceDropdown) workspaceMenuStyle = menuPositionStyle(workspaceAnchor);
  }

  function toggleItemTypeDropdown() {
    showWorkspaceDropdown = false;
    showItemTypeDropdown = !showItemTypeDropdown;
    if (showItemTypeDropdown) itemTypeMenuStyle = menuPositionStyle(itemTypeAnchor);
  }

  function closeDropdowns() {
    showWorkspaceDropdown = false;
    showItemTypeDropdown = false;
  }

  // Portalled menus live outside this component's DOM, so dismissal and
  // re-anchoring have to be handled on the window.
  useEventListener(
    () => window,
    'pointerdown',
    (event) => {
      if (!showWorkspaceDropdown && !showItemTypeDropdown) return;
      const target = /** @type {Element | null} */ (event.target);
      if (target?.closest?.('[data-quick-add-menu], [data-quick-add-menu-anchor]')) return;
      closeDropdowns();
    },
    { capture: true }
  );
  useEventListener(() => window, 'scroll', repositionMenus, { capture: true, passive: true });
  useEventListener(() => window, 'resize', repositionMenus);

  function handleKeydown(e) {
    if (matchesShortcut(e, createShortcut)) {
      e.preventDefault();
      onCreate(parentId);
    } else if (matchesShortcut(e, cancelShortcut)) {
      // An open selector swallows the first cancel: dismiss it before the form.
      if (showWorkspaceDropdown || showItemTypeDropdown) {
        closeDropdowns();
        return;
      }
      onCancel(parentId);
    }
  }

  function selectWorkspace(workspaceId) {
    onUpdateField(parentId, 'workspaceId', workspaceId);
    showWorkspaceDropdown = false;
  }

  function selectItemType(itemTypeId) {
    onUpdateField(parentId, 'itemTypeId', itemTypeId);
    showItemTypeDropdown = false;
  }
</script>

<div
  class="relative z-[100] border {compact ? 'rounded shadow-md' : 'rounded-[4px] px-3 py-2.5'}"
  style={cardBgStyle}
>
  <!-- Textarea area -->
  <div class={compact ? 'p-3 pb-2' : ''}>
    <Textarea
      value={formState.title}
      data-testid={`quick-add-title-${parentId}`}
      data-quick-add-parent={parentId}
      oninput={(e) => onUpdateField(parentId, 'title', e.currentTarget.value)}
      onkeydown={handleKeydown}
      placeholder={t('collections.enterSummary')}
      rows={2}
      class="w-full px-0 py-0 text-sm leading-5 resize-none border-0 focus:outline-none focus:ring-0"
      style="background-color: transparent; color: var(--ds-text); caret-color: var(--ds-text);"
    />
  </div>

  <!-- Divider (compact layout only; the board layout mirrors the card footer instead) -->
  {#if compact}
    <div class="border-t mx-3" style="border-color: var(--ctx-border, var(--ds-border));"></div>
  {/if}

  <!-- Actions Footer -->
  <div
    class="flex items-center {compact ? 'p-3 pt-2 gap-2' : 'mt-2.5 pt-2 gap-1.5'}"
    class:flex-wrap={compact}
  >
    <!-- Workspace Selector -->
    <div class="relative" bind:this={workspaceAnchor} data-quick-add-menu-anchor>
      <Button
        variant="default"
        size="small"
        onclick={toggleWorkspaceDropdown}
        class={compact ? '!size-7 !p-0' : '!size-6 !p-0 !rounded-[3px]'}
        dataTestid="quick-add-workspace"
        title={selectedWorkspace?.name || 'Select workspace'}
      >
        {#if selectedWorkspace?.avatar_url}
          <img
            src={selectedWorkspace.avatar_url}
            alt="{selectedWorkspace.name} avatar"
            class="{compact ? 'w-5 h-5' : 'w-4 h-4'} rounded object-cover"
          />
        {:else if selectedWorkspace?.icon}
          {@const WsIcon = iconMap[selectedWorkspace.icon] || Package}
          <WsIcon
            class={compact ? 'w-4 h-4' : 'w-3.5 h-3.5'}
            style="color: {selectedWorkspace?.color || 'var(--ds-icon)'};"
          />
        {:else}
          <Package class={compact ? 'w-4 h-4' : 'w-3.5 h-3.5'} style="color: var(--ds-icon);" />
        {/if}
      </Button>

      {#if showWorkspaceDropdown}
        <div
          use:portal
          data-quick-add-menu
          class="fixed z-[1000] overflow-y-auto rounded-md shadow-lg border py-1"
          style="{workspaceMenuStyle} background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
        >
          {#each workspaces as ws}
            <button
              type="button"
              onclick={() => selectWorkspace(ws.id)}
              data-testid={`quick-add-workspace-option-${ws.id}`}
              class="w-full px-3 py-2 text-left text-sm flex items-center gap-2 transition-colors"
              style="color: var(--ds-text);"
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
            >
              {#if ws.avatar_url}
                <img src={ws.avatar_url} alt="" class="w-5 h-5 rounded object-cover" />
              {:else}
                {@const WsDropdownIcon = iconMap[ws.icon] || Package}
                <div
                  class="w-5 h-5 rounded flex items-center justify-center"
                  style="background-color: {ws.color || 'var(--ds-interactive)'};"
                >
                  <WsDropdownIcon class="w-3 h-3 text-white" />
                </div>
              {/if}
              <span class="truncate">{ws.name}</span>
            </button>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Item Type Selector -->
    {#if formState.loadingTypes}
      <Button
        variant="default"
        size="small"
        class={compact ? '!size-7 !p-0' : '!h-6 !px-1.5 !gap-1 !rounded-[3px]'}
        dataTestid="quick-add-type-loading"
        title="Loading item types"
        disabled
      >
        <LoaderCircle class="w-3.5 h-3.5 animate-spin" />
        {#if !compact}<span class="text-xs">Loading types</span>{/if}
      </Button>
    {:else if formState.availableTypes?.length > 0}
      <div class="relative" bind:this={itemTypeAnchor} data-quick-add-menu-anchor>
        {#if compact}
          <Button
            variant="default"
            size="small"
            onclick={toggleItemTypeDropdown}
            class="!size-7 !p-0"
            title={selectedItemType?.name || 'Select type'}
          >
            {#if selectedItemType}
              <ItemTypeIcon
                icon={selectedItemType.icon}
                color={selectedItemType.color}
              />
            {:else}
              <Package class="w-4 h-4" style="color: var(--ds-icon);" />
            {/if}
          </Button>
        {:else}
          <!-- Board layout: icon-only type badge (matches the card footer badge) + chevron -->
          <Button
            variant="default"
            size="small"
            onclick={toggleItemTypeDropdown}
            class="!h-6 !px-1 !gap-0.5 !rounded-[3px]"
            dataTestid="quick-add-type"
            title={selectedItemType?.name || t('collections.selectType')}
          >
            {#if selectedItemType}
              <ItemTypeIcon
                icon={selectedItemType.icon}
                color={selectedItemType.color}
                title={selectedItemType.name}
              />
            {:else}
              <span class="text-xs" style="color: var(--ds-text-subtle);">{t('collections.selectType')}</span>
            {/if}
            <ChevronDown class="w-3 h-3" style="color: var(--ds-text-subtle);" />
          </Button>
        {/if}

        {#if showItemTypeDropdown}
          <div
            use:portal
            data-quick-add-menu
            class="fixed z-[1000] overflow-y-auto rounded-md shadow-lg border py-1"
            style="{itemTypeMenuStyle} background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
          >
            {#each formState.availableTypes as itemType}
              <button
                type="button"
                onclick={() => selectItemType(itemType.id)}
                data-testid={`quick-add-type-option-${itemType.id}`}
                class="w-full px-3 py-2 text-left text-sm flex items-center gap-2 transition-colors"
                style="color: var(--ds-text);"
                onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
                onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
              >
                <ItemTypeIcon {itemType} />
                <span class="truncate">{itemType.name}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    {#if !compact}
      <span class="flex-1"></span>
    {/if}

    <!-- Cancel Button. The ghost variant inherits --ctx-text, which a gradient
         background sets to white for the view chrome — inside this card the
         surface stays light, so pin it to the card's own text color. -->
    {#if compact}
      <Button
        variant="ghost"
        size="small"
        icon={X}
        onclick={() => onCancel(parentId)}
        class="!size-7 !p-0"
        style="color: var(--ds-text);"
        dataTestid="quick-add-cancel"
        title={t('common.cancel')}
      />
    {:else}
      <Button
        variant="ghost"
        size="small"
        onclick={() => onCancel(parentId)}
        class="!h-6 !px-2 !text-xs"
        style="color: var(--ds-text);"
        dataTestid="quick-add-cancel"
        title="{t('common.cancel')} ({getDisplayString(cancelShortcut)})"
      >
        {t('common.cancel')}
      </Button>
    {/if}

    <!-- Create Button -->
    {#if compact}
      <!-- shortcut-guard-exempt: Enter is handled by the quick-add textarea; the compact icon-only layout intentionally omits its visible hint. -->
      <Button
        variant="primary"
        size="small"
        icon={Plus}
        onclick={() => onCreate(parentId)}
        class="!size-7 !p-0"
        dataTestid="quick-add-create"
        title={t('common.create')}
        disabled={formState.loadingTypes || !formState.itemTypeId}
      />
    {:else}
      <Button
        variant="primary"
        size="small"
        keyboardHint={getDisplayString(createShortcut)}
        onclick={() => onCreate(parentId)}
        class="!h-6 !px-2 !text-xs !gap-1.5"
        dataTestid="quick-add-create"
        disabled={formState.loadingTypes || !formState.itemTypeId}
      >
        {t('common.create')}
      </Button>
    {/if}
  </div>

  <!-- Error message -->
  {#if formState.error}
    <div class="text-xs {compact ? 'px-3 pb-3' : 'mt-1.5'}" style="color: var(--ds-text-danger);">
      {formState.error}
    </div>
  {/if}
</div>
