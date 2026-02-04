<script>
  import { untrack } from 'svelte';
  import { useDebounce } from 'runed';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api.js';
  import Modal from './Modal.svelte';
  import Spinner from '../components/Spinner.svelte';
  import Button from '../components/Button.svelte';
  import { AlertTriangle, ChevronLeft, ChevronRight, Mail, MessageSquare, FileText } from 'lucide-svelte';
  import SearchInput from '../components/SearchInput.svelte';

  let { isOpen = false, channel = null, onClose = () => {} } = $props();

  let loading = $state(false);
  let error = $state(null);
  let data = $state(null);
  let page = $state(1);
  let search = $state('');
  const pageSize = 50;

  const debouncedSearch = useDebounce(() => {
    page = 1;
    loadLog();
  }, 300);

  $effect(() => {
    if (isOpen && channel) {
      search = '';
      page = 1;
      untrack(() => loadLog());
    }
  });

  async function loadLog() {
    try {
      loading = true;
      error = null;
      data = await api.channels.getEmailLog(channel.id, page, pageSize, search);
    } catch (err) {
      console.error('Failed to load email log:', err);
      error = err.message || 'Failed to load email log';
      data = null;
    } finally {
      loading = false;
    }
  }

  function onSearchInput(e) {
    search = e.target.value;
    debouncedSearch();
  }

  function prevPage() {
    if (page > 1) {
      page--;
      loadLog();
    }
  }

  function nextPage() {
    if (data && page * pageSize < data.total) {
      page++;
      loadLog();
    }
  }

  let totalPages = $derived(data ? Math.max(1, Math.ceil(data.total / pageSize)) : 1);

  function formatTime(dateStr) {
    if (!dateStr) return '';
    const date = new Date(dateStr);
    const now = new Date();
    const diff = now - date;
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    if (days < 7) return `${days}d ago`;
    return date.toLocaleDateString();
  }

  function getItemKey(msg) {
    if (msg.workspace_key) return `${msg.workspace_key}-${msg.workspace_item_number}`;
    if (msg.item_id) return `#${msg.item_id}`;
    return null;
  }

  function getWorkspaceId() {
    try {
      const config = JSON.parse(channel?.config || '{}');
      return config.email_workspace_id || null;
    } catch { return null; }
  }

  function getItemHref(msg) {
    const wsId = getWorkspaceId();
    if (wsId && msg.item_id) return `/workspaces/${wsId}/items/${msg.item_id}`;
    return null;
  }

  function getResultText(msg) {
    const key = getItemKey(msg);
    if (msg.comment_id && key) {
      return t('channel.emailLog.commentOn', 'Comment on {key}').replace('{key}', key);
    }
    if (key) {
      return t('channel.emailLog.newItem', 'New item {key}').replace('{key}', key);
    }
    return '-';
  }
</script>

<Modal
  {isOpen}
  onclose={onClose}
  maxWidth="max-w-3xl"
>
  <!-- Header -->
  <div class="px-6 py-4 border-b flex items-center gap-3" style="border-color: var(--ds-border);">
    <FileText class="w-5 h-5" style="color: var(--ds-text-subtle);" />
    <div>
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
        {t('channel.processingLog', 'Processing Log')}
      </h3>
      {#if channel}
        <p class="text-sm" style="color: var(--ds-text-subtle);">{channel.name}</p>
      {/if}
    </div>
  </div>

  <!-- Content -->
  <div class="p-6">
    {#if loading && !data}
      <div class="flex items-center justify-center py-12">
        <Spinner />
      </div>
    {:else if error}
      <div class="text-center py-12">
        <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
        <Button onclick={loadLog} variant="default" size="small" class="mt-3">
          {t('common.retry', 'Retry')}
        </Button>
      </div>
    {:else if data}
      <!-- Sync Status Banner -->
      <div class="rounded-lg border p-4 mb-6" style="border-color: var(--ds-border); background: var(--ds-surface-sunken, var(--ds-surface));">
        <div class="text-sm font-medium mb-2" style="color: var(--ds-text);">
          {t('channel.emailLog.syncStatus', 'Sync Status')}
        </div>
        <div class="flex flex-wrap gap-x-6 gap-y-1 text-sm">
          <div>
            <span style="color: var(--ds-text-subtle);">{t('channel.emailLog.lastChecked', 'Last checked')}:</span>
            <span style="color: var(--ds-text);">
              {data.state.last_checked_at ? formatTime(data.state.last_checked_at) : t('channel.emailLog.never', 'Never')}
            </span>
          </div>
          <div class="flex items-center gap-1.5">
            <span style="color: var(--ds-text-subtle);">{t('channel.emailLog.errors', 'Errors')}:</span>
            {#if data.state.error_count > 0}
              <span class="flex items-center gap-1" style="color: var(--ds-text-danger);">
                <AlertTriangle class="w-3.5 h-3.5" />
                {data.state.error_count}
              </span>
            {:else}
              <span style="color: var(--ds-text-success, var(--ds-text));">{t('channel.emailLog.noErrors', 'No errors')}</span>
            {/if}
          </div>
        </div>
        {#if data.state.last_error}
          <div class="mt-2 text-xs rounded p-2" style="background: var(--ds-surface-danger, rgba(239, 68, 68, 0.1)); color: var(--ds-text-danger);">
            {data.state.last_error}
          </div>
        {/if}
      </div>

      <!-- Search -->
      <SearchInput value={search} on_input={onSearchInput} className="mb-4" />

      <!-- Messages Table -->
      {#if data.messages.length === 0}
        <div class="text-center py-12">
          <Mail class="w-8 h-8 mx-auto mb-3" style="color: var(--ds-text-subtlest);" />
          <p class="text-sm" style="color: var(--ds-text-subtle);">
            {#if search}
              {t('channel.emailLog.noResults', 'No results found')}
            {:else}
              {t('channel.emailLog.noEmails', 'No emails processed yet')}
            {/if}
          </p>
        </div>
      {:else}
        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b" style="border-color: var(--ds-border);">
                <th class="text-left py-2 px-3 font-medium" style="color: var(--ds-text-subtle);">{t('channel.emailLog.from', 'From')}</th>
                <th class="text-left py-2 px-3 font-medium" style="color: var(--ds-text-subtle);">{t('channel.emailLog.subject', 'Subject')}</th>
                <th class="text-left py-2 px-3 font-medium" style="color: var(--ds-text-subtle);">{t('channel.emailLog.result', 'Result')}</th>
                <th class="text-left py-2 px-3 font-medium" style="color: var(--ds-text-subtle);">{t('channel.emailLog.processedAt', 'Processed')}</th>
              </tr>
            </thead>
            <tbody>
              {#each data.messages as msg (msg.id)}
                <tr class="border-b hover:bg-opacity-50" style="border-color: var(--ds-border);" onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, rgba(0,0,0,0.03))'} onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}>
                  <td class="py-2.5 px-3" style="color: var(--ds-text);">
                    <div class="font-medium truncate max-w-48">{msg.from_name || msg.from_email}</div>
                    {#if msg.from_name}
                      <div class="text-xs truncate max-w-48" style="color: var(--ds-text-subtle);">{msg.from_email}</div>
                    {/if}
                  </td>
                  <td class="py-2.5 px-3 truncate max-w-56" style="color: var(--ds-text);">{msg.subject}</td>
                  <td class="py-2.5 px-3">
                    {#if msg.item_id}
                      {@const href = getItemHref(msg)}
                      <svelte:element this={href ? 'a' : 'span'} href={href} class="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full {href ? 'hover:opacity-80' : ''}" style="background: var(--ds-surface-selected, rgba(59, 130, 246, 0.1)); color: var(--ds-text-accent, var(--ds-text)); {href ? 'text-decoration: none;' : ''}">
                        {#if msg.comment_id}
                          <MessageSquare class="w-3 h-3" />
                        {:else}
                          <Mail class="w-3 h-3" />
                        {/if}
                        {getResultText(msg)}
                      </svelte:element>
                    {:else}
                      <span style="color: var(--ds-text-subtlest);">-</span>
                    {/if}
                  </td>
                  <td class="py-2.5 px-3 whitespace-nowrap" style="color: var(--ds-text-subtle);">{formatTime(msg.processed_at)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>

        <!-- Pagination -->
        {#if totalPages > 1}
          <div class="flex items-center justify-between mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
            <Button
              onclick={prevPage}
              variant="ghost"
              size="small"
              icon={ChevronLeft}
              disabled={page <= 1 || loading}
            >
              {t('channel.emailLog.previous', 'Previous')}
            </Button>
            <span class="text-sm" style="color: var(--ds-text-subtle);">
              {t('channel.emailLog.page', 'Page {page} of {total}').replace('{page}', page).replace('{total}', totalPages)}
            </span>
            <Button
              onclick={nextPage}
              variant="ghost"
              size="small"
              disabled={page >= totalPages || loading}
            >
              {t('channel.emailLog.next', 'Next')}
              <ChevronRight class="w-4 h-4 ml-1" />
            </Button>
          </div>
        {/if}
      {/if}
    {/if}
  </div>
</Modal>
