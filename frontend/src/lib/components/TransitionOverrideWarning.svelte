<script>
  // Renders a yellow warning banner when conditions and approvals overlap on
  // the same workflow_transitions row. The component fetches the per-transition
  // governance whenever `transitionId` changes and decides which side of the
  // story to tell based on `perspective`:
  //
  //   - 'approval': editing an approval set; warn if `conditions` is non-empty
  //                 for the configured approve/deny transition. Wording focuses
  //                 on "the conditions you've already set will be silently
  //                 bypassed when this approval finalises."
  //   - 'condition': editing a condition set; warn if `approval_drivers` is
  //                  non-empty for the transition the user is attaching
  //                  conditions to. Wording focuses on "this transition is
  //                  driven by an approval — your conditions only apply to
  //                  direct user attempts (which the approval will block)."
  //
  // The component is designed to be visible in both places so the override is
  // never silent — admins see it whether they configured the condition first
  // and the approval second, or the other way around.

  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { api } from '../api.js';
  import { AlertTriangle } from 'lucide-svelte';

  let {
    transitionId = null,
    perspective = 'approval', // 'approval' | 'condition'
    label = '',                // optional context (e.g. "approve transition")
  } = $props();

  let governance = $state(null);
  let loading = $state(false);

  $effect(() => {
    const id = transitionId;
    if (!id) {
      governance = null;
      return;
    }
    loading = true;
    api.transitions
      .governance(id)
      .then((g) => {
        governance = g;
      })
      .catch(() => {
        governance = null;
      })
      .finally(() => {
        loading = false;
      });
  });

  const conditionTouches = $derived(governance?.conditions ?? []);
  const approvalDrivers = $derived(governance?.approval_drivers ?? []);

  // From the approval-set editor: warn iff conditions exist on the transition
  // we just configured as approve/deny.
  // From the condition-set editor: warn iff approval drivers exist on the
  // transition we're adding conditions to.
  const shouldWarn = $derived(
    perspective === 'approval'
      ? conditionTouches.length > 0
      : approvalDrivers.length > 0
  );
</script>

{#if shouldWarn}
  <div
    class="flex items-start gap-3 p-3 rounded border"
    style="border-color: var(--ds-border-warning, #d97706); background: var(--ds-background-warning, #fffbeb); color: var(--ds-text-warning, #92400e);"
    data-testid="override-warning"
    data-perspective={perspective}
  >
    <AlertTriangle class="w-5 h-5 flex-shrink-0 mt-0.5" />
    <div class="flex-1 min-w-0 text-sm">
      <div class="font-semibold mb-1">
        {perspective === 'approval'
          ? t('approvalSets.overrideWarningTitle')
          : t('approvalSets.overrideWarningCondReverseTitle')}
      </div>
      <p class="mb-2">
        {perspective === 'approval'
          ? t('approvalSets.overrideWarningBody')
          : t('approvalSets.overrideWarningCondReverseBody')}
      </p>
      {#if perspective === 'approval' && conditionTouches.length > 0}
        <ul class="list-disc list-inside space-y-1">
          {#each conditionTouches as ct (ct.condition_set_id)}
            <li>
              <span class="font-medium">{ct.condition_set_name}</span>
              <span style="opacity: 0.8;"> · {ct.condition_count} rule{ct.condition_count === 1 ? '' : 's'}</span>
            </li>
          {/each}
        </ul>
      {/if}
      {#if perspective === 'condition' && approvalDrivers.length > 0}
        <ul class="list-disc list-inside space-y-1">
          {#each approvalDrivers as ad (ad.approval_set_status_id + '-' + ad.role)}
            <li>
              <span class="font-medium">{ad.approval_set_name}</span>
              <span style="opacity: 0.8;"> · drives this transition as {ad.role === 'approve_transition_id' ? 'approve' : 'deny'}</span>
            </li>
          {/each}
        </ul>
      {/if}
      {#if label}
        <div class="text-xs mt-2 opacity-75">({label})</div>
      {/if}
    </div>
  </div>
{/if}
