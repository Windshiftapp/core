<script>
  import { api } from '../api.js';
  import { currentRoute, navigate } from '../router.js';
  import { workspacesStore } from '../stores';
  import { FileText } from '@lucide/svelte';
  import Input from '../components/Input.svelte';
  import NativeSelect from '../components/NativeSelect.svelte';
  import Textarea from '../components/Textarea.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import PriorityPicker from '../pickers/PriorityPicker.svelte';
  import UserPicker from '../pickers/UserPicker.svelte';
  import MilestoneCombobox from '../pickers/MilestoneCombobox.svelte';
  import WorkspaceLabelCombobox from '../pickers/WorkspaceLabelCombobox.svelte';
  import { loadMobileItemDetailSummary } from './mobileItemDetailData.js';
  import MobileEditorPage from './MobileEditorPage.svelte';
  import MobileConfirmSheet from './MobileConfirmSheet.svelte';
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
   * Full-page create flow for the phone surface (replaces the old modal
   * dialog). Modes come from the route:
   *   /m/new                      → work item
   *   /m/new?mode=personal        → personal task (title-only)
   *   /m/new?parent=<id>          → sub-item under a parent (types locked)
   *
   * @typedef {{ id: number, title: string }} ParentItem
   */

  const FIXED_SYSTEM_FIELDS = new Set(['title', 'description']);

  const query = $derived($currentRoute.query);
  const isPersonal = $derived(query.mode === 'personal');
  const parentId = $derived(query.parent ? Number(query.parent) : null);

  let parent = $state(/** @type {ParentItem | null} */ (null));
  let parentErrored = $state(false);
  // Loading while a parent id is set but context hasn't resolved (or failed).
  const parentLoading = $derived(parentId !== null && !parent && !parentErrored);

  let title = $state('');
  let description = $state('');
  let workspaceId = $state(null);
  let itemTypeId = $state(null);
  let itemTypes = $state([]);
  let typesLoading = $state(false);
  let saving = $state(false);
  let error = $state('');
  let lastTypeWorkspace = null;

  // Screen-configured create fields (WI-553): the same effective create screen
  // resolution as desktop, rendered as one vertical page. Required fields are
  // always visible; optional fields live in a collapsible section.
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

  // Work item templates (WI-538). Mirrors the desktop create modal
  // (workItemFormStore.loadTemplatesForCurrentType): load the templates valid
  // for the current (workspace, item type), auto-apply a mandatory template's
  // body into an empty description and lock the picker, or offer the
  // selectable templates.
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

  // Unsaved-input guard: leaving with a draft title asks for confirmation.
  let confirmDiscardOpen = $state(false);
  const isDirty = $derived(title.trim() !== '');

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

  const pageTitle = $derived(
    isPersonal ? 'New personal task' : isChild ? 'New sub-item' : 'New item'
  );

  const canSubmit = $derived(
    title.trim() !== '' &&
      !saving &&
      // Work mode needs a workspace + item type; personal mode just needs a
      // resolved personal workspace (item type resolves to the default on the
      // server, matching the desktop personal-task creation path).
      (isPersonal ? !!personalWorkspace : !!workspaceId && !!itemTypeId)
  );

  // Load parent context for sub-item creation: workspace + allowed sub-issue
  // types come from the same summary endpoint the item detail uses.
  $effect(() => {
    const pid = parentId;
    if (!pid) return;
    let cancelled = false;
    parentErrored = false;
    loadMobileItemDetailSummary(pid)
      .then((summary) => {
        if (cancelled) return;
        const item = summary?.item;
        if (!item) throw new Error('Parent item not found');
        parent = { id: item.id, title: item.title };
        const allowed = Array.isArray(summary?.available_sub_issue_types)
          ? summary.available_sub_issue_types
          : [];
        itemTypes = allowed;
        if (!allowed.some((t) => t.id === itemTypeId)) {
          itemTypeId = allowed[0]?.id ?? null;
        }
        workspaceId = item.workspace_id ?? null;
      })
      .catch((err) => {
        if (cancelled) return;
        console.error('Failed to load parent item:', err);
        parentErrored = true;
      });
    return () => {
      cancelled = true;
    };
  });

  // Default the workspace to the first regular workspace. Children get their
  // workspace from the parent summary instead — skip everything until it
  // resolves so the child types aren't clobbered by a workspace-default load.
  $effect(() => {
    if (isPersonal || parentId != null) return;
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

  // Personal mode targets the personal workspace; load it on demand.
  $effect(() => {
    if (isPersonal && !personalWorkspace) {
      workspacesStore.loadPersonalWorkspace();
    }
  });

  // Load the full workspace-scoped type list whenever the chosen workspace
  // changes (children adopt the parent-provided set instead).
  $effect(() => {
    const wsId = workspaceId;
    if (isPersonal || parentId != null) return;
    if (!wsId || wsId === lastTypeWorkspace) return;
    lastTypeWorkspace = wsId;
    loadTypes(wsId);
  });

  // Load reference data and config whenever the workspace changes.
  $effect(() => {
    if (isPersonal || !workspaceId) return;
    loadWorkspaceFieldData(workspaceId);
  });

  // Load the effective create screen whenever workspace/type are known.
  $effect(() => {
    if (isPersonal || !workspaceId || !itemTypeId || !customFieldsLoaded) return;
    if (configSetLoadedForWorkspace !== workspaceId) return;
    loadScreenFields(workspaceId, itemTypeId);
  });

  // Reload templates whenever the (workspace, item type) the page is working
  // with changes (WI-538). Skipped in personal mode.
  $effect(() => {
    if (isPersonal) return;
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
  // templates for the type.
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
        // Back to the Personal checklist; the tab remounts and loads the new
        // task, matching the desktop PersonalTasksPanel behavior of staying
        // in the list after adding.
        navigate('/m/personal', { replace: true });
      } else if (isChild) {
        // Back to the parent's detail view; it remounts and shows the new
        // sub-item in its list.
        navigate(`/m/items/${parent.id}`, { replace: true });
      } else {
        // Replace so back from the new item doesn't return to a stale form.
        if (result?.id) navigate(`/m/items/${result.id}`, { replace: true });
        else navigate('/m', { replace: true });
      }
    } catch (err) {
      console.error('Failed to create item:', err);
      error = err?.message || 'Could not create the item.';
    } finally {
      saving = false;
    }
  }

  // Cancel / back: a draft title asks for confirmation before being discarded.
  function requestCancel() {
    if (isDirty) {
      confirmDiscardOpen = true;
      return;
    }
    leave();
  }

  function leave() {
    // Explicit replace (not history.back) — deterministic even when the create
    // page was reached via deep link, and it drops the stale form entry.
    if (isPersonal) navigate('/m/personal', { replace: true });
    else if (isChild && parent) navigate(`/m/items/${parent.id}`, { replace: true });
    else navigate('/m', { replace: true });
  }
</script>

<MobileEditorPage
  title={pageTitle}
  saveLabel={isPersonal ? 'Add' : 'Create'}
  canSave={canSubmit}
  saving={saving}
  {error}
  onsave={submit}
  oncancel={requestCancel}
  dataTestid="mobile-create-page"
>
  {#if parentLoading}
    <p class="loading" data-testid="create-parent-loading">Loading parent…</p>
  {:else if parentErrored}
    <p class="error" data-testid="create-parent-error">Couldn't load the parent item.</p>
  {:else}
    <div class="create" data-testid="create-form">
      {#if isChild}
        <p class="parent" data-testid="create-parent">
          Under <strong>{parent?.title}</strong>
        </p>
      {/if}

      <label class="field">
        <span>{isPersonal ? 'Task' : 'Title'}</span>
        <Input
          bind:value={title}
          placeholder={isPersonal ? 'What do you need to do?' : 'What needs doing?'}
          dataTestid="create-title"
          autocomplete="off"
          enterkeyhint="next"
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
            rows={5}
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
    </div>
  {/if}
</MobileEditorPage>

<!-- Discard draft? Shown when cancelling with a non-empty title. -->
<MobileConfirmSheet
  bind:isOpen={confirmDiscardOpen}
  title="Discard this item?"
  message="What you typed will be lost."
  confirmLabel="Discard"
  cancelLabel="Keep editing"
  destructive
  onconfirm={leave}
  dataTestid="create-discard-sheet"
/>

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
  .create { display: flex; flex-direction: column; gap: 0.85rem; }
  .parent { margin: -0.25rem 0 0; font-size: 0.8125rem; color: var(--ds-text-subtle); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .row { display: flex; gap: 0.75rem; }
  .row .field { flex: 1; min-width: 0; }

  .field { display: flex; flex-direction: column; gap: 0.3rem; font-size: 0.75rem; color: var(--ds-text-subtle); }
  .field strong { color: var(--ds-text-danger, #ef4444); }
  .field em { font-style: normal; opacity: 0.7; }
  .field :global(.mobile-create-input), .field :global(.mobile-create-select), .field :global(.mobile-create-textarea) {
    padding: 0.6rem; border: 1px solid var(--ds-border); border-radius: var(--radius-md, 6px);
    background-color: var(--ds-background-input, var(--ds-surface)); color: var(--ds-text);
    font-size: max(1rem, 16px); /* >=16px avoids iOS zoom-on-focus (WI-1325) */
  }
  .field :global(.mobile-create-textarea) { resize: vertical; font-family: inherit; min-height: 7rem; }
  .field :global(.mobile-create-select):disabled { opacity: 0.7; }

  .field-section { border-top: 1px solid var(--ds-border); padding-top: 0.85rem; display: flex; flex-direction: column; gap: 0.75rem; }
  .field-section h3 { margin: 0; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--ds-text-subtle); }
  .optional { gap: 0.5rem; }
  .optional-toggle { min-height: 44px; display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; padding: 0.6rem 0; border: 0; background: transparent; color: var(--ds-text); font-size: 0.875rem; font-weight: 600; }
  .optional-body { display: flex; flex-direction: column; gap: 0.75rem; }
  .loading { margin: 0; font-size: 0.8125rem; color: var(--ds-text-subtle); }

  .template-chip { display: inline-flex; align-items: center; gap: 0.4rem; padding: 0.25rem 0.5rem; border-radius: var(--radius-md, 6px); font-size: 0.8125rem; align-self: flex-start; }
  .template-locked {
    background-color: var(--ds-background-neutral); color: var(--ds-text-subtle); opacity: 0.8;
  }
</style>
