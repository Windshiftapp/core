<script>
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { CheckCircle, XCircle, Clock, AlertTriangle, ArrowRight } from 'lucide-svelte';

  let { log, onclose } = $props();

  function parseTrace(log) {
    if (!log.execution_trace) return [];
    try {
      return JSON.parse(log.execution_trace);
    } catch {
      return [];
    }
  }

  function formatStepDescription(step) {
    const output = step.output || {};
    switch (step.node_type) {
      case 'set_status':
        const oldStatus = output.old_status_name || t('actions.config.anyStatus');
        const newStatus = output.new_status_name || t('actions.config.anyStatus');
        return t('actions.trace.setStatus', { from: oldStatus, to: newStatus });
      case 'set_field':
        const fieldName = output.field_name || t('common.unknown');
        const oldVal = output.old_value ?? t('common.empty');
        const newVal = output.new_value ?? t('common.empty');
        return t('actions.trace.setField', { field: fieldName, from: oldVal, to: newVal });
      case 'add_comment':
        const preview = (output.content || '').substring(0, 50);
        const suffix = (output.content?.length > 50) ? '...' : '';
        const privateLabel = output.is_private ? t('actions.config.private') + ' ' : '';
        return t('actions.trace.addComment', { prefix: privateLabel, content: preview + suffix });
      case 'notify_user':
        if (output.skipped) {
          return t('actions.trace.notifySkipped', { reason: output.reason || '' });
        }
        return t('actions.trace.notifyUser', { count: output.recipient_count || 0 });
      case 'condition':
        const result = output.condition_result ? t('actions.condition.true') : t('actions.condition.false');
        return t('actions.trace.conditionResult', { result });
      default:
        return step.node_type;
    }
  }

  function getNodeTypeLabel(nodeType) {
    const labels = {
      'trigger': t('actions.nodes.trigger'),
      'set_field': t('actions.nodes.setField'),
      'set_status': t('actions.nodes.setStatus'),
      'add_comment': t('actions.nodes.addComment'),
      'notify_user': t('actions.nodes.notifyUser'),
      'condition': t('actions.nodes.condition')
    };
    return labels[nodeType] || nodeType;
  }

  function getStatusIcon(status) {
    switch (status) {
      case 'completed':
        return CheckCircle;
      case 'failed':
        return XCircle;
      case 'running':
        return Clock;
      default:
        return AlertTriangle;
    }
  }

  function getStatusColor(status) {
    switch (status) {
      case 'completed':
        return 'var(--ds-success, #22c55e)';
      case 'failed':
        return 'var(--ds-error, #ef4444)';
      case 'running':
        return 'var(--ds-info, #3b82f6)';
      default:
        return 'var(--ds-text-subtle)';
    }
  }

  const steps = $derived(parseTrace(log));
</script>

<Modal isOpen={true} onclose={onclose} maxWidth="max-w-2xl">
  <ModalHeader
    title={t('actions.trace.title')}
    subtitle={log.item_title || ''}
    onClose={onclose}
  />

  <div class="p-6">
    {#if steps.length === 0}
      <div class="text-center py-8" style="color: var(--ds-text-subtle);">
        <AlertTriangle class="w-12 h-12 mx-auto mb-3 opacity-50" />
        <p>{t('actions.trace.noSteps')}</p>
      </div>
    {:else}
      <div class="timeline">
        {#each steps as step, index}
          <div class="timeline-item">
            <!-- Timeline connector -->
            <div class="timeline-connector">
              <div
                class="timeline-marker"
                style="background-color: {getStatusColor(step.status)};"
              >
                {@const StatusIcon = getStatusIcon(step.status)}
                <StatusIcon
                  class="w-4 h-4"
                  style="color: white;"
                />
              </div>
              {#if index < steps.length - 1}
                <div class="timeline-line"></div>
              {/if}
            </div>

            <!-- Step content -->
            <div class="timeline-content">
              <div class="step-header">
                <span class="step-type">{getNodeTypeLabel(step.node_type)}</span>
                <span
                  class="step-status"
                  style="color: {getStatusColor(step.status)};"
                >
                  {t(`actions.logs.${step.status}`)}
                </span>
              </div>
              <p class="step-description">{formatStepDescription(step)}</p>

              {#if step.error_message}
                <div class="step-error">
                  <XCircle class="w-4 h-4 inline-block mr-1" />
                  {step.error_message}
                </div>
              {/if}

              {#if step.node_type === 'condition' && step.output}
                <div class="condition-details">
                  <span class="detail-label">{t('actions.config.fieldToCheck')}:</span>
                  <code>{step.output.field_name}</code>
                  <span class="detail-label">{step.output.operator}</span>
                  <code>{step.output.compare_value}</code>
                  <ArrowRight class="w-4 h-4 inline-block mx-1" />
                  <span class="condition-result" style="color: {step.output.condition_result ? 'var(--ds-success)' : 'var(--ds-text-subtle)'};">
                    {step.output.condition_result ? t('actions.condition.true') : t('actions.condition.false')}
                  </span>
                </div>
              {/if}

              {#if step.node_type === 'set_status' && step.output}
                <div class="status-change-details">
                  <span class="status-badge old">{step.output.old_status_name}</span>
                  <ArrowRight class="w-4 h-4 inline-block mx-2" style="color: var(--ds-text-subtle);" />
                  <span class="status-badge new">{step.output.new_status_name}</span>
                </div>
              {/if}

              {#if step.node_type === 'set_field' && step.output}
                <div class="field-change-details">
                  <span class="field-name">{step.output.field_name}:</span>
                  <code class="old-value">{step.output.old_value ?? '-'}</code>
                  <ArrowRight class="w-4 h-4 inline-block mx-2" style="color: var(--ds-text-subtle);" />
                  <code class="new-value">{step.output.new_value ?? '-'}</code>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</Modal>

<style>
  .timeline {
    position: relative;
  }

  .timeline-item {
    display: flex;
    gap: 1rem;
    padding-bottom: 1.5rem;
  }

  .timeline-item:last-child {
    padding-bottom: 0;
  }

  .timeline-connector {
    display: flex;
    flex-direction: column;
    align-items: center;
    flex-shrink: 0;
  }

  .timeline-marker {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .timeline-line {
    width: 2px;
    flex-grow: 1;
    margin-top: 0.5rem;
    background-color: var(--ds-border);
  }

  .timeline-content {
    flex-grow: 1;
    padding-top: 0.25rem;
  }

  .step-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.25rem;
  }

  .step-type {
    font-weight: 600;
    color: var(--ds-text);
    font-size: 0.875rem;
  }

  .step-status {
    font-size: 0.75rem;
    text-transform: uppercase;
    font-weight: 500;
  }

  .step-description {
    color: var(--ds-text-subtle);
    font-size: 0.875rem;
    margin: 0;
  }

  .step-error {
    margin-top: 0.5rem;
    padding: 0.5rem 0.75rem;
    background-color: var(--ds-error-bg, rgba(239, 68, 68, 0.1));
    border-radius: 0.375rem;
    color: var(--ds-error, #ef4444);
    font-size: 0.8125rem;
  }

  .condition-details,
  .status-change-details,
  .field-change-details {
    margin-top: 0.5rem;
    padding: 0.5rem 0.75rem;
    background-color: var(--ds-surface-sunken, rgba(0, 0, 0, 0.03));
    border-radius: 0.375rem;
    font-size: 0.8125rem;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.25rem;
  }

  .detail-label {
    color: var(--ds-text-subtle);
  }

  code {
    padding: 0.125rem 0.375rem;
    background-color: var(--ds-surface-raised);
    border-radius: 0.25rem;
    font-family: ui-monospace, monospace;
    font-size: 0.75rem;
    color: var(--ds-text);
  }

  .status-badge {
    padding: 0.25rem 0.5rem;
    border-radius: 0.25rem;
    font-size: 0.75rem;
    font-weight: 500;
  }

  .status-badge.old {
    background-color: var(--ds-surface-sunken);
    color: var(--ds-text-subtle);
  }

  .status-badge.new {
    background-color: var(--ds-success-bg, rgba(34, 197, 94, 0.1));
    color: var(--ds-success, #22c55e);
  }

  .field-name {
    font-weight: 500;
    color: var(--ds-text);
  }

  .old-value {
    text-decoration: line-through;
    opacity: 0.7;
  }

  .new-value {
    background-color: var(--ds-success-bg, rgba(34, 197, 94, 0.1));
  }

  .condition-result {
    font-weight: 600;
  }
</style>
