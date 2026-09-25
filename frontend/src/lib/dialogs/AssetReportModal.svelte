<script>
  import { Trash2 } from '@lucide/svelte';
  import { api } from '../api.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import BasePicker from '../pickers/BasePicker.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import PortalModal from './PortalModal.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { iconMap as portalIconMap } from '../stores/portalPresentation.js';
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
  let availableRequestTypes = $state([]);
  // Request-type id → its fields, loaded lazily for the target-field picker.
  let requestTypeFields = $state({});

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
    success_message: '',
    row_actions: [],
    is_active: true
  });

  let isFormInitialized = $state(false);
  let lastOpenState = $state(false);

  async function loadPickers() {
    try {
      const [sets, itemTypes, requestTypes] = await Promise.all([
        api.assetSets.getAll(),
        api.itemTypes.getAll(),
        channelId ? api.requestTypes.getForChannel(channelId) : Promise.resolve([])
      ]);
      availableAssetSets = sets || [];
      availableItemTypes = itemTypes || [];
      availableRequestTypes = requestTypes || [];
      // A workspace load failure must not drop the other pickers.
      try {
        const workspaces = await workspacesStore.load();
        if (channelWorkspaceIds && channelWorkspaceIds.length > 0) {
          availableWorkspaces = (workspaces || []).filter((ws) => channelWorkspaceIds.includes(ws.id));
        } else {
          availableWorkspaces = workspaces || [];
        }
      } catch (err) {
        console.error('Failed to load workspaces:', err);
      }
      // Hydrate target-field options for existing row actions.
      await Promise.all(
        formData.row_actions.map((action) => loadRequestTypeFields(action.request_type_id))
      );
    } catch (err) {
      console.error('Failed to load modal data:', err);
    }
  }

  async function loadRequestTypeFields(requestTypeId) {
    if (!requestTypeId || requestTypeFields[requestTypeId]) return;
    try {
      const fields = await api.requestTypes.getFields(requestTypeId);
      requestTypeFields = { ...requestTypeFields, [requestTypeId]: fields || [] };
    } catch (err) {
      console.error('Failed to load request type fields:', err);
      requestTypeFields = { ...requestTypeFields, [requestTypeId]: [] };
    }
  }

  function requestTypeFieldItems(requestTypeId) {
    return requestTypeFields[requestTypeId] || [];
  }

  function addRowAction() {
    formData.row_actions = [
      ...formData.row_actions,
      {
        id: `ra_${Date.now()}_${formData.row_actions.length}`,
        label: '',
        request_type_id: null,
        target_field: '',
        source: 'asset_id'
      }
    ];
  }

  function removeRowAction(actionId) {
    formData.row_actions = formData.row_actions.filter((action) => action.id !== actionId);
  }

  // Changing the target request type invalidates the previously chosen field.
  function onRowActionRequestTypeChange(action) {
    action.target_field = '';
    loadRequestTypeFields(action.request_type_id);
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
              success_message: cfg.success_message || '',
              row_actions: Array.isArray(cfg.row_actions)
                ? cfg.row_actions.map((action) => ({ ...action }))
                : [],
              is_active: assetReport.is_active ?? true
            };
            requestTypeFields = {};
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
              success_message: '',
              row_actions: [],
              is_active: true
            };
            requestTypeFields = {};
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
    if (formData.run_mode === 'form' && !/\$\{[a-zA-Z0-9_-]+\}/.test(formData.cql_query)) {
      error = t('portal.qlQueryTokenRequired');
      return;
    }

    try {
      submitting = true;
      error = null;

      const configObj = {};
      if (formData.submit_button_text.trim()) configObj.submit_button_text = formData.submit_button_text.trim();
      if (formData.success_message.trim()) configObj.success_message = formData.success_message.trim();
      const rowActions = formData.row_actions
        .filter((action) => action.label?.trim() && action.request_type_id && action.target_field)
        .map((action) => ({
          id: action.id,
          label: action.label.trim(),
          request_type_id: action.request_type_id,
          target_field: action.target_field,
          source: action.source
        }));
      if (rowActions.length > 0) configObj.row_actions = rowActions;
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
        is_active: formData.is_active
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
          <div class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('portal.runMode')}
          </div>
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
            {t('common.name')} <span class="text-ds-text-danger">*</span>
          </label>
          <Input
            id="ar-name"
            bind:value={formData.name}
            type="text"
            placeholder={t('portal.assetReportNamePlaceholder')}
            required
            size="medium"
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
            {t('portal.assetSet')} <span class="text-ds-text-danger">*</span>
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
            {t('portal.qlQuery')} <span class="text-ds-text-danger">*</span>
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

        <Checkbox
          bind:checked={formData.is_active}
          label={t('common.active')}
          hint={t('portal.assetReportActiveHint')}
          dataTestid="asset-report-active"
        />

        {#if formData.run_mode === 'form'}
          <div class="pt-4 border-t" style="border-color: {isDarkMode ? '#334155' : '#e5e7eb'};">
            <h3 class="text-sm font-semibold mb-3" style="color: {isDarkMode ? '#e2e8f0' : '#111827'};">
              {t('portal.formConfiguration')}
            </h3>

            <div class="space-y-4">
              <div>
                <label for="ar-itemtype" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
                  {t('portal.itemType')} <span class="text-ds-text-danger">*</span>
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
                <Input
                  id="ar-submit"
                  bind:value={formData.submit_button_text}
                  type="text"
                  placeholder={t('portal.submitButtonPlaceholder')}
                  size="medium"
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

        <!-- Row actions. A row action renders a link on every result row that
             opens a request type with one field prefilled from that asset. -->
        <div class="pt-4 border-t" style="border-color: {isDarkMode ? '#334155' : '#e5e7eb'};">
          <div class="flex items-center justify-between mb-1">
            <h3 class="text-sm font-semibold" style="color: {isDarkMode ? '#e2e8f0' : '#111827'};">
              {t('portal.rowActions')}
            </h3>
            <button
              type="button"
              class="text-sm font-medium hover:underline"
              style="color: var(--ds-text-link);"
              data-testid="asset-report-add-row-action"
              onclick={addRowAction}
            >
              + {t('portal.addRowAction')}
            </button>
          </div>
          <p class="text-xs mb-3" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
            {t('portal.rowActionsHint')}
          </p>

          {#if formData.row_actions.length === 0}
            <p class="text-xs" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
              {t('portal.noRowActions')}
            </p>
          {:else}
            <div class="space-y-3">
              {#each formData.row_actions as action, index (action.id)}
                <div
                  class="rounded border p-3 space-y-3"
                  style="border-color: {isDarkMode ? '#475569' : '#e5e7eb'};"
                  data-testid={`asset-report-row-action-${index}`}
                >
                  <div class="flex items-start gap-2">
                    <div class="flex-1">
                      <label
                        for={`ar-row-action-label-${action.id}`}
                        class="block text-sm font-medium mb-1"
                        style="color: {isDarkMode ? '#9ca3af' : '#374151'};"
                      >
                        {t('portal.rowActionLabel')}
                      </label>
                      <Input
                        id={`ar-row-action-label-${action.id}`}
                        bind:value={action.label}
                        type="text"
                        placeholder={t('portal.rowActionLabelPlaceholder')}
                        size="small"
                        dataTestid={`asset-report-row-action-label-${index}`}
                      />
                    </div>
                    <button
                      type="button"
                      class="mt-6 p-1 rounded"
                      style="color: var(--ds-text-danger);"
                      title={t('portal.removeRowAction')}
                      data-testid={`asset-report-row-action-remove-${index}`}
                      onclick={() => removeRowAction(action.id)}
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>

                  <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <div>
                      <div class="block text-sm font-medium mb-1" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
                        {t('portal.rowActionRequestType')}
                      </div>
                      <BasePicker
                        bind:value={action.request_type_id}
                        items={availableRequestTypes}
                        placeholder={t('portal.selectRequestType')}
                        getValue={(item) => item.id}
                        getLabel={(item) => item.name}
                        inputTestid={`asset-report-row-action-request-type-${index}`}
                        onChange={() => onRowActionRequestTypeChange(action)}
                      />
                    </div>
                    <div>
                      <div class="block text-sm font-medium mb-1" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
                        {t('portal.rowActionTargetField')}
                      </div>
                      <BasePicker
                        bind:value={action.target_field}
                        items={requestTypeFieldItems(action.request_type_id)}
                        placeholder={t('portal.selectField')}
                        getValue={(field) => field.field_identifier}
                        getLabel={(field) =>
                          field.field_label || field.field_name || field.field_identifier}
                        inputTestid={`asset-report-row-action-target-field-${index}`}
                      />
                    </div>
                  </div>

                  <div>
                    <label
                      for={`ar-row-action-source-${action.id}`}
                      class="block text-sm font-medium mb-1"
                      style="color: {isDarkMode ? '#9ca3af' : '#374151'};"
                    >
                      {t('portal.rowActionSource')}
                    </label>
                    <select
                      id={`ar-row-action-source-${action.id}`}
                      bind:value={action.source}
                      class="w-full rounded border px-3 py-2 text-sm"
                      style="border-color: {isDarkMode ? '#475569' : '#d1d5db'}; background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'};"
                      data-testid={`asset-report-row-action-source-${index}`}
                    >
                      <option value="asset_id">{t('portal.rowActionSourceAssetId')}</option>
                      <option value="asset_tag">{t('portal.rowActionSourceAssetTag')}</option>
                      <option value="title">{t('portal.rowActionSourceTitle')}</option>
                    </select>
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </div>
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
