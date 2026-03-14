<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { writable } from 'svelte/store';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import Button from '../components/Button.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import { Plus, Link, Edit, Trash2, Power, PowerOff } from 'lucide-svelte';
  import ColorPicker from '../editors/ColorPicker.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  const linkTypes = writable([]);

  let showForm = $state(false);
  let editingLinkType = $state(null);
  let formData = $state({
    name: '',
    description: '',
    forward_label: '',
    reverse_label: '',
    color: '#6b7280',
    active: true
  });

  onMount(() => {
    loadLinkTypes();
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
      alert(t('settings.linkTypes.failedToSave') + ' ' + error.message);
    }
  }

  async function deleteLinkType(id, isSystem) {
    if (isSystem) {
      alert(t('settings.linkTypes.cannotDeleteSystem'));
      return;
    }

    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteLinkType'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await api.linkTypes.delete(id);
        await loadLinkTypes();
      } catch (error) {
        console.error('Failed to delete link type:', error);
        alert(t('dialogs.alerts.failedToDelete', { error: error.message }));
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
      alert(t('dialogs.alerts.failedToToggleStatus', { error: error.message }));
    }
  }

  function getStatusBadge(linkType) {
    if (linkType.is_system) {
      return { text: t('settings.linkTypes.system'), color: 'blue' };
    } else if (linkType.active) {
      return { text: t('settings.linkTypes.active'), color: 'green' };
    } else {
      return { text: t('settings.linkTypes.inactive'), color: 'gray' };
    }
  }

  // DataTable columns configuration
  const linkTypeColumns = $derived([
    {
      key: 'name',
      label: t('settings.linkTypes.name'),
      slot: 'name'
    },
    {
      key: 'color',
      label: t('settings.linkTypes.color'),
      slot: 'color'
    },
    {
      key: 'status',
      label: t('common.status'),
      slot: 'status'
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ]);

  // Build dropdown action items for each link type
  function buildLinkTypeActionItems(linkType) {
    const items = [];

    // Only show edit/delete for non-system types
    if (!linkType.is_system) {
      items.push({
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => showEditForm(linkType)
      });

      items.push({
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: t('common.delete'),
        hoverClass: 'hover:bg-red-50',
        onClick: () => deleteLinkType(linkType.id, linkType.is_system)
      });
    }

    // Add activate/deactivate for all types
    items.push({
      id: linkType.active ? 'deactivate' : 'activate',
      type: 'regular',
      icon: linkType.active ? PowerOff : Power,
      title: linkType.active ? t('common.deactivate') : t('common.activate'),
      color: linkType.active ? '#f59e0b' : '#10b981',
      hoverClass: linkType.active ? 'hover:bg-orange-50' : 'hover:bg-green-50',
      onClick: () => toggleActive(linkType)
    });

    return items;
  }
</script>

<PageHeader
  icon={Link}
  title={t('links.title')}
  subtitle={t('links.subtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      onclick={showAddForm}
      icon={Plus}
      size="medium"
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('linkTypes', 'add'), guard: () => !showForm }}
    >
      {t('settings.linkTypes.addLinkType')}
    </Button>
  {/snippet}
</PageHeader>

<Modal isOpen={showForm} onclose={() => showForm = false} maxWidth="max-w-2xl">
  <!-- Modal header -->
  <div class="px-6 py-4 border-b" style="border-color: var(--ds-border);">
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {editingLinkType ? t('settings.linkTypes.editLinkType') : t('settings.linkTypes.addLinkType')}
    </h3>
  </div>

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
        <div>
          <Label color="default" class="mb-2">{t('settings.linkTypes.name')}</Label>
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
          <ColorPicker bind:value={formData.color} label={t('settings.linkTypes.color')} />
        </div>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
        <div>
          <Label color="default" class="mb-2">{t('settings.linkTypes.forwardLabel')}</Label>
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
          <Label color="default" class="mb-2">{t('settings.linkTypes.reverseLabel')}</Label>
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
        <Label color="default" class="mb-2">{t('settings.linkTypes.description')}</Label>
        <Textarea
          bind:value={formData.description}
          rows={3}
          placeholder="Optional description of this relationship type"
        />
      </div>

      <div class="mb-4">
        <Checkbox
          bind:checked={formData.active}
          label={t('settings.linkTypes.active')}
          size="small"
        />
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={() => showForm = false}
    onConfirm={handleSubmit}
    confirmLabel={editingLinkType ? t('common.update') : t('common.create')}
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
  {#snippet name(linkType)}
    <div>
      <div class="text-sm font-medium" style="color: var(--ds-text);">{linkType.name}</div>
      {#if linkType.description}
        <div class="text-sm" style="color: var(--ds-text-subtle);">{linkType.description}</div>
      {/if}
    </div>
  {/snippet}

  <!-- Color column with preview and hex code -->
  {#snippet color(linkType)}
    <div class="flex items-center gap-2">
      <div
        class="w-6 h-6 rounded border border-gray-300"
        style="background-color: {linkType.color};"
      ></div>
      <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">{linkType.color}</span>
    </div>
  {/snippet}

  <!-- Status column with badge -->
  {#snippet status(linkType)}
    <Lozenge color={getStatusBadge(linkType).color} text={getStatusBadge(linkType).text} />
  {/snippet}
</DataTable>
