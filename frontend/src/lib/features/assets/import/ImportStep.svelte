<script>
  import { assetImportStore } from './AssetImportStore.svelte.js';
  import Progress from '../../../components/Progress.svelte';
  import AlertBox from '../../../components/AlertBox.svelte';
  import { Check, X, AlertTriangle } from 'lucide-svelte';

  let importData = $derived(assetImportStore.import);

  let percentage = $derived(
    importData.totalRows > 0
      ? Math.round(((importData.importedCount + importData.failedCount) / importData.totalRows) * 100)
      : 0
  );

  let isComplete = $derived(importData.result !== null);
  let isFailed = $derived(importData.error !== null && !isComplete);
  let isRunning = $derived(!isComplete && !isFailed && importData.phase !== 'idle');

  let showErrors = $state(false);
</script>

<div class="space-y-6">
  {#if isComplete}
    <!-- Success summary -->
    <div class="text-center py-4">
      <div class="w-12 h-12 rounded-full mx-auto flex items-center justify-center mb-3" style="background: var(--ds-background-success-subtle);">
        <Check class="w-6 h-6" style="color: var(--ds-text-success);" />
      </div>
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">Import Complete</h3>
      <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
        {importData.importedCount} assets imported successfully
        {#if importData.failedCount > 0}
          , {importData.failedCount} failed
        {/if}
      </p>
    </div>

    <!-- Stats -->
    <div class="grid grid-cols-3 gap-4">
      <div class="p-3 rounded-lg border text-center" style="border-color: var(--ds-border); background: var(--ds-background-input);">
        <p class="text-2xl font-semibold" style="color: var(--ds-text);">{importData.totalRows}</p>
        <p class="text-xs" style="color: var(--ds-text-subtle);">Total Rows</p>
      </div>
      <div class="p-3 rounded-lg border text-center" style="border-color: var(--ds-border-success); background: var(--ds-background-success-subtle);">
        <p class="text-2xl font-semibold" style="color: var(--ds-text-success);">{importData.importedCount}</p>
        <p class="text-xs" style="color: var(--ds-text-subtle);">Imported</p>
      </div>
      <div class="p-3 rounded-lg border text-center" style="border-color: {importData.failedCount > 0 ? 'var(--ds-border-danger)' : 'var(--ds-border)'}; background: {importData.failedCount > 0 ? 'var(--ds-background-danger-subtle)' : 'var(--ds-background-input)'};">
        <p class="text-2xl font-semibold" style="color: {importData.failedCount > 0 ? 'var(--ds-text-danger)' : 'var(--ds-text)'};">{importData.failedCount}</p>
        <p class="text-xs" style="color: var(--ds-text-subtle);">Failed</p>
      </div>
    </div>
  {:else if isFailed}
    <!-- Error state -->
    <AlertBox variant="error" message={importData.error} />
  {:else if isRunning}
    <!-- Progress -->
    <div class="space-y-4">
      <div class="text-center">
        <p class="text-sm font-medium" style="color: var(--ds-text);">
          Importing assets...
        </p>
        <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
          {importData.importedCount + importData.failedCount} of {importData.totalRows} rows processed
        </p>
      </div>

      <Progress value={percentage} showLabel={true} size="lg" />

      <div class="flex justify-center gap-6 text-sm">
        <span style="color: var(--ds-text-success);">
          <Check class="w-4 h-4 inline mr-1" />
          {importData.importedCount} imported
        </span>
        {#if importData.failedCount > 0}
          <span style="color: var(--ds-text-danger);">
            <X class="w-4 h-4 inline mr-1" />
            {importData.failedCount} failed
          </span>
        {/if}
      </div>
    </div>
  {:else}
    <!-- Not started yet -->
    <div class="text-center py-8">
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        Click "Start Import" to begin importing {assetImportStore.upload.totalRows} assets.
      </p>
    </div>
  {/if}

  <!-- Error details -->
  {#if importData.errors.length > 0}
    <div>
      <button
        class="text-sm flex items-center gap-1"
        style="color: var(--ds-text-subtle);"
        onclick={() => { showErrors = !showErrors; }}
      >
        <AlertTriangle class="w-3.5 h-3.5" />
        {importData.errors.length} error{importData.errors.length !== 1 ? 's' : ''}
        <span class="text-xs">({showErrors ? 'hide' : 'show'})</span>
      </button>
      {#if showErrors}
        <div class="mt-2 max-h-48 overflow-y-auto rounded border p-3 text-xs font-mono space-y-1" style="border-color: var(--ds-border); background: var(--ds-background-input); color: var(--ds-text-danger);">
          {#each importData.errors as error}
            <p>{error}</p>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</div>
