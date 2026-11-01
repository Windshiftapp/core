<script>
  import { priorities as prioritiesApi } from '../api/configuration.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import AlertCircle from '@lucide/svelte/icons/alert-circle';
  import { priorityIconMap, priorityIconOptions } from '../utils/icons.js';
  import Input from '../components/Input.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Textarea from '../components/Textarea.svelte';
  import IconSelector from '../pickers/IconSelector.svelte';
  import Toggle from '../components/Toggle.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { objectDisplayName } from '../utils/systemLabels.js';
  import LocalizedObjectFields from './LocalizedObjectFields.svelte';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let formData = $state({
    name: '',
    description: '',
    icon: 'AlertCircle',
    color: '#7c3aed',
    sort_order: 1,
    is_default: false,
  });
  let editingId = $state(null);
  let editingObject = $state(null);
  let translationEditor = $state(null);

  function getPriorityDisplayName(priority) {
    return objectDisplayName(priority);
  }

  function getConfigurationSetDisplayName(name, builtinKey = '') {
    return builtinKey === 'default' ? t('settings.itemTypes.defaultConfiguration') : name;
  }

  async function loadPriorities() {
    const priorities = await prioritiesApi.getAll();
    return priorities.sort((a, b) => a.sort_order - b.sort_order);
  }

  function openCreate(priorities) {
    formData = {
      name: '',
      description: '',
      icon: 'AlertCircle',
      color: '#7c3aed',
      sort_order: priorities.length > 0 ? Math.max(...priorities.map((p) => p.sort_order)) + 1 : 1,
      is_default: false,
    };
    editingId = null;
    editingObject = null;
  }

  function openEdit(priority) {
    editingObject = priority;
    formData = {
      name: priority.name,
      description: priority.description,
      icon: priority.icon,
      color: priority.color,
      sort_order: priority.sort_order,
      is_default: priority.is_default || false,
    };
    editingId = priority.id;
  }

  async function savePriority() {
    if (editingId) {
      translationEditor?.validate();
      await prioritiesApi.update(editingId, formData);
      await translationEditor?.save();
    } else {
      await prioritiesApi.create(formData);
    }
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }

  async function deletePriority(priority) {
    await prioritiesApi.delete(priority.id);
    window.dispatchEvent(new CustomEvent('refresh-workspace-data'));
  }

  const priorityColumns = [
    { key: 'icon', label: '', width: '40px', slot: 'icon' },
    { key: 'name', label: t('common.name'), render: (priority) => getPriorityDisplayName(priority) },
    { key: 'is_default', label: t('common.default'), width: '80px', slot: 'is_default' },
    { key: 'sort_order', label: t('common.order') },
    {
      key: 'configuration_set_names',
      label: t('configuration.title'),
      slot: 'configuration_set_names',
    },
    { key: 'actions', label: t('common.actions') },
  ];

  function buildPriorityDropdownItems(priority, { edit, remove }) {
    return [
      {
        id: 'edit',
        testid: `priority-edit-${priority.id}`,
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => edit(priority),
      },
      {
        id: 'delete',
        testid: `priority-delete-${priority.id}`,
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => remove(priority),
      },
    ];
  }
</script>

<EntityManagerPage
  icon={AlertCircle}
  title={t('priorities.title')}
  subtitle={t('priorities.subtitle')}
  addLabel={t('priorities.createPriority')}
  addTestid="priority-add"
  hotkeyScope="priorities"
  emptyMessage={t('priorities.noPriorities')}
  emptyIcon={AlertCircle}
  columns={priorityColumns}
  load={loadPriorities}
  create={savePriority}
  update={savePriority}
  remove={deletePriority}
  onOpenCreate={openCreate}
  onOpenEdit={openEdit}
  confirmDeleteMessage={(priority) =>
    t('priorities.confirmDelete', { name: getPriorityDisplayName(priority) })}
  rowTestid={(priority) => `priority-row-${priority.id}`}
  actionItems={buildPriorityDropdownItems}
  actionTriggerTestid={(priority) => `priority-actions-${priority.id}`}
  createTitle={t('priorities.createPriority')}
  editTitle={t('priorities.editPriority')}
  confirmTestid="dialog-confirm"
  onValidate={() => (formData.name.trim() ? null : t('priorities.nameRequired'))}
  errorLabels={{
    load: t('priorities.failedToLoad'),
    save: t('priorities.failedToSave'),
    delete: t('priorities.deleteFailed'),
  }}
  cellSnippets={{
    icon: priorityIconCell,
    is_default: priorityDefaultCell,
    configuration_set_names: priorityConfigurationSetsCell,
  }}
  formFields={priorityForm}
>
</EntityManagerPage>

{#snippet priorityIconCell(priority)}
  {@const PriorityIcon = priorityIconMap[priority.icon] || AlertCircle}
  <div class="flex items-center justify-center">
    <div class="w-6 h-6 rounded flex items-center justify-center" style="background-color: {priority.color}">
      <PriorityIcon size={12} color="white" />
    </div>
  </div>
{/snippet}

{#snippet priorityDefaultCell(priority)}
  <div class="flex items-center">
    {#if priority.is_default}
      <Lozenge color="green" text={t('common.default')} />
    {/if}
  </div>
{/snippet}

{#snippet priorityConfigurationSetsCell(priority)}
  <div class="flex flex-wrap gap-1">
    {#if priority.configuration_set_names && priority.configuration_set_names.length > 0}
      {#each priority.configuration_set_names as configSetName, index}
        <Lozenge
          color="gray"
          text={getConfigurationSetDisplayName(
            configSetName,
            priority.configuration_set_builtin_keys?.[index]
          )}
        />
      {/each}
    {:else}
      <span class="text-xs text-ds-text-subtle">{t('pickers.noConfigurationSetsFound')}</span>
    {/if}
  </div>
{/snippet}

{#snippet priorityForm()}
  {#if editingId}
    {#key editingId}
      <LocalizedObjectFields
        bind:this={translationEditor}
        objectType="priority"
        objectId={editingId}
        bind:canonicalName={formData.name}
        bind:canonicalDescription={formData.description}
        displayName={editingObject?.display_name || editingObject?.name}
        displayDescription={editingObject?.display_description || editingObject?.description}
      />
    {/key}
  {:else}
    <div class="form-group">
      <label for="name">{t('common.name')}</label>
      <Input
        type="text"
        id="name"
        placeholder={t('priorities.namePlaceholder')}
        bind:value={formData.name}
        required
      />
    </div>

    <div class="form-group">
      <label for="description">{t('common.description')}</label>
      <Textarea
        id="description"
        placeholder={t('placeholders.optionalDescription')}
        bind:value={formData.description}
        rows={2}
      />
    </div>
  {/if}

  <div class="form-group">
    <label for="sort_order">{t('common.order')}</label>
    <Input type="number" id="sort_order" min={1} bind:value={formData.sort_order} required />
  </div>

  <div class="form-group">
    <IconSelector
      bind:selectedIcon={formData.icon}
      bind:selectedColor={formData.color}
      iconMap={priorityIconMap}
      iconOptions={priorityIconOptions}
      compact
    />
  </div>

  <div class="form-group">
    <Toggle bind:checked={formData.is_default} label={t('common.default')} size="small" />
  </div>
{/snippet}
