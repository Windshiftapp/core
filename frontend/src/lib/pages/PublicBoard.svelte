<script>
  import { onMount, onDestroy } from 'svelte';
  import { publicBoard } from '../api/publicBoard.js';
  import { themeStore } from '../stores/theme.svelte.js';
  import PublicBoardItemDetail from './PublicBoardItemDetail.svelte';

  let { slug } = $props();

  let selectedItemKey = $state(null);

  let board = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let refreshInterval;

  // Card fields set for quick lookup
  let cardFieldSet = $derived(
    new Set((board?.card_fields || []).map(f => f.field_identifier))
  );

  // Whether to show a field on cards
  function showField(fieldId) {
    // If no card_fields configured, show defaults
    if (!board?.card_fields || board.card_fields.length === 0) {
      return ['title', 'priority', 'assignee', 'status'].includes(fieldId);
    }
    return cardFieldSet.has(fieldId);
  }

  async function loadBoard() {
    try {
      board = await publicBoard.get(slug);
      error = null;
    } catch (err) {
      if (err.status === 404) {
        error = 'not_found';
      } else {
        error = 'error';
      }
    } finally {
      loading = false;
    }
  }

  function toggleTheme() {
    const current = themeStore.resolvedTheme;
    themeStore.setTheme(current === 'dark' ? 'light' : 'dark');
  }

  onMount(() => {
    themeStore.init();
    loadBoard();
    // Auto-refresh every 60 seconds
    refreshInterval = setInterval(loadBoard, 60000);
  });

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
  });
</script>

<div class="public-board" style="background-color: var(--ds-surface); color: var(--ds-text); min-height: 100vh; display: flex; flex-direction: column;">
  <!-- Header -->
  <header class="public-board-header" style="background-color: var(--ds-surface-raised); border-bottom: 1px solid var(--ds-border); padding: 12px 24px; display: flex; align-items: center; justify-content: space-between; gap: 16px;">
    <div style="display: flex; align-items: center; gap: 12px;">
      <img src="/windshift-3.svg" alt="Windshift" style="width: 28px; height: 28px;" />
      {#if board}
        <div>
          <h1 style="font-size: 18px; font-weight: 600; margin: 0; line-height: 1.3;">{board.collection.name}</h1>
          {#if board.collection.description}
            <p style="font-size: 13px; color: var(--ds-text-subtle); margin: 0; line-height: 1.4;">{board.collection.description}</p>
          {/if}
        </div>
      {/if}
    </div>
    <button
      onclick={toggleTheme}
      style="background: none; border: 1px solid var(--ds-border); border-radius: 6px; padding: 6px 10px; cursor: pointer; color: var(--ds-text-subtle); font-size: 13px;"
      title="Toggle theme"
    >
      {themeStore.resolvedTheme === 'dark' ? '☀️' : '🌙'}
    </button>
  </header>

  <!-- Content -->
  <main style="flex: 1; overflow-x: auto; padding: 20px 24px;">
    {#if loading}
      <div style="display: flex; align-items: center; justify-content: center; height: 60vh;">
        <div style="text-align: center;">
          <div class="spinner" style="width: 32px; height: 32px; border: 3px solid var(--ds-border); border-top-color: #2874BB; border-radius: 50%; animation: spin 0.8s linear infinite; margin: 0 auto 12px;"></div>
          <p style="color: var(--ds-text-subtle); font-size: 14px;">Loading board...</p>
        </div>
      </div>
    {:else if error === 'not_found'}
      <div style="display: flex; align-items: center; justify-content: center; height: 60vh;">
        <div style="text-align: center;">
          <div style="font-size: 48px; margin-bottom: 16px; opacity: 0.3;">📋</div>
          <h2 style="font-size: 20px; font-weight: 600; margin: 0 0 8px;">Board not found</h2>
          <p style="color: var(--ds-text-subtle); font-size: 14px;">This board doesn't exist or is no longer public.</p>
        </div>
      </div>
    {:else if error}
      <div style="display: flex; align-items: center; justify-content: center; height: 60vh;">
        <div style="text-align: center;">
          <div style="font-size: 48px; margin-bottom: 16px; opacity: 0.3;">⚠️</div>
          <h2 style="font-size: 20px; font-weight: 600; margin: 0 0 8px;">Something went wrong</h2>
          <p style="color: var(--ds-text-subtle); font-size: 14px;">Failed to load the board. Please try again later.</p>
        </div>
      </div>
    {:else if board}
      <div class="board-columns" style="display: flex; gap: 16px; min-height: calc(100vh - 160px);">
        {#each board.columns as column}
          <div class="board-column" style="min-width: 280px; max-width: 320px; flex: 1; display: flex; flex-direction: column;">
            <!-- Column header -->
            <div style="padding: 10px 12px; margin-bottom: 8px; border-radius: 8px 8px 0 0; display: flex; align-items: center; justify-content: space-between; background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-bottom: 3px solid {column.color || 'var(--ds-border)'};">
              <div style="display: flex; align-items: center; gap: 8px;">
                <span style="font-size: 14px; font-weight: 600;">{column.name}</span>
                <span style="font-size: 12px; color: var(--ds-text-subtle); background: var(--ds-surface-sunken); padding: 1px 7px; border-radius: 10px;">{column.items.length}</span>
              </div>
              {#if column.wip_limit}
                <span
                  style="font-size: 11px; padding: 1px 6px; border-radius: 8px; {column.items.length > column.wip_limit ? 'background: #ef4444; color: white;' : 'color: var(--ds-text-subtle); background: var(--ds-surface-sunken);'}"
                  title="WIP limit: {column.wip_limit}"
                >
                  {column.items.length}/{column.wip_limit}
                </span>
              {/if}
            </div>

            <!-- Cards -->
            <div style="flex: 1; display: flex; flex-direction: column; gap: 8px; overflow-y: auto; padding: 0 2px 8px;">
              {#each column.items as card}
                <div
                  class="public-card"
                  style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-radius: 6px; padding: 12px; box-shadow: var(--ds-shadow-raised); cursor: pointer;"
                  onclick={() => selectedItemKey = card.key}
                  onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectedItemKey = card.key; } }}
                  role="button"
                  tabindex="0"
                >
                  <!-- Key -->
                  {#if showField('key')}
                    <div style="font-size: 11px; font-weight: 500; color: var(--ds-text-subtle); margin-bottom: 4px; font-family: monospace;">{card.key}</div>
                  {/if}

                  <!-- Title -->
                  <div style="font-size: 13px; font-weight: 500; line-height: 1.4; margin-bottom: 8px;">{card.title}</div>

                  <!-- Meta row -->
                  <div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap;">
                    <!-- Priority -->
                    {#if showField('priority') && card.priority_name}
                      <span
                        style="font-size: 11px; padding: 1px 6px; border-radius: 4px; background: {card.priority_color || 'var(--ds-surface-sunken)'}20; color: {card.priority_color || 'var(--ds-text-subtle)'}; border: 1px solid {card.priority_color || 'var(--ds-border)'}40;"
                        title={card.priority_name}
                      >
                        {card.priority_name}
                      </span>
                    {/if}

                    <!-- Item type -->
                    {#if showField('item_type') && card.item_type_name}
                      <span style="font-size: 11px; padding: 1px 6px; border-radius: 4px; background: var(--ds-surface-sunken); color: var(--ds-text-subtle);">
                        {card.item_type_name}
                      </span>
                    {/if}

                    <!-- Story points -->
                    {#if showField('story_points') && card.story_points != null}
                      <span style="font-size: 11px; padding: 1px 6px; border-radius: 4px; background: var(--ds-surface-sunken); color: var(--ds-text-subtle);" title="Story Points">
                        {card.story_points} SP
                      </span>
                    {/if}

                    <!-- Due date -->
                    {#if showField('due_date') && card.due_date}
                      {@const isOverdue = new Date(card.due_date) < new Date()}
                      <span style="font-size: 11px; padding: 1px 6px; border-radius: 4px; {isOverdue ? 'background: #ef444420; color: #ef4444;' : 'background: var(--ds-surface-sunken); color: var(--ds-text-subtle);'}">
                        {card.due_date}
                      </span>
                    {/if}

                    <!-- Labels -->
                    {#if showField('labels') && card.labels?.length}
                      {#each card.labels as label}
                        <span style="font-size: 10px; padding: 1px 6px; border-radius: 4px; background: {label.color}20; color: {label.color}; border: 1px solid {label.color}40;">
                          {label.name}
                        </span>
                      {/each}
                    {/if}

                    <div style="flex: 1;"></div>

                    <!-- Assignee -->
                    {#if showField('assignee') && card.assignee_name}
                      {#if card.assignee_avatar}
                        <img
                          src={card.assignee_avatar}
                          alt={card.assignee_name}
                          title={card.assignee_name}
                          style="width: 22px; height: 22px; border-radius: 50%; object-fit: cover;"
                        />
                      {:else}
                        <div
                          title={card.assignee_name}
                          style="width: 22px; height: 22px; border-radius: 50%; background: #2874BB; color: white; display: flex; align-items: center; justify-content: center; font-size: 10px; font-weight: 600;"
                        >
                          {card.assignee_name.split(' ').map(n => n[0]).join('').slice(0, 2).toUpperCase()}
                        </div>
                      {/if}
                    {/if}
                  </div>
                </div>
              {/each}

              {#if column.items.length === 0}
                <div style="padding: 24px 12px; text-align: center; color: var(--ds-text-disabled); font-size: 13px;">
                  No items
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </main>

  <!-- Footer -->
  <footer style="padding: 12px 24px; text-align: center; border-top: 1px solid var(--ds-border); background-color: var(--ds-surface-raised);">
    <div style="display: flex; align-items: center; justify-content: center; gap: 8px;">
      <img src="/windshift-3.svg" alt="Windshift" style="width: 16px; height: 16px; opacity: 0.5;" />
      <span style="font-size: 12px; color: var(--ds-text-subtle);">Powered by Windshift</span>
    </div>
  </footer>

  {#if selectedItemKey}
    <PublicBoardItemDetail {slug} itemKey={selectedItemKey} onclose={() => selectedItemKey = null} />
  {/if}
</div>

<style>
  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .public-card {
    transition: box-shadow 140ms ease-in-out, transform 80ms ease-in-out;
  }
  .public-card:hover {
    box-shadow: var(--ds-shadow-overlay) !important;
    transform: translateY(-1px);
  }

  .board-columns {
    scrollbar-width: thin;
    scrollbar-color: var(--ds-border) transparent;
  }

  /* Mobile responsive: stack columns */
  @media (max-width: 768px) {
    .board-columns {
      flex-direction: column !important;
    }
    .board-column {
      min-width: 100% !important;
      max-width: 100% !important;
    }
  }
</style>
