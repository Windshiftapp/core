<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { workspacePermissions } from '../stores';
  import { Trash2, AlertTriangle, Palette, Camera, Package, Settings, Clock, Shield } from 'lucide-svelte';
  import { workspaceIconMap } from '../utils/icons.js';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import WorkspaceConfigurationAssigner from './WorkspaceConfigurationAssigner.svelte';
  import WorkspaceConfigurationPreview from './WorkspaceConfigurationPreview.svelte';
  import WorkspaceSCMSettings from './WorkspaceSCMSettings.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Input from '../components/Input.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import CategoryMultiSelect from '../pickers/CategoryMultiSelect.svelte';
  import WorkspaceMembers from './WorkspaceMembers.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import Label from '../components/Label.svelte';
  import { getHexFromColorName } from '../utils/colors.js';
  import Toggle from '../components/Toggle.svelte';
  import { successToast, errorToast } from '../stores/toasts.svelte.js';
  
  export let workspaceId;
  export let activeTab = 'general'; // 'general', 'configuration', or 'danger'
  
  let workspace = null;
  let loading = true;
  let saving = false;
  let showDeleteConfirm = false;
  let deleteConfirmText = '';
  let timeProjects = [];
  let configurationRefreshKey = 0;
  
  // Time project categories state
  let timeProjectCategories = [];
  let selectedTimeProjectCategories = [];
  
  let formData = {
    name: '',
    key: '',
    description: '',
    active: true,
    time_project_id: null,
    default_view: 'board',
    icon: 'Package',
    color: '#3b82f6',
    avatar_url: null
  };

  // Avatar upload state
  let uploadingAvatar = false;
  let showAvatarUpload = false;
  let attachmentSettings = null;

  // Check if attachments are enabled
  $: attachmentsEnabled = attachmentSettings?.enabled && attachmentSettings?.attachment_path;

  // Permission check for workspace admin
  $: canAdmin = workspacePermissions.canAdminWorkspace(workspaceId);

  // Use centralized icon map for workspace icons
  const iconMap = workspaceIconMap;
  
  onMount(async () => {
    await moduleSettings.load();

    // Redirect from base settings route to general tab
    if (window.location.pathname === `/workspaces/${workspaceId}/settings`) {
      navigate(`/workspaces/${workspaceId}/settings/general`);
      return;
    }

    // Load all required data
    const loadPromises = [loadWorkspace(), loadTimeProjectCategories()];
    if ($moduleSettings.time_tracking_enabled) {
      loadPromises.push(loadTimeProjects());
    }

    // Load attachment settings
    try {
      attachmentSettings = await api.attachmentSettings.get();
    } catch (error) {
      console.error('Failed to load attachment settings:', error);
    }

    await Promise.all(loadPromises);
    loading = false;
  });
  
  async function loadWorkspace() {
    try {
      workspace = await api.workspaces.get(workspaceId);
      if (workspace) {
        formData = {
          name: workspace.name,
          key: workspace.key || '',
          description: workspace.description || '',
          active: workspace.active,
          time_project_id: workspace.time_project_id || null,
          default_view: workspace.default_view || 'board',
          icon: workspace.icon || 'Package',
          color: workspace.color || '#3b82f6',
          avatar_url: workspace.avatar_url || null
        };
      }
    } catch (error) {
      console.error('Failed to load workspace:', error);
    }
  }

  async function loadTimeProjects() {
    try {
      timeProjects = await api.time.projects.getAll() || [];
    } catch (error) {
      console.error('Failed to load time projects:', error);
      timeProjects = [];
    }
  }
  
  async function loadTimeProjectCategories() {
    try {
      timeProjectCategories = await api.time.projectCategories.getAll() || [];
      // Load workspace's selected categories if they exist
      if (workspace?.time_project_categories) {
        selectedTimeProjectCategories = workspace.time_project_categories;
      }
    } catch (error) {
      console.error('Failed to load time project categories:', error);
      timeProjectCategories = [];
    }
  }
  
  
  
  async function saveWorkspace() {
    if (!formData.name.trim()) {
      showToastError('Workspace name is required');
      return;
    }
    
    if (!formData.key.trim()) {
      showToastError('Workspace key is required');
      return;
    }
    
    try {
      saving = true;
      await api.workspaces.update(workspaceId, {
        ...formData,
        time_project_categories: selectedTimeProjectCategories
      });
      
      // Update local workspace object
      workspace = { ...workspace, ...formData };

      // Show success toast
      showToastSuccess('Workspace settings saved successfully');
    } catch (error) {
      console.error('Failed to save workspace:', error);
      showToastError('Failed to save workspace settings: ' + (error.message || error));
    } finally {
      saving = false;
    }
  }
  
  function showToastSuccess(message) {
    successToast(message);
  }

  function showToastError(message) {
    errorToast(message);
  }
  
  function cancelDeleteWorkspace() {
    showDeleteConfirm = false;
    deleteConfirmText = '';
  }
  
  async function deleteWorkspace() {
    if (deleteConfirmText !== workspace.name) {
      showToastError('Please enter the workspace name exactly as shown to confirm deletion');
      return;
    }
    
    try {
      await api.workspaces.delete(workspaceId);
      showToastSuccess(`Workspace "${workspace.name}" deleted successfully`);
      // Navigate after showing the toast
      setTimeout(() => {
        navigate('/workspaces');
      }, 1000);
    } catch (error) {
      console.error('Failed to delete workspace:', error);
      showToastError('Failed to delete workspace: ' + (error.message || error));
    }
  }
  
  function goBackToWorkspace() {
    navigate(`/workspaces/${workspaceId}`);
  }

  function goBackToWorkspaceList() {
    navigate('/workspaces');
  }

  function switchTab(tab) {
    if (tab === 'general') {
      navigate(`/workspaces/${workspaceId}/settings/general`);
    } else if (tab === 'appearance') {
      navigate(`/workspaces/${workspaceId}/settings/appearance`);
    } else if (tab === 'categories') {
      navigate(`/workspaces/${workspaceId}/settings/categories`);
    } else if (tab === 'members') {
      navigate(`/workspaces/${workspaceId}/settings/members`);
    } else if (tab === 'configuration') {
      navigate(`/workspaces/${workspaceId}/settings/configuration`);
    } else if (tab === 'source-control') {
      navigate(`/workspaces/${workspaceId}/settings/source-control`);
    } else if (tab === 'danger') {
      navigate(`/workspaces/${workspaceId}/settings/danger`);
    } else {
      navigate(`/workspaces/${workspaceId}/settings`);
    }
  }

  // Avatar upload functionality
  async function handleAvatarUpload(files) {
    if (!files || files.length === 0) return;

    if (!attachmentsEnabled) {
      showToastError('Attachments must be enabled to upload workspace icons');
      return;
    }

    const file = files[0];
    if (!file.type.startsWith('image/')) {
      showToastError('Please select an image file');
      return;
    }

    uploadingAvatar = true;
    try {
      const uploadFormData = new FormData();
      uploadFormData.append('file', file);
      uploadFormData.append('item_id', workspaceId.toString());
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
        showToastSuccess('Avatar uploaded successfully');
      }
    } catch (err) {
      showToastError('Failed to upload avatar: ' + (err.message || err));
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

</script>



{#if loading}
  <div class="rounded-xl p-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <div class="animate-pulse">
      <div class="h-4 rounded w-1/4 mb-4" style="background-color: var(--ds-surface);"></div>
      <div class="h-4 rounded w-3/4" style="background-color: var(--ds-surface);"></div>
    </div>
  </div>
{:else if !canAdmin}
  <div class="rounded-xl p-8 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <div class="text-center py-8">
      <Shield class="w-12 h-12 mx-auto mb-4 text-amber-500" />
      <h2 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">Access Denied</h2>
      <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">You need workspace administrator permissions to access settings.</p>
      <Button onclick={() => navigate(`/workspaces/${workspaceId}`)} variant="primary">
        Back to Workspace
      </Button>
    </div>
  </div>
{:else if workspace}
  <div class="space-y-6">
    <!-- Header -->
    <div class="mb-6">
      <!-- Breadcrumb Navigation -->
      <div class="flex items-center gap-2 text-sm mb-4" style="color: var(--ds-text-subtle);">
        <button
          on:click={goBackToWorkspaceList}
          class="breadcrumb-link transition-colors"
        >
          Workspaces
        </button>
        <span>/</span>
        <button
          on:click={goBackToWorkspace}
          class="breadcrumb-link transition-colors"
        >
          {workspace.name}
        </button>
        <span>/</span>
        <span class="flex items-center gap-1" style="color: var(--ds-text);">
          <Settings class="w-4 h-4" style="color: #3b82f6;" />
          Settings
        </span>
      </div>

      <PageHeader
        icon={Settings}
        title="Settings"
        subtitle="Configure settings for {workspace?.name || 'workspace'}"
      />
    </div>

    <!-- Tab Navigation -->
    <div class="rounded border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      <div class="flex border-b" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
        <button
          class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative settings-tab"
          style="color: {activeTab === 'general' ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}; {activeTab === 'general' ? 'margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : ''}"
          on:click={() => switchTab('general')}
        >
          General
        </button>
        <button
          class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative settings-tab"
          style="color: {activeTab === 'appearance' ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}; {activeTab === 'appearance' ? 'margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : ''}"
          on:click={() => switchTab('appearance')}
        >
          Appearance
        </button>
        <button
          class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative settings-tab"
          style="color: {activeTab === 'categories' ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}; {activeTab === 'categories' ? 'margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : ''}"
          on:click={() => switchTab('categories')}
        >
          Categories
        </button>
        <button
          class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative settings-tab"
          style="color: {activeTab === 'members' ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}; {activeTab === 'members' ? 'margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : ''}"
          on:click={() => switchTab('members')}
        >
          Members
        </button>
        <button
          class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative settings-tab"
          style="color: {activeTab === 'configuration' ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}; {activeTab === 'configuration' ? 'margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : ''}"
          on:click={() => switchTab('configuration')}
        >
          Configuration Sets
        </button>
        <button
          class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative settings-tab"
          style="color: {activeTab === 'source-control' ? 'var(--ds-interactive)' : 'var(--ds-text-subtle)'}; {activeTab === 'source-control' ? 'margin-bottom: -1px; border-bottom: 2px solid var(--ds-interactive);' : ''}"
          on:click={() => switchTab('source-control')}
        >
          Source Control
        </button>
        <button
          class="flex items-center gap-2 px-4 py-3 text-sm font-medium transition-all relative settings-tab-danger"
          style="color: {activeTab === 'danger' ? 'var(--ds-text-danger)' : 'var(--ds-text-subtle)'}; {activeTab === 'danger' ? 'margin-bottom: -1px; border-bottom: 2px solid var(--ds-text-danger);' : ''}"
          on:click={() => switchTab('danger')}
        >
          Remove Workspace
        </button>
      </div>
    </div>

    <!-- Tab Content -->
    {#if activeTab === 'general'}
      <!-- Basic Information -->
      <div class="rounded-xl p-8 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h3 class="text-lg font-medium mb-6" style="color: var(--ds-text);">Basic Information</h3>
      
      <div class="space-y-6">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <Label for="workspace-name" required class="mb-2">Workspace Name</Label>
            <Input
              id="workspace-name"
              bind:value={formData.name}
              placeholder="Enter workspace name"
              required
            />
          </div>

          <div>
            <Label for="workspace-key" required class="mb-2">Workspace Key</Label>
            <Input
              id="workspace-key"
              bind:value={formData.key}
              placeholder="e.g., DEV, TEST, PROD"
              required
            />
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              Used for item prefixes (e.g., DEV-123). Uppercase letters and numbers only.
            </p>
          </div>
        </div>

        <div>
          <Label for="workspace-description" class="mb-2">Description</Label>
          <Textarea
            id="workspace-description"
            bind:value={formData.description}
            rows={3}
            placeholder="Optional description for this workspace"
          />
        </div>

        {#if $moduleSettings.time_tracking_enabled}
          <div>
            <Label for="workspace-project" class="mb-2">Default Time Tracking Project</Label>
            <Select
              id="workspace-project"
              bind:value={formData.time_project_id}
            >
              <option value={null}>No default project</option>
              {#each timeProjects as project}
                <option value={project.id}>{project.name} ({project.customer_name})</option>
              {/each}
            </Select>
            <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
              Default project used when logging time from work items in this workspace. Can be overridden per work item.
            </p>
          </div>
        {/if}

        <div>
          <Label for="workspace-view" class="mb-2">Default Workspace View</Label>
          <Select
            id="workspace-view"
            bind:value={formData.default_view}
          >
            <option value="board">Board</option>
            <option value="backlog">Backlog</option>
            <option value="list">List</option>
            <option value="tree">Tree</option>
            <option value="map">Map</option>
            <option value="overview">Overview</option>
          </Select>
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
            Default view displayed when entering this workspace.
          </p>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm font-medium mb-1" style="color: var(--ds-text);">
              Active Workspace
            </div>
            <p class="text-xs" style="color: var(--ds-text-subtle);">
              When inactive, only system admins and workspace admins can access this workspace. All data is preserved.
            </p>
          </div>
<Toggle bind:checked={formData.active} />
        </div>
      </div>
      </div>

      <div class="flex items-center gap-3 mt-6">
        <Button
          variant="primary"
          size="medium"
          on:click={saveWorkspace}
          disabled={saving || !formData.name.trim() || !formData.key.trim()}
        >
          {#if saving}Saving...{:else}Save Changes{/if}
        </Button>
        <Button
          variant="secondary"
          size="medium"
          on:click={loadWorkspace}
        >
          Reset
        </Button>
      </div>
    {:else if activeTab === 'appearance'}
      <!-- Visual Identity Settings -->
      <div class="rounded-xl p-8 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-center gap-3 mb-6">
          <Palette class="w-5 h-5" style="color: var(--ds-text-subtle);" />
          <h3 class="text-lg font-medium" style="color: var(--ds-text);">Visual Identity</h3>
        </div>
        
        <p class="text-sm mb-6" style="color: var(--ds-text-subtle);">
          Customize the visual appearance of your workspace with icons, colors, and avatars.
        </p>
        
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
          <!-- Icon and Color Selection -->
          <div>
            <IconSelector
              selectedIcon={formData.icon}
              selectedColor={formData.color}
              label="Workspace Icon & Color"
              compact={true}
              on:change={handleIconChange}
            />
          </div>

          <!-- Avatar Upload -->
          <div>
            <Label class="mb-2">Workspace Avatar</Label>
            
            <div class="space-y-4">
              <!-- Current Avatar Display -->
              {#if formData.avatar_url}
                <div class="flex items-center gap-4 p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                  <img src={formData.avatar_url} alt="Workspace avatar" class="w-16 h-16 rounded object-cover" />
                  <div class="flex-1">
                    <div class="text-sm font-medium" style="color: var(--ds-text);">Custom Avatar</div>
                    <div class="text-xs" style="color: var(--ds-text-subtle);">Image uploaded successfully</div>
                  </div>
                  <Button
                    variant="default"
                    size="sm"
                    on:click={removeAvatar}
                    icon={Trash2}
                  >
                    Remove
                  </Button>
                </div>
              {:else}
                <div class="flex items-center gap-4 p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                  <div class="w-16 h-16 rounded flex items-center justify-center" style="background-color: {formData.color};">
                    <svelte:component this={iconMap[formData.icon] || Package} size={32} color="white" />
                  </div>
                  <div class="flex-1">
                    <div class="text-sm font-medium" style="color: var(--ds-text);">Default Icon</div>
                    <div class="text-xs" style="color: var(--ds-text-subtle);">Using selected icon and color</div>
                  </div>
                </div>
              {/if}

              <!-- Upload Controls -->
              <div>
                <Button
                  variant="default"
                  size="sm"
                  on:click={() => showAvatarUpload = !showAvatarUpload}
                  icon={Camera}
                  disabled={!attachmentsEnabled}
                >
                  {formData.avatar_url ? 'Change Avatar' : 'Upload Avatar'}
                </Button>
                {#if !attachmentsEnabled}
                  <p class="text-xs mt-1" style="color: var(--ds-text-warning);">
                    Attachments must be enabled to upload workspace icons
                  </p>
                {/if}
              </div>

              <!-- Upload Input (shown when toggled) -->
              {#if showAvatarUpload && attachmentsEnabled}
                <div class="p-4 rounded border" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                  <input
                    type="file"
                    accept="image/*"
                    on:change={(e) => handleAvatarUpload(e.target.files)}
                    disabled={uploadingAvatar}
                    class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-50"
                  />
                  {#if uploadingAvatar}
                    <div class="mt-2 text-sm text-blue-600">Uploading...</div>
                  {/if}
                  <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
                    Recommended: Square images, at least 256x256 pixels for best quality
                  </p>
                </div>
              {/if}
            </div>
            
            <p class="text-xs mt-3" style="color: var(--ds-text-subtle);">
              You can either use a custom avatar image or the icon & color combination above.
            </p>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3 mt-6">
        <Button
          variant="primary"
          size="medium"
          on:click={saveWorkspace}
          disabled={saving || !formData.name.trim() || !formData.key.trim()}
        >
          {#if saving}Saving...{:else}Save Changes{/if}
        </Button>
        <Button
          variant="secondary"
          size="medium"
          on:click={loadWorkspace}
        >
          Reset
        </Button>
      </div>
    {:else if activeTab === 'categories'}
      <!-- Project Category Restrictions -->
      <div class="rounded-xl border shadow-sm p-8" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-center gap-3 mb-6">
          <Clock class="w-5 h-5" style="color: var(--ds-text-subtle);" />
          <h3 class="text-lg font-medium" style="color: var(--ds-text);">Project Category Restrictions</h3>
        </div>

        <CategoryMultiSelect
          categories={timeProjectCategories}
          bind:selectedIds={selectedTimeProjectCategories}
          placeholder="Select project categories..."
          helperText="Optionally restrict project selection to specific categories for this workspace. When set, users can only select projects from the chosen categories."
        />

        <p class="text-xs mt-3" style="color: var(--ds-text-subtle);">
          Note: Leave empty to allow selection from all project categories.
        </p>
      </div>

      <div class="flex items-center gap-3 mt-6">
        <Button
          variant="primary"
          size="medium"
          on:click={saveWorkspace}
          disabled={saving || !formData.name.trim() || !formData.key.trim()}
        >
          {#if saving}Saving...{:else}Save Changes{/if}
        </Button>
        <Button
          variant="secondary"
          size="medium"
          on:click={loadWorkspace}
        >
          Reset
        </Button>
      </div>
    {:else if activeTab === 'members'}
      <!-- Workspace Members -->
      <div class="rounded-xl border shadow-sm p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <WorkspaceMembers {workspaceId} />
      </div>
    {:else if activeTab === 'configuration'}
      <!-- Configuration Sets -->
      <div class="rounded-xl border shadow-sm p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <WorkspaceConfigurationAssigner workspaceId={workspaceId} on:configurationChanged={() => configurationRefreshKey++} />
      </div>

      <!-- Active Configuration Preview -->
      <div class="rounded-xl border shadow-sm p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h3 class="text-lg font-medium mb-4" style="color: var(--ds-text);">Active Configuration</h3>
        {#key configurationRefreshKey}
          <WorkspaceConfigurationPreview {workspaceId} />
        {/key}
      </div>

    {:else if activeTab === 'source-control'}
      <!-- Source Control Settings -->
      <div class="rounded-xl border shadow-sm p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <WorkspaceSCMSettings {workspaceId} />
      </div>

    {:else if activeTab === 'danger'}
      <!-- Remove Workspace -->
      <div class="rounded-xl p-8 border border-red-200 shadow-sm" style="background-color: #fef2f2;">
        <div class="flex items-center gap-3 mb-6">
          <AlertTriangle class="w-5 h-5 text-red-600" />
          <h3 class="text-lg font-medium text-red-900">Permanent Removal</h3>
        </div>
        
        <div class="text-sm text-red-700 mb-6">
          <p class="mb-4">Removing this workspace will permanently delete:</p>
          <ul class="list-disc list-inside space-y-2 ml-4">
            <li>All work items and projects in this workspace</li>
            <li>All custom field configurations</li>
            <li>All screen configurations</li>
            <li>All uploaded files associated with work items</li>
          </ul>
          <p class="mt-4 font-medium">This action cannot be undone.</p>
        </div>

        {#if !showDeleteConfirm}
          <button
            on:click={() => showDeleteConfirm = true}
            class="flex items-center gap-2 px-4 py-2 bg-red-600 text-white text-sm font-medium rounded hover:bg-red-700 transition-colors"
          >
            <Trash2 class="w-4 h-4" />
            Remove Workspace
          </button>
        {:else}
          <div class="space-y-4">
            <div>
              <label for="delete-confirm" class="block text-sm font-medium text-red-900 mb-2">
                Type <strong>{workspace.name}</strong> to confirm removal:
              </label>
              <input
                id="delete-confirm"
                type="text"
                bind:value={deleteConfirmText}
                class="w-full px-4 py-2 rounded border border-red-300 text-red-900 bg-white focus:outline-none focus:ring-2 focus:ring-red-500"
                placeholder="Type '{workspace.name}' here"
              />
            </div>
            
            <div class="flex items-center gap-3">
              <button
                on:click={deleteWorkspace}
                disabled={deleteConfirmText !== workspace.name}
                class="px-4 py-2 bg-red-600 text-white text-sm font-medium rounded hover:bg-red-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Yes, Remove Workspace
              </button>
              <button
                on:click={cancelDeleteWorkspace}
                class="px-4 py-2 text-sm font-medium rounded border border-red-300 text-red-700 hover:bg-red-50 transition-colors"
              >
                Cancel
              </button>
            </div>
          </div>
        {/if}
      </div>
    {/if}

  </div>
{:else}
  <div class="rounded-xl p-6 border shadow-sm" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <p class="text-center" style="color: var(--ds-text-subtle);">Workspace not found.</p>
  </div>
{/if}

<style>
  .settings-tab:hover {
    color: var(--ds-text) !important;
  }

  .settings-tab-danger:hover {
    color: var(--ds-text-danger) !important;
  }

  .breadcrumb-link:hover {
    color: var(--ds-text) !important;
  }

  .toggle-off {
    background-color: var(--ds-surface);
  }
</style>