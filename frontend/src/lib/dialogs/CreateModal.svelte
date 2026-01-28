<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import { navigate, currentRoute } from '../router.js';
  import { milestonesStore } from '../stores/milestones.js';
  import { workspacesStore, shouldNavigateAfterCreate } from '../stores';
  import { api } from '../api.js';
  import { X, Target, Building, FolderOpen, ChevronRight, FileText } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';
  import Button from '../components/Button.svelte';
  import CompactWorkspaceSelector from '../pickers/CompactWorkspaceSelector.svelte';
  import FieldChip from '../components/FieldChip.svelte';
  import { getShortcut, matchesShortcut, getDisplayString } from '../utils/keyboardShortcuts.js';
  import { errorToast } from '../stores/toasts.svelte.js';

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
  const typeOptions = $derived([
    { value: 'work-item', label: t('createModal.workItem'), icon: FileText },
    { value: 'milestone', label: t('createModal.milestone'), icon: Target },
    { value: 'workspace', label: t('createModal.workspace'), icon: Building },
    { value: 'collection', label: t('createModal.collection'), icon: FolderOpen }
  ]);

  // Type display names - reactive for i18n
  const typeLabels = $derived({
    'work-item': t('createModal.workItem'),
    'milestone': t('createModal.milestone'),
    'workspace': t('createModal.workspace'),
    'collection': t('createModal.collection')
  });

  const dispatch = createEventDispatcher();

  // Get shortcut configurations
  const submitShortcut = getShortcut('modal', 'submit');
  const cancelShortcut = getShortcut('modal', 'cancel');

  let {
    isOpen = $bindable(false),
    compactMode = false,
    initialType = 'work-item'
  } = $props();

  let selectedType = $state(initialType);
  let selectedWorkspace = $state(null);
  let parentItem = $state(null);
  let restrictedItemTypes = $state(null);

  // Form references
  let workItemFormRef = $state(null);
  let milestoneFormRef = $state(null);
  let workspaceFormRef = $state(null);
  let collectionFormRef = $state(null);
  let nameInputRef = $state(null);

  // Form data for each type
  let workItemFormData = $state({
    name: '',
    description: '',
    due_date: '',
    workspace_id: null,
    priority_id: null,
    milestone_id: null,
    assignee_id: null,
    item_type_id: null
  });
  let workItemCustomFieldValues = $state({});
  let workItemValidationErrors = $state([]);

  let milestoneFormData = $state({
    name: '',
    description: '',
    target_date: '',
    status: 'planning'
  });

  let workspaceFormData = $state({
    name: '',
    key: '',
    description: ''
  });

  let collectionFormData = $state({
    name: '',
    description: '',
    workspace_id: null
  });
  let collectionCategoryId = $state(null);

  // Derived state for display
  let currentTypeName = $derived(typeLabels[selectedType] || 'Item');
  let currentFormData = $derived.by(() => {
    switch (selectedType) {
      case 'work-item': return workItemFormData;
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
        return workItemFormData.name.trim() !== '' && workItemFormData.workspace_id;
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
    selectedWorkspace = null;
    parentItem = null;
    restrictedItemTypes = null;

    // Reset all forms
    workItemFormData = {
      name: '',
      description: '',
      due_date: '',
      workspace_id: null,
      priority_id: null,
      milestone_id: null,
      assignee_id: null,
      item_type_id: null
    };
    workItemCustomFieldValues = {};
    workItemValidationErrors = [];

    milestoneFormData = {
      name: '',
      description: '',
      target_date: '',
      status: 'planning'
    };

    workspaceFormData = {
      name: '',
      key: '',
      description: ''
    };

    collectionFormData = {
      name: '',
      description: '',
      workspace_id: null
    };
    collectionCategoryId = null;

    dispatch('close');
  }

  function selectType(type) {
    selectedType = type;
    if (type === 'work-item' && !$workspacesStore.loaded) {
      loadWorkspaces();
    }
  }

  async function handleSubmit() {
    try {
      if (selectedType === 'work-item') {
        // Validate using form reference
        if (workItemFormRef && !workItemFormRef.validate()) {
          return;
        }

        const formData = workItemFormRef?.getFormData() || {
          workspace_id: selectedWorkspace?.id,
          title: workItemFormData.name,
          description: workItemFormData.description || '',
          priority_id: workItemFormData.priority_id || null,
          milestone_id: workItemFormData.milestone_id || null,
          assignee_id: workItemFormData.assignee_id || null,
          due_date: workItemFormData.due_date ? new Date(workItemFormData.due_date).toISOString() : null,
          status: 'open',
          item_type_id: workItemFormData.item_type_id,
          parent_id: parentItem ? parentItem.id : null,
          custom_field_values: workItemCustomFieldValues
        };

        if (!formData.workspace_id) {
          errorToast('Please select a workspace');
          return;
        }

        const result = await api.items.create(formData);

        window.dispatchEvent(new CustomEvent('refresh-work-items', { detail: { itemId: result.id } }));
        dispatch('created', result);

        if (shouldNavigateAfterCreate($currentRoute.view)) {
          navigate(`/workspaces/${formData.workspace_id}/items/${result.id}`);
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
        const result = await api.workspaces.create({
          name: workspaceFormData.name,
          key: workspaceFormData.key,
          description: workspaceFormData.description || '',
          active: true
        });

        window.dispatchEvent(new CustomEvent('refresh-workspaces'));
        navigate(`/workspaces/${result.id}`);
        close();
      } else if (selectedType === 'collection') {
        const result = await api.collections.create({
          name: collectionFormData.name,
          description: collectionFormData.description || '',
          cql_query: '',
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
    }
  }

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) {
      close();
    }
  }

  function handleKeydown(e) {
    if (matchesShortcut(e, cancelShortcut)) {
      close();
    }
    if (matchesShortcut(e, submitShortcut)) {
      e.preventDefault();
      if (isFormValid) {
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

  // Load workspaces when modal opens
  $effect(() => {
    if (isOpen && !$workspacesStore.loaded && $workspacesStore.regularWorkspaces.length === 0) {
      loadWorkspaces();
    }
  });

  // Sync selectedType when initialType prop changes (e.g. before modal opens)
  $effect(() => {
    selectedType = initialType;
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
      const workspaceId = event.detail.workspaceId;
      const workspaceIdNum = typeof workspaceId === 'string' ? parseInt(workspaceId, 10) : workspaceId;
      workItemFormData.workspace_id = workspaceIdNum;

      if ($workspacesStore.regularWorkspaces.length === 0) {
        loadWorkspaces().then(() => {
          selectedWorkspace = $workspacesStore.regularWorkspaces.find(w => w.id === workspaceIdNum);
        });
      } else {
        selectedWorkspace = $workspacesStore.regularWorkspaces.find(w => w.id === workspaceIdNum);
      }
    }
  }

  function handleSetCreateParent(event) {
    if (event.detail?.parentId) {
      parentItem = {
        id: event.detail.parentId,
        title: event.detail.parentTitle
      };
      restrictedItemTypes = event.detail.availableItemTypes || null;
    }
  }

  async function handleOpenCreateModal(event) {
    isOpen = true;
    if ($workspacesStore.regularWorkspaces.length === 0) {
      await loadWorkspaces();
    }
  }

  onMount(() => {
    window.addEventListener('open-create-modal', handleOpenCreateModal);
    window.addEventListener('set-create-type', handleSetCreateType);
    window.addEventListener('set-create-workspace', handleSetCreateWorkspace);
    window.addEventListener('set-create-parent', handleSetCreateParent);

    return () => {
      window.removeEventListener('open-create-modal', handleOpenCreateModal);
      window.removeEventListener('set-create-type', handleSetCreateType);
      window.removeEventListener('set-create-workspace', handleSetCreateWorkspace);
      window.removeEventListener('set-create-parent', handleSetCreateParent);
    };
  });
</script>

{#if isOpen}
  <!-- Backdrop -->
  <div
    transition:fade={{ duration: 150 }}
    class="fixed inset-0 flex items-start justify-center pt-16 overflow-y-auto z-50"
    style="background-color: rgba(0, 0, 0, 0.4); backdrop-filter: blur(2px);"
    tabindex="-1"
    onclick={handleBackdropClick}
    onkeydown={handleKeydown}
    role="dialog"
    aria-modal="true"
  >
    <!-- Modal -->
    <div class="rounded-xl shadow-2xl w-full max-w-lg mx-4 mb-8 flex flex-col" style="background-color: var(--ds-surface-raised);">
      <!-- Compact Header -->
      <div class="flex items-center gap-2 px-4 py-3 border-b" style="border-color: var(--ds-border);">
        <!-- Type Selector FIRST (independent of workspace) -->
        {#if !parentItem && !compactMode}
          <FieldChip
            label={t('createModal.type')}
            value={selectedType}
            displayValue={typeLabels[selectedType]}
            icon={typeIcons[selectedType]}
            placeholder={t('createModal.type')}
          >
            {#snippet children({ close: closePopover })}
              <div class="py-1">
                {#each typeOptions as type}
                  <button
                    type="button"
                    class="w-full flex items-center gap-3 px-3 py-2.5 text-left transition-colors"
                    style="color: var(--ds-text); background-color: {selectedType === type.value ? 'var(--ds-background-selected)' : 'transparent'};"
                    onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
                    onmouseout={(e) => e.currentTarget.style.backgroundColor = selectedType === type.value ? 'var(--ds-background-selected)' : 'transparent'}
                    onclick={() => {
                      selectType(type.value);
                      closePopover();
                    }}
                  >
                    <svelte:component this={type.icon} size={16} style="color: var(--ds-text-subtle);" />
                    <span class="font-medium">{type.label}</span>
                  </button>
                {/each}
              </div>
            {/snippet}
          </FieldChip>
          <ChevronRight size={14} style="color: var(--ds-text-subtle);" />
        {/if}

        <!-- Workspace Selector (only for work-items) -->
        {#if selectedType === 'work-item' && !parentItem}
          <CompactWorkspaceSelector
            bind:value={workItemFormData.workspace_id}
            workspaces={$workspacesStore.regularWorkspaces}
            onSelect={(workspace) => {
              if (workspace) {
                selectedWorkspace = workspace;
                workItemFormData.workspace_id = workspace.id;
              }
            }}
          />
          <ChevronRight size={14} style="color: var(--ds-text-subtle);" />
        {/if}

        <span class="font-medium" style="color: var(--ds-text);">
          {#if parentItem}
            {t('createModal.newChildItem')}
          {:else}
            {t('createModal.new')} {currentTypeName}
          {/if}
        </span>

        <button
          onclick={close}
          class="ml-auto p-1.5 rounded transition-colors"
          style="color: var(--ds-text-subtle);"
          onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
          aria-label="Close"
        >
          <X size={16} />
        </button>
      </div>

      <!-- Body -->
      <div class="px-4 py-3">
        {#if selectedType === 'work-item'}
          <WorkItemForm
            bind:this={workItemFormRef}
            bind:formData={workItemFormData}
            bind:customFieldValues={workItemCustomFieldValues}
            bind:validationErrorMessages={workItemValidationErrors}
            bind:nameInputRef={nameInputRef}
            {parentItem}
            {restrictedItemTypes}
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
      </div>

      <!-- Footer -->
      <div class="flex items-center justify-end px-4 py-3 border-t" style="border-color: var(--ds-border);">
        <Button
          onclick={handleSubmit}
          variant="primary"
          size="medium"
          keyboardHint={getDisplayString(submitShortcut)}
          disabled={!isFormValid}
        >
          {t('createModal.create')} {currentTypeName}
        </Button>
      </div>
    </div>
  </div>
{/if}
