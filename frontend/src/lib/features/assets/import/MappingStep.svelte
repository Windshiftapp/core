<script>
  import { assetImportStore } from './AssetImportStore.svelte.js';
  import Select from '../../../components/Select.svelte';
  import Button from '../../../components/Button.svelte';
  import Badge from '../../../components/Badge.svelte';
  import { Wand2 } from 'lucide-svelte';

  let upload = $derived(assetImportStore.upload);
  let target = $derived(assetImportStore.target);
  let mapping = $derived(assetImportStore.mapping);

  let headers = $derived(upload.headers || []);

  // Standard fields
  const standardFields = [
    { key: 'title', label: 'Title', required: true },
    { key: 'description', label: 'Description', required: false },
    { key: 'assetTag', label: 'Asset Tag', required: false },
    { key: 'categoryId', label: 'Category', required: false },
    { key: 'statusId', label: 'Status', required: false },
  ];

  function handleFieldChange(key, e) {
    assetImportStore.setFieldMapping(key, parseInt(e.target.value));
  }

  function handleCustomFieldChange(fieldId, e) {
    assetImportStore.setCustomFieldMapping(fieldId, parseInt(e.target.value));
  }

  // Get unique values for a mapped column (for category/status value mapping)
  function getUniqueValues(columnIndex) {
    if (columnIndex < 0) return [];
    const values = new Set();
    for (const row of upload.previewRows) {
      if (row[columnIndex]?.trim()) {
        values.add(row[columnIndex].trim());
      }
    }
    return Array.from(values).sort();
  }

  let categoryValues = $derived(getUniqueValues(mapping.categoryId));
  let statusValues = $derived(getUniqueValues(mapping.statusId));
</script>

<div class="flex gap-6">
  <!-- Mapping controls (left) -->
  <div class="flex-1 space-y-6 min-w-0">
    <div class="flex items-center justify-between">
      <h3 class="text-sm font-semibold" style="color: var(--ds-text);">Field Mapping</h3>
      <Button variant="ghost" size="small" onclick={() => assetImportStore.autoMap()}>
        <Wand2 class="w-3.5 h-3.5 mr-1" />
        Auto-map
      </Button>
    </div>

    <!-- Standard fields -->
    <div class="space-y-3">
      <p class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Standard Fields</p>
      {#each standardFields as field}
        <div class="flex items-center gap-3">
          <div class="w-32 flex-shrink-0 flex items-center gap-1.5">
            <span class="text-sm" style="color: var(--ds-text);">{field.label}</span>
            {#if field.required}
              <span class="text-red-500 text-xs">*</span>
            {/if}
          </div>
          <Select value={mapping[field.key] ?? -1} onchange={(e) => handleFieldChange(field.key, e)} size="small" class="flex-1">
            <option value={-1}>Not mapped</option>
            {#each headers as header, i}
              <option value={i}>{header}</option>
            {/each}
          </Select>
        </div>
      {/each}
    </div>

    <!-- Custom fields -->
    {#if target.typeFields.length > 0}
      <div class="space-y-3">
        <p class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Custom Fields</p>
        {#each target.typeFields as field}
          <div class="flex items-center gap-3">
            <div class="w-32 flex-shrink-0 flex items-center gap-1.5">
              <span class="text-sm" style="color: var(--ds-text);">{field.name || field.field_name}</span>
              {#if field.is_required}
                <span class="text-red-500 text-xs">*</span>
              {/if}
              <Badge variant="subtle" size="small">{field.field_type || field.type}</Badge>
            </div>
            <Select
              value={mapping.customFields[String(field.custom_field_id || field.id)] ?? -1}
              onchange={(e) => handleCustomFieldChange(field.custom_field_id || field.id, e)}
              size="small"
              class="flex-1"
            >
              <option value={-1}>Not mapped</option>
              {#each headers as header, i}
                <option value={i}>{header}</option>
              {/each}
            </Select>
          </div>
        {/each}
      </div>
    {/if}

    <!-- Category value mapping -->
    {#if mapping.categoryId >= 0 && categoryValues.length > 0 && target.categories.length > 0}
      <div class="space-y-3">
        <p class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Category Value Mapping</p>
        <p class="text-xs" style="color: var(--ds-text-subtle);">Map CSV category values to existing categories</p>
        {#each categoryValues as value}
          <div class="flex items-center gap-3">
            <div class="w-32 flex-shrink-0">
              <span class="text-sm font-mono" style="color: var(--ds-text);">{value}</span>
            </div>
            <Select
              value={mapping.categoryMap[value] ?? ''}
              onchange={(e) => assetImportStore.setCategoryValueMapping(value, parseInt(e.target.value) || null)}
              size="small"
              class="flex-1"
            >
              <option value="">Skip</option>
              {#each target.categories as cat}
                <option value={cat.id}>{cat.path ? `${cat.path}${cat.name}` : cat.name}</option>
              {/each}
            </Select>
          </div>
        {/each}
      </div>
    {/if}

    <!-- Status value mapping -->
    {#if mapping.statusId >= 0 && statusValues.length > 0 && target.statuses.length > 0}
      <div class="space-y-3">
        <p class="text-xs font-medium uppercase tracking-wide" style="color: var(--ds-text-subtle);">Status Value Mapping</p>
        <p class="text-xs" style="color: var(--ds-text-subtle);">Map CSV status values to existing statuses</p>
        {#each statusValues as value}
          <div class="flex items-center gap-3">
            <div class="w-32 flex-shrink-0">
              <span class="text-sm font-mono" style="color: var(--ds-text);">{value}</span>
            </div>
            <Select
              value={mapping.statusMap[value] ?? ''}
              onchange={(e) => assetImportStore.setStatusValueMapping(value, parseInt(e.target.value) || null)}
              size="small"
              class="flex-1"
            >
              <option value="">Use default</option>
              {#each target.statuses as status}
                <option value={status.id}>{status.name}</option>
              {/each}
            </Select>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- CSV preview (right) -->
  {#if upload.previewRows.length > 0}
    <div class="w-[40%] flex-shrink-0">
      <p class="text-xs font-medium uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">CSV Data Preview</p>
      <div class="overflow-x-auto rounded border" style="border-color: var(--ds-border);">
        <table class="w-full text-xs">
          <thead>
            <tr style="background: var(--ds-background-input);">
              {#each headers as header}
                <th class="px-2 py-1.5 text-left font-medium whitespace-nowrap" style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);">
                  {header}
                </th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each upload.previewRows as row}
              <tr>
                {#each row as cell}
                  <td class="px-2 py-1 whitespace-nowrap max-w-[120px] truncate" style="color: var(--ds-text); border-bottom: 1px solid var(--ds-border);">
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
</div>
