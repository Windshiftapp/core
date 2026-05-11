<script>
  import { onMount } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import { currentRoute, navigate } from '../router.js';
  import { api } from '../api.js';
  import { confirm } from '../composables/useConfirm.js';
  import { ArrowLeft, Plus, Trash2, ChevronDown, ChevronUp, HelpCircle } from '@lucide/svelte';
  import Button from '../components/Button.svelte';
  import Textarea from '../components/Textarea.svelte';
  import Label from '../components/Label.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import Modal from '../dialogs/Modal.svelte';
  import WorkflowPicker from '../pickers/WorkflowPicker.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import RolePicker from '../pickers/RolePicker.svelte';
  import DescriptionText from '../components/DescriptionText.svelte';
  import TransitionOverrideWarning from '../components/TransitionOverrideWarning.svelte';

  let conditionSetId = $state(null);
  let isNewMode = $state(false);
  let loading = $state(true);
  let saving = $state(false);

  // Reference data
  let workflows = $state([]);
  let groups = $state([]);
  let transitions = $state([]);
  let userFieldOptions = $state([]);

  const userSourceOptions = [
    { id: 'current_user', name: 'Current User' },
    { id: 'creator', name: 'Creator' },
    { id: 'assignee', name: 'Assignee' },
    { id: 'custom_field', name: 'Custom Field' },
  ];

  // Form state
  let formData = $state({
    name: '',
    description: '',
    workflow_id: null,
    transition_conditions: []
  });

  let originalFormData = $state('{}');
  const hasUnsavedChanges = $derived(JSON.stringify(formData) !== originalFormData);

  // Expanded transition editors
  let expandedTransitions = $state(new Set());

  // Script reference modal
  let showScriptHelp = $state(false);

  const scriptRefGlobals = [
    { variable: 'user_id', type: 'number', description: "Current user's ID" },
    { variable: 'item', type: 'object', description: 'The item being transitioned' },
  ];

  const scriptRefItemProps = [
    { variable: 'item.id', type: 'number', description: 'Item ID' },
    { variable: 'item.workspace_id', type: 'number', description: 'Workspace ID' },
    { variable: 'item.status_id', type: 'number', description: 'Current status ID' },
    { variable: 'item.item_type_id', type: 'number', description: 'Item type ID' },
    { variable: 'item.creator_id', type: 'number', description: "Creator's user ID" },
    { variable: 'item.assignee_id', type: 'number', description: "Assignee's user ID" },
    { variable: 'item.title', type: 'string', description: 'Item title' },
    { variable: 'item.custom_fields', type: 'object', description: 'Custom field values keyed by field ID' },
  ];

  // Condition type options
  const conditionTypes = [
    { id: 'user_in_role', name: 'User in Role' },
    { id: 'user_in_group', name: 'User in Group' },
    { id: 'field_value', name: 'Field Value' },
    { id: 'script', name: 'Script' }
  ];

  // Split transitions: those with conditions vs without
  const transitionsWithConditions = $derived(
    formData.transition_conditions.map(tc => {
      const trans = transitions.find(tr => tr.id === tc.transition_id);
      return { ...tc, transition: trans };
    }).filter(tc => tc.transition)
  );

  const transitionsWithoutConditions = $derived(
    transitions
      .filter(tr => tr.from_status_id != null) // Exclude initial transitions
      .filter(tr => !formData.transition_conditions.some(tc => tc.transition_id === tr.id))
  );

  // Subscribe to route changes
  $effect(() => {
    const id = $currentRoute.params?.id;
    if (id === 'new') {
      isNewMode = true;
      conditionSetId = null;
      resetForm();
      loading = false;
    } else if (id) {
      const newId = parseInt(id);
      if (newId && newId !== conditionSetId) {
        isNewMode = false;
        conditionSetId = newId;
        loadData();
      }
    }
  });

  onMount(() => {
    loadReferenceData();
  });

  function resetForm() {
    const data = {
      name: '',
      description: '',
      workflow_id: null,
      transition_conditions: []
    };
    formData = data;
    originalFormData = JSON.stringify(data);
    transitions = [];
    expandedTransitions = new Set();
  }

  async function loadReferenceData() {
    try {
      const [workflowsData, groupsData, fieldsData] = await Promise.all([
        api.workflows.getAll(),
        api.groups.getAll(),
        api.customFields.getAll()
      ]);
      workflows = workflowsData || [];
      groups = groupsData || [];
      userFieldOptions = (fieldsData?.data || fieldsData || [])
        .filter(f => f.field_type === 'user')
        .map(f => ({ id: f.id, name: f.name }));
    } catch (error) {
      console.error('Failed to load reference data:', error);
    }
  }

  async function loadData() {
    if (!conditionSetId) {
      loading = false;
      return;
    }

    try {
      loading = true;
      const data = await api.conditionSets.get(conditionSetId);

      formData = {
        name: data.name || '',
        description: data.description || '',
        workflow_id: data.workflow_id || null,
        transition_conditions: (data.transition_conditions || []).map(tc => ({
          transition_id: tc.transition_id,
          logic_mode: tc.logic_mode || 'and',
          conditions: (tc.conditions || []).map(c => ({
            condition_type: c.condition_type,
            config: typeof c.config === 'string' ? JSON.parse(c.config) : c.config,
            display_order: c.display_order || 0,
            mode: c.mode || 'condition',
            error_message: c.error_message || ''
          }))
        }))
      };

      originalFormData = JSON.stringify(formData);

      // Load transitions for this workflow
      if (data.workflow_id) {
        await loadTransitions(data.workflow_id);
      }
    } catch (error) {
      console.error('Failed to load condition set:', error);
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

  async function handleWorkflowChange(workflow) {
    const newWorkflowId = workflow?.id || null;

    if (formData.workflow_id && newWorkflowId !== formData.workflow_id && formData.transition_conditions.length > 0) {
      const confirmed = await confirm({
        title: t('conditionSets.changeWorkflow'),
        message: t('conditionSets.changeWorkflowConfirm'),
        confirmText: t('common.confirm'),
        cancelText: t('common.cancel'),
        variant: 'warning'
      });
      if (!confirmed) return;
    }

    formData.workflow_id = newWorkflowId;
    formData.transition_conditions = [];
    expandedTransitions = new Set();

    if (newWorkflowId) {
      await loadTransitions(newWorkflowId);
    } else {
      transitions = [];
    }
  }

  function addConditionsToTransition(transitionId) {
    formData.transition_conditions = [
      ...formData.transition_conditions,
      {
        transition_id: transitionId,
        logic_mode: 'and',
        conditions: []
      }
    ];
    expandedTransitions = new Set([...expandedTransitions, transitionId]);
  }

  function removeTransitionConditions(transitionId) {
    formData.transition_conditions = formData.transition_conditions.filter(
      tc => tc.transition_id !== transitionId
    );
    const newExpanded = new Set(expandedTransitions);
    newExpanded.delete(transitionId);
    expandedTransitions = newExpanded;
  }

  function toggleExpand(transitionId) {
    const newExpanded = new Set(expandedTransitions);
    if (newExpanded.has(transitionId)) {
      newExpanded.delete(transitionId);
    } else {
      newExpanded.add(transitionId);
    }
    expandedTransitions = newExpanded;
  }

  function setLogicMode(transitionId, mode) {
    formData.transition_conditions = formData.transition_conditions.map(tc =>
      tc.transition_id === transitionId ? { ...tc, logic_mode: mode } : tc
    );
  }

  function addCondition(transitionId) {
    formData.transition_conditions = formData.transition_conditions.map(tc => {
      if (tc.transition_id !== transitionId) return tc;
      return {
        ...tc,
        conditions: [
          ...tc.conditions,
          {
            condition_type: 'user_in_role',
            config: { source: 'current_user', role_id: null },
            display_order: tc.conditions.length,
            mode: 'condition',
            error_message: ''
          }
        ]
      };
    });
  }

  function removeCondition(transitionId, condIndex) {
    formData.transition_conditions = formData.transition_conditions.map(tc => {
      if (tc.transition_id !== transitionId) return tc;
      return {
        ...tc,
        conditions: tc.conditions.filter((_, i) => i !== condIndex)
      };
    });
  }

  function updateConditionType(transitionId, condIndex, newType) {
    formData.transition_conditions = formData.transition_conditions.map(tc => {
      if (tc.transition_id !== transitionId) return tc;
      const newConditions = [...tc.conditions];
      let config = {};
      if (newType === 'user_in_role') config = { source: 'current_user', role_id: null };
      else if (newType === 'user_in_group') config = { source: 'current_user', group_id: null };
      else if (newType === 'field_value') config = { field_identifier: '', pattern: '' };
      else if (newType === 'script') config = { script: '', timeout_ms: 1000 };
      newConditions[condIndex] = { ...newConditions[condIndex], condition_type: newType, config, mode: newConditions[condIndex].mode || 'condition' };
      return { ...tc, conditions: newConditions };
    });
  }

  function updateConditionConfig(transitionId, condIndex, key, value) {
    formData.transition_conditions = formData.transition_conditions.map(tc => {
      if (tc.transition_id !== transitionId) return tc;
      const newConditions = [...tc.conditions];
      newConditions[condIndex] = {
        ...newConditions[condIndex],
        config: { ...newConditions[condIndex].config, [key]: value }
      };
      return { ...tc, conditions: newConditions };
    });
  }

  function updateConditionMode(transitionId, condIndex, mode) {
    formData.transition_conditions = formData.transition_conditions.map(tc => {
      if (tc.transition_id !== transitionId) return tc;
      const newConditions = [...tc.conditions];
      newConditions[condIndex] = {
        ...newConditions[condIndex],
        mode: mode
      };
      return { ...tc, conditions: newConditions };
    });
  }

  function updateConditionErrorMessage(transitionId, condIndex, value) {
    formData.transition_conditions = formData.transition_conditions.map(tc => {
      if (tc.transition_id !== transitionId) return tc;
      const newConditions = [...tc.conditions];
      newConditions[condIndex] = {
        ...newConditions[condIndex],
        error_message: value || ''
      };
      return { ...tc, conditions: newConditions };
    });
  }

  function getTransitionLabel(trans) {
    if (!trans) return '';
    const from = trans.from_status_name || 'Initial';
    return `${from} → ${trans.to_status_name}`;
  }

  function getConditionSummary(conditions) {
    if (!conditions || conditions.length === 0) return 'No conditions';
    return conditions.map(c => {
      const modePrefix = c.mode === 'validator' ? '[V] ' : '';
      switch (c.condition_type) {
        case 'user_in_role': {
          const src = c.config?.source || 'current_user';
          return `${modePrefix}${src.replace('_', ' ')} has role`;
        }
        case 'user_in_group': {
          const src = c.config?.source || 'current_user';
          const group = groups.find(g => g.id === c.config?.group_id);
          return `${modePrefix}${src.replace('_', ' ')} in group "${group?.name || '?'}"`;
        }
        case 'field_value': return `${modePrefix}Field "${c.config?.field_identifier || '?'}" matches pattern`;
        case 'script': return `${modePrefix}Script condition`;
        default: return `${modePrefix}${c.condition_type}`;
      }
    }).join(', ');
  }

  async function save() {
    if (!formData.name.trim()) {
      errorToast(t('conditionSets.nameRequired'));
      return;
    }
    if (!formData.workflow_id) {
      errorToast(t('conditionSets.workflowRequired'));
      return;
    }

    try {
      saving = true;

      const payload = JSON.parse(JSON.stringify({
        name: formData.name,
        description: formData.description,
        workflow_id: formData.workflow_id,
        transition_conditions: formData.transition_conditions
      }));

      if (isNewMode) {
        const created = await api.conditionSets.create(payload);
        conditionSetId = created.id;
        isNewMode = false;
        navigate(`/admin/condition-sets/${created.id}`, { replace: true });
      } else {
        await api.conditionSets.update(conditionSetId, payload);
      }

      originalFormData = JSON.stringify(formData);
    } catch (error) {
      console.error('Failed to save condition set:', error);
      errorToast(t('dialogs.alerts.failedToSave', { error: error.message || JSON.stringify(error) }));
    } finally {
      saving = false;
    }
  }

  function goBack() {
    navigate('/admin/condition-sets');
  }
</script>

<div class="flex flex-col h-full" style="background-color: var(--ds-surface);">
  <!-- Header -->
  <div class="border-b px-6 py-4" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-4">
        <button
          onclick={goBack}
          class="transition-colors"
          style="color: var(--ds-text-subtle);"
          title={t('conditionSets.backToList')}
        >
          <ArrowLeft class="w-5 h-5" />
        </button>
        <div>
          <h1 class="text-xl font-semibold" style="color: var(--ds-text);">
            {#if loading}
              {t('common.loading')}
            {:else if isNewMode}
              {t('conditionSets.new')}
            {:else}
              {formData.name || t('conditionSets.title')}
            {/if}
          </h1>
          <p class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">
            {t('conditionSets.detailDesc')}
          </p>
        </div>
      </div>
      <div class="flex items-center gap-3">
        {#if hasUnsavedChanges}
          <span class="text-sm" style="color: var(--ds-text-subtle);">{t('settings.configSets.unsavedChanges')}</span>
        {/if}
        <Button variant="ghost" onclick={goBack}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" onclick={save} disabled={saving || !hasUnsavedChanges}>
          {saving ? t('common.saving') : t('common.save')}
        </Button>
      </div>
    </div>
  </div>

  <!-- Content -->
  <div class="flex-1 overflow-y-auto p-6" style="background-color: var(--ds-surface);">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <div style="color: var(--ds-text-subtle);">{t('common.loading')}</div>
      </div>
    {:else}
      <div class="max-w-4xl mx-auto space-y-8">

        <!-- General Section -->
        <div class="rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
          <h3 class="text-base font-medium mb-4" style="color: var(--ds-text);">{t('conditionSets.general')}</h3>
          <div class="space-y-4">
            <div>
              <Label color="default" required class="mb-1">{t('conditionSets.name')}</Label>
              <input
                type="text"
                bind:value={formData.name}
                class="w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
                placeholder={t('conditionSets.namePlaceholder')}
              />
            </div>
            <div>
              <Label color="default" class="mb-1">{t('conditionSets.description')}</Label>
              <Textarea
                bind:value={formData.description}
                rows={2}
                placeholder={t('conditionSets.descriptionPlaceholder')}
              />
            </div>
            <div>
              <Label color="default" required class="mb-1">{t('conditionSets.workflow')}</Label>
              <WorkflowPicker
                value={formData.workflow_id}
                items={workflows}
                placeholder={t('conditionSets.selectWorkflow')}
                disabled={!isNewMode}
                onSelect={handleWorkflowChange}
              />
              {#if !isNewMode}
                <DescriptionText>{t('conditionSets.workflowLocked')}</DescriptionText>
              {/if}
            </div>
          </div>
        </div>

        <!-- Transition Conditions Section -->
        {#if formData.workflow_id && transitions.length > 0}
          <div class="rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
            <h3 class="text-base font-medium mb-2" style="color: var(--ds-text);">{t('conditionSets.transitionConditions')}</h3>
            <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('conditionSets.transitionConditionsDesc')}</p>

            <!-- Transitions with conditions -->
            {#if transitionsWithConditions.length > 0}
              <div class="space-y-3 mb-6">
                {#each transitionsWithConditions as tc (tc.transition_id)}
                  {@const isExpanded = expandedTransitions.has(tc.transition_id)}
                  <div class="border rounded-lg" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                    <!-- Transition header -->
                    <div
                      class="flex items-center justify-between px-4 py-3 cursor-pointer"
                      onclick={() => toggleExpand(tc.transition_id)}
                      onkeydown={(e) => e.key === 'Enter' && toggleExpand(tc.transition_id)}
                      role="button"
                      tabindex="0"
                    >
                      <div class="flex items-center gap-3">
                        {#if isExpanded}
                          <ChevronUp class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                        {:else}
                          <ChevronDown class="w-4 h-4" style="color: var(--ds-icon-subtle);" />
                        {/if}
                        <span class="font-medium text-sm" style="color: var(--ds-text);">
                          {getTransitionLabel(tc.transition)}
                        </span>
                        <Lozenge color={tc.logic_mode === 'and' ? 'blue' : 'purple'} text={tc.logic_mode.toUpperCase()} />
                        <span class="text-xs" style="color: var(--ds-text-subtle);">
                          {tc.conditions.length} condition{tc.conditions.length !== 1 ? 's' : ''}
                        </span>
                      </div>
                      <div class="flex items-center gap-2" onclick={(e) => e.stopPropagation()}>
                        <button
                          class="p-1.5 rounded transition-colors"
                          style="color: var(--ds-text-subtle);"
                          onclick={() => removeTransitionConditions(tc.transition_id)}
                          title={t('conditionSets.removeConditions')}
                          onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text-danger)'}
                          onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
                        >
                          <Trash2 class="w-4 h-4" />
                        </button>
                      </div>
                    </div>

                    <!-- Expanded condition editor -->
                    {#if isExpanded}
                      <div class="border-t px-4 py-4 space-y-4" style="border-color: var(--ds-border);">
                        <!-- Override warning: fires when an approval set drives this
                             transition. Conditions still run for direct user attempts
                             (which approvals block anyway), but they're bypassed when
                             the approval engine fires the transition. -->
                        <TransitionOverrideWarning
                          transitionId={tc.transition_id}
                          perspective="condition"
                        />

                        <!-- Logic mode toggle -->
                        <div class="flex items-center gap-2">
                          <span class="text-sm" style="color: var(--ds-text-subtle);">{t('conditionSets.logicMode')}:</span>
                          <div class="flex rounded border" style="border-color: var(--ds-border);">
                            <button
                              class="px-3 py-1 text-xs font-medium transition-colors"
                              style={tc.logic_mode === 'and'
                                ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
                                : 'color: var(--ds-text-subtle);'}
                              onclick={() => setLogicMode(tc.transition_id, 'and')}
                            >
                              AND
                            </button>
                            <button
                              class="px-3 py-1 text-xs font-medium border-l transition-colors"
                              style={tc.logic_mode === 'or'
                                ? 'background-color: var(--ds-background-selected); color: var(--ds-text); border-color: var(--ds-border);'
                                : 'color: var(--ds-text-subtle); border-color: var(--ds-border);'}
                              onclick={() => setLogicMode(tc.transition_id, 'or')}
                            >
                              OR
                            </button>
                          </div>
                        </div>

                        <!-- Conditions list -->
                        {#each tc.conditions as cond, condIdx}
                          <div class="border rounded-md p-3 space-y-3" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
                            <div class="flex items-center justify-between">
                              <div class="flex items-center gap-3 flex-1">
                                <Label color="default" class="text-xs whitespace-nowrap">{t('conditionSets.type')}</Label>
                                <select
                                  class="px-2 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                                  style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                                  value={cond.condition_type}
                                  onchange={(e) => updateConditionType(tc.transition_id, condIdx, e.currentTarget.value)}
                                >
                                  {#each conditionTypes as ct}
                                    <option value={ct.id}>{ct.name}</option>
                                  {/each}
                                </select>
                              </div>
                              <button
                                class="p-1 rounded transition-colors"
                                style="color: var(--ds-text-subtle);"
                                onclick={() => removeCondition(tc.transition_id, condIdx)}
                                title={t('conditionSets.removeCondition')}
                                onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text-danger)'}
                                onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
                              >
                                <Trash2 class="w-3.5 h-3.5" />
                              </button>
                            </div>

                            <!-- Mode toggle -->
                            <div class="flex items-center gap-2">
                              <span class="text-xs" style="color: var(--ds-text-subtle);">{t('conditionSets.conditionMode')}:</span>
                              <div class="flex rounded border" style="border-color: var(--ds-border);">
                                <button
                                  class="px-3 py-1 text-xs font-medium transition-colors"
                                  style={(cond.mode || 'condition') === 'condition'
                                    ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
                                    : 'color: var(--ds-text-subtle);'}
                                  onclick={() => updateConditionMode(tc.transition_id, condIdx, 'condition')}
                                  title={t('conditionSets.modeConditionDesc')}
                                >
                                  {t('conditionSets.modeCondition')}
                                </button>
                                <button
                                  class="px-3 py-1 text-xs font-medium border-l transition-colors"
                                  style={(cond.mode || 'condition') === 'validator'
                                    ? 'background-color: var(--ds-background-selected); color: var(--ds-text); border-color: var(--ds-border);'
                                    : 'color: var(--ds-text-subtle); border-color: var(--ds-border);'}
                                  onclick={() => updateConditionMode(tc.transition_id, condIdx, 'validator')}
                                  title={t('conditionSets.modeValidatorDesc')}
                                >
                                  {t('conditionSets.modeValidator')}
                                </button>
                              </div>
                              <span class="text-xs" style="color: var(--ds-text-subtlest);">
                                {(cond.mode || 'condition') === 'validator' ? t('conditionSets.modeValidatorDesc') : t('conditionSets.modeConditionDesc')}
                              </span>
                            </div>

                            <!-- Type-specific config -->
                            {#if cond.condition_type === 'user_in_role'}
                              <div class="grid grid-cols-2 gap-3">
                                <div>
                                  <Label color="default" class="text-xs mb-1">{t('conditionSets.userEvaluated')}</Label>
                                  <select
                                    class="w-full px-2 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                                    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                                    value={cond.config?.source || 'current_user'}
                                    onchange={(e) => {
                                      updateConditionConfig(tc.transition_id, condIdx, 'source', e.currentTarget.value);
                                      if (e.currentTarget.value !== 'custom_field') {
                                        updateConditionConfig(tc.transition_id, condIdx, 'field_id', null);
                                      }
                                    }}
                                  >
                                    {#each userSourceOptions as opt}
                                      <option value={opt.id}>{opt.name}</option>
                                    {/each}
                                  </select>
                                </div>
                                <div>
                                  <Label color="default" class="text-xs mb-1">{t('conditionSets.againstRole')}</Label>
                                  <RolePicker
                                    value={cond.config?.role_id}
                                    placeholder={t('conditionSets.selectRole')}
                                    onSelect={(role) => updateConditionConfig(tc.transition_id, condIdx, 'role_id', role?.id || null)}
                                  />
                                </div>
                              </div>
                              {#if cond.config?.source === 'custom_field'}
                                <div>
                                  <Label color="default" class="text-xs mb-1">{t('conditionSets.userField')}</Label>
                                  <BasePicker
                                    value={cond.config?.field_id}
                                    items={userFieldOptions}
                                    placeholder={t('conditionSets.selectUserField')}
                                    getValue={(f) => f.id}
                                    getLabel={(f) => f.name}
                                    onSelect={(f) => updateConditionConfig(tc.transition_id, condIdx, 'field_id', f?.id || null)}
                                  />
                                </div>
                              {/if}

                            {:else if cond.condition_type === 'user_in_group'}
                              <div class="grid grid-cols-2 gap-3">
                                <div>
                                  <Label color="default" class="text-xs mb-1">{t('conditionSets.userEvaluated')}</Label>
                                  <select
                                    class="w-full px-2 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                                    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                                    value={cond.config?.source || 'current_user'}
                                    onchange={(e) => {
                                      updateConditionConfig(tc.transition_id, condIdx, 'source', e.currentTarget.value);
                                      if (e.currentTarget.value !== 'custom_field') {
                                        updateConditionConfig(tc.transition_id, condIdx, 'field_id', null);
                                      }
                                    }}
                                  >
                                    {#each userSourceOptions as opt}
                                      <option value={opt.id}>{opt.name}</option>
                                    {/each}
                                  </select>
                                </div>
                                <div>
                                  <Label color="default" class="text-xs mb-1">{t('conditionSets.againstGroup')}</Label>
                                  <BasePicker
                                    value={cond.config?.group_id}
                                    items={groups}
                                    placeholder={t('conditionSets.selectGroup')}
                                    getValue={(g) => g.id}
                                    getLabel={(g) => g.name}
                                    onSelect={(g) => updateConditionConfig(tc.transition_id, condIdx, 'group_id', g?.id || null)}
                                  />
                                </div>
                              </div>
                              {#if cond.config?.source === 'custom_field'}
                                <div>
                                  <Label color="default" class="text-xs mb-1">{t('conditionSets.userField')}</Label>
                                  <BasePicker
                                    value={cond.config?.field_id}
                                    items={userFieldOptions}
                                    placeholder={t('conditionSets.selectUserField')}
                                    getValue={(f) => f.id}
                                    getLabel={(f) => f.name}
                                    onSelect={(f) => updateConditionConfig(tc.transition_id, condIdx, 'field_id', f?.id || null)}
                                  />
                                </div>
                              {/if}

                            {:else if cond.condition_type === 'field_value'}
                              <div class="grid grid-cols-2 gap-3">
                                <div>
                                  <Label color="default" class="text-xs mb-1">{t('conditionSets.fieldIdentifier')}</Label>
                                  <input
                                    type="text"
                                    class="w-full px-2 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                                    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                                    placeholder="e.g. priority, custom_field_name"
                                    value={cond.config?.field_identifier || ''}
                                    oninput={(e) => updateConditionConfig(tc.transition_id, condIdx, 'field_identifier', e.currentTarget.value)}
                                  />
                                </div>
                                <div>
                                  <Label color="default" class="text-xs mb-1">{t('conditionSets.pattern')}</Label>
                                  <input
                                    type="text"
                                    class="w-full px-2 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono"
                                    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                                    placeholder="e.g. ^(high|critical)$"
                                    value={cond.config?.pattern || ''}
                                    oninput={(e) => updateConditionConfig(tc.transition_id, condIdx, 'pattern', e.currentTarget.value)}
                                  />
                                </div>
                              </div>

                            {:else if cond.condition_type === 'script'}
                              <div>
                                <div class="flex items-center gap-1.5 mb-1">
                                  <Label color="default" class="text-xs">{t('conditionSets.script')}</Label>
                                  <button
                                    type="button"
                                    class="p-0.5 rounded transition-colors"
                                    style="color: var(--ds-text-subtle);"
                                    onclick={() => showScriptHelp = true}
                                    title={t('conditionSets.scriptReference')}
                                    onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text)'}
                                    onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
                                  >
                                    <HelpCircle class="w-3.5 h-3.5" />
                                  </button>
                                </div>
                                <textarea
                                  class="w-full px-3 py-2 text-sm border rounded font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                                  style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                                  rows="4"
                                  placeholder="// Return true to allow transition&#10;// Available: item, user_id&#10;item.title !== ''"
                                  value={cond.config?.script || ''}
                                  oninput={(e) => updateConditionConfig(tc.transition_id, condIdx, 'script', e.currentTarget.value)}
                                ></textarea>
                                <DescriptionText>
                                  {t('conditionSets.scriptHelp')}
                                </DescriptionText>
                              </div>
                            {/if}

                            <!-- Error message (validator mode only) -->
                            {#if (cond.mode || 'condition') === 'validator'}
                              <div>
                                <Label color="default" class="text-xs mb-1">{t('conditionSets.errorMessage')}</Label>
                                <input
                                  type="text"
                                  class="w-full px-2 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                                  style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
                                  placeholder={t('conditionSets.errorMessagePlaceholder')}
                                  value={cond.error_message || ''}
                                  oninput={(e) => updateConditionErrorMessage(tc.transition_id, condIdx, e.currentTarget.value)}
                                />
                                <DescriptionText>
                                  {t('conditionSets.errorMessageHelp')}
                                </DescriptionText>
                              </div>
                            {/if}
                          </div>
                        {/each}

                        <!-- Add condition button -->
                        <Button
                          variant="ghost"
                          size="small"
                          icon={Plus}
                          onclick={() => addCondition(tc.transition_id)}
                        >
                          {t('conditionSets.addCondition')}
                        </Button>
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}

            <!-- Transitions without conditions -->
            {#if transitionsWithoutConditions.length > 0}
              <div>
                <h4 class="text-sm font-medium mb-2" style="color: var(--ds-text-subtle);">
                  {t('conditionSets.unconditionedTransitions')}
                </h4>
                <div class="border rounded-lg divide-y" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                  {#each transitionsWithoutConditions as trans (trans.id)}
                    <div class="flex items-center justify-between px-4 py-2.5">
                      <span class="text-sm" style="color: var(--ds-text);">
                        {getTransitionLabel(trans)}
                      </span>
                      <Button
                        variant="ghost"
                        size="small"
                        icon={Plus}
                        onclick={() => addConditionsToTransition(trans.id)}
                      >
                        {t('conditionSets.addConditions')}
                      </Button>
                    </div>
                  {/each}
                </div>
              </div>
            {/if}

            {#if transitions.filter(tr => tr.from_status_id != null).length === 0}
              <div class="text-sm py-4 text-center" style="color: var(--ds-text-subtle);">
                {t('conditionSets.noTransitions')}
              </div>
            {/if}
          </div>
        {:else if formData.workflow_id && transitions.length === 0 && !loading}
          <div class="rounded-lg border p-6" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
            <p class="text-sm" style="color: var(--ds-text-subtle);">{t('conditionSets.noTransitions')}</p>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

<!-- Script Reference Modal -->
<Modal bind:isOpen={showScriptHelp} maxWidth="max-w-md">
  <div class="p-5">
    <h2 class="text-base font-semibold mb-1" style="color: var(--ds-text);">{t('conditionSets.scriptReference')}</h2>
    <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('conditionSets.scriptReferenceDesc')}</p>

    <h3 class="text-xs font-semibold uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">{t('conditionSets.scriptRefGlobals')}</h3>
    <table class="w-full text-sm mb-4" style="color: var(--ds-text);">
      <thead>
        <tr class="border-b" style="border-color: var(--ds-border);">
          <th class="text-left py-1.5 pr-3 text-xs font-medium" style="color: var(--ds-text-subtle);">{t('conditionSets.scriptRefVariable')}</th>
          <th class="text-left py-1.5 pr-3 text-xs font-medium" style="color: var(--ds-text-subtle);">{t('conditionSets.scriptRefType')}</th>
          <th class="text-left py-1.5 text-xs font-medium" style="color: var(--ds-text-subtle);">{t('conditionSets.scriptRefDescription')}</th>
        </tr>
      </thead>
      <tbody>
        {#each scriptRefGlobals as ref, i}
          <tr class={i < scriptRefGlobals.length - 1 ? 'border-b' : ''} style="border-color: var(--ds-border);">
            <td class="py-1.5 pr-3 font-mono text-xs">{ref.variable}</td>
            <td class="py-1.5 pr-3 text-xs">{ref.type}</td>
            <td class="py-1.5 text-xs">{ref.description}</td>
          </tr>
        {/each}
      </tbody>
    </table>

    <h3 class="text-xs font-semibold uppercase tracking-wide mb-2" style="color: var(--ds-text-subtle);">{t('conditionSets.scriptRefItemProps')}</h3>
    <table class="w-full text-sm" style="color: var(--ds-text);">
      <thead>
        <tr class="border-b" style="border-color: var(--ds-border);">
          <th class="text-left py-1.5 pr-3 text-xs font-medium" style="color: var(--ds-text-subtle);">{t('conditionSets.scriptRefVariable')}</th>
          <th class="text-left py-1.5 pr-3 text-xs font-medium" style="color: var(--ds-text-subtle);">{t('conditionSets.scriptRefType')}</th>
          <th class="text-left py-1.5 text-xs font-medium" style="color: var(--ds-text-subtle);">{t('conditionSets.scriptRefDescription')}</th>
        </tr>
      </thead>
      <tbody>
        {#each scriptRefItemProps as ref, i}
          <tr class={i < scriptRefItemProps.length - 1 ? 'border-b' : ''} style="border-color: var(--ds-border);">
            <td class="py-1.5 pr-3 font-mono text-xs">{ref.variable}</td>
            <td class="py-1.5 pr-3 text-xs">{ref.type}</td>
            <td class="py-1.5 text-xs">{ref.description}</td>
          </tr>
        {/each}
      </tbody>
    </table>

    <div class="mt-4 flex justify-end">
      <Button variant="ghost" onclick={() => showScriptHelp = false}>{t('common.close')}</Button>
    </div>
  </div>
</Modal>
