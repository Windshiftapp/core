<script>
  import { onMount } from 'svelte';
  import PageHeader from '../../layout/PageHeader.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    title,
    subtitle,
    milestoneFilter = $bindable(null),
    oncreate,
    createEvent = '',
    leadingActions = null,
    primaryAction,
  } = $props();

  function updateURL() {
    const url = new URL(window.location.href);
    if (milestoneFilter) url.searchParams.set('milestone', milestoneFilter.toString());
    else url.searchParams.delete('milestone');
    window.history.replaceState({}, '', url);
  }

  function handleMilestoneSelect(result) {
    milestoneFilter = result.value;
    updateURL();
  }

  function handleKeydown(event) {
    const tagName = event.target?.tagName;
    if (tagName === 'INPUT' || tagName === 'TEXTAREA' || tagName === 'SELECT') return;
    if (event.key !== 'a' && event.key !== 'A') return;
    event.preventDefault();
    oncreate?.();
  }

  onMount(() => {
    const milestone = new URLSearchParams(window.location.search).get('milestone');
    if (milestone) milestoneFilter = parseInt(milestone);

    document.addEventListener('keydown', handleKeydown);
    if (createEvent) window.addEventListener(createEvent, oncreate);

    return () => {
      document.removeEventListener('keydown', handleKeydown);
      if (createEvent) window.removeEventListener(createEvent, oncreate);
    };
  });
</script>

<PageHeader {title} {subtitle}>
  {#snippet actions()}
    <div class="flex items-center gap-3">
      {#if leadingActions}
        {@render leadingActions()}
      {/if}
      <div class="w-48">
        <MilestoneCombobox
          bind:value={milestoneFilter}
          placeholder={t('milestones.allMilestones')}
          onSelect={handleMilestoneSelect}
        />
      </div>
      {@render primaryAction()}
    </div>
  {/snippet}
</PageHeader>
