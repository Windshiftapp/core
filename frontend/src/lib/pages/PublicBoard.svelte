<script>
  import { untrack } from 'svelte';
  import { publicBoard } from '../api/publicBoard.js';
  import { themeStore } from '../stores/theme.svelte.js';
  import CustomerHeader from '../customer/CustomerHeader.svelte';
  import IconClipboardList from '@tabler/icons-svelte-runes/icons/clipboard-list';
  import IconAlertTriangle from '@tabler/icons-svelte-runes/icons/alert-triangle';
  import PublicBoardItemDetail from './PublicBoardItemDetail.svelte';
  import { primaryMilestoneForItem, sortMilestonesForLanes, SWIMLANE_NONE_KEY } from '../features/collections/swimlanes.js';
  import { formatDateOnly } from '../utils/dateFormatter.js';
  import { canRunBackgroundSync, onBackgroundSyncAvailable } from '../utils/backgroundSync.js';
  import Badge from '../components/Badge.svelte';
  import Avatar from '../components/Avatar.svelte';
  import Card from '../components/Card.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import StatusPill from '../components/StatusPill.svelte';

  let { slug, client = publicBoard, embedded = false, initialItemKey = null, onitemchange = null, onaccessdenied = null } = $props();

  let selectedItemKey = $state(null);

  let board = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let loadId = 0;

  function selectItem(key) { selectedItemKey = key; onitemchange?.(key); }

  // Card fields set for quick lookup
  let cardFieldSet = $derived(
    new Set((board?.card_fields || []).map(f => f.field_identifier))
  );

  // Owner-configured milestone lanes. Read-only: public viewers cannot toggle
  // grouping, and only lanes that actually contain cards are rendered.
  let publicSwimlanes = $derived.by(() => {
    if (!board?.swimlanes) return null;

    const ordered = sortMilestonesForLanes(board.milestones || []);
    const laneKeys = ordered.map((milestone) => `milestone-${milestone.id}`);
    const hasAssigned = board.columns.some((column) =>
      column.items.some((card) => (card.milestone_ids || []).length > 0)
    );
    if (!hasAssigned) return null;

    const buckets = board.columns.map(() => new Map());
    const counts = new Map();
    board.columns.forEach((column, columnIndex) => {
      for (const card of column.items) {
        const primary = primaryMilestoneForItem(card, ordered);
        const key = primary ? `milestone-${primary.id}` : SWIMLANE_NONE_KEY;
        const bucket = buckets[columnIndex].get(key) || [];
        bucket.push(card);
        buckets[columnIndex].set(key, bucket);
        counts.set(key, (counts.get(key) || 0) + 1);
      }
    });

    const lanes = ordered
      .map((milestone) => {
        const key = `milestone-${milestone.id}`;
        return {
          key,
          title: milestone.name,
          milestone,
          count: counts.get(key) || 0,
          columns: board.columns.map((column, columnIndex) => ({
            column,
            items: buckets[columnIndex].get(key) || [],
          })),
        };
      })
      .filter((lane) => lane.count > 0);

    const unassignedCount = counts.get(SWIMLANE_NONE_KEY) || 0;
    if (unassignedCount > 0) {
      lanes.push({
        key: SWIMLANE_NONE_KEY,
        title: 'Unassigned',
        milestone: null,
        count: unassignedCount,
        columns: board.columns.map((column, columnIndex) => ({
          column,
          items: buckets[columnIndex].get(SWIMLANE_NONE_KEY) || [],
        })),
      });
    }

    return lanes.length > 0 ? lanes : null;
  });

  // Whether to show a field on cards
  function showField(fieldId) {
    // If no card_fields configured, show defaults
    if (!board?.card_fields || board.card_fields.length === 0) {
      return ['title', 'priority', 'assignee', 'status'].includes(fieldId);
    }
    return cardFieldSet.has(fieldId);
  }

  // The router reuses this component across public-board URLs, so a slug change
  // must reset scope-owned state and start the new board immediately. Each load
  // is bound to its slug generation: a superseded response, error or finally
  // must never commit over the board the user is now looking at.
  $effect(() => {
    const activeSlug = slug;
    const activeClient = client;
    const id = ++loadId;
    const controller = new AbortController();
    let inFlight = false;
    let disposed = false;
    let recoveryPending = false;

    board = null;
    error = null;
    loading = true;
    selectedItemKey = untrack(() => initialItemKey);

    async function load(silent, recovery = false) {
      if (silent && !canRunBackgroundSync()) return;
      if (disposed) return;
      if (inFlight) {
        if (recovery) recoveryPending = true;
        return;
      }
      recoveryPending = false;
      inFlight = true;
      if (!silent) loading = true;
      try {
        const result = await activeClient.get(activeSlug, { signal: controller.signal });
        if (disposed || id !== loadId) return;
        board = result;
        error = null;
      } catch (err) {
        if (disposed || id !== loadId || err?.name === 'AbortError') return;
        error = err?.status === 404 ? 'not_found' : 'error';
        board = null;
        selectedItemKey = null;
        if (err?.status === 401 || err?.status === 403) onaccessdenied?.(err.status);
      } finally {
        inFlight = false;
        if (!disposed && id === loadId) loading = false;
        if (recoveryPending && !disposed) void load(true);
      }
    }

    load(false);
    // Auto-refresh every 60 seconds without overlapping a slow request.
    const refreshInterval = setInterval(() => load(true), 60000);
    const stopRecovery = onBackgroundSyncAvailable(() => load(true, true));

    return () => {
      disposed = true;
      controller.abort();
      clearInterval(refreshInterval);
      stopRecovery();
    };
  });

  $effect(() => { selectedItemKey = initialItemKey; });
</script>

<div
  class="public-board"
  style="background-color: var(--ds-surface); color: var(--ds-text); min-height: {embedded ? '0' : '100vh'}; display: flex; flex-direction: column;"
  data-testid="public-board-page"
  data-ready={!loading}
>
  <!-- Header -->
  {#if !embedded}
    <CustomerHeader title={board?.collection.name || ''} description={board?.collection.description || ''} logo="windshift-3.svg" />
  {/if}

  <!-- Content -->
  <main style="flex: 1; overflow-x: auto; padding: 20px 24px;">
    {#if loading}
      <div data-testid="public-board-loading" style="display: flex; align-items: center; justify-content: center; height: 60vh;">
        <div style="text-align: center;">
          <div class="spinner" style="width: 32px; height: 32px; border: 3px solid var(--ds-border); border-top-color: #2874BB; border-radius: 50%; animation: spin 0.8s linear infinite; margin: 0 auto 12px;"></div>
          <p style="color: var(--ds-text-subtle); font-size: 14px;">Loading board...</p>
        </div>
      </div>
    {:else if error === 'not_found'}
      <div data-testid="public-board-not-found" style="display: flex; align-items: center; justify-content: center; height: 60vh;">
        <div style="text-align: center;">
          <IconClipboardList class="w-10 h-10 mx-auto mb-4" style="color: var(--ds-icon-disabled);" />
          <h2 style="font-size: 20px; font-weight: 600; margin: 0 0 8px;">Board not found</h2>
          <p style="color: var(--ds-text-subtle); font-size: 14px;">This board doesn't exist or is no longer public.</p>
        </div>
      </div>
    {:else if error}
      <div data-testid="public-board-error" style="display: flex; align-items: center; justify-content: center; height: 60vh;">
        <div style="text-align: center;">
          <IconAlertTriangle class="w-10 h-10 mx-auto mb-4" style="color: var(--ds-icon-danger);" />
          <h2 style="font-size: 20px; font-weight: 600; margin: 0 0 8px;">Something went wrong</h2>
          <p style="color: var(--ds-text-subtle); font-size: 14px;">Failed to load the board. Please try again later.</p>
        </div>
      </div>
    {:else if board}
      {#if board.truncated}
        <div
          data-testid="public-board-truncated"
          style="margin-bottom: 16px; padding: 10px 12px; border: 1px solid var(--ds-border-warning, #ca8a04); border-radius: 6px; background: var(--ds-background-warning, #fef9c3); color: var(--ds-text-warning, #854d0e); font-size: 13px;"
        >
          Showing the newest {board.loaded_items} of {board.total_items} matching items. Column counts are partial because this public board is limited to {board.item_limit} cards.
        </div>
      {/if}

      {#snippet publicCard(card)}
        <div
          class="public-card"
          onclick={() => selectItem(card.key)}
          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectItem(card.key); } }}
          role="button"
          tabindex="0"
          data-testid="public-board-card"
          data-item-key={card.key}
        >
          <Card variant="flat" padding="compact" hoverable style="background-color: var(--ds-surface-raised);">
            {#if showField('key')}
              <div style="font-size: 11px; font-weight: 500; color: var(--ds-text-subtle); margin-bottom: 4px; font-family: monospace;">{card.key}</div>
            {/if}

            {#if showField('title')}
              <div
                data-testid="public-board-card-title"
                style="font-size: 13px; font-weight: 500; line-height: 1.4; margin-bottom: 8px;"
              >
                {card.title}
              </div>
            {/if}

            <div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap;">
              {#if showField('priority') && card.priority_name}
                <Lozenge color="gray" customBg={card.priority_color || null} customText={themeStore.resolvedTheme === 'dark' ? 'var(--ds-text)' : null} text={card.priority_name} />
              {/if}

              {#if showField('status') && card.status_name}
                <span data-testid="public-board-card-status">
                  <StatusPill name={card.status_name} />
                </span>
              {/if}

              {#if showField('item_type') && card.item_type_name}
                <Badge size="xs">{card.item_type_name}</Badge>
              {/if}

              {#if showField('story_points') && card.story_points != null}
                <Badge size="xs" title="Story Points">{card.story_points} SP</Badge>
              {/if}

              {#if showField('due_date') && card.due_date}
                {@const isOverdue = new Date(card.due_date) < new Date()}
                <Badge size="xs" variant={isOverdue ? 'danger' : 'neutral'}>{card.due_date}</Badge>
              {/if}

              {#if showField('labels') && card.labels?.length}
                {#each card.labels as label}
                  <Lozenge color="gray" customBg={label.color || null} customText={themeStore.resolvedTheme === 'dark' ? 'var(--ds-text)' : null} text={label.name} />
                {/each}
              {/if}

              <div style="flex: 1;"></div>

              {#if showField('assignee') && card.assignee_name}
                <Avatar size="xs" src={card.assignee_avatar} name={card.assignee_name} title={card.assignee_name} />
              {/if}
            </div>
          </Card>
        </div>
      {/snippet}

      {#snippet publicColumn(column, items)}
        <div
          class="board-column"
          data-testid="public-board-column"
          data-column-name={column.name}
        >
          <Card variant="flat" padding="none" class="h-full" shadow style="background-color: var(--ds-surface-raised); border-top: 4px solid {column.color || 'var(--ds-border)'};">
            {#snippet header()}
              <div class="column-header">
                <div class="column-heading">
                  <span class="column-name">{column.name}</span>
                  <Badge size="xs">{items.length}</Badge>
                </div>
                {#if column.wip_limit}
                  <Badge
                    size="xs"
                    variant={items.length > column.wip_limit ? 'danger' : 'neutral'}
                    title="WIP limit: {column.wip_limit}"
                  >
                    {items.length}/{column.wip_limit}
                  </Badge>
                {/if}
              </div>
            {/snippet}

            <div class="column-cards">
              {#each items as card}
                {@render publicCard(card)}
              {/each}

              {#if items.length === 0}
                <div style="padding: 24px 12px; text-align: center; color: var(--ds-text-disabled); font-size: 13px;">
                  No items
                </div>
              {/if}
            </div>
          </Card>
        </div>
      {/snippet}

      {#if publicSwimlanes}
        <div data-testid="public-board-swimlanes">
          {#each publicSwimlanes as lane (lane.key)}
            <section
              data-testid={`public-board-lane-${lane.key}`}
              style="margin-bottom: 24px;"
            >
              <div
                data-testid={`public-board-lane-header-${lane.key}`}
                style="display: flex; align-items: center; gap: 8px; margin-bottom: 8px; padding: 6px 10px; border: 1px solid var(--ds-border); border-radius: 6px; background-color: var(--ds-surface-raised); width: fit-content;"
              >
                {#if lane.milestone?.category_color}
                  <span style="width: 8px; height: 8px; border-radius: 50%; background: {lane.milestone.category_color};"></span>
                {/if}
                <span style="font-size: 14px; font-weight: 600;">{lane.title}</span>
                {#if lane.milestone?.target_date}
                  <span style="font-size: 12px; color: var(--ds-text-subtle);">{formatDateOnly(lane.milestone.target_date)}</span>
                {/if}
                <span
                  data-testid={`public-board-lane-count-${lane.key}`}
                  style="font-size: 12px; color: var(--ds-text-subtle); background: var(--ds-surface-sunken); padding: 1px 7px; border-radius: 10px;"
                >
                  {lane.count}
                </span>
              </div>
              <div class="board-columns" style="display: flex; gap: 16px; min-height: 120px;">
                {#each lane.columns as { column, items } (column.name)}
                  {@render publicColumn(column, items)}
                {/each}
              </div>
            </section>
          {/each}
        </div>
      {:else}
        <div class="board-columns" style="display: flex; gap: 16px; min-height: calc(100vh - 160px);">
          {#each board.columns as column}
            {@render publicColumn(column, column.items)}
          {/each}
        </div>
      {/if}
    {/if}

  </main>

  <!-- Footer -->
  {#if !embedded}
  <footer style="padding: 12px 24px; text-align: center; border-top: 1px solid var(--ds-border); background-color: var(--ds-surface-raised);">
    <div style="display: flex; align-items: center; justify-content: center; gap: 8px;">
      <img src="windshift-3.svg" alt="Windshift" style="width: 16px; height: 16px; opacity: 0.5;" />
      <span style="font-size: 12px; color: var(--ds-text-subtle);">Powered by Windshift</span>
    </div>
  </footer>
  {/if}

  {#if selectedItemKey}
    {#key selectedItemKey}<PublicBoardItemDetail {slug} {client} itemKey={selectedItemKey} onclose={() => selectItem(null)} />{/key}
  {/if}
</div>

<style>
  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .public-card {
    min-width: 0;
    border-radius: var(--radius-lg, 8px);
    cursor: pointer;
    overflow-wrap: anywhere;
  }
  .public-card:focus-visible {
    outline: 2px solid var(--ds-border-focused);
    outline-offset: 2px;
  }

  .board-column {
    min-width: 280px;
    max-width: 320px;
    flex: 1;
  }

  .column-header,
  .column-heading {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .column-header {
    justify-content: space-between;
  }

  .column-name {
    font-size: 0.875rem;
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .column-cards {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px;
  }

  .board-columns {
    scrollbar-width: thin;
    scrollbar-color: var(--ds-border) transparent;
  }

  /* Mobile responsive: stack columns */
  @media (max-width: 768px) {
    .board-columns {
      flex-direction: column;
    }
    .board-column {
      min-width: 100%;
      max-width: 100%;
      flex: none;
    }
  }
</style>
