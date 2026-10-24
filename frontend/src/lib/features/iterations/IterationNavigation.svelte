<script>
  import { onMount } from 'svelte';
  import { navigate, currentRoute } from '../../router.js';
  import { iterationTypes as iterationTypesApi } from '../../api/milestones.js';
  import { getHexFromColorName } from '../../utils/colors.js';
  import { t } from '../../stores/i18n.svelte.js';
  import NavigationSidebar from '../../layout/NavigationSidebar.svelte';

  let iterationTypes = $state([]);

  // Get active type from URL params
  let activeTypeId = $derived($currentRoute.params?.typeId || null);
  let isAllActive = $derived(activeTypeId === null);

  onMount(async () => {
    await loadIterationTypes();
  });

  async function loadIterationTypes() {
    try {
      iterationTypes = await iterationTypesApi.getAll() || [];
    } catch (err) {
      console.error('Failed to load iteration types:', err);
      iterationTypes = [];
    }
  }

  function typeHref(typeId) {
    return typeId === null ? '/iterations' : `/iterations/type/${typeId}`;
  }

</script>

<!-- Iteration Navigation Sidebar -->
<NavigationSidebar title={t('iterations.title')} description={t('iterations.subtitle')}>
  <!-- Navigation -->
  <nav class="flex-1 space-y-1">
    <!-- All Types -->
    <a
      href={typeHref(null)}
      class="nav-link w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 no-underline"
      class:active={isAllActive}
    >
      <div class="w-4 h-4 rounded bg-gradient-to-br from-ds-nav-teal-from to-ds-nav-teal-to flex-shrink-0"></div>
      <span>{t('iterations.allTypes')}</span>
    </a>

    <!-- Type List -->
    {#each iterationTypes as type (type.id)}
      {@const isTypeActive = activeTypeId === type.id.toString()}
      <a
        href={typeHref(type.id)}
        class="nav-link w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 no-underline"
        class:active={isTypeActive}
        title={type.description || type.name}
      >
        <div
          class="w-4 h-4 rounded flex-shrink-0"
          style="background-color: {type.color?.startsWith('#') ? type.color : getHexFromColorName(type.color || 'teal')};"
        ></div>
        <span class="truncate">{type.name}</span>
      </a>
    {/each}
  </nav>

</NavigationSidebar>


<style>
  .nav-link {
    color: var(--ds-text-subtle);
  }

  .nav-link:hover:not(.active) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  .nav-link.active {
    background: var(--ds-surface-selected);
    color: var(--ds-text);
  }
</style>
