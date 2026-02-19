<script>
  import { ExternalLink, FileText, GripVertical, X } from 'lucide-svelte';
  import { hubStore, gradients } from '../stores/hub.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  let { portal, sectionId = null, draggable = false, onRemove = null } = $props();

  function handleClick() {
    if (!hubStore.isEditing && portal.slug) {
      window.location.href = `/portal/${portal.slug}`;
    }
  }

  function handleDragStart(e) {
    if (draggable && hubStore.isEditing) {
      hubStore.draggedPortal = { portal, sectionId };
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', JSON.stringify({ portalId: portal.id, sectionId }));
    }
  }

  function handleDragEnd() {
    hubStore.draggedPortal = null;
  }
</script>

<div
  class="portal-card group relative rounded-lg overflow-hidden cursor-pointer"
  class:border-2={hubStore.isEditing}
  class:border-dashed={hubStore.isEditing}
  onclick={handleClick}
  draggable={draggable && hubStore.isEditing}
  ondragstart={handleDragStart}
  ondragend={handleDragEnd}
  style="border-color: var(--ds-border);"
>
  <!-- Gradient Header -->
  <div
    class="h-24 relative"
    style="background: {gradients[portal.gradient || 0].value};"
  >
    <!-- Dark mode overlay -->
    {#if hubStore.isDarkMode}
      <div class="absolute inset-0 bg-black/30"></div>
    {/if}

    <!-- Drag handle (edit mode only) -->
    {#if hubStore.isEditing && draggable}
      <div class="absolute top-2 left-2 p-1 rounded bg-black/30 text-white/80 cursor-grab">
        <GripVertical class="w-4 h-4" />
      </div>
    {/if}

    <!-- Remove button (edit mode only) -->
    {#if hubStore.isEditing && onRemove}
      <button
        onclick={(e) => { e.stopPropagation(); onRemove(portal.id); }}
        class="absolute top-2 right-2 p-1 rounded bg-red-500/80 text-white hover:bg-red-600 transition-colors"
        title={t('common.remove', 'Remove')}
      >
        <X class="w-4 h-4" />
      </button>
    {/if}
  </div>

  <!-- Card Content -->
  <div class="p-4" style="background-color: var(--ds-surface-card);">
    <h3 class="text-lg mb-1 truncate" style="color: var(--ds-text);">
      {portal.name}
    </h3>

    {#if portal.description}
      <p class="text-sm mb-3 line-clamp-2" style="color: var(--ds-text-subtle);">
        {portal.description}
      </p>
    {:else}
      <p class="text-sm mb-3" style="color: var(--ds-text-subtle);">
        {t('hub.noDescription', 'No description')}
      </p>
    {/if}

    <div class="flex items-center justify-between">
      <span class="text-xs flex items-center gap-1" style="color: var(--ds-text-subtle);">
        <FileText class="w-3 h-3" />
        {portal.request_type_count || 0} {t('hub.requestTypes', 'request types')}
      </span>

      {#if !hubStore.isEditing}
        <span class="text-xs flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity" style="color: var(--ds-text-subtle);">
          {t('common.open', 'Open')}
          <ExternalLink class="w-3 h-3" />
        </span>
      {/if}
    </div>
  </div>
</div>

<style>
  .portal-card {
    min-width: 0;
    box-shadow: var(--ds-shadow-raised);
    transition: background-color 140ms ease-in-out, box-shadow 140ms ease-in-out;
  }

  .portal-card:hover {
    background-color: var(--ds-surface-raised-hovered) !important;
  }

  .line-clamp-2 {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
</style>
