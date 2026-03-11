<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { Sparkles, ArrowLeft, RotateCcw, CheckCircle, AlertCircle, CheckSquare, Square, ArrowRight, Calendar, Globe, Building2 } from 'lucide-svelte';
  import { navigate } from '../../router.js';
  import PageHeader from '../../layout/PageHeader.svelte';
  import Card from '../../components/Card.svelte';
  import Button from '../../components/Button.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { successToast, errorToast } from '../../stores/toasts.svelte.js';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { formatDateShort } from '../../utils/dateFormatter.js';

  let { iterationId } = $props();

  let loading = $state(false);
  let accepting = $state(false);
  let result = $state(null);
  let error = $state('');
  let accepted = $state(false);
  let acceptResult = $state(null);
  let connections = $state([]);
  let selectedConnectionId = $state(null);
  let selected = $state(new Set());
  let iteration = $state(null);
  let allIterations = $state([]);
  let compareIterationIds = $state([]);

  onMount(async () => {
    try {
      const [conns, iter, iters] = await Promise.all([
        api.llmProviders.getEnabled(),
        api.iterations.get(iterationId),
        api.iterations.getAll()
      ]);
      connections = conns;
      iteration = iter;
      allIterations = iters;
      const def = conns.find(c => c.is_default);
      if (def) selectedConnectionId = def.id;
    } catch (e) {
      console.error('Failed to load initial data:', e);
    }
  });

  // Auto-select all suggestions when result changes
  $effect(() => {
    if (result?.suggestions?.length > 0) {
      selected = new Set(result.suggestions.map((_, i) => i));
    }
  });

  async function analyze() {
    loading = true;
    error = '';
    result = null;
    accepted = false;
    acceptResult = null;
    selected = new Set();

    try {
      result = await api.ai.analyzeDependencies(
        iterationId,
        compareIterationIds.length > 0 ? { compare_iteration_ids: compareIterationIds } : {},
        selectedConnectionId
      );
    } catch (err) {
      console.error('Dependency analysis failed:', err);
      error = err.message || 'Failed to analyze dependencies. Is the LLM service running?';
    } finally {
      loading = false;
    }
  }

  async function handleAccept() {
    if (selected.size === 0) return;
    accepting = true;

    const payload = result.suggestions
      .filter((_, i) => selected.has(i))
      .map(s => ({
        source_item_id: s.source_item_id,
        target_item_id: s.target_item_id,
        link_type_id: s.link_type_id,
      }));

    try {
      acceptResult = await api.ai.acceptDependencies(iterationId, payload);
      accepted = true;
      successToast(`Created ${acceptResult.created} dependency link${acceptResult.created !== 1 ? 's' : ''}`);
    } catch (err) {
      console.error('Failed to accept dependencies:', err);
      errorToast(err.message || 'Failed to create dependency links');
    } finally {
      accepting = false;
    }
  }

  function toggleItem(index) {
    const next = new Set(selected);
    if (next.has(index)) {
      next.delete(index);
    } else {
      next.add(index);
    }
    selected = next;
  }

  function toggleAll() {
    if (!result?.suggestions) return;
    if (selected.size === result.suggestions.length) {
      selected = new Set();
    } else {
      selected = new Set(result.suggestions.map((_, i) => i));
    }
  }

  const relationshipColors = {
    depends_on: 'orange',
    blocks: 'red',
    relates_to: 'blue',
  };

  const relationshipLabels = {
    depends_on: 'Depends On',
    blocks: 'Blocks',
    relates_to: 'Relates To',
  };

  let allSelected = $derived(
    result?.suggestions?.length > 0 && selected.size === result.suggestions.length
  );

  // Iterations available for comparison (exclude current)
  let availableIterations = $derived(
    allIterations.filter(i => String(i.id) !== String(iterationId))
  );

  function getIterationStatusColor(status) {
    switch (status) {
      case 'active': return '#0052CC';
      case 'completed': return '#00875A';
      case 'cancelled': return '#6B778C';
      case 'planned': return '#5243AA';
      default: return '#6B778C';
    }
  }

  function capitalize(str) {
    return str ? str.charAt(0).toUpperCase() + str.slice(1) : '';
  }

  const iterationConfig = {
    icon: {
      type: 'component',
      source: (item) => item.is_global ? Globe : Building2
    },
    primary: { text: (item) => item.name },
    badges: [
      {
        text: (item) => item.is_global ? 'Global' : 'Workspace',
        bgColor: () => 'var(--ds-background-neutral)',
        textColor: () => 'var(--ds-text-subtle)'
      }
    ],
    metadata: [
      {
        type: 'date-range',
        icon: Calendar,
        startDate: (item) => item.start_date,
        endDate: (item) => item.end_date
      },
      {
        type: 'badge',
        text: (item) => item.status ? capitalize(item.status) : '',
        bgColor: (item) => item.status ? getIterationStatusColor(item.status) + '15' : 'transparent',
        textColor: (item) => item.status ? getIterationStatusColor(item.status) : 'var(--ds-text)'
      }
    ],
    searchFields: ['name', 'description'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name
  };
</script>

<div class="flex min-h-screen" style="background-color: var(--ds-surface);">
  <div class="flex-1 max-w-5xl mx-auto p-6">
  <PageHeader
    icon={Sparkles}
    title="Dependency Analysis"
    subtitle={iteration ? `Analyze dependencies for ${iteration.name}` : 'AI-powered dependency detection'}
  />

  <!-- Back link -->
  <button
    onclick={() => navigate(`/iterations/${iterationId}`)}
    class="flex items-center gap-2 text-sm font-medium hover:opacity-80 transition-opacity mb-6"
    style="color: var(--ds-text-subtle);"
  >
    <ArrowLeft class="w-4 h-4" />
    Back to Iteration
  </button>

  <div class="space-y-6">
    {#if !result && !loading}
      <!-- Initial state -->
      <Card variant="raised" padding="default">
        <div class="flex flex-col items-center py-8 gap-4">
          <div class="rounded-full p-4" style="background-color: var(--ds-surface-sunken);">
            <Sparkles size={32} style="color: var(--ds-text-subtle);" />
          </div>
          <p class="text-sm text-center max-w-md" style="color: var(--ds-text-subtle);">
            Analyze items in this iteration to discover dependency relationships between work items across workspaces.
          </p>
          {#if connections.length > 0}
            <div class="flex items-center gap-2">
              <label for="ai-model-select" class="text-xs" style="color: var(--ds-text-subtle);">AI Model:</label>
              <select
                id="ai-model-select"
                bind:value={selectedConnectionId}
                class="text-sm rounded px-2 py-1 border"
                style="border-color: var(--ds-border); background: var(--ds-surface); color: var(--ds-text);"
              >
                {#each connections as conn}
                  <option value={conn.id}>{conn.name}</option>
                {/each}
              </select>
            </div>
          {/if}
          {#if availableIterations.length > 0}
            <div class="w-full max-w-sm">
              <label class="text-xs block mb-1" style="color: var(--ds-text-subtle);">Compare with iterations (optional)</label>
              <ItemPicker
                multiSelect={true}
                bind:values={compareIterationIds}
                maxSelections={4}
                items={availableIterations}
                config={iterationConfig}
                placeholder="Select iterations to compare..."
                allowClear={true}
              />
            </div>
          {/if}
          <Button variant="primary" onclick={analyze}>
            Analyze Dependencies
          </Button>
        </div>
      </Card>

    {:else if loading}
      <!-- Loading state -->
      <Card variant="raised" padding="default">
        <div class="flex flex-col items-center py-8 gap-3">
          <div class="animate-spin rounded-full h-8 w-8 border-2 border-t-transparent" style="border-color: var(--ds-border); border-top-color: transparent;"></div>
          <p class="text-sm" style="color: var(--ds-text-subtle);">Analyzing dependencies...</p>
        </div>
      </Card>

    {:else if error}
      <!-- Error state -->
      <Card variant="outlined" padding="default">
        <div class="flex items-start gap-2">
          <AlertCircle size={16} style="color: var(--ds-text-danger); flex-shrink: 0; margin-top: 2px;" />
          <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
        </div>
      </Card>
      <Button variant="secondary" onclick={analyze} icon={RotateCcw}>
        Try Again
      </Button>

    {:else if accepted}
      <!-- Success state -->
      <Card variant="raised" padding="default">
        <div class="flex flex-col items-center py-6 gap-3">
          <div class="rounded-full p-3" style="background-color: color-mix(in srgb, var(--ds-icon-success) 15%, transparent);">
            <CheckCircle size={28} style="color: var(--ds-icon-success);" />
          </div>
          <p class="text-sm font-medium" style="color: var(--ds-text);">
            Created {acceptResult?.created || 0} dependency link{acceptResult?.created !== 1 ? 's' : ''}
            {#if acceptResult?.skipped > 0}
              <span style="color: var(--ds-text-subtle);"> ({acceptResult.skipped} already existed)</span>
            {/if}
          </p>
          <div class="flex items-center gap-3">
            <Button variant="primary" onclick={() => navigate(`/iterations/${iterationId}`)}>
              Back to Iteration
            </Button>
            <Button variant="secondary" onclick={analyze} icon={RotateCcw}>
              Analyze Again
            </Button>
          </div>
        </div>
      </Card>

    {:else if result}
      <!-- Results view -->

      <!-- Summary card -->
      <Card variant="raised" padding="default">
        <div class="text-sm space-y-1" style="color: var(--ds-text-subtle);">
          <p>
            <span class="font-medium" style="color: var(--ds-text);">{result.items_analyzed}</span> items analyzed
            across <span class="font-medium" style="color: var(--ds-text);">{result.workspaces_included?.length || 0}</span> workspace{result.workspaces_included?.length !== 1 ? 's' : ''}
          </p>
          {#if result.existing_links_filtered > 0}
            <p>{result.existing_links_filtered} existing link{result.existing_links_filtered !== 1 ? 's' : ''} filtered</p>
          {/if}
          {#if result.iterations_included?.length > 0}
            <p>Iterations: {result.iterations_included.join(', ')}</p>
          {/if}
        </div>
      </Card>

      {#if result.suggestions && result.suggestions.length > 0}
        <!-- Select all bar -->
        <div class="flex items-center gap-2 pb-2 border-b" style="border-color: var(--ds-border);">
          <button
            class="inline-flex items-center gap-2 text-xs font-medium transition-colors"
            style="color: var(--ds-text-subtle);"
            onclick={toggleAll}
          >
            {#if allSelected}
              <CheckSquare class="w-4 h-4" style="color: var(--ds-interactive);" />
            {:else}
              <Square class="w-4 h-4" />
            {/if}
            {allSelected ? 'Deselect all' : 'Select all'}
          </button>
          <span class="text-xs" style="color: var(--ds-text-subtle);">
            ({selected.size} of {result.suggestions.length} selected)
          </span>
        </div>

        <!-- Suggestion cards -->
        <div class="space-y-2">
          {#each result.suggestions as suggestion, i}
            <button
              class="w-full text-left p-4 rounded-lg border transition-colors"
              style="border-color: {selected.has(i) ? 'var(--ds-interactive)' : 'var(--ds-border)'}; background-color: {selected.has(i) ? 'var(--ds-surface-selected)' : 'var(--ds-surface-raised)'};"
              onclick={() => toggleItem(i)}
            >
              <div class="flex items-start gap-3">
                <!-- Checkbox -->
                <div class="flex-shrink-0 mt-0.5">
                  {#if selected.has(i)}
                    <CheckSquare class="w-4 h-4" style="color: var(--ds-interactive);" />
                  {:else}
                    <Square class="w-4 h-4" style="color: var(--ds-text-subtle);" />
                  {/if}
                </div>

                <!-- Content -->
                <div class="flex-1 min-w-0 space-y-2">
                  <!-- Source item -->
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="text-xs font-mono px-1.5 py-0.5 rounded" style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);">
                      {suggestion.source_item_key}
                    </span>
                    <span class="text-sm font-medium truncate" style="color: var(--ds-text);">
                      {suggestion.source_item_title}
                    </span>
                  </div>

                  <!-- Arrow -->
                  <div class="flex items-center gap-2 pl-1">
                    <ArrowRight class="w-3.5 h-3.5" style="color: var(--ds-text-subtlest);" />
                    <span class="text-xs" style="color: var(--ds-text-subtlest);">
                      {relationshipLabels[suggestion.relationship] || suggestion.relationship}
                    </span>
                  </div>

                  <!-- Target item -->
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="text-xs font-mono px-1.5 py-0.5 rounded" style="background-color: var(--ds-surface-sunken); color: var(--ds-text-subtle);">
                      {suggestion.target_item_key}
                    </span>
                    <span class="text-sm font-medium truncate" style="color: var(--ds-text);">
                      {suggestion.target_item_title}
                    </span>
                  </div>

                  <!-- Lozenges + Reason -->
                  <div class="flex items-center gap-2 flex-wrap mt-1">
                    <Lozenge
                      color={relationshipColors[suggestion.relationship] || 'blue'}
                      text={relationshipLabels[suggestion.relationship] || suggestion.relationship}
                    />
                    {#if suggestion.cross_iteration}
                      <Lozenge color="blue" text="Cross-Sprint" />
                    {/if}
                  </div>
                  {#if suggestion.reason}
                    <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">
                      {suggestion.reason}
                    </p>
                  {/if}
                </div>
              </div>
            </button>
          {/each}
        </div>

        <!-- Action bar -->
        <div class="flex items-center gap-3">
          <Button variant="primary" onclick={handleAccept} disabled={selected.size === 0} loading={accepting}>
            Accept Selected ({selected.size})
          </Button>
          <Button variant="secondary" onclick={analyze} icon={RotateCcw}>
            Re-analyze
          </Button>
        </div>
      {:else}
        <!-- Empty state -->
        <Card variant="raised" padding="default">
          <div class="flex flex-col items-center py-6 gap-3">
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              No dependencies detected between items in this iteration.
            </p>
            <div class="flex items-center gap-3">
              <Button variant="primary" onclick={() => navigate(`/iterations/${iterationId}`)}>
                Back to Iteration
              </Button>
              <Button variant="secondary" onclick={analyze} icon={RotateCcw}>
                Re-analyze
              </Button>
            </div>
          </div>
        </Card>
      {/if}
    {/if}
  </div>
  </div>
</div>
