<script>
  import { onMount } from 'svelte';
  import { currentRoute, navigate } from '../../router.js';
  import { statusCategories as statusCategoriesApi, statuses as statusesApi, workflows } from '../../api/workflows.js';
  import Save from '@lucide/svelte/icons/save';
  import X from '@lucide/svelte/icons/x';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import BackButton from '../../components/BackButton.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';

  let workflow = $state(null);
  let statuses = $state([]);
  let statusCategories = $state([]);
  let loading = $state(true);
  let loadingFlow = $state(false);
  let SvelteFlowDesigner = $state(null);

  // Get workflow ID from route params
  let workflowId = $derived($currentRoute.params?.id);

  onMount(async () => {
    if (workflowId) {
      await Promise.all([loadWorkflowData(), loadSvelteFlowDesigner()]);
    }
  });

  async function loadWorkflowData() {
    try {
      loading = true;
      
      // Load workflow, statuses, and status categories in parallel
      const [workflowData, statusesData, categoriesData] = await Promise.all([
        workflows.get(workflowId),
        statusesApi.getAll(),
        statusCategoriesApi.getAll()
      ]);
      
      workflow = workflowData;
      statuses = statusesData || [];
      statusCategories = categoriesData || [];
      
    } catch (error) {
      console.error('Failed to load workflow data:', error);
    } finally {
      loading = false;
    }
  }

  async function loadSvelteFlowDesigner() {
    if (SvelteFlowDesigner) return;
    
    try {
      loadingFlow = true;
      const module = await import('./SvelteFlowDesigner.svelte');
      SvelteFlowDesigner = module.default;
    } catch (error) {
      console.error('Failed to load Svelte Flow designer:', error);
      errorToast(t('workflows.failedToLoadDesigner'));
    } finally {
      loadingFlow = false;
    }
  }

  async function handleSave(allTransitions) {
    try {
      await workflows.updateTransitions(workflow.id, allTransitions);
      navigate('/admin/workflows');
    } catch (error) {
      throw error;
    }
  }

  function handleCancel() {
    navigate('/admin/workflows');
  }

</script>

<div class="min-h-screen workflow-theme">
  <!-- Header -->
  <div class="workflow-header border-b shadow-sm">
    <div class="max-w-full px-6 py-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <BackButton
            onclick={() => navigate('/admin/workflows')}
            label={t('workflows.backToWorkflows')}
          />
          <div>
            <h1 class="text-xl font-medium" style="color: var(--ds-text);">
              {workflow?.name || t('nav.loading')} - {t('workflows.workflowDesigner')}
            </h1>
            <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
              {t('workflows.designerHint')}
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>

  {#if loading}
    <div class="flex items-center justify-center h-64">
      <div class="animate-pulse loading-message">{t('workflows.loadingWorkflowData')}</div>
    </div>
  {:else if loadingFlow}
    <div class="flex items-center justify-center h-64">
      <div class="animate-pulse loading-message">{t('workflows.loadingDesigner')}</div>
    </div>
  {:else if SvelteFlowDesigner && workflow && statuses.length > 0}
    <div class="h-[calc(100vh-120px)]">
      <SvelteFlowDesigner
        {workflow}
        {statuses}
        {statusCategories}
        onSave={handleSave}
        onCancel={handleCancel}
      />
    </div>
  {:else}
    <div class="flex items-center justify-center h-64">
      <div class="text-center loading-message">
        <p class="text-lg mb-2">{t('workflows.failedToLoadDesignerTitle')}</p>
        <p class="text-sm">{t('workflows.refreshAndTryAgain')}</p>
      </div>
    </div>
  {/if}
</div>

<style>
  .workflow-header {
    background-color: var(--workflow-panel);
    border-color: var(--workflow-border);
  }

  .workflow-theme {
    background-color: var(--workflow-surface);
    color: var(--workflow-text);
  }

  .loading-message {
    color: var(--workflow-text-subtle);
  }
</style>
