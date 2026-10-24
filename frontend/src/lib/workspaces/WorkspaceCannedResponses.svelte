<script>
  // Workspace canned responses (WI-1138): reusable agent reply snippets.
  // Private snippets are internal notes; automations never send them to customers.

  import { onMount } from 'svelte';
  import MessageSquareText from '@lucide/svelte/icons/message-square-text';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { cannedResponses } from '../api/configuration.js';
  import Panel from '../components/Panel.svelte';
  import Button from '../components/Button.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import Label from '../components/Label.svelte';
  import SectionHeader from '../layout/SectionHeader.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import DialogFooter from '../dialogs/DialogFooter.svelte';
  import ConfirmDialog from '../dialogs/ConfirmDialog.svelte';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import TextField from '../components/TextField.svelte';
  import TextareaField from '../components/TextareaField.svelte';

  let { workspaceId } = $props();

  let loading = $state(true);
  let responses = $state([]);

  let showModal = $state(false);
  let editingId = $state(null);
  let formName = $state('');
  let formBody = $state('');
  let formIsPrivate = $state(false);
  let formIsActive = $state(true);
  let saving = $state(false);

  let deleteDialogOpen = $state(false);
  let pendingDelete = $state(null); // { id, name }

  async function load() {
    loading = true;
    try {
      responses = (await cannedResponses.getAll(workspaceId, true)) ?? [];
    } catch (err) {
      console.error('Failed to load canned responses:', err);
      errorToast(err?.message || t('cannedResponses.loadFailed'));
    } finally {
      loading = false;
    }
  }
  onMount(load);

  function openCreate() {
    editingId = null;
    formName = '';
    formBody = '';
    formIsPrivate = false;
    formIsActive = true;
    showModal = true;
  }

  function openEdit(response) {
    editingId = response.id;
    formName = response.name;
    formBody = response.body || '';
    formIsPrivate = !!response.is_private;
    formIsActive = response.is_active !== false;
    showModal = true;
  }

  function closeModal() {
    showModal = false;
    editingId = null;
  }

  let canSave = $derived(!!formName.trim() && !!formBody.trim() && !saving);

  async function save() {
    if (!canSave) return;
    saving = true;
    try {
      if (editingId === null) {
        await cannedResponses.create(workspaceId, { name: formName.trim(), body: formBody, is_private: formIsPrivate });
        successToast(t('cannedResponses.created'));
      } else {
        await cannedResponses.update(workspaceId, editingId, {
          name: formName.trim(),
          body: formBody,
          is_private: formIsPrivate,
          is_active: formIsActive,
        });
        successToast(t('cannedResponses.updated'));
      }
      closeModal();
      await load();
    } catch (err) {
      errorToast(err?.message || t('cannedResponses.saveFailed'));
      console.error('Failed to save canned response:', err);
    } finally {
      saving = false;
    }
  }

  function openDeleteDialog(response) {
    pendingDelete = { id: response.id, name: response.name };
    deleteDialogOpen = true;
  }

  async function confirmDelete() {
    const target = pendingDelete;
    deleteDialogOpen = false;
    pendingDelete = null;
    if (!target) return;
    try {
      await cannedResponses.delete(workspaceId, target.id);
      successToast(t('cannedResponses.deleted'));
      await load();
    } catch (err) {
      errorToast(err?.message || t('cannedResponses.deleteFailed'));
      console.error('Failed to delete canned response:', err);
    }
  }

  function visibilityLabel(response) {
    if (!response.is_active) return t('common.inactive');
    return response.is_private ? t('cannedResponses.private') : t('cannedResponses.public');
  }
</script>

<Panel padding="spacious">
  <SectionHeader>
    {#snippet actions()}
      <Button
        size="sm"
        icon={Plus}
        onclick={openCreate}
        dataTestid="canned-response-add"
        keyboardHint="A"
        hotkeyConfig={{ key: toHotkeyString('cannedResponses', 'add'), guard: () => !showModal }}
      >
        {t('cannedResponses.new')}
      </Button>
    {/snippet}
  </SectionHeader>

  {#if loading}
    <StateDisplay type="loading" />
  {:else if responses.length === 0}
    <EmptyState
      icon={MessageSquareText}
      title={t('cannedResponses.empty')}
      description={t('cannedResponses.emptyDescription')}
    >
      {#snippet action()}
        <!-- shortcut-guard-exempt: duplicate of the section-header "New canned response" action in an admin settings section -->
        <Button size="sm" icon={Plus} onclick={openCreate} dataTestid="canned-response-add-empty">
          {t('cannedResponses.new')}
        </Button>
      {/snippet}
    </EmptyState>
  {:else}
    <div class="border rounded-md overflow-hidden" style="border-color: var(--ds-border);">
      <table class="w-full text-sm" data-testid="canned-response-list">
        <thead>
          <tr style="background-color: var(--ds-background-neutral);">
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">{t('common.name')}</th>
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">{t('cannedResponses.visibility')}</th>
            <th class="text-left px-3 py-2 font-medium" style="color: var(--ds-text);">{t('cannedResponses.used')}</th>
            <th class="px-3 py-2"></th>
          </tr>
        </thead>
        <tbody>
          {#each responses as response (response.id)}
            <tr class="border-t" style="border-color: var(--ds-border);" data-testid="canned-response-row">
              <td class="px-3 py-2" style="color: var(--ds-text);">
                {response.name}
                {#if !response.is_active}
                  <span class="ml-2 text-xs" style="color: var(--ds-text-subtle);">{t('common.inactive')}</span>
                {/if}
              </td>
              <td class="px-3 py-2" style="color: var(--ds-text-subtle);">{visibilityLabel(response)}</td>
              <td class="px-3 py-2" style="color: var(--ds-text-subtle);">{response.used_count}</td>
              <td class="px-3 py-2 text-right whitespace-nowrap">
                <div class="flex items-center justify-end gap-2">
                  <Button variant="default" size="small" icon={Pencil} onclick={() => openEdit(response)} dataTestid="canned-response-edit">
                    {t('common.edit')}
                  </Button>
                  <Button variant="default" size="small" icon={Trash2} onclick={() => openDeleteDialog(response)} dataTestid="canned-response-delete">
                    {t('common.delete')}
                  </Button>
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</Panel>

<Modal isOpen={showModal} onclose={closeModal} onSubmit={save} submitDisabled={!canSave} maxWidth="max-w-2xl">
  {#snippet children(submitHint)}
    <ModalHeader
      title={editingId === null ? t('cannedResponses.new') : t('cannedResponses.edit')}
      icon={MessageSquareText}
      onclose={closeModal}
    />
    <div class="px-6 py-4 space-y-3" data-testid="canned-response-editor">
      <TextField
        label={t('common.name')}
        id="canned-response-name"
        required
        placeholder="refund-policy"
        dataTestid="canned-response-name"
        bind:value={formName}
      />

      <TextareaField
        label={t('cannedResponses.body')}
        id="canned-response-body"
        required
        rows={8}
        placeholder={t('cannedResponses.bodyPlaceholder')}
        dataTestid="canned-response-body"
        bind:value={formBody}
      />
      <p class="text-xs" style="color: var(--ds-text-subtle);">{t('cannedResponses.variablesHint')}</p>

      <div class="flex items-center gap-6">
        <span data-testid="canned-response-private">
          <Checkbox bind:checked={formIsPrivate} label={t('cannedResponses.privateSnippet')} />
        </span>
        {#if editingId !== null}
          <span data-testid="canned-response-active">
            <Checkbox bind:checked={formIsActive} label={t('common.active')} />
          </span>
        {/if}
      </div>
    </div>
    <DialogFooter
      onCancel={closeModal}
      onConfirm={save}
      confirmLabel={editingId === null ? t('common.create') : t('common.saveChanges')}
      disabled={!canSave}
      loading={saving}
      confirmTestid="canned-response-save"
      showKeyboardHint
      confirmKeyboardHint={submitHint}
    />
  {/snippet}
</Modal>

<ConfirmDialog
  bind:show={deleteDialogOpen}
  variant="danger"
  title={t('cannedResponses.deleteTitle')}
  message={t('cannedResponses.deleteMessage', { name: pendingDelete?.name ?? '' })}
  confirmText={t('cannedResponses.delete')}
  onconfirm={confirmDelete}
  oncancel={() => (pendingDelete = null)}
/>
