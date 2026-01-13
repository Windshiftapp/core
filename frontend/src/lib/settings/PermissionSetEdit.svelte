<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../router.js';
  import { api } from '../api.js';
  import { ArrowLeft, Save, X, UserPlus } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import RolePicker from '../pickers/RolePicker.svelte';
  import GroupPicker from '../pickers/GroupPicker.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import { confirm } from '../composables/useConfirm.js';

  let permissionSetId = null;
  let permissionSet = null;
  let permissions = [];
  let loading = true;
  let showAssignmentPicker = false;
  let assignmentPickerPermissionId = null;

  // Form state
  let formData = {
    name: '',
    description: ''
  };

  // Original form data for change tracking
  let originalFormData = {
    name: '',
    description: ''
  };

  // Assignment data
  let assignments = {
    role_assignments: [],
    group_assignments: [],
    user_assignments: []
  };

  // Reactive variable to force UI updates
  let assignmentsVersion = 0;

  // Reactive: Check if name/description have unsaved changes
  $: hasUnsavedChanges =
    formData.name !== originalFormData.name ||
    formData.description !== originalFormData.description;

  // Create a reactive derived state that combines permissions with their assignments
  $: permissionsWithAssignments = permissions.map(permission => {
    // Force re-evaluation when assignmentsVersion changes
    const _ = assignmentsVersion;

    const roleAssigns = assignments.role_assignments?.filter(a => a.permission_id === permission.id) || [];
    const groupAssigns = assignments.group_assignments?.filter(a => a.permission_id === permission.id) || [];
    const userAssigns = assignments.user_assignments?.filter(a => a.permission_id === permission.id) || [];

    return {
      ...permission,
      assigns: { roleAssigns, groupAssigns, userAssigns }
    };
  });

  // Subscribe to route changes
  $: if ($currentRoute.params?.id) {
    const newId = parseInt($currentRoute.params.id);
    if (newId && newId !== permissionSetId) {
      permissionSetId = newId;
      loadData();
    }
  }

  onMount(() => {
    loadPermissions();
  });

  async function loadData() {
    if (!permissionSetId) {
      loading = false;
      return;
    }

    try {
      loading = true;

      const [setData, assignmentData] = await Promise.all([
        api.get(`/permission-sets/${permissionSetId}`),
        api.get(`/permission-sets/${permissionSetId}/assignments`)
      ]);

      permissionSet = setData;
      formData = {
        name: setData.name || '',
        description: setData.description || ''
      };
      originalFormData = {
        name: setData.name || '',
        description: setData.description || ''
      };

      assignments = assignmentData || {
        role_assignments: [],
        group_assignments: [],
        user_assignments: []
      };

      assignmentsVersion++;
    } catch (error) {
      console.error('Failed to load permission set:', error);
      alert('Failed to load permission set: ' + (error.message || JSON.stringify(error)));
    } finally {
      loading = false;
    }
  }

  async function loadPermissions() {
    try {
      const data = await api.get('/permissions') || [];
      const workspacePerms = data.filter(p => p.scope === 'workspace');

      // Define permission order (least to most privileged)
      const permissionOrder = [
        'item.view',
        'item.edit',
        'item.delete',
        'item.comment',
        'comment.edit_others',
        'workspace.admin'
      ];

      // Sort permissions by defined order
      permissions = workspacePerms.sort((a, b) => {
        const indexA = permissionOrder.indexOf(a.permission_key);
        const indexB = permissionOrder.indexOf(b.permission_key);

        // If both are in the order list, sort by their position
        if (indexA !== -1 && indexB !== -1) {
          return indexA - indexB;
        }

        // Unknown permissions go to the end
        if (indexA === -1 && indexB !== -1) return 1;
        if (indexA !== -1 && indexB === -1) return -1;

        // If both unknown, sort alphabetically
        return a.permission_key.localeCompare(b.permission_key);
      });
    } catch (error) {
      console.error('Failed to load permissions:', error);
      permissions = [];
    }
  }

  async function updateMetadata() {
    try {
      if (!formData.name.trim()) {
        alert('Name is required');
        return;
      }

      const updated = await api.put(`/permission-sets/${permissionSetId}`, {
        name: formData.name,
        description: formData.description,
        permission_ids: []
      });

      permissionSet = updated;

      // Update original form data to reflect saved state
      originalFormData = {
        name: formData.name,
        description: formData.description
      };
    } catch (error) {
      console.error('Failed to update permission set:', error);
      alert('Failed to update permission set: ' + (error.message || error));
    }
  }

  function openAssignmentPicker(permissionId) {
    assignmentPickerPermissionId = permissionId;
    showAssignmentPicker = true;
  }

  async function addAssignment(type, entityId) {
    try {
      const payload = {
        permission_id: assignmentPickerPermissionId
      };

      if (type === 'role') payload.role_id = entityId;
      else if (type === 'group') payload.group_id = entityId;
      else if (type === 'user') payload.user_id = entityId;

      await api.post(`/permission-sets/${permissionSetId}/assignments`, payload);

      // Reload assignments
      const assignmentData = await api.get(`/permission-sets/${permissionSetId}/assignments`);

      assignments = {
        role_assignments: assignmentData.role_assignments || [],
        group_assignments: assignmentData.group_assignments || [],
        user_assignments: assignmentData.user_assignments || []
      };

      // Increment version to trigger reactive update
      assignmentsVersion++;

      // Close modal after successful add
      showAssignmentPicker = false;
      assignmentPickerPermissionId = null;
    } catch (error) {
      console.error('Failed to add assignment:', error);

      // If duplicate (409), just close modal silently
      if (error.message && error.message.includes('already exists')) {
        showAssignmentPicker = false;
        assignmentPickerPermissionId = null;
      } else {
        alert('Failed to add assignment: ' + (error.message || error));
      }
    }
  }

  async function removeAssignment(assignmentId, type) {
    const confirmed = await confirm({
      title: 'Remove Assignment',
      message: 'Are you sure you want to remove this assignment?',
      confirmText: 'Remove',
      cancelText: 'Cancel',
      variant: 'danger',
      icon: X
    });

    if (!confirmed) return;

    try {
      await api.delete(`/permission-sets/${permissionSetId}/assignments/${assignmentId}?type=${type}`);

      // Reload assignments
      const assignmentData = await api.get(`/permission-sets/${permissionSetId}/assignments`);
      assignments = {
        role_assignments: assignmentData.role_assignments || [],
        group_assignments: assignmentData.group_assignments || [],
        user_assignments: assignmentData.user_assignments || []
      };

      // Increment version to trigger reactive update
      assignmentsVersion++;
    } catch (error) {
      console.error('Failed to remove assignment:', error);
      alert('Failed to remove assignment: ' + (error.message || error));
    }
  }

  function goBack() {
    navigate('/admin/permission-sets');
  }
</script>

<!-- Header -->
<div class="border-b px-6 py-4" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  <div class="flex items-center gap-4">
    <button
      onclick={goBack}
      class="transition-colors"
      style="color: var(--ds-text-subtle);"
      title="Back to Permission Sets"
    >
      <ArrowLeft class="w-5 h-5" />
    </button>
    <div>
      <h1 class="text-xl font-semibold" style="color: var(--ds-text);">
        {loading ? 'Loading...' : permissionSet?.name || 'Permission Set'}
      </h1>
      <p class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">
        Manage permission set details and assignments
      </p>
    </div>
  </div>
</div>

<!-- Content -->
<div class="flex-1 overflow-y-auto p-6" style="background-color: var(--ds-surface);">
  {#if loading}
    <div class="flex items-center justify-center h-64">
      <div style="color: var(--ds-text-subtle);">Loading permission set...</div>
    </div>
  {:else}
      <div class="max-w-5xl mx-auto space-y-6">
      <!-- Basic Information Section -->
      <div class="rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Basic Information</h2>
        <div class="space-y-4">
          <div>
            <Label color="default" required class="mb-1">Name</Label>
            <input
              type="text"
              bind:value={formData.name}
              class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
              placeholder="e.g., Development Team Permissions"
            />
          </div>

          <div>
            <Label color="default" class="mb-1">Description</Label>
            <Textarea
              bind:value={formData.description}
              rows={3}
              placeholder="Optional description of this permission set"
            />
          </div>

          {#if hasUnsavedChanges}
            <div class="flex justify-end pt-2 border-t" style="border-color: var(--ds-border);">
              <Button variant="primary" size="sm" onclick={updateMetadata}>
                <Save class="w-4 h-4 mr-2" />
                Save Changes
              </Button>
            </div>
          {/if}
        </div>
      </div>

      <!-- Permission Assignments Section -->
      <div class="rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <h2 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">Permission Assignments</h2>
          <div class="space-y-3">
          {#each permissionsWithAssignments as permission (permission.id)}
            <div class="border rounded p-4 transition-colors" style="border-color: var(--ds-border);">
              <div class="flex items-start justify-between">
                <div class="flex-1">
                  <div class="font-medium text-sm" style="color: var(--ds-text);">{permission.permission_name}</div>
                  <div class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{permission.description}</div>
                  <div class="text-xs mt-0.5 font-mono" style="color: var(--ds-text-subtlest);">{permission.permission_key}</div>

                    <!-- Assigned entities -->
                    <div class="flex flex-wrap gap-2 mt-3">
                      {#each permission.assigns.roleAssigns as roleAssign}
                        <span class="inline-flex items-center px-2.5 py-1 rounded-full text-xs bg-blue-100 text-blue-800 font-medium">
                          <span class="mr-1.5">Role:</span> {roleAssign.role?.name || 'Unknown'}
                          <button
                            onclick={() => removeAssignment(roleAssign.id, 'role')}
                            class="ml-1.5 hover:text-blue-900"
                          >
                            <X class="w-3 h-3" />
                          </button>
                        </span>
                      {/each}

                      {#each permission.assigns.groupAssigns as groupAssign}
                        <span class="inline-flex items-center px-2.5 py-1 rounded-full text-xs bg-green-100 text-green-800 font-medium">
                          <span class="mr-1.5">Group:</span> {groupAssign.group?.group_name || 'Unknown'}
                          <button
                            onclick={() => removeAssignment(groupAssign.id, 'group')}
                            class="ml-1.5 hover:text-green-900"
                          >
                            <X class="w-3 h-3" />
                          </button>
                        </span>
                      {/each}

                      {#each permission.assigns.userAssigns as userAssign}
                        <span class="inline-flex items-center px-2.5 py-1 rounded-full text-xs bg-purple-100 text-purple-800 font-medium">
                          <span class="mr-1.5">User:</span> {userAssign.user?.username || 'Unknown'}
                          <button
                            onclick={() => removeAssignment(userAssign.id, 'user')}
                            class="ml-1.5 hover:text-purple-900"
                          >
                            <X class="w-3 h-3" />
                          </button>
                        </span>
                      {/each}

                      <button
                      onclick={() => openAssignmentPicker(permission.id)}
                      class="inline-flex items-center px-2.5 py-1 rounded-full text-xs border-2 border-dashed transition-colors hover:border-blue-400 hover:text-blue-600 hover:bg-blue-50"
                      style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
                    >
                      <UserPlus class="w-3 h-3 mr-1" />
                      Add
                    </button>
                    </div>
                  </div>
                </div>
              </div>
            {/each}
          </div>
        </div>
      </div>
    {/if}
</div>

<!-- Assignment Picker Modal -->
<Modal
  isOpen={showAssignmentPicker}
  maxWidth="max-w-md"
  onclose={() => {
    showAssignmentPicker = false;
    assignmentPickerPermissionId = null;
  }}
>
  <div class="p-6">
    <h3 class="text-lg font-semibold mb-2" style="color: var(--ds-text);">Add Assignment</h3>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
      Select a role, group, or user below to add them to this permission. The assignment will be added immediately when selected.
    </p>

    <div class="space-y-4 mb-6">
      <!-- Role Selection -->
      <div>
        <RolePicker
          label="Add by Role"
          placeholder="Search and select a role..."
          onselect={(e) => addAssignment('role', e.detail.id)}
        />
      </div>

      <!-- Group Selection -->
      <div>
        <GroupPicker
          label="Add by Group"
          placeholder="Search and select a group..."
          onselect={(e) => addAssignment('group', e.detail.id)}
        />
      </div>

      <!-- User Selection -->
      <div>
        <UserPicker
          label="Add by User"
          placeholder="Search and select a user..."
          onselect={(e) => addAssignment('user', e.detail.id)}
        />
      </div>
    </div>

    <div class="flex justify-end">
      <Button variant="secondary" onclick={() => {
        showAssignmentPicker = false;
        assignmentPickerPermissionId = null;
      }}>
        Done
      </Button>
    </div>
  </div>
</Modal>
