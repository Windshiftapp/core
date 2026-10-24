<script>
  import Search from '@lucide/svelte/icons/search';
  import Users from '@lucide/svelte/icons/users';
  import Plus from '@lucide/svelte/icons/plus';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Avatar from '../components/Avatar.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import SidebarHeader from '../layout/SidebarHeader.svelte';
  import NavigationSidebar from '../layout/NavigationSidebar.svelte';

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

</script>

{#snippet sidebarHeader()}
  <div class="px-6 pt-6 pb-3">
    <SidebarHeader title={t('workspaces.customers.title')} description={t('workspaces.customers.subtitle')} noBorder />

    <!-- Search remains available while the organisation list scrolls. -->
    <div class="relative">
      <Search class="w-4 h-4 absolute left-2.5 top-1/2 -translate-y-1/2" style="color: var(--ds-icon-subtle);" />
      <Input
        type="text"
        bind:value={searchQuery}
        placeholder={t('workspaces.customers.searchOrganisations')}
        class="pl-9"
        size="small"
      />
    </div>
  </div>
{/snippet}

{#snippet sidebarFooter()}
  <div class="px-6 pb-6 pt-4 border-t" style="border-color: var(--ds-border);">
    <Button variant="default" icon={Plus} onclick={onManageOrgs} class="w-full justify-center">
      {t('workspaces.customers.manageOrganisations')}
    </Button>
  </div>
{/snippet}

<NavigationSidebar
  title={t('workspaces.customers.title')}
  header={sidebarHeader}
  footer={sidebarFooter}
  contentClass="px-6 pb-6"
>
  <!-- Navigation -->
  <nav class="space-y-1 -mx-1 px-1 -mt-1 pt-1">
    <!-- Unassigned -->
    <button
      data-org-id="null"
      data-testid="customer-organization-unassigned"
      onclick={() => onSelect(null)}
      class="org-nav-btn w-full text-left px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 {dragOverOrgId === null ? 'ring-2 ring-ds-border-focused' : ''}"
      class:active={selectedOrgId === null}
      class:drag-over={dragOverOrgId === null}
    >
      <div class="w-6 h-6 flex items-center justify-center flex-shrink-0">
        <Users class="w-4 h-4" />
      </div>
      <span class="flex-1 truncate">{t('workspaces.customers.unassigned')}</span>
      <span class="text-xs px-1.5 py-0.5 rounded" style="background: var(--ds-interactive-subtle);">{unassignedCount}</span>
    </button>

    <!-- Organisation List -->
    {#each filteredOrgs as org (org.id)}
      {@const isActive = selectedOrgId === org.id}
      {@const isDragOver = dragOverOrgId === org.id}
      <button
        data-org-id={org.id}
        data-testid="customer-organization"
        onclick={() => onSelect(org.id)}
        class="org-nav-btn w-full text-left px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 {isDragOver ? 'ring-2 ring-ds-border-focused' : ''}"
        class:active={isActive}
        class:drag-over={isDragOver}
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
        <p class="text-sm">{t('workspaces.customers.noOrganisationsFound')}</p>
      </div>
    {/if}

    {#if organisations.length > 15 && !searchQuery}
      <p class="text-xs text-center py-2 px-2" style="color: var(--ds-text-subtlest);">
        {t('workspaces.customers.searchHintOrganisations')}
      </p>
    {/if}
  </nav>

</NavigationSidebar>

<style>
  .org-nav-btn {
    color: var(--ds-text-subtle);
  }

  .org-nav-btn:hover:not(.active):not(.drag-over) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  .org-nav-btn.active {
    background: var(--ds-surface-selected);
    color: var(--ds-text);
  }

  .org-nav-btn.drag-over {
    background: rgba(59, 130, 246, 0.1);
    color: var(--ds-text);
  }
</style>
