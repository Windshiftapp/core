<script>
  import { navigate } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { SquareKanban, Save, Tag } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import Select from '../../components/Select.svelte';
  import { createEventDispatcher } from 'svelte';

  const dispatch = createEventDispatcher();

  // Props
  export let collection = null;
  export let workspace = null;
  export let isEditing = false;
  export let canSave = false;
  export let categories = [];
  export let returnPath = null;

  // Computed: is this a global collection (no workspace)?
  $: isGlobal = !collection?.workspace_id;

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
    dispatch('save');
  }

  function handleAssociateWorkspace() {
    dispatch('associate-workspace');
  }

  function handleNameChange(event) {
    dispatch('name-change', event.target.value);
  }

  function handleDescriptionChange(event) {
    dispatch('description-change', event.target.value);
  }

  function handleCategoryChange(event) {
    const value = event.target.value;
    dispatch('category-change', value === '' || value === 'null' ? null : parseInt(value, 10));
  }

  $: workspaceName = workspace?.name
    ? `${workspace.name}${workspace.key ? ` (${workspace.key})` : ''}`
    : '';
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
        <Button
          onclick={handleAssociateWorkspace}
          variant="ghost"
          size="sm"
        >
          <SquareKanban class="w-4 h-4 mr-2" />
          {workspace ? t('collections.changeWorkspace') : t('collections.associateWorkspace')}
        </Button>
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
          <select
            value={collection.category_id || ''}
            onchange={handleCategoryChange}
            class="text-xs py-0.5 px-1 rounded border bg-transparent"
            style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
          >
            <option value="">{t('collections.noCategory')}</option>
            {#each categories as category}
              <option value={category.id}>{category.name}</option>
            {/each}
          </select>
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
