<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { writable } from 'svelte/store';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import { Plus, Link, Edit, Trash2, Power, PowerOff } from '@lucide/svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import DescriptionText from '../components/DescriptionText.svelte';
  import { objectDisplayValue } from '../utils/systemLabels.js';
  import LocalizedObjectFields from './LocalizedObjectFields.svelte';

  const linkTypes = writable([]);

  let showForm = $state(false);
  let editingLinkType = $state(null);
  let translationEditor = $state(null);

  function getLinkTypeDisplayValue(linkType, field) {
    return objectDisplayValue(linkType, field);
  }

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

  const submitDisabled = $derived(!formData.name || !formData.forward_label || !formData.reverse_label);

  async function handleSubmit() {
    if (submitDisabled) return;

    try {
      if (editingLinkType) {
        translationEditor?.validate();
        await api.linkTypes.update(editingLinkType.id, formData);
        await translationEditor?.save();
      } else {
        await api.linkTypes.create(formData);
      }
      await loadLinkTypes();
      showForm = false;
    } catch (error) {
      console.error('Failed to save link type:', error);
      errorToast(t('settings.linkTypes.failedToSave'));
    }
  }

  async function deleteLinkType(id, isSystem) {
    if (isSystem) {
      errorToast(t('settings.linkTypes.cannotDeleteSystem'));
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
        errorToast(t('dialogs.alerts.failedToDelete', { error: error.message }));
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
      errorToast(t('dialogs.alerts.failedToToggleStatus', { error: error.message }));
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
        hoverClass: 'hover-danger',
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

<Modal
  isOpen={showForm}
  onclose={() => showForm = false}
  onSubmit={handleSubmit}
  submitDisabled={submitDisabled}
  maxWidth="max-w-2xl"
>
  {#snippet children(submitHint)}
  <!-- Modal header -->
  <ModalHeader title={editingLinkType ? t('settings.linkTypes.editLinkType') : t('settings.linkTypes.addLinkType')} showCloseButton={false} />

  <!-- Modal content -->
  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
      {#if editingLinkType}
        {#key editingLinkType.id}
          <LocalizedObjectFields
            bind:this={translationEditor}
            objectType="link_type"
            objectId={editingLinkType.id}
            bind:canonicalName={formData.name}
            bind:canonicalDescription={formData.description}
            displayName={editingLinkType.display_name || editingLinkType.name}
            displayDescription={editingLinkType.display_description || editingLinkType.description}
          />
        {/key}
      {:else}
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
          <div>
            <Label color="default" class="mb-2">{t('settings.linkTypes.name')}</Label>
            <Input
              type="text"
              bind:value={formData.name}
              required
              placeholder={t('settings.linkTypes.namePlaceholder')}
              size="small"
            />
          </div>
          <div>
            <IconSelector bind:selectedColor={formData.color} colorOnly compact />
          </div>
        </div>
      {/if}
      {#if editingLinkType}
        <div>
          <IconSelector bind:selectedColor={formData.color} colorOnly compact />
        </div>
      {/if}

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
        <div>
          <Label color="default" class="mb-2">{t('settings.linkTypes.forwardLabel')}</Label>
          <Input
            type="text"
            bind:value={formData.forward_label}
            required
            placeholder={t('settings.linkTypes.forwardPlaceholder')}
            size="small"
          />
          <DescriptionText>{t('settings.linkTypes.forwardHint')}</DescriptionText>
        </div>
        <div>
          <Label color="default" class="mb-2">{t('settings.linkTypes.reverseLabel')}</Label>
          <Input
            type="text"
            bind:value={formData.reverse_label}
            required
            placeholder={t('settings.linkTypes.reversePlaceholder')}
            size="small"
          />
          <DescriptionText>{t('settings.linkTypes.reverseHint')}</DescriptionText>
        </div>
      </div>

      {#if !editingLinkType}
        <div class="mb-4">
          <Label color="default" class="mb-2">{t('settings.linkTypes.description')}</Label>
          <Textarea
            bind:value={formData.description}
            rows={3}
            placeholder={t('settings.linkTypes.descriptionPlaceholder')}
          />
        </div>
      {/if}

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
    disabled={submitDisabled}
    showKeyboardHint={true}
    confirmKeyboardHint={submitHint}
  />
  {/snippet}
</Modal>

<DataTable
  columns={linkTypeColumns}
  data={$linkTypes}
  keyField="id"
  emptyMessage={t('settings.linkTypes.empty')}
  emptyIcon={Link}
  actionItems={buildLinkTypeActionItems}
>
  <!-- Name column with description -->
  {#snippet name(linkType)}
    <div>
      <div class="text-sm font-medium" style="color: var(--ds-text);">{getLinkTypeDisplayValue(linkType, 'name')}</div>
      {#if linkType.description}
        <div class="text-sm" style="color: var(--ds-text-subtle);">{getLinkTypeDisplayValue(linkType, 'description')}</div>
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
