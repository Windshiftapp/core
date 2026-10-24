<script>
  import IconExternalLink from '@tabler/icons-svelte-runes/icons/external-link';
  import IconWorld from '@tabler/icons-svelte-runes/icons/world';
  import Button from '../../components/Button.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import CopyButton from '../../components/CopyButton.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';
  import Input from '../../components/Input.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import { publicBaseURL } from '../../runtime/contextPath.js';
  import { t } from '../../stores/i18n.svelte.js';

  let {
    isOpen = false,
    isPublic = false,
    publicSlug = null,
    publicMilestoneSwimlanes = false,
    slugSaved = false,
    saving = false,
    onclose = null,
    onsave = null,
  } = $props();

  let draftIsPublic = $state(false);
  let draftSlug = $state('');
  let draftPublicMilestoneSwimlanes = $state(false);
  let saveError = $state('');
  let wasOpen = false;

  const normalizedSlug = $derived(draftSlug.trim());
  const slugIsValid = $derived(/^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$/.test(normalizedSlug));
  const hasChanges = $derived(
    draftIsPublic !== isPublic ||
      normalizedSlug !== (publicSlug || '') ||
      draftPublicMilestoneSwimlanes !== publicMilestoneSwimlanes
  );
  const persistedUrlAvailable = $derived(
    isPublic && slugSaved && normalizedSlug === (publicSlug || '')
  );
  const publicBoardUrl = $derived(
    normalizedSlug ? `${publicBaseURL()}/board/${normalizedSlug}` : ''
  );

  $effect(() => {
    if (isOpen && !wasOpen) {
      draftIsPublic = isPublic;
      draftSlug = publicSlug || '';
      draftPublicMilestoneSwimlanes = publicMilestoneSwimlanes;
      saveError = '';
    }
    wasOpen = isOpen;
  });

  function close() {
    if (!saving) onclose?.();
  }

  async function save() {
    if (saving || (draftIsPublic && !slugIsValid)) return;
    saveError = '';
    const result = await onsave?.({
      isPublic: draftIsPublic,
      publicSlug: normalizedSlug || null,
      publicMilestoneSwimlanes: draftIsPublic ? draftPublicMilestoneSwimlanes : false,
    });
    if (result?.ok === false) {
      saveError = result.error || t('collections.publicBoardSharing.saveError');
      return;
    }
    close();
  }
</script>

<Modal
  {isOpen}
  preventClose={saving}
  closeOnBackdropClick={false}
  maxWidth="max-w-lg"
  onclose={close}
  onSubmit={save}
  submitDisabled={!hasChanges || (draftIsPublic && !slugIsValid)}
  dataTestid="public-board-dialog"
>
  <ModalHeader
    title={t('collections.publicBoardSharing.title')}
    subtitle={t('collections.publicBoardSharing.subtitle')}
    icon={IconWorld}
    onclose={close}
  />

  <div class="space-y-5 px-6 py-5">
    <div
      class="rounded-lg border p-4"
      style="border-color: var(--ds-border); background-color: var(--ds-surface);"
    >
      <Checkbox
        id="public-board-enabled-input"
        checked={draftIsPublic}
        onchange={(checked) => (draftIsPublic = checked)}
        label={t('collections.publicBoardSharing.enable')}
        hint={t('collections.publicBoardSharing.enableHint')}
        dataTestid="public-board-enabled"
      />
    </div>

    {#if draftIsPublic}
      <div>
        <label
          for="public-board-slug"
          class="mb-1.5 block text-sm font-medium"
          style="color: var(--ds-text);"
        >
          {t('collections.publicBoardSharing.boardUrl')}
        </label>
        <div class="flex items-center gap-2">
          <span class="shrink-0 text-sm" style="color: var(--ds-text-subtle);">/board/</span>
          <Input
            id="public-board-slug"
            dataTestid="public-board-slug"
            type="text"
            value={draftSlug}
            oninput={(event) => (draftSlug = event.currentTarget.value)}
            placeholder="my-board"
            class="min-w-0 flex-1"
            size="small"
          />
        </div>
        <DescriptionText variant="subtlest">
          {t('collections.publicBoardSharing.slugHint')}
        </DescriptionText>
      </div>

      {#if persistedUrlAvailable}
        <div class="flex flex-wrap items-center gap-2">
          <CopyButton
            getText={() => publicBoardUrl}
            size="sm"
            label={t('collections.publicBoardSharing.copyLink')}
            copiedLabel={t('collections.publicBoardSharing.copied')}
          />
          <Button
            href={publicBoardUrl}
            target="_blank"
            variant="default"
            size="sm"
            icon={IconExternalLink}
            dataTestid="public-board-preview"
          >
            {t('collections.publicBoardSharing.openBoard')}
          </Button>
        </div>
      {/if}

      <div
        class="rounded-lg border p-4"
        style="border-color: var(--ds-border); background-color: var(--ds-surface);"
      >
        <Checkbox
          id="public-board-milestone-swimlanes-input"
          checked={draftPublicMilestoneSwimlanes}
          onchange={(checked) => (draftPublicMilestoneSwimlanes = checked)}
          label={t('collections.publicBoardSharing.milestoneSwimlanes')}
          hint={t('collections.publicBoardSharing.milestoneSwimlanesHint')}
          dataTestid="public-board-milestone-swimlanes"
        />
      </div>
    {/if}

    <div
      class="rounded-md border px-3 py-2.5 text-xs leading-5"
      style="border-color: var(--ds-border); background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
    >
      {t('collections.publicBoardSharing.scopeNote')}
    </div>

    {#if saveError}
      <div
        data-testid="public-board-error"
        class="rounded-md px-3 py-2 text-sm"
        style="background-color: var(--ds-background-danger); color: var(--ds-text-danger);"
      >
        {saveError}
      </div>
    {/if}
  </div>

  <DialogFooter
    cancelLabel={t('dialogs.cancel')}
    confirmLabel={draftIsPublic
      ? t('collections.publicBoardSharing.save')
      : isPublic
        ? t('collections.publicBoardSharing.disable')
        : t('collections.publicBoardSharing.save')}
    loadingLabel={t('common.saving')}
    loading={saving}
    confirmDisabled={!hasChanges || (draftIsPublic && !slugIsValid)}
    onCancel={close}
    onConfirm={save}
    showKeyboardHint={true}
    cancelTestid="public-board-cancel"
    confirmTestid="public-board-save"
  />
</Modal>
