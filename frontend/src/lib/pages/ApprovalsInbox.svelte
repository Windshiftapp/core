<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { navigate } from '../router.js';
  import { IconRubberStamp } from '@tabler/icons-svelte-runes';
  import PageHeader from '../layout/PageHeader.svelte';
  import Card from '../components/Card.svelte';
  import EmptyState from '../components/EmptyState.svelte';
  import Badge from '../components/Badge.svelte';
  import Button from '../components/Button.svelte';
  import { formatDateTimeLocale } from '../utils/dateFormatter.js';

  let requests = $state([]);
  let loading = $state(true);
  let statusFilter = $state('pending');

  $effect(() => {
    load(statusFilter);
  });

  async function load(status) {
    try {
      loading = true;
      requests = (await api.approvals.mine(status)) || [];
    } catch (err) {
      console.error(err);
      errorToast(err.message || JSON.stringify(err));
      requests = [];
    } finally {
      loading = false;
    }
  }

  function open(req) {
    // Open the item detail view; the Approvals tab on that view will surface
    // the request and the decide buttons.
    navigate(`/items/${req.item_id}`);
  }
</script>

<div class="p-6 max-w-5xl mx-auto">
  <PageHeader
    icon={IconRubberStamp}
    title="My Approvals"
    subtitle="Approval requests where you're an active approver."
  />

  <div class="mb-4 flex gap-2">
    {#each ['pending', 'approved', 'rejected', 'cancelled'] as f}
      <Button
        variant={statusFilter === f ? 'primary' : 'default'}
        size="small"
        onclick={() => statusFilter = f}
      >
        {f}
      </Button>
    {/each}
  </div>

  {#if loading}
    <Card padding="loose"><div style="color: var(--ds-text-subtle);">Loading…</div></Card>
  {:else if requests.length === 0}
    <Card padding="generous">
      <EmptyState
        icon={IconRubberStamp}
        title="Nothing in this view"
        description={statusFilter === 'pending'
          ? "No approvals are waiting on your decision."
          : `No ${statusFilter} approvals.`}
      />
    </Card>
  {:else}
    <div class="space-y-3" data-testid="approvals-mine-list">
      {#each requests as req (req.id)}
        <Card padding="spacious">
          <div class="flex items-center justify-between gap-3">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-1">
                <span class="text-sm font-medium" style="color: var(--ds-text);">
                  Approval #{req.id}
                </span>
                <Badge size="sm" variant={req.status === 'pending' ? 'warning' : 'neutral'}>
                  {req.status}
                </Badge>
              </div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">
                Item #{req.item_id} · Opened {formatDateTimeLocale(req.created_at)}
              </div>
            </div>
            <Button variant="default" size="small" onclick={() => open(req)}>
              Open item
            </Button>
          </div>
        </Card>
      {/each}
    </div>
  {/if}
</div>
