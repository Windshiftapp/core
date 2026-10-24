<script>
  import Checkbox from '../../components/Checkbox.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import Milestone from '@lucide/svelte/icons/milestone';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    isOpen = false,
    globalMilestones = [],
    selectedGlobalMilestoneIds = [],
    showCompleted = true,
    showOpenFromCompleted = false,
    saving = false,
    onclose = null,
    onsave = null,
  } = $props();

  let draftIds = $state([]);
  let draftShowCompleted = $state(true);
  let draftShowOpenFromCompleted = $state(false);
  let wasOpen = false;

  $effect(() => {
    if (isOpen && !wasOpen) {
      draftIds = (selectedGlobalMilestoneIds || []).map(Number);
      draftShowCompleted = showCompleted !== false;
      draftShowOpenFromCompleted = showOpenFromCompleted === true;
    }
    wasOpen = isOpen;
  });

  const selectedCount = $derived(draftIds.length);

  const hasChanges = $derived.by(() => {
    if (draftShowCompleted !== (showCompleted !== false)) return true;
    if (draftShowOpenFromCompleted !== (showOpenFromCompleted === true)) return true;
    const current = new Set((selectedGlobalMilestoneIds || []).map(Number));
    if (current.size !== draftIds.length) return true;
    for (const id of draftIds) {
      if (!current.has(id)) return true;
    }
    return false;
  });

  function close() {
    if (!saving) onclose?.();
  }

  function save() {
    if (saving || !hasChanges) return;
    onsave?.({
      globalMilestoneIds: [...draftIds],
      showCompleted: draftShowCompleted,
      showOpenFromCompleted: draftShowOpenFromCompleted,
    });
  }
</script>

<Modal
  {isOpen}
  preventClose={saving}
  closeOnBackdropClick={false}
  maxWidth="max-w-lg"
  onclose={close}
  onSubmit={save}
  submitDisabled={!hasChanges}
  dataTestid="milestone-swimlane-config-dialog"
>
  <ModalHeader
    title={t('boardControls.configureMilestones')}
    subtitle={t('boardControls.configureMilestonesDescription')}
    icon={Milestone}
    onclose={close}
  />

  <div class="space-y-5 px-6 py-5">
    <div
      class="space-y-4 rounded-lg border p-4"
      style="border-color: var(--ds-border); background-color: var(--ds-surface);"
    >
      <Checkbox
        id="milestone-swimlanes-show-completed"
        checked={draftShowCompleted}
        onchange={(checked) => (draftShowCompleted = checked)}
        label={t('boardControls.showCompletedMilestones')}
        hint={t('boardControls.showCompletedMilestonesHint')}
        dataTestid="milestone-swimlanes-show-completed"
      />
      <Checkbox
        id="milestone-swimlanes-show-open-from-completed"
        checked={draftShowOpenFromCompleted}
        onchange={(checked) => (draftShowOpenFromCompleted = checked)}
        label={t('boardControls.showOpenFromCompletedMilestones')}
        hint={t('boardControls.showOpenFromCompletedMilestonesHint')}
        dataTestid="milestone-swimlanes-show-open-from-completed"
      />
    </div>

    <div>
      <div class="mb-2 flex items-center justify-between gap-2">
        <span class="text-sm font-medium" style="color: var(--ds-text);">
          {t('boardControls.globalMilestones')}
        </span>
        <span class="text-xs" style="color: var(--ds-text-subtle);">
          {t('boardControls.globalMilestonesSelected', { count: selectedCount })}
        </span>
      </div>
      <p class="mb-3 text-xs" style="color: var(--ds-text-subtle);">
        {t('boardControls.globalMilestonesHint')}
      </p>
      {#if globalMilestones.length === 0}
        <p
          class="rounded-md border px-3 py-2 text-sm"
          style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
          data-testid="milestone-swimlanes-no-globals"
        >
          {t('boardControls.noGlobalMilestones')}
        </p>
      {:else}
        <MilestoneCombobox
          bind:value={draftIds}
          multiple={true}
          milestones={globalMilestones}
          workspaceId={null}
          allowCreate={false}
          includeCompleted={true}
          placeholder={t('pickers.selectMilestones')}
          searchTestid="milestone-swimlanes-search"
          optionTestid={(option) => `milestone-swimlanes-global-${option.value}`}
          class="w-full"
        />
      {/if}
    </div>
  </div>

  <DialogFooter
    cancelLabel={t('dialogs.cancel')}
    confirmLabel={t('common.save')}
    loadingLabel={t('common.saving')}
    loading={saving}
    confirmDisabled={!hasChanges}
    onCancel={close}
    onConfirm={save}
    showKeyboardHint={true}
    cancelTestid="milestone-swimlanes-cancel"
    confirmTestid="milestone-swimlanes-save"
  />
</Modal>
