<script>
  import { assetImportStore } from './AssetImportStore.svelte.js';
  import AlertBox from '../../../components/AlertBox.svelte';
  import { Check, AlertTriangle } from 'lucide-svelte';

  let upload = $derived(assetImportStore.upload);
  let target = $derived(assetImportStore.target);
  let mapping = $derived(assetImportStore.mapping);

  // Build preview of mapped data
  let mappedPreview = $derived(() => {
    const headers = upload.headers;
    const getCol = (row, idx) => (idx >= 0 && idx < row.length) ? row[idx] : '';

    return upload.previewRows.map(row => ({
      title: getCol(row, mapping.title),
      description: getCol(row, mapping.description),
      assetTag: getCol(row, mapping.assetTag),
      category: getCol(row, mapping.categoryId),
      status: getCol(row, mapping.statusId),
    }));
  });

  let preview = $derived(mappedPreview());

  // Validation checks
  let validationIssues = $derived(() => {
    const issues = [];
    if (mapping.title < 0) {
      issues.push({ type: 'error', message: 'Title field is not mapped (required)' });
    }
    if (!target.assetTypeId) {
      issues.push({ type: 'error', message: 'No asset type selected' });
    }

    // Check for empty titles in preview
    const emptyTitles = preview.filter(r => !r.title.trim()).length;
    if (emptyTitles > 0 && mapping.title >= 0) {
      issues.push({ type: 'warning', message: `${emptyTitles} preview rows have empty titles (will be skipped)` });
    }

    return issues;
  });

  let issues = $derived(validationIssues());
  let hasErrors = $derived(issues.some(i => i.type === 'error'));

  // Find type name
  let typeName = $derived(target.types.find(t => t.id === target.assetTypeId)?.name || 'Unknown');
</script>

<div class="space-y-6">
  <!-- Summary -->
  <div class="grid grid-cols-3 gap-4">
    <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-background-input);">
      <p class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Total Rows</p>
      <p class="text-2xl font-semibold mt-1" style="color: var(--ds-text);">{upload.totalRows}</p>
    </div>
    <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-background-input);">
      <p class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Asset Type</p>
      <p class="text-sm font-medium mt-2" style="color: var(--ds-text);">{typeName}</p>
    </div>
    <div class="p-4 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-background-input);">
      <p class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Fields Mapped</p>
      <p class="text-2xl font-semibold mt-1" style="color: var(--ds-text);">
        {[mapping.title, mapping.description, mapping.assetTag, mapping.categoryId, mapping.statusId].filter(v => v >= 0).length + Object.values(mapping.customFields).filter(v => v >= 0).length}
      </p>
    </div>
  </div>

  <!-- Validation -->
  {#if issues.length > 0}
    <div class="space-y-2">
      {#each issues as issue}
        <div class="flex items-center gap-2 text-sm" style="color: {issue.type === 'error' ? 'var(--ds-text-danger)' : 'var(--ds-text-warning)'};">
          <AlertTriangle class="w-4 h-4 flex-shrink-0" />
          {issue.message}
        </div>
      {/each}
    </div>
  {:else}
    <div class="flex items-center gap-2 text-sm" style="color: var(--ds-text-success);">
      <Check class="w-4 h-4" />
      All validations passed. Ready to import.
    </div>
  {/if}

  <!-- Mapped data preview -->
  {#if preview.length > 0}
    <div>
      <p class="text-sm font-medium mb-2" style="color: var(--ds-text);">Mapped Data Preview (first {preview.length} rows)</p>
      <div class="overflow-x-auto rounded border" style="border-color: var(--ds-border);">
        <table class="w-full text-xs">
          <thead>
            <tr style="background: var(--ds-background-input);">
              <th class="px-3 py-2 text-left font-medium" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">Title</th>
              {#if mapping.description >= 0}
                <th class="px-3 py-2 text-left font-medium" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">Description</th>
              {/if}
              {#if mapping.assetTag >= 0}
                <th class="px-3 py-2 text-left font-medium" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">Asset Tag</th>
              {/if}
              {#if mapping.categoryId >= 0}
                <th class="px-3 py-2 text-left font-medium" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">Category</th>
              {/if}
              {#if mapping.statusId >= 0}
                <th class="px-3 py-2 text-left font-medium" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">Status</th>
              {/if}
            </tr>
          </thead>
          <tbody>
            {#each preview as row}
              <tr>
                <td class="px-3 py-1.5 whitespace-nowrap max-w-[200px] truncate" style="color: {row.title ? 'var(--ds-text)' : 'var(--ds-text-danger)'}; border-bottom: 1px solid var(--ds-border);">
                  {row.title || '(empty)'}
                </td>
                {#if mapping.description >= 0}
                  <td class="px-3 py-1.5 whitespace-nowrap max-w-[200px] truncate" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">{row.description}</td>
                {/if}
                {#if mapping.assetTag >= 0}
                  <td class="px-3 py-1.5 whitespace-nowrap" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">{row.assetTag}</td>
                {/if}
                {#if mapping.categoryId >= 0}
                  <td class="px-3 py-1.5 whitespace-nowrap" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">{row.category}</td>
                {/if}
                {#if mapping.statusId >= 0}
                  <td class="px-3 py-1.5 whitespace-nowrap" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">{row.status}</td>
                {/if}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>
