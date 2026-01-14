<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { writable } from 'svelte/store';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Label from '../components/Label.svelte';
  import { Plus, Link, Edit, Trash2, Power, PowerOff } from 'lucide-svelte';
  import ColorPicker from '../editors/ColorPicker.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { matchesShortcut } from '../utils/keyboardShortcuts.js';

  const linkTypes = writable([]);

  let showForm = false;
  let editingLinkType = null;
  let formData = {
    name: '',
    description: '',
    forward_label: '',
    reverse_label: '',
    color: '#6b7280',
    active: true
  };

  onMount(() => {
    loadLinkTypes();
    
    // Add keyboard shortcut for new link type
    function handleKeydown(event) {
      if (matchesShortcut(event, { key: 'a' })) {
        // Don't trigger if we're in an input/textarea
        if (event.target.tagName !== 'INPUT' && event.target.tagName !== 'TEXTAREA') {
          event.preventDefault();
          showAddForm();
        }
      }
    }
    
    document.addEventListener('keydown', handleKeydown);
    return () => document.removeEventListener('keydown', handleKeydown);
  });

  async function loadLinkTypes() {
    try {
      const types = await api.linkTypes.getAll(true); // include inactive
      linkTypes.set(types || []);
    } catch (error) {
      console.error('Failed to load link types:', error);
    }
  }

  function showAddForm() {
    showForm = true;
    editingLinkType = null;
    formData = {
      name: '',
      description: '',
      forward_label: '',
      reverse_label: '',
      color: '#6b7280',
      active: true
    };
  }

  function showEditForm(linkType) {
    showForm = true;
    editingLinkType = linkType;
    formData = {
      name: linkType.name,
      description: linkType.description,
      forward_label: linkType.forward_label,
      reverse_label: linkType.reverse_label,
      color: linkType.color,
      active: linkType.active
    };
  }

  async function handleSubmit() {
    try {
      if (editingLinkType) {
        await api.linkTypes.update(editingLinkType.id, formData);
      } else {
        await api.linkTypes.create(formData);
      }
      await loadLinkTypes();
      showForm = false;
    } catch (error) {
      console.error('Failed to save link type:', error);
      alert('Failed to save link type: ' + error.message);
    }
  }

  async function deleteLinkType(id, isSystem) {
    if (isSystem) {
      alert('Cannot delete system link types');
      return;
    }
    
    if (confirm('Are you sure you want to delete this link type? This will also remove all links of this type.')) {
      try {
        await api.linkTypes.delete(id);
        await loadLinkTypes();
      } catch (error) {
        console.error('Failed to delete link type:', error);
        alert('Failed to delete link type: ' + error.message);
      }
    }
  }

  async function toggleActive(linkType) {
    try {
      await api.linkTypes.update(linkType.id, {
        ...linkType,
        active: !linkType.active
      });
      await loadLinkTypes();
    } catch (error) {
      console.error('Failed to toggle link type status:', error);
      alert('Failed to toggle link type status: ' + error.message);
    }
  }

  function getStatusBadge(linkType) {
    if (linkType.is_system) {
      return { text: 'System', color: 'blue' };
    } else if (linkType.active) {
      return { text: 'Active', color: 'green' };
    } else {
      return { text: 'Inactive', color: 'gray' };
    }
  }

  // DataTable columns configuration
  const linkTypeColumns = [
    {
      key: 'name',
      label: 'Name',
      slot: 'name'
    },
    {
      key: 'color',
      label: 'Color',
      slot: 'color'
    },
    {
      key: 'status',
      label: 'Status',
      slot: 'status'
    },
    {
      key: 'actions',
      label: 'Actions'
    }
  ];

  // Build dropdown action items for each link type
  function buildLinkTypeActionItems(linkType) {
    const items = [];

    // Only show edit/delete for non-system types
    if (!linkType.is_system) {
      items.push({
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: 'Edit',
        hoverClass: 'hover-bg',
        onClick: () => showEditForm(linkType)
      });

      items.push({
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: 'Delete',
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteLinkType(linkType.id, linkType.is_system)
      });
    }

    // Add activate/deactivate for all types
    items.push({
      id: linkType.active ? 'deactivate' : 'activate',
      type: 'regular',
      icon: linkType.active ? PowerOff : Power,
      title: linkType.active ? 'Deactivate' : 'Activate',
      color: linkType.active ? '#f59e0b' : '#10b981',
      hoverClass: linkType.active ? 'hover:bg-orange-50' : 'hover:bg-green-50',
      onClick: () => toggleActive(linkType)
    });

    return items;
  }
</script>

<PageHeader 
  icon={Link} 
  title="Link Types" 
  subtitle="Manage relationship types between work items and test cases"
>
  {#snippet actions()}
    <Button
      variant="primary"
      onclick={showAddForm}
      icon={Plus}
      size="medium"
      keyboardHint="A"
    >
      Add Link Type
    </Button>
  {/snippet}
</PageHeader>

<Modal isOpen={showForm} onclose={() => showForm = false} maxWidth="max-w-2xl">
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {editingLinkType ? 'Edit Link Type' : 'Create Link Type'}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
        <div>
          <Label color="default" class="mb-2">Name</Label>
          <input
            type="text"
            bind:value={formData.name}
            required
            placeholder="e.g., Implements"
            class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
        </div>
        <div>
          <ColorPicker bind:value={formData.color} label="Color" />
        </div>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
        <div>
          <Label color="default" class="mb-2">Forward Label</Label>
          <input
            type="text"
            bind:value={formData.forward_label}
            required
            placeholder="e.g., implements"
            class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">When A links to B, show as "A implements B"</p>
        </div>
        <div>
          <Label color="default" class="mb-2">Reverse Label</Label>
          <input
            type="text"
            bind:value={formData.reverse_label}
            required
            placeholder="e.g., implemented by"
            class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
            style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
          />
          <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">When B is linked from A, show as "B implemented by A"</p>
        </div>
      </div>

      <div class="mb-4">
        <Label color="default" class="mb-2">Description</Label>
        <Textarea
          bind:value={formData.description}
          rows={3}
          placeholder="Optional description of this relationship type"
        />
      </div>

      <div class="mb-4">
        <label class="flex items-center">
          <input
            type="checkbox"
            bind:checked={formData.active}
            class="mr-2"
          />
          <span class="text-sm" style="color: var(--ds-text);">Active</span>
        </label>
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={() => showForm = false}
    onConfirm={handleSubmit}
    confirmLabel={editingLinkType ? 'Update Link Type' : 'Create Link Type'}
    disabled={!formData.name || !formData.forward_label || !formData.reverse_label}
  />
</Modal>

<DataTable
  columns={linkTypeColumns}
  data={$linkTypes}
  keyField="id"
  emptyMessage="No link types found. Create your first link type to enable item relationships."
  emptyIcon={Link}
  actionItems={buildLinkTypeActionItems}
>
  <!-- Name column with description -->
  <div slot="name" let:item={linkType}>
    <div>
      <div class="text-sm font-medium" style="color: var(--ds-text);">{linkType.name}</div>
      {#if linkType.description}
        <div class="text-sm" style="color: var(--ds-text-subtle);">{linkType.description}</div>
      {/if}
    </div>
  </div>

  <!-- Color column with preview and hex code -->
  <div slot="color" let:item={linkType}>
    <div class="flex items-center gap-2">
      <div
        class="w-6 h-6 rounded border border-gray-300"
        style="background-color: {linkType.color};"
      ></div>
      <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">{linkType.color}</span>
    </div>
  </div>

  <!-- Status column with badge -->
  <Lozenge slot="status" let:item={linkType} color={getStatusBadge(linkType).color} text={getStatusBadge(linkType).text} />
</DataTable>
