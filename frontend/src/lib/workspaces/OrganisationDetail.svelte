<script>
  import Users from '@tabler/icons-svelte-runes/icons/users';
  import Mail from '@tabler/icons-svelte-runes/icons/mail';
  import Search from '@tabler/icons-svelte-runes/icons/search';
  import GripVertical from '@tabler/icons-svelte-runes/icons/grip-vertical';
  import Plus from '@tabler/icons-svelte-runes/icons/plus';
  import Edit2 from '@tabler/icons-svelte-runes/icons/edit';
  import Trash2 from '@tabler/icons-svelte-runes/icons/trash';
  import MoreHorizontal from '@tabler/icons-svelte-runes/icons/dots';
  import Ticket from '@tabler/icons-svelte-runes/icons/ticket';
  import Share from '@tabler/icons-svelte-runes/icons/share';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import Avatar from '../components/Avatar.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import Card from '../components/Card.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Tabs from '../components/Tabs.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import DataTable from '../components/DataTable.svelte';
  import ItemKey from '../components/ItemKey.svelte';
  import Select from '../components/Select.svelte';
  import Label from '../components/Label.svelte';
  import Checkbox from '../components/Checkbox.svelte';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { contactRoles as contactRolesApi, customerOrganisations } from '../api/portal.js';
  import { formatDateShort } from '../utils/dateFormatter.js';
  import { itemUrl } from '../utils/urls.js';

  let {
    organisation = null,
    customers = [],
    filteredCount = 0,
    displayLimit = $bindable(15),
    customerSearch = $bindable(''),
    canManage = false,
    showCreateModal = false,
    onStartCreate = () => {},
    onOpenDetail = () => {},
    onDeleteCustomer = () => {},
    hasMoreCustomers = false,
    onLoadMore = () => {},
    buildCustomerActions = () => [],
    onOrganisationUpdated = () => {},
  } = $props();

  let activeTab = $state('contacts');

  let tabs = $derived([
    { id: 'contacts', label: t('workspaces.customers.contacts') || 'Contacts', icon: Users, testid: 'org-detail-contacts-tab' },
    { id: 'tickets', label: t('common.tickets') || 'Tickets', icon: Ticket, testid: 'org-detail-tickets-tab' },
    ...(canManage ? [{ id: 'sharing', label: t('workspaces.customers.sharing') || 'Sharing', icon: Share, testid: 'org-detail-sharing-tab' }] : []),
  ]);

  // Request sharing tab (WI-1139). Settings live in the organisation's flexible
  // settings blob; only customers.manage holders see the tab.
  let sharingMode = $state('disabled');
  let sharingAudience = $state('organisation');
  let sharingRoleIDs = $state([]);
  let sharingSaving = $state(false);
  let sharingSaved = $state(false);
  let contactRoles = $state([]);
  let contactRolesLoaded = $state(false);
  let initializedSharingOrgID = $state(null);

  $effect(() => {
    if (!organisation?.id || organisation.id === initializedSharingOrgID) return;
    initializedSharingOrgID = organisation.id;
    const settings = organisation.settings || {};
    sharingMode = settings.request_sharing || 'disabled';
    sharingAudience = settings.request_sharing_audience || 'organisation';
    sharingRoleIDs = Array.isArray(settings.request_visible_role_ids)
      ? [...settings.request_visible_role_ids]
      : [];
  });

  $effect(() => {
    if (activeTab !== 'sharing' || contactRolesLoaded) return;
    contactRolesLoaded = true;
    contactRolesApi
      .getAll()
      .then((roles) => {
        contactRoles = Array.isArray(roles) ? roles : (roles?.data ?? []);
      })
      .catch((err) => {
        console.error('Failed to load contact roles:', err);
        contactRoles = [];
      });
  });

  let sharingModeOptions = $derived([
    { value: 'disabled', label: t('workspaces.customers.sharingDisabled') || 'Disabled' },
    { value: 'requester_choice', label: t('workspaces.customers.sharingRequesterChoice') || 'Contacts choose per request' },
    { value: 'automatic', label: t('workspaces.customers.sharingAutomatic') || 'Always shared with the organisation' },
  ]);

  let sharingAudienceOptions = $derived([
    { value: 'organisation', label: t('workspaces.customers.sharingAudienceOrganisation') || 'Everyone in the organisation' },
    { value: 'roles', label: t('workspaces.customers.sharingAudienceRoles') || 'Only selected contact levels' },
  ]);

  function toggleSharingRole(roleID) {
    sharingSaved = false;
    sharingRoleIDs = sharingRoleIDs.includes(roleID)
      ? sharingRoleIDs.filter((id) => id !== roleID)
      : [...sharingRoleIDs, roleID];
  }

  async function saveSharing() {
    if (sharingSaving || !organisation?.id) return;
    sharingSaving = true;
    sharingSaved = false;
    try {
      await customerOrganisations.setRequestSharing(organisation.id, {
        request_sharing: sharingMode,
        request_sharing_audience: sharingAudience,
        visible_role_ids: sharingAudience === 'roles' ? sharingRoleIDs : [],
      });
      // Persist the saved values back into the organisation record so a revisit
      // does not re-hydrate the form from stale settings.
      onOrganisationUpdated(organisation.id, {
        ...(organisation.settings || {}),
        request_sharing: sharingMode,
        request_sharing_audience: sharingAudience,
        request_visible_role_ids: sharingAudience === 'roles' ? [...sharingRoleIDs] : [],
      });
      sharingSaved = true;
      successToast(t('workspaces.customers.sharingSaved') || 'Request sharing updated');
    } catch (err) {
      errorToast(err?.message || t('workspaces.customers.sharingSaveFailed') || 'Failed to update request sharing');
    } finally {
      sharingSaving = false;
    }
  }

  // Tickets tab state
  let orgTickets = $state([]);
  let orgTicketsLoading = $state(false);
  let orgTicketsLoaded = $state(false);
  let orgTicketsOrgID = null;

  $effect(() => {
    if (activeTab !== 'tickets' || !organisation?.id) return;
    const orgId = organisation.id;
    if (orgId === orgTicketsOrgID) return;
    orgTicketsOrgID = orgId;
    orgTickets = [];
    orgTicketsLoading = true;
    orgTicketsLoaded = false;
    customerOrganisations.getTickets(orgId)
      .then((result) => {
        if (orgTicketsOrgID !== orgId) return;
        const list = result?.data ?? result ?? [];
        orgTickets = Array.isArray(list) ? list : [];
      })
      .catch((err) => {
        if (orgTicketsOrgID !== orgId) return;
        console.error('Failed to load organisation tickets:', err);
        orgTickets = [];
      })
      .finally(() => {
        if (orgTicketsOrgID !== orgId) return;
        orgTicketsLoading = false;
        orgTicketsLoaded = true;
      });
  });

  let ticketColumns = $derived([
    { key: 'title', label: t('common.ticket') || 'Ticket', slot: 'ticket' },
    { key: 'creator_contact_name', label: t('workspaces.customers.contact') || 'Contact', slot: 'contact' },
    { key: 'status_name', label: t('common.status') || 'Status', slot: 'status' },
    { key: 'created_at', label: t('common.date') || 'Date', render: (item) => formatDateShort(item.created_at) },
  ]);

  function ticketHref(ticket) {
    if (!ticket?.workspace_id || !ticket?.id) return null;
    return itemUrl({ workspaceId: ticket.workspace_id, itemId: ticket.id });
  }
</script>

<!-- Header -->
<div class="flex items-center gap-4 mb-8">
  {#if organisation}
    <Avatar
      src={organisation.avatar_url}
      name={organisation.name}
      size="lg"
      variant="blue"
      rounded="md"
    />
  {/if}
  <div class="flex-1 min-w-0">
    <PageHeader
      marginClass="mb-0"
      title={organisation ? organisation.name : (t('workspaces.customers.unassignedCustomers'))}
      subtitle={t('workspaces.customers.customerCount', { count: filteredCount })}
    >
      {#snippet actions()}
        {#if canManage}
          <Button
            variant="primary"
            icon={Plus}
            onclick={onStartCreate}
            keyboardHint="A"
            hotkeyConfig={{ key: toHotkeyString('customers', 'add'), guard: () => !showCreateModal }}
          >
            {t('workspaces.customers.addCustomer')}
          </Button>
        {/if}
      {/snippet}
    </PageHeader>
  </div>
</div>

<!-- Tabs -->
<Tabs {tabs} bind:activeTab>
  {#if activeTab === 'contacts'}
    <!-- Customer Search -->
    <div class="mb-4">
      <div class="relative max-w-md">
        <Search class="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4" style="color: var(--ds-text-subtle);" />
        <Input
          type="text"
          bind:value={customerSearch}
          placeholder={t('workspaces.customers.searchCustomers')}
          class="pl-10"
          size="small"
        />
      </div>
    </div>

    <!-- Customer List -->
    {#if customers.length === 0}
      {@const subtext = customerSearch
        ? t('workspaces.customers.tryAdjustingSearch')
        : !organisation
          ? t('workspaces.customers.allCustomersAssigned')
          : t('workspaces.customers.dragCustomersHere')}
      <EmptyState
        icon={Users}
        title={t('workspaces.customers.noCustomersFound')}
        description={subtext}
      />
    {:else}
      <div class="divide-y" style="border-color: var(--ds-border);">
        {#each customers as customer (customer.id)}
          <div
            data-customer-id={customer.id}
            data-testid="portal-customer-row"
            class="p-4 hover:bg-opacity-50 transition-colors"
            style="background-color: transparent;"
          >
            <div class="flex items-start gap-3">
              <!-- Drag Handle -->
              <div data-drag-handle class="cursor-grab active:cursor-grabbing pt-1">
                <GripVertical class="w-5 h-5" style="color: var(--ds-text-subtle);" />
              </div>

              <div class="flex-1 min-w-0">
                <button
                  onclick={() => onOpenDetail(customer)}
                  class="truncate hover:underline text-left w-full"
                  style="color: var(--ds-text);"
                >
                  {customer.name}
                </button>
                {#if customer.deactivated_at}
                  <span
                    data-testid="customer-deactivated-badge"
                    class="inline-flex items-center mt-1 px-2 py-0.5 rounded-full text-xs font-medium"
                    style="background-color: var(--ds-surface-raised); color: var(--ds-text-subtle); border: 1px solid var(--ds-border);"
                  >
                    {t('workspaces.customers.deactivated')}
                  </span>
                {/if}
                <div class="flex items-center gap-2 mt-1">
                  <Mail class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
                  <span class="text-sm truncate" style="color: var(--ds-text-subtle);">
                    {customer.email}
                  </span>
                </div>
                {#if customer.user_name}
                  <div class="flex items-center gap-2 mt-1">
                    <Users class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
                    <span class="text-sm truncate" style="color: var(--ds-text-subtle);">
                      {t('workspaces.customers.linked')}: {customer.user_name}
                    </span>
                  </div>
                {/if}
              </div>

              <!-- Action Menu -->
              <DropdownMenu
                triggerText=""
                triggerIcon={MoreHorizontal}
                triggerClass="p-2 rounded hover-bg transition-colors"
                items={buildCustomerActions(customer)}
                align="right"
              />
            </div>
          </div>
        {/each}
      </div>

      <!-- Load More Button -->
      {#if hasMoreCustomers}
        <div class="p-4 border-t text-center" style="border-color: var(--ds-border);">
          <Button variant="default" onclick={onLoadMore}>
            {t('workspaces.customers.loadMore', { count: filteredCount - displayLimit })}
          </Button>
        </div>
      {/if}
    {/if}
  {:else if activeTab === 'tickets'}
    {#if orgTicketsLoading}
      <div class="flex items-center justify-center h-48">
        <Spinner />
      </div>
    {:else}
      <DataTable
        columns={ticketColumns}
        data={orgTickets}
        emptyMessage={t('common.noTickets') || 'No tickets yet'}
        emptyDescription={t('workspaces.customers.noOrgTickets') || "Tickets created by this organisation's contacts will appear here."}
        emptyIcon={Ticket}
      >
        {#snippet ticket(item)}
          {@const href = ticketHref(item)}
          {#if href}
            <a href={href} class="block no-underline" style="color: inherit;">
              <div class="font-medium truncate text-sm" style="color: var(--ds-text);">
                {item.title}
              </div>
              <div class="text-xs">
                <ItemKey item={item} clickable={false} />
              </div>
            </a>
          {:else}
            <div class="font-medium truncate text-sm" style="color: var(--ds-text);">
              {item.title}
            </div>
            <div class="text-xs">
              <ItemKey item={item} />
            </div>
          {/if}
        {/snippet}

        {#snippet contact(item)}
          <div class="text-sm" style="color: var(--ds-text);">
            {item.creator_contact_name}
          </div>
          {#if item.creator_contact_email}
            <div class="text-xs" style="color: var(--ds-text-subtle);">
              {item.creator_contact_email}
            </div>
          {/if}
        {/snippet}

        {#snippet status(item)}
          {#if item.status_name}
            <span
              class="inline-flex items-center px-1.5 py-0.5 rounded-full text-xs font-medium"
              style="background-color: {item.status_color}20; color: {item.status_color};"
            >
              {item.status_name}
            </span>
          {/if}
        {/snippet}
      </DataTable>
    {/if}
  {:else if activeTab === 'sharing'}
    <div class="max-w-xl space-y-5" data-testid="org-request-sharing">
      <div class="space-y-2">
        <Label for="org-sharing-mode">{t('workspaces.customers.sharingMode') || 'Request sharing'}</Label>
        <Select id="org-sharing-mode" bind:value={sharingMode} options={sharingModeOptions} />
        <p class="text-xs" style="color: var(--ds-text-subtle);">
          {t('workspaces.customers.sharingModeHelp') ||
            'Controls whether requests raised by this organisation\u2019s contacts are shared with the organisation.'}
        </p>
      </div>

      {#if sharingMode !== 'disabled'}
        <div class="space-y-2">
          <Label for="org-sharing-audience">{t('workspaces.customers.sharingAudience') || 'Who can see shared requests'}</Label>
          <Select id="org-sharing-audience" bind:value={sharingAudience} options={sharingAudienceOptions} />
        </div>

        {#if sharingAudience === 'roles'}
          <div class="space-y-2">
            <Label>{t('workspaces.customers.sharingRoles') || 'Allowed contact levels'}</Label>
            {#if contactRoles.length === 0}
              <p class="text-xs" style="color: var(--ds-text-subtle);">
                {t('workspaces.customers.sharingNoRoles') || 'No contact levels defined yet.'}
              </p>
            {:else}
              <div class="space-y-2">
                {#each contactRoles as role (role.id)}
                  <Checkbox
                    checked={sharingRoleIDs.includes(role.id)}
                    onchange={() => toggleSharingRole(role.id)}
                    label={role.name}
                    size="small"
                    dataTestid={`org-sharing-role-${role.id}`}
                  />
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      {/if}

      <div class="flex items-center gap-3">
        <Button
          variant="primary"
          size="small"
          onclick={saveSharing}
          disabled={sharingSaving}
          dataTestid="org-sharing-save"
        >
          {sharingSaving ? (t('common.saving') || 'Saving…') : (t('common.save') || 'Save')}
        </Button>
        {#if sharingSaved}
          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('common.saved') || 'Saved'}</span>
        {/if}
      </div>
    </div>
  {/if}
</Tabs>
