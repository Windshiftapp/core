<script>
  import { Search, Plus, X, AlertCircle } from 'lucide-svelte';
  import { itemTypeIconMap, priorityIconMap } from '../utils/icons.js';
  import { t } from '../stores/i18n.svelte.js';

  // Entity type determines rendering behavior
  let {
    entityType = 'priorities', // 'priorities' | 'item-types' | 'workspaces'
    allEntities = [],
    selectedIds = [],
    configurationSetId = null, // For conflict detection
    entityAssignments = {}, // Maps entity ID to other config sets (for workspaces)
    onchange
  } = $props();

  let searchQuery = $state('');

  // Filter entities by search query
  const filteredEntities = $derived(allEntities.filter(entity => {
    const name = entity.name || '';
    const key = entity.key || '';
    const description = entity.description || '';
    const query = searchQuery.toLowerCase();
    return name.toLowerCase().includes(query) ||
           key.toLowerCase().includes(query) ||
           description.toLowerCase().includes(query);
  }));

  // Split into assigned and available
  const assignedEntities = $derived(filteredEntities.filter(e => selectedIds.includes(e.id)));
  const availableEntities = $derived(filteredEntities.filter(e => !selectedIds.includes(e.id)));

  function addEntity(entityId) {
    // Check for conflicts (workspaces assigned elsewhere)
    if (entityAssignments[entityId]) {
      const assignment = entityAssignments[entityId];
      if (!confirm(t('pickers.entityAlreadyAssigned', { entity: getEntityLabel(), configSetName: assignment.configSetName }))) {
        return;
      }
    }
    onchange?.([...selectedIds, entityId]);
  }

  function removeEntity(entityId) {
    onchange?.(selectedIds.filter(id => id !== entityId));
  }

  function getEntityLabel() {
    switch (entityType) {
      case 'priorities': return t('common.priority');
      case 'item-types': return t('pickers.itemType');
      case 'workspaces': return t('workspaces.workspace');
      default: return t('common.item');
    }
  }

  function getEntityLabelPlural() {
    switch (entityType) {
      case 'priorities': return t('pickers.priorities');
      case 'item-types': return t('pickers.itemTypes');
      case 'workspaces': return t('workspaces.title');
      default: return t('common.items');
    }
  }
</script>

<div class="space-y-4">
  <!-- Search input -->
  <div class="relative">
    <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4" style="color: var(--ds-icon-subtle);" />
    <input
      type="text"
      placeholder={t('pickers.searchEntities', { entities: getEntityLabelPlural() })}
      bind:value={searchQuery}
      class="w-full pl-9 pr-4 py-2 border rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
      style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
    />
  </div>

  <!-- Assigned entities -->
  <div>
    <h4 class="text-sm font-medium mb-2" style="color: var(--ds-text);">
      {t('pickers.assigned')} ({assignedEntities.length})
    </h4>
    {#if assignedEntities.length === 0}
      <div class="p-4 text-center border rounded-lg" style="border-color: var(--ds-border); color: var(--ds-text-subtle);">
        {t('pickers.noEntitiesAssigned', { entities: getEntityLabelPlural() })}
      </div>
    {:else}
      <div class="border rounded-lg" style="border-color: var(--ds-border);">
        {#each assignedEntities as entity, i}
          <div class="flex items-center justify-between p-3" style="background-color: var(--ds-surface);{i > 0 ? ' border-top: 1px solid var(--ds-border);' : ''}">
            <div class="flex items-center gap-3 min-w-0">
              {#if entityType === 'priorities' || entityType === 'item-types'}
                <div
                  class="w-6 h-6 rounded flex items-center justify-center flex-shrink-0"
                  style="background-color: {entityType === 'item-types' ? entity.color || '#3b82f6' : (entity.color || '#3b82f6') + '20'};"
                >
                  {#if entityType === 'item-types' && entity.icon && itemTypeIconMap[entity.icon]}
                    <svelte:component this={itemTypeIconMap[entity.icon]} class="w-4 h-4 text-white" />
                  {:else if entityType === 'priorities' && entity.icon && priorityIconMap[entity.icon]}
                    <svelte:component this={priorityIconMap[entity.icon]} class="w-4 h-4" style="color: {entity.color || '#3b82f6'};" />
                  {:else}
                    <span style="color: {entityType === 'item-types' ? 'white' : entity.color || '#3b82f6'}; font-size: 12px;" class="font-medium">
                      {entity.name?.charAt(0) || '?'}
                    </span>
                  {/if}
                </div>
              {/if}
              <div class="min-w-0">
                <div class="font-medium text-sm truncate" style="color: var(--ds-text);">
                  {entity.name}
                </div>
                {#if entityType === 'workspaces' && entity.key}
                  <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
                    {entity.key}
                  </div>
                {:else if entityType === 'item-types' && entity.hierarchy_level !== undefined}
                  <div class="text-xs" style="color: var(--ds-text-subtle);">
                    {t('pickers.level')} {entity.hierarchy_level}
                  </div>
                {/if}
              </div>
            </div>
            <button
              type="button"
              onclick={() => removeEntity(entity.id)}
              class="p-1 rounded hover:bg-red-50 transition-colors flex-shrink-0"
              style="color: var(--ds-text-subtle);"
              title={t('common.remove')}
            >
              <X class="w-4 h-4" />
            </button>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <!-- Available entities -->
  <div>
    <h4 class="text-sm font-medium mb-2" style="color: var(--ds-text);">
      {t('pickers.available')} ({availableEntities.length})
    </h4>
    {#if availableEntities.length === 0}
      <div class="p-4 text-center border rounded-lg" style="border-color: var(--ds-border); color: var(--ds-text-subtle);">
        {#if searchQuery}
          {t('pickers.noEntitiesMatchSearch', { entities: getEntityLabelPlural() })}
        {:else}
          {t('pickers.allEntitiesAssigned', { entities: getEntityLabelPlural() })}
        {/if}
      </div>
    {:else}
      <div class="border rounded-lg max-h-60 overflow-y-auto" style="border-color: var(--ds-border);">
        {#each availableEntities as entity, i}
          {@const otherAssignment = entityAssignments[entity.id]}
          <div class="flex items-center justify-between p-3" style="background-color: var(--ds-surface);{i > 0 ? ' border-top: 1px solid var(--ds-border);' : ''}">
            <div class="flex items-center gap-3 min-w-0 flex-1">
              {#if entityType === 'priorities' || entityType === 'item-types'}
                <div
                  class="w-6 h-6 rounded flex items-center justify-center flex-shrink-0"
                  style="background-color: {entityType === 'item-types' ? entity.color || '#3b82f6' : (entity.color || '#3b82f6') + '20'};"
                >
                  {#if entityType === 'item-types' && entity.icon && itemTypeIconMap[entity.icon]}
                    <svelte:component this={itemTypeIconMap[entity.icon]} class="w-4 h-4 text-white" />
                  {:else if entityType === 'priorities' && entity.icon && priorityIconMap[entity.icon]}
                    <svelte:component this={priorityIconMap[entity.icon]} class="w-4 h-4" style="color: {entity.color || '#3b82f6'};" />
                  {:else}
                    <span style="color: {entityType === 'item-types' ? 'white' : entity.color || '#3b82f6'}; font-size: 12px;" class="font-medium">
                      {entity.name?.charAt(0) || '?'}
                    </span>
                  {/if}
                </div>
              {/if}
              <div class="min-w-0 flex-1">
                <div class="font-medium text-sm truncate" style="color: var(--ds-text);">
                  {entity.name}
                </div>
                {#if entityType === 'workspaces' && entity.key}
                  <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
                    {entity.key}
                  </div>
                {:else if entityType === 'item-types' && entity.hierarchy_level !== undefined}
                  <div class="text-xs" style="color: var(--ds-text-subtle);">
                    {t('pickers.level')} {entity.hierarchy_level}
                  </div>
                {/if}
              </div>
              {#if otherAssignment}
                <div class="flex items-center gap-1 text-xs px-2 py-1 rounded flex-shrink-0" style="background-color: var(--ds-surface-warning); color: var(--ds-text-warning);">
                  <AlertCircle class="w-3 h-3" />
                  <span class="truncate max-w-32">{t('pickers.inConfigSet', { name: otherAssignment.configSetName })}</span>
                </div>
              {/if}
            </div>
            <button
              type="button"
              onclick={() => addEntity(entity.id)}
              class="p-1 rounded hover:bg-blue-50 transition-colors flex-shrink-0 ml-2"
              style="color: var(--ds-interactive);"
              title={t('common.add')}
            >
              <Plus class="w-4 h-4" />
            </button>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>
