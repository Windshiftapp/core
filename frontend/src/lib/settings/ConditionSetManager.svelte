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
</script>

<SetListPage {config}>
  {#snippet meta(cs)}
    <div class="flex items-center gap-4 text-sm">
      <div class="flex items-center gap-1.5">
        <span style="color: var(--ds-text-subtle);">{t('conditionSets.workflow')}:</span>
        <span class="font-medium" style="color: var(--ds-text);">
          {cs.workflow_name || t('common.none')}
        </span>
      </div>
      {#if (cs.gated_transitions || []).length > 0}
        <div class="flex items-center gap-1.5 flex-wrap">
          <span style="color: var(--ds-text-subtle);">{t('conditionSets.transitions')}:</span>
          {#each cs.gated_transitions as gt (gt.transition_id)}
            <Lozenge color="blue" text={`${transitionFromLabel(gt)} → ${gt.to_status_name}`} />
          {/each}
        </div>
      {/if}
    </div>
  {/snippet}
</SetListPage>
