<script>
  import { api } from '../api.js';
  import { currentRoute, navigate, setNavigationInterceptor } from '../router.js';
  import { workspacesStore, workspacePermissions } from '../stores';
  import { FileText } from '@lucide/svelte';
  import NativeSelect from '../components/NativeSelect.svelte';
  import CustomFieldRenderer from '../features/items/CustomFieldRenderer.svelte';
  import { loadMobileItemDetailSummary } from './mobileItemDetailData.js';
  import MobileEditorPage from './MobileEditorPage.svelte';
  import MobileConfirmSheet from './MobileConfirmSheet.svelte';
  import MobileOptionSheet from './MobileOptionSheet.svelte';
  import { autoGrow, enterMovesFocus } from './autoGrowTextarea.js';
  import Avatar from '../components/Avatar.svelte';
  import {
    isCreateSystemFieldAutoManaged,
    isCreateSystemFieldRenderable,
    systemFieldIdentifiers,
  } from '../utils/screenFields.js';
  import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';
  import { dateInputToISOString } from '../utils/dateFormatter.js';
  import { parseDuration } from '../utils/timeUtils.js';
  import { isBooleanCustomFieldType } from '../utils/customFieldTypes.js';
  import { t, translateError } from '../stores/i18n.svelte.js';

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
  let descriptionField = $state(null);
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
  let configSetLoadedForWorkspace = $state(null);
  let screenFields = $state([]);
  // Non-reactive sync guards — see templatesInFlightKey above.
  let screenFieldsLoadedForKey = null;
  let screenFieldsLoadingForKey = null;
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
  let selectedLabels = $state([]);

  // Sheet picker data (the mobile replacement for the desktop pickers that
  // this page used to reuse). Priorities and labels are workspace-scoped and
  // load with the other workspace field data; assignees load lazily on first
  // open, like the item detail.
  let priorities = $state([]);
  let labelOptions = $state([]);
  let assigneeOptions = $state(null);
  let assigneeLoading = $state(false);
  let prioritySheetOpen = $state(false);
  let assigneeSheetOpen = $state(false);
  let milestoneSheetOpen = $state(false);
  let labelsSheetOpen = $state(false);

  // Work item templates (WI-538). Mirrors the desktop create modal
  // (workItemFormStore.loadTemplatesForCurrentType): load the templates valid
  // for the current (workspace, item type), auto-apply a mandatory template's
  // body into an empty description and lock the picker, or offer the
  // selectable templates.
  let templateOptions = $state([]);
  let mandatoryTemplate = $state(null);
  let selectedTemplateId = $state(null);
  let templatesLoading = $state(false);
  // Non-reactive sync guards: read (and written) inside the template-loading
  // effect's call path, so $state here would make the effect a dependency of
  // itself and loop forever.
  let templatesInFlightKey = null;

  const templateLocked = $derived(!!mandatoryTemplate);
  const isChild = $derived(!!parent);
  // Only workspaces where the user holds item.create are valid creation
  // targets — same gate as the desktop create modal (WI-1438/1440).
  const workspaces = $derived(
    ($workspacesStore.regularWorkspaces ?? []).filter((ws) =>
      workspacePermissions.canCreate(ws.id)
    )
  );
  // Personal workspace is loaded on-demand; the store keeps it once fetched.
  const personalWorkspace = $derived($workspacesStore.personalWorkspace ?? null);

  // Unsaved-input guard: leaving with a draft title asks for confirmation —
  // via the header Cancel and via the back gesture (router interceptor).
  let confirmDiscardOpen = $state(false);
  const isDirty = $derived(title.trim() !== '');

  $effect(() => {
    setNavigationInterceptor(() => {
      if (!isDirty) return false;
      confirmDiscardOpen = true;
      return true;
    });
    return () => setNavigationInterceptor(null);
  });

  // Draft persistence (sessionStorage): title/description survive a reload
  // or app kill; cleared on successful create and on explicit discard.
  const draftKey = $derived(
    isPersonal
      ? 'ws-draft:m-create:personal'
      : parentId != null
        ? `ws-draft:m-create:child-${parentId}`
        : 'ws-draft:m-create:work'
  );
  let hydratedDraftKey = $state(null);
  let draftRetired = false;
  $effect(() => {
    const key = draftKey;
    if (hydratedDraftKey === key) return;
    hydratedDraftKey = key;
    try {
      const raw = sessionStorage.getItem(key);
      if (!raw) return;
      const draft = JSON.parse(raw);
      if (!title && typeof draft?.title === 'string') title = draft.title;
      if (!description && typeof draft?.description === 'string') description = draft.description;
    } catch {
      /* corrupt draft — start clean */
    }
  });
  $effect(() => {
    if (!hydratedDraftKey || saving || draftRetired) return;
    try {
      if (title || description) {
        sessionStorage.setItem(draftKey, JSON.stringify({ title, description }));
      } else {
        sessionStorage.removeItem(draftKey);
      }
    } catch {
      /* storage unavailable — drafts are best-effort */
    }
  });

  function clearDraft() {
    // Retire the write-back effect first: when `saving` flips to false in
    // submit's finally, the effect must not resurrect the draft before the
    // page unmounts.
    draftRetired = true;
    try {
      sessionStorage.removeItem(draftKey);
    } catch {
      /* ignore */
    }
  }

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

  const requiredCustomFields = $derived(
    configuredCustomFields.filter((entry) => entry.screenField.is_required === true)
  );
  const optionalCustomFields = $derived(
    configuredCustomFields.filter((entry) => entry.screenField.is_required !== true)
  );

  const pageTitle = $derived(
    isPersonal ? t('mobile.create.personal') : isChild ? t('mobile.create.child') : t('mobile.create.item')
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

  let labelsWorkspaceId = null;

  $effect(() => {
    const nextWorkspaceId = workspaceId;
    if (labelsWorkspaceId != null && labelsWorkspaceId !== nextWorkspaceId) {
      selectedLabels = [];
      milestoneIds = [];
      priorityId = null;
      assigneeId = null;
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
      loadPriorities(wsId),
      loadLabels(wsId),
      loadMilestones(wsId),
      loadIterations(wsId),
      loadTimeProjects(wsId),
    ]);
  }

  // The workspace's configured priority set when one exists, otherwise the
  // global list — resolved through the shared effective-config cache.
  async function loadPriorities(wsId) {
    try {
      let list = [];
      const config = await workspaceDataStore.screenConfig(null);
      const configured = config?.priorities || [];
      list = configured.length > 0 ? configured : await api.priorities.getAll();
      priorities = [...list].sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0));
    } catch (err) {
      console.error('Failed to load priorities:', err);
      priorities = [];
    }
  }

  async function loadLabels(wsId) {
    try {
      labelOptions = (await api.labels.getAll(wsId)) || [];
    } catch (err) {
      console.error('Failed to load labels:', err);
      labelOptions = [];
    }
  }

  function userLabel(user) {
    if (!user) return '';
    return `${user.first_name || ''} ${user.last_name || ''}`.trim() || user.email || user.username || '';
  }

  async function openAssigneeSheet() {
    assigneeSheetOpen = true;
    if (assigneeOptions) return;
    assigneeLoading = true;
    try {
      assigneeOptions = (await api.getAssignableUsers(workspaceId)) ?? [];
    } catch (err) {
      console.error('Failed to load assignable users:', err);
      assigneeOptions = [];
    } finally {
      assigneeLoading = false;
    }
  }

  const priorityName = $derived(priorities.find((p) => p.id === priorityId)?.name ?? null);
  const assigneeName = $derived(userLabel(assigneeOptions?.find((u) => u.id === assigneeId)) || null);
  const milestoneNames = $derived(
    milestones.filter((m) => milestoneIds.includes(m.id)).map((m) => m.name)
  );
  const labelNames = $derived(selectedLabels.map((l) => l?.name).filter(Boolean));

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
      if (Number(workspaceDataStore.workspaceId) !== Number(wsId)) {
        await workspaceDataStore.initialize(wsId);
      }
      // Warm the shared cache; screen and priority loads resolve through it.
      await workspaceDataStore.screenConfig(null);
    } catch (err) {
      console.error('Failed to load configuration set:', err);
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
      const config = await workspaceDataStore.screenConfig(typeId);
      const screenId = config?.screens?.create ?? 1;
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
      case 'priority': return t('common.priority');
      case 'assignee': return t('common.assignee');
      case 'milestone': return t('common.milestone');
      case 'iteration': return t('common.iteration');
      case 'project': return t('common.project');
      case 'labels': return t('common.labels');
      case 'due_date': return t('common.dueDate');
      case 'start_date': return t('common.startDate');
      case 'end_date': return t('common.endDate');
      case 'story_points': return t('items.storyPoints');
      case 'estimate':
      case 'estimate_minutes': return t('items.estimate');
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
          error = t('mobile.create.requiredField', { field: labelForSystemField(field) });
          return false;
        }
        if (field.field_identifier === 'story_points' && parsedStoryPoints() === null) {
          error = t('mobile.create.invalidStoryPoints');
          return false;
        }
        if (
          systemFieldIdentifiers('estimate').includes(field.field_identifier) &&
          parsedEstimateMinutes() === null
        ) {
          error = t('mobile.create.invalidEstimate');
          return false;
        }
      } else if (field.field_type === 'custom') {
        const fieldId = parseInt(field.field_identifier, 10);
        const value = customFieldValues[fieldId];
        const fieldDef = customFieldsById.get(fieldId);
        if (isBooleanCustomFieldType(fieldDef?.field_type)) continue;
        if (isEmptyValue(value)) {
          error = t('mobile.create.requiredField', { field: fieldDef?.name || t('mobile.create.customField') });
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
      clearDraft();
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
      error = (err?.code || err?.errorCode || err?.message) ? translateError(err) : t('mobile.create.failed');
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

  // Discard from the confirm sheet: the sheet pushes no history sentinel on
  // this page (the navigation interceptor owns the back gesture), so the
  // deterministic replace from leave() is safe — including via deep link.
  function discardAndLeave() {
    clearDraft();
    setNavigationInterceptor(null);
    confirmDiscardOpen = false;
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
  saveLabel={isPersonal ? t('common.add') : t('common.create')}
  canSave={canSubmit}
  saving={saving}
  {error}
  onsave={submit}
  oncancel={requestCancel}
  dataTestid="mobile-create-page"
>
  {#if parentLoading}
    <p class="loading" data-testid="create-parent-loading">{t('mobile.create.loadingParent')}</p>
  {:else if parentErrored}
    <p class="error" data-testid="create-parent-error">{t('mobile.create.parentFailed')}</p>
  {:else}
    <div class="create" data-testid="create-form">
      {#if isChild}
        <p class="parent" data-testid="create-parent">
          {t('mobile.create.under')} <strong>{parent?.title}</strong>
        </p>
      {/if}

      <!-- Linear-style borderless hero fields: the title and description are
           the form; properties live in the chip bar pinned at the bottom.
           The title textarea wraps and grows so long titles stay visible. -->
      <textarea
        class="hero-title"
        bind:value={title}
        placeholder={isPersonal ? t('mobile.create.taskTitle') : t('createModal.issueTitle')}
        autocomplete="off"
        rows={1}
        enterkeyhint="next"
        use:autoGrow={title}
        use:enterMovesFocus={{ next: descriptionField }}
        data-testid="create-title"
      ></textarea>
      <textarea
        class="hero-desc"
        bind:value={description}
        bind:this={descriptionField}
        rows={4}
        placeholder={t('mobile.item.descriptionPlaceholder')}
        use:autoGrow={description}
        data-testid="create-description"
        readonly={templateLocked}
      ></textarea>

      {#if !isPersonal}
        <!-- Work item templates (WI-538). When the selected type enforces a
             mandatory template the body is auto-applied into the description
             above (and locked); otherwise offer the selectable templates valid
             for the type. Mirrors the desktop create modal. -->
        {#if templateLocked}
          <span
            class="template-chip template-locked"
            title={t('mobile.create.enforcedTemplateHelp', { name: mandatoryTemplate?.name ?? '' })}
            data-testid="template-picker-locked"
          >
            <FileText size={14} style="flex-shrink: 0;" />
            <span>{t('mobile.create.enforcedTemplate', { name: mandatoryTemplate?.name ?? '' })}</span>
          </span>
        {:else if templateOptions.length >= 1}
          <label class="inline-field">
            <span>{t('workspaces.template')}</span>
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
              options={[
                { value: '', label: t('mobile.create.noTemplate') },
                ...templateOptions.map((template) => ({ value: template.id, label: template.name })),
              ]}
            />
          </label>
        {/if}

        {#if fieldsLoading}
          <p class="loading">{t('mobile.create.loadingFields')}</p>
        {/if}

        <!-- Screen-configured custom fields stay in the flow (arbitrary
             widget types); system properties are chips in the footer bar. -->
        {#if requiredCustomFields.length > 0}
          <section class="field-section" data-testid="configured-required-fields">
            <h3>{t('mobile.create.requiredFields')}</h3>
            {#each requiredCustomFields as entry (entry.screenField.field_identifier)}
              {@render customField(entry, true)}
            {/each}
          </section>
        {/if}

        {#if optionalCustomFields.length > 0}
          <section class="field-section optional" data-testid="configured-optional-fields">
            <button
              type="button"
              class="optional-toggle"
              data-testid="create-optional-toggle"
              onclick={() => showOptionalFields = !showOptionalFields}
            >
              <span>{t('mobile.create.optionalFields', { count: optionalCustomFields.length })}</span>
              <span aria-hidden="true">{showOptionalFields ? '−' : '+'}</span>
            </button>
            {#if showOptionalFields}
              <div class="optional-body">
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

  {#snippet footer()}
    {#if !isPersonal}
      <!-- Linear-style property bar: horizontal chips, one per configured
           system property. Chips open sheets / native pickers. -->
      <div class="chips-bar" data-testid="create-properties">
        <div class="chips-scroll">
          <div class="chip chip-control" data-testid="create-chip-workspace">
            <NativeSelect
              bind:value={workspaceId}
              disabled={isChild}
              dataTestid="create-workspace"
              ariaLabel={t('common.workspace')}
              options={workspaces.map((ws) => ({ value: ws.id, label: ws.name }))}
            />
          </div>
          <div class="chip chip-control" data-testid="create-chip-type">
            <NativeSelect
              bind:value={itemTypeId}
              disabled={typesLoading || itemTypes.length === 0}
              dataTestid="create-type"
              ariaLabel={t('common.type')}
              options={itemTypes.map((itemType) => ({ value: itemType.id, label: itemType.name }))}
            />
          </div>
          {#each configuredSystemFields as field (field.field_identifier)}
            {@render propertyChip(field, field.is_required === true)}
          {/each}
        </div>
      </div>
    {/if}
  {/snippet}
</MobileEditorPage>

<!-- Discard draft? Shown when cancelling (or pressing back) with a non-empty
     title. The sheet pushes no history sentinel: the navigation interceptor
     owns the back gesture on this page. -->
<MobileConfirmSheet
  bind:isOpen={confirmDiscardOpen}
  title={t('mobile.create.discardTitle')}
  message={t('mobile.create.discardMessage')}
  confirmLabel={t('common.discard')}
  cancelLabel={t('mobile.item.keepEditing')}
  destructive
  pushHistory={false}
  onconfirm={discardAndLeave}
  dataTestid="create-discard-sheet"
/>

<!-- Property pickers (sheet-based; the desktop pickers this page used to
     embed don't work well on touch). -->
{#if !isPersonal}
  <MobileOptionSheet
    bind:isOpen={prioritySheetOpen}
    title={t('common.priority')}
    options={priorities}
    getValue={(p) => p.id}
    getLabel={(p) => p.name}
    selectedValue={priorityId}
    allowClear={true}
    clearLabel={t('pickers.noPriority')}
    onSelect={(p) => (priorityId = p?.id ?? null)}
    onClear={() => (priorityId = null)}
    emptyText={t('mobile.create.noPriorities')}
    dataTestid="priority-sheet"
  >
    {#snippet row(p)}
      <span class="opt">
        <span class="opt-dot" style={p.color ? `background-color: ${p.color};` : ''}></span>
        {p.name}
      </span>
    {/snippet}
  </MobileOptionSheet>

  <MobileOptionSheet
    bind:isOpen={assigneeSheetOpen}
    title={t('common.assignee')}
    options={assigneeOptions ?? []}
    loading={assigneeLoading}
    getValue={(u) => u.id}
    getLabel={userLabel}
    selectedValue={assigneeId}
    allowClear={true}
    clearLabel={t('common.unassigned')}
    onSelect={(u) => (assigneeId = u?.id ?? null)}
    onClear={() => (assigneeId = null)}
    emptyText={t('mobile.item.noAssignees')}
    dataTestid="create-assignee-sheet"
  >
    {#snippet row(u)}
      <span class="opt">
        <Avatar src={u.avatar_url} name={userLabel(u)} size="xs" variant="teal" />
        <span>{userLabel(u)}</span>
      </span>
    {/snippet}
  </MobileOptionSheet>

  <MobileOptionSheet
    bind:isOpen={milestoneSheetOpen}
    title={t('common.milestone')}
    options={milestones}
    getValue={(m) => m.id}
    getLabel={(m) => m.name}
    selectedValues={milestoneIds}
    multiple={true}
    onToggle={(m) => {
      milestoneIds = milestoneIds.includes(m.id)
        ? milestoneIds.filter((id) => id !== m.id)
        : [...milestoneIds, m.id];
    }}
    emptyText={t('mobile.create.noMilestones')}
    dataTestid="milestone-sheet"
  />

  <MobileOptionSheet
    bind:isOpen={labelsSheetOpen}
    title={t('common.labels')}
    options={labelOptions}
    getValue={(l) => l.id}
    getLabel={(l) => l.name}
    selectedValues={selectedLabels.map((l) => l.id)}
    multiple={true}
    onToggle={(label) => {
      selectedLabels = selectedLabels.some((l) => l.id === label.id)
        ? selectedLabels.filter((l) => l.id !== label.id)
        : [...selectedLabels, label];
    }}
    emptyText={t('pages.labelsEmpty')}
    dataTestid="labels-sheet"
  />
{/if}

{#snippet propertyChip(field, required)}
  <div class="chip-wrap" data-testid={`configured-system-${field.field_identifier}`}>
    {#if field.field_identifier === 'priority'}
      <button class="chip" class:filled={!!priorityName} onclick={() => (prioritySheetOpen = true)} data-testid="create-field-priority" type="button">
        {#if priorityName}<span class="chip-dot" style={`background-color: ${priorities.find((p) => p.id === priorityId)?.color || 'var(--ds-text-subtle)'};`}></span>{/if}
        <span class="chip-value" class:unset={!priorityName}>{priorityName ?? t('common.priority')}{#if required}<span class="req">*</span>{/if}</span>
      </button>
    {:else if field.field_identifier === 'assignee'}
      <button class="chip" class:filled={!!assigneeName} onclick={openAssigneeSheet} data-testid="create-field-assignee" type="button">
        <span class="chip-value" class:unset={!assigneeName}>{assigneeName ?? t('common.assignee')}{#if required}<span class="req">*</span>{/if}</span>
      </button>
    {:else if field.field_identifier === 'milestone'}
      <button class="chip" class:filled={milestoneNames.length > 0} onclick={() => (milestoneSheetOpen = true)} data-testid="create-field-milestone" type="button">
        <span class="chip-value" class:unset={milestoneNames.length === 0}>{milestoneNames.length > 0 ? milestoneNames.join(', ') : t('common.milestone')}{#if required}<span class="req">*</span>{/if}</span>
      </button>
    {:else if field.field_identifier === 'labels'}
      <button class="chip" class:filled={labelNames.length > 0} onclick={() => (labelsSheetOpen = true)} data-testid="create-field-labels" type="button">
        <span class="chip-value" class:unset={labelNames.length === 0}>{labelNames.length > 0 ? labelNames.join(', ') : t('common.labels')}{#if required}<span class="req">*</span>{/if}</span>
      </button>
    {:else if field.field_identifier === 'iteration'}
      <div class="chip chip-control">
        <NativeSelect
          bind:value={iterationId}
          ariaLabel={t('common.iteration')}
          options={[
            { value: null, label: t('common.iteration') },
            ...iterations.map((iteration) => ({ value: iteration.id, label: iteration.name })),
          ]}
        />
      </div>
    {:else if field.field_identifier === 'project'}
      <div class="chip chip-control">
        <NativeSelect
          bind:value={projectId}
          ariaLabel={t('common.project')}
          options={[
            { value: null, label: t('common.project') },
            ...timeProjects.map((project) => ({ value: project.id, label: project.name })),
          ]}
        />
      </div>
    {:else if field.field_identifier === 'due_date'}
      <label class="chip chip-control">
        <input class="chip-input" type="date" bind:value={dueDate} aria-label={t('common.dueDate')} />
      </label>
    {:else if field.field_identifier === 'start_date'}
      <label class="chip chip-control">
        <input class="chip-input" type="date" bind:value={startDate} aria-label={t('common.startDate')} />
      </label>
    {:else if field.field_identifier === 'end_date'}
      <label class="chip chip-control">
        <input class="chip-input" type="date" bind:value={endDate} aria-label={t('common.endDate')} />
      </label>
    {:else if field.field_identifier === 'story_points'}
      <label class="chip chip-control">
        <input class="chip-input" type="number" min="0" step="0.5" bind:value={storyPoints} placeholder={t('items.storyPoints')} aria-label={t('items.storyPoints')} />
      </label>
    {:else if field.field_identifier === 'estimate' || field.field_identifier === 'estimate_minutes'}
      <label class="chip chip-control">
        <input class="chip-input" type="text" bind:value={estimate} placeholder={t('items.estimate')} aria-label={t('items.estimate')} />
      </label>
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
  .create { display: flex; flex-direction: column; gap: 0.5rem; }
  .parent { margin: 0 0 0.25rem; font-size: 0.8125rem; color: var(--ds-text-subtle); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  /* Linear-style borderless hero fields. The title textarea wraps and grows
     with its content (autoGrow action) so long titles stay fully visible. */
  .hero-title {
    width: 100%;
    margin: 0.75rem 0 0;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text);
    font-family: inherit;
    font-size: 1.35rem;
    font-weight: var(--font-semibold, 600);
    line-height: 1.25;
    overflow: hidden;
    resize: none;
  }
  .hero-title::placeholder { color: var(--ds-text-subtlest, var(--ds-text-subtle)); font-weight: var(--font-semibold, 600); }
  .hero-desc {
    width: 100%;
    min-height: 5.5rem;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text);
    font-family: inherit;
    font-size: max(1rem, 16px);
    line-height: 1.5;
    resize: none;
  }
  .hero-desc::placeholder { color: var(--ds-text-subtlest, var(--ds-text-subtle)); }
  .hero-title:focus,
  .hero-desc:focus { outline: none; }

  /* In-flow secondary controls (template picker, custom fields) — minimal,
     borderless: a small label plus the control, no boxes. */
  .inline-field { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.75rem; color: var(--ds-text-subtle); margin-top: 0.5rem; }
  .inline-field :global(select) { font-size: max(0.9375rem, 16px); }

  .field { display: flex; flex-direction: column; gap: 0.3rem; font-size: 0.75rem; color: var(--ds-text-subtle); }
  .field strong { color: var(--ds-text-danger, #ef4444); }
  /* Borderless custom-field controls (CustomFieldRenderer emits boxed inputs
     shared with desktop — strip the boxes on the phone surface). */
  .configured-field :global(input),
  .configured-field :global(select),
  .configured-field :global(textarea) {
    border: none;
    background-color: transparent;
    padding-left: 0;
    margin-left: 1px; /* keep focus rings visually aligned */
    color: var(--ds-text);
    font-size: max(1rem, 16px);
  }

  /* Property chip bar (Linear-style): one pill per configured system
     property, horizontally scrollable, pinned above the safe area. */
  .chips-bar { min-width: 0; }
  .chips-scroll {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    overflow-x: auto;
    padding: 0.25rem 0;
    scrollbar-width: none;
  }
  .chips-scroll::-webkit-scrollbar { display: none; }
  .chip-wrap { flex-shrink: 0; }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    min-height: 38px;
    max-width: 11rem;
    padding: 0.3rem 0.85rem;
    border: none;
    border-radius: var(--radius-full, 9999px);
    background-color: var(--ds-background-neutral);
    color: var(--ds-text);
    font-size: max(0.9375rem, 16px);
    text-align: left;
    cursor: pointer;
  }
  button.chip:active { background-color: var(--ds-background-neutral-hovered); }
  .chip-value {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .chip-value.unset { color: var(--ds-text-subtle); }
  .chip .req { color: var(--ds-text-danger, #ef4444); margin-left: 1px; }
  .chip-dot {
    width: 8px;
    height: 8px;
    border-radius: var(--radius-full, 9999px);
    flex-shrink: 0;
  }

  /* Native selects/dates/numbers inside chips. NativeSelect paints its own
     inline background/border — override with !important to stay borderless. */
  .chip-control { padding: 0; }
  .chip-control :global(select) {
    min-height: 38px;
    padding: 0.3rem 0.85rem !important;
    border: none !important;
    border-radius: var(--radius-full, 9999px) !important;
    background-color: transparent !important;
    color: var(--ds-text);
    font-size: max(0.9375rem, 16px);
    width: auto;
  }
  .chip-input {
    min-height: 32px;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--ds-text);
    font-family: inherit;
    font-size: max(0.9375rem, 16px);
  }
  .chip-input:focus { outline: none; }

  .opt { display: inline-flex; align-items: center; gap: 0.5rem; min-width: 0; }
  .opt-dot { width: 8px; height: 8px; border-radius: var(--radius-full, 9999px); background-color: var(--ds-icon-subtle, var(--ds-text-subtle)); flex-shrink: 0; }

  .field-section { border-top: 1px solid var(--ds-border); padding-top: 0.85rem; display: flex; flex-direction: column; gap: 0.75rem; margin-top: 0.75rem; }
  .field-section h3 { margin: 0; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--ds-text-subtle); }
  .optional { gap: 0.5rem; }
  .optional-toggle { min-height: 44px; display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; padding: 0.6rem 0; border: 0; background: transparent; color: var(--ds-text); font-size: 0.875rem; font-weight: 600; }
  .optional-body { display: flex; flex-direction: column; gap: 0.75rem; }
  .loading { margin: 0; font-size: 0.8125rem; color: var(--ds-text-subtle); }
  .error { margin: 0; font-size: 0.8125rem; color: var(--ds-text-danger, var(--ds-danger)); }

  .template-chip { display: inline-flex; align-items: center; gap: 0.4rem; padding: 0.25rem 0.5rem; border-radius: var(--radius-md, 6px); font-size: 0.8125rem; align-self: flex-start; }
  .template-locked {
    background-color: var(--ds-background-neutral); color: var(--ds-text-subtle); opacity: 0.8;
  }
</style>
