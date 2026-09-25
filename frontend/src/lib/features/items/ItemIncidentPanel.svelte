<script>
  import { Siren, Check, BellOff, RotateCcw } from '@lucide/svelte';
  import Button from '../../components/Button.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Text from '../../components/Text.svelte';
  import { api } from '../../api.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';

  let { itemId, item = null, canEdit = false, onchanged = null } = $props();

  let incident = $state(null);
  let loading = $state(true);
  let busy = $state(false);
  let loadToken = 0;

  async function load(id) {
    const token = ++loadToken;
    loading = true;
    try {
      const result = await api.itemIncidents.get(id);
      if (token === loadToken) incident = result;
    } catch (error) {
      if (token !== loadToken) return;
      if (error?.status === 404) {
        incident = null;
      } else if (error?.name !== 'AbortError') {
        console.error('Failed to load incident:', error);
      }
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  $effect(() => {
    const id = itemId;
    if (id == null) return;
    load(id);
  });

  async function run(action, successMessage) {
    if (busy) return;
    busy = true;
    try {
      incident = await action();
      successToast(successMessage);
      onchanged?.();
    } catch (error) {
      if (error?.status === 403) {
        errorToast(t('items.incidentResponderRequired'));
      } else {
        errorToast(error?.message || t('items.incidentActionFailed'));
      }
    } finally {
      busy = false;
    }
  }

  function declareIncident() {
    return run(() => api.itemIncidents.trigger(itemId), t('items.incidentDeclared'));
  }
  function acknowledge() {
    return run(() => api.itemIncidents.acknowledge(itemId), t('items.incidentAcknowledged'));
  }
  function unacknowledge() {
    return run(() => api.itemIncidents.unacknowledge(itemId), t('items.incidentUnacknowledged'));
  }
  function resolve() {
    return run(() => api.itemIncidents.resolve(itemId), t('items.incidentResolved'));
  }

  const statusAppearance = $derived(
    incident?.status === 'triggered'
      ? { color: 'red', label: t('items.incidentTriggered') }
      : incident?.status === 'acknowledged'
        ? { color: 'amber', label: t('items.incidentAcknowledged') }
        : { color: 'green', label: t('items.incidentResolved') }
  );

  function formatTimestamp(value) {
    if (!value) return '';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? '' : date.toLocaleString();
  }
</script>

{#if loading}
  <div class="mb-4 h-14 rounded animate-pulse" style="background-color: var(--ds-background-neutral);" data-testid="item-incident-loading"></div>
{:else if incident}
  <div
    class="mb-4 rounded border p-3"
    style="border-color: var(--ds-border); background-color: var(--ds-surface);"
    data-testid="item-incident-panel"
    data-incident-status={incident.status}
  >
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-2">
        <Siren size={16} style="color: var(--ds-text-danger, #ef4444);" />
        <Text weight="semibold" size="sm">{t('items.incident')}</Text>
        <Lozenge color={statusAppearance.color} dataTestid="item-incident-status">{statusAppearance.label}</Lozenge>
        {#if incident.urgency === 'low'}
          <Lozenge color="gray">{t('items.incidentLowUrgency')}</Lozenge>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        {#if incident.status === 'triggered'}
          <Button size="sm" variant="primary" disabled={busy} onclick={acknowledge} dataTestid="item-incident-acknowledge">{t('items.incidentAck')}</Button>
        {:else if incident.status === 'acknowledged'}
          <Button size="sm" variant="secondary" disabled={busy} onclick={unacknowledge} dataTestid="item-incident-unacknowledge">
            <RotateCcw size={14} /> {t('items.incidentUnack')}
          </Button>
        {/if}
        {#if incident.status !== 'resolved'}
          <Button size="sm" variant="secondary" disabled={busy} onclick={resolve} dataTestid="item-incident-resolve">
            <Check size={14} /> {t('items.incidentResolve')}
          </Button>
        {:else if canEdit && item?.team_id}
          <Button size="sm" variant="secondary" disabled={busy} onclick={declareIncident} dataTestid="item-incident-retrigger">{t('items.incidentDeclare')}</Button>
        {/if}
      </div>
    </div>

    <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs" style="color: var(--ds-text-subtle);">
      {#if incident.policy_name}
        <span>{t('items.incidentPolicy')}: {incident.policy_name}</span>
      {/if}
      <span>{t('items.incidentTriggeredAt')}: {formatTimestamp(incident.triggered_at)}</span>
      {#if incident.acknowledged_by_name}
        <span>{t('items.incidentAckedBy')}: {incident.acknowledged_by_name}</span>
      {/if}
      {#if incident.resolved_by_name}
        <span>{t('items.incidentResolvedBy')}: {incident.resolved_by_name}</span>
      {/if}
      {#if incident.escalation_repeat_count > 0}
        <span>{t('items.incidentRepeat')}: {incident.escalation_repeat_count}</span>
      {/if}
    </div>
  </div>
{:else if canEdit && item?.team_id}
  <div class="mb-4" data-testid="item-incident-none">
    <Button size="sm" variant="secondary" disabled={busy} onclick={declareIncident} dataTestid="item-incident-trigger">
      <Siren size={14} /> {t('items.incidentDeclare')}
    </Button>
  </div>
{/if}
