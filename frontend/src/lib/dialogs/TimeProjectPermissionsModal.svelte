<script>
  import { onMount } from 'svelte';
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import AssigneePicker from '../pickers/AssigneePicker.svelte';
  import Button from '../components/Button.svelte';
  import Spinner from '../components/Spinner.svelte';
  import ActionButton from '../layout/ActionButton.svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateTimeLocale } from '../utils/dateFormatter.js';
  import { User, Users, X, Plus, Shield, UserCheck } from 'lucide-svelte';

  // Props
  let {
    isOpen = false,
    project = null,
    onClose = () => {}
  } = $props();

  // State
  let activeTab = $state('managers'); // 'managers' or 'members'
  let managers = $state([]);
  let members = $state([]);
  let loading = $state(false);
  let saving = $state(false);
  let showAddForm = $state(false);
  let addType = $state('user');
  let selectedUserId = $state(null);
  let selectedGroupId = $state(null);
  let showRemoveConfirmation = $state(false);
  let itemToRemove = $state(null);
  let removeType = $state('manager'); // 'manager' or 'member'

  // Load data when modal opens
  $effect(() => {
    if (isOpen && project?.id) {
      loadData();
    }
  });

  async function loadData() {
    try {
      loading = true;
      const [managersResult, membersResult] = await Promise.all([
        api.time.projects.getManagers(project.id),
        api.time.projects.getMembers(project.id)
      ]);
      managers = managersResult || [];
      members = membersResult || [];
    } catch (err) {
      console.error('Failed to load permissions:', err);
      managers = [];
      members = [];
    } finally {
      loading = false;
    }
  }

  function toggleAddForm() {
    showAddForm = !showAddForm;
    if (!showAddForm) {
      resetAddForm();
    }
  }

  function resetAddForm() {
    addType = 'user';
    selectedUserId = null;
    selectedGroupId = null;
  }

  async function handleAdd() {
    try {
      saving = true;
      const id = addType === 'user' ? selectedUserId : selectedGroupId;

      if (activeTab === 'managers') {
        await api.time.projects.addManager(project.id, addType, id);
      } else {
        await api.time.projects.addMember(project.id, addType, id);
      }

      await loadData();
      toggleAddForm();
    } catch (err) {
      console.error('Failed to add:', err);
      alert(t('time.permissions.failedToAdd') + ': ' + (err.message || err));
    } finally {
      saving = false;
    }
  }

  function initiateRemove(item, type) {
    itemToRemove = item;
    removeType = type;
    showRemoveConfirmation = true;
  }

  async function confirmRemove() {
    try {
      if (removeType === 'manager') {
        await api.time.projects.removeManager(project.id, itemToRemove.id);
      } else {
        await api.time.projects.removeMember(project.id, itemToRemove.id);
      }
      await loadData();
    } catch (err) {
      console.error('Failed to remove:', err);
      alert(t('time.permissions.failedToRemove') + ': ' + (err.message || err));
    } finally {
      showRemoveConfirmation = false;
      itemToRemove = null;
    }
  }

  function handleClose() {
    showAddForm = false;
    resetAddForm();
    onClose();
  }
</script>

<Modal {isOpen} maxWidth="max-w-2xl" onclose={handleClose}>
  <ModalHeader
    title={t('time.permissions.title')}
    subtitle={project?.name || ''}
    icon={Shield}
    onClose={handleClose}
  />

  <div class="p-6">
    <!-- Tabs -->
    <div class="flex border-b mb-6" style="border-color: var(--ds-border);">
      <button
        class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'managers' ? 'border-blue-500 text-blue-600' : 'border-transparent'}"
        style="{activeTab !== 'managers' ? 'color: var(--ds-text-subtle);' : ''}"
        onclick={() => { activeTab = 'managers'; showAddForm = false; resetAddForm(); }}
      >
        {t('time.permissions.managers')} ({managers.length})
      </button>
      <button
        class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px {activeTab === 'members' ? 'border-blue-500 text-blue-600' : 'border-transparent'}"
        style="{activeTab !== 'members' ? 'color: var(--ds-text-subtle);' : ''}"
        onclick={() => { activeTab = 'members'; showAddForm = false; resetAddForm(); }}
      >
        {t('time.permissions.members')} ({members.length})
      </button>
    </div>

    <!-- Add Button -->
    <div class="flex justify-end mb-4">
      <Button
        onclick={toggleAddForm}
        variant="primary"
        size="medium"
        icon={Plus}
      >
        {activeTab === 'managers' ? t('time.permissions.addManager') : t('time.permissions.addMember')}
      </Button>
    </div>

    <!-- Add Form -->
    {#if showAddForm}
      <div
        class="p-4 rounded border mb-4"
        style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
      >
        <h4 class="text-sm font-medium mb-4" style="color: var(--ds-text);">
          {activeTab === 'managers' ? t('time.permissions.addManager') : t('time.permissions.addMember')}
        </h4>

        <AssigneePicker
          bind:type={addType}
          bind:userId={selectedUserId}
          bind:groupId={selectedGroupId}
          confirmText={t('common.add')}
          cancelText={t('common.cancel')}
          on_confirm={handleAdd}
          on_cancel={toggleAddForm}
          disabled={saving}
        />
      </div>
    {/if}

    <!-- List -->
    <div class="space-y-2">
      {#if loading}
        <div class="flex items-center justify-center py-12">
          <Spinner />
        </div>
      {:else if activeTab === 'managers'}
        {#if managers.length === 0}
          <div
            class="text-center py-12 rounded border-2 border-dashed"
            style="border-color: var(--ds-border);"
          >
            <div class="flex justify-center mb-3">
              <Shield class="w-12 h-12" style="color: var(--ds-text-subtle);" />
            </div>
            <p class="text-sm font-medium" style="color: var(--ds-text);">
              {t('time.permissions.noManagers')}
            </p>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              {t('time.permissions.noManagersHint')}
            </p>
          </div>
        {:else}
          {#each managers as manager}
            <div
              class="flex items-center gap-4 p-4 rounded border"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
            >
              <!-- Icon -->
              <div
                class="flex-shrink-0 w-10 h-10 rounded-full flex items-center justify-center"
                style="background-color: {manager.manager_type === 'user' ? 'var(--ds-interactive-subtle)' : 'var(--ds-surface)'};"
              >
                {#if manager.manager_type === 'user'}
                  <User class="w-5 h-5" style="color: var(--ds-interactive);" />
                {:else}
                  <Users class="w-5 h-5" style="color: var(--ds-text-subtle);" />
                {/if}
              </div>

              <!-- Info -->
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <h4 class="text-sm font-medium" style="color: var(--ds-text);">
                    {manager.manager_name}
                  </h4>
                  <span
                    class="px-2 py-0.5 text-xs font-medium rounded"
                    style="background-color: {manager.manager_type === 'user' ? 'var(--ds-interactive-subtle)' : 'var(--ds-surface)'}; color: {manager.manager_type === 'user' ? 'var(--ds-interactive-pressed)' : 'var(--ds-text-subtle)'};"
                  >
                    {manager.manager_type === 'user' ? t('common.user') : t('common.group')}
                  </span>
                </div>
                {#if manager.manager_email}
                  <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                    {manager.manager_email}
                  </p>
                {/if}
                <p class="text-xs mt-1" style="color: var(--ds-text-disabled);">
                  {t('time.permissions.grantedAt')} {formatDateTimeLocale(manager.granted_at) || '-'}
                </p>
              </div>

              <!-- Remove Button -->
              <ActionButton
                icon={X}
                variant="ghost-danger"
                title={t('time.permissions.removeManager')}
                onclick={() => initiateRemove(manager, 'manager')}
              />
            </div>
          {/each}
        {/if}
      {:else}
        {#if members.length === 0}
          <div
            class="text-center py-12 rounded border-2 border-dashed"
            style="border-color: var(--ds-border);"
          >
            <div class="flex justify-center mb-3">
              <UserCheck class="w-12 h-12" style="color: var(--ds-text-subtle);" />
            </div>
            <p class="text-sm font-medium" style="color: var(--ds-text);">
              {t('time.permissions.noMembers')}
            </p>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              {t('time.permissions.noMembersHint')}
            </p>
          </div>
        {:else}
          {#each members as member}
            <div
              class="flex items-center gap-4 p-4 rounded border"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
            >
              <!-- Icon -->
              <div
                class="flex-shrink-0 w-10 h-10 rounded-full flex items-center justify-center"
                style="background-color: {member.member_type === 'user' ? 'var(--ds-interactive-subtle)' : 'var(--ds-surface)'};"
              >
                {#if member.member_type === 'user'}
                  <User class="w-5 h-5" style="color: var(--ds-interactive);" />
                {:else}
                  <Users class="w-5 h-5" style="color: var(--ds-text-subtle);" />
                {/if}
              </div>

              <!-- Info -->
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <h4 class="text-sm font-medium" style="color: var(--ds-text);">
                    {member.member_name}
                  </h4>
                  <span
                    class="px-2 py-0.5 text-xs font-medium rounded"
                    style="background-color: {member.member_type === 'user' ? 'var(--ds-interactive-subtle)' : 'var(--ds-surface)'}; color: {member.member_type === 'user' ? 'var(--ds-interactive-pressed)' : 'var(--ds-text-subtle)'};"
                  >
                    {member.member_type === 'user' ? t('common.user') : t('common.group')}
                  </span>
                </div>
                {#if member.member_email}
                  <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                    {member.member_email}
                  </p>
                {/if}
                <p class="text-xs mt-1" style="color: var(--ds-text-disabled);">
                  {t('time.permissions.grantedAt')} {formatDateTimeLocale(member.granted_at) || '-'}
                </p>
              </div>

              <!-- Remove Button -->
              <ActionButton
                icon={X}
                variant="ghost-danger"
                title={t('time.permissions.removeMember')}
                onclick={() => initiateRemove(member, 'member')}
              />
            </div>
          {/each}
        {/if}
      {/if}
    </div>

    <!-- Help Text -->
    <div
      class="mt-6 p-4 rounded"
      style="background-color: var(--ds-surface);"
    >
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        {#if activeTab === 'managers'}
          <strong style="color: var(--ds-text);">{t('time.permissions.managersNote')}</strong> {t('time.permissions.managersNoteText')}
        {:else}
          <strong style="color: var(--ds-text);">{t('time.permissions.membersNote')}</strong> {t('time.permissions.membersNoteText')}
        {/if}
      </p>
    </div>
  </div>
</Modal>

<!-- Remove Confirmation Dialog -->
<ConfirmDialog
  bind:show={showRemoveConfirmation}
  title={removeType === 'manager' ? t('time.permissions.removeManager') : t('time.permissions.removeMember')}
  message={t('time.permissions.confirmRemove', { name: itemToRemove?.manager_name || itemToRemove?.member_name })}
  confirmText={t('common.remove')}
  cancelText={t('common.cancel')}
  variant="danger"
  onconfirm={confirmRemove}
  oncancel={() => {
    showRemoveConfirmation = false;
    itemToRemove = null;
  }}
/>
