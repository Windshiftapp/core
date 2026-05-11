<script>
  import { ExternalLink, Inbox as InboxIcon } from '@lucide/svelte';
  import { hubStore } from '../stores/hub.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateShort, formatDateWithOptions } from '../utils/dateFormatter.js';
  import { portalUrl, portalRequestUrl } from '../utils/urls.js';
  import Spinner from '../components/Spinner.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Select from '../components/Select.svelte';
  import DataTable from '../components/DataTable.svelte';

  function formatDate(dateStr) {
    return formatDateShort(dateStr);
  }

  function formatTime(dateStr) {
    return formatDateWithOptions(dateStr, { hour: '2-digit', minute: '2-digit' });
  }

  let currentPage = $state(hubStore.inboxPage);
  $effect(() => { currentPage = hubStore.inboxPage; });

  const columns = [
    { key: 'title', label: t('hub.request', 'Request'), slot: 'request' },
    { key: 'portal', label: t('hub.portal', 'Portal'), slot: 'portal' },
    { key: 'submitter', label: t('hub.submitter', 'Submitter'), slot: 'submitter' },
    { key: 'status', label: t('hub.status', 'Status'), slot: 'status' },
    { key: 'created_at', label: t('hub.date', 'Date'), slot: 'date' },
  ];
</script>

<div>
  <!-- Inbox Header -->
  <PageHeader title={t('hub.inbox', 'Inbox')} subtitle={t('hub.inboxDescription', 'Requests from all portals')}>
    {#snippet actions()}
      <div class="flex items-center gap-2">
        <Select
          value={hubStore.inboxPortalFilter}
          onchange={(v) => hubStore.setInboxFilters(v, hubStore.inboxStatusFilter)}
          size="small"
          placeholder={t('hub.allPortals', 'All Portals')}
          options={[{ value: '', label: t('hub.allPortals', 'All Portals') }, ...hubStore.portals.map(p => ({ value: p.id, label: p.name }))]}
        />

        {#if hubStore.inboxStatusFacets.length > 0}
          <Select
            value={hubStore.inboxStatusFilter}
            onchange={(v) => hubStore.setInboxFilters(hubStore.inboxPortalFilter, v)}
            size="small"
            placeholder={t('hub.allStatuses', 'All Statuses')}
            options={[
              { value: '', label: t('hub.allStatuses', 'All Statuses') },
              ...hubStore.inboxStatusFacets.map(f => ({ value: f.name, label: f.name })),
            ]}
          />
        {/if}
      </div>
    {/snippet}
  </PageHeader>

  {#if hubStore.inboxLoading}
    <div class="flex items-center justify-center py-12">
      <Spinner size="md" />
    </div>
  {:else if hubStore.inboxItems.length === 0}
    <EmptyState
      icon={InboxIcon}
      title={t('hub.noRequests', 'No requests yet')}
      description={t('hub.noRequestsDescription', 'Requests submitted through your portals will appear here')}
    />
  {:else}
    <DataTable
      {columns}
      data={hubStore.inboxItems}
      keyField="id"
      pagination
      pageSize={hubStore.inboxPerPage}
      totalItems={hubStore.inboxTotal}
      bind:currentPage
      onPageChange={(p) => hubStore.setInboxPage(p)}
    >
      {#snippet request(item)}
        <a
          href={portalRequestUrl(item.portal_slug, item.id)}
          class="block no-underline"
          style="color: inherit;"
        >
          <div class="font-medium text-sm" style="color: var(--ds-text);">{item.title}</div>
          <div class="text-xs" style="color: var(--ds-text-subtle);">
            {item.workspace_key}-{item.workspace_item_number}
          </div>
        </a>
      {/snippet}

      {#snippet portal(item)}
        <a
          href={portalUrl(item.portal_slug)}
          target="_blank"
          rel="noopener"
          class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs font-medium transition-colors hover:opacity-80 no-underline"
          style="background-color: var(--ds-background-neutral); color: var(--ds-text);"
        >
          {item.portal_name}
          <ExternalLink class="w-3 h-3" />
        </a>
      {/snippet}

      {#snippet submitter(item)}
        {#if item.submitter_name || item.submitter_email}
          <div class="text-sm" style="color: var(--ds-text);">{item.submitter_name || 'Unknown'}</div>
          {#if item.submitter_email}
            <div class="text-xs" style="color: var(--ds-text-subtle);">{item.submitter_email}</div>
          {/if}
        {:else}
          <span class="text-xs" style="color: var(--ds-text-subtle);">{t('hub.anonymous', 'Anonymous')}</span>
        {/if}
      {/snippet}

      {#snippet status(item)}
        <span
          class="inline-flex items-center px-1.5 py-0.5 rounded-full text-xs font-medium"
          style="background-color: {item.status_color}20; color: {item.status_color};"
        >
          {item.status_name}
        </span>
      {/snippet}

      {#snippet date(item)}
        <div class="text-sm" style="color: var(--ds-text);">{formatDate(item.created_at)}</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">{formatTime(item.created_at)}</div>
      {/snippet}
    </DataTable>
  {/if}
</div>
