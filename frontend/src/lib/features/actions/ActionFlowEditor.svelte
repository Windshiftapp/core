<script>
  import { onMount } from 'svelte';
  import { Pencil, RefreshCw, MessageSquare, Bell, HelpCircle, Database, PlusSquare, Box, Globe, Sparkles, Bot, Milestone, UsersRound, X } from '@lucide/svelte';
  import { toHotkeyString, getShortcutDisplay } from '../../utils/keyboardShortcuts.js';
  import { api } from '../../api.js';
  import FieldSelector from '../../pickers/FieldSelector.svelte';
  import MilestoneCombobox from '../../pickers/MilestoneCombobox.svelte';
  import TriggerNode from './nodes/TriggerNode.svelte';
  import SetFieldNode from './nodes/SetFieldNode.svelte';
  import SetStatusNode from './nodes/SetStatusNode.svelte';
  import AddCommentNode from './nodes/AddCommentNode.svelte';
  import NotifyUserNode from './nodes/NotifyUserNode.svelte';
  import ConditionNode from './nodes/ConditionNode.svelte';
  import UpdateAssetNode from './nodes/UpdateAssetNode.svelte';
  import CreateAssetNode from './nodes/CreateAssetNode.svelte';
  import RelatedItemsNode from './nodes/RelatedItemsNode.svelte';
  import TransitionItemNode from './nodes/TransitionItemNode.svelte';
  import RoundRobinAssignNode from './nodes/RoundRobinAssignNode.svelte';
  import ContainerRunNode from './nodes/ContainerRunNode.svelte';
  import HTTPRequestNode from './nodes/HTTPRequestNode.svelte';
  import AIExtractNode from './nodes/AIExtractNode.svelte';
  import AIAgentNode from './nodes/AIAgentNode.svelte';
  import CreateMilestoneNode from './nodes/CreateMilestoneNode.svelte';
  import CreateMilestoneConfigPanel from './CreateMilestoneConfigPanel.svelte';
  import UpdateAssetConfigPanel from './UpdateAssetConfigPanel.svelte';
  import CreateAssetConfigPanel from './CreateAssetConfigPanel.svelte';
  import PlaceholderReferenceModal from './PlaceholderReferenceModal.svelte';
  import BaseActionFlowEditor from './shared/BaseActionFlowEditor.svelte';
  import HttpHeadersEditor from './shared/HttpHeadersEditor.svelte';
  import { getFieldSelectorValue, backendFieldName, standardFieldTypes, collectOutputFields, isValidOutputFieldName } from './shared/fieldNameMapping.js';
  import { t } from '../../stores/i18n.svelte.js';
  import Checkbox from '../../components/Checkbox.svelte';
  import Input from '../../components/Input.svelte';
  import Select from '../../components/Select.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import UserPicker from '../../pickers/UserPicker.svelte';
  import RolePicker from '../../pickers/RolePicker.svelte';
  import { actionFlowStore } from '../../stores/actionFlowStore.svelte.js';
  import { permissionStore } from '../../stores';
  import { agentRuns } from '../../stores/agentRuns.svelte.js';
  import { infoToast } from '../../stores/toasts.svelte.js';

  let {
    action,
    statuses = [],
    onSave,
    onCancel
  } = $props();

  let showPlaceholderModal = $state(false);
  let milestones = $state([]);
  let teams = $state([]);
  let linkTypes = $state([]);
  let itemTypes = $state([]);
  let statusCategories = $state([]);
  let assignableUsers = $state([]);

  // Actor override: null means the action runs under the triggering user's
  // permissions. Only users with the global action.set_actor permission can
  // change this; others see a read-only display of the current value.
  // svelte-ignore state_referenced_locally
  let actorUserId = $state(action?.actor_user_id ?? null);
  let lastActorSource = $state(null);
  // svelte-ignore state_referenced_locally
  let allowedRoleIds = $state([...(action?.allowed_role_ids ?? [])]);
  let lastAllowedRolesSource = $state(null);
  let canSetActor = $derived(permissionStore.hasPermissionKey('action.set_actor'));

  $effect(() => {
    const source = `${action?.id ?? 'new'}:${action?.actor_user_id ?? ''}`;
    if (source === lastActorSource) return;
    lastActorSource = source;
    actorUserId = action?.actor_user_id ?? null;
  });

  $effect(() => {
    const roleIDs = action?.allowed_role_ids ?? [];
    const source = `${action?.id ?? 'new'}:${roleIDs.join(',')}`;
    if (source === lastAllowedRolesSource) return;
    lastAllowedRolesSource = source;
    allowedRoleIds = [...roleIDs];
  });

  const nodeTypes = {
    trigger: TriggerNode,
    set_field: SetFieldNode,
    set_status: SetStatusNode,
    add_comment: AddCommentNode,
    notify_user: NotifyUserNode,
    condition: ConditionNode,
    update_asset: UpdateAssetNode,
    create_asset: CreateAssetNode,
    related_items: RelatedItemsNode,
    transition_item: TransitionItemNode,
    round_robin_assign: RoundRobinAssignNode,
    container_run: ContainerRunNode,
    http_request: HTTPRequestNode,
    ai_extract: AIExtractNode,
    ai_agent: AIAgentNode,
    create_milestone: CreateMilestoneNode,
  };

  // Mirror each node's accentColor in the minimap so the overview reflects
  // the canvas colour coding instead of rendering every node the same grey.
  const nodeTypeAccents = {
    trigger: 'amber',
    set_field: 'purple',
    set_status: 'teal',
    add_comment: 'orange',
    notify_user: 'magenta',
    condition: 'yellow',
    update_asset: 'teal',
    create_asset: 'green',
    related_items: 'indigo',
    transition_item: 'teal',
    round_robin_assign: 'magenta',
    container_run: 'blue',
    http_request: 'cyan',
    ai_extract: 'purple',
    ai_agent: 'magenta',
    create_milestone: 'green',
  };

  function minimapNodeColor(node) {
    const accent = nodeTypeAccents[node.type];
    if (!accent) return 'var(--ds-accent-gray-subtle, #94a3b8)';
    return `var(--ds-accent-${accent}-subtle)`;
  }

  function minimapNodeStroke(node) {
    const accent = nodeTypeAccents[node.type];
    if (!accent) return 'var(--ds-border, #64748b)';
    return `var(--ds-accent-${accent})`;
  }

  // Map node types to lucide icons (icons stay client-side; everything else
  // about the palette comes from the server-provided catalog so adding a new
  // node type only requires registering it in internal/services/actioncatalog
  // and rebuilding — the editor picks it up automatically). Types missing
  // from this map render with the trigger icon as a neutral fallback.
  const typeIcons = {
    set_field: Pencil,
    set_status: RefreshCw,
    add_comment: MessageSquare,
    notify_user: Bell,
    condition: HelpCircle,
    update_asset: Database,
    create_asset: PlusSquare,
    http_request: Globe,
    container_run: Box,
    ai_extract: Sparkles,
    ai_agent: Bot,
    transition_item: RefreshCw,
    related_items: RefreshCw,
    round_robin_assign: UsersRound,
    create_milestone: Milestone,
  };

  // i18n keys for node types. When a type isn't in this map, the palette
  // falls back to the server-provided label string (English). New node types
  // can ship as catalog-only entries; translations land here in a follow-up.
  const typeI18nKeys = {
    set_field: 'actions.nodes.setField',
    set_status: 'actions.nodes.setStatus',
    add_comment: 'actions.nodes.addComment',
    notify_user: 'actions.nodes.notifyUser',
    condition: 'actions.nodes.condition',
    update_asset: 'actions.nodes.updateAsset',
    create_asset: 'actions.nodes.createAsset',
    http_request: 'actions.nodes.httpRequest',
    container_run: 'actions.nodes.containerRun',
    ai_extract: 'actions.nodes.aiExtract',
    ai_agent: 'actions.nodes.aiAgent',
    transition_item: 'actions.nodes.transitionItem',
    related_items: 'actions.nodes.relatedItems',
    round_robin_assign: 'actions.nodes.roundRobinAssign',
  };

  // nodePalette is built from the catalog response on mount. Trigger nodes
  // are filtered out (they're created implicitly by the editor) along with
  // any type the editor doesn't yet have a custom Svelte node component for.
  let nodePalette = $state([]);
  let triggerTypes = $state([]);

  function buildPaletteFromCatalog(catalog) {
    nodePalette = (catalog?.nodes ?? [])
      .filter((n) => n.type !== 'trigger' && nodeTypes[n.type])
      .map((n) => ({
        type: n.type,
        label: typeI18nKeys[n.type] ? t(typeI18nKeys[n.type]) : n.label,
        icon: typeIcons[n.type] ?? Pencil,
      }));
    triggerTypes = (catalog?.triggers ?? []).map((tr) => ({
      value: tr.type,
      label: triggerI18nKey(tr.type) ? t(triggerI18nKey(tr.type)) : tr.label,
    }));
  }

  function triggerI18nKey(triggerType) {
    switch (triggerType) {
      case 'status_transition': return 'actions.trigger.statusTransition';
      case 'item_created': return 'actions.trigger.itemCreated';
      case 'item_updated': return 'actions.trigger.itemUpdated';
      case 'item_linked': return 'actions.trigger.itemLinked';
      case 'manual': return 'actions.trigger.manual';
      default: return null;
    }
  }

  // Workspace-scoped capability lists for the picker. Loaded once per
  // capability type when the editor mounts, then reused as the user clicks
  // through capability-consuming nodes.
  let capabilitiesByType = $state({
    docker_environment: [],
    http_client: [],
    llm_connection: [],
  });

  async function loadCapabilities(type) {
    if (!action?.workspace_id) return;
    try {
      const list = await api.actionCapabilities.getForWorkspace(action.workspace_id, type);
      capabilitiesByType[type] = list || [];
    } catch (err) {
      console.error(`Failed to load ${type} capabilities for workspace`, err);
    }
  }

  async function loadCatalog() {
    if (!action?.workspace_id) return;
    try {
      const catalog = await api.actions.getCatalog(action.workspace_id);
      buildPaletteFromCatalog(catalog);
    } catch (err) {
      console.error('Failed to load action catalog; palette will be empty', err);
    }
  }

  async function loadMilestones() {
    if (!action?.workspace_id) return;
    try {
      milestones = await api.milestones.getAll({ workspace_id: action.workspace_id, include_global: true }) || [];
    } catch (err) {
      console.error('Failed to load milestones for action editor', err);
      milestones = [];
    }
  }

  async function loadTeams() {
    try {
      teams = await api.teams.getAll() || [];
    } catch (err) {
      console.error('Failed to load teams for action editor', err);
      teams = [];
    }
  }

  async function loadLinkTypes() {
    try {
      linkTypes = await api.linkTypes.getAll() || [];
    } catch (err) {
      console.error('Failed to load link types for action editor', err);
      linkTypes = [];
    }
  }

  async function loadItemTypes() {
    if (!action?.workspace_id) return;
    try {
      itemTypes = await api.itemTypes.getAll({ workspace_id: action.workspace_id }) || [];
    } catch (err) {
      console.error('Failed to load item types for action editor', err);
      itemTypes = [];
    }
  }

  async function loadStatusCategories() {
    try {
      statusCategories = await api.statusCategories.getAll() || [];
    } catch (err) {
      console.error('Failed to load status categories for action editor', err);
      statusCategories = [];
    }
  }

  async function loadAssignableUsers() {
    if (!action?.workspace_id) return;
    try {
      assignableUsers = await api.getAssignableUsers(action.workspace_id) || [];
    } catch (err) {
      console.error('Failed to load users for action editor', err);
      assignableUsers = [];
    }
  }

  onMount(() => {
    if (action?.workspace_id) {
      loadCatalog();
      loadCapabilities('docker_environment');
      loadCapabilities('http_client');
      loadCapabilities('llm_connection');
      loadMilestones();
      loadTeams();
      loadLinkTypes();
      loadItemTypes();
      loadStatusCategories();
      loadAssignableUsers();
    }

    // Live-reload after every AI chat agent run, regardless of tool calls:
    // any agent activity may have touched this action indirectly, so refetch
    // and rehydrate to reflect the latest server state.
    const unsub = agentRuns.subscribe(async () => {
      if (!action?.id) return;
      try {
        const fresh = await api.actions.get(action.workspace_id, action.id);
        actionFlowStore.init(fresh, statuses);
        infoToast(t('actions.aiUpdated', 'Action updated by AI'));
      } catch (err) {
        console.error('Failed to reload action after agent run:', err);
      }
    });
    return unsub;
  });

  function capabilityOptions(type) {
    const empty = [{ value: '', label: t('actions.config.selectCapability') }];
    const list = capabilitiesByType[type] || [];
    if (list.length === 0) {
      return [{ value: '', label: t('actions.config.noCapabilitiesForWorkspace') }];
    }
    return empty.concat(list.map((c) => ({ value: String(c.id), label: c.name })));
  }

  // Block save on invalid or duplicate output field names. doSave() in
  // BaseActionFlowEditor wraps onSave in try/catch + error toast, so throwing
  // here surfaces the message and aborts the save.
  function flowOutputErrors() {
    const errors = [];
    const counts = {};
    for (const { name } of collectOutputFields(actionFlowStore.nodes)) {
      if (!isValidOutputFieldName(name)) {
        errors.push(`Invalid output field name: ${name}`);
      }
      counts[name] = (counts[name] || 0) + 1;
    }
    for (const [name, count] of Object.entries(counts)) {
      if (count > 1) {
        errors.push(`Output field "${name}" is produced by more than one node`);
      }
    }
    return errors;
  }

  async function handleSave(apiData) {
    const errors = flowOutputErrors();
    if (errors.length > 0) {
      throw new Error(errors[0]);
    }
    // Inject actor override before forwarding to the caller. Backend only
    // enforces action.set_actor when the value actually changes vs the stored
    // action, so passing through unchanged is a no-op.
    apiData.actor_user_id = actorUserId;
    apiData.allowed_role_ids = apiData.trigger_type === 'manual' ? allowedRoleIds : [];
    await onSave(apiData);
  }

  function setFieldConfigForSelection(field) {
    if (field.customFieldId) {
      return {
        target: 'custom_field',
        custom_field_id: field.customFieldId,
        field_name: '',
        field_display_name: field.name,
        field_type: field.type || '',
        value: '',
        value_display_name: '',
      };
    }
    const fieldName = field.id === 'milestone' ? 'milestone_ids' : backendFieldName(field);
    const updates = {
      target: 'column',
      custom_field_id: 0,
      field_name: fieldName,
      field_display_name: field.name,
      field_type: field.type || standardFieldTypes[field.id] || '',
      value: '',
      value_display_name: '',
    };
    if (fieldName === 'milestone_ids') updates.value = '[]';
    return updates;
  }

  function isMilestoneSetField(config) {
    return config?.field_name === 'milestone_ids' || config?.field_name === 'milestone_id';
  }

  function getSetFieldMilestoneIDs(config) {
    const value = config?.value;
    if (Array.isArray(value)) return value.map((id) => parseInt(id, 10)).filter(Boolean);
    if (typeof value === 'number') return [value];
    if (typeof value !== 'string' || value.trim() === '') return [];
    try {
      const parsed = JSON.parse(value);
      if (Array.isArray(parsed)) return parsed.map((id) => parseInt(id, 10)).filter(Boolean);
      const parsedID = parseInt(parsed, 10);
      return parsedID ? [parsedID] : [];
    } catch {
      return value.split(',').map((id) => parseInt(id.trim(), 10)).filter(Boolean);
    }
  }

  function updateSetFieldMilestones(nodeId, ids) {
    const safeIDs = Array.isArray(ids) ? ids : [];
    actionFlowStore.updateNodeConfig(nodeId, {
      field_name: 'milestone_ids',
      field_display_name: t('common.milestone', 'Milestone'),
      field_type: 'enum',
      value: JSON.stringify(safeIDs),
      value_display_name: safeIDs
        .map((id) => milestones.find((m) => m.id === id)?.name)
        .filter(Boolean)
        .join(', '),
    });
  }

  function isUserSetField(config) {
    return config?.field_type === 'user' || config?.field_name === 'assignee_id' || config?.field_name === 'creator_id';
  }

  function getSetFieldUserID(config) {
    const raw = config?.value;
    if (raw === null || raw === undefined || raw === '') return null;
    if (typeof raw === 'number') return raw;
    if (typeof raw === 'string') {
      const trimmed = raw.trim();
      if (/^\d+$/.test(trimmed)) return parseInt(trimmed, 10);
      try {
        const parsed = JSON.parse(trimmed);
        if (typeof parsed === 'number') return parsed;
        if (parsed?.id) return parseInt(parsed.id, 10) || null;
      } catch {
        // Template values like {{item.creator_id}} intentionally stay editable in the text input below.
      }
    }
    if (typeof raw === 'object' && raw?.id) return parseInt(raw.id, 10) || null;
    return null;
  }

  function getUserDisplayName(user) {
    if (!user) return '';
    const fullName = `${user.first_name || ''} ${user.last_name || ''}`.trim();
    return fullName || user.username || user.email || `User #${user.id}`;
  }

  function updateSetFieldUser(nodeId, user) {
    actionFlowStore.updateNodeConfig(nodeId, {
      value: user ? String(user.id) : '',
      value_display_name: user ? getUserDisplayName(user) : '',
    });
  }

  // notify_user "specific" recipients: recipients[] holds user-id strings.
  // The "assignee"/"creator" choices live in recipient_type, not this list, so
  // filter to numeric ids when rendering the recipient chips.
  function specificRecipientIds(config) {
    return (config?.recipients || []).filter((r) => /^\d+$/.test(String(r)));
  }

  function recipientDisplayName(idStr) {
    const user = assignableUsers.find((u) => String(u.id) === String(idStr));
    return user ? getUserDisplayName(user) : `#${idStr}`;
  }

  function addRecipient(nodeId, config, user) {
    if (!user?.id) return;
    const current = config?.recipients || [];
    const idStr = String(user.id);
    if (current.includes(idStr)) return;
    actionFlowStore.updateNodeConfig(nodeId, {
      recipient_type: 'specific',
      recipients: [...current, idStr],
    });
  }

  function removeRecipient(nodeId, config, idStr) {
    const current = config?.recipients || [];
    actionFlowStore.updateNodeConfig(nodeId, {
      recipients: current.filter((r) => String(r) !== String(idStr)),
    });
  }

  // Common execution-context fields offered as ai_agent input suggestions
  // alongside any output_field produced by upstream nodes.
  const COMMON_CONTEXT_FIELDS = [
    'item.id', 'item.title', 'item.description', 'item.status',
    'item.assignee_id', 'item.creator_id', 'item.priority',
  ];

  function inputFieldSuggestions(currentNodeId, config) {
    const outputs = collectOutputFields(actionFlowStore.nodes)
      .filter((o) => o.nodeId !== currentNodeId)
      .map((o) => o.name);
    const current = config?.input_fields || [];
    return [...new Set([...COMMON_CONTEXT_FIELDS, ...outputs])].filter((s) => !current.includes(s));
  }

  function addInputField(nodeId, config, name) {
    const v = (name || '').trim();
    if (!v) return;
    const current = config?.input_fields || [];
    if (current.includes(v)) return;
    actionFlowStore.updateNodeConfig(nodeId, { input_fields: [...current, v] });
  }

  function removeInputField(nodeId, config, name) {
    const current = config?.input_fields || [];
    actionFlowStore.updateNodeConfig(nodeId, { input_fields: current.filter((f) => f !== name) });
  }

  // Output-field validation (container_run / http_request / ai_extract /
  // ai_agent): the name becomes a {{context}} variable, so it must be a bare
  // identifier and unique across the flow.
  function outputFieldError(nodeId, config) {
    const name = (config?.output_field || '').trim();
    if (!name) return '';
    if (!isValidOutputFieldName(name)) {
      return 'Use letters, numbers and underscores (must start with a letter or underscore)';
    }
    const dupes = collectOutputFields(actionFlowStore.nodes)
      .filter((o) => o.nodeId !== nodeId && o.name === name);
    if (dupes.length > 0) {
      return 'Another node already produces this output name';
    }
    return '';
  }

  function updateRoundRobinTeam(nodeId, teamId) {
    const parsedTeamId = teamId ? parseInt(teamId, 10) : 0;
    actionFlowStore.updateNodeConfig(nodeId, {
      team_id: parsedTeamId,
      team_name: teams.find((team) => team.id === parsedTeamId)?.name || '',
    });
  }
</script>

<BaseActionFlowEditor
  {action}
  flowStore={actionFlowStore}
  initArgs={[statuses]}
  {nodeTypes}
  {nodePalette}
  {triggerTypes}
  sidebarTitle={t('actions.title')}
  addNodesLabel={t('actions.addNodes')}
  tipsLabel={t('actions.tips')}
  tips={[
    t('actions.tipDragToConnect'),
    t('actions.tipClickToEdit'),
    t('actions.tipConditionBranches'),
  ]}
  nodeConfigLabel={t('actions.nodeConfig')}
  newActionLabel={t('actions.newAction')}
  cancelLabel={t('common.cancel')}
  saveLabel={t('common.save')}
  switchToVerticalLabel={t('actions.switchToVertical')}
  switchToHorizontalLabel={t('actions.switchToHorizontal')}
  saveErrorMessage={t('actions.failedToSave')}
  cancelButtonProps={{
    keyboardHint: getShortcutDisplay('actions', 'cancel'),
    hotkeyConfig: { key: toHotkeyString('actions', 'cancel'), guard: () => !actionFlowStore.saving },
  }}
  saveButtonProps={{
    keyboardHint: getShortcutDisplay('actions', 'save'),
    hotkeyConfig: { key: toHotkeyString('actions', 'save'), guard: () => !actionFlowStore.saving },
  }}
  minimapClass="action-minimap"
  minimapNodeColor={minimapNodeColor}
  minimapNodeStrokeColor={minimapNodeStroke}
  minimapNodeStrokeWidth={2}
  minimapNodeBorderRadius={3}
  minimapMaskColor="var(--action-minimap-mask, rgba(15, 23, 42, 0.55))"
  onSave={handleSave}
  {onCancel}
>
  {#snippet sidebarTop()}
    <h3 class="text-sm font-medium sidebar-title mb-2">{t('actions.runAs')}</h3>
    {#if canSetActor}
      <UserPicker
        bind:value={actorUserId}
        placeholder={t('actions.runAsTriggerUser')}
        showUnassigned={true}
        unassignedLabel={t('actions.runAsTriggerUser')}
        onSelect={(user) => { actorUserId = user?.id ?? null; }}
      />
      <p class="mt-2 text-xs sidebar-hints">{t('actions.runAsHint')}</p>
    {:else if action?.actor_user_id && action?.actor_name}
      <div class="text-xs sidebar-subtitle">
        <div class="font-medium" style="color: var(--ds-text);">{action.actor_name}</div>
        <div class="mt-1">{t('actions.runAsReadonlyHint')}</div>
      </div>
    {:else}
      <p class="text-xs sidebar-hints">{t('actions.runAsTriggerUser')}</p>
    {/if}
  {/snippet}

  {#snippet triggerConfig(selectedNode, store)}
    <div>
      <label for="config-trigger-type" class="block text-xs font-medium mb-1">{t('actions.config.triggerType')}</label>
      <Select
        id="config-trigger-type"
        options={triggerTypes}
        value={selectedNode.data?.triggerType || action?.trigger_type || 'status_transition'}
        onchange={(v) => {
          store.updateNodeData(selectedNode.id, { triggerType: v });
          store.updateTriggerType(v);
        }}
        size="small"
      />
    </div>
    {#if (selectedNode.data?.triggerType || action?.trigger_type) === 'manual'}
      <div
        id="manual-action-role-selector"
        class="pt-4 border-t cascade-option"
        role="group"
        aria-labelledby="manual-action-role-selector-label"
        aria-describedby="manual-action-role-selector-hint"
      >
        <label
          id="manual-action-role-selector-label"
          for="manual-action-role-selector-input"
          class="block text-xs font-medium mb-1"
        >
          {t('actions.manualAccess.label')}
        </label>
        <RolePicker
          bind:value={allowedRoleIds}
          id="manual-action-role-selector-input"
          multiple={true}
          placeholder={t('actions.manualAccess.allEditors')}
          onChange={(roleIDs) => { allowedRoleIds = roleIDs; }}
        />
        <p id="manual-action-role-selector-hint" class="mt-2 text-xs sidebar-hints">
          {allowedRoleIds.length > 0
            ? t('actions.manualAccess.restrictedHint')
            : t('actions.manualAccess.unrestrictedHint')}
        </p>
      </div>
    {/if}
    {#if (selectedNode.data?.triggerType || action?.trigger_type) === 'status_transition'}
      <div>
        <label for="config-from-status" class="block text-xs font-medium mb-1">{t('actions.config.fromStatus')}</label>
        <Select
          id="config-from-status"
          options={[{ value: '', label: t('actions.config.anyStatus') }, ...statuses.map(s => ({ value: s.id, label: s.name }))]}
          value={selectedNode.data?.config?.from_status_id || ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { from_status_id: v ? parseInt(v) : null })}
          size="small"
        />
      </div>
      <div>
        <label for="config-to-status" class="block text-xs font-medium mb-1">{t('actions.config.toStatus')}</label>
        <Select
          id="config-to-status"
          options={[{ value: '', label: t('actions.config.anyStatus') }, ...statuses.map(s => ({ value: s.id, label: s.name }))]}
          value={selectedNode.data?.config?.to_status_id || ''}
          disabled={selectedNode.data?.config?.to_status_category_completed === true}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { to_status_id: v ? parseInt(v) : null })}
          size="small"
        />
      </div>
      <div>
        <Checkbox
          checked={selectedNode.data?.config?.to_status_category_completed === true}
          onchange={(checked) => store.updateNodeConfig(selectedNode.id, {
            to_status_category_completed: checked || undefined,
            ...(checked ? { to_status_id: undefined } : {}),
          })}
          label="Any completed status"
          hint="Fire when moving into any status in a completed category"
          dataTestid="trigger-completed-toggle"
          size="small"
        />
      </div>
    {/if}
    {#if (selectedNode.data?.triggerType || action?.trigger_type) === 'item_updated'}
      <div>
        <div class="block text-xs font-medium mb-1">{t('actions.config.triggerField')}</div>
        <FieldSelector
          placeholder={t('actions.config.anyField')}
          selectedField={getFieldSelectorValue(selectedNode.data?.config)}
          onSelect={(field) => {
            store.updateNodeConfig(selectedNode.id, { field_name: backendFieldName(field) });
          }}
          onClear={() => store.updateNodeConfig(selectedNode.id, { field_name: '' })}
        />
      </div>
    {/if}
    {#if ['item_created', 'item_updated'].includes(selectedNode.data?.triggerType || action?.trigger_type)}
      <div>
        <label for="config-item-type" class="block text-xs font-medium mb-1">Item type</label>
        <Select
          id="config-item-type"
          options={[{ value: '', label: 'Any item type' }, ...itemTypes.map(it => ({ value: it.id, label: it.name }))]}
          value={selectedNode.data?.config?.item_type_id || ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { item_type_id: v ? parseInt(v) : null })}
          size="small"
        />
      </div>
    {/if}
    {#if (selectedNode.data?.triggerType || action?.trigger_type) === 'item_linked'}
      <div>
        <label for="config-trigger-link-type" class="block text-xs font-medium mb-1">Link type</label>
        <Select
          id="config-trigger-link-type"
          options={[{ value: '', label: 'Any link type' }, ...linkTypes.map(lt => ({ value: lt.id, label: lt.name }))]}
          value={selectedNode.data?.config?.link_type_id || ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { link_type_id: v ? parseInt(v) : null })}
          size="small"
        />
      </div>
    {/if}
    <div class="pt-4 border-t cascade-option">
      <Checkbox
        checked={selectedNode.data?.config?.respond_to_cascades || false}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { respond_to_cascades: checked })}
        label={t('actions.trigger.respondToCascades')}
        hint={t('actions.trigger.respondToCascadesHint')}
        size="small"
      />
    </div>
  {/snippet}

  {#snippet nodeConfig(selectedNode, store, _handleDeleteNode)}
    {#snippet outputFieldErrorMsg(nodeId, config)}
      {@const err = outputFieldError(nodeId, config)}
      {#if err}
        <p class="output-field-error" data-testid="output-field-error">{err}</p>
      {/if}
    {/snippet}
    {#if selectedNode.type === 'set_status'}
      <div>
        <label for="config-target-status" class="block text-xs font-medium mb-1">{t('actions.config.targetStatus')}</label>
        <Select
          id="config-target-status"
          options={[{ value: '', label: t('actions.config.selectStatus') }, ...statuses.map(s => ({ value: s.id, label: s.name }))]}
          value={selectedNode.data?.config?.status_id || ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { status_id: parseInt(v) })}
          size="small"
        />
      </div>
    {:else if selectedNode.type === 'set_field'}
      <div>
        <label for="config-set-field-name" class="block text-xs font-medium mb-1">{t('actions.config.fieldName')}</label>
        <FieldSelector
          selectedField={getFieldSelectorValue(selectedNode.data?.config)}
          excludedFieldIds={['itemType', 'createdAt', 'updatedAt', 'labels']}
          onSelect={(field) => {
            store.updateNodeConfig(selectedNode.id, setFieldConfigForSelection(field));
          }}
          onClear={() => store.updateNodeConfig(selectedNode.id, { target: 'column', custom_field_id: 0, field_name: '', field_display_name: '', field_type: '', value: '', value_display_name: '' })}
        />
      </div>
      <div>
        <div class="flex items-center gap-1 mb-1">
          <label for="config-set-field-value" class="block text-xs font-medium">{t('actions.config.value')}</label>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        {#if isMilestoneSetField(selectedNode.data?.config)}
          <MilestoneCombobox
            value={getSetFieldMilestoneIDs(selectedNode.data?.config)}
            workspaceId={action?.workspace_id}
            multiple={true}
            placeholder={t('pickers.selectMilestones', 'Select milestones')}
            onSelect={({ ids }) => updateSetFieldMilestones(selectedNode.id, ids)}
          />
          <p class="text-xs mt-1 sidebar-hints">
            {t('actions.config.milestonePickerHint')}
          </p>
        {:else if isUserSetField(selectedNode.data?.config)}
          <UserPicker
            value={getSetFieldUserID(selectedNode.data?.config)}
            workspaceId={action?.workspace_id}
            placeholder={t('pickers.selectUser')}
            showUnassigned={true}
            onSelect={(user) => updateSetFieldUser(selectedNode.id, user)}
          />
          <p class="text-xs mt-1 sidebar-hints">
            {t('actions.config.userPickerHint')}
          </p>
          <Input
            id="config-set-field-value"
            type="text"
            class="mt-2"
            value={selectedNode.data?.config?.value || ''}
            oninput={(e) => store.updateNodeConfig(selectedNode.id, { value: e.currentTarget.value, value_display_name: '' })}
            placeholder="{'{{'}item.creator_id{'}}'}"
          />
        {:else}
          <Input
            id="config-set-field-value"
            type="text"
            value={selectedNode.data?.config?.value || ''}
            oninput={(e) => store.updateNodeConfig(selectedNode.id, { value: e.currentTarget.value, value_display_name: '' })}
            placeholder="{'{{'}item.creator_id{'}}'}"
          />
        {/if}
      </div>
    {:else if selectedNode.type === 'add_comment'}
      <div>
        <div class="flex items-center gap-1 mb-1">
          <label for="config-comment-content" class="block text-xs font-medium">{t('actions.config.commentContent')}</label>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        <Textarea
          id="config-comment-content"
          rows={4}
          value={selectedNode.data?.config?.content || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { content: e.currentTarget.value })}
          placeholder={t('actions.config.commentPlaceholder')}
          size="small"
        />
      </div>
      <Checkbox
        checked={selectedNode.data?.config?.is_private || false}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { is_private: checked })}
        label={t('actions.config.privateComment')}
        size="small"
      />
    {:else if selectedNode.type === 'condition'}
      <div>
        <label for="config-condition-field" class="block text-xs font-medium mb-1">{t('actions.config.fieldToCheck')}</label>
        <FieldSelector
          selectedField={getFieldSelectorValue(selectedNode.data?.config)}
          onSelect={(field) => store.updateNodeConfig(selectedNode.id, { field_name: backendFieldName(field) })}
          onClear={() => store.updateNodeConfig(selectedNode.id, { field_name: '' })}
        />
      </div>
      <div>
        <label for="config-condition-operator" class="block text-xs font-medium mb-1">{t('actions.config.operator')}</label>
        <Select
          id="config-condition-operator"
          options={[
            { value: 'eq', label: t('actions.operators.equals') },
            { value: 'ne', label: t('actions.operators.notEquals') },
            { value: 'contains', label: t('actions.operators.contains') },
            { value: 'gt', label: t('actions.operators.greaterThan') },
            { value: 'lt', label: t('actions.operators.lessThan') },
            { value: 'is_empty', label: t('actions.operators.isEmpty') },
            { value: 'is_not_empty', label: t('actions.operators.isNotEmpty') }
          ]}
          value={selectedNode.data?.config?.operator || 'eq'}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { operator: v })}
          size="small"
        />
      </div>
      <div>
        <label for="config-condition-value" class="block text-xs font-medium mb-1">{t('actions.config.compareValue')}</label>
        <Input
          id="config-condition-value"
          type="text"
          value={selectedNode.data?.config?.value || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { value: e.currentTarget.value })}
        />
      </div>
    {:else if selectedNode.type === 'notify_user'}
      <div>
        <label for="config-recipient-type" class="block text-xs font-medium mb-1">{t('actions.config.recipientType')}</label>
        <Select
          id="config-recipient-type"
          options={[
            { value: 'assignee', label: t('actions.recipients.assignee') },
            { value: 'creator', label: t('actions.recipients.creator') },
            { value: 'specific', label: t('actions.recipients.specific') }
          ]}
          value={selectedNode.data?.config?.recipient_type || selectedNode.data?.config?.recipients?.[0] || 'assignee'}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { recipient_type: v, recipients: v === 'specific' ? specificRecipientIds(selectedNode.data?.config) : [v] })}
          size="small"
        />
      </div>
      {#if (selectedNode.data?.config?.recipient_type || selectedNode.data?.config?.recipients?.[0]) === 'specific'}
        {@const recipientIds = specificRecipientIds(selectedNode.data?.config)}
        <div>
          <span class="block text-xs font-medium mb-1">Recipients</span>
          {#if recipientIds.length > 0}
            <div class="flex flex-wrap gap-1.5 mb-2">
              {#each recipientIds as idStr (idStr)}
                <span class="chip" data-testid={`notify-recipient-chip-${idStr}`}>
                  {recipientDisplayName(idStr)}
                  <button
                    type="button"
                    class="chip-remove"
                    onclick={() => removeRecipient(selectedNode.id, selectedNode.data?.config, idStr)}
                    aria-label="Remove recipient"
                  >
                    <X class="w-3 h-3" />
                  </button>
                </span>
              {/each}
            </div>
          {/if}
          <div data-testid="notify-recipient-add">
            <UserPicker
              value={null}
              users={assignableUsers}
              workspaceId={action?.workspace_id}
              placeholder="Add recipient"
              showSelectedInTrigger={false}
              allowClear={false}
              onSelect={(user) => addRecipient(selectedNode.id, selectedNode.data?.config, user)}
            />
          </div>
          <p class="text-xs mt-1 sidebar-hints">These specific users are notified.</p>
        </div>
      {/if}
      <div>
        <div class="flex items-center gap-1 mb-1">
          <label for="config-notify-message" class="block text-xs font-medium">{t('actions.config.notifyMessage')}</label>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        <Textarea
          id="config-notify-message"
          rows={4}
          value={selectedNode.data?.config?.message || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { message: e.currentTarget.value })}
          placeholder={t('actions.config.notifyPlaceholder')}
          size="small"
        />
      </div>
      <Checkbox
        checked={selectedNode.data?.config?.include_link ?? true}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { include_link: checked })}
        label={t('actions.config.includeLink')}
        size="small"
      />
    {:else if selectedNode.type === 'update_asset'}
      <UpdateAssetConfigPanel {selectedNode} flowStore={store} bind:showPlaceholderModal />
    {:else if selectedNode.type === 'create_asset'}
      <CreateAssetConfigPanel {selectedNode} flowStore={store} bind:showPlaceholderModal />
    {:else if selectedNode.type === 'create_milestone'}
      <CreateMilestoneConfigPanel {selectedNode} flowStore={store} bind:showPlaceholderModal />
    {:else if selectedNode.type === 'related_items'}
      <div>
        <label for="config-related-relation" class="block text-xs font-medium mb-1">Relation</label>
        <Select
          id="config-related-relation"
          options={[
            { value: 'descendants', label: 'Descendants' },
            { value: 'direct_children', label: 'Direct children' },
            { value: 'ancestors', label: 'Ancestors' },
            { value: 'linked', label: 'Linked items' },
          ]}
          value={selectedNode.data?.config?.relation || 'descendants'}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { relation: v })}
          size="small"
        />
      </div>
      {#if (selectedNode.data?.config?.relation || 'descendants') === 'linked'}
        <div>
          <label for="config-related-link-type" class="block text-xs font-medium mb-1">Link type</label>
          <Select
            id="config-related-link-type"
            options={[{ value: '', label: 'Any link type' }, ...linkTypes.map(lt => ({ value: lt.id, label: lt.name }))]}
            value={selectedNode.data?.config?.link_type_id ?? ''}
            onchange={(v) => store.updateNodeConfig(selectedNode.id, { link_type_id: v ? parseInt(v) : null })}
            size="small"
          />
        </div>
        <div>
          <label for="config-related-direction" class="block text-xs font-medium mb-1">Direction</label>
          <Select
            id="config-related-direction"
            options={[
              { value: 'both', label: 'Both directions' },
              { value: 'outgoing', label: 'Outgoing' },
              { value: 'incoming', label: 'Incoming' },
            ]}
            value={selectedNode.data?.config?.link_direction || 'both'}
            onchange={(v) => store.updateNodeConfig(selectedNode.id, { link_direction: v })}
            size="small"
          />
        </div>
      {/if}
      <!-- cross_workspace is relation-independent in the backend config, so it
           applies to linked relations too, not only the hierarchy relations. -->
      <Checkbox
        checked={selectedNode.data?.config?.cross_workspace || false}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { cross_workspace: checked })}
        label="Cross workspace"
        size="small"
        dataTestid="related-cross-workspace"
      />
      <div>
        <label for="config-related-max-items" class="block text-xs font-medium mb-1">Max items</label>
        <Input
          id="config-related-max-items"
          dataTestid="related-max-items"
          type="number"
          min="0"
          value={selectedNode.data?.config?.max_items || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { max_items: e.currentTarget.value ? parseInt(e.currentTarget.value) : 0 })}
          placeholder="Engine default"
        />
      </div>
    {:else if selectedNode.type === 'round_robin_assign'}
      <div>
        <label for="config-round-robin-team" class="block text-xs font-medium mb-1">Team</label>
        <Select
          id="config-round-robin-team"
          options={[{ value: '', label: 'Select team' }, ...teams.map(team => ({ value: team.id, label: team.name }))]}
          value={selectedNode.data?.config?.team_id || ''}
          onchange={(v) => updateRoundRobinTeam(selectedNode.id, v)}
          size="small"
        />
      </div>
      <Checkbox
        checked={selectedNode.data?.config?.skip_on_leave_members ?? true}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { skip_on_leave_members: checked })}
        label="Skip members on leave"
        size="small"
      />
      <Checkbox
        checked={selectedNode.data?.config?.use_leave_substitutes ?? true}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { use_leave_substitutes: checked })}
        label="Use leave substitutes"
        size="small"
      />
    {:else if selectedNode.type === 'transition_item'}
      {@const target = selectedNode.data?.config?.target || { mode: 'matching_terminal' }}
      <div>
        <label for="config-transition-mode" class="block text-xs font-medium mb-1">Transition target</label>
        <Select
          id="config-transition-mode"
          options={[
            { value: 'matching_terminal', label: 'Match trigger terminal category' },
            { value: 'explicit', label: 'Explicit status' },
            { value: 'category_name', label: 'Terminal by category name' },
          ]}
          value={target.mode || 'matching_terminal'}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { target: { ...target, mode: v } })}
          size="small"
        />
      </div>
      {#if (target.mode || 'matching_terminal') === 'explicit'}
        <div>
          <label for="config-transition-status" class="block text-xs font-medium mb-1">{t('actions.config.targetStatus')}</label>
          <Select
            id="config-transition-status"
            options={[{ value: '', label: t('actions.config.selectStatus') }, ...statuses.map(s => ({ value: s.id, label: s.name }))]}
            value={target.status_id || ''}
            onchange={(v) => store.updateNodeConfig(selectedNode.id, { target: { ...target, status_id: v ? parseInt(v) : 0 } })}
            size="small"
          />
        </div>
      {:else if target.mode === 'category_name'}
        <div>
          <label for="config-transition-category" class="block text-xs font-medium mb-1">Category name</label>
          {#if statusCategories.length > 0}
            <Select
              id="config-transition-category"
              options={[{ value: '', label: 'Select category' }, ...statusCategories.map(c => ({ value: c.name, label: c.name }))]}
              value={target.category_name || ''}
              onchange={(v) => store.updateNodeConfig(selectedNode.id, { target: { ...target, category_name: v } })}
              size="small"
            />
          {:else}
            <Input
              id="config-transition-category"
              type="text"
              value={target.category_name || ''}
              oninput={(e) => store.updateNodeConfig(selectedNode.id, { target: { ...target, category_name: e.currentTarget.value } })}
              placeholder="Done"
            />
          {/if}
        </div>
      {/if}
      <Checkbox
        checked={selectedNode.data?.config?.skip_if_already_matching ?? true}
        onchange={(checked) => store.updateNodeConfig(selectedNode.id, { skip_if_already_matching: checked })}
        label="Skip if already matching"
        size="small"
      />
    {:else if selectedNode.type === 'container_run'}
      <div>
        <label for="config-container-cap" class="block text-xs font-medium mb-1">{t('actions.config.dockerCapability')}</label>
        <Select
          id="config-container-cap"
          options={capabilityOptions('docker_environment')}
          value={selectedNode.data?.config?.capability_id ? String(selectedNode.data.config.capability_id) : ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { capability_id: v ? parseInt(v) : null })}
          size="small"
        />
      </div>
      <div>
        <label for="config-container-output" class="block text-xs font-medium mb-1">{t('actions.config.outputField')}</label>
        <Input
          id="config-container-output"
          type="text"
          value={selectedNode.data?.config?.output_field || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { output_field: e.currentTarget.value })}
          placeholder={t('actions.config.outputFieldPlaceholder')}
        />
        {@render outputFieldErrorMsg(selectedNode.id, selectedNode.data?.config)}
      </div>
      <div>
        <label for="config-container-timeout" class="block text-xs font-medium mb-1">{t('actions.config.timeoutSecs')}</label>
        <Input
          id="config-container-timeout"
          type="number"
          min="1"
          value={selectedNode.data?.config?.timeout_secs || 60}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { timeout_secs: parseInt(e.currentTarget.value) || 60 })}
        />
      </div>
    {:else if selectedNode.type === 'http_request'}
      <div>
        <label for="config-http-cap" class="block text-xs font-medium mb-1">{t('actions.config.httpCapability')}</label>
        <Select
          id="config-http-cap"
          options={capabilityOptions('http_client')}
          value={selectedNode.data?.config?.capability_id ? String(selectedNode.data.config.capability_id) : ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { capability_id: v ? parseInt(v) : undefined })}
          size="small"
        />
      </div>
      <div>
        <label for="config-http-method" class="block text-xs font-medium mb-1">{t('actions.config.httpMethod')}</label>
        <Select
          id="config-http-method"
          options={[
            { value: 'GET', label: 'GET' },
            { value: 'POST', label: 'POST' },
            { value: 'PUT', label: 'PUT' },
            { value: 'PATCH', label: 'PATCH' },
            { value: 'DELETE', label: 'DELETE' },
          ]}
          value={selectedNode.data?.config?.method || 'GET'}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { method: v })}
          size="small"
        />
      </div>
      <div>
        <div class="flex items-center gap-1 mb-1">
          <label for="config-http-url" class="block text-xs font-medium">{t('actions.config.urlTemplate')}</label>
          <button
            onclick={() => showPlaceholderModal = true}
            class="text-[var(--ds-text-subtlest)] hover:text-[var(--ds-interactive)] transition-colors"
            title={t('actions.placeholders.showReference')}
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </div>
        <Input
          id="config-http-url"
          type="text"
          value={selectedNode.data?.config?.url_template || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { url_template: e.currentTarget.value })}
          placeholder="https://example.com/api/items/{'{{'}item.id{'}}'}"
        />
      </div>
      <div>
        <label for="config-http-body" class="block text-xs font-medium mb-1">{t('actions.config.requestBody')}</label>
        <Textarea
          id="config-http-body"
          rows={3}
          value={selectedNode.data?.config?.body || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { body: e.currentTarget.value })}
          placeholder={t('actions.config.requestBodyPlaceholder')}
          size="small"
        />
      </div>
      <div>
        <span class="block text-xs font-medium mb-1">{t('actions.config.httpHeaders')}</span>
        <HttpHeadersEditor
          headers={selectedNode.data?.config?.headers || {}}
          nodeId={selectedNode.id}
          onchange={(headers) => store.updateNodeConfig(selectedNode.id, { headers })}
        />
      </div>
      <div>
        <label for="config-http-output" class="block text-xs font-medium mb-1">{t('actions.config.outputField')}</label>
        <Input
          id="config-http-output"
          type="text"
          value={selectedNode.data?.config?.output_field || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { output_field: e.currentTarget.value })}
          placeholder="response"
        />
        {@render outputFieldErrorMsg(selectedNode.id, selectedNode.data?.config)}
      </div>
    {:else if selectedNode.type === 'ai_extract'}
      <div>
        <label for="config-aix-cap" class="block text-xs font-medium mb-1">{t('actions.config.llmCapability')}</label>
        <Select
          id="config-aix-cap"
          options={capabilityOptions('llm_connection')}
          value={selectedNode.data?.config?.capability_id ? String(selectedNode.data.config.capability_id) : ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { capability_id: v ? parseInt(v) : null })}
          size="small"
        />
      </div>
      <div>
        <label for="config-aix-input" class="block text-xs font-medium mb-1">{t('actions.config.inputField')}</label>
        <Input
          id="config-aix-input"
          type="text"
          value={selectedNode.data?.config?.input_field || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { input_field: e.currentTarget.value })}
          placeholder={t('actions.config.inputFieldPlaceholder')}
        />
      </div>
      <div>
        <label for="config-aix-prompt" class="block text-xs font-medium mb-1">{t('actions.config.aiPrompt')}</label>
        <Textarea
          id="config-aix-prompt"
          rows={4}
          value={selectedNode.data?.config?.prompt || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { prompt: e.currentTarget.value })}
          placeholder={t('actions.config.aiExtractPromptPlaceholder')}
          size="small"
        />
      </div>
      <div>
        <label for="config-aix-schema" class="block text-xs font-medium mb-1">{t('actions.config.outputSchema')}</label>
        <Textarea
          id="config-aix-schema"
          rows={4}
          value={selectedNode.data?.config?.output_schema || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { output_schema: e.currentTarget.value })}
          placeholder={'{"type":"object","properties":{...}}'}
          style="font-family: monospace;"
          size="small"
        />
      </div>
      <div>
        <label for="config-aix-output" class="block text-xs font-medium mb-1">{t('actions.config.outputField')}</label>
        <Input
          id="config-aix-output"
          type="text"
          value={selectedNode.data?.config?.output_field || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { output_field: e.currentTarget.value })}
          placeholder="extracted_data"
        />
        {@render outputFieldErrorMsg(selectedNode.id, selectedNode.data?.config)}
      </div>
    {:else if selectedNode.type === 'ai_agent'}
      <div>
        <label for="config-aia-cap" class="block text-xs font-medium mb-1">{t('actions.config.llmCapability')}</label>
        <Select
          id="config-aia-cap"
          options={capabilityOptions('llm_connection')}
          value={selectedNode.data?.config?.capability_id ? String(selectedNode.data.config.capability_id) : ''}
          onchange={(v) => store.updateNodeConfig(selectedNode.id, { capability_id: v ? parseInt(v) : null })}
          size="small"
        />
      </div>
      <div>
        <label for="config-aia-prompt" class="block text-xs font-medium mb-1">{t('actions.config.systemPrompt')}</label>
        <Textarea
          id="config-aia-prompt"
          rows={4}
          value={selectedNode.data?.config?.prompt || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { prompt: e.currentTarget.value })}
          placeholder={t('actions.config.systemPromptPlaceholder')}
          size="small"
        />
      </div>
      <div>
        <label for="config-aia-input-fields" class="block text-xs font-medium mb-1">{t('actions.config.inputFields')}</label>
        {#if (selectedNode.data?.config?.input_fields || []).length > 0}
          <div class="flex flex-wrap gap-1.5 mb-2">
            {#each selectedNode.data?.config?.input_fields as fieldName (fieldName)}
              <span class="chip" data-testid={`ai-input-chip-${fieldName}`}>
                {fieldName}
                <button
                  type="button"
                  class="chip-remove"
                  onclick={() => removeInputField(selectedNode.id, selectedNode.data?.config, fieldName)}
                  aria-label="Remove input field"
                >
                  <X class="w-3 h-3" />
                </button>
              </span>
            {/each}
          </div>
        {/if}
        <Input
          id="config-aia-input-fields"
          dataTestid="ai-input-field-add"
          type="text"
          list={`ai-input-suggestions-${selectedNode.id}`}
          placeholder={t('actions.config.inputFieldsPlaceholder')}
          onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addInputField(selectedNode.id, selectedNode.data?.config, e.currentTarget.value); e.currentTarget.value = ''; } }}
          onchange={(e) => { if (e.currentTarget.value) { addInputField(selectedNode.id, selectedNode.data?.config, e.currentTarget.value); e.currentTarget.value = ''; } }}
        />
        <datalist id={`ai-input-suggestions-${selectedNode.id}`}>
          {#each inputFieldSuggestions(selectedNode.id, selectedNode.data?.config) as s (s)}
            <option value={s}></option>
          {/each}
        </datalist>
      </div>
      <div>
        <label for="config-aia-tools" class="block text-xs font-medium mb-1">{t('actions.config.agentTools')}</label>
        <p class="text-xs mb-2" style="color: var(--ds-text-subtle);">{t('actions.config.agentToolsHint')}</p>
        {#if (capabilitiesByType.http_client || []).length === 0}
          <p class="text-xs" style="color: var(--ds-text-subtle); font-style: italic;">{t('actions.config.noToolsAvailable')}</p>
        {:else}
          {#each capabilitiesByType.http_client as cap}
            <Checkbox
                checked={(selectedNode.data?.config?.tools || []).includes(String(cap.id))}
                onchange={(checked) => {
                  const current = selectedNode.data?.config?.tools || [];
                  const next = checked
                    ? [...current.filter((id) => id !== String(cap.id)), String(cap.id)]
                    : current.filter((id) => id !== String(cap.id));
                  store.updateNodeConfig(selectedNode.id, { tools: next });
                }}
                label={cap.name}
                class="py-1"
                size="small"
              />
          {/each}
        {/if}
      </div>
      <div>
        <label for="config-aia-max-steps" class="block text-xs font-medium mb-1">{t('actions.config.maxSteps')}</label>
        <Input
          id="config-aia-max-steps"
          type="number"
          min="1"
          max="50"
          value={selectedNode.data?.config?.max_steps || 10}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { max_steps: parseInt(e.currentTarget.value) || 10 })}
        />
      </div>
      <div>
        <label for="config-aia-output" class="block text-xs font-medium mb-1">{t('actions.config.outputField')}</label>
        <Input
          id="config-aia-output"
          type="text"
          value={selectedNode.data?.config?.output_field || ''}
          oninput={(e) => store.updateNodeConfig(selectedNode.id, { output_field: e.currentTarget.value })}
          placeholder="agent_answer"
        />
        {@render outputFieldErrorMsg(selectedNode.id, selectedNode.data?.config)}
      </div>
    {/if}
  {/snippet}
</BaseActionFlowEditor>

{#if showPlaceholderModal}
  <PlaceholderReferenceModal onclose={() => showPlaceholderModal = false} />
{/if}

<style>
  :global(.action-minimap) {
    background-color: var(--ds-surface-raised) !important;
    border: 1px solid var(--ds-border) !important;
    border-radius: 8px !important;
    box-shadow: var(--shadow-md) !important;
    overflow: hidden;
  }

  :global(.action-minimap .svelte-flow__minimap-mask) {
    fill: var(--action-minimap-mask, rgba(15, 23, 42, 0.55));
  }

  .cascade-option {
    border-color: var(--ds-border);
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 4px 2px 8px;
    font-size: 12px;
    border-radius: 9999px;
    background-color: var(--ds-background-neutral);
    color: var(--ds-text);
    border: 1px solid var(--ds-border);
  }

  .chip-remove {
    display: inline-flex;
    align-items: center;
    border-radius: 9999px;
    color: var(--ds-text-subtle);
    cursor: pointer;
  }

  .chip-remove:hover {
    color: var(--ds-icon-danger);
  }

  .output-field-error {
    margin-top: 4px;
    font-size: 11px;
    color: var(--ds-text-danger, #dc2626);
  }
</style>
