<script>
  import { Search, Users, Plus } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Avatar from '../components/Avatar.svelte';

  let {
    organisations = [],
    selectedOrgId = null,
    unassignedCount = 0,
    searchQuery = $bindable(''),
    customerCounts = {},
    dragOverOrgId = undefined,
    onSelect = () => {},
    onManageOrgs = () => {}
  } = $props();

  let filteredOrgs = $derived(
    organisations.filter(org =>
      org.name.toLowerCase().includes(searchQuery.toLowerCase())
    )
  );

  function getButtonStyle(orgId, isActive, isDragOver) {
    if (isDragOver) {
      return 'background: rgba(59, 130, 246, 0.1); color: var(--ds-text); ring: 2px solid rgba(59, 130, 246, 0.5);';
    }
    if (isActive) {
      return 'background: var(--ds-surface-selected); color: var(--ds-text);';
    }
    return 'color: var(--ds-text-subtle);';
  }
</script>

<div class="w-64 min-w-64 flex-shrink-0 border-r flex flex-col p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
  <!-- Header -->
  <div class="mb-6">
    <h2 class="text-xl font-semibold" style="color: var(--ds-text);">Customers</h2>
    <p class="text-sm" style="color: var(--ds-text-subtle);">Portal customer management</p>
  </div>

  <!-- Search -->
  <div class="relative mb-4">
    <Search class="w-4 h-4 absolute left-2.5 top-1/2 -translate-y-1/2" style="color: var(--ds-icon-subtle);" />
    <input
      type="text"
      bind:value={searchQuery}
      placeholder="Search organisations..."
      class="w-full pl-9 pr-3 py-2 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
      style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
    />
  </div>

  <!-- Navigation -->
  <nav class="flex-1 overflow-y-auto space-y-1 -mx-1 px-1 -mt-1 pt-1">
    <!-- Unassigned -->
    <button
      data-org-id="null"
      onclick={() => onSelect(null)}
      class="w-full text-left px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 {dragOverOrgId === null ? 'ring-2 ring-blue-400' : ''}"
      style={getButtonStyle(null, selectedOrgId === null, dragOverOrgId === null)}
      onmouseenter={(e) => { if (selectedOrgId !== null && dragOverOrgId !== null) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
      onmouseleave={(e) => { if (selectedOrgId !== null && dragOverOrgId !== null) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
    >
      <div class="w-6 h-6 flex items-center justify-center flex-shrink-0">
        <Users class="w-4 h-4" />
      </div>
      <span class="flex-1 truncate">Unassigned</span>
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-interactive-subtle);">{unassignedCount}</span>
    </button>

    <!-- Organisation List -->
    {#each filteredOrgs as org (org.id)}
      {@const isActive = selectedOrgId === org.id}
      {@const isDragOver = dragOverOrgId === org.id}
      <button
        data-org-id={org.id}
        onclick={() => onSelect(org.id)}
        class="w-full text-left px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 {isDragOver ? 'ring-2 ring-blue-400' : ''}"
        style={getButtonStyle(org.id, isActive, isDragOver)}
        onmouseenter={(e) => { if (!isActive && !isDragOver) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if (!isActive && !isDragOver) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
      >
        <Avatar
          src={org.avatar_url}
          name={org.name}
          size="xs"
          variant="blue"
          rounded="md"
        />
        <span class="flex-1 truncate">{org.name}</span>
        <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-interactive-subtle);">{customerCounts[org.id] || 0}</span>
      </button>
    {/each}

    {#if filteredOrgs.length === 0 && searchQuery}
      <div class="text-center py-4" style="color: var(--ds-text-subtle);">
        <p class="text-sm">No organisations found</p>
      </div>
    {/if}
  </nav>

  <!-- Footer -->
  <div class="pt-4 border-t" style="border-color: var(--ds-border);">
    <Button variant="default" icon={Plus} onclick={onManageOrgs} class="w-full justify-center">
      Manage Organisations
    </Button>
  </div>
</div>
