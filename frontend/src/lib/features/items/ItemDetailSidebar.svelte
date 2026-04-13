<script>
  import { AlertCircle, ChevronDown, MoreHorizontal, TrendingUpDown, ChevronsUp, Briefcase, Calendar, Globe, Building2, Repeat } from 'lucide-svelte';
  import { rruleToText } from '../../editors/rruleUtils.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import Avatar from '../../components/Avatar.svelte';
  import Text from '../../components/Text.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import CustomFieldRenderer from '../items/CustomFieldRenderer.svelte';
  import PersonalTasksPanel from '../personal/PersonalTasksPanel.svelte';
  import ItemSCMLinks from './ItemSCMLinks.svelte';
  import AddSCMLinkModal from '../../dialogs/AddSCMLinkModal.svelte';
  import CreateBranchModal from '../../dialogs/CreateBranchModal.svelte';
  import CreatePRFromBranchModal from '../../dialogs/CreatePRFromBranchModal.svelte';
  import { getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import { workspacePermissions } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateShort } from '../../utils/dateFormatter.js';
  import StatusBadge from '../../components/StatusBadge.svelte';

  // Click outside action
  function clickOutside(node) {
    const handleClick = (event) => {
      if (!node.contains(event.target)) {
        node.dispatchEvent(new CustomEvent('clickOutside'));
      }
    };
    
    document.addEventListener('click', handleClick, true);
    
    return {
      destroy() {
        document.removeEventListener('click', handleClick, true);
      }
    };
  }
  
  // Helper: Get status color for iterations
  function getIterationStatusColor(status) {
    switch (status) {
      case 'active': return '#0052CC';
      case 'completed': return '#00875A';
      case 'cancelled': return '#6B778C';
      case 'planned': return '#5243AA';
      default: return '#6B778C';
    }
  }

  // Helper: Capitalize first letter
  function capitalize(str) {
    return str ? str.charAt(0).toUpperCase() + str.slice(1) : '';
  }

  // Iteration picker configuration
  const iterationConfig = {
    icon: {
      type: 'component',
      source: (item) => item.is_global ? Globe : Building2
    },
    primary: {
      text: (item) => item.name
    },
    badges: [
      {
        text: (item) => item.is_global ? 'Global' : 'Workspace',
        bgColor: () => 'var(--ds-background-neutral)',
        textColor: () => 'var(--ds-text-subtle)'
      }
    ],
    metadata: [
      {
        type: 'date-range',
        icon: Calendar,
        startDate: (item) => item.start_date,
        endDate: (item) => item.end_date
      },
      {
        type: 'badge',
        text: (item) => item.status ? capitalize(item.status) : '',
        bgColor: (item) => item.status ? getIterationStatusColor(item.status) + '15' : 'transparent',
        textColor: (item) => item.status ? getIterationStatusColor(item.status) : 'var(--ds-text)'
      }
    ],
    searchFields: ['name', 'description'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name
  };

  // Priority picker configuration
  const priorityConfig = {
    icon: {
      type: 'component',
      source: (item) => {
        // Map icon name string to Lucide component
        const iconMap = {
          AlertCircle, ChevronsUp, TrendingUpDown
        };
        return iconMap[item.icon] || AlertCircle;
      }
    },
    primary: {
      text: (item) => item.name
    },
    searchFields: ['name'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name
  };

  // Status picker configuration
  const statusConfig = {
    icon: {
      type: 'color-dot',
      source: (item) => item.categoryColor || '#9CA3AF',
      size: 'w-2 h-2'
    },
    primary: {
      text: (item) => item.label
    },
    searchFields: ['label', 'value'],
    getValue: (item) => item.id,
    getLabel: (item) => item.label
  };

  // Project picker configuration
  const projectConfig = {
    icon: {
      type: 'component',
      source: () => Briefcase
    },
    primary: {
      text: (item) => item.name
    },
    searchFields: ['name'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name
  };

  // Props
  let {
    item,
    workspace = null,
    statusOptions = [],
    editingStatus = false,
    editingDueDate = false,
    editingStartDate = false,
    editingEndDate = false,
    editingCustomFields = {},
    editCustomFieldValues = {},
    workspaceScreenFields = [],
    workspaceScreenSystemFields = [],
    customFieldDefinitions = [],
    milestones = [],
    iterations = [],
    priorities = [],
    timeProjects = [],
    moduleSettings = {},
    dropdownItems = [],
    onsaveField = null,
    oncancelEdit = null,
    onstartEditingCustomField = null,
    onstartEditingAssignee = null,
    onstartEditingMilestone = null,
    onstartEditingPriority = null,
    onstartEditingDueDate = null,
    onstartEditingStartDate = null,
    onstartEditingEndDate = null,
    onstartEditingStatus = null,
    onstartEditingProject = null,
    onstartEditingIteration = null,
    recurrenceRule = null,
    onsetupRecurrence = null,
    oneditRecurrence = null,
  } = $props();

  // State for SCM Link modals
  let showAddSCMLinkModal = $state(false);
  let showCreateBranchModal = $state(false);
  let showCreatePRFromBranchModal = $state(false);
  let selectedBranchLink = $state(null);
  let scmLinksRef = $state(null);

  // Scheduling section collapse state
  const SCHEDULING_COLLAPSED_KEY = 'windshift-scheduling-collapsed';
  let schedulingUserPref = $state(
    typeof localStorage !== 'undefined'
      ? localStorage.getItem(SCHEDULING_COLLAPSED_KEY) === 'true'
      : false
  );
  let schedulingExpanded = $derived(
    (item?.due_date || item?.end_date) ? true : schedulingUserPref
  );
  let schedulingForcedOpen = $derived(!!(item?.due_date || item?.end_date));

  function toggleScheduling() {
    if (schedulingForcedOpen) return;
    schedulingUserPref = !schedulingUserPref;
    localStorage.setItem(SCHEDULING_COLLAPSED_KEY, String(schedulingUserPref));
  }

  // Story points inline editing
  let editingStoryPoints = $state(false);
  let storyPointsEditValue = $state('');

  function saveStoryPoints() {
    editingStoryPoints = false;
    const raw = storyPointsEditValue;
    const parsed = raw === '' || raw == null ? null : parseFloat(raw);
    const value = parsed != null && !Number.isNaN(parsed) && parsed >= 0 ? parsed : null;
    if (value === (item?.story_points ?? null)) return;
    onsaveField?.({ field: 'story_points', value });
  }

  // Computed item key for SCM operations
  const itemKey = $derived(
    workspace?.key && item?.workspace_item_number
      ? `${workspace.key}-${item.workspace_item_number}`
      : null
  );

  // Permission-based editability
  const canEdit = $derived.by(() => {
    const wsId = workspace?.id || item?.workspace_id;
    return wsId ? workspacePermissions.canEdit(wsId) : false;
  });

  function getCustomFieldDefinition(fieldId) {
    return customFieldDefinitions.find(field => field.id === parseInt(fieldId));
  }

  function startEditingCustomField(fieldId) {
    if (!canEdit) return;
    onstartEditingCustomField?.({ fieldId });
  }

  function startEditingAssignee() {
    if (!canEdit) return;
    onstartEditingAssignee?.();
  }

  // Milestone helpers
  function startEditingMilestone() {
    if (!canEdit) return;
    onstartEditingMilestone?.();
  }

  // Priority helpers
  function startEditingPriority() {
    if (!canEdit) return;
    onstartEditingPriority?.();
  }

  let selectedPriority = $derived(
    item?.priority_id && priorities
      ? priorities.find(p => p.id === item.priority_id)
      : null
  );

  // Due Date helpers
  function startEditingDueDate() {
    if (!canEdit) return;
    onstartEditingDueDate?.();
  }

  // Start Date helpers
  function startEditingStartDate() {
    if (!canEdit) return;
    onstartEditingStartDate?.();
  }

  // End Date helpers
  function startEditingEndDate() {
    if (!canEdit) return;
    onstartEditingEndDate?.();
  }

  // Svelte action to focus and show date picker
  function focusAndShowPicker(node) {
    node.focus();
    // Use setTimeout to ensure the focus has taken effect
    setTimeout(() => {
      try {
        node.showPicker();
      } catch (e) {
        // showPicker() may not be supported in all browsers
      }
    }, 0);
  }

  // Helper to check if a system field should be shown
  function shouldShowSystemField(fieldName) {
    // If no system fields are configured, show all fields (default behavior)
    if (!workspaceScreenSystemFields || workspaceScreenSystemFields.length === 0) {
      return true;
    }
    // Otherwise, only show if the field is in the configured list
    return workspaceScreenSystemFields.includes(fieldName);
  }

  // Status helpers
  function startEditingStatus() {
    if (!canEdit) return;
    onstartEditingStatus?.();
  }

  let selectedStatus = $derived(
    item?.status_id && statusOptions
      ? statusOptions.find(s => s.id === item.status_id)
      : null
  );

  // Project helpers
  function startEditingProject() {
    if (!canEdit) return;
    onstartEditingProject?.();
  }

  // Create merged project items array with special items
  let projectItems = $derived.by(() => {
    const items = [];

    // Add "None" special item
    items.push({
      id: 'none',
      name: 'None',
      isSpecial: true,
      specialType: 'none'
    });

    // Add "Inherit" special item if item has a parent
    if (item?.parent_id) {
      items.push({
        id: 'inherit',
        name: getInheritLabel(item),
        isSpecial: true,
        specialType: 'inherit'
      });
    }

    // Add actual projects
    items.push(...timeProjects);

    return items;
  });

  // Get selected project (handling special cases)
  let selectedProject = $derived.by(() => {
    if (item?.inherit_project) {
      return {
        id: 'inherit',
        name: getInheritLabel(item),
        isSpecial: true,
        specialType: 'inherit'
      };
    } else if (item?.project_id === null || item?.project_id === undefined) {
      return {
        id: 'none',
        name: 'None',
        isSpecial: true,
        specialType: 'none'
      };
    } else if (item?.project_id && timeProjects) {
      return timeProjects.find(p => p.id === item.project_id) || null;
    }
    return null;
  });

  // Iteration helpers
  function startEditingIteration() {
    if (!canEdit) return;
    onstartEditingIteration?.();
  }

  let selectedIteration = $derived(
    item?.iteration_id && iterations
      ? iterations.find(i => i.id === item.iteration_id)
      : null
  );

  // Project display helpers
  function getProjectDisplayText(item) {
    if (item.inherit_project) {
      // Inheriting
      return item.effective_project_name
        ? `${item.effective_project_name} (inherited)`
        : 'Inherit';
    }
    if (item.project_id === null || item.project_id === undefined) {
      // None
      return 'Project: None';
    }
    // Direct assignment
    return item.project_name || 'Set project';
  }

  function getInheritLabel(item) {
    if (item.effective_project_name) {
      return `Inherit (${item.effective_project_name})`;
    }
    return 'Inherit';
  }

  function handleClickOutsideSidebar() {
    // Cancel all custom field editing if any are active
    Object.keys(editingCustomFields).forEach(fieldId => {
      if (editingCustomFields[fieldId]) {
        oncancelEdit?.({ field: `custom_field_${fieldId}` });
      }
    });
  }
</script>

<!-- Linear-style Right Panel -->
<div
  class="h-full border-l flex flex-col"
  style="background-color: var(--ds-surface); border-color: var(--ds-border);"
  use:clickOutside
  onclickOutside={handleClickOutsideSidebar}
>
  <!-- Panel Content -->
  <div class="flex-1 px-4 py-4 overflow-y-auto">
    <!-- DETAILS Section Header -->
    <div class="flex items-center justify-between mb-4">
      <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">{t('common.details')}</Text>
      <div class="flex items-center gap-1">
        <DropdownMenu
          triggerText=""
          triggerIcon={MoreHorizontal}
          triggerClass="flex items-center justify-center p-1.5 rounded-md transition-colors"
          triggerStyle="color: var(--ds-text-subtle);"
          items={dropdownItems}
          align="right"
        />
      </div>
    </div>
    <!-- Status Field -->
    {#if shouldShowSystemField('status')}
    <div class="mb-3">
      <ItemPicker
        value={item?.status_id ?? null}
        items={statusOptions}
        config={statusConfig}
        placeholder="Select status..."
        showUnassigned={false}
        autoOpen={editingStatus}
        class="w-full"
        onSelect={(selectedStatus) => {
          onsaveField?.({
            field: 'status_id',
            value: selectedStatus?.id || null
          });
        }}
        onCancel={() => {
          oncancelEdit?.({ field: 'status_id' });
        }}
      >
        {#snippet children()}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          >
            <div class="flex items-center gap-2">
              <Text variant="subtle" size="sm">{t('common.status')}</Text>
              <kbd class="px-1.5 py-0.5 text-xs font-medium rounded border opacity-0 group-hover:opacity-70 transition-opacity"
                   style="background-color: var(--ds-background-neutral-subtle); border-color: var(--ds-border); color: var(--ds-text-subtle);">
                {getShortcutDisplay('itemDetail', 'focusStatus')}
              </kbd>
            </div>
            {#if selectedStatus}
              <StatusBadge status={selectedStatus} />
            {:else}
              <Text variant="subtle" size="sm">{t('items.setStatus')}</Text>
            {/if}
          </div>
        {/snippet}
      </ItemPicker>
    </div>
    {/if}
    <!-- Priority Field -->
    {#if shouldShowSystemField('priority')}
    <div class="mb-3">
      <ItemPicker
        value={item?.priority_id ?? null}
        items={priorities}
        config={priorityConfig}
        placeholder="Select priority..."
        showUnassigned={true}
        unassignedLabel="No priority"
        class="w-full"
        onSelect={(selectedPriority) => {
          onsaveField?.({
            field: 'priority_id',
            value: selectedPriority?.id || null
          });
        }}
      >
        {#snippet children()}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          >
            <Text variant="subtle" size="sm">{t('common.priority')}</Text>
            <div class="flex items-center gap-2">
              {#if selectedPriority}
                <ChevronsUp size={14} class="flex-shrink-0" style="color: {selectedPriority.color};" />
                <span style="color: var(--ds-text);">{selectedPriority.name}</span>
              {:else}
                <Text variant="subtle" size="sm">{t('common.none')}</Text>
              {/if}
            </div>
          </div>
        {/snippet}
      </ItemPicker>
    </div>
    {/if}

    <!-- Project Field -->
    {#if shouldShowSystemField('project') && moduleSettings.time_tracking_enabled}
      <div class="mb-3">
        <ItemPicker
          value={selectedProject?.id ?? null}
          items={projectItems}
          config={projectConfig}
          placeholder="Select project..."
          showUnassigned={false}
          class="w-full"
          onSelect={(selectedProject) => {
            // Handle special items
            if (selectedProject?.specialType === 'none') {
              onsaveField?.({
                field: 'project',
                value: { project_id: null, inherit_project: false }
              });
            } else if (selectedProject?.specialType === 'inherit') {
              onsaveField?.({
                field: 'project',
                value: { project_id: null, inherit_project: true }
              });
            } else if (selectedProject) {
              // Regular project
              onsaveField?.({
                field: 'project',
                value: { project_id: selectedProject.id, inherit_project: false }
              });
            }
          }}
        >
          {#snippet children()}
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
            >
              <Text variant="subtle" size="sm">{t('items.project')}</Text>
              <div class="flex items-center gap-2">
                {#if item.effective_project_name || item.project_name}
                  <Briefcase size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
                  <span style="color: var(--ds-text);">{getProjectDisplayText(item)}</span>
                {:else}
                  <Text variant="subtle" size="sm">{t('common.none')}</Text>
                {/if}
              </div>
            </div>
          {/snippet}
        </ItemPicker>
      </div>
    {/if}
    <!-- Assignee Field -->
    {#if shouldShowSystemField('assignee')}
    <div class="mb-3">
      <UserPicker
        value={item.assignee_id ?? null}
        placeholder="Select assignee..."
        showUnassigned={true}
        disabled={!canEdit}
        workspaceId={item?.workspace_id}
        class="w-full"
        onSelect={(selectedUser) => {
          onsaveField?.({
            field: 'assignee',
            value: selectedUser?.id || null,
            assigneeName: selectedUser ? `${selectedUser.first_name} ${selectedUser.last_name}`.trim() : null
          });
        }}
      >
        {#snippet children()}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          >
            <Text variant="subtle" size="sm">{t('common.assignee')}</Text>
            <div class="flex items-center gap-2">
              {#if item.assignee_id && item.assignee_name}
                <Avatar src={item.assignee_avatar} name={item.assignee_name} size="xs" variant="teal" />
                <span style="color: var(--ds-text);">{item.assignee_name}</span>
              {:else}
                <Text variant="subtle" size="sm">{t('items.unassigned')}</Text>
              {/if}
            </div>
          </div>
        {/snippet}
      </UserPicker>
    </div>
    {/if}

    <!-- Milestone Field -->
    {#if shouldShowSystemField('milestone')}
    {@const selectedMilestone = item.milestone_id ? milestones?.find(m => m.id === item.milestone_id) : null}
    <div class="mb-3">
      <ItemPicker
        value={item.milestone_id ?? null}
        items={milestones}
        config={{
          getValue: (item) => item.id,
          getLabel: (item) => item.name,
          searchFields: ['name']
        }}
        placeholder="Select milestone..."
        showUnassigned={true}
        unassignedLabel="No milestone"
        disabled={!canEdit}
        class="w-full"
        onSelect={(item) => {
          onsaveField?.({
            field: 'milestone',
            value: item?.id || null
          });
        }}
      >
        {#snippet children()}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          >
            <Text variant="subtle" size="sm">{t('items.milestone')}</Text>
            <span style="color: {selectedMilestone ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};">
              {selectedMilestone ? selectedMilestone.name : t('common.none')}
            </span>
          </div>
        {/snippet}
      </ItemPicker>
    </div>
    {/if}

    <!-- Iteration Field -->
    {#if shouldShowSystemField('iteration')}
    <div class="mb-3">
      <ItemPicker
        value={item?.iteration_id ?? null}
        items={iterations}
        config={iterationConfig}
        placeholder="Select iteration..."
        showUnassigned={true}
        unassignedLabel="No iteration"
        class="w-full"
        onSelect={(selectedIteration) => {
          onsaveField?.({
            field: 'iteration',
            value: selectedIteration?.id || null,
            iterationName: selectedIteration?.name || null
          });
        }}
      >
        {#snippet children()}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          >
            <Text variant="subtle" size="sm">{t('items.iteration')}</Text>
            <div class="flex items-center gap-2">
              {#if selectedIteration}
                {#if selectedIteration.is_global}
                  <Globe size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
                {:else}
                  <Building2 size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
                {/if}
                <span style="color: var(--ds-text);">{selectedIteration.name}</span>
              {:else}
                <Text variant="subtle" size="sm">{t('common.none')}</Text>
              {/if}
            </div>
          </div>
        {/snippet}
      </ItemPicker>
    </div>
    {/if}

    <!-- Story Points Field -->
    {#if shouldShowSystemField('story_points')}
    <div class="mb-3">
      <div
        class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
        role="button"
        tabindex="0"
        onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.background = ''}
      >
        <Text variant="subtle" size="sm">{t('items.storyPoints')}</Text>
        <div class="flex items-center gap-2">
          {#if editingStoryPoints}
            <input
              type="number"
              step="0.5"
              min="0"
              class="w-20 text-right text-sm rounded px-1.5 py-0.5 border focus:outline-none focus:ring-1"
              style="background: var(--ds-surface-sunken); border-color: var(--ds-border); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
              value={storyPointsEditValue ?? ''}
              onfocus={(e) => e.target.select()}
              oninput={(e) => storyPointsEditValue = e.target.value}
              onblur={() => saveStoryPoints()}
              onkeydown={(e) => {
                if (e.key === 'Enter') { e.target.blur(); }
                if (e.key === 'Escape') { editingStoryPoints = false; }
              }}
              autofocus
            />
          {:else}
            <button
              class="cursor-pointer hover:underline"
              style="color: var(--ds-text);"
              disabled={!canEdit}
              onclick={() => {
                storyPointsEditValue = item?.story_points ?? '';
                editingStoryPoints = true;
              }}
            >
              {#if item?.story_points != null && item?.story_points !== 0}
                <span>{item.story_points}</span>
              {:else}
                <Text variant="subtle" size="sm">{t('common.none')}</Text>
              {/if}
            </button>
          {/if}
        </div>
      </div>
    </div>
    {/if}

    <!-- Recurrence Section -->
    {#if recurrenceRule}
    <div class="mb-3">
      <div
        class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
        role="button"
        tabindex="0"
        onclick={() => oneditRecurrence?.()}
        onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); oneditRecurrence?.(); } }}
        onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-surface-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.background = ''}
      >
        <div class="flex items-center gap-1.5">
          <Repeat class="w-3.5 h-3.5" style="color: var(--ds-icon-subtle);" />
          <Text variant="subtle" size="sm">{t('recurrence.title')}</Text>
        </div>
        <div class="flex items-center gap-2">
            <Lozenge color={recurrenceRule.is_active ? 'green' : 'neutral'} text={recurrenceRule.is_active ? t('recurrence.active') : t('recurrence.inactive')} />
            <span class="text-sm" style="color: var(--ds-text);">{rruleToText(recurrenceRule.rrule)}</span>
        </div>
      </div>
    </div>
    {/if}

    <!-- Scheduling Section (collapsible) -->
    {#if shouldShowSystemField('due_date') || shouldShowSystemField('start_date') || shouldShowSystemField('end_date')}
      <div class="border-t my-4" style="border-color: var(--ds-border);"></div>

      <!-- Scheduling Header -->
      <button
        class="flex items-center justify-between w-full mb-3"
        onclick={toggleScheduling}
        disabled={schedulingForcedOpen}
        style="cursor: {schedulingForcedOpen ? 'default' : 'pointer'};"
      >
        <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">{t('common.scheduling')}</Text>
        {#if !schedulingForcedOpen}
          <ChevronDown
            size={14}
            class="transition-transform duration-200 flex-shrink-0"
            style="color: var(--ds-text-subtle); transform: rotate({schedulingExpanded ? '0' : '-90'}deg);"
          />
        {/if}
      </button>

      {#if schedulingExpanded}
      <!-- Due Date Field -->
      {#if shouldShowSystemField('due_date')}
      <div class="mb-3">
        {#if editingDueDate}
          <div class="w-full py-1.5" use:clickOutside onclickOutside={() => {
            oncancelEdit?.({ field: 'due_date' });
          }}>
            <input
              type="date"
              value={item?.due_date ? item.due_date.split('T')[0] : ''}
              onchange={(e) => {
                onsaveField?.({
                  field: 'due_date',
                  value: e.target.value || null
                });
              }}
              class="w-full px-2 py-1 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
              use:focusAndShowPicker
            />
          </div>
        {:else}
          <button
            onclick={startEditingDueDate}
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          >
            <Text variant="subtle" size="sm">{t('common.dueDate')}</Text>
            <div class="flex items-center gap-2">
              {#if item?.due_date}
                <Calendar size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
                <span style="color: var(--ds-text);">{formatDateShort(item.due_date)}</span>
              {:else}
                <Text variant="subtle" size="sm">{t('common.none')}</Text>
              {/if}
            </div>
          </button>
        {/if}
      </div>
      {/if}

      <!-- Start Date Field -->
      {#if shouldShowSystemField('start_date')}
      <div class="mb-3">
        {#if editingStartDate}
          <div class="w-full py-1.5" use:clickOutside onclickOutside={() => {
            oncancelEdit?.({ field: 'start_date' });
          }}>
            <input
              type="date"
              value={item?.start_date ? item.start_date.split('T')[0] : ''}
              onchange={(e) => {
                onsaveField?.({
                  field: 'start_date',
                  value: e.target.value || null
                });
              }}
              class="w-full px-2 py-1 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
              use:focusAndShowPicker
            />
          </div>
        {:else}
          <button
            onclick={startEditingStartDate}
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          >
            <Text variant="subtle" size="sm">{t('common.startDate')}</Text>
            <div class="flex items-center gap-2">
              {#if item?.start_date}
                <Calendar size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
                <span style="color: var(--ds-text);">{formatDateShort(item.start_date)}</span>
              {:else}
                <Text variant="subtle" size="sm">{t('common.none')}</Text>
              {/if}
            </div>
          </button>
        {/if}
      </div>
      {/if}

      <!-- End Date Field -->
      {#if shouldShowSystemField('end_date')}
      <div class="mb-3">
        {#if editingEndDate}
          <div class="w-full py-1.5" use:clickOutside onclickOutside={() => {
            oncancelEdit?.({ field: 'end_date' });
          }}>
            <input
              type="date"
              value={item?.end_date ? item.end_date.split('T')[0] : ''}
              onchange={(e) => {
                onsaveField?.({
                  field: 'end_date',
                  value: e.target.value || null
                });
              }}
              class="w-full px-2 py-1 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
              use:focusAndShowPicker
            />
          </div>
        {:else}
          <button
            onclick={startEditingEndDate}
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          >
            <Text variant="subtle" size="sm">{t('common.endDate')}</Text>
            <div class="flex items-center gap-2">
              {#if item?.end_date}
                <Calendar size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
                <span style="color: var(--ds-text);">{formatDateShort(item.end_date)}</span>
              {:else}
                <Text variant="subtle" size="sm">{t('common.none')}</Text>
              {/if}
            </div>
          </button>
        {/if}
      </div>
      {/if}

      <!-- Date-type Custom Fields in Scheduling -->
      {#if workspaceScreenFields && workspaceScreenFields.length > 0}
        {@const dateCustomFields = workspaceScreenFields.filter(f => f.field_type === 'custom' && getCustomFieldDefinition(f.field_identifier)?.field_type === 'date')}
        {#each dateCustomFields as screenField}
          {@const fieldDef = getCustomFieldDefinition(screenField.field_identifier)}
          {@const storedValue = item.custom_field_values?.[screenField.field_identifier]}
          {@const isEditing = editingCustomFields[screenField.field_identifier]}
          {@const currentValue = isEditing ? editCustomFieldValues[screenField.field_identifier] : storedValue}
          {#if fieldDef}
            <div class="mb-3">
              {#if isEditing}
                <CustomFieldRenderer
                  field={fieldDef}
                  value={currentValue}
                  readonly={false}
                  disabled={!canEdit}
                  {milestones}
                  {iterations}
                  itemId={item?.id}
                  required={screenField.is_required}
                  onChange={(val) => {
                    editCustomFieldValues[screenField.field_identifier] = val;
                    onsaveField?.({ field: `custom_field_${screenField.field_identifier}` });
                  }}
                  onStartEdit={() => startEditingCustomField(screenField.field_identifier)}
                  onCancel={() => oncancelEdit?.({ field: `custom_field_${screenField.field_identifier}` })}
                />
              {:else}
                <button
                  onclick={() => startEditingCustomField(screenField.field_identifier)}
                  class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
                  onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
                  onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
                  disabled={!canEdit}
                >
                  <Text variant="subtle" size="sm">{fieldDef.name}</Text>
                  <span style="color: {currentValue ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};">
                    {#if currentValue !== null && currentValue !== undefined && currentValue !== ''}
                      {formatDateShort(currentValue)}
                    {:else}
                      {t('common.none')}
                    {/if}
                  </span>
                </button>
              {/if}
            </div>
          {/if}
        {/each}
      {/if}
      {/if}
    {/if}

    <!-- Custom Fields Section (non-date) -->
    {#if workspaceScreenFields && workspaceScreenFields.length > 0}
      {@const configuredCustomFields = workspaceScreenFields.filter(field => field.field_type === 'custom')}
      {@const nonDateCustomFields = configuredCustomFields.filter(f => getCustomFieldDefinition(f.field_identifier)?.field_type !== 'date')}
      {#if nonDateCustomFields.length > 0}
        <!-- Divider before Custom Fields -->
        <div class="border-t my-4" style="border-color: var(--ds-border);"></div>

        <!-- Custom Fields Header -->
        <div class="flex items-center justify-between mb-3">
          <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">{t('fields.title')}</Text>
        </div>

        <div class="space-y-1">
          {#each nonDateCustomFields as screenField}
            {@const fieldDef = getCustomFieldDefinition(screenField.field_identifier)}
            {@const storedValue = item.custom_field_values?.[screenField.field_identifier]}
            {@const isEditing = editingCustomFields[screenField.field_identifier]}
            {@const currentValue = isEditing ? editCustomFieldValues[screenField.field_identifier] : storedValue}
            {#if fieldDef}
              <div class="mb-3">
                {#if fieldDef.field_type === 'linking'}
                  <div class="px-2 py-1.5">
                    <Text variant="subtle" size="sm" class="mb-1">{fieldDef.name}</Text>
                    <CustomFieldRenderer
                      field={fieldDef}
                      value={currentValue}
                      readonly={!canEdit}
                      disabled={!canEdit}
                      itemId={item?.id}
                    />
                  </div>
                {:else if isEditing}
                  <CustomFieldRenderer
                    field={fieldDef}
                    value={currentValue}
                    readonly={false}
                    disabled={!canEdit}
                    {milestones}
                    {iterations}
                    itemId={item?.id}
                    required={screenField.is_required}
                    onChange={(val) => {
                      editCustomFieldValues[screenField.field_identifier] = val;
                      onsaveField?.({ field: `custom_field_${screenField.field_identifier}` });
                    }}
                    onStartEdit={() => startEditingCustomField(screenField.field_identifier)}
                    onCancel={() => oncancelEdit?.({ field: `custom_field_${screenField.field_identifier}` })}
                  />
                {:else}
                  <button
                    onclick={() => startEditingCustomField(screenField.field_identifier)}
                    class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
                    onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
                    onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
                    disabled={!canEdit}
                  >
                    <Text variant="subtle" size="sm">{fieldDef.name}</Text>
                    <span style="color: {currentValue ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};">
                      {#if currentValue !== null && currentValue !== undefined && currentValue !== ''}
                        {#if fieldDef.field_type === 'checkbox'}
                          {currentValue ? t('common.yes') : t('common.no')}
                        {:else if fieldDef.field_type === 'date'}
                          {formatDateShort(currentValue)}
                        {:else if fieldDef.field_type === 'user' && typeof currentValue === 'object'}
                          {currentValue.name || t('common.selected')}
                        {:else if Array.isArray(currentValue)}
                          {currentValue.map(v => typeof v === 'object' ? v.title || v.name || v.label || v.value : v).join(', ')}
                        {:else if typeof currentValue === 'object'}
                          {currentValue.title || currentValue.name || currentValue.label || currentValue.value || JSON.stringify(currentValue)}
                        {:else}
                          {currentValue}
                        {/if}
                      {:else}
                        {t('common.none')}
                      {/if}
                    </span>
                  </button>
                {/if}
              </div>
            {/if}
          {/each}
        </div>
      {/if}
    {/if}

    <!-- Development Links (SCM) - only show for non-personal workspaces -->
    {#if workspace && !workspace.is_personal && item?.id}
      <ItemSCMLinks
        bind:this={scmLinksRef}
        itemId={item.id}
        onaddlink={() => showAddSCMLinkModal = true}
        oncreatebranch={() => showCreateBranchModal = true}
        oncreatepr={(detail) => {
          selectedBranchLink = detail.link;
          showCreatePRFromBranchModal = true;
        }}
      />
    {/if}

    <!-- Personal Tasks (only show for non-personal workspaces) -->
    {#if workspace && !workspace.is_personal}
      <PersonalTasksPanel
        itemId={item?.id}
        workspaceId={item?.workspace_id}
      />
    {/if}
  </div>
</div>

<!-- Add SCM Link Modal -->
{#if showAddSCMLinkModal && item?.id}
  <AddSCMLinkModal
    itemId={item.id}
    onclose={() => showAddSCMLinkModal = false}
    oncreated={() => {
      showAddSCMLinkModal = false;
      // Refresh the links
      if (scmLinksRef) {
        scmLinksRef.loadLinks?.();
      }
    }}
  />
{/if}

<!-- Create Branch Modal -->
{#if showCreateBranchModal && item?.id && itemKey}
  <CreateBranchModal
    itemId={item.id}
    itemKey={itemKey}
    itemTitle={item.title || ''}
    onclose={() => showCreateBranchModal = false}
    oncreated={() => {
      showCreateBranchModal = false;
      // Refresh the links
      if (scmLinksRef) {
        scmLinksRef.loadLinks?.();
      }
    }}
  />
{/if}

<!-- Create PR from Branch Modal -->
{#if showCreatePRFromBranchModal && selectedBranchLink}
  <CreatePRFromBranchModal
    branchLink={selectedBranchLink}
    itemKey={itemKey}
    itemTitle={item?.title || ''}
    onclose={() => {
      showCreatePRFromBranchModal = false;
      selectedBranchLink = null;
    }}
    oncreated={() => {
      showCreatePRFromBranchModal = false;
      selectedBranchLink = null;
      // Refresh the links
      if (scmLinksRef) {
        scmLinksRef.loadLinks?.();
      }
    }}
  />
{/if}

<style>
  /* Override Tailwind hover states for dark mode compatibility */
  :global(.group):hover :global(.hover\:bg-gray-50) {
    background-color: var(--ds-background-neutral-hovered) !important;
  }

  :global(.hover\:bg-gray-50):hover {
    background-color: var(--ds-background-neutral-hovered) !important;
  }

  :global(.hover\:bg-gray-200):hover {
    background-color: var(--ds-background-neutral-hovered) !important;
  }

  :global(.text-gray-600) {
    color: var(--ds-text-subtle) !important;
  }
</style>
