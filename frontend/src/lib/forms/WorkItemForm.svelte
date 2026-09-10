<script>
  import { MoreHorizontal, Calendar, Flag, User, Layers, ChevronDown, FileText, Briefcase, Hash, Clock } from '@lucide/svelte';
  import ItemTypeIcon from '../components/ItemTypeIcon.svelte';
  import { workItemFormStore } from '../stores/workItemFormStore.svelte.js';
  import { workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateOnly, formatDueDate } from '../utils/dateFormatter.js';
  import MilkdownEditor from '../editors/LazyMilkdownEditor.svelte';
  import ChipPicker from '../pickers/ChipPicker.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import PriorityPicker from '../pickers/PriorityPicker.svelte';
  import MilestoneCombobox from '../pickers/MilestoneCombobox.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import WorkspaceLabelCombobox from '../pickers/WorkspaceLabelCombobox.svelte';
  import Label from '../components/Label.svelte';
  import Input from '../components/Input.svelte';
  import NativeSelect from '../components/NativeSelect.svelte';
  import AlertBox from '../components/AlertBox.svelte';
  import { createPopover, melt } from '@melt-ui/svelte'; // used for dueDateTrigger/dueDateContent
  import { Milestone as MilestoneIcon } from '@lucide/svelte';

  let {
    nameInputRef = $bindable(null),
    formStore = workItemFormStore,
  } = $props();

  // Use the store
  let store = $derived(formStore);

  // Track priority object for display (not persisted)
  let selectedPriorityObj = $state(null);

  // Derived state for UI
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

  // Optional dates are presented as chips. Open the browser's native calendar
  // as soon as the chip mounts its input so setting a date remains a one-click
  // interaction. The focused, visible input is the fallback when showPicker is
  // unavailable or the browser declines the programmatic picker request.
  function focusAndShowDatePicker(node) {
    node.focus();
    try {
      node.showPicker?.();
    } catch {
      // Keep the focused input available for manual entry.
    }
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
  <Input
    bind:inputRef={nameInputRef}
    bind:value={store.formData.name}
    type="text"
    id="work-item-title"
    ariaLabel={t('createModal.issueTitle')}
    variant="ghost"
    class="w-full text-lg font-medium border-0 outline-none bg-transparent"
    style="color: var(--ds-text);"
    placeholder={t('createModal.issueTitle')}
  />

  <!-- Description. Keyed on templateApplyNonce so applying a template body
       (selectable pick or mandatory auto-apply) re-mounts the editor with the
       new content — the editable Milkdown editor does not sync external
       content changes otherwise (WI-438). -->
  <div class="min-h-[60px]">
    {#key store.templateApplyNonce}
      <MilkdownEditor
        bind:content={store.formData.description}
        placeholder={t('createModal.addDescription')}
        compact={true}
        showToolbar={false}
        readonly={false}
        itemId={null}
        deferImageUploads={true}
        onDeferredImageUpload={(image) => store.addPendingDescriptionImage(image)}
      />
    {/key}
  </div>

  <!-- Field Chips Row -->
  <div class="flex flex-wrap items-center gap-2 pt-2 border-t" style="border-color: var(--ds-border);">
    <!-- Item Type Chip -->
    {#if store.availableItemTypes.length >= 1}
      <!-- Keep the picker inert until the workspace's config set resolves:
           applying its item-type restrictions late re-renders the option list
           (and can switch the selection), which hijacks an in-flight click
           onto the wrong option. -->
      <ChipPicker
        value={store.formData.item_type_id}
        items={store.availableItemTypes}
        getValue={(t) => t.id}
        getLabel={(t) => t.name}
        icon={Layers}
        placeholder={t('createModal.type')}
        testId="create-item-type-chip"
        disabled={!!store.formData.workspace_id && store.configSetLoadedForWorkspace !== store.formData.workspace_id}
        onSelect={(itemType) => store.setItemType(itemType.id)}
      >
        {#snippet triggerSnippet({ item })}
          <ItemTypeIcon itemType={item} />
        {/snippet}
        {#snippet itemSnippet({ item })}
          <ItemTypeIcon itemType={item} />
          <span>{item.name}</span>
        {/snippet}
      </ChipPicker>
    {/if}

    <!-- Template Chip (WI-438). When the selected type enforces a mandatory
         template the picker is locked (body already auto-applied); otherwise it
         offers the selectable templates valid for the type plus globals. -->
    {#if store.templateLocked}
      <span
        class="inline-flex items-center gap-1.5 px-2 py-1 rounded text-sm"
        style="color: var(--ds-text-subtle); background-color: var(--ds-background-neutral); opacity: 0.8;"
        title={`This item type enforces the "${store.mandatoryTemplate?.name}" template`}
        data-testid="template-picker-locked"
      >
        <FileText size={14} style="flex-shrink: 0;" />
        <span>{store.mandatoryTemplate?.name} (enforced)</span>
      </span>
    {:else if store.templateOptions.length >= 1}
      <ChipPicker
        value={store.selectedTemplateId}
        items={store.templateOptions}
        getValue={(tmpl) => tmpl.id}
        getLabel={(tmpl) => tmpl.name}
        icon={FileText}
        placeholder={t('createModal.template')}
        testId="template-picker"
        onSelect={(tmpl) => store.applyTemplate(tmpl.id)}
      />
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
            <!-- svelte-ignore a11y_no_static_element_interactions a11y_no_noninteractive_element_to_interactive_role -->
            <div
              role="button"
              tabindex="0"
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
          <!-- svelte-ignore a11y_no_static_element_interactions a11y_no_noninteractive_element_to_interactive_role -->
          <div
            role="button"
            tabindex="0"
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
        data-testid="create-due-date-chip"
        data-value={store.formData.due_date}
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
          class="z-[70] rounded-lg shadow-lg p-3"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <input
            type="date"
            data-testid="create-due-date-input"
            bind:value={store.formData.due_date}
            use:focusAndShowDatePicker
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
        data-testid="create-start-date-chip"
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {store.formData.start_date ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
      >
        <Calendar size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
        <span class="truncate max-w-[120px]">{store.formData.start_date ? formatDateOnly(store.formData.start_date) : t('common.startDate')}</span>
        <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      </button>

      {#if $startDateOpen}
        <div
          use:melt={$startDateContent}
          class="z-[70] rounded-lg shadow-lg p-3"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <input
            type="date"
            bind:value={store.formData.start_date}
            use:focusAndShowDatePicker
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
        data-testid="create-end-date-chip"
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {store.formData.end_date ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
      >
        <Calendar size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
        <span class="truncate max-w-[120px]">{store.formData.end_date ? formatDateOnly(store.formData.end_date) : t('common.endDate')}</span>
        <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      </button>

      {#if $endDateOpen}
        <div
          use:melt={$endDateContent}
          class="z-[70] rounded-lg shadow-lg p-3"
          style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
        >
          <input
            type="date"
            bind:value={store.formData.end_date}
            use:focusAndShowDatePicker
            aria-label={t('common.endDate')}
            class="w-full px-3 py-2 rounded border text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
            onchange={() => $endDateOpen = false}
          />
        </div>
      {/if}
    {/if}

    <!-- Iteration Chip -->
    {#if store.isFieldConfigured('iteration') && !store.isFieldRequired('iteration')}
      <ChipPicker
        value={store.formData.iteration_id}
        items={[{ id: null, name: t('common.none') }, ...store.iterations]}
        getValue={(iteration) => iteration.id}
        getLabel={(iteration) => iteration.name}
        icon={Calendar}
        placeholder={t('items.iteration')}
        searchable={true}
        searchFields={['name', 'description']}
        onSelect={(iteration) => store.formData.iteration_id = iteration?.id || null}
      />
    {/if}

    <!-- Project Chip -->
    {#if store.isFieldConfigured('project') && !store.isFieldRequired('project')}
      <ChipPicker
        value={store.formData.project_id}
        items={[{ id: null, name: t('common.none') }, ...store.timeProjects]}
        getValue={(project) => project.id}
        getLabel={(project) => project.name}
        icon={Briefcase}
        placeholder={t('items.project')}
        searchable={true}
        searchFields={['name']}
        onSelect={(project) => store.formData.project_id = project?.id || null}
      />
    {/if}

    <!-- Story Points Chip -->
    {#if store.isFieldConfigured('story_points') && !store.isFieldRequired('story_points')}
      <label
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text-subtle);"
      >
        <Hash size={14} style="flex-shrink: 0;" />
        <Input
          type="number"
          min="0"
          step="0.5"
          bind:value={store.formData.story_points}
          variant="ghost"
          class="w-20 bg-transparent outline-none text-sm"
          style="color: var(--ds-text);"
          placeholder={t('items.storyPoints')}
          ariaLabel={t('items.storyPoints')}
        />
      </label>
    {/if}

    <!-- Estimate Chip -->
    {#if store.isFieldConfigured('estimate') && !store.isFieldRequired('estimate')}
      <label
        class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
        style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text-subtle);"
      >
        <Clock size={14} style="flex-shrink: 0;" />
        <Input
          type="text"
          bind:value={store.formData.estimate}
          variant="ghost"
          class="w-20 bg-transparent outline-none text-sm"
          style="color: var(--ds-text);"
          placeholder={t('items.estimate') || 'Estimate'}
          ariaLabel={t('items.estimate') || 'Estimate'}
        />
      </label>
    {/if}

    <!-- Milestones Chip (multi) -->
    {#if store.isFieldConfigured('milestone') && !store.isFieldRequired('milestone')}
      <MilestoneCombobox
        multiple={true}
        bind:value={store.formData.milestone_ids}
        workspaceId={store.selectedWorkspace?.id}
        milestones={store.milestones}
        loading={store.milestonesLoading}
        placeholder={t('createModal.noMilestone')}
      >
        {#snippet children()}
          {@const selectedIds = Array.isArray(store.formData.milestone_ids) ? store.formData.milestone_ids : []}
          {@const selected = store.selectedMilestones}
          <!-- svelte-ignore a11y_no_static_element_interactions a11y_no_noninteractive_element_to_interactive_role -->
          <div
            role="button"
            tabindex="0"
            data-testid="create-milestone-chip"
            class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
            style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: {selectedIds.length > 0 ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
          >
            {#if selectedIds.length === 1 && selected[0]?.category_color}
              <div class="w-2 h-2 rounded-full flex-shrink-0" style="background-color: {selected[0].category_color};"></div>
            {:else}
              <MilestoneIcon size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
            {/if}
            <span class="truncate max-w-[160px]">
              {#if selectedIds.length === 0}
                {t('createModal.milestoneField')}
              {:else if selectedIds.length === 1 && selected[0]?.name}
                {selected[0].name}
              {:else}
                {t('pickers.milestonesSelected', { count: selectedIds.length })}
              {/if}
            </span>
            <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          </div>
        {/snippet}
      </MilestoneCombobox>
    {/if}

    <!-- Toggle for additional non-required full-width fields -->
    {#if store.nonRequiredFullSystemFields.length > 0 || store.nonRequiredCustomFields.length > 0}
      <button
        onclick={() => showAdditionalFields = !showAdditionalFields}
        aria-label={t('createModal.additionalFields')}
        data-testid="create-additional-fields-toggle"
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
  {#if showAdditionalFields && (store.nonRequiredFullSystemFields.length > 0 || store.nonRequiredCustomFields.length > 0)}
    <div class="space-y-3 pt-3 border-t" style="border-color: var(--ds-border);">
      <div class="text-xs font-medium" style="color: var(--ds-text-subtle);">
        {t('createModal.additionalFields')}
      </div>
      {#each store.nonRequiredFullSystemFields as field}
        {#if field.field_identifier === 'labels'}
          <div class="space-y-1">
            <Label color="default">{t('items.labels') || 'Labels'}</Label>
            <WorkspaceLabelCombobox
              id="create-label-picker-input"
              testidPrefix="create-label-picker"
              workspaceId={store.formData.workspace_id}
              bind:value={store.formData.label_names}
              placeholder={t('items.selectOrCreateLabels') || 'Select or create labels...'}
              onSelect={(result) => {
                store.formData.label_names = result?.value || [];
                store.selectedLabels = result?.labels || [];
              }}
            />
          </div>
        {/if}
      {/each}
      {#each store.nonRequiredCustomFields as field}
        <div class="space-y-1">
          <Label color="default">{field.name}</Label>
          <CustomFieldRenderer
            {field}
            bind:value={store.customFieldValues[field.id]}
            readonly={false}
            onChange={(val) => store.customFieldValues[field.id] = val}
            milestones={store.milestones}
            iterations={store.iterations}
            isDarkMode={false}
            autoOpenPickers={false}
          />
        </div>
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
            <Input
              type="date"
              id="work-item-due-date-required"
              bind:value={store.formData.due_date}
              size="medium"
            />
          </div>
        {:else if field.field_identifier === 'start_date'}
          <div class="space-y-1">
            <Label color="default" for="work-item-start-date-required">
              {t('common.startDate')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <Input
              type="date"
              id="work-item-start-date-required"
              bind:value={store.formData.start_date}
              size="medium"
            />
          </div>
        {:else if field.field_identifier === 'end_date'}
          <div class="space-y-1">
            <Label color="default" for="work-item-end-date-required">
              {t('common.endDate')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <Input
              type="date"
              id="work-item-end-date-required"
              bind:value={store.formData.end_date}
              size="medium"
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
              milestones={store.milestones}
              loading={store.milestonesLoading}
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
        {:else if field.field_identifier === 'iteration'}
          <div class="space-y-1">
            <Label color="default" for="work-item-iteration-required">
              {t('items.iteration')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <NativeSelect
              id="work-item-iteration-required"
              value={store.formData.iteration_id == null ? '' : String(store.formData.iteration_id)}
              options={store.iterations.map((iteration) => ({ value: String(iteration.id), label: iteration.name }))}
              placeholder={t('items.selectIteration')}
              onchange={(value) => store.formData.iteration_id = value ? Number(value) : null}
            />
          </div>
        {:else if field.field_identifier === 'project'}
          <div class="space-y-1">
            <Label color="default" for="work-item-project-required">
              {t('items.project')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <NativeSelect
              id="work-item-project-required"
              value={store.formData.project_id == null ? '' : String(store.formData.project_id)}
              options={store.timeProjects.map((project) => ({ value: String(project.id), label: project.name }))}
              placeholder={t('pickers.selectProject')}
              onchange={(value) => store.formData.project_id = value ? Number(value) : null}
            />
          </div>
        {:else if field.field_identifier === 'labels'}
          <div class="space-y-1">
            <Label color="default">
              {t('items.labels') || 'Labels'} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <WorkspaceLabelCombobox
              id="create-label-picker-input"
              testidPrefix="create-label-picker"
              workspaceId={store.formData.workspace_id}
              bind:value={store.formData.label_names}
              placeholder={t('items.selectOrCreateLabels') || 'Select or create labels...'}
              onSelect={(result) => {
                store.formData.label_names = result?.value || [];
                store.selectedLabels = result?.labels || [];
              }}
            />
          </div>
        {:else if field.field_identifier === 'story_points'}
          <div class="space-y-1">
            <Label color="default" for="work-item-story-points-required">
              {t('items.storyPoints')} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <Input
              id="work-item-story-points-required"
              type="number"
              min="0"
              step="0.5"
              bind:value={store.formData.story_points}
              size="medium"
            />
          </div>
        {:else if field.field_identifier === 'estimate' || field.field_identifier === 'estimate_minutes'}
          <div class="space-y-1">
            <Label color="default" for="work-item-estimate-required">
              {t('items.estimate') || 'Estimate'} <span style="color: var(--ds-text-danger, #ef4444);">*</span>
            </Label>
            <Input
              id="work-item-estimate-required"
              type="text"
              bind:value={store.formData.estimate}
              placeholder="3d 4h"
              size="medium"
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
            iterations={store.iterations}
            isDarkMode={false}
          />
        </div>
      {/each}
    </div>
  {/if}
</div>
