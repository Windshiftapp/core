<script>
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import SquareKanban from '@tabler/icons-svelte-runes/icons/layout-kanban';
  import Save from '@tabler/icons-svelte-runes/icons/device-floppy';
  import Tag from '@tabler/icons-svelte-runes/icons/tag';
  import IconWorld from '@tabler/icons-svelte-runes/icons/world';
  import IconLock from '@tabler/icons-svelte-runes/icons/lock';
  import IconLockOpen2 from '@tabler/icons-svelte-runes/icons/lock-open-2';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import PublicBoardSharingDialog from './PublicBoardSharingDialog.svelte';

  let {
    collection = null,
    workspace = null,
    isEditing = false,
    canEdit = true,
    canSave = false,
    categories = [],
    returnPath = null,
    onsave = null,
    onassociateworkspace = null,
    onnamechange = null,
    ondescriptionchange = null,
    oncategorychange = null,
    isPublic = false,
    publicSlug = null,
    publicMilestoneSwimlanes = false,
    onpublicsave = null,
    saving = false,
    slugSaved = false,
    showPublicBoard = false,
    isPrivate = false,
    showPrivacyToggle = false,
    onprivacytoggle = null,
  } = $props();

  // Computed: is this a global collection (no workspace)?
  let isGlobal = $derived(!collection?.workspace_id);

  let showPublicDialog = $state(false);

  function openPublicDialog() {
    showPublicDialog = true;
  }

  function closePublicDialog() {
    showPublicDialog = false;
  }

  function handleNavigateWorkspaces() {
    navigate('/workspaces');
  }

  function handleNavigateWorkspace() {
    if (workspace?.id) {
      navigate(`/workspaces/${workspace.id}`);
    }
  }

  function handleNavigateCollections() {
    navigate('/collections');
  }

  function handleCancel() {
    navigate(returnPath || '/collections');
  }

  function handleSave() {
    onsave?.();
  }

  function handleAssociateWorkspace() {
    onassociateworkspace?.();
  }

  function handleNameChange(event) {
    onnamechange?.(event.currentTarget.value);
  }

  function handleDescriptionChange(event) {
    ondescriptionchange?.(event.currentTarget.value);
  }

  function handleCategoryChange(value) {
    oncategorychange?.(value === '' || value === 'null' ? null : parseInt(value, 10));
  }

  let workspaceName = $derived(workspace?.name
    ? `${workspace.name}${workspace.key ? ` (${workspace.key})` : ''}`
    : '');
</script>

<div class="mb-4">
  <!-- Breadcrumb navigation -->
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2 text-sm" style="color: var(--ds-text-subtle);">
      {#if collection?.workspace_id && workspace}
        <!-- Workspace collection breadcrumb -->
        <button
          onclick={handleNavigateWorkspaces}
          class="hover:underline transition-colors"
          style="color: var(--ds-text-subtle);"
        >
          {t('workspaces.title')}
        </button>
        <span>/</span>
        <button
          onclick={handleNavigateWorkspace}
          class="hover:underline transition-colors"
          style="color: var(--ds-text-subtle);"
        >
          {workspace.name}
        </button>
        <span>/</span>
      {:else}
        <!-- Global collection breadcrumb -->
        <span class="shrink-0 whitespace-nowrap">{t('collections.globalCollection')}</span>
        <span>/</span>
      {/if}

      {#if isEditing && collection && canEdit}
        <!-- Editable collection name -->
        <Input
          dataTestid="collection-name"
          type="text"
          value={collection.name}
          oninput={handleNameChange}
          class="text-sm font-medium bg-transparent border-none p-0 focus:outline-none focus:ring-0"
          style="background-color: transparent; border-color: transparent; color: var(--ds-text); min-width: 150px;"
          placeholder={t('collections.collectionName')}
        />
      {:else if collection}
        <span style="color: var(--ds-text);" class="font-medium">{collection.name}</span>
      {:else}
        <span style="color: var(--ds-text);" class="font-medium">{t('collections.newCollection')}</span>
      {/if}
    </div>

    <!-- Action buttons -->
    <div class="flex items-center gap-2">
      {#if isEditing && collection && canEdit}
        {#if showPrivacyToggle}
          <Button
            dataTestid="collection-privacy-toggle"
            onclick={() => onprivacytoggle?.()}
            variant={isPrivate ? 'selected' : 'default'}
            size="sm"
            icon={isPrivate ? IconLock : IconLockOpen2}
          >
            {isPrivate ? t('collections.private') : t('collections.shared')}
          </Button>
        {/if}

        {#if showPublicBoard}
          <Button
            dataTestid="public-board-button"
            onclick={openPublicDialog}
            variant={isPublic ? 'selected' : 'default'}
            size="sm"
            icon={IconWorld}
          >
            {isPublic ? t('collections.public') : t('collections.publish')}
          </Button>
        {/if}

        <Tooltip content={workspace ? t('collections.changeWorkspace') : t('collections.associateWorkspace')}>
          <button
            onclick={handleAssociateWorkspace}
            class="inline-flex items-center justify-center w-8 h-8 rounded-md border cursor-pointer transition-colors public-inactive"
          >
            <SquareKanban class="w-4 h-4" />
          </button>
        </Tooltip>

        <!-- Divider between icon buttons and text buttons -->
        <div class="w-px h-6 mx-0.5" style="background-color: var(--ds-border);"></div>

        <Button
          onclick={handleCancel}
          variant="default"
          size="sm"
        >
          {t('common.cancel')}
        </Button>
      {/if}
      {#if !collection || canEdit}
        <Button
          dataTestid="collection-save"
          onclick={handleSave}
          variant="primary"
          size="sm"
          disabled={!canSave}
        >
          <Save class="w-4 h-4 mr-2" />
          {#if isEditing && collection}
            {t('collections.updateCollection')}
          {:else}
            {t('collections.saveCollection')}
          {/if}
        </Button>
      {/if}
    </div>
  </div>

  <!-- Editable description (only when editing) -->
  {#if isEditing && collection && canEdit}
    <div class="mt-2 flex items-center gap-4">
      <Input
        type="text"
        value={collection.description || ''}
        oninput={handleDescriptionChange}
        class="text-sm bg-transparent border-none p-0 focus:outline-none focus:ring-0 flex-1"
        style="background-color: transparent; border-color: transparent; color: var(--ds-text-subtle);"
        placeholder={t('collections.collectionDescription')}
      />

      <!-- Category selector for global collections -->
      {#if isGlobal && categories.length > 0}
        <div class="flex items-center gap-2">
          <Tag class="w-3 h-3" style="color: var(--ds-text-subtlest);" />
          <Select
            options={[{ value: '', label: t('collections.noCategory') }, ...categories.map(c => ({ value: c.id, label: c.name }))]}
            value={collection.category_id || ''}
            onchange={handleCategoryChange}
            size="small"
          />
        </div>
      {/if}

      <div class="flex items-center gap-1 text-xs" style="color: var(--ds-text-subtlest);">
        <SquareKanban class="w-3 h-3" />
        {#if workspace}
          <span>{workspaceName}</span>
        {:else}
          <span>{t('collections.globalCollection')}</span>
        {/if}
      </div>
    </div>
  {/if}
</div>

<PublicBoardSharingDialog
  isOpen={showPublicDialog}
  {isPublic}
  {publicSlug}
  {publicMilestoneSwimlanes}
  {slugSaved}
  {saving}
  onclose={closePublicDialog}
  onsave={onpublicsave}
/>

<style>
  .public-inactive {
    background-color: var(--ds-surface);
    border-color: var(--ds-border);
    color: var(--ds-text-subtle);
  }
  .public-inactive:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
