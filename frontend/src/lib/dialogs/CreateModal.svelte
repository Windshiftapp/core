<script>
  import { useEventListener } from 'runed';
  import { navigate, currentRoute } from '../router.js';
  import { milestonesStore } from '../stores/milestones.js';
  import { workspacesStore, shouldNavigateAfterCreate, workItemFormStore, permissionStore, isSystemAdmin } from '../stores';
  import { api } from '../api.js';
  import { X, Target, Building, FolderOpen, ChevronRight, FileText } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';
  import Button from '../components/Button.svelte';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import CreateFormFrame from '../forms/CreateFormFrame.svelte';
  import ChipPicker from '../pickers/ChipPicker.svelte';
  import { getShortcut, matchesShortcut, getDisplayString } from '../utils/keyboardShortcuts.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { showCreatedItemToast } from '../utils/createdItemToast.js';

  // Import form components
  import WorkItemForm from '../forms/WorkItemForm.svelte';
  import MilestoneForm from '../forms/MilestoneForm.svelte';
  import WorkspaceForm from '../forms/WorkspaceForm.svelte';
  import CollectionForm from '../forms/CollectionForm.svelte';

  // Type icons and options
  const typeIcons = {
    'work-item': FileText,
    'milestone': Target,
    'workspace': Building,
    'collection': FolderOpen
  };

  // Type options - reactive for i18n
  const typeOptions = $derived.by(() => {
    const options = [
      { value: 'work-item', label: t('createModal.workItem'), icon: FileText }
    ];

    if ($permissionStore.userPermissionKeys?.has('milestone.create') || $isSystemAdmin) {
      options.push({ value: 'milestone', label: t('createModal.milestone'), icon: Target });
    }

    if ($permissionStore.userPermissionKeys?.has('workspace.create') || $isSystemAdmin) {
      options.push({ value: 'workspace', label: t('createModal.workspace'), icon: Building });
    }

    options.push({ value: 'collection', label: t('createModal.collection'), icon: FolderOpen });

    return options;
  });

  // Type display names - reactive for i18n
  const typeLabels = $derived({
    'work-item': t('createModal.workItem'),
    'milestone': t('createModal.milestone'),
    'workspace': t('createModal.workspace'),
    'collection': t('createModal.collection')
  });

  const submitShortcut = getShortcut('modal', 'submit');

  let {
    isOpen = $bindable(false),
    compactMode = false,
    initialType = 'work-item',
    initialWorkspaceId = null,
    skipNavigate = false,
    onclose = null,
    oncreated = null
  } = $props();

  // svelte-ignore state_referenced_locally
  let selectedType = $state(initialType);

  // Form references
  let milestoneFormRef = $state(null);
  let workspaceFormRef = $state(null);
  let collectionFormRef = $state(null);
  let nameInputRef = $state(null);

  // Non-work-item form data
  let milestoneFormData = $state({
    name: '',
    description: '',
    target_date: '',
    status: 'planning'
  });

  let workspaceFormData = $state({
    name: '',
    key: '',
    description: '',
    template_workspace_id: null
  });

  let workspaceTemplateOptions = $state([]);
  let workspaceTemplatesLoading = $state(false);
  let workspaceTemplatesError = $state(null);
  let workspaceTemplatesLoaded = $state(false);

  let collectionFormData = $state({
    name: '',
    description: '',
    workspace_id: null
  });
  let collectionCategoryId = $state(null);
  let submitting = $state(false);

  // Derived state for display
  let currentTypeName = $derived(typeLabels[selectedType] || 'Item');
  let currentFormData = $derived.by(() => {
    switch (selectedType) {
      case 'work-item': return workItemFormStore.formData;
      case 'milestone': return milestoneFormData;
      case 'workspace': return workspaceFormData;
      case 'collection': return collectionFormData;
      default: return { name: '' };
    }
  });

  // Check if form is valid for submit button
  let isFormValid = $derived.by(() => {
    switch (selectedType) {
      case 'work-item':
        return workItemFormStore.formData.name.trim() !== '' && workItemFormStore.formData.workspace_id;
      case 'milestone':
        return milestoneFormData.name.trim() !== '' && milestoneFormData.target_date;
      case 'workspace':
        return workspaceFormData.name.trim() !== '' && workspaceFormData.key.trim() !== '';
      case 'collection':
        return collectionFormData.name.trim() !== '';
      default:
        return false;
    }
  });

  async function loadWorkspaces() {
    await workspacesStore.load();
  }

  function close() {
    isOpen = false;
    selectedType = initialType;

    // Reset work item form store
    workItemFormStore.resetForm();

    // Reset other forms
    milestoneFormData = {
      name: '',
      description: '',
      target_date: '',
      status: 'planning'
    };

    workspaceFormData = {
      name: '',
      key: '',
      description: '',
      template_workspace_id: null
    };

    collectionFormData = {
      name: '',
      description: '',
      workspace_id: null
    };
    collectionCategoryId = null;

    onclose?.();
  }

  function selectType(type) {
    selectedType = type;
    if (type === 'work-item' && !$workspacesStore.loaded) {
      loadWorkspaces();
    }
  }

  async function loadWorkspaceTemplates() {
    if (workspaceTemplatesLoaded || workspaceTemplatesLoading) return;
    workspaceTemplatesLoading = true;
    workspaceTemplatesError = null;
    try {
      const templates = await api.workspaces.getTemplates();
      workspaceTemplateOptions = Array.isArray(templates) ? templates : [];
      workspaceTemplatesLoaded = true;
    } catch (error) {
      console.error('Failed to load workspace templates:', error);
      workspaceTemplateOptions = [];
      workspaceTemplatesError = error?.message || String(error);
    } finally {
      workspaceTemplatesLoading = false;
    }
  }

  async function uploadPendingDescriptionImages(itemId, description) {
    if (workItemFormStore.pendingDescriptionImages.length === 0) {
      return description;
    }

    let updatedDescription = description || '';
    for (const image of workItemFormStore.pendingDescriptionImages) {
      if (!updatedDescription.includes(image.url)) continue;

      const attachment = await api.attachments.uploadToItem(itemId, image.file);
      const attachmentId = attachment?.id;
      if (!attachmentId) {
        throw new Error('Image upload failed');
      }

      const downloadUrl = api.attachments.getDownloadUrl(attachmentId);
      updatedDescription = updatedDescription.split(image.url).join(downloadUrl);
    }

    return updatedDescription;
  }

  async function handleSubmit() {
    if (submitting) return;
    submitting = true;
    try {
      if (selectedType === 'work-item') {
        // Validate using store
        if (!workItemFormStore.validate()) {
          return;
        }

        const formData = workItemFormStore.getFormData();

        if (!formData.workspace_id) {
          errorToast('Please select a workspace');
          return;
        }

        let result = await api.items.create(formData);
        const originalDescription = formData.description || '';
        const updatedDescription = await uploadPendingDescriptionImages(result.id, originalDescription);
        if (updatedDescription !== originalDescription) {
          result = await api.items.update(result.id, { description: updatedDescription });
          formData.description = updatedDescription;
        }

        window.dispatchEvent(new CustomEvent('refresh-work-items', {
          detail: { item: result, itemId: result.id, parentId: formData.parent_id ?? null }
        }));
        oncreated?.(result);

        // When creating a child issue, stay on the parent so the user can see the new
        // child appear in the children list rather than being navigated away.
        const shouldNavigate = !formData.parent_id && shouldNavigateAfterCreate($currentRoute.view);
        if (shouldNavigate) {
          navigate(`/workspaces/${formData.workspace_id}/items/${result.id}`);
        } else {
          showCreatedItemToast(result);
        }
        close();
      } else if (selectedType === 'milestone') {
        await milestonesStore.add({
          name: milestoneFormData.name,
          description: milestoneFormData.description,
          target_date: milestoneFormData.target_date || null,
          status: milestoneFormData.status,
          category_id: null
        });

        navigate('/milestones');
        close();
      } else if (selectedType === 'workspace') {
        const payload = {
          name: workspaceFormData.name,
          key: workspaceFormData.key,
          description: workspaceFormData.description || '',
          icon: 'Package',
          color: '#3b82f6',
          active: true
        };
        if (workspaceFormData.template_workspace_id) {
          payload.template_workspace_id = workspaceFormData.template_workspace_id;
        }
        const result = await api.workspaces.create(payload);

        workspacesStore.add(result);
        window.dispatchEvent(new CustomEvent('refresh-workspaces', {
          detail: { workspace: result }
        }));
        if (!skipNavigate) {
          navigate(`/workspaces/${result.id}`);
        }
        close();
      } else if (selectedType === 'collection') {
        const result = await api.collections.create({
          name: collectionFormData.name,
          description: collectionFormData.description || '',
          ql_query: '',
          is_public: false,
          workspace_id: collectionFormData.workspace_id,
          category_id: collectionCategoryId
        });

        navigate(`/collections/${result.id}`);
        close();
      }
    } catch (error) {
      console.error('Failed to create item:', error);
      const errorMsg = error.message || String(error);
      if (errorMsg.includes('UNIQUE constraint failed: workspaces.key')) {
        errorToast('A workspace with this key already exists. Please choose a different key.');
      } else {
        errorToast(`Failed to create ${currentTypeName.toLowerCase()}: ${errorMsg}`);
      }
    } finally {
      submitting = false;
    }
  }

  function handleKeydown(e) {
    if (!isOpen) return;
    if (matchesShortcut(e, submitShortcut)) {
      e.preventDefault();
      if (isFormValid && !submitting) {
        handleSubmit();
      }
    }
  }

  // Focus first input when modal opens
  $effect(() => {
    if (isOpen && nameInputRef) {
      setTimeout(() => {
        nameInputRef?.focus();
      }, 100);
    }
  });

  // Initialize store when modal opens for work items
  $effect(() => {
    if (isOpen && selectedType === 'work-item') {
      workItemFormStore.init();
    }
  });

  // Load workspaces when modal opens
  $effect(() => {
    if (isOpen && !$workspacesStore.loaded && $workspacesStore.regularWorkspaces.length === 0) {
      loadWorkspaces();
    }
  });

  // Load template summaries when the workspace form opens for a user who may
  // create workspaces. Blank creation works without them.
  $effect(() => {
    if (
      isOpen &&
      selectedType === 'workspace' &&
      ($permissionStore.userPermissionKeys?.has('workspace.create') || $isSystemAdmin)
    ) {
      loadWorkspaceTemplates();
    }
  });

  // Sync selectedType when initialType prop changes (e.g. before modal opens)
  $effect(() => {
    selectedType = initialType;
  });

  // The modal is lazy-loaded, so workspace context must arrive as state rather
  // than a timer-based window event that can fire before this component mounts.
  $effect(() => {
    if (
      isOpen &&
      initialWorkspaceId &&
      workItemFormStore.formData.workspace_id !== Number(initialWorkspaceId)
    ) {
      applyWorkspace(initialWorkspaceId);
    }
  });

  // Force work-item type when compact mode is enabled
  $effect(() => {
    if (compactMode && selectedType !== 'work-item') {
      selectedType = 'work-item';
    }
  });

  // Event handlers for global events
  function handleSetCreateType(event) {
    if (event.detail?.type) {
      selectedType = event.detail.type;
      if (event.detail.type === 'work-item' && $workspacesStore.regularWorkspaces.length === 0) {
        loadWorkspaces();
      }
    }
  }

  function handleSetCreateWorkspace(event) {
    if (event.detail?.workspaceId) {
      applyWorkspace(event.detail.workspaceId);
    }
  }

  async function applyWorkspace(workspaceId) {
    const workspaceIdNum = Number.parseInt(String(workspaceId), 10);
    if (!Number.isFinite(workspaceIdNum)) return;

    collectionFormData.workspace_id = workspaceIdNum;
    if ($workspacesStore.regularWorkspaces.length === 0) {
      await loadWorkspaces();
    }
    const workspace = $workspacesStore.regularWorkspaces.find(w => w.id === workspaceIdNum);
    if (workspace) {
      workItemFormStore.setWorkspace(workspace);
    }
  }

  function handleSetCreateParent(event) {
    if (event.detail?.parentId) {
      const parent = {
        id: event.detail.parentId,
        title: event.detail.parentTitle
      };
      const allowedItemTypes = event.detail.availableItemTypes || null;
      workItemFormStore.setParentItem(parent, allowedItemTypes);
    }
  }

  async function handleOpenCreateModal(event) {
    isOpen = true;
    if ($workspacesStore.regularWorkspaces.length === 0) {
      await loadWorkspaces();
    }
  }

  useEventListener(() => window, 'open-create-modal', handleOpenCreateModal);
  useEventListener(() => window, 'set-create-type', handleSetCreateType);
  useEventListener(() => window, 'set-create-workspace', handleSetCreateWorkspace);
  useEventListener(() => window, 'set-create-parent', handleSetCreateParent);
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- shortcut-guard-exempt: Cmd+Enter submit is handled via svelte:window onkeydown (matchesShortcut) above, outside the ModalBackdrop block the guard scans. -->
<ModalBackdrop bind:show={isOpen} opacity={0.4} align="top" paddingTop="pt-16" scrollable zIndex={60} closeOnClick={false} onclose={close}>
    <CreateFormFrame dataTestid="create-modal">
      {#snippet header()}
        <!-- Type Selector FIRST (independent of workspace) -->
        {#if !workItemFormStore.parentItem && !compactMode}
          <ChipPicker
            value={selectedType}
            items={typeOptions}
            getValue={(t) => t.value}
            getLabel={(t) => t.label}
            icon={typeIcons[selectedType]}
            placeholder={t('createModal.type')}
            testId="create-type-chip"
            onSelect={(type) => selectType(type.value)}
          >
            {#snippet itemSnippet({ item })}
              <item.icon size={16} style="color: var(--ds-text-subtle);" />
              <span class="font-medium">{item.label}</span>
            {/snippet}
          </ChipPicker>
          <ChevronRight size={14} style="color: var(--ds-text-subtle);" />
        {/if}

        <!-- Workspace Selector (only for work-items) -->
        {#if selectedType === 'work-item' && !workItemFormStore.parentItem}
          <ChipPicker
            value={workItemFormStore.formData.workspace_id}
            items={$workspacesStore.regularWorkspaces}
            getValue={(w) => w.id}
            getLabel={(w) => w.key || w.name}
            icon={Building}
            placeholder={t('workspaces.workspace')}
            searchable={true}
            searchFields={['name', 'key']}
            testId="create-workspace-chip"
            onSelect={(workspace) => {
              if (workspace) {
                workItemFormStore.setWorkspace(workspace);
              }
            }}
          >
            {#snippet itemSnippet({ item })}
              {#if item.avatar_url}
                <img
                  src={item.avatar_url}
                  alt={item.name}
                  class="w-5 h-5 rounded flex-shrink-0"
                />
              {:else}
                <div
                  class="w-5 h-5 rounded flex items-center justify-center flex-shrink-0"
                  style="background-color: {item.color || '#6366f1'};"
                >
                  <Building size={10} style="color: #fff;" />
                </div>
              {/if}
              <span class="font-medium truncate">{item.name}</span>
              {#if item.key}
                <span
                  class="text-xs px-1.5 py-0.5 rounded flex-shrink-0"
                  style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                >
                  {item.key}
                </span>
              {/if}
            {/snippet}
          </ChipPicker>
          <ChevronRight size={14} style="color: var(--ds-text-subtle);" />
        {/if}

        <span class="font-medium" style="color: var(--ds-text);">
          {#if workItemFormStore.parentItem}
            {t('createModal.newChildItem')}
          {:else}
            {t('createModal.new')} {currentTypeName}
          {/if}
        </span>

        <button
          data-testid="create-modal-close"
          onclick={close}
          class="ml-auto p-1.5 rounded transition-colors"
          style="color: var(--ds-text-subtle);"
          onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
          onfocus={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onblur={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
          aria-label="Close"
        >
          <X size={16} />
        </button>
      {/snippet}

      {#snippet body()}
        {#if selectedType === 'work-item'}
          <WorkItemForm
            bind:nameInputRef={nameInputRef}
          />
        {:else if selectedType === 'milestone'}
          <MilestoneForm
            bind:this={milestoneFormRef}
            bind:formData={milestoneFormData}
            bind:nameInputRef={nameInputRef}
          />
        {:else if selectedType === 'workspace'}
          <WorkspaceForm
            bind:this={workspaceFormRef}
            bind:formData={workspaceFormData}
            templates={workspaceTemplateOptions}
            templatesLoading={workspaceTemplatesLoading}
            templatesError={workspaceTemplatesError}
            bind:nameInputRef={nameInputRef}
          />
        {:else if selectedType === 'collection'}
          <CollectionForm
            bind:this={collectionFormRef}
            bind:formData={collectionFormData}
            bind:categoryId={collectionCategoryId}
            bind:nameInputRef={nameInputRef}
          />
        {/if}
      {/snippet}

      {#snippet footer()}
        <Button
          id="create-modal-submit"
          onclick={handleSubmit}
          variant="primary"
          size="medium"
          keyboardHint={getDisplayString(submitShortcut)}
          loading={submitting}
          disabled={!isFormValid || submitting}
        >
          {t('createModal.create')} {currentTypeName}
        </Button>
      {/snippet}
    </CreateFormFrame>
</ModalBackdrop>
