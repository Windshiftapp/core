<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { packs as packsApi } from '../api/packs.js';
  import IconPackage from '@tabler/icons-svelte-runes/icons/package';
  import EmptyState from '../components/EmptyState.svelte';
  import Panel from '../components/Panel.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Lozenge from '../components/Lozenge.svelte';

  // Read-only catalog of the packs embedded in the server. Applying a pack is
  // done by picking one while creating a workspace (CreateModal → Workspace
  // form), so this screen only reports what the server ships.
  let packs = $state([]);
  let loading = $state(true);
  let loadError = $state(null);

  onMount(loadPacks);

  async function loadPacks() {
    try {
      loading = true;
      loadError = null;
      packs = (await packsApi.list()) || [];
    } catch (error) {
      console.error('Failed to load built-in packs:', error);
      loadError = t('settings.featurePacks.loadFailed');
    } finally {
      loading = false;
    }
  }
</script>

<PageHeader
  icon={IconPackage}
  title={t('settings.featurePacks.title')}
  subtitle={t('settings.featurePacks.subtitle')}
/>

{#if loading}
  <Panel padding="spacious" class="text-center">
    <div class="animate-pulse" style="color: var(--ds-text-subtle);">
      {t('settings.featurePacks.loading')}
    </div>
  </Panel>
{:else if loadError}
  <Panel padding="spacious">
    <p style="color: var(--ds-text-danger, #dc2626);" data-testid="pack-load-error">{loadError}</p>
  </Panel>
{:else if packs.length === 0}
  <Panel padding="spacious">
    <EmptyState icon={IconPackage} title={t('settings.featurePacks.empty')} />
  </Panel>
{:else}
  <div class="space-y-3">
    {#each packs as pack (pack.name)}
      <div data-testid={`pack-row-${pack.name}`}>
        <Panel>
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="font-medium" style="color: var(--ds-text);">{pack.name}</span>
              <Lozenge appearance="default">{pack.version}</Lozenge>
              {#if pack.has_content}
                <Lozenge appearance="info">{t('settings.featurePacks.content')}</Lozenge>
              {:else}
                <Lozenge appearance="default">{t('settings.featurePacks.noContent')}</Lozenge>
              {/if}
            </div>
            {#if pack.description}
              <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">{pack.description}</p>
            {/if}
            {#if pack.plugins?.length}
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                {t('settings.featurePacks.plugins')}: {pack.plugins.join(', ')}
              </p>
            {/if}
          </div>
        </Panel>
      </div>
    {/each}
  </div>
{/if}
