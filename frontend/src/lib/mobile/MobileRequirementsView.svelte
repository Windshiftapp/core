<script>
  import { onMount } from 'svelte';
  import { ChevronRight, Command as CommandIcon, Search } from '@lucide/svelte';
  import { navigate } from '../router.js';
  import { workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import { formatRelativeCompact } from '../utils/dateFormatter.js';
  import Lozenge from '../components/Lozenge.svelte';
  import { requirementStatusLozenge } from '../features/requirements/requirementStatuses.js';
  import MobileHeader from './MobileHeader.svelte';
  import MobileListState from './MobileListState.svelte';
  import { fetchWorkspaceRequirementSections } from './mobileRequirementsData.js';
  import { mobilePalette } from './mobilePalette.svelte.js';

  let sections = $state([]);
  let loading = $state(true);
  let errored = $state(false);
  let filter = $state('');
  let loadSeq = 0;

  const trimmedFilter = $derived(filter.trim().toLowerCase());

  const visibleSections = $derived(
    !trimmedFilter
      ? sections
      : sections
          .map((s) => ({
            ...s,
            requirements: s.requirements.filter((req) => {
              const haystack = [req.key, req.page_title, String(req.requirement_number)]
                .filter(Boolean)
                .join(' ')
                .toLowerCase();
              return haystack.includes(trimmedFilter);
            }),
          }))
          .filter((s) => s.requirements.length > 0)
  );

  function openRequirement(workspaceId, requirementNumber) {
    navigate(`/m/requirements/${workspaceId}/${requirementNumber}`);
  }

  async function load() {
    const seq = ++loadSeq;
    loading = true;
    errored = false;
    try {
      if (!$workspacesStore.personalWorkspace) await workspacesStore.loadPersonalWorkspace?.();
      const workspaces = [
        ...($workspacesStore.personalWorkspace ? [$workspacesStore.personalWorkspace] : []),
        ...$workspacesStore.regularWorkspaces,
      ];
      const loaded = await fetchWorkspaceRequirementSections(workspaces);
      if (seq !== loadSeq) return;
      sections = loaded;
    } catch (err) {
      console.error('Failed to load requirements:', err);
      if (seq === loadSeq) errored = true;
    } finally {
      if (seq === loadSeq) loading = false;
    }
  }

  onMount(load);
</script>

<MobileHeader title={t('requirements.navTitle')}>
  {#snippet right()}
    <button
      class="hdr-btn"
      onclick={() => mobilePalette.open()}
      data-testid="mobile-palette-open"
      aria-label="Command palette"
      type="button"
    >
      <CommandIcon size={20} />
    </button>
  {/snippet}
  {#snippet children()}
    <div class="field">
      <Search size={16} class="f-icon" />
      <input
        bind:value={filter}
        data-testid="mobile-requirements-filter"
        type="search"
        enterkeyhint="search"
        autocomplete="off"
        placeholder={t('requirements.mobile.filterPlaceholder')}
      />
    </div>
  {/snippet}
</MobileHeader>

<div class="requirements-list" data-testid="mobile-requirements-view">
  <MobileListState
    loading={loading}
    errored={errored}
    rowCount={visibleSections.reduce((n, s) => n + s.requirements.length, 0)}
    errorMessage={t('requirements.mobile.loadError')}
    emptyMessage={trimmedFilter ? t('requirements.mobile.emptyFiltered') : t('requirements.emptyTitle')}
    onretry={load}
  >
    {#each visibleSections as section (section.workspace.id)}
      <div class="ws-block" data-testid={`mobile-requirements-section-${section.workspace.id}`}>
        <h2 class="ws-name">{section.workspace.name}</h2>
        {#if section.totalItems > section.requirements.length}
          <p class="ws-hint">{t('requirements.mobile.truncatedList', { count: section.totalItems })}</p>
        {/if}
        <div class="rows">
          {#each section.requirements as req (req.requirement_number)}
            <button
              class="row"
              onclick={() => openRequirement(section.workspace.id, req.requirement_number)}
              data-testid="mobile-requirement-row"
              data-requirement-number={req.requirement_number}
              type="button"
            >
              <span class="row-main">
                <span class="row-key">
                  <Lozenge color="blue">{req.key}</Lozenge>
                </span>
                <span class="row-title">{req.page_title}</span>
                <span class="row-meta">
                  <Lozenge color="grey">{t(`requirements.type.${req.requirement_type}`)}</Lozenge>
                  <Lozenge color={requirementStatusLozenge(req.status)}>
                    {t(`requirements.status.${req.status}`)}
                  </Lozenge>
                </span>
              </span>
              <span class="row-side">
                {#if req.updated_at}
                  <time class="row-time">{formatRelativeCompact(new Date(req.updated_at))}</time>
                {/if}
                <ChevronRight size={16} class="chev" />
              </span>
            </button>
          {/each}
        </div>
      </div>
    {/each}
  </MobileListState>
</div>

<style>
  .hdr-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border: none;
    background: transparent;
    color: var(--ds-text);
    cursor: pointer;
  }

  .field {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-height: 38px;
    margin: 0 0.75rem 0.5rem;
    padding: 0 0.625rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background-color: var(--ds-surface-raised);
  }
  .field :global(.f-icon) {
    color: var(--ds-text-subtle);
    flex-shrink: 0;
  }
  .field input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    font-size: 0.9375rem;
    color: var(--ds-text);
  }

  .requirements-list { padding: 0.25rem 0 1rem; }

  .ws-name {
    padding: 0.75rem 0.875rem 0.3rem;
    font-size: 0.75rem;
    font-weight: var(--font-semibold, 600);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--ds-text-subtle);
    margin: 0;
  }
  .ws-hint {
    margin: 0;
    padding: 0 0.875rem 0.35rem;
    font-size: 0.75rem;
    color: var(--ds-text-subtlest, var(--ds-text-subtle));
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    width: 100%;
    min-height: 56px;
    padding: 0.5rem 0.875rem;
    border: none;
    border-bottom: 1px solid var(--ds-border);
    background: transparent;
    text-align: left;
    cursor: pointer;
  }
  .row:active {
    background-color: var(--ds-background-neutral-hovered);
  }
  .row-main {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 0;
  }
  .row-title {
    font-size: 0.9375rem;
    font-weight: var(--font-medium, 500);
    color: var(--ds-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }
  .row-side {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    flex-shrink: 0;
  }
  .row-time {
    font-size: 0.75rem;
    color: var(--ds-text-subtlest, var(--ds-text-subtle));
  }
  .row-side :global(.chev) {
    color: var(--ds-icon-subtle, var(--ds-text-subtle));
  }
</style>
