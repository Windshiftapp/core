<script>
  import { onMount } from 'svelte';
  import Plus from '@lucide/svelte/icons/plus';
  import Edit from '@lucide/svelte/icons/edit';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Settings from '@lucide/svelte/icons/settings';
  import Circle from '@lucide/svelte/icons/circle';
  import Layout from '@lucide/svelte/icons/layout';
  import { screenEditorStore } from '../stores/screenEditorStore.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { formatDateSimple } from '../utils/dateFormatter.js';
  import Button from '../components/Button.svelte';
  import TextField from '../components/TextField.svelte';
  import TextareaField from '../components/TextareaField.svelte';
  import DataTable from '../components/DataTable.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import LocalizedObjectFields from '../settings/LocalizedObjectFields.svelte';
  import { objectDisplayValue } from '../utils/systemLabels.js';
  import ScreenFieldLayoutEditor from '../editors/ScreenFieldLayoutEditor.svelte';

  // Bind to store values
  let screens = $derived(screenEditorStore.screens);
  let showCreateForm = $derived(screenEditorStore.showCreateForm);
  let editingScreen = $derived(screenEditorStore.editingScreen);
  let formData = $derived(screenEditorStore.formData);
  // Which screen's fields are open, and the editor instance so the header
  // actions can drive its save/cancel. The editor owns loading the fields.
  let fieldEditorScreen = $state(null);
  let fieldEditorRef = $state(null);
  let screenCanonicalName = $state('');
  let screenCanonicalDescription = $state('');
  let translationEditor = $state(null);

  function getScreenDisplayValue(screen, field) {
    return objectDisplayValue(screen, field, 'screen');
  }

  onMount(async () => {
    await screenEditorStore.loadScreens();
  });

  function startCreate() {
    screenEditorStore.startCreate();
  }

  function startEdit(screen) {
    screenCanonicalName = screen.name || '';
    screenCanonicalDescription = screen.description || '';
    screenEditorStore.startEdit(screen);
  }

  function startEditFields(screen) {
    fieldEditorScreen = screen;
  }

  function cancelForm() {
    screenEditorStore.cancelForm();
  }

  function cancelFieldEditor() {
    fieldEditorRef?.cancel();
    fieldEditorScreen = null;
  }

  async function saveScreen() {
    try {
      if (editingScreen) {
        translationEditor?.validate();
        setFormData('name', screenCanonicalName);
        setFormData('description', screenCanonicalDescription);
      }
      const editor = translationEditor;
      await screenEditorStore.saveScreen();
      await editor?.save();
      await screenEditorStore.loadScreens();
    } catch (error) {
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  async function saveScreenFields() {
    try {
      await fieldEditorRef?.save();
      fieldEditorScreen = null;
      await screenEditorStore.loadScreens();
    } catch (error) {
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message || error }));
    }
  }

  async function deleteScreen(screen) {
    if (screen.builtin_key) {
      errorToast(t('dialogs.alerts.cannotDeleteDefaultScreen'));
      return;
    }

    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dialogs.confirmations.deleteScreen', { name: getScreenDisplayValue(screen, 'name') }),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (confirmed) {
      try {
        await screenEditorStore.deleteScreen(screen);
      } catch (error) {
        errorToast(t('dialogs.alerts.failedToDelete', { error: error.message || error }));
      }
    }
  }

  function setFormData(key, value) {
    screenEditorStore.formData[key] = value;
  }

  function buildScreenDropdownItems(screen) {
    const items = [
      {
        id: 'fields',
        type: 'regular',
        icon: Settings,
        title: t('screensPage.fields'),
        hoverClass: 'hover-bg',
        testid: 'screen-action-fields',
        onClick: () => startEditFields(screen)
      },
      {
        id: 'edit',
        type: 'regular',
        icon: Edit,
        title: t('common.edit'),
        hoverClass: 'hover-bg',
        testid: 'screen-action-edit',
        onClick: () => startEdit(screen)
      }
    ];

    // Built-in screens back the default configuration set and cannot be
    // removed.
    if (!screen.builtin_key) {
      items.push({
        id: 'delete',
        type: 'regular',
        icon: Trash2,
        title: t('common.delete'),
        color: 'var(--ds-text-danger)',
        hoverClass: 'hover-danger',
        testid: 'screen-action-delete',
        onClick: () => deleteScreen(screen)
      });
    }

    return items;
  }

  // Table column definitions
  const screenColumns = [
    {
      key: 'name',
      label: t('screensPage.screen'),
      slot: 'name'
    },
    {
      key: 'created_at',
      label: t('common.created'),
      render: (screen) => formatDateSimple(screen.created_at),
      textColor: 'var(--ds-text-subtle)'
    },
    {
      key: 'actions',
      label: t('common.actions')
    }
  ];
</script>

{#if !fieldEditorScreen}
  <PageHeader
    icon={Layout}
    title={t('screens.title')}
    subtitle={t('screens.subtitle')}
  >
    {#snippet actions()}
      <Button
        variant="primary"
        icon={Plus}
        onclick={startCreate}
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('screens', 'add'), guard: () => !showCreateForm }}
      >
        {t('screensPage.addScreen')}
      </Button>
    {/snippet}
  </PageHeader>


    <Modal isOpen={showCreateForm} onclose={cancelForm} maxWidth="max-w-lg">
      <ModalHeader title={editingScreen ? t('screensPage.editScreen') : t('screensPage.createScreen')} showCloseButton={false} />

      <!-- Modal content -->
      <div class="px-6 py-4">
        <form onsubmit={(e) => { e.preventDefault(); saveScreen(); }}>
          {#if editingScreen}
            {#key editingScreen.id}
              <LocalizedObjectFields
                bind:this={translationEditor}
                objectType="screen"
                objectId={editingScreen.id}
                bind:canonicalName={screenCanonicalName}
                bind:canonicalDescription={screenCanonicalDescription}
                displayName={editingScreen.display_name || editingScreen.name}
                displayDescription={editingScreen.display_description || editingScreen.description}
              />
            {/key}
          {:else}
          <div class="grid grid-cols-1 gap-6">
            <div>
              <TextField
                label={t('screensPage.screenName')}
                id="screen-name"
                labelClass="mb-2"
                value={formData.name}
                oninput={(e) => setFormData('name', e.target.value)}
                placeholder={t('screensPage.screenNamePlaceholder')}
                required
              />
            </div>

            <div>
              <TextareaField
                label={t('screensPage.description')}
                id="screen-description"
                labelClass="mb-2"
                value={formData.description}
                oninput={(e) => setFormData('description', e.target.value)}
                rows={3}
                placeholder={t('screensPage.optionalDescription')}
              />
            </div>
          </div>
          {/if}
        </form>
      </div>

      <DialogFooter
        onCancel={cancelForm}
        onConfirm={saveScreen}
        confirmLabel={editingScreen ? t('screensPage.updateScreen') : t('screensPage.createScreen')}
        disabled={!formData.name.trim()}
      />
    </Modal>

    <DataTable
      columns={screenColumns}
      data={screens}
      keyField="id"
      emptyMessage={t('screensPage.noScreens')}
      emptyIcon={Circle}
      actionItems={buildScreenDropdownItems}
      actionTriggerTestid={(screen) => `screen-actions-${screen.id}`}
    >
      {#snippet name(screen)}
        <div>
          <div style="color: var(--ds-text);">{getScreenDisplayValue(screen, 'name')}</div>
          {#if getScreenDisplayValue(screen, 'description')}
            <div class="text-sm mt-1" style="color: var(--ds-text-subtle);">{getScreenDisplayValue(screen, 'description')}</div>
          {/if}
        </div>
      {/snippet}
    </DataTable>
  {:else}
    <PageHeader
      icon={Settings}
      title={t('screensPage.configureFields')}
      subtitle={t('screensPage.screenSubtitle', { name: getScreenDisplayValue(fieldEditorScreen, 'name') })}
    >
      {#snippet actions()}
        <div class="flex gap-3">
          <Button
            variant="primary"
            onclick={saveScreenFields}
            dataTestid="screens-save-fields"
          >
            {t('screensPage.saveFields')}
          </Button>
          <Button
            variant="default"
            onclick={cancelFieldEditor}
            dataTestid="screens-cancel-fields"
          >
            {t('common.cancel')}
          </Button>
        </div>
      {/snippet}
    </PageHeader>

    <ScreenFieldLayoutEditor bind:this={fieldEditorRef} screen={fieldEditorScreen} />

  {/if}
