<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { AlertCircle, Check, X, MessageSquare, RotateCcw, ChevronUp, ChevronDown, Clock, ShieldX } from '@lucide/svelte';
  import Button from '../../components/Button.svelte';
  import Textarea from '../../components/Textarea.svelte';
  import Badge from '../../components/Badge.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import { formatAuthenticatedDateTime as formatDateTimeLocale } from '../../utils/authenticatedDateFormatter.js';
  import { authStore } from '../../stores';
  import { confirm } from '../../composables/useConfirm.js';

  let {
    itemId,
    canCancel = false,
    initialRequests = null,
    ondecisionMade = null,
  } = $props();

  let requests = $state([]);
  let loading = $state(true);
  let acting = $state(false);
  let commentsByRequest = $state({});
  let expandedRequests = $state(new Set());
  let lastInitialRequests = null;

  function applyRequests(nextRequests) {
    requests = nextRequests || [];
    commentsByRequest = Object.fromEntries(
      requests.map((request) => [request.id, commentsByRequest[request.id] ?? ''])
    );
    const next = new Set(expandedRequests);
    for (const request of requests) if (request.status === 'pending') next.add(request.id);
    expandedRequests = next;
  }

  $effect(() => {
    if (initialRequests === null || initialRequests === lastInitialRequests) return;
    lastInitialRequests = initialRequests;
    applyRequests(initialRequests);
    loading = false;
  });

  onMount(() => {
    if (initialRequests === null) void load();
  });

  async function load() {
    if (!itemId) return;
    try {
      loading = true;
      applyRequests((await api.approvals.forItem(itemId)) || []);
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
      case 'pending': return { variant: 'warning', label: t('items.approvals.statuses.pending') };
      case 'approved': return { variant: 'success', label: t('items.approvals.statuses.approved') };
      case 'rejected': return { variant: 'danger', label: t('items.approvals.statuses.rejected') };
      case 'cancelled': return { variant: 'neutral', label: t('items.approvals.statuses.cancelled') };
      default: return { variant: 'neutral', label: status };
    }
  }

  function activeStepForCurrentUser(req) {
    const me = authStore.currentUser?.id;
    if (!me) return undefined;
    return req.step_instances?.find(si =>
      si.status === 'pending' &&
      si.started_at &&
      si.approvers?.some(a => a.user_id === me && a.is_active)
    );
  }

  function canCancelRequest(req) {
    return canCancel || req.triggered_by_user_id === authStore.currentUser?.id;
  }

  function hasActiveApprovers(step) {
    return step.approvers?.some(a => a.is_active) ?? false;
  }

  async function decide(req, decision) {
    if (decision !== 'comment') {
      const isApprove = decision === 'approve';
      const ok = await confirm({
        title: isApprove ? t('items.approvals.approveTitle') : t('items.approvals.rejectTitle'),
        message: isApprove
          ? t('items.approvals.approveMessage')
          : t('items.approvals.rejectMessage'),
        confirmText: isApprove ? t('items.approvals.approve') : t('items.approvals.reject'),
        cancelText: t('common.cancel'),
        variant: isApprove ? 'info' : 'danger',
      });
      if (!ok) return;
    }
    acting = true;
    try {
      await api.approvals.decide(req.id, decision, commentsByRequest[req.id] ?? '');
      commentsByRequest[req.id] = '';
      successToast(t('items.approvals.decisionRecorded', { decision: decisionLabel({ decision }) }));
      await load();
      ondecisionMade?.(requests);
    } catch (err) {
      errorToast(err.message || JSON.stringify(err));
    } finally {
      acting = false;
    }
  }

  async function cancelReq(req) {
    const ok = await confirm({
      title: t('items.approvals.cancelTitle'),
      message: t('items.approvals.cancelMessage'),
      confirmText: t('items.approvals.cancelRequest'),
      cancelText: t('items.approvals.keepOpen'),
      variant: 'warning',
    });
    if (!ok) return;
    acting = true;
    try {
      await api.approvals.cancel(req.id, commentsByRequest[req.id] ?? '');
      commentsByRequest[req.id] = '';
      successToast(t('items.approvals.cancelledSuccess'));
      await load();
      ondecisionMade?.(requests);
    } catch (err) {
      errorToast(err.message || JSON.stringify(err));
    } finally {
      acting = false;
    }
  }

  // Decision-row formatting for the audit log.
  function decisionLabel(d) {
    switch (d.decision) {
      case 'approve': return t('items.approvals.decisions.approve');
      case 'reject': return t('items.approvals.decisions.reject');
      case 'comment': return t('items.approvals.decisions.comment');
      case 'cancel': return t('items.approvals.decisions.cancel');
      case 'delegate': return t('items.approvals.decisions.delegate', { userId: d.delegated_to_user_id });
      case 'reassign': return t('items.approvals.decisions.reassign');
      case 'escalate': return t('items.approvals.decisions.escalate');
      case 'substitute': return t('items.approvals.decisions.substitute');
      case 'requested': return t('items.approvals.decisions.requested');
      case 'completed': return t('items.approvals.decisions.completed');
      default: return d.decision;
    }
  }
</script>

<div class="space-y-4" data-testid="approvals-timeline">
  {#if loading}
    <div class="text-sm" style="color: var(--ds-text-subtle);">{t('items.approvals.loading')}</div>
  {:else if requests.length === 0}
    <EmptyState icon={ShieldX} title={t('items.approvals.emptyTitle')} description={t('items.approvals.emptyDescription')} />
  {:else}
    {#each requests as req (req.id)}
      {@const expanded = expandedRequests.has(req.id)}
      {@const badge = statusBadge(req.status)}
      {@const myStep = activeStepForCurrentUser(req)}
      <div
        class="border rounded-lg"
        style="border-color: var(--ds-border); background: var(--ds-surface-raised);"
        data-testid={`approval-request-${req.id}`}
      >
        <button type="button" class="w-full flex items-center justify-between p-3 text-left"
                onclick={() => toggleExpand(req.id)}>
          <div class="flex items-center gap-3 min-w-0">
            {#if expanded}<ChevronUp class="w-4 h-4" />{:else}<ChevronDown class="w-4 h-4" />{/if}
            <div>
              <div class="text-sm font-medium" style="color: var(--ds-text);">
                {t('items.approvals.requestNumber', { id: req.id })}
              </div>
              <div class="text-xs" style="color: var(--ds-text-subtle);">
                {t('items.approvals.opened', { date: formatDateTimeLocale(req.created_at) })}
                {#if req.completed_at} · {t('items.approvals.closed', { date: formatDateTimeLocale(req.completed_at) })}{/if}
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
                <div
                  class="flex items-start gap-3 p-2 rounded"
                  style="background: var(--ds-surface);"
                  data-testid={`approval-step-${si.id}`}
                  data-step-status={si.status}
                >
                  <div class="text-xs font-mono w-6 text-center" style="color: var(--ds-text-subtle);">
                    {si.display_order + 1}
                  </div>
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <span class="text-sm font-medium" style="color: var(--ds-text);">
                        {t('items.approvals.stepNumber', { number: si.display_order + 1 })}
                      </span>
                      <Badge size="xs" variant={statusBadge(si.status).variant}>{statusBadge(si.status).label}</Badge>
                      {#if si.escalation_count > 0}
                        <span class="text-xs" style="color: var(--ds-text-warning, #d97706);">
                          {t('items.approvals.escalatedCount', { count: si.escalation_count })}
                        </span>
                      {/if}
                    </div>
                    {#if si.approvers?.length > 0}
                      <div class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                        {t('items.approvals.approvers')}: {si.approvers.filter(a => a.is_active).map(a => `#${a.user_id}`).join(', ') || t('common.none')}
                      </div>
                    {/if}
                    {#if si.status === 'pending' && si.started_at && !hasActiveApprovers(si)}
                      <div
                        class="flex items-start gap-2 mt-2 p-2 rounded text-xs"
                        style="color: var(--ds-text-warning, #d97706); background: color-mix(in srgb, var(--ds-text-warning, #d97706) 10%, transparent);"
                        data-testid="approval-empty-pool-warning"
                      >
                        <AlertCircle class="w-3.5 h-3.5 mt-0.5 shrink-0" />
                        <div>
                          <div class="font-medium">{t('items.approvals.noEligibleApprovers')}</div>
                          {#if req.triggered_by_user_id === authStore.currentUser?.id}
                            <div class="mt-0.5">
                              {t('items.approvals.selfApprovalHint')}
                            </div>
                          {:else}
                            <div class="mt-0.5">
                              {t('items.approvals.approverSourceHint')}
                            </div>
                          {/if}
                        </div>
                      </div>
                    {/if}
                    {#if si.escalation_due_at && si.status === 'pending'}
                      <div class="text-xs mt-1 flex items-center gap-1" style="color: var(--ds-text-subtle);">
                        <Clock class="w-3 h-3" /> {t('items.approvals.escalates', { date: formatDateTimeLocale(si.escalation_due_at) })}
                      </div>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>

            <!-- Decision actions for the active pool -->
            {#if req.status === 'pending' && myStep}
              <div class="border-t pt-4 space-y-3" style="border-color: var(--ds-border);">
                <div class="text-sm font-medium" style="color: var(--ds-text);">
                  {t('items.approvals.decisionRequired')}
                </div>
                <div class="flex flex-wrap gap-2">
                  <Button variant="primary" icon={Check} disabled={acting}
                          onclick={() => decide(req, 'approve')}
                          dataTestid="approval-decision-approve">
                    {t('items.approvals.approve')}
                  </Button>
                  <Button variant="danger" icon={X} disabled={acting}
                          onclick={() => decide(req, 'reject')}
                          dataTestid="approval-decision-reject">
                    {t('items.approvals.reject')}
                  </Button>
                  <Button variant="default" icon={MessageSquare}
                          disabled={acting || commentsByRequest[req.id].trim() === ''}
                          onclick={() => decide(req, 'comment')}
                          dataTestid="approval-decision-comment-submit">
                    {t('items.approvals.comment')}
                  </Button>
                </div>
                <Textarea
                  class="text-sm"
                  style="background: var(--ds-surface);"
                  rows={2}
                  placeholder={t('items.approvals.commentPlaceholder')}
                  bind:value={commentsByRequest[req.id]}
                  data-testid="approval-decision-comment"
                  size="small"
                />
              </div>
            {/if}

            <!-- Cancel for requestor or users with item.edit permission. -->
            {#if req.status === 'pending' && canCancelRequest(req)}
              <div class="border-t pt-4" style="border-color: var(--ds-border);">
                <Button variant="ghost" icon={RotateCcw} disabled={acting}
                        onclick={() => cancelReq(req)}>
                  {t('items.approvals.cancelRequest')}
                </Button>
              </div>
            {/if}

            <!-- Audit log -->
            {#if req.decisions?.length > 0}
              <div class="border-t pt-4" style="border-color: var(--ds-border);">
                <div class="text-xs font-medium mb-2" style="color: var(--ds-text-subtle);">{t('items.approvals.auditLog')}</div>
                <ul class="space-y-1 text-xs">
                  {#each req.decisions as d (d.id)}
                    <li style="color: var(--ds-text-subtle);">
                      <span style="color: var(--ds-text);">
                        {d.actor_user_id ? t('items.approvals.userNumber', { id: d.actor_user_id }) : t('items.approvals.system')}
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
