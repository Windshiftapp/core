<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { packs as packsApi } from '../api/packs.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import IconPackage from '@tabler/icons-svelte-runes/icons/package';
  import Button from '../components/Button.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Panel from '../components/Panel.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Input from '../components/Input.svelte';

  let packs = $state([]);
  let loading = $state(true);
  let loadError = $state(null);

  let activePack = $state(null);
  let newWorkspaceName = $state('');
  let busy = $state(null);
  let report = $state(null);
  let reportAction = $state(null);

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

  function startPack(name) {
    activePack = name;
    newWorkspaceName = '';
    report = null;
    reportAction = null;
  }

  function cancelPack() {
    activePack = null;
    report = null;
    reportAction = null;
  }

  function resolveTarget() {
    const name = newWorkspaceName.trim();
    if (!name) return null;
    return { workspace_name: name };
  }

  async function runPack(action) {
    const target = resolveTarget();
    if (!target) {
      errorToast(t('settings.featurePacks.targetRequired'));
      return;
    }
    busy = action;
    report = null;
    try {
      const result =
        action === 'verify'
          ? await packsApi.verify(activePack, target)
          : await packsApi.apply(activePack, target);
      report = result;
      reportAction = action;
      if (result?.status === 'failed') {
        errorToast(t('settings.featurePacks.failed'));
      } else if (action === 'verify') {
        successToast(t('settings.featurePacks.verified'));
      } else {
        successToast(t('settings.featurePacks.applied'));
        // A pack provisions a new workspace; refresh the directory.
        await workspacesStore.reload();
      }
    } catch (error) {
      console.error(`Failed to ${action} pack:`, error);
      errorToast(error?.message || String(error));
    } finally {
      busy = null;
    }
  }

  function stageLabel(name) {
    const key = {
      plugins: 'stagePlugins',
      workspace: 'stageWorkspace',
      schema: 'stageSchema',
      content: 'stageContent',
      conformance: 'stageConformance',
    }[name];
    return key ? t(`settings.featurePacks.${key}`) : name;
  }

  function stageStatusLabel(status) {
    const key = { ok: 'stageOk', skipped: 'stageSkipped', failed: 'stageFailed' }[status];
    return key ? t(`settings.featurePacks.${key}`) : status;
  }

  function stageAppearance(status) {
    if (status === 'failed') return 'error';
    if (status === 'skipped') return 'default';
    return 'success';
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
          <div class="flex flex-wrap items-start justify-between gap-3">
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
            <Button
              variant="secondary"
              dataTestid={`pack-apply-${pack.name}`}
              onclick={() => startPack(pack.name)}
            >
              {t('settings.featurePacks.apply')}
            </Button>
          </div>

          {#if activePack === pack.name}
            <div
              data-testid={`pack-target-${pack.name}`}
              class="mt-4 pt-4 space-y-3"
              style="border-top: 1px solid var(--ds-border);"
            >
              <p class="text-sm" style="color: var(--ds-text-subtle);">
                {t('settings.featurePacks.targetNew')}
              </p>
              <Input
                bind:value={newWorkspaceName}
                type="text"
                placeholder={t('settings.featurePacks.newWorkspaceName')}
                ariaLabel={t('settings.featurePacks.newWorkspaceName')}
                id={`pack-new-name-${pack.name}`}
                dataTestid="pack-new-name"
              />

              <div class="flex items-center gap-2">
                <Button
                  variant="primary"
                  dataTestid="pack-run-apply"
                  disabled={busy !== null}
                  onclick={() => runPack('apply')}
                >
                  {t('settings.featurePacks.apply')}
                </Button>
                <Button
                  variant="secondary"
                  dataTestid="pack-run-verify"
                  disabled={busy !== null}
                  onclick={() => runPack('verify')}
                >
                  {t('settings.featurePacks.verify')}
                </Button>
                <Button variant="ghost" dataTestid="pack-cancel" onclick={cancelPack}>
                  {t('settings.featurePacks.cancel')}
                </Button>
              </div>

              {#if report}
                <div data-testid="pack-report">
                  <Panel padding="compact">
                    <div class="flex items-center gap-2 mb-2">
                      <span class="text-sm font-medium" style="color: var(--ds-text);">
                        {t('settings.featurePacks.reportTitle')}
                      </span>
                      <Lozenge appearance={report.status === 'failed' ? 'error' : 'success'}>
                        {reportAction === 'verify' ? 'verified' : report.status}
                      </Lozenge>
                    </div>
                    <ul class="space-y-1 text-sm">
                      {#each report.stages ?? [] as stage (stage.name)}
                        <li
                          class="flex items-start gap-2"
                          data-testid={`pack-stage-${stage.name}`}
                        >
                          <Lozenge appearance={stageAppearance(stage.status)}>
                            {stageStatusLabel(stage.status)}
                          </Lozenge>
                          <span style="color: var(--ds-text);">{stageLabel(stage.name)}</span>
                          {#if stage.detail}
                            <span style="color: var(--ds-text-subtle);">— {stage.detail}</span>
                          {/if}
                        </li>
                      {/each}
                    </ul>
                    {#if report.conformance}
                      <div class="mt-2 text-sm" data-testid="pack-conformance">
                        {#if report.conformance.conformant}
                          <Lozenge appearance="success">{t('settings.featurePacks.conformant')}</Lozenge>
                        {:else}
                          <Lozenge appearance="error">
                            {t('settings.featurePacks.driftRows', {
                              count: report.conformance.drift_count ?? 0,
                            })}
                          </Lozenge>
                        {/if}
                      </div>
                    {/if}
                  </Panel>
                </div>
              {/if}
            </div>
          {/if}
        </Panel>
      </div>
    {/each}
  </div>
{/if}
