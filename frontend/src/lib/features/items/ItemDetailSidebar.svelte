<script>
  import { AlertCircle, ChevronDown, ChevronUp, MoreHorizontal, Briefcase, Calendar, Globe, Building2, Repeat, Shield } from '@lucide/svelte';
  import { priorityIconMap } from '../../utils/icons.js';
  import { buildIterationPickerConfig } from '../iterations/iterationPickerUtils.js';
  import { rruleToText } from '../../editors/rruleUtils.js';
  import Lozenge from '../../components/Lozenge.svelte';
  import Avatar from '../../components/Avatar.svelte';
  import Input from '../../components/Input.svelte';
  import Text from '../../components/Text.svelte';
  import TruncatedFieldValue from '../../components/TruncatedFieldValue.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import CustomFieldRenderer from '../items/CustomFieldRenderer.svelte';
  import WorkspaceLabelCombobox from '../../pickers/WorkspaceLabelCombobox.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import PersonalTasksPanel from '../personal/PersonalTasksPanel.svelte';
  import { api } from '../../api.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import ItemSCMLinks from './ItemSCMLinks.svelte';
  import ItemIntegrationLinks from './ItemIntegrationLinks.svelte';
  import AddSCMLinkModal from '../../dialogs/AddSCMLinkModal.svelte';
  import AddIntegrationLinkModal from '../../dialogs/AddIntegrationLinkModal.svelte';
  import CreateBranchModal from '../../dialogs/CreateBranchModal.svelte';
  import CreatePRFromBranchModal from '../../dialogs/CreatePRFromBranchModal.svelte';
  import { getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import { workspacePermissions } from '../../stores';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateOnly, formatCustomFieldDate } from '../../utils/dateFormatter.js';
  import { parseDuration, durationToString } from '../../utils/timeUtils.js';
  import { resolveOptionLabel, resolveOptionLabels } from '../../utils/optionUtils.js';
  import { booleanCustomFieldChecked, isBooleanCustomFieldType } from '../../utils/customFieldTypes.js';
  import { customFieldLinkHref } from '../../utils/customFieldLinks.js';
  import { isSystemFieldConfigured, systemFieldIdentifiers } from '../../utils/screenFields.js';
  import StatusBadge from '../../components/StatusBadge.svelte';
  import { systemPriorityName, systemStatusName } from '../../utils/systemLabels.js';
  import Badge from '../../components/Badge.svelte';
  import ApprovalsTimeline from './ApprovalsTimeline.svelte';

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
  
  const iterationConfig = buildIterationPickerConfig({
    icon: {
      type: 'component',
      source: (item) => (item.is_global ? Globe : Building2),
    },
    searchFields: ['name', 'description'],
  });

  // Priority picker configuration
  const priorityConfig = {
    icon: {
      type: 'component',
      source: (item) => priorityIconMap[item.icon] || AlertCircle
    },
    primary: {
      text: (item) => systemPriorityName(item)
    },
    searchFields: ['name'],
    getValue: (item) => item.id,
    getLabel: (item) => systemPriorityName(item)
  };

  // Status picker configuration
  const statusConfig = {
    icon: {
      type: 'color-dot',
      source: (item) => item.categoryColor || '#9CA3AF',
      size: 'w-2 h-2'
    },
    primary: {
      text: (item) => systemStatusName(item.label)
    },
    searchFields: ['label', 'value'],
    getValue: (item) => item.id,
    getLabel: (item) => systemStatusName(item.label)
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
    pendingApproval = null,
    onapprovalsChanged = null,
    editingStatus = false,
    editingDueDate = false,
    editingStartDate = false,
    editingEndDate = false,
    editingPriority = false,
    editingProject = false,
    editingAssignee = false,
    editingMilestone = false,
    editingIteration = false,
    editingCustomFields = {},
    editCustomFieldValues = {},
    workspaceScreenFields = [],
    workspaceScreenSystemFields = [],
    editableScreenFieldIds = null,
    editableScreenSystemFields = null,
    customFieldDefinitions = [],
    requestTypeFields = [],
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

  // State for Integration Link modals
  let showAddIntegrationLinkModal = $state(false);
  let integrationLinksRef = $state(null);

  // Approvals section collapse state
  const APPROVALS_COLLAPSED_KEY = 'windshift-approvals-collapsed';
  let approvalsUserPref = $state(
    typeof localStorage !== 'undefined'
      ? localStorage.getItem(APPROVALS_COLLAPSED_KEY) === 'true'
      : false
  );
  let approvalsForcedOpen = $derived(!!pendingApproval);
  let approvalsExpanded = $derived(approvalsForcedOpen ? true : !approvalsUserPref);

  function toggleApprovals() {
    if (approvalsForcedOpen) return;
    approvalsUserPref = !approvalsUserPref;
    localStorage.setItem(APPROVALS_COLLAPSED_KEY, String(approvalsUserPref));
  }

  // Skip rendering the Approvals section entirely when the item has no
  // approval activity (current or historical). Keep the fetched list so the
  // timeline can render it without downloading the same payload again.
  let approvalRequests = $state(null);

  async function loadApprovalRequests(id) {
    try {
      approvalRequests = (await api.approvals.forItem(id)) ?? [];
    } catch {
      approvalRequests = [];
    }
  }

  // Re-fetch when the item changes OR after a transition (status_id change),
  // since a transition into an approval-bound status opens a new request.
  $effect(() => {
    const id = item?.id;
    void item?.status_id;
    if (id) {
      loadApprovalRequests(id);
    } else {
      approvalRequests = null;
    }
  });

  let showApprovalsSection = $derived(
    !!pendingApproval || (approvalRequests != null && approvalRequests.length > 0)
  );

  // Scheduling section collapse state
  const SCHEDULING_COLLAPSED_KEY = 'windshift-scheduling-collapsed';
  let schedulingUserPref = $state(
    typeof localStorage !== 'undefined'
      ? localStorage.getItem(SCHEDULING_COLLAPSED_KEY) === 'true'
      : false
  );
  let hasDateCustomFields = $derived(
    !!(workspaceScreenFields && workspaceScreenFields.some(f => f.field_type === 'custom' && getCustomFieldDefinition(f.field_identifier)?.field_type === 'date'))
  );
  let schedulingExpanded = $derived(
    (item?.due_date || item?.end_date || hasDateCustomFields) ? true : schedulingUserPref
  );
  let schedulingForcedOpen = $derived(!!(item?.due_date || item?.end_date || hasDateCustomFields));

  function toggleScheduling() {
    if (schedulingForcedOpen) return;
    schedulingUserPref = !schedulingUserPref;
    localStorage.setItem(SCHEDULING_COLLAPSED_KEY, String(schedulingUserPref));
  }

  // Story points inline editing
  let editingStoryPoints = $state(false);
  let storyPointsEditValue = $state('');

  // Estimate inline editing (duration string parsed to minutes)
  let editingEstimate = $state(false);
  let estimateEditValue = $state('');
  let estimateError = $state(false);

  // Workspace labels inline editing
  let editingLabels = $state(false);

  async function saveLabels(result) {
    const labelIds = (result?.labels || [])
      .map((l) => l?.id)
      .filter((id) => Number.isFinite(id));

    if (item) item.labels = result?.labels || [];
    editingLabels = false;

    try {
      const updated = await api.labels.setForItem(item.id, labelIds);
      if (item) item.labels = updated || [];
    } catch (err) {
      console.error('Failed to save labels:', err);
      errorToast(err?.message || t('items.failedToSaveLabels'));
    }
  }

  function saveStoryPoints() {
    editingStoryPoints = false;
    const raw = storyPointsEditValue;
    const parsed = raw === '' || raw == null ? null : parseFloat(raw);
    const value = parsed != null && !Number.isNaN(parsed) && parsed >= 0 ? parsed : null;
    if (value === (item?.story_points ?? null)) return;
    onsaveField?.({ field: 'story_points', value });
  }

  function saveEstimate() {
    const raw = (estimateEditValue ?? '').trim();
    if (raw === '') {
      editingEstimate = false;
      estimateError = false;
      if ((item?.estimate_minutes ?? null) !== null) {
        onsaveField?.({ field: 'estimate_minutes', value: null });
      }
      return;
    }
    const minutes = parseDuration(raw);
    if (!Number.isFinite(minutes) || minutes <= 0) {
      estimateError = true;
      return;
    }
    editingEstimate = false;
    estimateError = false;
    const rounded = Math.round(minutes);
    if (rounded === (item?.estimate_minutes ?? null)) return;
    onsaveField?.({ field: 'estimate_minutes', value: rounded });
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

  function formatCustomFieldValue(fieldDef, value) {
    if (isBooleanCustomFieldType(fieldDef.field_type)) {
      return booleanCustomFieldChecked(value) ? t('common.yes') : t('common.no');
    }
    if (fieldDef.field_type === 'select') {
      return resolveOptionLabel(fieldDef.options, value);
    }
    if (fieldDef.field_type === 'multiselect') {
      return resolveOptionLabels(fieldDef.options, Array.isArray(value) ? value : []).join(', ');
    }
    if (fieldDef.field_type === 'date') {
      return formatCustomFieldDate(value);
    }
    if (fieldDef.field_type === 'user' && typeof value === 'object') {
      return value.name || t('common.selected');
    }
    if (Array.isArray(value)) {
      return value.map(v => typeof v === 'object' ? v.title || v.name || v.label || v.value : v).join(', ');
    }
    if (typeof value === 'object') {
      return value.title || value.name || value.label || value.value || JSON.stringify(value);
    }
    return value;
  }

  function formatVirtualFieldValue(field, value) {
    if (field.virtual_field_type === 'checkbox') {
      return value ? t('common.yes') : t('common.no');
    }
    if (field.virtual_field_type === 'select') {
      try {
        const opts = JSON.parse(field.virtual_field_options || '[]');
        const match = Array.isArray(opts) ? opts.find(o => o.value === value) : null;
        return match?.label ?? value;
      } catch {
        return value;
      }
    }
    return value;
  }

  function startEditingCustomField(fieldId) {
    if (!canEdit || !isCustomFieldEditable(parseInt(fieldId))) return;
    onstartEditingCustomField?.({ fieldId });
  }

  function startEditingAssignee() {
    if (!canEdit || !isSystemFieldEditable('assignee')) return;
    onstartEditingAssignee?.();
  }

  // Milestone helpers
  function startEditingMilestone() {
    if (!canEdit || !isSystemFieldEditable('milestone')) return;
    onstartEditingMilestone?.();
  }

  // Priority helpers
  function startEditingPriority() {
    if (!canEdit || !isSystemFieldEditable('priority')) return;
    onstartEditingPriority?.();
  }

  let selectedPriority = $derived(
    item?.priority_id && priorities
      ? priorities.find(p => p.id === item.priority_id)
      : null
  );

  // Due Date helpers
  function startEditingDueDate() {
    if (!canEdit || !isSystemFieldEditable('due_date')) return;
    onstartEditingDueDate?.();
  }

  // Start Date helpers
  function startEditingStartDate() {
    if (!canEdit || !isSystemFieldEditable('start_date')) return;
    onstartEditingStartDate?.();
  }

  // End Date helpers
  function startEditingEndDate() {
    if (!canEdit || !isSystemFieldEditable('end_date')) return;
    onstartEditingEndDate?.();
  }

  // Estimate helpers
  function startEditingEstimate() {
    if (!canEdit || !isSystemFieldEditable('estimate')) return;
    estimateEditValue = item?.estimate_minutes != null
      ? durationToString(item.estimate_minutes, { withDays: true })
      : '';
    estimateError = false;
    editingEstimate = true;
  }

  // Story Points helpers
  function startEditingStoryPoints() {
    if (!canEdit || !isSystemFieldEditable('story_points')) return;
    storyPointsEditValue = item?.story_points ?? '';
    editingStoryPoints = true;
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

  // Helper to check if a system field should be shown. Keep the legacy
  // "show all" fallback only when no field configuration at all was resolved.
  // A custom-only screen is still a configured screen and should not leak every
  // system field into the sidebar.
  function shouldShowSystemField(fieldName) {
    const hasConfiguredFields =
      (workspaceScreenSystemFields && workspaceScreenSystemFields.length > 0) ||
      (workspaceScreenFields && workspaceScreenFields.length > 0);
    if (!hasConfiguredFields) {
      return true;
    }
    if (!workspaceScreenSystemFields || workspaceScreenSystemFields.length === 0) {
      return false;
    }
    return isSystemFieldConfigured(workspaceScreenSystemFields, fieldName);
  }

  // Status sits at the top of the sidebar and the date fields live in the
  // collapsible Scheduling section below, so the "middle group" is everything
  // else that the screen wants to surface. Their order follows the screen
  // configuration; when no screen is configured we fall back to a default.
  const MIDDLE_FIELDS_DEFAULT = [
    'priority', 'project', 'assignee', 'milestone', 'iteration',
    'labels', 'estimate', 'story_points',
  ];
  const MIDDLE_FIELDS_EXCLUDED = new Set([
    'title', 'description', 'status', 'due_date', 'start_date', 'end_date', 'created_at',
  ]);
  const orderedMiddleFields = $derived.by(() => {
    if (!workspaceScreenSystemFields || workspaceScreenSystemFields.length === 0) {
      return MIDDLE_FIELDS_DEFAULT;
    }
    // Normalize the estimate_minutes alias and drop anything that belongs to
    // a fixed section. Preserves the screen's display_order.
    const seen = new Set();
    const out = [];
    for (const raw of workspaceScreenSystemFields) {
      const ident = raw === 'estimate_minutes' ? 'estimate' : raw;
      if (MIDDLE_FIELDS_EXCLUDED.has(ident) || seen.has(ident)) continue;
      seen.add(ident);
      out.push(ident);
    }
    return out;
  });

  // When the workspace has separate Edit and View screens, fields on the
  // view screen but not the edit screen are visible-but-read-only. The
  // editable* sets are null when no separation is in play (backwards
  // compatible: every visible field is editable).
  function isSystemFieldEditable(fieldName) {
    if (!editableScreenSystemFields) return true;
    return systemFieldIdentifiers(fieldName).some((identifier) =>
      editableScreenSystemFields.has(identifier)
    );
  }
  function isCustomFieldEditable(fieldId) {
    if (!editableScreenFieldIds) return true;
    return editableScreenFieldIds.has(fieldId);
  }

  // Status helpers
  function startEditingStatus() {
    if (!canEdit || !isSystemFieldEditable('status')) return;
    onstartEditingStatus?.();
  }

  let selectedStatus = $derived(
    item?.status_id && statusOptions
      ? statusOptions.find(s => s.id === item.status_id)
      : null
  );

  // Project helpers
  function startEditingProject() {
    if (!canEdit || !isSystemFieldEditable('project')) return;
    onstartEditingProject?.();
  }

  // Create merged project items array with special items
  let projectItems = $derived.by(() => {
    const items = [];

    // Add "None" special item
    items.push({
      id: 'none',
      name: t('common.none'),
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
        name: t('common.none'),
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
    if (!canEdit || !isSystemFieldEditable('iteration')) return;
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
        ? t('items.projectInherited', { name: item.effective_project_name })
        : t('items.inheritProject');
    }
    if (item.project_id === null || item.project_id === undefined) {
      // None
      return t('items.noProject');
    }
    // Direct assignment
    return item.project_name || t('items.setProject');
  }

  function getInheritLabel(item) {
    if (item.effective_project_name) {
      return t('items.inheritProjectFrom', { name: item.effective_project_name });
    }
    return t('items.inheritProject');
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
      <div class="flex items-center gap-1" data-testid="item-detail-actions">
        <DropdownMenu
          triggerText=""
          triggerIcon={MoreHorizontal}
          triggerTestid="item-detail-actions-menu"
          triggerClass="flex items-center justify-center p-1.5 rounded-md transition-colors"
          triggerStyle="color: var(--ds-text-subtle);"
          items={dropdownItems}
          align="right"
        />
      </div>
    </div>
    <!-- Approvals Section -->
    {#if item?.id && showApprovalsSection}
      <div class="mb-3" data-testid="approvals-sidebar">
        <button
          type="button"
          class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded"
          onclick={toggleApprovals}
          disabled={approvalsForcedOpen}
        >
          <div class="flex items-center gap-2">
            <Shield class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
            <Text variant="subtle" size="sm">{t('items.approvals.title')}</Text>
            {#if pendingApproval?.you_can_decide}
              <Badge variant="warning" size="xs">{t('items.approvals.actionRequired')}</Badge>
            {:else if pendingApproval}
              <Badge variant="neutral" size="xs">{t('items.approvals.statuses.pending')}</Badge>
            {/if}
          </div>
          {#if !approvalsForcedOpen}
            {#if approvalsExpanded}
              <ChevronUp class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            {:else}
              <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
            {/if}
          {/if}
        </button>
        {#if approvalsExpanded}
          <div class="mt-2 px-1">
            <ApprovalsTimeline
              itemId={item.id}
              canCancel={canEdit}
              initialRequests={approvalRequests}
              ondecisionMade={(requests) => {
                approvalRequests = requests;
                onapprovalsChanged?.();
              }}
            />
          </div>
        {/if}
      </div>
    {/if}

    <!-- Status Field -->
    {#if shouldShowSystemField('status')}
    <div class="mb-3" data-testid="status-field">
      <ItemPicker
        value={item?.status_id ?? null}
        items={statusOptions}
        config={statusConfig}
        placeholder={t('pickers.selectStatus')}
        showUnassigned={false}
        autoOpen={editingStatus}
        disabled={!canEdit || !isSystemFieldEditable('status')}
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
              <StatusBadge status={{ ...selectedStatus, label: systemStatusName(selectedStatus.label) }} />
            {:else}
              <Text variant="subtle" size="sm">{t('items.setStatus')}</Text>
            {/if}
          </div>
        {/snippet}
        {#snippet footer()}
          {#if pendingApproval}
            <div
              class="px-3 py-2 text-xs flex items-start gap-1.5"
              style="color: var(--ds-text-accent-yellow);"
              data-testid="status-approval-hint"
            >
              <Shield class="w-3 h-3 mt-0.5 flex-shrink-0" />
              <span>
                {#if pendingApproval.you_can_decide}
                  {t('items.approvals.decisionRequiredHint')}
                {:else}
                  {t('items.approvals.transitionsHiddenHint')}
                {/if}
              </span>
            </div>
          {/if}
        {/snippet}
      </ItemPicker>
    </div>
    {/if}
    {#snippet priorityField()}
      <div class="mb-3" data-testid="priority-field">
        <ItemPicker
          value={item?.priority_id ?? null}
          items={priorities}
          config={priorityConfig}
          placeholder={t('pickers.selectPriority')}
          showUnassigned={true}
          unassignedLabel={t('pickers.noPriority')}
          disabled={!canEdit || !isSystemFieldEditable('priority')}
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
                  {@const PriorityIcon = priorityIconMap[selectedPriority.icon] || AlertCircle}
                  <PriorityIcon size={14} class="flex-shrink-0" style="color: {selectedPriority.color};" />
                  <span style="color: var(--ds-text);">{systemPriorityName(selectedPriority)}</span>
                {:else}
                  <Text variant="subtle" size="sm">{t('common.none')}</Text>
                {/if}
              </div>
            </div>
          {/snippet}
        </ItemPicker>
      </div>
    {/snippet}

    {#snippet projectField()}
      {#if moduleSettings.time_tracking_enabled}
        <div class="mb-3">
          <ItemPicker
            value={selectedProject?.id ?? null}
            items={projectItems}
            config={projectConfig}
            placeholder={t('pickers.selectProject')}
            showUnassigned={false}
            disabled={!canEdit || !isSystemFieldEditable('project')}
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
    {/snippet}

    {#snippet assigneeField()}
      <div class="mb-3">
        <UserPicker
          value={item.assignee_id ?? null}
          placeholder={t('items.selectAssignee')}
          showUnassigned={true}
          disabled={!canEdit || !isSystemFieldEditable('assignee')}
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
    {/snippet}

    {#snippet milestoneField()}
      {@const itemMilestones = (item.milestones || []).map(m => m.id)}
      {@const selectedMilestones = (item.milestones || [])}
      <div class="mb-3" data-testid="milestone-field">
        <MilestoneCombobox
          multiple={true}
          value={itemMilestones}
          workspaceId={item.workspace_id}
          disabled={!canEdit || !isSystemFieldEditable('milestone')}
          class="w-full"
          onSelect={({ ids }) => {
            onsaveField?.({
              field: 'milestone',
              value: ids
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
              <div class="flex items-center gap-1 flex-wrap justify-end">
                {#if selectedMilestones.length === 0}
                  <span style="color: var(--ds-text-subtle);">{t('common.none')}</span>
                {:else}
                  {#each selectedMilestones as ms (ms.id)}
                    <span
                      class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs"
                      style="background-color: {ms.category_color || '#9CA3AF'}1A; border: 1px solid {ms.category_color || '#9CA3AF'}; color: var(--ds-text);"
                    >
                      {ms.name}
                    </span>
                  {/each}
                {/if}
              </div>
            </div>
          {/snippet}
        </MilestoneCombobox>
      </div>
    {/snippet}

    {#snippet iterationField()}
      <div class="mb-3" data-testid="iteration-field">
        <ItemPicker
          value={item?.iteration_id ?? null}
          items={iterations}
          config={iterationConfig}
          placeholder={t('pickers.selectIteration')}
          showUnassigned={true}
          unassignedLabel={t('pickers.noIteration')}
          disabled={!canEdit || !isSystemFieldEditable('iteration')}
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
    {/snippet}

    {#snippet labelsField()}
      {#if item?.id}
        <div class="mb-3" data-testid="labels-field">
          {#if editingLabels}
            <div class="px-2 py-1.5">
              <WorkspaceLabelCombobox
                workspaceId={item?.workspace_id || workspace?.id}
                value={(item?.labels || []).map((l) => l.name)}
                placeholder={t('items.selectOrCreateLabels') || 'Select or create labels...'}
                disabled={!canEdit || !isSystemFieldEditable('labels')}
                onSelect={saveLabels}
                onClose={() => (editingLabels = false)}
                onCancel={() => (editingLabels = false)}
              />
            </div>
          {:else}
            <button
              onclick={() => canEdit && isSystemFieldEditable('labels') && (editingLabels = true)}
              class="w-full flex items-start justify-between gap-2 px-2 py-1.5 text-sm transition-colors rounded group text-left"
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
              disabled={!canEdit || !isSystemFieldEditable('labels')}
            >
              <Text variant="subtle" size="sm" class="shrink-0">{t('items.labels') || 'Labels'}</Text>
              <div class="flex flex-wrap justify-end gap-1.5 min-w-0">
                {#each (item?.labels || []) as label (label.id)}
                  <span
                    class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs"
                    style="background-color: {label.color || '#3B82F6'}1A; color: var(--ds-text); border: 1px solid {label.color || '#3B82F6'};"
                    data-testid="item-label"
                  >
                    <span
                      class="inline-block w-2 h-2 rounded-full"
                      style="background-color: {label.color || '#3B82F6'};"
                      aria-hidden="true"
                    ></span>
                    {label.name}
                  </span>
                {:else}
                  <Text variant="subtle" size="sm">
                    {canEdit ? (t('items.addLabel') || '+ Add label') : (t('common.none') || 'None')}
                  </Text>
                {/each}
              </div>
            </button>
          {/if}
        </div>
      {/if}
    {/snippet}

    {#snippet estimateField()}
      <div class="mb-3">
        {#if editingEstimate}
          <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm">
            <Text variant="subtle" size="sm">{t('items.estimate') || 'Estimate'}</Text>
            <Input
              type="text"
              placeholder="3d 4h"
              class="w-24 text-right text-sm rounded px-1.5 py-0.5 border focus:outline-none focus:ring-1"
              style="background: var(--ds-surface-sunken); border-color: {estimateError ? 'var(--ds-border-danger, #cc3344)' : 'var(--ds-border)'}; color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
              value={estimateEditValue ?? ''}
              onfocus={(e) => e.currentTarget.select()}
              oninput={(e) => { estimateEditValue = e.currentTarget.value; estimateError = false; }}
              onblur={() => saveEstimate()}
              onkeydown={(e) => {
                if (e.key === 'Enter') { e.currentTarget.blur(); }
                if (e.key === 'Escape') { editingEstimate = false; estimateError = false; }
              }}
              autofocus
              size="small"
            />
          </div>
        {:else}
          <button
            onclick={startEditingEstimate}
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
            disabled={!canEdit || !isSystemFieldEditable('estimate')}
          >
            <Text variant="subtle" size="sm">{t('items.estimate') || 'Estimate'}</Text>
            <div class="flex items-center gap-2">
              {#if item?.estimate_minutes != null && item?.estimate_minutes > 0}
                <span style="color: var(--ds-text);">{durationToString(item.estimate_minutes, { withDays: true })}</span>
              {:else}
                <Text variant="subtle" size="sm">{t('common.none')}</Text>
              {/if}
            </div>
          </button>
        {/if}
      </div>
    {/snippet}

    {#snippet storyPointsField()}
      <div class="mb-3">
        {#if editingStoryPoints}
          <div class="w-full flex items-center justify-between px-2 py-1.5 text-sm">
            <Text variant="subtle" size="sm">{t('items.storyPoints')}</Text>
            <Input
              type="number"
              step="0.5"
              min="0"
              class="w-20 text-right text-sm rounded px-1.5 py-0.5 border focus:outline-none focus:ring-1"
              style="background: var(--ds-surface-sunken); border-color: var(--ds-border); color: var(--ds-text); --tw-ring-color: var(--ds-border-focused);"
              value={storyPointsEditValue ?? ''}
              onfocus={(e) => e.currentTarget.select()}
              oninput={(e) => storyPointsEditValue = e.currentTarget.value}
              onblur={() => saveStoryPoints()}
              onkeydown={(e) => {
                if (e.key === 'Enter') { e.currentTarget.blur(); }
                if (e.key === 'Escape') { editingStoryPoints = false; }
              }}
              autofocus
              size="small"
            />
          </div>
        {:else}
          <button
            onclick={startEditingStoryPoints}
            class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
            disabled={!canEdit || !isSystemFieldEditable('story_points')}
          >
            <Text variant="subtle" size="sm">{t('items.storyPoints')}</Text>
            <div class="flex items-center gap-2">
              {#if item?.story_points != null && item?.story_points !== 0}
                <span style="color: var(--ds-text);">{item.story_points}</span>
              {:else}
                <Text variant="subtle" size="sm">{t('common.none')}</Text>
              {/if}
            </div>
          </button>
        {/if}
      </div>
    {/snippet}

    {#each orderedMiddleFields as ident (ident)}
      {#if ident === 'priority'}{@render priorityField()}
      {:else if ident === 'project'}{@render projectField()}
      {:else if ident === 'assignee'}{@render assigneeField()}
      {:else if ident === 'milestone'}{@render milestoneField()}
      {:else if ident === 'iteration'}{@render iterationField()}
      {:else if ident === 'labels'}{@render labelsField()}
      {:else if ident === 'estimate'}{@render estimateField()}
      {:else if ident === 'story_points'}{@render storyPointsField()}
      {/if}
    {/each}

    <!-- Recurrence Section -->
    {#if recurrenceRule}
    <div class="mb-3">
      <div
        data-testid="item-recurrence-summary"
        class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
        role="button"
        tabindex="0"
        onclick={() => oneditRecurrence?.()}
        onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); oneditRecurrence?.(); } }}
        onmouseenter={(e) => e.currentTarget.style.background = 'var(--ds-background-neutral-hovered)'}
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
    {#if shouldShowSystemField('due_date') || shouldShowSystemField('start_date') || shouldShowSystemField('end_date') || hasDateCustomFields}
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
                  value: e.currentTarget.value || null
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
            disabled={!canEdit || !isSystemFieldEditable('due_date')}
          >
            <Text variant="subtle" size="sm">{t('common.dueDate')}</Text>
            <div class="flex items-center gap-2">
              {#if item?.due_date}
                <Calendar size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
                <span style="color: var(--ds-text);">{formatDateOnly(item.due_date)}</span>
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
                  value: e.currentTarget.value || null
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
            disabled={!canEdit || !isSystemFieldEditable('start_date')}
          >
            <Text variant="subtle" size="sm">{t('common.startDate')}</Text>
            <div class="flex items-center gap-2">
              {#if item?.start_date}
                <Calendar size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
                <span style="color: var(--ds-text);">{formatDateOnly(item.start_date)}</span>
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
                  value: e.currentTarget.value || null
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
            disabled={!canEdit || !isSystemFieldEditable('end_date')}
          >
            <Text variant="subtle" size="sm">{t('common.endDate')}</Text>
            <div class="flex items-center gap-2">
              {#if item?.end_date}
                <Calendar size={14} class="flex-shrink-0" style="color: var(--ds-text-subtle);" />
                <span style="color: var(--ds-text);">{formatDateOnly(item.end_date)}</span>
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
            {@const fieldEditable = isCustomFieldEditable(fieldDef.id)}
            <div class="mb-3" data-testid={`item-custom-field-${fieldDef.id}`}>
              {#if isEditing}
                <CustomFieldRenderer
                  field={fieldDef}
                  value={currentValue}
                  readonly={!fieldEditable}
                  disabled={!canEdit || !fieldEditable}
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
                  data-testid={`item-custom-field-edit-${fieldDef.id}`}
                  class="w-full flex items-center justify-between px-2 py-1.5 text-sm transition-colors rounded group"
                  onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
                  onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
                  disabled={!canEdit || !fieldEditable}
                >
                  <Text variant="subtle" size="sm">{fieldDef.name}</Text>
                  <span style="color: {currentValue ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};">
                    {#if currentValue !== null && currentValue !== undefined && currentValue !== ''}
                      {formatCustomFieldDate(currentValue)}
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
              {@const fieldEditable = isCustomFieldEditable(fieldDef.id)}
              <div class="mb-3" data-testid={`item-custom-field-${fieldDef.id}`}>
                {#if fieldDef.field_type === 'linking'}
                  <div class="px-2 py-1.5">
                    <Text variant="subtle" size="sm" class="mb-1">{fieldDef.name}</Text>
                    <CustomFieldRenderer
                      field={fieldDef}
                      value={currentValue}
                      readonly={!canEdit || !fieldEditable}
                      disabled={!canEdit || !fieldEditable}
                      itemId={item?.id}
                    />
                  </div>
                {:else if isEditing}
                  <CustomFieldRenderer
                    field={fieldDef}
                    value={currentValue}
                    readonly={!fieldEditable}
                    disabled={!canEdit || !fieldEditable}
                    {milestones}
                    {iterations}
                    itemId={item?.id}
                    required={screenField.is_required}
                    onChange={(val) => {
                      editCustomFieldValues[screenField.field_identifier] = val;
                      if (fieldDef.field_type !== 'text') {
                        onsaveField?.({ field: `custom_field_${screenField.field_identifier}` });
                      }
                    }}
                    onCommit={(val) => onsaveField?.({
                      field: `custom_field_${screenField.field_identifier}`,
                      value: val,
                    })}
                    onStartEdit={() => startEditingCustomField(screenField.field_identifier)}
                    onCancel={() => oncancelEdit?.({ field: `custom_field_${screenField.field_identifier}` })}
                  />
                {:else if ['user', 'multi_user', 'asset'].includes(fieldDef.field_type)}
                  <div class="flex w-full min-w-0 items-center gap-4 px-2 text-sm">
                    <span
                      class="max-w-[45%] shrink-0 truncate"
                      data-testid={`item-custom-field-label-${fieldDef.id}`}
                    >
                      <Text variant="subtle" size="sm">{fieldDef.name}</Text>
                    </span>
                    <div class="min-w-0 flex-1 text-right">
                      <CustomFieldRenderer
                        field={fieldDef}
                        value={currentValue}
                        readonly={true}
                        disabled={!canEdit || !fieldEditable}
                        noPadding={true}
                        displayAlignment="end"
                        truncateDisplay={true}
                        displayTestId={`item-custom-field-display-${fieldDef.id}`}
                        {milestones}
                        {iterations}
                        itemId={item?.id}
                        onStartEdit={() => startEditingCustomField(screenField.field_identifier)}
                      />
                    </div>
                  </div>
                {:else}
                  {@const hasValue = currentValue !== null && currentValue !== undefined && currentValue !== ''}
                  {@const displayValue = hasValue ? formatCustomFieldValue(fieldDef, currentValue) : t('common.none')}
                  {@const valueHref = customFieldLinkHref(fieldDef.field_type, currentValue)}
                  <div
                    class="group flex min-w-0 items-center gap-4 rounded px-2 py-1.5 text-sm transition-colors hover:bg-[var(--ds-background-neutral-hovered)]"
                  >
                    {#if valueHref}
                      <button
                        type="button"
                        onclick={() => startEditingCustomField(screenField.field_identifier)}
                        data-testid={`item-custom-field-edit-${fieldDef.id}`}
                        class="max-w-[45%] shrink-0 truncate rounded-sm text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-default"
                        style="color: var(--ds-text-subtle); outline-color: var(--ds-border-focused);"
                        disabled={!canEdit || !fieldEditable}
                      >
                        {fieldDef.name}
                      </button>
                    {:else}
                      <Text variant="subtle" size="sm" class="max-w-[45%] shrink-0 truncate">{fieldDef.name}</Text>
                    {/if}

                    <div class="min-w-0 flex-1 text-right" style="color: {hasValue ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};">
                      <TruncatedFieldValue
                        value={displayValue}
                        href={valueHref}
                        onactivate={valueHref ? null : () => startEditingCustomField(screenField.field_identifier)}
                        disabled={!canEdit || !fieldEditable}
                        subtle={!hasValue}
                        testId={valueHref
                          ? `item-custom-field-value-${fieldDef.id}`
                          : `item-custom-field-edit-${fieldDef.id}`}
                      />
                    </div>
                  </div>
                {/if}
              </div>
            {/if}
          {/each}
        </div>
      {/if}
    {/if}

    <!-- Virtual Fields Section (read-only, from request portal submission) -->
    {#if item?.request_type_id && requestTypeFields.length > 0}
      {@const visibleVirtualFields = requestTypeFields.filter(f => {
        if (f.field_type !== 'virtual') return false;
        const v = item.virtual_field_data?.[f.field_identifier];
        return v !== undefined && v !== null && v !== '';
      })}
      {#if visibleVirtualFields.length > 0}
        <div class="border-t my-4" style="border-color: var(--ds-border);"></div>
        <div class="flex items-center justify-between mb-3">
          <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">{t('fields.requestFormFields')}</Text>
        </div>
        <div class="space-y-1">
          {#each visibleVirtualFields as field (field.id)}
            {@const value = item.virtual_field_data[field.field_identifier]}
            {@const label = field.display_name || field.field_label || field.field_name || field.field_identifier}
            <div class="px-2 py-1.5 flex items-start justify-between gap-3 text-sm">
              <Text variant="subtle" size="sm">{label}</Text>
              <span class="text-right break-words" style="color: var(--ds-text); white-space: pre-wrap;">
                {formatVirtualFieldValue(field, value)}
              </span>
            </div>
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

    <!-- Integration Links (Notion, etc.) -->
    {#if item?.id}
      <ItemIntegrationLinks
        bind:this={integrationLinksRef}
        itemId={item.id}
        onaddlink={() => showAddIntegrationLinkModal = true}
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

<!-- Add Integration Link Modal -->
{#if showAddIntegrationLinkModal && item?.id}
  <AddIntegrationLinkModal
    itemId={item.id}
    onclose={() => showAddIntegrationLinkModal = false}
    oncreated={() => {
      showAddIntegrationLinkModal = false;
      if (integrationLinksRef) {
        integrationLinksRef.loadLinks?.();
      }
    }}
  />
{/if}

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
