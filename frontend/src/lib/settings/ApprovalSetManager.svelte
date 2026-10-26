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
</script>

<SetListPage {config}>
  {#snippet meta(s)}
    <div class="flex items-center gap-4 text-sm">
      <div class="flex items-center gap-1.5">
        <span style="color: var(--ds-text-subtle);">{t('approvalSets.workflow')}:</span>
        <span class="font-medium" style="color: var(--ds-text);">
          {s.workflow_name || t('common.none')}
        </span>
      </div>
      {#if (s.gated_statuses || []).length > 0}
        <div class="flex items-center gap-1.5 flex-wrap">
          <span style="color: var(--ds-text-subtle);">{t('approvalSets.statuses')}:</span>
          {#each s.gated_statuses as gs (gs.status_id)}
            <Lozenge customBg={gs.category_color || '#3b82f6'} text={gs.status_name} />
          {/each}
        </div>
      {/if}
    </div>
  {/snippet}
</SetListPage>
