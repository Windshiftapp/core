<script>
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import Textarea from '../components/Textarea.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Label from '../components/Label.svelte';
  import PortalModal from './PortalModal.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = $bindable(false),
    report = null,
    portalSlug = '',
    isDarkMode = false,
    onclose = () => {}
  } = $props();

  let fields = $state([]);
  let customFieldDefinitions = $state([]);
  let values = $state({});
  let loading = $state(false);
  let submitting = $state(false);
  let error = $state(null);
  let results = $state(null);

  function parseConfig(cfg) {
    if (!cfg) return {};
    if (typeof cfg === 'string') {
      try { return JSON.parse(cfg); } catch { return {}; }
    }
    return cfg;
  }

  const config = $derived(parseConfig(report?.config));
  const submitLabel = $derived(config.submit_button_text || t('portal.runReport'));
  const successMessage = $derived(config.success_message || '');

  const defaultFieldRows = $derived(fields.filter((f) => f.field_type === 'default'));
  const customFieldRows = $derived(fields.filter((f) => f.field_type === 'custom'));
  const virtualFieldRows = $derived(fields.filter((f) => f.field_type === 'virtual'));

  $effect(() => {
    if (isOpen && report) {
      loadFields();
    } else if (!isOpen) {
      reset();
    }
  });

  function reset() {
    fields = [];
    values = {};
    customFieldDefinitions = [];
    error = null;
    results = null;
    submitting = false;
  }

  async function loadFields() {
    try {
      loading = true;
      error = null;
      const [fieldList, defs] = await Promise.all([
        api.assetReports.getPortalFields(portalSlug, report.id),
        api.portal.getCustomFields(portalSlug).catch(() => [])
      ]);
      fields = fieldList || [];
      customFieldDefinitions = defs || [];

      const initial = {};
      for (const f of fields) {
        if (f.field_type === 'virtual' && f.virtual_field_type === 'checkbox') {
          initial[f.field_identifier] = false;
        } else {
          initial[f.field_identifier] = '';
        }
      }
      values = initial;
    } catch (err) {
      console.error('Failed to load asset report fields:', err);
      error = err.message || t('requestForm.failedToLoadFields');
    } finally {
      loading = false;
    }
  }

  function getCustomFieldDefinition(fieldId) {
    return customFieldDefinitions.find((f) => String(f.id) === String(fieldId));
  }

  function getFieldLabel(field) {
    return field.display_name || field.field_label || field.field_name || field.field_identifier;
  }

  function parseSelectOptions(optionsJson) {
    try {
      const parsed = typeof optionsJson === 'string' ? JSON.parse(optionsJson) : optionsJson;
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  }

  function validateRequired() {
    for (const f of fields) {
      if (!f.is_required) continue;
      const v = values[f.field_identifier];
      if (f.field_type === 'virtual' && f.virtual_field_type === 'checkbox') continue;
      if (v === '' || v === null || v === undefined || (Array.isArray(v) && v.length === 0)) {
        return t('portal.fieldRequired', { field: getFieldLabel(f) });
      }
    }
    return null;
  }

  // Normalize rich field values (user/asset/org pickers return objects) into CQL-friendly primitives.
  function normalizeForCQL(v) {
    if (v === null || v === undefined) return undefined;
    if (Array.isArray(v)) {
      const mapped = v
        .map(normalizeForCQL)
        .filter((x) => x !== undefined && x !== '');
      return mapped.length > 0 ? mapped : undefined;
    }
    if (typeof v === 'object') {
      if ('id' in v && v.id != null) return v.id;
      return undefined;
    }
    return v;
  }

  async function handleSubmit() {
    const validationError = validateRequired();
    if (validationError) {
      error = validationError;
      return;
    }
    try {
      submitting = true;
      error = null;
      const params = {};
      for (const f of fields) {
        const raw = values[f.field_identifier];
        const normalized = normalizeForCQL(raw);
        if (normalized === undefined || normalized === '') continue;
        params[f.field_identifier] = normalized;
      }
      results = await api.assetReports.submit(portalSlug, report.id, { params, page: 1, perPage: 50 });
    } catch (err) {
      console.error('Failed to run asset report:', err);
      error = err.message || t('portal.failedToRunReport');
    } finally {
      submitting = false;
    }
  }

  function close() {
    onclose?.();
  }
</script>

{#if isOpen && report}
  <PortalModal
    isOpen={isOpen}
    isDarkMode={isDarkMode}
    maxWidth="max-w-3xl"
    title={report.name}
    subtitle={report.description}
    onClose={close}
    bodyClass="px-6 py-4 max-h-[75vh] overflow-y-auto"
  >
    {#if loading}
      <div class="flex items-center justify-center py-12">
        <Spinner size="lg" />
      </div>
    {:else}
      {#if error}
        <AlertBox variant="error" message={error} class="mb-4" />
      {/if}

      {#if !results}
        {#if fields.length === 0}
          <AlertBox variant="info" message={t('portal.noFieldsConfigured')} class="mb-4" />
          <div class="flex justify-end gap-2">
            <Button variant="default" onclick={close}>{t('common.cancel')}</Button>
            <Button variant="primary" onclick={handleSubmit} disabled={submitting}>
              {submitting ? t('common.submitting') : submitLabel}
            </Button>
          </div>
        {:else}
          <div class="space-y-4">
            <!-- Default fields (title, description) — treated as free-form params -->
            {#each defaultFieldRows as field (field.id)}
              <div>
                <Label required={field.is_required} class="mb-2">
                  {getFieldLabel(field)}
                </Label>
                {#if field.field_identifier === 'description'}
                  <Textarea
                    bind:value={values[field.field_identifier]}
                    rows={3}
                    placeholder={getFieldLabel(field)}
                  />
                {:else}
                  <input
                    type="text"
                    bind:value={values[field.field_identifier]}
                    class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
                    style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
                    placeholder={getFieldLabel(field)}
                  />
                {/if}
                {#if field.description}
                  <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                    {field.description}
                  </p>
                {/if}
              </div>
            {/each}

            <!-- Custom fields — rendered with CustomFieldRenderer -->
            {#each customFieldRows as field (field.id)}
              {@const fieldDef = getCustomFieldDefinition(field.field_identifier)}
              {#if fieldDef}
                <div>
                  {#if field.display_name || field.description}
                    <Label required={field.is_required} class="mb-2">
                      {field.display_name || fieldDef.name}
                    </Label>
                    {#if field.description}
                      <p class="text-xs mb-2" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                        {field.description}
                      </p>
                    {/if}
                    <CustomFieldRenderer
                      field={{ ...fieldDef, is_required: field.is_required, name: '' }}
                      value={values[field.field_identifier]}
                      readonly={false}
                      onChange={(val) => (values[field.field_identifier] = val)}
                      milestones={[]}
                      {isDarkMode}
                    />
                  {:else}
                    <CustomFieldRenderer
                      field={{ ...fieldDef, is_required: field.is_required }}
                      value={values[field.field_identifier]}
                      readonly={false}
                      onChange={(val) => (values[field.field_identifier] = val)}
                      milestones={[]}
                      {isDarkMode}
                    />
                  {/if}
                </div>
              {/if}
            {/each}

            <!-- Virtual fields -->
            {#each virtualFieldRows as field (field.id)}
              <div>
                <Label required={field.is_required} class="mb-2">
                  {getFieldLabel(field)}
                </Label>
                {#if field.description}
                  <p class="text-xs mb-2" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                    {field.description}
                  </p>
                {/if}

                {#if field.virtual_field_type === 'textarea'}
                  <Textarea
                    bind:value={values[field.field_identifier]}
                    rows={4}
                    placeholder={getFieldLabel(field)}
                  />
                {:else if field.virtual_field_type === 'select'}
                  <BasePicker
                    bind:value={values[field.field_identifier]}
                    items={parseSelectOptions(field.virtual_field_options)}
                    placeholder={t('requestForm.selectOption')}
                    showUnassigned={true}
                    unassignedLabel={t('requestForm.selectOption')}
                    getValue={(option) => option.value}
                    getLabel={(option) => option.label}
                  />
                {:else if field.virtual_field_type === 'checkbox'}
                  <label class="flex items-center gap-3 cursor-pointer">
                    <input
                      type="checkbox"
                      bind:checked={values[field.field_identifier]}
                      class="h-5 w-5 rounded border-gray-300 focus:ring-2 focus:ring-blue-500"
                    />
                    <span class="text-sm" style="color: {isDarkMode ? '#e2e8f0' : '#374151'};">
                      {getFieldLabel(field)}
                    </span>
                  </label>
                {:else if field.virtual_field_type === 'number'}
                  <input
                    type="number"
                    bind:value={values[field.field_identifier]}
                    class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
                    style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
                    placeholder={getFieldLabel(field)}
                  />
                {:else}
                  <input
                    type="text"
                    bind:value={values[field.field_identifier]}
                    class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
                    style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
                    placeholder={getFieldLabel(field)}
                  />
                {/if}
              </div>
            {/each}
          </div>

          <div class="flex justify-end gap-2 mt-6">
            <Button variant="default" onclick={close}>{t('common.cancel')}</Button>
            <Button variant="primary" onclick={handleSubmit} disabled={submitting}>
              {submitting ? t('common.submitting') : submitLabel}
            </Button>
          </div>
        {/if}
      {:else}
        {#if successMessage}
          <AlertBox variant="success" message={successMessage} class="mb-4" />
        {/if}

        <div class="mb-3 flex items-center justify-between">
          <div class="text-sm" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
            {t('portal.resultsCount', { total: results.total })}
          </div>
          <Button variant="default" size="sm" onclick={() => { results = null; }}>
            {t('portal.editCriteria')}
          </Button>
        </div>

        {#if results.assets.length === 0}
          <div class="text-center py-12 rounded border" style="border-color: {isDarkMode ? '#334155' : '#e5e7eb'}; color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
            {t('portal.noAssetsFound')}
          </div>
        {:else}
          <div class="overflow-x-auto rounded border" style="border-color: {isDarkMode ? '#334155' : '#e5e7eb'};">
            <table class="min-w-full text-sm">
              <thead style="background-color: {isDarkMode ? '#1e293b' : '#f9fafb'};">
                <tr>
                  {#each results.columns as col}
                    <th class="px-4 py-2 text-left font-medium" style="color: {isDarkMode ? '#94a3b8' : '#374151'};">{col}</th>
                  {/each}
                </tr>
              </thead>
              <tbody>
                {#each results.assets as asset}
                  <tr class="border-t" style="border-color: {isDarkMode ? '#334155' : '#e5e7eb'};">
                    {#each results.columns as col}
                      {@const val = col === 'title' ? asset.title :
                                    col === 'asset_tag' ? asset.asset_tag :
                                    col === 'status_id' ? (asset.status_name || '') :
                                    col === 'asset_type_id' ? (asset.asset_type_name || '') :
                                    col.startsWith('cf_') ? (asset.custom_field_values?.[col.slice(3)] ?? '') :
                                    ''}
                      <td class="px-4 py-2" style="color: {isDarkMode ? '#e2e8f0' : '#111827'};">{val ?? ''}</td>
                    {/each}
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}

        <div class="flex justify-end mt-6">
          <Button variant="default" onclick={close}>{t('common.close')}</Button>
        </div>
      {/if}
    {/if}
  </PortalModal>
{/if}
