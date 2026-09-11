<script>
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { workspacesStore } from '../stores';
  import Modal from '../dialogs/Modal.svelte';
  import Input from '../components/Input.svelte';
  import NativeSelect from '../components/NativeSelect.svelte';
  import Textarea from '../components/Textarea.svelte';
  import { FileText } from '@lucide/svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import PriorityPicker from '../pickers/PriorityPicker.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import MilestoneCombobox from '../pickers/MilestoneCombobox.svelte';
  import WorkspaceLabelCombobox from '../pickers/WorkspaceLabelCombobox.svelte';
  import {
    isCreateSystemFieldAutoManaged,
    isCreateSystemFieldRenderable,
    resolveEffectiveScreenIds,
    systemFieldIdentifiers,
  } from '../utils/screenFields.js';
  import { dateInputToISOString } from '../utils/dateFormatter.js';
  import { parseDuration } from '../utils/timeUtils.js';
  import { isBooleanCustomFieldType } from '../utils/customFieldTypes.js';

  /**
   * @typedef {'work' | 'personal'} CreateMode
   * 'personal' targets the user's personal workspace and submits a
   * title-only task (no item type) - the same shape the desktop
   * PersonalTasksPanel uses to add a personal task. 'work' is the
   * default full work-item form for regular workspaces.
   */

  /**
   * @typedef {Object} ParentItem
   * @property {number} id
   * @property {string} title
   */

  /**
   * @param {Object} opts
   * @param {boolean} [opts.isOpen]
   * @param {CreateMode} [opts.mode] - 'work' (default) or 'personal'.
   * @param {(() => void) | null} [opts.onclose]
   * @param {ParentItem | null} [opts.parent] - when set, the new item is
   *   created as a child of this item and the type picker is locked to the
   *   available sub-issue types for that item's level.
   * @param {Array<{id: number, name?: string}> | null} [opts.availableItemTypes]
   *   - sub-issue types allowed under `parent`; passed in by the caller (the
   *   mobile item detail computes them the same way the desktop store does).
   * @param {number | null} [opts.workspaceId] - the parent item's workspace,
   *   used to lock the workspace picker when creating a child.
   */
  let {
    isOpen = $bindable(false),
    mode = 'work',
    onclose = null,
    parent = null,
    availableItemTypes = null,
    workspaceId: lockedWorkspaceId = null,
  } = $props();

  const FIXED_SYSTEM_FIELDS = new Set(['title', 'description']);

  let title = $state('');
  let description = $state('');
  let workspaceId = $state(null);
  let itemTypeId = $state(null);
  let itemTypes = $state([]);
  let typesLoading = $state(false);
  let saving = $state(false);
  let error = $state('');
  let lastTypeWorkspace = null;

  // Screen-configured create fields (WI-553): mobile uses the same effective
  // create screen resolution as desktop, but renders fields as a vertical,
  // scrollable form. Required fields are always visible; optional fields live
  // in a collapsible section so large screens remain usable on phones.
  let allCustomFields = $state([]);
  let customFieldsLoaded = $state(false);
  let currentConfigSet = $state(null);
  let configSetLoadedForWorkspace = $state(null);
  let screenFields = $state([]);
  let screenFieldsLoadedForKey = $state(null);
  let screenFieldsLoadingForKey = $state(null);
  let fieldsLoading = $state(false);
  let customFieldValues = $state({});
  let milestones = $state([]);
  let iterations = $state([]);
  let timeProjects = $state([]);
  let showOptionalFields = $state(false);

  let priorityId = $state(null);
  let assigneeId = $state(null);
  let milestoneIds = $state([]);
  let iterationId = $state(null);
  let projectId = $state(null);
  let dueDate = $state('');
  let startDate = $state('');
  let endDate = $state('');
  let storyPoints = $state('');
  let estimate = $state('');
  let labelNames = $state([]);
  let selectedLabels = $state([]);
  let labelsWorkspaceId = null;

  // === Work item templates (WI-538). Mirrors the desktop create modal
  // (workItemFormStore.loadTemplatesForCurrentType): load the templates valid
  // for the current (workspace, item type), auto-apply a mandatory template's
  // body into an empty description and lock the picker, or offer the
  // selectable templates. PWA previously skipped this entirely, so templates
  // were neither enforced nor visible on mobile. ===
  let templateOptions = $state([]);
  let mandatoryTemplate = $state(null);
  let selectedTemplateId = $state(null);
  let templatesLoading = $state(false);
  let templatesInFlightKey = $state(null);

  const templateLocked = $derived(!!mandatoryTemplate);

  const isChild = $derived(!!parent);
  const workspaces = $derived($workspacesStore.regularWorkspaces ?? []);
  // Personal workspace is loaded on-demand; the store keeps it once fetched.
  const personalWorkspace = $derived($workspacesStore.personalWorkspace ?? null);
  const isPersonal = $derived(mode === 'personal');

  const customFieldsById = $derived.by(() => {
    const map = new Map();
    for (const field of allCustomFields) map.set(field.id, field);
    return map;
  });

  const configuredCustomFields = $derived.by(() =>
    screenFields
      .filter((field) => field.field_type === 'custom')
      .map((screenField) => ({
        screenField,
        fieldDef: customFieldsById.get(parseInt(screenField.field_identifier, 10)),
      }))
      .filter((entry) => !!entry.fieldDef)
  );

  const configuredSystemFields = $derived.by(() =>
    screenFields.filter(
      (field) =>
        field.field_type === 'system' &&
        isCreateSystemFieldRenderable(field.field_identifier) &&
        !FIXED_SYSTEM_FIELDS.has(field.field_identifier) &&
        !isCreateSystemFieldAutoManaged(field.field_identifier)
    )
  );

  const requiredSystemFields = $derived(
    configuredSystemFields.filter((field) => field.is_required === true)
  );
  const optionalSystemFields = $derived(
    configuredSystemFields.filter((field) => field.is_required !== true)
  );
  const requiredCustomFields = $derived(
    configuredCustomFields.filter((entry) => entry.screenField.is_required === true)
  );
  const optionalCustomFields = $derived(
    configuredCustomFields.filter((entry) => entry.screenField.is_required !== true)
  );
  const optionalFieldCount = $derived(optionalSystemFields.length + optionalCustomFields.length);

  const canSubmit = $derived(
    title.trim() !== '' &&
      !saving &&
      // Work mode needs a workspace + item type; personal mode just needs a
      // resolved personal workspace (item type resolves to the default on the
      // server, matching the desktop personal-task creation path).
      (isPersonal ? !!personalWorkspace : !!workspaceId && !!itemTypeId)
  );

  // Default the workspace when the dialog opens (first regular workspace), or
  // lock it to the parent item's workspace when creating a child. Personal
  // mode targets the personal workspace and skips this entirely.
  $effect(() => {
    if (!isOpen || isPersonal) return;
    if (isChild && lockedWorkspaceId) {
      workspaceId = lockedWorkspaceId;
      return;
    }
    if (!workspaceId && workspaces.length > 0) {
      workspaceId = workspaces[0].id;
    }
  });

  $effect(() => {
    const nextWorkspaceId = workspaceId;
    if (labelsWorkspaceId != null && labelsWorkspaceId !== nextWorkspaceId) {
      labelNames = [];
      selectedLabels = [];
    }
    labelsWorkspaceId = nextWorkspaceId;
  });

  // Load the personal workspace on-demand when the dialog opens in personal
  // mode (the mobile shell otherwise never touches it outside the Personal tab).
  $effect(() => {
    if (isOpen && isPersonal && !personalWorkspace) {
      workspacesStore.loadPersonalWorkspace();
    }
  });

  // Resolve the item-type list. Personal mode submits a title-only task and
  // needs no type. When creating a child the caller hands us the exact set of
  // allowed sub-issue types (pre-computed from the hierarchy), so there's
  // nothing to fetch - we just adopt them. Otherwise we load the full
  // workspace-scoped list whenever the chosen workspace changes.
  $effect(() => {
    if (!isOpen || isPersonal) return;
    if (isChild) {
      const allowed = Array.isArray(availableItemTypes) ? availableItemTypes : [];
      itemTypes = allowed;
      if (!allowed.some((t) => t.id === itemTypeId)) {
        itemTypeId = allowed[0]?.id ?? null;
      }
      return;
    }
    const wsId = workspaceId;
    if (!wsId || wsId === lastTypeWorkspace) return;
    lastTypeWorkspace = wsId;
    loadTypes(wsId);
  });

  // Load reference data and config whenever the workspace changes.
  $effect(() => {
    if (!isOpen || isPersonal || !workspaceId) return;
    loadWorkspaceFieldData(workspaceId);
  });

  // Load the effective create screen whenever workspace/type are known.
  $effect(() => {
    if (!isOpen || isPersonal || !workspaceId || !itemTypeId || !customFieldsLoaded) return;
    if (configSetLoadedForWorkspace !== workspaceId) return;
    loadScreenFields(workspaceId, itemTypeId);
  });

  // Reload templates whenever the (workspace, item type) the dialog is working
  // with changes (WI-538). Tracks both ids reactively so it fires after
  // loadTypes resolves a new default type too. Skipped in personal mode.
  $effect(() => {
    if (!isOpen || isPersonal) return;
    // Read both deps so the effect re-runs when either changes.
    const wsId = workspaceId;
    const typeId = itemTypeId;
    if (!wsId || !typeId) return;
    loadTemplatesForCurrentType();
  });

  async function loadTypes(wsId) {
    typesLoading = true;
    try {
      const res = await api.itemTypes.getAll({ workspace_id: wsId });
      itemTypes = Array.isArray(res) ? res : (res?.items ?? []);
      // Keep the current type if still valid, else default to the first.
      if (!itemTypes.some((t) => t.id === itemTypeId)) {
        itemTypeId = itemTypes[0]?.id ?? null;
      }
    } catch (err) {
      console.error('Failed to load item types:', err);
      itemTypes = [];
      itemTypeId = null;
    } finally {
      typesLoading = false;
    }
  }

  async function loadWorkspaceFieldData(wsId) {
    await Promise.all([
      loadCustomFields(),
      loadConfigSetForWorkspace(wsId),
      loadMilestones(wsId),
      loadIterations(wsId),
      loadTimeProjects(wsId),
    ]);
  }

  async function loadCustomFields() {
    if (customFieldsLoaded) return;
    try {
      allCustomFields = await api.customFields.getAll();
    } catch (err) {
      console.error('Failed to load custom fields:', err);
      allCustomFields = [];
    } finally {
      customFieldsLoaded = true;
    }
  }

  async function loadConfigSetForWorkspace(wsId) {
    if (configSetLoadedForWorkspace === wsId) return;
    try {
      const response = await api.configurationSets.getAll();
      const configSets = response?.configuration_sets || [];
      let nextConfigSet = null;
      let defaultConfigSet = null;

      for (const configSet of configSets) {
        if (configSet.is_default) defaultConfigSet = configSet;
        if (configSet.workspace_ids?.includes(wsId)) {
          nextConfigSet = await api.configurationSets.get(configSet.id);
          break;
        }
      }

      if (!nextConfigSet && defaultConfigSet) {
        nextConfigSet = await api.configurationSets.get(defaultConfigSet.id);
      }

      currentConfigSet = nextConfigSet;
    } catch (err) {
      console.error('Failed to load configuration set:', err);
      currentConfigSet = null;
    } finally {
      configSetLoadedForWorkspace = wsId;
    }
  }

  async function loadScreenFields(wsId, typeId) {
    const key = `${wsId}-${typeId}`;
    if (screenFieldsLoadedForKey === key || screenFieldsLoadingForKey === key) return;

    fieldsLoading = true;
    screenFieldsLoadingForKey = key;
    try {
      const screenId = resolveEffectiveScreenIds(currentConfigSet, typeId, 1).create;
      const fields = (await api.screens.getFields(screenId)) || [];
      // Ignore an out-of-order response after a workspace/type change.
      if (`${workspaceId}-${itemTypeId}` !== key) return;

      screenFields = fields;
      const customIds = fields
        .filter((field) => field.field_type === 'custom')
        .map((field) => parseInt(field.field_identifier, 10));
      // Preserve entered values for fields that remain configured across the
      // workspace/type change; only fields new to the screen get defaults.
      const previousValues = customFieldValues;
      customFieldValues = {};
      for (const field of allCustomFields) {
        if (customIds.includes(field.id)) {
          const previous = previousValues[field.id];
          customFieldValues[field.id] =
            previous !== undefined && previous !== null && previous !== ''
              ? previous
              : isBooleanCustomFieldType(field.field_type)
                ? false
                : '';
        }
      }
      screenFieldsLoadedForKey = key;
    } catch (err) {
      console.error('Failed to load screen fields:', err);
      screenFields = [];
      customFieldValues = {};
      screenFieldsLoadedForKey = key;
    } finally {
      if (screenFieldsLoadingForKey === key) {
        fieldsLoading = false;
        screenFieldsLoadingForKey = null;
      }
    }
  }

  async function loadMilestones(wsId) {
    try {
      milestones = (await api.milestones.getAll({ workspace_id: wsId, include_global: true })) || [];
    } catch (err) {
      console.error('Failed to load milestones:', err);
      milestones = [];
    }
  }

  async function loadIterations(wsId) {
    try {
      iterations = (await api.iterations.getAll({ workspace_id: wsId, include_global: true })) || [];
    } catch (err) {
      console.error('Failed to load iterations:', err);
      iterations = [];
    }
  }

  async function loadTimeProjects(wsId) {
    try {
      timeProjects = (await api.time.projects.getByWorkspace(wsId)) || [];
    } catch (err) {
      console.error('Failed to load time projects:', err);
      timeProjects = [];
    }
  }

  // Load the work item templates valid for the current (workspace, item type)
  // (WI-538). Auto-applies a mandatory template's body into an empty
  // description and locks the picker; otherwise offers the selectable
  // templates for the type. No-op until both workspace and item type are set,
  // and skipped entirely in personal mode (title-only task, no item type).
  async function loadTemplatesForCurrentType() {
    const wsId = workspaceId;
    const typeId = itemTypeId;
    if (isPersonal || !wsId || !typeId) {
      templateOptions = [];
      mandatoryTemplate = null;
      selectedTemplateId = null;
      templatesInFlightKey = null;
      return;
    }
    const key = `${wsId}:${typeId}`;
    // Dedup only against a fetch in flight for the same key (never permanently
    // cache) — a template created after open must still be picked up.
    if (templatesInFlightKey === key) return;
    templatesInFlightKey = key;

    templatesLoading = true;
    try {
      const list =
        (await api.itemTemplates.getAll(wsId, { item_type_id: typeId })) ?? [];
      // Guard against an out-of-order response after another type change.
      if (`${workspaceId}:${itemTypeId}` !== key) return;

      const mandatory = list.find((t) => t.mode === 'mandatory') || null;
      templateOptions = list.filter((t) => t.mode === 'selectable');
      mandatoryTemplate = mandatory;
      if (mandatory) {
        selectedTemplateId = mandatory.id;
        // Only fill an empty description — mirrors the server's "apply only when
        // empty" rule (services.CreateItem) so an async load can't clobber text
        // the user already typed. The picker stays locked.
        if (!description?.trim()) {
          description = mandatory.description_body || '';
        }
      } else {
        selectedTemplateId = null;
      }
    } catch (err) {
      console.error('Failed to load item templates:', err);
      templateOptions = [];
      mandatoryTemplate = null;
    } finally {
      if (templatesInFlightKey === key) templatesInFlightKey = null;
      templatesLoading = false;
    }
  }

  // Apply a selectable template's body into the description (from the picker).
  function applyTemplate(templateId) {
    const tmpl = templateOptions.find((t) => t.id === templateId);
    if (!tmpl) return;
    description = tmpl.description_body || '';
    selectedTemplateId = templateId;
  }

  function resetConfiguredFields() {
    screenFields = [];
    screenFieldsLoadedForKey = null;
    screenFieldsLoadingForKey = null;
    customFieldValues = {};
    showOptionalFields = false;
    priorityId = null;
    assigneeId = null;
    milestoneIds = [];
    iterationId = null;
    projectId = null;
    dueDate = '';
    startDate = '';
    endDate = '';
    storyPoints = '';
    estimate = '';
    labelNames = [];
    selectedLabels = [];
  }

  function reset() {
    title = '';
    description = '';
    itemTypeId = null;
    itemTypes = [];
    error = '';
    lastTypeWorkspace = null;
    resetConfiguredFields();
    // Reset template state so a new dialog doesn't inherit the prior type's
    // picker options / mandatory lock (WI-538).
    templateOptions = [];
    mandatoryTemplate = null;
    selectedTemplateId = null;
    templatesInFlightKey = null;
    // workspaceId is intentionally kept so repeated creates default to the same.
  }

  function handleClose() {
    isOpen = false;
    reset();
    onclose?.();
  }

  function labelForSystemField(field) {
    switch (field.field_identifier) {
      case 'priority': return 'Priority';
      case 'assignee': return 'Assignee';
      case 'milestone': return 'Milestone';
      case 'iteration': return 'Iteration';
      case 'project': return 'Project';
      case 'labels': return 'Labels';
      case 'due_date': return 'Due date';
      case 'start_date': return 'Start date';
      case 'end_date': return 'End date';
      case 'story_points': return 'Story points';
      case 'estimate':
      case 'estimate_minutes': return 'Estimate';
      default: return field.field_identifier;
    }
  }

  function selectedLabelIds() {
    return (selectedLabels || [])
      .map((label) => label?.id)
      .filter((id) => Number.isFinite(id));
  }

  function systemFieldValue(field) {
    switch (field.field_identifier) {
      case 'priority': return priorityId;
      case 'assignee': return assigneeId;
      case 'milestone': return milestoneIds;
      case 'iteration': return iterationId;
      case 'project': return projectId;
      case 'labels': return selectedLabelIds();
      case 'due_date': return dueDate;
      case 'start_date': return startDate;
      case 'end_date': return endDate;
      case 'story_points': return storyPoints;
      case 'estimate':
      case 'estimate_minutes': return estimate;
      default: return null;
    }
  }

  function isEmptyValue(value) {
    if (Array.isArray(value)) return value.length === 0;
    return value === undefined || value === null || value === '';
  }

  function parsedStoryPoints() {
    if (storyPoints === '' || storyPoints === null || storyPoints === undefined) return null;
    const parsed = parseFloat(storyPoints);
    return Number.isFinite(parsed) && parsed >= 0 ? parsed : null;
  }

  function parsedEstimateMinutes() {
    const raw = (estimate || '').trim();
    if (!raw) return null;
    const minutes = parseDuration(raw);
    return Number.isFinite(minutes) && minutes > 0 ? Math.round(minutes) : null;
  }

  function validateConfiguredFields() {
    for (const field of screenFields) {
      if (!field.is_required) continue;
      if (field.field_type === 'system') {
        if (
          isCreateSystemFieldAutoManaged(field.field_identifier) ||
          !isCreateSystemFieldRenderable(field.field_identifier) ||
          FIXED_SYSTEM_FIELDS.has(field.field_identifier)
        ) {
          continue;
        }
        const value = systemFieldValue(field);
        if (isEmptyValue(value)) {
          error = `${labelForSystemField(field)} is required.`;
          return false;
        }
        if (field.field_identifier === 'story_points' && parsedStoryPoints() === null) {
          error = 'Story points must be a valid number.';
          return false;
        }
        if (
          systemFieldIdentifiers('estimate').includes(field.field_identifier) &&
          parsedEstimateMinutes() === null
        ) {
          error = 'Estimate must be a valid duration.';
          return false;
        }
      } else if (field.field_type === 'custom') {
        const fieldId = parseInt(field.field_identifier, 10);
        const value = customFieldValues[fieldId];
        const fieldDef = customFieldsById.get(fieldId);
        if (isBooleanCustomFieldType(fieldDef?.field_type)) continue;
        if (isEmptyValue(value)) {
          error = `${fieldDef?.name || 'Custom field'} is required.`;
          return false;
        }
      }
    }
    return true;
  }

  function createPayload() {
    const payload = isPersonal
      ? { title: title.trim(), workspace_id: personalWorkspace.id }
      : {
          title: title.trim(),
          description: description.trim(),
          workspace_id: workspaceId,
          item_type_id: itemTypeId,
          priority_id: priorityId || null,
          assignee_id: assigneeId || null,
          milestone_ids: Array.isArray(milestoneIds) ? milestoneIds : [],
          label_ids: selectedLabelIds(),
          iteration_id: iterationId || null,
          project_id: projectId || null,
          due_date: dateInputToISOString(dueDate),
          start_date: dateInputToISOString(startDate),
          end_date: dateInputToISOString(endDate),
          story_points: parsedStoryPoints(),
          estimate_minutes: parsedEstimateMinutes(),
          custom_field_values: customFieldValues,
          // Creating a child: pin it to the parent so it shows up under it.
          parent_id: isChild ? parent.id : undefined,
        };
    return payload;
  }

  async function submit() {
    if (!canSubmit) return;
    saving = true;
    error = '';
    try {
      if (!isPersonal && !validateConfiguredFields()) return;

      const result = await api.items.create(createPayload());
      if (isPersonal) {
        // The newly created personal task lives in this tab's list - let the
        // active Personal view refresh itself. BroadcastChannel excludes the
        // posting tab, so the same-tab notice is a window event instead.
        window.dispatchEvent(new CustomEvent('personal-task-created'));
        handleClose();
        // Stay on the Personal checklist so the user can keep adding tasks,
        // matching the desktop PersonalTasksPanel behavior.
      } else if (isChild) {
        // When creating a child we stay on the parent's detail view and let the
        // caller refresh the sub-item list rather than navigating away.
        handleClose();
      } else {
        handleClose();
        if (result?.id) navigate(`/m/items/${result.id}`);
      }
    } catch (err) {
      console.error('Failed to create item:', err);
      error = err?.message || 'Could not create the item.';
    } finally {
      saving = false;
    }
  }
</script>

<Modal bind:isOpen maxWidth="max-w-md" zIndexClass="z-[500]" onSubmit={submit} submitDisabled={!canSubmit} onclose={handleClose}>
  <div class="create" data-testid="mobile-create-dialog">
    <h2 class="title">{isPersonal ? 'New personal task' : isChild ? 'New sub-item' : 'New item'}</h2>

    {#if isChild}
      <p class="parent" data-testid="create-parent">
        Under <strong>{parent.title}</strong>
      </p>
    {/if}

    <label class="field">
      <span>{isPersonal ? 'Task' : 'Title'}</span>
      <Input
        bind:value={title}
        placeholder={isPersonal ? 'What do you need to do?' : 'What needs doing?'}
        dataTestid="create-title"
        autocomplete="off"
        class="mobile-create-input"
      />
    </label>

    {#if !isPersonal}
      <div class="row">
        <label class="field">
          <span>Workspace</span>
          <NativeSelect
            bind:value={workspaceId}
            disabled={isChild}
            dataTestid="create-workspace"
            class="mobile-create-select"
            options={workspaces.map((ws) => ({ value: ws.id, label: ws.name }))}
          />
        </label>

        <label class="field">
          <span>Type</span>
          <NativeSelect
            bind:value={itemTypeId}
            disabled={typesLoading || itemTypes.length === 0}
            dataTestid="create-type"
            class="mobile-create-select"
            options={itemTypes.map((itemType) => ({ value: itemType.id, label: itemType.name }))}
          />
        </label>
      </div>

      <label class="field">
        <span>Description <em>(optional)</em></span>
        <Textarea
          bind:value={description}
          rows={3}
          placeholder="Add detail…"
          data-testid="create-description"
          readonly={templateLocked}
          class="mobile-create-textarea"
        />
      </label>

      <!-- Work item templates (WI-538). When the selected type enforces a
           mandatory template the body is auto-applied into the description
           above (and locked); otherwise offer the selectable templates valid
           for the type. Mirrors the desktop create modal. -->
      {#if templateLocked}
        <span
          class="template-chip template-locked"
          title={`This item type enforces the "${mandatoryTemplate?.name}" template`}
          data-testid="template-picker-locked"
        >
          <FileText size={14} style="flex-shrink: 0;" />
          <span>{mandatoryTemplate?.name} (enforced)</span>
        </span>
      {:else if templateOptions.length >= 1}
        <label class="field">
          <span>Template</span>
          <NativeSelect
            value={selectedTemplateId ?? ''}
            onchange={(value) => {
              const id = value;
              if (id === '') {
                selectedTemplateId = null;
                return;
              }
              applyTemplate(Number(id));
            }}
            disabled={templatesLoading}
            dataTestid="template-picker"
            class="mobile-create-select"
            options={[
              { value: '', label: 'No template' },
              ...templateOptions.map((template) => ({ value: template.id, label: template.name })),
            ]}
          />
        </label>
      {/if}

      {#if fieldsLoading}
        <p class="loading">Loading configured fields…</p>
      {/if}

      {#if requiredSystemFields.length > 0 || requiredCustomFields.length > 0}
        <section class="field-section" data-testid="configured-required-fields">
          <h3>Required fields</h3>
          {#each requiredSystemFields as field (field.field_identifier)}
            {@render systemField(field, true)}
          {/each}
          {#each requiredCustomFields as entry (entry.screenField.field_identifier)}
            {@render customField(entry, true)}
          {/each}
        </section>
      {/if}

      {#if optionalFieldCount > 0}
        <section class="field-section optional" data-testid="configured-optional-fields">
          <button
            type="button"
            class="optional-toggle"
            data-testid="create-optional-toggle"
            onclick={() => showOptionalFields = !showOptionalFields}
          >
            <span>Optional fields ({optionalFieldCount})</span>
            <span aria-hidden="true">{showOptionalFields ? '−' : '+'}</span>
          </button>
          {#if showOptionalFields}
            <div class="optional-body">
              {#each optionalSystemFields as field (field.field_identifier)}
                {@render systemField(field, false)}
              {/each}
              {#each optionalCustomFields as entry (entry.screenField.field_identifier)}
                {@render customField(entry, false)}
              {/each}
            </div>
          {/if}
        </section>
      {/if}
    {/if}

    {#if error}<p class="error" data-testid="create-error">{error}</p>{/if}

    <div class="actions">
      <button class="btn-cancel" onclick={handleClose} type="button">Cancel</button>
      <button class="btn-create" onclick={submit} disabled={!canSubmit} data-testid="create-submit" type="button">
        {saving ? 'Creating…' : isPersonal ? 'Add task' : 'Create'}
      </button>
    </div>
  </div>
</Modal>

{#snippet systemField(field, required)}
  <div class="field configured-field" data-testid={`configured-system-${field.field_identifier}`}>
    <span>{labelForSystemField(field)} {#if required}<strong>*</strong>{/if}</span>
    {#if field.field_identifier === 'priority'}
      <PriorityPicker
        workspaceId={workspaceId}
        selectedPriorityId={priorityId}
        onChange={(id) => priorityId = id}
        placeholder="No priority"
      />
    {:else if field.field_identifier === 'assignee'}
      <UserPicker
        bind:value={assigneeId}
        workspaceId={workspaceId}
        showUnassigned={true}
        placeholder="Unassigned"
      />
    {:else if field.field_identifier === 'milestone'}
      <MilestoneCombobox
        multiple={true}
        bind:value={milestoneIds}
        workspaceId={workspaceId}
        placeholder="No milestone"
      />
    {:else if field.field_identifier === 'iteration'}
      <NativeSelect
        bind:value={iterationId}
        class="mobile-create-select"
        options={[
          { value: null, label: 'No iteration' },
          ...iterations.map((iteration) => ({ value: iteration.id, label: iteration.name })),
        ]}
      />
    {:else if field.field_identifier === 'project'}
      <NativeSelect
        bind:value={projectId}
        class="mobile-create-select"
        options={[
          { value: null, label: 'No project' },
          ...timeProjects.map((project) => ({ value: project.id, label: project.name })),
        ]}
      />
    {:else if field.field_identifier === 'labels'}
      <WorkspaceLabelCombobox
        {workspaceId}
        bind:value={labelNames}
        placeholder="Select or create labels..."
        onSelect={(result) => {
          labelNames = result?.value || [];
          selectedLabels = result?.labels || [];
        }}
      />
    {:else if field.field_identifier === 'due_date'}
      <Input type="date" bind:value={dueDate} class="mobile-create-input" />
    {:else if field.field_identifier === 'start_date'}
      <Input type="date" bind:value={startDate} class="mobile-create-input" />
    {:else if field.field_identifier === 'end_date'}
      <Input type="date" bind:value={endDate} class="mobile-create-input" />
    {:else if field.field_identifier === 'story_points'}
      <Input type="number" min="0" step="0.5" bind:value={storyPoints} placeholder="Story points" class="mobile-create-input" />
    {:else if field.field_identifier === 'estimate' || field.field_identifier === 'estimate_minutes'}
      <Input type="text" bind:value={estimate} placeholder="3d 4h" class="mobile-create-input" />
    {/if}
  </div>
{/snippet}

{#snippet customField(entry, required)}
  <div class="field configured-field" data-testid={`configured-custom-${entry.fieldDef.id}`}>
    <span>{entry.fieldDef.name} {#if required}<strong>*</strong>{/if}</span>
    <CustomFieldRenderer
      field={entry.fieldDef}
      bind:value={customFieldValues[entry.fieldDef.id]}
      readonly={false}
      onChange={(val) => customFieldValues[entry.fieldDef.id] = val}
      {milestones}
      {iterations}
      autoOpenPickers={false}
    />
  </div>
{/snippet}

<style>
  .create { display: flex; flex-direction: column; gap: 0.85rem; padding: 1rem; }
  .title { margin: 0; font-size: 1.0625rem; font-weight: var(--font-semibold, 600); color: var(--ds-text); }

  .parent { margin: -0.25rem 0 0; font-size: 0.8125rem; color: var(--ds-text-subtle); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .row { display: flex; gap: 0.75rem; }
  .row .field { flex: 1; min-width: 0; }

  .field { display: flex; flex-direction: column; gap: 0.3rem; font-size: 0.75rem; color: var(--ds-text-subtle); }
  .field strong { color: var(--ds-text-danger, #ef4444); }
  .field em { font-style: normal; opacity: 0.7; }
  .field :global(.mobile-create-input), .field :global(.mobile-create-select), .field :global(.mobile-create-textarea) {
    padding: 0.6rem; border: 1px solid var(--ds-border); border-radius: var(--radius-md, 6px);
    background-color: var(--ds-background-input, var(--ds-surface)); color: var(--ds-text);
    font-size: 1rem; /* >=16px avoids iOS zoom-on-focus */
  }
  .field :global(.mobile-create-textarea) { resize: vertical; font-family: inherit; }
  .field :global(.mobile-create-select):disabled { opacity: 0.7; }
  .configured-field :global([role='combobox']),
  .configured-field :global(button),
  .configured-field :global(input),
  .configured-field :global(select),
  .configured-field :global(textarea) { min-height: 44px; }

  .field-section { border-top: 1px solid var(--ds-border); padding-top: 0.85rem; display: flex; flex-direction: column; gap: 0.75rem; }
  .field-section h3 { margin: 0; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--ds-text-subtle); }
  .optional { gap: 0.5rem; }
  .optional-toggle { min-height: 44px; display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; padding: 0.6rem 0; border: 0; background: transparent; color: var(--ds-text); font-size: 0.875rem; font-weight: 600; }
  .optional-body { display: flex; flex-direction: column; gap: 0.75rem; }
  .loading { margin: 0; font-size: 0.8125rem; color: var(--ds-text-subtle); }

  .template-chip { display: inline-flex; align-items: center; gap: 0.4rem; padding: 0.25rem 0.5rem; border-radius: var(--radius-md, 6px); font-size: 0.8125rem; }
  .template-locked {
    align-self: flex-start;
    background-color: var(--ds-background-neutral); color: var(--ds-text-subtle); opacity: 0.8;
  }

  .error { margin: 0; font-size: 0.8125rem; color: var(--ds-text-danger, var(--ds-danger)); }

  .actions { display: flex; justify-content: flex-end; gap: 0.5rem; margin-top: 0.25rem; }
  .btn-cancel {
    padding: 0.6rem 1rem; border: 1px solid var(--ds-border); border-radius: var(--radius-md, 6px);
    background: var(--ds-surface); color: var(--ds-text); cursor: pointer; min-height: 44px;
  }
  .btn-create {
    padding: 0.6rem 1.5rem; border: none; border-radius: var(--radius-md, 6px);
    background: var(--ds-interactive); color: var(--ds-text-inverse, #fff);
    font-weight: var(--font-semibold, 600); cursor: pointer; min-height: 44px;
  }
  .btn-create:disabled { opacity: 0.6; }
</style>
