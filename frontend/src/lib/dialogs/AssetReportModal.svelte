<script>
  import { api } from '../api.js';
  import BasePicker from '../pickers/BasePicker.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import Textarea from '../components/Textarea.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import PortalModal from './PortalModal.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { iconMap as portalIconMap } from '../stores/portal.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  const portalIconOptions = Object.keys(portalIconMap).sort();

  let {
    isOpen = false,
    mode = 'create',
    assetReport = null,
    channelId = null,
    channelWorkspaceIds = [],
    isDarkMode = false,
    onsaved = undefined,
    onclose = undefined
  } = $props();

  let submitting = $state(false);
  let error = $state(null);
  let success = $state(false);
  let availableAssetSets = $state([]);
  let availableItemTypes = $state([]);
  let availableWorkspaces = $state([]);

  let formData = $state({
    name: '',
    description: '',
    icon: 'Table2',
    color: '#6b7280',
    asset_set_id: null,
    cql_query: '',
    run_mode: 'direct',
    item_type_id: null,
    workspace_id: null,
    submit_button_text: '',
    success_message: ''
  });

  let isFormInitialized = $state(false);
  let lastOpenState = $state(false);

  async function loadPickers() {
    try {
      const [sets, itemTypes, workspaces] = await Promise.all([
        api.assetSets.getAll(),
        api.itemTypes.getAll(),
        api.workspaces.getAll()
      ]);
      availableAssetSets = sets || [];
      availableItemTypes = itemTypes || [];
      if (channelWorkspaceIds && channelWorkspaceIds.length > 0) {
        availableWorkspaces = (workspaces || []).filter((ws) => channelWorkspaceIds.includes(ws.id));
      } else {
        availableWorkspaces = workspaces || [];
      }
    } catch (err) {
      console.error('Failed to load modal data:', err);
    }
  }

  function parseConfig(cfg) {
    if (!cfg) return {};
    if (typeof cfg === 'string') {
      try { return JSON.parse(cfg); } catch { return {}; }
    }
    return cfg;
  }

  $effect(() => {
    if (isOpen !== lastOpenState) {
      lastOpenState = isOpen;

      if (isOpen) {
        if (!isFormInitialized) {
          if (mode === 'edit' && assetReport) {
            const cfg = parseConfig(assetReport.config);
            formData = {
              name: assetReport.name || '',
              description: assetReport.description || '',
              icon: assetReport.icon || 'Table2',
              color: assetReport.color || '#6b7280',
              asset_set_id: assetReport.asset_set_id || null,
              cql_query: assetReport.cql_query || '',
              run_mode: assetReport.run_mode || 'direct',
              item_type_id: assetReport.item_type_id || null,
              workspace_id: assetReport.workspace_id || null,
              submit_button_text: cfg.submit_button_text || '',
              success_message: cfg.success_message || ''
            };
          } else {
            formData = {
              name: '',
              description: '',
              icon: 'Table2',
              color: '#6b7280',
              asset_set_id: null,
              cql_query: '',
              run_mode: 'direct',
              item_type_id: null,
              workspace_id: null,
              submit_button_text: '',
              success_message: ''
            };
          }
          isFormInitialized = true;
          loadPickers();
        }
        error = null;
        success = false;
      } else {
        isFormInitialized = false;
        error = null;
        success = false;
      }
    }
  });

  async function handleSubmit() {
    if (!formData.name.trim()) {
      error = t('portal.nameRequired');
      return;
    }
    if (!formData.asset_set_id) {
      error = t('portal.assetSetRequired');
      return;
    }
    if (!formData.cql_query.trim()) {
      error = t('portal.qlQueryRequired');
      return;
    }
    if (formData.run_mode === 'form' && !formData.item_type_id) {
      error = t('portal.itemTypeRequired');
      return;
    }
    if (formData.run_mode === 'form' && !/\{[a-zA-Z0-9_]+\}/.test(formData.cql_query)) {
      error = t('portal.qlQueryTokenRequired');
      return;
    }

    try {
      submitting = true;
      error = null;

      const configObj = {};
      if (formData.submit_button_text.trim()) configObj.submit_button_text = formData.submit_button_text.trim();
      if (formData.success_message.trim()) configObj.success_message = formData.success_message.trim();
      const configJson = Object.keys(configObj).length > 0 ? JSON.stringify(configObj) : null;

      const payload = {
        name: formData.name.trim(),
        description: formData.description.trim(),
        icon: formData.icon,
        color: formData.color,
        asset_set_id: formData.asset_set_id,
        cql_query: formData.cql_query.trim(),
        run_mode: formData.run_mode,
        item_type_id: formData.run_mode === 'form' ? formData.item_type_id : null,
        workspace_id: formData.run_mode === 'form' ? formData.workspace_id : null,
        config: configJson,
        is_active: true
      };

      if (mode === 'create') {
        await api.assetReports.create(channelId, payload);
      } else {
        await api.assetReports.update(channelId, assetReport.id, {
          ...payload,
          column_config: assetReport.column_config,
          visibility_group_ids: assetReport.visibility_group_ids,
          visibility_org_ids: assetReport.visibility_org_ids,
          display_order: assetReport.display_order
        });
      }

      success = true;
      handleClose();
      onsaved?.();
    } catch (err) {
      console.error('Failed to save asset report:', err);
      error = err.message || t('portal.failedToSaveAssetReport');
    } finally {
      submitting = false;
    }
  }

  function handleClose() {
    onclose?.();
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
        <AlertBox variant="error" message={error} class="mb-4" />
      {/if}

      <div class="space-y-4">
        <!-- Mode selector -->
        <div>
          <label class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('portal.runMode')}
          </label>
          <div class="grid grid-cols-2 gap-2">
            <button
              type="button"
              class="px-3 py-2 rounded border text-sm text-left transition-all"
              style="border-color: {formData.run_mode === 'direct' ? '#3b82f6' : (isDarkMode ? '#475569' : '#d1d5db')}; background-color: {formData.run_mode === 'direct' ? (isDarkMode ? 'rgba(59,130,246,0.15)' : '#eff6ff') : 'transparent'}; color: {isDarkMode ? '#e2e8f0' : '#111827'};"
              onclick={() => formData.run_mode = 'direct'}
            >
              <div class="font-medium">{t('portal.runModeDirect')}</div>
              <div class="text-xs opacity-75">{t('portal.runModeDirectHint')}</div>
            </button>
            <button
              type="button"
              class="px-3 py-2 rounded border text-sm text-left transition-all"
              style="border-color: {formData.run_mode === 'form' ? '#3b82f6' : (isDarkMode ? '#475569' : '#d1d5db')}; background-color: {formData.run_mode === 'form' ? (isDarkMode ? 'rgba(59,130,246,0.15)' : '#eff6ff') : 'transparent'}; color: {isDarkMode ? '#e2e8f0' : '#111827'};"
              onclick={() => formData.run_mode = 'form'}
            >
              <div class="font-medium">{t('portal.runModeForm')}</div>
              <div class="text-xs opacity-75">{t('portal.runModeFormHint')}</div>
            </button>
          </div>
        </div>

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
            rows={3}
            placeholder={t('portal.assetReportDescriptionPlaceholder')}
          />
        </div>

        <div>
          <IconSelector
            bind:selectedIcon={formData.icon}
            bind:selectedColor={formData.color}
            label={t('portal.iconAndColor')}
            compact={true}
            iconMap={portalIconMap}
            iconOptions={portalIconOptions}
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
            {t('portal.assetSetHint')}
          </p>
        </div>

        <div>
          <label for="ar-cql" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('portal.qlQuery')} <span class="text-red-500">*</span>
          </label>
          <Textarea
            id="ar-cql"
            bind:value={formData.cql_query}
            rows={4}
            placeholder={formData.run_mode === 'form' ? t('portal.qlQueryFormPlaceholder') : t('portal.qlQueryPlaceholder')}
          />
          <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
            {formData.run_mode === 'form' ? t('portal.qlQueryFormHint') : t('portal.qlQueryHint')}
          </p>
        </div>

        {#if formData.run_mode === 'form'}
          <div class="pt-4 border-t" style="border-color: {isDarkMode ? '#334155' : '#e5e7eb'};">
            <h3 class="text-sm font-semibold mb-3" style="color: {isDarkMode ? '#e2e8f0' : '#111827'};">
              {t('portal.formConfiguration')}
            </h3>

            <div class="space-y-4">
              <div>
                <label for="ar-itemtype" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
                  {t('portal.itemType')} <span class="text-red-500">*</span>
                </label>
                <BasePicker
                  bind:value={formData.item_type_id}
                  items={availableItemTypes}
                  placeholder={t('portal.selectItemType')}
                  getValue={(item) => item.id}
                  getLabel={(item) => item.name}
                />
                <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                  {t('portal.assetReportItemTypeHint')}
                </p>
              </div>

              <div>
                <label for="ar-workspace" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
                  {t('common.workspace')}
                </label>
                <BasePicker
                  bind:value={formData.workspace_id}
                  items={availableWorkspaces}
                  placeholder={t('portal.selectWorkspace', 'Select workspace')}
                  getValue={(item) => item.id}
                  getLabel={(item) => item.name}
                  allowClear={true}
                />
                <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                  {t('portal.workspaceFieldResolution', 'Used to resolve available custom fields from the workspace configuration.')}
                </p>
              </div>

              <div>
                <label for="ar-submit" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
                  {t('portal.submitButtonLabel')}
                </label>
                <input
                  id="ar-submit"
                  bind:value={formData.submit_button_text}
                  type="text"
                  class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
                  style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
                  placeholder={t('portal.submitButtonPlaceholder')}
                />
              </div>

              <div>
                <label for="ar-success" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
                  {t('portal.successMessage')}
                </label>
                <Textarea
                  id="ar-success"
                  bind:value={formData.success_message}
                  rows={2}
                  placeholder={t('portal.successMessagePlaceholder')}
                />
              </div>

              {#if mode === 'edit'}
                <p class="text-xs" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                  {t('portal.assetReportConfigureFieldsHint')}
                </p>
              {:else}
                <p class="text-xs" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
                  {t('portal.assetReportFieldsAfterCreate')}
                </p>
              {/if}
            </div>
          </div>
        {/if}
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
