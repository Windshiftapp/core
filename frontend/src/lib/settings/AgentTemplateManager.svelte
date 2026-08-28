<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { Plus, Edit, Trash2 } from '@lucide/svelte';
  import { IconUserStar as AgentIcon } from '@tabler/icons-svelte-runes';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import Button from '../components/Button.svelte';
  import DataTable from '../components/DataTable.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import Select from '../components/Select.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Input from '../components/Input.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import './settings-form.css';

  let entries = $state([]);
  let defaults = $state([]);
  let isLoading = $state(true);
  let error = $state(null);
  let editingId = $state(null);
  let showCreateForm = $state(false);
  let keyLocked = $state(false);

  let formData = $state({
    template_key: '',
    name: '',
    default_type: 'standard',
    instructions: '',
    enabled: true,
  });

  const profileTypeOptions = $derived([
    { value: 'standard', label: t('settings.adminOperations.agentTemplates.standard') },
    { value: 'coding', label: t('settings.adminOperations.agentTemplates.coding') },
  ]);

  const defaultOptions = $derived([
    { value: '', label: t('settings.adminOperations.agentTemplates.startFromNew') },
    ...defaults.map((d) => ({ value: d.key, label: d.name })),
  ]);

  onMount(() => {
    loadEntries();
    loadDefaults();
  });

  async function loadEntries() {
    try {
      isLoading = true;
      error = null;
      entries = await api.agentTemplates.getAll();
    } catch (err) {
      error = t('settings.adminOperations.agentTemplates.loadFailed', { error: err.message });
    } finally {
      isLoading = false;
    }
  }

  async function loadDefaults() {
    try {
      defaults = await api.agentTemplates.defaults();
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

  function startCreate() {
    resetForm();
    editingId = null;
    showCreateForm = true;
  }

  // Seed the create form from a built-in default so the new override
  // overwrites that template. Overrides the default's name, type, and
  // instructions; blank fields fall back to the default.
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

  function startEdit(entry) {
    formData = {
      template_key: entry.template_key,
      name: entry.name,
      default_type: entry.default_type,
      instructions: entry.instructions || '',
      enabled: entry.enabled,
    };
    keyLocked = false;
    editingId = entry.id;
    showCreateForm = true;
  }

  function cancelEdit() {
    showCreateForm = false;
    editingId = null;
    keyLocked = false;
  }

  async function saveEntry() {
    try {
      if (!formData.template_key.trim()) {
        errorToast(t('settings.adminOperations.agentTemplates.templateKeyRequired'));
        return;
      }
      if (!formData.name.trim()) {
        errorToast(t('settings.adminOperations.agentTemplates.nameRequired'));
        return;
      }

      if (editingId) {
        await api.agentTemplates.update(editingId, {
          name: formData.name,
          default_type: formData.default_type,
          instructions: formData.instructions,
          enabled: formData.enabled,
        });
      } else {
        await api.agentTemplates.create({
          template_key: formData.template_key,
          name: formData.name,
          default_type: formData.default_type,
          instructions: formData.instructions,
          enabled: formData.enabled,
        });
      }

      await loadEntries();
      cancelEdit();
      error = null;
    } catch (err) {
      errorToast(t('settings.adminOperations.agentTemplates.saveFailed', { error: err.message }));
    }
  }

  async function deleteEntry(id, name) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('settings.adminOperations.agentTemplates.deleteMessage', { name }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;

    try {
      await api.agentTemplates.delete(id);
      await loadEntries();
      error = null;
    } catch (err) {
      error = err.message;
    }
  }

  const columns = $derived([
    { key: 'template_key', label: t('settings.adminOperations.agentTemplates.key') },
    { key: 'name', label: t('common.name') },
    { key: 'default_type', label: t('settings.adminOperations.agentTemplates.profileType'), slot: 'default_type' },
    { key: 'enabled', label: t('common.enabled'), slot: 'enabled' },
    { key: 'actions', label: t('common.actions') },
  ]);

  function buildDropdownItems(entry) {
    return [
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        onClick: () => startEdit(entry),
      },
      {
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        onClick: () => deleteEntry(entry.id, entry.name),
      },
    ];
  }
</script>

<PageHeader
  icon={AgentIcon}
  title={t('settings.adminOperations.agentTemplates.title')}
  subtitle={t('settings.adminOperations.agentTemplates.subtitle')}
>
  {#snippet actions()}
    <Button
      variant="primary"
      icon={Plus}
      onclick={startCreate}
      disabled={isLoading}
      dataTestid="agent-template-add"
      keyboardHint="A"
      hotkeyConfig={{ key: toHotkeyString('agentTemplates', 'add'), guard: () => !showCreateForm }}
    >
      {t('settings.adminOperations.agentTemplates.addTemplate')}
    </Button>
  {/snippet}
</PageHeader>

{#if error}
  <div class="error">
    {error}
  </div>
{/if}

<DataTable
  columns={columns}
  data={entries}
  keyField="id"
  loading={isLoading}
  emptyMessage={t('settings.adminOperations.agentTemplates.empty')}
  emptyIcon={AgentIcon}
  actionItems={buildDropdownItems}
  actionTriggerTestid={(entry) => `agent-template-actions-${entry.id}`}
  rowAttrs={(entry) => ({
    'data-testid': `agent-template-row-${entry.id}`,
  })}
>
  {#snippet default_type(entry)}
    <Lozenge
      color={entry.default_type === 'coding' ? 'purple' : 'blue'}
      text={entry.default_type === 'coding' ? t('settings.adminOperations.agentTemplates.coding') : t('settings.adminOperations.agentTemplates.standard')}
    />
  {/snippet}

  {#snippet enabled(entry)}
    <Lozenge
      color={entry.enabled ? 'green' : 'gray'}
      text={entry.enabled ? t('common.yes') : t('common.no')}
    />
  {/snippet}
</DataTable>

<Modal isOpen={showCreateForm} onclose={cancelEdit} onSubmit={saveEntry} maxWidth="max-w-2xl">
  <ModalHeader
    title={editingId ? t('settings.adminOperations.agentTemplates.editOverride') : t('settings.adminOperations.agentTemplates.addOverride')}
    showCloseButton={false}
  />

  <div class="px-6 py-4">
    <form onsubmit={(e) => { e.preventDefault(); saveEntry(); }}>
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
        <Select
          id="default_type"
          bind:value={formData.default_type}
          options={profileTypeOptions}
          required
        />
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
        <Checkbox
          bind:checked={formData.enabled}
          label={t('common.enabled')}
        />
      </div>
    </form>
  </div>

  <DialogFooter
    onCancel={cancelEdit}
    onConfirm={saveEntry}
    confirmLabel={editingId ? t('common.update') : t('common.create')}
    showKeyboardHint
  />
</Modal>
