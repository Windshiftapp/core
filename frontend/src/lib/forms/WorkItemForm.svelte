<script>
  import { MoreHorizontal, Calendar, Flag, User, Layers, ChevronDown } from '@lucide/svelte';
  import { itemTypeIconMap } from '../utils/icons.js';
  import { workItemFormStore } from '../stores/workItemFormStore.svelte.js';
  import { workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateWithOptions } from '../utils/dateFormatter.js';
  import MilkdownEditor from '../editors/LazyMilkdownEditor.svelte';
  import ChipPicker from '../pickers/ChipPicker.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import PriorityPicker from '../pickers/PriorityPicker.svelte';
  import MilestoneCombobox from '../pickers/MilestoneCombobox.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import Label from '../components/Label.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import { createPopover, melt } from '@melt-ui/svelte'; // used for dueDateTrigger/dueDateContent
  import { Milestone as MilestoneIcon } from '@lucide/svelte';

  let {
    nameInputRef = $bindable(null)
  } = $props();

  // Use the store
  const store = workItemFormStore;

  // Track priority object for display (not persisted)
  let selectedPriorityObj = $state(null);

  // Derived state for UI
  let selectedItemTypeIcon = $derived(
    store.selectedItemType?.icon ? itemTypeIconMap[store.selectedItemType.icon] : Layers
  );

  // Additional fields toggle
  let showAdditionalFields = $state(false);

  // Due date popover
  const {
    elements: { trigger: dueDateTrigger, content: dueDateContent },
    states: { open: dueDateOpen }
  } = createPopover({
    positioning: { placement: 'bottom-start', gutter: 4 },
    portal: 'body',
    forceVisible: true
  });

  // Start date popover
  const {
    elements: { trigger: startDateTrigger, content: startDateContent },
    states: { open: startDateOpen }
  } = createPopover({
    positioning: { placement: 'bottom-start', gutter: 4 },
    portal: 'body',
    forceVisible: true
  });

  // End date popover
  const {
    elements: { trigger: endDateTrigger, content: endDateContent },
    states: { open: endDateOpen }
  } = createPopover({
    positioning: { placement: 'bottom-start', gutter: 4 },
    portal: 'body',
    forceVisible: true
  });

  // Helper functions
  function formatDueDate(dateStr) {
    if (!dateStr) return null;
    const date = new Date(dateStr);
    const today = new Date();
    const diffDays = Math.ceil((date.getTime() - today.getTime()) / (1000 * 60 * 60 * 24));
    if (diffDays === 0) return t('common.today');
    if (diffDays === 1) return t('common.tomorrow');
    if (diffDays === -1) return t('common.yesterday');
    if (diffDays > 0 && diffDays <= 7) return `${diffDays} days`;
    return formatDateWithOptions(date, { month: 'short', day: 'numeric' });
  }

  // Reactive effects for data loading based on form state

  // Load workspace details when workspace changes
  $effect(() => {
    if (store.selectedWorkspace) {
      store.loadWorkspaceDetails(store.selectedWorkspace.id);
    }
  });

  // Load config set when workspace_id changes
  $effect(() => {
    if (store.formData.workspace_id && store.configSetLoadedForWorkspace !== store.formData.workspace_id) {
      store.loadConfigSetForWorkspace(store.formData.workspace_id);
    }
  });

  // Load screen fields when workspace and item type are ready
  $effect(() => {
    if (
      store.formData.workspace_id &&
      store.formData.item_type_id &&
      store.customFieldsLoaded &&
      store.configSetLoadedForWorkspace === store.formData.workspace_id
    ) {
      const key = `${store.formData.workspace_id}-${store.formData.item_type_id}`;
      if (store.screenFieldsLoadedForKey !== key) {
        store.loadScreenFieldsForItemType(store.formData.workspace_id, store.formData.item_type_id);
      }
    }
  });

  // Apply stored workspace when workspaces are available
  $effect(() => {
    if (!store.formData.workspace_id && store.storedWorkspaceId && $workspacesStore.regularWorkspaces.length > 0) {
      store.applyStoredWorkspace($workspacesStore.regularWorkspaces);
    }
  });

  // Apply stored item type when available types are loaded
  $effect(() => {
    store.applyStoredItemType();
  });

  // Apply config set default item type
  $effect(() => {
    store.applyConfigSetDefault();
  });

  // Persist workspace selection
  $effect(() => {
    if (store.selectedWorkspace?.id) {
      // Persistence is handled in setWorkspace method
    }
  });

  // Persist item type selection
  $effect(() => {
    if (store.formData.item_type_id && store.formData.item_type_id !== store.lastPersistedItemTypeId) {
      store.setItemType(store.formData.item_type_id);
    }
  });

  // Auto-select first workspace if only one exists
  $effect(() => {
    if ($workspacesStore.regularWorkspaces.length === 1 && !store.formData.workspace_id) {
      store.setWorkspace($workspacesStore.regularWorkspaces[0]);
    }
  });

  // Sync selectedWorkspace when formData.workspace_id changes externally
  $effect(() => {
    if (store.formData.workspace_id && $workspacesStore.regularWorkspaces.length > 0) {
      const workspace = $workspacesStore.regularWorkspaces.find(w => w.id === store.formData.workspace_id);
      if (workspace && (!store.selectedWorkspace || store.selectedWorkspace.id !== workspace.id)) {
        store.selectedWorkspace = workspace;
      }
    }
  });
</script>

<div class="space-y-3">
  <!-- Validation Errors -->
  {#if store.validationErrors.length > 0}
    <AlertBox variant="error">
      <p class="font-medium mb-1">{t('createModal.fillRequiredFields')}</p>
      <ul class="list-disc list-inside">
        {#each store.validationErrors as error}
          <li>{error}</li>
        {/each}
      </ul>
    </AlertBox>
  {/if}

  <!-- Parent Item Info -->
  {#if store.parentItem}
    <div class="text-xs px-2 py-1.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
      {t('createModal.parent')}: {store.parentItem.title}
    </div>
  {/if}

  <!-- Title Input -->
  <input
    bind:this={nameInputRef}
    bind:value={store.formData.name}
    type="text"
    id="work-item-title"
    aria-label={t('createModal.issueTitle')}
    class="w-full text-lg font-medium border-0 outline-none bg-transparent"
    style="color: var(--ds-text);"
    placeholder={t('createModal.issueTitle')}
  />

  <!-- Description -->
  <div class="min-h-[60px]">
    <MilkdownEditor
      bind:content={store.formData.description}
      placeholder={t('createModal.addDescription')}
      compact={true}
      showToolbar={false}
      readonly={false}
      itemId={null}
    />
  </div>

  <!-- Field Chips Row -->
  <div class="flex flex-wrap items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
    <!-- Item Type Chip -->
    {#if store.availableItemTypes.length >= 1}
      <ChipPicker
        value={store.formData.item_type_id}
        items={store.availableItemTypes}
        getValue={(t) => t.id}
        getLabel={(t) => t.name}
        icon={selectedItemTypeIcon}
        placeholder={t('createModal.type')}
        onSelect={(itemType) => store.setItemType(itemType.id)}
      >
        {#snippet itemSnippet({ item })}
          {@const TypeIcon = item.icon ? itemTypeIconMap[item.icon] : Layers}
          <TypeIcon size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          <span>{item.name}</span>
        {/snippet}
      </ChipPicker>
    {/if}

    <!-- Priority Chip -->
    {#if store.isFieldConfigured('priority') && !store.isFieldRequired('priority')}
      {#if store.selectedWorkspace}
        <PriorityPicker
          workspaceId={store.selectedWorkspace.id}
          items={store.configSetPriorities}
          selectedPriorityId={store.formData.priority_id}
          onChange={(priorityId, priority) => {
            store.formData.priority_id = priorityId;
            selectedPriorityObj = priority;
          }}
          showUnassigned={true}
          unassignedLabel={t('createModal.noPriority')}
        >
          {#snippet children()}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
              style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {store.formData.priority_id ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
            >
              <Flag size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
              <span class="truncate max-w-[120px]">{selectedPriorityObj?.name || t('createModal.priority')}</span>
              <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
            </div>
          {/snippet}
        </PriorityPicker>
      {:else}
        <div
          class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm cursor-not-allowed opacity-50"
          style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text-subtle);"
        >
          <Flag size={14} style="flex-shrink: 0;" />
          <span>{t('createModal.priority')}</span>
          <ChevronDown size={12} style="flex-shrink: 0;" />
        </div>
      {/if}
    {/if}

    <!-- Assignee Chip -->
    {#if store.isFieldConfigured('assignee') && !store.isFieldRequired('assignee')}
      <UserPicker
        bind:value={store.formData.assignee_id}
        showUnassigned={true}
        unassignedLabel={t('createModal.unassigned')}
        workspaceId={store.formData.workspace_id}
      >
        {#snippet children()}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
            style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {store.formData.assignee_id ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
          >
            <User size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
            <span class="truncate max-w-[120px]">{store.selectedAssignee?.name || store.selectedAssignee?.email || t('createModal.assignee')}</span>
            <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          </div>
        {/snippet}
      </UserPicker>
    {/if}

    <!-- Due Date Chip -->
    {#if store.isFieldConfigured('due_date') && !store.isFieldRequired('due_date')}
      <button
        use:melt={$dueDateTrigger}
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {store.formData.due_date ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
      >
        <Calendar size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
        <span class="truncate max-w-[120px]">{store.formData.due_date ? formatDueDate(store.formData.due_date) : t('createModal.dueDate')}</span>
        <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      </button>

      {#if $dueDateOpen}
        <div
          use:melt={$dueDateContent}
          class="z-50 rounded-lg shadow-lg p-3"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <input
            type="date"
            bind:value={store.formData.due_date}
            aria-label={t('createModal.dueDate')}
            class="w-full px-3 py-2 rounded border text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            onchange={() => $dueDateOpen = false}
          />
        </div>
      {/if}
    {/if}

    <!-- Start Date Chip -->
    {#if store.isFieldConfigured('start_date') && !store.isFieldRequired('start_date')}
      <button
        use:melt={$startDateTrigger}
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {store.formData.start_date ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
      >
        <Calendar size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
        <span class="truncate max-w-[120px]">{store.formData.start_date ? formatDueDate(store.formData.start_date) : t('common.startDate')}</span>
        <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      </button>

      {#if $startDateOpen}
        <div
          use:melt={$startDateContent}
          class="z-50 rounded-lg shadow-lg p-3"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <input
            type="date"
            bind:value={store.formData.start_date}
            aria-label={t('common.startDate')}
            class="w-full px-3 py-2 rounded border text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            onchange={() => $startDateOpen = false}
          />
        </div>
      {/if}
    {/if}

    <!-- End Date Chip -->
    {#if store.isFieldConfigured('end_date') && !store.isFieldRequired('end_date')}
      <button
        use:melt={$endDateTrigger}
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {store.formData.end_date ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
      >
        <Calendar size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
        <span class="truncate max-w-[120px]">{store.formData.end_date ? formatDueDate(store.formData.end_date) : t('common.endDate')}</span>
        <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      </button>

      {#if $endDateOpen}
        <div
          use:melt={$endDateContent}
          class="z-50 rounded-lg shadow-lg p-3"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <input
            type="date"
            bind:value={store.formData.end_date}
            aria-label={t('common.endDate')}
            class="w-full px-3 py-2 rounded border text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            onchange={() => $endDateOpen = false}
          />
        </div>
      {/if}
    {/if}

    <!-- Milestones Chip (multi) -->
    {#if store.isFieldConfigured('milestone') && !store.isFieldRequired('milestone')}
      <MilestoneCombobox
        multiple={true}
        bind:value={store.formData.milestone_ids}
        workspaceId={store.selectedWorkspace?.id}
        placeholder={t('createModal.noMilestone')}
      >
        {#snippet children()}
          {@const selected = store.selectedMilestones}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
            style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {selected.length > 0 ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
          >
            {#if selected.length > 0 && selected[0].category_color}
              <div class="w-2 h-2 rounded-full flex-shrink-0" style="background-color: {selected[0].category_color};"></div>
            {:else}
              <MilestoneIcon size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
            {/if}
            <span class="truncate max-w-[160px]">
              {#if selected.length === 0}
                {t('createModal.milestoneField')}
              {:else if selected.length === 1}
                {selected[0].name}
              {:else}
                {selected[0].name} +{selected.length - 1}
              {/if}
            </span>
            <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          </div>
        {/snippet}
      </MilestoneCombobox>
    {/if}

    <!-- Toggle for Non-Required Custom Fields -->
    {#if store.nonRequiredCustomFields.length > 0}
      <button
        onclick={() => showAdditionalFields = !showAdditionalFields}
        class="inline-flex items-center px-2 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text-subtle);"
        onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
        onmouseout={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
        onfocus={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
        onblur={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
      >
        <MoreHorizontal size={14} />
      </button>
    {/if}
  </div>

  <!-- Additional Fields (inline, shown on toggle) -->
  {#if showAdditionalFields && store.nonRequiredCustomFields.length > 0}
    <div class="space-y-3 pt-3 border-t" style="border-color: var(--ds-border);">
      <div class="text-xs font-medium" style="color: var(--ds-text-subtle);">
        {t('createModal.additionalFields')}
      </div>
      {#each store.nonRequiredCustomFields as field}
        <CustomFieldRenderer
          {field}
          bind:value={store.customFieldValues[field.id]}
          readonly={false}
          onChange={(val) => store.customFieldValues[field.id] = val}
          milestones={store.milestones}
          isDarkMode={false}
          autoOpenPickers={false}
        />
      {/each}
    </div>
  {/if}

  <!-- Required System Fields Section -->
  {#if store.requiredSystemFields.length > 0}
    <div class="space-y-3 pt-3 border-t" style="border-color: var(--ds-border);">
      {#each store.requiredSystemFields as field}
        {#if field.field_identifier === 'priority'}
          <div class="space-y-1">
            <Label color="default">
              {t('createModal.priority')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            {#if store.selectedWorkspace}
              <PriorityPicker
                workspaceId={store.selectedWorkspace.id}
                items={store.configSetPriorities}
                selectedPriorityId={store.formData.priority_id}
                onChange={(priorityId) => store.formData.priority_id = priorityId}
                placeholder={t('createModal.noPriority')}
              />
            {:else}
              <div class="px-3 py-2 text-sm rounded border" style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text-subtle);">
                {t('createModal.selectWorkspaceFirst')}
              </div>
            {/if}
          </div>
        {:else if field.field_identifier === 'due_date'}
          <div class="space-y-1">
            <Label color="default" for="work-item-due-date-required">
              {t('createModal.dueDate')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <input
              type="date"
              id="work-item-due-date-required"
              bind:value={store.formData.due_date}
              class="w-full px-3 py-2 rounded border text-sm"
              style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            />
          </div>
        {:else if field.field_identifier === 'start_date'}
          <div class="space-y-1">
            <Label color="default" for="work-item-start-date-required">
              {t('common.startDate')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <input
              type="date"
              id="work-item-start-date-required"
              bind:value={store.formData.start_date}
              class="w-full px-3 py-2 rounded border text-sm"
              style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            />
          </div>
        {:else if field.field_identifier === 'end_date'}
          <div class="space-y-1">
            <Label color="default" for="work-item-end-date-required">
              {t('common.endDate')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <input
              type="date"
              id="work-item-end-date-required"
              bind:value={store.formData.end_date}
              class="w-full px-3 py-2 rounded border text-sm"
              style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            />
          </div>
        {:else if field.field_identifier === 'milestone'}
          <div class="space-y-1">
            <Label color="default">
              {t('createModal.milestoneField')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <MilestoneCombobox
              multiple={true}
              bind:value={store.formData.milestone_ids}
              workspaceId={store.selectedWorkspace?.id}
              placeholder={t('createModal.noMilestone')}
            />
          </div>
        {:else if field.field_identifier === 'assignee'}
          <div class="space-y-1">
            <Label color="default">
              {t('createModal.assignee')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <UserPicker
              bind:value={store.formData.assignee_id}
              placeholder={t('createModal.unassigned')}
              workspaceId={store.formData.workspace_id}
            />
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  <!-- Required Custom Fields Section -->
  {#if store.requiredCustomFields.length > 0}
    <div class="space-y-3 pt-3 border-t" style="border-color: var(--ds-border);">
      {#each store.requiredCustomFields as field}
        <div class="space-y-1">
          <Label color="default">
            {field.name} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
          </Label>
          <CustomFieldRenderer
            {field}
            bind:value={store.customFieldValues[field.id]}
            readonly={false}
            onChange={(val) => store.customFieldValues[field.id] = val}
            milestones={store.milestones}
            isDarkMode={false}
          />
        </div>
      {/each}
    </div>
  {/if}
</div>
