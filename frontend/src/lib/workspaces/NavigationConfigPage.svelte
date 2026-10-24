<script>
  import { onMount } from 'svelte';
  import Home from '@tabler/icons-svelte-runes/icons/home';
  import Palette from '@tabler/icons-svelte-runes/icons/palette';
  import Rotate from '@tabler/icons-svelte-runes/icons/rotate';
  import Settings from '@tabler/icons-svelte-runes/icons/settings';
  import Toggle from '../components/Toggle.svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import {
    COLLECTION_VIEW_IDS,
    testNavigationItems,
    workspaceOnlyViews,
    workspaceViewItems,
  } from '../navigation/workspaceNavigation.js';
  import { viewSettingsStore } from '../stores/viewSettings.svelte.js';
  import { workspacePermissions } from '../stores/workspacePermissions.svelte.js';
  import { currentWorkspace } from '../stores/workspaces.svelte.js';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import { collections } from '../api/collections.js';
  import { workspaces } from '../api/workspaces.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';

  let { workspaceId = null, collectionId = null } = $props();

  let loading = $state(true);
  let loadError = $state(null);
  let config = $state(null);
  // Enabled nav ids for the scope being configured.
  let enabled = $state([]);
  // Last set the server acknowledged. Failures roll back to this, not to a
  // snapshot that may already be older than a later toggle.
  let persisted = [];
  let toggleGeneration = 0;
  let inherited = $state(true);
  let saving = $state(false);
  let collectionName = $state('');
  let defaultView = $state(null);

  // Mirrors the sidebar's row order: overview, views, tests, tools. Fixed
  // sections render without toggles for orientation; test entries are not
  // nav-configurable (module and permission gating decide their visibility).
  // A global collection lives outside any workspace nav, so only the views
  // section applies there. A workspace collection cannot toggle the
  // workspace-only tools either — they follow the workspace setting.
  const isCollectionScope = $derived(Boolean(collectionId));
  const sections = $derived.by(() => {
    // Collection scopes may only toggle the collection-scoped views; the
    // workspace scope also gets the workspace-scoped views-group entries
    // (the queue) in the same section.
    const viewsSection = {
      id: 'views',
      title: t('settings.boardConfig.views'),
      rows: isCollectionScope
        ? workspaceViewItems.filter((view) => COLLECTION_VIEW_IDS.has(view.id))
        : workspaceViewItems,
    };
    if (!workspaceId) {
      return [viewsSection];
    }
    const scoped = [
      { id: 'overview', title: null, fixed: true, rows: [{ id: 'overview', labelKey: 'workspaceSettings.views.overview', icon: Home }] },
      viewsSection,
      { id: 'tests', title: t('commandPalette.commands.tests.label'), fixed: true, moduleGated: true, rows: testNavigationItems },
    ];
    if (isCollectionScope) {
      return scoped;
    }
    return [
      ...scoped,
      {
        id: 'tools',
        title: t('actions.config.tools'),
        rows: [
          ...workspaceOnlyViews,
          { id: 'look-and-feel', labelKey: 'lookAndFeel.title', icon: Palette, fixed: true, adminOnly: true },
          { id: 'settings', labelKey: 'workspaceSettings.title', icon: Settings, fixed: true, adminOnly: true },
        ],
      },
    ];
  });

  const workspaceName = $derived($currentWorkspace?.name || t('common.workspace'));
  const canAdmin = $derived(workspacePermissions.canAdminWorkspace(workspaceId));

  const scopeTitle = $derived(
    isCollectionScope && collectionName
      ? t('navConfig.scopeCollection', { name: collectionName })
      : t('navConfig.scopeWorkspace', { name: workspaceName })
  );

  $effect(() => {
    if (workspaceId) {
      viewSettingsStore.load(workspaceId, collectionId);
    }
  });

  onMount(loadData);

  async function loadData() {
    loading = true;
    loadError = null;
    try {
      const requests = [
        collections.getBoardConfiguration(collectionId ?? null, workspaceId ?? null),
      ];
      if (isCollectionScope) {
        requests.push(collections.get(collectionId));
      } else {
        requests.push(workspaces.get(workspaceId));
      }
      const [data, scope] = await Promise.all(requests);
      config = data;
      if (isCollectionScope) {
        collectionName = scope?.name || '';
      } else {
        defaultView = scope?.default_view || null;
      }
      applyEffectiveViews(data);
    } catch (error) {
      loadError = error?.message || String(error);
    } finally {
      loading = false;
    }
  }

  function applyEffectiveViews(data) {
    const effective = data?.view_settings?.enabled_views;
    // Absent or empty settings mean "everything enabled" — the storage
    // default for a scope that has never been configured.
    enabled =
      Array.isArray(effective) && effective.length > 0
        ? [...effective]
        : [...viewSettingsStore.allNavIds];
    if (isCollectionScope) {
      // The collection effective set inherits workspace-only tools ids
      // underneath any override; they are read-only in this scope and must
      // never round-trip into its payload.
      enabled = enabled.filter((id) => COLLECTION_VIEW_IDS.has(id));
    }
    persisted = [...enabled];
    inherited = Boolean(data?.view_settings_inherited) || !data?.view_settings;
  }

  function isEnabled(id) {
    return enabled.includes(id);
  }

  // Server invariants mirrored as disabled toggles: the workspace default
  // view must stay enabled and the set must never empty out.
  // Whether a toggleable row may be switched off: the workspace default
  // view must stay enabled and the set must never empty out. Only consulted
  // for rows that are currently on.
  function canToggleOff(row) {
    if (!isCollectionScope && row.id === defaultView) return false;
    if (enabled.length <= 1) return false;
    return true;
  }

  // Serializes persistence; each write captures the set at execution time so
  // rapid toggles coalesce into last-write-wins instead of racing PUTs.
  let writeChain = Promise.resolve();

  function toggleRow(row) {
    const generation = ++toggleGeneration;
    enabled = isEnabled(row.id) ? enabled.filter((x) => x !== row.id) : [...enabled, row.id];
    writeChain = writeChain.then(async () => {
      try {
        await persistCurrent();
      } catch (error) {
        // Roll back to the last saved set only when no newer toggle arrived;
        // otherwise the later toggle's queued save owns the current state.
        if (generation === toggleGeneration) {
          enabled = [...persisted];
        }
        errorToast(t('navConfig.saveError', { error: error?.message || error }));
      }
    });
  }

  async function persistCurrent() {
    saving = true;
    // Capture the exact set being written so a later toggle during the await
    // does not get marked as persisted.
    const sent = [...enabled];
    try {
      // Round-trip the stored configuration fields: a PUT rewrites them, so
      // an omitted columns array would wipe the board's saved columns.
      const payload = {
        columns: config?.columns || [],
        backlog_status_ids: config?.backlog_status_ids || [],
        list_columns: config?.list_columns || [],
        roadmap_config: config?.roadmap_config || null,
        card_fields: (config?.card_fields || []).map((f, i) => ({
          field_identifier: f.field_identifier,
          field_type: f.field_type,
          display_order: f.display_order ?? i,
          width: f.width ?? 0,
        })),
        show_rightmost_column_last_50: Boolean(config?.show_rightmost_column_last_50),
        completed_item_retention_days: config?.completed_item_retention_days ?? null,
        view_settings: { enabled_views: sent },
      };
      // The PUT response carries the saved config with its effective view
      // settings; adopt it without touching the local toggle state so rows
      // do not re-render from a refetch.
      config = config?.id
        ? await collections.updateBoardConfiguration(collectionId, config.id, payload, workspaceId)
        : await collections.createBoardConfiguration(collectionId, workspaceId, payload);
      persisted = sent;
      inherited = Boolean(config?.view_settings_inherited) || !config?.view_settings;
      // Reflect the saved set immediately: navigation reads this store, and
      // dropping the entry would fall back to the everything-enabled default
      // until some later refetch. A workspace-scope save also changes every
      // inheriting collection, so drop those caches and let them refetch.
      if (collectionId) {
        viewSettingsStore.applyEnabledViews(workspaceId, collectionId, sent, inherited);
      } else {
        viewSettingsStore.invalidateWorkspace(workspaceId);
        viewSettingsStore.applyEnabledViews(workspaceId, null, sent, inherited);
      }
    } finally {
      saving = false;
    }
  }

  async function resetToInherited() {
    const before = enabled.slice();
    enabled = [];
    writeChain = writeChain.then(async () => {
      try {
        saving = true;
        try {
          await collections.updateBoardConfiguration(collectionId, config?.id, {
            view_settings: { enabled_views: null },
          }, workspaceId);
          applyEffectiveViews(
            await collections.getBoardConfiguration(collectionId ?? null, workspaceId ?? null)
          );
          viewSettingsStore.applyEnabledViews(workspaceId, collectionId, enabled, inherited);
        } finally {
          saving = false;
        }
      } catch (error) {
        enabled = before;
        errorToast(t('navConfig.saveError', { error: error?.message || error }));
      }
    });
  }
</script>

<div class="max-w-3xl mx-auto px-6 py-8" data-testid="navigation-config-page">
  <header class="mb-6">
    <h1 class="text-2xl font-semibold" style="color: var(--ds-text);">
      {t('navConfig.configureTitle')}
    </h1>
    <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">{scopeTitle}</p>
    {#if isCollectionScope}
      <p class="mt-1 text-sm" style="color: var(--ds-text-subtle);">
        {t('navConfig.collectionScopeHelp')}
      </p>
    {/if}
  </header>

  {#if loading}
    <p class="text-sm" style="color: var(--ds-text-subtle);" data-testid="nav-config-loading">
      {t('common.loading')}
    </p>
  {:else if loadError}
    <div class="text-sm" style="color: var(--ds-text-danger);" data-testid="nav-config-error">
      {t('navConfig.loadError', { error: loadError })}
      <button type="button" class="underline ml-2" onclick={loadData}>{t('common.retry')}</button>
    </div>
  {:else}
    {#if isCollectionScope && inherited}
      <p
        class="mb-4 text-xs rounded px-3 py-2 inline-block"
        style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
        data-testid="nav-config-inherited-badge"
      >
        {t('navConfig.inheritedBadge')}
      </p>
    {/if}

    <section class="space-y-6">
      {#each sections as section (section.id)}
        <div data-testid={`nav-config-section-${section.id}`}>
          {#if section.title}
            <h2 class="text-xs font-semibold uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">
              {section.title}
            </h2>
          {/if}
          <div class="rounded-lg border divide-y" style="border-color: var(--ds-border);">
            {#each section.rows as row (row.id)}
              {@render navRow(row, section)}
            {/each}
          </div>
        </div>
      {/each}
    </section>

    {#if isCollectionScope}
      <button
        type="button"
        class="mt-6 inline-flex items-center gap-2 text-sm rounded px-3 py-2 transition-colors hover:bg-[var(--ds-background-neutral)] disabled:opacity-50"
        style="color: var(--ds-text-subtle);"
        data-testid="nav-config-reset"
        disabled={saving}
        onclick={resetToInherited}
      >
        <Rotate size={15} />
        {t('navConfig.resetToInherited')}
      </button>
    {/if}
  {/if}
</div>

{#snippet navRow(row, section)}
  {@const ItemIcon = row.icon}
  {@const label = t(row.labelKey)}
  {@const locked = row.fixed || section.fixed}
  {@const on = locked || isEnabled(row.id)}
  <div
    class="flex items-center justify-between gap-4 px-4 py-3 transition-opacity {on ? '' : 'opacity-40 hover:opacity-70'}"
    data-testid={`nav-config-row-${row.id}`}
  >
    <div class="flex items-center gap-3 min-w-0">
      <span class="shrink-0 inline-flex" style="color: var(--ds-text-subtle);">
        <ItemIcon size={17} />
      </span>
      <div class="min-w-0">
        <div class="text-sm font-medium truncate" style="color: var(--ds-text);">{label}</div>
        {#if row.fixed && row.adminOnly}
          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('navConfig.adminOnlyHint')}</div>
        {:else if section.moduleGated && !$moduleSettings.test_management_enabled}
          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('navConfig.moduleHint')}</div>
        {:else if !locked && !isCollectionScope && row.id === defaultView}
          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('navConfig.defaultViewHint')}</div>
        {/if}
      </div>
    </div>
    {#if locked}
      <span class="text-xs shrink-0" style="color: var(--ds-text-subtle);">
        {t('navConfig.alwaysVisible')}
      </span>
    {:else}
      <Tooltip content={label} placement="left">
        <Toggle
          size="small"
          checked={on}
          disabled={on && !canToggleOff(row)}
          ariaLabel={label}
          dataTestid={`nav-config-toggle-${row.id}`}
          onchange={() => toggleRow(row)}
        />
      </Tooltip>
    {/if}
  </div>
{/snippet}
