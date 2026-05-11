<script>
  import { onMount } from 'svelte';
  import { currentRoute } from '../../router.js';
  import { logbookStore } from '../../stores/logbook.svelte.js';
  import TabNav from '../../components/TabNav.svelte';
  import BucketNavigation from './BucketNavigation.svelte';
  import DocumentList from './DocumentList.svelte';
  import Spinner from '../../components/Spinner.svelte';

  let activeBucketId = $derived($currentRoute.params?.bucketId || null);
  let activeSection = $derived($currentRoute.query?.subtab || 'documents');

  // Lazy-load the actions settings component
  let LogbookActionsSettings = $state(null);
  $effect(() => {
    if (activeSection === 'actions' && !LogbookActionsSettings) {
      import('../logbook-actions/LogbookActionsSettings.svelte').then(m => {
        LogbookActionsSettings = m.default;
      });
    }
  });

  onMount(async () => {
    if (!logbookStore.bucketsLoaded) {
      await logbookStore.loadBuckets();
    }
  });

  // Load documents when bucket changes (only when viewing documents)
  $effect(() => {
    if (activeSection !== 'documents') return;
    if (activeBucketId) {
      logbookStore.loadDocuments(activeBucketId);
    } else {
      logbookStore.loadAllDocuments();
    }
  });
</script>

<!-- Main container with two-panel layout -->
<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <!-- Left Sidebar - Bucket Navigation -->
  <BucketNavigation {activeBucketId} />

  <!-- Main Content -->
  <div class="flex-1 flex flex-col">
    {#if logbookStore.bucketsLoading && !logbookStore.bucketsLoaded}
      <div class="flex items-center justify-center h-64">
        <Spinner />
      </div>
    {:else}
      <!-- Tab bar (only when a bucket is selected) -->
      {#if activeBucketId}
        <div style="padding: 0 24px;">
          <TabNav
            tabs={[
              { id: 'documents', label: 'Documents' },
              { id: 'actions', label: 'Actions' }
            ]}
            basePath={`/logbook/bucket/${activeBucketId}`}
            defaultTab="documents"
          />
        </div>
      {/if}

      <!-- Content -->
      <div class="flex-1">
        {#if activeSection === 'actions' && activeBucketId}
          {#if LogbookActionsSettings}
            <LogbookActionsSettings bucketId={activeBucketId} />
          {:else}
            <div class="flex items-center justify-center h-64">
              <Spinner />
            </div>
          {/if}
        {:else}
          <DocumentList {activeBucketId} />
        {/if}
      </div>
    {/if}
  </div>
</div>
