<script>
  import { agentTemplates } from '../api/admin.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import AgentIcon from '@tabler/icons-svelte-runes/icons/user-star';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Input from '../components/Input.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import EntityManagerPage from './EntityManagerPage.svelte';

  let defaults = $state([]);
  let editingId = $state(null);
  let keyLocked = $state(false);

  let formData = $state({
    template_key: '',
    name: '',
    default_type: 'standard',
    instructions: '',
    enabled: true,
  });

  const profileTypeOptions = [
    { value: 'standard', label: t('settings.adminOperations.agentTemplates.standard') },
    { value: 'coding', label: t('settings.adminOperations.agentTemplates.coding') },
  ];

  const defaultOptions = $derived([
    { value: '', label: t('settings.adminOperations.agentTemplates.startFromNew') },
    ...defaults.map((d) => ({ value: d.key, label: d.name })),
  ]);

  async function loadEntries() {
    const [entries] = await Promise.all([agentTemplates.getAll(), loadDefaults()]);
    return entries;
  }

  async function loadDefaults() {
    try {
      defaults = await agentTemplates.defaults();
    } catch {
      // Non-fatal: admins can still create overrides by typing a key.
    }
  }

  function resetForm() {
    formData = {
      template_key: '',
      name: '',
      default_type: 'standard',
      instructions: '',
      enabled: true,
    };
    keyLocked = false;
  }

  function openCreate() {
    resetForm();
    editingId = null;
  }

  function openEdit(entry) {
    formData = {
      template_key: entry.template_key,
      name: entry.name,
      default_type: entry.default_type,
      instructions: entry.instructions || '',
      enabled: entry.enabled,
    };
    keyLocked = false;
    editingId = entry.id;
  }

  // Seed the create form from a built-in default so the new override
  // overwrites that template.
  function onDefaultSelected() {
    const selected = defaults.find((d) => d.key === formData.template_key);
    if (!selected) {
      resetForm();
      return;
    }
    formData = {
      template_key: selected.key,
      name: selected.name,
      default_type: selected.default_type,
      instructions: selected.instructions || '',
      enabled: true,
    };
    keyLocked = true;
  }

  class ValidationError extends Error {}

  async function saveEntry() {
    if (!formData.template_key.trim()) {
      errorToast(t('settings.adminOperations.agentTemplates.templateKeyRequired'));
      throw new ValidationError();
    }
    if (!formData.name.trim()) {
      errorToast(t('settings.adminOperations.agentTemplates.nameRequired'));
      throw new ValidationError();
    }

    if (editingId) {
      await agentTemplates.update(editingId, {
        name: formData.name,
        default_type: formData.default_type,
        instructions: formData.instructions,
        enabled: formData.enabled,
      });
    } else {
      await agentTemplates.create({
        template_key: formData.template_key,
        name: formData.name,
        default_type: formData.default_type,
        instructions: formData.instructions,
        enabled: formData.enabled,
      });
    }
  }

  async function deleteEntry(entry) {
    await agentTemplates.delete(entry.id);
  }

  function handleError(err, phase) {
    if (err instanceof ValidationError) return true;
    const key = {
      load: 'settings.adminOperations.agentTemplates.loadFailed',
      save: 'settings.adminOperations.agentTemplates.saveFailed',
      delete: 'settings.adminOperations.agentTemplates.saveFailed',
    }[phase];
    errorToast(t(key, { error: err?.message || err }));
    return true;
  }

  const columns = [
    { key: 'template_key', label: t('settings.adminOperations.agentTemplates.key') },
    { key: 'name', label: t('common.name') },
    {
      key: 'default_type',
      label: t('settings.adminOperations.agentTemplates.profileType'),
      slot: 'default_type',
    },
    { key: 'enabled', label: t('common.enabled'), slot: 'enabled' },
    { key: 'actions', label: t('common.actions') },
  ];

  function buildDropdownItems(entry, { edit, remove }) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => edit(entry),
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => remove(entry),
      },
    ];
  }
</script>

<EntityManagerPage
  icon={AgentIcon}
  title={t('settings.adminOperations.agentTemplates.title')}
  subtitle={t('settings.adminOperations.agentTemplates.subtitle')}
  addLabel={t('settings.adminOperations.agentTemplates.addTemplate')}
  addTestid="agent-template-add"
  hotkeyScope="agentTemplates"
  emptyMessage={t('settings.adminOperations.agentTemplates.empty')}
  emptyIcon={AgentIcon}
  columns={columns}
  load={loadEntries}
  create={saveEntry}
  update={saveEntry}
  remove={deleteEntry}
  onOpenCreate={openCreate}
  onOpenEdit={openEdit}
  confirmDeleteMessage={(entry) =>
    t('settings.adminOperations.agentTemplates.deleteMessage', { name: entry.name })}
  rowTestid={(entry) => `agent-template-row-${entry.id}`}
  actionItems={buildDropdownItems}
  actionTriggerTestid={(entry) => `agent-template-actions-${entry.id}`}
  createTitle={t('settings.adminOperations.agentTemplates.addOverride')}
  editTitle={t('settings.adminOperations.agentTemplates.editOverride')}
  confirmTestid="dialog-confirm"
  onError={handleError}
  cellSnippets={{ default_type: templateTypeCell, enabled: templateEnabledCell }}
  formFields={templateForm}
>
</EntityManagerPage>

{#snippet templateTypeCell(entry)}
  <Lozenge
    color={entry.default_type === 'coding' ? 'purple' : 'blue'}
    text={entry.default_type === 'coding'
      ? t('settings.adminOperations.agentTemplates.coding')
      : t('settings.adminOperations.agentTemplates.standard')}
  />
{/snippet}

{#snippet templateEnabledCell(entry)}
  <Lozenge
    color={entry.enabled ? 'green' : 'gray'}
    text={entry.enabled ? t('common.yes') : t('common.no')}
  />
{/snippet}

{#snippet templateForm()}
  {#if !editingId}
    <div class="form-group">
      <label for="start_from_default">{t('settings.adminOperations.agentTemplates.startFromDefault')}</label>
      <Select
        id="start_from_default"
        bind:value={formData.template_key}
        onchange={onDefaultSelected}
        options={defaultOptions}
        placeholder={t('settings.adminOperations.agentTemplates.startFromPlaceholder')}
      />
      <span class="text-xs" style="color: var(--ds-text-subtlest);">
        {t('settings.adminOperations.agentTemplates.startFromHelp')}
      </span>
    </div>
  {/if}

  <div class="form-group">
    <label for="template_key">{t('settings.adminOperations.agentTemplates.templateKey')}</label>
    <Input
      id="template_key"
      placeholder={t('settings.adminOperations.agentTemplates.templateKeyPlaceholder')}
      bind:value={formData.template_key}
      disabled={!!editingId || keyLocked}
      required
    />
    {#if keyLocked}
      <span class="text-xs" style="color: var(--ds-text-subtlest);">
        {t('settings.adminOperations.agentTemplates.builtInOverride', { name: formData.name })}
      </span>
    {/if}
  </div>

  <div class="form-group">
    <label for="name">{t('common.name')}</label>
    <Input
      id="name"
      placeholder={t('settings.adminOperations.agentTemplates.namePlaceholder')}
      bind:value={formData.name}
      required
    />
  </div>

  <div class="form-group">
    <label for="default_type">{t('settings.adminOperations.agentTemplates.profileType')}</label>
    <Select id="default_type" bind:value={formData.default_type} options={profileTypeOptions} required />
  </div>

  <div class="form-group">
    <label for="instructions">{t('settings.adminOperations.agentTemplates.instructions')}</label>
    <Textarea
      id="instructions"
      placeholder={t('settings.adminOperations.agentTemplates.instructionsPlaceholder')}
      bind:value={formData.instructions}
      rows={4}
    />
  </div>

  <div class="form-group">
    <Checkbox bind:checked={formData.enabled} label={t('common.enabled')} />
  </div>
{/snippet}
