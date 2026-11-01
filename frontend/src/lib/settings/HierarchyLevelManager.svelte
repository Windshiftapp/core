<script>
  import { hierarchyLevels as hierarchyLevelsApi } from '../api/configuration.js';
  import { t } from '../stores/i18n.svelte.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import ChevronUp from '@lucide/svelte/icons/chevron-up';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import Circle from '@lucide/svelte/icons/circle';
  import Network from '@lucide/svelte/icons/network';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Input from '../components/Input.svelte';
  import Textarea from '../components/Textarea.svelte';
  import { objectDisplayValue } from '../utils/systemLabels.js';
  import LocalizedObjectFields from './LocalizedObjectFields.svelte';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let formData = $state({ level: 0, name: '', description: '' });
  let editingId = $state(null);
  let editingObject = $state(null);
  let translationEditor = $state(null);

  function getDisplayValue(hierarchyLevel, field) {
    return objectDisplayValue(hierarchyLevel, field);
  }

  async function loadHierarchyLevels() {
    const data = await hierarchyLevelsApi.getAll();
    return data.sort((a, b) => a.level - b.level);
  }

  function openCreate(levels) {
    formData = {
      level: levels.length > 0 ? Math.max(...levels.map((h) => h.level)) + 1 : 0,
      name: '',
      description: '',
    };
    editingId = null;
    editingObject = null;
  }

  function openEdit(hierarchyLevel) {
    editingObject = hierarchyLevel;
    formData = {
      level: hierarchyLevel.level,
      name: hierarchyLevel.name,
      description: hierarchyLevel.description,
    };
    editingId = hierarchyLevel.id;
  }

  async function saveHierarchyLevel() {
    if (editingId) {
      translationEditor?.validate();
      await hierarchyLevelsApi.update(editingId, formData);
      await translationEditor?.save();
    } else {
      await hierarchyLevelsApi.create(formData);
    }
  }

  async function deleteHierarchyLevel(hierarchyLevel) {
    await hierarchyLevelsApi.delete(hierarchyLevel.id);
  }

  async function moveLevel(hierarchyLevel, direction, levels, reload) {
    const newLevel = direction === 'up' ? hierarchyLevel.level - 1 : hierarchyLevel.level + 1;

    const conflictLevel = levels.find((h) => h.level === newLevel);
    if (conflictLevel) {
      errorToast(
        t('settings.hierarchyLevels.levelOccupied', {
          level: newLevel,
          name: getDisplayValue(conflictLevel, 'name'),
        })
      );
      return;
    }

    if (newLevel < 0) {
      errorToast(t('settings.hierarchyLevels.negativeLevel'));
      return;
    }

    await hierarchyLevelsApi.update(hierarchyLevel.id, {
      ...hierarchyLevel,
      level: newLevel,
    });
    reload();
  }

  const hierarchyColumns = [
    { key: 'level', label: t('settings.hierarchyLevels.level'), width: '60px' },
    {
      key: 'name',
      label: t('settings.hierarchyLevels.name'),
      render: (hierarchyLevel) => getDisplayValue(hierarchyLevel, 'name'),
    },
    {
      key: 'description',
      label: t('settings.hierarchyLevels.description'),
      render: (hierarchyLevel) => getDisplayValue(hierarchyLevel, 'description'),
    },
    { key: 'actions', label: t('common.actions') },
  ];

  function buildHierarchyDropdownItems(hierarchyLevel, { edit, remove, items, reload }) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => edit(hierarchyLevel),
      },
      {
        id: 'move-up',
        type: 'regular',
        icon: ChevronUp,
        title: t('common.moveUp'),
        hoverClass: 'hover-bg',
        disabled: hierarchyLevel.level === 0,
        onClick: () => moveLevel(hierarchyLevel, 'up', items, reload),
      },
      {
        id: 'move-down',
        type: 'regular',
        icon: ChevronDown,
        title: t('common.moveDown'),
        hoverClass: 'hover-bg',
        onClick: () => moveLevel(hierarchyLevel, 'down', items, reload),
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => remove(hierarchyLevel),
      },
    ];
  }
</script>

<EntityManagerPage
  icon={Network}
  title={t('settings.hierarchyLevels.title')}
  subtitle={t('settings.hierarchyLevels.subtitle')}
  addLabel={t('settings.hierarchyLevels.addHierarchyLevel')}
  hotkeyScope="hierarchyLevels"
  emptyMessage={t('settings.hierarchyLevels.empty')}
  emptyIcon={Circle}
  columns={hierarchyColumns}
  load={loadHierarchyLevels}
  create={saveHierarchyLevel}
  update={saveHierarchyLevel}
  remove={deleteHierarchyLevel}
  onOpenCreate={openCreate}
  onOpenEdit={openEdit}
  confirmDeleteMessage={(hierarchyLevel) =>
    t('settings.hierarchyLevels.confirmDelete', {
      name: getDisplayValue(hierarchyLevel, 'name'),
    })}
  createTitle={t('settings.hierarchyLevels.createTitle')}
  editTitle={t('settings.hierarchyLevels.editTitle')}
  onValidate={() => (formData.name.trim() ? null : t('settings.hierarchyLevels.nameRequired'))}
  errorLabels={{
    load: t('settings.hierarchyLevels.failedToLoad'),
    save: t('settings.hierarchyLevels.failedToSave'),
    delete: t('settings.hierarchyLevels.deleteFailed'),
  }}
  actionItems={buildHierarchyDropdownItems}
  formFields={hierarchyLevelForm}
>
</EntityManagerPage>

{#snippet hierarchyLevelForm()}
  <div class="form-group">
    <label for="level">{t('settings.hierarchyLevels.level')}</label>
    <Input type="number" id="level" min="0" max="10" bind:value={formData.level} required />
    <small>{t('settings.hierarchyLevels.numericHint')}</small>
  </div>

  {#if editingId}
    {#key editingId}
      <LocalizedObjectFields
        bind:this={translationEditor}
        objectType="hierarchy_level"
        objectId={editingId}
        bind:canonicalName={formData.name}
        bind:canonicalDescription={formData.description}
        displayName={editingObject?.display_name || editingObject?.name}
        displayDescription={editingObject?.display_description || editingObject?.description}
      />
    {/key}
  {:else}
    <div class="form-group">
      <label for="name">{t('settings.hierarchyLevels.name')}</label>
      <Input
        type="text"
        id="name"
        placeholder={t('settings.hierarchyLevels.namePlaceholder')}
        bind:value={formData.name}
        required
      />
    </div>

    <div class="form-group">
      <label for="description">{t('settings.hierarchyLevels.description')}</label>
      <Textarea
        id="description"
        placeholder={t('settings.hierarchyLevels.descriptionPlaceholder')}
        bind:value={formData.description}
        rows={3}
      />
    </div>
  {/if}
{/snippet}
