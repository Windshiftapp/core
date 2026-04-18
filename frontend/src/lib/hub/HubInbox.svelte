<script>
  import { ExternalLink, Filter, ChevronLeft, ChevronRight, Inbox as InboxIcon } from 'lucide-svelte';
  import { hubStore } from '../stores/hub.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateShort, formatDateWithOptions } from '../utils/dateFormatter.js';
  import { portalUrl, portalRequestUrl } from '../utils/urls.js';
  import Spinner from '../components/Spinner.svelte';
  import PageHeader from '../layout/PageHeader.svelte';
  import Select from '../components/Select.svelte';

  // Format date for display
  function formatDate(dateStr) {
    return formatDateShort(dateStr);
  }

  function formatTime(dateStr) {
    return formatDateWithOptions(dateStr, {
      hour: '2-digit',
      minute: '2-digit'
    });
  }
</script>

<div>
  <!-- Inbox Header -->
  <PageHeader title={t('hub.inbox', 'Inbox')} subtitle={t('hub.inboxDescription', 'Requests from all portals')}>
    {#snippet actions()}
      <div class="flex items-center gap-2">
        <!-- Portal Filter -->
        <Select
          value={hubStore.inboxPortalFilter}
          onchange={(v) => hubStore.setInboxFilters(v, hubStore.inboxStatusFilter)}
          size="small"
          placeholder={t('hub.allPortals', 'All Portals')}
          options={[{ value: '', label: t('hub.allPortals', 'All Portals') }, ...hubStore.portals.map(p => ({ value: p.id, label: p.name }))]}
        />

        <!-- Status Filter — options are derived from the inbox response
             so custom or renamed statuses always stay in sync. -->
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
            <th class="px-3 py-2 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text);">
              {t('hub.request', 'Request')}
            </th>
            <th class="px-3 py-2 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text);">
              {t('hub.portal', 'Portal')}
            </th>
            <th class="px-3 py-2 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text);">
              {t('hub.submitter', 'Submitter')}
            </th>
            <th class="px-3 py-2 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text);">
              {t('hub.status', 'Status')}
            </th>
            <th class="px-3 py-2 text-left text-xs font-semibold tracking-wide" style="color: var(--ds-text);">
              {t('hub.date', 'Date')}
            </th>
          </tr>
        </thead>
        <tbody class="divide-y" style="background-color: var(--ds-surface-card); --tw-divide-opacity: 1; border-color: var(--ds-border);">
          {#each hubStore.inboxItems as item (item.id)}
            <tr class="inbox-row transition-colors hover:bg-black/5">
              <td class="p-0">
                <a
                  href={portalRequestUrl(item.portal_slug, item.id)}
                  class="block px-3 py-2.5 no-underline"
                  style="color: inherit;"
                >
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
                </a>
              </td>
              <td class="px-3 py-2.5">
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
