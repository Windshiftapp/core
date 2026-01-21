<script>
  import { Plus, ChevronUp, ChevronDown, Trash2, GripVertical } from 'lucide-svelte';
  import { hubStore, gradients } from '../stores/hub.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import HubPortalCard from './HubPortalCard.svelte';

  function handleDrop(e, sectionId) {
    e.preventDefault();
    const data = e.dataTransfer.getData('text/plain');
    if (data) {
      try {
        const { portalId, sectionId: fromSectionId } = JSON.parse(data);
        if (fromSectionId) {
          // Move portal from one section to another
          hubStore.removePortalFromSection(fromSectionId, portalId);
        }
        hubStore.addPortalToSection(sectionId, portalId);
      } catch (err) {
        console.error('Drop error:', err);
      }
    }
    hubStore.draggedPortal = null;
  }

  function handleDragOver(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
  }
</script>

<!-- Sections Container -->
<div class="space-y-8">
  {#if hubStore.hubSections.length === 0 && !hubStore.isEditing}
    <!-- Default view: Show all portals in a grid -->
    {#if hubStore.portals.length > 0}
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {#each hubStore.portals as portal (portal.id)}
          <HubPortalCard {portal} />
        {/each}
      </div>
    {:else}
      <div class="text-center py-12">
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          {t('hub.noPortals', 'No portals available yet')}
        </p>
      </div>
    {/if}
  {:else}
    <!-- Configured sections -->
    {#each hubStore.hubSections as section, index (section.id)}
      <div
        class="section-container"
        class:border-2={hubStore.isEditing}
        class:border-dashed={hubStore.isEditing}
        class:rounded-lg={hubStore.isEditing}
        class:p-4={hubStore.isEditing}
        style="border-color: {hubStore.isEditing ? 'var(--ds-border)' : 'transparent'};"
        ondrop={(e) => handleDrop(e, section.id)}
        ondragover={handleDragOver}
      >
        <!-- Section Header -->
        <div class="flex items-start justify-between mb-4">
          <div class="flex-1">
            {#if hubStore.isEditing}
              <input
                type="text"
                value={section.title}
                oninput={(e) => hubStore.updateSection(section.id, 'title', e.target.value)}
                class="text-lg font-semibold w-full bg-transparent focus:outline-none border-b border-dashed"
                style="color: var(--ds-text); border-color: var(--ds-border);"
                placeholder="Section title"
              />
              <input
                type="text"
                value={section.subtitle || ''}
                oninput={(e) => hubStore.updateSection(section.id, 'subtitle', e.target.value)}
                class="text-sm mt-1 w-full bg-transparent focus:outline-none border-b border-dashed"
                style="color: var(--ds-text-subtle); border-color: var(--ds-border);"
                placeholder="Section subtitle (optional)"
              />
            {:else}
              {#if section.title}
                <h2 class="text-lg font-semibold" style="color: var(--ds-text);">
                  {section.title}
                </h2>
              {/if}
              {#if section.subtitle}
                <p class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">
                  {section.subtitle}
                </p>
              {/if}
            {/if}
          </div>

          <!-- Section Controls (edit mode) -->
          {#if hubStore.isEditing}
            <div class="flex items-center gap-1 ml-4">
              <button
                onclick={() => hubStore.moveSectionUp(index)}
                disabled={index === 0}
                class="p-1 rounded hover:bg-black/5 disabled:opacity-30"
                title="Move up"
              >
                <ChevronUp class="w-5 h-5" style="color: var(--ds-text-subtle);" />
              </button>
              <button
                onclick={() => hubStore.moveSectionDown(index)}
                disabled={index === hubStore.hubSections.length - 1}
                class="p-1 rounded hover:bg-black/5 disabled:opacity-30"
                title="Move down"
              >
                <ChevronDown class="w-5 h-5" style="color: var(--ds-text-subtle);" />
              </button>
              <button
                onclick={() => hubStore.deleteSection(section.id)}
                class="p-1 rounded hover:bg-red-100 text-red-600"
                title="Delete section"
              >
                <Trash2 class="w-5 h-5" />
              </button>
            </div>
          {/if}
        </div>

        <!-- Section Portals -->
        {#if hubStore.getSectionPortals(section).length > 0}
          <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {#each hubStore.getSectionPortals(section) as portal (portal.id)}
              <HubPortalCard
                {portal}
                sectionId={section.id}
                draggable={true}
                onRemove={(portalId) => hubStore.removePortalFromSection(section.id, portalId)}
              />
            {/each}
          </div>
        {:else if hubStore.isEditing}
          <div
            class="border-2 border-dashed rounded-lg p-8 text-center"
            style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
          >
            <p style="color: var(--ds-text-subtle);">
              {t('hub.dropPortalsHere', 'Drag portals here or add them from the customize panel')}
            </p>
          </div>
        {/if}
      </div>
    {/each}

    <!-- Unassigned Portals (edit mode only) -->
    {#if hubStore.isEditing}
      {@const unassigned = hubStore.getUnassignedPortals()}
      {#if unassigned.length > 0}
        <div class="mt-6 pt-6 border-t" style="border-color: var(--ds-border);">
          <h3 class="text-base font-semibold mb-3" style="color: var(--ds-text);">
            {t('hub.unassignedPortals', 'Unassigned Portals')}
          </h3>
          <p class="text-sm mb-3" style="color: var(--ds-text-subtle);">
            {t('hub.dragToSection', 'Drag these portals to a section above')}
          </p>
          <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {#each unassigned as portal (portal.id)}
              <HubPortalCard {portal} draggable={true} />
            {/each}
          </div>
        </div>
      {/if}

      <!-- Add Section Button -->
      <div class="mt-6 flex justify-center">
        <button
          onclick={() => hubStore.addSection()}
          class="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg border-2 border-dashed transition-all hover:border-solid"
          style="border-color: var(--ds-border); color: var(--ds-text-subtle);"
        >
          <Plus class="w-4 h-4" />
          <span>{t('hub.addSection', 'Add Section')}</span>
        </button>
      </div>
    {/if}
  {/if}
</div>
