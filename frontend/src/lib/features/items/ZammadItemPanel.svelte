<script>
  import { useEventListener } from 'runed';
  import Spinner from '../../components/Spinner.svelte';
  import {
    TicketCheck,
    Plus,
    RefreshCw,
    Loader2,
    ExternalLink,
    AlertTriangle,
    Edit2,
    Trash2,
    ChevronDown,
    ChevronUp,
  } from '@lucide/svelte';
  import { api } from '../../api.js';
  import { isExpectedBackgroundSyncError } from '../../utils/backgroundSync.js';
  import Button from '../../components/Button.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Text from '../../components/Text.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import NativeSelect from '../../components/NativeSelect.svelte';
  import FormField from '../../components/FormField.svelte';
  import Input from '../../components/Input.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { authStore } from '../../stores';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import { safeHref } from '../../utils/sanitize';
  import { formatDateTimeLocale, getUserTimezone } from '../../utils/dateFormatter.js';
  import {
    getZammadObservedValueLabel,
    getZammadStatusAppearance,
    getZammadStatusBucketLabel,
  } from '../../utils/zammadObservations.js';
  import {
    isCurrentZammadMetadataRequest,
    isCurrentZammadPanelContext,
    isUsableZammadGroup,
    isCurrentZammadTimelineRequest,
  } from './zammadPanelContext.js';

  let { itemId, workspaceId, canEdit = false } = $props();

  let connections = $state([]);
  let links = $state([]);
  let metadata = $state({ groups: [], states: [] });
  let editMetadata = $state({ groups: [], states: [] });
  let editOwners = $state([]);
  let loading = $state(true);
  let loadingMetadata = $state(false);
  let loadingEditMetadata = $state(false);
  let loadingEditOwners = $state(false);
  let creating = $state(false);
  let linking = $state(false);
  let savingEdit = $state(false);
  let refreshingId = $state(null);
  let removingId = $state(null);
  let showCreate = $state(false);
  let showEdit = $state(false);
  let dialogMode = $state('create');
  let selectedConnectionId = $state('');
  let selectedGroupId = $state('');
  let ticketNumber = $state('');
  let editingLink = $state(null);
  let selectedEditStateId = $state('');
  let selectedEditGroupId = $state('');
  let selectedEditOwnerId = $state('1');
  let initialEdit = $state({ stateId: '', groupId: '', ownerId: '1' });
  let error = $state('');
  let formError = $state('');
  let editError = $state('');
  let loadVersion = 0;
  let contextVersion = 0;
  let metadataVersion = 0;
  let editVersion = 0;
  let ownersVersion = 0;
  let timelineEvents = $state([]);
  let timelineLoading = $state(false);
  let timelineError = $state('');
  let showTimeline = $state(false);
  let timelineLoaded = $state(false);
  let timelineVersion = 0;
  let timezone = $derived(getUserTimezone(authStore.currentUser));
  const observedTimelineFields = new Set(['status', 'group', 'owner']);
  const loadOutcomes = Object.freeze({
    loaded: 'loaded',
    failed: 'failed',
    superseded: 'superseded',
  });

  let usableConnections = $derived(connections.filter(isConnectionUsable));
  let unavailableConnections = $derived(connections.filter((connection) => !isConnectionUsable(connection)));
  let editGroups = $derived(editMetadata.groups.filter(isUsableZammadGroup));
  let editStates = $derived(editMetadata.states.filter((state) => state.active !== false));
  let editOwnerOptions = $derived([
    { value: '1', label: t('zammad.unassignedOwner') },
    ...editOwners
      .filter((owner) => Number(owner.id) !== 1)
      .map((owner) => ({ value: String(owner.id), label: owner.name })),
  ]);
  let editPayloadAvailable = $derived(Object.keys(editPayload()).length > 0);

  $effect(() => {
    const currentItemId = itemId;
    const currentWorkspaceId = workspaceId;
    const currentVersion = ++contextVersion;
    resetContext();
    void load(currentItemId, currentWorkspaceId, currentVersion);
  });

  useEventListener(() => window, 'item-zammad-links-changed', (/** @type {CustomEvent<{itemId?: number|string}>} */ event) => {
    const id = event?.detail?.itemId;
    if (id == null || String(id) !== String(itemId)) return;
    void load(itemId, workspaceId, contextVersion);
  });

  function isCurrentContext(version, currentItemId = itemId, currentWorkspaceId = workspaceId) {
    return isCurrentZammadPanelContext(
      version,
      contextVersion,
      currentItemId,
      itemId,
      currentWorkspaceId,
      workspaceId,
    );
  }

  function resetContext() {
    loadVersion += 1;
    metadataVersion += 1;
    editVersion += 1;
    ownersVersion += 1;
    timelineVersion += 1;
    connections = [];
    links = [];
    metadata = { groups: [], states: [] };
    editMetadata = { groups: [], states: [] };
    editOwners = [];
    loading = Boolean(itemId && workspaceId);
    loadingMetadata = false;
    loadingEditMetadata = false;
    loadingEditOwners = false;
    creating = false;
    linking = false;
    savingEdit = false;
    refreshingId = null;
    removingId = null;
    showCreate = false;
    showEdit = false;
    dialogMode = 'create';
    selectedConnectionId = '';
    selectedGroupId = '';
    ticketNumber = '';
    editingLink = null;
    selectedEditStateId = '';
    selectedEditGroupId = '';
    selectedEditOwnerId = '1';
    initialEdit = { stateId: '', groupId: '', ownerId: '1' };
    error = '';
    formError = '';
    editError = '';
    timelineEvents = [];
    timelineLoading = false;
    timelineError = '';
    showTimeline = false;
    timelineLoaded = false;
  }

  function isConnectionUsable(connection) {
    if (connection.ready === false || connection.reauthorization_required === true) return false;
    return connection.auth_method !== 'oauth' || connection.oauth_connected === true;
  }

  function usableGroups(connection, loadedMetadata) {
    const allowedIds = (connection?.allowed_groups || []).map((group) => group.id);
    return loadedMetadata.groups.filter((group) => {
      if (!isUsableZammadGroup(group)) return false;
      if (allowedIds.length > 0) return allowedIds.includes(group.id);
      return group.id === connection?.default_group_id || group.name === connection?.default_group_name;
    });
  }

  function selectedConnection() {
    return usableConnections.find((connection) => connection.id === selectedConnectionId);
  }

  function ticketHeading(link) {
    const title = typeof link.ticket_title === 'string' ? link.ticket_title.trim() : '';
    if (title) return title;
    if (link.ticket_number) return t('zammad.ticketNumber', { number: link.ticket_number });
    return t(`zammad.syncState.${link.sync_state}`);
  }

  function ticketStatusLabel(link) {
    const statusName = typeof link.last_status_name === 'string' ? link.last_status_name.trim() : '';
    if (statusName || Number(link.last_status_id) > 0) {
      return getZammadStatusBucketLabel(
        { id: link.last_status_id, name: link.last_status_name },
        t,
      );
    }
    return t(`zammad.syncState.${link.sync_state}`);
  }

  function ticketStatusAppearance(link) {
    if (!link.last_status_id) {
      if (link.sync_state === 'sync_failed') return 'error';
      if (link.sync_state === 'creation_uncertain' || link.sync_state === 'creating' || link.sync_state === 'pending') return 'warning';
    }
    const statusName = typeof link.last_status_name === 'string' ? link.last_status_name.trim() : '';
    if ((Number(link.last_status_id) > 0 || statusName) && typeof link.closed !== 'boolean') {
      return 'default';
    }
    return getZammadStatusAppearance(
      { id: link.last_status_id, name: link.last_status_name },
      link.closed === true,
    );
  }

  function invalidateTimeline() {
    timelineLoaded = false;
    if (showTimeline) void loadTimeline(contextVersion);
  }

  function timelineFieldLabel(field) {
    const fields = {
      status: 'status',
      owner: 'owner',
      group: 'group',
    };
    return t(`zammad.timeline.field.${fields[field]}`);
  }

  function timelineValueLabel(value, field) {
    return getZammadObservedValueLabel(value, t, field);
  }

  function timelineChangeLabel(event) {
    return t('zammad.timeline.change', {
      field: timelineFieldLabel(event.field),
      from: timelineValueLabel(event.old_value, event.field),
      to: timelineValueLabel(event.new_value, event.field),
    });
  }

  async function toggleTimeline() {
    showTimeline = !showTimeline;
    if (showTimeline && !timelineLoaded && !timelineLoading) {
      await loadTimeline(contextVersion);
    }
  }

  async function loadTimeline(version = contextVersion) {
    const currentItemId = itemId;
    const currentWorkspaceId = workspaceId;
    const requestVersion = ++timelineVersion;
    if (!currentItemId) return;
    const isCurrentTimelineRequest = () =>
      requestVersion === timelineVersion &&
      isCurrentZammadTimelineRequest(
        version,
        contextVersion,
        currentItemId,
        itemId,
        currentWorkspaceId,
        workspaceId,
      );
    timelineLoading = true;
    timelineError = '';
    try {
      const response = await api.zammadTickets.history(currentItemId, { limit: 6 });
      if (!isCurrentTimelineRequest()) return;
      timelineEvents = Array.isArray(response?.events)
        ? response.events.filter((event) => observedTimelineFields.has(event?.field)).slice(0, 6)
        : [];
      timelineLoaded = true;
    } catch (err) {
      if (!isCurrentTimelineRequest()) return;
      console.error('Failed to load Zammad timeline:', err);
      timelineEvents = [];
      timelineError = t('zammad.timeline.loadFailed');
    } finally {
      if (isCurrentTimelineRequest()) {
        timelineLoading = false;
      }
    }
  }

  function selectedEditConnection() {
    return usableConnections.find((connection) => connection.id === editingLink?.connection_id);
  }

  function syncActionHint(link) {
    if (link.ticket_id) return '';
    if (link.sync_state === 'sync_failed') return t('zammad.syncFailedNoTicket');
    if (link.sync_state === 'creation_uncertain') return t('zammad.creationUncertainNoTicket');
    return t('zammad.ticketCreationInProgress');
  }

  function addMutationFallback(link) {
    const existing = links.find((entry) => entry.id === link.id);
    const fallback = { ...(existing || {}), ...link };
    const responseTicketId = Number(link.ticket_id) || 0;
    const existingTicketId = Number(existing?.ticket_id) || 0;
    const responseTicketNumber = typeof link.ticket_number === 'string' ? link.ticket_number.trim() : '';
    const existingTicketNumber = typeof existing?.ticket_number === 'string' ? existing.ticket_number.trim() : '';
    const sameTicket = Boolean(existing) && (
      responseTicketId > 0 || existingTicketId > 0
        ? responseTicketId > 0 && responseTicketId === existingTicketId
        : Boolean(responseTicketNumber && responseTicketNumber === existingTicketNumber)
    );
    if (!(typeof link.ticket_title === 'string' && link.ticket_title.trim())) {
      if (sameTicket && typeof existing?.ticket_title === 'string' && existing.ticket_title.trim()) {
        fallback.ticket_title = existing.ticket_title;
      } else {
        delete fallback.ticket_title;
      }
    }
    if (typeof link.closed !== 'boolean') {
      const statusUnchanged = Number(fallback.last_status_id) === Number(existing?.last_status_id);
      if (sameTicket && statusUnchanged && typeof existing?.closed === 'boolean') {
        fallback.closed = existing.closed;
      } else {
        delete fallback.closed;
      }
    }
    links = [fallback, ...links.filter((entry) => entry.id !== link.id)];
    invalidateTimeline();
  }

  function notifyMutationReload(outcome, successMessage) {
    if (outcome === loadOutcomes.failed) errorToast(t('zammad.ticketReloadAfterChangeFailed'));
    else successToast(successMessage);
  }

  async function load(
    currentItemId = itemId,
    currentWorkspaceId = workspaceId,
    version = contextVersion,
    { preserveExisting = false } = {},
  ) {
    const currentVersion = ++loadVersion;
    if (!currentItemId || !currentWorkspaceId) {
      connections = [];
      links = [];
      loading = false;
      error = '';
      return loadOutcomes.loaded;
    }

    loading = true;
    error = '';
    try {
      const [loadedConnections, loadedLinks] = await Promise.all([
        api.zammadConnections.forWorkspace(currentWorkspaceId),
        api.zammadTickets.forItem(currentItemId),
      ]);
      if (currentVersion !== loadVersion || !isCurrentContext(version, currentItemId, currentWorkspaceId)) {
        return loadOutcomes.superseded;
      }
      connections = loadedConnections;
      links = loadedLinks;
      invalidateTimeline();
      return loadOutcomes.loaded;
    } catch (err) {
      if (currentVersion !== loadVersion || !isCurrentContext(version, currentItemId, currentWorkspaceId)) {
        return loadOutcomes.superseded;
      }
      if (isExpectedBackgroundSyncError(err)) return loadOutcomes.superseded;
      console.error('Failed to load Zammad links:', err);
      if (!preserveExisting) error = t('zammad.loadLinksFailed');
      return loadOutcomes.failed;
    } finally {
      if (currentVersion === loadVersion && isCurrentContext(version, currentItemId, currentWorkspaceId)) loading = false;
    }
  }

  async function openCreateDialog() {
    dialogMode = 'create';
    formError = '';
    ticketNumber = '';
    selectedConnectionId = usableConnections[0]?.id || '';
    selectedGroupId = '';
    showCreate = true;
    await loadMetadata(contextVersion);
  }

  function openLinkDialog() {
    metadataVersion += 1;
    loadingMetadata = false;
    metadata = { groups: [], states: [] };
    dialogMode = 'link';
    formError = '';
    ticketNumber = '';
    selectedConnectionId = usableConnections[0]?.id || '';
    selectedGroupId = '';
    showCreate = true;
  }

  function closeCreateDialog() {
    if (creating || linking) return;
    metadataVersion += 1;
    loadingMetadata = false;
    showCreate = false;
    formError = '';
  }

  async function handleCreateConnectionChange() {
    formError = '';
    if (dialogMode === 'create') await loadMetadata(contextVersion);
  }

  async function loadMetadata(version = contextVersion) {
    const requestVersion = ++metadataVersion;
    const connectionId = selectedConnectionId;
    const connection = selectedConnection();
    if (!connection) return;
    const isCurrentRequest = () =>
      isCurrentContext(version) &&
      isCurrentZammadMetadataRequest(
        requestVersion,
        metadataVersion,
        connectionId,
        selectedConnectionId,
        showCreate,
        dialogMode,
      );
    loadingMetadata = true;
    try {
      const loadedMetadata = await api.zammadConnections.metadata(workspaceId, connection.id);
      if (!isCurrentRequest()) return;
      const groups = usableGroups(connection, loadedMetadata);
      metadata = { ...loadedMetadata, groups };
      const defaultGroup = metadata.groups.find(
        (group) => group.id === connection.default_group_id || group.name === connection.default_group_name,
      );
      selectedGroupId = String(defaultGroup?.id || metadata.groups[0]?.id || '');
    } catch (err) {
      if (!isCurrentRequest()) return;
      console.error('Failed to load Zammad metadata:', err);
      formError = t('zammad.loadMetadataFailed');
      metadata = { groups: [], states: [] };
      selectedGroupId = '';
    } finally {
      if (isCurrentRequest()) {
        loadingMetadata = false;
      }
    }
  }

  async function createTicket() {
    const version = contextVersion;
    const currentItemId = itemId;
    const group = metadata.groups.find((entry) => entry.id === Number(selectedGroupId));
    if (!selectedConnectionId || !group) return;
    creating = true;
    formError = '';
    try {
      const link = await api.zammadTickets.create(currentItemId, {
        connection_id: selectedConnectionId,
        group_id: group.id,
      });
      if (!isCurrentContext(version, currentItemId)) return;
      addMutationFallback(link);
      const reloaded = await load(currentItemId, workspaceId, version, { preserveExisting: true });
      if (!isCurrentContext(version, currentItemId)) return;
      showCreate = false;
      notifyMutationReload(
        reloaded,
        link.sync_state === 'linked' ? t('zammad.ticketCreated') : t('zammad.ticketCreationStarted'),
      );
    } catch (err) {
      if (!isCurrentContext(version, currentItemId)) return;
      console.error('Failed to create Zammad ticket:', err);
      formError = err.message || t('zammad.ticketCreateFailed');
      errorToast(t('zammad.ticketCreateFailed'));
      await load(currentItemId, workspaceId, version, { preserveExisting: true });
    } finally {
      if (isCurrentContext(version, currentItemId)) creating = false;
    }
  }

  async function linkExistingTicket() {
    const version = contextVersion;
    const currentItemId = itemId;
    const trimmedTicketNumber = ticketNumber.trim();
    if (!selectedConnectionId || !trimmedTicketNumber) return;
    linking = true;
    formError = '';
    try {
      const link = await api.zammadTickets.link(currentItemId, {
        connection_id: selectedConnectionId,
        ticket_number: trimmedTicketNumber,
      });
      if (!isCurrentContext(version, currentItemId)) return;
      addMutationFallback(link);
      const reloaded = await load(currentItemId, workspaceId, version, { preserveExisting: true });
      if (!isCurrentContext(version, currentItemId)) return;
      showCreate = false;
      notifyMutationReload(reloaded, t('zammad.ticketLinked'));
    } catch (err) {
      if (!isCurrentContext(version, currentItemId)) return;
      console.error('Failed to link existing Zammad ticket:', err);
      formError = err.message || t('zammad.ticketLinkFailed');
      errorToast(t('zammad.ticketLinkFailed'));
    } finally {
      if (isCurrentContext(version, currentItemId)) linking = false;
    }
  }

  async function refresh(link) {
    if (!link.ticket_id) return;
    const version = contextVersion;
    const currentItemId = itemId;
    refreshingId = link.id;
    try {
      await api.zammadTickets.refresh(link.id);
      if (!isCurrentContext(version, currentItemId)) return;
      const reloaded = await load(currentItemId, workspaceId, version, { preserveExisting: true });
      if (!isCurrentContext(version, currentItemId)) return;
      notifyMutationReload(reloaded, t('zammad.ticketRefreshed'));
    } catch (err) {
      if (!isCurrentContext(version, currentItemId)) return;
      console.error('Failed to refresh Zammad ticket:', err);
      errorToast(t('zammad.ticketRefreshFailed'));
      await load(currentItemId, workspaceId, version, { preserveExisting: true });
    } finally {
      if (isCurrentContext(version, currentItemId)) refreshingId = null;
    }
  }

  async function openEditDialog(link) {
    const version = ++editVersion;
    const currentItemId = itemId;
    const currentWorkspaceId = workspaceId;
    const connection = usableConnections.find((entry) => entry.id === link.connection_id);
    if (!connection) {
      errorToast(t('zammad.connectionUnavailable'));
      return;
    }

    editingLink = link;
    editError = '';
    editMetadata = { groups: [], states: [] };
    editOwners = [];
    selectedEditStateId = String(link.last_status_id || '');
    selectedEditGroupId = String(link.group_id || connection.default_group_id || '');
    selectedEditOwnerId = String(link.owner_id || 1);
    showEdit = true;
    loadingEditMetadata = true;
    try {
      const loadedMetadata = await api.zammadConnections.metadata(currentWorkspaceId, connection.id);
      if (version !== editVersion || !isCurrentContext(contextVersion, currentItemId, currentWorkspaceId) || !showEdit || editingLink?.id !== link.id) return;
      editMetadata = { ...loadedMetadata, groups: usableGroups(connection, loadedMetadata) };
      if (!editGroups.some((group) => String(group.id) === selectedEditGroupId)) {
        selectedEditGroupId = String(editGroups[0]?.id || '');
      }
      await loadEditOwners();
      if (version !== editVersion || !isCurrentContext(contextVersion, currentItemId, currentWorkspaceId) || !showEdit || editingLink?.id !== link.id) return;
      initialEdit = {
        stateId: selectedEditStateId,
        groupId: selectedEditGroupId,
        ownerId: selectedEditOwnerId,
      };
    } catch (err) {
      if (version !== editVersion || !isCurrentContext(contextVersion, currentItemId, currentWorkspaceId)) return;
      console.error('Failed to load Zammad ticket metadata:', err);
      editError = t('zammad.loadMetadataFailed');
    } finally {
      if (version === editVersion && isCurrentContext(contextVersion, currentItemId, currentWorkspaceId)) loadingEditMetadata = false;
    }
  }

  function closeEditDialog() {
    if (savingEdit) return;
    showEdit = false;
    editError = '';
    editingLink = null;
  }

  async function loadEditOwners() {
    const version = editVersion;
    const requestVersion = ++ownersVersion;
    const currentItemId = itemId;
    const currentWorkspaceId = workspaceId;
    const connection = selectedEditConnection();
    if (!connection || !selectedEditGroupId) return;
    loadingEditOwners = true;
    try {
      const loadedOwners = await api.zammadConnections.owners(
        currentWorkspaceId,
        connection.id,
        Number(selectedEditGroupId),
      );
      if (requestVersion !== ownersVersion || version !== editVersion || !isCurrentContext(contextVersion, currentItemId, currentWorkspaceId) || !showEdit) return;
      editOwners = loadedOwners;
      if (!editOwners.some((owner) => String(owner.id) === selectedEditOwnerId)) {
        selectedEditOwnerId = '1';
      }
    } catch (err) {
      if (requestVersion !== ownersVersion || version !== editVersion || !isCurrentContext(contextVersion, currentItemId, currentWorkspaceId)) return;
      console.error('Failed to load Zammad owners:', err);
      editError = t('zammad.loadOwnersFailed');
      editOwners = [];
      selectedEditOwnerId = '1';
    } finally {
      if (requestVersion === ownersVersion && version === editVersion && isCurrentContext(contextVersion, currentItemId, currentWorkspaceId)) loadingEditOwners = false;
    }
  }

  async function changeEditGroup() {
    editError = '';
    selectedEditOwnerId = '1';
    await loadEditOwners();
  }

  function editPayload() {
    const payload = {};
    if (selectedEditStateId && selectedEditStateId !== initialEdit.stateId) {
      payload.state_id = Number(selectedEditStateId);
    }
    if (selectedEditGroupId && selectedEditGroupId !== initialEdit.groupId) {
      payload.group_id = Number(selectedEditGroupId);
    }
    if (selectedEditOwnerId && selectedEditOwnerId !== initialEdit.ownerId) {
      payload.owner_id = Number(selectedEditOwnerId);
    }
    return payload;
  }

  async function saveEdit() {
    const context = contextVersion;
    const version = editVersion;
    const currentItemId = itemId;
    const payload = editPayload();
    if (!editingLink || Object.keys(payload).length === 0) return;
    savingEdit = true;
    editError = '';
    try {
      await api.zammadTickets.update(editingLink.id, payload);
      if (version !== editVersion || !isCurrentContext(context, currentItemId)) return;
      showEdit = false;
      const reloaded = await load(currentItemId, workspaceId, context, { preserveExisting: true });
      if (version !== editVersion || !isCurrentContext(context, currentItemId)) return;
      notifyMutationReload(reloaded, t('zammad.ticketUpdated'));
    } catch (err) {
      if (version !== editVersion || !isCurrentContext(context, currentItemId)) return;
      console.error('Failed to update Zammad ticket:', err);
      editError = err.message || t('zammad.ticketUpdateFailed');
      errorToast(t('zammad.ticketUpdateFailed'));
    } finally {
      if (version === editVersion && isCurrentContext(context, currentItemId)) savingEdit = false;
    }
  }

  async function removeLink(link) {
    const version = contextVersion;
    const currentItemId = itemId;
    const currentWorkspaceId = workspaceId;
    const accepted = await confirm({
      title: t('zammad.removeTicketLink'),
      message: t('zammad.removeTicketLinkConfirm', { number: link.ticket_number }),
      confirmText: t('zammad.removeTicketLink'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!accepted) return;
    if (!isCurrentContext(version, currentItemId)) return;

    removingId = link.id;
    try {
      await api.zammadTickets.delete(link.id);
      if (!isCurrentContext(version, currentItemId, currentWorkspaceId)) return;
      links = links.filter((entry) => entry.id !== link.id);
      invalidateTimeline();
      const reloaded = await load(currentItemId, currentWorkspaceId, version, { preserveExisting: true });
      if (!isCurrentContext(version, currentItemId, currentWorkspaceId)) return;
      notifyMutationReload(reloaded, t('zammad.ticketLinkRemoved'));
    } catch (err) {
      if (!isCurrentContext(version, currentItemId, currentWorkspaceId)) return;
      console.error('Failed to remove Zammad ticket link:', err);
      errorToast(t('zammad.ticketLinkRemoveFailed'));
      await load(currentItemId, currentWorkspaceId, version, { preserveExisting: true });
    } finally {
      if (isCurrentContext(version, currentItemId, currentWorkspaceId)) removingId = null;
    }
  }
</script>

{#if loading || connections.length > 0 || links.length > 0}
  <div class="mb-4">
    <div class="border-t my-4" style="border-color: var(--ds-border);"></div>
    <div class="flex items-center justify-between mb-3">
      <div class="flex items-center gap-2">
        <TicketCheck class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        <Text variant="subtle" size="xs" weight="semibold" class="uppercase tracking-wider">{t('zammad.tickets')}</Text>
      </div>
      {#if usableConnections.length > 0 && canEdit}
        <div class="flex items-center gap-1">
          <!-- shortcut-guard-exempt: item-local integration actions are reached from the focused item panel -->
          <Button variant="ghost" size="small" icon={Plus} onclick={openLinkDialog}>{t('zammad.linkExistingTicket')}</Button>
          <Button variant="ghost" size="small" icon={Plus} onclick={openCreateDialog}>{t('zammad.createTicket')}</Button>
        </div>
      {/if}
    </div>

    {#if loading}
      <div class="flex justify-center py-3"><Spinner size="sm" /></div>
    {:else if error}
      <p class="text-xs" style="color: var(--ds-text-danger);">{error}</p>
    {:else}
      {#if unavailableConnections.length > 0}
        <p class="text-xs mb-2" style="color: var(--ds-text-warning);">{t('zammad.connectionUnavailable')}</p>
      {/if}
      {#if links.length === 0}
        <p class="text-xs" style="color: var(--ds-text-subtle);">{t('zammad.noTickets')}</p>
      {:else}
        <div class="space-y-2">
          {#each links as link}
            {@const linkConnectionUsable = usableConnections.some((connection) => connection.id === link.connection_id)}
            <div class="rounded-md border px-3 py-2" style="border-color: var(--ds-border); background-color: var(--ds-background-neutral);" data-testid={`zammad-ticket-card-${link.id}`}>
              <div class="flex items-center gap-2">
                <div class="flex-1 min-w-0">
                  {#if link.ticket_url}
                    <a href={safeHref(link.ticket_url)} target="_blank" rel="noopener noreferrer" class="text-sm hover:underline inline-flex items-center gap-1" style="color: var(--ds-link);">
                      {ticketHeading(link)}<ExternalLink class="w-3 h-3" />
                    </a>
                  {:else}
                    <span class="text-sm">{ticketHeading(link)}</span>
                  {/if}
                  <div class="text-xs mt-1 space-y-0.5" style="color: var(--ds-text-subtle);">
                    <div>{link.connection_name}</div>
                    <div>{t('zammad.group')}: {link.group_name || t('zammad.unknown')}</div>
                    <div>{t('zammad.owner')}: {link.owner_name || t('zammad.unassignedOwner')}</div>
                    <div>{link.last_synced_at ? t('zammad.lastSynced', { time: new Date(link.last_synced_at).toLocaleString() }) : t('zammad.notSynced')}</div>
                  </div>
                </div>
                <Lozenge appearance={ticketStatusAppearance(link)} text={ticketStatusLabel(link)} />
                {#if canEdit}
                  <div class="flex items-center gap-1">
                    {#if linkConnectionUsable && link.ticket_id && link.sync_state !== 'creating'}
                      <button class="p-1 rounded" onclick={() => openEditDialog(link)} disabled={savingEdit} title={t('zammad.editTicket')}>
                        <Edit2 class="w-4 h-4" />
                      </button>
                      <button class="p-1 rounded" onclick={() => refresh(link)} disabled={refreshingId === link.id} title={t('zammad.refreshTicket')}>
                        {#if refreshingId === link.id}<Spinner size="sm" />{:else}<RefreshCw class="w-4 h-4" />{/if}
                      </button>
                    {/if}
                    <button class="p-1 rounded" onclick={() => removeLink(link)} disabled={removingId === link.id} title={t('zammad.removeTicketLink')}>
                      {#if removingId === link.id}<Spinner size="sm" />{:else}<Trash2 class="w-4 h-4" />{/if}
                    </button>
                  </div>
                {/if}
              </div>
              {#if !link.ticket_id && link.sync_state !== 'linked'}
                <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">{syncActionHint(link)}</p>
              {/if}
              {#if link.last_error}
                <div class="flex items-start gap-1.5 text-xs mt-2" style="color: var(--ds-text-danger);">
                  <AlertTriangle class="w-3.5 h-3.5 flex-shrink-0 mt-0.5" />
                  <span>{link.last_error}</span>
                </div>
              {/if}
            </div>
          {/each}
        </div>
        <div class="mt-3 border-t pt-2" style="border-color: var(--ds-border);">
          <button
            class="inline-flex items-center gap-1 text-xs font-medium hover:underline"
            onclick={toggleTimeline}
            aria-expanded={showTimeline}
            aria-controls={`zammad-timeline-${itemId}`}
            style="color: var(--ds-link);"
          >
            {t('zammad.timeline.toggle')}
            {#if showTimeline}<ChevronUp class="h-3.5 w-3.5" aria-hidden="true" />{:else}<ChevronDown class="h-3.5 w-3.5" aria-hidden="true" />{/if}
          </button>
          {#if showTimeline}
            <div id={`zammad-timeline-${itemId}`} class="mt-2">
              <p class="text-xs" style="color: var(--ds-text-subtle);">{t('zammad.timeline.observedHint')}</p>
              {#if timelineLoading}
                <div class="flex py-2"><Loader2 class="h-4 w-4 animate-spin" aria-label={t('common.loading')} /></div>
              {:else if timelineError}
                <div class="flex items-center gap-2 py-2 text-xs" role="status" style="color: var(--ds-text-danger);">
                  <span>{timelineError}</span>
                  <button class="underline" onclick={() => loadTimeline(contextVersion)}>{t('common.retry')}</button>
                </div>
              {:else if timelineEvents.length === 0}
                <p class="py-2 text-xs" style="color: var(--ds-text-subtle);">{t('zammad.timeline.empty')}</p>
              {:else}
                <ol class="mt-2 space-y-2">
                  {#each timelineEvents as event (event.id)}
                    <li class="flex items-start gap-2 text-xs">
                      <TicketCheck class="mt-0.5 h-3.5 w-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" aria-hidden="true" />
                      <div>
                        <span class="font-medium" style="color: var(--ds-text);">{t('zammad.ticketNumber', { number: event.ticket_number })}</span>
                        <p style="color: var(--ds-text);">{timelineChangeLabel(event)}</p>
                        <time datetime={event.observed_at} style="color: var(--ds-text-subtle);">{formatDateTimeLocale(event.observed_at, timezone)}</time>
                      </div>
                    </li>
                  {/each}
                </ol>
              {/if}
            </div>
          {/if}
        </div>
      {/if}
    {/if}
  </div>
{/if}

<Modal bind:isOpen={showCreate} preventClose={creating || linking} closeOnBackdropClick={!creating && !linking}>
  <ModalHeader title={dialogMode === 'create' ? t('zammad.createTicket') : t('zammad.linkExistingTicket')} onclose={closeCreateDialog} />
  <div class="p-4 space-y-4">
    <p class="text-sm" style="color: var(--ds-text-subtle);">
      {dialogMode === 'create' ? t('zammad.createTicketConfirm') : t('zammad.linkExistingTicketConfirm')}
    </p>
    {#if formError}
      <p class="text-sm" style="color: var(--ds-text-danger);">{formError}</p>
    {/if}
    <FormField label={t('zammad.connection')} required>
      <NativeSelect
        bind:value={selectedConnectionId}
        options={usableConnections.map((connection) => ({ value: connection.id, label: connection.name }))}
        onchange={handleCreateConnectionChange}
      />
    </FormField>
    {#if dialogMode === 'create'}
      <FormField label={t('zammad.group')} required>
        {#if loadingMetadata}
          <Spinner size="sm" />
        {:else}
          <NativeSelect
            bind:value={selectedGroupId}
            options={metadata.groups.map((group) => ({ value: String(group.id), label: group.name }))}
            disabled={metadata.groups.length === 0}
          />
        {/if}
      </FormField>
    {:else}
      <FormField label={t('zammad.ticketNumberLabel')} required>
        <Input bind:value={ticketNumber} placeholder={t('zammad.ticketNumberPlaceholder')} disabled={linking} />
      </FormField>
    {/if}
    <div class="flex justify-end gap-2">
      <Button variant="ghost" onclick={closeCreateDialog} disabled={creating || linking}>{t('common.cancel')}</Button>
      {#if dialogMode === 'create'}
        <Button variant="primary" onclick={createTicket} disabled={creating || loadingMetadata || !selectedConnectionId || !selectedGroupId}>
          {#if creating}<Spinner size="sm" />{/if}
          {t('zammad.createTicket')}
        </Button>
      {:else}
        <Button variant="primary" onclick={linkExistingTicket} disabled={linking || !selectedConnectionId || !ticketNumber.trim()}>
          {#if linking}<Spinner size="sm" />{/if}
          {t('zammad.linkExistingTicket')}
        </Button>
      {/if}
    </div>
  </div>
</Modal>

<Modal bind:isOpen={showEdit} preventClose={savingEdit} closeOnBackdropClick={!savingEdit}>
  <ModalHeader title={t('zammad.editTicket')} onclose={closeEditDialog} />
  <div class="p-4 space-y-4">
    {#if editError}
      <p class="text-sm" style="color: var(--ds-text-danger);">{editError}</p>
    {/if}
    {#if loadingEditMetadata}
      <div class="flex justify-center py-3"><Spinner size="sm" /></div>
    {:else}
      <FormField label={t('zammad.status')}>
        <NativeSelect
          bind:value={selectedEditStateId}
          options={editStates.map((state) => ({ value: String(state.id), label: state.name }))}
          placeholder={t('zammad.leaveUnchanged')}
          disabled={savingEdit || editStates.length === 0}
        />
      </FormField>
      <FormField label={t('zammad.group')}>
        <NativeSelect
          bind:value={selectedEditGroupId}
          options={editGroups.map((group) => ({ value: String(group.id), label: group.name }))}
          disabled={savingEdit || editGroups.length === 0}
          onchange={changeEditGroup}
        />
      </FormField>
      <FormField label={t('zammad.owner')}>
        {#if loadingEditOwners}
          <Spinner size="sm" />
        {:else}
          <NativeSelect
            bind:value={selectedEditOwnerId}
            options={editOwnerOptions}
            disabled={savingEdit || !selectedEditGroupId}
          />
        {/if}
      </FormField>
    {/if}
    <div class="flex justify-end gap-2">
      <Button variant="ghost" onclick={closeEditDialog} disabled={savingEdit}>{t('common.cancel')}</Button>
      <Button variant="primary" onclick={saveEdit} disabled={savingEdit || loadingEditMetadata || loadingEditOwners || !editPayloadAvailable}>
        {#if savingEdit}<Spinner size="sm" />{/if}
        {t('common.save')}
      </Button>
    </div>
  </div>
</Modal>
