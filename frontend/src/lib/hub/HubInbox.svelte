<script>
  import { onMount } from 'svelte';
  import { ExternalLink, Filter, ChevronLeft, ChevronRight, Inbox as InboxIcon } from 'lucide-svelte';
  import { hubStore, gradients } from '../stores/hub.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import Spinner from '../components/Spinner.svelte';

  // Format date for display
  function formatDate(dateStr) {
    const date = new Date(dateStr);
    return date.toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    });
  }

  function formatTime(dateStr) {
    const date = new Date(dateStr);
    return date.toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit'
    });
  }

  // Navigate to item detail
  function openItem(item) {
    if (item.workspace_key && item.workspace_item_number) {
      window.location.href = `/${item.workspace_key}-${item.workspace_item_number}`;
    }
  }

  // Open portal in new tab
  function openPortal(e, portalSlug) {
    e.stopPropagation();
    window.open(`/portal/${portalSlug}`, '_blank');
  }
</script>

<div>
  <!-- Inbox Header -->
  <div class="flex items-center justify-between mb-4">
    <div class="flex items-center gap-2">
      <div class="w-8 h-8 rounded-lg flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
        <InboxIcon class="w-4 h-4" style="color: var(--ds-text-subtle);" />
      </div>
      <div>
        <h1 class="text-lg font-semibold" style="color: var(--ds-text);">
          {t('hub.inbox', 'Inbox')}
        </h1>
        <p class="text-xs" style="color: var(--ds-text-subtle);">
          {t('hub.inboxDescription', 'Requests from all portals')}
        </p>
      </div>
    </div>

    <!-- Filters -->
    <div class="flex items-center gap-2">
      <!-- Portal Filter -->
      <select
        value={hubStore.inboxPortalFilter}
        onchange={(e) => hubStore.setInboxFilters(e.target.value, hubStore.inboxStatusFilter)}
        class="px-2 py-1.5 rounded border text-xs"
        style="background-color: var(--ds-surface-card); border-color: var(--ds-border); color: var(--ds-text);"
      >
        <option value="">{t('hub.allPortals', 'All Portals')}</option>
        {#each hubStore.portals as portal}
          <option value={portal.id}>{portal.name}</option>
        {/each}
      </select>

      <!-- Status Filter -->
      <select
        value={hubStore.inboxStatusFilter}
        onchange={(e) => hubStore.setInboxFilters(hubStore.inboxPortalFilter, e.target.value)}
        class="px-2 py-1.5 rounded border text-xs"
        style="background-color: var(--ds-surface-card); border-color: var(--ds-border); color: var(--ds-text);"
      >
        <option value="">{t('hub.allStatuses', 'All Statuses')}</option>
        <option value="Open">{t('status.open', 'Open')}</option>
        <option value="In Progress">{t('status.inProgress', 'In Progress')}</option>
        <option value="Closed">{t('status.closed', 'Closed')}</option>
      </select>
    </div>
  </div>

  <!-- Inbox Content -->
  {#if hubStore.inboxLoading}
    <div class="flex items-center justify-center py-12">
      <Spinner size="md" />
    </div>
  {:else if hubStore.inboxItems.length === 0}
    <div class="text-center py-12">
      <div class="w-12 h-12 mx-auto mb-3 rounded-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
        <InboxIcon class="w-6 h-6" style="color: var(--ds-text-subtle);" />
      </div>
      <h3 class="text-base font-semibold mb-1" style="color: var(--ds-text);">
        {t('hub.noRequests', 'No requests yet')}
      </h3>
      <p class="text-xs" style="color: var(--ds-text-subtle);">
        {t('hub.noRequestsDescription', 'Requests submitted through your portals will appear here')}
      </p>
    </div>
  {:else}
    <!-- Items Table -->
    <div class="rounded-lg border overflow-hidden" style="border-color: var(--ds-border);">
      <table class="w-full text-sm">
        <thead>
          <tr style="background-color: var(--ds-surface-raised);">
            <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">
              {t('hub.request', 'Request')}
            </th>
            <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">
              {t('hub.portal', 'Portal')}
            </th>
            <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">
              {t('hub.submitter', 'Submitter')}
            </th>
            <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">
              {t('hub.status', 'Status')}
            </th>
            <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--ds-text-subtle);">
              {t('hub.date', 'Date')}
            </th>
          </tr>
        </thead>
        <tbody class="divide-y" style="background-color: var(--ds-surface-card); --tw-divide-opacity: 1; border-color: var(--ds-border);">
          {#each hubStore.inboxItems as item (item.id)}
            <tr
              class="cursor-pointer transition-colors hover:bg-black/5"
              onclick={() => openItem(item)}
            >
              <td class="px-3 py-2.5">
                <div class="flex items-center gap-2">
                  <div class="flex-1 min-w-0">
                    <div class="font-medium truncate text-sm" style="color: var(--ds-text);">
                      {item.title}
                    </div>
                    <div class="text-xs" style="color: var(--ds-text-subtle);">
                      {item.workspace_key}-{item.workspace_item_number}
                    </div>
                  </div>
                </div>
              </td>
              <td class="px-3 py-2.5">
                <button
                  onclick={(e) => openPortal(e, item.portal_slug)}
                  class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs font-medium transition-colors hover:opacity-80"
                  style="background-color: var(--ds-background-neutral); color: var(--ds-text);"
                >
                  {item.portal_name}
                  <ExternalLink class="w-3 h-3" />
                </button>
              </td>
              <td class="px-3 py-2.5">
                {#if item.submitter_name || item.submitter_email}
                  <div class="text-sm" style="color: var(--ds-text);">
                    {item.submitter_name || 'Unknown'}
                  </div>
                  {#if item.submitter_email}
                    <div class="text-xs" style="color: var(--ds-text-subtle);">
                      {item.submitter_email}
                    </div>
                  {/if}
                {:else}
                  <span class="text-xs" style="color: var(--ds-text-subtle);">
                    {t('hub.anonymous', 'Anonymous')}
                  </span>
                {/if}
              </td>
              <td class="px-3 py-2.5">
                <span
                  class="inline-flex items-center px-1.5 py-0.5 rounded-full text-xs font-medium"
                  style="background-color: {item.status_color}20; color: {item.status_color};"
                >
                  {item.status_name}
                </span>
              </td>
              <td class="px-3 py-2.5">
                <div class="text-sm" style="color: var(--ds-text);">
                  {formatDate(item.created_at)}
                </div>
                <div class="text-xs" style="color: var(--ds-text-subtle);">
                  {formatTime(item.created_at)}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <!-- Pagination -->
    {#if hubStore.inboxTotalPages > 1}
      <div class="flex items-center justify-between mt-3 px-1">
        <div class="text-xs" style="color: var(--ds-text-subtle);">
          {t('hub.showingResults', 'Showing')} {((hubStore.inboxPage - 1) * hubStore.inboxPerPage) + 1} - {Math.min(hubStore.inboxPage * hubStore.inboxPerPage, hubStore.inboxTotal)} {t('hub.of', 'of')} {hubStore.inboxTotal}
        </div>
        <div class="flex items-center gap-1">
          <button
            onclick={() => hubStore.setInboxPage(hubStore.inboxPage - 1)}
            disabled={hubStore.inboxPage <= 1}
            class="p-1.5 rounded border transition-colors disabled:opacity-30"
            style="border-color: var(--ds-border); color: var(--ds-text);"
          >
            <ChevronLeft class="w-3.5 h-3.5" />
          </button>
          <span class="text-xs px-2" style="color: var(--ds-text);">
            {hubStore.inboxPage} / {hubStore.inboxTotalPages}
          </span>
          <button
            onclick={() => hubStore.setInboxPage(hubStore.inboxPage + 1)}
            disabled={hubStore.inboxPage >= hubStore.inboxTotalPages}
            class="p-1.5 rounded border transition-colors disabled:opacity-30"
            style="border-color: var(--ds-border); color: var(--ds-text);"
          >
            <ChevronRight class="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    {/if}
  {/if}
</div>
