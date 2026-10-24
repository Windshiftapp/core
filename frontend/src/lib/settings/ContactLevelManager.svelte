<script>
  // Contact levels (WI-1139). Global, customers.manage-managed catalog that
  // backs the portal organisation request-sharing audience. Mirrors the other
  // admin catalog managers (CapabilityManager/PriorityManager) and reuses the
  // shared DataTable, EntityRowActions, and EntityFormModal components.
  import { onMount } from 'svelte';
  import { contactRoles } from '../api/portal.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Users from '@lucide/svelte/icons/users';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import Input from '../components/Input.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Textarea from '../components/Textarea.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import EntityFormModal from './EntityFormModal.svelte';
  import EntityRowActions from './EntityRowActions.svelte';
  import { confirm } from '../composables/useConfirm.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import { t } from '../stores/i18n.svelte.js';
  import './settings-form.css';

  let levels = $state([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state(null);
  let showForm = $state(false);
  let editingId = $state(null);
  let formData = $state({ name: '', description: '', sort_order: 1 });

  onMount(loadLevels);

  async function loadLevels() {
    try {
      loading = true;
      error = null;
      const result = await contactRoles.getAll();
      levels = (Array.isArray(result) ? result : (result?.data ?? [])).sort(
        (a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0)
      );
    } catch (_err) {
      error = t('contactLevels.failedToLoad');
    } finally {
      loading = false;
    }
  }

  function nextSortOrder() {
    return levels.length > 0 ? Math.max(...levels.map((level) => level.sort_order ?? 0)) + 1 : 1;
  }

  function startCreate() {
    editingId = null;
    formData = { name: '', description: '', sort_order: nextSortOrder() };
    showForm = true;
  }

  function startEdit(level) {
    editingId = level.id;
    formData = {
      name: level.name,
      description: level.description || '',
      sort_order: level.sort_order ?? 0,
    };
    showForm = true;
  }

  function closeForm() {
    showForm = false;
    editingId = null;
  }

  async function saveLevel() {
    if (saving) return;
    saving = true;
    try {
      if (editingId) {
        await contactRoles.update(editingId, formData);
      } else {
        await contactRoles.create(formData);
      }
      await loadLevels();
      closeForm();
    } catch (_err) {
      error = t('contactLevels.failedToSave');
    } finally {
      saving = false;
    }
  }

  async function deleteLevel(level) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('contactLevels.confirmDelete', { name: level.name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    try {
      await contactRoles.delete(level.id);
      await loadLevels();
    } catch (_err) {
      error = t('contactLevels.deleteFailed');
    }
  }

  const columns = $derived([
    { key: 'name', label: t('common.name') },
    { key: 'description', label: t('common.description') },
    { key: 'sort_order', label: t('common.order'), width: 'w-24' },
    { key: 'is_system', label: '', width: 'w-24', slot: 'is_system' },
    {
      key: 'actions',
      label: t('common.actions'),
      slot: 'actions',
      align: 'text-right',
      width: 'w-32',
    },
  ]);
</script>

<PageHeader icon={Users} title={t('contactLevels.title')} subtitle={t('contactLevels.subtitle')}>
  {#snippet actions()}
    <Button
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={loading}
      dataTestid="contact-level-add"
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('contactLevels', 'add'), guard: () => !showForm }}
    >
      {t('contactLevels.createLevel')}
    </Button>
  {/snippet}
</PageHeader>

{#if error}
  <div class="error">
    {error}
  </div>
{/if}

<DataTable
  {columns}
  data={levels}
  keyField="id"
  emptyMessage={t('contactLevels.noLevels')}
  emptyIcon={Users}
  rowAttrs={(level) => ({ 'data-testid': `contact-level-row-${level.id}` })}
>
  {#snippet is_system(level)}
    {#if level.is_system}
      <Lozenge color="gray" text={t('contactLevels.system')} />
    {/if}
  {/snippet}

  {#snippet actions(level)}
    {#if !level.is_system}
      <EntityRowActions
        actions={[
          {
            id: 'edit',
            icon: Edit,
            title: t('common.edit'),
            testId: `contact-level-edit-${level.id}`,
            onclick: () => startEdit(level),
          },
          {
            id: 'delete',
            icon: Trash2,
            title: t('common.delete'),
            testId: `contact-level-delete-${level.id}`,
            danger: true,
            onclick: () => deleteLevel(level),
          },
        ]}
      />
    {/if}
  {/snippet}
</DataTable>

{#if showForm}
  <EntityFormModal
    title={editingId ? t('contactLevels.editLevel') : t('contactLevels.createLevel')}
    onclose={closeForm}
    onsubmit={saveLevel}
    disabled={!formData.name.trim()}
    {saving}
    confirmLabel={editingId ? t('common.update') : t('common.create')}
  >
    {#snippet fields()}
      <div class="form-group">
        <label for="contact-level-name">{t('common.name')}</label>
        <Input
          type="text"
          id="contact-level-name"
          placeholder={t('contactLevels.namePlaceholder')}
          bind:value={formData.name}
          required
        />
      </div>

      <div class="form-group">
        <label for="contact-level-description">{t('common.description')}</label>
        <Textarea
          id="contact-level-description"
          placeholder={t('placeholders.optionalDescription')}
          bind:value={formData.description}
          rows={2}
        />
      </div>

      <div class="form-group">
        <label for="contact-level-order">{t('common.order')}</label>
        <Input
          type="number"
          id="contact-level-order"
          min={0}
          bind:value={formData.sort_order}
          required
        />
      </div>
    {/snippet}
  </EntityFormModal>
{/if}
