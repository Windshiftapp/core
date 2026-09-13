<script>
  import { Star, Play, Loader, ChevronDown, ChevronRight, GitPullRequest, Bot, RefreshCw, Plus, Check, Pencil } from '@lucide/svelte';
  import { api } from '../api.js';
  import { loadMobileItemDetailSummary } from './mobileItemDetailData.js';
  import { agentRuns as agentRunBus } from '../stores/agentRuns.svelte.js';
  import { navigate } from '../router.js';
  import { notificationActions } from '../stores/notifications.js';
  import { errorToast, infoToast } from '../stores/toasts.svelte.js';
  import { timerStore } from '../stores/timerStore.svelte.js';
  import { useWorkItemPoller } from '../composables/useWorkItemPoller.svelte.js';
  import { useItemEventStream } from '../composables/useItemEventStream.svelte.js';
  import { itemLiveUpdates } from '../stores/itemLiveUpdates.svelte.js';
  import { usePullToRefresh } from '../composables/usePullToRefresh.svelte.js';
  import { formatDateOnly } from '../utils/dateFormatter.js';
  import { formatItemKey } from '../utils/itemKey.js';
  import { workspacesStore } from '../stores';
  import MobileHeader from './MobileHeader.svelte';
  import MobileItemRow from './MobileItemRow.svelte';
  import MobileOptionSheet from './MobileOptionSheet.svelte';
  import StatusPill from '../components/StatusPill.svelte';
  import Comments from '../features/items/Comments.svelte';
  import ItemSCMLinks from '../features/items/ItemSCMLinks.svelte';
  import ItemAgentLog from '../features/items/ItemAgentLog.svelte';
  import Avatar from '../components/Avatar.svelte';
  import SafeMarkdown from '../components/SafeMarkdown.svelte';

  let { itemId } = $props();

  let item = $state(null);
  let loading = $state(true);
  let errored = $state(false);
  let transitions = $state([]);
  let transitioning = $state(false);
  let isWatching = $state(false);
  let watchBusy = $state(false);

  // Workflow-less personal tasks use the permitted transition endpoint to toggle
  // globally stable Open/Done IDs, matching desktop and PWA personal views.
  const PERSONAL_STATUS_OPEN = 1;
  const PERSONAL_STATUS_DONE = 3;
  let personalTaskCount = $state(0);
  let startingTimer = $state(false);
  let children = $state([]);
  let ancestors = $state([]);
  // One-level-deeper item types allowed as children; empty hides Add sub-item.
  let availableSubIssueTypes = $state([]);

  // Property editing happens in bottom sheets (the mobile replacement for the
  // desktop floating pickers). Status options come from the workflow's
  // available-transitions endpoint; assignee options load lazily from the
  // workspace-scoped assignable-users endpoint on first open.
  let statusSheetOpen = $state(false);
  let assigneeSheetOpen = $state(false);
  let assigneeOptions = $state(null);
  let assigneeLoading = $state(false);

  function userLabel(user) {
    if (!user) return '';
    return `${user.first_name || ''} ${user.last_name || ''}`.trim() || user.email || user.username || '';
  }

  async function openAssigneeSheet() {
    assigneeSheetOpen = true;
    if (assigneeOptions) return;
    assigneeLoading = true;
    try {
      assigneeOptions = (await api.getAssignableUsers(item.workspace_id)) ?? [];
    } catch (err) {
      console.error('Failed to load assignable users:', err);
      errorToast('Could not load assignees.');
      assigneeOptions = [];
    } finally {
      assigneeLoading = false;
    }
  }
  // Prevent prior-item loads from writing stale state during in-place navigation.
  let loadToken = 0;

  // Load SCM bodies and agent logs only when their available panels expand.
  let scmAvailable = $state(false);
  let hasAgentRuns = $state(false);
  let scmOpen = $state(false);
  let agentOpen = $state(false);

  const itemKey = $derived(formatItemKey(item) ?? '');
  const projectId = $derived(item?.time_project_id ?? item?.effective_project_id ?? null);
  const canStartTimer = $derived(!!item && !timerStore.hasActive && !!projectId);
  const canCreateChild = $derived(availableSubIssueTypes.length > 0);

  // A personal task is one that lives in the user's personal (workflow-less)
  // workspace. workspacesStore loads the workspace list eagerly in MobileShell;
  // personalWorkspace is fetched on demand once an item resolves here.
  const personalWorkspaceId = $derived($workspacesStore?.personalWorkspace?.id ?? null);
  const isPersonalItem = $derived(!!item && personalWorkspaceId != null && item.workspace_id === personalWorkspaceId);

  function normalizeChild(c) {
    return {
      itemId: c.id,
      itemKey: formatItemKey(c),
      title: c.title,
      statusName: c.status_name,
      statusColor: c.status_color,
      priorityName: c.priority_name,
      priorityColor: c.priority_color,
      dueDate: c.due_date || null,
    };
  }

  async function loadItem(id, token) {
    loading = true;
    errored = false;
    try {
      const summary = await loadMobileItemDetailSummary(id);
      if (token !== loadToken) return;
      item = summary?.item ?? null;
      transitions = summary?.transitions?.available_transitions ?? [];
      isWatching = summary?.watching || false;
      personalTaskCount = summary?.personal_task_count ?? 0;
      const childItems = Array.isArray(summary?.children) ? summary.children : [];
      children = childItems.filter((child) => child?.id).map(normalizeChild);
      ancestors = Array.isArray(summary?.ancestors)
        ? summary.ancestors.filter((ancestor) => ancestor?.id)
        : [];
      availableSubIssueTypes = summary?.available_sub_issue_types ?? [];
      scmAvailable = summary?.scm_available || false;
      hasAgentRuns = summary?.has_agent_runs || false;
      if (!item) throw new Error('Item detail summary did not include an item');
    } catch (err) {
      if (token !== loadToken) return;
      console.error('Failed to load item:', err);
      errored = true;
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  async function retryLoadItem() {
    if (itemId == null || loading) return;
    const token = ++loadToken;
    item = null;
    await loadItem(itemId, token);
    if (token === loadToken && !errored) {
      notificationActions.markItemAsRead(itemId);
    }
  }

  async function refreshTransitionState(id, token) {
    try {
      const res = await api.items.getAvailableStatusTransitions(id);
      if (token === loadToken) transitions = res?.available_transitions ?? [];
    } catch (err) {
      console.error('Failed to load transitions:', err);
    }
  }

  async function changeStatus(statusId) {
    if (transitioning) return;
    transitioning = true;
    try {
      const updated = await api.items.transition(itemId, statusId);
      item = { ...item, ...updated };
      await refreshTransitionState(itemId, loadToken);
    } catch (err) {
      console.error('Failed to transition item:', err);
    } finally {
      transitioning = false;
    }
  }

  // Personal-workspace (workflow-less) tasks get no available status
  // transitions, so the workflow status picker would be permanently disabled
  // and the task could never be completed. Mirror the PWA's PersonalView: a
  // plain Done checkbox that toggles status_id between Open (1) and Done (3)
  // via the existing transition endpoint (which permits any transition for a
  // workflow-less workspace). See WI-537.
  const personalIsDone = $derived(item?.status_id === PERSONAL_STATUS_DONE);

  async function togglePersonalDone() {
    if (transitioning) return;
    const target = personalIsDone ? PERSONAL_STATUS_OPEN : PERSONAL_STATUS_DONE;
    transitioning = true;
    try {
      const updated = await api.items.transition(itemId, target);
      item = { ...item, ...updated };
    } catch (err) {
      console.error('Failed to toggle personal task:', err);
    } finally {
      transitioning = false;
    }
  }

  async function updateAssignee(user) {
    const assigneeId = user?.id ?? null;
    if (assigneeId === (item.assignee_id ?? null)) return;
    try {
      const updated = await api.items.update(itemId, { assignee_id: assigneeId });
      const name = user
        ? `${user.first_name || ''} ${user.last_name || ''}`.trim() || user.email || user.username || ''
        : null;
      item = { ...item, ...updated, assignee_id: assigneeId, assignee_name: name, assignee_avatar: user?.avatar_url ?? null };
    } catch (err) {
      console.error('Failed to update assignee:', err);
    }
  }

  async function toggleWatch() {
    if (watchBusy) return;
    watchBusy = true;
    try {
      if (isWatching) {
        await api.items.removeWatch(itemId);
        isWatching = false;
      } else {
        await api.items.addWatch(itemId);
        isWatching = true;
      }
    } catch (err) {
      console.error('Failed to toggle watch:', err);
    } finally {
      watchBusy = false;
    }
  }

  async function startTimer() {
    if (!canStartTimer || startingTimer) return;
    startingTimer = true;
    try {
      await timerStore.start({
        workspace_id: item.workspace_id,
        item_id: item.id,
        project_id: projectId,
        description: `Working on ${item.title}`,
      });
      navigate('/m/timer');
    } catch (err) {
      console.error('Failed to start timer:', err);
    } finally {
      startingTimer = false;
    }
  }

  function openCreateChild() {
    if (!canCreateChild) return;
    // Dedicated create page with the parent pinned via query param; returning
    // to the detail remounts this view and picks up the new sub-item.
    navigate(`/m/new?parent=${itemId}`);
  }

  // Reload whenever the item id changes — the component is not remounted when
  // navigating item → item (e.g. tapping a sub-item), so onMount wouldn't fire.
  $effect(() => {
    const id = itemId;
    if (id == null) return;
    const token = ++loadToken;
    // Reset transient state for the incoming item.
    item = null;
    transitions = [];
    isWatching = false;
    personalTaskCount = 0;
    children = [];
    ancestors = [];
    scmAvailable = false;
    hasAgentRuns = false;
    scmOpen = false;
    agentOpen = false;
    availableSubIssueTypes = [];
    statusSheetOpen = false;
    assigneeSheetOpen = false;
    loadItem(id, token).then(() => {
      if (token === loadToken && !errored) {
        // Clear notifications pointing at this item — viewing an item should
        // mark its notifications read regardless of entry point (PWA push,
        // deep link), not only when opened from the notification list.
        notificationActions.markItemAsRead(id);
      }
    });
  });

  // Ensure the personal workspace is known so isPersonalItem can resolve.
  // workspacesStore loads the workspace list eagerly in MobileShell; the
  // personal workspace is fetched on demand here. Idempotent / guarded.
  $effect(() => {
    if (!personalWorkspaceId) workspacesStore.loadPersonalWorkspace?.();
  });

  // Silent refresh (no loading flash) of the fields that change in place —
  // mirrors the desktop refreshCurrentItem: item record, available transitions
  // (status may have changed), sub-items, and watch state.
  async function refresh() {
    const token = loadToken;
    if (itemId == null || !item || transitioning) return;
    try {
      const fresh = await api.items.get(itemId);
      if (token !== loadToken) return;
      item = { ...item, ...fresh };
    } catch (err) {
      // 404 => the item was deleted out from under us; leave the stale view.
      if (err?.status === 404) handleDeleted();
      return;
    }
    try {
      const res = await api.items.getAvailableStatusTransitions(itemId);
      if (token === loadToken) transitions = res?.available_transitions ?? [];
    } catch { /* keep prior */ }
    try {
      const res = await api.items.getChildren(itemId);
      const list = Array.isArray(res) ? res : (res?.items ?? []);
      if (token === loadToken) children = list.filter((c) => c?.id).map(normalizeChild);
    } catch { /* keep prior */ }
    try {
      const res = await api.items.getWatchStatus(itemId);
      if (token === loadToken) isWatching = res?.watching || false;
    } catch { /* keep prior */ }
  }

  // Match desktop freshness: adaptive background poll (30s active / 5m idle) for
  // changes made elsewhere (other users, automations, workflow side effects),
  // plus an instant refresh when an AI run completes (chatStore emits on the bus).
  // While the SSE stream is healthy the poll is demoted (WI-484); it resumes if
  // the stream drops or is unsupported.
  useWorkItemPoller(() => refresh(), { enabled: () => !itemLiveUpdates.isLive(itemId) });
  $effect(() => agentRunBus.subscribe(() => refresh()));

  // Live updates (WI-484). Mobile reloads via refresh() (item record,
  // transitions, children, watch) for every granular kind — the same work the
  // poll did — and dispatches the comment event the embedded Comments listens
  // for. connect/reconnect/stale also run refresh() to reconcile.
  useItemEventStream(() => itemId, {
    // Full reconcile also refreshes the embedded Comments (separate component),
    // so a comment that arrived before the stream connected isn't missed.
    onReconcile: () => {
      refresh();
      window.dispatchEvent(new CustomEvent('item-comments-changed', { detail: { itemId } }));
      window.dispatchEvent(new CustomEvent('item-scm-links-changed', { detail: { itemId } }));
    },
    onItem: () => refresh(),
    onChildren: () => refresh(),
    onLinks: () => {
      refresh();
      window.dispatchEvent(new CustomEvent('item-scm-links-changed', { detail: { itemId } }));
    },
    onDeleted: () => handleDeleted(),
    onComment: () => window.dispatchEvent(new CustomEvent('item-comments-changed', { detail: { itemId } })),
  });

  // The viewed item was deleted elsewhere: toast and leave the now-stale detail.
  function handleDeleted() {
    infoToast('This item was deleted.');
    if (window.history.length > 1) window.history.back();
    else navigate('/m');
  }

  // Pull-to-refresh: dragging the canvas down from the top triggers a manual
  // reload (same silent refresh() the background poller uses). Listeners attach
  // to the nearest scrollable ancestor (the mobile scroll surface) so the
  // scrollTop check + overscroll-suppression act on the real scroller.
  let detailEl = $state(null);
  function getScrollContainer() {
    // Walk up from the detail content to the scroll surface owned by MobileShell.
    let el = detailEl?.parentElement;
    while (el) {
      if (getComputedStyle(el).overflowY === 'auto' || getComputedStyle(el).overflowY === 'scroll') {
        return el;
      }
      el = el.parentElement;
    }
    return null;
  }
  const ptr = usePullToRefresh(getScrollContainer, () => refresh());
</script>

<MobileHeader title={itemKey} onback={() => (window.history.length > 1 ? window.history.back() : navigate('/m'))} />

{#if loading}
  <div class="center" data-testid="detail-loading"><Loader class="spin" size={22} /></div>
{:else if errored || !item}
  <div class="msg" data-testid="detail-error">
    <p>Couldn't load this item.</p>
    <button class="retry" onclick={retryLoadItem} disabled={loading} type="button">Retry</button>
  </div>
{:else}
  <div
    class="detail"
    class:pulling={ptr.pulling || ptr.refreshing}
    bind:this={detailEl}
    data-testid="mobile-item-detail"
    style:transform={ptr.pullDistance > 0 || ptr.refreshing ? `translateY(${ptr.refreshing ? ptr.threshold : ptr.pullDistance}px)` : ''}
    style:transition={ptr.pulling ? 'none' : ''}
  >
    <!-- Pull-to-refresh indicator. Sits above the content, growing as the user
         drags; spins while the reload is in flight. -->
    <div
      class="ptr-indicator"
      class:ready={ptr.pullDistance >= ptr.threshold}
      data-testid="detail-pull-indicator"
      aria-hidden={ptr.pulling || ptr.refreshing ? 'true' : 'false'}
    >
      {#if ptr.refreshing}
        <Loader size={18} class="spin" />
      {:else}
        <span
          class="ptr-arrow"
          style:transform={ptr.pullDistance > 0 ? `rotate(${Math.min(ptr.pullDistance * 3, 360)}deg)` : ''}
        >
          <RefreshCw size={18} />
        </span>
      {/if}
    </div>

    {#if ancestors.length > 0}
      <nav class="breadcrumb" data-testid="detail-breadcrumb" aria-label="Parent items">
        {#each ancestors as anc, i (anc.id)}
          {#if i > 0}<ChevronRight size={13} class="bc-sep" />{/if}
          <button class="bc-link" onclick={() => navigate(`/m/items/${anc.id}`)} data-testid="breadcrumb-link" type="button">
            {formatItemKey(anc) ?? anc.title}
          </button>
        {/each}
      </nav>
    {/if}

    {#if item.item_type_name}
      <div class="status-line"><span class="type">{item.item_type_name}</span></div>
    {/if}

    <div class="title-row">
      <h1 class="title" data-testid="detail-title">{item.title}</h1>
      <button
        class="edit-btn"
        onclick={() => navigate(`/m/items/${item.id}/edit`)}
        data-testid="detail-edit"
        aria-label="Edit title and description"
        type="button"
      >
        <Pencil size={16} />
        <span>Edit</span>
      </button>
    </div>

    <!-- Status + assignee rows open bottom sheets. Status options come from
         the workflow's available-transitions endpoint (not hardcoded), so
         custom workflows and custom statuses work as-is. Assignee users come
         from the workspace-scoped assignable-users endpoint. -->
    <div class="fields">
      {#if isPersonalItem}
        <!-- Personal (workflow-less) tasks: the available-transitions endpoint
             returns nothing, so a status picker would be permanently disabled
             and the task could never be completed. Mirror the desktop personal
             task view instead: a Done toggle. See WI-537. -->
        <button
          class="field"
          onclick={togglePersonalDone}
          disabled={transitioning}
          data-testid="personal-done-toggle"
          aria-pressed={personalIsDone}
          type="button"
        >
          <span class="field-label">Status</span>
          <span class="field-value" data-testid="detail-status">
            <span class="done-check" class:done={personalIsDone} aria-hidden="true">
              {#if personalIsDone}<Check size={12} strokeWidth={3} />{/if}
            </span>
            {personalIsDone ? 'Done' : (item.status_name || 'Open')}
          </span>
        </button>
      {:else}
      <button
        class="field"
        onclick={() => (statusSheetOpen = true)}
        disabled={transitioning || transitions.length === 0}
        data-testid="status-picker-trigger"
        type="button"
      >
        <span class="field-label">Status</span>
        <span class="field-value" data-testid="detail-status">
          <StatusPill name={item.status_name} color={item.status_color} />
          <ChevronDown size={16} class="chev" />
        </span>
      </button>
      {/if}

      <button
        class="field"
        onclick={openAssigneeSheet}
        data-testid="assignee-picker-trigger"
        type="button"
      >
        <span class="field-label">Assignee</span>
        <span class="field-value">
          {#if item.assignee_id && item.assignee_name}
            <Avatar src={item.assignee_avatar} name={item.assignee_name} size="xs" variant="teal" />
            <span class="assignee-name">{item.assignee_name}</span>
          {:else}
            <span class="muted">Unassigned</span>
          {/if}
          <ChevronDown size={16} class="chev" />
        </span>
      </button>
    </div>

    {#if item.description}
      <div class="html-content desc" data-testid="detail-description">
        <SafeMarkdown html={item.description_html} source={item.description} />
      </div>
    {/if}

    <!-- Meta -->
    {#if item.due_date || personalTaskCount > 0}
      <dl class="meta">
        {#if item.due_date}
          <div><dt>Due</dt><dd>{formatDateOnly(item.due_date)}</dd></div>
        {/if}
        {#if personalTaskCount > 0}
          <div><dt>Personal tasks</dt><dd>{personalTaskCount} linked</dd></div>
        {/if}
      </dl>
    {/if}

    <!-- Actions -->
    <div class="actions">
      <button class="act" class:on={isWatching} onclick={toggleWatch} disabled={watchBusy} data-testid="detail-watch" type="button">
        <Star size={16} fill={isWatching ? 'currentColor' : 'none'} />
        {isWatching ? 'Watching' : 'Watch'}
      </button>
      {#if canStartTimer}
        <button class="act" onclick={startTimer} disabled={startingTimer} data-testid="detail-start-timer" type="button">
          <Play size={16} /> Start timer
        </button>
      {/if}
    </div>

    <!-- Sub-items. Shown whenever a child type is allowed for this item's
         hierarchy level, so the "Add sub-item" affordance is reachable even
         before any children exist (mirrors the desktop createChild gate). -->
    {#if canCreateChild}
      <section class="subitems" data-testid="detail-subitems">
        <h2 class="section-title">
          Sub-items {#if children.length > 0}<span class="count">{children.length}</span>{/if}
          <button
            class="add-child"
            onclick={openCreateChild}
            data-testid="detail-add-sub-item"
            type="button"
            aria-label="Add sub-item"
          >
            <Plus size={16} /> Add
          </button>
        </h2>
        {#if children.length > 0}
          <div class="subitems-list">
            {#each children as child (child.itemId)}
              <MobileItemRow {...child} />
            {/each}
          </div>
        {/if}
      </section>
    {/if}

    <!-- Commits & pull requests (collapsible; mounts on open) -->
    {#if scmAvailable}
      <section class="panel" data-testid="scm-panel">
        <button class="panel-head" onclick={() => (scmOpen = !scmOpen)} aria-expanded={scmOpen} data-testid="scm-panel-toggle" type="button">
          <span class="panel-title"><GitPullRequest size={16} /> Commits &amp; pull requests</span>
          <ChevronDown size={18} class={scmOpen ? 'chev open' : 'chev'} />
        </button>
        {#if scmOpen}
          <div class="panel-body" data-testid="scm-panel-body">
            <!-- Read-only on mobile: SCM write actions stay on desktop, so the
                 create handlers are no-ops (also satisfies required props). -->
            <ItemSCMLinks itemId={item.id} onaddlink={() => {}} oncreatebranch={() => {}} oncreatepr={() => {}} />
          </div>
        {/if}
      </section>
    {/if}

    <!-- Coding agent (collapsible; only shown when a session exists) -->
    {#if hasAgentRuns}
      <section class="panel" data-testid="agent-panel">
        <button class="panel-head" onclick={() => (agentOpen = !agentOpen)} aria-expanded={agentOpen} data-testid="agent-panel-toggle" type="button">
          <span class="panel-title"><Bot size={16} /> Coding agent</span>
          <ChevronDown size={18} class={agentOpen ? 'chev open' : 'chev'} />
        </button>
        {#if agentOpen}
          <div class="panel-body" data-testid="agent-panel-body">
            <ItemAgentLog itemId={item.id} workspaceId={item.workspace_id} />
          </div>
        {/if}
      </section>
    {/if}

    <!-- Comments. Keyed on itemId so it reloads when navigating item → item. -->
    <section class="comments">
      {#key itemId}
        <Comments {itemId} workspaceId={item.workspace_id} onCommentsLoaded={() => {}} />
      {/key}
    </section>
  </div>
{/if}

{#if item && transitions.length > 0}
  <MobileOptionSheet
    bind:isOpen={statusSheetOpen}
    title="Status"
    options={transitions}
    getValue={(s) => s.id}
    getLabel={(s) => s.name}
    selectedValue={item.status_id ?? null}
    onSelect={(s) => s && changeStatus(s.id)}
    emptyText="No status options"
    dataTestid="status-sheet"
  >
    {#snippet row(s)}
      <span class="opt">
        <span class="opt-dot" style={s.category_color ? `background-color: ${s.category_color};` : ''}></span>
        {s.name}
      </span>
    {/snippet}
  </MobileOptionSheet>
{/if}

{#if item}
  <MobileOptionSheet
    bind:isOpen={assigneeSheetOpen}
    title="Assignee"
    options={assigneeOptions ?? []}
    loading={assigneeLoading}
    getValue={(u) => u.id}
    getLabel={userLabel}
    selectedValue={item.assignee_id ?? null}
    allowClear={true}
    clearLabel="Unassigned"
    onSelect={updateAssignee}
    onClear={() => updateAssignee(null)}
    emptyText="No assignable users"
    dataTestid="assignee-sheet"
  >
    {#snippet row(user)}
      <span class="opt">
        <Avatar src={user.avatar_url} name={userLabel(user)} size="xs" variant="teal" />
        <span class="assignee-option-label">{userLabel(user)}</span>
      </span>
    {/snippet}
  </MobileOptionSheet>
{/if}

<style>
  .center { display: flex; justify-content: center; padding: 3rem; color: var(--ds-text-subtle); }
  :global(.spin) { animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }

  .msg { padding: 3rem 1.25rem; text-align: center; color: var(--ds-text-subtle); }
  .msg p { margin: 0; }
  .retry {
    min-height: 40px;
    margin-top: 0.75rem;
    padding: 0.45rem 1rem;
    border: 1px solid var(--ds-interactive);
    border-radius: var(--radius-md, 6px);
    background: var(--ds-interactive);
    color: var(--ds-text-inverse, #fff);
    font: inherit;
    font-weight: var(--font-semibold, 600);
    cursor: pointer;
  }
  .retry:disabled { opacity: 0.6; }

  .detail { padding: 0.75rem 0.875rem 2rem; position: relative; }
  /* Promote to its own layer only while the pull gesture is active. A
     persistent `will-change: transform` here makes .detail a containing block
     for position:fixed descendants — which includes the @-mention picker
     rendered inside the comment editor. That reinterprets the picker's
     viewport coords relative to the scrolled .detail box, so on long items
     (e.g. ones with the commits/PR + coding-agent panels) the picker lands far
     below the viewport and never appears (WI-431). The pull transform itself
     establishes the layer during the gesture, so we only need the hint then. */
  .detail.pulling { will-change: transform; }
  /* Ease the content back to rest after a release (and the snap on fire). */
  .detail:not(.pulling) { transition: transform var(--duration-fast, 100ms) ease; }

  /* Pull-to-refresh indicator: anchored above the content, vertically centered
     on the threshold line. Fades/scales in as the pull grows. */
  .ptr-indicator {
    position: absolute;
    top: 0;
    left: 50%;
    transform: translate(-50%, -100%);
    display: flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    margin-top: -6px;
    color: var(--ds-icon-subtle, var(--ds-text-subtle));
    opacity: 0;
    transition: opacity var(--duration-fast, 100ms) ease;
    pointer-events: none;
  }
  .detail.pulling .ptr-indicator { opacity: 1; }
  /* Once past the threshold the indicator goes active/ready (brand color). */
  .ptr-indicator.ready { color: var(--ds-interactive); }
  .ptr-arrow { display: inline-flex; transition: transform var(--duration-fast, 100ms) ease; }

  .breadcrumb {
    display: flex; align-items: center; flex-wrap: nowrap; gap: 0.15rem;
    margin-bottom: 0.6rem; overflow-x: auto; -webkit-overflow-scrolling: touch;
  }
  .bc-link {
    flex-shrink: 0; border: none; background: transparent; cursor: pointer;
    padding: 2px 4px; font-family: var(--font-mono, monospace); font-size: 0.75rem;
    color: var(--ds-text-link, var(--ds-interactive)); white-space: nowrap;
  }
  .breadcrumb :global(.bc-sep) { flex-shrink: 0; color: var(--ds-text-subtlest, var(--ds-text-subtle)); }

  .status-line { display: flex; align-items: center; gap: 0.5rem; margin-bottom: 0.5rem; }
  .type { font-size: 0.75rem; color: var(--ds-text-subtle); text-transform: uppercase; letter-spacing: 0.02em; }

  .title-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.5rem;
    margin: 0 0 1rem;
  }
  .title { font-size: 1.25rem; font-weight: var(--font-semibold, 600); color: var(--ds-text); margin: 0; line-height: 1.3; }
  .edit-btn {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    min-height: 44px;
    padding: 0.25rem 0.5rem;
    margin: -0.25rem -0.25rem 0 0;
    border: none;
    border-radius: var(--radius-md, 6px);
    background: transparent;
    color: var(--ds-interactive);
    font-size: 0.8125rem;
    font-weight: var(--font-medium, 500);
    cursor: pointer;
  }
  .edit-btn:active { background-color: var(--ds-background-neutral-hovered); }

  /* Status + assignee rows (buttons styled as list rows) */
  .fields {
    margin-bottom: 1rem; border: 1px solid var(--ds-border); border-radius: var(--radius-lg, 8px); overflow: hidden;
  }
  .field {
    display: flex; align-items: center; justify-content: space-between; gap: 1rem;
    width: 100%; min-height: 48px; padding: 0.5rem 0.85rem; cursor: pointer;
    border: none; background: transparent; color: inherit; font: inherit; text-align: left;
  }
  .field:not(:last-child) { border-bottom: 1px solid var(--ds-border); }
  .field:active { background-color: var(--ds-background-neutral-hovered); }
  .field:disabled { opacity: 0.6; cursor: default; }
  .field-label { font-size: 0.8125rem; color: var(--ds-text-subtle); }
  .field-value { display: inline-flex; align-items: center; gap: 0.5rem; min-width: 0; color: var(--ds-text); font-size: 0.875rem; }
  .field-value :global(.chev) { color: var(--ds-icon-subtle, var(--ds-text-subtle)); flex-shrink: 0; }
  .assignee-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 9rem; }
  .muted { color: var(--ds-text-subtle); }
  /* Personal-task Done toggle: a compact check that mirrors the desktop/
     PersonalView affordance. Filled green when the task is complete. */
  .done-check {
    width: 18px; height: 18px; flex-shrink: 0;
    display: inline-flex; align-items: center; justify-content: center;
    border: 2px solid var(--ds-border-bold, var(--ds-border)); border-radius: var(--radius-full, 9999px);
    color: #fff; background: transparent;
  }
  .done-check.done { background-color: var(--ds-success, #4cb782); border-color: var(--ds-success, #4cb782); }
  .opt { display: inline-flex; align-items: center; gap: 0.5rem; min-width: 0; }
  .opt-dot { width: 8px; height: 8px; border-radius: var(--radius-full, 9999px); background-color: var(--ds-icon-subtle, var(--ds-text-subtle)); flex-shrink: 0; }
  .assignee-option-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .desc {
    padding: 0.5rem 0 1rem; border-bottom: 1px solid var(--ds-border); margin-bottom: 1rem;
    color: var(--ds-text); font-size: 0.9375rem; line-height: 1.55; overflow-wrap: anywhere;
  }

  .meta { margin: 0 0 1rem; display: flex; flex-direction: column; gap: 0.6rem; }
  .meta div { display: flex; justify-content: space-between; gap: 1rem; }
  .meta dt { color: var(--ds-text-subtle); font-size: 0.8125rem; margin: 0; }
  .meta dd { color: var(--ds-text); font-size: 0.875rem; margin: 0; text-align: right; }

  .actions { display: flex; gap: 0.5rem; margin-bottom: 1.5rem; }
  .act {
    flex: 1; display: inline-flex; align-items: center; justify-content: center; gap: 0.4rem;
    min-height: 44px; border: 1px solid var(--ds-border); border-radius: var(--radius-lg, 8px);
    background-color: var(--ds-surface); color: var(--ds-text); font-size: 0.875rem; font-weight: var(--font-medium, 500); cursor: pointer;
  }
  .act.on { color: var(--ds-interactive); border-color: var(--ds-interactive); }
  .act:disabled { opacity: 0.6; }

  .panel { border: 1px solid var(--ds-border); border-radius: var(--radius-lg, 8px); margin-bottom: 0.75rem; overflow: hidden; }
  .panel-head {
    width: 100%; display: flex; align-items: center; justify-content: space-between; gap: 0.5rem;
    min-height: 48px; padding: 0 0.85rem; background-color: var(--ds-surface); color: var(--ds-text);
    border: none; cursor: pointer; font-size: 0.9375rem; font-weight: var(--font-medium, 500);
  }
  .panel-title { display: inline-flex; align-items: center; gap: 0.5rem; }
  :global(.chev) { transition: transform var(--duration-fast, 100ms) ease; color: var(--ds-icon-subtle, var(--ds-text-subtle)); }
  :global(.chev.open) { transform: rotate(180deg); }
  .panel-body { padding: 0.5rem 0.85rem 0.85rem; border-top: 1px solid var(--ds-border); overflow-x: auto; }

  .subitems { margin-bottom: 1rem; }
  .section-title {
    display: flex; align-items: center; gap: 0.5rem;
    font-size: 0.9375rem; font-weight: var(--font-semibold, 600); color: var(--ds-text); margin: 0 0 0.5rem;
  }
  .section-title .count {
    font-size: 0.75rem; font-weight: var(--font-medium, 500); color: var(--ds-text-subtle);
    background-color: var(--ds-background-neutral); border-radius: var(--radius-full, 9999px); padding: 0 0.45rem;
  }
  /* "Add sub-item" affordance pushed to the trailing edge of the title row. */
  .add-child {
    margin-left: auto; display: inline-flex; align-items: center; gap: 0.3rem;
    border: none; background: transparent; cursor: pointer;
    font-size: 0.8125rem; font-weight: var(--font-medium, 500); color: var(--ds-interactive);
    padding: 0.25rem 0.4rem; border-radius: var(--radius-md, 6px);
  }
  .add-child:active { background-color: var(--ds-background-neutral-hovered); }
  .subitems-list {
    border: 1px solid var(--ds-border); border-radius: var(--radius-lg, 8px); overflow: hidden;
  }
  /* MobileItemRow draws its own bottom border; drop it on the last row so it
     doesn't double the container border. */
  .subitems-list :global([data-testid='mobile-item-row']:last-child) { border-bottom: none; }

  .comments { border-top: 1px solid var(--ds-border); padding-top: 1rem; }
</style>
