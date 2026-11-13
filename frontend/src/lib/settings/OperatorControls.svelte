<script>
  import { onMount } from 'svelte';
  import Shield from '@lucide/svelte/icons/shield';
  import { fetchV2Data } from '../api/core.js';
  import AlertBox from '../components/AlertBox.svelte';
  import Panel from '../components/Panel.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import Link from '../components/Link.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let loading = $state(true);
  let error = $state('');
  let status = $state(null);
  const limits = $derived(Object.entries(status?.policy?.limits || {}).filter(([, value]) => value > 0));

  onMount(async () => {
    try {
      status = await fetchV2Data('/operator-controls');
    } catch (err) {
      error = err?.message || t('operatorControls.loadFailed', 'Could not load operator controls.');
    } finally {
      loading = false;
    }
  });

  function label(name) {
    return t(`operatorControls.labels.${name}`, name.replaceAll('_', ' '));
  }
</script>

<div class="space-y-6" data-testid="operator-controls-page">
  <PageHeader icon={Shield} title={t('operatorControls.title', 'Operator controls')} />
  <AlertBox variant="info" message={t('operatorControls.description', 'These deployment controls are read-only. Contact the instance operator to change restrictions or allowances. Organization preferences can be more restrictive, but cannot exceed these ceilings.')} />
  {#if loading}
    <StateDisplay type="loading" />
  {:else if error}
    <StateDisplay type="error" description={error} />
  {:else}
    <Panel>
      <h2 class="font-semibold text-ds-text">{t('operatorControls.restrictions', 'Restricted capabilities')}</h2>
      {#if status?.policy?.deny?.length}
        <ul class="mt-3 space-y-1 text-ds-text-subtle" data-testid="operator-controls-restrictions">
          {#each status.policy.deny as capability}
            <li>{label(capability)}</li>
          {/each}
        </ul>
      {:else}
        <p class="mt-3 text-ds-text-subtle">{t('operatorControls.unrestricted', 'No additional capability restrictions.')}</p>
      {/if}
      {#if status?.policy?.mail_mode}
        <p class="mt-3 text-ds-text-subtle">{t('operatorControls.mailMode', 'Mail delivery mode')}: {status.policy.mail_mode}</p>
      {/if}
      {#if status?.policy?.import_admission_required}
        <p class="mt-3 text-ds-text-subtle">{t('operatorControls.importAdmission', 'Bulk imports require operator admission.')}</p>
      {/if}
      {#if status?.policy?.ai_models?.length}
        <p class="mt-3 text-ds-text-subtle">{t('operatorControls.models', 'Available AI models')}: {status.policy.ai_models.join(', ')}</p>
      {/if}
    </Panel>
    <Panel>
      <h2 class="font-semibold text-ds-text">{t('operatorControls.limits', 'Instance ceilings')}</h2>
      <dl class="mt-3 space-y-2 text-ds-text-subtle" data-testid="operator-controls-limits">
        {#each limits as [name, value]}
          <div class="flex justify-between gap-4"><dt>{label(name)}</dt><dd>{value.toLocaleString()}</dd></div>
        {/each}
      </dl>
    </Panel>
    <Panel>
      <h2 class="font-semibold text-ds-text">{t('operatorControls.usage', 'Current interval usage')}</h2>
      <dl class="mt-3 space-y-2 text-ds-text-subtle" data-testid="operator-controls-usage">
        {#each Object.entries(status?.usage || {}) as [name, value]}
          <div class="flex justify-between gap-4"><dt>{label(name)}</dt><dd>{Number(value).toLocaleString()}</dd></div>
        {/each}
      </dl>
    </Panel>
    {#if status?.blocked_definitions?.length}
      <Panel>
        <h2 class="font-semibold text-ds-text">{t('operatorControls.blockedDefinitions', 'Definitions blocked by operator controls')}</h2>
        <ul class="mt-3 space-y-2" data-testid="operator-controls-blocked-definitions">
          {#each status.blocked_definitions as definition}
            <li>
              {#if definition.kind === 'condition_set'}
                <Link href="/admin/condition-sets/{definition.id}">{definition.name}</Link>
              {:else}
                <span>{definition.name} ({definition.kind} #{definition.id})</span>
              {/if}
            </li>
          {/each}
        </ul>
        {#if status.blocked_definitions_truncated}
          <p class="mt-3 text-ds-text-subtle">{t('operatorControls.truncated', 'The first 1,000 blocked definitions are shown.')}</p>
        {/if}
      </Panel>
    {/if}
  {/if}
</div>
