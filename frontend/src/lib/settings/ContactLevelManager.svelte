<script>
  // Contact levels (WI-1139). Global, customers.manage-managed catalog that
  // backs the portal organisation request-sharing audience. Uses the shared
  // EntityManagerPage scaffold for the list/search/modal/delete shape.
  import Users from '@lucide/svelte/icons/users';
  import Input from '../components/Input.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Textarea from '../components/Textarea.svelte';
  import { contactRoles } from '../api/portal.js';
  import { t } from '../stores/i18n.svelte.js';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let form = $state({ name: '', description: '', sort_order: 1 });

  const columns = [
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
  ];

  async function loadLevels() {
    const result = await contactRoles.getAll();
    return (Array.isArray(result) ? result : (result?.data ?? [])).sort(
      (a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0)
    );
  }

  function nextSortOrder(levels) {
    return levels.length > 0 ? Math.max(...levels.map((level) => level.sort_order ?? 0)) + 1 : 1;
  }

  function openCreate(levels) {
    form = { name: '', description: '', sort_order: nextSortOrder(levels) };
  }

  function openEdit(level) {
    form = {
      name: level.name,
      description: level.description || '',
      sort_order: level.sort_order ?? 0,
    };
  }
</script>

<EntityManagerPage
  icon={Users}
  title={t('contactLevels.title')}
  subtitle={t('contactLevels.subtitle')}
  addLabel={t('contactLevels.createLevel')}
  addTestid="contact-level-add"
  hotkeyScope="contactLevels"
  actionTestidPrefix="contact-level"
  emptyMessage={t('contactLevels.noLevels')}
  emptyIcon={Users}
  {columns}
  load={loadLevels}
  create={() => contactRoles.create(form)}
  update={(id) => contactRoles.update(id, form)}
  remove={(level) => contactRoles.delete(level.id)}
  onOpenCreate={openCreate}
  onOpenEdit={openEdit}
  confirmDeleteMessage={(level) => t('contactLevels.confirmDelete', { name: level.name })}
  canActOn={(level) => !level.is_system}
  rowTestid={(level) => `contact-level-row-${level.id}`}
  createTitle={t('contactLevels.createLevel')}
  editTitle={t('contactLevels.editLevel')}
  submitDisabled={() => !form.name.trim()}
  errorLabels={{
    load: t('contactLevels.failedToLoad'),
    save: t('contactLevels.failedToSave'),
    delete: t('contactLevels.deleteFailed'),
  }}
  cellSnippets={{ is_system: isSystemCell }}
  formFields={contactLevelFields}
>
</EntityManagerPage>

{#snippet isSystemCell(level)}
  {#if level.is_system}
    <Lozenge color="gray" text={t('contactLevels.system')} />
  {/if}
{/snippet}

{#snippet contactLevelFields()}
  <div class="form-group">
    <label for="contact-level-name">{t('common.name')}</label>
    <Input
      type="text"
      id="contact-level-name"
      placeholder={t('contactLevels.namePlaceholder')}
      bind:value={form.name}
      required
    />
  </div>

  <div class="form-group">
    <label for="contact-level-description">{t('common.description')}</label>
    <Textarea
      id="contact-level-description"
      placeholder={t('placeholders.optionalDescription')}
      bind:value={form.description}
      rows={2}
    />
  </div>

  <div class="form-group">
    <label for="contact-level-order">{t('common.order')}</label>
    <Input
      type="number"
      id="contact-level-order"
      min={0}
      bind:value={form.sort_order}
      required
    />
  </div>
{/snippet}
