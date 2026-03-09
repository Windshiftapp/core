<script>
  import { createEventDispatcher } from 'svelte';
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import Textarea from '../components/Textarea.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import PortalModal from './PortalModal.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import Checkbox from '../components/Checkbox.svelte';

  const dispatch = createEventDispatcher();

  export let isOpen = false;
  export let mode = 'create'; // 'create' or 'edit'
  /** @type {{ name: string, description: string, icon: string, color: string, asset_set_id: string|null, cql_query: string, column_config: any[], is_active: boolean } | null} */
  export let assetReport = null;
  export let channelId = null;
  export let availableAssetSets = [];
  export let isDarkMode = false;

  let submitting = false;
  let error = null;
  let success = false;

  // Form data
  let formData = {
    name: '',
    description: '',
    icon: 'Table2',
    color: '#6b7280',
    asset_set_id: null,
    cql_query: '',
    column_config: [],
    is_active: true
  };

  // Track if form has been initialized to prevent re-initialization
  let isFormInitialized = false;
  let lastOpenState = false;

  // Consolidated reactive statement to handle modal state changes
  $: {
    if (isOpen !== lastOpenState) {
      lastOpenState = isOpen;

      if (isOpen) {
        if (!isFormInitialized) {
          if (mode === 'edit' && assetReport) {
            formData = {
              name: assetReport.name || '',
              description: assetReport.description || '',
              icon: assetReport.icon || 'Table2',
              color: assetReport.color || '#6b7280',
              asset_set_id: assetReport.asset_set_id || null,
              cql_query: assetReport.cql_query || '',
              column_config: assetReport.column_config || [],
              is_active: assetReport.is_active !== false
            };
          } else if (mode === 'create') {
            formData = {
              name: '',
              description: '',
              icon: 'Table2',
              color: '#6b7280',
              asset_set_id: availableAssetSets.length > 0 ? availableAssetSets[0].id : null,
              cql_query: '',
              column_config: [],
              is_active: true
            };
          }
          isFormInitialized = true;
        }
        error = null;
        success = false;
      } else {
        formData = {
          name: '',
          description: '',
          icon: 'Table2',
          color: '#6b7280',
          asset_set_id: null,
          cql_query: '',
          column_config: [],
          is_active: true
        };
        error = null;
        success = false;
        isFormInitialized = false;
      }
    }
  }

  async function handleSubmit() {
    try {
      if (!formData.name.trim()) {
        error = t('portal.nameRequired');
        return;
      }

      if (!formData.asset_set_id) {
        error = t('portal.assetSetRequired');
        return;
      }

      submitting = true;
      error = null;

      if (mode === 'create') {
        await api.assetReports.create(channelId, {
          name: formData.name.trim(),
          description: formData.description.trim(),
          icon: formData.icon,
          color: formData.color,
          asset_set_id: formData.asset_set_id,
          cql_query: formData.cql_query.trim(),
          column_config: formData.column_config,
          is_active: formData.is_active
        });
      } else {
        await api.assetReports.update(assetReport.id, {
          name: formData.name.trim(),
          description: formData.description.trim(),
          icon: formData.icon,
          color: formData.color,
          asset_set_id: formData.asset_set_id,
          cql_query: formData.cql_query.trim(),
          column_config: formData.column_config,
          is_active: formData.is_active
        });
      }

      success = true;
      handleClose();
      dispatch('saved');
    } catch (err) {
      console.error('Failed to save asset report:', err);
      error = err.message || t('portal.failedToSaveAssetReport');
    } finally {
      submitting = false;
    }
  }

  function handleClose() {
    dispatch('close');
  }
</script>

{#if isOpen}
  <PortalModal
    isOpen={isOpen}
    isDarkMode={isDarkMode}
    maxWidth="max-w-2xl"
    title={mode === 'create' ? t('portal.createAssetReport') : t('portal.editAssetReport')}
    subtitle={mode === 'create' ? t('portal.addAssetReportSubtitle') : t('portal.editAssetReportSubtitle')}
    onClose={handleClose}
    bodyClass="px-6 py-4 max-h-[60vh] overflow-y-auto"
  >
    {#if success}
      <div class="mb-4">
        <AlertBox variant="success" message={mode === 'create' ? t('portal.assetReportCreated') : t('portal.assetReportUpdated')} />
      </div>
    {:else}
      {#if error}
        <div
          class="mb-4 p-3 rounded border"
          style="background-color: {isDarkMode ? 'rgba(239, 68, 68, 0.1)' : '#fef2f2'}; border-color: {isDarkMode ? 'rgba(239, 68, 68, 0.3)' : '#fecaca'};"
        >
          <p class="text-sm" style="color: {isDarkMode ? '#fca5a5' : '#dc2626'};">
            {error}
          </p>
        </div>
      {/if}

      <div class="space-y-4">
        <div>
          <label for="ar-name" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('common.name')} <span class="text-red-500">*</span>
          </label>
          <input
            id="ar-name"
            bind:value={formData.name}
            type="text"
            class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
            style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
            placeholder={t('portal.assetReportNamePlaceholder')}
            required
          />
        </div>

        <div>
          <label for="ar-description" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('portal.descriptionOptional')}
          </label>
          <Textarea
            id="ar-description"
            bind:value={formData.description}
            rows={2}
            placeholder={t('portal.assetReportDescriptionPlaceholder')}
          />
        </div>

        <div>
          <IconSelector
            bind:selectedIcon={formData.icon}
            bind:selectedColor={formData.color}
            label={t('portal.iconAndColor')}
          />
        </div>

        <div>
          <label for="ar-assetset" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('portal.assetSet')} <span class="text-red-500">*</span>
          </label>
          <BasePicker
            bind:value={formData.asset_set_id}
            items={availableAssetSets}
            placeholder={t('portal.selectAssetSet')}
            getValue={(item) => item.id}
            getLabel={(item) => item.name}
          />
          <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
            {t('portal.assetSetDescription')}
          </p>
        </div>

        <div>
          <label for="ar-cql" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('portal.cqlQuery')}
          </label>
          <Textarea
            id="ar-cql"
            bind:value={formData.cql_query}
            rows={3}
            placeholder={t('portal.cqlQueryPlaceholder')}
          />
          <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
            {t('portal.cqlQueryHint')}
          </p>
        </div>

        <Checkbox
          bind:checked={formData.is_active}
          label={t('portal.activeReport')}
          size="small"
        />
      </div>

      <DialogFooter
        onCancel={handleClose}
        onConfirm={handleSubmit}
        confirmLabel={mode === 'create' ? t('portal.createAssetReport') : t('common.saveChanges')}
        loading={submitting}
        loadingLabel={mode === 'create' ? t('portal.creating') : t('common.saving')}
        class="mt-6 -mx-6 -mb-4"
      />
    {/if}
  </PortalModal>
{/if}
