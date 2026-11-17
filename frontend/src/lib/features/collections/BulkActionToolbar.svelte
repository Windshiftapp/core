<script>
  import { t } from '../../stores/i18n.svelte.js';
  import { objectDisplayName } from '../../utils/systemLabels.js';
  import Button from '../../components/Button.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import WorkspaceLabelCombobox from '../../pickers/WorkspaceLabelCombobox.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';

  // The bottom bulk-action bar shared by the board and list views. Both views
  // pass the same selection object from useItemBulkSelection, so the actions
  // and their permissions stay identical; only the test ids differ.
  let {
    selection,
    moveTargets = [],
    priorities = [],
    testidPrefix = 'board',
    menuTestidPrefix = testidPrefix === 'board' ? 'bulk' : `${testidPrefix}-bulk`,
  } = $props();

  let bulkStatusMenuItems = $derived(
    moveTargets.map((target) => ({
      id: `${menuTestidPrefix}-status-${target.id}`,
      testid: `${menuTestidPrefix}-status-${target.statusId}`,
      title: target.title,
      iconDot: true,
      iconColor: target.color || 'var(--ds-text-subtle)',
      onClick: () => selection.bulkMoveToStatus(target.statusId),
    }))
  );

  let bulkPriorityMenuItems = $derived([
    {
      id: `${menuTestidPrefix}-priority-none`,
      testid: `${menuTestidPrefix}-priority-none`,
      title: t('pickers.noPriority'),
      onClick: () => selection.bulkUpdateFields({ priority_id: null }),
    },
    ...priorities.map((priority) => ({
      id: `${menuTestidPrefix}-priority-${priority.id}`,
      testid: `${menuTestidPrefix}-priority-${priority.id}`,
      title: objectDisplayName(priority, 'priority'),
      iconDot: true,
      iconColor: priority.color || 'var(--ds-text-subtle)',
      onClick: () => selection.bulkUpdateFields({ priority_id: priority.id }),
    })),
  ]);
</script>

{#if selection.selectionMode}
  <div
    class="fixed bottom-4 left-1/2 z-40 flex max-w-[calc(100vw-2rem)] -translate-x-1/2 flex-wrap items-center gap-2 rounded-xl border border-ds-border-bold bg-ds-surface-overlay px-4 py-2.5"
    style="box-shadow: 0 12px 32px rgba(9, 30, 66, 0.32), 0 0 0 1px var(--ds-border-bold);"
    data-testid={`${testidPrefix}-bulk-toolbar`}
  >
    {#if selection.selectedCount > 0}
      <span
        class="mr-1 inline-flex items-center rounded-full bg-ds-interactive px-3 py-1 text-sm font-semibold text-white"
        data-testid={`${testidPrefix}-bulk-count`}
      >
        {t('collections.bulkSelected', { count: selection.selectedCount })}
      </span>
    {:else}
      <span
        class="mr-1 text-sm font-medium text-ds-text-subtle"
        data-testid={`${testidPrefix}-bulk-hint`}
      >
        {t('collections.bulkSelectHint')}
      </span>
    {/if}
    <DropdownMenu
      items={bulkStatusMenuItems}
      triggerText={t('collections.bulkSetStatus')}
      placement="top-start"
      maxWidth="max-w-xs"
      disabled={selection.bulkBusy || !selection.selectedItemsEditable}
      triggerClass="px-3 py-1.5 rounded text-sm font-medium"
      triggerStyle="color: var(--ds-text); background-color: var(--ds-background-neutral);"
      triggerTestid={`${testidPrefix}-bulk-status`}
    />
    <Button
      variant="secondary"
      size="sm"
      onclick={selection.openBulkAssigneeModal}
      disabled={selection.bulkBusy || !selection.selectedItemsEditable}
      dataTestid={`${testidPrefix}-bulk-assignee`}
    >
      {t('collections.bulkSetAssignee')}
    </Button>
    <DropdownMenu
      items={bulkPriorityMenuItems}
      triggerText={t('collections.bulkSetPriority')}
      placement="top-start"
      maxWidth="max-w-xs"
      disabled={selection.bulkBusy || !selection.selectedItemsEditable}
      triggerClass="px-3 py-1.5 rounded text-sm font-medium"
      triggerStyle="color: var(--ds-text); background-color: var(--ds-background-neutral);"
      triggerTestid={`${testidPrefix}-bulk-priority`}
    />
    <Button
      variant="secondary"
      size="sm"
      onclick={selection.openBulkLabelsModal}
      disabled={selection.bulkBusy || !selection.selectedItemsEditable}
      dataTestid={`${testidPrefix}-bulk-labels`}
    >
      {t('collections.bulkLabels')}
    </Button>
    <Button
      variant="ghost"
      size="sm"
      onclick={selection.clearSelection}
      disabled={selection.bulkBusy || selection.selectedCount === 0}
      dataTestid={`${testidPrefix}-bulk-clear`}
    >
      {t('collections.bulkClear')}
    </Button>
    <Button
      variant="primary"
      size="sm"
      onclick={selection.exitSelectionMode}
      disabled={selection.bulkBusy}
      dataTestid={`${testidPrefix}-bulk-done`}
    >
      {t('collections.bulkDone')}
    </Button>
  </div>

  <Modal
    bind:isOpen={selection.bulkAssigneeModalOpen}
    maxWidth="max-w-lg"
    closeOnBackdropClick={false}
    onclose={() => (selection.bulkAssigneeModalOpen = false)}
    dataTestid={`${testidPrefix}-bulk-assignee-modal`}
  >
    <ModalHeader
      title={t('collections.bulkSetAssignee')}
      subtitle={t('collections.bulkSelected', { count: selection.selectedCount })}
      onclose={() => (selection.bulkAssigneeModalOpen = false)}
    />
    <div class="p-6">
      <UserPicker
        bind:value={selection.bulkAssigneeId}
        users={selection.assignableUsers}
        loading={selection.assignableUsersLoading}
        showUnassigned={true}
        unassignedLabel={t('common.unassigned')}
        placeholder={t('pickers.searchUser')}
        onSelect={() => (selection.bulkAssigneeChosen = true)}
      />
      {#if selection.bulkAssigneeSpansWorkspaces}
        <p class="mt-2 text-xs text-ds-text-subtle">
          {t('collections.bulkAssigneeHint')}
        </p>
      {/if}
    </div>
    <DialogFooter
      onCancel={() => (selection.bulkAssigneeModalOpen = false)}
      onConfirm={selection.applyBulkAssignee}
      confirmLabel={t('common.apply')}
      loading={selection.bulkBusy}
      confirmDisabled={!selection.bulkAssigneeChosen}
      confirmTestid={`${testidPrefix}-bulk-assignee-apply`}
      cancelTestid={`${testidPrefix}-bulk-assignee-cancel`}
    />
  </Modal>

  <Modal
    bind:isOpen={selection.bulkLabelsModalOpen}
    maxWidth="max-w-lg"
    closeOnBackdropClick={false}
    onclose={() => (selection.bulkLabelsModalOpen = false)}
    dataTestid={`${testidPrefix}-bulk-labels-modal`}
  >
    <ModalHeader
      title={t('collections.bulkLabels')}
      subtitle={t('collections.bulkLabelsHint', { count: selection.selectedCount })}
      onclose={() => (selection.bulkLabelsModalOpen = false)}
    />
    <div class="p-6">
      <WorkspaceLabelCombobox
        id={`${testidPrefix}-bulk-label-picker-input`}
        workspaceId={selection.bulkLabelsWorkspaceId}
        testidPrefix={`${testidPrefix}-bulk-label-picker`}
        bind:value={selection.bulkLabelNames}
        placeholder={t('items.selectOrCreateLabels')}
        onSelect={(result) => (selection.bulkLabelIds = (result?.labels || []).map((label) => label.id))}
      />
    </div>
    <DialogFooter
      onCancel={() => (selection.bulkLabelsModalOpen = false)}
      onConfirm={selection.applyBulkLabels}
      confirmLabel={t('collections.bulkLabelsApply')}
      loading={selection.bulkBusy}
      confirmTestid={`${testidPrefix}-bulk-labels-apply`}
      cancelTestid={`${testidPrefix}-bulk-labels-cancel`}
    />
  </Modal>
{/if}
