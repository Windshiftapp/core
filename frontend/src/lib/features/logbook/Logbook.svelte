<script>
  import { onMount } from 'svelte';
  import { currentRoute } from '../../router.js';
  import { logbookStore } from '../../stores/logbook.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import BucketNavigation from './BucketNavigation.svelte';
  import DocumentList from './DocumentList.svelte';
  import Spinner from '../../components/Spinner.svelte';

  let activeBucketId = $derived($currentRoute.params?.bucketId || null);

  onMount(async () => {
    if (!logbookStore.bucketsLoaded) {
      await logbookStore.loadBuckets();
    }
  });

  // Load documents when bucket changes
  $effect(() => {
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
  <div class="flex-1">
    {#if logbookStore.bucketsLoading && !logbookStore.bucketsLoaded}
      <div class="flex items-center justify-center h-64">
        <Spinner />
      </div>
    {:else}
      <DocumentList {activeBucketId} />
    {/if}
  </div>
</div>
