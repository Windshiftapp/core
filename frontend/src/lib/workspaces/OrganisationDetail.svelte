<script>
  import { IconUsers as Users, IconMail as Mail, IconSearch as Search, IconGripVertical as GripVertical, IconPlus as Plus, IconEdit as Edit2, IconTrash as Trash2, IconDots as MoreHorizontal, IconFile as FileIcon, IconTicket as Ticket, IconFileText as FileText, IconNote as StickyNote, IconExternalLink as ExternalLink } from '@tabler/icons-svelte-runes';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';
  import Button from '../components/Button.svelte';
  import Avatar from '../components/Avatar.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import Card from '../components/Card.svelte';
  import Tabs from '../components/Tabs.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import DataTable from '../components/DataTable.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api.js';
  import { logbook } from '../api/logbook.js';
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
  } = $props();

  let activeTab = $state('contacts');

  const tabs = [
    { id: 'contacts', label: t('workspaces.customers.contacts') || 'Contacts', icon: Users },
    { id: 'files', label: t('common.files') || 'Files', icon: FileIcon },
    { id: 'tickets', label: t('common.tickets') || 'Tickets', icon: Ticket },
  ];

  // Files tab state
  let orgDocuments = $state([]);
  let orgDocsLoading = $state(false);
  let orgDocsLoaded = $state(false);

  // Tickets tab state
  let orgTickets = $state([]);
  let orgTicketsLoading = $state(false);
  let orgTicketsLoaded = $state(false);

  $effect(() => {
    if (activeTab === 'files' && organisation?.id) {
      const orgId = organisation.id;
      orgDocsLoading = true;
      orgDocsLoaded = false;
      logbook.listDocumentsByOrganisation(orgId)
        .then((result) => {
          orgDocuments = result?.data ?? result ?? [];
          if (!Array.isArray(orgDocuments)) orgDocuments = [];
        })
        .catch((err) => {
          console.error('Failed to load organisation documents:', err);
          orgDocuments = [];
        })
        .finally(() => {
          orgDocsLoading = false;
          orgDocsLoaded = true;
        });
    }
  });

  $effect(() => {
    if (activeTab === 'tickets' && organisation?.id && !orgTicketsLoaded) {
      const orgId = organisation.id;
      orgTicketsLoading = true;
      api.customerOrganisations.getTickets(orgId)
        .then((result) => {
          orgTickets = result?.data ?? result ?? [];
          if (!Array.isArray(orgTickets)) orgTickets = [];
        })
        .catch((err) => {
          console.error('Failed to load organisation tickets:', err);
          orgTickets = [];
        })
        .finally(() => {
          orgTicketsLoading = false;
          orgTicketsLoaded = true;
        });
    }
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

  function getSourceIcon(sourceType) {
    switch (sourceType) {
      case 'upload': return FileText;
      case 'note': return StickyNote;
      case 'email': return Mail;
      default: return FileText;
    }
  }

  function getStatusColor(status) {
    switch (status) {
      case 'pending': return 'grey';
      case 'processing': return 'blue';
      case 'ready': return 'green';
      case 'error': return 'red';
      default: return 'grey';
    }
  }
</script>

<!-- Header -->
<div class="flex items-center gap-4">
  {#if organisation}
    <Avatar
      src={organisation.avatar_url}
      name={organisation.name}
      size="lg"
      variant="blue"
      rounded="md"
    />
  {/if}
  <div class="flex-1">
    <PageHeader
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
        <input
          type="text"
          bind:value={customerSearch}
          placeholder={t('workspaces.customers.searchCustomers')}
          class="w-full pl-10 pr-4 py-2 rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
          style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
        />
      </div>
    </div>

    <!-- Customer List -->
    {#if customers.length === 0}
      <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
        <Users class="w-12 h-12 mx-auto mb-3 opacity-50" />
        <p>{t('workspaces.customers.noCustomersFound')}</p>
        {#if customerSearch}
          <p class="text-sm mt-1">{t('workspaces.customers.tryAdjustingSearch')}</p>
        {:else if !organisation}
          <p class="text-sm mt-1">{t('workspaces.customers.allCustomersAssigned')}</p>
        {:else}
          <p class="text-sm mt-1">{t('workspaces.customers.dragCustomersHere')}</p>
        {/if}
      </div>
    {:else}
      <div class="divide-y" style="border-color: var(--ds-border);">
        {#each customers as customer (customer.id)}
          <div
            data-customer-id={customer.id}
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
  {:else if activeTab === 'files'}
    {#if orgDocsLoading}
      <div class="flex items-center justify-center h-48">
        <Spinner />
      </div>
    {:else if orgDocuments.length === 0}
      <div class="p-8 text-center" style="color: var(--ds-text-subtle);">
        <FileIcon class="w-12 h-12 mx-auto mb-3 opacity-50" />
        <p class="font-medium">{t('common.noFiles') || 'No files yet'}</p>
        <p class="text-sm mt-1">{t('workspaces.customers.noOrgFiles') || 'Documents associated with this organisation will appear here.'}</p>
      </div>
    {:else}
      <div class="grid gap-4" style="grid-template-columns: repeat(auto-fill, minmax(150px, 200px));">
        {#each orgDocuments as doc (doc.id)}
          {@const SourceIcon = getSourceIcon(doc.source_type)}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            onclick={() => window.open(logbook.getDocumentFileUrl(doc.id), '_blank')}
            class="group text-left rounded-xl border transition-all duration-200 hover:shadow-md cursor-pointer overflow-hidden flex flex-col"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
            onmouseenter={(e) => e.currentTarget.style.borderColor = 'var(--ds-border-focused)'}
            onmouseleave={(e) => e.currentTarget.style.borderColor = 'var(--ds-border)'}
          >
            <div class="relative aspect-[210/297] w-full overflow-hidden" style="background-color: var(--ds-surface);">
              {#if doc.status === 'pending' || doc.status === 'processing'}
                <div class="w-full h-full flex items-center justify-center">
                  <Spinner />
                </div>
              {:else if doc.has_thumbnail}
                <img
                  src={logbook.getDocumentThumbnailUrl(doc.id)}
                  alt=""
                  class="w-full h-full object-contain"
                  loading="lazy"
                />
              {:else}
                <div class="w-full h-full flex items-center justify-center">
                  <SourceIcon class="w-10 h-10" style="color: var(--ds-icon-subtle);" />
                </div>
              {/if}

              <!-- Open in new tab button -->
              <div class="absolute top-2 right-2 flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                <button
                  onclick={(e) => { e.stopPropagation(); window.open(logbook.getDocumentFileUrl(doc.id), '_blank'); }}
                  class="p-1.5 rounded-lg shadow-sm border transition-colors hover:bg-opacity-90"
                  style="background-color: var(--ds-surface-overlay); border-color: var(--ds-border);"
                >
                  <ExternalLink class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
                </button>
              </div>
            </div>

            <div class="p-3 flex-1 flex flex-col justify-between">
              <div class="mb-2">
                <h3 class="text-sm truncate" style="color: var(--ds-text);">
                  {doc.title || 'Untitled'}
                </h3>
                <p class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">
                  {doc.bucket_name || ''}
                  {#if doc.author}
                    &middot; {doc.author}
                  {/if}
                </p>
              </div>

              <div class="flex items-center justify-between">
                <Lozenge color={getStatusColor(doc.status)} text={doc.status} />
                <span class="text-xs" style="color: var(--ds-text-subtlest);">
                  {formatDateShort(doc.created_at)}
                </span>
              </div>
            </div>
          </div>
        {/each}
      </div>
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
              <div class="text-xs" style="color: var(--ds-text-subtle);">
                {item.workspace_key}-{item.workspace_item_number}
              </div>
            </a>
          {:else}
            <div class="font-medium truncate text-sm" style="color: var(--ds-text);">
              {item.title}
            </div>
            <div class="text-xs" style="color: var(--ds-text-subtle);">
              {item.workspace_key}-{item.workspace_item_number}
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
  {/if}
</Tabs>
