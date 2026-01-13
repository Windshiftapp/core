<script>
  import { onMount } from 'svelte';
  import { Target } from 'lucide-svelte';
  import { navigate, currentRoute } from '../../router.js';
  import { api } from '../../api.js';
  import { getHexFromColorName } from '../../utils/colors.js';

  let iterationTypes = $state([]);

  // Get active type from URL params
  let activeTypeId = $derived($currentRoute.params?.typeId || null);
  let isAllActive = $derived(activeTypeId === null);

  onMount(async () => {
    await loadIterationTypes();
  });

  async function loadIterationTypes() {
    try {
      iterationTypes = await api.iterationTypes.getAll() || [];
    } catch (err) {
      console.error('Failed to load iteration types:', err);
      iterationTypes = [];
    }
  }

  function handleTypeClick(typeId) {
    if (typeId === null) {
      navigate('/iterations');
    } else {
      navigate(`/iterations/type/${typeId}`);
    }
  }

  function handleManageTypes() {
    // Emit event to parent to show type management modal
    const event = new CustomEvent('manage-iteration-types');
    document.dispatchEvent(event);
  }
</script>

<!-- Iteration Navigation Sidebar -->
<div class="w-64 border-r flex flex-col p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
  <!-- Header -->
  <div class="mb-6">
    <div class="flex items-center gap-3 mb-2">
      <h2 class="text-xl font-semibold" style="color: var(--ds-text);">Iterations</h2>
    </div>
    <p class="text-sm" style="color: var(--ds-text-subtle);">Manage sprints and releases</p>
  </div>

  <!-- Navigation -->
  <nav class="flex-1 space-y-1">
    <!-- All Types -->
    <button
      onclick={() => handleTypeClick(null)}
      class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
      style={isAllActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
      onmouseenter={(e) => { if (!isAllActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
      onmouseleave={(e) => { if (!isAllActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
    >
      <div class="w-4 h-4 rounded bg-gradient-to-br from-teal-400 to-teal-600 flex-shrink-0"></div>
      <span>All Types</span>
    </button>

    <!-- Type List -->
    {#each iterationTypes as type (type.id)}
      {@const isTypeActive = activeTypeId === type.id.toString()}
      <button
        onclick={() => handleTypeClick(type.id)}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
        style={isTypeActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if (!isTypeActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if (!isTypeActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
        title={type.description || type.name}
      >
        <div
          class="w-4 h-4 rounded flex-shrink-0"
          style="background-color: {type.color?.startsWith('#') ? type.color : getHexFromColorName(type.color || 'teal')};"
        ></div>
        <span class="truncate">{type.name}</span>
      </button>
    {/each}
  </nav>

  <!-- Footer - Manage Types -->
  <div class="pt-4 border-t" style="border-color: var(--ds-border);">
    <button
      onclick={handleManageTypes}
      class="w-full px-4 py-2 rounded-lg text-sm font-medium flex items-center justify-center gap-2 transition-colors"
      style="background-color: var(--ds-surface); border: 1px solid var(--ds-border); color: var(--ds-text);"
      onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered)'}
      onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface)'}
    >
      <Target class="w-4 h-4" />
      Manage Types
    </button>
  </div>
</div>
