<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { Plus, Edit, Trash2, Shield } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { matchesShortcut } from '../utils/keyboardShortcuts.js';

  let permissionSets = [];
  let loading = true;
  let showCreateModal = false;

  // Form state for create modal
  let formData = {
    name: '',
    description: ''
  };

  onMount(async () => {
    await loadPermissionSets();

    // Add global keyboard listener
    window.addEventListener('keydown', handleGlobalKeydown);
  });

  onDestroy(() => {
    window.removeEventListener('keydown', handleGlobalKeydown);
  });

  function handleGlobalKeydown(event) {
    // 'a' to add/create new permission set
    if (matchesShortcut(event, { key: 'a' }) && !showCreateModal) {
      const target = event.target;
      if (target.tagName !== 'INPUT' && target.tagName !== 'TEXTAREA' && !target.contentEditable.includes('true')) {
        event.preventDefault();
        startCreate();
      }
    }
  }

  function handleModalKeydown(event) {
    // Enter to submit (only if not in textarea)
    if (event.key === 'Enter' && !event.shiftKey && !event.ctrlKey && !event.metaKey) {
      const target = event.target;
      if (target.tagName !== 'TEXTAREA') {
        event.preventDefault();
        createPermissionSet();
      }
    }
    // Escape to cancel
    if (event.key === 'Escape') {
      event.preventDefault();
      cancelCreate();
    }
  }

  async function loadPermissionSets() {
    try {
      loading = true;
      const data = await api.get('/permission-sets') || [];
      permissionSets = data;
    } catch (error) {
      console.error('Failed to load permission sets:', error);
      permissionSets = [];
    } finally {
      loading = false;
    }
  }

  function startCreate() {
    formData = {
      name: '',
      description: ''
    };
    showCreateModal = true;
  }

  function startEdit(permSet) {
    navigate(`/admin/permission-sets/${permSet.id}`);
  }

  function cancelCreate() {
    showCreateModal = false;
    formData = {
      name: '',
      description: ''
    };
  }

  async function createPermissionSet() {
    try {
      if (!formData.name.trim()) {
        alert('Name is required');
        return;
      }

      const created = await api.post('/permission-sets', {
        name: formData.name,
        description: formData.description,
        permission_ids: []
      });
      permissionSets = [...permissionSets, created];
      cancelCreate();
    } catch (error) {
      console.error('Failed to create permission set:', error);
      alert('Failed to create permission set: ' + (error.message || error));
    }
  }

  async function deletePermissionSet(permSet) {
    const confirmed = await confirm({
      title: 'Delete Permission Set',
      message: `Are you sure you want to delete the permission set "${permSet.name}"? This action cannot be undone.`,
      confirmText: 'Delete',
      cancelText: 'Cancel',
      variant: 'danger',
      icon: Trash2
    });

    if (!confirmed) return;

    try {
      await api.delete(`/permission-sets/${permSet.id}`);
      permissionSets = permissionSets.filter(ps => ps.id !== permSet.id);
    } catch (error) {
      console.error('Failed to delete permission set:', error);
      alert('Failed to delete permission set: ' + (error.message || error));
    }
  }

  function buildPermSetDropdownItems(permSet) {
    const items = [
      {
        id: 'edit',
        title: 'Edit',
        icon: Edit,
        iconColor: '#3b82f6',
        onClick: () => startEdit(permSet)
      }
    ];

    if (!permSet.is_system) {
      items.push({
        id: 'delete',
        title: 'Delete',
        icon: Trash2,
        iconColor: '#dc2626',
        onClick: () => deletePermissionSet(permSet),
        color: '#dc2626'
      });
    }

    return items;
  }

  const columns = [
    { key: 'name', label: 'Name', sortable: true },
    { key: 'description', label: 'Description' },
    {
      key: 'is_system',
      label: 'Type',
      render: (item) => item.is_system ? 'System' : 'Custom',
      sortable: true
    },
    { key: 'actions', label: '', width: 'w-16' }
  ];
</script>

<div class="space-y-6">
  <PageHeader
    title="Permission Sets"
    subtitle="Manage bundles of permissions that can be assigned to configuration sets"
    icon={Shield}
  >
    {#snippet actions()}
      <Button onclick={startCreate} size="sm" variant="primary" keyboardHint="A">
        <Plus class="w-4 h-4 mr-2" />
        Create Permission Set
      </Button>
    {/snippet}
  </PageHeader>

  <!-- Create Modal -->
  {#if showCreateModal}
    <div
      class="fixed inset-0 flex items-center justify-center p-4 z-50"
      style="background-color: rgba(0, 0, 0, 0.3); backdrop-filter: blur(2px);"
      onkeydown={handleModalKeydown}
    >
      <div class="rounded shadow-xl max-w-lg w-full p-6" style="background-color: var(--ds-surface-overlay)">
        <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text)">Create Permission Set</h3>

        <div class="space-y-4">
          <div>
            <Label color="default" required class="mb-1">Name</Label>
            <input
              type="text"
              bind:value={formData.name}
              class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text)"
              placeholder="e.g., Development Team Permissions"
              autofocus
            />
          </div>

          <div>
            <Label color="default" class="mb-1">Description</Label>
            <Textarea
              bind:value={formData.description}
              rows={2}
              placeholder="Optional description of this permission set"
            />
          </div>
        </div>

        <div class="flex justify-end space-x-3 mt-6">
          <Button variant="secondary" onclick={cancelCreate} keyboardHint="Esc">
            Cancel
          </Button>
          <Button variant="primary" onclick={createPermissionSet} keyboardHint="↵">
            Create
          </Button>
        </div>
      </div>
    </div>
  {/if}

  <DataTable
    data={permissionSets}
    {columns}
    {loading}
    actionItems={buildPermSetDropdownItems}
    emptyMessage="No permission sets found. Create one to get started."
  />
</div>
