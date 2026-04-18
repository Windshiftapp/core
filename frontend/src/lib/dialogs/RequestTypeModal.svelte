<script>
  import { api } from '../api.js';
  import Button from '../components/Button.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import Textarea from '../components/Textarea.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import PortalModal from './PortalModal.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    isOpen = false,
    mode = 'create',
    requestType = null,
    channelId = null,
    availableItemTypes = [],
    channelWorkspaceIds = [],
    isDarkMode = false,
    onsaved = undefined,
    onclose = undefined
  } = $props();

  let submitting = $state(false);
  let error = $state(null);
  let success = $state(false);
  let availableWorkspaces = $state([]);

  // Form data
  let formData = $state({
    name: '',
    description: '',
    icon: 'FileText',
    color: '#6b7280',
    item_type_id: null,
    workspace_id: null
  });

  // Track if form has been initialized to prevent re-initialization
  let isFormInitialized = $state(false);
  let lastOpenState = $state(false);

  // Load workspaces filtered to channel's configured IDs
  async function loadWorkspaces() {
    try {
      const allWorkspaces = await api.workspaces.getAll();
      if (channelWorkspaceIds && channelWorkspaceIds.length > 0) {
        availableWorkspaces = allWorkspaces.filter(ws => channelWorkspaceIds.includes(ws.id));
      } else {
        availableWorkspaces = allWorkspaces;
      }
    } catch (err) {
      console.error('Failed to load workspaces:', err);
      availableWorkspaces = [];
    }
  }

  // Consolidated reactive statement to handle modal state changes
  $effect(() => {
    if (isOpen !== lastOpenState) {
      lastOpenState = isOpen;

      if (isOpen) {
        if (!isFormInitialized) {
          if (mode === 'edit' && requestType) {
            formData = {
              name: requestType.name || '',
              description: requestType.description || '',
              icon: requestType.icon || 'FileText',
              color: requestType.color || '#6b7280',
              item_type_id: requestType.item_type_id || null,
              workspace_id: requestType.workspace_id || null
            };
          } else if (mode === 'create') {
            formData = {
              name: '',
              description: '',
              icon: 'FileText',
              color: '#6b7280',
              item_type_id: availableItemTypes.length > 0 ? availableItemTypes[0].id : null,
              workspace_id: null
            };
          }
          isFormInitialized = true;
          loadWorkspaces();
        }
        error = null;
        success = false;
      } else {
        formData = {
          name: '',
          description: '',
          icon: 'FileText',
          color: '#6b7280',
          item_type_id: null,
          workspace_id: null
        };
        error = null;
        success = false;
        isFormInitialized = false;
      }
    }
  });

  async function handleSubmit() {
    try {
      if (!formData.name.trim()) {
        error = t('portal.nameRequired');
        return;
      }

      if (!formData.item_type_id) {
        error = t('portal.itemTypeRequired');
        return;
      }

      submitting = true;
      error = null;

      if (mode === 'create') {
        await api.requestTypes.create(channelId, {
          name: formData.name.trim(),
          description: formData.description.trim(),
          icon: formData.icon,
          color: formData.color,
          item_type_id: formData.item_type_id,
          workspace_id: formData.workspace_id || null,
          is_active: true
        });
      } else {
        await api.requestTypes.update(requestType.id, {
          name: formData.name.trim(),
          description: formData.description.trim(),
          icon: formData.icon,
          color: formData.color,
          item_type_id: formData.item_type_id,
          workspace_id: formData.workspace_id || null,
          is_active: true
        });
      }

      success = true;
      handleClose();
      onsaved?.();
    } catch (err) {
      console.error('Failed to save request type:', err);
      error = err.message || t('portal.failedToSaveRequestType');
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
    title={mode === 'create' ? t('portal.createRequestType') : t('portal.editRequestType')}
    subtitle={mode === 'create' ? t('portal.addRequestTypeSubtitle') : t('portal.editRequestTypeSubtitle')}
    onClose={handleClose}
    bodyClass="px-6 py-4 max-h-[60vh] overflow-y-auto"
  >
    {#if success}
      <div class="mb-4">
        <AlertBox variant="success" message={mode === 'create' ? t('portal.requestTypeCreated') : t('portal.requestTypeUpdated')} />
      </div>
    {:else}
      {#if error}
        <AlertBox variant="error" message={error} class="mb-4" />
      {/if}

      <div class="space-y-4">
        <div>
          <label for="rt-name" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('common.name')} <span class="text-red-500">*</span>
          </label>
          <input
            id="rt-name"
            bind:value={formData.name}
            type="text"
            class="w-full px-4 py-3 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
            style="background-color: {isDarkMode ? '#1e293b' : '#ffffff'}; color: {isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {isDarkMode ? '#475569' : '#d1d5db'};"
            placeholder={t('portal.requestTypeNamePlaceholder')}
            required
          />
        </div>

        <div>
          <label for="rt-description" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('portal.descriptionOptional')}
          </label>
          <Textarea
            id="rt-description"
            bind:value={formData.description}
            rows={3}
            placeholder={t('portal.requestTypeDescriptionPlaceholder')}
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
          <label for="rt-itemtype" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('portal.createsItemType')} <span class="text-red-500">*</span>
          </label>
          <BasePicker
            bind:value={formData.item_type_id}
            items={availableItemTypes}
            placeholder={t('portal.selectItemType')}
            getValue={(item) => item.id}
            getLabel={(item) => item.name}
          />
          <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
            {t('portal.submissionsCreateItemType')}
          </p>
        </div>

        <div>
          <label for="rt-workspace" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
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
      </div>

      <DialogFooter
        onCancel={handleClose}
        onConfirm={handleSubmit}
        confirmLabel={mode === 'create' ? t('portal.createRequestType') : t('common.saveChanges')}
        loading={submitting}
        loadingLabel={mode === 'create' ? t('portal.creating') : t('common.saving')}
        class="mt-6 -mx-6 -mb-4"
      />
    {/if}
  </PortalModal>
{/if}
