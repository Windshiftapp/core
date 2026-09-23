<script>
  import { untrack } from 'svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { useDebounce } from 'runed';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api.js';
  import { formatDateSimple } from '../utils/dateFormatter.js';
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import Button from '../components/Button.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import DataTable from '../components/DataTable.svelte';
  import SelectField from '../components/SelectField.svelte';
  import { AlertTriangle, ChevronLeft, ChevronRight, Mail, MessageSquare, FileText } from '@lucide/svelte';
  import SearchInput from '../components/SearchInput.svelte';

  let { isOpen = false, channel = null, onClose = () => {} } = $props();

  let loading = $state(false);
  let error = $state(null);
  let data = $state(null);
  let page = $state(1);
  let search = $state('');
  let requeueing = $state(false);
  const pageSize = 50;

  // Outbound customer-reply queue state.
  let activeTab = $state('inbound');
  let outboxStatus = $state('pending');
  let outboxData = $state(null);
  let outboxLoading = $state(false);
  let outboxError = $state(null);
  let outboxPage = $state(1);
  let actingCommentID = $state(null);

  const debouncedSearch = useDebounce(() => {
    page = 1;
    loadLog();
  }, 300);

  $effect(() => {
    if (isOpen && channel) {
      search = '';
      page = 1;
      outboxPage = 1;
      untrack(() => {
        loadLog();
        loadOutbox();
      });
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

  async function loadOutbox() {
    try {
      outboxLoading = true;
      outboxError = null;
      outboxData = await api.channels.getEmailReplies(channel.id, outboxStatus, outboxPage, pageSize);
    } catch (err) {
      console.error('Failed to load customer reply queue:', err);
      outboxError = err.message || 'Failed to load customer reply queue';
      outboxData = null;
    } finally {
      outboxLoading = false;
    }
  }

  function onSearchInput(e) {
    search = e.target.value;
    debouncedSearch();
  }

  function onTabChange(event) {
    activeTab = event.tab;
    if (activeTab === 'outbox' && !outboxData) {
      untrack(() => loadOutbox());
    }
  }

  function onOutboxStatusChange() {
    outboxPage = 1;
    loadOutbox();
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

  function outboxPrevPage() {
    if (outboxPage > 1) {
      outboxPage--;
      loadOutbox();
    }
  }

  function outboxNextPage() {
    if (outboxData && outboxPage * pageSize < outboxData.total) {
      outboxPage++;
      loadOutbox();
    }
  }

  let totalPages = $derived(data ? Math.max(1, Math.ceil(data.total / pageSize)) : 1);
  let outboxTotalPages = $derived(outboxData ? Math.max(1, Math.ceil(outboxData.total / pageSize)) : 1);

  function formatTime(dateStr) {
    if (!dateStr) return '';
    const date = new Date(dateStr);
    const now = new Date();
    const diff = now.getTime() - date.getTime();
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    if (days < 7) return `${days}d ago`;
    return formatDateSimple(date);
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
      return t('channel.emailLog.commentOn', { key });
    }
    if (key) {
      return t('channel.emailLog.newItem', { key });
    }
    return '-';
  }

  function replyStatus(reply) {
    if (reply.discarded_at) return 'discarded';
    if (reply.delivered_at) return 'delivered';
    if (reply.attempt_count > 0) return 'failed';
    return 'pending';
  }

  function replyStatusText(reply) {
    switch (replyStatus(reply)) {
      case 'discarded': return t('channel.emailLog.outboxDiscarded');
      case 'delivered': return t('channel.emailLog.outboxDelivered');
      case 'failed': return t('channel.emailLog.outboxFailed', { count: reply.attempt_count });
      default: return t('channel.emailLog.outboxPending');
    }
  }

  function isReplyPending(reply) {
    return replyStatus(reply) === 'pending' || replyStatus(reply) === 'failed';
  }

  async function retryReply(reply) {
    if (actingCommentID !== null) return;
    try {
      actingCommentID = reply.comment_id;
      await api.channels.retryEmailReply(channel.id, reply.comment_id);
      await loadOutbox();
    } catch (err) {
      console.error('Failed to retry customer reply:', err);
      outboxError = err.message || 'Failed to retry customer reply';
    } finally {
      actingCommentID = null;
    }
  }

  async function discardReply(reply) {
    if (actingCommentID !== null) return;
    try {
      actingCommentID = reply.comment_id;
      await api.channels.discardEmailReply(channel.id, reply.comment_id);
      await loadOutbox();
    } catch (err) {
      console.error('Failed to discard customer reply:', err);
      outboxError = err.message || 'Failed to discard customer reply';
    } finally {
      actingCommentID = null;
    }
  }

  async function requeueRateLimited() {
    if (requeueing || !channel) return;
    try {
      requeueing = true;
      await api.channels.requeueRateLimitedEmail(channel.id);
      await loadLog();
    } catch (err) {
      console.error('Failed to requeue rate-limited emails:', err);
      error = err.message || 'Failed to requeue rate-limited emails';
    } finally {
      requeueing = false;
    }
  }

  const messageColumns = [
    { key: 'from', label: t('channel.emailLog.from'), slot: 'from' },
    { key: 'subject', label: t('channel.emailLog.subject'), slot: 'subject' },
    { key: 'result', label: t('channel.emailLog.result'), slot: 'result' },
    { key: 'processed_at', label: t('channel.emailLog.processedAt'), render: (msg) => formatTime(msg.processed_at), textColor: 'var(--ds-text-subtle)' },
  ];

  const outboxColumns = [
    { key: 'to', label: t('channel.emailLog.outboxTo'), slot: 'to' },
    { key: 'subject', label: t('channel.emailLog.subject'), slot: 'outboxSubject' },
    { key: 'status', label: t('channel.emailLog.outboxStatus'), slot: 'outboxStatus' },
    { key: 'next_attempt', label: t('channel.emailLog.outboxNextAttempt'), slot: 'outboxNextAttempt', textColor: 'var(--ds-text-subtle)' },
    { key: 'actions', label: '', slot: 'outboxActions' },
  ];

  const outboxStatusOptions = [
    { value: 'pending', label: t('channel.emailLog.outboxFilterPending') },
    { value: 'delivered', label: t('channel.emailLog.outboxDelivered') },
    { value: 'discarded', label: t('channel.emailLog.outboxDiscarded') },
    { value: 'all', label: t('channel.emailLog.outboxFilterAll') },
  ];

  const modalTabs = [
    { id: 'inbound', label: t('channel.emailLog.tabInbound'), testid: 'email-log-tab-inbound' },
    { id: 'outbox', label: t('channel.emailLog.tabOutbox'), testid: 'email-log-tab-outbox' },
  ];
</script>

<Modal
  {isOpen}
  onclose={onClose}
  maxWidth="max-w-3xl"
>
  <ModalHeader
    icon={FileText}
    title={t('channel.processingLog')}
    subtitle={channel?.name || ''}
    showCloseButton={false}
  />

  <!-- Content -->
  <div class="p-6">
    <div class="border-b mb-4" style="border-color: var(--ds-border);">
      <nav class="-mb-px flex space-x-6" aria-label="Sections">
        {#each modalTabs as tab (tab.id)}
          <button
            type="button"
            data-testid={tab.testid}
            class="whitespace-nowrap py-2 px-1 border-b-2 font-medium text-sm transition-colors"
            style={activeTab === tab.id
              ? 'border-color: var(--ds-interactive); color: var(--ds-interactive);'
              : 'border-color: transparent; color: var(--ds-text-subtle);'}
            onclick={() => onTabChange(tab.id)}
          >
            {tab.label}
          </button>
        {/each}
      </nav>
    </div>

    {#if activeTab === 'inbound'}
    <div class="pt-4">
      {#if loading && !data}
        <StateDisplay type="loading" />
      {:else if error}
        <div class="text-center py-12">
          <p class="text-sm" style="color: var(--ds-text-danger);">{error}</p>
          <Button onclick={loadLog} variant="default" size="small" class="mt-3">
            {t('common.retry')}
          </Button>
        </div>
      {:else if data}
        <!-- Sync Status Banner -->
        <div class="rounded-lg border p-4 mb-6" style="border-color: var(--ds-border); background: var(--ds-surface-sunken, var(--ds-surface));">
          <div class="text-sm font-medium mb-2" style="color: var(--ds-text);">
            {t('channel.emailLog.syncStatus')}
          </div>
          <div class="flex flex-wrap gap-x-6 gap-y-1 text-sm">
            <div>
              <span style="color: var(--ds-text-subtle);">{t('channel.emailLog.lastChecked')}:</span>
              <span style="color: var(--ds-text);">
                {data.state.last_checked_at ? formatTime(data.state.last_checked_at) : t('channel.emailLog.never')}
              </span>
            </div>
            <div class="flex items-center gap-1.5">
              <span style="color: var(--ds-text-subtle);">{t('channel.emailLog.errors')}:</span>
              {#if data.state.error_count > 0}
                <span class="flex items-center gap-1" style="color: var(--ds-text-danger);">
                  <AlertTriangle class="w-3.5 h-3.5" />
                  {data.state.error_count}
                </span>
              {:else}
                <span style="color: var(--ds-text-success, var(--ds-text));">{t('channel.emailLog.noErrors')}</span>
              {/if}
            </div>
          </div>
          {#if data.state.last_error}
            <div class="mt-2 text-xs rounded p-2" style="background: var(--ds-surface-danger, rgba(239, 68, 68, 0.1)); color: var(--ds-text-danger);">
              {data.state.last_error}
            </div>
          {/if}
          {#if data.state.rate_limited_count > 0}
            <div class="mt-2 flex items-center justify-between gap-3 rounded p-2" style="background: var(--ds-surface-warning, rgba(234, 88, 12, 0.1));" data-testid="email-log-rate-limited-banner">
              <div class="text-xs" style="color: var(--ds-text-warning, var(--ds-text));">
                {t('channel.emailLog.rateLimitedCount', { count: data.state.rate_limited_count })}
              </div>
              <Button
                onclick={requeueRateLimited}
                variant="default"
                size="small"
                disabled={requeueing}
                dataTestid="email-log-requeue-button"
              >
                {requeueing ? t('channel.emailLog.requeueing') : t('channel.emailLog.requeue')}
              </Button>
            </div>
          {/if}
        </div>

        <!-- Search -->
        <SearchInput value={search} on_input={onSearchInput} className="mb-4" />

        <!-- Messages Table -->
        {#if data.messages.length === 0}
          <EmptyState
            icon={Mail}
            title={search
              ? t('channel.emailLog.noResults')
              : t('channel.emailLog.noEmails')}
          />
        {:else}
          <DataTable columns={messageColumns} data={data.messages} keyField="id">
            {#snippet from(msg)}
              <div class="font-medium truncate max-w-48" style="color: var(--ds-text);">{msg.from_name || msg.from_email}</div>
              {#if msg.from_name}
                <div class="text-xs truncate max-w-48" style="color: var(--ds-text-subtle);">{msg.from_email}</div>
              {/if}
            {/snippet}
            {#snippet subject(msg)}
              <span class="truncate max-w-56 inline-block" style="color: var(--ds-text);">{msg.subject}</span>
            {/snippet}
            {#snippet result(msg)}
              {#if msg.rate_limited_at}
                <span class="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full" style="background: var(--ds-surface-warning, rgba(234, 88, 12, 0.12)); color: var(--ds-text-warning, var(--ds-text));" data-testid="email-log-rate-limited-badge">
                  <AlertTriangle class="w-3 h-3" />
                  {t('channel.emailLog.rateLimited')}
                </span>
              {:else if msg.item_id}
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
            {/snippet}
          </DataTable>

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
                {t('channel.emailLog.previous')}
              </Button>
              <span class="text-sm" style="color: var(--ds-text-subtle);">
                {t('channel.emailLog.page', { page, total: totalPages })}
              </span>
              <Button
                onclick={nextPage}
                variant="ghost"
                size="small"
                disabled={page >= totalPages || loading}
              >
                {t('channel.emailLog.next')}
                <ChevronRight class="w-4 h-4 ml-1" />
              </Button>
            </div>
          {/if}
        {/if}
      {/if}
    </div>
    {:else}
    <div class="pt-4">
      {#if outboxLoading && !outboxData}
        <StateDisplay type="loading" />
      {:else if outboxError}
        <div class="text-center py-12">
          <p class="text-sm" style="color: var(--ds-text-danger);">{outboxError}</p>
          <Button onclick={loadOutbox} variant="default" size="small" class="mt-3">
            {t('common.retry')}
          </Button>
        </div>
      {:else if outboxData}
        <div class="flex items-center justify-between mb-4" data-testid="email-outbox-section">
          <div class="text-sm" style="color: var(--ds-text-subtle);">
            {t('channel.emailLog.outboxDescription')}
          </div>
          <div data-testid="email-outbox-status-filter">
            <SelectField
              label=""
              labelColor="default"
              options={outboxStatusOptions}
              bind:value={outboxStatus}
              onchange={onOutboxStatusChange}
              ariaLabel={t('channel.emailLog.outboxStatus')}
            />
          </div>
        </div>

        {#if outboxData.replies.length === 0}
          <EmptyState
            icon={MessageSquare}
            title={t('channel.emailLog.outboxEmpty')}
          />
        {:else}
          <DataTable columns={outboxColumns} data={outboxData.replies} keyField="comment_id">
            {#snippet to(reply)}
              <div class="font-medium truncate max-w-44" style="color: var(--ds-text);">{reply.to_name || reply.to_email}</div>
              {#if reply.to_name}
                <div class="text-xs truncate max-w-44" style="color: var(--ds-text-subtle);">{reply.to_email}</div>
              {/if}
            {/snippet}
            {#snippet outboxSubject(reply)}
              <span class="truncate max-w-44 inline-block" style="color: var(--ds-text);">{reply.subject}</span>
              {#if reply.last_error}
                <div class="text-xs truncate max-w-44 mt-0.5" style="color: var(--ds-text-danger);" title={reply.last_error}>
                  {reply.last_error}
                </div>
              {/if}
            {/snippet}
            {#snippet outboxStatus(reply)}
            <span
              class="inline-flex items-center text-xs font-medium px-2 py-0.5 rounded-full"
              style={replyStatus(reply) === 'failed'
                ? 'background: var(--ds-surface-warning, rgba(234, 88, 12, 0.12)); color: var(--ds-text-warning, var(--ds-text));'
                : 'background: var(--ds-background-neutral, rgba(0,0,0,0.05)); color: var(--ds-text-subtle);'}
              data-testid="email-outbox-status-{reply.comment_id}"
            >
              {replyStatusText(reply)}
            </span>
            {/snippet}
            {#snippet outboxNextAttempt(reply)}
              {#if reply.delivered_at}
                {formatTime(reply.delivered_at)}
              {:else if reply.discarded_at}
                {formatTime(reply.discarded_at)}
              {:else if reply.next_attempt_at}
                {formatTime(reply.next_attempt_at)}
              {:else}
                -
              {/if}
            {/snippet}
            {#snippet outboxActions(reply)}
              {#if isReplyPending(reply)}
                <div class="flex items-center gap-1.5">
                  <Button
                    onclick={() => retryReply(reply)}
                    variant="ghost"
                    size="small"
                    disabled={actingCommentID !== null}
                    dataTestid="email-outbox-retry-{reply.comment_id}"
                  >
                    {actingCommentID === reply.comment_id ? t('channel.emailLog.outboxRetrying') : t('channel.emailLog.outboxRetry')}
                  </Button>
                  <Button
                    onclick={() => discardReply(reply)}
                    variant="ghost"
                    size="small"
                    disabled={actingCommentID !== null}
                    dataTestid="email-outbox-discard-{reply.comment_id}"
                  >
                    {actingCommentID === reply.comment_id ? t('channel.emailLog.outboxDiscarding') : t('channel.emailLog.outboxDiscard')}
                  </Button>
                </div>
              {/if}
            {/snippet}
          </DataTable>

          {#if outboxTotalPages > 1}
            <div class="flex items-center justify-between mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
              <Button
                onclick={outboxPrevPage}
                variant="ghost"
                size="small"
                icon={ChevronLeft}
                disabled={outboxPage <= 1 || outboxLoading}
              >
                {t('channel.emailLog.previous')}
              </Button>
              <span class="text-sm" style="color: var(--ds-text-subtle);">
                {t('channel.emailLog.page', { page: outboxPage, total: outboxTotalPages })}
              </span>
              <Button
                onclick={outboxNextPage}
                variant="ghost"
                size="small"
                disabled={outboxPage >= outboxTotalPages || outboxLoading}
              >
                {t('channel.emailLog.next')}
                <ChevronRight class="w-4 h-4 ml-1" />
              </Button>
            </div>
          {/if}
        {/if}
      {/if}
    </div>
    {/if}
  </div>
</Modal>
