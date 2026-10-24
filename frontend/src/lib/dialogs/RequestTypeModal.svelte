<script>
  import { workspaces } from '../api/workspaces.js';
  import { requestTypes } from '../api/channels.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
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
  let resolvingWorkspaces = $state(false);

  // Form data
  let formData = $state({
    name: '',
    description: '',
    icon: 'FileText',
    color: '#6b7280',
    item_type_id: null,
    workspace_id: null,
    title_template: ''
  });

  // Track if form has been initialized to prevent re-initialization
  let isFormInitialized = $state(false);
  let lastOpenState = $state(false);

  // workspace_id and item_type_id are fixed once a request type exists: the
  // configured fields are resolved from both. The system Email intake type is
  // the exception — it carries no fields and its item type stays editable.
  const isSystemEmail = $derived(requestType?.kind === 'email');
  const bindingLocked = $derived(mode === 'edit');

  // Load workspaces filtered to channel's configured IDs. The directory
  // store caches only the first server page, so a restricted channel
  // resolves each configured id it names explicitly — the allowed set is the
  // channel's own small list, not whatever the cache happens to hold
  // (WI-1585). Unrestricted channels keep the cached page and gain
  // server-backed search over the rest of the directory.
  let workspaceSearchResults = $state(null);

  async function loadWorkspaces() {
    try {
      const cached = (await workspacesStore.load()) || [];
      const byId = new Map(cached.map(ws => [ws.id, ws]));
      const configured = channelWorkspaceIds || [];
      const missing = configured.filter(id => !byId.has(id));
      const fetched = await Promise.allSettled(missing.map(id => workspaces.get(id)));
      for (const result of fetched) {
        if (result.status === 'fulfilled' && result.value?.id) {
          byId.set(result.value.id, result.value);
        } else if (result.status === 'rejected') {
          console.error('Failed to resolve configured workspace:', result.reason);
        }
      }
      availableWorkspaces = configured.length > 0
        ? configured.map(id => byId.get(id)).filter(Boolean)
        : [...byId.values()];
    } catch (err) {
      console.error('Failed to load workspaces:', err);
      availableWorkspaces = [];
    }
  }

  // Server-backed directory search for unrestricted channels.
  async function onWorkspaceSearch(query) {
    const trimmed = (query || '').trim();
    if (!trimmed) {
      workspaceSearchResults = null;
      return;
    }
    const { workspaces } = await workspacesStore.searchWorkspaces(trimmed, { limit: 25 });
    workspaceSearchResults = workspaces;
  }

  const workspacePickerItems = $derived.by(() => {
    if (channelWorkspaceIds && channelWorkspaceIds.length > 0) return availableWorkspaces;
    return workspaceSearchResults ?? availableWorkspaces;
  });

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
              workspace_id: requestType.workspace_id || null,
              title_template: requestType.title_template || ''
            };
            // title_template is edited from the FieldsModal but lives on
            // the request_type row, so this modal still has to round-trip
            // it through the basic Update payload. Refetch here so we
            // don't silently clobber a value the FieldsModal just saved.
            requestTypes.get(requestType.id)
              .then(fresh => { formData.title_template = fresh?.title_template || ''; })
              .catch(err => console.error('Failed to refresh request type:', err));
          } else if (mode === 'create') {
            formData = {
              name: '',
              description: '',
              icon: 'FileText',
              color: '#6b7280',
              item_type_id: availableItemTypes.length > 0 ? availableItemTypes[0].id : null,
              workspace_id: null,
              title_template: ''
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
          workspace_id: null,
          title_template: ''
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

      if (mode === 'create' && !formData.workspace_id) {
        error = t('channel.targetWorkspaceRequired');
        return;
      }

      submitting = true;
      error = null;

      if (mode === 'create') {
        await requestTypes.create(channelId, {
          name: formData.name.trim(),
          description: formData.description.trim(),
          icon: formData.icon,
          color: formData.color,
          item_type_id: formData.item_type_id,
          workspace_id: formData.workspace_id || null,
          title_template: formData.title_template.trim(),
          is_active: true
        });
      } else {
        // is_active is omitted on edit so saving the form does not reactivate
        // a request type that was deliberately disabled.
        await requestTypes.update(channelId, requestType.id, {
          name: formData.name.trim(),
          description: formData.description.trim(),
          icon: formData.icon,
          color: formData.color,
          item_type_id: formData.item_type_id,
          workspace_id: formData.workspace_id || null,
          title_template: formData.title_template.trim()
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
            {t('common.name')} <span class="text-ds-text-danger">*</span>
          </label>
          <Input
            id="rt-name"
            bind:value={formData.name}
            type="text"
            placeholder={t('portal.requestTypeNamePlaceholder')}
            required
            size="medium"
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
            {t('portal.createsItemType')} <span class="text-ds-text-danger">*</span>
          </label>
          <BasePicker
            bind:value={formData.item_type_id}
            items={availableItemTypes}
            placeholder={t('portal.selectItemType')}
            getValue={(item) => item.id}
            getLabel={(item) => item.name}
            disabled={bindingLocked && !isSystemEmail}
          />
          <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
            {#if bindingLocked && !isSystemEmail}
              {t('portal.bindingLocked', 'Fixed after creation. Delete and recreate the request type to change it.')}
            {:else}
              {t('portal.submissionsCreateItemType')}
            {/if}
          </p>
        </div>

        <div>
          <label for="rt-workspace" class="block text-sm font-medium mb-2" style="color: {isDarkMode ? '#9ca3af' : '#374151'};">
            {t('common.workspace')} {#if mode === 'create'}<span class="text-ds-text-danger">*</span>{/if}
          </label>
          <BasePicker
            bind:value={formData.workspace_id}
            items={workspacePickerItems}
            serverSearch={!(channelWorkspaceIds && channelWorkspaceIds.length > 0)}
            onSearchChange={onWorkspaceSearch}
            loading={resolvingWorkspaces}
            placeholder={t('portal.selectWorkspace', 'Select workspace')}
            getValue={(item) => item.id}
            getLabel={(item) => item.name}
            allowClear={mode === 'create'}
            disabled={bindingLocked}
          />
          <p class="text-xs mt-1" style="color: {isDarkMode ? '#94a3b8' : '#6b7280'};">
            {#if bindingLocked}
              {t('portal.bindingLocked', 'Fixed after creation. Delete and recreate the request type to change it.')}
            {:else}
              {t('portal.workspaceFieldResolution', 'Used to resolve available custom fields from the workspace configuration.')}
            {/if}
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
