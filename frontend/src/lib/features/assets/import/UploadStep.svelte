<script>
  import { assetImportStore } from './AssetImportStore.svelte.js';
  import Select from '../../../components/Select.svelte';
  import Input from '../../../components/Input.svelte';
  import Checkbox from '../../../components/Checkbox.svelte';
  import Button from '../../../components/Button.svelte';
  import AlertBox from '../../../components/AlertBox.svelte';
  import Spinner from '../../../components/Spinner.svelte';
  import { IconUpload, IconFileSpreadsheet, IconPlus, IconSparkles, IconX } from '@tabler/icons-svelte-runes';
  import DescriptionText from '../../../components/DescriptionText.svelte';

  let upload = $derived(assetImportStore.upload);
  let target = $derived(assetImportStore.target);
  let ct = $derived(assetImportStore.createType);

  let dragOver = $state(false);
  let fileInput = $state(null);

  const fieldTypeOptions = [
    { value: 'text', label: 'Text' },
    { value: 'textarea', label: 'Textarea' },
    { value: 'number', label: 'Number' },
    { value: 'date', label: 'Date' },
    { value: 'select', label: 'Select' },
  ];

  function handleDrop(e) {
    e.preventDefault();
    dragOver = false;
    const file = e.dataTransfer?.files?.[0];
    if (file) handleFile(file);
  }

  function handleFileSelect(e) {
    const file = e.target.files?.[0];
    if (file) handleFile(file);
  }

  async function handleFile(file) {
    const ext = file.name.split('.').pop()?.toLowerCase();
    if (!['csv', 'tsv', 'txt'].includes(ext)) {
      upload.error = 'Please select a CSV or TSV file';
      return;
    }
    await assetImportStore.uploadFile(file);
  }

  function handleTypeChange(e) {
    assetImportStore.setAssetType(parseInt(e.target.value) || null);
  }

  function formatFileSize(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / 1048576).toFixed(1)} MB`;
  }
</script>

<div class="space-y-6">
  <!-- Asset Type Selection -->
  <div>
    <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text);">Asset Type <span class="text-red-500">*</span></label>
    <div class="flex items-center gap-2">
      <div class="flex-1">
        <Select
          value={target.assetTypeId || ''}
          onchange={(v) => handleTypeChange({ target: { value: v } })}
          size="small"
          disabled={ct.isOpen}
          placeholder="Select asset type..."
          options={[{ value: '', label: 'Select asset type...' }, ...target.types.map(t => ({ value: t.id, label: t.name }))]}
        />
      </div>
      {#if !ct.isOpen}
        <Button variant="ghost" size="small" onclick={() => assetImportStore.toggleCreateType()}>
          <IconPlus class="w-4 h-4" />
          Create New Type
        </Button>
      {/if}
    </div>
  </div>

  <!-- Create New Type Panel -->
  {#if ct.isOpen}
    <div class="rounded-lg border p-4 space-y-4" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
      <div class="flex items-center justify-between">
        <h4 class="text-sm font-semibold" style="color: var(--ds-text);">Create New Asset Type</h4>
        <button
          class="p-1 rounded hover:bg-gray-100 dark:hover:bg-gray-700"
          onclick={() => assetImportStore.toggleCreateType()}
        >
          <IconX class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        </button>
      </div>

      <!-- Type Name -->
      <div>
        <label class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">Type Name <span class="text-red-500">*</span></label>
        <Input
          size="small"
          placeholder="e.g. Laptop, Vehicle, License..."
          bind:value={ct.name}
        />
      </div>

      <!-- Suggest Fields Button -->
      <div>
        <Button
          variant="secondary"
          size="small"
          icon={IconSparkles}
          disabled={!upload.uploadId || ct.isLoadingSuggestions}
          loading={ct.isLoadingSuggestions}
          onclick={() => assetImportStore.suggestFields()}
        >
          Suggest Fields from CSV
        </Button>
        {#if !upload.uploadId}
          <DescriptionText>Upload a CSV file first to enable field suggestions</DescriptionText>
        {/if}
      </div>

      <!-- Custom Fields -->
      {#if ct.editedFields.length > 0}
        <div>
          <p class="text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">Custom Fields</p>
          <div class="space-y-2">
            {#each ct.editedFields as field, idx}
              <div class="flex items-start gap-2 p-2 rounded border" style="border-color: var(--ds-border); background: var(--ds-background-input);">
                <div class="pt-1">
                  <Checkbox
                    checked={field.enabled}
                    onchange={(e) => { ct.editedFields[idx].enabled = e.target.checked; }}
                  />
                </div>
                <div class="flex-1 min-w-0 space-y-1">
                  <div class="flex items-center gap-2">
                    <input
                      type="text"
                      class="flex-1 px-2 py-1 text-sm rounded border"
                      style="background: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                      bind:value={ct.editedFields[idx].name}
                      disabled={!field.enabled}
                    />
                    <Select
                      options={fieldTypeOptions}
                      bind:value={ct.editedFields[idx].field_type}
                      disabled={!field.enabled}
                      size="small"
                    />
                  </div>
                  {#if field.sample_values?.length > 0}
                    <p class="text-xs truncate" style="color: var(--ds-text-subtle);">
                      e.g. {field.sample_values.slice(0, 3).join(', ')}
                    </p>
                  {/if}
                  {#if field.field_type === 'select' && field.options}
                    <p class="text-xs" style="color: var(--ds-text-subtle);">
                      Options: {Array.isArray(field.options) ? field.options.join(', ') : (field.options.items || []).map(o => o.label).join(', ')}
                    </p>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      {#if ct.error}
        <AlertBox variant="error" message={ct.error} />
      {/if}

      <!-- Actions -->
      <div class="flex items-center justify-end gap-2 pt-2">
        <Button variant="default" size="small" onclick={() => assetImportStore.toggleCreateType()}>
          Cancel
        </Button>
        <Button
          variant="primary"
          size="small"
          disabled={!ct.name || ct.isCreating}
          loading={ct.isCreating}
          onclick={() => assetImportStore.createTypeFromImport()}
        >
          Create Type
        </Button>
      </div>
    </div>
  {/if}

  <!-- File Upload -->
  {#if !upload.uploadId}
    <div>
      <label class="block text-sm font-medium mb-1.5" style="color: var(--ds-text);">CSV File <span class="text-red-500">*</span></label>

      <!-- Drop zone -->
      <button
        type="button"
        class="w-full border-2 border-dashed rounded-lg p-8 text-center transition-colors cursor-pointer"
        style="border-color: {dragOver ? 'var(--ds-border-focused)' : 'var(--ds-border)'}; background: {dragOver ? 'var(--ds-background-input)' : 'transparent'};"
        ondragover={(e) => { e.preventDefault(); dragOver = true; }}
        ondragleave={() => { dragOver = false; }}
        ondrop={handleDrop}
        onclick={() => fileInput?.click()}
      >
        {#if upload.isUploading}
          <Spinner size="md" />
          <p class="mt-2 text-sm" style="color: var(--ds-text-subtle);">Uploading...</p>
        {:else}
          <IconUpload class="w-8 h-8 mx-auto mb-2" style="color: var(--ds-text-subtle);" />
          <p class="text-sm font-medium" style="color: var(--ds-text);">
            Drop your CSV file here or click to browse
          </p>
          <DescriptionText>
            Supports .csv, .tsv files
          </DescriptionText>
        {/if}
      </button>

      <input
        bind:this={fileInput}
        type="file"
        accept=".csv,.tsv,.txt"
        class="hidden"
        onchange={handleFileSelect}
      />
    </div>

    <!-- Header row toggle -->
    <Checkbox
      checked={upload.hasHeaderRow}
      onchange={(e) => { upload.hasHeaderRow = e.target.checked; }}
      label="First row contains column headers"
    />
  {:else}
    <!-- File info -->
    <div class="flex items-center gap-3 p-3 rounded-lg border" style="border-color: var(--ds-border); background: var(--ds-background-input);">
      <IconFileSpreadsheet class="w-5 h-5 flex-shrink-0" style="color: var(--ds-text-accent);" />
      <div class="flex-1 min-w-0">
        <p class="text-sm font-medium truncate" style="color: var(--ds-text);">{upload.fileName}</p>
        <p class="text-xs" style="color: var(--ds-text-subtle);">
          {formatFileSize(upload.fileSize)} &middot; {upload.totalRows} rows &middot; delimiter: {upload.delimiter === 'tab' ? 'Tab' : `"${upload.delimiter}"`}
        </p>
      </div>
      <button
        class="text-xs px-2 py-1 rounded border"
        style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
        onclick={() => {
          upload.uploadId = null;
          upload.headers = [];
          upload.previewRows = [];
        }}
      >
        Change
      </button>
    </div>

    <!-- CSV Preview -->
    {#if upload.previewRows.length > 0}
      <div>
        <p class="text-sm font-medium mb-2" style="color: var(--ds-text);">Preview</p>
        <div class="overflow-x-auto rounded border" style="border-color: var(--ds-border);">
          <table class="w-full text-xs">
            <thead>
              <tr style="background: var(--ds-background-input);">
                {#each upload.headers as header}
                  <th class="px-3 py-2 text-left font-medium whitespace-nowrap" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">
                    {header}
                  </th>
                {/each}
              </tr>
            </thead>
            <tbody>
              {#each upload.previewRows as row}
                <tr>
                  {#each row as cell}
                    <td class="px-3 py-1.5 whitespace-nowrap max-w-[200px] truncate" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">
                      {cell}
                    </td>
                  {/each}
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
    {/if}

    {#if upload.headerWarning}
      <AlertBox variant="warning" message={upload.headerWarning} />
    {/if}
  {/if}

  {#if upload.error}
    <AlertBox variant="error" message={upload.error} />
  {/if}
</div>
