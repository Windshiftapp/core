<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { Check, X, MessageSquare, RotateCcw, ChevronUp, ChevronDown, Clock, ShieldX } from 'lucide-svelte';
  import Button from '../../components/Button.svelte';
  import Badge from '../../components/Badge.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { formatDateTimeLocale } from '../../utils/dateFormatter.js';
  import { authStore } from '../../stores';

  let { itemId } = $props();

  let requests = $state([]);
  let loading = $state(true);
  let acting = $state(false);
  let comment = $state('');
  let expandedRequests = $state(new Set());

  onMount(load);

  async function load() {
    if (!itemId) return;
    try {
      loading = true;
      requests = (await api.approvals.forItem(itemId)) || [];
      // Auto-expand any pending request so the user lands on the actionable card.
      const next = new Set(expandedRequests);
      for (const r of requests) if (r.status === 'pending') next.add(r.id);
      expandedRequests = next;
    } catch (err) {
      console.error('load approvals', err);
      errorToast(err.message || JSON.stringify(err));
      requests = [];
    } finally {
      loading = false;
    }
  }

  function toggleExpand(id) {
    const next = new Set(expandedRequests);
    if (next.has(id)) next.delete(id); else next.add(id);
    expandedRequests = next;
  }

  function statusBadge(status) {
    switch (status) {
      case 'pending': return { variant: 'warning', label: 'Pending' };
      case 'approved': return { variant: 'success', label: 'Approved' };
      case 'rejected': return { variant: 'danger', label: 'Rejected' };
      case 'cancelled': return { variant: 'neutral', label: 'Cancelled' };
      default: return { variant: 'neutral', label: status };
    }
  }

  function activeStep(req) {
    return req.step_instances?.find(si => si.status === 'pending' && si.started_at);
  }

  function isInActivePool(req) {
    const me = authStore.currentUser?.id;
    if (!me) return false;
    const step = activeStep(req);
    return !!step?.approvers?.some(a => a.user_id === me && a.is_active);
  }

  async function decide(req, decision) {
    if (decision !== 'comment' && !window.confirm(`${decision === 'approve' ? 'Approve' : 'Reject'} this request?`)) return;
    acting = true;
    try {
      await api.approvals.decide(req.id, decision, comment);
      comment = '';
      successToast(`Decision recorded: ${decision}`);
      await load();
    } catch (err) {
      errorToast(err.message || JSON.stringify(err));
    } finally {
      acting = false;
    }
  }

  async function cancelReq(req) {
    if (!window.confirm('Cancel this approval request?')) return;
    acting = true;
    try {
      await api.approvals.cancel(req.id, comment);
      comment = '';
      successToast('Approval cancelled');
      await load();
    } catch (err) {
      errorToast(err.message || JSON.stringify(err));
    } finally {
      acting = false;
    }
  }

  // Decision-row formatting for the audit log.
  function decisionLabel(d) {
    switch (d.decision) {
      case 'approve': return 'approved';
      case 'reject': return 'rejected';
      case 'comment': return 'commented';
      case 'cancel': return 'cancelled the request';
      case 'delegate': return `delegated to user #${d.delegated_to_user_id}`;
      case 'reassign': return 'reassigned approvers';
      case 'escalate': return 'was escalated';
      case 'substitute': return 'used a substitute';
      case 'requested': return 'opened the request';
      case 'completed': return 'finalized the request';
      default: return d.decision;
    }
  }
</script>

<div class="space-y-4" data-testid="approvals-timeline">
  {#if loading}
    <div class="text-sm" style="color: var(--ds-text-subtle);">Loading approvals…</div>
  {:else if requests.length === 0}
    <EmptyState icon={ShieldX} title="No approvals" description="No approval activity has happened on this item." />
  {:else}
    {#each requests as req (req.id)}
      {@const expanded = expandedRequests.has(req.id)}
      {@const badge = statusBadge(req.status)}
      {@const inPool = isInActivePool(req)}
      {@const myStep = activeStep(req)}
      <div class="border rounded-lg" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
        <button type="button" class="w-full flex items-center justify-between p-3 text-left"
                onclick={() => toggleExpand(req.id)}>
          <div class="flex items-center gap-3 min-w-0">
            {#if expanded}<ChevronUp class="w-4 h-4" />{:else}<ChevronDown class="w-4 h-4" />{/if}
            <div>
              <div class="text-sm font-medium" style="color: var(--ds-text);">
                Approval #{req.id}
              </div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">
                Opened {formatDateTimeLocale(req.created_at)}
                {#if req.completed_at} · Closed {formatDateTimeLocale(req.completed_at)}{/if}
              </div>
            </div>
          </div>
          <Badge variant={badge.variant} size="sm">{badge.label}</Badge>
        </button>

        {#if expanded}
          <div class="px-4 pb-4 space-y-4 border-t pt-4" style="border-color: var(--ds-border);">
            <!-- Step list -->
            <div class="space-y-2">
              {#each req.step_instances ?? [] as si (si.id)}
                <div class="flex items-start gap-3 p-2 rounded" style="background: var(--ds-surface);">
                  <div class="text-xs font-mono w-6 text-center" style="color: var(--ds-text-subtle);">
                    {si.display_order + 1}
                  </div>
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <span class="text-sm font-medium" style="color: var(--ds-text);">
                        Step {si.display_order + 1}
                      </span>
                      <Badge size="xs" variant={statusBadge(si.status).variant}>{si.status}</Badge>
                      {#if si.escalation_count > 0}
                        <span class="text-xs" style="color: var(--ds-text-warning, #d97706);">
                          ↑ escalated {si.escalation_count}×
                        </span>
                      {/if}
                    </div>
                    {#if si.approvers?.length > 0}
                      <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                        Approvers: {si.approvers.filter(a => a.is_active).map(a => `#${a.user_id}`).join(', ') || '(none)'}
                      </div>
                    {/if}
                    {#if si.escalation_due_at && si.status === 'pending'}
                      <div class="text-xs mt-1 flex items-center gap-1" style="color: var(--ds-text-subtle);">
                        <Clock class="w-3 h-3" /> Escalates {formatDateTimeLocale(si.escalation_due_at)}
                      </div>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>

            <!-- Decision actions for the active pool -->
            {#if req.status === 'pending' && inPool && myStep}
              <div class="border-t pt-4 space-y-3" style="border-color: var(--ds-border);">
                <div class="text-sm font-medium" style="color: var(--ds-text);">
                  Your decision is required
                </div>
                <textarea
                  class="w-full px-3 py-2 border rounded text-sm"
                  style="border-color: var(--ds-border); background: var(--ds-surface);"
                  rows="2"
                  placeholder="Optional comment…"
                  bind:value={comment}
                  data-testid="approval-decision-comment"
                ></textarea>
                <div class="flex gap-2">
                  <Button variant="primary" icon={Check} disabled={acting}
                          onclick={() => decide(req, 'approve')}
                          dataTestid="approval-decision-approve">
                    Approve
                  </Button>
                  <Button variant="danger" icon={X} disabled={acting}
                          onclick={() => decide(req, 'reject')}
                          dataTestid="approval-decision-reject">
                    Reject
                  </Button>
                  <Button variant="default" icon={MessageSquare} disabled={acting}
                          onclick={() => decide(req, 'comment')}>
                    Comment
                  </Button>
                </div>
              </div>
            {/if}

            <!-- Cancel for requestor -->
            {#if req.status === 'pending' && req.triggered_by_user_id === authStore.currentUser?.id}
              <div class="border-t pt-4" style="border-color: var(--ds-border);">
                <Button variant="ghost" icon={RotateCcw} disabled={acting}
                        onclick={() => cancelReq(req)}>
                  Cancel approval request
                </Button>
              </div>
            {/if}

            <!-- Audit log -->
            {#if req.decisions?.length > 0}
              <div class="border-t pt-4" style="border-color: var(--ds-border);">
                <div class="text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">Audit log</div>
                <ul class="space-y-1 text-xs">
                  {#each req.decisions as d (d.id)}
                    <li style="color: var(--ds-text-subtle);">
                      <span style="color: var(--ds-text);">
                        {d.actor_user_id ? `User #${d.actor_user_id}` : 'System'}
                      </span>
                      {decisionLabel(d)}
                      <span class="opacity-60"> · {formatDateTimeLocale(d.created_at)}</span>
                      {#if d.comment}
                        <div class="ml-4 mt-1 italic">"{d.comment}"</div>
                      {/if}
                    </li>
                  {/each}
                </ul>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  {/if}
</div>
