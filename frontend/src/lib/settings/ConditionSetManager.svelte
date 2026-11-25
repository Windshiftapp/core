<script>
  import IconBarrierBlock from '@tabler/icons-svelte-runes/icons/barrier-block';
  import Lozenge from '../components/Lozenge.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { conditionSets as api } from '../api/conditionSets.js';
  import SetListPage from './SetListPage.svelte';
  import { transitionFromLabel } from './transitionLabels.js';

  const config = {
    api,
    icon: IconBarrierBlock,
    titleKey: 'conditionSets.title',
    subtitleKey: 'conditionSets.subtitle',
    addKey: 'conditionSets.add',
    searchPlaceholderKey: 'conditionSets.searchPlaceholder',
    emptyKey: 'conditionSets.empty',
    emptyDescKey: 'conditionSets.emptyDesc',
    createFirstKey: 'conditionSets.createFirst',
    noMatchingResultsKey: 'conditionSets.noMatchingResults',
    hotkeyScope: 'conditionSets',
    newRoute: '/admin/condition-sets/new',
    detailRoute: (id) => `/admin/condition-sets/${id}`,
    addTestid: 'condition-set-add',
    searchTestid: 'condition-set-search',
    listTestid: undefined,
    rowTestidPrefix: 'condition-set',
    errorLabel: 'condition sets',
  };

  const columns = [
    { key: 'name', label: t('common.name'), sortable: true, slot: 'name' },
    {
      key: 'workflow_name',
      label: t('conditionSets.workflow'),
      sortable: true,
      render: (cs) => cs.workflow_name || t('common.none'),
    },
    { key: 'gated', label: t('conditionSets.transitions'), slot: 'gated' },
  ];
</script>

<SetListPage {config} {columns} cellSnippets={{ name: nameCell, gated: gatedCell }} />

{#snippet nameCell(cs)}
  <div>
    <div class="font-medium" style="color: var(--ds-text);">{cs.name}</div>
    {#if cs.description}
      <div class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{cs.description}</div>
    {/if}
  </div>
{/snippet}

{#snippet gatedCell(cs)}
  <div class="flex items-center gap-1.5 flex-wrap">
    {#each cs.gated_transitions || [] as gt (gt.transition_id)}
      <Lozenge color="blue" text={`${transitionFromLabel(gt)} → ${gt.to_status_name}`} />
    {:else}
      <span style="color: var(--ds-text-subtle);">{t('common.none')}</span>
    {/each}
  </div>
{/snippet}
