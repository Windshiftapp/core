<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { Edit, Trash2, Plus, Circle, Camera, Grip } from 'lucide-svelte';
  import { workspaceIconMap } from '../utils/icons.js';
  import Button from '../components/Button.svelte';
  import Label from '../components/Label.svelte';
  import Input from '../components/Input.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { createShortcutHandler, getShortcutDisplay } from '../utils/keyboardShortcuts.js';
  import { workspacesStore } from '../stores';

  // Props
  export let showPageHeader = true; // Whether to show admin header and use admin layout
  let showCreateForm = false;
  let editingWorkspace = null;
  let activeWorkspace = null;
  let formData = {
    name: '',
    key: '',
    description: '',
    active: true,
    icon: 'Grip',
    color: '#3b82f6',
    avatar_url: null
  };

  // Avatar upload state
  let uploadingAvatar = false;
  let showAvatarUpload = false;
  export let noPadding = false;

  // Use centralized icon map for workspace icons
  const iconMap = workspaceIconMap;

  onMount(async () => {
    // Load workspaces from store
    await workspacesStore.load();

    // Add global keyboard shortcut for "A" key
    window.addEventListener('keydown', handleGlobalKeydown);

    // Cleanup event listener
    return () => {
      window.removeEventListener('keydown', handleGlobalKeydown);
    };
  });

  function startCreate() {
    showCreateForm = true;
    editingWorkspace = null;
    resetForm();
  }

  function startEdit(workspace) {
    editingWorkspace = workspace;
    formData = {
      name: workspace.name,
      key: workspace.key || '',
      description: workspace.description || '',
      active: workspace.active,
      icon: workspace.icon || 'Grip',
      color: workspace.color || '#3b82f6',
      avatar_url: workspace.avatar_url || null
    };
    showCreateForm = true;
  }

  function resetForm() {
    formData = {
      name: '',
      key: '',
      description: '',
      active: true,
      icon: 'Grip',
      color: '#3b82f6',
      avatar_url: null
    };
    showAvatarUpload = false;
  }

  function cancelForm() {
    showCreateForm = false;
    editingWorkspace = null;
    resetForm();
  }

  function handleModalKeydown(event) {
    if (event.key === 'Enter' && !event.shiftKey && !event.ctrlKey && !event.altKey) {
      // Only trigger if we have required fields filled
      if (formData.name.trim() && formData.key.trim()) {
        event.preventDefault();
        saveWorkspace();
      }
    }
    // ESC key handling is done by the Modal component itself
  }

  const handleGlobalKeydown = createShortcutHandler({
    addWorkspace: () => {
      if (!showCreateForm) {
        startCreate();
      }
    }
  }, 'workspaces');

  // Avatar upload functionality
  async function handleAvatarUpload(files) {
    if (!files || files.length === 0) return;

    const file = files[0];
    if (!file.type.startsWith('image/')) {
      alert('Please select an image file');
      return;
    }

    uploadingAvatar = true;
    try {
      const uploadFormData = new FormData();
      uploadFormData.append('file', file);
      uploadFormData.append('item_id', editingWorkspace ? editingWorkspace.id.toString() : '0'); // Use workspace ID when editing
      uploadFormData.append('category', 'workspace_avatar');

      const response = await fetch('/api/attachments/upload', {
        method: 'POST',
        body: uploadFormData,
      });

      if (!response.ok) {
        throw new Error(`Upload failed: ${response.statusText}`);
      }

      const uploadResult = await response.json();
      
      if (uploadResult && uploadResult.success && uploadResult.avatar_url) {
        formData.avatar_url = uploadResult.avatar_url;
        showAvatarUpload = false;
      }
    } catch (err) {
      alert('Failed to upload avatar: ' + (err.message || err));
    } finally {
      uploadingAvatar = false;
    }
  }

  function removeAvatar() {
    formData.avatar_url = null;
  }

  function handleIconChange(event) {
    formData.icon = event.detail.icon;
    formData.color = event.detail.color;
  }

  async function saveWorkspace() {
    try {
      if (editingWorkspace) {
        await api.workspaces.update(editingWorkspace.id, formData);
      } else {
        await api.workspaces.create(formData);
      }

      // Reload workspaces from store
      await workspacesStore.reload();

      cancelForm();
    } catch (error) {
      console.error('Failed to save workspace:', error);
      alert('Failed to save workspace: ' + (error.message || error));
    }
  }

  async function deleteWorkspace(workspace) {
    if (confirm(`Are you sure you want to delete workspace "${workspace.name}"? This will affect all associated projects.`)) {
      try {
        await api.workspaces.delete(workspace.id);
        await workspacesStore.reload();
      } catch (error) {
        console.error('Failed to delete workspace:', error);
        alert('Failed to delete workspace: ' + (error.message || error));
      }
    }
  }

  function getStatusBadgeClass(active) {
    return active 
      ? 'bg-green-100 text-green-800' 
      : 'bg-gray-100 text-gray-800';
  }
  
  function buildWorkspaceDropdownItems(workspace) {
    // Personal workspaces cannot be edited
    if (workspace.is_personal) {
      return [];
    }

    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover:bg-gray-100',
        onClick: () => startEdit(workspace)
      }
      // Delete action removed - workspaces can only be deleted from workspace settings
    ];
  }

  // Table column definitions
  const workspaceColumns = [
    {
      key: 'name',
      label: 'Workspace',
      slot: 'name'
    },
    {
      key: 'active',
      label: 'Status',
      slot: 'status'
    },
    {
      key: 'created_at',
      label: 'Created',
      render: (workspace) => new Date(workspace.created_at).toLocaleDateString(),
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];

  onDestroy(() => {
    // Additional cleanup in case component is destroyed outside of onMount cleanup
    window.removeEventListener('keydown', handleGlobalKeydown);
  });

</script>

<div class="min-h-screen" style="background-color: var(--ds-surface);">
    <div class="{noPadding ? '' : 'px-6 pt-6'}">
      <PageHeader
        icon={Grip}
        title="Workspaces"
        subtitle="Organize and manage your projects within workspaces"
      >
        {#snippet actions()}
          <Button
            variant="primary"
            icon={Plus}
            onclick={startCreate}
            keyboardHint={getShortcutDisplay('workspaces', 'addWorkspace')}
          >
            Add Workspace
          </Button>
        {/snippet}
      </PageHeader>
    </div>


    <div class="{noPadding ? '' : 'px-6 pb-6'}">
      <DataTable
        columns={workspaceColumns}
        data={$workspacesStore.regularWorkspaces}
        keyField="id"
        emptyMessage="No workspaces found. Create your first workspace to get started."
        emptyIcon={Circle}
        actionItems={buildWorkspaceDropdownItems}
        onRowClick={(workspace) => navigate(`/workspaces/${workspace.id}`)}
      >
    <div slot="name" let:item={workspace}>
      <div class="flex items-center gap-3">
        <!-- Workspace Visual Identity -->
        {#if workspace.avatar_url}
          <img src={workspace.avatar_url} alt="{workspace.name} avatar" class="w-8 h-8 rounded-md object-cover flex-shrink-0" />
        {:else}
          <div class="w-8 h-8 rounded-md flex items-center justify-center flex-shrink-0" style="background-color: {workspace.color || '#3b82f6'};">
            <svelte:component this={iconMap[workspace.icon] || Grip} size={16} color="white" />
          </div>
        {/if}
        
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2">
            <div class="font-semibold" style="color: var(--ds-text);">{workspace.name}</div>
            {#if workspace.is_personal}
              <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800">
                Personal
              </span>
            {/if}
          </div>
          {#if workspace.description}
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{workspace.description}</div>
          {/if}
        </div>
      </div>
    </div>

    <Lozenge slot="status" let:item={workspace} color={workspace.active ? 'green' : 'gray'} text={workspace.active ? 'Active' : 'Inactive'} />
  </DataTable>
    </div>
</div>

<!-- Workspace Create/Edit Modal -->
<Modal 
  isOpen={showCreateForm} 
  onclose={cancelForm}
  maxWidth="max-w-2xl"
>
  <div onkeydown={handleModalKeydown}>
    <!-- Modal Header -->
    <div class="px-8 py-6 border-b" style="border-color: var(--ds-border);">
      <h2 class="text-2xl font-semibold" style="color: var(--ds-text);">
        {editingWorkspace ? 'Edit Workspace' : 'New Workspace'}
      </h2>
    </div>

    <!-- Modal Content -->
    <div class="px-8 py-6">
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div>
          <Label for="workspace-name" required class="mb-2">Workspace Name</Label>
          <Input
            id="workspace-name"
            bind:value={formData.name}
            placeholder="e.g., Development, Testing, Production"
            required
          />
        </div>

        <div>
          <Label for="workspace-key" required class="mb-2">Workspace Key</Label>
          <Input
            id="workspace-key"
            bind:value={formData.key}
            placeholder="e.g., DEV, TEST, PROD"
            maxlength="10"
            pattern="[A-Z0-9_]+"
            title="Uppercase letters, numbers, and underscores only (max 10 characters)"
            required
          />
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            Used for item prefixes (e.g., DEV-123). Uppercase letters, numbers, and underscores only.
          </p>
        </div>
      </div>

      <div class="mt-6">
        <Label for="workspace-description" class="mb-2">Description</Label>
        <Input
          id="workspace-description"
          bind:value={formData.description}
          placeholder="Optional description for this workspace"
        />
      </div>

      <!-- Visual Identity Section -->
      <div class="mt-6">
        <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">Visual Identity</h3>
        
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <!-- Icon and Color Selection -->
          <div>
            <IconSelector
              selectedIcon={formData.icon}
              selectedColor={formData.color}
              label="Workspace Icon & Color"
              compact={true}
              onchange={handleIconChange}
            />
          </div>

          <!-- Avatar Upload -->
          <div>
            <Label class="mb-2">Workspace Avatar</Label>
            
            <div class="space-y-3">
              <!-- Current Avatar Display -->
              {#if formData.avatar_url}
                <div class="flex items-center gap-3 p-3 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                  <img src={formData.avatar_url} alt="Workspace avatar" class="w-12 h-12 rounded-md object-cover" />
                  <div class="flex-1">
                    <div class="text-sm font-medium" style="color: var(--ds-text);">Custom Avatar</div>
                    <div class="text-xs" style="color: var(--ds-text-subtle);">Image uploaded</div>
                  </div>
                  <Button
                    variant="default"
                    size="sm"
                    onclick={removeAvatar}
                    icon={Trash2}
                  >
                    Remove
                  </Button>
                </div>
              {:else}
                <div class="flex items-center gap-3 p-3 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                  <div class="w-12 h-12 rounded-md flex items-center justify-center" style="background-color: {formData.color};">
                    <svelte:component this={iconMap[formData.icon] || Grip} size={20} color="white" />
                  </div>
                  <div class="flex-1">
                    <div class="text-sm font-medium" style="color: var(--ds-text);">Default Icon</div>
                    <div class="text-xs" style="color: var(--ds-text-subtle);">Using selected icon and color</div>
                  </div>
                </div>
              {/if}

              <!-- Upload Controls -->
              <div class="flex gap-2">
                <Button
                  variant="default"
                  size="sm"
                  onclick={() => showAvatarUpload = !showAvatarUpload}
                  icon={Camera}
                >
                  {formData.avatar_url ? 'Change Avatar' : 'Upload Avatar'}
                </Button>
              </div>

              <!-- Upload Input (shown when toggled) -->
              {#if showAvatarUpload}
                <div class="p-3 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                  <input
                    type="file"
                    accept="image/*"
                    onchange={(e) => handleAvatarUpload(e.target.files)}
                    disabled={uploadingAvatar}
                    class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-50"
                  />
                  {#if uploadingAvatar}
                    <div class="mt-2 text-sm text-blue-600">Uploading...</div>
                  {/if}
                </div>
              {/if}
            </div>
          </div>
        </div>
      </div>

      <div class="mt-6">
        <label class="flex items-center">
          <input
            type="checkbox"
            bind:checked={formData.active}
            class="mr-3 h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
          />
          <span class="text-sm" style="color: var(--ds-text);">Active workspace</span>
        </label>
      </div>
    </div>

    <!-- Modal Footer -->
    <div class="px-8 py-6 border-t flex gap-3" style="border-color: var(--ds-border);">
      <Button
        variant="primary"
        onclick={saveWorkspace}
        disabled={!formData.name.trim() || !formData.key.trim()}
        keyboardHint={getShortcutDisplay('workspaces', 'submitForm')}
      >
        {editingWorkspace ? 'Update' : 'Create'} Workspace
      </Button>
      <Button
        variant="default"
        onclick={cancelForm}
        keyboardHint={getShortcutDisplay('workspaces', 'cancelForm')}
      >
        Cancel
      </Button>
    </div>
  </div>
</Modal>
