<script>
  import { t } from '../../stores/i18n.svelte.js';
  import { api } from '../../api.js';
  import InlineFieldEditor from '../../editors/InlineFieldEditor.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Checkbox from '../../components/Checkbox.svelte';
  import ListCustomFieldCell from './ListCustomFieldCell.svelte';
  import { Calendar, User, Target, Globe, Building2, FolderKanban } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import { formatDate } from '../../utils/dateFormatter.js';

  let {
    item,
    column,
    workspace,
    collectionId = null,
    canEdit = true,
    statuses = [],
    statusCategories = [],
    priorities = [],
    milestones = [],
    iterations = [],
    users = [],
    projects = [],
    itemTypes = [],
    customFieldDefinitions = [],
    onitemUpdated,
    onupdateError,
  } = $props();

  // Get the field definition for custom fields
  let fieldDefinition = $derived(
    column.field_type === 'custom'
      ? customFieldDefinitions.find(f => String(f.id) === column.field_identifier)
      : null
  );

  // Get custom field value from item
  function getCustomFieldValue(item, fieldIdentifier) {
    if (!item.custom_field_values) return null;
    return item.custom_field_values[fieldIdentifier] ?? null;
  }

  // Handle item updates
  async function handleItemUpdate(field, value) {
    try {
      const updatedItem = await api.items.update(item.id, { [field]: value });
      onitemUpdated?.({ item: updatedItem, field, value });
    } catch (error) {
      onupdateError?.({ error: error.message, field, value });
    }
  }

  // Handle custom field updates
  async function handleCustomFieldUpdate(fieldIdentifier, value) {
    try {
      const currentCustomValues = item.custom_field_values || {};
      const updatedItem = await api.items.update(item.id, {
        custom_field_values: {
          ...currentCustomValues,
          [fieldIdentifier]: value
        }
      });
      onitemUpdated?.({ item: updatedItem, field: fieldIdentifier, value });
    } catch (error) {
      onupdateError?.({ error: error.message, field: fieldIdentifier, value });
    }
  }

  // Handle task checkbox toggle
  async function toggleTaskStatus(isCompleted) {
    const newStatus = isCompleted ? 'completed' : 'open';
    try {
      await api.items.update(item.id, { status: newStatus });
      item.status = newStatus;
      onitemUpdated?.({ item, field: 'status', value: newStatus });
    } catch (error) {
      onupdateError?.({ error: error.message, field: 'status', value: newStatus });
    }
  }

  // Configs for pickers
  const statusConfig = {
    icon: {
      type: 'color-dot',
      source: (status) => {
        const category = statusCategories.find(sc => sc.id === status.category_id);
        return category?.color || '#6b7280';
      },
      size: 'w-2 h-2'
    },
    primary: { text: (status) => status.name },
    getValue: (status) => status.id,
    getLabel: (status) => status.name,
    searchFields: ['name']
  };

  const priorityConfig = {
    icon: {
      type: 'color-dot',
      source: (priority) => priority.color || '#6b7280',
      size: 'w-2 h-2'
    },
    primary: { text: (priority) => priority.name },
    getValue: (priority) => priority.id,
    getLabel: (priority) => priority.name,
    searchFields: ['name']
  };

  const milestoneConfig = {
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
    searchFields: ['name'],
    groupBy: null
  };

  const iterationConfig = {
    getValue: (item) => item.id,
    getLabel: (item) => item.name,
    searchFields: ['name'],
    groupBy: (item) => item.is_global ? 'Global' : 'Team'
  };

  const projectConfig = {
    getValue: (project) => project.id,
    getLabel: (project) => project.name,
    searchFields: ['name']
  };
</script>

{#if column.field_type === 'system'}
  {#if column.field_identifier === 'key'}
    <!-- Item Key -->
    <div class="flex items-center gap-2 min-w-0">
      <ItemKey
        {item}
        {workspace}
        href={collectionId
          ? `/workspaces/${workspace.id}/collections/${collectionId}/items/${item.id}`
          : `/workspaces/${workspace.id}/items/${item.id}`}
      />
    </div>

  {:else if column.field_identifier === 'title'}
    <!-- Title with type icon -->
    <div class="flex items-center gap-2 min-w-0">
      {#if item.item_type_id && itemTypes.length > 0}
        {@const itemType = itemTypes.find(type => type.id === item.item_type_id)}
        {#if itemType}
          <div
            class="w-4 h-4 rounded flex items-center justify-center text-white text-xs flex-shrink-0"
            style="background-color: {itemType.color};"
            title={itemType.name}
          >
            {@const CellTypeIcon = itemTypeIconMap[itemType.icon] || itemTypeIconMap.FileText}
            <CellTypeIcon class="w-3 h-3" />
          </div>
        {/if}
      {/if}
      <div class="flex-1 min-w-0">
        {#if canEdit}
          <InlineFieldEditor
            {item}
            field="title"
            fieldType="text"
            placeholder="Enter title..."
            required={true}
            className="font-medium"
            onitemUpdated={(detail) => onitemUpdated?.(detail)}
            onupdateError={(detail) => onupdateError?.(detail)}
          />
        {:else}
          <span class="font-medium truncate" style="color: var(--ds-text);">{item.title}</span>
        {/if}
      </div>
    </div>

  {:else if column.field_identifier === 'status'}
    <!-- Status / Task Checkbox -->
    {#if item.is_task}
      <Checkbox
        checked={item.status === 'completed'}
        onchange={(checked) => toggleTaskStatus(checked)}
        label={item.status === 'completed' ? 'Done' : 'Todo'}
        size="small"
        disabled={!canEdit}
      />
    {:else}
      {@const selectedStatus = statuses.find(s => s.id === item.status_id)}
      {@const statusCategory = selectedStatus ? statusCategories.find(sc => sc.id === selectedStatus.category_id) : null}
      {#if canEdit}
        <ItemPicker
          value={item.status_id}
          items={statuses}
          config={statusConfig}
          placeholder="Set status"
          showUnassigned={false}
          allowClear={false}
          onSelect={async (selected) => {
            const statusId = selected?.id;
            if (statusId && statusId !== item.status_id) {
              await handleItemUpdate('status_id', statusId);
            }
          }}
        >
          {#snippet children()}
            <span class="cursor-pointer">
              <Lozenge
                text={selectedStatus ? selectedStatus.name : 'Set status'}
                customBg={statusCategory?.color || '#6b7280'}
              />
            </span>
          {/snippet}
        </ItemPicker>
      {:else}
        <Lozenge
          text={selectedStatus ? selectedStatus.name : '-'}
          customBg={statusCategory?.color || '#6b7280'}
        />
      {/if}
    {/if}

  {:else if column.field_identifier === 'priority'}
    <!-- Priority -->
    {@const selectedPriority = priorities.find(p => p.id === item.priority_id)}
    {#if canEdit}
      <ItemPicker
        value={item.priority_id}
        items={priorities}
        config={priorityConfig}
        placeholder="Select priority"
        showUnassigned={true}
        unassignedLabel="No priority"
        allowClear={true}
        onSelect={async (selected) => {
          const priorityId = selected?.id || null;
          await handleItemUpdate('priority_id', priorityId);
        }}
      >
        {#snippet children()}
          {#if selectedPriority}
            <span
              class="w-full flex items-center justify-start gap-2 text-sm text-left cursor-pointer"
              style="color: {selectedPriority.color || 'var(--ds-text-subtle)'};"
            >
              <ColorDot color={selectedPriority.color} />
              {selectedPriority.name}
            </span>
          {:else}
            <span
              class="w-full flex items-center justify-start gap-2 text-sm text-left cursor-pointer"
              style="color: var(--ds-text-subtle);"
            >
              {t('pickers.selectPriority')}
            </span>
          {/if}
        {/snippet}
      </ItemPicker>
    {:else}
      {#if selectedPriority}
        <span class="flex items-center gap-2 text-sm" style="color: {selectedPriority.color || 'var(--ds-text-subtle)'};">
          <ColorDot color={selectedPriority.color} />
          {selectedPriority.name}
        </span>
      {:else}
        <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
      {/if}
    {/if}

  {:else if column.field_identifier === 'assignee'}
    <!-- Assignee -->
    {@const assignee = users.find(u => u.id === item.assignee_id)}
    {#if canEdit}
      <UserPicker
        value={item.assignee_id}
        placeholder="Assign"
        showUnassigned={true}
        onSelect={async (selectedUser) => {
          const userId = selectedUser?.id || null;
          await handleItemUpdate('assignee_id', userId);
        }}
      >
        {#snippet children()}
          {#if assignee}
            <div class="flex items-center gap-2 cursor-pointer">
              <div class="w-5 h-5 rounded-full bg-blue-500 flex items-center justify-center text-white text-[10px] font-medium">
                {(assignee.first_name?.[0] || '') + (assignee.last_name?.[0] || '') || assignee.username?.[0]?.toUpperCase() || '?'}
              </div>
              <span class="text-sm truncate" style="color: var(--ds-text);">
                {assignee.first_name} {assignee.last_name}
              </span>
            </div>
          {:else}
            <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text-subtle);">
              <User class="w-4 h-4" />
              {t('pickers.assignee')}
            </span>
          {/if}
        {/snippet}
      </UserPicker>
    {:else}
      {#if assignee}
        <div class="flex items-center gap-2">
          <div class="w-5 h-5 rounded-full bg-blue-500 flex items-center justify-center text-white text-[10px] font-medium">
            {(assignee.first_name?.[0] || '') + (assignee.last_name?.[0] || '') || assignee.username?.[0]?.toUpperCase() || '?'}
          </div>
          <span class="text-sm truncate" style="color: var(--ds-text);">
            {assignee.first_name} {assignee.last_name}
          </span>
        </div>
      {:else}
        <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
      {/if}
    {/if}

  {:else if column.field_identifier === 'milestone'}
    <!-- Milestone -->
    {@const milestone = milestones.find(m => m.id === item.milestone_id)}
    {#if canEdit}
      <ItemPicker
        value={item.milestone_id}
        items={milestones}
        config={milestoneConfig}
        placeholder="Set milestone"
        showUnassigned={true}
        unassignedLabel="No milestone"
        allowClear={true}
        onSelect={async (selected) => {
          const milestoneId = selected?.id || null;
          await handleItemUpdate('milestone_id', milestoneId);
        }}
      >
        {#snippet children()}
          {#if milestone}
            <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
              <ColorDot color={milestone.category_color || '#9CA3AF'} />
              {milestone.name}
            </span>
          {:else}
            <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text-subtle);">
              <Target class="w-4 h-4" />
              {t('pickers.selectMilestone')}
            </span>
          {/if}
        {/snippet}
      </ItemPicker>
    {:else}
      {#if milestone}
        <span class="flex items-center gap-2 text-sm" style="color: var(--ds-text);">
          <ColorDot color={milestone.category_color || '#9CA3AF'} />
          {milestone.name}
        </span>
      {:else}
        <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
      {/if}
    {/if}

  {:else if column.field_identifier === 'iteration'}
    <!-- Iteration -->
    {@const iteration = iterations.find(i => i.id === item.iteration_id)}
    {#if canEdit}
      <ItemPicker
        value={item.iteration_id}
        items={iterations}
        config={iterationConfig}
        placeholder="Set iteration"
        showUnassigned={true}
        unassignedLabel="No iteration"
        allowClear={true}
        onSelect={async (selected) => {
          const iterationId = selected?.id || null;
          await handleItemUpdate('iteration_id', iterationId);
        }}
      >
        {#snippet children()}
          {#if iteration}
            <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
              {#if iteration.is_global}
                <Globe class="w-4 h-4" style="color: var(--ds-text-subtle);" />
              {:else}
                <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
              {/if}
              {iteration.name}
            </span>
          {:else}
            <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text-subtle);">
              <Calendar class="w-4 h-4" />
              {t('items.selectIteration')}
            </span>
          {/if}
        {/snippet}
      </ItemPicker>
    {:else}
      {#if iteration}
        <span class="flex items-center gap-2 text-sm" style="color: var(--ds-text);">
          {#if iteration.is_global}
            <Globe class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          {:else}
            <Building2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          {/if}
          {iteration.name}
        </span>
      {:else}
        <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
      {/if}
    {/if}

  {:else if column.field_identifier === 'due_date'}
    <!-- Due Date -->
    {#if canEdit}
      <InlineFieldEditor
        {item}
        field="due_date"
        fieldType="date"
        placeholder="Set due date"
        onitemUpdated={(detail) => onitemUpdated?.(detail)}
        onupdateError={(detail) => onupdateError?.(detail)}
      />
    {:else}
      <div class="flex items-center gap-1 text-sm whitespace-nowrap" style="color: var(--ds-text-subtle);">
        <Calendar class="w-4 h-4 flex-shrink-0" />
        {item.due_date ? formatDate(item.due_date) : '-'}
      </div>
    {/if}

  {:else if column.field_identifier === 'created_at'}
    <!-- Created Date (always read-only) -->
    <div class="flex items-center gap-1 text-sm whitespace-nowrap" style="color: var(--ds-text-subtle);">
      <Calendar class="w-4 h-4 flex-shrink-0" />
      {formatDate(item.created_at) || '-'}
    </div>

  {:else if column.field_identifier === 'project'}
    <!-- Project -->
    {@const project = projects.find(p => p.id === item.project_id)}
    {#if canEdit}
      <ItemPicker
        value={item.project_id}
        items={projects}
        config={projectConfig}
        placeholder="Set project"
        showUnassigned={true}
        unassignedLabel="No project"
        allowClear={true}
        onSelect={async (selected) => {
          const projectId = selected?.id || null;
          await handleItemUpdate('project_id', projectId);
        }}
      >
        {#snippet children()}
          {#if project}
            <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text);">
              <FolderKanban class="w-4 h-4" style="color: var(--ds-text-subtle);" />
              {project.name}
            </span>
          {:else}
            <span class="flex items-center gap-2 text-sm cursor-pointer" style="color: var(--ds-text-subtle);">
              <FolderKanban class="w-4 h-4" />
              {t('pickers.selectProject')}
            </span>
          {/if}
        {/snippet}
      </ItemPicker>
    {:else}
      {#if project}
        <span class="flex items-center gap-2 text-sm" style="color: var(--ds-text);">
          <FolderKanban class="w-4 h-4" style="color: var(--ds-text-subtle);" />
          {project.name}
        </span>
      {:else}
        <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
      {/if}
    {/if}

  {:else}
    <!-- Unknown system field -->
    <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
  {/if}

{:else if column.field_type === 'custom' && fieldDefinition}
  <!-- Custom Field -->
  {@const customValue = getCustomFieldValue(item, column.field_identifier)}
  <ListCustomFieldCell
    field={fieldDefinition}
    value={customValue}
    {canEdit}
    {milestones}
    {iterations}
    {users}
    onChange={(newValue) => handleCustomFieldUpdate(column.field_identifier, newValue)}
  />
{:else}
  <!-- Unknown field type or missing definition -->
  <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
{/if}

