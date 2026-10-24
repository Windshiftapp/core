<script>
  import IconInbox from '@tabler/icons-svelte-runes/icons/inbox';
  import IconMailExclamation from '@tabler/icons-svelte-runes/icons/mail-exclamation';
  import Badge from '../../components/Badge.svelte';
  import Card from '../../components/Card.svelte';
  import { getEmailIntakeDiagnostics } from '../../api/diagnostics.js';
  import DiagnosticsSection from './DiagnosticsSection.svelte';
  import { formatUtcTime } from './format-utils.js';

  const WINDOWS = [
    { value: '24h', label: 'Last 24 hours' },
    { value: '168h', label: 'Last 7 days' },
    { value: '720h', label: 'Last 30 days' },
  ];

  const CATEGORY_LABELS = {
    delivery_failure: 'Delivery failure',
    auto_reply: 'Auto-reply',
  };

  let window = $state('24h');
  let view = $state({ loading: true, error: null, counts: [], messages: [] });
  let lastRefreshed = $state(null);

  async function load() {
    view = { ...view, loading: true, error: null };
    try {
      const data = await getEmailIntakeDiagnostics({ since: window, limit: 100 });
      view = {
        loading: false,
        error: null,
        counts: data?.counts ?? [],
        messages: data?.messages ?? [],
      };
      lastRefreshed = new Date();
    } catch (error) {
      view = { ...view, loading: false, error: error?.message ?? String(error) };
    }
  }

  const total = $derived(view.counts.reduce((sum, entry) => sum + (entry.count ?? 0), 0));

  function categoryLabel(category) {
    return CATEGORY_LABELS[category] ?? category ?? 'Unknown';
  }

  function categoryVariant(category) {
    return category === 'delivery_failure' ? 'danger' : 'neutral';
  }
</script>

<DiagnosticsSection
  title="Email intake: system-generated mail"
  subtitle="Bounces and auto-replies that arrived by email and were deliberately not turned into work items. They are kept here so delivery failures and responder loops stay visible without cluttering the backlog."
  dataTestId="diagnostics-email-intake"
  onLoad={load}
  lastRefreshed={lastRefreshed}
  bind:loading={view.loading}
  bind:error={view.error}
>
  {#snippet children()}
    <Card>
      <div class="flex items-center justify-between gap-4 flex-wrap">
        <div class="flex items-center gap-3">
          <IconInbox size={20} stroke={1.75} style="color: var(--ds-icon-subtle);" />
          <div>
            <div class="text-sm font-medium" style="color: var(--ds-text);">
              {total} skipped message{total === 1 ? '' : 's'}
            </div>
            <div class="text-xs" style="color: var(--ds-text-subtle);">
              Not created as work items; recorded for diagnostics only.
            </div>
          </div>
        </div>
        <label class="text-sm flex items-center gap-2" style="color: var(--ds-text-subtle);">
          Window
          <select
            bind:value={window}
            onchange={load}
            class="rounded-md border px-2 py-1 text-sm"
            style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
            data-testid="email-intake-window"
          >
            {#each WINDOWS as option (option.value)}
              <option value={option.value}>{option.label}</option>
            {/each}
          </select>
        </label>
      </div>
    </Card>

    {#if !view.loading && !view.error && total === 0}
      <Card variant="outlined">
        <p class="text-sm" style="color: var(--ds-text-subtle);">
          No system-generated mail was skipped in this window.
        </p>
      </Card>
    {:else if !view.error}
      {#if view.counts.length > 0}
        <div class="flex flex-wrap gap-2" data-testid="email-intake-counts">
          {#each view.counts as entry (entry.category)}
            <Badge variant={categoryVariant(entry.category)}>
              {categoryLabel(entry.category)}: {entry.count}
            </Badge>
          {/each}
        </div>
      {/if}

      <Card variant="outlined">
        <div class="overflow-x-auto">
          <table class="w-full text-sm" data-testid="email-intake-table">
            <thead>
              <tr class="text-left text-xs uppercase tracking-wide" style="color: var(--ds-text-subtle);">
                <th class="py-2 pr-4 font-medium">Received</th>
                <th class="py-2 pr-4 font-medium">Type</th>
                <th class="py-2 pr-4 font-medium">From</th>
                <th class="py-2 pr-4 font-medium">Subject</th>
                <th class="py-2 pr-4 font-medium">Mailbox</th>
              </tr>
            </thead>
            <tbody>
              {#each view.messages as message (message.id)}
                <tr
                  class="border-t"
                  style="border-color: var(--ds-border);"
                  data-testid={`email-intake-row-${message.id}`}
                >
                  <td class="py-2 pr-4 whitespace-nowrap" style="color: var(--ds-text-subtle);">
                    {formatUtcTime(message.processed_at)}
                  </td>
                  <td class="py-2 pr-4">
                    <Badge variant={categoryVariant(message.category)} size="xs">
                      {categoryLabel(message.category)}
                    </Badge>
                  </td>
                  <td class="py-2 pr-4" style="color: var(--ds-text);">
                    {#if message.from_name}
                      <span class="font-medium">{message.from_name}</span>
                      <span style="color: var(--ds-text-subtle);">&lt;{message.from_email}&gt;</span>
                    {:else}
                      {message.from_email}
                    {/if}
                  </td>
                  <td class="py-2 pr-4" style="color: var(--ds-text);">{message.subject || '—'}</td>
                  <td class="py-2 pr-4 whitespace-nowrap" style="color: var(--ds-text-subtle);">
                    {message.channel_name}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </Card>

      {#if view.counts.some((entry) => entry.category === 'delivery_failure')}
        <Card variant="outlined">
          <div class="flex items-start gap-3" data-testid="email-intake-delivery-hint">
            <IconMailExclamation size={18} stroke={1.75} style="flex-shrink: 0; margin-top: 2px; color: var(--ds-text-danger);" />
            <p class="text-sm" style="color: var(--ds-text-subtle);">
              Delivery failures usually point at an outbound configuration problem (SPF, DKIM, or DMARC).
              Check the bounce details in the mailbox before they are marked read or deleted.
            </p>
          </div>
        </Card>
      {/if}
    {/if}
  {/snippet}
</DiagnosticsSection>
