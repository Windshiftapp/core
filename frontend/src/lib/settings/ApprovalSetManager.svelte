<script>
  import IconRubberStamp from '@tabler/icons-svelte-runes/icons/rubber-stamp';
  import Lozenge from '../components/Lozenge.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { approvalSets as api } from '../api/approvalSets.js';
  import SetListPage from './SetListPage.svelte';

  const config = {
    api,
    icon: IconRubberStamp,
    titleKey: 'approvalSets.title',
    subtitleKey: 'approvalSets.subtitle',
    addKey: 'approvalSets.add',
    searchPlaceholderKey: 'approvalSets.searchPlaceholder',
    emptyKey: 'approvalSets.empty',
    emptyDescKey: 'approvalSets.emptyDesc',
    createFirstKey: 'approvalSets.createFirst',
    noMatchingResultsKey: 'approvalSets.noMatchingResults',
    hotkeyScope: 'approvalSets',
    newRoute: '/admin/approval-sets/new',
    detailRoute: (id) => `/admin/approval-sets/${id}`,
    addTestid: 'approval-sets-add',
    searchTestid: 'approval-set-search',
    listTestid: 'approval-sets-list',
    rowTestidPrefix: 'approval-set',
    errorLabel: 'approval sets',
  };

  const columns = [
    { key: 'name', label: t('common.name'), sortable: true, slot: 'name' },
    {
      key: 'workflow_name',
      label: t('approvalSets.workflow'),
      sortable: true,
      render: (s) => s.workflow_name || t('common.none'),
    },
    { key: 'gated', label: t('approvalSets.statuses'), slot: 'gated' },
  ];
</script>

<SetListPage {config} {columns} cellSnippets={{ name: nameCell, gated: gatedCell }} />

{#snippet nameCell(s)}
  <div>
    <div class="font-medium" style="color: var(--ds-text);">{s.name}</div>
    {#if s.description}
      <div class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{s.description}</div>
    {/if}
  </div>
{/snippet}

{#snippet gatedCell(s)}
  <div class="flex items-center gap-1.5 flex-wrap">
    {#each s.gated_statuses || [] as gs (gs.status_id)}
      <Lozenge customBg={gs.category_color || '#3b82f6'} text={gs.status_name} />
    {:else}
      <span style="color: var(--ds-text-subtle);">{t('common.none')}</span>
    {/each}
  </div>
{/snippet}
