<script>
  import { linkTypes as linkTypesApi } from '../api/configuration.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Link from '@lucide/svelte/icons/link';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Power from '@lucide/svelte/icons/power';
  import PowerOff from '@lucide/svelte/icons/power-off';
  import Lozenge from '../components/Lozenge.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';
  import TextField from '../components/TextField.svelte';
  import TextareaField from '../components/TextareaField.svelte';
  import { objectDisplayValue } from '../utils/systemLabels.js';
  import LocalizedObjectFields from './LocalizedObjectFields.svelte';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let editingLinkType = $state(null);
  let translationEditor = $state(null);

  let formData = $state({
    name: '',
    description: '',
    forward_label: '',
    reverse_label: '',
    color: '#6b7280',
    active: true,
  });

  function getLinkTypeDisplayValue(linkType, field) {
    return objectDisplayValue(linkType, field);
  }

  async function loadLinkTypes() {
    return (await linkTypesApi.getAll(true)) || []; // include inactive
  }

  function openCreate() {
    editingLinkType = null;
    formData = {
      name: '',
      description: '',
      forward_label: '',
      reverse_label: '',
      color: '#6b7280',
      active: true,
    };
  }

  function openEdit(linkType) {
    editingLinkType = linkType;
    formData = {
      name: linkType.name,
      description: linkType.description,
      forward_label: linkType.forward_label,
      reverse_label: linkType.reverse_label,
      color: linkType.color,
      active: linkType.active,
    };
  }

  async function saveLinkType() {
    if (editingLinkType) {
      translationEditor?.validate();
      await linkTypesApi.update(editingLinkType.id, formData);
      await translationEditor?.save();
    } else {
      await linkTypesApi.create(formData);
    }
  }

  async function deleteLinkType(linkType) {
    await linkTypesApi.delete(linkType.id);
  }

  async function toggleActive(linkType, reload) {
    try {
      await linkTypesApi.update(linkType.id, { active: !linkType.active });
      reload();
    } catch (error) {
      console.error('Failed to toggle link type status:', error);
      errorToast(t('dialogs.alerts.failedToToggleStatus', { error: error.message }));
    }
  }

  function getStatusBadge(linkType) {
    if (linkType.is_system) {
      return { text: t('settings.linkTypes.system'), color: 'blue' };
    }
    if (linkType.active) {
      return { text: t('settings.linkTypes.active'), color: 'green' };
    }
    return { text: t('settings.linkTypes.inactive'), color: 'gray' };
  }

  const linkTypeColumns = [
    { key: 'name', label: t('settings.linkTypes.name'), slot: 'name' },
    { key: 'color', label: t('settings.linkTypes.color'), slot: 'color' },
    { key: 'status', label: t('common.status'), slot: 'status' },
    { key: 'actions', label: t('common.actions') },
  ];

  function buildLinkTypeActionItems(linkType, { edit, remove, reload }) {
    const items = [];

    if (!linkType.is_system) {
      items.push({
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => edit(linkType),
      });

      items.push({
        id: 'delete',
        type: 'danger',
        icon: Trash2,
        title: t('common.delete'),
        hoverClass: 'hover-danger',
        onClick: () => remove(linkType),
      });
    }

    items.push({
      id: linkType.active ? 'deactivate' : 'activate',
      type: 'regular',
      icon: linkType.active ? PowerOff : Power,
      title: linkType.active ? t('common.deactivate') : t('common.activate'),
      color: linkType.active ? '#f59e0b' : '#10b981',
      hoverClass: linkType.active ? 'hover:bg-ds-accent-orange-subtle' : 'hover:bg-ds-success-subtle',
      onClick: () => toggleActive(linkType, reload),
    });

    return items;
  }
</script>

<EntityManagerPage
  icon={Link}
  title={t('links.title')}
  subtitle={t('links.subtitle')}
  addLabel={t('settings.linkTypes.addLinkType')}
  hotkeyScope="linkTypes"
  emptyMessage={t('settings.linkTypes.empty')}
  emptyIcon={Link}
  columns={linkTypeColumns}
  load={loadLinkTypes}
  create={saveLinkType}
  update={saveLinkType}
  remove={deleteLinkType}
  onOpenCreate={openCreate}
  onOpenEdit={openEdit}
  confirmDeleteMessage={() => t('dialogs.confirmations.deleteLinkType')}
  createTitle={t('settings.linkTypes.addLinkType')}
  editTitle={t('settings.linkTypes.editLinkType')}
  submitDisabled={() => !formData.name || !formData.forward_label || !formData.reverse_label}
  errorLabels={{ save: t('settings.linkTypes.failedToSave') }}
  actionItems={buildLinkTypeActionItems}
  cellSnippets={{ name: linkTypeNameCell, color: linkTypeColorCell, status: linkTypeStatusCell }}
  formFields={linkTypeForm}
>
</EntityManagerPage>

{#snippet linkTypeNameCell(linkType)}
  <div>
    <div class="text-sm font-medium" style="color: var(--ds-text);">
      {getLinkTypeDisplayValue(linkType, 'name')}
    </div>
    {#if linkType.description}
      <div class="text-sm" style="color: var(--ds-text-subtle);">
        {getLinkTypeDisplayValue(linkType, 'description')}
      </div>
    {/if}
  </div>
{/snippet}

{#snippet linkTypeColorCell(linkType)}
  <div class="flex items-center gap-2">
    <div class="w-6 h-6 rounded border border-ds-border-bold" style="background-color: {linkType.color};"></div>
    <span class="text-sm font-mono" style="color: var(--ds-text-subtle);">{linkType.color}</span>
  </div>
{/snippet}

{#snippet linkTypeStatusCell(linkType)}
  <Lozenge color={getStatusBadge(linkType).color} text={getStatusBadge(linkType).text} />
{/snippet}

{#snippet linkTypeForm()}
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
        <TextField
          label={t('settings.linkTypes.name')}
          required
          labelColor="default"
          type="text"
          placeholder={t('settings.linkTypes.namePlaceholder')}
          size="small"
          bind:value={formData.name}
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
      <TextField
        label={t('settings.linkTypes.forwardLabel')}
        required
        labelColor="default"
        type="text"
        placeholder={t('settings.linkTypes.forwardPlaceholder')}
        size="small"
        bind:value={formData.forward_label}
      />
      <DescriptionText>{t('settings.linkTypes.forwardHint')}</DescriptionText>
    </div>
    <div>
      <TextField
        label={t('settings.linkTypes.reverseLabel')}
        required
        labelColor="default"
        type="text"
        placeholder={t('settings.linkTypes.reversePlaceholder')}
        size="small"
        bind:value={formData.reverse_label}
      />
      <DescriptionText>{t('settings.linkTypes.reverseHint')}</DescriptionText>
    </div>
  </div>

  {#if !editingLinkType}
    <div class="mb-4">
      <TextareaField
        label={t('settings.linkTypes.description')}
        labelColor="default"
        rows={3}
        placeholder={t('settings.linkTypes.descriptionPlaceholder')}
        bind:value={formData.description}
      />
    </div>
  {/if}

  <div class="mb-4">
    <Checkbox bind:checked={formData.active} label={t('settings.linkTypes.active')} size="small" />
  </div>
{/snippet}
