<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast, successToast } from '../stores/toasts.svelte.js';
  import { currentRoute, navigate } from '../router.js';
  import { api } from '../api.js';
  import { confirm } from '../composables/useConfirm.js';
  import { ArrowLeft, Plus, Trash2, ChevronDown, ChevronUp, AlertTriangle } from 'lucide-svelte';
  import Button from '../components/Button.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import WorkflowPicker from '../pickers/WorkflowPicker.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import TransitionOverrideWarning from '../components/TransitionOverrideWarning.svelte';
  import { toHotkeyString } from '../utils/keyboardShortcuts.js';

  // Vocabulary mirrored from internal/models/configuration.go.
  const STEP_MODES = [
    { id: 'sequential', name: 'Sequential' },
    { id: 'parallel', name: 'Parallel' },
  ];

  const QUORUM_MODES = [
    { id: 'any', name: 'Any (1 approver)' },
    { id: 'all', name: 'All approvers' },
    { id: 'count', name: 'Count (N approvers)' },
    { id: 'percent', name: 'Percent (% of pool)' },
  ];

  const REJECTION_POLICIES = [
    { id: 'any_rejection_fails', name: 'Any rejection fails' },
    { id: 'requires_quorum_to_fail', name: 'Requires quorum to fail' },
  ];

  const ON_LEAVE = [
    { id: 'use_substitute', name: 'Use substitute (default)' },
    { id: 'skip', name: 'Skip approver (escalate if pool empty)' },
    { id: 'keep', name: 'Keep approver (must respond)' },
  ];

  const ESCALATION_ACTIONS = [
    { id: '', name: 'Disabled' },
    { id: 'reassign', name: 'Reassign to target' },
    { id: 'skip_step', name: 'Skip the step (treat as approved)' },
    { id: 'auto_reject', name: 'Auto-reject the request' },
  ];

  const APPROVER_SOURCES = [
    { id: 'creator', name: 'Creator' },
    { id: 'assignee', name: 'Assignee' },
    { id: 'current_user', name: 'Person who triggered the approval' },
    { id: 'user', name: 'Specific user' },
    { id: 'regular_field', name: 'Regular field (assignee_id, creator_id, …)' },
    { id: 'custom_field', name: 'Custom field (user-typed)' },
    { id: 'role', name: 'Workspace role' },
    { id: 'group', name: 'Group' },
  ];

  const REGULAR_FIELD_OPTIONS = [
    { id: 'assignee_id', name: 'assignee_id' },
    { id: 'creator_id', name: 'creator_id' },
    { id: 'reporter_id', name: 'reporter_id' },
  ];

  let approvalSetId = $state(null);
  let isNewMode = $state(false);
  let loading = $state(true);
  let saving = $state(false);

  let workflows = $state([]);
  let statusesAll = $state([]);
  let groups = $state([]);
  let users = $state([]);
  let roles = $state([]);
  let userCustomFields = $state([]);
  let transitions = $state([]);

  let formData = $state({
    name: '',
    description: '',
    workflow_id: null,
    set_statuses: [],
  });
  let originalFormData = $state('{}');
  const hasUnsavedChanges = $derived(JSON.stringify(formData) !== originalFormData);

  let expandedStatuses = $state(new Set());
  let expandedSteps = $state(new Set()); // keys: "<setStatusIdx>:<stepIdx>"

  $effect(() => {
    const id = $currentRoute.params?.id;
    if (id === 'new') {
      isNewMode = true;
      approvalSetId = null;
      resetForm();
      loading = false;
    } else if (id) {
      const newId = parseInt(id);
      if (newId && newId !== approvalSetId) {
        isNewMode = false;
        approvalSetId = newId;
        loadData();
      }
    }
  });

  onMount(loadReferenceData);

  function resetForm() {
    const data = { name: '', description: '', workflow_id: null, set_statuses: [] };
    formData = data;
    originalFormData = JSON.stringify(data);
    transitions = [];
    expandedStatuses = new Set();
    expandedSteps = new Set();
  }

  async function loadReferenceData() {
    try {
      const [wf, st, gr, us, rs, cf] = await Promise.all([
        api.workflows.getAll().catch(() => []),
        api.statuses.getAll().catch(() => []),
        api.groups.getAll().catch(() => []),
        api.getUsers().catch(() => []),
        api.workspaceRoles.getAll().catch(() => []),
        api.customFields.getAll().catch(() => []),
      ]);
      workflows = wf || [];
      statusesAll = st || [];
      groups = gr || [];
      users = (Array.isArray(us) ? us : us?.users) || [];
      roles = Array.isArray(rs) ? rs : (rs?.roles ?? []);
      userCustomFields = (cf?.data || cf || [])
        .filter(f => f.field_type === 'user' || f.field_type === 'multi_user')
        .map(f => ({ id: f.id, name: f.name }));
    } catch (error) {
      console.error('Failed to load reference data:', error);
    }
  }

  async function loadData() {
    if (!approvalSetId) {
      loading = false;
      return;
    }
    try {
      loading = true;
      const data = await api.approvalSets.get(approvalSetId);
      formData = {
        name: data.name || '',
        description: data.description || '',
        workflow_id: data.workflow_id || null,
        set_statuses: (data.set_statuses || []).map(ass => ({
          id: ass.id,
          status_id: ass.status_id,
          approve_transition_id: ass.approve_transition_id,
          deny_transition_id: ass.deny_transition_id,
          step_mode: ass.step_mode || 'sequential',
          steps: (ass.steps || []).map((s, i) => normalizeStep(s, i)),
        })),
      };
      originalFormData = JSON.stringify(formData);
      if (data.workflow_id) await loadTransitions(data.workflow_id);
    } catch (error) {
      console.error('Failed to load approval set:', error);
      errorToast(t('dialogs.alerts.failedToLoad', { error: error.message || JSON.stringify(error) }));
    } finally {
      loading = false;
    }
  }

  async function loadTransitions(workflowId) {
    if (!workflowId) {
      transitions = [];
      return;
    }
    try {
      transitions = (await api.workflows.getTransitions(workflowId)) || [];
    } catch (error) {
      console.error('Failed to load transitions:', error);
      transitions = [];
    }
  }

  function normalizeStep(s, displayOrder) {
    return {
      id: s.id,
      display_order: s.display_order ?? displayOrder ?? 0,
      name: s.name || '',
      quorum_mode: s.quorum_mode || 'any',
      quorum_count: s.quorum_count ?? null,
      quorum_percent: s.quorum_percent ?? null,
      rejection_policy: s.rejection_policy || 'any_rejection_fails',
      approver_source: s.approver_source || 'assignee',
      approver_field_identifier: s.approver_field_identifier || '',
      approver_field_id: s.approver_field_id ?? null,
      approver_role_id: s.approver_role_id ?? null,
      approver_group_id: s.approver_group_id ?? null,
      approver_user_id: s.approver_user_id ?? null,
      allow_self_approval: !!s.allow_self_approval,
      on_leave_strategy: s.on_leave_strategy || 'use_substitute',
      escalation_after_hours: s.escalation_after_hours ?? null,
      escalation_action: s.escalation_action || '',
      escalation_target_source: s.escalation_target_source || '',
      escalation_target_field_identifier: s.escalation_target_field_identifier || '',
      escalation_target_field_id: s.escalation_target_field_id ?? null,
      escalation_target_role_id: s.escalation_target_role_id ?? null,
      escalation_target_group_id: s.escalation_target_group_id ?? null,
      escalation_target_user_id: s.escalation_target_user_id ?? null,
      max_escalations: s.max_escalations ?? null,
    };
  }

  async function handleWorkflowChange(workflow) {
    const newWorkflowId = workflow?.id || null;
    if (formData.workflow_id && newWorkflowId !== formData.workflow_id && formData.set_statuses.length > 0) {
      const ok = await confirm({
        title: t('approvalSets.workflowLocked'),
        message: 'Changing the workflow will remove all configured statuses and steps. Continue?',
        confirmText: t('common.confirm'),
        cancelText: t('common.cancel'),
        variant: 'warning',
      });
      if (!ok) return;
    }
    formData.workflow_id = newWorkflowId;
    formData.set_statuses = [];
    expandedStatuses = new Set();
    expandedSteps = new Set();
    if (newWorkflowId) await loadTransitions(newWorkflowId);
    else transitions = [];
  }

  function statusName(id) {
    return statusesAll.find(s => s.id === id)?.name ?? '—';
  }

  function transitionLabel(tr) {
    if (!tr) return '—';
    const from = tr.from_status_id ? statusName(tr.from_status_id) : '(initial)';
    return `${from} → ${statusName(tr.to_status_id)}`;
  }

  // Transitions originating from a given status — usable as approve/deny targets.
  function transitionsFromStatus(statusId) {
    return transitions.filter(tr => tr.from_status_id === statusId);
  }

  function addSetStatus() {
    formData.set_statuses = [
      ...formData.set_statuses,
      {
        status_id: null,
        approve_transition_id: null,
        deny_transition_id: null,
        step_mode: 'sequential',
        steps: [newStep(0)],
      },
    ];
    expandedStatuses = new Set([...expandedStatuses, formData.set_statuses.length - 1]);
  }

  function removeSetStatus(idx) {
    formData.set_statuses = formData.set_statuses.filter((_, i) => i !== idx);
    const next = new Set(); for (const e of expandedStatuses) if (e !== idx) next.add(e > idx ? e - 1 : e);
    expandedStatuses = next;
  }

  function newStep(displayOrder) {
    return normalizeStep({ display_order: displayOrder, name: 'Approval', approver_source: 'assignee' }, displayOrder);
  }

  // Keep escalation fields coherent: switching to "No escalation" should
  // wipe the timer and target so saved data matches what the user sees.
  // Switching to an active action with no timer set seeds a sensible default
  // so the user can't save a "reassign in 0 hours" misconfiguration.
  function syncEscalationFields(step) {
    if (!step.escalation_action) {
      step.escalation_after_hours = null;
      step.escalation_target_source = '';
      step.escalation_target_field_identifier = '';
      step.escalation_target_field_id = null;
      step.escalation_target_role_id = null;
      step.escalation_target_group_id = null;
      step.escalation_target_user_id = null;
      step.max_escalations = null;
    } else if (!step.escalation_after_hours || step.escalation_after_hours < 1) {
      step.escalation_after_hours = 24;
    }
  }

  function addStep(setIdx) {
    const s = formData.set_statuses[setIdx];
    s.steps = [...s.steps, newStep(s.steps.length)];
    formData.set_statuses[setIdx] = s;
  }

  function removeStep(setIdx, stepIdx) {
    const s = formData.set_statuses[setIdx];
    s.steps = s.steps.filter((_, i) => i !== stepIdx).map((step, i) => ({ ...step, display_order: i }));
    formData.set_statuses[setIdx] = s;
  }

  function toggleStatusExpand(idx) {
    const next = new Set(expandedStatuses);
    if (next.has(idx)) next.delete(idx); else next.add(idx);
    expandedStatuses = next;
  }

  function stepKey(setIdx, stepIdx) { return `${setIdx}:${stepIdx}`; }
  function toggleStepExpand(setIdx, stepIdx) {
    const k = stepKey(setIdx, stepIdx);
    const next = new Set(expandedSteps);
    if (next.has(k)) next.delete(k); else next.add(k);
    expandedSteps = next;
  }

  function validate() {
    if (!formData.name.trim()) return t('approvalSets.nameRequired');
    if (!formData.workflow_id) return t('approvalSets.workflowRequired');
    for (const [i, ass] of formData.set_statuses.entries()) {
      if (!ass.status_id) return `Approval status #${i + 1}: select a status.`;
      if (!ass.approve_transition_id || !ass.deny_transition_id)
        return `Approval status #${i + 1}: select approve and deny transitions.`;
      if (ass.approve_transition_id === ass.deny_transition_id)
        return t('approvalSets.transitionsMustDiffer');
      const allowedIDs = new Set(transitionsFromStatus(ass.status_id).map(tr => tr.id));
      if (!allowedIDs.has(ass.approve_transition_id) || !allowedIDs.has(ass.deny_transition_id))
        return t('approvalSets.transitionsMustExitStatus');
      if (ass.steps.length === 0)
        return `Approval status #${i + 1}: at least one step is required.`;
      for (const [j, step] of ass.steps.entries()) {
        if (!step.name.trim()) return `Step #${j + 1} of approval status #${i + 1}: ${t('approvalSets.stepNameRequired')}`;
        if (step.quorum_mode === 'count' && (!step.quorum_count || step.quorum_count < 1))
          return `Step #${j + 1}: count quorum requires count >= 1.`;
        if (step.quorum_mode === 'percent' && (!step.quorum_percent || step.quorum_percent < 1 || step.quorum_percent > 100))
          return `Step #${j + 1}: percent quorum must be 1..100.`;
      }
    }
    return null;
  }

  async function save() {
    const err = validate();
    if (err) { errorToast(err); return; }

    saving = true;
    try {
      const payload = {
        name: formData.name,
        description: formData.description,
        workflow_id: formData.workflow_id,
        set_statuses: formData.set_statuses.map(ass => ({
          status_id: ass.status_id,
          approve_transition_id: ass.approve_transition_id,
          deny_transition_id: ass.deny_transition_id,
          step_mode: ass.step_mode,
          steps: ass.steps.map((s, i) => ({ ...s, display_order: i })),
        })),
      };

      if (isNewMode) {
        const created = await api.approvalSets.create(payload);
        successToast('Approval set created');
        navigate(`/admin/approval-sets/${created.id}`);
      } else {
        await api.approvalSets.update(approvalSetId, payload);
        await loadData();
        successToast('Approval set saved');
      }
    } catch (error) {
      console.error('Failed to save approval set:', error);
      errorToast(error.message || JSON.stringify(error));
    } finally {
      saving = false;
    }
  }

  function back() {
    navigate('/admin/approval-sets');
  }
</script>

<div class="p-6 max-w-5xl mx-auto">
  <div class="mb-4">
    <Button variant="ghost" icon={ArrowLeft} onclick={back}>
      {t('approvalSets.backToList')}
    </Button>
  </div>

  {#if loading}
    <div class="text-center py-12" style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
  {:else}
    <div class="mb-6">
      <h1 class="text-2xl font-semibold mb-1" style="color: var(--ds-text);">
        {isNewMode ? t('approvalSets.new') : formData.name || '…'}
      </h1>
      <p class="text-sm" style="color: var(--ds-text-subtle);">{t('approvalSets.detailDesc')}</p>
    </div>

    <!-- General -->
    <section class="mb-8">
      <h2 class="text-lg font-medium mb-3" style="color: var(--ds-text);">{t('approvalSets.general')}</h2>
      <div class="space-y-4">
        <div>
          <Label required>{t('approvalSets.name')}</Label>
          <input
            type="text"
            class="w-full px-3 py-2 border rounded text-sm"
            style="border-color: var(--ds-border); background: var(--ds-surface-raised); color: var(--ds-text);"
            placeholder={t('approvalSets.namePlaceholder')}
            bind:value={formData.name}
            data-testid="approval-set-name"
          />
        </div>
        <div>
          <Label>{t('approvalSets.description')}</Label>
          <Textarea
            placeholder={t('approvalSets.descriptionPlaceholder')}
            bind:value={formData.description}
            rows={2}
          />
        </div>
        <div>
          <Label required>{t('approvalSets.workflow')}</Label>
          {#if isNewMode}
            <WorkflowPicker
              items={workflows}
              value={formData.workflow_id}
              onSelect={handleWorkflowChange}
              dataTestid="approval-set-workflow"
            />
          {:else}
            <div class="text-sm py-2" style="color: var(--ds-text-subtle);">
              {workflows.find(w => w.id === formData.workflow_id)?.name ?? '—'}
              <span class="ml-2 text-xs">({t('approvalSets.workflowLocked')})</span>
            </div>
          {/if}
        </div>
      </div>
    </section>

    <!-- Approval statuses -->
    <section class="mb-8">
      <div class="flex items-center justify-between mb-2">
        <h2 class="text-lg font-medium" style="color: var(--ds-text);">{t('approvalSets.setStatuses')}</h2>
        <Button
          variant="default"
          size="small"
          icon={Plus}
          onclick={addSetStatus}
          disabled={!formData.workflow_id}
          keyboardHint="S"
          hotkeyConfig={{ key: toHotkeyString('approvalSets', 'addStatus'), guard: () => !!formData.workflow_id }}
          dataTestid="approval-set-add-status"
        >
          {t('approvalSets.addStatus')}
        </Button>
      </div>
      <p class="text-xs mb-4" style="color: var(--ds-text-subtle);">{t('approvalSets.setStatusesDesc')}</p>

      {#if formData.set_statuses.length === 0}
        <div class="border-2 border-dashed rounded p-6 text-center text-sm"
             style="border-color: var(--ds-border); color: var(--ds-text-subtle);">
          No approval statuses yet.
        </div>
      {/if}

      <div class="space-y-3">
        {#each formData.set_statuses as ass, idx (idx)}
          {@const fromStatusTransitions = transitionsFromStatus(ass.status_id)}
          {@const isExpanded = expandedStatuses.has(idx)}
          <div class="border rounded" style="border-color: var(--ds-border); background: var(--ds-surface-raised);">
            <button type="button"
                    class="w-full flex items-center justify-between p-4 text-left"
                    onclick={() => toggleStatusExpand(idx)}>
              <div class="flex items-center gap-3 min-w-0">
                {#if isExpanded}<ChevronUp class="w-4 h-4" />{:else}<ChevronDown class="w-4 h-4" />{/if}
                <div class="font-medium" style="color: var(--ds-text);">
                  {ass.status_id ? statusName(ass.status_id) : 'Unconfigured status'}
                </div>
                <div class="text-xs" style="color: var(--ds-text-subtle);">
                  {ass.steps.length} step(s) · {ass.step_mode}
                </div>
              </div>
              <Button
                variant="ghost"
                size="small"
                icon={Trash2}
                onclick={(e) => { e.stopPropagation(); removeSetStatus(idx); }}
              />
            </button>

            {#if isExpanded}
              <div class="px-4 pb-4 space-y-4 border-t pt-4" style="border-color: var(--ds-border);">
                <div>
                  <Label required>{t('approvalSets.status')}</Label>
                  <BasePicker
                    items={statusesAll}
                    value={ass.status_id}
                    placeholder={t('approvalSets.selectStatus')}
                    getValue={(s) => s.id}
                    getLabel={(s) => s.name}
                    onSelect={(s) => {
                      ass.status_id = s?.id || null;
                      ass.approve_transition_id = null;
                      ass.deny_transition_id = null;
                    }}
                  />
                </div>

                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <Label required>{t('approvalSets.approveTransition')}</Label>
                    <BasePicker
                      items={fromStatusTransitions}
                      value={ass.approve_transition_id}
                      disabled={!ass.status_id}
                      placeholder={t('approvalSets.selectTransition')}
                      getValue={(tr) => tr.id}
                      getLabel={(tr) => transitionLabel(tr)}
                      onSelect={(tr) => ass.approve_transition_id = tr?.id || null}
                    />
                  </div>
                  <div>
                    <Label required>{t('approvalSets.denyTransition')}</Label>
                    <BasePicker
                      items={fromStatusTransitions}
                      value={ass.deny_transition_id}
                      disabled={!ass.status_id}
                      placeholder={t('approvalSets.selectTransition')}
                      getValue={(tr) => tr.id}
                      getLabel={(tr) => transitionLabel(tr)}
                      onSelect={(tr) => ass.deny_transition_id = tr?.id || null}
                    />
                  </div>
                </div>

                <!-- Override warnings: warn if the configured approve/deny transitions
                     already have conditions attached. The same warning shows on the
                     condition-set side too (perspective='condition'). -->
                {#if ass.approve_transition_id}
                  <TransitionOverrideWarning
                    transitionId={ass.approve_transition_id}
                    perspective="approval"
                    label={t('approvalSets.approveTransition')}
                  />
                {/if}
                {#if ass.deny_transition_id && ass.deny_transition_id !== ass.approve_transition_id}
                  <TransitionOverrideWarning
                    transitionId={ass.deny_transition_id}
                    perspective="approval"
                    label={t('approvalSets.denyTransition')}
                  />
                {/if}

                <div>
                  <Label>{t('approvalSets.stepMode')}</Label>
                  <div class="flex gap-3">
                    {#each STEP_MODES as mode}
                      <label class="flex items-start gap-2 text-sm cursor-pointer p-2 rounded border flex-1"
                             style="border-color: {ass.step_mode === mode.id ? 'var(--ds-border-bold)' : 'var(--ds-border)'};">
                        <input type="radio" bind:group={ass.step_mode} value={mode.id} class="mt-0.5" />
                        <div>
                          <div class="font-medium" style="color: var(--ds-text);">{mode.name}</div>
                          <div class="text-xs" style="color: var(--ds-text-subtle);">
                            {mode.id === 'sequential' ? t('approvalSets.stepModeSequentialDesc') : t('approvalSets.stepModeParallelDesc')}
                          </div>
                        </div>
                      </label>
                    {/each}
                  </div>
                </div>

                <!-- Steps -->
                <div>
                  <div class="flex items-center justify-between mb-2">
                    <Label>{t('approvalSets.steps')}</Label>
                    <Button variant="ghost" size="small" icon={Plus} onclick={() => addStep(idx)}>
                      {t('approvalSets.addStep')}
                    </Button>
                  </div>
                  <div class="space-y-2">
                    {#each ass.steps as step, sIdx (sIdx)}
                      {@const sk = stepKey(idx, sIdx)}
                      {@const stepExpanded = expandedSteps.has(sk)}
                      <div class="border rounded" style="border-color: var(--ds-border);">
                        <button type="button" class="w-full flex items-center justify-between p-3 text-left"
                                onclick={() => toggleStepExpand(idx, sIdx)}>
                          <div class="flex items-center gap-2 min-w-0">
                            {#if stepExpanded}<ChevronUp class="w-4 h-4" />{:else}<ChevronDown class="w-4 h-4" />{/if}
                            <span class="font-medium" style="color: var(--ds-text);">
                              {sIdx + 1}. {step.name || '(unnamed)'}
                            </span>
                            <span class="text-xs" style="color: var(--ds-text-subtle);">
                              {step.approver_source} · {step.quorum_mode}
                            </span>
                          </div>
                          <Button variant="ghost" size="small" icon={Trash2}
                                  onclick={(e) => { e.stopPropagation(); removeStep(idx, sIdx); }} />
                        </button>

                        {#if stepExpanded}
                          <div class="p-3 border-t space-y-3" style="border-color: var(--ds-border);">
                            <div>
                              <Label required>{t('approvalSets.stepName')}</Label>
                              <input type="text" class="w-full px-3 py-2 border rounded text-sm"
                                     style="border-color: var(--ds-border); background: var(--ds-surface);"
                                     placeholder={t('approvalSets.stepNamePlaceholder')}
                                     bind:value={step.name} />
                            </div>

                            <div class="grid grid-cols-2 gap-3">
                              <div>
                                <Label>{t('approvalSets.quorum')}</Label>
                                <select class="w-full px-3 py-2 border rounded text-sm"
                                        style="border-color: var(--ds-border); background: var(--ds-surface);"
                                        bind:value={step.quorum_mode}>
                                  {#each QUORUM_MODES as q}
                                    <option value={q.id}>{q.name}</option>
                                  {/each}
                                </select>
                                {#if step.quorum_mode === 'count'}
                                  <input type="number" min="1" class="mt-2 w-full px-3 py-2 border rounded text-sm"
                                         placeholder={t('approvalSets.quorumCountValue')}
                                         bind:value={step.quorum_count} />
                                {:else if step.quorum_mode === 'percent'}
                                  <input type="number" min="1" max="100" class="mt-2 w-full px-3 py-2 border rounded text-sm"
                                         placeholder={t('approvalSets.quorumPercentValue')}
                                         bind:value={step.quorum_percent} />
                                {/if}
                              </div>
                              <div>
                                <Label>{t('approvalSets.rejectionPolicy')}</Label>
                                <select class="w-full px-3 py-2 border rounded text-sm"
                                        style="border-color: var(--ds-border); background: var(--ds-surface);"
                                        bind:value={step.rejection_policy}>
                                  {#each REJECTION_POLICIES as r}
                                    <option value={r.id}>{r.name}</option>
                                  {/each}
                                </select>
                              </div>
                            </div>

                            <div class="grid grid-cols-2 gap-3">
                              <div>
                                <Label>{t('approvalSets.approverSource')}</Label>
                                <select class="w-full px-3 py-2 border rounded text-sm"
                                        style="border-color: var(--ds-border); background: var(--ds-surface);"
                                        bind:value={step.approver_source}>
                                  {#each APPROVER_SOURCES as src}
                                    <option value={src.id}>{src.name}</option>
                                  {/each}
                                </select>
                              </div>
                              <div>
                                {#if step.approver_source === 'regular_field'}
                                  <Label>{t('approvalSets.approverFieldIdentifier')}</Label>
                                  <select class="w-full px-3 py-2 border rounded text-sm"
                                          style="border-color: var(--ds-border); background: var(--ds-surface);"
                                          bind:value={step.approver_field_identifier}>
                                    <option value="">…</option>
                                    {#each REGULAR_FIELD_OPTIONS as f}
                                      <option value={f.id}>{f.name}</option>
                                    {/each}
                                  </select>
                                {:else if step.approver_source === 'custom_field'}
                                  <Label>{t('approvalSets.approverFieldId')}</Label>
                                  <select class="w-full px-3 py-2 border rounded text-sm"
                                          style="border-color: var(--ds-border); background: var(--ds-surface);"
                                          bind:value={step.approver_field_id}>
                                    <option value={null}>…</option>
                                    {#each userCustomFields as f}
                                      <option value={f.id}>{f.name}</option>
                                    {/each}
                                  </select>
                                {:else if step.approver_source === 'role'}
                                  <Label>{t('approvalSets.approverRole')}</Label>
                                  <select class="w-full px-3 py-2 border rounded text-sm"
                                          style="border-color: var(--ds-border); background: var(--ds-surface);"
                                          bind:value={step.approver_role_id}>
                                    <option value={null}>…</option>
                                    {#each roles as r}
                                      <option value={r.id}>{r.name}</option>
                                    {/each}
                                  </select>
                                {:else if step.approver_source === 'group'}
                                  <Label>{t('approvalSets.approverGroup')}</Label>
                                  <select class="w-full px-3 py-2 border rounded text-sm"
                                          style="border-color: var(--ds-border); background: var(--ds-surface);"
                                          bind:value={step.approver_group_id}>
                                    <option value={null}>…</option>
                                    {#each groups as g}
                                      <option value={g.id}>{g.name}</option>
                                    {/each}
                                  </select>
                                {:else if step.approver_source === 'user'}
                                  <Label>{t('approvalSets.approverUser')}</Label>
                                  <select class="w-full px-3 py-2 border rounded text-sm"
                                          style="border-color: var(--ds-border); background: var(--ds-surface);"
                                          bind:value={step.approver_user_id}>
                                    <option value={null}>…</option>
                                    {#each users as u}
                                      <option value={u.id}>{u.username || u.email}</option>
                                    {/each}
                                  </select>
                                {/if}
                              </div>
                            </div>

                            <label class="flex items-center gap-2 text-sm cursor-pointer">
                              <input type="checkbox" bind:checked={step.allow_self_approval} />
                              <span>{t('approvalSets.allowSelfApproval')}</span>
                            </label>

                            <!-- Fallback chain: leave handling + timeout escalation. The
                                 two rules are presented as a single panel because they
                                 chain together — when leave handling produces an empty
                                 pool, the escalation policy fires immediately instead
                                 of waiting for the timer. -->
                            <div class="border rounded-md p-3" style="border-color: var(--ds-border); background: var(--ds-background-neutral);">
                              <div class="mb-3">
                                <div class="text-sm font-semibold" style="color: var(--ds-text);">
                                  {t('approvalSets.fallbackTitle')}
                                </div>
                                <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                                  {t('approvalSets.fallbackDesc')}
                                </p>
                              </div>

                              <!-- Step 1: on-leave rule -->
                              <div class="mb-3">
                                <div class="text-xs font-medium uppercase mb-2" style="color: var(--ds-text-subtle); letter-spacing: 0.04em;">
                                  1. {t('approvalSets.onLeave')}
                                </div>
                                <div class="space-y-1.5">
                                  {#each ON_LEAVE as ol}
                                    <label class="flex items-start gap-2 text-sm cursor-pointer p-2 rounded border"
                                           style="border-color: {step.on_leave_strategy === ol.id ? 'var(--ds-border-bold)' : 'var(--ds-border)'}; background: var(--ds-surface);">
                                      <input type="radio" bind:group={step.on_leave_strategy} value={ol.id} class="mt-0.5" />
                                      <div class="flex-1 min-w-0">
                                        <div class="font-medium" style="color: var(--ds-text);">{ol.name}</div>
                                        {#if ol.id === 'use_substitute'}
                                          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('approvalSets.onLeaveUseSubstituteDesc')}</div>
                                        {:else if ol.id === 'skip'}
                                          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('approvalSets.onLeaveSkipDesc')}</div>
                                        {:else if ol.id === 'keep'}
                                          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('approvalSets.onLeaveKeepDesc')}</div>
                                        {/if}
                                      </div>
                                    </label>
                                  {/each}
                                </div>
                              </div>

                              <!-- Step 2: escalation policy -->
                              <div>
                                <div class="text-xs font-medium uppercase mb-2" style="color: var(--ds-text-subtle); letter-spacing: 0.04em;">
                                  2. {t('approvalSets.escalation')}
                                </div>
                                <div class="space-y-1.5">
                                  {#each ESCALATION_ACTIONS as a}
                                    {@const isSelected = (step.escalation_action || '') === a.id}
                                    <label class="flex items-start gap-2 text-sm cursor-pointer p-2 rounded border"
                                           style="border-color: {isSelected ? 'var(--ds-border-bold)' : 'var(--ds-border)'}; background: var(--ds-surface);">
                                      <input type="radio" bind:group={step.escalation_action} value={a.id}
                                             onchange={() => syncEscalationFields(step)} class="mt-0.5" />
                                      <div class="flex-1 min-w-0">
                                        <div class="font-medium" style="color: var(--ds-text);">{a.name}</div>
                                        {#if a.id === ''}
                                          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('approvalSets.escalationDisabledDesc')}</div>
                                        {:else if a.id === 'reassign'}
                                          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('approvalSets.escalationActionReassignDesc')}</div>
                                        {:else if a.id === 'skip_step'}
                                          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('approvalSets.escalationActionSkipStepDesc')}</div>
                                        {:else if a.id === 'auto_reject'}
                                          <div class="text-xs" style="color: var(--ds-text-subtle);">{t('approvalSets.escalationActionAutoRejectDesc')}</div>
                                        {/if}
                                      </div>
                                    </label>
                                  {/each}
                                </div>

                                <!-- Per-action sub-config -->
                                {#if step.escalation_action && step.escalation_action !== ''}
                                  <div class="mt-3 pl-4 border-l-2 space-y-2" style="border-color: var(--ds-border-bold);">
                                    <div>
                                      <Label>{t('approvalSets.escalationAfterHours')}</Label>
                                      <input type="number" min="1" class="w-full px-3 py-2 border rounded text-sm"
                                             style="border-color: var(--ds-border); background: var(--ds-surface);"
                                             placeholder="24"
                                             bind:value={step.escalation_after_hours} />
                                    </div>
                                    {#if step.escalation_action === 'reassign'}
                                      <div class="grid grid-cols-2 gap-3">
                                        <div>
                                          <Label>{t('approvalSets.escalationTarget')}</Label>
                                          <select class="w-full px-3 py-2 border rounded text-sm"
                                                  style="border-color: var(--ds-border); background: var(--ds-surface);"
                                                  bind:value={step.escalation_target_source}>
                                            <option value="">…</option>
                                            {#each APPROVER_SOURCES as src}
                                              <option value={src.id}>{src.name}</option>
                                            {/each}
                                          </select>
                                        </div>
                                        <div>
                                          <Label>{t('approvalSets.maxEscalations')}</Label>
                                          <input type="number" min="1" class="w-full px-3 py-2 border rounded text-sm"
                                                 style="border-color: var(--ds-border); background: var(--ds-surface);"
                                                 placeholder="∞"
                                                 bind:value={step.max_escalations} />
                                        </div>
                                      </div>
                                      <p class="text-xs" style="color: var(--ds-text-subtle);">
                                        {t('approvalSets.chainedNote')}
                                      </p>
                                    {/if}
                                  </div>
                                {/if}
                              </div>

                              <!-- Connector note tying the two rules together -->
                              <div class="mt-3 p-2 rounded text-xs flex items-start gap-2"
                                   style="background: var(--ds-background-information, #eff6ff); color: var(--ds-text-information, #1e40af);">
                                <span class="mt-0.5">↳</span>
                                <span>{t('approvalSets.fallbackConnector')}</span>
                              </div>
                            </div>
                          </div>
                        {/if}
                      </div>
                    {/each}
                  </div>
                </div>
              </div>
            {/if}
          </div>
        {/each}
      </div>
    </section>

    <div class="flex justify-end gap-3">
      <Button variant="ghost" onclick={back}>{t('common.cancel')}</Button>
      <Button
        variant="primary"
        onclick={save}
        disabled={saving || !hasUnsavedChanges}
        hotkeyConfig={{ key: toHotkeyString('approvalSets', 'save'), guard: () => !saving && hasUnsavedChanges }}
        dataTestid="approval-set-save"
      >
        {saving ? t('common.saving') : t('common.save')}
      </Button>
    </div>
  {/if}
</div>
