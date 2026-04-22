<script>
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { IconLayoutKanban as SquareKanban, IconDeviceFloppy as Save, IconTag as Tag, IconWorld, IconExternalLink, IconLoader2 } from '@tabler/icons-svelte-runes';
  import CopyButton from '../../components/CopyButton.svelte';
  import Button from '../../components/Button.svelte';
  import Select from '../../components/Select.svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import DescriptionText from '../../components/DescriptionText.svelte';

  let {
    collection = null,
    workspace = null,
    isEditing = false,
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
    onpublictoggle = null,
    onslugchange = null,
    onslugsave = null,
    saving = false,
    slugSaved = false,
    showPublicBoard = false,
  } = $props();

  // Computed: is this a global collection (no workspace)?
  let isGlobal = $derived(!collection?.workspace_id);

  // Public board popover state
  let showPublicPopover = $state(false);
  let popoverRef = $state(null);

  function togglePopover() {
    showPublicPopover = !showPublicPopover;
  }

  function handleClickOutside(event) {
    if (popoverRef && !popoverRef.contains(event.target)) {
      showPublicPopover = false;
    }
  }

  function handleKeydown(event) {
    if (event.key === 'Escape') {
      showPublicPopover = false;
    }
  }

  $effect(() => {
    if (showPublicPopover) {
      document.addEventListener('click', handleClickOutside, true);
      document.addEventListener('keydown', handleKeydown);
      return () => {
        document.removeEventListener('click', handleClickOutside, true);
        document.removeEventListener('keydown', handleKeydown);
      };
    }
  });

  const publicBoardUrl = () =>
    publicSlug ? `${window.location.origin}/board/${publicSlug}` : '';

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
    onnamechange?.(event.target.value);
  }

  function handleDescriptionChange(event) {
    ondescriptionchange?.(event.target.value);
  }

  function handleCategoryChange(event) {
    const value = event.target.value;
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
        <span>{t('collections.globalCollection')}</span>
        <span>/</span>
      {/if}

      {#if isEditing && collection}
        <!-- Editable collection name -->
        <input
          type="text"
          value={collection.name}
          oninput={handleNameChange}
          class="text-sm font-medium bg-transparent border-none p-0 focus:outline-none focus:ring-0"
          style="color: var(--ds-text); min-width: 150px;"
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
      {#if isEditing && collection}
        {#if showPublicBoard}
          <!-- Public Board button with popover -->
          <div class="relative" bind:this={popoverRef}>
            <Tooltip content="Public Board">
              <button
                onclick={togglePopover}
                class="inline-flex items-center justify-center w-8 h-8 rounded-md border cursor-pointer transition-colors"
                class:public-active={isPublic}
                class:public-inactive={!isPublic}
              >
                <IconWorld class="w-4 h-4" />
              </button>
            </Tooltip>

            {#if showPublicPopover}
              <div class="absolute right-0 top-full mt-2 w-80 rounded-lg border shadow-lg z-50 p-4 space-y-3" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                <div class="text-sm font-medium" style="color: var(--ds-text);">Public Board</div>
                <p class="text-xs" style="color: var(--ds-text-subtle);">Share a read-only Kanban board publicly.</p>

                <!-- Public toggle -->
                <label class="flex items-center gap-3 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={isPublic}
                    onchange={() => onpublictoggle?.()}
                    class="w-4 h-4 rounded"
                    style="accent-color: #2874BB;"
                  />
                  <span class="text-sm" style="color: var(--ds-text);">Enable public sharing</span>
                </label>

                {#if isPublic}
                  <!-- Slug input -->
                  <div>
                    <label class="block text-xs font-medium mb-1" style="color: var(--ds-text);">Board URL slug</label>
                    <div class="flex items-center gap-1.5">
                      <span class="text-xs" style="color: var(--ds-text-subtle);">/board/</span>
                      <input
                        type="text"
                        value={publicSlug || ''}
                        oninput={(e) => onslugchange?.(e.target.value || null)}
                        placeholder="my-board"
                        class="flex-1 px-2 py-1 text-sm rounded border"
                        style="border-color: var(--ds-border); background-color: var(--ds-input); color: var(--ds-text);"
                      />
                      <button
                        onclick={() => onslugsave?.()}
                        disabled={saving || !publicSlug}
                        class="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium rounded border cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                        style="border-color: var(--ds-border-brand); background-color: var(--ds-surface-brand); color: white;"
                      >
                        {#if saving}
                          <IconLoader2 class="w-3.5 h-3.5 animate-spin" />
                        {:else}
                          <Save class="w-3.5 h-3.5" />
                        {/if}
                        Save
                      </button>
                    </div>
                    <DescriptionText variant="subtlest">Lowercase letters, numbers, and hyphens.</DescriptionText>
                  </div>

                  <!-- Copy URL + Preview (only when slug is persisted) -->
                  {#if publicSlug && slugSaved}
                    <div class="flex items-center gap-2 pt-1">
                      <CopyButton
                        getText={publicBoardUrl}
                        size="sm"
                        label="Copy URL"
                        copiedLabel="Copied!"
                      />
                      <a
                        href="/board/{publicSlug}"
                        target="_blank"
                        class="inline-flex items-center gap-1 text-xs underline"
                        style="color: var(--ds-link);"
                      >
                        <IconExternalLink class="w-3.5 h-3.5" />
                        Preview
                      </a>
                    </div>
                  {/if}
                {/if}
              </div>
            {/if}
          </div>
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
      <Button
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
    </div>
  </div>

  <!-- Editable description (only when editing) -->
  {#if isEditing && collection}
    <div class="mt-2 flex items-center gap-4">
      <input
        type="text"
        value={collection.description || ''}
        oninput={handleDescriptionChange}
        class="text-sm bg-transparent border-none p-0 focus:outline-none focus:ring-0 flex-1"
        style="color: var(--ds-text-subtle);"
        placeholder={t('collections.collectionDescription')}
      />

      <!-- Category selector for global collections -->
      {#if isGlobal && categories.length > 0}
        <div class="flex items-center gap-2">
          <Tag class="w-3 h-3" style="color: var(--ds-text-subtlest);" />
          <Select
            options={[{ value: '', label: t('collections.noCategory') }, ...categories.map(c => ({ value: c.id, label: c.name }))]}
            value={collection.category_id || ''}
            onchange={(v) => handleCategoryChange({ target: { value: v } })}
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

<style>
  .public-active {
    background-color: color-mix(in srgb, #16a34a 12%, transparent);
    border-color: color-mix(in srgb, #16a34a 30%, transparent);
    color: #16a34a;
  }
  .public-active:hover {
    background-color: color-mix(in srgb, #16a34a 18%, transparent);
  }
  .public-inactive {
    background-color: var(--ds-surface);
    border-color: var(--ds-border);
    color: var(--ds-text-subtle);
  }
  .public-inactive:hover {
    background-color: var(--ds-surface-hovered);
  }
</style>
